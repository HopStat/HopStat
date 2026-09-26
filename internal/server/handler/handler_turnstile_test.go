package handler

import (
	"database/sql"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/HopStat/HopStat/internal/config"
	"github.com/HopStat/HopStat/internal/domain"
	"github.com/HopStat/HopStat/internal/server/middleware"
	"github.com/HopStat/HopStat/internal/sitecache"
	"github.com/HopStat/HopStat/internal/store/queries"
	"github.com/HopStat/HopStat/internal/store/repo"
	"github.com/HopStat/HopStat/internal/turnstile"
	"github.com/gin-gonic/gin"
)

type turnstileRoundTrip func(*http.Request) (*http.Response, error)

func (f turnstileRoundTrip) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func storeTurnstile(t *testing.T, db *sql.DB, siteKey, secret, hostnames, cleared string) {
	t.Helper()
	if err := queries.New(db).SetSettings(map[string]string{
		turnstile.SettingSiteKey:   siteKey,
		turnstile.SettingSecret:    secret,
		turnstile.SettingHostnames: hostnames,
		turnstile.SettingCleared:   cleared,
	}); err != nil {
		t.Fatal(err)
	}
	if err := sitecache.RefreshSettings(db, 0); err != nil {
		t.Fatal(err)
	}
}

func TestSubmitQuery_TurnstileRejectsAPIWithoutToken(t *testing.T) {
	db := setupDB(t)
	storeTurnstile(t, db, "site-key", "secret", "lg.example", "")
	cfg := testConfig()
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
	storeTurnstile(t, db, "site-key", "secret", "lg.example", "")
	cfg := testConfig()
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
	storeTurnstile(t, db, " site-key ", "secret", "lg.example", "")
	c, w := setupContext(db, http.MethodGet, "/settings", "")

	GetPublicSettings(db, config.BGPConfig{})(c)

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

	storeTurnstile(t, db, "site-key", "secret", "lg.example", "")
	cfg := testConfig()
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

func TestSubmitQuery_UsesStoredTurnstileSecret(t *testing.T) {
	db := setupDB(t)
	storeTurnstile(t, db, "panel-key", "panel-secret", "lg.example", "")
	cfg := testConfig()
	h := New(db, cfg, nil, nil)
	var secret string
	h.turnstile.UseHTTPClient(&http.Client{Transport: turnstileRoundTrip(func(r *http.Request) (*http.Response, error) {
		payload, _ := io.ReadAll(r.Body)
		secret = string(payload)
		return &http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(strings.NewReader(`{"success":false,"hostname":"lg.example","action":"query"}`)),
			Header:     make(http.Header),
			Request:    r,
		}, nil
	})})
	body := `{"node_id":1,"command":"ping","target":"8.8.8.8","cf-turnstile-response":"token"}`
	c, w := setupContext(db, http.MethodPost, "/query", body)
	h.SubmitQuery()(c)
	if w.Code != http.StatusForbidden {
		t.Fatalf("status = %d", w.Code)
	}
	if !strings.Contains(secret, "secret=panel-secret") {
		t.Fatalf("siteverify used %s", secret)
	}
}

func TestSubmitQuery_ClearedTurnstileStaysOff(t *testing.T) {
	db := setupDB(t)
	storeTurnstile(t, db, "site-key", "secret", "lg.example", "1")
	cfg := testConfig()
	node, err := repo.NewNodeRepo(db, "").Create(t.Context(), &domain.Node{
		Name: "n", Type: domain.NodeTypeStandalone, Active: true,
		EnabledCmds: domain.DefaultEnabledCmds(),
	})
	if err != nil {
		t.Fatal(err)
	}
	refreshTestSiteCache(t, db)
	body := fmt.Sprintf(`{"node_id":%d,"command":"ping","target":"8.8.8.8","options":{"ping_count":1}}`, node.ID)
	c, w := setupContext(db, http.MethodPost, "/query", body)
	New(db, cfg, nil, nil).SubmitQuery()(c)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", w.Code, w.Body.String())
	}
}

func TestTurnstileAdmin_SaveClearAndErrors(t *testing.T) {
	db := setupDB(t)

	c, w := setupAdminContext(db, http.MethodGet, "/admin/turnstile", "", 1)
	TurnstileStatus(db)(c)
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), `"configured":false`) || strings.Contains(w.Body.String(), "file-secret") {
		t.Fatalf("status = %d body = %s", w.Code, w.Body.String())
	}

	c, w = setupAdminContext(db, http.MethodPut, "/admin/turnstile", `{"site_key":"panel-key","secret":"panel-secret","hostnames":"lg.example"}`, 1)
	UpdateTurnstile(db)(c)
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), `"secret_set":true`) || strings.Contains(w.Body.String(), "panel-secret") {
		t.Fatalf("save status = %d body = %s", w.Code, w.Body.String())
	}
	stored, err := queries.New(db).GetSettings()
	if err != nil || stored[turnstile.SettingSecret] != "panel-secret" {
		t.Fatalf("stored = %#v err = %v", stored, err)
	}

	c, w = setupAdminContext(db, http.MethodPut, "/admin/turnstile", `{"site_key":"panel-key","hostnames":"lg.example"}`, 1)
	UpdateTurnstile(db)(c)
	if w.Code != http.StatusOK {
		t.Fatalf("keep secret status = %d body = %s", w.Code, w.Body.String())
	}
	stored, _ = queries.New(db).GetSettings()
	if stored[turnstile.SettingSecret] != "panel-secret" {
		t.Fatalf("secret was replaced: %q", stored[turnstile.SettingSecret])
	}

	prev := refreshSettingsCacheFn
	refreshSettingsCacheFn = func(*sql.DB, uint32) error { return errors.New("refresh fail") }
	t.Cleanup(func() { refreshSettingsCacheFn = prev })
	c, w = setupAdminContext(db, http.MethodPut, "/admin/turnstile", `{"clear":true}`, 1)
	UpdateTurnstile(db)(c)
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), `"configured":false`) {
		t.Fatalf("clear status = %d body = %s", w.Code, w.Body.String())
	}

	c, w = setupAdminContext(db, http.MethodPut, "/admin/turnstile", `{`, 1)
	UpdateTurnstile(db)(c)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("json status = %d", w.Code)
	}
	c, w = setupAdminContext(db, http.MethodPut, "/admin/turnstile", `{"site_key":"only"}`, 1)
	UpdateTurnstile(db)(c)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("partial status = %d body = %s", w.Code, w.Body.String())
	}

	closed := setupDB(t)
	closed.Close()
	c, w = setupAdminContext(closed, http.MethodGet, "/admin/turnstile", "", 1)
	TurnstileStatus(closed)(c)
	if w.Code != http.StatusInternalServerError {
		t.Fatalf("status load = %d", w.Code)
	}
	c, w = setupAdminContext(closed, http.MethodPut, "/admin/turnstile", `{"clear":true}`, 1)
	UpdateTurnstile(closed)(c)
	if w.Code != http.StatusInternalServerError {
		t.Fatalf("update load = %d", w.Code)
	}
}

func TestTurnstileAdmin_SaveFails(t *testing.T) {
	db := setupDB(t)
	if _, err := db.Exec(`CREATE TRIGGER settings_block BEFORE INSERT ON settings BEGIN SELECT RAISE(ABORT, 'blocked'); END`); err != nil {
		t.Fatal(err)
	}
	c, w := setupAdminContext(db, http.MethodPut, "/admin/turnstile", `{"site_key":"k","secret":"s","hostnames":"lg.example"}`, 1)
	UpdateTurnstile(db)(c)
	if w.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d body = %s", w.Code, w.Body.String())
	}
}
