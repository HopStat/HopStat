package bgp

import (
	"context"
	"testing"
	"time"

	"github.com/HopStat/HopStat/internal/config"
	"github.com/HopStat/HopStat/internal/domain"
	api "github.com/osrg/gobgp/v3/api"
)

func TestAddNeighborUsesConfigLocalAS(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	mgr := NewSessionManager(config.BGPConfig{LocalAS: 65000, RouterID: "10.4.4.1", ListenPort: freeListenPort(t)})
	if err := mgr.Start(ctx); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer mgr.Stop()

	neighbor := &domain.BGPNeighbor{
		ID:         1,
		NodeID:     1,
		LocalAS:    99999,
		RemoteAS:   174,
		PeeringIP:  "10.4.4.1",
		NeighborIP: "10.4.4.3",
	}

	if err := mgr.AddNeighbor(neighbor); err != nil {
		t.Fatalf("AddNeighbor: %v", err)
	}
	if mgr.globalAS != 65000 {
		t.Fatalf("globalAS = %d, want 65000 from config", mgr.globalAS)
	}
}

func TestAddNeighborRequiresConfigLocalAS(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	mgr := NewSessionManager(config.BGPConfig{ListenPort: freeListenPort(t)})
	if err := mgr.Start(ctx); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer mgr.Stop()

	neighbor := &domain.BGPNeighbor{
		ID:         1,
		NodeID:     1,
		LocalAS:    65000,
		RemoteAS:   174,
		PeeringIP:  "10.4.4.1",
		NeighborIP: "10.4.4.3",
	}

	err := mgr.AddNeighbor(neighbor)
	if err == nil {
		t.Fatal("expected error when bgp.local_as is not configured")
	}
}

func TestNeighborIPsForNode(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	mgr := NewSessionManager(config.BGPConfig{LocalAS: 65000, RouterID: "10.4.4.1", ListenPort: freeListenPort(t)})
	if err := mgr.Start(ctx); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer mgr.Stop()

	neighbor := &domain.BGPNeighbor{
		ID:             1,
		NodeID:         10,
		LocalAS:        65000,
		RemoteAS:       174,
		PeeringIP:      "10.4.4.1",
		NeighborIP:     "10.4.4.3",
		IPv6PeeringIP:  "2001:db8::1",
		IPv6NeighborIP: "2001:db8::3",
	}
	if err := mgr.AddNeighbor(neighbor); err != nil {
		t.Fatalf("AddNeighbor: %v", err)
	}

	ips := mgr.neighborIPsForNode(10)
	if len(ips) != 2 {
		t.Fatalf("neighborIPsForNode = %d IPs, want 2", len(ips))
	}
	if _, ok := ips["10.4.4.3"]; !ok {
		t.Fatal("expected IPv4 neighbor IP")
	}
	if _, ok := ips["2001:db8::3"]; !ok {
		t.Fatal("expected IPv6 neighbor IP")
	}
	if len(mgr.neighborIPsForNode(99)) != 0 {
		t.Fatal("expected no IPs for unknown node")
	}
}

func TestBuildPeerConfigPeerTypes(t *testing.T) {
	mgr := NewSessionManager(config.BGPConfig{LocalAS: 65000})

	internal := mgr.buildPeerConfig(&domain.BGPNeighbor{RemoteAS: 65000}, "10.0.0.1", "10.0.0.2")
	if internal.Conf.Type != api.PeerType_INTERNAL {
		t.Fatalf("internal peer type = %v, want INTERNAL", internal.Conf.Type)
	}
	if internal.EbgpMultihop != nil {
		t.Fatal("expected no ebgp multihop for internal peer")
	}

	external := mgr.buildPeerConfig(&domain.BGPNeighbor{RemoteAS: 174, Multihop: true}, "10.0.0.1", "10.0.0.3")
	if external.Conf.Type != api.PeerType_EXTERNAL {
		t.Fatalf("external peer type = %v, want EXTERNAL", external.Conf.Type)
	}
	if external.EbgpMultihop == nil || !external.EbgpMultihop.Enabled {
		t.Fatal("expected ebgp multihop for external multihop peer")
	}

	internalMultihop := mgr.buildPeerConfig(&domain.BGPNeighbor{RemoteAS: 65000, Multihop: true}, "10.0.0.1", "10.0.0.4")
	if internalMultihop.EbgpMultihop != nil {
		t.Fatal("expected multihop ignored for internal peer")
	}
}

func TestBuildPeerConfigPassiveMode(t *testing.T) {
	mgr := NewSessionManager(config.BGPConfig{LocalAS: 65000})

	outbound := mgr.buildPeerConfig(&domain.BGPNeighbor{RemoteAS: 65000}, "10.0.0.1", "10.0.0.2")
	if outbound.Transport.PassiveMode {
		t.Fatal("expected outbound (non-passive) by default")
	}

	passive := mgr.buildPeerConfig(&domain.BGPNeighbor{RemoteAS: 65000, PassiveMode: true}, "10.0.0.1", "10.0.0.2")
	if !passive.Transport.PassiveMode {
		t.Fatal("expected passive mode when configured")
	}
}

func TestBuildPeerConfigEnablesAddPathReceive(t *testing.T) {
	mgr := NewSessionManager(config.BGPConfig{LocalAS: 65000, AddPathReceive: true})
	peer := mgr.buildPeerConfig(&domain.BGPNeighbor{RemoteAS: 174}, "10.0.0.1", "10.0.0.2")
	if len(peer.AfiSafis) != 1 {
		t.Fatalf("afi-safis = %d, want 1", len(peer.AfiSafis))
	}
	v4 := peer.AfiSafis[0]
	if !v4.Config.Enabled {
		t.Fatal("expected ipv4 unicast enabled for ipv4 peer")
	}
	if v4.AddPaths == nil || v4.AddPaths.Config == nil || !v4.AddPaths.Config.Receive {
		t.Fatal("expected add-path receive on enabled ipv4 unicast")
	}

	mgrDisabled := NewSessionManager(config.BGPConfig{LocalAS: 65000, AddPathReceive: false})
	peerDisabled := mgrDisabled.buildPeerConfig(&domain.BGPNeighbor{RemoteAS: 174}, "10.0.0.1", "10.0.0.2")
	if peerDisabled.AfiSafis[0].AddPaths != nil {
		t.Fatal("expected add-path disabled when config is false")
	}
}

// A session must announce exactly the family it carries: GoBGP advertises a multiprotocol
// capability for every afi-safi handed to it, so a spare entry would make an IPv4-only
// session claim IPv6 and draw an NLRI mismatch from the router.
func TestBuildPeerConfigAnnouncesOnlyItsOwnFamily(t *testing.T) {
	mgr := NewSessionManager(config.BGPConfig{LocalAS: 65000})

	for _, tc := range []struct {
		name     string
		neighbor string
		want     api.Family_Afi
	}{
		{"ipv4", "10.0.0.2", api.Family_AFI_IP},
		{"ipv6", "2001:db8::2", api.Family_AFI_IP6},
	} {
		t.Run(tc.name, func(t *testing.T) {
			peer := mgr.buildPeerConfig(&domain.BGPNeighbor{RemoteAS: 174}, "", tc.neighbor)
			if len(peer.AfiSafis) != 1 {
				t.Fatalf("afi-safis = %d, want 1", len(peer.AfiSafis))
			}
			af := peer.AfiSafis[0].Config
			if af.Family.Afi != tc.want {
				t.Fatalf("afi = %v, want %v", af.Family.Afi, tc.want)
			}
			if af.Family.Safi != api.Family_SAFI_UNICAST {
				t.Fatalf("safi = %v, want UNICAST", af.Family.Safi)
			}
			if !af.Enabled {
				t.Fatal("expected the announced family to be enabled")
			}
		})
	}
}
