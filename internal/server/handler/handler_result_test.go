package handler

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/HopStat/HopStat/internal/domain"
	"github.com/HopStat/HopStat/internal/sitecache"
	"github.com/HopStat/HopStat/internal/store/repo"
)

func TestGetResult_Found(t *testing.T) {
	db := setupDB(t)
	queryID := "test-query-id"
	queryStore.Set(queryID, &domain.QueryResult{
		ID:     queryID,
		Status: domain.StatusDone,
	})

	c, w := setupContext(db, http.MethodGet, "/query/"+queryID, "")
	c.Params = gin.Params{{Key: "id", Value: queryID}}

	GetResult(db)(c)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", w.Code, w.Body.String())
	}
}

func TestExportAudit_CSV(t *testing.T) {
	db := setupDB(t)
	uid := int64(1)
	auditRepo := repo.NewAuditRepo(db)
	if err := auditRepo.Log(context.Background(), &domain.AuditEntry{
		UserID:  &uid,
		Command: "ping",
		Params:  "8.8.8.8",
		Success: true,
	}); err != nil {
		t.Fatalf("log audit: %v", err)
	}

	c, w := setupAdminContext(db, http.MethodGet, "/admin/audit/export", "", 1)
	ExportAudit(db)(c)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", w.Code, w.Body.String())
	}
	if ct := w.Header().Get("Content-Type"); !strings.Contains(ct, "text/csv") {
		t.Errorf("content-type = %q, want csv", ct)
	}
	if !strings.Contains(w.Body.String(), "ping") {
		t.Error("expected csv to contain ping command")
	}
}

func TestToggleCommunityRule(t *testing.T) {
	db := setupDB(t)
	createBody := `{"community":"65535:1","severity":"info","message_i18n":"x","scope":"global","active":true}`
	c, w := setupAdminContext(db, http.MethodPost, "/admin/community-rules", createBody, 1)
	CreateCommunityRule(db)(c)
	if w.Code != http.StatusCreated {
		t.Fatalf("create status = %d", w.Code)
	}

	var created struct {
		Data struct {
			ID int64 `json:"id"`
		} `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &created); err != nil {
		t.Fatalf("parse: %v", err)
	}

	c, w = setupAdminContext(db, http.MethodPatch, fmt.Sprintf("/admin/community-rules/%d/toggle", created.Data.ID), "", 1)
	c.Params = gin.Params{{Key: "id", Value: fmt.Sprint(created.Data.ID)}}
	ToggleCommunityRule(db)(c)

	if w.Code != http.StatusOK {
		t.Fatalf("toggle status = %d, body = %s", w.Code, w.Body.String())
	}
}

func TestListNodes_WithActiveNode(t *testing.T) {
	db := setupDB(t)
	nodeRepo := repo.NewNodeRepo(db, "")
	_, err := nodeRepo.Create(context.Background(), &domain.Node{
		Name: "pub", Type: domain.NodeTypeStandalone, Active: true,
		EnabledCmds: domain.DefaultEnabledCmds(),
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if err := sitecache.RefreshNodes(db, ""); err != nil {
		t.Fatalf("refresh node cache: %v", err)
	}

	c, w := setupContext(db, http.MethodGet, "/nodes", "")
	ListNodes(db, "", nil)(c)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", w.Code, w.Body.String())
	}
}
