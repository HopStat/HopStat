package server

import (
	"testing"

	"github.com/HopStat/HopStat/internal/bgp"
	"github.com/HopStat/HopStat/internal/config"
)

func TestNewServerWithBGPManager(t *testing.T) {
	db := setupTestDB(t)
	mgr := bgp.NewSessionManager(config.BGPConfig{})

	srv := New(testServerConfig(), db, nil, newTestServerFS(), mgr, "dev")
	if srv.updater == nil || srv.bgpMgr != mgr {
		t.Fatal("expected the updater and BGP manager to be wired")
	}
}
