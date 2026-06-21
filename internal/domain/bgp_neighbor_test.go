package domain

import "testing"

func TestPeerTypeFor(t *testing.T) {
	if got := PeerTypeFor(65001, 65001); got != BGPPeerInternal {
		t.Fatalf("same AS want internal, got %q", got)
	}
	if got := PeerTypeFor(65001, 65002); got != BGPPeerExternal {
		t.Fatalf("different AS want external, got %q", got)
	}
	if got := PeerTypeFor(0, 65001); got != BGPPeerExternal {
		t.Fatalf("zero local AS want external, got %q", got)
	}
}

func TestBGPPeerTypeIsInternal(t *testing.T) {
	if !BGPPeerInternal.IsInternal() {
		t.Fatal("internal peer should report IsInternal true")
	}
	if BGPPeerExternal.IsInternal() {
		t.Fatal("external peer should report IsInternal false")
	}
}
