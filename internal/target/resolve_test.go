package target

import (
	"context"
	"errors"
	"net"
	"testing"
	"time"

	"github.com/HopStat/HopStat/internal/domain"
)

func TestNormalizeBGPLookupIP(t *testing.T) {
	got, err := NormalizeBGPLookup(context.Background(), "8.8.8.8")
	if err != nil {
		t.Fatal(err)
	}
	if got != "8.8.8.8" {
		t.Fatalf("got %q", got)
	}
}

func TestNormalizeBGPLookupCIDR(t *testing.T) {
	got, err := NormalizeBGPLookup(context.Background(), "1.1.1.0/24")
	if err != nil {
		t.Fatal(err)
	}
	if got != "1.1.1.0/24" {
		t.Fatalf("got %q", got)
	}
}

func TestNormalizeBGPLookupHostCIDR(t *testing.T) {
	got, err := NormalizeBGPLookup(context.Background(), "185.193.165.238/32")
	if err != nil {
		t.Fatal(err)
	}
	if got != "185.193.165.238" {
		t.Fatalf("got %q", got)
	}
}

func TestValidateQueryTargetBGPHostCIDR(t *testing.T) {
	got, err := ValidateQueryTarget(context.Background(), "bgp_route", "185.193.165.238/32")
	if err != nil {
		t.Fatal(err)
	}
	if got != "185.193.165.238" {
		t.Fatalf("got %q", got)
	}
}

func TestNormalizeBGPLookupBlocked(t *testing.T) {
	_, err := NormalizeBGPLookup(context.Background(), "127.0.0.1")
	if err == nil {
		t.Fatal("expected blocked IP error")
	}
}

func TestValidateQueryTargetIP(t *testing.T) {
	got, err := ValidateQueryTarget(context.Background(), "ping", "8.8.8.8")
	if err != nil {
		t.Fatal(err)
	}
	if got != "8.8.8.8" {
		t.Fatalf("got %q", got)
	}
}

func TestValidateQueryTargetCIDR(t *testing.T) {
	got, err := ValidateQueryTarget(context.Background(), "bgp_route", "1.1.1.0/24")
	if err != nil {
		t.Fatal(err)
	}
	if got != "1.1.1.0/24" {
		t.Fatalf("got %q", got)
	}
}

func TestValidateQueryTargetDNSFailure(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	_, err := ValidateQueryTarget(ctx, "ping", "hopstat-invalid-host.example")
	if !errors.Is(err, domain.ErrDNSNotFound) {
		t.Fatalf("expected ErrDNSNotFound, got %v", err)
	}
}

func TestValidateQueryTargetBlocked(t *testing.T) {
	_, err := ValidateQueryTarget(context.Background(), "ping", "127.0.0.1")
	if err == nil {
		t.Fatal("expected blocked IP error")
	}
}

// Regression: 255.255.255.255 used to pass IsBlockedIP. IsMulticast covers only
// 224.0.0.0/4, so the top of the address space fell through every branch and a
// query naming it reached internal/driver/standalone, which execs ping against the
// broadcast address.
func TestIsBlockedIPRejectsLimitedBroadcast(t *testing.T) {
	if !IsBlockedIP(net.ParseIP("255.255.255.255")) {
		t.Fatal("IsBlockedIP(255.255.255.255) = false; the limited broadcast address must be blocked")
	}
	// The behavioural contract: the query gate must refuse it, not just the helper.
	if _, err := ValidateQueryTarget(context.Background(), "ping", "255.255.255.255"); err == nil {
		t.Fatal("ValidateQueryTarget accepted 255.255.255.255 as a ping target")
	}
	// Controls: ordinary public addresses stay dialable.
	for _, s := range []string{"8.8.8.8", "1.1.1.1", "93.184.216.34"} {
		if IsBlockedIP(net.ParseIP(s)) {
			t.Errorf("IsBlockedIP(%q) = true; public addresses must stay dialable", s)
		}
	}
}
