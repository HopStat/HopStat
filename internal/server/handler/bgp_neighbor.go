package handler

import (
	"context"
	"database/sql"
	"log/slog"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/HopStat/HopStat/internal/bgp"
	"github.com/HopStat/HopStat/internal/config"
	"github.com/HopStat/HopStat/internal/domain"
	"github.com/HopStat/HopStat/internal/sitecache"
	"github.com/HopStat/HopStat/internal/store/repo"
)

type bgpNeighborRequest struct {
	NodeID         int64  `json:"node_id" binding:"required"`
	RemoteAS       uint32 `json:"remote_as" binding:"required"`
	PeeringIP      string `json:"peering_ip"`
	NeighborIP     string `json:"neighbor_ip"`
	IPv6PeeringIP  string `json:"ipv6_peering_ip"`
	IPv6NeighborIP string `json:"ipv6_neighbor_ip"`
	Multihop       bool   `json:"multihop"`
	DefaultRouteAS uint32 `json:"default_route_as"`
}

func validateBGPConfigured(cfg config.BGPConfig) string {
	if cfg.LocalAS == 0 {
		return "bgp.local_as is not configured in config.yaml — set your ASN and restart HopStat"
	}
	return ""
}

func (r *bgpNeighborRequest) Validate() string {
	if r.NodeID <= 0 {
		return "node is required"
	}
	if r.RemoteAS == 0 {
		return "remote_as must be > 0"
	}

	r.PeeringIP = strings.TrimSpace(r.PeeringIP)
	r.NeighborIP = strings.TrimSpace(r.NeighborIP)
	r.IPv6PeeringIP = strings.TrimSpace(r.IPv6PeeringIP)
	r.IPv6NeighborIP = strings.TrimSpace(r.IPv6NeighborIP)

	hasV4 := r.PeeringIP != "" || r.NeighborIP != ""
	hasV6 := r.IPv6PeeringIP != "" || r.IPv6NeighborIP != ""
	if !hasV4 && !hasV6 {
		return "IPv4 or IPv6 peering addresses are required"
	}
	if r.PeeringIP != "" && r.NeighborIP == "" {
		return "neighbor_ip is required when peering_ip is set"
	}
	if r.NeighborIP != "" && r.PeeringIP == "" {
		return "peering_ip is required when neighbor_ip is set"
	}
	if r.IPv6PeeringIP != "" && r.IPv6NeighborIP == "" {
		return "ipv6_neighbor_ip is required when ipv6_peering_ip is set"
	}
	if r.IPv6NeighborIP != "" && r.IPv6PeeringIP == "" {
		return "ipv6_peering_ip is required when ipv6_neighbor_ip is set"
	}
	if r.PeeringIP != "" {
		if net.ParseIP(r.PeeringIP) == nil {
			return "invalid peering_ip"
		}
		if net.ParseIP(r.NeighborIP) == nil {
			return "invalid neighbor_ip"
		}
	}
	if r.IPv6PeeringIP != "" {
		if net.ParseIP(r.IPv6PeeringIP) == nil {
			return "invalid ipv6_peering_ip"
		}
		if net.ParseIP(r.IPv6NeighborIP) == nil {
			return "invalid ipv6_neighbor_ip"
		}
	}
	return ""
}

func bgpStatuses(mgr *bgp.SessionManager) map[int64]domain.BGPSessionState {
	if mgr == nil {
		return map[int64]domain.BGPSessionState{}
	}
	return mgr.GetAllStatuses()
}

func syncBGPNeighbor(mgr *bgp.SessionManager, action string, fn func() error) error {
	if mgr == nil {
		return nil
	}
	if err := fn(); err != nil {
		slog.Warn("bgp session "+action+" failed", "error", err)
		return err
	}
	return nil
}

func removeBGPNeighborsForNode(ctx context.Context, db *sql.DB, bgpMgr *bgp.SessionManager, nodeID int64) error {
	if bgpMgr == nil {
		return nil
	}
	r := repo.NewBGPNeighborRepo(db)
	neighbors, err := r.GetByNodeID(ctx, nodeID)
	if err != nil {
		return err
	}
	for _, n := range neighbors {
		if err := syncBGPNeighbor(bgpMgr, "remove", func() error { return bgpMgrRemoveNeighbor(bgpMgr, n.ID) }); err != nil {
			return err
		}
	}
	return nil
}

var bgpMgrRemoveNeighbor = func(mgr *bgp.SessionManager, id int64) error {
	return mgr.RemoveNeighbor(id)
}

var bgpMgrAddNeighbor = func(mgr *bgp.SessionManager, n *domain.BGPNeighbor) error {
	return mgr.AddNeighbor(n)
}

var bgpMgrUpdateNeighbor = func(mgr *bgp.SessionManager, n *domain.BGPNeighbor) error {
	return mgr.UpdateNeighbor(n)
}

var bgpNeighborStatusesFn = bgpStatuses

var deleteBGPNeighborRecordFn = func(ctx context.Context, r domain.BGPNeighborRepository, id int64) error {
	return r.Delete(ctx, id)
}

func GetBGPConfig(cfg config.BGPConfig, bgpMgr *bgp.SessionManager) gin.HandlerFunc {
	return func(c *gin.Context) {
		status := "not_configured"
		if cfg.LocalAS > 0 {
			status = "ready"
			if bgpMgr == nil || !bgpMgr.IsReady() {
				status = "restart_required"
			}
		}
		c.JSON(http.StatusOK, gin.H{"data": gin.H{
			"local_as":         cfg.LocalAS,
			"router_id":        cfg.RouterID,
			"listen_port":      cfg.ListenPort,
			"add_path_receive": cfg.AddPathReceive,
			"status":           status,
		}})
	}
}

func ListBGPNeighbors(db *sql.DB, bgpMgr *bgp.SessionManager, bgpCfg config.BGPConfig) gin.HandlerFunc {
	return func(c *gin.Context) {
		r := repo.NewBGPNeighborRepo(db)
		neighbors, err := r.GetAll(c.Request.Context())
		if err != nil {
			slog.Error("failed to list bgp neighbors", "error", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": sanitizeError(err)})
			return
		}
		statuses := bgpNeighborStatusesFn(bgpMgr)
		routeCtx, routeCancel := context.WithTimeout(c.Request.Context(), 15*time.Second)
		defer routeCancel()
		type neighborWithStatus struct {
			*domain.BGPNeighbor
			Status             domain.BGPSessionState `json:"status"`
			PrefixesReceived   int                    `json:"prefixes_received"`
		}
		result := make([]neighborWithStatus, len(neighbors))
		for i, n := range neighbors {
			if bgpCfg.LocalAS > 0 {
				n.LocalAS = bgpCfg.LocalAS
			}
			prefixes := 0
			if bgpMgr != nil && statuses[n.ID] == domain.BGPSessionEstablished {
				prefixes = bgpMgr.GetPrefixesReceived(routeCtx, n.ID)
			}
			result[i] = neighborWithStatus{
				BGPNeighbor:      n,
				Status:           statuses[n.ID],
				PrefixesReceived: prefixes,
			}
		}
		c.JSON(http.StatusOK, gin.H{"data": result})
	}
}

func CreateBGPNeighbor(db *sql.DB, bgpMgr *bgp.SessionManager, bgpCfg config.BGPConfig) gin.HandlerFunc {
	return func(c *gin.Context) {
		var req bgpNeighborRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		if msg := req.Validate(); msg != "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": msg})
			return
		}
		if msg := validateBGPConfigured(bgpCfg); msg != "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": msg})
			return
		}
		neighbor := &domain.BGPNeighbor{
			NodeID:         req.NodeID,
			LocalAS:        bgpCfg.LocalAS,
			RemoteAS:       req.RemoteAS,
			PeeringIP:      req.PeeringIP,
			NeighborIP:     req.NeighborIP,
			IPv6PeeringIP:  req.IPv6PeeringIP,
			IPv6NeighborIP: req.IPv6NeighborIP,
			Multihop:       req.Multihop,
			PeerType:       domain.PeerTypeFor(bgpCfg.LocalAS, req.RemoteAS),
			DefaultRouteAS: req.DefaultRouteAS,
		}
		r := repo.NewBGPNeighborRepo(db)
		created, err := r.Create(c.Request.Context(), neighbor)
		if err != nil {
			slog.Error("failed to create bgp neighbor", "error", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": sanitizeError(err)})
			return
		}
		if err := syncBGPNeighbor(bgpMgr, "add", func() error { return bgpMgrAddNeighbor(bgpMgr, created) }); err != nil {
			if delErr := deleteBGPNeighborRecordFn(c.Request.Context(), r, created.ID); delErr != nil {
				slog.Warn("failed to rollback bgp neighbor after session error", "id", created.ID, "error", delErr)
			}
			c.JSON(http.StatusBadRequest, gin.H{"error": sanitizeError(err)})
			return
		}
		c.JSON(http.StatusCreated, gin.H{"data": created})
	}
}

func UpdateBGPNeighbor(db *sql.DB, bgpMgr *bgp.SessionManager, bgpCfg config.BGPConfig) gin.HandlerFunc {
	return func(c *gin.Context) {
		id, err := strconv.ParseInt(c.Param("id"), 10, 64)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
			return
		}
		var req bgpNeighborRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		if msg := req.Validate(); msg != "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": msg})
			return
		}
		if msg := validateBGPConfigured(bgpCfg); msg != "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": msg})
			return
		}
		neighbor := &domain.BGPNeighbor{
			ID:             id,
			NodeID:         req.NodeID,
			LocalAS:        bgpCfg.LocalAS,
			RemoteAS:       req.RemoteAS,
			PeeringIP:      req.PeeringIP,
			NeighborIP:     req.NeighborIP,
			IPv6PeeringIP:  req.IPv6PeeringIP,
			IPv6NeighborIP: req.IPv6NeighborIP,
			Multihop:       req.Multihop,
			PeerType:       domain.PeerTypeFor(bgpCfg.LocalAS, req.RemoteAS),
			DefaultRouteAS: req.DefaultRouteAS,
		}
		r := repo.NewBGPNeighborRepo(db)
		updated, err := r.Update(c.Request.Context(), neighbor)
		if err != nil {
			slog.Error("failed to update bgp neighbor", "error", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": sanitizeError(err)})
			return
		}
		if err := syncBGPNeighbor(bgpMgr, "update", func() error { return bgpMgrUpdateNeighbor(bgpMgr, updated) }); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": sanitizeError(err)})
			return
		}
		c.JSON(http.StatusOK, gin.H{"data": updated})
	}
}

func DeleteBGPNeighbor(db *sql.DB, bgpMgr *bgp.SessionManager) gin.HandlerFunc {
	return func(c *gin.Context) {
		id, err := strconv.ParseInt(c.Param("id"), 10, 64)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
			return
		}
		if err := syncBGPNeighbor(bgpMgr, "remove", func() error { return bgpMgrRemoveNeighbor(bgpMgr, id) }); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": sanitizeError(err)})
			return
		}
		r := repo.NewBGPNeighborRepo(db)
		if err := r.Delete(c.Request.Context(), id); err != nil {
			slog.Error("failed to delete bgp neighbor", "error", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": sanitizeError(err)})
			return
		}
		c.JSON(http.StatusOK, gin.H{"data": "deleted"})
	}
}

func GetBGPNeighborStatuses(bgpMgr *bgp.SessionManager) gin.HandlerFunc {
	return func(c *gin.Context) {
		if bgpMgr == nil {
			c.JSON(http.StatusOK, gin.H{"data": []*domain.BGPSessionStatus{}})
			return
		}
		statuses := bgpMgr.GetSessionStatuses()
		c.JSON(http.StatusOK, gin.H{"data": statuses})
	}
}

func StopBGPNeighbor(bgpMgr *bgp.SessionManager) gin.HandlerFunc {
	return bgpNeighborAction(bgpMgr, "stop", func(mgr *bgp.SessionManager, id int64) error {
		return mgr.StopNeighbor(id)
	})
}

func RestartBGPNeighbor(bgpMgr *bgp.SessionManager) gin.HandlerFunc {
	return bgpNeighborAction(bgpMgr, "restart", func(mgr *bgp.SessionManager, id int64) error {
		return mgr.RestartNeighbor(id)
	})
}

func GetBGPNeighborLogs(bgpMgr *bgp.SessionManager) gin.HandlerFunc {
	return func(c *gin.Context) {
		id, err := strconv.ParseInt(c.Param("id"), 10, 64)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
			return
		}
		if bgpMgr == nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{"error": "bgp session manager is not available"})
			return
		}
		limit := 100
		if raw := c.Query("limit"); raw != "" {
			if n, err := strconv.Atoi(raw); err == nil && n > 0 {
				limit = n
			}
		}
		c.JSON(http.StatusOK, gin.H{"data": bgpMgr.GetNeighborLogs(id, limit)})
	}
}

type bgpNeighborActionFn func(mgr *bgp.SessionManager, id int64) error

func bgpNeighborAction(bgpMgr *bgp.SessionManager, action string, fn bgpNeighborActionFn) gin.HandlerFunc {
	return func(c *gin.Context) {
		id, err := strconv.ParseInt(c.Param("id"), 10, 64)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
			return
		}
		if bgpMgr == nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{"error": "bgp session manager is not available"})
			return
		}
		if err := fn(bgpMgr, id); err != nil {
			slog.Warn("bgp neighbor "+action+" failed", "id", id, "error", err)
			c.JSON(http.StatusBadRequest, gin.H{"error": sanitizeError(err)})
			return
		}
		c.JSON(http.StatusOK, gin.H{"data": gin.H{
			"id":     id,
			"action": action,
			"status": bgpMgr.GetStatus(id),
		}})
	}
}

// LookupBGPPaths exposes the raw paths behind a prefix, for diagnosing a disagreement
// between what HopStat shows and what the router reports.
func LookupBGPPaths(bgpMgr *bgp.SessionManager) gin.HandlerFunc {
	return func(c *gin.Context) {
		if bgpMgr == nil || !bgpMgr.IsReady() {
			c.JSON(http.StatusServiceUnavailable, gin.H{"error": "bgp is not configured"})
			return
		}

		prefix := strings.TrimSpace(c.Query("prefix"))
		if prefix == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "prefix is required"})
			return
		}

		var nodeID int64
		if raw := strings.TrimSpace(c.Query("node_id")); raw != "" {
			parsed, err := strconv.ParseInt(raw, 10, 64)
			if err != nil || parsed < 0 {
				c.JSON(http.StatusBadRequest, gin.H{"error": "invalid node_id"})
				return
			}
			nodeID = parsed
		}

		details, err := bgpMgr.LookupPathDetails(c.Request.Context(), nodeID, prefix, bgpPathNodeNamer(bgpMgr))
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, gin.H{"data": details})
	}
}

// bgpPathNodeNamer labels each path with the node whose session carried it, from the
// in-memory node snapshot — names only, so no credential key is involved.
func bgpPathNodeNamer(bgpMgr *bgp.SessionManager) func(string) string {
	cache := map[int64]string{}
	return func(neighborIP string) string {
		nodeID, ok := bgpMgr.NodeIDForNeighborIP(neighborIP)
		if !ok {
			return ""
		}
		if name, ok := cache[nodeID]; ok {
			return name
		}
		name := ""
		if node, ok := sitecache.NodeByID(nodeID); ok {
			name = node.Name
		}
		cache[nodeID] = name
		return name
	}
}
