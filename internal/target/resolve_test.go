package target

import (
	"context"
	"errors"
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
