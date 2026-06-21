package handler

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/HopStat/HopStat/internal/config"
	"github.com/HopStat/HopStat/internal/domain"
	"github.com/HopStat/HopStat/internal/store/repo"
)

func TestBGPNeighborRequestValidate(t *testing.T) {
	tests := []struct {
		name string
		req  bgpNeighborRequest
		want string
	}{
		{
			name: "missing node",
			req:  bgpNeighborRequest{RemoteAS: 174, PeeringIP: "10.0.0.1", NeighborIP: "10.0.0.2"},
			want: "node is required",
		},
		{
			name: "missing addresses",
			req:  bgpNeighborRequest{NodeID: 1, RemoteAS: 174},
			want: "IPv4 or IPv6 peering addresses are required",
		},
		{
			name: "valid ipv4",
			req:  bgpNeighborRequest{NodeID: 1, RemoteAS: 174, PeeringIP: "10.0.0.1", NeighborIP: "10.0.0.2"},
			want: "",
		},
		{
			name: "valid ipv6 only",
			req:  bgpNeighborRequest{NodeID: 1, RemoteAS: 174, IPv6PeeringIP: "2001:db8::1", IPv6NeighborIP: "2001:db8::2"},
			want: "",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.req.Validate(); got != tc.want {
				t.Fatalf("Validate() = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestPeerTypeFor(t *testing.T) {
	if got := domain.PeerTypeFor(65000, 65000); got != domain.BGPPeerInternal {
		t.Fatalf("same AS = %q, want internal", got)
	}
	if got := domain.PeerTypeFor(65000, 174); got != domain.BGPPeerExternal {
		t.Fatalf("different AS = %q, want external", got)
	}
}

func TestCreateBGPNeighbor_NilBGPManager(t *testing.T) {
	db := setupDB(t)
	adminID := seedAdminPassword(t, db, "testpassword123")

	nodeRepo := repo.NewNodeRepo(db, "")
	created, err := nodeRepo.Create(context.Background(), &domain.Node{
		Name:        "Test Node",
		Type:        domain.NodeTypeStandalone,
		Active:      true,
		EnabledCmds: []domain.CommandType{domain.CmdPing},
	})
	if err != nil {
		t.Fatalf("create node: %v", err)
	}

	body := fmt.Sprintf(`{
		"node_id": %d,
		"remote_as": 174,
		"peering_ip": "192.168.1.1",
		"neighbor_ip": "10.0.0.1"
	}`, created.ID)

	bgpCfg := config.BGPConfig{LocalAS: 65000}
	c, w := setupAdminContext(db, http.MethodPost, "/admin/bgp-neighbors", body, adminID)
	CreateBGPNeighbor(db, nil, bgpCfg)(c)

	if w.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d; body: %s", w.Code, http.StatusCreated, w.Body.String())
	}

	var resp map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("parse response: %v", err)
	}
	data, ok := resp["data"].(map[string]interface{})
	if !ok {
		t.Fatalf("data is %T, want map", resp["data"])
	}
	if data["neighbor_ip"] != "10.0.0.1" {
		t.Fatalf("neighbor_ip = %v", data["neighbor_ip"])
	}
	if int(data["local_as"].(float64)) != 65000 {
		t.Fatalf("local_as = %v, want 65000 from config", data["local_as"])
	}

	listC, listW := setupAdminContext(db, http.MethodGet, "/admin/bgp-neighbors", "", adminID)
	ListBGPNeighbors(db, nil, bgpCfg)(listC)
	if listW.Code != http.StatusOK {
		t.Fatalf("list status = %d, want %d; body: %s", listW.Code, http.StatusOK, listW.Body.String())
	}
}

func TestBGPNeighborActions_NilBGPManager(t *testing.T) {
	db := setupDB(t)
	adminID := seedAdminPassword(t, db, "testpassword123")

	stopC, stopW := setupAdminContext(db, http.MethodPost, "/admin/bgp-neighbors/1/stop", "", adminID)
	stopC.Params = gin.Params{{Key: "id", Value: "1"}}
	StopBGPNeighbor(nil)(stopC)
	if stopW.Code != http.StatusServiceUnavailable {
		t.Fatalf("stop status = %d, want %d; body: %s", stopW.Code, http.StatusServiceUnavailable, stopW.Body.String())
	}

	restartC, restartW := setupAdminContext(db, http.MethodPost, "/admin/bgp-neighbors/1/restart", "", adminID)
	restartC.Params = gin.Params{{Key: "id", Value: "1"}}
	RestartBGPNeighbor(nil)(restartC)
	if restartW.Code != http.StatusServiceUnavailable {
		t.Fatalf("restart status = %d, want %d; body: %s", restartW.Code, http.StatusServiceUnavailable, restartW.Body.String())
	}

	logsC, logsW := setupAdminContext(db, http.MethodGet, "/admin/bgp-neighbors/1/logs", "", adminID)
	logsC.Params = gin.Params{{Key: "id", Value: "1"}}
	GetBGPNeighborLogs(nil)(logsC)
	if logsW.Code != http.StatusServiceUnavailable {
		t.Fatalf("logs status = %d, want %d", logsW.Code, http.StatusServiceUnavailable)
	}
}
