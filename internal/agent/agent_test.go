package agent

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/HopStat/HopStat/internal/config"
)

func init() {
	gin.SetMode(gin.TestMode)
}

func testAgentConfig() *config.Config {
	return &config.Config{
		Server: config.ServerConfig{
			Host: "0.0.0.0",
			Port: 8080,
			Mode: "agent",
		},
		Agent: config.AgentConfig{
			Port:  9090,
			Token: "test-token",
		},
	}
}

func TestAgentSetup(t *testing.T) {
	cfg := testAgentConfig()
	agent := New(cfg)
	if agent == nil {
		t.Fatal("expected non-nil agent")
	}
}

func TestAgentAuth(t *testing.T) {
	cfg := testAgentConfig()
	cfg.Agent.Token = "secret-token"

	handler := agentAuth(cfg.Agent.Token)

	// Missing header
	req := httptest.NewRequest(http.MethodPost, "/test", nil)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = req
	handler(c)
	if !c.IsAborted() {
		t.Error("expected abort for missing auth header")
	}

	// Wrong token
	req = httptest.NewRequest(http.MethodPost, "/test", nil)
	req.Header.Set("Authorization", "Bearer wrong-token")
	w = httptest.NewRecorder()
	c, _ = gin.CreateTestContext(w)
	c.Request = req
	handler(c)
	if !c.IsAborted() {
		t.Error("expected abort for wrong token")
	}

	// Correct token
	req = httptest.NewRequest(http.MethodPost, "/test", nil)
	req.Header.Set("Authorization", "Bearer secret-token")
	w = httptest.NewRecorder()
	c, _ = gin.CreateTestContext(w)
	c.Request = req
	handler(c)
	if c.IsAborted() {
		t.Error("should not abort for correct token")
	}
}

func TestAgentHandleHealth(t *testing.T) {
	cfg := testAgentConfig()
	agent := New(cfg)

	r := gin.New()
	r.GET("/health", agent.handleHealth)

	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", w.Code, http.StatusOK)
	}

	var resp map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &resp)
	if resp["status"] != "ok" {
		t.Errorf("status = %v, want ok", resp["status"])
	}
}

func TestAgentHandleCapabilities(t *testing.T) {
	cfg := testAgentConfig()
	agent := New(cfg)

	r := gin.New()
	r.GET("/capabilities", agent.handleCapabilities)

	req := httptest.NewRequest(http.MethodGet, "/capabilities", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", w.Code, http.StatusOK)
	}

	var resp map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &resp)
	cmds, ok := resp["commands"].([]interface{})
	if !ok || len(cmds) != 5 {
		t.Errorf("expected 5 commands, got %v", resp["commands"])
	}
}

func TestAgentHandlePingBadJSON(t *testing.T) {
	cfg := testAgentConfig()
	agent := New(cfg)

	r := gin.New()
	r.POST("/ping", agent.handlePing)

	req := httptest.NewRequest(http.MethodPost, "/ping", bytes.NewBufferString("not json"))
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", w.Code, http.StatusBadRequest)
	}
}

func TestAgentHandleTracerouteBadJSON(t *testing.T) {
	cfg := testAgentConfig()
	agent := New(cfg)

	r := gin.New()
	r.POST("/traceroute", agent.handleTraceroute)

	req := httptest.NewRequest(http.MethodPost, "/traceroute", bytes.NewBufferString("not json"))
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", w.Code, http.StatusBadRequest)
	}
}

func TestAgentHandleMTRBadJSON(t *testing.T) {
	cfg := testAgentConfig()
	agent := New(cfg)

	r := gin.New()
	r.POST("/mtr", agent.handleMTR)

	req := httptest.NewRequest(http.MethodPost, "/mtr", bytes.NewBufferString("not json"))
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", w.Code, http.StatusBadRequest)
	}
}

func TestAgentHandleBGPRouteBadJSON(t *testing.T) {
	cfg := testAgentConfig()
	agent := New(cfg)

	r := gin.New()
	r.POST("/bgp/route", agent.handleBGPRoute)

	req := httptest.NewRequest(http.MethodPost, "/bgp/route", bytes.NewBufferString("not json"))
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", w.Code, http.StatusBadRequest)
	}
}

func TestAgentHandleASPathBadJSON(t *testing.T) {
	cfg := testAgentConfig()
	agent := New(cfg)

	r := gin.New()
	r.POST("/bgp/aspath", agent.handleASPath)

	req := httptest.NewRequest(http.MethodPost, "/bgp/aspath", bytes.NewBufferString("not json"))
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", w.Code, http.StatusBadRequest)
	}
}

func TestAgentRoutesRegistered(t *testing.T) {
	cfg := testAgentConfig()
	agent := New(cfg)

	routes := agent.router.Routes()
	expected := map[string]bool{
		"POST-/agent/v1/ping":       false,
		"POST-/agent/v1/traceroute": false,
		"POST-/agent/v1/mtr":        false,
		"POST-/agent/v1/bgp/route":  false,
		"POST-/agent/v1/bgp/aspath": false,
		"GET-/agent/v1/capabilities": false,
		"GET-/agent/v1/health":      false,
	}

	for _, route := range routes {
		key := route.Method + "-" + route.Path
		if _, ok := expected[key]; ok {
			expected[key] = true
		}
	}

	for key, found := range expected {
		if !found {
			t.Errorf("route %s not registered", key)
		}
	}
}
