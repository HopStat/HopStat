package geo

import (
	"context"
	"testing"
)

func TestCountryToFlag(t *testing.T) {
	tests := []struct {
		code     string
		expected bool
	}{
		{"US", true},
		{"TR", true},
		{"DE", true},
		{"JP", true},
		{"", false},
		{"A", false},
		{"USA", false},
	}

	for _, tt := range tests {
		result := CountryToFlag(tt.code)
		if tt.expected && result == "" {
			t.Errorf("CountryToFlag(%q) expected non-empty", tt.code)
		}
		if !tt.expected && result != "" {
			t.Errorf("CountryToFlag(%q) expected empty, got %q", tt.code, result)
		}
	}
}

func TestNewGeoIPDBDisabled(t *testing.T) {
	g := New("", "")
	if g.Enabled() {
		t.Error("expected disabled with empty paths")
	}

	g = New("/nonexistent/path.mmdb", "/nonexistent/city.mmdb")
	if g.Enabled() {
		t.Error("expected disabled with nonexistent paths")
	}
}

func TestGeoIPDBClose(t *testing.T) {
	g := New("", "")
	g.Close()
}

func TestGeoIPDBResolveASNEmpty(t *testing.T) {
	g := New("", "")
	info, err := g.ResolveASN(context.Background(), "invalid-ip")
	if err == nil {
		t.Error("expected error for invalid IP")
	}
	_ = info
}

func TestGeoIPDBLookupCityDisabled(t *testing.T) {
	g := New("", "")
	_, err := g.LookupCity("8.8.8.8")
	if err == nil {
		t.Error("expected error when city db not loaded")
	}
}

func TestGeoIPDBReloadDisabled(t *testing.T) {
	g := New("", "")
	if err := g.Reload(); err != nil {
		t.Errorf("Reload on disabled db should succeed, got: %v", err)
	}
}
