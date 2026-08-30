package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/HopStat/HopStat/internal/domain"
	"github.com/HopStat/HopStat/internal/geo"
	"github.com/HopStat/HopStat/internal/server/middleware"
	"github.com/HopStat/HopStat/internal/store"
	"github.com/HopStat/HopStat/internal/store/repo"
	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
)

func TestSanitizeError(t *testing.T) {
	if got := sanitizeError(errors.New("db boom")); got != "internal error" {
		t.Fatalf("got %q", got)
	}
}

func TestFriendlyConnErrorBranches(t *testing.T) {
	cases := map[string]string{
		"context deadline exceeded": "connection timed out",
		"connection refused":        "connection refused",
		"no route to host":          "no route to host",
	}
	for msg, wantPrefix := range cases {
		got := friendlyConnError(errors.New(msg))
		if !strings.HasPrefix(got, wantPrefix) {
			t.Fatalf("msg %q => %q, want prefix %q", msg, got, wantPrefix)
		}
	}
	if friendlyConnError(nil) != "" {
		t.Fatal("nil should be empty")
	}
}

func TestGetResult_MissingID(t *testing.T) {
	db := setupDB(t)
	c, w := setupContext(db, http.MethodGet, "/query/", "")
	c.Params = gin.Params{{Key: "id", Value: ""}}

	GetResult(db)(c)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d", w.Code)
	}
}

func TestStreamResult_NotFound(t *testing.T) {
	db := setupDB(t)
	c, w := setupContext(db, http.MethodGet, "/query/missing/stream", "")
	c.Params = gin.Params{{Key: "id", Value: "missing"}}

	StreamResult(db)(c)

	if w.Code != http.StatusNotFound {
		t.Fatalf("status = %d", w.Code)
	}
}

func TestStreamResult_MissingID(t *testing.T) {
	db := setupDB(t)
	c, w := setupContext(db, http.MethodGet, "/query//stream", "")
	c.Params = gin.Params{{Key: "id", Value: ""}}

	StreamResult(db)(c)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d", w.Code)
	}
}

func TestStreamResult_Completes(t *testing.T) {
	db := setupDB(t)
	queryID := "stream-test-id"
	queryStore.SetRunning(queryID)
	queryStore.AppendLine(queryID, "line one")
	queryStore.MarkOutputComplete(queryID)
	queryStore.Set(queryID, &domain.QueryResult{
		ID:     queryID,
		Status: domain.StatusDone,
		Raw:    "done",
	})

	c, w := setupContext(db, http.MethodGet, "/query/"+queryID+"/stream", "")
	c.Params = gin.Params{{Key: "id", Value: queryID}}

	StreamResult(db)(c)

	body := w.Body.String()
	if !strings.Contains(body, "event: output") {
		t.Fatalf("expected output event, got %q", body)
	}
	if !strings.Contains(body, "event: output_done") {
		t.Fatalf("expected output_done event, got %q", body)
	}
	if !strings.Contains(body, "event: result") {
		t.Fatalf("expected result event, got %q", body)
	}
}

func TestStreamResult_PartialWhileRunning(t *testing.T) {
	db := setupDB(t)
	queryID := "partial-stream"
	queryStore.SetRunning(queryID)
	queryStore.MergePartial(queryID, &domain.QueryResult{
		Status: domain.StatusRunning,
		ASPath: []uint32{64512, 15169},
		Parsed: &domain.BGPResult{},
	})

	done := make(chan struct{})
	go func() {
		defer close(done)
		time.Sleep(50 * time.Millisecond)
		queryStore.MarkOutputComplete(queryID)
		queryStore.Set(queryID, &domain.QueryResult{ID: queryID, Status: domain.StatusDone})
	}()

	c, w := setupContext(db, http.MethodGet, "/query/"+queryID+"/stream", "")
	c.Params = gin.Params{{Key: "id", Value: queryID}}
	c.Request = c.Request.WithContext(context.Background())

	StreamResult(db)(c)
	<-done

	if !strings.Contains(w.Body.String(), "event: partial") {
		t.Fatalf("expected partial event, got %q", w.Body.String())
	}
}

func TestLogout_WithJTI(t *testing.T) {
	cfg := testConfig()
	denyList := middleware.NewJTIDenyList()
	c, w := setupContext(nil, http.MethodPost, "/auth/logout", "")
	c.Set("jti", "revoke-me")
	c.Set("token_exp", time.Now().Add(time.Hour))

	Logout(cfg, denyList)(c)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d", w.Code)
	}
}

func TestLogout_WithValidBearer(t *testing.T) {
	cfg := testConfig()
	denyList := middleware.NewJTIDenyList()
	token, err := generateJWT(1, "admin", cfg.Security.JWTSecret)
	if err != nil {
		t.Fatal(err)
	}

	c, w := setupContext(nil, http.MethodPost, "/auth/logout", "")
	c.Request.Header.Set("Authorization", "Bearer "+token)

	Logout(cfg, denyList)(c)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d", w.Code)
	}
}

func TestLogout_InvalidBearerSigningMethod(t *testing.T) {
	cfg := testConfig()
	denyList := middleware.NewJTIDenyList()
	token := jwt.NewWithClaims(jwt.SigningMethodNone, jwt.MapClaims{"jti": "x"})
	tokenStr, _ := token.SignedString(jwt.UnsafeAllowNoneSignatureType)

	c, w := setupContext(nil, http.MethodPost, "/auth/logout", "")
	c.Request.Header.Set("Authorization", "Bearer "+tokenStr)

	Logout(cfg, denyList)(c)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d", w.Code)
	}
}

func TestGetAccount_Unauthorized(t *testing.T) {
	db := setupDB(t)
	c, w := setupContext(db, http.MethodGet, "/admin/account", "")

	GetAccount(db)(c)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d", w.Code)
	}

	c, w = setupAdminContext(db, http.MethodGet, "/admin/account", "", 1)
	c.Set("user_id", "bad")
	GetAccount(db)(c)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("bad user_id status = %d", w.Code)
	}
}

func TestGetAccount_NotFound(t *testing.T) {
	db := setupDB(t)
	c, w := setupAdminContext(db, http.MethodGet, "/admin/account", "", 9999)

	GetAccount(db)(c)

	if w.Code != http.StatusNotFound {
		t.Fatalf("status = %d", w.Code)
	}
}

func TestUpdateAccount_ValidationErrors(t *testing.T) {
	db := setupDB(t)
	adminID := seedAdminPassword(t, db, "password12345")

	c, w := setupAdminContext(db, http.MethodPut, "/admin/account", `{bad`, adminID)
	UpdateAccount(db)(c)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("invalid json status = %d", w.Code)
	}

	c, w = setupAdminContext(db, http.MethodPut, "/admin/account", `{"email":"not-an-email","current_password":"password12345"}`, adminID)
	UpdateAccount(db)(c)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("invalid email status = %d", w.Code)
	}

	c, w = setupAdminContext(db, http.MethodPut, "/admin/account", `{"email":"`+store.DefaultAdminEmail+`","current_password":"password12345"}`, adminID)
	UpdateAccount(db)(c)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("no changes status = %d", w.Code)
	}
}

func TestUpdateAccount_EmailChange(t *testing.T) {
	db := setupDB(t)
	adminID := seedAdminPassword(t, db, "password12345")
	newEmail := "admin2@hopstat.local"
	body := fmt.Sprintf(`{"email":%q,"current_password":"password12345"}`, newEmail)
	c, w := setupAdminContext(db, http.MethodPut, "/admin/account", body, adminID)

	UpdateAccount(db)(c)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", w.Code, w.Body.String())
	}
}

func TestUpdateAccount_Unauthorized(t *testing.T) {
	db := setupDB(t)
	c, w := setupContext(db, http.MethodPut, "/admin/account", `{}`)

	UpdateAccount(db)(c)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d", w.Code)
	}
}

func TestListAllNodes_ClosedDB(t *testing.T) {
	db := setupDB(t)
	db.Close()
	c, w := setupAdminContext(db, http.MethodGet, "/admin/nodes", "", 1)

	ListAllNodes(db, "")(c)

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, body = %s", w.Code, w.Body.String())
	}
}

func TestCreateNode_LGNodeSuccess(t *testing.T) {
	db := setupDB(t)
	body := `{"name":"remote","type":"lg_node","agent_url":"http://127.0.0.1:8080","agent_token":"secret","enabled_cmds":["ping"],"active":true}`
	c, w := setupAdminContext(db, http.MethodPost, "/admin/nodes", body, 1)

	CreateNode(db, "")(c)

	if w.Code != http.StatusCreated {
		t.Fatalf("status = %d, body = %s", w.Code, w.Body.String())
	}
}

func TestCreateNode_InvalidJSON(t *testing.T) {
	db := setupDB(t)
	c, w := setupAdminContext(db, http.MethodPost, "/admin/nodes", `{`, 1)

	CreateNode(db, "")(c)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d", w.Code)
	}
}

func TestUpdateNode_ValidationAndNotFound(t *testing.T) {
	db := setupDB(t)

	c, w := setupAdminContext(db, http.MethodPut, "/admin/nodes/abc", `{}`, 1)
	c.Params = gin.Params{{Key: "id", Value: "abc"}}
	UpdateNode(db, "")(c)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("invalid id status = %d", w.Code)
	}

	c, w = setupAdminContext(db, http.MethodPut, "/admin/nodes/999", `{}`, 1)
	c.Params = gin.Params{{Key: "id", Value: "999"}}
	UpdateNode(db, "")(c)
	if w.Code != http.StatusNotFound {
		t.Fatalf("not found status = %d", w.Code)
	}

	nodeRepo := repo.NewNodeRepo(db, "")
	created, _ := nodeRepo.Create(context.Background(), &domain.Node{
		Name: "n", Type: domain.NodeTypeStandalone, Active: true,
		EnabledCmds: domain.DefaultEnabledCmds(), AgentToken: "tok",
	})

	body := `{"type":"bad-type"}`
	c, w = setupAdminContext(db, http.MethodPut, fmt.Sprintf("/admin/nodes/%d", created.ID), body, 1)
	c.Params = gin.Params{{Key: "id", Value: fmt.Sprint(created.ID)}}
	UpdateNode(db, "")(c)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("bad type status = %d", w.Code)
	}

	body = `{"enabled_cmds":["not-a-command"]}`
	c, w = setupAdminContext(db, http.MethodPut, fmt.Sprintf("/admin/nodes/%d", created.ID), body, 1)
	c.Params = gin.Params{{Key: "id", Value: fmt.Sprint(created.ID)}}
	UpdateNode(db, "")(c)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("bad cmds status = %d", w.Code)
	}
}

func TestUpdateNode_FullFields(t *testing.T) {
	db := setupDB(t)
	nodeRepo := repo.NewNodeRepo(db, "")
	active := true
	lat, lon := 41.0, 29.0
	created, _ := nodeRepo.Create(context.Background(), &domain.Node{
		Name: "n", Type: domain.NodeTypeStandalone, Active: active,
		EnabledCmds: domain.DefaultEnabledCmds(), AgentToken: "tok",
	})

	body := fmt.Sprintf(`{"name":"updated","description":"desc","city":"Istanbul","country":"TR","lat":%v,"lon":%v,"active":false,"enabled_cmds":["ping","traceroute"],"agent_token":"newtok"}`, lat, lon)
	c, w := setupAdminContext(db, http.MethodPut, fmt.Sprintf("/admin/nodes/%d", created.ID), body, 1)
	c.Params = gin.Params{{Key: "id", Value: fmt.Sprint(created.ID)}}

	UpdateNode(db, "")(c)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", w.Code, w.Body.String())
	}
}

func TestSetDefaultNode_Errors(t *testing.T) {
	db := setupDB(t)

	c, w := setupAdminContext(db, http.MethodPost, "/admin/nodes/x/default", "", 1)
	c.Params = gin.Params{{Key: "id", Value: "x"}}
	SetDefaultNode(db, "")(c)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("invalid id status = %d", w.Code)
	}

	c, w = setupAdminContext(db, http.MethodPost, "/admin/nodes/999/default", "", 1)
	c.Params = gin.Params{{Key: "id", Value: "999"}}
	SetDefaultNode(db, "")(c)
	if w.Code != http.StatusNotFound {
		t.Fatalf("not found status = %d", w.Code)
	}
}

func TestDeleteNode_InvalidID(t *testing.T) {
	db := setupDB(t)
	c, w := setupAdminContext(db, http.MethodDelete, "/admin/nodes/x", "", 1)
	c.Params = gin.Params{{Key: "id", Value: "x"}}

	DeleteNode(db, "", nil)(c)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d", w.Code)
	}
}

func TestTestNode_Errors(t *testing.T) {
	db := setupDB(t)

	c, w := setupAdminContext(db, http.MethodPost, "/admin/nodes/x/test", "", 1)
	c.Params = gin.Params{{Key: "id", Value: "x"}}
	TestNode(db, "", testConfig())(c)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("invalid id status = %d", w.Code)
	}

	c, w = setupAdminContext(db, http.MethodPost, "/admin/nodes/999/test", "", 1)
	c.Params = gin.Params{{Key: "id", Value: "999"}}
	TestNode(db, "", testConfig())(c)
	if w.Code != http.StatusNotFound {
		t.Fatalf("not found status = %d", w.Code)
	}

	nodeRepo := repo.NewNodeRepo(db, "")
	standalone, _ := nodeRepo.Create(context.Background(), &domain.Node{
		Name: "local", Type: domain.NodeTypeStandalone, Active: true,
		EnabledCmds: domain.DefaultEnabledCmds(),
	})
	c, w = setupAdminContext(db, http.MethodPost, fmt.Sprintf("/admin/nodes/%d/test", standalone.ID), "", 1)
	c.Params = gin.Params{{Key: "id", Value: fmt.Sprint(standalone.ID)}}
	TestNode(db, "", testConfig())(c)
	if w.Code != http.StatusOK {
		t.Fatalf("missing token status = %d", w.Code)
	}
	var body struct {
		Data struct {
			Status string `json:"status"`
		} `json:"data"`
	}
	_ = json.Unmarshal(w.Body.Bytes(), &body)
	if body.Data.Status != "error" {
		t.Fatalf("status = %q, want error", body.Data.Status)
	}

	lgNoURL, _ := nodeRepo.Create(context.Background(), &domain.Node{
		Name: "lg", Type: domain.NodeTypeLGNode, Active: true,
		EnabledCmds: domain.DefaultEnabledCmds(), AgentToken: "tok",
	})
	c, w = setupAdminContext(db, http.MethodPost, fmt.Sprintf("/admin/nodes/%d/test", lgNoURL.ID), "", 1)
	c.Params = gin.Params{{Key: "id", Value: fmt.Sprint(lgNoURL.ID)}}
	TestNode(db, "", testConfig())(c)
	_ = json.Unmarshal(w.Body.Bytes(), &body)
	if body.Data.Status != "error" {
		t.Fatalf("lg missing url status = %q", body.Data.Status)
	}

	lgNoTok, _ := nodeRepo.Create(context.Background(), &domain.Node{
		Name: "lg2", Type: domain.NodeTypeLGNode, Active: true,
		EnabledCmds: domain.DefaultEnabledCmds(), AgentURL: "https://agent.example.com",
	})
	c, w = setupAdminContext(db, http.MethodPost, fmt.Sprintf("/admin/nodes/%d/test", lgNoTok.ID), "", 1)
	c.Params = gin.Params{{Key: "id", Value: fmt.Sprint(lgNoTok.ID)}}
	TestNode(db, "", testConfig())(c)
	_ = json.Unmarshal(w.Body.Bytes(), &body)
	if body.Data.Status != "error" {
		t.Fatalf("lg missing token status = %q", body.Data.Status)
	}
}

func TestTestNode_StandaloneSuccess(t *testing.T) {
	db := setupDB(t)
	nodeRepo := repo.NewNodeRepo(db, "")
	created, _ := nodeRepo.Create(context.Background(), &domain.Node{
		Name: "local", Type: domain.NodeTypeStandalone, Active: true,
		EnabledCmds: domain.DefaultEnabledCmds(), AgentToken: "secret",
	})

	c, w := setupAdminContext(db, http.MethodPost, fmt.Sprintf("/admin/nodes/%d/test", created.ID), "", 1)
	c.Params = gin.Params{{Key: "id", Value: fmt.Sprint(created.ID)}}
	TestNode(db, "", testConfig())(c)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", w.Code, w.Body.String())
	}
}

func TestListAudit_WithFilters(t *testing.T) {
	db := setupDB(t)
	nodeRepo := repo.NewNodeRepo(db, "")
	created, _ := nodeRepo.Create(context.Background(), &domain.Node{
		Name: "n", Type: domain.NodeTypeStandalone, Active: true,
		EnabledCmds: domain.DefaultEnabledCmds(),
	})
	auditRepo := repo.NewAuditRepo(db)
	if err := auditRepo.Log(context.Background(), &domain.AuditEntry{
		NodeID:   &created.ID,
		Command:  "ping",
		Params:   "=8.8.8.8",
		SourceIP: "1.2.3.4",
		Success:  true,
	}); err != nil {
		t.Fatal(err)
	}

	path := fmt.Sprintf("/admin/audit?node_id=%d&command=ping&source_ip=1.2.3.4&limit=10&page=0", created.ID)
	c, w := setupAdminContext(db, http.MethodGet, path, "", 1)
	ListAudit(db)(c)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", w.Code, w.Body.String())
	}
}

func TestListAudit_ClosedDB(t *testing.T) {
	db := setupDB(t)
	db.Close()
	c, w := setupAdminContext(db, http.MethodGet, "/admin/audit", "", 1)

	ListAudit(db)(c)

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d", w.Code)
	}
}

func TestExportAudit_FormulaInjection(t *testing.T) {
	db := setupDB(t)
	auditRepo := repo.NewAuditRepo(db)
	if err := auditRepo.Log(context.Background(), &domain.AuditEntry{
		Command:  "ping",
		Params:   "=cmd|'/c calc'!A0",
		ErrorMsg: "+evil",
		Success:  false,
	}); err != nil {
		t.Fatal(err)
	}

	c, w := setupAdminContext(db, http.MethodGet, "/admin/audit/export", "", 1)
	ExportAudit(db)(c)

	csv := w.Body.String()
	if !strings.Contains(csv, "'=cmd") {
		t.Fatalf("expected escaped formula, got %q", csv)
	}
}

func TestCommunityRules_ValidationErrors(t *testing.T) {
	db := setupDB(t)

	c, w := setupAdminContext(db, http.MethodPost, "/admin/community-rules", `{`, 1)
	CreateCommunityRule(db)(c)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("create json status = %d", w.Code)
	}

	body := `{"community":"1:1","severity":"info","scope":"bad"}`
	c, w = setupAdminContext(db, http.MethodPost, "/admin/community-rules", body, 1)
	CreateCommunityRule(db)(c)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("create scope status = %d", w.Code)
	}

	c, w = setupAdminContext(db, http.MethodPut, "/admin/community-rules/x", body, 1)
	c.Params = gin.Params{{Key: "id", Value: "x"}}
	UpdateCommunityRule(db)(c)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("update invalid id status = %d", w.Code)
	}

	c, w = setupAdminContext(db, http.MethodDelete, "/admin/community-rules/x", "", 1)
	c.Params = gin.Params{{Key: "id", Value: "x"}}
	DeleteCommunityRule(db)(c)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("delete invalid id status = %d", w.Code)
	}

	c, w = setupAdminContext(db, http.MethodPatch, "/admin/community-rules/x/toggle", "", 1)
	c.Params = gin.Params{{Key: "id", Value: "x"}}
	ToggleCommunityRule(db)(c)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("toggle invalid id status = %d", w.Code)
	}
}

func TestGetAdminSettings_ClosedDB(t *testing.T) {
	db := setupDB(t)
	db.Close()
	c, w := setupAdminContext(db, http.MethodGet, "/admin/settings", "", 1)

	GetAdminSettings(db)(c)

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d", w.Code)
	}
}

func TestUpdateSettings_ClearsLogo(t *testing.T) {
	db := setupDB(t)
	uploadDir := t.TempDir()
	SetLogoUploadsDir(uploadDir)
	t.Cleanup(func() { SetLogoUploadsDir("") })

	logoPath := uploadDir + "/logo.png"
	if err := os.WriteFile(logoPath, []byte("png"), 0o644); err != nil {
		t.Fatal(err)
	}

	body := `{"logo_path":""}`
	c, w := setupAdminContext(db, http.MethodPut, "/admin/settings", body, 1)
	UpdateSettings(db)(c)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", w.Code, w.Body.String())
	}
	if _, err := os.Stat(logoPath); !os.IsNotExist(err) {
		t.Fatal("expected logo file removed")
	}
}

func TestUpdateSettings_InvalidJSON(t *testing.T) {
	db := setupDB(t)
	c, w := setupAdminContext(db, http.MethodPut, "/admin/settings", `{`, 1)
	UpdateSettings(db)(c)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d", w.Code)
	}
}

func TestSubmitQuery_OptionBounds(t *testing.T) {
	db := setupDB(t)
	cfg := testConfig()
	nodeRepo := repo.NewNodeRepo(db, "")
	created, _ := nodeRepo.Create(context.Background(), &domain.Node{
		Name: "n", Type: domain.NodeTypeStandalone, Active: true,
		EnabledCmds: domain.DefaultEnabledCmds(), AgentToken: "tok",
	})
	refreshTestSiteCache(t, db)

	body := fmt.Sprintf(`{"node_id":%d,"command":"ping","target":"8.8.8.8","protocol":"icmp","options":{"ping_count":99,"max_hops":99}}`, created.ID)
	c, w := setupContext(db, http.MethodPost, "/query", body)
	SubmitQuery(db, cfg, nil, nil)(c)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", w.Code, w.Body.String())
	}
}

func TestSubmitQuery_TracerouteWithGeo(t *testing.T) {
	db := setupDB(t)
	cfg := testConfig()
	g := geo.New("", "")
	nodeRepo := repo.NewNodeRepo(db, "")
	created, _ := nodeRepo.Create(context.Background(), &domain.Node{
		Name: "n", Type: domain.NodeTypeStandalone, Active: true,
		EnabledCmds: domain.DefaultEnabledCmds(), AgentToken: "tok",
	})
	refreshTestSiteCache(t, db)

	body := fmt.Sprintf(`{"node_id":%d,"command":"traceroute","target":"8.8.8.8","options":{"max_hops":1,"ping_count":1}}`, created.ID)
	c, w := setupContext(db, http.MethodPost, "/query", body)
	SubmitQuery(db, cfg, g, nil)(c)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", w.Code, w.Body.String())
	}

	var resp struct {
		Data struct {
			QueryID string `json:"query_id"`
		} `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}

	deadline := time.Now().Add(15 * time.Second)
	for time.Now().Before(deadline) {
		if result, ok := queryStore.Get(resp.Data.QueryID); ok && result != nil && result.Status != domain.StatusRunning {
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatal("query did not finish")
}

func TestWaitForASPathEnrichment(t *testing.T) {
	queryID := "aspath-wait"
	queryStore.SetRunning(queryID)
	queryStore.MergePartial(queryID, &domain.QueryResult{
		Status: domain.StatusRunning,
		ASPath: []uint32{64512, 15169},
		ASPathEnriched: []domain.ASInfo{
			{ASN: 64512, OrgName: "A"},
			{ASN: 15169, OrgName: "B"},
		},
	})
	waitForASPathEnrichment(queryID, 200*time.Millisecond)
}

func TestDBSettingsProvider(t *testing.T) {
	db := setupDB(t)
	_ = db
	p := &dbSettingsProvider{}
	settings, err := p.GetSettings(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if settings == nil {
		t.Fatal("expected settings map")
	}
}

func TestHandlerNew(t *testing.T) {
	db := setupDB(t)
	cfg := testConfig()
	h := New(db, cfg, nil, nil)
	if h == nil || h.engine == nil {
		t.Fatal("expected handler with engine")
	}
}

func TestListNodes_SkipsNilEntry(t *testing.T) {
	db := setupDB(t)
	nodeRepo := repo.NewNodeRepo(db, "")
	created, _ := nodeRepo.Create(context.Background(), &domain.Node{
		Name: "x", Type: domain.NodeTypeStandalone, Active: true,
		EnabledCmds: domain.DefaultEnabledCmds(),
	})
	refreshTestSiteCache(t, db)
	_ = created

	c, w := setupContext(db, http.MethodGet, "/nodes", "")
	ListNodes(db, "", nil)(c)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d", w.Code)
	}
}

func TestMyIP_WithGeoDB(t *testing.T) {
	g := geo.New("", "")
	c, w := setupContext(nil, http.MethodGet, "/myip", "")
	c.Request.RemoteAddr = "8.8.8.8:1234"
	MyIP(g)(c)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d", w.Code)
	}
}

func TestUploadLogo_NoFile(t *testing.T) {
	db := setupDB(t)
	c, w := setupAdminContext(db, http.MethodPost, "/admin/settings/logo", "", 1)
	UploadLogo(db)(c)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d", w.Code)
	}
}

func TestUploadLogo_PNGSuccess(t *testing.T) {
	db := setupDB(t)
	uploadDir := t.TempDir()
	SetLogoUploadsDir(uploadDir)
	t.Cleanup(func() { SetLogoUploadsDir("") })

	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	part, err := writer.CreateFormFile("logo", "logo.png")
	if err != nil {
		t.Fatal(err)
	}
	pngHeader := []byte{0x89, 0x50, 0x4e, 0x47, 0x0d, 0x0a, 0x1a, 0x0a, 0, 0, 0, 0}
	if _, err := part.Write(pngHeader); err != nil {
		t.Fatal(err)
	}
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

func TestUploadLogo_InvalidType(t *testing.T) {
	db := setupDB(t)
	uploadDir := t.TempDir()
	SetLogoUploadsDir(uploadDir)

	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	part, _ := writer.CreateFormFile("logo", "logo.txt")
	_, _ = io.WriteString(part, "plain text file")
	writer.Close()

	req := httptest.NewRequest(http.MethodPost, "/admin/settings/logo", body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = req

	UploadLogo(db)(c)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d", w.Code)
	}
}

func TestUploadLogo_BadSVG(t *testing.T) {
	db := setupDB(t)
	uploadDir := t.TempDir()
	SetLogoUploadsDir(uploadDir)

	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	part, _ := writer.CreateFormFile("logo", "logo.svg")
	_, _ = io.WriteString(part, `<svg xmlns="http://www.w3.org/2000/svg"><script>alert(1)</script></svg>`)
	writer.Close()

	req := httptest.NewRequest(http.MethodPost, "/admin/settings/logo", body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = req

	UploadLogo(db)(c)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, body = %s", w.Code, w.Body.String())
	}
}

func TestUploadLogo_TooLarge(t *testing.T) {
	db := setupDB(t)
	uploadDir := t.TempDir()
	SetLogoUploadsDir(uploadDir)

	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	part, _ := writer.CreateFormFile("logo", "logo.png")
	large := make([]byte, 2*1024*1024+1)
	large[0] = 0x89
	copy(large[1:4], []byte("PNG"))
	_, _ = part.Write(large)
	writer.Close()

	req := httptest.NewRequest(http.MethodPost, "/admin/settings/logo", body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = req

	UploadLogo(db)(c)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d", w.Code)
	}
}
