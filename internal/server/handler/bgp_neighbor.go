package handler

import (
	"database/sql"
	"log/slog"
	"net"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/HopStat/HopStat/internal/bgp"
	"github.com/HopStat/HopStat/internal/domain"
	"github.com/HopStat/HopStat/internal/store/repo"
)

type bgpNeighborRequest struct {
	NodeID     int64  `json:"node_id" binding:"required"`
	LocalAS    uint32 `json:"local_as" binding:"required"`
	RemoteAS   uint32 `json:"remote_as" binding:"required"`
	PeeringIP  string `json:"peering_ip" binding:"required"`
	NeighborIP string `json:"neighbor_ip" binding:"required"`
	Multihop   bool   `json:"multihop"`
}

func (r *bgpNeighborRequest) Validate() string {
	if net.ParseIP(r.PeeringIP) == nil {
		return "invalid peering_ip"
	}
	if net.ParseIP(r.NeighborIP) == nil {
		return "invalid neighbor_ip"
	}
	if r.LocalAS == 0 || r.RemoteAS == 0 {
		return "local_as and remote_as must be > 0"
	}
	return ""
}

func ListBGPNeighbors(db *sql.DB, bgpMgr *bgp.SessionManager) gin.HandlerFunc {
	return func(c *gin.Context) {
		r := repo.NewBGPNeighborRepo(db)
		neighbors, err := r.GetAll(c.Request.Context())
		if err != nil {
			slog.Error("failed to list bgp neighbors", "error", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": sanitizeError(err)})
			return
		}
		statuses := bgpMgr.GetAllStatuses()
		type neighborWithStatus struct {
			*domain.BGPNeighbor
			Status domain.BGPSessionState `json:"status"`
		}
		result := make([]neighborWithStatus, len(neighbors))
		for i, n := range neighbors {
			result[i] = neighborWithStatus{
				BGPNeighbor: n,
				Status:      statuses[n.ID],
			}
		}
		c.JSON(http.StatusOK, gin.H{"data": result})
	}
}

func CreateBGPNeighbor(db *sql.DB, bgpMgr *bgp.SessionManager) gin.HandlerFunc {
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
		neighbor := &domain.BGPNeighbor{
			NodeID:     req.NodeID,
			LocalAS:    req.LocalAS,
			RemoteAS:   req.RemoteAS,
			PeeringIP:  req.PeeringIP,
			NeighborIP: req.NeighborIP,
			Multihop:   req.Multihop,
		}
		r := repo.NewBGPNeighborRepo(db)
		created, err := r.Create(c.Request.Context(), neighbor)
		if err != nil {
			slog.Error("failed to create bgp neighbor", "error", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": sanitizeError(err)})
			return
		}
		if err := bgpMgr.AddNeighbor(created); err != nil {
			slog.Warn("bgp session add failed", "id", created.ID, "error", err)
		}
		c.JSON(http.StatusCreated, gin.H{"data": created})
	}
}

func UpdateBGPNeighbor(db *sql.DB, bgpMgr *bgp.SessionManager) gin.HandlerFunc {
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
		neighbor := &domain.BGPNeighbor{
			ID:         id,
			NodeID:     req.NodeID,
			LocalAS:    req.LocalAS,
			RemoteAS:   req.RemoteAS,
			PeeringIP:  req.PeeringIP,
			NeighborIP: req.NeighborIP,
			Multihop:   req.Multihop,
		}
		r := repo.NewBGPNeighborRepo(db)
		updated, err := r.Update(c.Request.Context(), neighbor)
		if err != nil {
			slog.Error("failed to update bgp neighbor", "error", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": sanitizeError(err)})
			return
		}
		if err := bgpMgr.UpdateNeighbor(updated); err != nil {
			slog.Warn("bgp session update failed", "id", id, "error", err)
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
		if err := bgpMgr.RemoveNeighbor(id); err != nil {
			slog.Warn("bgp session remove failed", "id", id, "error", err)
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
		statuses := bgpMgr.GetSessionStatuses()
		c.JSON(http.StatusOK, gin.H{"data": statuses})
	}
}
