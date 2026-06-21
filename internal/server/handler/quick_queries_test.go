package handler

import (
	"encoding/json"
	"fmt"
	"net/http"
	"testing"

	"github.com/HopStat/HopStat/internal/domain"
	"github.com/HopStat/HopStat/internal/sitecache"
	"github.com/HopStat/HopStat/internal/store"
	"github.com/gin-gonic/gin"
)

func TestQuickQueries_AdminListAndToggle(t *testing.T) {
	db := setupDB(t)

	createBody := `{"command":"ping","name":"CF","target":"1.1.1.1","active":true}`
	c, w := setupAdminContext(db, http.MethodPost, "/admin/quick-queries", createBody, 1)
	CreateQuickQuery(db)(c)
	if w.Code != http.StatusCreated {
		t.Fatalf("create status = %d", w.Code)
	}

	var created struct {
		Data struct {
			ID int64 `json:"id"`
		} `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &created); err != nil {
		t.Fatal(err)
	}

	c, w = setupAdminContext(db, http.MethodGet, "/admin/quick-queries", "", 1)
	ListQuickQueries(db)(c)
	if w.Code != http.StatusOK {
		t.Fatalf("list status = %d", w.Code)
	}

	c, w = setupAdminContext(db, http.MethodPatch, fmt.Sprintf("/admin/quick-queries/%d/toggle", created.Data.ID), "", 1)
	c.Params = gin.Params{{Key: "id", Value: fmt.Sprint(created.Data.ID)}}
	ToggleQuickQuery(db)(c)
	if w.Code != http.StatusOK {
		t.Fatalf("toggle status = %d, body = %s", w.Code, w.Body.String())
	}
}

func TestQuickQueries_ValidationErrors(t *testing.T) {
	db := setupDB(t)

	c, w := setupAdminContext(db, http.MethodPost, "/admin/quick-queries", `{`, 1)
	CreateQuickQuery(db)(c)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("json status = %d", w.Code)
	}

	body := `{"command":"bad","name":"x","target":"1.1.1.1"}`
	c, w = setupAdminContext(db, http.MethodPost, "/admin/quick-queries", body, 1)
	CreateQuickQuery(db)(c)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("command status = %d", w.Code)
	}

	body = `{"command":"ping","name":"","target":"1.1.1.1"}`
	c, w = setupAdminContext(db, http.MethodPost, "/admin/quick-queries", body, 1)
	CreateQuickQuery(db)(c)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("name status = %d", w.Code)
	}

	body = `{"command":"ping","name":"x","target":"not-valid"}`
	c, w = setupAdminContext(db, http.MethodPost, "/admin/quick-queries", body, 1)
	CreateQuickQuery(db)(c)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("target status = %d", w.Code)
	}

	body = `{"command":"bgp_route","name":"x","target":"not-valid"}`
	c, w = setupAdminContext(db, http.MethodPost, "/admin/quick-queries", body, 1)
	CreateQuickQuery(db)(c)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("bgp target status = %d", w.Code)
	}

	c, w = setupAdminContext(db, http.MethodPut, "/admin/quick-queries/0", body, 1)
	c.Params = gin.Params{{Key: "id", Value: "0"}}
	UpdateQuickQuery(db)(c)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("update invalid id status = %d", w.Code)
	}

	c, w = setupAdminContext(db, http.MethodDelete, "/admin/quick-queries/0", "", 1)
	c.Params = gin.Params{{Key: "id", Value: "0"}}
	DeleteQuickQuery(db)(c)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("delete invalid id status = %d", w.Code)
	}
}

func TestValidQuickQueryTarget(t *testing.T) {
	if !validQuickQueryTarget("ping", "1.1.1.1") {
		t.Fatal("expected valid ping target")
	}
	if validQuickQueryTarget("ping", "bad") {
		t.Fatal("expected invalid ping target")
	}
	if !validQuickQueryTarget("bgp_route", "8.8.8.8/32") {
		t.Fatal("expected valid cidr")
	}
	if validQuickQueryTarget("unknown", "1.1.1.1") {
		t.Fatal("expected invalid command")
	}
}

func TestMapQuickQueryNil(t *testing.T) {
	if (mapQuickQuery(nil) != domain.QuickQuery{}) {
		t.Fatal("expected zero value")
	}
}

func TestQuickQueries_CRUD(t *testing.T) {
	db := setupDB(t)
	if err := store.Migrate(db); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	if err := sitecache.Load(db, "", 0); err != nil {
		t.Fatalf("load site cache: %v", err)
	}

	createBody := `{"command":"ping","name":"Cloudflare","target":"1.1.1.1","active":true}`
	c, w := setupAdminContext(db, http.MethodPost, "/admin/quick-queries", createBody, 1)
	CreateQuickQuery(db)(c)
	if w.Code != http.StatusCreated {
		t.Fatalf("create status = %d body = %s", w.Code, w.Body.String())
	}

	var created struct {
		Data struct {
			ID int64 `json:"id"`
		} `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &created); err != nil {
		t.Fatalf("decode create: %v", err)
	}

	c, w = setupAdminContext(db, http.MethodGet, "/quick-queries", "", 0)
	ListPublicQuickQueries()(c)
	if w.Code != http.StatusOK {
		t.Fatalf("public list status = %d", w.Code)
	}

	updateBody := `{"command":"ping","name":"Cloudflare DNS","target":"1.1.1.1","active":true}`
	c, w = setupAdminContext(db, http.MethodPut, fmt.Sprintf("/admin/quick-queries/%d", created.Data.ID), updateBody, 1)
	c.Params = gin.Params{{Key: "id", Value: fmt.Sprintf("%d", created.Data.ID)}}
	UpdateQuickQuery(db)(c)
	if w.Code != http.StatusOK {
		t.Fatalf("update status = %d body = %s", w.Code, w.Body.String())
	}

	c, w = setupAdminContext(db, http.MethodDelete, fmt.Sprintf("/admin/quick-queries/%d", created.Data.ID), "", 1)
	c.Params = gin.Params{{Key: "id", Value: fmt.Sprintf("%d", created.Data.ID)}}
	DeleteQuickQuery(db)(c)
	if w.Code != http.StatusOK {
		t.Fatalf("delete status = %d", w.Code)
	}
}
