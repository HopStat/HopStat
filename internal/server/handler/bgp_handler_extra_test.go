package handler

import (
	"net/http"
	"testing"

	"github.com/HopStat/HopStat/internal/config"
)

func TestListBGPNeighbors_NilManager(t *testing.T) {
	db := setupDB(t)
	c, w := setupAdminContext(db, http.MethodGet, "/admin/bgp-neighbors", "", 1)

	ListBGPNeighbors(db, nil, config.BGPConfig{})(c)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", w.Code, w.Body.String())
	}
}

func TestGetBGPConfig(t *testing.T) {
	c, w := setupAdminContext(nil, http.MethodGet, "/admin/bgp/config", "", 1)

	GetBGPConfig(config.BGPConfig{LocalAS: 64512}, nil)(c)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", w.Code, w.Body.String())
	}
}

func TestGetBGPNeighborStatuses_NilManager(t *testing.T) {
	c, w := setupAdminContext(nil, http.MethodGet, "/admin/bgp-neighbors/statuses", "", 1)

	GetBGPNeighborStatuses(nil)(c)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", w.Code, w.Body.String())
	}
}
