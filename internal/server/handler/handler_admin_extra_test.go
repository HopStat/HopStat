package handler

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"testing"
	"time"

	"github.com/HopStat/HopStat/internal/bgp"
	"github.com/HopStat/HopStat/internal/config"
	"github.com/HopStat/HopStat/internal/domain"
	"github.com/HopStat/HopStat/internal/store/repo"
	"github.com/gin-gonic/gin"
)

func TestCreateNode_StandaloneSuccess(t *testing.T) {
	db := setupDB(t)
	body := `{"name":"local","type":"standalone","active":true,"enabled_cmds":["ping"]}`
	c, w := setupAdminContext(db, http.MethodPost, "/admin/nodes", body, 1)

	CreateNode(db, "")(c)

	if w.Code != http.StatusCreated {
		t.Fatalf("status = %d, body = %s", w.Code, w.Body.String())
	}
	var resp struct {
		Data struct {
			Name string `json:"name"`
			Type string `json:"type"`
		} `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("parse: %v", err)
	}
	if resp.Data.Name != "local" || resp.Data.Type != "standalone" {
		t.Fatalf("unexpected node: %+v", resp.Data)
	}
}

func TestCreateNode_InvalidType(t *testing.T) {
	db := setupDB(t)
	body := `{"name":"bad","type":"invalid"}`
	c, w := setupAdminContext(db, http.MethodPost, "/admin/nodes", body, 1)

	CreateNode(db, "")(c)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", w.Code)
	}
}

func TestUpdateNode_Rename(t *testing.T) {
	db := setupDB(t)
	nodeRepo := repo.NewNodeRepo(db, "")
	created, err := nodeRepo.Create(context.Background(), &domain.Node{
		Name: "old", Type: domain.NodeTypeStandalone, Active: true,
		EnabledCmds: domain.DefaultEnabledCmds(),
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	body := `{"name":"new-name"}`
	c, w := setupAdminContext(db, http.MethodPut, fmt.Sprintf("/admin/nodes/%d", created.ID), body, 1)
	c.Params = gin.Params{{Key: "id", Value: fmt.Sprint(created.ID)}}

	UpdateNode(db, "")(c)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", w.Code, w.Body.String())
	}
}

func TestDeleteNode_Success(t *testing.T) {
	db := setupDB(t)
	nodeRepo := repo.NewNodeRepo(db, "")
	created, _ := nodeRepo.Create(context.Background(), &domain.Node{
		Name: "gone", Type: domain.NodeTypeStandalone, Active: true,
		EnabledCmds: domain.DefaultEnabledCmds(),
	})

	c, w := setupAdminContext(db, http.MethodDelete, fmt.Sprintf("/admin/nodes/%d", created.ID), "", 1)
	c.Params = gin.Params{{Key: "id", Value: fmt.Sprint(created.ID)}}

	DeleteNode(db, "", nil)(c)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", w.Code, w.Body.String())
	}
}

func TestDeleteNode_RemovesBGPNeighbors(t *testing.T) {
	db := setupDB(t)
	nodeRepo := repo.NewNodeRepo(db, "")
	bgpRepo := repo.NewBGPNeighborRepo(db)

	created, err := nodeRepo.Create(context.Background(), &domain.Node{
		Name: "with-bgp", Type: domain.NodeTypeStandalone, Active: true,
		EnabledCmds: domain.DefaultEnabledCmds(),
	})
	if err != nil {
		t.Fatalf("create node: %v", err)
	}

	neighbor, err := bgpRepo.Create(context.Background(), &domain.BGPNeighbor{
		NodeID:     created.ID,
		LocalAS:    65000,
		RemoteAS:   174,
		PeeringIP:  "10.4.4.1",
		NeighborIP: "10.4.4.3",
	})
	if err != nil {
		t.Fatalf("create bgp neighbor: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	listenPort := freeListenPort(t)
	mgr := bgp.NewSessionManager(config.BGPConfig{LocalAS: 65000, RouterID: "10.4.4.1", ListenPort: listenPort})
	if err := mgr.Start(ctx); err != nil {
		t.Fatalf("start bgp manager: %v", err)
	}
	defer mgr.Stop()
	if err := mgr.AddNeighbor(neighbor); err != nil {
		t.Fatalf("add neighbor: %v", err)
	}

	c, w := setupAdminContext(db, http.MethodDelete, fmt.Sprintf("/admin/nodes/%d", created.ID), "", 1)
	c.Params = gin.Params{{Key: "id", Value: fmt.Sprint(created.ID)}}
	DeleteNode(db, "", mgr)(c)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", w.Code, w.Body.String())
	}

	remaining, err := bgpRepo.GetByNodeID(context.Background(), created.ID)
	if err != nil {
		t.Fatalf("GetByNodeID: %v", err)
	}
	if len(remaining) != 0 {
		t.Fatalf("expected 0 bgp neighbors after node delete, got %d", len(remaining))
	}
	if _, ok := mgr.GetAllStatuses()[neighbor.ID]; ok {
		t.Fatal("expected neighbor removed from session manager")
	}
	if mgr.HasNeighbors(created.ID) {
		t.Fatal("expected no active neighbors for deleted node")
	}
}

func TestGetPublicSettings(t *testing.T) {
	db := setupDB(t)
	c, w := setupContext(db, http.MethodGet, "/settings", "")

	GetPublicSettings(db, config.BGPConfig{})(c)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", w.Code, w.Body.String())
	}
}

func TestGetAdminSettings(t *testing.T) {
	db := setupDB(t)
	c, w := setupAdminContext(db, http.MethodGet, "/admin/settings", "", 1)

	GetAdminSettings(db)(c)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", w.Code, w.Body.String())
	}
}

func TestCommunityRules_CRUD(t *testing.T) {
	db := setupDB(t)

	createBody := `{"community":"65535:100","severity":"warning","message_i18n":"test","scope":"global","active":true}`
	c, w := setupAdminContext(db, http.MethodPost, "/admin/community-rules", createBody, 1)
	CreateCommunityRule(db)(c)
	if w.Code != http.StatusCreated {
		t.Fatalf("create status = %d, body = %s", w.Code, w.Body.String())
	}

	var created struct {
		Data struct {
			ID int64 `json:"id"`
		} `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &created); err != nil {
		t.Fatalf("parse create: %v", err)
	}

	c, w = setupAdminContext(db, http.MethodGet, "/admin/community-rules", "", 1)
	ListCommunityRules(db)(c)
	if w.Code != http.StatusOK {
		t.Fatalf("list status = %d", w.Code)
	}

	updateBody := `{"community":"65535:200","severity":"info","message_i18n":"updated","scope":"global","active":false}`
	c, w = setupAdminContext(db, http.MethodPut, fmt.Sprintf("/admin/community-rules/%d", created.Data.ID), updateBody, 1)
	c.Params = gin.Params{{Key: "id", Value: fmt.Sprint(created.Data.ID)}}
	UpdateCommunityRule(db)(c)
	if w.Code != http.StatusOK {
		t.Fatalf("update status = %d, body = %s", w.Code, w.Body.String())
	}

	c, w = setupAdminContext(db, http.MethodDelete, fmt.Sprintf("/admin/community-rules/%d", created.Data.ID), "", 1)
	c.Params = gin.Params{{Key: "id", Value: fmt.Sprint(created.Data.ID)}}
	DeleteCommunityRule(db)(c)
	if w.Code != http.StatusOK {
		t.Fatalf("delete status = %d, body = %s", w.Code, w.Body.String())
	}
}

func TestListPublicCommunities_Empty(t *testing.T) {
	db := setupDB(t)
	c, w := setupContext(db, http.MethodGet, "/communities", "")

	ListPublicCommunities(db)(c)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d", w.Code)
	}
}

func TestCreateCommunityRule_InvalidSeverity(t *testing.T) {
	db := setupDB(t)
	body := `{"community":"1:1","severity":"critical"}`
	c, w := setupAdminContext(db, http.MethodPost, "/admin/community-rules", body, 1)

	CreateCommunityRule(db)(c)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", w.Code)
	}
}

func freeListenPort(t *testing.T) int {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	port := ln.Addr().(*net.TCPAddr).Port
	ln.Close()
	return port
}
