package handler

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"testing"

	"github.com/HopStat/HopStat/internal/domain"
	"github.com/HopStat/HopStat/internal/store/repo"
	"github.com/gin-gonic/gin"
)

func TestSubmitQuery_InvalidCommand(t *testing.T) {
	db := setupDB(t)
	cfg := testConfig()
	body := `{"node_id":1,"command":"invalid","target":"8.8.8.8"}`
	c, w := setupContext(db, http.MethodPost, "/query", body)

	SubmitQuery(db, cfg, nil, nil)(c)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, body = %s", w.Code, w.Body.String())
	}
}

func TestSubmitQuery_NodeNotFound(t *testing.T) {
	db := setupDB(t)
	cfg := testConfig()
	body := `{"node_id":999,"command":"ping","target":"8.8.8.8"}`
	c, w := setupContext(db, http.MethodPost, "/query", body)

	SubmitQuery(db, cfg, nil, nil)(c)

	if w.Code != http.StatusNotFound {
		t.Fatalf("status = %d, body = %s", w.Code, w.Body.String())
	}
}

func TestSubmitQuery_CommandDisabled(t *testing.T) {
	db := setupDB(t)
	cfg := testConfig()
	nodeRepo := repo.NewNodeRepo(db, "")
	created, err := nodeRepo.Create(context.Background(), &domain.Node{
		Name: "n", Type: domain.NodeTypeStandalone, Active: true,
		EnabledCmds: []domain.CommandType{domain.CmdTraceroute},
	})
	if err != nil {
		t.Fatalf("create node: %v", err)
	}
	refreshTestSiteCache(t, db)

	body := fmt.Sprintf(`{"node_id":%d,"command":"ping","target":"8.8.8.8"}`, created.ID)
	c, w := setupContext(db, http.MethodPost, "/query", body)

	SubmitQuery(db, cfg, nil, nil)(c)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, body = %s", w.Code, w.Body.String())
	}
}

func TestSubmitQuery_InvalidTarget(t *testing.T) {
	db := setupDB(t)
	cfg := testConfig()
	nodeRepo := repo.NewNodeRepo(db, "")
	created, _ := nodeRepo.Create(context.Background(), &domain.Node{
		Name: "n", Type: domain.NodeTypeStandalone, Active: true,
		EnabledCmds: domain.DefaultEnabledCmds(),
	})
	refreshTestSiteCache(t, db)

	body := fmt.Sprintf(`{"node_id":%d,"command":"ping","target":"127.0.0.1"}`, created.ID)
	c, w := setupContext(db, http.MethodPost, "/query", body)

	SubmitQuery(db, cfg, nil, nil)(c)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, body = %s", w.Code, w.Body.String())
	}
}

func TestSubmitQuery_Accepted(t *testing.T) {
	db := setupDB(t)
	cfg := testConfig()
	nodeRepo := repo.NewNodeRepo(db, "")
	created, _ := nodeRepo.Create(context.Background(), &domain.Node{
		Name: "n", Type: domain.NodeTypeStandalone, Active: true,
		EnabledCmds: domain.DefaultEnabledCmds(),
	})
	refreshTestSiteCache(t, db)

	body := fmt.Sprintf(`{"node_id":%d,"command":"ping","target":"8.8.8.8","options":{"ping_count":1}}`, created.ID)
	c, w := setupContext(db, http.MethodPost, "/query", body)

	SubmitQuery(db, cfg, nil, nil)(c)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", w.Code, w.Body.String())
	}
	var resp struct {
		Data struct {
			QueryID string `json:"query_id"`
		} `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("parse: %v", err)
	}
	if resp.Data.QueryID == "" {
		t.Fatal("expected query id")
	}
}

func TestGetNode_Success(t *testing.T) {
	db := setupDB(t)
	nodeRepo := repo.NewNodeRepo(db, "")
	created, _ := nodeRepo.Create(context.Background(), &domain.Node{
		Name: "visible", Type: domain.NodeTypeStandalone, Active: true,
		EnabledCmds: domain.DefaultEnabledCmds(),
	})

	c, w := setupContext(db, http.MethodGet, fmt.Sprintf("/nodes/%d", created.ID), "")
	c.Params = gin.Params{{Key: "id", Value: fmt.Sprint(created.ID)}}

	GetNode(db, "")(c)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", w.Code, w.Body.String())
	}
}

func TestSetDefaultNode(t *testing.T) {
	db := setupDB(t)
	nodeRepo := repo.NewNodeRepo(db, "")
	n1, _ := nodeRepo.Create(context.Background(), &domain.Node{
		Name: "a", Type: domain.NodeTypeStandalone, Active: true, IsDefault: true,
		EnabledCmds: domain.DefaultEnabledCmds(),
	})
	n2, _ := nodeRepo.Create(context.Background(), &domain.Node{
		Name: "b", Type: domain.NodeTypeStandalone, Active: true,
		EnabledCmds: domain.DefaultEnabledCmds(),
	})

	c, w := setupAdminContext(db, http.MethodPost, fmt.Sprintf("/admin/nodes/%d/default", n2.ID), "", 1)
	c.Params = gin.Params{{Key: "id", Value: fmt.Sprint(n2.ID)}}

	SetDefaultNode(db, "")(c)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", w.Code, w.Body.String())
	}
	got, _ := nodeRepo.GetByID(context.Background(), n1.ID)
	if got.IsDefault {
		t.Error("n1 should no longer be default")
	}
}

func TestUpdateSettings(t *testing.T) {
	db := setupDB(t)
	body := `{"site_name":"HopStat Test","ping_count":"4"}`
	c, w := setupAdminContext(db, http.MethodPut, "/admin/settings", body, 1)

	UpdateSettings(db)(c)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", w.Code, w.Body.String())
	}
}
