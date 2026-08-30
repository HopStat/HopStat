//go:build smoke

package smoke

import (
	"encoding/json"
	"io"
	"net/http"
	"strconv"
	"strings"
	"testing"
)

func TestHealthReportsServerMode(t *testing.T) {
	resp := doJSON(t, newClient(t), http.MethodGet, "/health", nil)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	var health map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&health); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if health["status"] != "ok" {
		t.Errorf("status = %v, want ok", health["status"])
	}
	if health["mode"] != "server" {
		t.Errorf("mode = %v, want server", health["mode"])
	}
}

func TestPublicSettingsAreServed(t *testing.T) {
	resp := doJSON(t, newClient(t), http.MethodGet, "/api/v1/settings", nil)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	var settings map[string]interface{}
	decodeData(t, resp, &settings)
	if _, ok := settings["header_color"]; !ok {
		t.Errorf("header_color missing from public settings: %v", settings)
	}
}

// The public node list must never carry the agent token — it is the credential a caller
// would need to impersonate the server against the remote agent.
func TestPublicNodeListHidesAgentToken(t *testing.T) {
	resp := doJSON(t, newClient(t), http.MethodGet, "/api/v1/nodes", nil)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}
	if strings.Contains(string(raw), agentToken) {
		t.Fatalf("public node list leaked the agent token: %s", raw)
	}

	var nodes []map[string]interface{}
	if err := json.Unmarshal(extractData(t, raw), &nodes); err != nil {
		t.Fatalf("decode nodes: %v", err)
	}
	if len(nodes) != 1 {
		t.Fatalf("node count = %d, want the one seeded node", len(nodes))
	}
	if nodes[0]["name"] != "SMOKE" {
		t.Errorf("node name = %v, want SMOKE", nodes[0]["name"])
	}
}

func TestAdminRoutesRejectAnonymousCallers(t *testing.T) {
	for _, path := range []string{"/api/v1/admin/nodes", "/api/v1/admin/settings", "/api/v1/admin/audit"} {
		t.Run(path, func(t *testing.T) {
			resp := doJSON(t, newClient(t), http.MethodGet, path, nil)
			defer resp.Body.Close()
			if resp.StatusCode != http.StatusUnauthorized {
				t.Fatalf("status = %d, want 401", resp.StatusCode)
			}
		})
	}
}

func TestLoginSessionLogoutCycle(t *testing.T) {
	c := newClient(t)

	bad := doJSON(t, c, http.MethodPost, "/api/v1/auth/login",
		map[string]string{"email": "admin@hopstat.local", "password": "definitely-not-it"})
	if bad.StatusCode != http.StatusUnauthorized {
		t.Fatalf("bad password status = %d, want 401", bad.StatusCode)
	}
	_ = bad.Body.Close()

	good := doJSON(t, c, http.MethodPost, "/api/v1/auth/login",
		map[string]string{"email": "admin@hopstat.local", "password": adminPassword})
	if good.StatusCode != http.StatusOK {
		t.Fatalf("login status = %d, want 200", good.StatusCode)
	}
	// The token rides in an httpOnly cookie; the body must not hand it back.
	loginBody, _ := io.ReadAll(good.Body)
	_ = good.Body.Close()
	if strings.Contains(string(loginBody), "token") {
		t.Errorf("login body should not carry a token: %s", loginBody)
	}
	var sawAuthCookie bool
	for _, cookie := range good.Cookies() {
		if cookie.Name == "lg_token" {
			sawAuthCookie = true
			if !cookie.HttpOnly {
				t.Error("lg_token cookie is not HttpOnly")
			}
		}
	}
	if !sawAuthCookie {
		t.Fatal("login did not set the lg_token cookie")
	}

	session := doJSON(t, c, http.MethodGet, "/api/v1/auth/session", nil)
	if session.StatusCode != http.StatusOK {
		t.Fatalf("session status = %d, want 200", session.StatusCode)
	}
	_ = session.Body.Close()

	logout := doJSON(t, c, http.MethodPost, "/api/v1/auth/logout", nil)
	if logout.StatusCode != http.StatusOK {
		t.Fatalf("logout status = %d, want 200", logout.StatusCode)
	}
	_ = logout.Body.Close()

	// Proves the revocation deny-list actually takes effect, not just that the cookie cleared.
	after := doJSON(t, c, http.MethodGet, "/api/v1/auth/session", nil)
	defer after.Body.Close()
	if after.StatusCode != http.StatusUnauthorized {
		t.Fatalf("session after logout = %d, want 401", after.StatusCode)
	}
}

// The API namespace must 404 as JSON rather than falling through to the SPA shell.
func TestUnknownAPIPathIsJSONNotTheSPA(t *testing.T) {
	resp := doJSON(t, newClient(t), http.MethodGet, "/api/v1/no-such-endpoint", nil)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", resp.StatusCode)
	}
	body := decodeError(t, resp)
	if strings.Contains(body, "<!doctype html") || strings.Contains(body, "<html") {
		t.Fatalf("API 404 fell through to the SPA shell: %s", body)
	}
}

func TestSPAShellCarriesTheBootstrapPayload(t *testing.T) {
	resp := doJSON(t, newClient(t), http.MethodGet, "/", nil)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	body := decodeError(t, resp)
	// Injected server-side so the page paints in the operator's colour before React runs.
	if !strings.Contains(body, "window.__HOPSTAT_BOOTSTRAP__") {
		t.Fatalf("index.html served without the bootstrap payload:\n%s", truncate(body, 600))
	}
	if !strings.Contains(body, "header_color") {
		t.Errorf("bootstrap payload carries no header_color:\n%s", truncate(body, 600))
	}
}

func TestDeepSPARouteServesTheShell(t *testing.T) {
	resp := doJSON(t, newClient(t), http.MethodGet, "/admin/nodes", nil)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	if ct := resp.Header.Get("Content-Type"); !strings.Contains(ct, "text/html") {
		t.Errorf("content type = %q, want html", ct)
	}
}

func TestTargetValidationRejectsUnsafeAndBlockedTargets(t *testing.T) {
	c := loggedInClient(t)
	for _, tc := range []struct{ name, target string }{
		{"shell metacharacters", "1.1.1.1; rm -rf /"},
		{"loopback", "127.0.0.1"},
		{"private range", "10.0.0.1"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			resp := doJSON(t, c, http.MethodPost, "/api/v1/query", map[string]interface{}{
				"node_id": seedNodeID, "command": "ping", "target": tc.target,
			})
			defer resp.Body.Close()
			if resp.StatusCode != http.StatusBadRequest {
				t.Fatalf("status = %d, want 400 for %q", resp.StatusCode, tc.target)
			}
		})
	}
}

func TestUnknownNodeIsRejected(t *testing.T) {
	resp := doJSON(t, loggedInClient(t), http.MethodPost, "/api/v1/query", map[string]interface{}{
		"node_id": 999999, "command": "ping", "target": smokeTarget,
	})
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusOK {
		t.Fatalf("querying a nonexistent node succeeded, want a client error")
	}
}

func TestUnknownQueryStreamIs404(t *testing.T) {
	resp := doJSON(t, newClient(t), http.MethodGet,
		"/api/v1/query/00000000-0000-0000-0000-000000000000/stream", nil)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", resp.StatusCode)
	}
}

func TestAdminNodeCRUDRoundTrip(t *testing.T) {
	c := loggedInClient(t)

	created := doJSON(t, c, http.MethodPost, "/api/v1/admin/nodes", map[string]interface{}{
		"name": "CRUD-SMOKE", "type": "lg_node", "active": true,
		"agent_url": stubAgentURL, "agent_token": agentToken,
		"enabled_cmds": []string{"ping"},
	})
	if created.StatusCode != http.StatusOK && created.StatusCode != http.StatusCreated {
		t.Fatalf("create status = %d", created.StatusCode)
	}
	var node struct {
		ID   int64  `json:"id"`
		Name string `json:"name"`
	}
	decodeData(t, created, &node)
	_ = created.Body.Close()
	if node.ID == 0 {
		t.Fatal("created node has no id")
	}

	path := "/api/v1/admin/nodes/" + itoa(node.ID)

	got := doJSON(t, c, http.MethodGet, path, nil)
	if got.StatusCode != http.StatusOK {
		t.Fatalf("get status = %d", got.StatusCode)
	}
	_ = got.Body.Close()

	updated := doJSON(t, c, http.MethodPut, path, map[string]interface{}{
		"name": "CRUD-SMOKE-RENAMED", "type": "lg_node", "active": true,
		"agent_url": stubAgentURL, "agent_token": agentToken,
		"enabled_cmds": []string{"ping", "traceroute"},
	})
	if updated.StatusCode != http.StatusOK {
		t.Fatalf("update status = %d", updated.StatusCode)
	}
	_ = updated.Body.Close()

	deleted := doJSON(t, c, http.MethodDelete, path, nil)
	if deleted.StatusCode != http.StatusOK && deleted.StatusCode != http.StatusNoContent {
		t.Fatalf("delete status = %d", deleted.StatusCode)
	}
	_ = deleted.Body.Close()

	gone := doJSON(t, c, http.MethodGet, path, nil)
	defer gone.Body.Close()
	if gone.StatusCode != http.StatusNotFound {
		t.Fatalf("get after delete = %d, want 404", gone.StatusCode)
	}
}

func extractData(t *testing.T, raw []byte) []byte {
	t.Helper()
	var envelope struct {
		Data json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal(raw, &envelope); err != nil {
		t.Fatalf("decode envelope: %v (body %s)", err, raw)
	}
	return envelope.Data
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}

func itoa(n int64) string {
	return strconv.FormatInt(n, 10)
}
