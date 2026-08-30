//go:build smoke

package smoke

import (
	"bufio"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/HopStat/HopStat/internal/domain"
	"github.com/HopStat/HopStat/internal/store"
)

// The password the harness seeds so it can log in. There is no default admin password —
// without LG_ADMIN_PASSWORD the binary invents a random one and only prints it once.
const adminPassword = "smoke-harness-8Fq2xLp4"

// The token the fake agent expects. The node record stores it encrypted, so a successful
// query also proves the credential round-trip through the database.
const agentToken = "smoke-agent-3Kd9vQz1"

// The smoke target. 192.0.2.1 is TEST-NET-1: a literal IP, so target validation short
// circuits before any DNS, and it is not in the blocked ranges (loopback/private/CGNAT).
const smokeTarget = "192.0.2.1"

var (
	baseURL      string
	stubAgentURL string
	// Requests the fake agent received, so tests can assert the server actually reached it.
	agentHits = make(chan string, 64)
	// The real agent (internal/agent/agent.go) registers no /stream routes, so production
	// lg_node queries always 404 and fall back to the blocking endpoint. The stub mirrors
	// that by default; a test flips this on to pin the behaviour we would get if the agent
	// ever grew the streaming endpoints the driver already asks for.
	stubStreaming atomic.Bool
	// Flipped on to make the stub agent fail, so the harness can assert the server turns an
	// agent failure into a clean, credential-free error.
	stubFailing atomic.Bool
)

func TestMain(m *testing.M) {
	code, err := runSuite(m)
	if err != nil {
		fmt.Fprintf(os.Stderr, "smoke harness: %v\n", err)
		os.Exit(1)
	}
	os.Exit(code)
}

func runSuite(m *testing.M) (int, error) {
	if _, err := repoRoot(); err != nil {
		return 0, err
	}

	dir, err := os.MkdirTemp("", "hopstat-smoke-")
	if err != nil {
		return 0, err
	}
	defer os.RemoveAll(dir)

	binary := filepath.Join(dir, "hopstat")
	if out, err := exec.Command("go", "build", "-o", binary, "./cmd/lg/").CombinedOutput(); err != nil {
		return 0, fmt.Errorf("build binary: %v\n%s", err, out)
	}

	stub := startStubAgent()
	defer stub.Close()
	stubAgentURL = stub.URL

	port, err := freePort()
	if err != nil {
		return 0, err
	}
	bgpPort, err := freePort()
	if err != nil {
		return 0, err
	}
	configPath := filepath.Join(dir, "config.yaml")
	if err := writeConfig(configPath, filepath.Join(dir, "smoke.db"), port, bgpPort); err != nil {
		return 0, err
	}

	// --bootstrap creates the schema and the admin user, then exits. Doing it as its own
	// step means the server starts against a database that is already in its final shape.
	bootstrap := exec.Command(binary, "--bootstrap", "--config", configPath)
	bootstrap.Env = append(os.Environ(),
		"LG_ADMIN_PASSWORD="+adminPassword,
		"LG_FORCE_ADMIN_PASSWORD=1",
	)
	bootstrap.Dir = dir
	if out, err := bootstrap.CombinedOutput(); err != nil {
		return 0, fmt.Errorf("bootstrap: %v\n%s", err, out)
	}

	server := exec.Command(binary, "--mode", "server", "--config", configPath)
	server.Dir = dir
	var serverLog strings.Builder
	server.Stdout = &serverLog
	server.Stderr = &serverLog
	if err := server.Start(); err != nil {
		return 0, fmt.Errorf("start server: %w", err)
	}
	defer func() {
		_ = server.Process.Kill()
		_, _ = server.Process.Wait()
	}()

	baseURL = fmt.Sprintf("http://127.0.0.1:%d", port)
	if err := waitHealthy(baseURL); err != nil {
		return 0, fmt.Errorf("%w\n--- server output ---\n%s", err, serverLog.String())
	}

	// Seeded over the real API rather than by writing SQL, so node creation is itself
	// covered. A fresh database seeds no nodes, so without this every query would 404.
	if err := seedNode(); err != nil {
		return 0, fmt.Errorf("%w\n--- server output ---\n%s", err, serverLog.String())
	}

	code := m.Run()
	if code != 0 {
		fmt.Fprintf(os.Stderr, "--- server output ---\n%s\n", serverLog.String())
	}
	return code, nil
}

func repoRoot() (string, error) {
	dir, err := os.Getwd()
	if err != nil {
		return "", err
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir, os.Chdir(dir)
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", fmt.Errorf("go.mod not found above %s", dir)
		}
		dir = parent
	}
}

func freePort() (int, error) {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return 0, err
	}
	defer l.Close()
	return l.Addr().(*net.TCPAddr).Port, nil
}

func randomHex(n int) string {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		panic(err)
	}
	return hex.EncodeToString(b)
}

func writeConfig(path, dbPath string, port, bgpPort int) error {
	// Written rather than auto-generated so the port and database path are exact. Viper's
	// AutomaticEnv does not reliably reach Unmarshal, so LG_SERVER_PORT is not dependable.
	cfg := fmt.Sprintf(`server:
  host: "127.0.0.1"
  port: %d
  mode: "server"

database:
  path: %q

security:
  jwt_secret: %q
  credential_key: %q

flood_control:
  enabled: false

query:
  max_concurrent: 8
  default_timeout_sec: 10
  traceroute_timeout_sec: 10

bgp:
  listen_port: %d
  local_as: 0
`, port, dbPath, randomHex(32), randomHex(32), bgpPort)
	return os.WriteFile(path, []byte(cfg), 0o600)
}

func waitHealthy(base string) error {
	deadline := time.Now().Add(30 * time.Second)
	for time.Now().Before(deadline) {
		resp, err := http.Get(base + "/health")
		if err == nil {
			_ = resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				return nil
			}
		}
		time.Sleep(100 * time.Millisecond)
	}
	return fmt.Errorf("server did not become healthy at %s", base)
}

// seedNodeID is the lg_node every query test runs against.
var seedNodeID int64

func seedNode() error {
	jar, err := cookiejar.New(nil)
	if err != nil {
		return err
	}
	c := &http.Client{Jar: jar, Timeout: 30 * time.Second}

	login, _ := json.Marshal(map[string]string{
		"email": store.DefaultAdminEmail, "password": adminPassword,
	})
	resp, err := c.Post(baseURL+"/api/v1/auth/login", "application/json", strings.NewReader(string(login)))
	if err != nil {
		return fmt.Errorf("seed login: %w", err)
	}
	body, _ := io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("seed login status %d: %s", resp.StatusCode, body)
	}

	node, _ := json.Marshal(map[string]interface{}{
		"name":         "SMOKE",
		"type":         "lg_node",
		"city":         "Testville",
		"country":      "TR",
		"active":       true,
		"agent_url":    stubAgentURL,
		"agent_token":  agentToken,
		"enabled_cmds": []string{"ping", "traceroute", "bgp_route"},
	})
	resp, err = c.Post(baseURL+"/api/v1/admin/nodes", "application/json", strings.NewReader(string(node)))
	if err != nil {
		return fmt.Errorf("seed node: %w", err)
	}
	body, _ = io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		return fmt.Errorf("seed node status %d: %s", resp.StatusCode, body)
	}

	var envelope struct {
		Data struct {
			ID int64 `json:"id"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &envelope); err != nil {
		return fmt.Errorf("decode seeded node: %w (body %s)", err, body)
	}
	if envelope.Data.ID == 0 {
		return fmt.Errorf("seeded node has no id: %s", body)
	}
	seedNodeID = envelope.Data.ID
	return nil
}

// startStubAgent stands in for a remote lg_node agent. Pointing a node at it exercises the
// whole query pipeline — handler, engine, pool, driver, parser, store, SSE — without ICMP,
// root, or any external network.
func startStubAgent() *httptest.Server {
	mux := http.NewServeMux()

	authorized := func(w http.ResponseWriter, r *http.Request) bool {
		if r.Header.Get("Authorization") != "Bearer "+agentToken {
			w.WriteHeader(http.StatusUnauthorized)
			return false
		}
		select {
		case agentHits <- r.URL.Path:
		default:
		}
		if stubFailing.Load() {
			w.WriteHeader(http.StatusInternalServerError)
			return false
		}
		return true
	}

	mux.HandleFunc("/agent/v1/health", func(w http.ResponseWriter, r *http.Request) {
		if !authorized(w, r) {
			return
		}
		writeJSON(w, map[string]string{"status": "ok"})
	})

	mux.HandleFunc("/agent/v1/ping", func(w http.ResponseWriter, r *http.Request) {
		if !authorized(w, r) {
			return
		}
		writeJSON(w, stubPingResult())
	})

	mux.HandleFunc("/agent/v1/traceroute", func(w http.ResponseWriter, r *http.Request) {
		if !authorized(w, r) {
			return
		}
		writeJSON(w, stubTracerouteResult())
	})

	mux.HandleFunc("/agent/v1/bgp/route", func(w http.ResponseWriter, r *http.Request) {
		if !authorized(w, r) {
			return
		}
		writeJSON(w, stubBGPResult())
	})

	// The server sets an OnLine callback for every query, so these streaming variants are
	// the paths production actually takes.
	mux.HandleFunc("/agent/v1/ping/stream", func(w http.ResponseWriter, r *http.Request) {
		if !authorized(w, r) {
			return
		}
		if !stubStreaming.Load() {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		streamAgentResult(w, []string{
			"PING " + smokeTarget + ": 56 data bytes",
			"64 bytes from " + smokeTarget + ": icmp_seq=0 ttl=57 time=11.4 ms",
		}, stubPingResult())
	})

	mux.HandleFunc("/agent/v1/traceroute/stream", func(w http.ResponseWriter, r *http.Request) {
		if !authorized(w, r) {
			return
		}
		if !stubStreaming.Load() {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		streamAgentResult(w, []string{
			"traceroute to " + smokeTarget + ", 30 hops max",
			" 1  10.0.0.1  1.201 ms",
			" 2  " + smokeTarget + "  11.402 ms",
		}, stubTracerouteResult())
	})

	return httptest.NewServer(mux)
}

func stubPingResult() domain.PingResult {
	return domain.PingResult{
		PacketsSent: 2,
		PacketsRecv: 2,
		PacketLoss:  0,
		MinRTT:      10.9,
		AvgRTT:      11.4,
		MaxRTT:      12.1,
		Raw:         "2 packets transmitted, 2 received, 0% packet loss",
	}
}

func stubTracerouteResult() domain.TracerouteResult {
	return domain.TracerouteResult{
		Hops: []domain.Hop{
			{Number: 1, Host: "10.0.0.1", IP: "10.0.0.1", RTT: []float64{1.2}},
			{Number: 2, Host: smokeTarget, IP: smokeTarget, RTT: []float64{11.4}},
		},
		Raw: "traceroute to " + smokeTarget + ", 30 hops max",
	}
}

func stubBGPResult() domain.BGPResult {
	return domain.BGPResult{
		Routes: []domain.BGPRoute{{
			Prefix:      "1.1.1.0/24",
			NextHop:     "10.0.0.1",
			ASPath:      []uint32{64500, 13335},
			Origin:      "IGP",
			Status:      "*>",
			Protocol:    "BGP",
			Best:        true,
			Communities: []string{"64500:100"},
		}},
		Raw: "BGP routing table entry for 1.1.1.0/24",
	}
}

func streamAgentResult(w http.ResponseWriter, lines []string, result interface{}) {
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	flusher, ok := w.(http.Flusher)
	if !ok {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	for _, line := range lines {
		payload, _ := json.Marshal(map[string]string{"line": line})
		fmt.Fprintf(w, "event: output\ndata: %s\n\n", payload)
		flusher.Flush()
	}
	payload, _ := json.Marshal(result)
	fmt.Fprintf(w, "event: result\ndata: %s\n\n", payload)
	flusher.Flush()
}

func writeJSON(w http.ResponseWriter, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}

// --- client helpers -------------------------------------------------------------------

// newClient returns an HTTP client with a cookie jar, so a login carries into later calls
// exactly the way a browser would hold the lg_token cookie.
func newClient(t *testing.T) *http.Client {
	t.Helper()
	jar, err := cookiejar.New(nil)
	if err != nil {
		t.Fatalf("cookie jar: %v", err)
	}
	return &http.Client{Jar: jar, Timeout: 30 * time.Second}
}

func loggedInClient(t *testing.T) *http.Client {
	t.Helper()
	c := newClient(t)
	body := map[string]string{"email": store.DefaultAdminEmail, "password": adminPassword}
	resp := doJSON(t, c, http.MethodPost, "/api/v1/auth/login", body)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("login status = %d, want 200", resp.StatusCode)
	}
	return c
}

func doJSON(t *testing.T, c *http.Client, method, path string, body interface{}) *http.Response {
	t.Helper()
	var reader io.Reader
	if body != nil {
		encoded, err := json.Marshal(body)
		if err != nil {
			t.Fatalf("marshal body: %v", err)
		}
		reader = strings.NewReader(string(encoded))
	}
	req, err := http.NewRequest(method, baseURL+path, reader)
	if err != nil {
		t.Fatalf("build %s %s: %v", method, path, err)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := c.Do(req)
	if err != nil {
		t.Fatalf("%s %s: %v", method, path, err)
	}
	return resp
}

// decodeData unwraps the {"data": …} envelope every successful endpoint returns.
func decodeData(t *testing.T, resp *http.Response, into interface{}) {
	t.Helper()
	var envelope struct {
		Data json.RawMessage `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&envelope); err != nil {
		t.Fatalf("decode envelope: %v", err)
	}
	if into != nil {
		if err := json.Unmarshal(envelope.Data, into); err != nil {
			t.Fatalf("decode data: %v", err)
		}
	}
}

func decodeError(t *testing.T, resp *http.Response) string {
	t.Helper()
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read error body: %v", err)
	}
	return string(raw)
}

// readSSEUntilResult consumes the query stream and returns the event names seen in order
// plus the payload of the terminal `result` event.
func readSSEUntilResult(t *testing.T, c *http.Client, queryID string) ([]string, map[string]interface{}) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, baseURL+"/api/v1/query/"+queryID+"/stream", nil)
	if err != nil {
		t.Fatalf("build stream request: %v", err)
	}
	req.Header.Set("Accept", "text/event-stream")
	resp, err := c.Do(req)
	if err != nil {
		t.Fatalf("open stream: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("stream status = %d, want 200", resp.StatusCode)
	}

	var events []string
	var result map[string]interface{}
	scanner := bufio.NewScanner(resp.Body)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	event := ""
	for scanner.Scan() {
		line := scanner.Text()
		switch {
		case strings.HasPrefix(line, "event: "):
			event = strings.TrimPrefix(line, "event: ")
			events = append(events, event)
		case strings.HasPrefix(line, "data: "):
			if event == "result" {
				if err := json.Unmarshal([]byte(strings.TrimPrefix(line, "data: ")), &result); err != nil {
					t.Fatalf("decode result event: %v", err)
				}
				return events, result
			}
		}
	}
	t.Fatalf("stream ended without a result event; saw %v", events)
	return events, result
}
