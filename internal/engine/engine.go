package engine

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"strconv"
	"strings"
	"time"

	"github.com/yourorg/lg-looking-glass/internal/bgp"
	"github.com/yourorg/lg-looking-glass/internal/circuitbreaker"
	"github.com/yourorg/lg-looking-glass/internal/domain"
	"github.com/yourorg/lg-looking-glass/internal/driver"
	"github.com/yourorg/lg-looking-glass/internal/geo"
)

type QueryEngine struct {
	pool      *QueryPool
	cfg       *QueryConfig
	nodeRepo  domain.NodeRepository
	rateLimit *RateLimiter
	geoDB     *geo.GeoIPDB
	bgpMgr    *bgp.SessionManager
}

type QueryConfig struct {
	MaxConcurrent        int
	DefaultTimeoutSec    int
	MTRTimeoutSec        int
	TracerouteTimeoutSec int
}

func New(cfg *QueryConfig, nodeRepo domain.NodeRepository, geoDB *geo.GeoIPDB, bgpMgr *bgp.SessionManager) *QueryEngine {
	return &QueryEngine{
		pool:      NewQueryPool(cfg.MaxConcurrent),
		cfg:       cfg,
		nodeRepo:  nodeRepo,
		rateLimit: NewRateLimiter(10, time.Minute),
		geoDB:     geoDB,
		bgpMgr:    bgpMgr,
	}
}

type ExecuteOption struct {
	OnLine     func(string)
	ShouldStop func() bool
}

func (e *QueryEngine) Execute(ctx context.Context, query *domain.Query, opts ...ExecuteOption) (*domain.QueryResult, error) {
	var opt ExecuteOption
	if len(opts) > 0 {
		opt = opts[0]
	}

	if !e.rateLimit.Allow(query.SourceIP) {
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

	drv, err := driver.NewDriver(node, nil)
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
	case domain.CmdMTR:
		timeout = time.Duration(e.cfg.MTRTimeoutSec) * time.Second
	case domain.CmdTraceroute:
		timeout = time.Duration(e.cfg.TracerouteTimeoutSec) * time.Second
	}

	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	if opt.OnLine != nil {
		ctx = domain.WithOnLine(ctx, opt.OnLine)
	}

	// Early stop: cancel context when ShouldStop returns true
	if opt.ShouldStop != nil {
		stopCtx, stopCancel := context.WithCancel(ctx)
		ctx = stopCtx
		stopDone := make(chan struct{})
		go func() {
			defer close(stopDone)
			ticker := time.NewTicker(500 * time.Millisecond)
			defer ticker.Stop()
			for {
				select {
				case <-ticker.C:
					if opt.ShouldStop() {
						stopCancel()
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

		case domain.CmdMTR:
			mr, err := drv.MTR(ctx, query.Target, query.Options.MTRCycles)
			if err != nil {
				return err
			}
			e.enrichMTRHops(ctx, mr.Hops)
			result.Parsed = mr
			result.Raw = mr.Raw

		case domain.CmdBGPRoute:
			if e.bgpMgr != nil && e.bgpMgr.HasActiveSession(query.NodeID) {
				entries, err := e.bgpMgr.LookupRoute(ctx, query.NodeID, query.Target)
				if err == nil && len(entries) > 0 {
					raw := formatBGPRouteEntries(entries)
					br := &domain.BGPResult{Raw: raw}
					for _, entry := range entries {
						br.Routes = append(br.Routes, domain.BGPRoute{
							Prefix:    entry.Prefix,
							NextHop:   entry.NextHop,
							ASPath:    parseASPath(entry.ASPath),
							Origin:    entry.Origin,
							LocalPref: parseUint(entry.LocalPref),
							MED:       parseUint(entry.MED),
							Age:       entry.Age,
						})
					}
					result.Parsed = br
					result.Raw = br.Raw
					e.enrichASPath(ctx, br, result)
					return nil
				}
			}
			br, err := drv.BGPRoute(ctx, query.Target)
			if err != nil {
				return err
			}
			result.Parsed = br
			result.Raw = br.Raw
			e.enrichASPath(ctx, br, result)

		case domain.CmdASPath:
			asn, _ := strconv.ParseUint(query.Target, 10, 32)
			if e.bgpMgr != nil && e.bgpMgr.HasActiveSession(query.NodeID) {
				entries, err := e.bgpMgr.LookupASPath(ctx, query.NodeID, uint32(asn))
				if err == nil && len(entries) > 0 {
					raw := formatBGPRouteEntries(entries)
					ar := &domain.ASPathResult{Raw: raw, ASN: uint32(asn)}
					for _, entry := range entries {
						ar.Prefixes = append(ar.Prefixes, domain.ASPathEntry{
							Prefix: entry.Prefix,
							ASPath: parseASPath(entry.ASPath),
						})
					}
					result.Parsed = ar
					result.Raw = ar.Raw
					return nil
				}
			}
			ar, err := drv.ASPath(ctx, uint32(asn))
			if err != nil {
				return err
			}
			result.Parsed = ar
			result.Raw = ar.Raw
		}
		return nil
	})

	result.DurationMS = time.Since(start).Milliseconds()

	if err != nil {
		result.Status = domain.StatusError
		result.ErrorCode = classifyError(err)
		result.ErrorMsg = err.Error()
	} else {
		result.Status = domain.StatusDone
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
	if e.geoDB == nil {
		return
	}
	for i := range hops {
		ip := net.ParseIP(hops[i].IP)
		if ip == nil {
			continue
		}
		info, err := e.geoDB.ResolveASN(ctx, hops[i].IP)
		if err == nil && info != nil && info.ASN > 0 {
			hops[i].ASInfo = info
		}
	}
}

func (e *QueryEngine) enrichMTRHops(ctx context.Context, hops []domain.MTRHop) {
	if e.geoDB == nil {
		return
	}
	for i := range hops {
		ip := net.ParseIP(hops[i].Host)
		if ip == nil {
			continue
		}
		info, err := e.geoDB.ResolveASN(ctx, hops[i].Host)
		if err == nil && info != nil && info.ASN > 0 {
			hops[i].ASInfo = info
		}
	}
}

func (e *QueryEngine) enrichASPath(ctx context.Context, br *domain.BGPResult, result *domain.QueryResult) {
	if e.geoDB == nil || !e.geoDB.Enabled() || br == nil {
		return
	}
	seen := map[uint32]bool{}
	for _, route := range br.Routes {
		for _, asn := range route.ASPath {
			if seen[asn] || asn == 0 {
				continue
			}
			seen[asn] = true
			info, err := e.geoDB.ResolveASN(ctx, fmt.Sprintf("%d", asn))
			if err == nil && info != nil {
				info.ASN = asn
				result.ASPathEnriched = append(result.ASPathEnriched, *info)
			} else {
				result.ASPathEnriched = append(result.ASPathEnriched, domain.ASInfo{ASN: asn})
			}
		}
	}
}

func classifyError(err error) string {
	switch err {
	case domain.ErrNodeNotFound:
		return "NODE_NOT_FOUND"
	case domain.ErrCommandDisabled:
		return "COMMAND_DISABLED"
	case domain.ErrInvalidTarget:
		return "INVALID_TARGET"
	case domain.ErrTimeout:
		return "COMMAND_TIMEOUT"
	case circuitbreaker.ErrCircuitOpen:
		return "NODE_UNAVAILABLE"
	case domain.ErrQueryPoolFull:
		return "POOL_FULL"
	case domain.ErrRateLimited:
		return "RATE_LIMITED"
	default:
		if err == context.Canceled {
			return "COMMAND_TIMEOUT"
		}
		if err == context.DeadlineExceeded {
			return "COMMAND_TIMEOUT"
		}
		return "INTERNAL_ERROR"
	}
}

func formatBGPRouteEntries(entries []*domain.BGPRouteEntry) string {
	var buf strings.Builder
	for _, e := range entries {
		fmt.Fprintf(&buf, "%-20s via %s  AS_PATH: %s  Origin: %s", e.Prefix, e.NextHop, e.ASPath, e.Origin)
		if e.Best {
			buf.WriteString("  [best]")
		}
		buf.WriteByte('\n')
	}
	return buf.String()
}

func parseASPath(raw string) []uint32 {
	if raw == "" {
		return nil
	}
	var asns []uint32
	for _, part := range strings.FieldsFunc(raw, func(r rune) bool {
		return r == ' ' || r == '[' || r == ']' || r == ','
	}) {
		if v, err := strconv.ParseUint(part, 10, 32); err == nil {
			asns = append(asns, uint32(v))
		}
	}
	return asns
}

func parseUint(s string) uint32 {
	v, _ := strconv.ParseUint(s, 10, 32)
	return uint32(v)
}
