package handler

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/HopStat/HopStat/internal/geo"
)

func TestGeoIPStatus_NilDB(t *testing.T) {
	db := setupDB(t)
	cfg := testConfig()
	c, w := setupAdminContext(db, http.MethodGet, "/admin/geoip/status", "", 1)

	GeoIPStatus(db, cfg, nil)(c)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", w.Code, w.Body.String())
	}
}

func TestGeoIPLookup_NilDB(t *testing.T) {
	c, w := setupAdminContext(nil, http.MethodGet, "/admin/geoip/lookup?ip=8.8.8.8", "", 1)

	GeoIPLookup(nil)(c)

	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503", w.Code)
	}
}

func TestGeoIPLookup_MissingIP(t *testing.T) {
	g := geo.New("", "")
	c, w := setupAdminContext(nil, http.MethodGet, "/admin/geoip/lookup", "", 1)

	GeoIPLookup(g)(c)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", w.Code)
	}
}

func TestGeoIPLookup_Success(t *testing.T) {
	g := geo.New("", "")
	c, w := setupAdminContext(nil, http.MethodGet, "/admin/geoip/lookup?ip=8.8.8.8", "", 1)

	GeoIPLookup(g)(c)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", w.Code, w.Body.String())
	}
	var body map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body["data"] == nil {
		t.Fatal("expected data")
	}
}

func TestGeoIPLookup_InvalidIP(t *testing.T) {
	g := geo.New("", "")
	c, w := setupAdminContext(nil, http.MethodGet, "/admin/geoip/lookup?ip=not-an-ip", "", 1)

	GeoIPLookup(g)(c)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", w.Code)
	}
}

func TestGeoIPStatus_WithDB(t *testing.T) {
	db := setupDB(t)
	cfg := testConfig()
	g := geo.New("", "")
	c, w := setupAdminContext(db, http.MethodGet, "/admin/geoip/status", "", 1)

	GeoIPStatus(db, cfg, g)(c)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", w.Code, w.Body.String())
	}
}

func TestGeoIPStatus_ClosedDB(t *testing.T) {
	db := setupDB(t)
	db.Close()
	cfg := testConfig()
	c, w := setupAdminContext(db, http.MethodGet, "/admin/geoip/status", "", 1)

	GeoIPStatus(db, cfg, nil)(c)

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d", w.Code)
	}
}
