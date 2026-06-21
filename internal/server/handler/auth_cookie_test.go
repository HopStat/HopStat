package handler

import (
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/HopStat/HopStat/internal/server/middleware"
	"github.com/HopStat/HopStat/internal/store"
)

func TestLogin_SuccessSetsHttpOnlyCookie(t *testing.T) {
	db := setupDB(t)
	cfg := testConfig()
	password := "securepassword123"
	seedAdminPassword(t, db, password)

	body := `{"email":"` + store.DefaultAdminEmail + `","password":"` + password + `"}`
	c, w := setupContext(db, http.MethodPost, "/auth/login", body)

	Login(db, cfg)(c)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body: %s", w.Code, w.Body.String())
	}

	resp := w.Result()
	defer resp.Body.Close()
	var cookie *http.Cookie
	for _, c := range resp.Cookies() {
		if c.Name == middleware.AuthCookieName {
			cookie = c
			break
		}
	}
	if cookie == nil {
		t.Fatal("expected lg_token cookie")
	}
	if !cookie.HttpOnly {
		t.Error("expected httpOnly cookie")
	}
	if cookie.Value == "" {
		t.Error("expected non-empty cookie value")
	}
}

func TestLogout_ClearsCookie(t *testing.T) {
	db := setupDB(t)
	cfg := testConfig()
	denyList := middleware.NewJTIDenyList()

	c, w := setupContext(db, http.MethodPost, "/auth/logout", "")
	c.Request.AddCookie(&http.Cookie{Name: middleware.AuthCookieName, Value: "invalid.token.value"})

	Logout(cfg, denyList)(c)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	resp := w.Result()
	defer resp.Body.Close()
	for _, ck := range resp.Cookies() {
		if ck.Name == middleware.AuthCookieName {
			if ck.MaxAge != -1 {
				t.Errorf("expected clearing cookie MaxAge=-1, got %d", ck.MaxAge)
			}
			return
		}
	}
	t.Fatal("expected clear cookie in response")
}

func TestSession_ReturnsAuthenticated(t *testing.T) {
	exp := time.Now().Add(time.Hour).UTC()
	c, w := setupAdminContext(nil, http.MethodGet, "/auth/session", "", 1)
	c.Set("token_exp", exp)

	Session()(c)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	var body map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("parse response: %v", err)
	}
	data, ok := body["data"].(map[string]interface{})
	if !ok {
		t.Fatal("response missing data")
	}
	if auth, _ := data["authenticated"].(bool); !auth {
		t.Fatal("expected authenticated=true")
	}
}

func TestCreateNode_RejectsPrivateAgentURL(t *testing.T) {
	db := setupDB(t)
	body := `{"name":"remote","type":"lg_node","agent_url":"http://10.0.0.5:9090","enabled_cmds":["ping"]}`
	c, w := setupAdminContext(db, http.MethodPost, "/admin/nodes", body, 1)

	CreateNode(db, "")(c)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400; body: %s", w.Code, w.Body.String())
	}
}
