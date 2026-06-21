package handler

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/HopStat/HopStat/internal/bgp"
	"github.com/HopStat/HopStat/internal/config"
	"github.com/HopStat/HopStat/internal/domain"
	"github.com/HopStat/HopStat/internal/store/repo"
)

func TestCreateBGPNeighbor_ValidationErrors(t *testing.T) {
	db := setupDB(t)
	bgpCfg := config.BGPConfig{LocalAS: 65000}

	c, w := setupAdminContext(db, http.MethodPost, "/admin/bgp-neighbors", `{`, 1)
	CreateBGPNeighbor(db, nil, bgpCfg)(c)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("json status = %d", w.Code)
	}

	body := `{"node_id":1,"remote_as":174}`
	c, w = setupAdminContext(db, http.MethodPost, "/admin/bgp-neighbors", body, 1)
	CreateBGPNeighbor(db, nil, bgpCfg)(c)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("validate status = %d, body = %s", w.Code, w.Body.String())
	}
}

func TestCreateBGPNeighbor_NotConfigured(t *testing.T) {
	db := setupDB(t)
	body := `{"node_id":1,"remote_as":174,"peering_ip":"10.0.0.1","neighbor_ip":"10.0.0.2"}`
	c, w := setupAdminContext(db, http.MethodPost, "/admin/bgp-neighbors", body, 1)

	CreateBGPNeighbor(db, nil, config.BGPConfig{})(c)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, body = %s", w.Code, w.Body.String())
	}
}

func TestBGPNeighbor_CRUDWithManager(t *testing.T) {
	db := setupDB(t)
	nodeRepo := repo.NewNodeRepo(db, "")
	created, err := nodeRepo.Create(context.Background(), &domain.Node{
		Name: "bgp-node", Type: domain.NodeTypeStandalone, Active: true,
		EnabledCmds: domain.DefaultEnabledCmds(),
	})
	if err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	listenPort := freeListenPort(t)
	bgpCfg := config.BGPConfig{LocalAS: 65000, RouterID: "10.4.4.1", ListenPort: listenPort}
	mgr := bgp.NewSessionManager(bgpCfg)
	if err := mgr.Start(ctx); err != nil {
		t.Fatalf("start bgp: %v", err)
	}
	defer mgr.Stop()

	createBody := fmt.Sprintf(`{
		"node_id": %d,
		"remote_as": 174,
		"peering_ip": "10.4.4.1",
		"neighbor_ip": "10.4.4.3"
	}`, created.ID)
	c, w := setupAdminContext(db, http.MethodPost, "/admin/bgp-neighbors", createBody, 1)
	CreateBGPNeighbor(db, mgr, bgpCfg)(c)
	if w.Code != http.StatusCreated {
		t.Fatalf("create status = %d, body = %s", w.Code, w.Body.String())
	}

	var createdResp struct {
		Data struct {
			ID int64 `json:"id"`
		} `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &createdResp); err != nil {
		t.Fatal(err)
	}
	neighborID := createdResp.Data.ID

	c, w = setupAdminContext(db, http.MethodGet, "/admin/bgp-neighbors", "", 1)
	ListBGPNeighbors(db, mgr, bgpCfg)(c)
	if w.Code != http.StatusOK {
		t.Fatalf("list status = %d", w.Code)
	}

	updateBody := fmt.Sprintf(`{
		"node_id": %d,
		"remote_as": 174,
		"peering_ip": "10.4.4.1",
		"neighbor_ip": "10.4.4.3",
		"multihop": true
	}`, created.ID)
	c, w = setupAdminContext(db, http.MethodPut, fmt.Sprintf("/admin/bgp-neighbors/%d", neighborID), updateBody, 1)
	c.Params = gin.Params{{Key: "id", Value: fmt.Sprint(neighborID)}}
	UpdateBGPNeighbor(db, mgr, bgpCfg)(c)
	if w.Code != http.StatusOK {
		t.Fatalf("update status = %d, body = %s", w.Code, w.Body.String())
	}

	c, w = setupAdminContext(db, http.MethodGet, fmt.Sprintf("/admin/bgp-neighbors/%d/logs?limit=5", neighborID), "", 1)
	c.Params = gin.Params{{Key: "id", Value: fmt.Sprint(neighborID)}}
	GetBGPNeighborLogs(mgr)(c)
	if w.Code != http.StatusOK {
		t.Fatalf("logs status = %d, body = %s", w.Code, w.Body.String())
	}

	c, w = setupAdminContext(db, http.MethodPost, fmt.Sprintf("/admin/bgp-neighbors/%d/stop", neighborID), "", 1)
	c.Params = gin.Params{{Key: "id", Value: fmt.Sprint(neighborID)}}
	StopBGPNeighbor(mgr)(c)
	if w.Code != http.StatusOK && w.Code != http.StatusBadRequest {
		t.Fatalf("stop status = %d, body = %s", w.Code, w.Body.String())
	}

	c, w = setupAdminContext(db, http.MethodPost, fmt.Sprintf("/admin/bgp-neighbors/%d/restart", neighborID), "", 1)
	c.Params = gin.Params{{Key: "id", Value: fmt.Sprint(neighborID)}}
	RestartBGPNeighbor(mgr)(c)
	if w.Code != http.StatusOK && w.Code != http.StatusBadRequest {
		t.Fatalf("restart status = %d, body = %s", w.Code, w.Body.String())
	}

	c, w = setupAdminContext(db, http.MethodDelete, fmt.Sprintf("/admin/bgp-neighbors/%d", neighborID), "", 1)
	c.Params = gin.Params{{Key: "id", Value: fmt.Sprint(neighborID)}}
	DeleteBGPNeighbor(db, mgr)(c)
	if w.Code != http.StatusOK {
		t.Fatalf("delete status = %d, body = %s", w.Code, w.Body.String())
	}
}

func TestUpdateBGPNeighbor_InvalidID(t *testing.T) {
	db := setupDB(t)
	bgpCfg := config.BGPConfig{LocalAS: 65000}
	body := `{"node_id":1,"remote_as":174,"peering_ip":"10.0.0.1","neighbor_ip":"10.0.0.2"}`

	c, w := setupAdminContext(db, http.MethodPut, "/admin/bgp-neighbors/x", body, 1)
	c.Params = gin.Params{{Key: "id", Value: "x"}}
	UpdateBGPNeighbor(db, nil, bgpCfg)(c)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d", w.Code)
	}
}

func TestDeleteBGPNeighbor_InvalidID(t *testing.T) {
	db := setupDB(t)
	c, w := setupAdminContext(db, http.MethodDelete, "/admin/bgp-neighbors/x", "", 1)
	c.Params = gin.Params{{Key: "id", Value: "x"}}
	DeleteBGPNeighbor(db, nil)(c)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d", w.Code)
	}
}

func TestGetBGPNeighborLogs_InvalidID(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	mgr := bgp.NewSessionManager(config.BGPConfig{LocalAS: 65000, RouterID: "10.0.0.1", ListenPort: freeListenPort(t)})
	_ = mgr.Start(ctx)
	defer mgr.Stop()

	c, w := setupAdminContext(nil, http.MethodGet, "/admin/bgp-neighbors/x/logs", "", 1)
	c.Params = gin.Params{{Key: "id", Value: "x"}}
	GetBGPNeighborLogs(mgr)(c)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d", w.Code)
	}
}

func TestBGPNeighborAction_InvalidID(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	mgr := bgp.NewSessionManager(config.BGPConfig{LocalAS: 65000, RouterID: "10.0.0.1", ListenPort: freeListenPort(t)})
	_ = mgr.Start(ctx)
	defer mgr.Stop()

	c, w := setupAdminContext(nil, http.MethodPost, "/admin/bgp-neighbors/x/stop", "", 1)
	c.Params = gin.Params{{Key: "id", Value: "x"}}
	StopBGPNeighbor(mgr)(c)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d", w.Code)
	}
}

func TestGetBGPConfig_RestartRequired(t *testing.T) {
	c, w := setupAdminContext(nil, http.MethodGet, "/admin/bgp/config", "", 1)
	GetBGPConfig(config.BGPConfig{LocalAS: 64512}, nil)(c)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d", w.Code)
	}
	var resp struct {
		Data struct {
			Status string `json:"status"`
		} `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if resp.Data.Status != "restart_required" {
		t.Fatalf("status = %q", resp.Data.Status)
	}
}

func TestGetBGPNeighborStatuses_WithManager(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	mgr := bgp.NewSessionManager(config.BGPConfig{LocalAS: 65000, RouterID: "10.0.0.1", ListenPort: freeListenPort(t)})
	_ = mgr.Start(ctx)
	defer mgr.Stop()

	c, w := setupAdminContext(nil, http.MethodGet, "/admin/bgp-neighbors/statuses", "", 1)
	GetBGPNeighborStatuses(mgr)(c)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d", w.Code)
	}
}

func TestValidateBGPConfigured(t *testing.T) {
	if msg := validateBGPConfigured(config.BGPConfig{}); msg == "" {
		t.Fatal("expected error message")
	}
	if msg := validateBGPConfigured(config.BGPConfig{LocalAS: 65000}); msg != "" {
		t.Fatalf("unexpected msg %q", msg)
	}
}

func TestBGPNeighborRequestValidate_IPErrors(t *testing.T) {
	req := bgpNeighborRequest{
		NodeID: 1, RemoteAS: 174,
		PeeringIP: "bad", NeighborIP: "10.0.0.2",
	}
	if got := req.Validate(); got != "invalid peering_ip" {
		t.Fatalf("got %q", got)
	}
}
