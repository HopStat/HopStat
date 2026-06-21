package bgp

import (
	"context"
	"fmt"
	"testing"
	"time"

	api "github.com/osrg/gobgp/v3/api"
	"github.com/osrg/gobgp/v3/pkg/apiutil"
	"github.com/osrg/gobgp/v3/pkg/packet/bgp"

	"github.com/HopStat/HopStat/internal/config"
)

// Simulates large RIB prefix counting: GetTable is O(1) metadata, ListPath is O(routes).
func TestLargeRIBPrefixCountSimulation(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping RIB scale simulation in -short mode")
	}

	const routeCount = 5000

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	mgr := NewSessionManager(config.BGPConfig{LocalAS: 65001, RouterID: "127.0.0.1"})
	if err := mgr.Start(ctx); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer mgr.Stop()

	for i := range routeCount {
		prefixLen := uint8(24)
		prefix := fmt.Sprintf("10.%d.%d.0", (i/256)+1, i%256)
		path, err := apiutil.NewPath(
			bgp.NewIPAddrPrefix(prefixLen, prefix),
			false,
			[]bgp.PathAttributeInterface{
				bgp.NewPathAttributeOrigin(bgp.BGP_ORIGIN_ATTR_TYPE_INCOMPLETE),
				bgp.NewPathAttributeNextHop("127.0.0.1"),
			},
			time.Now(),
		)
		if err != nil {
			t.Fatalf("NewPath: %v", err)
		}
		if _, err := mgr.bgpServer.AddPath(ctx, &api.AddPathRequest{
			TableType: api.TableType_GLOBAL,
			Path:      path,
		}); err != nil {
			t.Fatalf("AddPath %q: %v", prefix, err)
		}
	}

	listStart := time.Now()
	listCount := 0
	err := mgr.bgpServer.ListPath(ctx, &api.ListPathRequest{
		TableType: api.TableType_GLOBAL,
		Family:    &api.Family{Afi: api.Family_AFI_IP, Safi: api.Family_SAFI_UNICAST},
	}, func(dst *api.Destination) {
		listCount++
	})
	if err != nil {
		t.Fatalf("ListPath: %v", err)
	}
	listElapsed := time.Since(listStart)

	tableStart := time.Now()
	resp, err := mgr.bgpServer.GetTable(ctx, &api.GetTableRequest{
		TableType: api.TableType_GLOBAL,
		Family:    &api.Family{Afi: api.Family_AFI_IP, Safi: api.Family_SAFI_UNICAST},
	})
	if err != nil {
		t.Fatalf("GetTable: %v", err)
	}
	tableElapsed := time.Since(tableStart)

	if listCount != routeCount {
		t.Fatalf("ListPath count = %d, want %d", listCount, routeCount)
	}
	if int(resp.NumDestination) != routeCount {
		t.Fatalf("GetTable NumDestination = %d, want %d", resp.NumDestination, routeCount)
	}

	t.Logf("routes=%d ListPath=%s GetTable=%s speedup=%.1fx",
		routeCount, listElapsed, tableElapsed, float64(listElapsed)/float64(max(tableElapsed, time.Nanosecond)))

	if tableElapsed >= listElapsed/2 {
		t.Fatalf("expected GetTable to be much faster than ListPath at scale (table=%s list=%s)", tableElapsed, listElapsed)
	}
}

func BenchmarkAdjPrefixCountGetTable(b *testing.B) {
	ctx := context.Background()
	mgr := NewSessionManager(config.BGPConfig{LocalAS: 65001, RouterID: "127.0.0.1"})
	if err := mgr.Start(ctx); err != nil {
		b.Fatalf("Start: %v", err)
	}
	defer mgr.Stop()

	const routeCount = 2000
	for i := range routeCount {
		prefix := fmt.Sprintf("172.%d.%d.0", (i/256)+1, i%256)
		path, err := apiutil.NewPath(
			bgp.NewIPAddrPrefix(24, prefix),
			false,
			[]bgp.PathAttributeInterface{
				bgp.NewPathAttributeOrigin(bgp.BGP_ORIGIN_ATTR_TYPE_INCOMPLETE),
				bgp.NewPathAttributeNextHop("127.0.0.1"),
			},
			time.Now(),
		)
		if err != nil {
			b.Fatalf("NewPath: %v", err)
		}
		if _, err := mgr.bgpServer.AddPath(ctx, &api.AddPathRequest{
			TableType: api.TableType_GLOBAL,
			Path:      path,
		}); err != nil {
			b.Fatalf("AddPath: %v", err)
		}
	}

	b.ResetTimer()
	for range b.N {
		resp, err := mgr.bgpServer.GetTable(ctx, &api.GetTableRequest{
			TableType: api.TableType_GLOBAL,
			Family:    &api.Family{Afi: api.Family_AFI_IP, Safi: api.Family_SAFI_UNICAST},
		})
		if err != nil {
			b.Fatalf("GetTable: %v", err)
		}
		if resp.NumDestination == 0 {
			b.Fatal("expected routes in table")
		}
	}
}
