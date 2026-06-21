package geo

import (
	"net"
	"os"
	"path/filepath"
	"testing"

	"github.com/HopStat/HopStat/internal/domain"
)

func TestPickASNInfoPrefersBlockOverMMDB(t *testing.T) {
	block := domain.ASInfo{ASN: 9121, CountryCode: "TR", OrgName: "TTNet"}
	mm := domain.ASInfo{ASN: 4538, CountryCode: "CN", OrgName: "China Education and Research Network Center"}
	got := pickASNInfo(&mm, true, block, true)
	if got.ASN != 9121 {
		t.Fatalf("asn: got %d, want 9121", got.ASN)
	}
	if got.OrgName != "TTNet" {
		t.Fatalf("org: got %q", got.OrgName)
	}
}

func TestPickASNInfoFallsBackToMMDB(t *testing.T) {
	mm := domain.ASInfo{ASN: 15169, OrgName: "GOOGLE"}
	got := pickASNInfo(&mm, true, domain.ASInfo{}, false)
	if got.ASN != 15169 {
		t.Fatalf("asn: got %d, want 15169", got.ASN)
	}
}

func TestLookupLongestASNBlockTurkNetPrefix(t *testing.T) {
	dir := t.TempDir()
	locPath := filepath.Join(dir, cityLocationsName)
	if err := os.WriteFile(locPath, []byte(`geoname_id,locale_code,continent_code,continent_name,country_iso_code,country_name,is_in_european_union
6252001,en,AS,Asia,TR,Turkey,0
`), 0644); err != nil {
		t.Fatal(err)
	}
	blocksPath := filepath.Join(dir, asnBlocksIPv4Name)
	if err := os.WriteFile(blocksPath, []byte(`network,geoname_id,registered_country_geoname_id,represented_country_geoname_id,is_anonymous_proxy,is_satellite_provider,autonomous_system_number,autonomous_system_organization
31.223.39.0/24,6252001,,,0,0,9121,TTNet
0.0.0.0/0,,,,0,0,4538,China Education and Research Network Center
`), 0644); err != nil {
		t.Fatal(err)
	}

	countryByGeoname, err := loadCountryByGeoname(locPath)
	if err != nil {
		t.Fatal(err)
	}
	blocks, err := loadASNNetworkBlocks(blocksPath, countryByGeoname)
	if err != nil {
		t.Fatal(err)
	}

	info, ok := lookupLongestASNBlock(blocks, mustParseIP(t, "31.223.39.210"))
	if !ok {
		t.Fatal("expected block match")
	}
	if info.ASN != 9121 {
		t.Fatalf("asn: got %d, want 9121", info.ASN)
	}
	if info.CountryCode != "TR" {
		t.Fatalf("country: got %q, want TR", info.CountryCode)
	}

	mm := domain.ASInfo{ASN: 4538, CountryCode: "CN", OrgName: "China Education and Research Network Center"}
	got := pickASNInfo(&mm, true, info, true)
	if got.ASN != 9121 {
		t.Fatalf("picked asn: got %d, want 9121", got.ASN)
	}
}

func TestLookupLongestASNBlockTurkishPrefix(t *testing.T) {
	dir := t.TempDir()
	locPath := filepath.Join(dir, cityLocationsName)
	if err := os.WriteFile(locPath, []byte(`geoname_id,locale_code,continent_code,continent_name,country_iso_code,country_name,is_in_european_union
6252001,en,AS,Asia,TR,Turkey,0
`), 0644); err != nil {
		t.Fatal(err)
	}
	blocksPath := filepath.Join(dir, asnBlocksIPv4Name)
	if err := os.WriteFile(blocksPath, []byte(`network,geoname_id,registered_country_geoname_id,represented_country_geoname_id,is_anonymous_proxy,is_satellite_provider,autonomous_system_number,autonomous_system_organization
31.223.93.0/24,6252001,,,0,0,9121,Turk Telekom
0.0.0.0/0,,,,0,0,4134,CHINANET-BACKBONE
`), 0644); err != nil {
		t.Fatal(err)
	}

	countryByGeoname, err := loadCountryByGeoname(locPath)
	if err != nil {
		t.Fatal(err)
	}
	blocks, err := loadASNNetworkBlocks(blocksPath, countryByGeoname)
	if err != nil {
		t.Fatal(err)
	}

	info, ok := lookupLongestASNBlock(blocks, mustParseIP(t, "31.223.93.42"))
	if !ok {
		t.Fatal("expected block match")
	}
	if info.ASN != 9121 {
		t.Fatalf("asn: got %d, want 9121", info.ASN)
	}
	if info.OrgName != "Turk Telekom" {
		t.Fatalf("org: got %q", info.OrgName)
	}
	if info.CountryCode != "TR" {
		t.Fatalf("country: got %q, want TR", info.CountryCode)
	}
}

func TestMergeASInfoPreferExistingKeepsMaxMindCountry(t *testing.T) {
	base := &domain.ASInfo{ASN: 9121, CountryCode: "TR", OrgName: "Turk Telekom"}
	fallback := &domain.ASInfo{ASN: 4134, CountryCode: "CN", OrgName: "CHINANET-BACKBONE"}
	merged := mergeASInfoPreferExisting(base, fallback)
	if merged.CountryCode != "TR" {
		t.Fatalf("country: got %q, want TR", merged.CountryCode)
	}
	if merged.OrgName != "Turk Telekom" {
		t.Fatalf("org: got %q", merged.OrgName)
	}
}

func mustParseIP(t *testing.T, s string) net.IP {
	t.Helper()
	ip := net.ParseIP(s)
	if ip == nil {
		t.Fatalf("invalid ip %q", s)
	}
	return ip
}
