package hostips

import (
	"net"
	"testing"
)

func TestList(t *testing.T) {
	v4, v6, err := List()
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if v4 == nil || v6 == nil {
		t.Fatal("expected non-nil address slices")
	}
	for _, addr := range v4 {
		parsed := net.ParseIP(addr.IP)
		if parsed == nil || parsed.To4() == nil {
			t.Fatalf("expected ipv4, got %+v", addr)
		}
		if parsed.IsLoopback() {
			t.Fatalf("loopback in ipv4 list: %+v", addr)
		}
	}
	for _, addr := range v6 {
		parsed := net.ParseIP(addr.IP)
		if parsed == nil || parsed.To4() != nil {
			t.Fatalf("expected ipv6, got %+v", addr)
		}
		if parsed.IsLoopback() {
			t.Fatalf("loopback in ipv6 list: %+v", addr)
		}
	}
}

func TestAppendUnique(t *testing.T) {
	list := AppendUnique([]Address{{IP: "10.4.4.56"}}, "10.4.4.57")
	if len(list) != 2 {
		t.Fatalf("len = %d, want 2", len(list))
	}
	list = AppendUnique(list, "10.4.4.56")
	if len(list) != 2 {
		t.Fatalf("duplicate added, len = %d", len(list))
	}
}
