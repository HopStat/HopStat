package handler

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/HopStat/HopStat/internal/config"
	"github.com/HopStat/HopStat/internal/domain"
	"github.com/gin-gonic/gin"
)

func writeSSE(c *gin.Context, flusher http.Flusher, event string, payload interface{}) {
	data, _ := json.Marshal(payload)
	fmt.Fprintf(c.Writer, "event: %s\ndata: %s\n\n", event, data)
	if flusher != nil {
		flusher.Flush()
	}
}

func agentPingStream(cfg *config.Config) gin.HandlerFunc {
	return func(c *gin.Context) {
		node, ok := agentNodeFromContext(c)
		if !ok {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "internal error"})
			return
		}

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
		if req.Count > 50 {
			req.Count = 50
		}

		drv, err := agentLocalDriver(node, cfg)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to create driver"})
			return
		}

		c.Header("Content-Type", "text/event-stream")
		c.Header("Cache-Control", "no-cache")
		c.Header("Connection", "keep-alive")
		c.Header("X-Accel-Buffering", "no")
		flusher, _ := c.Writer.(http.Flusher)

		ctx := domain.WithOnLine(c.Request.Context(), func(line string) {
			writeSSE(c, flusher, "output", gin.H{"line": line})
		})

		result, err := drv.Ping(ctx, req.Target, req.Count)
		if err != nil {
			payload := gin.H{"error": err.Error()}
			if result != nil && result.Raw != "" {
				payload["raw"] = result.Raw
			}
			writeSSE(c, flusher, "error", payload)
			return
		}
		writeSSE(c, flusher, "result", result)
	}
}

func agentTracerouteStream(cfg *config.Config) gin.HandlerFunc {
	return func(c *gin.Context) {
		node, ok := agentNodeFromContext(c)
		if !ok {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "internal error"})
			return
		}

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
		if req.MaxHops > 64 {
			req.MaxHops = 64
		}

		drv, err := agentLocalDriver(node, cfg)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to create driver"})
			return
		}

		c.Header("Content-Type", "text/event-stream")
		c.Header("Cache-Control", "no-cache")
		c.Header("Connection", "keep-alive")
		c.Header("X-Accel-Buffering", "no")
		flusher, _ := c.Writer.(http.Flusher)

		ctx := domain.WithOnLine(c.Request.Context(), func(line string) {
			writeSSE(c, flusher, "output", gin.H{"line": line})
		})

		result, err := drv.Traceroute(ctx, req.Target, req.MaxHops)
		if err != nil {
			payload := gin.H{"error": err.Error()}
			if result != nil && result.Raw != "" {
				payload["raw"] = result.Raw
			}
			writeSSE(c, flusher, "error", payload)
			return
		}
		writeSSE(c, flusher, "result", result)
	}
}
