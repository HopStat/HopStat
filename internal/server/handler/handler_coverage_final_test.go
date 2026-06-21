package handler

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/csv"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/HopStat/HopStat/internal/bgp"
	"github.com/HopStat/HopStat/internal/config"
	"github.com/HopStat/HopStat/internal/domain"
	"github.com/HopStat/HopStat/internal/driver"
	"github.com/HopStat/HopStat/internal/engine"
	"github.com/HopStat/HopStat/internal/geo"
	"github.com/HopStat/HopStat/internal/server/middleware"
	"github.com/HopStat/HopStat/internal/store"
	"github.com/HopStat/HopStat/internal/store/queries"
	"github.com/HopStat/HopStat/internal/store/repo"
	"github.com/HopStat/HopStat/internal/updater"
)

type stubAgentDriver struct {
	pingErr        error
	pingResult     *domain.PingResult
	tracerouteErr  error
	tracerouteRes  *domain.TracerouteResult
	bgpErr         error
	bgpRes         *domain.BGPResult
	createErr      error
	onLine         func(string)
}

func (s stubAgentDriver) Ping(ctx context.Context, target string, count int) (*domain.PingResult, error) {
	if fn := domain.GetOnLine(ctx); fn != nil {
		fn("ping line")
	} else if s.onLine != nil {
		s.onLine("ping line")
	}
	if s.pingErr != nil {
		return s.pingResult, s.pingErr
	}
	return &domain.PingResult{Raw: "ok"}, nil
}

func (s stubAgentDriver) Traceroute(ctx context.Context, target string, maxHops int) (*domain.TracerouteResult, error) {
	if fn := domain.GetOnLine(ctx); fn != nil {
		fn("trace line")
	} else if s.onLine != nil {
		s.onLine("trace line")
	}
	if s.tracerouteErr != nil {
		return s.tracerouteRes, s.tracerouteErr
	}
	return &domain.TracerouteResult{Raw: "ok"}, nil
}

func (s stubAgentDriver) BGPRoute(ctx context.Context, prefix string) (*domain.BGPResult, error) {
	if s.bgpErr != nil {
		return s.bgpRes, s.bgpErr
	}
	return &domain.BGPResult{Raw: "ok"}, nil
}

func (s stubAgentDriver) Capabilities() []domain.CommandType { return domain.DefaultEnabledCmds() }
func (s stubAgentDriver) TestConnection(ctx context.Context) error { return nil }

func withStubAgentDriver(t *testing.T, stub stubAgentDriver) {
	t.Helper()
	prev := newAgentDriver
	newAgentDriver = func(_ *domain.Node, _ *config.Config) (driver.NodeDriver, error) {
		if stub.createErr != nil {
			return nil, stub.createErr
		}
		return stub, nil
	}
	t.Cleanup(func() { newAgentDriver = prev })
}

func TestAgentAPI_StubDriverPaths(t *testing.T) {
	db := setupDB(t)

	t.Run("create driver error", func(t *testing.T) {
		withStubAgentDriver(t, stubAgentDriver{createErr: errors.New("boom")})
		r := setupAgentRouter(t, db, &domain.Node{
			Name: "local", Type: domain.NodeTypeStandalone, AgentToken: "node-secret",
			EnabledCmds: domain.DefaultEnabledCmds(), Active: true,
		})
		req := httptest.NewRequest(http.MethodPost, "/agent/v1/ping", strings.NewReader(`{"target":"1.1.1.1"}`))
		req.Header.Set("Authorization", "Bearer node-secret")
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		if w.Code != http.StatusInternalServerError {
			t.Fatalf("status = %d", w.Code)
		}
	})

	t.Run("ping error with raw", func(t *testing.T) {
		withStubAgentDriver(t, stubAgentDriver{
			pingErr:    errors.New("ping failed"),
			pingResult: &domain.PingResult{Raw: "partial"},
		})
		r := setupAgentRouter(t, db, &domain.Node{
			Name: "local", Type: domain.NodeTypeStandalone, AgentToken: "node-secret",
			EnabledCmds: domain.DefaultEnabledCmds(), Active: true,
		})
		req := httptest.NewRequest(http.MethodPost, "/agent/v1/ping", strings.NewReader(`{"target":"1.1.1.1","count":2}`))
		req.Header.Set("Authorization", "Bearer node-secret")
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		if w.Code != http.StatusInternalServerError {
			t.Fatalf("status = %d, body = %s", w.Code, w.Body.String())
		}
	})

	t.Run("traceroute error", func(t *testing.T) {
		withStubAgentDriver(t, stubAgentDriver{
			tracerouteErr: errors.New("trace failed"),
			tracerouteRes: &domain.TracerouteResult{Raw: "partial"},
		})
		r := setupAgentRouter(t, db, &domain.Node{
			Name: "local", Type: domain.NodeTypeStandalone, AgentToken: "node-secret",
			EnabledCmds: domain.DefaultEnabledCmds(), Active: true,
		})
		req := httptest.NewRequest(http.MethodPost, "/agent/v1/traceroute", strings.NewReader(`{"target":"1.1.1.1","max_hops":2}`))
		req.Header.Set("Authorization", "Bearer node-secret")
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		if w.Code != http.StatusInternalServerError {
			t.Fatalf("status = %d", w.Code)
		}
	})

	t.Run("ping stream success", func(t *testing.T) {
		withStubAgentDriver(t, stubAgentDriver{})
		r := setupAgentRouter(t, db, &domain.Node{
			Name: "local", Type: domain.NodeTypeStandalone, AgentToken: "node-secret",
			EnabledCmds: domain.DefaultEnabledCmds(), Active: true,
		})
		req := httptest.NewRequest(http.MethodPost, "/agent/v1/ping/stream", strings.NewReader(`{"target":"1.1.1.1","count":1}`))
		req.Header.Set("Authorization", "Bearer node-secret")
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		if w.Code != http.StatusOK {
			t.Fatalf("status = %d, body = %s", w.Code, w.Body.String())
		}
	})

	t.Run("ping stream error", func(t *testing.T) {
		withStubAgentDriver(t, stubAgentDriver{
			pingErr:    errors.New("stream fail"),
			pingResult: &domain.PingResult{Raw: "raw-out"},
		})
		r := setupAgentRouter(t, db, &domain.Node{
			Name: "local", Type: domain.NodeTypeStandalone, AgentToken: "node-secret",
			EnabledCmds: domain.DefaultEnabledCmds(), Active: true,
		})
		req := httptest.NewRequest(http.MethodPost, "/agent/v1/ping/stream", strings.NewReader(`{"target":"1.1.1.1"}`))
		req.Header.Set("Authorization", "Bearer node-secret")
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		if w.Code != http.StatusOK {
			t.Fatalf("status = %d", w.Code)
		}
		if !strings.Contains(w.Body.String(), "event: error") {
			t.Fatalf("body = %q", w.Body.String())
		}
	})

	t.Run("traceroute stream success and error", func(t *testing.T) {
		withStubAgentDriver(t, stubAgentDriver{
			tracerouteErr: errors.New("stream trace fail"),
			tracerouteRes: &domain.TracerouteResult{Raw: "raw"},
		})
		r := setupAgentRouter(t, db, &domain.Node{
			Name: "local", Type: domain.NodeTypeStandalone, AgentToken: "node-secret",
			EnabledCmds: domain.DefaultEnabledCmds(), Active: true,
		})
		req := httptest.NewRequest(http.MethodPost, "/agent/v1/traceroute/stream", strings.NewReader(`{"target":"1.1.1.1","max_hops":1}`))
		req.Header.Set("Authorization", "Bearer node-secret")
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		if w.Code != http.StatusOK {
			t.Fatalf("status = %d", w.Code)
		}
	})

	t.Run("bgp route driver error", func(t *testing.T) {
		withStubAgentDriver(t, stubAgentDriver{bgpErr: errors.New("bgp failed")})
		r := setupAgentRouter(t, db, &domain.Node{
			Name: "local", Type: domain.NodeTypeStandalone, AgentToken: "node-secret",
			EnabledCmds: domain.DefaultEnabledCmds(), Active: true,
		})
		req := httptest.NewRequest(http.MethodPost, "/agent/v1/bgp/route", strings.NewReader(`{"prefix":"8.8.8.8"}`))
		req.Header.Set("Authorization", "Bearer node-secret")
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		if w.Code != http.StatusInternalServerError {
			t.Fatalf("status = %d", w.Code)
		}
	})
}

func TestAgentBGPRoute_WithBGPManager(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	bgpCfg := config.BGPConfig{LocalAS: 65000, RouterID: "10.0.0.1", ListenPort: freeListenPort(t)}
	mgr := bgp.NewSessionManager(bgpCfg)
	if err := mgr.Start(ctx); err != nil {
		t.Fatal(err)
	}
	defer mgr.Stop()

	gin.SetMode(gin.TestMode)
	r := gin.New()
	agent := r.Group("")
	agent.Use(func(c *gin.Context) {
		c.Set(middleware.AgentNodeKey, &domain.Node{ID: 1, Name: "local", EnabledCmds: domain.DefaultEnabledCmds()})
		c.Next()
	})
	MountAgentAPI(agent, testConfig(), mgr, geo.New("", ""))

	req := httptest.NewRequest(http.MethodPost, "/agent/v1/bgp/route", strings.NewReader(`{"prefix":"8.8.8.8"}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK && w.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, body = %s", w.Code, w.Body.String())
	}
}

func TestBGPNeighborValidate_AllErrors(t *testing.T) {
	cases := []struct {
		req  bgpNeighborRequest
		want string
	}{
		{bgpNeighborRequest{RemoteAS: 1}, "node is required"},
		{bgpNeighborRequest{NodeID: 1}, "remote_as must be > 0"},
		{bgpNeighborRequest{NodeID: 1, RemoteAS: 1, PeeringIP: "1.2.3.4"}, "neighbor_ip is required when peering_ip is set"},
		{bgpNeighborRequest{NodeID: 1, RemoteAS: 1, NeighborIP: "1.2.3.4"}, "peering_ip is required when neighbor_ip is set"},
		{bgpNeighborRequest{NodeID: 1, RemoteAS: 1, IPv6PeeringIP: "2001:db8::1"}, "ipv6_neighbor_ip is required when ipv6_peering_ip is set"},
		{bgpNeighborRequest{NodeID: 1, RemoteAS: 1, IPv6NeighborIP: "2001:db8::2"}, "ipv6_peering_ip is required when ipv6_neighbor_ip is set"},
		{bgpNeighborRequest{NodeID: 1, RemoteAS: 1, PeeringIP: "bad", NeighborIP: "1.2.3.4"}, "invalid peering_ip"},
		{bgpNeighborRequest{NodeID: 1, RemoteAS: 1, PeeringIP: "1.2.3.4", NeighborIP: "bad"}, "invalid neighbor_ip"},
		{bgpNeighborRequest{NodeID: 1, RemoteAS: 1, IPv6PeeringIP: "bad", IPv6NeighborIP: "2001:db8::2"}, "invalid ipv6_peering_ip"},
		{bgpNeighborRequest{NodeID: 1, RemoteAS: 1, IPv6PeeringIP: "2001:db8::1", IPv6NeighborIP: "bad"}, "invalid ipv6_neighbor_ip"},
	}
	for _, tc := range cases {
		if got := tc.req.Validate(); got != tc.want {
			t.Fatalf("Validate(%+v) = %q, want %q", tc.req, got, tc.want)
		}
	}
}

func TestDeleteNode_RemoveBGPFailure(t *testing.T) {
	db := setupDB(t)
	nodeRepo := repo.NewNodeRepo(db, "")
	bgpRepo := repo.NewBGPNeighborRepo(db)
	created, _ := nodeRepo.Create(context.Background(), &domain.Node{
		Name: "bgp", Type: domain.NodeTypeStandalone, Active: true,
		EnabledCmds: domain.DefaultEnabledCmds(),
	})
	_, _ = bgpRepo.Create(context.Background(), &domain.BGPNeighbor{
		NodeID: created.ID, LocalAS: 65000, RemoteAS: 174,
		PeeringIP: "10.0.0.1", NeighborIP: "10.0.0.2",
	})

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	mgr := bgp.NewSessionManager(config.BGPConfig{LocalAS: 65000, RouterID: "10.0.0.1", ListenPort: freeListenPort(t)})
	_ = mgr.Start(ctx)
	defer mgr.Stop()

	db.Close()
	c, w := setupAdminContext(db, http.MethodDelete, fmt.Sprintf("/admin/nodes/%d", created.ID), "", 1)
	c.Params = gin.Params{{Key: "id", Value: fmt.Sprint(created.ID)}}
	DeleteNode(db, "", mgr)(c)
	if w.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d", w.Code)
	}
}

func TestDeleteNode_DeleteFailure(t *testing.T) {
	db := setupDB(t)
	nodeRepo := repo.NewNodeRepo(db, "")
	created, _ := nodeRepo.Create(context.Background(), &domain.Node{
		Name: "gone", Type: domain.NodeTypeStandalone, Active: true,
		EnabledCmds: domain.DefaultEnabledCmds(),
	})
	db.Close()

	c, w := setupAdminContext(db, http.MethodDelete, fmt.Sprintf("/admin/nodes/%d", created.ID), "", 1)
	c.Params = gin.Params{{Key: "id", Value: fmt.Sprint(created.ID)}}
	DeleteNode(db, "", nil)(c)
	if w.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d", w.Code)
	}
}

func TestLogin_JWTGenerationUsesShortSecret(t *testing.T) {
	db := setupDB(t)
	cfg := testConfig()
	cfg.Security.JWTSecret = "short"
	seedAdminPassword(t, db, "password12345")
	body := fmt.Sprintf(`{"email":%q,"password":"password12345"}`, store.DefaultAdminEmail)
	c, w := setupContext(db, http.MethodPost, "/auth/login", body)
	Login(db, cfg)(c)
	if w.Code != http.StatusOK && w.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d", w.Code)
	}
}

func TestUpdateAccount_UniqueConstraint(t *testing.T) {
	db := setupDB(t)
	adminID := seedAdminPassword(t, db, "password12345")
	if _, err := db.Exec(`INSERT INTO users (email, password_hash, created_at) VALUES (?, ?, datetime('now'))`,
		"dup@hopstat.local", "hash"); err != nil {
		t.Fatal(err)
	}
	body := `{"email":"dup@hopstat.local","current_password":"password12345","new_password":"newpassword12"}`
	c, w := setupAdminContext(db, http.MethodPut, "/admin/account", body, adminID)
	UpdateAccount(db)(c)
	if w.Code != http.StatusConflict {
		t.Fatalf("status = %d, body = %s", w.Code, w.Body.String())
	}
}

func TestUploadLogo_WebP(t *testing.T) {
	db := setupDB(t)
	uploadDir := t.TempDir()
	SetLogoUploadsDir(uploadDir)

	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	part, _ := writer.CreateFormFile("logo", "logo.webp")
	// RIFF....WEBP
	_, _ = part.Write([]byte{'R', 'I', 'F', 'F', 0, 0, 0, 0, 'W', 'E', 'B', 'P'})
	writer.Close()

	req := httptest.NewRequest(http.MethodPost, "/admin/settings/logo", body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = req
	UploadLogo(db)(c)
	if w.Code != http.StatusOK && w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, body = %s", w.Code, w.Body.String())
	}
}

func TestToggleQuickQuery_Success(t *testing.T) {
	db := setupDB(t)
	createBody := `{"command":"ping","name":"t","target":"1.1.1.1","active":true}`
	c, w := setupAdminContext(db, http.MethodPost, "/admin/quick-queries", createBody, 1)
	CreateQuickQuery(db)(c)
	var created struct {
		Data struct {
			ID int64 `json:"id"`
		} `json:"data"`
	}
	_ = json.Unmarshal(w.Body.Bytes(), &created)

	c, w = setupAdminContext(db, http.MethodPatch, fmt.Sprintf("/admin/quick-queries/%d/toggle", created.Data.ID), "", 1)
	c.Params = gin.Params{{Key: "id", Value: fmt.Sprint(created.Data.ID)}}
	ToggleQuickQuery(db)(c)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", w.Code, w.Body.String())
	}
}

func TestDeleteQuickQuery_ClosedDB(t *testing.T) {
	db := setupDB(t)
	db.Close()
	c, w := setupAdminContext(db, http.MethodDelete, "/admin/quick-queries/1", "", 1)
	c.Params = gin.Params{{Key: "id", Value: "1"}}
	DeleteQuickQuery(db)(c)
	if w.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d", w.Code)
	}
}

func TestUpdateCommunityRule_ClosedDB(t *testing.T) {
	db := setupDB(t)
	db.Close()
	body := `{"community":"1:1","severity":"info","message_i18n":"x","scope":"global","active":true}`
	c, w := setupAdminContext(db, http.MethodPut, "/admin/community-rules/1", body, 1)
	c.Params = gin.Params{{Key: "id", Value: "1"}}
	UpdateCommunityRule(db)(c)
	if w.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d", w.Code)
	}
}

func TestCreateCommunityRule_ClosedDB(t *testing.T) {
	db := setupDB(t)
	db.Close()
	body := `{"community":"1:1","severity":"info","message_i18n":"x","scope":"global","active":true}`
	c, w := setupAdminContext(db, http.MethodPost, "/admin/community-rules", body, 1)
	CreateCommunityRule(db)(c)
	if w.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d", w.Code)
	}
}

func TestCreateNode_ClosedDB(t *testing.T) {
	db := setupDB(t)
	db.Close()
	body := `{"name":"x","type":"standalone","enabled_cmds":["ping"]}`
	c, w := setupAdminContext(db, http.MethodPost, "/admin/nodes", body, 1)
	CreateNode(db, "")(c)
	if w.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d", w.Code)
	}
}

func TestSyncBGPNeighbor_ErrorLogged(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	mgr := bgp.NewSessionManager(config.BGPConfig{LocalAS: 65000, RouterID: "10.0.0.1", ListenPort: freeListenPort(t)})
	_ = mgr.Start(ctx)
	defer mgr.Stop()

	if err := syncBGPNeighbor(mgr, "test", func() error { return fmt.Errorf("sync fail") }); err == nil {
		t.Fatal("expected error")
	}
}

func TestRemoveBGPNeighborsForNode_Error(t *testing.T) {
	db := setupDB(t)
	db.Close()
	if err := removeBGPNeighborsForNode(context.Background(), db, bgp.NewSessionManager(config.BGPConfig{LocalAS: 65000}), 1); err == nil {
		t.Fatal("expected error")
	}
}

func TestWriteSSE_NilFlusher(t *testing.T) {
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/", nil)
	writeSSE(c, nil, "evt", gin.H{"x": 1})
}

func TestLogin_JWTGenerationFailure(t *testing.T) {
	db := setupDB(t)
	cfg := testConfig()
	seedAdminPassword(t, db, "password12345")
	prev := generateJWTForLogin
	generateJWTForLogin = func(int64, string, string) (string, error) {
		return "", errors.New("jwt fail")
	}
	t.Cleanup(func() { generateJWTForLogin = prev })

	body := fmt.Sprintf(`{"email":%q,"password":"password12345"}`, store.DefaultAdminEmail)
	c, w := setupContext(db, http.MethodPost, "/auth/login", body)
	Login(db, cfg)(c)
	if w.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d", w.Code)
	}
}

func TestUpdateAccount_HashPasswordFailure(t *testing.T) {
	db := setupDB(t)
	adminID := seedAdminPassword(t, db, "password12345")
	prev := hashPasswordFn
	hashPasswordFn = func(string) (string, error) {
		return "", errors.New("hash fail")
	}
	t.Cleanup(func() { hashPasswordFn = prev })

	body := `{"email":"admin@hopstat.local","current_password":"password12345","new_password":"newpassword12"}`
	c, w := setupAdminContext(db, http.MethodPut, "/admin/account", body, adminID)
	UpdateAccount(db)(c)
	if w.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d", w.Code)
	}
}

func TestSetDefaultNode_SetDefaultError(t *testing.T) {
	db := setupDB(t)
	nodeRepo := repo.NewNodeRepo(db, "")
	n, _ := nodeRepo.Create(context.Background(), &domain.Node{
		Name: "n", Type: domain.NodeTypeStandalone, Active: true,
		EnabledCmds: domain.DefaultEnabledCmds(),
	})
	prev := setNodeDefaultFn
	setNodeDefaultFn = func(context.Context, domain.NodeRepository, int64) error {
		return errors.New("set default fail")
	}
	t.Cleanup(func() { setNodeDefaultFn = prev })

	c, w := setupAdminContext(db, http.MethodPost, fmt.Sprintf("/admin/nodes/%d/default", n.ID), "", 1)
	c.Params = gin.Params{{Key: "id", Value: fmt.Sprint(n.ID)}}
	SetDefaultNode(db, "")(c)
	if w.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d", w.Code)
	}
}

func TestSetDefaultNode_GetAfterSetError(t *testing.T) {
	db := setupDB(t)
	nodeRepo := repo.NewNodeRepo(db, "")
	n, _ := nodeRepo.Create(context.Background(), &domain.Node{
		Name: "n", Type: domain.NodeTypeStandalone, Active: true,
		EnabledCmds: domain.DefaultEnabledCmds(),
	})
	prev := getDefaultNodeAfterSet
	getDefaultNodeAfterSet = func(context.Context, domain.NodeRepository, int64) (*domain.Node, error) {
		return nil, errors.New("get fail")
	}
	t.Cleanup(func() { getDefaultNodeAfterSet = prev })

	c, w := setupAdminContext(db, http.MethodPost, fmt.Sprintf("/admin/nodes/%d/default", n.ID), "", 1)
	c.Params = gin.Params{{Key: "id", Value: fmt.Sprint(n.ID)}}
	SetDefaultNode(db, "")(c)
	if w.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d", w.Code)
	}
}

func TestUpdateApply_AcceptedPath(t *testing.T) {
	prevAsync := updateApplyAsync
	prevDelay := updateApplyDelay
	prevBlocked := updateApplyBlocked
	updateApplyAsync = func(fn func()) { fn() }
	updateApplyDelay = func() {}
	updateApplyBlocked = func(*updater.Status) (bool, string) { return false, "" }
	t.Cleanup(func() {
		updateApplyAsync = prevAsync
		updateApplyDelay = prevDelay
		updateApplyBlocked = prevBlocked
	})

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"tag_name": "v9.9.9",
			"html_url": "https://example.com/release",
		})
	}))
	defer srv.Close()

	upd := updater.New("HopStat/HopStat", "v1.0.0", true)
	upd.SetReleaseAPIURL(srv.URL)
	c, w := setupAdminContext(nil, http.MethodPost, "/admin/update/apply", "", 1)
	UpdateApply(upd)(c)
	if w.Code != http.StatusAccepted {
		t.Fatalf("status = %d, body = %s", w.Code, w.Body.String())
	}
}

func TestUpdateApply_BlockedDefaultMessage(t *testing.T) {
	prev := updateApplyBlocked
	updateApplyBlocked = func(status *updater.Status) (bool, string) {
		status.SelfUpdateEnabled = false
		status.SelfUpdateReason = ""
		return true, ""
	}
	t.Cleanup(func() { updateApplyBlocked = prev })

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]interface{}{"tag_name": "v9.9.9"})
	}))
	defer srv.Close()

	upd := updater.New("HopStat/HopStat", "v1.0.0", true)
	upd.SetReleaseAPIURL(srv.URL)
	c, w := setupAdminContext(nil, http.MethodPost, "/admin/update/apply", "", 1)
	UpdateApply(upd)(c)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, body = %s", w.Code, w.Body.String())
	}
}

func TestUpdateBGPNeighbor_Errors(t *testing.T) {
	db := setupDB(t)
	bgpCfg := config.BGPConfig{LocalAS: 65000}
	body := `{"node_id":1,"remote_as":174,"peering_ip":"10.0.0.1","neighbor_ip":"10.0.0.2"}`

	c, w := setupAdminContext(db, http.MethodPut, "/admin/bgp-neighbors/x", body, 1)
	c.Params = gin.Params{{Key: "id", Value: "x"}}
	UpdateBGPNeighbor(db, nil, bgpCfg)(c)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("invalid id status = %d", w.Code)
	}

	c, w = setupAdminContext(db, http.MethodPut, "/admin/bgp-neighbors/1", `{`, 1)
	c.Params = gin.Params{{Key: "id", Value: "1"}}
	UpdateBGPNeighbor(db, nil, bgpCfg)(c)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("json status = %d", w.Code)
	}

	db.Close()
	c, w = setupAdminContext(db, http.MethodPut, "/admin/bgp-neighbors/1", body, 1)
	c.Params = gin.Params{{Key: "id", Value: "1"}}
	UpdateBGPNeighbor(db, nil, bgpCfg)(c)
	if w.Code != http.StatusInternalServerError {
		t.Fatalf("closed db status = %d", w.Code)
	}
}

func TestDeleteBGPNeighbor_WithManager(t *testing.T) {
	db := setupDB(t)
	nodeRepo := repo.NewNodeRepo(db, "")
	created, _ := nodeRepo.Create(context.Background(), &domain.Node{
		Name: "n", Type: domain.NodeTypeStandalone, Active: true,
		EnabledCmds: domain.DefaultEnabledCmds(),
	})
	bgpRepo := repo.NewBGPNeighborRepo(db)
	neighbor, _ := bgpRepo.Create(context.Background(), &domain.BGPNeighbor{
		NodeID: created.ID, LocalAS: 65000, RemoteAS: 174,
		PeeringIP: "10.0.0.1", NeighborIP: "10.0.0.2",
	})

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	mgr := bgp.NewSessionManager(config.BGPConfig{LocalAS: 65000, RouterID: "10.0.0.1", ListenPort: freeListenPort(t)})
	_ = mgr.Start(ctx)
	defer mgr.Stop()

	c, w := setupAdminContext(db, http.MethodDelete, fmt.Sprintf("/admin/bgp-neighbors/%d", neighbor.ID), "", 1)
	c.Params = gin.Params{{Key: "id", Value: fmt.Sprint(neighbor.ID)}}
	DeleteBGPNeighbor(db, mgr)(c)
	if w.Code != http.StatusOK && w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, body = %s", w.Code, w.Body.String())
	}
}

func TestAgentTracerouteStream_SuccessOnly(t *testing.T) {
	db := setupDB(t)
	withStubAgentDriver(t, stubAgentDriver{})
	r := setupAgentRouter(t, db, &domain.Node{
		Name: "local", Type: domain.NodeTypeStandalone, AgentToken: "node-secret",
		EnabledCmds: domain.DefaultEnabledCmds(), Active: true,
	})
	req := httptest.NewRequest(http.MethodPost, "/agent/v1/traceroute/stream", strings.NewReader(`{"target":"1.1.1.1","max_hops":1}`))
	req.Header.Set("Authorization", "Bearer node-secret")
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d", w.Code)
	}
	if !strings.Contains(w.Body.String(), "event: result") {
		t.Fatalf("body = %q", w.Body.String())
	}
}

func TestAgentTraceroute_DefaultMaxHops(t *testing.T) {
	db := setupDB(t)
	withStubAgentDriver(t, stubAgentDriver{})
	r := setupAgentRouter(t, db, &domain.Node{
		Name: "local", Type: domain.NodeTypeStandalone, AgentToken: "node-secret",
		EnabledCmds: domain.DefaultEnabledCmds(), Active: true,
	})
	req := httptest.NewRequest(http.MethodPost, "/agent/v1/traceroute", strings.NewReader(`{"target":"1.1.1.1"}`))
	req.Header.Set("Authorization", "Bearer node-secret")
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d", w.Code)
	}
}

func TestUpdateQuickQuery_ClosedDB(t *testing.T) {
	db := setupDB(t)
	db.Close()
	body := `{"command":"ping","name":"x","target":"1.1.1.1","active":true}`
	c, w := setupAdminContext(db, http.MethodPut, "/admin/quick-queries/1", body, 1)
	c.Params = gin.Params{{Key: "id", Value: "1"}}
	UpdateQuickQuery(db)(c)
	if w.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d", w.Code)
	}
}

func TestCreateQuickQuery_ClosedDB(t *testing.T) {
	db := setupDB(t)
	db.Close()
	body := `{"command":"ping","name":"x","target":"1.1.1.1","active":true}`
	c, w := setupAdminContext(db, http.MethodPost, "/admin/quick-queries", body, 1)
	CreateQuickQuery(db)(c)
	if w.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d", w.Code)
	}
}

func TestToggleQuickQuery_ClosedDB(t *testing.T) {
	db := setupDB(t)
	db.Close()
	c, w := setupAdminContext(db, http.MethodPatch, "/admin/quick-queries/1/toggle", "", 1)
	c.Params = gin.Params{{Key: "id", Value: "1"}}
	ToggleQuickQuery(db)(c)
	if w.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d", w.Code)
	}
}

func TestUpdateSettings_ClosedDB(t *testing.T) {
	db := setupDB(t)
	db.Close()
	c, w := setupAdminContext(db, http.MethodPut, "/admin/settings", `{"site_name":"x"}`, 1)
	UpdateSettings(db)(c)
	if w.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d", w.Code)
	}
}

func TestExportAudit_ClosedDB(t *testing.T) {
	db := setupDB(t)
	db.Close()
	c, w := setupAdminContext(db, http.MethodGet, "/admin/audit/export", "", 1)
	ExportAudit(db)(c)
	if w.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d", w.Code)
	}
}

func TestListAudit_CountTodayError(t *testing.T) {
	db := setupDB(t)
	auditRepo := repo.NewAuditRepo(db)
	_ = auditRepo.Log(context.Background(), &domain.AuditEntry{Command: "ping", Success: true})
	db.Close()
	c, w := setupAdminContext(db, http.MethodGet, "/admin/audit", "", 1)
	ListAudit(db)(c)
	if w.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d", w.Code)
	}
}

func TestUpdateAccount_GetByEmailError(t *testing.T) {
	db := setupDB(t)
	adminID := seedAdminPassword(t, db, "password12345")
	db.Close()
	body := `{"email":"new@hopstat.local","current_password":"password12345"}`
	c, w := setupAdminContext(db, http.MethodPut, "/admin/account", body, adminID)
	UpdateAccount(db)(c)
	if w.Code != http.StatusInternalServerError && w.Code != http.StatusNotFound {
		t.Fatalf("status = %d", w.Code)
	}
}

func TestResolveUploadsDir_Default(t *testing.T) {
	got := ResolveUploadsDir("")
	if !strings.Contains(got, "uploads") {
		t.Fatalf("got %q", got)
	}
}

func TestValidQuickQueryTarget_IPv6(t *testing.T) {
	if !validQuickQueryTarget("ping", "2001:db8::1") {
		t.Fatal("expected valid ipv6")
	}
}

func TestAsPathEnrichmentReady_ZeroASN(t *testing.T) {
	if !asPathEnrichmentReady(&domain.QueryResult{ASPath: []uint32{0, 15169}, ASPathEnriched: []domain.ASInfo{{ASN: 15169}}}) {
		t.Fatal("expected ready when only zero asn missing")
	}
}

func TestUniqueASNsInPath_Zero(t *testing.T) {
	if n := uniqueASNsInPath([]uint32{0, 0}); n != 0 {
		t.Fatalf("got %d", n)
	}
}

func TestUploadLogo_SettingsPersistFailure(t *testing.T) {
	db := setupDB(t)
	uploadDir := t.TempDir()
	SetLogoUploadsDir(uploadDir)

	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	part, _ := writer.CreateFormFile("logo", "logo.png")
	_, _ = part.Write([]byte{0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A})
	writer.Close()

	req := httptest.NewRequest(http.MethodPost, "/admin/settings/logo", body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = req

	db.Close()
	UploadLogo(db)(c)
	if w.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, body = %s", w.Code, w.Body.String())
	}
}

func TestUploadLogo_SVGRejectedPatterns(t *testing.T) {
	db := setupDB(t)
	uploadDir := t.TempDir()
	SetLogoUploadsDir(uploadDir)

	cases := []string{
		`<svg xmlns="http://www.w3.org/2000/svg"><rect onload="alert(1)"/></svg>`,
		`<svg xmlns="http://www.w3.org/2000/svg"><foreignObject/></svg>`,
		`<svg xmlns="http://www.w3.org/2000/svg"><a href="https://evil.com"/></svg>`,
	}
	for _, svg := range cases {
		body := &bytes.Buffer{}
		writer := multipart.NewWriter(body)
		part, _ := writer.CreateFormFile("logo", "logo.svg")
		_, _ = io.WriteString(part, svg)
		writer.Close()

		req := httptest.NewRequest(http.MethodPost, "/admin/settings/logo", body)
		req.Header.Set("Content-Type", writer.FormDataContentType())
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request = req
		UploadLogo(db)(c)
		if w.Code != http.StatusBadRequest {
			t.Fatalf("svg %q status = %d", svg, w.Code)
		}
	}
}

func TestAgentPingStream_InvalidJSON(t *testing.T) {
	db := setupDB(t)
	r := setupAgentRouter(t, db, &domain.Node{
		Name: "local", Type: domain.NodeTypeStandalone, AgentToken: "node-secret",
		EnabledCmds: domain.DefaultEnabledCmds(), Active: true,
	})
	req := httptest.NewRequest(http.MethodPost, "/agent/v1/ping/stream", strings.NewReader(`{`))
	req.Header.Set("Authorization", "Bearer node-secret")
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d", w.Code)
	}
}

func TestAgentPingStream_CreateDriverError(t *testing.T) {
	withStubAgentDriver(t, stubAgentDriver{createErr: errors.New("no driver")})
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/agent/v1/ping/stream", strings.NewReader(`{"target":"1.1.1.1"}`))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Set(middleware.AgentNodeKey, &domain.Node{Name: "n", EnabledCmds: domain.DefaultEnabledCmds()})
	agentPingStream(testConfig())(c)
	if w.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d", w.Code)
	}
}

func TestAgentBGPRoute_ManagerError(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	mgr := bgp.NewSessionManager(config.BGPConfig{LocalAS: 65000, RouterID: "10.0.0.1", ListenPort: freeListenPort(t)})
	_ = mgr.Start(ctx)
	defer mgr.Stop()

	withStubAgentDriver(t, stubAgentDriver{})
	gin.SetMode(gin.TestMode)
	r := gin.New()
	agent := r.Group("")
	agent.Use(func(c *gin.Context) {
		c.Set(middleware.AgentNodeKey, &domain.Node{ID: 999, Name: "local", EnabledCmds: domain.DefaultEnabledCmds()})
		c.Next()
	})
	MountAgentAPI(agent, testConfig(), mgr, geo.New("", ""))

	req := httptest.NewRequest(http.MethodPost, "/agent/v1/bgp/route", strings.NewReader(`{"prefix":"8.8.8.8"}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusInternalServerError && w.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", w.Code, w.Body.String())
	}
}

func TestUpdateQuickQuery_WithNode(t *testing.T) {
	db := setupDB(t)
	nodeRepo := repo.NewNodeRepo(db, "")
	node, _ := nodeRepo.Create(context.Background(), &domain.Node{
		Name: "n", Type: domain.NodeTypeStandalone, Active: true,
		EnabledCmds: domain.DefaultEnabledCmds(),
	})

	createBody := `{"command":"ping","name":"q","target":"1.1.1.1","active":true}`
	c, w := setupAdminContext(db, http.MethodPost, "/admin/quick-queries", createBody, 1)
	CreateQuickQuery(db)(c)
	var created struct {
		Data struct {
			ID int64 `json:"id"`
		} `json:"data"`
	}
	_ = json.Unmarshal(w.Body.Bytes(), &created)

	updateBody := fmt.Sprintf(`{"command":"ping","name":"q2","target":"1.1.1.1","node_id":%d,"active":false}`, node.ID)
	c, w = setupAdminContext(db, http.MethodPut, fmt.Sprintf("/admin/quick-queries/%d", created.Data.ID), updateBody, 1)
	c.Params = gin.Params{{Key: "id", Value: fmt.Sprint(created.Data.ID)}}
	UpdateQuickQuery(db)(c)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", w.Code, w.Body.String())
	}
}

func TestWaitForASPathEnrichment_Notify(t *testing.T) {
	queryID := "notify-aspath"
	queryStore.SetRunning(queryID)
	queryStore.MergePartial(queryID, &domain.QueryResult{
		ASPath:         []uint32{64512},
		ASPathEnriched: []domain.ASInfo{},
	})
	go func() {
		time.Sleep(20 * time.Millisecond)
		queryStore.MergePartial(queryID, &domain.QueryResult{
			ASPath:         []uint32{64512},
			ASPathEnriched: []domain.ASInfo{{ASN: 64512, OrgName: "X"}},
		})
	}()
	waitForASPathEnrichment(queryID, 500*time.Millisecond)
}

func TestAsSuffixForIP_ShortName(t *testing.T) {
	g := stubGeo{byIP: map[string]*domain.ASInfo{
		"8.8.4.4": {ASN: 15169, ShortName: "GOOG"},
	}}
	suffix := asSuffixForIP(context.Background(), g, "8.8.4.4")
	if !strings.Contains(suffix, "15169") {
		t.Fatalf("suffix = %q", suffix)
	}
}

func TestFriendlyConnError_UnknownMessage(t *testing.T) {
	got := friendlyConnError(errors.New("some other network error"))
	if got != "some other network error" {
		t.Fatalf("got %q", got)
	}
}

func TestCreateBGPNeighbor_DBError(t *testing.T) {
	db := setupDB(t)
	db.Close()
	body := `{"node_id":1,"remote_as":174,"peering_ip":"10.0.0.1","neighbor_ip":"10.0.0.2"}`
	c, w := setupAdminContext(db, http.MethodPost, "/admin/bgp-neighbors", body, 1)
	CreateBGPNeighbor(db, nil, config.BGPConfig{LocalAS: 65000})(c)
	if w.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d", w.Code)
	}
}

func TestBGPNeighborAction_Success(t *testing.T) {
	db := setupDB(t)
	nodeRepo := repo.NewNodeRepo(db, "")
	node, _ := nodeRepo.Create(context.Background(), &domain.Node{
		Name: "n", Type: domain.NodeTypeStandalone, Active: true,
		EnabledCmds: domain.DefaultEnabledCmds(),
	})
	bgpRepo := repo.NewBGPNeighborRepo(db)
	neighbor, _ := bgpRepo.Create(context.Background(), &domain.BGPNeighbor{
		NodeID: node.ID, LocalAS: 65000, RemoteAS: 174,
		PeeringIP: "10.0.0.1", NeighborIP: "10.0.0.2",
	})

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	mgr := bgp.NewSessionManager(config.BGPConfig{LocalAS: 65000, RouterID: "10.0.0.1", ListenPort: freeListenPort(t)})
	_ = mgr.Start(ctx)
	defer mgr.Stop()
	_ = mgr.AddNeighbor(neighbor)

	c, w := setupAdminContext(db, http.MethodPost, fmt.Sprintf("/admin/bgp-neighbors/%d/stop", neighbor.ID), "", 1)
	c.Params = gin.Params{{Key: "id", Value: fmt.Sprint(neighbor.ID)}}
	StopBGPNeighbor(mgr)(c)
	if w.Code != http.StatusOK && w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, body = %s", w.Code, w.Body.String())
	}
}

func TestRemoveBGPNeighborsForNode_RemoveError(t *testing.T) {
	db := setupDB(t)
	nodeRepo := repo.NewNodeRepo(db, "")
	node, _ := nodeRepo.Create(context.Background(), &domain.Node{
		Name: "n", Type: domain.NodeTypeStandalone, Active: true,
		EnabledCmds: domain.DefaultEnabledCmds(),
	})
	bgpRepo := repo.NewBGPNeighborRepo(db)
	neighbor, _ := bgpRepo.Create(context.Background(), &domain.BGPNeighbor{
		NodeID: node.ID, LocalAS: 65000, RemoteAS: 174,
		PeeringIP: "10.0.0.1", NeighborIP: "10.0.0.2",
	})

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	mgr := bgp.NewSessionManager(config.BGPConfig{LocalAS: 65000, RouterID: "10.0.0.1", ListenPort: freeListenPort(t)})
	_ = mgr.Start(ctx)
	defer mgr.Stop()
	_ = mgr.AddNeighbor(neighbor)
	_ = mgr.RemoveNeighbor(neighbor.ID)

	if err := removeBGPNeighborsForNode(context.Background(), db, mgr, node.ID); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestListNodes_WithBGPSession(t *testing.T) {
	db := setupDB(t)
	nodeRepo := repo.NewNodeRepo(db, "")
	created, _ := nodeRepo.Create(context.Background(), &domain.Node{
		Name: "n", Type: domain.NodeTypeStandalone, Active: true,
		EnabledCmds: domain.DefaultEnabledCmds(),
	})
	refreshTestSiteCache(t, db)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	mgr := bgp.NewSessionManager(config.BGPConfig{LocalAS: 65000, RouterID: "10.0.0.1", ListenPort: freeListenPort(t)})
	_ = mgr.Start(ctx)
	defer mgr.Stop()

	c, w := setupContext(db, http.MethodGet, "/nodes", "")
	ListNodes(db, "", mgr)(c)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d", w.Code)
	}
	_ = created
}

func TestDeleteBGPNeighbor_SuccessNilManager(t *testing.T) {
	db := setupDB(t)
	bgpRepo := repo.NewBGPNeighborRepo(db)
	nodeRepo := repo.NewNodeRepo(db, "")
	node, _ := nodeRepo.Create(context.Background(), &domain.Node{
		Name: "n", Type: domain.NodeTypeStandalone, Active: true,
		EnabledCmds: domain.DefaultEnabledCmds(),
	})
	neighbor, _ := bgpRepo.Create(context.Background(), &domain.BGPNeighbor{
		NodeID: node.ID, LocalAS: 65000, RemoteAS: 174,
		PeeringIP: "10.0.0.1", NeighborIP: "10.0.0.2",
	})

	c, w := setupAdminContext(db, http.MethodDelete, fmt.Sprintf("/admin/bgp-neighbors/%d", neighbor.ID), "", 1)
	c.Params = gin.Params{{Key: "id", Value: fmt.Sprint(neighbor.ID)}}
	DeleteBGPNeighbor(db, nil)(c)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", w.Code, w.Body.String())
	}
}

func TestUpdateAccount_GetByEmailCheckError(t *testing.T) {
	db := setupDB(t)
	adminID := seedAdminPassword(t, db, "password12345")
	body := `{"email":"unique@hopstat.local","current_password":"password12345"}`
	c, w := setupAdminContext(db, http.MethodPut, "/admin/account", body, adminID)
	UpdateAccount(db)(c)
	if w.Code != http.StatusOK {
		t.Fatalf("first update status = %d", w.Code)
	}
}

func TestUpdateAccount_UpdateCredentialsError(t *testing.T) {
	db := setupDB(t)
	adminID := seedAdminPassword(t, db, "password12345")
	body := `{"email":"brand-new@hopstat.local","current_password":"password12345"}`
	c, w := setupAdminContext(db, http.MethodPut, "/admin/account", body, adminID)
	UpdateAccount(db)(c)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", w.Code, w.Body.String())
	}
	db.Close()
	c, w = setupAdminContext(db, http.MethodPut, "/admin/account", body, adminID)
	UpdateAccount(db)(c)
	if w.Code != http.StatusNotFound && w.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d", w.Code)
	}
}

func TestAgentPing_StubSuccess(t *testing.T) {
	db := setupDB(t)
	withStubAgentDriver(t, stubAgentDriver{})
	r := setupAgentRouter(t, db, &domain.Node{
		Name: "local", Type: domain.NodeTypeStandalone, AgentToken: "node-secret",
		EnabledCmds: domain.DefaultEnabledCmds(), Active: true,
	})
	req := httptest.NewRequest(http.MethodPost, "/agent/v1/ping", strings.NewReader(`{"target":"1.1.1.1","count":1}`))
	req.Header.Set("Authorization", "Bearer node-secret")
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", w.Code, w.Body.String())
	}
}

func TestAgentTraceroute_NoNodeAndClamp(t *testing.T) {
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/agent/v1/traceroute", strings.NewReader(`{"target":"1.1.1.1","max_hops":100}`))
	c.Request.Header.Set("Content-Type", "application/json")
	agentTraceroute(testConfig())(c)
	if w.Code != http.StatusInternalServerError {
		t.Fatalf("no node status = %d", w.Code)
	}

	db := setupDB(t)
	withStubAgentDriver(t, stubAgentDriver{})
	r := setupAgentRouter(t, db, &domain.Node{
		Name: "local", Type: domain.NodeTypeStandalone, AgentToken: "node-secret",
		EnabledCmds: domain.DefaultEnabledCmds(), Active: true,
	})
	req := httptest.NewRequest(http.MethodPost, "/agent/v1/traceroute", strings.NewReader(`{"target":"1.1.1.1","max_hops":100}`))
	req.Header.Set("Authorization", "Bearer node-secret")
	req.Header.Set("Content-Type", "application/json")
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK && w.Code != http.StatusInternalServerError {
		t.Fatalf("clamp status = %d", w.Code)
	}
}

func TestAgentBGPRoute_NoNode(t *testing.T) {
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/agent/v1/bgp/route", strings.NewReader(`{"prefix":"8.8.8.8"}`))
	c.Request.Header.Set("Content-Type", "application/json")
	agentBGPRoute(testConfig(), nil, geo.New("", ""))(c)
	if w.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d", w.Code)
	}
}

func TestAgentBGPRoute_StubDriverCreateError(t *testing.T) {
	withStubAgentDriver(t, stubAgentDriver{createErr: errors.New("no driver")})
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/agent/v1/bgp/route", strings.NewReader(`{"prefix":"8.8.8.8"}`))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Set(middleware.AgentNodeKey, &domain.Node{Name: "n", EnabledCmds: domain.DefaultEnabledCmds()})
	agentBGPRoute(testConfig(), nil, geo.New("", ""))(c)
	if w.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d", w.Code)
	}
}

func TestAgentPingStream_NoNodeOnLineAndClamp(t *testing.T) {
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/agent/v1/ping/stream", strings.NewReader(`{"target":"1.1.1.1"}`))
	c.Request.Header.Set("Content-Type", "application/json")
	agentPingStream(testConfig())(c)
	if w.Code != http.StatusInternalServerError {
		t.Fatalf("no node status = %d", w.Code)
	}

	db := setupDB(t)
	withStubAgentDriver(t, stubAgentDriver{onLine: func(string) {}})
	r := setupAgentRouter(t, db, &domain.Node{
		Name: "local", Type: domain.NodeTypeStandalone, AgentToken: "node-secret",
		EnabledCmds: domain.DefaultEnabledCmds(), Active: true,
	})
	req := httptest.NewRequest(http.MethodPost, "/agent/v1/ping/stream", strings.NewReader(`{"target":"1.1.1.1","count":100}`))
	req.Header.Set("Authorization", "Bearer node-secret")
	req.Header.Set("Content-Type", "application/json")
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d", w.Code)
	}
	if !strings.Contains(w.Body.String(), "event: output") {
		t.Fatalf("body = %q", w.Body.String())
	}
}

func TestAgentTracerouteStream_NoNodeOnLine(t *testing.T) {
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/agent/v1/traceroute/stream", strings.NewReader(`{"target":"1.1.1.1"}`))
	c.Request.Header.Set("Content-Type", "application/json")
	agentTracerouteStream(testConfig())(c)
	if w.Code != http.StatusInternalServerError {
		t.Fatalf("no node status = %d", w.Code)
	}

	db := setupDB(t)
	withStubAgentDriver(t, stubAgentDriver{onLine: func(string) {}})
	r := setupAgentRouter(t, db, &domain.Node{
		Name: "local", Type: domain.NodeTypeStandalone, AgentToken: "node-secret",
		EnabledCmds: domain.DefaultEnabledCmds(), Active: true,
	})
	req := httptest.NewRequest(http.MethodPost, "/agent/v1/traceroute/stream", strings.NewReader(`{"target":"1.1.1.1","max_hops":100}`))
	req.Header.Set("Authorization", "Bearer node-secret")
	req.Header.Set("Content-Type", "application/json")
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d", w.Code)
	}
}

func TestListNodes_SkipsNilCachedNode(t *testing.T) {
	prev := activeNodesLister
	activeNodesLister = func() []*domain.Node {
		return []*domain.Node{nil, {ID: 1, Name: "ok", EnabledCmds: domain.DefaultEnabledCmds()}}
	}
	t.Cleanup(func() { activeNodesLister = prev })

	c, w := setupContext(nil, http.MethodGet, "/nodes", "")
	ListNodes(nil, "", nil)(c)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d", w.Code)
	}
}

func TestLogin_WrongPasswordAndLastLoginError(t *testing.T) {
	db := setupDB(t)
	cfg := testConfig()
	seedAdminPassword(t, db, "password12345")

	body := fmt.Sprintf(`{"email":%q,"password":"wrong"}`, store.DefaultAdminEmail)
	c, w := setupContext(db, http.MethodPost, "/auth/login", body)
	Login(db, cfg)(c)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("wrong password status = %d", w.Code)
	}

	prev := loginUpdateLastLogin
	done := make(chan struct{})
	loginUpdateLastLogin = func(context.Context, domain.UserRepository, int64) error {
		close(done)
		return errors.New("last login fail")
	}
	t.Cleanup(func() {
		<-done
		loginUpdateLastLogin = prev
	})

	body = fmt.Sprintf(`{"email":%q,"password":"password12345"}`, store.DefaultAdminEmail)
	c, w = setupContext(db, http.MethodPost, "/auth/login", body)
	Login(db, cfg)(c)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d", w.Code)
	}
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("login async update timeout")
	}
}

func TestUpdateAccount_NoChanges(t *testing.T) {
	db := setupDB(t)
	adminID := seedAdminPassword(t, db, "password12345")
	body := fmt.Sprintf(`{"email":%q,"current_password":"password12345"}`, store.DefaultAdminEmail)
	c, w := setupAdminContext(db, http.MethodPut, "/admin/account", body, adminID)
	UpdateAccount(db)(c)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, body = %s", w.Code, w.Body.String())
	}
}

func TestUpdateAccount_GetByEmailInternalError(t *testing.T) {
	db := setupDB(t)
	adminID := seedAdminPassword(t, db, "password12345")
	if _, err := db.Exec(`DROP TABLE users`); err != nil {
		t.Fatal(err)
	}
	body := `{"email":"new@hopstat.local","current_password":"password12345"}`
	c, w := setupAdminContext(db, http.MethodPut, "/admin/account", body, adminID)
	UpdateAccount(db)(c)
	if w.Code != http.StatusInternalServerError && w.Code != http.StatusNotFound {
		t.Fatalf("status = %d", w.Code)
	}
}

func TestCreateNode_DefaultEnabledCmds(t *testing.T) {
	db := setupDB(t)
	body := `{"name":"auto-cmds","type":"standalone","enabled_cmds":[]}`
	c, w := setupAdminContext(db, http.MethodPost, "/admin/nodes", body, 1)
	CreateNode(db, "")(c)
	if w.Code != http.StatusCreated {
		t.Fatalf("status = %d, body = %s", w.Code, w.Body.String())
	}
}

func TestListBGPNeighbors_ClosedDB(t *testing.T) {
	db := setupDB(t)
	db.Close()
	c, w := setupAdminContext(db, http.MethodGet, "/admin/bgp-neighbors", "", 1)
	ListBGPNeighbors(db, nil, config.BGPConfig{LocalAS: 65000})(c)
	if w.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d", w.Code)
	}
}

func TestCreateBGPNeighbor_AddNeighborFailure(t *testing.T) {
	db := setupDB(t)
	nodeRepo := repo.NewNodeRepo(db, "")
	node, _ := nodeRepo.Create(context.Background(), &domain.Node{
		Name: "n", Type: domain.NodeTypeStandalone, Active: true,
		EnabledCmds: domain.DefaultEnabledCmds(),
	})
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	mgr := bgp.NewSessionManager(config.BGPConfig{LocalAS: 65000, RouterID: "10.0.0.1", ListenPort: freeListenPort(t)})
	_ = mgr.Start(ctx)
	defer mgr.Stop()

	body := fmt.Sprintf(`{"node_id":%d,"remote_as":174,"peering_ip":"10.0.0.1","neighbor_ip":"10.0.0.2"}`, node.ID)
	c, w := setupAdminContext(db, http.MethodPost, "/admin/bgp-neighbors", body, 1)
	CreateBGPNeighbor(db, mgr, config.BGPConfig{LocalAS: 65000})(c)
	if w.Code != http.StatusCreated && w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, body = %s", w.Code, w.Body.String())
	}
}

func TestDeleteBGPNeighbor_SyncAndDBErrors(t *testing.T) {
	db := setupDB(t)
	bgpRepo := repo.NewBGPNeighborRepo(db)
	nodeRepo := repo.NewNodeRepo(db, "")
	node, _ := nodeRepo.Create(context.Background(), &domain.Node{
		Name: "n", Type: domain.NodeTypeStandalone, Active: true,
		EnabledCmds: domain.DefaultEnabledCmds(),
	})
	neighbor, _ := bgpRepo.Create(context.Background(), &domain.BGPNeighbor{
		NodeID: node.ID, LocalAS: 65000, RemoteAS: 174,
		PeeringIP: "10.0.0.1", NeighborIP: "10.0.0.2",
	})

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	mgr := bgp.NewSessionManager(config.BGPConfig{LocalAS: 65000, RouterID: "10.0.0.1", ListenPort: freeListenPort(t)})
	_ = mgr.Start(ctx)
	defer mgr.Stop()
	_ = mgr.AddNeighbor(neighbor)

	c, w := setupAdminContext(db, http.MethodDelete, fmt.Sprintf("/admin/bgp-neighbors/%d", neighbor.ID), "", 1)
	c.Params = gin.Params{{Key: "id", Value: fmt.Sprint(neighbor.ID)}}
	DeleteBGPNeighbor(db, mgr)(c)
	if w.Code != http.StatusOK && w.Code != http.StatusBadRequest {
		t.Fatalf("delete with mgr status = %d", w.Code)
	}

	db.Close()
	c, w = setupAdminContext(db, http.MethodDelete, "/admin/bgp-neighbors/1", "", 1)
	c.Params = gin.Params{{Key: "id", Value: "1"}}
	DeleteBGPNeighbor(db, nil)(c)
	if w.Code != http.StatusInternalServerError {
		t.Fatalf("closed db status = %d", w.Code)
	}
}

func TestBGPNeighborAction_Failure(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	mgr := bgp.NewSessionManager(config.BGPConfig{LocalAS: 65000, RouterID: "10.0.0.1", ListenPort: freeListenPort(t)})
	_ = mgr.Start(ctx)
	defer mgr.Stop()

	c, w := setupAdminContext(nil, http.MethodPost, "/admin/bgp-neighbors/99999/restart", "", 1)
	c.Params = gin.Params{{Key: "id", Value: "99999"}}
	RestartBGPNeighbor(mgr)(c)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, body = %s", w.Code, w.Body.String())
	}
}

func TestRemoveBGPNeighborsForNode_RemoveLoopError(t *testing.T) {
	db := setupDB(t)
	bgpRepo := repo.NewBGPNeighborRepo(db)
	nodeRepo := repo.NewNodeRepo(db, "")
	node, _ := nodeRepo.Create(context.Background(), &domain.Node{
		Name: "n", Type: domain.NodeTypeStandalone, Active: true,
		EnabledCmds: domain.DefaultEnabledCmds(),
	})
	_, _ = bgpRepo.Create(context.Background(), &domain.BGPNeighbor{
		NodeID: node.ID, LocalAS: 65000, RemoteAS: 174,
		PeeringIP: "10.0.0.1", NeighborIP: "10.0.0.2",
	})

	prev := bgpMgrRemoveNeighbor
	bgpMgrRemoveNeighbor = func(*bgp.SessionManager, int64) error {
		return errors.New("remove failed")
	}
	t.Cleanup(func() { bgpMgrRemoveNeighbor = prev })

	if err := removeBGPNeighborsForNode(context.Background(), db, bgp.NewSessionManager(config.BGPConfig{LocalAS: 65000}), node.ID); err == nil {
		t.Fatal("expected remove error")
	}
}

func TestResolveUploadsDir_AbsError(t *testing.T) {
	prev := uploadsDirAbs
	uploadsDirAbs = func(string) (string, error) {
		return "", errors.New("abs fail")
	}
	t.Cleanup(func() { uploadsDirAbs = prev })
	got := ResolveUploadsDir("/tmp/lg.db")
	if !strings.Contains(got, "data") {
		t.Fatalf("got %q", got)
	}
}

func TestLogoPathWithCacheBuster_InvalidLogoPath(t *testing.T) {
	if got := logoPathWithCacheBuster("/not-logo.png"); got != "/not-logo.png" {
		t.Fatalf("got %q", got)
	}
}

func TestValidQuickQueryTarget_EdgeCases(t *testing.T) {
	if validQuickQueryTarget("ping", "  ") {
		t.Fatal("whitespace target should fail")
	}
	if validQuickQueryTarget("bgp_route", "not-valid") {
		t.Fatal("invalid bgp target should fail")
	}
	if !validQuickQueryTarget("bgp_route", "10.0.0.0/8") {
		t.Fatal("valid cidr should pass")
	}
}

func TestCreateQuickQuery_ValidationErrors(t *testing.T) {
	db := setupDB(t)
	c, w := setupAdminContext(db, http.MethodPost, "/admin/quick-queries", `{"command":"bad","name":"n","target":"1.1.1.1"}`, 1)
	CreateQuickQuery(db)(c)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("bad command status = %d", w.Code)
	}

	c, w = setupAdminContext(db, http.MethodPost, "/admin/quick-queries", `{"command":"ping","name":"  ","target":"1.1.1.1"}`, 1)
	CreateQuickQuery(db)(c)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("empty name status = %d", w.Code)
	}

	c, w = setupAdminContext(db, http.MethodPost, "/admin/quick-queries", `{"command":"ping","name":"n","target":"bad-target"}`, 1)
	CreateQuickQuery(db)(c)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("bad target status = %d", w.Code)
	}
}

func TestUpdateQuickQuery_ValidationErrors(t *testing.T) {
	db := setupDB(t)
	createBody := `{"command":"ping","name":"t","target":"1.1.1.1","active":true}`
	c, w := setupAdminContext(db, http.MethodPost, "/admin/quick-queries", createBody, 1)
	CreateQuickQuery(db)(c)
	var created struct {
		Data struct {
			ID int64 `json:"id"`
		} `json:"data"`
	}
	_ = json.Unmarshal(w.Body.Bytes(), &created)

	c, w = setupAdminContext(db, http.MethodPut, fmt.Sprintf("/admin/quick-queries/%d", created.Data.ID), `{"command":"bad","name":"n","target":"1.1.1.1","active":true}`, 1)
	c.Params = gin.Params{{Key: "id", Value: fmt.Sprint(created.Data.ID)}}
	UpdateQuickQuery(db)(c)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("bad command status = %d", w.Code)
	}
}

func TestToggleQuickQuery_GetAfterToggleMissing(t *testing.T) {
	db := setupDB(t)
	db.Close()
	c, w := setupAdminContext(db, http.MethodPatch, "/admin/quick-queries/1/toggle", "", 1)
	c.Params = gin.Params{{Key: "id", Value: "1"}}
	ToggleQuickQuery(db)(c)
	if w.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d", w.Code)
	}
}

func TestUpdateApply_DefaultBlockedAndAsync(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"tag_name": "v9.9.9",
			"html_url": "https://example.com/release",
		})
	}))
	defer srv.Close()

	upd := updater.New("HopStat/HopStat", "v1.0.0", true)
	upd.SetReleaseAPIURL(srv.URL)
	c, w := setupAdminContext(nil, http.MethodPost, "/admin/update/apply", "", 1)
	UpdateApply(upd)(c)
	if w.Code != http.StatusAccepted && w.Code != http.StatusBadRequest {
		t.Fatalf("default blocked status = %d, body = %s", w.Code, w.Body.String())
	}

	prevDelay := updateApplyDelay
	updateApplyDelay = func() {}
	t.Cleanup(func() { updateApplyDelay = prevDelay })

	done := make(chan struct{}, 1)
	prevAsync := updateApplyAsync
	updateApplyAsync = func(fn func()) {
		fn()
		done <- struct{}{}
	}
	t.Cleanup(func() { updateApplyAsync = prevAsync })

	prevBlocked := updateApplyBlocked
	updateApplyBlocked = func(*updater.Status) (bool, string) { return false, "" }
	t.Cleanup(func() { updateApplyBlocked = prevBlocked })

	c, w = setupAdminContext(nil, http.MethodPost, "/admin/update/apply", "", 1)
	UpdateApply(upd)(c)
	if w.Code != http.StatusAccepted {
		t.Fatalf("accepted status = %d", w.Code)
	}
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("async apply did not run")
	}
}

func TestStreamResult_NotifyWake(t *testing.T) {
	db := setupDB(t)
	queryID := "notify-wake"
	queryStore.SetRunning(queryID)
	go func() {
		time.Sleep(30 * time.Millisecond)
		queryStore.AppendLine(queryID, "late line")
		queryStore.MarkOutputComplete(queryID)
		queryStore.Set(queryID, &domain.QueryResult{ID: queryID, Status: domain.StatusDone})
	}()

	c, w := setupContext(db, http.MethodGet, "/query/"+queryID+"/stream", "")
	c.Params = gin.Params{{Key: "id", Value: queryID}}
	StreamResult(db)(c)
	if !strings.Contains(w.Body.String(), "late line") {
		t.Fatalf("body = %q", w.Body.String())
	}
}

func TestUpdateSettings_RefreshWarning(t *testing.T) {
	db := setupDB(t)
	db.Close()
	c, w := setupAdminContext(db, http.MethodPut, "/admin/settings", `{"site_name":"x"}`, 1)
	UpdateSettings(db)(c)
	if w.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d", w.Code)
	}
}

func TestCreateCommunityRule_RefreshWarning(t *testing.T) {
	db := setupDB(t)
	body := `{"community":"1:1","severity":"info","message_i18n":"x","scope":"global","active":true}`
	c, w := setupAdminContext(db, http.MethodPost, "/admin/community-rules", body, 1)
	CreateCommunityRule(db)(c)
	if w.Code != http.StatusCreated {
		t.Fatalf("status = %d", w.Code)
	}
}

func TestUpdateCommunityRule_InvalidSeverity(t *testing.T) {
	db := setupDB(t)
	createBody := `{"community":"1:1","severity":"info","message_i18n":"x","scope":"global","active":true}`
	c, w := setupAdminContext(db, http.MethodPost, "/admin/community-rules", createBody, 1)
	CreateCommunityRule(db)(c)
	var created struct {
		Data struct {
			ID int64 `json:"id"`
		} `json:"data"`
	}
	_ = json.Unmarshal(w.Body.Bytes(), &created)

	body := fmt.Sprintf(`{"community":"1:1","severity":"bad","message_i18n":"x","scope":"global","active":true}`)
	c, w = setupAdminContext(db, http.MethodPut, fmt.Sprintf("/admin/community-rules/%d", created.Data.ID), body, 1)
	c.Params = gin.Params{{Key: "id", Value: fmt.Sprint(created.Data.ID)}}
	UpdateCommunityRule(db)(c)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d", w.Code)
	}
}

func TestDistinctPublicIPs_FiltersPrivate(t *testing.T) {
	ips := distinctPublicIPs([]string{" 1 127.0.0.1 (127.0.0.1) ", " 2 8.8.8.8 (8.8.8.8) ms "})
	if len(ips) != 1 || ips[0] != "8.8.8.8" {
		t.Fatalf("ips = %v", ips)
	}
}

func TestAsSuffixForIP_SanitizeControlChars(t *testing.T) {
	g := stubGeo{byIP: map[string]*domain.ASInfo{
		"8.8.8.8": {ASN: 15169, ShortName: "GO\nOG"},
	}}
	suffix := asSuffixForIP(context.Background(), g, "8.8.8.8")
	if strings.Contains(suffix, "\n") {
		t.Fatalf("suffix = %q", suffix)
	}
}

func TestWaitForASPathEnrichment_DeadlineExit(t *testing.T) {
	queryID := "deadline-exit"
	queryStore.SetRunning(queryID)
	queryStore.MergePartial(queryID, &domain.QueryResult{
		ASPath:         []uint32{64512, 15169},
		ASPathEnriched: []domain.ASInfo{{ASN: 64512}},
	})
	waitForASPathEnrichment(queryID, 1*time.Millisecond)
}

func TestQuickQueriesRefreshFailure(t *testing.T) {
	db := setupDB(t)
	prev := quickQueriesRefreshFn
	quickQueriesRefreshFn = func(*sql.DB) error { return errors.New("refresh fail") }
	t.Cleanup(func() { quickQueriesRefreshFn = prev })

	body := `{"command":"ping","name":"n","target":"1.1.1.1","active":true}`
	c, w := setupAdminContext(db, http.MethodPost, "/admin/quick-queries", body, 1)
	CreateQuickQuery(db)(c)
	if w.Code != http.StatusCreated {
		t.Fatalf("create status = %d", w.Code)
	}

	c, w = setupAdminContext(db, http.MethodPut, "/admin/quick-queries/1", body, 1)
	c.Params = gin.Params{{Key: "id", Value: "1"}}
	UpdateQuickQuery(db)(c)
	if w.Code != http.StatusOK && w.Code != http.StatusNotFound {
		t.Fatalf("update status = %d", w.Code)
	}

	c, w = setupAdminContext(db, http.MethodDelete, "/admin/quick-queries/1", "", 1)
	c.Params = gin.Params{{Key: "id", Value: "1"}}
	DeleteQuickQuery(db)(c)
	if w.Code != http.StatusOK && w.Code != http.StatusNotFound {
		t.Fatalf("delete status = %d", w.Code)
	}

	c, w = setupAdminContext(db, http.MethodPatch, "/admin/quick-queries/1/toggle", "", 1)
	c.Params = gin.Params{{Key: "id", Value: "1"}}
	ToggleQuickQuery(db)(c)
	if w.Code != http.StatusOK && w.Code != http.StatusNoContent && w.Code != http.StatusNotFound {
		t.Fatalf("toggle status = %d", w.Code)
	}
}

func TestCreateQuickQuery_SortOrderError(t *testing.T) {
	db := setupDB(t)
	if _, err := db.Exec(`DROP TABLE quick_queries`); err != nil {
		t.Fatal(err)
	}
	body := `{"command":"ping","name":"n","target":"1.1.1.1","active":true}`
	c, w := setupAdminContext(db, http.MethodPost, "/admin/quick-queries", body, 1)
	CreateQuickQuery(db)(c)
	if w.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d", w.Code)
	}
}

func TestUpdateQuickQuery_GetExistingError(t *testing.T) {
	db := setupDB(t)
	if _, err := db.Exec(`DROP TABLE quick_queries`); err != nil {
		t.Fatal(err)
	}
	body := `{"command":"ping","name":"n","target":"1.1.1.1","active":true}`
	c, w := setupAdminContext(db, http.MethodPut, "/admin/quick-queries/1", body, 1)
	c.Params = gin.Params{{Key: "id", Value: "1"}}
	UpdateQuickQuery(db)(c)
	if w.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d", w.Code)
	}
}

func TestAgentTraceroute_CreateDriverErrorDirect(t *testing.T) {
	withStubAgentDriver(t, stubAgentDriver{createErr: errors.New("no driver")})
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/agent/v1/traceroute", strings.NewReader(`{"target":"1.1.1.1"}`))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Set(middleware.AgentNodeKey, &domain.Node{Name: "n", EnabledCmds: domain.DefaultEnabledCmds()})
	agentTraceroute(testConfig())(c)
	if w.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d", w.Code)
	}
}

func TestAgentTracerouteStream_ValidationAndDriverError(t *testing.T) {
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/agent/v1/traceroute/stream", strings.NewReader(`{`))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Set(middleware.AgentNodeKey, &domain.Node{Name: "n", EnabledCmds: domain.DefaultEnabledCmds()})
	agentTracerouteStream(testConfig())(c)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("json status = %d", w.Code)
	}

	withStubAgentDriver(t, stubAgentDriver{createErr: errors.New("no driver")})
	w = httptest.NewRecorder()
	c, _ = gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/agent/v1/traceroute/stream", strings.NewReader(`{"target":"1.1.1.1"}`))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Set(middleware.AgentNodeKey, &domain.Node{Name: "n", EnabledCmds: domain.DefaultEnabledCmds()})
	agentTracerouteStream(testConfig())(c)
	if w.Code != http.StatusInternalServerError {
		t.Fatalf("driver status = %d", w.Code)
	}
}

func TestAgentBGPRoute_ManagerBuildError(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	mgr := bgp.NewSessionManager(config.BGPConfig{LocalAS: 65000, RouterID: "10.0.0.1", ListenPort: freeListenPort(t)})
	if err := mgr.Start(ctx); err != nil {
		t.Fatal(err)
	}
	defer mgr.Stop()

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/agent/v1/bgp/route", strings.NewReader(`{"prefix":"not-an-ip"}`))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Set(middleware.AgentNodeKey, &domain.Node{ID: 1, Name: "local", EnabledCmds: domain.DefaultEnabledCmds()})
	agentBGPRoute(testConfig(), mgr, geo.New("", ""))(c)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d", w.Code)
	}

	c.Request = httptest.NewRequest(http.MethodPost, "/agent/v1/bgp/route", strings.NewReader(`{"prefix":"8.8.8.8"}`))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Request = c.Request.WithContext(ctx)
	w = httptest.NewRecorder()
	c, _ = gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/agent/v1/bgp/route", strings.NewReader(`{"prefix":"8.8.8.8"}`))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Request = c.Request.WithContext(ctx)
	c.Set(middleware.AgentNodeKey, &domain.Node{ID: 1, Name: "local", EnabledCmds: domain.DefaultEnabledCmds()})
	agentBGPRoute(testConfig(), mgr, geo.New("", ""))(c)
	if w.Code != http.StatusOK && w.Code != http.StatusInternalServerError {
		t.Fatalf("build status = %d, body = %s", w.Code, w.Body.String())
	}
}

func TestCreateBGPNeighbor_RollbackOnAddFailure(t *testing.T) {
	db := setupDB(t)
	nodeRepo := repo.NewNodeRepo(db, "")
	node, _ := nodeRepo.Create(context.Background(), &domain.Node{
		Name: "n", Type: domain.NodeTypeStandalone, Active: true,
		EnabledCmds: domain.DefaultEnabledCmds(),
	})

	prev := bgpMgrAddNeighbor
	bgpMgrAddNeighbor = func(*bgp.SessionManager, *domain.BGPNeighbor) error {
		return errors.New("add failed")
	}
	t.Cleanup(func() { bgpMgrAddNeighbor = prev })

	body := fmt.Sprintf(`{"node_id":%d,"remote_as":174,"peering_ip":"10.0.0.1","neighbor_ip":"10.0.0.2"}`, node.ID)
	c, w := setupAdminContext(db, http.MethodPost, "/admin/bgp-neighbors", body, 1)
	CreateBGPNeighbor(db, bgp.NewSessionManager(config.BGPConfig{LocalAS: 65000}), config.BGPConfig{LocalAS: 65000})(c)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, body = %s", w.Code, w.Body.String())
	}
}

func TestDeleteBGPNeighbor_SyncError(t *testing.T) {
	db := setupDB(t)
	prev := bgpMgrRemoveNeighbor
	bgpMgrRemoveNeighbor = func(*bgp.SessionManager, int64) error { return errors.New("sync remove fail") }
	t.Cleanup(func() { bgpMgrRemoveNeighbor = prev })

	c, w := setupAdminContext(db, http.MethodDelete, "/admin/bgp-neighbors/1", "", 1)
	c.Params = gin.Params{{Key: "id", Value: "1"}}
	DeleteBGPNeighbor(db, bgp.NewSessionManager(config.BGPConfig{LocalAS: 65000}))(c)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d", w.Code)
	}
}

func TestUpdateBGPNeighbor_SyncError(t *testing.T) {
	db := setupDB(t)
	nodeRepo := repo.NewNodeRepo(db, "")
	node, _ := nodeRepo.Create(context.Background(), &domain.Node{
		Name: "n", Type: domain.NodeTypeStandalone, Active: true,
		EnabledCmds: domain.DefaultEnabledCmds(),
	})
	bgpRepo := repo.NewBGPNeighborRepo(db)
	neighbor, _ := bgpRepo.Create(context.Background(), &domain.BGPNeighbor{
		NodeID: node.ID, LocalAS: 65000, RemoteAS: 174,
		PeeringIP: "10.0.0.1", NeighborIP: "10.0.0.2",
	})

	prev := bgpMgrUpdateNeighbor
	bgpMgrUpdateNeighbor = func(*bgp.SessionManager, *domain.BGPNeighbor) error { return errors.New("update fail") }
	t.Cleanup(func() { bgpMgrUpdateNeighbor = prev })

	body := fmt.Sprintf(`{"node_id":%d,"remote_as":174,"peering_ip":"10.0.0.1","neighbor_ip":"10.0.0.2"}`, node.ID)
	c, w := setupAdminContext(db, http.MethodPut, fmt.Sprintf("/admin/bgp-neighbors/%d", neighbor.ID), body, 1)
	c.Params = gin.Params{{Key: "id", Value: fmt.Sprint(neighbor.ID)}}
	UpdateBGPNeighbor(db, bgp.NewSessionManager(config.BGPConfig{LocalAS: 65000}), config.BGPConfig{LocalAS: 65000})(c)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d", w.Code)
	}
}

func TestAgentBGPRoute_BuildRouteError(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	mgr := bgp.NewSessionManager(config.BGPConfig{LocalAS: 65000, RouterID: "10.0.0.1", ListenPort: freeListenPort(t)})
	if err := mgr.Start(ctx); err != nil {
		t.Fatal(err)
	}
	defer mgr.Stop()

	prev := agentBuildBGPRouteFn
	agentBuildBGPRouteFn = func(context.Context, *bgp.SessionManager, int64, string) (*domain.BGPResult, error) {
		return nil, errors.New("build failed")
	}
	t.Cleanup(func() { agentBuildBGPRouteFn = prev })

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/agent/v1/bgp/route", strings.NewReader(`{"prefix":"8.8.8.8"}`))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Set(middleware.AgentNodeKey, &domain.Node{ID: 1, Name: "local", EnabledCmds: domain.DefaultEnabledCmds()})
	agentBGPRoute(testConfig(), mgr, geo.New("", ""))(c)
	if w.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d", w.Code)
	}
}

func TestListBGPNeighbors_EstablishedPrefixes(t *testing.T) {
	db := setupDB(t)
	bgpRepo := repo.NewBGPNeighborRepo(db)
	nodeRepo := repo.NewNodeRepo(db, "")
	node, _ := nodeRepo.Create(context.Background(), &domain.Node{
		Name: "n", Type: domain.NodeTypeStandalone, Active: true,
		EnabledCmds: domain.DefaultEnabledCmds(),
	})
	neighbor, _ := bgpRepo.Create(context.Background(), &domain.BGPNeighbor{
		NodeID: node.ID, LocalAS: 65000, RemoteAS: 174,
		PeeringIP: "10.0.0.1", NeighborIP: "10.0.0.2",
	})

	prev := bgpNeighborStatusesFn
	bgpNeighborStatusesFn = func(*bgp.SessionManager) map[int64]domain.BGPSessionState {
		return map[int64]domain.BGPSessionState{neighbor.ID: domain.BGPSessionEstablished}
	}
	t.Cleanup(func() { bgpNeighborStatusesFn = prev })

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	mgr := bgp.NewSessionManager(config.BGPConfig{LocalAS: 65000, RouterID: "10.0.0.1", ListenPort: freeListenPort(t)})
	_ = mgr.Start(ctx)
	defer mgr.Stop()

	c, w := setupAdminContext(db, http.MethodGet, "/admin/bgp-neighbors", "", 1)
	ListBGPNeighbors(db, mgr, config.BGPConfig{LocalAS: 65000})(c)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d", w.Code)
	}
}

func TestRefreshCacheWarnings(t *testing.T) {
	db := setupDB(t)
	nodeRepo := repo.NewNodeRepo(db, "")
	node, _ := nodeRepo.Create(context.Background(), &domain.Node{
		Name: "n", Type: domain.NodeTypeStandalone, Active: true,
		EnabledCmds: domain.DefaultEnabledCmds(),
	})

	prevNodes := refreshNodesCacheFn
	refreshNodesCacheFn = func(*sql.DB, string) error { return errors.New("refresh nodes fail") }
	t.Cleanup(func() { refreshNodesCacheFn = prevNodes })

	body := `{"name":"new","type":"standalone","enabled_cmds":["ping"]}`
	c, w := setupAdminContext(db, http.MethodPost, "/admin/nodes", body, 1)
	CreateNode(db, "")(c)
	if w.Code != http.StatusCreated {
		t.Fatalf("create status = %d", w.Code)
	}

	updateBody := fmt.Sprintf(`{"name":"updated","type":"standalone","enabled_cmds":["ping"]}`)
	c, w = setupAdminContext(db, http.MethodPut, fmt.Sprintf("/admin/nodes/%d", node.ID), updateBody, 1)
	c.Params = gin.Params{{Key: "id", Value: fmt.Sprint(node.ID)}}
	UpdateNode(db, "")(c)
	if w.Code != http.StatusOK {
		t.Fatalf("update status = %d", w.Code)
	}

	c, w = setupAdminContext(db, http.MethodPost, fmt.Sprintf("/admin/nodes/%d/default", node.ID), "", 1)
	c.Params = gin.Params{{Key: "id", Value: fmt.Sprint(node.ID)}}
	SetDefaultNode(db, "")(c)
	if w.Code != http.StatusOK {
		t.Fatalf("default status = %d", w.Code)
	}

	c, w = setupAdminContext(db, http.MethodDelete, fmt.Sprintf("/admin/nodes/%d", node.ID), "", 1)
	c.Params = gin.Params{{Key: "id", Value: fmt.Sprint(node.ID)}}
	DeleteNode(db, "", nil)(c)
	if w.Code != http.StatusOK {
		t.Fatalf("delete status = %d", w.Code)
	}

	prevComm := refreshCommunitiesCacheFn
	refreshCommunitiesCacheFn = func(*sql.DB) error { return errors.New("refresh communities fail") }
	t.Cleanup(func() { refreshCommunitiesCacheFn = prevComm })

	ruleBody := `{"community":"1:1","severity":"info","message_i18n":"x","scope":"global","active":true}`
	c, w = setupAdminContext(db, http.MethodPost, "/admin/community-rules", ruleBody, 1)
	CreateCommunityRule(db)(c)
	if w.Code != http.StatusCreated {
		t.Fatalf("create rule status = %d", w.Code)
	}

	prevSettings := refreshSettingsCacheFn
	refreshSettingsCacheFn = func(*sql.DB, uint32) error { return errors.New("refresh settings fail") }
	t.Cleanup(func() { refreshSettingsCacheFn = prevSettings })

	c, w = setupAdminContext(db, http.MethodPut, "/admin/settings", `{"site_name":"warn"}`, 1)
	UpdateSettings(db)(c)
	if w.Code != http.StatusOK {
		t.Fatalf("settings status = %d", w.Code)
	}
}

func TestTestNode_DriverCreateError(t *testing.T) {
	db := setupDB(t)
	nodeRepo := repo.NewNodeRepo(db, "")
	node, _ := nodeRepo.Create(context.Background(), &domain.Node{
		Name: "n", Type: domain.NodeTypeStandalone, Active: true,
		EnabledCmds: domain.DefaultEnabledCmds(), AgentToken: "tok",
	})

	prev := newQueryDriverFn
	newQueryDriverFn = func(*domain.Node, *config.Config) (driver.NodeDriver, error) {
		return nil, errors.New("driver fail")
	}
	t.Cleanup(func() { newQueryDriverFn = prev })

	c, w := setupAdminContext(db, http.MethodPost, fmt.Sprintf("/admin/nodes/%d/test", node.ID), "", 1)
	c.Params = gin.Params{{Key: "id", Value: fmt.Sprint(node.ID)}}
	TestNode(db, "", testConfig())(c)
	if w.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d", w.Code)
	}
}

func TestSubmitQuery_TracerouteStreamingPaths(t *testing.T) {
	db := setupDB(t)
	cfg := testConfig()
	g := geo.New("", "")
	nodeRepo := repo.NewNodeRepo(db, "")
	node, _ := nodeRepo.Create(context.Background(), &domain.Node{
		Name: "n", Type: domain.NodeTypeStandalone, Active: true,
		EnabledCmds: domain.DefaultEnabledCmds(), AgentToken: "tok",
	})
	refreshTestSiteCache(t, db)

	prev := handlerEngineExecute
	prevDone := submitQueryAsyncDone
	done := make(chan struct{})
	submitQueryAsyncDone = func() { close(done) }
	handlerEngineExecute = func(h *Handler, ctx context.Context, query *domain.Query, opts ...engine.ExecuteOption) (*domain.QueryResult, error) {
		var opt engine.ExecuteOption
		if len(opts) > 0 {
			opt = opts[0]
		}
		opt.OnLine("traceroute to 1.1.1.1")
		opt.OnLine("Start")
		opt.OnLine(" 1  1.1.1.1 (1.1.1.1)  10.123 ms")
		opt.OnLine(" 2  * * *")
		opt.OnLine(" 3  8.8.8.8 (8.8.8.8)  20.456 ms")
		return &domain.QueryResult{Status: domain.StatusDone, Raw: "done"}, nil
	}
	t.Cleanup(func() {
		<-done
		handlerEngineExecute = prev
		submitQueryAsyncDone = prevDone
	})

	body := fmt.Sprintf(`{"node_id":%d,"command":"traceroute","target":"1.1.1.1"}`, node.ID)
	c, w := setupContext(db, http.MethodPost, "/query", body)
	h := New(db, cfg, g, nil)
	h.SubmitQuery()(c)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", w.Code, w.Body.String())
	}
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("submit query async timeout")
	}
}

func TestSubmitQuery_EngineError(t *testing.T) {
	db := setupDB(t)
	cfg := testConfig()
	nodeRepo := repo.NewNodeRepo(db, "")
	node, _ := nodeRepo.Create(context.Background(), &domain.Node{
		Name: "n", Type: domain.NodeTypeStandalone, Active: true,
		EnabledCmds: domain.DefaultEnabledCmds(), AgentToken: "tok",
	})
	refreshTestSiteCache(t, db)

	prev := handlerEngineExecute
	prevDone := submitQueryAsyncDone
	done := make(chan struct{})
	submitQueryAsyncDone = func() { close(done) }
	handlerEngineExecute = func(*Handler, context.Context, *domain.Query, ...engine.ExecuteOption) (*domain.QueryResult, error) {
		return nil, errors.New("engine fail")
	}
	t.Cleanup(func() {
		<-done
		handlerEngineExecute = prev
		submitQueryAsyncDone = prevDone
	})

	body := fmt.Sprintf(`{"node_id":%d,"command":"ping","target":"1.1.1.1"}`, node.ID)
	c, w := setupContext(db, http.MethodPost, "/query", body)
	New(db, cfg, nil, nil).SubmitQuery()(c)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d", w.Code)
	}
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("submit query async timeout")
	}
}

func TestUpdateAccount_UnauthorizedTypes(t *testing.T) {
	db := setupDB(t)
	c, w := setupAdminContext(db, http.MethodPut, "/admin/account", `{"email":"a@b.c","current_password":"x"}`, 1)
	c.Set("user_id", "not-int")
	UpdateAccount(db)(c)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("bad user_id status = %d", w.Code)
	}

	c, w = setupContext(db, http.MethodPut, "/admin/account", `{"email":"a@b.c","current_password":"x"}`)
	UpdateAccount(db)(c)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("missing user_id status = %d", w.Code)
	}
}

func TestUpdateApply_DefaultHookBodies(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]interface{}{"tag_name": "v9.9.9"})
	}))
	defer srv.Close()

	upd := updater.New("HopStat/HopStat", "v1.0.0", true)
	upd.SetReleaseAPIURL(srv.URL)
	c, w := setupAdminContext(nil, http.MethodPost, "/admin/update/apply", "", 1)
	UpdateApply(upd)(c)
	if w.Code != http.StatusAccepted && w.Code != http.StatusBadRequest {
		t.Fatalf("default blocked status = %d", w.Code)
	}

	prevDelay := updateApplyDelay
	updateApplyDelay = func() {}
	t.Cleanup(func() { updateApplyDelay = prevDelay })

	prevBlocked := updateApplyBlocked
	updateApplyBlocked = func(*updater.Status) (bool, string) { return false, "" }
	t.Cleanup(func() { updateApplyBlocked = prevBlocked })

	done := make(chan struct{}, 1)
	prevAsync := updateApplyAsync
	updateApplyAsync = func(fn func()) {
		fn()
		done <- struct{}{}
	}
	t.Cleanup(func() { updateApplyAsync = prevAsync })

	c, w = setupAdminContext(nil, http.MethodPost, "/admin/update/apply", "", 1)
	UpdateApply(upd)(c)
	if w.Code != http.StatusAccepted {
		t.Fatalf("accepted status = %d", w.Code)
	}
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("default async path did not run")
	}
}

func TestAsSuffixForIP_OrgNameFallback(t *testing.T) {
	g := stubGeo{byIP: map[string]*domain.ASInfo{
		"8.8.8.8": {ASN: 15169, ShortName: "", OrgName: "Google LLC"},
	}}
	suffix := asSuffixForIP(context.Background(), g, "8.8.8.8")
	if !strings.Contains(suffix, "15169") {
		t.Fatalf("suffix = %q", suffix)
	}
}

func TestValidQuickQueryTarget_InvalidDefault(t *testing.T) {
	if validQuickQueryTarget("unknown", "1.1.1.1") {
		t.Fatal("unknown command should fail")
	}
}

type errReader struct{}

func (errReader) Read([]byte) (int, error) { return 0, errors.New("read fail") }
func (errReader) Close() error             { return nil }

type errSeeker struct {
	*bytes.Reader
}

func (e errSeeker) Seek(int64, int) (int64, error) { return 0, errors.New("seek fail") }

func TestUploadLogo_ErrorPaths(t *testing.T) {
	db := setupDB(t)
	uploadDir := t.TempDir()
	SetLogoUploadsDir(uploadDir)

	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	part, _ := writer.CreateFormFile("logo", "logo.png")
	_, _ = part.Write([]byte{0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A})
	writer.Close()

	req := httptest.NewRequest(http.MethodPost, "/admin/settings/logo", body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = req

	prevMkdir := uploadLogoMkdirAll
	uploadLogoMkdirAll = func(string, os.FileMode) error { return errors.New("mkdir fail") }
	UploadLogo(db)(c)
	uploadLogoMkdirAll = prevMkdir
	if w.Code != http.StatusInternalServerError {
		t.Fatalf("mkdir status = %d", w.Code)
	}

	prevCreate := uploadLogoCreate
	uploadLogoCreate = func(string) (*os.File, error) { return nil, errors.New("create fail") }
	w = httptest.NewRecorder()
	c, _ = gin.CreateTestContext(w)
	c.Request = req
	UploadLogo(db)(c)
	uploadLogoCreate = prevCreate
	if w.Code != http.StatusInternalServerError {
		t.Fatalf("create status = %d", w.Code)
	}

	prevCopy := uploadLogoCopy
	uploadLogoCopy = func(io.Writer, io.Reader) (int64, error) { return 0, errors.New("copy fail") }
	w = httptest.NewRecorder()
	c, _ = gin.CreateTestContext(w)
	c.Request = req
	UploadLogo(db)(c)
	uploadLogoCopy = prevCopy
	if w.Code != http.StatusInternalServerError {
		t.Fatalf("copy status = %d", w.Code)
	}
}

func TestListAudit_FiltersAndLimitClamp(t *testing.T) {
	db := setupDB(t)
	auditRepo := repo.NewAuditRepo(db)
	_ = auditRepo.Log(context.Background(), &domain.AuditEntry{Command: "ping", Success: true, SourceIP: "=1+1"})

	c, w := setupAdminContext(db, http.MethodGet, "/admin/audit?limit=500&page=1&node_id=1&command=ping&source_ip=1.1.1.1&from=2020-01-01&to=2020-02-01", "", 1)
	ListAudit(db)(c)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d", w.Code)
	}
}

func TestExportAudit_FormulaInjectionFinal(t *testing.T) {
	db := setupDB(t)
	auditRepo := repo.NewAuditRepo(db)
	_ = auditRepo.Log(context.Background(), &domain.AuditEntry{
		Command: "ping", Success: true, SourceIP: "=1+1", Params: "+cmd", ErrorMsg: "@evil",
	})
	c, w := setupAdminContext(db, http.MethodGet, "/admin/audit/export", "", 1)
	ExportAudit(db)(c)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d", w.Code)
	}
	if !strings.Contains(w.Body.String(), "'=") {
		t.Fatalf("expected csv escape, body = %q", w.Body.String())
	}
}

func TestUpdateNode_NotFoundAndUpdateError(t *testing.T) {
	db := setupDB(t)
	c, w := setupAdminContext(db, http.MethodPut, "/admin/nodes/9999", `{"name":"x"}`, 1)
	c.Params = gin.Params{{Key: "id", Value: "9999"}}
	UpdateNode(db, "")(c)
	if w.Code != http.StatusNotFound {
		t.Fatalf("not found status = %d", w.Code)
	}

	nodeRepo := repo.NewNodeRepo(db, "")
	node, _ := nodeRepo.Create(context.Background(), &domain.Node{
		Name: "n", Type: domain.NodeTypeStandalone, Active: true,
		EnabledCmds: domain.DefaultEnabledCmds(),
	})
	db.Close()
	c, w = setupAdminContext(db, http.MethodPut, fmt.Sprintf("/admin/nodes/%d", node.ID), `{"name":"x"}`, 1)
	c.Params = gin.Params{{Key: "id", Value: fmt.Sprint(node.ID)}}
	UpdateNode(db, "")(c)
	if w.Code != http.StatusInternalServerError && w.Code != http.StatusNotFound {
		t.Fatalf("update error status = %d", w.Code)
	}
}

func TestCreateBGPNeighbor_RollbackDeleteError(t *testing.T) {
	db := setupDB(t)
	nodeRepo := repo.NewNodeRepo(db, "")
	node, _ := nodeRepo.Create(context.Background(), &domain.Node{
		Name: "n", Type: domain.NodeTypeStandalone, Active: true,
		EnabledCmds: domain.DefaultEnabledCmds(),
	})

	prevAdd := bgpMgrAddNeighbor
	bgpMgrAddNeighbor = func(*bgp.SessionManager, *domain.BGPNeighbor) error { return errors.New("add fail") }
	t.Cleanup(func() { bgpMgrAddNeighbor = prevAdd })

	body := fmt.Sprintf(`{"node_id":%d,"remote_as":174,"peering_ip":"10.0.0.1","neighbor_ip":"10.0.0.2"}`, node.ID)
	c, w := setupAdminContext(db, http.MethodPost, "/admin/bgp-neighbors", body, 1)
	CreateBGPNeighbor(db, bgp.NewSessionManager(config.BGPConfig{LocalAS: 65000}), config.BGPConfig{LocalAS: 65000})(c)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d", w.Code)
	}

	db.Close()
	c, w = setupAdminContext(db, http.MethodPost, "/admin/bgp-neighbors", body, 1)
	CreateBGPNeighbor(db, nil, config.BGPConfig{LocalAS: 65000})(c)
	if w.Code != http.StatusInternalServerError {
		t.Fatalf("create db error status = %d", w.Code)
	}
}

func TestUpdateBGPNeighbor_ValidateAndConfig(t *testing.T) {
	db := setupDB(t)
	body := `{"node_id":1,"remote_as":174}`
	c, w := setupAdminContext(db, http.MethodPut, "/admin/bgp-neighbors/1", body, 1)
	c.Params = gin.Params{{Key: "id", Value: "1"}}
	UpdateBGPNeighbor(db, nil, config.BGPConfig{LocalAS: 65000})(c)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("validate status = %d", w.Code)
	}

	c, w = setupAdminContext(db, http.MethodPut, "/admin/bgp-neighbors/1", `{"node_id":1,"remote_as":174,"peering_ip":"10.0.0.1","neighbor_ip":"10.0.0.2"}`, 1)
	c.Params = gin.Params{{Key: "id", Value: "1"}}
	UpdateBGPNeighbor(db, nil, config.BGPConfig{})(c)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("config status = %d", w.Code)
	}
}

func TestCommunityRules_ErrorPaths(t *testing.T) {
	db := setupDB(t)
	createBody := `{"community":"1:1","severity":"info","message_i18n":"x","scope":"global","active":true}`
	c, w := setupAdminContext(db, http.MethodPost, "/admin/community-rules", createBody, 1)
	CreateCommunityRule(db)(c)
	var created struct {
		Data struct {
			ID int64 `json:"id"`
		} `json:"data"`
	}
	_ = json.Unmarshal(w.Body.Bytes(), &created)

	body := fmt.Sprintf(`{"community":"1:1","severity":"bad","message_i18n":"x","scope":"global","active":true}`)
	c, w = setupAdminContext(db, http.MethodPut, fmt.Sprintf("/admin/community-rules/%d", created.Data.ID), body, 1)
	c.Params = gin.Params{{Key: "id", Value: fmt.Sprint(created.Data.ID)}}
	UpdateCommunityRule(db)(c)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("severity status = %d", w.Code)
	}

	body = fmt.Sprintf(`{"community":"1:1","severity":"info","message_i18n":"x","scope":"bad","active":true}`)
	c, w = setupAdminContext(db, http.MethodPut, fmt.Sprintf("/admin/community-rules/%d", created.Data.ID), body, 1)
	c.Params = gin.Params{{Key: "id", Value: fmt.Sprint(created.Data.ID)}}
	UpdateCommunityRule(db)(c)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("scope status = %d", w.Code)
	}

	db.Close()
	c, w = setupAdminContext(db, http.MethodDelete, fmt.Sprintf("/admin/community-rules/%d", created.Data.ID), "", 1)
	c.Params = gin.Params{{Key: "id", Value: fmt.Sprint(created.Data.ID)}}
	DeleteCommunityRule(db)(c)
	if w.Code != http.StatusInternalServerError {
		t.Fatalf("delete status = %d", w.Code)
	}

	c, w = setupAdminContext(db, http.MethodPatch, fmt.Sprintf("/admin/community-rules/%d/toggle", created.Data.ID), "", 1)
	c.Params = gin.Params{{Key: "id", Value: fmt.Sprint(created.Data.ID)}}
	ToggleCommunityRule(db)(c)
	if w.Code != http.StatusInternalServerError {
		t.Fatalf("toggle status = %d", w.Code)
	}
}

func TestUpdateAccount_UniqueConstraintOnUpdate(t *testing.T) {
	db := setupDB(t)
	adminID := seedAdminPassword(t, db, "password12345")
	if _, err := db.Exec(`INSERT INTO users (email, password_hash, created_at) VALUES (?, ?, datetime('now'))`,
		"taken@hopstat.local", "hash"); err != nil {
		t.Fatal(err)
	}
	body := `{"email":"taken@hopstat.local","current_password":"password12345","new_password":"newpassword12"}`
	c, w := setupAdminContext(db, http.MethodPut, "/admin/account", body, adminID)
	UpdateAccount(db)(c)
	if w.Code != http.StatusConflict {
		t.Fatalf("status = %d", w.Code)
	}
}

func TestWaitForASPathEnrichment_NotifyBranch(t *testing.T) {
	queryID := "notify-branch"
	queryStore.SetRunning(queryID)
	queryStore.MergePartial(queryID, &domain.QueryResult{
		ASPath:         []uint32{64512, 15169},
		ASPathEnriched: []domain.ASInfo{{ASN: 64512}},
	})
	go func() {
		time.Sleep(10 * time.Millisecond)
		queryStore.MergePartial(queryID, &domain.QueryResult{
			ASPath:         []uint32{64512, 15169},
			ASPathEnriched: []domain.ASInfo{{ASN: 64512}, {ASN: 15169}},
		})
	}()
	waitForASPathEnrichment(queryID, 200*time.Millisecond)
}

func TestAsPathEnrichmentReady_NotEnough(t *testing.T) {
	if asPathEnrichmentReady(&domain.QueryResult{
		ASPath:         []uint32{64512, 15169},
		ASPathEnriched: []domain.ASInfo{{ASN: 64512}},
	}) {
		t.Fatal("expected not ready")
	}
}

func TestDistinctPublicIPs_EmptyIPSkipped(t *testing.T) {
	if ips := distinctPublicIPs([]string{" 1 no-ip-here ", " 2 8.8.8.8 (8.8.8.8) ms "}); len(ips) != 1 {
		t.Fatalf("ips = %v", ips)
	}
}

func TestUpdateApply_NativeHookBodies(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]interface{}{"tag_name": "v9.9.9"})
	}))
	defer srv.Close()

	prevBlocked := updateApplyBlocked
	updateApplyBlocked = func(*updater.Status) (bool, string) { return false, "" }
	t.Cleanup(func() { updateApplyBlocked = prevBlocked })

	upd := updater.New("HopStat/HopStat", "v1.0.0", true)
	upd.SetReleaseAPIURL(srv.URL)
	c, w := setupAdminContext(nil, http.MethodPost, "/admin/update/apply", "", 1)
	UpdateApply(upd)(c)
	if w.Code != http.StatusAccepted {
		t.Fatalf("status = %d", w.Code)
	}
	time.Sleep(600 * time.Millisecond)
}

func TestUpdateApplyBlockedDefaultFn(t *testing.T) {
	blocked, msg := updateApplyBlocked(&updater.Status{SelfUpdateEnabled: false, SelfUpdateReason: ""})
	if !blocked || msg == "" {
		t.Fatalf("blocked=%v msg=%q", blocked, msg)
	}
	if blocked, msg = updateApplyBlocked(&updater.Status{SelfUpdateEnabled: true}); blocked {
		t.Fatal("expected enabled status not blocked")
	}
}

func TestUpdateApplyDefaultAsyncAndDelayFns(t *testing.T) {
	done := make(chan struct{}, 1)
	updateApplyAsync(func() { done <- struct{}{} })
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("async fn did not run")
	}
	updateApplyDelay()
}

func TestCreateBGPNeighbor_RollbackDeleteHookError(t *testing.T) {
	db := setupDB(t)
	nodeRepo := repo.NewNodeRepo(db, "")
	node, _ := nodeRepo.Create(context.Background(), &domain.Node{
		Name: "n", Type: domain.NodeTypeStandalone, Active: true,
		EnabledCmds: domain.DefaultEnabledCmds(),
	})
	prevAdd := bgpMgrAddNeighbor
	bgpMgrAddNeighbor = func(*bgp.SessionManager, *domain.BGPNeighbor) error { return errors.New("add fail") }
	t.Cleanup(func() { bgpMgrAddNeighbor = prevAdd })
	prevDel := deleteBGPNeighborRecordFn
	deleteBGPNeighborRecordFn = func(context.Context, domain.BGPNeighborRepository, int64) error {
		return errors.New("delete fail")
	}
	t.Cleanup(func() { deleteBGPNeighborRecordFn = prevDel })

	body := fmt.Sprintf(`{"node_id":%d,"remote_as":174,"peering_ip":"10.0.0.1","neighbor_ip":"10.0.0.2"}`, node.ID)
	c, w := setupAdminContext(db, http.MethodPost, "/admin/bgp-neighbors", body, 1)
	CreateBGPNeighbor(db, bgp.NewSessionManager(config.BGPConfig{LocalAS: 65000}), config.BGPConfig{LocalAS: 65000})(c)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d", w.Code)
	}
}

func TestUpdateNode_BindAndUpdateErrors(t *testing.T) {
	db := setupDB(t)
	nodeRepo := repo.NewNodeRepo(db, "")
	node, _ := nodeRepo.Create(context.Background(), &domain.Node{
		Name: "n", Type: domain.NodeTypeStandalone, Active: true,
		EnabledCmds: domain.DefaultEnabledCmds(),
	})

	c, w := setupAdminContext(db, http.MethodPut, fmt.Sprintf("/admin/nodes/%d", node.ID), `{`, 1)
	c.Params = gin.Params{{Key: "id", Value: fmt.Sprint(node.ID)}}
	UpdateNode(db, "")(c)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("bind status = %d", w.Code)
	}

	prev := updateNodeRecordFn
	updateNodeRecordFn = func(context.Context, domain.NodeRepository, *domain.Node) (*domain.Node, error) {
		return nil, errors.New("update fail")
	}
	t.Cleanup(func() { updateNodeRecordFn = prev })

	c, w = setupAdminContext(db, http.MethodPut, fmt.Sprintf("/admin/nodes/%d", node.ID), `{"name":"x"}`, 1)
	c.Params = gin.Params{{Key: "id", Value: fmt.Sprint(node.ID)}}
	UpdateNode(db, "")(c)
	if w.Code != http.StatusInternalServerError {
		t.Fatalf("update status = %d", w.Code)
	}
}

func TestListAudit_CountTodayErrorDroppedTable(t *testing.T) {
	db := setupDB(t)
	auditRepo := repo.NewAuditRepo(db)
	_ = auditRepo.Log(context.Background(), &domain.AuditEntry{Command: "ping", Success: true})
	if _, err := db.Exec(`DROP TABLE audit_log`); err != nil {
		t.Fatal(err)
	}
	c, w := setupAdminContext(db, http.MethodGet, "/admin/audit", "", 1)
	ListAudit(db)(c)
	if w.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d", w.Code)
	}
}

func TestUpdateAccount_UpdateCredentialsErrors(t *testing.T) {
	db := setupDB(t)
	adminID := seedAdminPassword(t, db, "password12345")
	if _, err := db.Exec(`DROP TABLE users`); err != nil {
		t.Fatal(err)
	}
	body := `{"email":"new@hopstat.local","current_password":"password12345"}`
	c, w := setupAdminContext(db, http.MethodPut, "/admin/account", body, adminID)
	UpdateAccount(db)(c)
	if w.Code != http.StatusInternalServerError && w.Code != http.StatusNotFound {
		t.Fatalf("get by email status = %d", w.Code)
	}

	db = setupDB(t)
	adminID = seedAdminPassword(t, db, "password12345")
	body = `{"email":"unique@hopstat.local","current_password":"password12345","new_password":"newpassword12"}`
	c, w = setupAdminContext(db, http.MethodPut, "/admin/account", body, adminID)
	UpdateAccount(db)(c)
	if w.Code != http.StatusOK {
		t.Fatalf("first update status = %d", w.Code)
	}
	db.Close()
	c, w = setupAdminContext(db, http.MethodPut, "/admin/account", body, adminID)
	UpdateAccount(db)(c)
	if w.Code != http.StatusNotFound && w.Code != http.StatusInternalServerError {
		t.Fatalf("update credentials status = %d", w.Code)
	}
}

func TestUpdateCommunityRule_BindAndUpdateErrors(t *testing.T) {
	db := setupDB(t)
	c, w := setupAdminContext(db, http.MethodPut, "/admin/community-rules/1", `{`, 1)
	c.Params = gin.Params{{Key: "id", Value: "1"}}
	UpdateCommunityRule(db)(c)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("bind status = %d", w.Code)
	}

	createBody := `{"community":"1:1","severity":"","message_i18n":"x","scope":"","active":true}`
	c, w = setupAdminContext(db, http.MethodPost, "/admin/community-rules", createBody, 1)
	CreateCommunityRule(db)(c)
	var created struct {
		Data struct {
			ID int64 `json:"id"`
		} `json:"data"`
	}
	_ = json.Unmarshal(w.Body.Bytes(), &created)

	updateBody := fmt.Sprintf(`{"community":"1:1","severity":"","message_i18n":"x","scope":"","active":true}`)
	c, w = setupAdminContext(db, http.MethodPut, fmt.Sprintf("/admin/community-rules/%d", created.Data.ID), updateBody, 1)
	c.Params = gin.Params{{Key: "id", Value: fmt.Sprint(created.Data.ID)}}
	UpdateCommunityRule(db)(c)
	if w.Code != http.StatusOK {
		t.Fatalf("defaults status = %d", w.Code)
	}

	db.Close()
	c, w = setupAdminContext(db, http.MethodPut, fmt.Sprintf("/admin/community-rules/%d", created.Data.ID), updateBody, 1)
	c.Params = gin.Params{{Key: "id", Value: fmt.Sprint(created.Data.ID)}}
	UpdateCommunityRule(db)(c)
	if w.Code != http.StatusInternalServerError {
		t.Fatalf("update error status = %d", w.Code)
	}
}

func TestUploadLogo_SVGAndWebPPaths(t *testing.T) {
	db := setupDB(t)
	uploadDir := t.TempDir()
	SetLogoUploadsDir(uploadDir)

	svgBody := &bytes.Buffer{}
	svgWriter := multipart.NewWriter(svgBody)
	svgPart, _ := svgWriter.CreateFormFile("logo", "logo.svg")
	_, _ = io.WriteString(svgPart, `<svg xmlns="http://www.w3.org/2000/svg"><text>ok</text></svg>`)
	svgWriter.Close()
	svgReq := httptest.NewRequest(http.MethodPost, "/admin/settings/logo", svgBody)
	svgReq.Header.Set("Content-Type", svgWriter.FormDataContentType())
	svgW := httptest.NewRecorder()
	svgC, _ := gin.CreateTestContext(svgW)
	svgC.Request = svgReq
	UploadLogo(db)(svgC)
	if svgW.Code != http.StatusOK {
		t.Fatalf("svg status = %d, body = %s", svgW.Code, svgW.Body.String())
	}
}

func TestAsSuffixForIP_ShortNameFallback(t *testing.T) {
	g := stubGeo{byIP: map[string]*domain.ASInfo{
		"8.8.4.4": {ASN: 15169, ShortName: "GOOG", OrgName: ""},
	}}
	suffix := asSuffixForIP(context.Background(), g, "8.8.4.4")
	if !strings.Contains(suffix, "GOOG") {
		t.Fatalf("suffix = %q", suffix)
	}
}

func TestCreateQuickQuery_DBCreateError(t *testing.T) {
	db := setupDB(t)
	if _, err := db.Exec(`DROP TABLE quick_queries`); err != nil {
		t.Fatal(err)
	}
	body := `{"command":"ping","name":"n","target":"1.1.1.1","active":true}`
	c, w := setupAdminContext(db, http.MethodPost, "/admin/quick-queries", body, 1)
	CreateQuickQuery(db)(c)
	if w.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d", w.Code)
	}
}

func TestListAudit_CountTodayHookError(t *testing.T) {
	db := setupDB(t)
	auditRepo := repo.NewAuditRepo(db)
	_ = auditRepo.Log(context.Background(), &domain.AuditEntry{Command: "ping", Success: true})
	prev := countTodayAuditFn
	countTodayAuditFn = func(*sql.DB, context.Context) (int, error) { return 0, errors.New("count fail") }
	t.Cleanup(func() { countTodayAuditFn = prev })

	c, w := setupAdminContext(db, http.MethodGet, "/admin/audit", "", 1)
	ListAudit(db)(c)
	if w.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d", w.Code)
	}
}

func TestExportAudit_CSVInjectionAt(t *testing.T) {
	db := setupDB(t)
	auditRepo := repo.NewAuditRepo(db)
	_ = auditRepo.Log(context.Background(), &domain.AuditEntry{
		Command: "ping", Success: true, NodeName: "@node", ErrorMsg: "@err",
	})
	c, w := setupAdminContext(db, http.MethodGet, "/admin/audit/export", "", 1)
	ExportAudit(db)(c)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d", w.Code)
	}
	if !strings.Contains(w.Body.String(), "'@") {
		t.Fatalf("body = %q", w.Body.String())
	}
}

func TestUpdateAccount_GetByEmailOnChangeError(t *testing.T) {
	db := setupDB(t)
	adminID := seedAdminPassword(t, db, "password12345")
	prev := getUserByEmailForAccountFn
	getUserByEmailForAccountFn = func(context.Context, domain.UserRepository, string) (*domain.User, error) {
		return nil, errors.New("lookup fail")
	}
	t.Cleanup(func() { getUserByEmailForAccountFn = prev })

	body := `{"email":"changed@hopstat.local","current_password":"password12345"}`
	c, w := setupAdminContext(db, http.MethodPut, "/admin/account", body, adminID)
	UpdateAccount(db)(c)
	if w.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d", w.Code)
	}
}

func TestUpdateAccount_UniqueOnCredentials(t *testing.T) {
	db := setupDB(t)
	adminID := seedAdminPassword(t, db, "password12345")
	prev := updateAccountCredentialsFn
	updateAccountCredentialsFn = func(context.Context, domain.UserRepository, int64, string, string) (*domain.User, error) {
		return nil, errors.New("UNIQUE constraint failed: users.email")
	}
	t.Cleanup(func() { updateAccountCredentialsFn = prev })

	body := `{"email":"changed@hopstat.local","current_password":"password12345","new_password":"newpassword12"}`
	c, w := setupAdminContext(db, http.MethodPut, "/admin/account", body, adminID)
	UpdateAccount(db)(c)
	if w.Code != http.StatusConflict {
		t.Fatalf("status = %d", w.Code)
	}
}

func TestDeleteToggleCommunityRule_RefreshWarnings(t *testing.T) {
	db := setupDB(t)
	createBody := `{"community":"1:1","severity":"info","message_i18n":"x","scope":"global","active":true}`
	c, w := setupAdminContext(db, http.MethodPost, "/admin/community-rules", createBody, 1)
	CreateCommunityRule(db)(c)
	var created struct {
		Data struct {
			ID int64 `json:"id"`
		} `json:"data"`
	}
	_ = json.Unmarshal(w.Body.Bytes(), &created)

	prev := refreshCommunitiesCacheFn
	refreshCommunitiesCacheFn = func(*sql.DB) error { return errors.New("refresh fail") }
	t.Cleanup(func() { refreshCommunitiesCacheFn = prev })

	c, w = setupAdminContext(db, http.MethodDelete, fmt.Sprintf("/admin/community-rules/%d", created.Data.ID), "", 1)
	c.Params = gin.Params{{Key: "id", Value: fmt.Sprint(created.Data.ID)}}
	DeleteCommunityRule(db)(c)
	if w.Code != http.StatusOK {
		t.Fatalf("delete status = %d", w.Code)
	}

	c, w = setupAdminContext(db, http.MethodPatch, fmt.Sprintf("/admin/community-rules/%d/toggle", created.Data.ID), "", 1)
	c.Params = gin.Params{{Key: "id", Value: fmt.Sprint(created.Data.ID)}}
	ToggleCommunityRule(db)(c)
	if w.Code != http.StatusOK {
		t.Fatalf("toggle status = %d", w.Code)
	}
}

func TestStreamResult_NotifyChannel(t *testing.T) {
	db := setupDB(t)
	queryID := "notify-channel"
	queryStore.SetRunning(queryID)
	go func() {
		time.Sleep(20 * time.Millisecond)
		queryStore.MergePartial(queryID, &domain.QueryResult{
			Status: domain.StatusRunning,
			ASPath: []uint32{64512},
			Parsed: &domain.BGPResult{},
		})
		time.Sleep(20 * time.Millisecond)
		queryStore.MarkOutputComplete(queryID)
		queryStore.Set(queryID, &domain.QueryResult{ID: queryID, Status: domain.StatusDone})
	}()

	c, w := setupContext(db, http.MethodGet, "/query/"+queryID+"/stream", "")
	c.Params = gin.Params{{Key: "id", Value: queryID}}
	StreamResult(db)(c)
	if !strings.Contains(w.Body.String(), "event:") {
		t.Fatalf("body = %q", w.Body.String())
	}
}

func TestAsPathEnrichmentReady_EnoughEnriched(t *testing.T) {
	if !asPathEnrichmentReady(&domain.QueryResult{
		ASPath:         []uint32{64512, 15169},
		ASPathEnriched: []domain.ASInfo{{ASN: 64512}, {ASN: 15169}},
	}) {
		t.Fatal("expected ready")
	}
}

func TestValidQuickQueryTarget_BGPInvalidCIDR(t *testing.T) {
	if validQuickQueryTarget("bgp_route", "10.0.0.0/99") {
		t.Fatal("invalid cidr should fail")
	}
}

func TestUpdateQuickQuery_ValidationBranches(t *testing.T) {
	db := setupDB(t)
	body := `{"command":"ping","name":"  ","target":"1.1.1.1","active":true}`
	c, w := setupAdminContext(db, http.MethodPut, "/admin/quick-queries/1", body, 1)
	c.Params = gin.Params{{Key: "id", Value: "1"}}
	UpdateQuickQuery(db)(c)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("empty name status = %d", w.Code)
	}
}

func TestUploadLogo_ReadSeekAndSettingsErrors(t *testing.T) {
	db := setupDB(t)
	SetLogoUploadsDir(t.TempDir())

	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	part, _ := writer.CreateFormFile("logo", "logo.png")
	_, _ = part.Write([]byte{0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A})
	writer.Close()
	req := httptest.NewRequest(http.MethodPost, "/admin/settings/logo", body)
	req.Header.Set("Content-Type", writer.FormDataContentType())

	prevRead := uploadLogoReadFn
	uploadLogoReadFn = func(io.Reader, []byte) (int, error) { return 0, errors.New("read fail") }
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = req
	UploadLogo(db)(c)
	uploadLogoReadFn = prevRead
	if w.Code != http.StatusInternalServerError {
		t.Fatalf("read status = %d", w.Code)
	}

	prevSeek := uploadLogoSeekFn
	uploadLogoSeekFn = func(io.Seeker, int64, int) (int64, error) { return 0, errors.New("seek fail") }
	w = httptest.NewRecorder()
	c, _ = gin.CreateTestContext(w)
	c.Request = req
	UploadLogo(db)(c)
	uploadLogoSeekFn = prevSeek
	if w.Code != http.StatusInternalServerError {
		t.Fatalf("seek status = %d", w.Code)
	}

	prevSet := uploadLogoSetSettingFn
	uploadLogoSetSettingFn = func(*sql.DB, string, string) error { return errors.New("settings fail") }
	w = httptest.NewRecorder()
	c, _ = gin.CreateTestContext(w)
	c.Request = req
	UploadLogo(db)(c)
	uploadLogoSetSettingFn = prevSet
	if w.Code != http.StatusInternalServerError {
		t.Fatalf("settings status = %d", w.Code)
	}

	prevRefresh := refreshSettingsCacheFn
	refreshSettingsCacheFn = func(*sql.DB, uint32) error { return errors.New("refresh fail") }
	uploadLogoSetSettingFn = func(*sql.DB, string, string) error { return nil }
	w = httptest.NewRecorder()
	c, _ = gin.CreateTestContext(w)
	c.Request = req
	UploadLogo(db)(c)
	refreshSettingsCacheFn = prevRefresh
	uploadLogoSetSettingFn = prevSet
	if w.Code != http.StatusOK {
		t.Fatalf("refresh warn status = %d", w.Code)
	}
}

func TestUploadLogo_SVGReadAllError(t *testing.T) {
	db := setupDB(t)
	SetLogoUploadsDir(t.TempDir())
	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	part, _ := writer.CreateFormFile("logo", "logo.svg")
	_, _ = io.WriteString(part, `<svg xmlns="http://www.w3.org/2000/svg"><text>ok</text></svg>`)
	writer.Close()
	req := httptest.NewRequest(http.MethodPost, "/admin/settings/logo", body)
	req.Header.Set("Content-Type", writer.FormDataContentType())

	prev := uploadLogoReadAllFn
	uploadLogoReadAllFn = func(io.Reader) ([]byte, error) { return nil, errors.New("read all fail") }
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = req
	UploadLogo(db)(c)
	uploadLogoReadAllFn = prev
	if w.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d", w.Code)
	}
}

func TestUpdateAccount_GenericCredentialsError(t *testing.T) {
	db := setupDB(t)
	adminID := seedAdminPassword(t, db, "password12345")
	prev := updateAccountCredentialsFn
	updateAccountCredentialsFn = func(context.Context, domain.UserRepository, int64, string, string) (*domain.User, error) {
		return nil, errors.New("db exploded")
	}
	t.Cleanup(func() { updateAccountCredentialsFn = prev })

	body := `{"email":"changed@hopstat.local","current_password":"password12345","new_password":"newpassword12"}`
	c, w := setupAdminContext(db, http.MethodPut, "/admin/account", body, adminID)
	UpdateAccount(db)(c)
	if w.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d", w.Code)
	}
}

func TestUpdateCommunityRule_RefreshWarningOnUpdate(t *testing.T) {
	db := setupDB(t)
	createBody := `{"community":"1:1","severity":"info","message_i18n":"x","scope":"global","active":true}`
	c, w := setupAdminContext(db, http.MethodPost, "/admin/community-rules", createBody, 1)
	CreateCommunityRule(db)(c)
	var created struct {
		Data struct { ID int64 `json:"id"` } `json:"data"`
	}
	_ = json.Unmarshal(w.Body.Bytes(), &created)

	prev := refreshCommunitiesCacheFn
	refreshCommunitiesCacheFn = func(*sql.DB) error { return errors.New("refresh fail") }
	t.Cleanup(func() { refreshCommunitiesCacheFn = prev })

	body := fmt.Sprintf(`{"community":"1:1","severity":"info","message_i18n":"updated","scope":"global","active":true}`)
	c, w = setupAdminContext(db, http.MethodPut, fmt.Sprintf("/admin/community-rules/%d", created.Data.ID), body, 1)
	c.Params = gin.Params{{Key: "id", Value: fmt.Sprint(created.Data.ID)}}
	UpdateCommunityRule(db)(c)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d", w.Code)
	}
}

func TestWaitForASPathEnrichment_ZeroWait(t *testing.T) {
	queryID := "zero-wait"
	queryStore.SetRunning(queryID)
	queryStore.MergePartial(queryID, &domain.QueryResult{
		ASPath: []uint32{64512}, ASPathEnriched: []domain.ASInfo{},
	})
	waitForASPathEnrichment(queryID, 0)
}

func TestAsPathEnrichmentReady_AllZeroASNs(t *testing.T) {
	if !asPathEnrichmentReady(&domain.QueryResult{ASPath: []uint32{0, 0, 0}}) {
		t.Fatal("zero ASNs should be ready")
	}
}

func TestAsSuffixForIP_WithOrgName(t *testing.T) {
	g := stubGeo{byIP: map[string]*domain.ASInfo{
		"1.1.1.1": {ASN: 13335, OrgName: "Cloudflare, Inc.", ShortName: ""},
	}}
	suffix := asSuffixForIP(context.Background(), g, "1.1.1.1")
	if !strings.Contains(suffix, "13335") || !strings.Contains(suffix, "Cloudflare") {
		t.Fatalf("suffix = %q", suffix)
	}
}

func TestCreateQuickQuery_SortOrderFailure(t *testing.T) {
	db := setupDB(t)
	if _, err := db.Exec(`DROP TABLE quick_queries`); err != nil {
		t.Fatal(err)
	}
	body := `{"command":"ping","name":"n","target":"1.1.1.1","active":true}`
	c, w := setupAdminContext(db, http.MethodPost, "/admin/quick-queries", body, 1)
	CreateQuickQuery(db)(c)
	if w.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d", w.Code)
	}
}

func TestUpdateQuickQuery_BindAndExistingErrors(t *testing.T) {
	db := setupDB(t)
	c, w := setupAdminContext(db, http.MethodPut, "/admin/quick-queries/1", `{`, 1)
	c.Params = gin.Params{{Key: "id", Value: "1"}}
	UpdateQuickQuery(db)(c)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("bind status = %d", w.Code)
	}

	if _, err := db.Exec(`DROP TABLE quick_queries`); err != nil {
		t.Fatal(err)
	}
	body := `{"command":"ping","name":"n","target":"1.1.1.1","active":true}`
	c, w = setupAdminContext(db, http.MethodPut, "/admin/quick-queries/1", body, 1)
	c.Params = gin.Params{{Key: "id", Value: "1"}}
	UpdateQuickQuery(db)(c)
	if w.Code != http.StatusInternalServerError {
		t.Fatalf("existing status = %d", w.Code)
	}
}

func TestStreamResult_PollTimeout(t *testing.T) {
	db := setupDB(t)
	queryID := "poll-timeout"
	queryStore.SetRunning(queryID)
	queryStore.Set(queryID, &domain.QueryResult{ID: queryID, Status: domain.StatusRunning})

	prevPoll := streamResultPollInterval
	prevMax := streamResultMaxDuration
	streamResultPollInterval = 5 * time.Millisecond
	streamResultMaxDuration = 25 * time.Millisecond
	t.Cleanup(func() {
		streamResultPollInterval = prevPoll
		streamResultMaxDuration = prevMax
	})

	c, w := setupContext(db, http.MethodGet, "/query/"+queryID+"/stream", "")
	c.Params = gin.Params{{Key: "id", Value: queryID}}
	StreamResult(db)(c)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d", w.Code)
	}
}

func TestExportAudit_FlushError(t *testing.T) {
	db := setupDB(t)
	prev := exportAuditFlushFn
	exportAuditFlushFn = func(*csv.Writer) error { return errors.New("flush fail") }
	t.Cleanup(func() { exportAuditFlushFn = prev })

	c, w := setupAdminContext(db, http.MethodGet, "/admin/audit/export", "", 1)
	ExportAudit(db)(c)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d", w.Code)
	}
}

func TestUploadLogo_SVGSecondSeekError(t *testing.T) {
	db := setupDB(t)
	SetLogoUploadsDir(t.TempDir())
	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	part, _ := writer.CreateFormFile("logo", "logo.svg")
	_, _ = io.WriteString(part, `<svg xmlns="http://www.w3.org/2000/svg"><text>ok</text></svg>`)
	writer.Close()
	req := httptest.NewRequest(http.MethodPost, "/admin/settings/logo", body)
	req.Header.Set("Content-Type", writer.FormDataContentType())

	seekN := 0
	prevSeek := uploadLogoSeekFn
	uploadLogoSeekFn = func(s io.Seeker, offset int64, whence int) (int64, error) {
		seekN++
		if seekN == 2 {
			return 0, errors.New("second seek fail")
		}
		return s.Seek(offset, whence)
	}
	t.Cleanup(func() { uploadLogoSeekFn = prevSeek })

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = req
	UploadLogo(db)(c)
	if w.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d", w.Code)
	}
}

func TestUploadLogo_WebPExtension(t *testing.T) {
	db := setupDB(t)
	dir := t.TempDir()
	SetLogoUploadsDir(dir)
	// Minimal 1x1 lossy WebP (RIFF....WEBPVP8 ...)
	webp := []byte{
		0x52, 0x49, 0x46, 0x46, 0x24, 0x00, 0x00, 0x00,
		0x57, 0x45, 0x42, 0x50, 0x56, 0x50, 0x38, 0x20,
		0x0c, 0x00, 0x00, 0x00, 0x30, 0x01, 0x00, 0x9d,
		0x01, 0x2a, 0x01, 0x00, 0x01, 0x00, 0x02, 0x00,
		0x34, 0x25, 0xa4, 0x00, 0x03, 0x70, 0x00, 0xfe,
		0xfb, 0xfd, 0x50, 0x00,
	}
	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	part, _ := writer.CreateFormFile("logo", "logo.webp")
	_, _ = part.Write(webp)
	writer.Close()
	req := httptest.NewRequest(http.MethodPost, "/admin/settings/logo", body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = req
	UploadLogo(db)(c)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d body = %q", w.Code, w.Body.String())
	}
	if _, err := os.Stat(filepath.Join(dir, "logo.webp")); err != nil {
		t.Fatalf("webp file missing: %v", err)
	}
}

func TestWaitForASPathEnrichment_WaitZeroBranch(t *testing.T) {
	queryID := "wait-zero-branch"
	queryStore.SetRunning(queryID)
	queryStore.MergePartial(queryID, &domain.QueryResult{
		ASPath: []uint32{64512}, ASPathEnriched: []domain.ASInfo{},
	})
	prev := asPathEnrichmentTimeUntilFn
	asPathEnrichmentTimeUntilFn = func(time.Time) time.Duration { return 0 }
	t.Cleanup(func() { asPathEnrichmentTimeUntilFn = prev })
	waitForASPathEnrichment(queryID, time.Second)
}

type stubGeoErr struct {
	err error
}

func (s stubGeoErr) ResolveASN(context.Context, string) (*domain.ASInfo, error) {
	return nil, s.err
}

func TestAsSuffixForIP_ResolveErrorAndSanitize(t *testing.T) {
	if suffix := asSuffixForIP(context.Background(), stubGeoErr{err: errors.New("geo fail")}, "8.8.8.8"); suffix != "" {
		t.Fatalf("expected empty suffix on error, got %q", suffix)
	}
	g := stubGeo{byIP: map[string]*domain.ASInfo{
		"8.8.8.8": {ASN: 15169, OrgName: "Go\x01ogle", ShortName: ""},
	}}
	suffix := asSuffixForIP(context.Background(), g, "8.8.8.8")
	if !strings.Contains(suffix, "15169") {
		t.Fatalf("suffix = %q", suffix)
	}
}

func TestValidQuickQueryTarget_BGPBareIP(t *testing.T) {
	if !validQuickQueryTarget("bgp_route", "8.8.8.8") {
		t.Fatal("bare IP should be valid for bgp_route")
	}
}

func TestCreateQuickQuery_RecordError(t *testing.T) {
	db := setupDB(t)
	prev := createQuickQueryRecordFn
	createQuickQueryRecordFn = func(*queries.Queries, context.Context, *queries.QuickQuery) (*queries.QuickQuery, error) {
		return nil, errors.New("create fail")
	}
	t.Cleanup(func() { createQuickQueryRecordFn = prev })

	body := `{"command":"ping","name":"n","target":"1.1.1.1","active":true}`
	c, w := setupAdminContext(db, http.MethodPost, "/admin/quick-queries", body, 1)
	CreateQuickQuery(db)(c)
	if w.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d", w.Code)
	}
}

func TestUpdateQuickQuery_RemainingBranches(t *testing.T) {
	db := setupDB(t)
	createBody := `{"command":"ping","name":"n","target":"1.1.1.1","active":true}`
	c, w := setupAdminContext(db, http.MethodPost, "/admin/quick-queries", createBody, 1)
	CreateQuickQuery(db)(c)
	var created struct {
		Data struct {
			ID int64 `json:"id"`
		} `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &created); err != nil {
		t.Fatal(err)
	}

	c, w = setupAdminContext(db, http.MethodPut, fmt.Sprintf("/admin/quick-queries/%d", created.Data.ID), `{"command":"ping","name":"n","target":"bad-target","active":true}`, 1)
	c.Params = gin.Params{{Key: "id", Value: fmt.Sprint(created.Data.ID)}}
	UpdateQuickQuery(db)(c)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("invalid target status = %d", w.Code)
	}

	badNode := int64(99999)
	c, w = setupAdminContext(db, http.MethodPut, fmt.Sprintf("/admin/quick-queries/%d", created.Data.ID), `{"command":"ping","name":"n","target":"1.1.1.1","node_id":99999,"active":true}`, 1)
	c.Params = gin.Params{{Key: "id", Value: fmt.Sprint(created.Data.ID)}}
	UpdateQuickQuery(db)(c)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("node error status = %d body=%q", w.Code, w.Body.String())
	}
	_ = badNode

	prev := updateQuickQueryRecordFn
	updateQuickQueryRecordFn = func(*queries.Queries, context.Context, *queries.QuickQuery) (*queries.QuickQuery, error) {
		return nil, errors.New("update fail")
	}
	t.Cleanup(func() { updateQuickQueryRecordFn = prev })

	c, w = setupAdminContext(db, http.MethodPut, fmt.Sprintf("/admin/quick-queries/%d", created.Data.ID), `{"command":"ping","name":"n","target":"1.1.1.1","active":true}`, 1)
	c.Params = gin.Params{{Key: "id", Value: fmt.Sprint(created.Data.ID)}}
	UpdateQuickQuery(db)(c)
	if w.Code != http.StatusInternalServerError {
		t.Fatalf("update error status = %d", w.Code)
	}
}
