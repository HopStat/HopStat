package engine

import (
	"context"
	"errors"
	"log/slog"
	"net"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/HopStat/HopStat/internal/bgp"
	"github.com/HopStat/HopStat/internal/circuitbreaker"
	"github.com/HopStat/HopStat/internal/config"
	"github.com/HopStat/HopStat/internal/domain"
	"github.com/HopStat/HopStat/internal/driver"
	"github.com/HopStat/HopStat/internal/geo"
)

var lookupASForPath = func(g *geo.GeoIPDB, ctx context.Context, asn uint32) (*domain.ASInfo, error) {
	if g == nil {
		return nil, nil
	}
	return g.LookupASByNumber(ctx, asn)
}

type SettingsProvider interface {
	GetSettings(ctx context.Context) (map[string]string, error)
}

type QueryEngine struct {
	pool          *QueryPool
	cfg           *QueryConfig
	nodeRepo      domain.NodeRepository
	communityRepo domain.CommunityRuleRepository
	settings      SettingsProvider
	bgpLocalAS    uint32
	rateLimit     *RateLimiter
	geoDB         *geo.GeoIPDB
	bgpMgr        *bgp.SessionManager
}

type QueryConfig struct {
	MaxConcurrent        int
	DefaultTimeoutSec    int
	TracerouteTimeoutSec int
	ServerPort           int
	FloodControlEnabled  bool
	QueryRateLimitPerMin int
}

func New(cfg *QueryConfig, nodeRepo domain.NodeRepository, communityRepo domain.CommunityRuleRepository, geoDB *geo.GeoIPDB, bgpMgr *bgp.SessionManager, settings SettingsProvider, bgpLocalAS uint32) *QueryEngine {
	e := &QueryEngine{
		pool:          NewQueryPool(cfg.MaxConcurrent),
		cfg:           cfg,
		nodeRepo:      nodeRepo,
		communityRepo: communityRepo,
		settings:      settings,
		bgpLocalAS:    bgpLocalAS,
		geoDB:         geoDB,
		bgpMgr:        bgpMgr,
	}
	if cfg.FloodControlEnabled && cfg.QueryRateLimitPerMin > 0 {
		e.rateLimit = NewRateLimiter(cfg.QueryRateLimitPerMin, time.Minute)
	}
	return e
}

func (e *QueryEngine) driverConfig() *config.Config {
	if e.cfg.ServerPort <= 0 {
		return nil
	}
	return &config.Config{Server: config.ServerConfig{Port: e.cfg.ServerPort}}
}

func emitRawLines(raw string, onLine func(string)) {
	for _, line := range strings.Split(raw, "\n") {
		line = strings.TrimRight(line, "\r")
		if line != "" {
			onLine(line)
		}
	}
}

type ExecuteOption struct {
	OnLine     func(string)
	OnPartial  func(*domain.QueryResult)
	ShouldStop func() bool
}

func (e *QueryEngine) Execute(ctx context.Context, query *domain.Query, opts ...ExecuteOption) (*domain.QueryResult, error) {
	var opt ExecuteOption
	if len(opts) > 0 {
		opt = opts[0]
	}

	var linesStreamed bool
	if opt.OnLine != nil {
		orig := opt.OnLine
		opt.OnLine = func(line string) {
			linesStreamed = true
			orig(line)
		}
	}

	if e.rateLimit != nil && !e.rateLimit.Allow(query.SourceIP) {
		return &domain.QueryResult{
			ID:        query.ID,
			Status:    domain.StatusError,
			ErrorCode: "RATE_LIMITED",
			ErrorMsg:  "rate limit exceeded",
		}, domain.ErrRateLimited
	}

	node, err := e.nodeRepo.GetByID(ctx, query.NodeID)
	if err != nil {
		return &domain.QueryResult{
			ID:        query.ID,
			Status:    domain.StatusError,
			ErrorCode: "NODE_NOT_FOUND",
			ErrorMsg:  err.Error(),
		}, err
	}

	if !node.CanExecute(query.Command) {
		return &domain.QueryResult{
			ID:        query.ID,
			Status:    domain.StatusError,
			ErrorCode: "COMMAND_DISABLED",
			ErrorMsg:  "command not enabled for this node",
		}, domain.ErrCommandDisabled
	}

	drv, err := driver.NewDriver(node, e.driverConfig())
	if err != nil {
		return &domain.QueryResult{
			ID:        query.ID,
			Status:    domain.StatusError,
			ErrorCode: "DRIVER_ERROR",
			ErrorMsg:  err.Error(),
		}, err
	}

	timeout := time.Duration(e.cfg.DefaultTimeoutSec) * time.Second
	switch query.Command {
	case domain.CmdTraceroute:
		timeout = time.Duration(e.cfg.TracerouteTimeoutSec) * time.Second
	}

	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	if opt.OnLine != nil {
		ctx = domain.WithOnLine(ctx, opt.OnLine)
	}

	// Early stop: cancel context when ShouldStop returns true
	var earlyStop atomic.Bool
	if opt.ShouldStop != nil {
		stopCtx, stopCancel := context.WithCancelCause(ctx)
		ctx = stopCtx
		stopDone := make(chan struct{})
		go func() {
			defer close(stopDone)
			ticker := time.NewTicker(100 * time.Millisecond)
			defer ticker.Stop()
			for {
				select {
				case <-ticker.C:
					if opt.ShouldStop() {
						earlyStop.Store(true)
						stopCancel(domain.ErrEarlyStop)
						return
					}
				case <-ctx.Done():
					return
				}
			}
		}()
		defer func() { <-stopDone }()
	}

	result := &domain.QueryResult{ID: query.ID, Status: domain.StatusRunning}
	start := time.Now()

	if query.Command == domain.CmdPing || query.Command == domain.CmdTraceroute {
		go e.prefetchBGPASPath(context.WithoutCancel(ctx), drv, query.Target, query.NodeID, node.Type, opt)
	}

	err = e.pool.Execute(ctx, func() error {
		switch query.Command {
		case domain.CmdPing:
			pr, err := drv.Ping(ctx, query.Target, query.Options.PingCount)
			if err != nil {
				return err
			}
			result.Parsed = pr
			result.Raw = pr.Raw

		case domain.CmdTraceroute:
			tr, err := drv.Traceroute(ctx, query.Target, query.Options.MaxHops)
			if err != nil {
				return err
			}
			e.enrichHops(ctx, tr.Hops)
			result.Parsed = tr
			result.Raw = tr.Raw

		case domain.CmdBGPRoute:
			br, err := e.lookupBGPRoutes(ctx, drv, query.Target, query.NodeID, node.Type)
			if err != nil {
				return err
			}
			e.attachRouteNodeNames(ctx, br, query.NodeID)
			if !bgp.HasRouteASPath(br) {
				bgp.EnrichResultTargetAS(ctx, e.geoDB, br, query.Target)
			}
			result.Parsed = br
			result.Raw = br.Raw
			e.matchCommunities(ctx, query.NodeID, br, result)
			if opt.OnPartial != nil {
				opt.OnPartial(result)
			}
			if opt.OnLine != nil && result.Raw != "" {
				emitRawLines(result.Raw, opt.OnLine)
			}
			e.applyBGPASPath(ctx, br, query.Target, result, opt)
		}
		return nil
	})

	if err == nil && opt.OnLine != nil && result.Raw != "" && !linesStreamed && query.Command != domain.CmdBGPRoute {
		emitRawLines(result.Raw, opt.OnLine)
	}

	result.DurationMS = time.Since(start).Milliseconds()

	if err != nil && earlyStop.Load() {
		err = nil
	}

	if err != nil {
		result.Status = domain.StatusError
		result.ErrorCode = ClassifyError(err)
		result.ErrorMsg = SanitizeErrorMsg(err)
	} else {
		result.Status = domain.StatusDone
		result.ErrorCode = ""
		result.ErrorMsg = ""
	}

	slog.Info("query executed",
		"query_id", query.ID,
		"node_id", query.NodeID,
		"command", query.Command,
		"target", query.Target,
		"duration_ms", result.DurationMS,
		"error", result.ErrorMsg,
	)

	return result, nil
}

func (e *QueryEngine) enrichHops(ctx context.Context, hops []domain.Hop) {
	if e.geoDB == nil || len(hops) == 0 {
		return
	}
	const maxWorkers = 8
	sem := make(chan struct{}, maxWorkers)
	var wg sync.WaitGroup
	for i := range hops {
		if net.ParseIP(hops[i].IP) == nil {
			continue
		}
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			info, err := e.geoDB.ResolveASN(ctx, hops[idx].IP)
			if err == nil && info != nil && info.ASN > 0 {
				hops[idx].ASInfo = info
			}
		}(i)
	}
	wg.Wait()
}

func (e *QueryEngine) matchCommunities(ctx context.Context, nodeID int64, br *domain.BGPResult, result *domain.QueryResult) {
	if e.communityRepo == nil || br == nil {
		return
	}
	rules, err := e.communityRepo.GetActiveRulesForNode(ctx, nodeID)
	if err != nil || len(rules) == 0 {
		return
	}
	seen := map[int64]bool{}
	var allMatched []*domain.CommunityRule
	for i := range br.Routes {
		matched := domain.MatchCommunityRules(rules, br.Routes[i].Communities)
		br.Routes[i].MatchedRules = matched
		for _, rule := range matched {
			if rule == nil || seen[rule.ID] {
				continue
			}
			seen[rule.ID] = true
			allMatched = append(allMatched, rule)
		}
	}
	result.MatchedRules = allMatched
}

func (e *QueryEngine) lookupBGPRoutes(ctx context.Context, drv driver.NodeDriver, target string, queryNodeID int64, nodeType domain.NodeType) (*domain.BGPResult, error) {
	var br *domain.BGPResult
	var err error
	if e.bgpMgr != nil && e.bgpMgr.IsReady() && queryNodeID > 0 && e.bgpMgr.HasNeighbors(queryNodeID) {
		br, err = e.bgpMgr.BuildRouteResult(ctx, queryNodeID, target, e.nodeNameForNeighborIP(ctx))
	} else {
		br, err = drv.BGPRoute(ctx, target)
		if err != nil {
			return nil, err
		}
		bgp.EnsureBestAmongRoutes(br.Routes)
		bgp.SortRoutesBestFirst(br.Routes)
	}
	if err != nil {
		return nil, err
	}
	if nodeType != domain.NodeTypeLGNode {
		bgp.ApplyOriginASPathToRoutes(br.Routes, e.localAS())
	}
	return br, nil
}

func (e *QueryEngine) nodeName(ctx context.Context, nodeID int64) string {
	if e.nodeRepo == nil || nodeID == 0 {
		return ""
	}
	node, err := e.nodeRepo.GetByID(ctx, nodeID)
	if err != nil || node == nil {
		return ""
	}
	return node.Name
}

func (e *QueryEngine) nodeNameForNeighborIP(ctx context.Context) func(string) string {
	cache := map[int64]string{}
	return func(neighborIP string) string {
		if e.bgpMgr == nil {
			return ""
		}
		nodeID, ok := e.bgpMgr.NodeIDForNeighborIP(neighborIP)
		if !ok {
			return ""
		}
		if name, ok := cache[nodeID]; ok {
			return name
		}
		name := e.nodeName(ctx, nodeID)
		cache[nodeID] = name
		return name
	}
}

func (e *QueryEngine) attachRouteNodeNames(ctx context.Context, br *domain.BGPResult, queryNodeID int64) {
	if br == nil || len(br.Routes) == 0 {
		return
	}
	fallback := e.nodeName(ctx, queryNodeID)
	for i := range br.Routes {
		if br.Routes[i].NodeName == "" {
			br.Routes[i].NodeName = fallback
		}
	}
}

func (e *QueryEngine) localAS() uint32 {
	if e.bgpMgr != nil {
		if v := e.bgpMgr.LocalAS(); v > 0 {
			return v
		}
	}
	return e.bgpLocalAS
}

func (e *QueryEngine) prefetchBGPASPath(ctx context.Context, drv driver.NodeDriver, target string, queryNodeID int64, nodeType domain.NodeType, opt ExecuteOption) {
	partial := &domain.QueryResult{}
	br, err := e.lookupBGPRoutes(ctx, drv, target, queryNodeID, nodeType)
	if err != nil {
		return
	}
	e.applyBGPASPath(ctx, br, target, partial, opt)
}

func (e *QueryEngine) applyBGPASPath(ctx context.Context, br *domain.BGPResult, target string, result *domain.QueryResult, opt ExecuteOption) {
	if br == nil {
		return
	}

	if bgp.HasRouteASPath(br) {
		route := bgp.BestRoute(br.Routes)
		result.ASPathPrefix = route.Prefix
		result.ASPath = append([]uint32(nil), route.ASPath...)
		e.emitASPathPartial(result, opt)
		e.enrichASPath(ctx, br, result, opt)
		return
	}

	if br.TargetAS == nil {
		bgp.EnrichResultTargetAS(ctx, e.geoDB, br, target)
	}
	if br.TargetAS == nil || br.TargetAS.ASN == 0 {
		return
	}
	result.ASPath = []uint32{br.TargetAS.ASN}
	result.ASPathPrefix = strings.TrimSpace(target)
	e.emitASPathPartial(result, opt)
	e.ensureTargetASInResult(br, result, opt)
}

func (e *QueryEngine) ensureTargetASInResult(br *domain.BGPResult, result *domain.QueryResult, opt ExecuteOption) {
	if br == nil || br.TargetAS == nil || br.TargetAS.ASN == 0 {
		return
	}
	targetAS := *br.TargetAS
	for _, entry := range result.ASPathEnriched {
		if entry.ASN == targetAS.ASN {
			return
		}
	}
	result.ASPathEnriched = append(result.ASPathEnriched, targetAS)
	e.emitASPathPartial(result, opt)
}

func (e *QueryEngine) emitASPathPartial(result *domain.QueryResult, opt ExecuteOption) {
	if opt.OnPartial == nil || len(result.ASPath) == 0 {
		return
	}
	opt.OnPartial(&domain.QueryResult{
		ASPath:         append([]uint32(nil), result.ASPath...),
		ASPathPrefix:   result.ASPathPrefix,
		ASPathEnriched: append([]domain.ASInfo(nil), result.ASPathEnriched...),
	})
}

func (e *QueryEngine) enrichASPath(ctx context.Context, br *domain.BGPResult, result *domain.QueryResult, opt ExecuteOption) {
	if br == nil || len(br.Routes) == 0 {
		return
	}
	if len(result.ASPath) == 0 {
		if route := bgp.BestRoute(br.Routes); route != nil {
			result.ASPath = append([]uint32(nil), route.ASPath...)
		}
	}
	if result.ASPathPrefix == "" {
		if route := bgp.BestRoute(br.Routes); route != nil {
			result.ASPathPrefix = route.Prefix
		}
	}
	if e.geoDB == nil {
		e.emitASPathPartial(result, opt)
		return
	}
	result.ASPathEnriched = result.ASPathEnriched[:0]
	for _, asn := range collectBGPASPathASNs(result, br) {
		info, err := lookupASForPath(e.geoDB, ctx, asn)
		if err == nil && info != nil && (info.OrgName != "" || info.ShortName != "" || info.CountryCode != "") {
			info.ASN = asn
			if info.CountryCode != "" && info.FlagEmoji == "" {
				info.FlagEmoji = geo.CountryToFlag(info.CountryCode)
			}
			result.ASPathEnriched = append(result.ASPathEnriched, *info)
		} else {
			result.ASPathEnriched = append(result.ASPathEnriched, domain.ASInfo{ASN: asn})
		}
	}
	e.emitASPathPartial(result, opt)
}

func collectBGPASPathASNs(result *domain.QueryResult, br *domain.BGPResult) []uint32 {
	seen := map[uint32]bool{}
	var asns []uint32
	add := func(path []uint32) {
		for _, asn := range path {
			if asn == 0 || seen[asn] {
				continue
			}
			seen[asn] = true
			asns = append(asns, asn)
		}
	}
	if result != nil {
		add(result.ASPath)
	}
	if br != nil {
		for i := range br.Routes {
			add(br.Routes[i].ASPath)
		}
	}
	return asns
}

func SanitizeErrorMsg(err error) string {
	switch {
	case errors.Is(err, domain.ErrNodeNotFound):
		return "node not found"
	case errors.Is(err, domain.ErrCommandDisabled):
		return "command not enabled for this node"
	case errors.Is(err, domain.ErrDNSNotFound):
		return "dns resolution failed"
	case errors.Is(err, domain.ErrInvalidTarget):
		return "invalid target"
	case errors.Is(err, domain.ErrTimeout):
		return "command timed out"
	case errors.Is(err, domain.ErrRateLimited):
		return "rate limit exceeded"
	case errors.Is(err, domain.ErrQueryPoolFull):
		return "server is busy, try again later"
	default:
		if err == context.Canceled || err == context.DeadlineExceeded {
			return "command timed out"
		}
		return "command execution failed"
	}
}

func ClassifyError(err error) string {
	switch {
	case errors.Is(err, domain.ErrNodeNotFound):
		return "NODE_NOT_FOUND"
	case errors.Is(err, domain.ErrCommandDisabled):
		return "COMMAND_DISABLED"
	case errors.Is(err, domain.ErrDNSNotFound):
		return "DNS_NOT_FOUND"
	case errors.Is(err, domain.ErrInvalidTarget):
		return "INVALID_TARGET"
	case errors.Is(err, domain.ErrTimeout):
		return "COMMAND_TIMEOUT"
	case errors.Is(err, circuitbreaker.ErrCircuitOpen):
		return "NODE_UNAVAILABLE"
	case errors.Is(err, domain.ErrQueryPoolFull):
		return "POOL_FULL"
	case errors.Is(err, domain.ErrRateLimited):
		return "RATE_LIMITED"
	default:
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return "COMMAND_TIMEOUT"
		}
		return "INTERNAL_ERROR"
	}
}
