package handler

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/HopStat/HopStat/internal/domain"
	"github.com/HopStat/HopStat/internal/geo"
	"github.com/HopStat/HopStat/internal/server/middleware"
	"github.com/HopStat/HopStat/internal/store/repo"
	"github.com/gin-gonic/gin"
)

func setupAgentRouter(t *testing.T, db *sql.DB, node *domain.Node) *gin.Engine {
	t.Helper()
	if node != nil && node.ID == 0 {
		nodeRepo := repo.NewNodeRepo(db, "")
		// Persisted for its side effect: NodeAgentAuth looks the node up in the database.
		if _, err := nodeRepo.Create(t.Context(), node); err != nil {
			t.Fatalf("create node: %v", err)
		}
	}

	gin.SetMode(gin.TestMode)
	r := gin.New()
	agent := r.Group("")
	agent.Use(middleware.NodeAgentAuth(db, ""))
	MountAgentAPI(agent, testConfig(), nil, geo.New("", ""))
	return r
}

func TestAgentHealth_NoNodeInContext(t *testing.T) {
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/agent/v1/health", nil)

	agentHealth(c)

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d", w.Code)
	}
}

func TestAgentNodeFromContext_WrongType(t *testing.T) {
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Set(middleware.AgentNodeKey, "not-a-node")

	if _, ok := agentNodeFromContext(c); ok {
		t.Fatal("expected false for wrong type")
	}
}

func TestAgentCapabilities(t *testing.T) {
	db := setupDB(t)
	r := setupAgentRouter(t, db, &domain.Node{
		Name:        "local",
		Type:        domain.NodeTypeStandalone,
		AgentToken:  "node-secret",
		EnabledCmds: []domain.CommandType{domain.CmdPing, domain.CmdTraceroute},
		Active:      true,
	})

	req := httptest.NewRequest(http.MethodGet, "/agent/v1/capabilities", nil)
	req.Header.Set("Authorization", "Bearer node-secret")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", w.Code, w.Body.String())
	}
}

func TestAgentPing_ValidationAndSuccess(t *testing.T) {
	db := setupDB(t)
	r := setupAgentRouter(t, db, &domain.Node{
		Name:        "local",
		Type:        domain.NodeTypeStandalone,
		AgentToken:  "node-secret",
		EnabledCmds: []domain.CommandType{domain.CmdPing},
		Active:      true,
	})

	req := httptest.NewRequest(http.MethodPost, "/agent/v1/ping", strings.NewReader(`{}`))
	req.Header.Set("Authorization", "Bearer node-secret")
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("missing target status = %d", w.Code)
	}

	body := `{"target":"127.0.0.1","count":1}`
	req = httptest.NewRequest(http.MethodPost, "/agent/v1/ping", strings.NewReader(body))
	req.Header.Set("Authorization", "Bearer node-secret")
	req.Header.Set("Content-Type", "application/json")
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK && w.Code != http.StatusInternalServerError {
		t.Fatalf("ping status = %d, body = %s", w.Code, w.Body.String())
	}
}

func TestAgentTraceroute(t *testing.T) {
	db := setupDB(t)
	r := setupAgentRouter(t, db, &domain.Node{
		Name:        "local",
		Type:        domain.NodeTypeStandalone,
		AgentToken:  "node-secret",
		EnabledCmds: []domain.CommandType{domain.CmdTraceroute},
		Active:      true,
	})

	body := `{"target":"127.0.0.1","max_hops":1}`
	req := httptest.NewRequest(http.MethodPost, "/agent/v1/traceroute", strings.NewReader(body))
	req.Header.Set("Authorization", "Bearer node-secret")
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK && w.Code != http.StatusInternalServerError {
		t.Fatalf("traceroute status = %d, body = %s", w.Code, w.Body.String())
	}
}

func TestAgentBGPRoute(t *testing.T) {
	db := setupDB(t)
	r := setupAgentRouter(t, db, &domain.Node{
		Name:        "local",
		Type:        domain.NodeTypeStandalone,
		AgentToken:  "node-secret",
		EnabledCmds: []domain.CommandType{domain.CmdBGPRoute},
		Active:      true,
	})

	tests := []struct {
		name string
		body string
		code int
	}{
		{"missing prefix", `{}`, http.StatusBadRequest},
		{"invalid ip", `{"prefix":"not-an-ip"}`, http.StatusBadRequest},
		{"invalid cidr", `{"prefix":"1.2.3.4/99"}`, http.StatusBadRequest},
		{"valid ip", `{"prefix":"8.8.8.8"}`, http.StatusOK},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, "/agent/v1/bgp/route", strings.NewReader(tc.body))
			req.Header.Set("Authorization", "Bearer node-secret")
			req.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()
			r.ServeHTTP(w, req)
			if tc.name == "valid ip" {
				if w.Code != http.StatusOK && w.Code != http.StatusInternalServerError {
					t.Fatalf("status = %d, body = %s", w.Code, w.Body.String())
				}
				return
			}
			if w.Code != tc.code {
				t.Fatalf("status = %d, body = %s", w.Code, w.Body.String())
			}
		})
	}
}

func TestAgentPingStream(t *testing.T) {
	db := setupDB(t)
	r := setupAgentRouter(t, db, &domain.Node{
		Name:        "local",
		Type:        domain.NodeTypeStandalone,
		AgentToken:  "node-secret",
		EnabledCmds: []domain.CommandType{domain.CmdPing},
		Active:      true,
	})

	body := `{"target":"127.0.0.1","count":1}`
	req := httptest.NewRequest(http.MethodPost, "/agent/v1/ping/stream", strings.NewReader(body))
	req.Header.Set("Authorization", "Bearer node-secret")
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	resp := w.Body.String()
	if w.Code != http.StatusOK && w.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, body = %s", w.Code, resp)
	}
	if w.Code == http.StatusOK && !strings.Contains(resp, "event:") {
		t.Fatalf("expected SSE events, got %q", resp)
	}
}

func TestAgentTracerouteStream(t *testing.T) {
	db := setupDB(t)
	r := setupAgentRouter(t, db, &domain.Node{
		Name:        "local",
		Type:        domain.NodeTypeStandalone,
		AgentToken:  "node-secret",
		EnabledCmds: []domain.CommandType{domain.CmdTraceroute},
		Active:      true,
	})

	body := `{"target":"127.0.0.1","max_hops":1}`
	req := httptest.NewRequest(http.MethodPost, "/agent/v1/traceroute/stream", strings.NewReader(body))
	req.Header.Set("Authorization", "Bearer node-secret")
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK && w.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, body = %s", w.Code, w.Body.String())
	}
}

func TestWriteSSE(t *testing.T) {
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/", nil)
	flusher, _ := c.Writer.(http.Flusher)

	writeSSE(c, flusher, "test", gin.H{"ok": true})

	if !strings.Contains(w.Body.String(), "event: test") {
		t.Fatalf("body = %q", w.Body.String())
	}
}

func TestAgentLocalDriver(t *testing.T) {
	node := &domain.Node{Name: "n", Type: domain.NodeTypeStandalone, EnabledCmds: domain.DefaultEnabledCmds()}
	drv, err := agentLocalDriver(node, testConfig())
	if err != nil {
		t.Fatal(err)
	}
	if drv == nil {
		t.Fatal("expected driver")
	}
}

func TestAgentPing_NoNodeDirect(t *testing.T) {
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/agent/v1/ping", bytes.NewReader([]byte(`{"target":"1.1.1.1"}`)))
	c.Request.Header.Set("Content-Type", "application/json")

	agentPing(testConfig())(c)

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d", w.Code)
	}
}

func TestAgentBGPRoute_ValidCIDR(t *testing.T) {
	db := setupDB(t)
	r := setupAgentRouter(t, db, &domain.Node{
		Name:        "local",
		Type:        domain.NodeTypeStandalone,
		AgentToken:  "node-secret",
		EnabledCmds: []domain.CommandType{domain.CmdBGPRoute},
		Active:      true,
	})

	req := httptest.NewRequest(http.MethodPost, "/agent/v1/bgp/route", strings.NewReader(`{"prefix":"8.8.8.8/32"}`))
	req.Header.Set("Authorization", "Bearer node-secret")
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK && w.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, body = %s", w.Code, w.Body.String())
	}
	var body map[string]interface{}
	_ = json.Unmarshal(w.Body.Bytes(), &body)
}
