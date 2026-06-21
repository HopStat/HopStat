package handler

import (
	"context"
	"net"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/HopStat/HopStat/internal/bgp"
	"github.com/HopStat/HopStat/internal/config"
	"github.com/HopStat/HopStat/internal/domain"
	"github.com/HopStat/HopStat/internal/driver"
	"github.com/HopStat/HopStat/internal/driver/standalone"
	"github.com/HopStat/HopStat/internal/geo"
	"github.com/HopStat/HopStat/internal/server/middleware"
)

func MountAgentAPI(r gin.IRouter, cfg *config.Config, bgpMgr *bgp.SessionManager, geoDB *geo.GeoIPDB) {
	v1 := r.Group("/agent/v1")
	{
		v1.GET("/health", agentHealth)
		v1.GET("/capabilities", agentCapabilities)
		v1.POST("/ping", agentPing(cfg))
		v1.POST("/ping/stream", agentPingStream(cfg))
		v1.POST("/traceroute", agentTraceroute(cfg))
		v1.POST("/traceroute/stream", agentTracerouteStream(cfg))
		v1.POST("/bgp/route", agentBGPRoute(cfg, bgpMgr, geoDB))
	}
}

func agentNodeFromContext(c *gin.Context) (*domain.Node, bool) {
	v, ok := c.Get(middleware.AgentNodeKey)
	if !ok {
		return nil, false
	}
	node, ok := v.(*domain.Node)
	return node, ok
}

func agentHealth(c *gin.Context) {
	node, ok := agentNodeFromContext(c)
	if !ok {
		c.JSON(500, gin.H{"error": "internal error"})
		return
	}
	c.JSON(200, gin.H{
		"status": "ok",
		"mode":   "standalone",
		"node":   node.Name,
	})
}

func agentCapabilities(c *gin.Context) {
	node, ok := agentNodeFromContext(c)
	if !ok {
		c.JSON(500, gin.H{"error": "internal error"})
		return
	}
	cmds := make([]string, len(node.EnabledCmds))
	for i, cmd := range node.EnabledCmds {
		cmds[i] = string(cmd)
	}
	c.JSON(200, gin.H{"commands": cmds})
}

func agentLocalDriver(node *domain.Node, cfg *config.Config) (driver.NodeDriver, error) {
	return newAgentDriver(node, cfg)
}

var newAgentDriver = func(node *domain.Node, cfg *config.Config) (driver.NodeDriver, error) {
	return standalone.NewDriver(node, cfg)
}

var agentBuildBGPRouteFn = func(ctx context.Context, mgr *bgp.SessionManager, nodeID int64, prefix string) (*domain.BGPResult, error) {
	return mgr.BuildRouteResult(ctx, nodeID, prefix, nil)
}

func agentPing(cfg *config.Config) gin.HandlerFunc {
	return func(c *gin.Context) {
		node, ok := agentNodeFromContext(c)
		if !ok {
			c.JSON(500, gin.H{"error": "internal error"})
			return
		}
		var req struct {
			Target string `json:"target" binding:"required"`
			Count  int    `json:"count"`
		}
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(400, gin.H{"error": err.Error()})
			return
		}
		if req.Count <= 0 {
			req.Count = 5
		}
		if req.Count > 50 {
			req.Count = 50
		}

		drv, err := agentLocalDriver(node, cfg)
		if err != nil {
			c.JSON(500, gin.H{"error": "failed to create driver"})
			return
		}
		result, err := drv.Ping(c.Request.Context(), req.Target, req.Count)
		if err != nil {
			raw := ""
			if result != nil {
				raw = result.Raw
			}
			c.JSON(500, gin.H{"error": err.Error(), "raw": raw, "packets_sent": req.Count})
			return
		}
		c.JSON(200, result)
	}
}

func agentTraceroute(cfg *config.Config) gin.HandlerFunc {
	return func(c *gin.Context) {
		node, ok := agentNodeFromContext(c)
		if !ok {
			c.JSON(500, gin.H{"error": "internal error"})
			return
		}
		var req struct {
			Target  string `json:"target" binding:"required"`
			MaxHops int    `json:"max_hops"`
		}
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(400, gin.H{"error": err.Error()})
			return
		}
		if req.MaxHops <= 0 {
			req.MaxHops = 30
		}
		if req.MaxHops > 64 {
			req.MaxHops = 64
		}

		drv, err := agentLocalDriver(node, cfg)
		if err != nil {
			c.JSON(500, gin.H{"error": "failed to create driver"})
			return
		}
		result, err := drv.Traceroute(c.Request.Context(), req.Target, req.MaxHops)
		if err != nil {
			raw := ""
			if result != nil {
				raw = result.Raw
			}
			c.JSON(500, gin.H{"error": err.Error(), "raw": raw})
			return
		}
		c.JSON(200, result)
	}
}

func agentBGPRoute(cfg *config.Config, bgpMgr *bgp.SessionManager, geoDB *geo.GeoIPDB) gin.HandlerFunc {
	return func(c *gin.Context) {
		node, ok := agentNodeFromContext(c)
		if !ok {
			c.JSON(500, gin.H{"error": "internal error"})
			return
		}
		var req struct {
			Prefix string `json:"prefix" binding:"required"`
		}
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(400, gin.H{"error": err.Error()})
			return
		}
		if strings.Contains(req.Prefix, "/") {
			if _, _, err := net.ParseCIDR(req.Prefix); err != nil {
				c.JSON(400, gin.H{"error": "invalid prefix"})
				return
			}
		} else if net.ParseIP(req.Prefix) == nil {
			c.JSON(400, gin.H{"error": "invalid IP address"})
			return
		}

		if bgpMgr != nil && bgpMgr.IsReady() {
			result, err := agentBuildBGPRouteFn(c.Request.Context(), bgpMgr, node.ID, req.Prefix)
			if err != nil {
				c.JSON(500, gin.H{"error": err.Error(), "raw": ""})
				return
			}
			bgp.ApplyOriginASPathToRoutes(result.Routes, bgpMgr.LocalAS())
			bgp.EnrichResultTargetAS(c.Request.Context(), geoDB, result, req.Prefix)
			c.JSON(200, result)
			return
		}

		drv, err := agentLocalDriver(node, cfg)
		if err != nil {
			c.JSON(500, gin.H{"error": "failed to create driver"})
			return
		}
		result, err := drv.BGPRoute(c.Request.Context(), req.Prefix)
		if err != nil {
			c.JSON(500, gin.H{"error": err.Error(), "raw": ""})
			return
		}
		c.JSON(200, result)
	}
}
