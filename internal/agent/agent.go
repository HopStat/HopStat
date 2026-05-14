package agent

import (
	"context"
	"crypto/subtle"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/HopStat/HopStat/internal/config"
)

type Agent struct {
	cfg    *config.Config
	router *gin.Engine
	server *http.Server
}

func New(cfg *config.Config) *Agent {
	gin.SetMode(gin.ReleaseMode)
	router := gin.New()
	router.Use(gin.Recovery())

	agent := &Agent{
		cfg:    cfg,
		router: router,
	}

	agent.setupRoutes()
	return agent
}

func (a *Agent) setupRoutes() {
	r := a.router

	r.Use(agentAuth(a.cfg.Agent.Token))

	v1 := r.Group("/agent/v1")
	{
		v1.POST("/ping", a.handlePing)
		v1.POST("/traceroute", a.handleTraceroute)
		v1.POST("/mtr", a.handleMTR)
		v1.POST("/bgp/route", a.handleBGPRoute)
		v1.POST("/bgp/aspath", a.handleASPath)
		v1.GET("/capabilities", a.handleCapabilities)
		v1.GET("/health", a.handleHealth)
	}
}

func (a *Agent) Run(ctx context.Context) error {
	addr := fmt.Sprintf("%s:%d", a.cfg.Server.Host, a.cfg.Agent.Port)
	a.server = &http.Server{
		Addr:    addr,
		Handler: a.router,
	}

	slog.Info("agent starting", "addr", addr)

	go func() {
		if err := a.server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			slog.Error("agent server error", "error", err)
		}
	}()

	<-ctx.Done()

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	return a.server.Shutdown(shutdownCtx)
}

func agentAuth(token string) gin.HandlerFunc {
	return func(c *gin.Context) {
		auth := c.GetHeader("Authorization")
		if auth == "" {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "missing authorization header"})
			return
		}

		if subtle.ConstantTimeCompare([]byte(auth), []byte("Bearer "+token)) != 1 {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "invalid token"})
			return
		}

		c.Next()
	}
}

func (a *Agent) handlePing(c *gin.Context) {
	var req struct {
		Target string `json:"target" binding:"required"`
		Count  int    `json:"count"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if req.Count <= 0 {
		req.Count = 5
	}

	result, err := runPing(c.Request.Context(), req.Target, req.Count)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"error": err.Error(), "raw": result.Raw, "packets_sent": result.PacketsSent})
		return
	}
	c.JSON(http.StatusOK, result)
}

func (a *Agent) handleTraceroute(c *gin.Context) {
	var req struct {
		Target  string `json:"target" binding:"required"`
		MaxHops int    `json:"max_hops"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if req.MaxHops <= 0 {
		req.MaxHops = 30
	}

	result, err := runTraceroute(c.Request.Context(), req.Target, req.MaxHops)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"error": err.Error(), "raw": result.Raw})
		return
	}
	c.JSON(http.StatusOK, result)
}

func (a *Agent) handleMTR(c *gin.Context) {
	var req struct {
		Target string `json:"target" binding:"required"`
		Cycles int    `json:"cycles"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if req.Cycles <= 0 {
		req.Cycles = 10
	}

	result, err := runMTR(c.Request.Context(), req.Target, req.Cycles)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"error": err.Error(), "raw": result.Raw})
		return
	}
	c.JSON(http.StatusOK, result)
}

func (a *Agent) handleBGPRoute(c *gin.Context) {
	var req struct {
		Prefix string `json:"prefix" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if strings.Contains(req.Prefix, "/") {
		if _, _, err := net.ParseCIDR(req.Prefix); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid prefix"})
			return
		}
	} else {
		if net.ParseIP(req.Prefix) == nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid IP address"})
			return
		}
	}

	result, err := runBGPRoute(c.Request.Context(), req.Prefix)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"error": err.Error(), "raw": ""})
		return
	}
	c.JSON(http.StatusOK, result)
}

func (a *Agent) handleASPath(c *gin.Context) {
	var req struct {
		ASN uint32 `json:"asn" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	result, err := runASPath(c.Request.Context(), req.ASN)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"error": err.Error(), "raw": ""})
		return
	}
	c.JSON(http.StatusOK, result)
}

func (a *Agent) handleCapabilities(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"commands": []string{"ping", "traceroute", "mtr", "bgp_route", "as_path"},
	})
}

func (a *Agent) handleHealth(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"status": "ok",
		"mode":   "agent",
	})
}