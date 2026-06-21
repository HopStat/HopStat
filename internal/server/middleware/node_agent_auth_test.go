package middleware

import (
	"context"
	"database/sql"
	"net/http"
	"net/http/httptest"
	"testing"

	_ "modernc.org/sqlite"

	"github.com/HopStat/HopStat/internal/domain"
	"github.com/HopStat/HopStat/internal/store"
	"github.com/HopStat/HopStat/internal/store/repo"
)

func setupAgentDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if err := store.Migrate(db); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return db
}

func TestNodeAgentAuth_ValidToken(t *testing.T) {
	db := setupAgentDB(t)
	nodeRepo := repo.NewNodeRepo(db, "")
	_, err := nodeRepo.Create(context.Background(), &domain.Node{
		Name: "agent", Type: domain.NodeTypeStandalone, Active: true,
		AgentToken: "agent-secret", EnabledCmds: domain.DefaultEnabledCmds(),
	})
	if err != nil {
		t.Fatalf("create node: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/agent/v1/health", nil)
	req.Header.Set("Authorization", "Bearer agent-secret")
	c, w := newTestContext(req)

	NodeAgentAuth(db, "")(c)

	if c.IsAborted() {
		t.Fatalf("auth failed: %d %s", w.Code, w.Body.String())
	}
	if _, ok := c.Get(AgentNodeKey); !ok {
		t.Fatal("expected agent_node in context")
	}
}

func TestNodeAgentAuth_MissingHeader(t *testing.T) {
	db := setupAgentDB(t)
	req := httptest.NewRequest(http.MethodGet, "/agent/v1/health", nil)
	c, w := newTestContext(req)

	NodeAgentAuth(db, "")(c)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", w.Code)
	}
}

func TestNodeAgentAuth_InvalidToken(t *testing.T) {
	db := setupAgentDB(t)
	req := httptest.NewRequest(http.MethodGet, "/agent/v1/health", nil)
	req.Header.Set("Authorization", "Bearer wrong")
	c, w := newTestContext(req)

	NodeAgentAuth(db, "")(c)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", w.Code)
	}
}
