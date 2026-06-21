package bgp

import (
	"context"
	"fmt"
	"net"
	"testing"
	"time"

	api "github.com/osrg/gobgp/v3/api"
	"github.com/osrg/gobgp/v3/pkg/server"

	"github.com/HopStat/HopStat/internal/config"
	"github.com/HopStat/HopStat/internal/domain"
)

const testLoopback = "127.0.0.1"

func freeTCPPort(t *testing.T) int {
	t.Helper()
	ln, err := net.Listen("tcp", testLoopback+":0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	port := ln.Addr().(*net.TCPAddr).Port
	ln.Close()
	return port
}

func startStandaloneGoBGP(t *testing.T, localAS uint32, routerID string, listenPort int32) *server.BgpServer {
	t.Helper()
	ctx := context.Background()
	s := server.NewBgpServer()
	go s.Serve()

	if err := s.StartBgp(ctx, &api.StartBgpRequest{
		Global: &api.Global{
			Asn:             localAS,
			RouterId:        routerID,
			ListenPort:      listenPort,
			ListenAddresses: []string{testLoopback},
		},
	}); err != nil {
		t.Fatalf("StartBgp as=%d: %v", localAS, err)
	}
	t.Cleanup(func() { s.Stop() })
	return s
}

func addPassivePeer(t *testing.T, s *server.BgpServer, neighborIP string, localAS, remoteAS uint32, peerType api.PeerType) {
	t.Helper()
	if err := s.AddPeer(context.Background(), &api.AddPeerRequest{
		Peer: &api.Peer{
			Conf: &api.PeerConf{
				LocalAsn:        localAS,
				NeighborAddress: neighborIP,
				PeerAsn:         remoteAS,
				Type:            peerType,
			},
			Transport: &api.Transport{
				PassiveMode: true,
			},
			AfiSafis: []*api.AfiSafi{{
				Config: &api.AfiSafiConfig{
					Family:  &api.Family{Afi: api.Family_AFI_IP, Safi: api.Family_SAFI_UNICAST},
					Enabled: true,
				},
			}},
		},
	}); err != nil {
		t.Fatalf("AddPeer passive %s: %v", neighborIP, err)
	}
}

func waitForPeerState(t *testing.T, s *server.BgpServer, neighborIP string, want api.PeerState_SessionState, timeout time.Duration) *api.Peer {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		var found *api.Peer
		err := s.ListPeer(context.Background(), &api.ListPeerRequest{
			Address: neighborIP,
		}, func(p *api.Peer) {
			found = p
		})
		if err != nil {
			t.Fatalf("ListPeer: %v", err)
		}
		if found != nil && found.State != nil && found.State.SessionState == want {
			return found
		}
		time.Sleep(200 * time.Millisecond)
	}
	t.Fatalf("peer %s did not reach state %s within %s", neighborIP, want, timeout)
	return nil
}

func hopStatPeerFromNeighbor(mgr *SessionManager, n *domain.BGPNeighbor, remotePort uint32) *api.Peer {
	peer := mgr.buildPeerConfig(n, testLoopback, testLoopback)
	if remotePort > 0 {
		peer.Transport.RemotePort = remotePort
	}
	return peer
}

func TestExternalBGPSessionEstablishes(t *testing.T) {
	const (
		hopLocalAS   uint32 = 65000
		peerRemoteAS uint32 = 174
	)

	hopPort := freeTCPPort(t)
	peerPort := int32(freeTCPPort(t))

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	remote := startStandaloneGoBGP(t, peerRemoteAS, "127.0.0.2", peerPort)
	addPassivePeer(t, remote, testLoopback, peerRemoteAS, hopLocalAS, api.PeerType_EXTERNAL)

	mgr := NewSessionManager(config.BGPConfig{
		LocalAS:         hopLocalAS,
		RouterID:        "127.0.0.1",
		ListenPort:      hopPort,
		ListenAddresses: []string{testLoopback},
	})
	if err := mgr.Start(ctx); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer mgr.Stop()

	neighbor := &domain.BGPNeighbor{
		ID:         1,
		NodeID:     1,
		RemoteAS:   peerRemoteAS,
		PeeringIP:  testLoopback,
		NeighborIP: testLoopback,
		Multihop:   true,
	}
	peer := hopStatPeerFromNeighbor(mgr, neighbor, uint32(peerPort))
	if peer.Conf.Type != api.PeerType_EXTERNAL {
		t.Fatalf("built peer type = %v, want EXTERNAL", peer.Conf.Type)
	}
	if peer.EbgpMultihop == nil || !peer.EbgpMultihop.Enabled {
		t.Fatal("expected ebgp multihop enabled for external peer on loopback")
	}
	if err := mgr.bgpServer.AddPeer(ctx, &api.AddPeerRequest{Peer: peer}); err != nil {
		t.Fatalf("AddPeer: %v", err)
	}

	got := waitForPeerState(t, mgr.bgpServer, testLoopback, api.PeerState_ESTABLISHED, 15*time.Second)
	if got.Conf.Type != api.PeerType_EXTERNAL {
		t.Fatalf("established peer type = %v, want EXTERNAL", got.Conf.Type)
	}
	if got.State.PeerAsn != peerRemoteAS {
		t.Fatalf("negotiated peer AS = %d, want %d", got.State.PeerAsn, peerRemoteAS)
	}

	remotePeer := waitForPeerState(t, remote, testLoopback, api.PeerState_ESTABLISHED, 5*time.Second)
	if remotePeer.Conf.Type != api.PeerType_EXTERNAL {
		t.Fatalf("remote side peer type = %v, want EXTERNAL", remotePeer.Conf.Type)
	}
	t.Logf("external eBGP established: hopstat AS%d ↔ peer AS%d", hopLocalAS, remotePeer.State.PeerAsn)
}

func TestInternalBGPSessionEstablishes(t *testing.T) {
	const localAS uint32 = 65000

	hopPort := freeTCPPort(t)
	peerPort := int32(freeTCPPort(t))

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	remote := startStandaloneGoBGP(t, localAS, "127.0.0.2", peerPort)
	addPassivePeer(t, remote, testLoopback, localAS, localAS, api.PeerType_INTERNAL)

	mgr := NewSessionManager(config.BGPConfig{
		LocalAS:         localAS,
		RouterID:        "127.0.0.1",
		ListenPort:      hopPort,
		ListenAddresses: []string{testLoopback},
	})
	if err := mgr.Start(ctx); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer mgr.Stop()

	neighbor := &domain.BGPNeighbor{
		ID:         2,
		NodeID:     1,
		RemoteAS:   localAS,
		PeeringIP:  testLoopback,
		NeighborIP: testLoopback,
	}
	peer := hopStatPeerFromNeighbor(mgr, neighbor, uint32(peerPort))
	if peer.Conf.Type != api.PeerType_INTERNAL {
		t.Fatalf("built peer type = %v, want INTERNAL", peer.Conf.Type)
	}
	if peer.EbgpMultihop != nil {
		t.Fatal("expected no ebgp multihop for internal peer")
	}
	if err := mgr.bgpServer.AddPeer(ctx, &api.AddPeerRequest{Peer: peer}); err != nil {
		t.Fatalf("AddPeer: %v", err)
	}

	got := waitForPeerState(t, mgr.bgpServer, testLoopback, api.PeerState_ESTABLISHED, 15*time.Second)
	if got.Conf.Type != api.PeerType_INTERNAL {
		t.Fatalf("established peer type = %v, want INTERNAL", got.Conf.Type)
	}
	t.Logf("internal iBGP established: AS%d ↔ AS%d", localAS, got.State.PeerAsn)
}

func TestAddNeighborExternalViaSessionManager(t *testing.T) {
	const (
		hopLocalAS   uint32 = 65000
		peerRemoteAS uint32 = 6453
	)

	hopPort := freeTCPPort(t)
	peerPort := int32(freeTCPPort(t))

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	remote := startStandaloneGoBGP(t, peerRemoteAS, "127.0.0.2", peerPort)
	addPassivePeer(t, remote, testLoopback, peerRemoteAS, hopLocalAS, api.PeerType_EXTERNAL)

	mgr := NewSessionManager(config.BGPConfig{
		LocalAS:         hopLocalAS,
		RouterID:        "127.0.0.1",
		ListenPort:      hopPort,
		ListenAddresses: []string{testLoopback},
	})
	if err := mgr.Start(ctx); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer mgr.Stop()

	neighbor := &domain.BGPNeighbor{
		ID:         3,
		NodeID:     1,
		RemoteAS:   peerRemoteAS,
		PeeringIP:  testLoopback,
		NeighborIP: testLoopback,
		Multihop:   true,
	}

	// Production AddNeighbor dials port 179; lab uses explicit remote_port like multihop eBGP on loopback.
	peer := hopStatPeerFromNeighbor(mgr, neighbor, uint32(peerPort))
	if err := mgr.bgpServer.AddPeer(ctx, &api.AddPeerRequest{Peer: peer}); err != nil {
		t.Fatalf("AddPeer: %v", err)
	}

	got := waitForPeerState(t, mgr.bgpServer, testLoopback, api.PeerState_ESTABLISHED, 15*time.Second)
	if got.Conf.Type != api.PeerType_EXTERNAL {
		t.Fatalf("peer type = %v, want EXTERNAL for remote_as=%d local_as=%d", got.Conf.Type, peerRemoteAS, hopLocalAS)
	}
	fmt.Printf("external session established: peer_as=%d type=%s state=%s\n", got.State.PeerAsn, got.Conf.Type, got.State.SessionState)
}
