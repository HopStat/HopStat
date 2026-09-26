package handler

import (
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/HopStat/HopStat/internal/config"
	"github.com/HopStat/HopStat/internal/domain"
	"github.com/HopStat/HopStat/internal/server/middleware"
	"github.com/HopStat/HopStat/internal/store/repo"
	"github.com/gin-gonic/gin"
)

type turnstileRoundTrip func(*http.Request) (*http.Response, error)

func (f turnstileRoundTrip) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func turnstileConfig() config.TurnstileConfig {
	return config.TurnstileConfig{
		SiteKey:   "site-key",
		Secret:    "secret",
		Hostnames: []string{"lg.example"},
	}
}

func TestSubmitQuery_TurnstileRejectsAPIWithoutToken(t *testing.T) {
	db := setupDB(t)
	cfg := testConfig()
	cfg.Turnstile = turnstileConfig()
	h := New(db, cfg, nil, nil)

	body := `{"node_id":1,"command":"ping","target":"8.8.8.8"}`
	c, w := setupContext(db, http.MethodPost, "/query", body)
	c.Request.Header.Set("Authorization", "Bearer node-secret")

	h.SubmitQuery()(c)

	if w.Code != http.StatusForbidden {
		t.Fatalf("status = %d, body = %s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), `"error_code":"TURNSTILE"`) {
		t.Fatalf("body = %s", w.Body.String())
	}
}

func TestSubmitQuery_TurnstileAcceptsThenRejectsReplay(t *testing.T) {
	db := setupDB(t)
	cfg := testConfig()
	cfg.Turnstile = turnstileConfig()
	nodeRepo := repo.NewNodeRepo(db, "")
	created, err := nodeRepo.Create(t.Context(), &domain.Node{
		Name: "n", Type: domain.NodeTypeStandalone, Active: true,
		EnabledCmds: domain.DefaultEnabledCmds(),
	})
	if err != nil {
		t.Fatalf("create node: %v", err)
	}
	refreshTestSiteCache(t, db)

	calls := 0
	h := New(db, cfg, nil, nil)
	h.turnstile.UseHTTPClient(&http.Client{Transport: turnstileRoundTrip(func(r *http.Request) (*http.Response, error) {
		calls++
		payload, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("read siteverify body: %v", err)
		}
		if !strings.Contains(string(payload), "response=fresh-token") {
			t.Fatalf("siteverify payload = %s", payload)
		}
		body := `{"success":true,"action":"query","hostname":"lg.example"}`
		if calls > 1 {
			body = `{"success":false,"action":"query","hostname":"lg.example","error-codes":["timeout-or-duplicate"]}`
		}
		return &http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(strings.NewReader(body)),
			Header:     make(http.Header),
			Request:    r,
		}, nil
	})})

	body := fmt.Sprintf(`{"node_id":%d,"command":"ping","target":"8.8.8.8","options":{"ping_count":1},"cf-turnstile-response":"fresh-token"}`, created.ID)

	c, w := setupContext(db, http.MethodPost, "/query", body)
	h.SubmitQuery()(c)
	if w.Code != http.StatusOK {
		t.Fatalf("first status = %d, body = %s", w.Code, w.Body.String())
	}

	c, w = setupContext(db, http.MethodPost, "/query", body)
	h.SubmitQuery()(c)
	if w.Code != http.StatusForbidden {
		t.Fatalf("replay status = %d, body = %s", w.Code, w.Body.String())
	}
}

func TestGetPublicSettingsIncludesTurnstileSiteKeyOnly(t *testing.T) {
	db := setupDB(t)
	c, w := setupContext(db, http.MethodGet, "/settings", "")

	GetPublicSettings(db, config.BGPConfig{}, " site-key ")(c)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), `"turnstile_site_key":"site-key"`) {
		t.Fatalf("body = %s", w.Body.String())
	}
	if strings.Contains(w.Body.String(), "turnstile_secret") {
		t.Fatalf("secret leaked: %s", w.Body.String())
	}
}

func TestAgentAPI_NodeKeySkipsTurnstile(t *testing.T) {
	db := setupDB(t)
	if _, err := repo.NewNodeRepo(db, "").Create(t.Context(), &domain.Node{
		Name: "local", Type: domain.NodeTypeStandalone, Active: true,
		AgentToken:  "node-secret",
		EnabledCmds: []domain.CommandType{domain.CmdPing},
	}); err != nil {
		t.Fatalf("create node: %v", err)
	}

	cfg := testConfig()
	cfg.Turnstile = turnstileConfig()
	r := gin.New()
	agent := r.Group("")
	agent.Use(middleware.NodeAgentAuth(db, ""))
	MountAgentAPI(agent, cfg, nil, nil)

	req := httptest.NewRequest(http.MethodPost, "/agent/v1/ping", strings.NewReader(`{}`))
	req.Header.Set("Authorization", "Bearer node-secret")
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, body = %s", w.Code, w.Body.String())
	}
}
