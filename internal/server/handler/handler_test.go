package handler

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
	"golang.org/x/crypto/bcrypt"
	_ "modernc.org/sqlite"

	"github.com/HopStat/HopStat/internal/config"
	"github.com/HopStat/HopStat/internal/domain"
	"github.com/HopStat/HopStat/internal/geo"
	"github.com/HopStat/HopStat/internal/sitecache"
	"github.com/HopStat/HopStat/internal/store"
	"github.com/HopStat/HopStat/internal/store/repo"
)

func init() {
	gin.SetMode(gin.TestMode)
}

func setupDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("open in-memory db: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	if _, err := db.Exec("PRAGMA foreign_keys=ON"); err != nil {
		t.Fatalf("enable foreign_keys: %v", err)
	}
	if err := store.Migrate(db); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	if err := sitecache.Load(db, "", 0); err != nil {
		t.Fatalf("load site cache: %v", err)
	}
	return db
}

func refreshTestSiteCache(t *testing.T, db *sql.DB) {
	t.Helper()
	if err := sitecache.RefreshNodes(db, ""); err != nil {
		t.Fatalf("refresh nodes cache: %v", err)
	}
}

func testConfig() *config.Config {
	return &config.Config{
		Server:   config.ServerConfig{Mode: "server"},
		Security: config.SecurityConfig{JWTSecret: "test-secret-that-is-at-least-32-chars-long"},
		Query:    config.QueryConfig{MaxConcurrent: 10, DefaultTimeoutSec: 30},
	}
}

func setupContext(db *sql.DB, method, path string, body string) (*gin.Context, *httptest.ResponseRecorder) {
	var req *http.Request
	if body != "" {
		req = httptest.NewRequest(method, path, strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
	} else {
		req = httptest.NewRequest(method, path, nil)
	}
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = req
	return c, w
}

func setupAdminContext(db *sql.DB, method, path, body string, userID int64) (*gin.Context, *httptest.ResponseRecorder) {
	c, w := setupContext(db, method, path, body)
	c.Set("user_id", userID)
	c.Set("user_role", "admin")
	return c, w
}

func seedAdminPassword(t *testing.T, db *sql.DB, password string) int64 {
	t.Helper()
	if err := store.SetAdminPassword(db, password); err != nil {
		t.Fatalf("set admin password: %v", err)
	}
	var id int64
	if err := db.QueryRow(`SELECT id FROM users WHERE email = ?`, store.DefaultAdminEmail).Scan(&id); err != nil {
		t.Fatalf("get admin id: %v", err)
	}
	return id
}

func TestGetAccount(t *testing.T) {
	db := setupDB(t)
	adminID := seedAdminPassword(t, db, "testpassword123")
	c, w := setupAdminContext(db, http.MethodGet, "/admin/account", "", adminID)

	GetAccount(db)(c)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body: %s", w.Code, http.StatusOK, w.Body.String())
	}
	var body map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("parse response: %v", err)
	}
	data, ok := body["data"].(map[string]interface{})
	if !ok {
		t.Fatalf("data is %T, want map", body["data"])
	}
	if data["email"] != store.DefaultAdminEmail {
		t.Errorf("email = %v, want %s", data["email"], store.DefaultAdminEmail)
	}
	if _, hasPw := data["password_hash"]; hasPw {
		t.Error("account should not expose password_hash")
	}
}

func TestUpdateAccount_ChangePassword(t *testing.T) {
	db := setupDB(t)
	adminID := seedAdminPassword(t, db, "oldpassword123")
	body := `{"email":"admin@hopstat.local","current_password":"oldpassword123","new_password":"newpassword123"}`
	c, w := setupAdminContext(db, http.MethodPut, "/admin/account", body, adminID)

	UpdateAccount(db)(c)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body: %s", w.Code, http.StatusOK, w.Body.String())
	}

	userRepo := repo.NewUserRepo(db)
	user, err := userRepo.GetByID(context.Background(), adminID)
	if err != nil || user == nil {
		t.Fatalf("get user: %v", err)
	}
	if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte("newpassword123")); err != nil {
		t.Fatal("password was not updated")
	}
}

func TestUpdateAccount_WrongPassword(t *testing.T) {
	db := setupDB(t)
	adminID := seedAdminPassword(t, db, "correctpassword")
	body := `{"email":"admin@hopstat.local","current_password":"wrongpassword","new_password":"newpassword123"}`
	c, w := setupAdminContext(db, http.MethodPut, "/admin/account", body, adminID)

	UpdateAccount(db)(c)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusUnauthorized)
	}
}

// ---------------------------------------------------------------------------
// Tests
// ---------------------------------------------------------------------------

func TestListNodes_Empty(t *testing.T) {
	db := setupDB(t)
	c, w := setupContext(db, http.MethodGet, "/nodes", "")

	ListNodes(db, "", nil)(c)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusOK)
	}

	var body map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("parse response: %v", err)
	}
	data, ok := body["data"]
	if !ok {
		t.Fatal("response missing 'data' key")
	}
	arr, ok := data.([]interface{})
	if !ok {
		t.Fatalf("data is %T, want slice", data)
	}
	if len(arr) != 0 {
		t.Fatalf("data length = %d, want 0", len(arr))
	}
}

func TestListNodes_ReturnsDefaultFlag(t *testing.T) {
	db := setupDB(t)
	nodeRepo := repo.NewNodeRepo(db, "")
	_, err := nodeRepo.Create(context.Background(), &domain.Node{
		Name:        "alpha",
		Type:        domain.NodeTypeStandalone,
		EnabledCmds: []domain.CommandType{domain.CmdPing},
		Active:      true,
	})
	if err != nil {
		t.Fatalf("create alpha: %v", err)
	}
	beta, err := nodeRepo.Create(context.Background(), &domain.Node{
		Name:        "beta",
		Type:        domain.NodeTypeStandalone,
		EnabledCmds: []domain.CommandType{domain.CmdPing},
		Active:      true,
	})
	if err != nil {
		t.Fatalf("create beta: %v", err)
	}
	if err := nodeRepo.SetDefault(context.Background(), beta.ID); err != nil {
		t.Fatalf("set default: %v", err)
	}
	if err := sitecache.RefreshNodes(db, ""); err != nil {
		t.Fatalf("refresh node cache: %v", err)
	}

	c, w := setupContext(db, http.MethodGet, "/nodes", "")
	ListNodes(db, "", nil)(c)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", w.Code, w.Body.String())
	}
	var body struct {
		Data []struct {
			ID        int64 `json:"id"`
			IsDefault bool  `json:"is_default"`
		} `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("parse response: %v", err)
	}
	if len(body.Data) != 2 {
		t.Fatalf("node count = %d, want 2", len(body.Data))
	}
	var defaults int
	for _, n := range body.Data {
		if n.IsDefault {
			defaults++
			if n.ID != beta.ID {
				t.Fatalf("default node id = %d, want %d", n.ID, beta.ID)
			}
		}
	}
	if defaults != 1 {
		t.Fatalf("default count = %d, want 1", defaults)
	}
}

func TestGetNode_NotFound(t *testing.T) {
	db := setupDB(t)
	c, w := setupContext(db, http.MethodGet, "/nodes/999", "")
	c.Params = gin.Params{{Key: "id", Value: "999"}}

	GetNode(db, "")(c)

	if w.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusNotFound)
	}
}

func TestGetNode_InvalidID(t *testing.T) {
	db := setupDB(t)
	c, w := setupContext(db, http.MethodGet, "/nodes/abc", "")
	c.Params = gin.Params{{Key: "id", Value: "abc"}}

	GetNode(db, "")(c)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusBadRequest)
	}
}

func TestSubmitQuery_InvalidJSON(t *testing.T) {
	db := setupDB(t)
	cfg := testConfig()
	c, w := setupContext(db, http.MethodPost, "/query", "{bad json")

	SubmitQuery(db, cfg, nil, nil)(c)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusBadRequest)
	}
}

func TestGetResult_NotFound(t *testing.T) {
	db := setupDB(t)
	c, w := setupContext(db, http.MethodGet, "/query/nonexistent-id", "")
	c.Params = gin.Params{{Key: "id", Value: "nonexistent-id"}}

	GetResult(db)(c)

	if w.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusNotFound)
	}
}

func TestLogin_InvalidJSON(t *testing.T) {
	db := setupDB(t)
	cfg := testConfig()
	c, w := setupContext(db, http.MethodPost, "/auth/login", "not json")

	Login(db, cfg)(c)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusBadRequest)
	}
}

func TestLogin_InvalidCredentials(t *testing.T) {
	db := setupDB(t)
	cfg := testConfig()
	c, w := setupContext(db, http.MethodPost, "/auth/login",
		`{"email":"nobody@example.com","password":"wrong"}`)

	Login(db, cfg)(c)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusUnauthorized)
	}
}

func TestMyIP(t *testing.T) {
	db := setupDB(t)
	c, w := setupContext(db, http.MethodGet, "/myip", "")
	// Override ClientIP by setting the RemoteAddr
	c.Request.RemoteAddr = "1.2.3.4:1234"

	MyIP(nil)(c)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusOK)
	}

	var body map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("parse response: %v", err)
	}
	data, ok := body["data"].(map[string]interface{})
	if !ok {
		t.Fatal("response 'data' is not an object")
	}
	ip, ok := data["ip"].(string)
	if !ok || ip == "" {
		t.Fatal("response 'data.ip' missing or empty")
	}
}

func TestMyIPUsesCymruWhenMaxMindDisabled(t *testing.T) {
	g := geo.New("", "")
	if g.Enabled() {
		t.Fatal("expected disabled geo db fixture")
	}

	c, w := setupContext(nil, http.MethodGet, "/myip", "")
	c.Request.RemoteAddr = "8.8.8.8:1234"

	MyIP(g)(c)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusOK)
	}

	var body map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("parse response: %v", err)
	}
	data, ok := body["data"].(map[string]interface{})
	if !ok {
		t.Fatal("response 'data' is not an object")
	}
	if data["ip"] != "8.8.8.8" {
		t.Fatalf("ip = %v, want 8.8.8.8", data["ip"])
	}

	asn, ok := data["asn"].(float64)
	if !ok || asn <= 0 {
		t.Fatalf("expected asn from cymru fallback, got %#v", data["asn"])
	}
	org, _ := data["asn_org"].(string)
	if strings.TrimSpace(org) == "" {
		t.Fatalf("expected asn_org from cymru fallback, got %#v", data["asn_org"])
	}
	cc, _ := data["country_code"].(string)
	if strings.TrimSpace(cc) == "" {
		t.Fatalf("expected country_code from cymru fallback, got %#v", data["country_code"])
	}
}

func TestListAllNodes_Empty(t *testing.T) {
	db := setupDB(t)
	c, w := setupContext(db, http.MethodGet, "/admin/nodes", "")

	ListAllNodes(db, "")(c)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusOK)
	}

	var body map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("parse response: %v", err)
	}
	data, ok := body["data"].([]interface{})
	if !ok {
		t.Fatalf("data is %T, want slice", body["data"])
	}
	if len(data) != 0 {
		t.Fatalf("data length = %d, want 0", len(data))
	}
}

func TestListAudit_Empty(t *testing.T) {
	db := setupDB(t)
	c, w := setupContext(db, http.MethodGet, "/admin/audit", "")

	ListAudit(db)(c)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusOK)
	}

	var body map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("parse response: %v", err)
	}
	if _, ok := body["data"]; !ok {
		t.Fatal("response missing 'data' key")
	}
	if _, ok := body["meta"]; !ok {
		t.Fatal("response missing 'meta' key")
	}
	meta, ok := body["meta"].(map[string]interface{})
	if !ok {
		t.Fatalf("meta is %T, want map", body["meta"])
	}
	for _, key := range []string{"total", "today", "page", "limit"} {
		if _, exists := meta[key]; !exists {
			t.Errorf("meta missing key %q", key)
		}
	}
}

func TestGenerateJWT(t *testing.T) {
	secret := "test-secret-that-is-at-least-32-chars-long"
	userID := int64(42)

	tokenStr, err := generateJWT(userID, "admin", secret)
	if err != nil {
		t.Fatalf("generateJWT error: %v", err)
	}
	if tokenStr == "" {
		t.Fatal("generateJWT returned empty token")
	}

	token, err := jwt.Parse(tokenStr, func(t *jwt.Token) (interface{}, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", t.Header["alg"])
		}
		return []byte(secret), nil
	})
	if err != nil {
		t.Fatalf("parse token: %v", err)
	}

	claims, ok := token.Claims.(jwt.MapClaims)
	if !ok {
		t.Fatal("token claims are not MapClaims")
	}
	gotUserID, ok := claims["user_id"]
	if !ok {
		t.Fatal("claims missing user_id")
	}
	// JSON numbers decode as float64
	if uid, ok := gotUserID.(float64); !ok || int64(uid) != userID {
		t.Fatalf("user_id = %v, want %d", gotUserID, userID)
	}
	gotRole, ok := claims["role"]
	if !ok {
		t.Fatal("claims missing role")
	}
	if r, ok := gotRole.(string); !ok || r != "admin" {
		t.Fatalf("role = %v, want admin", gotRole)
	}
}

func TestTestNode_LGNode(t *testing.T) {
	agent := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/agent/v1/health" {
			http.NotFound(w, r)
			return
		}
		if r.Header.Get("Authorization") != "Bearer secret-token" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer agent.Close()

	db := setupDB(t)
	nodeRepo := repo.NewNodeRepo(db, "")
	created, err := nodeRepo.Create(context.Background(), &domain.Node{
		Name:        "remote",
		Type:        domain.NodeTypeLGNode,
		AgentURL:    agent.URL,
		AgentToken:  "secret-token",
		EnabledCmds: []domain.CommandType{domain.CmdPing},
		Active:      true,
	})
	if err != nil {
		t.Fatalf("create node: %v", err)
	}

	c, w := setupContext(db, http.MethodPost, fmt.Sprintf("/admin/nodes/%d/test", created.ID), "")
	c.Params = gin.Params{{Key: "id", Value: fmt.Sprint(created.ID)}}
	TestNode(db, "", testConfig())(c)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", w.Code, w.Body.String())
	}
	var body struct {
		Data struct {
			Status  string `json:"status"`
			Message string `json:"message"`
		} `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("parse response: %v", err)
	}
	if body.Data.Status != "ok" {
		t.Fatalf("status = %q, message = %q", body.Data.Status, body.Data.Message)
	}
}

func TestTestNode_LGNodeReportsConnectionError(t *testing.T) {
	db := setupDB(t)
	nodeRepo := repo.NewNodeRepo(db, "")
	created, err := nodeRepo.Create(context.Background(), &domain.Node{
		Name:        "remote",
		Type:        domain.NodeTypeLGNode,
		AgentURL:    "http://127.0.0.1:1",
		AgentToken:  "secret-token",
		EnabledCmds: []domain.CommandType{domain.CmdPing},
		Active:      true,
	})
	if err != nil {
		t.Fatalf("create node: %v", err)
	}

	c, w := setupContext(db, http.MethodPost, fmt.Sprintf("/admin/nodes/%d/test", created.ID), "")
	c.Params = gin.Params{{Key: "id", Value: fmt.Sprint(created.ID)}}
	TestNode(db, "", testConfig())(c)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", w.Code, w.Body.String())
	}
	var body struct {
		Data struct {
			Status  string `json:"status"`
			Message string `json:"message"`
		} `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("parse response: %v", err)
	}
	if body.Data.Status != "error" {
		t.Fatalf("status = %q, want error", body.Data.Status)
	}
	if body.Data.Message == "" || body.Data.Message == "internal error" {
		t.Fatalf("message = %q, want concrete connection error", body.Data.Message)
	}
}

func TestHashPassword(t *testing.T) {
	password := "super-secret-password"

	hash, err := hashPassword(password)
	if err != nil {
		t.Fatalf("hashPassword error: %v", err)
	}
	if hash == "" {
		t.Fatal("hashPassword returned empty hash")
	}
	if hash == password {
		t.Fatal("hash matches the original password")
	}

	// Verify the hash is valid bcrypt
	if err := bcrypt.CompareHashAndPassword([]byte(hash), []byte(password)); err != nil {
		t.Fatalf("bcrypt compare failed: %v", err)
	}

	// Wrong password should not match
	if err := bcrypt.CompareHashAndPassword([]byte(hash), []byte("wrong")); err == nil {
		t.Fatal("expected bcrypt compare to fail for wrong password")
	}
}
