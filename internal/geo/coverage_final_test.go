package geo

import (
	"context"
	"database/sql"
	"errors"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/HopStat/HopStat/internal/config"
	"github.com/HopStat/HopStat/internal/domain"
	"github.com/HopStat/HopStat/internal/store"
	"github.com/HopStat/HopStat/internal/store/queries"
)

func TestBuildASNNetworkBlocksIPv6AndMissingFiles(t *testing.T) {
	dir := t.TempDir()
	_ = os.WriteFile(filepath.Join(dir, asnBlocksIPv6Name), []byte(`network,geoname_id,registered_country_geoname_id,represented_country_geoname_id,is_anonymous_proxy,is_satellite_provider,autonomous_system_number,autonomous_system_organization
2001:db8::/32,,,,0,0,65000,Lab
`), 0644)
	blocks := buildASNNetworkBlocks(dir)
	if len(blocks) == 0 {
		t.Fatal("expected ipv6 blocks")
	}
}

func TestCountryFromGeonameRow(t *testing.T) {
	m := map[int64]string{1: "US"}
	row := []string{"1.2.3.0/24", "1", "", "", "0", "0", "99", "Org"}
	if cc := countryFromGeonameRow(row, 1, -1, m); cc != "US" {
		t.Fatalf("cc=%q", cc)
	}
	if cc := countryFromGeonameRow(row, 1, -1, nil); cc != "" {
		t.Fatalf("cc=%q", cc)
	}
	if cc := countryFromGeonameRow([]string{"x"}, 0, -1, m); cc != "" {
		t.Fatalf("cc=%q", cc)
	}
}

func TestReloadBothDBsFail(t *testing.T) {
	dir := t.TempDir()
	asnPath := filepath.Join(dir, "asn.mmdb")
	cityPath := filepath.Join(dir, "city.mmdb")
	_ = os.WriteFile(asnPath, []byte("bad"), 0644)
	_ = os.WriteFile(cityPath, []byte("bad"), 0644)
	g := New("", "")
	g.asnPath = asnPath
	g.cityPath = cityPath
	if err := g.Reload(); err == nil {
		t.Fatal("expected reload errors")
	}
}

func TestNeedsDownloadZeroLast(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "GeoLite2-ASN.mmdb")
	_ = os.WriteFile(path, []byte("db"), 0644)
	u := NewUpdater(config.GeoIPConfig{UpdateInterval: "72h"}, New("", ""))
	should, _, _ := u.needsDownload("GeoLite2-ASN", path)
	if !should {
		t.Fatal("expected download when last is zero")
	}
}

func TestRunUpdaterMkdirError(t *testing.T) {
	u := NewUpdater(config.GeoIPConfig{}, New("", ""))
	u.asnPath = string([]byte{0})
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	u.Run(ctx)
}

func TestLookupASByNumberFromDNSOnly(t *testing.T) {
	g := New("", "")
	info, err := g.LookupASByNumber(context.Background(), 15169)
	if err != nil || info.ASN != 15169 {
		t.Fatalf("info=%+v err=%v", info, err)
	}
}

func TestMergeASByNumberDNSNilFallback(t *testing.T) {
	g := New("", "")
	out := g.mergeASByNumberDNS(context.Background(), &domain.ASInfo{ASN: 15169, OrgName: "GOOGLE"})
	if out.OrgName != "GOOGLE" {
		t.Fatalf("out=%+v", out)
	}
}

func TestCollectStatusFileModFallback(t *testing.T) {
	dir := t.TempDir()
	cityPath := filepath.Join(dir, "GeoLite2-City.mmdb")
	_ = os.WriteFile(cityPath, readTestFile(t, testCityPath(t)), 0644)
	settings := map[string]string{SettingLicenseKey: "k", SettingAccountID: "a"}
	st := CollectStatus(settings, config.GeoIPConfig{CityDBPath: cityPath}, testGeoDB(t))
	if st.CityLastDownload == "" {
		t.Fatalf("status=%+v", st)
	}
}

func TestLastDownloadFromSettingsEmptyEdition(t *testing.T) {
	if !LastDownloadFromSettings(map[string]string{}, "GeoLite2-ASN").IsZero() {
		t.Fatal("expected zero")
	}
}

func TestUpdaterResolveIntervalCustom(t *testing.T) {
	u := NewUpdater(config.GeoIPConfig{}, New("", ""))
	u.SetUpdateInterval(func() time.Duration { return 2 * time.Hour })
	if got := u.resolveInterval(); got != 2*time.Hour {
		t.Fatalf("got %v", got)
	}
}

func TestFormatTracerouteOrgNameBranches(t *testing.T) {
	if got := FormatTracerouteOrgName("Example Corp Inc."); got == "" {
		t.Fatal("expected formatted name")
	}
	if got := FormatTracerouteOrgName("Very Long Organization Name Here"); len(got) > tracerouteOrgMaxLen {
		t.Fatalf("too long: %q", got)
	}
	if got := abbreviateTracerouteName("Alpha Beta", 8); len(got) > 8 {
		t.Fatalf("too long: %q", got)
	}
	if got := abbreviateTracerouteName("ABCDEFGHIJKLMNOP", 10); len(got) > 10 {
		t.Fatalf("too long: %q", got)
	}
}

func TestMergeASByNumberDNSWithCountry(t *testing.T) {
	g := New("", "")
	out := g.mergeASByNumberDNS(context.Background(), &domain.ASInfo{ASN: 15169, OrgName: "GOOGLE"})
	if out.CountryCode == "" && out.OrgName != "GOOGLE" {
		t.Fatalf("out=%+v", out)
	}
}

func TestLookupASByNumberZero(t *testing.T) {
	g := New("", "")
	info, err := g.LookupASByNumber(context.Background(), 0)
	if err != nil || info == nil {
		t.Fatalf("info=%+v err=%v", info, err)
	}
}

func TestLookupASByNumberIndexWithCountry(t *testing.T) {
	g := New("", "")
	g.asnIndex = map[uint32]domain.ASInfo{1: {ASN: 1, OrgName: "Test", CountryCode: "US"}}
	info, err := g.LookupASByNumber(context.Background(), 1)
	if err != nil || info.CountryCode != "US" {
		t.Fatalf("info=%+v err=%v", info, err)
	}
}

func TestParseCymruASNRecordShort(t *testing.T) {
	cc, org := parseCymruASNRecord("15169")
	if cc != "" || org != "" {
		t.Fatalf("cc=%q org=%q", cc, org)
	}
	cc, org = parseCymruASNRecord("15169 | US")
	if cc != "US" || org != "" {
		t.Fatalf("cc=%q org=%q", cc, org)
	}
}

func TestCountryFromOrgSuffix(t *testing.T) {
	if cc := countryFromOrgSuffix("Example, US"); cc != "US" {
		t.Fatalf("cc=%q", cc)
	}
	if cc := countryFromOrgSuffix("Example, USA"); cc != "" {
		t.Fatalf("cc=%q", cc)
	}
}

func TestSyncSettingsEmptyConfig(t *testing.T) {
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err := store.Migrate(db); err != nil {
		t.Fatal(err)
	}
	if err := SyncSettings(queries.New(db), config.GeoIPConfig{}); err != nil {
		t.Fatal(err)
	}
}

func TestSyncSettingsPopulatesEmpty(t *testing.T) {
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err := store.Migrate(db); err != nil {
		t.Fatal(err)
	}
	if err := SyncSettings(queries.New(db), config.GeoIPConfig{
		LicenseKey: "key", AccountID: "acct", UpdateInterval: "24h",
	}); err != nil {
		t.Fatal(err)
	}
}

func TestWriteArchiveFileCloseAndRenameErrors(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "file.csv")

	oldCreate := osCreate
	osCreate = func(string) (*os.File, error) { return nil, errors.New("create failed") }
	if err := writeArchiveFile(strings.NewReader("x"), target); err == nil {
		t.Fatal("expected create error")
	}
	osCreate = oldCreate

	oldRename := osRename
	osRename = func(string, string) error { return errors.New("rename failed") }
	if err := writeArchiveFile(strings.NewReader("x"), target); err == nil {
		t.Fatal("expected rename error")
	}
	osRename = oldRename
}

func TestChosenSourceAndInfer(t *testing.T) {
	block := &domain.ASInfo{ASN: 1}
	mm := &domain.ASInfo{ASN: 2}
	if got := chosenSource(true, false, block, block, mm); got != SourceBlocks {
		t.Fatalf("got %q", got)
	}
	if got := chosenSource(false, true, mm, block, mm); got != SourceMMDB {
		t.Fatalf("got %q", got)
	}
	if got := inferChosenSource(false, false, true); got != SourceDNS {
		t.Fatalf("got %q", got)
	}
}

func TestLoadASNNetworkBlocksMissingColumns(t *testing.T) {
	path := filepath.Join(t.TempDir(), "blocks.csv")
	_ = os.WriteFile(path, []byte("bad,header\n"), 0644)
	if _, err := loadASNNetworkBlocks(path, nil); err == nil {
		t.Fatal("expected missing columns error")
	}
}

func TestCountryFromGeonameRowRegistered(t *testing.T) {
	m := map[int64]string{2: "DE"}
	row := []string{"1.2.3.0/24", "", "2", "", "0", "0", "99", "Org"}
	if cc := countryFromGeonameRow(row, 1, 2, m); cc != "DE" {
		t.Fatalf("cc=%q", cc)
	}
}

func TestLookupLongestASNBlockNetworkMiss(t *testing.T) {
	_, network, ok := lookupLongestASNBlockNetwork(nil, net.ParseIP("1.1.1.1"))
	if ok || network != "" {
		t.Fatalf("ok=%v network=%q", ok, network)
	}
}

func TestRunUpdaterSuccessTick(t *testing.T) {
	dir := t.TempDir()
	asnPath := writeASNEditionFiles(t, dir)
	u := NewUpdater(config.GeoIPConfig{UpdateInterval: "1ms"}, New("", ""))
	u.asnPath = asnPath
	u.cityPath = filepath.Join(dir, "GeoLite2-City.mmdb")
	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		time.Sleep(20 * time.Millisecond)
		cancel()
	}()
	u.Run(ctx)
}
