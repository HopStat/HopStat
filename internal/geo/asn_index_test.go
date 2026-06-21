package geo

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadASNIndexFromBlocks(t *testing.T) {
	dir := t.TempDir()
	locPath := filepath.Join(dir, cityLocationsName)
	if err := os.WriteFile(locPath, []byte(`geoname_id,locale_code,continent_code,continent_name,country_iso_code,country_name,is_in_european_union
6252001,en,NA,"North America",US,"United States",0
`), 0644); err != nil {
		t.Fatal(err)
	}
	blocksPath := filepath.Join(dir, asnBlocksIPv4Name)
	if err := os.WriteFile(blocksPath, []byte(`network,geoname_id,registered_country_geoname_id,represented_country_geoname_id,is_anonymous_proxy,is_satellite_provider,autonomous_system_number,autonomous_system_organization
1.1.1.0/24,,6252001,,0,0,13335,CLOUDFLARENET
`), 0644); err != nil {
		t.Fatal(err)
	}

	countryByGeoname, err := loadCountryByGeoname(locPath)
	if err != nil {
		t.Fatal(err)
	}
	idx, err := loadASNIndexFromBlocks(blocksPath, countryByGeoname)
	if err != nil {
		t.Fatal(err)
	}
	info, ok := idx[13335]
	if !ok {
		t.Fatal("expected AS13335 in index")
	}
	if info.OrgName != "CLOUDFLARENET" {
		t.Errorf("org: got %q", info.OrgName)
	}
	if info.CountryCode != "US" {
		t.Errorf("country: got %q, want US", info.CountryCode)
	}
}

func TestBuildASNIndexEmptyDir(t *testing.T) {
	if idx := buildASNIndex(t.TempDir()); idx != nil {
		t.Error("expected nil index for empty dir")
	}
}
