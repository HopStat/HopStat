package handler

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/csv"
	"encoding/json"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/mail"
	"os"
	"regexp"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"
	"github.com/HopStat/HopStat/internal/config"
	"github.com/HopStat/HopStat/internal/domain"
	"github.com/HopStat/HopStat/internal/driver"
	"github.com/HopStat/HopStat/internal/engine"
	"github.com/HopStat/HopStat/internal/geo"
	"github.com/HopStat/HopStat/internal/store/queries"
	"github.com/HopStat/HopStat/internal/store/querystore"
	"github.com/HopStat/HopStat/internal/store/repo"
)

var queryStore = querystore.New()

type Handler struct {
	db       *sql.DB
	cfg      *config.Config
	engine   *engine.QueryEngine
	geoDB    *geo.GeoIPDB
	nodeRepo domain.NodeRepository
}

func New(db *sql.DB, cfg *config.Config, geoDB *geo.GeoIPDB) *Handler {
	h := &Handler{
		db:       db,
		cfg:      cfg,
		geoDB:    geoDB,
		nodeRepo: repo.NewNodeRepo(db),
	}
	h.engine = engine.New(&engine.QueryConfig{
		MaxConcurrent:        cfg.Query.MaxConcurrent,
		DefaultTimeoutSec:    cfg.Query.DefaultTimeoutSec,
		MTRTimeoutSec:        cfg.Query.MTRTimeoutSec,
		TracerouteTimeoutSec: cfg.Query.TracerouteTimeoutSec,
	}, h.nodeRepo, h.geoDB, nil)
	return h
}

func sanitizeError(err error) string {
	return "internal error"
}

func ListNodes(db *sql.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		repo := repo.NewNodeRepo(db)
		nodes, err := repo.GetActive(c.Request.Context())
		if err != nil {
			slog.Error("failed to list active nodes", "error", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": sanitizeError(err)})
			return
		}
		for _, n := range nodes {
			n.AgentToken = ""
		}
		c.JSON(http.StatusOK, gin.H{"data": nodes})
	}
}

func GetNode(db *sql.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		id, err := strconv.ParseInt(c.Param("id"), 10, 64)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid node id"})
			return
		}
		repo := repo.NewNodeRepo(db)
		node, err := repo.GetByID(c.Request.Context(), id)
		if err != nil {
			c.JSON(http.StatusNotFound, gin.H{"error": "node not found"})
			return
		}
		node.AgentToken = ""
		c.JSON(http.StatusOK, gin.H{"data": node})
	}
}

func SubmitQuery(db *sql.DB, cfg *config.Config, geoDB *geo.GeoIPDB) gin.HandlerFunc {
	h := New(db, cfg, geoDB)
	return func(c *gin.Context) {
		var req struct {
			NodeID   int64  `json:"node_id" binding:"required"`
			Command  string `json:"command" binding:"required"`
			Target   string `json:"target" binding:"required"`
			Protocol string `json:"protocol"`
			Options  struct {
				PingCount int `json:"ping_count"`
				MaxHops   int `json:"max_hops"`
				MTRCycles int `json:"mtr_cycles"`
			} `json:"options"`
		}

		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}

		// V-01: Validate command type
		validCmds := map[string]bool{
			"ping": true, "traceroute": true, "mtr": true,
			"bgp_route": true, "as_path": true,
		}
		if !validCmds[req.Command] {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid command. Must be one of: ping, traceroute, mtr, bgp_route, as_path"})
			return
		}

		// V-01b: Check node's enabled commands
		{
			nodeRepo := repo.NewNodeRepo(db)
			node, err := nodeRepo.GetByID(c.Request.Context(), req.NodeID)
			if err != nil {
				c.JSON(http.StatusNotFound, gin.H{"error": "node not found"})
				return
			}
			if len(node.EnabledCmds) > 0 && !node.CanExecute(domain.CommandType(req.Command)) {
				c.JSON(http.StatusBadRequest, gin.H{"error": "command not enabled for this node"})
				return
			}
		}

		if req.Options.PingCount <= 0 {
			req.Options.PingCount = 5
		}
		if req.Options.MaxHops <= 0 {
			req.Options.MaxHops = 30
		}
		if req.Options.MTRCycles <= 0 {
			req.Options.MTRCycles = 10
		}

		// V-02: Enforce upper bounds
		if req.Options.PingCount > 20 {
			req.Options.PingCount = 20
		}
		if req.Options.MaxHops > 64 {
			req.Options.MaxHops = 64
		}
		if req.Options.MTRCycles > 100 {
			req.Options.MTRCycles = 100
		}

		queryID := uuid.New().String()

		query := &domain.Query{
			ID:        queryID,
			NodeID:    req.NodeID,
			Command:   domain.CommandType(req.Command),
			Target:    req.Target,
			SourceIP:  c.ClientIP(),
			Options: domain.QueryOptions{
				PingCount: req.Options.PingCount,
				MaxHops:   req.Options.MaxHops,
				MTRCycles: req.Options.MTRCycles,
				Protocol:  req.Protocol,
			},
			CreatedAt: time.Now(),
		}

		// Log query to audit
		paramsJSON, _ := json.Marshal(map[string]string{"target": req.Target})
		auditRepo := repo.NewAuditRepo(db)
		auditEntry := &domain.AuditEntry{
			SourceIP:  c.ClientIP(),
			NodeID:    &req.NodeID,
			Command:   req.Command,
			Params:    string(paramsJSON),
			CreatedAt: time.Now(),
		}
		go auditRepo.Log(context.Background(), auditEntry)

		// Store running entry immediately so SSE can start streaming
		queryStore.SetRunning(queryID)

		// Execute query asynchronously with line streaming
		go func() {
			var consecutiveTimeouts int64
			const maxTimeouts = 5

			result, err := h.engine.Execute(context.Background(), query, engine.ExecuteOption{
				OnLine: func(line string) {
					enriched := line

					// Inline AS enrichment for traceroute/MTR hop lines (skip header)
					isHeader := strings.HasPrefix(strings.TrimSpace(line), "traceroute to") || strings.HasPrefix(strings.TrimSpace(line), "Start")
					if h.geoDB != nil && (req.Command == "traceroute" || req.Command == "mtr") && !isHeader {
						enriched = enrichLineWithAS(context.Background(), h.geoDB, line)
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

					queryStore.AppendLine(queryID, enriched)
				},
				ShouldStop: func() bool {
					return atomic.LoadInt64(&consecutiveTimeouts) >= maxTimeouts
				},
			})
			if err != nil {
				slog.Error("query failed", "error", err, "query_id", queryID)
			}
			queryStore.Set(queryID, result)
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

		for i := 0; i < 600; i++ { // 600 * 100ms = 60s max
			// Stream new output lines
			lines, _ := queryStore.GetLines(queryID)
			for idx := lastLineIdx; idx < len(lines); idx++ {
				data, _ := json.Marshal(gin.H{"line": lines[idx]})
				c.Writer.WriteString("event: output\ndata: " + string(data) + "\n\n")
			}
			if len(lines) > lastLineIdx {
				lastLineIdx = len(lines)
				if canFlush {
					flusher.Flush()
				}
			}

			// Check for final result
			result, ok := queryStore.Get(queryID)
			if ok && (result.Status == domain.StatusDone || result.Status == domain.StatusError) {
				data, _ := json.Marshal(result)
				c.Writer.WriteString("event: result\ndata: " + string(data) + "\n\n")
				if canFlush {
					flusher.Flush()
				}
				return
			}

			select {
			case <-c.Request.Context().Done():
				return
			case <-time.After(100 * time.Millisecond):
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

		// A-06: Update last login timestamp
		go userRepo.UpdateLastLogin(context.Background(), user.ID)

		token, err := generateJWT(user.ID, user.Role, cfg.Security.JWTSecret)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to generate token"})
			return
		}

		c.JSON(http.StatusOK, gin.H{
			"data": gin.H{
				"token":      token,
				"expires_at": time.Now().Add(24 * time.Hour).Format(time.RFC3339),
			},
		})
	}
}

func Logout() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"data": "logged out"})
	}
}

// Admin handlers
func ListAllNodes(db *sql.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		repo := repo.NewNodeRepo(db)
		nodes, err := repo.GetAll(c.Request.Context())
		if err != nil {
			slog.Error("failed to list all nodes", "error", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": sanitizeError(err)})
			return
		}
		c.JSON(http.StatusOK, gin.H{"data": nodes})
	}
}

func CreateNode(db *sql.DB) gin.HandlerFunc {
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
		if req.Type == "lg_node" && req.AgentURL != "" && !strings.HasPrefix(req.AgentURL, "https://") {
			c.JSON(http.StatusBadRequest, gin.H{"error": "agent_url must use HTTPS to protect the agent token"})
			return
		}

		validCmdSet := map[string]bool{
			"ping": true, "traceroute": true, "mtr": true, "bgp_route": true, "as_path": true,
		}
		var enabledCmds []domain.CommandType
		for _, cmd := range req.EnabledCmds {
			if !validCmdSet[cmd] {
				c.JSON(http.StatusBadRequest, gin.H{"error": "invalid command in enabled_cmds: " + cmd})
				return
			}
			enabledCmds = append(enabledCmds, domain.CommandType(cmd))
		}
		if len(enabledCmds) == 0 {
			enabledCmds = []domain.CommandType{domain.CmdPing, domain.CmdTraceroute, domain.CmdBGPRoute, domain.CmdASPath}
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

		repo := repo.NewNodeRepo(db)
		created, err := repo.Create(c.Request.Context(), node)
		if err != nil {
			slog.Error("failed to create node", "error", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": sanitizeError(err)})
			return
		}

		created.AgentToken = ""
		c.JSON(http.StatusCreated, gin.H{"data": created})
	}
}

func UpdateNode(db *sql.DB) gin.HandlerFunc {
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

		repo := repo.NewNodeRepo(db)
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
			validCmdSet := map[string]bool{
				"ping": true, "traceroute": true, "mtr": true, "bgp_route": true, "as_path": true,
			}
			var enabledCmds []domain.CommandType
			for _, cmd := range req.EnabledCmds {
				if !validCmdSet[cmd] {
					c.JSON(http.StatusBadRequest, gin.H{"error": "invalid command in enabled_cmds: " + cmd})
					return
				}
				enabledCmds = append(enabledCmds, domain.CommandType(cmd))
			}
			node.EnabledCmds = enabledCmds
		}
		if req.AgentURL != "" {
			node.AgentURL = req.AgentURL
		}
		if req.AgentToken != "" {
			node.AgentToken = req.AgentToken
		}
		if string(node.Type) == "lg_node" && node.AgentURL != "" && !strings.HasPrefix(node.AgentURL, "https://") {
			c.JSON(http.StatusBadRequest, gin.H{"error": "agent_url must use HTTPS to protect the agent token"})
			return
		}

		updated, err := repo.Update(c.Request.Context(), node)
		if err != nil {
			slog.Error("failed to update node", "error", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": sanitizeError(err)})
			return
		}

		updated.AgentToken = ""
		c.JSON(http.StatusOK, gin.H{"data": updated})
	}
}

func DeleteNode(db *sql.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		id, err := strconv.ParseInt(c.Param("id"), 10, 64)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
			return
		}
		repo := repo.NewNodeRepo(db)
		if err := repo.Delete(c.Request.Context(), id); err != nil {
			slog.Error("failed to delete node", "error", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": sanitizeError(err)})
			return
		}
		c.JSON(http.StatusOK, gin.H{"data": "deleted"})
	}
}

func TestNode(db *sql.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		id, err := strconv.ParseInt(c.Param("id"), 10, 64)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
			return
		}
		repo := repo.NewNodeRepo(db)
		node, err := repo.GetByID(c.Request.Context(), id)
		if err != nil {
			c.JSON(http.StatusNotFound, gin.H{"error": "node not found"})
			return
		}

		// Create driver and test connection
		drv, err := driver.NewDriver(node, nil)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to create driver"})
			return
		}

		testCtx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
		defer cancel()

		if err := drv.TestConnection(testCtx); err != nil {
			slog.Warn("node test failed", "node_id", node.ID, "error", err)
			c.JSON(http.StatusOK, gin.H{
				"data": gin.H{
					"status":  "error",
					"message": sanitizeError(err),
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

		c.JSON(http.StatusOK, gin.H{
			"data": entries,
			"meta": gin.H{
				"total": total,
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

		// Write header
		writer.Write([]string{"ID", "Created At", "Source IP", "User ID", "Node ID", "Command", "Params", "Duration (ms)", "Success", "Error"})

		// Write data
		for _, e := range entries {
			success := "false"
			if e.Success {
				success = "true"
			}
			writer.Write([]string{
				strconv.FormatInt(e.ID, 10),
				e.CreatedAt.Format(time.RFC3339),
				e.SourceIP,
				formatInt64Ptr(e.UserID),
				formatInt64Ptr(e.NodeID),
				e.Command,
				e.Params,
				strconv.FormatInt(e.DurationMS, 10),
				success,
				e.ErrorMsg,
			})
		}
		writer.Flush()
	}
}

func ListUsers(db *sql.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		repo := repo.NewUserRepo(db)
		users, err := repo.List(c.Request.Context())
		if err != nil {
			slog.Error("failed to list users", "error", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": sanitizeError(err)})
			return
		}
		// SEC-03: Clear password hashes before returning
		for _, u := range users {
			u.PasswordHash = ""
		}
		c.JSON(http.StatusOK, gin.H{"data": users})
	}
}

func CreateUser(db *sql.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		var req struct {
			Email    string `json:"email" binding:"required"`
			Password string `json:"password" binding:"required,min=8,max=128"`
			Role     string `json:"role"`
		}
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}

		// V-06: Validate email format
		if _, err := mail.ParseAddress(req.Email); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid email format"})
			return
		}

		hashedPassword, err := hashPassword(req.Password)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to hash password"})
			return
		}

		validRoles := map[string]bool{"admin": true, "viewer": true}
		role := req.Role
		if role == "" {
			role = "viewer"
		} else if !validRoles[role] {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid role: must be 'admin' or 'viewer'"})
			return
		}

		user := &domain.User{
			Email:        req.Email,
			PasswordHash: hashedPassword,
			Role:         role,
		}

		repo := repo.NewUserRepo(db)
		created, err := repo.Create(c.Request.Context(), user)
		if err != nil {
			slog.Error("failed to create user", "error", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": sanitizeError(err)})
			return
		}

		created.PasswordHash = ""
		c.JSON(http.StatusCreated, gin.H{"data": created})
	}
}

func DeleteUser(db *sql.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		id, err := strconv.ParseInt(c.Param("id"), 10, 64)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
			return
		}
		actingID, _ := c.Get("user_id")
		if actingID == id {
			c.JSON(http.StatusBadRequest, gin.H{"error": "cannot delete your own account"})
			return
		}
		userRepo := repo.NewUserRepo(db)
		allUsers, err := userRepo.List(c.Request.Context())
		if err == nil {
			adminCount := 0
			for _, u := range allUsers {
				if u.Role == "admin" && u.ID != id {
					adminCount++
				}
			}
			if adminCount == 0 {
				c.JSON(http.StatusBadRequest, gin.H{"error": "cannot delete the last admin account"})
				return
			}
		}
		if err := userRepo.Delete(c.Request.Context(), id); err != nil {
			slog.Error("failed to delete user", "error", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": sanitizeError(err)})
			return
		}
		c.JSON(http.StatusOK, gin.H{"data": "deleted"})
	}
}

// MyIP returns the client's IP address with optional GeoIP enrichment
func MyIP(geoDB *geo.GeoIPDB) gin.HandlerFunc {
	return func(c *gin.Context) {
		ip := c.ClientIP()
		resp := gin.H{
			"ip": ip,
		}

		if geoDB != nil && geoDB.Enabled() {
			if city, err := geoDB.LookupCity(ip); err == nil {
				resp["city"] = city.City
				resp["country"] = city.Country
				resp["country_code"] = city.CountryISO
				resp["country_flag"] = city.CountryFlag
				resp["latitude"] = city.Latitude
				resp["longitude"] = city.Longitude
				resp["timezone"] = city.TimeZone
			}
			if asn, err := geoDB.ResolveASN(c.Request.Context(), ip); err == nil && asn != nil && asn.ASN > 0 {
				resp["asn"] = asn.ASN
				resp["asn_org"] = asn.OrgName
			}
		}

		c.JSON(http.StatusOK, gin.H{"data": resp})
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
		severity := domain.Severity(req.Severity)
		if severity == "" {
			severity = domain.SeverityInfo
		}
		scope := req.Scope
		if scope == "" {
			scope = "global"
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
		rule := &domain.CommunityRule{
			ID:          id,
			Community:   req.Community,
			Severity:    domain.Severity(req.Severity),
			MessageI18n: req.MessageI18n,
			Scope:       req.Scope,
			Active:      req.Active,
		}
		repo := repo.NewCommunityRuleRepo(db)
		updated, err := repo.Update(c.Request.Context(), rule)
		if err != nil {
			slog.Error("failed to update community rule", "error", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": sanitizeError(err)})
			return
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
		c.JSON(http.StatusOK, gin.H{"data": "toggled"})
	}
}

// Helper functions
func generateJWT(userID int64, role string, secret string) (string, error) {
	claims := jwt.MapClaims{
		"user_id": userID,
		"role":    role,
		"exp":     time.Now().Add(24 * time.Hour).Unix(),
		"iat":     time.Now().Unix(),
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString([]byte(secret))
}

func hashPassword(password string) (string, error) {
	bytes, err := bcrypt.GenerateFromPassword([]byte(password), 12)
	return string(bytes), err
}

func formatInt64Ptr(v *int64) string {
	if v == nil {
		return ""
	}
	return strconv.FormatInt(*v, 10)
}

var publicSettingKeys = map[string]bool{
	"site_name": true, "site_description": true, "logo_path": true,
	"header_color": true, "url_website": true, "url_peeringdb": true,
	"url_contact": true, "url_terms": true, "url_privacy": true,
}

func GetPublicSettings(db *sql.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		q := queries.New(db)
		settings, err := q.GetSettings()
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to load settings"})
			return
		}
		public := make(map[string]string, len(publicSettingKeys))
		for k, v := range settings {
			if publicSettingKeys[k] {
				public[k] = v
			}
		}
		c.JSON(http.StatusOK, gin.H{"data": public})
	}
}

func GetAdminSettings(db *sql.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		q := queries.New(db)
		settings, err := q.GetSettings()
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to load settings"})
			return
		}
		c.JSON(http.StatusOK, gin.H{"data": settings})
	}
}

var allowedSettingKeys = map[string]bool{
	"site_name": true, "site_description": true, "header_color": true,
	"url_website": true, "url_peeringdb": true, "url_contact": true, "url_terms": true, "url_privacy": true,
	"ping_count": true, "max_hops": true, "mtr_cycles": true,
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
		q := queries.New(db)
		if err := q.SetSettings(filtered); err != nil {
			slog.Error("failed to update settings", "error", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to update settings"})
			return
		}
		settings, _ := q.GetSettings()
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
		n, _ := file.Read(buf)
		mime := http.DetectContentType(buf[:n])

		// http.DetectContentType cannot detect SVG reliably; check content
		if (strings.HasPrefix(mime, "text/xml") || strings.HasPrefix(mime, "application/xml") || strings.HasPrefix(mime, "text/plain")) && bytes.Contains(buf[:n], []byte("<svg")) {
			mime = "image/svg+xml"
		}

		if mime != "image/png" && mime != "image/jpeg" && mime != "image/svg+xml" && mime != "image/webp" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid file type (allowed: png, jpeg, svg, webp)"})
			return
		}
		file.Seek(0, io.SeekStart)

		if mime == "image/svg+xml" {
			all, err := io.ReadAll(file)
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
			file.Seek(0, io.SeekStart)
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

		os.MkdirAll("data/uploads", 0o755)
		for _, oldExt := range []string{".png", ".jpg", ".svg", ".webp"} {
			if oldExt != ext {
				os.Remove("data/uploads/logo" + oldExt)
			}
		}
		outPath := "data/uploads/logo" + ext
		out, err := os.Create(outPath)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to save logo"})
			return
		}
		defer out.Close()

		if _, err := io.Copy(out, file); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to save logo"})
			return
		}

		logoPath := "/logo" + ext
		q := queries.New(db)
		if err := q.SetSetting("logo_path", logoPath); err != nil {
			slog.Error("failed to persist logo path", "error", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "logo saved but settings update failed"})
			return
		}

		c.JSON(http.StatusOK, gin.H{"data": gin.H{"logo_path": logoPath}})
	}
}

var ipRegex = regexp.MustCompile(`\b(\d{1,3}\.\d{1,3}\.\d{1,3}\.\d{1,3})\b`)

// svgEventHandlers matches any on* attribute (e.g. onload=, onbegin=, onclick=).
// Applied to lowercased SVG content to catch mixed-case bypasses.
var svgEventHandlers = regexp.MustCompile(`\bon[a-z]+=`)

// svgExternalRef matches external URL references in href/src/xlink:href attributes.
var svgExternalRef = regexp.MustCompile(`(?:x?link:href|href|src)\s*=\s*["']https?://`)

func enrichLineWithAS(ctx context.Context, geoDB interface{ ResolveASN(context.Context, string) (*domain.ASInfo, error) }, line string) string {
	matches := ipRegex.FindStringSubmatch(line)
	if len(matches) < 2 {
		return line
	}
	ip := matches[1]
	parsed := net.ParseIP(ip)
	if parsed == nil || parsed.IsPrivate() || parsed.IsLoopback() || parsed.IsLinkLocalUnicast() {
		return line
	}
	info, err := geoDB.ResolveASN(ctx, ip)
	if err != nil || info == nil || info.ASN == 0 {
		return line
	}
	sanitizeName := func(s string) string {
		return strings.Map(func(r rune) rune {
			if r == '\n' || r == '\r' || r < 32 { return -1 }
			return r
		}, s)
	}
	suffix := " [AS" + strconv.FormatUint(uint64(info.ASN), 10) + " -"
	name := sanitizeName(info.ShortName)
	if name == "" {
		name = sanitizeName(info.OrgName)
	}
	if name != "" {
		suffix += " " + name
	}
	suffix += "]"
	return line + suffix
}
