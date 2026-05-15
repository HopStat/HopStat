package handler

import (
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
	"github.com/HopStat/HopStat/internal/store"
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
	if err := store.Migrate(db); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return db
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

// ---------------------------------------------------------------------------
// Tests
// ---------------------------------------------------------------------------

func TestListNodes_Empty(t *testing.T) {
	db := setupDB(t)
	c, w := setupContext(db, http.MethodGet, "/nodes", "")

	ListNodes(db, "")(c)

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

	SubmitQuery(db, cfg, nil)(c)

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
	for _, key := range []string{"total", "page", "limit"} {
		if _, exists := meta[key]; !exists {
			t.Errorf("meta missing key %q", key)
		}
	}
}

func TestListUsers_WithSeed(t *testing.T) {
	db := setupDB(t)
	c, w := setupContext(db, http.MethodGet, "/admin/users", "")

	ListUsers(db)(c)

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
	if len(data) < 1 {
		t.Fatalf("data length = %d, want at least 1 (seed admin)", len(data))
	}
	seedUser := data[0].(map[string]interface{})
	if seedUser["email"] != "admin@lookingglass.local" {
		t.Errorf("seed user email = %v, want admin@lookingglass.local", seedUser["email"])
	}
	if _, hasPw := seedUser["password_hash"]; hasPw {
		t.Error("seed user should not expose password_hash")
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
