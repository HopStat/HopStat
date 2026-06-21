package handler

import (
	"bytes"
	"context"
	"encoding/json"
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
	"github.com/HopStat/HopStat/internal/geo"
	"github.com/HopStat/HopStat/internal/hostips"
	"github.com/HopStat/HopStat/internal/sitecache"
	"github.com/HopStat/HopStat/internal/store"
	"github.com/HopStat/HopStat/internal/store/queries"
	"github.com/HopStat/HopStat/internal/store/repo"
	"github.com/HopStat/HopStat/internal/updater"
)

func TestHoststatsCollectorSnapshot(t *testing.T) {
	var c hoststatsCollector
	if _, err := c.Snapshot(context.Background()); err != nil {
		t.Fatalf("snapshot: %v", err)
	}
}

func TestSystemAddresses_Error(t *testing.T) {
	prev := systemAddressLister
	systemAddressLister = func() ([]hostips.Address, []hostips.Address, error) {
		return nil, nil, fmt.Errorf("list failed")
	}
	t.Cleanup(func() { systemAddressLister = prev })

	c, w := setupAdminContext(nil, http.MethodGet, "/admin/system/addresses", "", 1)
	SystemAddresses()(c)

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d", w.Code)
	}
}

func TestWaitForASPathEnrichment_Timeout(t *testing.T) {
	queryID := "aspath-timeout"
	queryStore.SetRunning(queryID)
	queryStore.MergePartial(queryID, &domain.QueryResult{
		Status:         domain.StatusRunning,
		ASPath:         []uint32{64512, 15169},
		ASPathEnriched: []domain.ASInfo{{ASN: 64512}},
	})
	waitForASPathEnrichment(queryID, 50*time.Millisecond)
}

func TestSubmitQuery_BGPRoute(t *testing.T) {
	db := setupDB(t)
	cfg := testConfig()
	g := geo.New("", "")

	nodeRepo := repo.NewNodeRepo(db, "")
	created, _ := nodeRepo.Create(context.Background(), &domain.Node{
		Name: "n", Type: domain.NodeTypeStandalone, Active: true,
		EnabledCmds: domain.DefaultEnabledCmds(), AgentToken: "tok",
	})
	refreshTestSiteCache(t, db)

	body := fmt.Sprintf(`{"node_id":%d,"command":"bgp_route","target":"8.8.8.8"}`, created.ID)
	c, w := setupContext(db, http.MethodPost, "/query", body)
	SubmitQuery(db, cfg, g, nil)(c)
	if w.Code != http.StatusOK {
		t.Fatalf("bgp status = %d, body = %s", w.Code, w.Body.String())
	}
}

func TestStreamResult_ContextCancel(t *testing.T) {
	db := setupDB(t)
	queryID := "cancel-stream"
	queryStore.SetRunning(queryID)

	ctx, cancel := context.WithCancel(context.Background())
	c, _ := setupContext(db, http.MethodGet, "/query/"+queryID+"/stream", "")
	c.Params = gin.Params{{Key: "id", Value: queryID}}
	c.Request = c.Request.WithContext(ctx)

	done := make(chan struct{})
	go func() {
		StreamResult(db)(c)
		close(done)
	}()
	time.Sleep(20 * time.Millisecond)
	cancel()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("stream did not exit on cancel")
	}
}

func TestLogin_UpdatesLastLogin(t *testing.T) {
	db := setupDB(t)
	cfg := testConfig()
	password := "securepassword123"
	adminID := seedAdminPassword(t, db, password)

	body := fmt.Sprintf(`{"email":%q,"password":%q}`, store.DefaultAdminEmail, password)
	c, w := setupContext(db, http.MethodPost, "/auth/login", body)
	Login(db, cfg)(c)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d", w.Code)
	}

	time.Sleep(100 * time.Millisecond)
	userRepo := repo.NewUserRepo(db)
	user, err := userRepo.GetByID(context.Background(), adminID)
	if err != nil || user == nil {
		t.Fatalf("get user: %v", err)
	}
	if user.LastLogin == nil {
		t.Fatal("expected last login updated")
	}
}

func TestUpdateAccount_EmailConflict(t *testing.T) {
	db := setupDB(t)
	adminID := seedAdminPassword(t, db, "password12345")
	if _, err := db.Exec(`INSERT INTO users (email, password_hash, created_at) VALUES (?, ?, datetime('now'))`,
		"other@hopstat.local", "hash"); err != nil {
		t.Fatal(err)
	}

	body := `{"email":"other@hopstat.local","current_password":"password12345"}`
	c, w := setupAdminContext(db, http.MethodPut, "/admin/account", body, adminID)
	UpdateAccount(db)(c)
	if w.Code != http.StatusConflict {
		t.Fatalf("status = %d, body = %s", w.Code, w.Body.String())
	}
}

func TestMyIP_WithMaxMindPaths(t *testing.T) {
	asnPath := filepath.Join("..", "..", "geo", "testdata", "GeoLite2-ASN-Test.mmdb")
	cityPath := filepath.Join("..", "..", "geo", "testdata", "GeoLite2-City-Test.mmdb")
	if _, err := os.Stat(asnPath); err != nil {
		t.Skip("geo test databases not available")
	}
	g := geo.New(asnPath, cityPath)
	if !g.Enabled() {
		t.Skip("geo db not enabled")
	}

	c, w := setupContext(nil, http.MethodGet, "/myip", "")
	c.Request.RemoteAddr = "8.8.8.8:1234"
	MyIP(g)(c)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d", w.Code)
	}
}

func TestListCommunityRules_ClosedDB(t *testing.T) {
	db := setupDB(t)
	db.Close()
	c, w := setupAdminContext(db, http.MethodGet, "/admin/community-rules", "", 1)
	ListCommunityRules(db)(c)
	if w.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d", w.Code)
	}
}

func TestUpdateCommunityRule_InvalidScope(t *testing.T) {
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

	body := `{"community":"1:1","severity":"info","message_i18n":"x","scope":"bad","active":true}`
	c, w = setupAdminContext(db, http.MethodPut, fmt.Sprintf("/admin/community-rules/%d", created.Data.ID), body, 1)
	c.Params = gin.Params{{Key: "id", Value: fmt.Sprint(created.Data.ID)}}
	UpdateCommunityRule(db)(c)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d", w.Code)
	}
}

func TestDeleteCommunityRule_ClosedDB(t *testing.T) {
	db := setupDB(t)
	db.Close()
	c, w := setupAdminContext(db, http.MethodDelete, "/admin/community-rules/1", "", 1)
	c.Params = gin.Params{{Key: "id", Value: "1"}}
	DeleteCommunityRule(db)(c)
	if w.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d", w.Code)
	}
}

func TestToggleCommunityRule_ClosedDB(t *testing.T) {
	db := setupDB(t)
	db.Close()
	c, w := setupAdminContext(db, http.MethodPatch, "/admin/community-rules/1/toggle", "", 1)
	c.Params = gin.Params{{Key: "id", Value: "1"}}
	ToggleCommunityRule(db)(c)
	if w.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d", w.Code)
	}
}

func TestUploadLogo_SVGSuccess(t *testing.T) {
	db := setupDB(t)
	uploadDir := t.TempDir()
	SetLogoUploadsDir(uploadDir)

	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	part, _ := writer.CreateFormFile("logo", "logo.svg")
	_, _ = io.WriteString(part, `<svg xmlns="http://www.w3.org/2000/svg"><rect width="10" height="10"/></svg>`)
	writer.Close()

	req := httptest.NewRequest(http.MethodPost, "/admin/settings/logo", body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = req

	UploadLogo(db)(c)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", w.Code, w.Body.String())
	}
}

func TestUploadLogo_JPEG(t *testing.T) {
	db := setupDB(t)
	uploadDir := t.TempDir()
	SetLogoUploadsDir(uploadDir)

	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	part, _ := writer.CreateFormFile("logo", "logo.jpg")
	_, _ = part.Write([]byte{0xFF, 0xD8, 0xFF, 0xE0, 0, 0x10, 'J', 'F', 'I', 'F', 0})
	writer.Close()

	req := httptest.NewRequest(http.MethodPost, "/admin/settings/logo", body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = req

	UploadLogo(db)(c)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", w.Code, w.Body.String())
	}
}

func TestQuickQuery_WithNodeID(t *testing.T) {
	db := setupDB(t)
	nodeRepo := repo.NewNodeRepo(db, "")
	created, _ := nodeRepo.Create(context.Background(), &domain.Node{
		Name: "n", Type: domain.NodeTypeStandalone, Active: true,
		EnabledCmds: domain.DefaultEnabledCmds(),
	})

	body := fmt.Sprintf(`{"command":"ping","name":"local","target":"1.1.1.1","node_id":%d,"active":true}`, created.ID)
	c, w := setupAdminContext(db, http.MethodPost, "/admin/quick-queries", body, 1)
	CreateQuickQuery(db)(c)
	if w.Code != http.StatusCreated {
		t.Fatalf("status = %d, body = %s", w.Code, w.Body.String())
	}

	body = `{"command":"ping","name":"x","target":"1.1.1.1","node_id":9999,"active":true}`
	c, w = setupAdminContext(db, http.MethodPost, "/admin/quick-queries", body, 1)
	CreateQuickQuery(db)(c)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("missing node status = %d", w.Code)
	}
}

func TestUpdateQuickQuery_NotFound(t *testing.T) {
	db := setupDB(t)
	body := `{"command":"ping","name":"x","target":"1.1.1.1","active":true}`
	c, w := setupAdminContext(db, http.MethodPut, "/admin/quick-queries/9999", body, 1)
	c.Params = gin.Params{{Key: "id", Value: "9999"}}
	UpdateQuickQuery(db)(c)
	if w.Code != http.StatusNotFound {
		t.Fatalf("status = %d", w.Code)
	}
}

func TestToggleQuickQuery_InvalidID(t *testing.T) {
	db := setupDB(t)
	c, w := setupAdminContext(db, http.MethodPatch, "/admin/quick-queries/0/toggle", "", 1)
	c.Params = gin.Params{{Key: "id", Value: "0"}}
	ToggleQuickQuery(db)(c)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d", w.Code)
	}
}

func TestUpdateApply_SelfUpdateDisabledOrAccepted(t *testing.T) {
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
		t.Fatalf("status = %d, body = %s", w.Code, w.Body.String())
	}
}

func TestAgentCapabilities_NoNode(t *testing.T) {
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/agent/v1/capabilities", nil)
	agentCapabilities(c)
	if w.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d", w.Code)
	}
}

func TestAgentTraceroute_InvalidJSON(t *testing.T) {
	db := setupDB(t)
	r := setupAgentRouter(t, db, &domain.Node{
		Name: "local", Type: domain.NodeTypeStandalone, AgentToken: "node-secret",
		EnabledCmds: []domain.CommandType{domain.CmdTraceroute}, Active: true,
	})
	req := httptest.NewRequest(http.MethodPost, "/agent/v1/traceroute", strings.NewReader(`{`))
	req.Header.Set("Authorization", "Bearer node-secret")
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d", w.Code)
	}
}

func TestAgentPing_CountClamp(t *testing.T) {
	db := setupDB(t)
	r := setupAgentRouter(t, db, &domain.Node{
		Name: "local", Type: domain.NodeTypeStandalone, AgentToken: "node-secret",
		EnabledCmds: []domain.CommandType{domain.CmdPing}, Active: true,
	})
	body := `{"target":"127.0.0.1","count":100}`
	req := httptest.NewRequest(http.MethodPost, "/agent/v1/ping", strings.NewReader(body))
	req.Header.Set("Authorization", "Bearer node-secret")
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK && w.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d", w.Code)
	}
}

func TestCreateBGPNeighbor_WithManager(t *testing.T) {
	db := setupDB(t)
	nodeRepo := repo.NewNodeRepo(db, "")
	created, _ := nodeRepo.Create(context.Background(), &domain.Node{
		Name: "n", Type: domain.NodeTypeStandalone, Active: true,
		EnabledCmds: domain.DefaultEnabledCmds(),
	})

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	bgpCfg := config.BGPConfig{LocalAS: 65000, RouterID: "10.0.0.1", ListenPort: freeListenPort(t)}
	mgr := bgp.NewSessionManager(bgpCfg)
	if err := mgr.Start(ctx); err != nil {
		t.Fatal(err)
	}
	defer mgr.Stop()

	body := fmt.Sprintf(`{"node_id":%d,"remote_as":174,"peering_ip":"10.0.0.1","neighbor_ip":"10.0.0.2"}`, created.ID)
	c, w := setupAdminContext(db, http.MethodPost, "/admin/bgp-neighbors", body, 1)
	CreateBGPNeighbor(db, mgr, bgpCfg)(c)
	if w.Code != http.StatusCreated {
		t.Fatalf("status = %d, body = %s", w.Code, w.Body.String())
	}
}

func TestRemoveBGPNeighborsForNode_NilManager(t *testing.T) {
	if err := removeBGPNeighborsForNode(context.Background(), setupDB(t), nil, 1); err != nil {
		t.Fatal(err)
	}
}

func TestSyncBGPNeighbor_Nil(t *testing.T) {
	if err := syncBGPNeighbor(nil, "noop", func() error { return fmt.Errorf("fail") }); err != nil {
		t.Fatal("nil mgr should skip")
	}
}

func TestDistinctPublicIPsPrivateSkipped(t *testing.T) {
	ips := distinctPublicIPs([]string{"127.0.0.1 (127.0.0.1) 1 ms"})
	if len(ips) != 0 {
		t.Fatalf("got %v", ips)
	}
}

func TestAsSuffixForIP_WithOrg(t *testing.T) {
	g := stubGeo{byIP: map[string]*domain.ASInfo{
		"8.8.8.8": {ASN: 15169, OrgName: "Google\nLLC"},
	}}
	suffix := asSuffixForIP(context.Background(), g, "8.8.8.8")
	if !strings.Contains(suffix, "Google") || strings.Contains(suffix, "\n") {
		t.Fatalf("suffix = %q", suffix)
	}
}

func TestEnrichProbeSegmentNoIP(t *testing.T) {
	got := enrichProbeSegment(context.Background(), stubGeo{}, "* * *")
	if got != "* * *" {
		t.Fatalf("got %q", got)
	}
}

func TestSubmitQuery_TracerouteTimeoutStop(t *testing.T) {
	db := setupDB(t)
	cfg := testConfig()
	q := queries.New(db)
	_ = q.SetSetting("traceroute_max_timeouts", "1")
	_ = sitecache.RefreshSettings(db, 0)

	nodeRepo := repo.NewNodeRepo(db, "")
	created, _ := nodeRepo.Create(context.Background(), &domain.Node{
		Name: "n", Type: domain.NodeTypeStandalone, Active: true,
		EnabledCmds: domain.DefaultEnabledCmds(), AgentToken: "tok",
	})
	refreshTestSiteCache(t, db)

	body := fmt.Sprintf(`{"node_id":%d,"command":"traceroute","target":"8.8.8.8","options":{"max_hops":2}}`, created.ID)
	c, w := setupContext(db, http.MethodPost, "/query", body)
	SubmitQuery(db, cfg, geo.New("", ""), nil)(c)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d", w.Code)
	}
}

func TestUpdateNode_InvalidAgentURL(t *testing.T) {
	db := setupDB(t)
	nodeRepo := repo.NewNodeRepo(db, "")
	created, _ := nodeRepo.Create(context.Background(), &domain.Node{
		Name: "lg", Type: domain.NodeTypeLGNode, Active: true,
		EnabledCmds: domain.DefaultEnabledCmds(), AgentToken: "tok",
	})
	body := `{"type":"lg_node","agent_url":"http://10.0.0.1:8080"}`
	c, w := setupAdminContext(db, http.MethodPut, fmt.Sprintf("/admin/nodes/%d", created.ID), body, 1)
	c.Params = gin.Params{{Key: "id", Value: fmt.Sprint(created.ID)}}
	UpdateNode(db, "")(c)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d", w.Code)
	}
}

func TestCreateCommunityRule_DefaultSeverity(t *testing.T) {
	db := setupDB(t)
	body := `{"community":"1:2","message_i18n":"msg","active":true}`
	c, w := setupAdminContext(db, http.MethodPost, "/admin/community-rules", body, 1)
	CreateCommunityRule(db)(c)
	if w.Code != http.StatusCreated {
		t.Fatalf("status = %d, body = %s", w.Code, w.Body.String())
	}
}

func TestValidQuickQueryTarget_CIDRInvalid(t *testing.T) {
	if validQuickQueryTarget("bgp_route", "1.2.3.4/99") {
		t.Fatal("expected invalid cidr")
	}
}

func TestQuickQueryNodeIDFromRequest_DBError(t *testing.T) {
	db := setupDB(t)
	db.Close()
	nodeID := int64(1)
	c, _ := setupContext(db, http.MethodPost, "/", "")
	_, msg := quickQueryNodeIDFromRequest(db, c, &nodeID)
	if msg == "" {
		t.Fatal("expected error message")
	}
}
