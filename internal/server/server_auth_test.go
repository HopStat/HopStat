package server

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/HopStat/HopStat/internal/store"
)

func setupMigratedDB(t *testing.T) *sql.DB {
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

func TestServerAuthLoginAndSession(t *testing.T) {
	db := setupMigratedDB(t)
	password := "integration-password"
	if err := store.SetAdminPassword(db, password); err != nil {
		t.Fatalf("SetAdminPassword: %v", err)
	}

	cfg := testServerConfig()
	srv := New(cfg, db, nil, newTestServerFS(), nil, "dev")

	loginBody := `{"email":"` + store.DefaultAdminEmail + `","password":"` + password + `"}`
	loginReq := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", bytes.NewBufferString(loginBody))
	loginReq.Header.Set("Content-Type", "application/json")
	loginW := httptest.NewRecorder()
	srv.router.ServeHTTP(loginW, loginReq)

	if loginW.Code != http.StatusOK {
		t.Fatalf("login status = %d, body = %s", loginW.Code, loginW.Body.String())
	}

	loginResp := loginW.Result()
	defer loginResp.Body.Close()
	var cookie *http.Cookie
	for _, ck := range loginResp.Cookies() {
		if ck.Name == "lg_token" {
			cookie = ck
			break
		}
	}
	if cookie == nil || cookie.Value == "" {
		t.Fatal("expected lg_token cookie from login")
	}

	sessionReq := httptest.NewRequest(http.MethodGet, "/api/v1/auth/session", nil)
	sessionReq.AddCookie(cookie)
	sessionW := httptest.NewRecorder()
	srv.router.ServeHTTP(sessionW, sessionReq)

	if sessionW.Code != http.StatusOK {
		t.Fatalf("session status = %d, body = %s", sessionW.Code, sessionW.Body.String())
	}
	var body map[string]interface{}
	if err := json.Unmarshal(sessionW.Body.Bytes(), &body); err != nil {
		t.Fatalf("parse session: %v", err)
	}
	data, _ := body["data"].(map[string]interface{})
	if auth, _ := data["authenticated"].(bool); !auth {
		t.Fatal("expected authenticated session")
	}
}

func TestServerPublicSettings(t *testing.T) {
	db := setupMigratedDB(t)
	srv := New(testServerConfig(), db, nil, newTestServerFS(), nil, "dev")

	req := httptest.NewRequest(http.MethodGet, "/api/v1/settings", nil)
	w := httptest.NewRecorder()
	srv.router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", w.Code, w.Body.String())
	}
}
