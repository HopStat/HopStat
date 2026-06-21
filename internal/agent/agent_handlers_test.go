package agent

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func agentAuthHeader(token string) string {
	return "Bearer " + token
}

func serveAgent(t *testing.T, agent *Agent, method, path string, body []byte) *httptest.ResponseRecorder {
	t.Helper()
	var reader *bytes.Reader
	if body != nil {
		reader = bytes.NewReader(body)
	} else {
		reader = bytes.NewReader(nil)
	}
	req := httptest.NewRequest(method, path, reader)
	req.Header.Set("Authorization", agentAuthHeader(agent.cfg.Agent.Token))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	agent.router.ServeHTTP(w, req)
	return w
}

func TestAgentRunShutdown(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := ln.Addr().(*net.TCPAddr).Port
	ln.Close()

	cfg := testAgentConfig()
	cfg.Server.Host = "127.0.0.1"
	cfg.Agent.Port = port
	agent := New(cfg)

	ctx, cancel := context.WithCancel(context.Background())
	errCh := make(chan error, 1)
	go func() {
		errCh <- agent.Run(ctx)
	}()

	client := &http.Client{Timeout: time.Second}
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		req, err := http.NewRequest(http.MethodGet, fmt.Sprintf("http://127.0.0.1:%d/agent/v1/health", port), nil)
		if err != nil {
			t.Fatal(err)
		}
		req.Header.Set("Authorization", agentAuthHeader(cfg.Agent.Token))
		resp, err := client.Do(req)
		if err == nil {
			resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				break
			}
		}
		time.Sleep(20 * time.Millisecond)
	}

	cancel()
	select {
	case err := <-errCh:
		if err != nil {
			t.Fatalf("Run returned error: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Run did not return after cancel")
	}
}

func TestAgentHandlePingSuccess(t *testing.T) {
	prependFakeBin(t, map[string]string{
		"ping": `cat <<'EOF'
` + samplePingOutput + `
EOF
exit 0`,
	})
	agent := New(testAgentConfig())
	body, _ := json.Marshal(map[string]any{"target": "8.8.8.8", "count": 2})
	w := serveAgent(t, agent, http.MethodPost, "/agent/v1/ping", body)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", w.Code, w.Body.String())
	}
}

func TestAgentHandlePingInvalidTarget(t *testing.T) {
	agent := New(testAgentConfig())
	body, _ := json.Marshal(map[string]any{"target": "127.0.0.1", "count": 1})
	w := serveAgent(t, agent, http.MethodPost, "/agent/v1/ping", body)
	if w.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500", w.Code)
	}
}

func TestAgentHandlePingCountDefaults(t *testing.T) {
	prependFakeBin(t, map[string]string{
		"ping": `echo "$@" 1>&2
cat <<'EOF'
` + samplePingOutput + `
EOF
exit 0`,
	})
	agent := New(testAgentConfig())
	body, _ := json.Marshal(map[string]any{"target": "8.8.8.8", "count": 0})
	w := serveAgent(t, agent, http.MethodPost, "/agent/v1/ping", body)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", w.Code, w.Body.String())
	}
	body, _ = json.Marshal(map[string]any{"target": "8.8.8.8", "count": 100})
	w = serveAgent(t, agent, http.MethodPost, "/agent/v1/ping", body)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", w.Code, w.Body.String())
	}
}

func TestAgentHandlePingEmptyOutputError(t *testing.T) {
	prependFakeBin(t, map[string]string{"ping": "exit 1"})
	agent := New(testAgentConfig())
	body, _ := json.Marshal(map[string]any{"target": "8.8.8.8", "count": 1})
	w := serveAgent(t, agent, http.MethodPost, "/agent/v1/ping", body)
	if w.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500", w.Code)
	}
}

func TestAgentHandleTracerouteSuccess(t *testing.T) {
	prependFakeBin(t, map[string]string{
		"traceroute": `cat <<'EOF'
` + sampleTracerouteOutput + `
EOF
exit 0`,
	})
	agent := New(testAgentConfig())
	body, _ := json.Marshal(map[string]any{"target": "8.8.8.8", "max_hops": 30})
	w := serveAgent(t, agent, http.MethodPost, "/agent/v1/traceroute", body)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", w.Code, w.Body.String())
	}
}

func TestAgentHandleTracerouteDefaults(t *testing.T) {
	prependFakeBin(t, map[string]string{
		"traceroute": `cat <<'EOF'
` + sampleTracerouteOutput + `
EOF
exit 0`,
	})
	agent := New(testAgentConfig())
	body, _ := json.Marshal(map[string]any{"target": "8.8.8.8", "max_hops": 0})
	w := serveAgent(t, agent, http.MethodPost, "/agent/v1/traceroute", body)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", w.Code, w.Body.String())
	}
	body, _ = json.Marshal(map[string]any{"target": "8.8.8.8", "max_hops": 128})
	w = serveAgent(t, agent, http.MethodPost, "/agent/v1/traceroute", body)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", w.Code, w.Body.String())
	}
}

func TestAgentHandleTracerouteEmptyOutputError(t *testing.T) {
	prependFakeBin(t, map[string]string{"traceroute": "exit 1"})
	agent := New(testAgentConfig())
	body, _ := json.Marshal(map[string]any{"target": "8.8.8.8"})
	w := serveAgent(t, agent, http.MethodPost, "/agent/v1/traceroute", body)
	if w.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500", w.Code)
	}
}

func TestAgentHandleBGPRouteSuccess(t *testing.T) {
	prependFakeBin(t, map[string]string{
		"birdc": `cat <<'EOF'
` + sampleBirdBGPOutput + `
EOF
exit 0`,
		"vtysh": "exit 1",
	})
	agent := New(testAgentConfig())
	body, _ := json.Marshal(map[string]any{"prefix": "8.8.8.8"})
	w := serveAgent(t, agent, http.MethodPost, "/agent/v1/bgp/route", body)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", w.Code, w.Body.String())
	}
}

func TestAgentHandleBGPRouteInvalidPrefix(t *testing.T) {
	agent := New(testAgentConfig())
	body, _ := json.Marshal(map[string]any{"prefix": "not-a-prefix/24"})
	w := serveAgent(t, agent, http.MethodPost, "/agent/v1/bgp/route", body)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", w.Code)
	}
}

func TestAgentHandleBGPRouteInvalidIP(t *testing.T) {
	agent := New(testAgentConfig())
	body, _ := json.Marshal(map[string]any{"prefix": "not-an-ip"})
	w := serveAgent(t, agent, http.MethodPost, "/agent/v1/bgp/route", body)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", w.Code)
	}
}

func TestAgentHandleBGPRouteLookupError(t *testing.T) {
	agent := New(testAgentConfig())
	body, _ := json.Marshal(map[string]any{"prefix": "127.0.0.1"})
	w := serveAgent(t, agent, http.MethodPost, "/agent/v1/bgp/route", body)
	if w.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500", w.Code)
	}
}

func TestAgentRunListenError(t *testing.T) {
	cfg := testAgentConfig()
	cfg.Server.Host = "127.0.0.1"
	cfg.Agent.Port = 1 // privileged port likely unavailable without root
	agent := New(cfg)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	errCh := make(chan error, 1)
	go func() {
		errCh <- agent.Run(ctx)
	}()

	select {
	case err := <-errCh:
		if err != nil {
			t.Fatalf("Run returned error: %v", err)
		}
	case <-time.After(2 * time.Second):
		cancel()
		if err := <-errCh; err != nil {
			t.Fatalf("Run returned error: %v", err)
		}
	}
}

func TestAgentHandleHealthAndCapabilitiesWithAuth(t *testing.T) {
	agent := New(testAgentConfig())
	w := serveAgent(t, agent, http.MethodGet, "/agent/v1/health", nil)
	if w.Code != http.StatusOK {
		t.Fatalf("health status = %d", w.Code)
	}
	w = serveAgent(t, agent, http.MethodGet, "/agent/v1/capabilities", nil)
	if w.Code != http.StatusOK {
		t.Fatalf("capabilities status = %d", w.Code)
	}
}
