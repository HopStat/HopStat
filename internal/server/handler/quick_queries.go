package handler

import (
	"context"
	"database/sql"
	"log/slog"
	"net"
	"net/http"
	"strconv"
	"strings"

	"github.com/HopStat/HopStat/internal/domain"
	"github.com/HopStat/HopStat/internal/quickqueries"
	"github.com/HopStat/HopStat/internal/store/queries"
	"github.com/gin-gonic/gin"
)

var quickQueriesRefreshFn = quickqueries.Refresh

var createQuickQueryRecordFn = func(q *queries.Queries, ctx context.Context, item *queries.QuickQuery) (*queries.QuickQuery, error) {
	return q.CreateQuickQuery(ctx, item)
}

var updateQuickQueryRecordFn = func(q *queries.Queries, ctx context.Context, item *queries.QuickQuery) (*queries.QuickQuery, error) {
	return q.UpdateQuickQuery(ctx, item)
}

var validQuickQueryCommands = map[string]bool{
	string(domain.CmdPing):       true,
	string(domain.CmdTraceroute): true,
	string(domain.CmdBGPRoute):   true,
}

func validQuickQueryTarget(command, target string) bool {
	target = strings.TrimSpace(target)
	if target == "" {
		return false
	}
	switch command {
	case string(domain.CmdPing), string(domain.CmdTraceroute):
		return net.ParseIP(target) != nil
	case string(domain.CmdBGPRoute):
		if net.ParseIP(target) != nil {
			return true
		}
		if strings.Contains(target, "/") {
			_, _, err := net.ParseCIDR(target)
			return err == nil
		}
		return false
	default:
		return false
	}
}

func ListPublicQuickQueries() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"data": quickqueries.Active()})
	}
}

func ListQuickQueries(db *sql.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		_ = db
		c.JSON(http.StatusOK, gin.H{"data": quickqueries.All()})
	}
}

func CreateQuickQuery(db *sql.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		var req struct {
			Command string `json:"command" binding:"required"`
			Name    string `json:"name" binding:"required"`
			Target  string `json:"target" binding:"required"`
			NodeID  *int64 `json:"node_id"`
			Active  *bool  `json:"active"`
		}
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		command := strings.TrimSpace(req.Command)
		name := strings.TrimSpace(req.Name)
		target := strings.TrimSpace(req.Target)
		if !validQuickQueryCommands[command] {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid command"})
			return
		}
		if name == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "name is required"})
			return
		}
		if !validQuickQueryTarget(command, target) {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid target"})
			return
		}
		active := true
		if req.Active != nil {
			active = *req.Active
		}
		q := queries.New(db)
		sortOrder, err := q.NextQuickQuerySortOrder(c.Request.Context())
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": sanitizeError(err)})
			return
		}
		activeInt := 0
		if active {
			activeInt = 1
		}
		nodeID, nodeErr := quickQueryNodeIDFromRequest(db, c, req.NodeID)
		if nodeErr != "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": nodeErr})
			return
		}
		created, err := createQuickQueryRecordFn(q, c.Request.Context(), &queries.QuickQuery{
			Command:   command,
			Name:      name,
			Target:    target,
			NodeID:    nodeID,
			SortOrder: sortOrder,
			Active:    activeInt,
		})
		if err != nil {
			slog.Error("failed to create quick query", "error", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": sanitizeError(err)})
			return
		}
		if err := quickQueriesRefreshFn(db); err != nil {
			slog.Warn("failed to refresh quick queries cache", "error", err)
		}
		c.JSON(http.StatusCreated, gin.H{"data": mapQuickQuery(created)})
	}
}

func UpdateQuickQuery(db *sql.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		id, err := strconv.ParseInt(c.Param("id"), 10, 64)
		if err != nil || id <= 0 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
			return
		}
		var req struct {
			Command string `json:"command" binding:"required"`
			Name    string `json:"name" binding:"required"`
			Target  string `json:"target" binding:"required"`
			NodeID  *int64 `json:"node_id"`
			Active  bool   `json:"active"`
		}
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		command := strings.TrimSpace(req.Command)
		name := strings.TrimSpace(req.Name)
		target := strings.TrimSpace(req.Target)
		if !validQuickQueryCommands[command] {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid command"})
			return
		}
		if name == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "name is required"})
			return
		}
		if !validQuickQueryTarget(command, target) {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid target"})
			return
		}
		q := queries.New(db)
		existing, err := q.GetQuickQueryByID(c.Request.Context(), id)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": sanitizeError(err)})
			return
		}
		if existing == nil {
			c.JSON(http.StatusNotFound, gin.H{"error": "not found"})
			return
		}
		activeInt := 0
		if req.Active {
			activeInt = 1
		}
		nodeID, nodeErr := quickQueryNodeIDFromRequest(db, c, req.NodeID)
		if nodeErr != "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": nodeErr})
			return
		}
		updated, err := updateQuickQueryRecordFn(q, c.Request.Context(), &queries.QuickQuery{
			ID:        id,
			Command:   command,
			Name:      name,
			Target:    target,
			NodeID:    nodeID,
			SortOrder: existing.SortOrder,
			Active:    activeInt,
		})
		if err != nil {
			slog.Error("failed to update quick query", "error", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": sanitizeError(err)})
			return
		}
		if err := quickQueriesRefreshFn(db); err != nil {
			slog.Warn("failed to refresh quick queries cache", "error", err)
		}
		c.JSON(http.StatusOK, gin.H{"data": mapQuickQuery(updated)})
	}
}

func DeleteQuickQuery(db *sql.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		id, err := strconv.ParseInt(c.Param("id"), 10, 64)
		if err != nil || id <= 0 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
			return
		}
		q := queries.New(db)
		if err := q.DeleteQuickQuery(c.Request.Context(), id); err != nil {
			slog.Error("failed to delete quick query", "error", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": sanitizeError(err)})
			return
		}
		if err := quickQueriesRefreshFn(db); err != nil {
			slog.Warn("failed to refresh quick queries cache", "error", err)
		}
		c.JSON(http.StatusOK, gin.H{"data": "deleted"})
	}
}

func ToggleQuickQuery(db *sql.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		id, err := strconv.ParseInt(c.Param("id"), 10, 64)
		if err != nil || id <= 0 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
			return
		}
		q := queries.New(db)
		if err := q.ToggleQuickQuery(c.Request.Context(), id); err != nil {
			slog.Error("failed to toggle quick query", "error", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": sanitizeError(err)})
			return
		}
		if err := quickQueriesRefreshFn(db); err != nil {
			slog.Warn("failed to refresh quick queries cache", "error", err)
		}
		updated, err := q.GetQuickQueryByID(c.Request.Context(), id)
		if err != nil || updated == nil {
			c.Status(http.StatusNoContent)
			return
		}
		c.JSON(http.StatusOK, gin.H{"data": mapQuickQuery(updated)})
	}
}

func mapQuickQuery(item *queries.QuickQuery) domain.QuickQuery {
	if item == nil {
		return domain.QuickQuery{}
	}
	out := domain.QuickQuery{
		ID:        item.ID,
		Command:   item.Command,
		Name:      item.Name,
		Target:    item.Target,
		SortOrder: item.SortOrder,
		Active:    item.Active == 1,
	}
	if item.NodeID.Valid && item.NodeID.Int64 > 0 {
		nodeID := item.NodeID.Int64
		out.NodeID = &nodeID
	}
	return out
}

func quickQueryNodeIDFromRequest(db *sql.DB, c *gin.Context, nodeID *int64) (sql.NullInt64, string) {
	if nodeID == nil || *nodeID <= 0 {
		return sql.NullInt64{}, ""
	}
	q := queries.New(db)
	node, err := q.GetNodeByID(c.Request.Context(), *nodeID)
	if err != nil {
		return sql.NullInt64{}, sanitizeError(err)
	}
	if node == nil {
		return sql.NullInt64{}, "node not found"
	}
	return sql.NullInt64{Int64: *nodeID, Valid: true}, ""
}
