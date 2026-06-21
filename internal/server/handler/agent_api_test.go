package handler

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/HopStat/HopStat/internal/domain"
	"github.com/HopStat/HopStat/internal/server/middleware"
	"github.com/HopStat/HopStat/internal/store/repo"
)

func TestStandaloneAgentAPI_Auth(t *testing.T) {
	db := setupDB(t)
	nodeRepo := repo.NewNodeRepo(db, "")
	created, err := nodeRepo.Create(t.Context(), &domain.Node{
		Name:        "local",
		Type:        domain.NodeTypeStandalone,
		AgentToken:  "node-secret",
		EnabledCmds: []domain.CommandType{domain.CmdPing},
		Active:      true,
	})
	if err != nil {
		t.Fatalf("create node: %v", err)
	}

	gin.SetMode(gin.TestMode)
	r := gin.New()
	agent := r.Group("")
	agent.Use(middleware.NodeAgentAuth(db, ""))
	MountAgentAPI(agent, testConfig(), nil, nil)

	t.Run("missing auth", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/agent/v1/health", nil)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		if w.Code != http.StatusUnauthorized {
			t.Fatalf("status = %d, want %d", w.Code, http.StatusUnauthorized)
		}
	})

	t.Run("valid token", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/agent/v1/health", nil)
		req.Header.Set("Authorization", "Bearer node-secret")
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		if w.Code != http.StatusOK {
			t.Fatalf("status = %d, body = %s", w.Code, w.Body.String())
		}
		var body map[string]interface{}
		if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
			t.Fatalf("parse: %v", err)
		}
		if body["status"] != "ok" {
			t.Fatalf("status = %v", body["status"])
		}
		if body["node"] != "local" {
			t.Fatalf("node = %v, want local", body["node"])
		}
		_ = created
	})

	t.Run("invalid token", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/agent/v1/health", nil)
		req.Header.Set("Authorization", "Bearer wrong")
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		if w.Code != http.StatusUnauthorized {
			t.Fatalf("status = %d, want %d", w.Code, http.StatusUnauthorized)
		}
	})
}
