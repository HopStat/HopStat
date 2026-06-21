package geo

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestParseCymruASNRecord(t *testing.T) {
	t.Run("arin", func(t *testing.T) {
		country, org := parseCymruASNRecord("15169 | 38.0.0.0/8 | US | arin | 2001-02-08 | GOOGLE")
		if country != "US" {
			t.Errorf("country: got %q, want US", country)
		}
		if org != "GOOGLE" {
			t.Errorf("org: got %q, want GOOGLE", org)
		}
	})
	t.Run("ripe", func(t *testing.T) {
		country, org := parseCymruASNRecord("43260 | TR | ripencc | 2007-07-04 | AS43260 - DGN TEKNOLOJI A.S., TR")
		if country != "TR" {
			t.Errorf("country: got %q, want TR", country)
		}
		if org != "AS43260 - DGN TEKNOLOJI A.S., TR" {
			t.Errorf("org: got %q", org)
		}
	})
	t.Run("org suffix country", func(t *testing.T) {
		if got := countryFromOrgSuffix("GOOGLE - Google LLC, US"); got != "US" {
			t.Errorf("got %q, want US", got)
		}
	})
}

func TestShortenOrgName(t *testing.T) {
	if got := shortenOrgName("AS43260 - DGN TEKNOLOJI A.S., TR"); got != "DGN" {
		t.Errorf("got %q, want DGN", got)
	}
	if got := shortenOrgName("CLOUDFLARENET - Cloudflare, Inc., US"); got != "CLOUDFLARENET" {
		t.Errorf("got %q, want CLOUDFLARENET", got)
	}
}

func TestFormatTracerouteOrgName(t *testing.T) {
	tests := []struct {
		in, want string
	}{
		{"AS43260 - DGN TEKNOLOJI A.S., TR", "DGN TEKNOLOJI"},
		{"CLOUDFLARENET - Cloudflare, Inc., US", "CLOUDFLARENET"},
		{"TURK TELEKOMUNIKASYON ANONIM SIRKETI", "TURK TELEKOMUNIKASYO"},
		{"INTERNATIONAL BUSINESS MACHINES CORPORATION", "INTERNATIONAL BUSINE"},
	}
	for _, tc := range tests {
		if got := FormatTracerouteOrgName(tc.in); got != tc.want {
			t.Errorf("FormatTracerouteOrgName(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestCountryToFlag(t *testing.T) {
	if flag := CountryToFlag("TR"); flag != "\U0001F1F9\U0001F1F7" {
		t.Errorf("TR flag: got %U, want U+1F1F9 U+1F1F7", []rune(flag))
	}
	if flag := CountryToFlag("US"); flag != "\U0001F1FA\U0001F1F8" {
		t.Errorf("US flag: got %U, want U+1F1FA U+1F1F8", []rune(flag))
	}
	if CountryToFlag("") != "" || CountryToFlag("USA") != "" {
		t.Error("expected empty for invalid codes")
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

func TestGeoIPDBLookupASByNumberUsesMaxMindIndex(t *testing.T) {
	dir := t.TempDir()
	blocksPath := filepath.Join(dir, asnBlocksIPv4Name)
	if err := os.WriteFile(blocksPath, []byte(`network,geoname_id,registered_country_geoname_id,represented_country_geoname_id,is_anonymous_proxy,is_satellite_provider,autonomous_system_number,autonomous_system_organization
31.223.39.0/24,,,,0,0,9121,TTNet
`), 0644); err != nil {
		t.Fatal(err)
	}

	g := New(filepath.Join(dir, "missing.mmdb"), "")
	g.asnPath = filepath.Join(dir, "missing.mmdb")
	g.reloadASNIndex()

	info, err := g.LookupASByNumber(context.Background(), 9121)
	if err != nil {
		t.Fatal(err)
	}
	if info.OrgName != "TTNet" {
		t.Errorf("org: got %q, want TTNet", info.OrgName)
	}
	if info.CountryCode != "TR" {
		t.Errorf("country: got %q, want TR (Cymru fallback when index lacks country)", info.CountryCode)
	}
}

func TestGeoIPDBReloadDisabled(t *testing.T) {
	g := New("", "")
	if err := g.Reload(); err != nil {
		t.Errorf("Reload on disabled db should succeed, got: %v", err)
	}
}

func TestReloadRebuildsASNNetworkBlocks(t *testing.T) {
	dir := t.TempDir()
	blocksPath := filepath.Join(dir, asnBlocksIPv4Name)
	if err := os.WriteFile(blocksPath, []byte(`network,geoname_id,registered_country_geoname_id,represented_country_geoname_id,is_anonymous_proxy,is_satellite_provider,autonomous_system_number,autonomous_system_organization
31.223.39.0/24,,,,0,0,9121,TTNet
`), 0644); err != nil {
		t.Fatal(err)
	}

	asnPath := filepath.Join(dir, "GeoLite2-ASN.mmdb")
	if err := os.WriteFile(asnPath, []byte("not-a-real-mmdb"), 0644); err != nil {
		t.Fatal(err)
	}

	g := New("", "")
	g.asnPath = asnPath
	g.cityPath = ""
	g.reloadASNIndex()
	if len(g.asnNetworkBlocks) == 0 {
		t.Fatal("expected blocks loaded before reload")
	}

	g.asnNetworkBlocks = nil
	if err := g.Reload(); err == nil {
		t.Fatal("expected reload error for invalid mmdb")
	}
	if len(g.asnNetworkBlocks) == 0 {
		t.Fatal("expected blocks rebuilt after reload even when mmdb open fails")
	}
}
