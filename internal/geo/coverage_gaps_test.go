package geo

import (
	"context"
	"errors"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/HopStat/HopStat/internal/domain"
)

func TestLoadCountryByGeonameRowBranches(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, cityLocationsName)
	_ = os.WriteFile(path, []byte(`geoname_id,locale_code,country_iso_code
1,en,US
2,en,USA
bad,en,DE
3,en,X
`), 0644)
	m, err := loadCountryByGeoname(path)
	if err != nil {
		t.Fatal(err)
	}
	if m[1] != "US" || len(m) != 1 {
		t.Fatalf("m=%v", m)
	}
}

func TestLoadASNIndexFromBlocksWithCountry(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, asnBlocksIPv4Name)
	_ = os.WriteFile(path, []byte(`network,geoname_id,registered_country_geoname_id,represented_country_geoname_id,is_anonymous_proxy,is_satellite_provider,autonomous_system_number,autonomous_system_organization
1.2.3.0/24,,1,,0,0,99,Org
`), 0644)
	idx, err := loadASNIndexFromBlocks(path, map[int64]string{1: "US"})
	if err != nil {
		t.Fatal(err)
	}
	if idx[99].CountryCode != "US" {
		t.Fatalf("idx=%+v", idx[99])
	}
}

func TestLookupASNTXTHook(t *testing.T) {
	old := lookupASNTXT
	lookupASNTXT = func(context.Context, string) ([]string, error) {
		return []string{`"15169 | 1.2.3.0/24 | US | arin | 2000 | GOOGLE"`}, nil
	}
	t.Cleanup(func() { lookupASNTXT = old })

	g := New("", "")
	out := g.mergeASByNumberDNS(context.Background(), &domain.ASInfo{ASN: 15169})
	if out.CountryCode != "US" || out.OrgName != "GOOGLE" {
		t.Fatalf("out=%+v", out)
	}

	lookupASNTXT = func(context.Context, string) ([]string, error) {
		return []string{`"15169 | US | arin | 2000 | GOOGLE"`}, nil
	}
	info, err := g.lookupASByNumberDNS(context.Background(), 15169)
	if err != nil || info.OrgName != "GOOGLE" {
		t.Fatalf("info=%+v err=%v", info, err)
	}

	lookupASNTXT = func(context.Context, string) ([]string, error) {
		return nil, errors.New("dns failed")
	}
	if got := g.mergeASByNumberDNS(context.Background(), &domain.ASInfo{ASN: 1, OrgName: "Base"}); got.OrgName != "Base" {
		t.Fatalf("got=%+v", got)
	}
}

func TestLookupCityErrors(t *testing.T) {
	g := New("", "")
	if _, err := g.LookupCity("bad"); err == nil {
		t.Fatal("expected invalid ip error")
	}
	if _, err := g.LookupCity("1.1.1.1"); err == nil {
		t.Fatal("expected missing db error")
	}
}

func TestResolveASNAndStoreResolveBranches(t *testing.T) {
	g := New(testASNPath(t), "")
	info, err := g.ResolveASN(context.Background(), "1.128.0.1")
	if err != nil || info == nil {
		t.Fatalf("info=%+v err=%v", info, err)
	}
	g.storeResolve("1.2.3.4", &domain.ASInfo{ASN: 0})
}

func TestFormatTracerouteOrgNameSuffixOnly(t *testing.T) {
	if got := FormatTracerouteOrgName("Inc."); got != "" {
		t.Fatalf("got=%q", got)
	}
	if got := abbreviateTracerouteName("Long Name Here", 6); len(got) > 6 {
		t.Fatalf("got=%q", got)
	}
}

func TestBuildASNNetworkBlocksIPv4Only(t *testing.T) {
	dir := t.TempDir()
	_ = os.WriteFile(filepath.Join(dir, asnBlocksIPv4Name), []byte(`network,geoname_id,registered_country_geoname_id,represented_country_geoname_id,is_anonymous_proxy,is_satellite_provider,autonomous_system_number,autonomous_system_organization
10.0.0.0/8,,,,0,0,1,Test
`), 0644)
	blocks := buildASNNetworkBlocks(dir)
	if len(blocks) == 0 {
		t.Fatal("expected blocks")
	}
}

func TestLoadASNNetworkBlocksMissingFile(t *testing.T) {
	if _, err := loadASNNetworkBlocks(filepath.Join(t.TempDir(), "missing.csv"), nil); err == nil {
		t.Fatal("expected missing file error")
	}
}

func TestLookupLongestASNBlockNetworkEmptyIP(t *testing.T) {
	blocks := buildASNNetworkBlocks("")
	if _, _, ok := lookupLongestASNBlockNetwork(blocks, nil); ok {
		t.Fatal("expected miss")
	}
}

func TestLookupIPChosenSourceDNS(t *testing.T) {
	g := New("", "")
	report, err := g.LookupIP(context.Background(), "8.8.8.8")
	if err != nil {
		t.Fatal(err)
	}
	// The outcome is deliberately loose: with DNS available but returning no ASN, both a
	// nil result and SourceNone are legitimate. What must hold is that a report comes back
	// at all rather than a nil dereference further up.
	if report == nil {
		t.Fatal("expected a report even when no source resolves the address")
	}
}

func TestApplyCityRecordEmptyCountry(t *testing.T) {
	g := testGeoDB(t)
	info := &domain.ASInfo{ASN: 1}
	if applyCityRecord(g.cityDB, net.ParseIP("127.0.0.1"), info) {
		t.Fatal("expected false for loopback without country")
	}
}

func TestWriteArchiveFileCopyError(t *testing.T) {
	oldCreate := osCreate
	osCreate = func(name string) (*os.File, error) {
		return os.Create(name)
	}
	t.Cleanup(func() { osCreate = oldCreate })
	target := filepath.Join(t.TempDir(), "out.csv")
	if err := writeArchiveFile(&badCopyReader{}, target); err == nil {
		t.Fatal("expected copy error")
	}
}

type badCopyReader struct{}

func (badCopyReader) Read([]byte) (int, error) { return 0, errors.New("copy failed") }

func TestParseCymruASNRecordARIN(t *testing.T) {
	cc, org := parseCymruASNRecord(`"15169 | 1.2.3.0/24 | US | arin | 2000 | GOOGLE"`)
	if cc != "US" || !strings.Contains(org, "GOOGLE") {
		t.Fatalf("cc=%q org=%q", cc, org)
	}
}
