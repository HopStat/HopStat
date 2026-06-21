package target

import (
	"context"
	"errors"
	"net"
	"testing"
	"time"

	"github.com/HopStat/HopStat/internal/domain"
)

func TestIsBlockedIP(t *testing.T) {
	tests := []struct {
		name    string
		ip      string
		blocked bool
	}{
		{"loopback", "127.0.0.1", true},
		{"unspecified", "0.0.0.0", true},
		{"private", "10.0.0.1", true},
		{"link local", "169.254.10.1", true},
		{"multicast", "224.0.0.1", true},
		{"cgnat", "100.64.0.1", true},
		{"zero first octet", "0.1.2.3", true},
		{"public", "8.8.8.8", false},
		{"ipv6 loopback blocked via private check", "::1", true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := IsBlockedIP(net.ParseIP(tc.ip))
			if got != tc.blocked {
				t.Fatalf("IsBlockedIP(%q) = %v, want %v", tc.ip, got, tc.blocked)
			}
		})
	}
}

func TestResolveHost(t *testing.T) {
	tests := []struct {
		name    string
		target  string
		wantErr bool
		check   func(string) bool
	}{
		{"empty", "", true, nil},
		{"bare ip", "8.8.8.8", false, func(got string) bool { return got == "8.8.8.8" }},
		{"blocked ip", "10.0.0.1", true, nil},
		{"dns success", "one.one.one.one", false, func(got string) bool { return net.ParseIP(got) != nil }},
		{"dns failure", "hopstat-invalid-host.example", true, nil},
	}
	ctx := context.Background()
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := ResolveHost(ctx, tc.target)
			if tc.wantErr {
				if err == nil {
					t.Fatal("expected error")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if tc.check != nil && !tc.check(got) {
				t.Fatalf("unexpected result %q", got)
			}
		})
	}
}

func TestValidateQueryTarget(t *testing.T) {
	tests := []struct {
		name    string
		cmd     string
		target  string
		wantErr error
	}{
		{"empty", "ping", "", domain.ErrInvalidTarget},
		{"unsafe chars", "ping", "8.8.8.8;rm", domain.ErrInvalidTarget},
		{"invalid cidr", "bgp_route", "1.2.3.4/99", domain.ErrInvalidTarget},
		{"valid cidr", "bgp_route", "1.1.1.0/24", nil},
		{"blocked ip", "ping", "127.0.0.1", domain.ErrInvalidTarget},
		{"dns not found", "ping", "hopstat-invalid-host.example", domain.ErrDNSNotFound},
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := ValidateQueryTarget(ctx, tc.cmd, tc.target)
			if tc.wantErr == nil {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				return
			}
			if !errors.Is(err, tc.wantErr) {
				t.Fatalf("error = %v, want %v", err, tc.wantErr)
			}
		})
	}
}

func TestNormalizeBGPLookup(t *testing.T) {
	tests := []struct {
		name    string
		target  string
		wantErr bool
	}{
		{"empty", "", true},
		{"invalid cidr", "1.1.1.1/99", true},
		{"valid cidr", "1.1.1.0/24", false},
		{"blocked ip", "127.0.0.1", true},
		{"hostname", "one.one.one.one", false},
	}
	ctx := context.Background()
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := NormalizeBGPLookup(ctx, tc.target)
			if tc.wantErr {
				if err == nil {
					t.Fatal("expected error")
				}
				return
			}
			if err != nil || got == "" {
				t.Fatalf("got %q err %v", got, err)
			}
		})
	}
}

func TestAllowedAgentHost(t *testing.T) {
	tests := []struct {
		host string
		want bool
	}{
		{"", false},
		{"localhost", true},
		{"127.0.0.1", true},
		{"[::1]", true},
		{"8.8.8.8", true},
		{"169.254.169.254", false},
		{"10.0.0.1", false},
		{"192.168.1.1", false},
	}
	ctx := context.Background()
	for _, tc := range tests {
		if got := AllowedAgentHost(ctx, tc.host); got != tc.want {
			t.Errorf("AllowedAgentHost(%q) = %v, want %v", tc.host, got, tc.want)
		}
	}
}

func TestAllowedAgentHostDNS(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if !AllowedAgentHost(ctx, "one.one.one.one") {
		t.Fatal("expected public hostname to be allowed")
	}
}

func TestValidAgentURLExtra(t *testing.T) {
	tests := []struct {
		raw  string
		want bool
	}{
		{"https://127.0.0.1:8080", true},
		{"http://[::1]:9090", true},
		{"not-a-url", false},
		{"http:///missing-host", false},
	}
	for _, tc := range tests {
		if got := ValidAgentURL(tc.raw); got != tc.want {
			t.Errorf("ValidAgentURL(%q) = %v, want %v", tc.raw, got, tc.want)
		}
	}
}

func TestIsLocalInterfaceIP(t *testing.T) {
	if !isLocalInterfaceIP(net.ParseIP("127.0.0.1")) {
		t.Fatal("expected loopback interface to match")
	}
	if isLocalInterfaceIP(net.ParseIP("240.0.0.1")) {
		t.Fatal("expected non-local IP to not match")
	}
}
