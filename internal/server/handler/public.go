package handler

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/csv"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/mail"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"github.com/HopStat/HopStat/internal/bgp"
	"github.com/HopStat/HopStat/internal/config"
	"github.com/HopStat/HopStat/internal/domain"
	"github.com/HopStat/HopStat/internal/driver"
	"github.com/HopStat/HopStat/internal/engine"
	"github.com/HopStat/HopStat/internal/geo"
	"github.com/HopStat/HopStat/internal/server/middleware"
	"github.com/HopStat/HopStat/internal/sitecache"
	"github.com/HopStat/HopStat/internal/store/queries"
	"github.com/HopStat/HopStat/internal/store/querystore"
	"github.com/HopStat/HopStat/internal/store/repo"
	"github.com/HopStat/HopStat/internal/target"
	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"
)

var queryStore = querystore.New()

type Handler struct {
	db       *sql.DB
	cfg      *config.Config
	engine   *engine.QueryEngine
	geoDB    *geo.GeoIPDB
	nodeRepo domain.NodeRepository
}

func New(db *sql.DB, cfg *config.Config, geoDB *geo.GeoIPDB, bgpMgr *bgp.SessionManager) *Handler {
	credKey := cfg.Security.CredentialKey
	h := &Handler{
		db:       db,
		cfg:      cfg,
		geoDB:    geoDB,
		nodeRepo: sitecache.NewCachedNodeRepo(db, credKey),
	}
	h.engine = engine.New(&engine.QueryConfig{
		MaxConcurrent:        cfg.Query.MaxConcurrent,
		DefaultTimeoutSec:    cfg.Query.DefaultTimeoutSec,
		TracerouteTimeoutSec: cfg.Query.TracerouteTimeoutSec,
		ServerPort:           cfg.Server.Port,
		FloodControlEnabled:  cfg.FloodControl.Enabled,
		QueryRateLimitPerMin: cfg.FloodControl.QueryRateLimitPerMin,
	}, h.nodeRepo, sitecache.NewCachedCommunityRuleRepo(db), h.geoDB, bgpMgr, &dbSettingsProvider{}, cfg.BGP.LocalAS)
	return h
}

func sanitizeError(err error) string {
	slog.Error("handler error", "error", err)
	return "internal error"
}

func adminDiagError(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}

func friendlyConnError(err error) string {
	if err == nil {
		return ""
	}
	msg := err.Error()
	if errors.Is(err, context.DeadlineExceeded) || strings.Contains(msg, "context deadline exceeded") {
		return "connection timed out — check agent URL, firewall, and that the agent is running"
	}
	if strings.Contains(msg, "connection refused") {
		return "connection refused — agent is not listening on that address"
	}
	if strings.Contains(msg, "no route to host") {
		return "no route to host — check network connectivity"
	}
	if strings.Contains(msg, "health check failed: 401") {
		return "invalid agent token"
	}
	return msg
}

func validAgentURL(raw string) bool {
	return target.ValidAgentURL(raw)
}

func ListNodes(db *sql.DB, credKey string, bgpMgr *bgp.SessionManager) gin.HandlerFunc {
	return func(c *gin.Context) {
		_ = db
		_ = credKey
		nodes := activeNodesLister()
		type publicNode struct {
			domain.Node
			BGPActive bool `json:"bgp_active"`
		}
		result := make([]publicNode, len(nodes))
		for i, n := range nodes {
			if n == nil {
				continue
			}
			result[i] = publicNode{
				Node:      *n,
				BGPActive: bgpMgr != nil && bgpMgr.HasActiveSession(n.ID),
			}
		}
		c.JSON(http.StatusOK, gin.H{"data": result})
	}
}

func GetNode(db *sql.DB, credKey string) gin.HandlerFunc {
	return func(c *gin.Context) {
		id, err := strconv.ParseInt(c.Param("id"), 10, 64)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid node id"})
			return
		}
		repo := repo.NewNodeRepo(db, credKey)
		node, err := repo.GetByID(c.Request.Context(), id)
		if err != nil {
			c.JSON(http.StatusNotFound, gin.H{"error": "node not found"})
			return
		}
		node.AgentToken = ""
		c.JSON(http.StatusOK, gin.H{"data": node})
	}
}

func SubmitQuery(db *sql.DB, cfg *config.Config, geoDB *geo.GeoIPDB, bgpMgr *bgp.SessionManager) gin.HandlerFunc {
	return New(db, cfg, geoDB, bgpMgr).SubmitQuery()
}

func (h *Handler) SubmitQuery() gin.HandlerFunc {
	return func(c *gin.Context) {
		var req struct {
			NodeID   int64  `json:"node_id" binding:"required"`
			Command  string `json:"command" binding:"required"`
			Target   string `json:"target" binding:"required"`
			Protocol string `json:"protocol"`
			Options  struct {
				PingCount int `json:"ping_count"`
				MaxHops   int `json:"max_hops"`
			} `json:"options"`
		}

		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}

		// V-01: Validate command type
		if !domain.IsSupportedNodeCommand(req.Command) {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid command. Must be one of: ping, traceroute, bgp_route"})
			return
		}

		// V-01b: Check node's enabled commands (memory cache, no DB round-trip)
		if node, ok := sitecache.NodeByID(req.NodeID); ok {
			if len(node.EnabledCmds) > 0 && !node.CanExecute(domain.CommandType(req.Command)) {
				c.JSON(http.StatusBadRequest, gin.H{
					"error":      engine.SanitizeErrorMsg(domain.ErrCommandDisabled),
					"error_code": engine.ClassifyError(domain.ErrCommandDisabled),
				})
				return
			}
		} else {
			c.JSON(http.StatusNotFound, gin.H{"error": "node not found"})
			return
		}

		if req.Options.PingCount <= 0 {
			req.Options.PingCount = 5
		}
		if req.Options.MaxHops <= 0 {
			req.Options.MaxHops = 30
		}

		// V-02: Enforce upper bounds
		if req.Options.PingCount > 20 {
			req.Options.PingCount = 20
		}
		if req.Options.MaxHops > 64 {
			req.Options.MaxHops = 64
		}

		resolveCtx, cancel := context.WithTimeout(c.Request.Context(), 3*time.Second)
		resolvedTarget, resolveErr := target.ValidateQueryTarget(resolveCtx, req.Command, req.Target)
		cancel()
		if resolveErr != nil {
			c.JSON(http.StatusBadRequest, gin.H{
				"error":      engine.SanitizeErrorMsg(resolveErr),
				"error_code": engine.ClassifyError(resolveErr),
			})
			return
		}
		req.Target = resolvedTarget

		queryID := uuid.New().String()

		query := &domain.Query{
			ID:       queryID,
			NodeID:   req.NodeID,
			Command:  domain.CommandType(req.Command),
			Target:   req.Target,
			SourceIP: c.ClientIP(),
			Options: domain.QueryOptions{
				PingCount: req.Options.PingCount,
				MaxHops:   req.Options.MaxHops,
				Protocol:  req.Protocol,
			},
			CreatedAt: time.Now(),
		}

		auditParams := domain.FormatQueryAuditParams(
			req.Command, req.Target, req.Protocol,
			req.Options.PingCount, req.Options.MaxHops,
		)
		auditRepo := repo.NewAuditRepo(h.db)
		sourceIP := c.ClientIP()

		// Store running entry immediately so SSE can start streaming
		queryStore.SetRunning(queryID)

		maxTimeouts := 5
		if siteSettings := sitecache.AllSettings(); len(siteSettings) > 0 {
			maxTimeouts = settingInt(siteSettings, "traceroute_max_timeouts", 5, 1, 20)
		}

		// Execute query asynchronously with line streaming
		go func() {
			if submitQueryAsyncDone != nil {
				defer submitQueryAsyncDone()
			}
			var consecutiveTimeouts int64

			result, err := handlerEngineExecute(h, context.Background(), query, engine.ExecuteOption{
				OnPartial: func(partial *domain.QueryResult) {
					queryStore.MergePartial(queryID, partial)
				},
				OnLine: func(line string) {
					enriched := line

					// Inline AS enrichment for traceroute hop lines (skip header)
					isHeader := strings.HasPrefix(strings.TrimSpace(line), "traceroute to") || strings.HasPrefix(strings.TrimSpace(line), "Start")
					if h.geoDB != nil && req.Command == "traceroute" && !isHeader {
						enriched = formatTracerouteLine(context.Background(), h.geoDB, line)
					}

					// Detect timeout hops for early termination
					if req.Command == "traceroute" {
						trimmed := strings.TrimSpace(line)
						stripped := strings.TrimLeft(trimmed, "0123456789. \t")
						if strings.HasPrefix(stripped, "* * *") || stripped == "* * *" {
							atomic.AddInt64(&consecutiveTimeouts, 1)
						} else if strings.Contains(stripped, "ms") {
							atomic.StoreInt64(&consecutiveTimeouts, 0)
						}
					}

					for _, part := range strings.Split(enriched, "\n") {
						if part != "" {
							queryStore.AppendLine(queryID, part)
						}
					}
				},
				ShouldStop: func() bool {
					return atomic.LoadInt64(&consecutiveTimeouts) >= int64(maxTimeouts)
				},
			})
			if err != nil {
				slog.Error("query failed", "error", err, "query_id", queryID)
			}
			queryStore.MarkOutputComplete(queryID)
			if !skipASPathWaitForBGP(req.Command, result) {
				waitForASPathEnrichment(queryID, 8*time.Second)
			}
			if stored, ok := queryStore.Get(queryID); ok && stored != nil && result != nil {
				mergeASPathFields(result, stored)
			}
			queryStore.Set(queryID, result)

			if result != nil {
				auditEntry := &domain.AuditEntry{
					SourceIP:   sourceIP,
					NodeID:     &req.NodeID,
					Command:    req.Command,
					Params:     auditParams,
					DurationMS: result.DurationMS,
					Success:    result.Status == domain.StatusDone,
					ErrorMsg:   result.ErrorMsg,
				}
				if err := auditRepo.Log(context.Background(), auditEntry); err != nil {
					slog.Error("failed to write audit log", "error", err)
				}
			}
		}()

		c.JSON(http.StatusOK, gin.H{
			"data": gin.H{
				"query_id":   queryID,
				"status":     "running",
				"stream_url": "/api/v1/query/" + queryID + "/stream",
			},
		})
	}
}

func GetResult(db *sql.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		queryID := c.Param("id")
		if queryID == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "missing query id"})
			return
		}

		result, ok := queryStore.Get(queryID)
		if !ok {
			c.JSON(http.StatusNotFound, gin.H{"error": "query result not found or expired"})
			return
		}

		c.JSON(http.StatusOK, gin.H{"data": result})
	}
}

func StreamResult(db *sql.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		queryID := c.Param("id")
		if queryID == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "missing query id"})
			return
		}

		if _, exists := queryStore.Get(queryID); !exists {
			c.JSON(http.StatusNotFound, gin.H{"error": "query not found"})
			return
		}

		c.Header("Content-Type", "text/event-stream")
		c.Header("Cache-Control", "no-cache")
		c.Header("Connection", "keep-alive")

		flusher, canFlush := c.Writer.(http.Flusher)
		lastLineIdx := 0
		lastPartialSig := ""
		outputDoneSent := false
		deadline := time.Now().Add(streamResultMaxDuration)

		for time.Now().Before(deadline) {
			// Stream new output lines
			lines, _ := queryStore.GetLines(queryID)
			for idx := lastLineIdx; idx < len(lines); idx++ {
				data, _ := json.Marshal(gin.H{"line": lines[idx]})
				_, _ = c.Writer.WriteString("event: output\ndata: " + string(data) + "\n\n")
			}
			if len(lines) > lastLineIdx {
				lastLineIdx = len(lines)
				if canFlush {
					flusher.Flush()
				}
			}

			if !outputDoneSent && queryStore.IsOutputComplete(queryID) {
				_, _ = c.Writer.WriteString("event: output_done\ndata: {}\n\n")
				outputDoneSent = true
				if canFlush {
					flusher.Flush()
				}
			}

			// Stream partial parsed data (BGP AS path, community matches)
			result, ok := queryStore.Get(queryID)
			if ok && result.Status == domain.StatusRunning {
				payload, _ := json.Marshal(gin.H{
					"parsed":           result.Parsed,
					"raw":              result.Raw,
					"matched_rules":    result.MatchedRules,
					"as_path":          result.ASPath,
					"as_path_prefix":   result.ASPathPrefix,
					"as_path_enriched": result.ASPathEnriched,
					"as_path_nodes":    result.ASPathNodes,
				})
				sig := string(payload)
				if sig != lastPartialSig {
					lastPartialSig = sig
					_, _ = c.Writer.WriteString("event: partial\ndata: " + sig + "\n\n")
					if canFlush {
						flusher.Flush()
					}
				}
			}

			// Check for final result
			if ok && (result.Status == domain.StatusDone || result.Status == domain.StatusError) {
				data, _ := json.Marshal(result)
				_, _ = c.Writer.WriteString("event: result\ndata: " + string(data) + "\n\n")
				if canFlush {
					flusher.Flush()
				}
				return
			}

			select {
			case <-c.Request.Context().Done():
				return
			case <-queryStore.NotifyCh(queryID):
			case <-time.After(streamResultPollInterval):
			}
		}
	}
}

func Login(db *sql.DB, cfg *config.Config) gin.HandlerFunc {
	return func(c *gin.Context) {
		var req struct {
			Email    string `json:"email" binding:"required"`
			Password string `json:"password" binding:"required"`
		}

		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}

		userRepo := repo.NewUserRepo(db)
		user, err := userRepo.GetByEmail(c.Request.Context(), req.Email)
		if err != nil || user == nil {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid credentials"})
			return
		}

		if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(req.Password)); err != nil {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid credentials"})
			return
		}

		go func() {
			if err := loginUpdateLastLogin(context.Background(), userRepo, user.ID); err != nil {
				slog.Error("failed to update last login", "user_id", user.ID, "error", err)
			}
		}()

		token, err := generateJWTForLogin(user.ID, "admin", cfg.Security.JWTSecret)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to generate token"})
			return
		}

		middleware.SetAuthCookie(c, token, cfg)

		expiresAt := time.Now().Add(24 * time.Hour).Format(time.RFC3339)
		c.JSON(http.StatusOK, gin.H{
			"data": gin.H{
				"expires_at": expiresAt,
			},
		})
	}
}

func Session() gin.HandlerFunc {
	return func(c *gin.Context) {
		expiresAt := ""
		if expVal, ok := c.Get("token_exp"); ok {
			if t, ok := expVal.(time.Time); ok && !t.IsZero() {
				expiresAt = t.Format(time.RFC3339)
			}
		}
		c.JSON(http.StatusOK, gin.H{
			"data": gin.H{
				"authenticated": true,
				"expires_at":    expiresAt,
			},
		})
	}
}

func Logout(cfg *config.Config, denyList *middleware.JTIDenyList) gin.HandlerFunc {
	return func(c *gin.Context) {
		if jti, ok := c.Get("jti"); ok {
			if jtiStr, ok := jti.(string); ok && jtiStr != "" {
				exp := time.Now().Add(24 * time.Hour)
				if expVal, ok := c.Get("token_exp"); ok {
					if t, ok := expVal.(time.Time); ok && !t.IsZero() {
						exp = t
					}
				}
				denyList.Revoke(jtiStr, exp)
			}
		} else if tokenStr := middleware.ExtractBearerToken(c); tokenStr != "" {
			claims := &middleware.Claims{}
			token, err := jwt.ParseWithClaims(tokenStr, claims, func(token *jwt.Token) (interface{}, error) {
				if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
					return nil, fmt.Errorf("unexpected signing method")
				}
				return []byte(cfg.Security.JWTSecret), nil
			})
			if err == nil && token.Valid && claims.ID != "" {
				exp := time.Now().Add(24 * time.Hour)
				if claims.ExpiresAt != nil {
					exp = claims.ExpiresAt.Time
				}
				denyList.Revoke(claims.ID, exp)
			}
		}
		middleware.ClearAuthCookie(c, cfg)
		c.JSON(http.StatusOK, gin.H{"data": "logged out"})
	}
}

// Admin handlers
func ListAllNodes(db *sql.DB, credKey string) gin.HandlerFunc {
	return func(c *gin.Context) {
		repo := repo.NewNodeRepo(db, credKey)
		nodes, err := repo.GetAll(c.Request.Context())
		if err != nil {
			slog.Error("failed to list all nodes", "error", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": sanitizeError(err)})
			return
		}
		c.JSON(http.StatusOK, gin.H{"data": nodes})
	}
}

func CreateNode(db *sql.DB, credKey string) gin.HandlerFunc {
	return func(c *gin.Context) {
		var req struct {
			Name        string   `json:"name" binding:"required"`
			Description string   `json:"description"`
			Type        string   `json:"type" binding:"required"`
			City        string   `json:"city"`
			Country     string   `json:"country"`
			Lat         *float64 `json:"lat"`
			Lon         *float64 `json:"lon"`
			Active      bool     `json:"active"`
			EnabledCmds []string `json:"enabled_cmds"`
			AgentURL    string   `json:"agent_url"`
			AgentToken  string   `json:"agent_token"`
		}

		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}

		// V-05: Validate node type
		if req.Type != "standalone" && req.Type != "lg_node" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid node type. Must be 'standalone' or 'lg_node'"})
			return
		}
		if req.Type == "lg_node" && req.AgentURL != "" && !validAgentURL(req.AgentURL) {
			c.JSON(http.StatusBadRequest, gin.H{"error": "agent_url must be a valid HTTP or HTTPS URL"})
			return
		}

		enabledCmds := domain.NormalizeEnabledCmdStrings(req.EnabledCmds)
		if len(enabledCmds) == 0 {
			enabledCmds = domain.DefaultEnabledCmds()
		}

		node := &domain.Node{
			Name:        req.Name,
			Description: req.Description,
			Type:        domain.NodeType(req.Type),
			City:        req.City,
			Country:     req.Country,
			Lat:         req.Lat,
			Lon:         req.Lon,
			Active:      req.Active,
			EnabledCmds: enabledCmds,
			AgentURL:    req.AgentURL,
			AgentToken:  req.AgentToken,
		}

		repo := repo.NewNodeRepo(db, credKey)
		created, err := repo.Create(c.Request.Context(), node)
		if err != nil {
			slog.Error("failed to create node", "error", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": sanitizeError(err)})
			return
		}

		created.AgentToken = ""
		if err := refreshNodesCacheFn(db, credKey); err != nil {
			slog.Warn("failed to refresh node cache", "error", err)
		}
		c.JSON(http.StatusCreated, gin.H{"data": created})
	}
}

func UpdateNode(db *sql.DB, credKey string) gin.HandlerFunc {
	return func(c *gin.Context) {
		id, err := strconv.ParseInt(c.Param("id"), 10, 64)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
			return
		}

		var req struct {
			Name        string   `json:"name"`
			Description string   `json:"description"`
			Type        string   `json:"type"`
			City        string   `json:"city"`
			Country     string   `json:"country"`
			Lat         *float64 `json:"lat"`
			Lon         *float64 `json:"lon"`
			Active      *bool    `json:"active"`
			EnabledCmds []string `json:"enabled_cmds"`
			AgentURL    string   `json:"agent_url"`
			AgentToken  string   `json:"agent_token"`
		}

		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}

		repo := repo.NewNodeRepo(db, credKey)
		node, err := repo.GetByID(c.Request.Context(), id)
		if err != nil {
			c.JSON(http.StatusNotFound, gin.H{"error": "node not found"})
			return
		}

		if req.Name != "" {
			node.Name = req.Name
		}
		if req.Description != "" {
			node.Description = req.Description
		}
		if req.Type != "" {
			if req.Type != "standalone" && req.Type != "lg_node" {
				c.JSON(http.StatusBadRequest, gin.H{"error": "invalid node type. Must be 'standalone' or 'lg_node'"})
				return
			}
			node.Type = domain.NodeType(req.Type)
		}
		if req.City != "" {
			node.City = req.City
		}
		if req.Country != "" {
			node.Country = req.Country
		}
		if req.Lat != nil {
			node.Lat = req.Lat
		}
		if req.Lon != nil {
			node.Lon = req.Lon
		}
		if req.Active != nil {
			node.Active = *req.Active
		}
		if len(req.EnabledCmds) > 0 {
			enabledCmds := domain.NormalizeEnabledCmdStrings(req.EnabledCmds)
			if len(enabledCmds) == 0 {
				c.JSON(http.StatusBadRequest, gin.H{"error": "enabled_cmds must include at least one supported command"})
				return
			}
			node.EnabledCmds = enabledCmds
		}
		node.AgentURL = req.AgentURL
		if req.AgentToken != "" {
			node.AgentToken = req.AgentToken
		}
		if string(node.Type) == "lg_node" && node.AgentURL != "" && !validAgentURL(node.AgentURL) {
			c.JSON(http.StatusBadRequest, gin.H{"error": "agent_url must be a valid HTTP or HTTPS URL"})
			return
		}

		updated, err := updateNodeRecordFn(c.Request.Context(), repo, node)
		if err != nil {
			slog.Error("failed to update node", "error", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": sanitizeError(err)})
			return
		}

		if err := refreshNodesCacheFn(db, credKey); err != nil {
			slog.Warn("failed to refresh node cache", "error", err)
		}
		c.JSON(http.StatusOK, gin.H{"data": updated})
	}
}

func SetDefaultNode(db *sql.DB, credKey string) gin.HandlerFunc {
	return func(c *gin.Context) {
		id, err := strconv.ParseInt(c.Param("id"), 10, 64)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
			return
		}
		repo := repo.NewNodeRepo(db, credKey)
		if _, err := repo.GetByID(c.Request.Context(), id); err != nil {
			c.JSON(http.StatusNotFound, gin.H{"error": "node not found"})
			return
		}
		if err := setNodeDefaultFn(c.Request.Context(), repo, id); err != nil {
			slog.Error("failed to set default node", "error", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": sanitizeError(err)})
			return
		}
		node, err := getDefaultNodeAfterSet(c.Request.Context(), repo, id)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": sanitizeError(err)})
			return
		}
		node.AgentToken = ""
		if err := refreshNodesCacheFn(db, credKey); err != nil {
			slog.Warn("failed to refresh node cache", "error", err)
		}
		c.JSON(http.StatusOK, gin.H{"data": node})
	}
}

func DeleteNode(db *sql.DB, credKey string, bgpMgr *bgp.SessionManager) gin.HandlerFunc {
	return func(c *gin.Context) {
		id, err := strconv.ParseInt(c.Param("id"), 10, 64)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
			return
		}
		if err := removeBGPNeighborsForNode(c.Request.Context(), db, bgpMgr, id); err != nil {
			slog.Error("failed to remove bgp neighbors for node", "node_id", id, "error", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": sanitizeError(err)})
			return
		}
		repo := repo.NewNodeRepo(db, credKey)
		if err := repo.Delete(c.Request.Context(), id); err != nil {
			slog.Error("failed to delete node", "error", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": sanitizeError(err)})
			return
		}
		if err := refreshNodesCacheFn(db, credKey); err != nil {
			slog.Warn("failed to refresh node cache", "error", err)
		}
		c.JSON(http.StatusOK, gin.H{"data": "deleted"})
	}
}

func TestNode(db *sql.DB, credKey string, cfg *config.Config) gin.HandlerFunc {
	return func(c *gin.Context) {
		id, err := strconv.ParseInt(c.Param("id"), 10, 64)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
			return
		}
		repo := repo.NewNodeRepo(db, credKey)
		node, err := repo.GetByID(c.Request.Context(), id)
		if err != nil {
			c.JSON(http.StatusNotFound, gin.H{"error": "node not found"})
			return
		}

		if node.Type == domain.NodeTypeLGNode {
			if strings.TrimSpace(node.AgentURL) == "" {
				c.JSON(http.StatusOK, gin.H{
					"data": gin.H{
						"status":  "error",
						"message": "agent URL is not configured",
						"node_id": node.ID,
					},
				})
				return
			}
			if strings.TrimSpace(node.AgentToken) == "" {
				c.JSON(http.StatusOK, gin.H{
					"data": gin.H{
						"status":  "error",
						"message": "agent token is not configured",
						"node_id": node.ID,
					},
				})
				return
			}
		}
		if node.Type == domain.NodeTypeStandalone && strings.TrimSpace(node.AgentToken) == "" {
			c.JSON(http.StatusOK, gin.H{
				"data": gin.H{
					"status":  "error",
					"message": "agent token is not configured",
					"node_id": node.ID,
				},
			})
			return
		}

		// Create driver and test connection (same path as main-page queries)
		testCtx, cancel := context.WithTimeout(c.Request.Context(), 15*time.Second)
		defer cancel()

		drv, err := newQueryDriverFn(node, cfg)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to create driver"})
			return
		}

		if err := drv.TestConnection(testCtx); err != nil {
			slog.Warn("node test failed", "node_id", node.ID, "error", err)
			c.JSON(http.StatusOK, gin.H{
				"data": gin.H{
					"status":  "error",
					"message": friendlyConnError(err),
					"node_id": node.ID,
				},
			})
			return
		}

		c.JSON(http.StatusOK, gin.H{
			"data": gin.H{
				"status":  "ok",
				"message": "connection successful",
				"node_id": node.ID,
			},
		})
	}
}

func ListAudit(db *sql.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		filter := domain.AuditFilter{}

		if nodeIDStr := c.Query("node_id"); nodeIDStr != "" {
			if nodeID, err := strconv.ParseInt(nodeIDStr, 10, 64); err == nil {
				filter.NodeID = &nodeID
			}
		}
		filter.Command = c.Query("command")
		filter.SourceIP = c.Query("source_ip")
		filter.From = c.Query("from")
		filter.To = c.Query("to")

		if limitStr := c.Query("limit"); limitStr != "" {
			if limit, err := strconv.Atoi(limitStr); err == nil && limit > 0 {
				if limit > 200 {
					limit = 200
				}
				filter.Limit = limit
			}
		}
		if pageStr := c.Query("page"); pageStr != "" {
			if page, err := strconv.Atoi(pageStr); err == nil && page >= 0 {
				filter.Page = page
			}
		}
		if filter.Limit == 0 {
			filter.Limit = 50
		}

		repo := repo.NewAuditRepo(db)
		entries, total, err := repo.List(c.Request.Context(), filter)
		if err != nil {
			slog.Error("failed to list audit entries", "error", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": sanitizeError(err)})
			return
		}

		today, err := countTodayAuditFn(db, c.Request.Context())
		if err != nil {
			slog.Error("failed to count today's audit entries", "error", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": sanitizeError(err)})
			return
		}

		for i := range entries {
			entries[i].Params = domain.DisplayAuditParams(entries[i].Params)
		}

		c.JSON(http.StatusOK, gin.H{
			"data": entries,
			"meta": gin.H{
				"total": total,
				"today": today,
				"page":  filter.Page,
				"limit": filter.Limit,
			},
		})
	}
}

func ExportAudit(db *sql.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		repo := repo.NewAuditRepo(db)
		entries, _, err := repo.List(c.Request.Context(), domain.AuditFilter{Limit: 10000})
		if err != nil {
			slog.Error("failed to export audit entries", "error", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": sanitizeError(err)})
			return
		}

		c.Header("Content-Type", "text/csv")
		c.Header("Content-Disposition", "attachment;filename=audit_log.csv")

		writer := csv.NewWriter(c.Writer)

		writeRow := func(fields []string) {
			for i, f := range fields {
				// Prevent CSV formula injection when opened in spreadsheet software.
				if len(f) > 0 {
					switch f[0] {
					case '=', '+', '-', '@', '|', '%', '\t', '\r':
						fields[i] = "'" + f
					}
				}
			}
			writer.Write(fields) //nolint:errcheck // errors surface via writer.Error()
		}

		writeRow([]string{"ID", "Created At", "Source IP", "User ID", "Node ID", "Node Name", "Command", "Params", "Duration (ms)", "Success", "Error"})

		for _, e := range entries {
			success := "false"
			if e.Success {
				success = "true"
			}
			writeRow([]string{
				strconv.FormatInt(e.ID, 10),
				e.CreatedAt.Format(time.RFC3339),
				e.SourceIP,
				formatInt64Ptr(e.UserID),
				formatInt64Ptr(e.NodeID),
				e.NodeName,
				e.Command,
				domain.DisplayAuditParams(e.Params),
				strconv.FormatInt(e.DurationMS, 10),
				success,
				e.ErrorMsg,
			})
		}
		writer.Flush()
		if err := exportAuditFlushFn(writer); err != nil {
			slog.Error("csv flush error", "error", err)
		}
	}
}

func GetAccount(db *sql.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		userID, ok := c.Get("user_id")
		if !ok {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
			return
		}
		id, ok := userID.(int64)
		if !ok {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
			return
		}

		userRepo := repo.NewUserRepo(db)
		user, err := userRepo.GetByID(c.Request.Context(), id)
		if err != nil || user == nil {
			c.JSON(http.StatusNotFound, gin.H{"error": "user not found"})
			return
		}
		user.PasswordHash = ""
		c.JSON(http.StatusOK, gin.H{"data": user})
	}
}

func UpdateAccount(db *sql.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		userID, ok := c.Get("user_id")
		if !ok {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
			return
		}
		id, ok := userID.(int64)
		if !ok {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
			return
		}

		var req struct {
			Email           string `json:"email" binding:"required"`
			CurrentPassword string `json:"current_password" binding:"required"`
			NewPassword     string `json:"new_password" binding:"omitempty,min=8,max=128"`
		}
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}

		if _, err := mail.ParseAddress(req.Email); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid email format"})
			return
		}

		userRepo := repo.NewUserRepo(db)
		user, err := userRepo.GetByID(c.Request.Context(), id)
		if err != nil || user == nil {
			c.JSON(http.StatusNotFound, gin.H{"error": "user not found"})
			return
		}

		if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(req.CurrentPassword)); err != nil {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid current password"})
			return
		}

		email := strings.TrimSpace(req.Email)
		passwordHash := user.PasswordHash
		if req.NewPassword != "" {
			hashed, err := hashPassword(req.NewPassword)
			if err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to hash password"})
				return
			}
			passwordHash = hashed
		}

		if email == user.Email && passwordHash == user.PasswordHash {
			c.JSON(http.StatusBadRequest, gin.H{"error": "no changes to save"})
			return
		}

		if email != user.Email {
			existing, err := getUserByEmailForAccountFn(c.Request.Context(), userRepo, email)
			if err != nil {
				slog.Error("failed to check email uniqueness", "error", err)
				c.JSON(http.StatusInternalServerError, gin.H{"error": sanitizeError(err)})
				return
			}
			if existing != nil && existing.ID != id {
				c.JSON(http.StatusConflict, gin.H{"error": "email already in use"})
				return
			}
		}

		updated, err := updateAccountCredentialsFn(c.Request.Context(), userRepo, id, email, passwordHash)
		if err != nil {
			if strings.Contains(err.Error(), "UNIQUE constraint failed") {
				c.JSON(http.StatusConflict, gin.H{"error": "email already in use"})
				return
			}
			slog.Error("failed to update account", "error", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": sanitizeError(err)})
			return
		}

		updated.PasswordHash = ""
		c.JSON(http.StatusOK, gin.H{"data": updated})
	}
}

// MyIP returns the client's IP address with optional GeoIP enrichment.
// MaxMind databases are used when loaded; ASN/org/country fall back to Team Cymru DNS.
func MyIP(geoDB *geo.GeoIPDB) gin.HandlerFunc {
	return func(c *gin.Context) {
		ip := c.ClientIP()
		resp := gin.H{
			"ip": ip,
		}

		if geoDB == nil {
			c.JSON(http.StatusOK, gin.H{"data": resp})
			return
		}

		if geoDB.Enabled() {
			if city, err := geoDB.LookupCity(ip); err == nil {
				resp["city"] = city.City
				resp["country"] = city.Country
				resp["country_code"] = city.CountryISO
				resp["country_flag"] = city.CountryFlag
				resp["latitude"] = city.Latitude
				resp["longitude"] = city.Longitude
				resp["timezone"] = city.TimeZone
			}
		}

		if asn, err := geoDB.ResolveASN(c.Request.Context(), ip); err == nil && asn != nil && asn.ASN > 0 {
			resp["asn"] = asn.ASN
			if org := strings.TrimSpace(asn.OrgName); org != "" {
				resp["asn_org"] = org
			}
			if _, hasCountry := resp["country_code"]; !hasCountry {
				if cc := strings.TrimSpace(asn.CountryCode); cc != "" {
					resp["country_code"] = cc
					resp["country_flag"] = geo.CountryToFlag(cc)
				}
			}
		}

		c.JSON(http.StatusOK, gin.H{"data": resp})
	}
}

// ListPublicCommunities returns active community rules for the public communities page.
func ListPublicCommunities(db *sql.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		_ = db
		c.JSON(http.StatusOK, gin.H{"data": sitecache.ActiveCommunities()})
	}
}

// Community Rules handlers
func ListCommunityRules(db *sql.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		repo := repo.NewCommunityRuleRepo(db)
		rules, err := repo.GetAll(c.Request.Context())
		if err != nil {
			slog.Error("failed to list community rules", "error", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": sanitizeError(err)})
			return
		}
		c.JSON(http.StatusOK, gin.H{"data": rules})
	}
}

func CreateCommunityRule(db *sql.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		var req struct {
			Community   string `json:"community" binding:"required"`
			Severity    string `json:"severity"`
			MessageI18n string `json:"message_i18n"`
			Scope       string `json:"scope"`
			Active      bool   `json:"active"`
		}
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		validSeverities := map[domain.Severity]bool{
			domain.SeverityAlert: true, domain.SeverityWarning: true,
			domain.SeverityInfo: true, domain.SeveritySuccess: true,
		}
		severity := domain.NormalizeSeverity(domain.Severity(req.Severity))
		if severity == "" {
			severity = domain.SeverityInfo
		} else if !validSeverities[severity] {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid severity: must be alert, warning, info, or success"})
			return
		}
		scope := req.Scope
		if scope == "" {
			scope = "global"
		} else if scope != "global" && scope != "node" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid scope: must be 'global' or 'node'"})
			return
		}
		rule := &domain.CommunityRule{
			Community:   req.Community,
			Severity:    severity,
			MessageI18n: req.MessageI18n,
			Scope:       scope,
			Active:      req.Active,
		}
		repo := repo.NewCommunityRuleRepo(db)
		created, err := repo.Create(c.Request.Context(), rule)
		if err != nil {
			slog.Error("failed to create community rule", "error", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": sanitizeError(err)})
			return
		}
		if err := refreshCommunitiesCacheFn(db); err != nil {
			slog.Warn("failed to refresh community cache", "error", err)
		}
		c.JSON(http.StatusCreated, gin.H{"data": created})
	}
}

func UpdateCommunityRule(db *sql.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		id, err := strconv.ParseInt(c.Param("id"), 10, 64)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
			return
		}
		var req struct {
			Community   string `json:"community" binding:"required"`
			Severity    string `json:"severity"`
			MessageI18n string `json:"message_i18n"`
			Scope       string `json:"scope"`
			Active      bool   `json:"active"`
		}
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		validSeverities := map[domain.Severity]bool{
			domain.SeverityAlert: true, domain.SeverityWarning: true,
			domain.SeverityInfo: true, domain.SeveritySuccess: true,
		}
		severity := domain.NormalizeSeverity(domain.Severity(req.Severity))
		if severity == "" {
			severity = domain.SeverityInfo
		} else if !validSeverities[severity] {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid severity: must be alert, warning, info, or success"})
			return
		}
		scope := req.Scope
		if scope == "" {
			scope = "global"
		} else if scope != "global" && scope != "node" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid scope: must be 'global' or 'node'"})
			return
		}
		rule := &domain.CommunityRule{
			ID:          id,
			Community:   req.Community,
			Severity:    severity,
			MessageI18n: req.MessageI18n,
			Scope:       scope,
			Active:      req.Active,
		}
		repo := repo.NewCommunityRuleRepo(db)
		updated, err := repo.Update(c.Request.Context(), rule)
		if err != nil {
			slog.Error("failed to update community rule", "error", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": sanitizeError(err)})
			return
		}
		if err := refreshCommunitiesCacheFn(db); err != nil {
			slog.Warn("failed to refresh community cache", "error", err)
		}
		c.JSON(http.StatusOK, gin.H{"data": updated})
	}
}

func DeleteCommunityRule(db *sql.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		id, err := strconv.ParseInt(c.Param("id"), 10, 64)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
			return
		}
		repo := repo.NewCommunityRuleRepo(db)
		if err := repo.Delete(c.Request.Context(), id); err != nil {
			slog.Error("failed to delete community rule", "error", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": sanitizeError(err)})
			return
		}
		if err := refreshCommunitiesCacheFn(db); err != nil {
			slog.Warn("failed to refresh community cache", "error", err)
		}
		c.JSON(http.StatusOK, gin.H{"data": "deleted"})
	}
}

func ToggleCommunityRule(db *sql.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		id, err := strconv.ParseInt(c.Param("id"), 10, 64)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
			return
		}
		repo := repo.NewCommunityRuleRepo(db)
		if err := repo.Toggle(c.Request.Context(), id); err != nil {
			slog.Error("failed to toggle community rule", "error", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": sanitizeError(err)})
			return
		}
		if err := refreshCommunitiesCacheFn(db); err != nil {
			slog.Warn("failed to refresh community cache", "error", err)
		}
		c.JSON(http.StatusOK, gin.H{"data": "toggled"})
	}
}

// Helper functions
func generateJWT(userID int64, role string, secret string) (string, error) {
	return generateJWTForLogin(userID, role, secret)
}

var activeNodesLister = sitecache.ActiveNodes

var (
	refreshNodesCacheFn       = sitecache.RefreshNodes
	refreshSettingsCacheFn    = sitecache.RefreshSettings
	refreshCommunitiesCacheFn = sitecache.RefreshCommunities
	newQueryDriverFn          = driver.NewDriver
)

var handlerEngineExecute = func(h *Handler, ctx context.Context, query *domain.Query, opts ...engine.ExecuteOption) (*domain.QueryResult, error) {
	return h.engine.Execute(ctx, query, opts...)
}

var submitQueryAsyncDone func()

var (
	uploadLogoMkdirAll     = os.MkdirAll
	uploadLogoCreate       = os.Create
	uploadLogoCopy         = io.Copy
	uploadLogoReadFn       = func(r io.Reader, p []byte) (int, error) { return r.Read(p) }
	uploadLogoSeekFn       = func(s io.Seeker, offset int64, whence int) (int64, error) { return s.Seek(offset, whence) }
	uploadLogoReadAllFn    = func(r io.Reader) ([]byte, error) { return io.ReadAll(r) }
	uploadLogoSetSettingFn = func(db *sql.DB, key, value string) error {
		return queries.New(db).SetSetting(key, value)
	}
)

var updateNodeRecordFn = func(ctx context.Context, r domain.NodeRepository, node *domain.Node) (*domain.Node, error) {
	return r.Update(ctx, node)
}

var countTodayAuditFn = func(db *sql.DB, ctx context.Context) (int, error) {
	return repo.NewAuditRepo(db).CountToday(ctx)
}

var getUserByEmailForAccountFn = func(ctx context.Context, userRepo domain.UserRepository, email string) (*domain.User, error) {
	return userRepo.GetByEmail(ctx, email)
}

var updateAccountCredentialsFn = func(ctx context.Context, userRepo domain.UserRepository, id int64, email, passwordHash string) (*domain.User, error) {
	return userRepo.UpdateCredentials(ctx, id, email, passwordHash)
}

var exportAuditFlushFn = func(w *csv.Writer) error {
	w.Flush()
	return w.Error()
}

var streamResultPollInterval = 2 * time.Second

var streamResultMaxDuration = 60 * time.Second

var asPathEnrichmentTimeUntilFn = time.Until

var loginUpdateLastLogin = func(ctx context.Context, userRepo domain.UserRepository, userID int64) error {
	return userRepo.UpdateLastLogin(ctx, userID)
}

var generateJWTForLogin = func(userID int64, role string, secret string) (string, error) {
	claims := jwt.MapClaims{
		"user_id": userID,
		"role":    role,
		"jti":     uuid.NewString(),
		"exp":     time.Now().Add(24 * time.Hour).Unix(),
		"iat":     time.Now().Unix(),
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString([]byte(secret))
}

func hashPassword(password string) (string, error) {
	return hashPasswordFn(password)
}

var hashPasswordFn = func(password string) (string, error) {
	bytes, err := bcrypt.GenerateFromPassword([]byte(password), 12)
	return string(bytes), err
}

var getDefaultNodeAfterSet = func(ctx context.Context, repo domain.NodeRepository, id int64) (*domain.Node, error) {
	return repo.GetByID(ctx, id)
}

var setNodeDefaultFn = func(ctx context.Context, repo domain.NodeRepository, id int64) error {
	return repo.SetDefault(ctx, id)
}

func formatInt64Ptr(v *int64) string {
	if v == nil {
		return ""
	}
	return strconv.FormatInt(*v, 10)
}

func settingInt(settings map[string]string, key string, defaultVal, min, max int) int {
	raw, ok := settings[key]
	if !ok {
		return defaultVal
	}
	n, err := strconv.Atoi(strings.TrimSpace(raw))
	if err != nil || n < min {
		return defaultVal
	}
	if n > max {
		return max
	}
	return n
}

func GetPublicSettings(db *sql.DB, bgpCfg config.BGPConfig) gin.HandlerFunc {
	return func(c *gin.Context) {
		_ = db
		_ = bgpCfg
		c.JSON(http.StatusOK, gin.H{"data": sitecache.PublicSettings()})
	}
}

// secretSettingKeys never leave the server. The admin panel is told whether one is stored
// through a dedicated status endpoint instead — the same treatment node agent tokens get.
var secretSettingKeys = []string{geo.SettingLicenseKey}

func GetAdminSettings(db *sql.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		q := queries.New(db)
		settings, err := q.GetSettings()
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to load settings"})
			return
		}
		enrichSettingsLogoPath(settings)
		for _, key := range secretSettingKeys {
			delete(settings, key)
		}
		c.JSON(http.StatusOK, gin.H{"data": settings})
	}
}

var allowedSettingKeys = map[string]bool{
	"site_name": true, "site_description": true, "logo_path": true, "header_color": true,
	"url_website": true, "url_peeringdb": true, "url_contact": true, "url_terms": true, "url_privacy": true,
	"ping_count": true, "max_hops": true, "traceroute_max_timeouts": true, "active_languages": true,
}

func UpdateSettings(db *sql.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		var req map[string]string
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		filtered := make(map[string]string, len(req))
		for k, v := range req {
			if allowedSettingKeys[k] {
				filtered[k] = v
			}
		}
		if logoPath, ok := filtered["logo_path"]; ok && strings.TrimSpace(logoPath) == "" {
			removeLogoFiles()
		}
		q := queries.New(db)
		if err := q.SetSettings(filtered); err != nil {
			slog.Error("failed to update settings", "error", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to update settings"})
			return
		}
		if err := refreshSettingsCacheFn(db, 0); err != nil {
			slog.Warn("failed to refresh settings cache", "error", err)
		}
		settings := sitecache.AllSettings()
		enrichSettingsLogoPath(settings)
		c.JSON(http.StatusOK, gin.H{"data": settings})
	}
}

func UploadLogo(db *sql.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		file, header, err := c.Request.FormFile("logo")
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "no file uploaded"})
			return
		}
		defer file.Close()

		if header.Size > 2*1024*1024 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "file too large (max 2MB)"})
			return
		}

		buf := make([]byte, 512)
		n, err := uploadLogoReadFn(file, buf)
		if err != nil && err != io.EOF {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to read file"})
			return
		}
		mime := http.DetectContentType(buf[:n])

		// http.DetectContentType cannot detect SVG reliably; check content
		if (strings.HasPrefix(mime, "text/xml") || strings.HasPrefix(mime, "application/xml") || strings.HasPrefix(mime, "text/plain")) && bytes.Contains(buf[:n], []byte("<svg")) {
			mime = "image/svg+xml"
		}

		if mime != "image/png" && mime != "image/jpeg" && mime != "image/svg+xml" && mime != "image/webp" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid file type (allowed: png, jpeg, svg, webp)"})
			return
		}
		if _, err := uploadLogoSeekFn(file, 0, io.SeekStart); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to read file"})
			return
		}

		if mime == "image/svg+xml" {
			all, err := uploadLogoReadAllFn(file)
			if err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to read file"})
				return
			}
			lower := strings.ToLower(string(all))
			if svgEventHandlers.MatchString(lower) ||
				strings.Contains(lower, "<script") ||
				strings.Contains(lower, "javascript:") ||
				strings.Contains(lower, "<foreignobject") ||
				strings.Contains(lower, "data:") ||
				svgExternalRef.MatchString(lower) {
				c.JSON(http.StatusBadRequest, gin.H{"error": "SVG contains disallowed content"})
				return
			}
			if _, err := uploadLogoSeekFn(file, 0, io.SeekStart); err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to read file"})
				return
			}
		}

		ext := ".png"
		switch mime {
		case "image/jpeg":
			ext = ".jpg"
		case "image/svg+xml":
			ext = ".svg"
		case "image/webp":
			ext = ".webp"
		}

		if err := uploadLogoMkdirAll(LogoUploadsDir(), 0o755); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to create upload directory"})
			return
		}
		for _, oldExt := range []string{".png", ".jpg", ".svg", ".webp"} {
			if oldExt != ext {
				os.Remove(filepath.Join(LogoUploadsDir(), "logo"+oldExt))
			}
		}
		outPath := filepath.Join(LogoUploadsDir(), "logo"+ext)
		out, err := uploadLogoCreate(outPath)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to save logo"})
			return
		}
		defer out.Close()

		if _, err := uploadLogoCopy(out, file); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to save logo"})
			return
		}

		logoPath := "/logo" + ext
		if err := uploadLogoSetSettingFn(db, "logo_path", logoPath); err != nil {
			slog.Error("failed to persist logo path", "error", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "logo saved but settings update failed"})
			return
		}
		if err := refreshSettingsCacheFn(db, 0); err != nil {
			slog.Warn("failed to refresh settings cache", "error", err)
		}

		c.JSON(http.StatusOK, gin.H{"data": gin.H{"logo_path": logoPathWithCacheBuster(logoPath)}})
	}
}

var ipRegex = regexp.MustCompile(`\b(\d{1,3}\.\d{1,3}\.\d{1,3}\.\d{1,3})\b`)

var (
	tracerouteHopRe    = regexp.MustCompile(`^(\s*\d+)\s+(.+)$`)
	tracerouteHostIPRe = regexp.MustCompile(`(\S+?)\s+\((\d{1,3}\.\d{1,3}\.\d{1,3}\.\d{1,3})\)`)
)

// tracerouteLineIP returns the responder IP from a traceroute hop line.
// Prefer the address in parentheses (actual probe target) over dotted quads
// embedded in reverse-DNS hostnames like 166.103.192.193.static.turk.net.
func tracerouteLineIP(line string) string {
	if m := tracerouteHostIPRe.FindStringSubmatch(line); len(m) >= 3 {
		return m[2]
	}
	if m := ipRegex.FindStringSubmatch(line); len(m) >= 2 {
		return m[1]
	}
	return ""
}

// svgEventHandlers matches any on* attribute (e.g. onload=, onbegin=, onclick=).
// Applied to lowercased SVG content to catch mixed-case bypasses.
var svgEventHandlers = regexp.MustCompile(`\bon[a-z]+=`)

// svgExternalRef matches external URL references in href/src/xlink:href attributes.
var svgExternalRef = regexp.MustCompile(`(?:x?link:href|href|src)\s*=\s*["']https?://`)

func mergeASPathFields(dest, stored *domain.QueryResult) {
	if dest == nil || stored == nil {
		return
	}
	if len(dest.ASPath) == 0 && len(stored.ASPath) > 0 {
		dest.ASPath = stored.ASPath
	}
	if dest.ASPathPrefix == "" && stored.ASPathPrefix != "" {
		dest.ASPathPrefix = stored.ASPathPrefix
	}
	if len(dest.ASPathNodes) == 0 && len(stored.ASPathNodes) > 0 {
		dest.ASPathNodes = stored.ASPathNodes
	}
	if len(stored.ASPathEnriched) == 0 {
		return
	}
	if len(stored.ASPathEnriched) > len(dest.ASPathEnriched) || !asPathEnrichedHasLabels(dest.ASPathEnriched) {
		dest.ASPathEnriched = stored.ASPathEnriched
	}
}

func asPathEnrichedHasLabels(entries []domain.ASInfo) bool {
	for _, e := range entries {
		if strings.TrimSpace(e.OrgName) != "" || strings.TrimSpace(e.ShortName) != "" {
			return true
		}
	}
	return false
}

func skipASPathWaitForBGP(command string, result *domain.QueryResult) bool {
	if command != string(domain.CmdBGPRoute) || result == nil {
		return false
	}
	br, ok := result.Parsed.(*domain.BGPResult)
	if !ok || br == nil {
		return false
	}
	return len(br.Routes) == 0
}

func waitForASPathEnrichment(queryID string, timeout time.Duration) {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		stored, ok := queryStore.Get(queryID)
		if ok && asPathEnrichmentReady(stored) {
			return
		}
		wait := asPathEnrichmentTimeUntilFn(deadline)
		if wait <= 0 {
			return
		}
		if wait > 100*time.Millisecond {
			wait = 100 * time.Millisecond
		}
		ch := queryStore.NotifyCh(queryID)
		select {
		case <-ch:
		case <-time.After(wait):
		}
	}
}

func asPathEnrichmentReady(r *domain.QueryResult) bool {
	if r == nil || len(r.ASPath) == 0 {
		return true
	}
	need := uniqueASNsInPath(r.ASPath)
	if need == 0 {
		return true
	}
	return len(r.ASPathEnriched) >= need
}

func uniqueASNsInPath(path []uint32) int {
	seen := make(map[uint32]struct{})
	for _, asn := range path {
		if asn == 0 {
			continue
		}
		seen[asn] = struct{}{}
	}
	return len(seen)
}

func enrichLineWithAS(ctx context.Context, geoDB interface {
	ResolveASN(context.Context, string) (*domain.ASInfo, error)
}, line string) string {
	ip := tracerouteLineIP(line)
	if ip == "" {
		return line
	}
	return line + asSuffixForIP(ctx, geoDB, ip)
}

// formatTracerouteLine splits ECMP hops (multiple responder IPs on one hop) into
// indented sub-lines and attaches AS info per probe instead of one tag at the end.
func formatTracerouteLine(ctx context.Context, geoDB interface {
	ResolveASN(context.Context, string) (*domain.ASInfo, error)
}, line string) string {
	trimmed := strings.TrimSpace(line)
	if trimmed == "" || strings.HasPrefix(trimmed, "traceroute to") || strings.HasPrefix(trimmed, "Start") {
		return line
	}

	m := tracerouteHopRe.FindStringSubmatch(line)
	if m == nil {
		return enrichLineWithAS(ctx, geoDB, line)
	}

	hopPrefix := m[1]
	body := strings.TrimSpace(m[2])
	probes := splitTracerouteProbes(body)
	if len(probes) <= 1 || len(distinctPublicIPs(probes)) <= 1 {
		return enrichLineWithAS(ctx, geoDB, line)
	}

	pad := strings.Repeat(" ", len(hopPrefix)+2)
	var b strings.Builder
	for i, probe := range probes {
		segment := enrichProbeSegment(ctx, geoDB, probe)
		if i == 0 {
			b.WriteString(hopPrefix)
			b.WriteString("  ")
			b.WriteString(segment)
		} else {
			b.WriteString("\n")
			b.WriteString(pad)
			b.WriteString(segment)
		}
	}
	return b.String()
}

func splitTracerouteProbes(body string) []string {
	body = strings.TrimSpace(body)
	if body == "" {
		return nil
	}
	locs := tracerouteHostIPRe.FindAllStringSubmatchIndex(body, -1)
	if len(locs) == 0 {
		return []string{body}
	}
	out := make([]string, 0, len(locs))
	for i, loc := range locs {
		start := loc[0]
		end := len(body)
		if i+1 < len(locs) {
			end = locs[i+1][0]
		}
		if segment := strings.TrimSpace(body[start:end]); segment != "" {
			out = append(out, segment)
		}
	}
	return out
}

func distinctPublicIPs(segments []string) []string {
	seen := make(map[string]struct{})
	var ips []string
	for _, seg := range segments {
		ip := tracerouteLineIP(seg)
		if ip == "" {
			continue
		}
		parsed := net.ParseIP(ip)
		if parsed == nil || parsed.IsPrivate() || parsed.IsLoopback() || parsed.IsLinkLocalUnicast() {
			continue
		}
		if _, ok := seen[ip]; ok {
			continue
		}
		seen[ip] = struct{}{}
		ips = append(ips, ip)
	}
	return ips
}

func enrichProbeSegment(ctx context.Context, geoDB interface {
	ResolveASN(context.Context, string) (*domain.ASInfo, error)
}, segment string) string {
	ip := tracerouteLineIP(segment)
	if ip == "" {
		return segment
	}
	return segment + asSuffixForIP(ctx, geoDB, ip)
}

func asSuffixForIP(ctx context.Context, geoDB interface {
	ResolveASN(context.Context, string) (*domain.ASInfo, error)
}, ip string) string {
	parsed := net.ParseIP(ip)
	if parsed == nil || parsed.IsPrivate() || parsed.IsLoopback() || parsed.IsLinkLocalUnicast() {
		return ""
	}
	info, err := geoDB.ResolveASN(ctx, ip)
	if err != nil || info == nil || info.ASN == 0 {
		return ""
	}
	sanitizeName := func(s string) string {
		return strings.Map(func(r rune) rune {
			if r == '\n' || r == '\r' || r < 32 {
				return -1
			}
			return r
		}, s)
	}
	suffix := " [AS" + strconv.FormatUint(uint64(info.ASN), 10) + " -"
	name := sanitizeName(geo.FormatTracerouteOrgName(info.OrgName))
	if name == "" {
		name = sanitizeName(geo.FormatTracerouteOrgName(info.ShortName))
	}
	if name != "" {
		suffix += " " + name
	}
	suffix += "]"
	return suffix
}
