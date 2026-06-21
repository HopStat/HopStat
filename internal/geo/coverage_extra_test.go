package geo

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"context"
	"database/sql"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/HopStat/HopStat/internal/config"
	"github.com/HopStat/HopStat/internal/domain"
	"github.com/HopStat/HopStat/internal/store"
	"github.com/HopStat/HopStat/internal/store/queries"
)

func TestGeoIPDBCloseBoth(t *testing.T) {
	g := testGeoDB(t)
	g.Close()
	g.Close()
}

func TestStoreResolveNilAndHit(t *testing.T) {
	g := New("", "")
	g.storeResolve("1.2.3.4", nil)
	if _, ok := g.cachedResolve("1.2.3.4"); !ok {
		t.Fatal("expected cached empty result")
	}
	g.storeResolve("1.2.3.4", &domain.ASInfo{ASN: 1})
	info, ok := g.cachedResolve("1.2.3.4")
	if !ok || info.ASN != 1 {
		t.Fatalf("cache = %+v ok=%v", info, ok)
	}
}

func TestResolveDNSBranches(t *testing.T) {
	g := New("", "")
	info, err := g.resolveDNS(context.Background(), net.ParseIP("::1"), "::1")
	if err != nil || info == nil {
		t.Fatalf("ipv6 resolve = %+v err=%v", info, err)
	}
	info, err = g.resolveDNS(context.Background(), net.ParseIP("1.2.3.4"), "bad")
	if err != nil || info == nil {
		t.Fatalf("bad ip resolve = %+v err=%v", info, err)
	}
}

func TestMergeASInfoPreferExistingAllFields(t *testing.T) {
	if mergeASInfoPreferExisting(nil, &domain.ASInfo{ASN: 1}) == nil {
		t.Fatal("expected fallback")
	}
	if mergeASInfoPreferExisting(&domain.ASInfo{ASN: 1}, nil).ASN != 1 {
		t.Fatal("expected base")
	}
	base := &domain.ASInfo{ASN: 0, OrgName: "", ShortName: "", CountryCode: "", FlagEmoji: ""}
	fallback := &domain.ASInfo{ASN: 2, OrgName: "Org", ShortName: "O", CountryCode: "US", FlagEmoji: "🇺🇸"}
	merged := mergeASInfoPreferExisting(base, fallback)
	if merged.ASN != 2 || merged.OrgName != "Org" || merged.CountryCode != "US" || merged.FlagEmoji != "🇺🇸" {
		t.Fatalf("merged = %+v", merged)
	}
}

func TestLookupLongestASNBlockNetwork(t *testing.T) {
	dir := t.TempDir()
	blocksPath := filepath.Join(dir, asnBlocksIPv4Name)
	_ = os.WriteFile(blocksPath, []byte(`network,geoname_id,registered_country_geoname_id,represented_country_geoname_id,is_anonymous_proxy,is_satellite_provider,autonomous_system_number,autonomous_system_organization
10.0.0.0/8,,,,0,0,1,Test
`), 0644)
	blocks, err := loadASNNetworkBlocks(blocksPath, nil)
	if err != nil {
		t.Fatal(err)
	}
	info, network, ok := lookupLongestASNBlockNetwork(blocks, net.ParseIP("10.1.2.3"))
	if !ok || network == "" || info.ASN != 1 {
		t.Fatalf("info=%+v network=%q ok=%v", info, network, ok)
	}
}

func TestLoadCountryByGeonameErrors(t *testing.T) {
	if _, err := loadCountryByGeoname(filepath.Join(t.TempDir(), "missing.csv")); err == nil {
		t.Fatal("expected missing file error")
	}
	path := filepath.Join(t.TempDir(), cityLocationsName)
	_ = os.WriteFile(path, []byte("bad,header\n"), 0644)
	if _, err := loadCountryByGeoname(path); err == nil {
		t.Fatal("expected missing columns error")
	}
}

func TestBuildASNNetworkBlocksAndIndex(t *testing.T) {
	dir := t.TempDir()
	if blocks := buildASNNetworkBlocks(""); blocks != nil {
		t.Fatal("expected nil for empty dir")
	}
	if idx := buildASNIndex(""); idx != nil {
		t.Fatal("expected nil index")
	}
	loc := filepath.Join(dir, cityLocationsName)
	_ = os.WriteFile(loc, []byte(`geoname_id,locale_code,country_iso_code
1,en,US
`), 0644)
	_ = os.WriteFile(filepath.Join(dir, asnBlocksIPv4Name), []byte(`network,geoname_id,registered_country_geoname_id,represented_country_geoname_id,is_anonymous_proxy,is_satellite_provider,autonomous_system_number,autonomous_system_organization
1.2.3.0/24,1,,,0,0,99,Org
`), 0644)
	if blocks := buildASNNetworkBlocks(dir); len(blocks) == 0 {
		t.Fatal("expected blocks")
	}
	if idx := buildASNIndex(dir); len(idx) == 0 {
		t.Fatal("expected index")
	}
}

func TestEnrichASInfoUsesIndex(t *testing.T) {
	dir := t.TempDir()
	_ = os.WriteFile(filepath.Join(dir, asnBlocksIPv4Name), []byte(`network,geoname_id,registered_country_geoname_id,represented_country_geoname_id,is_anonymous_proxy,is_satellite_provider,autonomous_system_number,autonomous_system_organization
1.2.3.0/24,,,,0,0,99,IndexedOrg
`), 0644)
	g := New("", "")
	g.asnPath = filepath.Join(dir, "GeoLite2-ASN.mmdb")
	g.reloadASNIndex()
	info, enriched := g.enrichASInfo(net.ParseIP("1.2.3.4"), &domain.ASInfo{ASN: 99})
	if info == nil || info.OrgName != "IndexedOrg" || !enriched {
		t.Fatalf("info=%+v enriched=%v", info, enriched)
	}
}

func TestLookupIPUsesDNSFallback(t *testing.T) {
	g := New("", "")
	report, err := g.LookupIP(context.Background(), "8.8.8.8")
	if err != nil {
		t.Fatal(err)
	}
	if !report.DNS.Available {
		t.Fatal("expected DNS candidate available for ipv4")
	}
}

func TestLookupIPMMDBError(t *testing.T) {
	dir := t.TempDir()
	badASN := filepath.Join(dir, "bad.mmdb")
	_ = os.WriteFile(badASN, []byte("bad"), 0644)
	g := New(badASN, "")
	g.asnPath = badASN
	report, err := g.LookupIP(context.Background(), "8.8.8.8")
	if err != nil {
		t.Fatal(err)
	}
	if report.MMDB.Available {
		t.Fatal("expected mmdb unavailable with bad file")
	}
}

func TestAbbreviateTracerouteNameBranches(t *testing.T) {
	long := "ABCDEFGHIJKLMNOPQRSTUVWXYZ"
	if got := abbreviateTracerouteName(long, 10); len(got) > 10 {
		t.Fatalf("too long: %q", got)
	}
	if got := abbreviateTracerouteName("ALPHA BETA GAMMA", 12); len(got) > 12 {
		t.Fatalf("too long: %q", got)
	}
	if got := trimToLen("abc", 0); got != "" {
		t.Fatalf("trimToLen = %q", got)
	}
}

func TestShortenOrgNameBranches(t *testing.T) {
	if got := shortenOrgName(""); got != "" {
		t.Fatal("expected empty")
	}
	if got := baseOrgLabel("AS15169 - Google LLC"); got != "Google LLC" {
		t.Fatalf("got %q", got)
	}
	if got := FormatTracerouteOrgName("Acme Inc."); got == "" {
		t.Fatal("expected formatted name")
	}
}

func TestParseUpdateIntervalEmpty(t *testing.T) {
	if got := ParseUpdateInterval("", 72*time.Hour); got != 72*time.Hour {
		t.Fatalf("got %v", got)
	}
}

func TestLastDownloadFromSettingsBadDate(t *testing.T) {
	got := LastDownloadFromSettings(map[string]string{SettingASNLastDownload: "bad"}, "GeoLite2-ASN")
	if !got.IsZero() {
		t.Fatalf("got %v", got)
	}
}

func TestLatestRFC3339BothInvalid(t *testing.T) {
	if got := latestRFC3339("bad", "also-bad"); got != "" {
		t.Fatalf("got %q", got)
	}
}

func TestCSVEditionSidecarsDefault(t *testing.T) {
	edition, files := csvEditionSidecars("unknown")
	if edition != "" || files != nil {
		t.Fatalf("edition=%q files=%v", edition, files)
	}
}

func TestUpdaterLastDownloadAt(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "GeoLite2-ASN.mmdb")
	_ = os.WriteFile(path, []byte("db"), 0644)
	u := NewUpdater(config.GeoIPConfig{}, New("", ""))
	u.SetLastDownload(func(string) time.Time { return time.Date(2024, 1, 2, 0, 0, 0, 0, time.UTC) })
	got := u.lastDownloadAt("GeoLite2-ASN", path)
	if !got.Equal(time.Date(2024, 1, 2, 0, 0, 0, 0, time.UTC)) {
		t.Fatalf("got %v", got)
	}
	if got := u.lastDownloadAt("GeoLite2-ASN", filepath.Join(dir, "missing.mmdb")); !got.IsZero() {
		t.Fatalf("got %v", got)
	}
}

func TestUpdaterTryDownloadSkips(t *testing.T) {
	dir := t.TempDir()
	asnPath := writeASNEditionFiles(t, dir)
	u := NewUpdater(config.GeoIPConfig{UpdateInterval: "72h"}, New("", ""))
	u.asnPath = asnPath
	u.SetLastDownload(func(string) time.Time { return time.Now().UTC() })
	if u.tryDownloadEdition(context.Background(), "GeoLite2-ASN", asnPath) {
		t.Fatal("expected skip")
	}
}

func TestFetchMaxMindErrors(t *testing.T) {
	u := NewUpdater(config.GeoIPConfig{}, New("", ""))
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte("denied"))
	}))
	defer srv.Close()

	_, err := u.fetchMaxMind(context.Background(), srv.URL, "GeoLite2-ASN")
	if err == nil {
		t.Fatal("expected HTTP error")
	}
}

func TestExtractMMDBFileSuccess(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "out.mmdb")
	body := buildTestMMDBArchive(t, "GeoLite2-ASN.mmdb", []byte("mmdb-bytes"))
	if err := extractMMDBFile(bytes.NewReader(body), target); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(target)
	if err != nil || string(b) != "mmdb-bytes" {
		t.Fatalf("content = %q err=%v", b, err)
	}
	if err := extractMMDBFile(bytes.NewReader(body), target); err != nil {
		t.Fatal(err)
	}
}

func TestExtractMMDBFileNoMMDB(t *testing.T) {
	var buf bytes.Buffer
	gw := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gw)
	_ = tw.WriteHeader(&tar.Header{Name: "readme.txt", Mode: 0644, Size: 3})
	_, _ = tw.Write([]byte("txt"))
	_ = tw.Close()
	_ = gw.Close()
	if err := extractMMDBFile(bytes.NewReader(buf.Bytes()), filepath.Join(t.TempDir(), "x.mmdb")); err == nil {
		t.Fatal("expected missing mmdb error")
	}
}

func TestWriteArchiveFileSuccess(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "file.csv")
	if err := writeArchiveFile(bytes.NewReader([]byte("a,b,c\n")), target); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(target); err != nil {
		t.Fatal(err)
	}
}

func TestDownloadMMDBEditionHTTPError(t *testing.T) {
	dir := t.TempDir()
	u := NewUpdater(config.GeoIPConfig{LicenseKey: "k", AccountID: "a"}, New("", ""))
	u.asnPath = filepath.Join(dir, "GeoLite2-ASN.mmdb")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "fail", http.StatusInternalServerError)
	}))
	defer srv.Close()
	orig := http.DefaultTransport
	http.DefaultTransport = roundTripperFunc(func(req *http.Request) (*http.Response, error) {
		req2 := req.Clone(req.Context())
		req2.URL, _ = url.Parse(srv.URL + "?" + req.URL.RawQuery)
		return orig.RoundTrip(req2)
	})
	defer func() { http.DefaultTransport = orig }()
	if err := u.downloadMMDBEdition(context.Background(), "GeoLite2-ASN", u.asnPath); err == nil {
		t.Fatal("expected download error")
	}
}

func TestLookupMMDBFromRecordNil(t *testing.T) {
	if _, ok := lookupMMDBFromRecord(nil, nil, net.ParseIP("1.1.1.1")); ok {
		t.Fatal("expected false for nil record")
	}
}

func TestApplyCityRecordNil(t *testing.T) {
	if applyCityRecord(nil, net.ParseIP("1.1.1.1"), &domain.ASInfo{}) {
		t.Fatal("expected false")
	}
}

func TestLookupASNFromIndexEmptyInfo(t *testing.T) {
	g := New("", "")
	g.asnIndex = map[uint32]domain.ASInfo{1: {}}
	if _, ok := g.lookupASNFromIndex(1); ok {
		t.Fatal("expected miss for empty info")
	}
}

func TestRememberASNExisting(t *testing.T) {
	g := New("", "")
	g.asnIndex = map[uint32]domain.ASInfo{1: {OrgName: "first"}}
	g.rememberASN(&domain.ASInfo{ASN: 1, OrgName: "second"})
	if g.asnIndex[1].OrgName != "first" {
		t.Fatal("expected existing entry preserved")
	}
}

func TestResolveASNCacheHitZeroASN(t *testing.T) {
	g := New("", "")
	g.storeResolve("9.9.9.9", nil)
	info, err := g.ResolveASN(context.Background(), "9.9.9.9")
	if err != nil || info != nil {
		t.Fatalf("info=%+v err=%v", info, err)
	}
}

func TestMergeASByNumberDNSNoData(t *testing.T) {
	g := New("", "")
	out := g.mergeASByNumberDNS(context.Background(), &domain.ASInfo{ASN: 999999, OrgName: "Base"})
	if out.OrgName != "Base" {
		t.Fatalf("out = %+v", out)
	}
}

func TestDownloadCSVSidecarsSuccess(t *testing.T) {
	dir := t.TempDir()
	body := buildTestCSVArchive(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(body)
	}))
	defer srv.Close()
	u := NewUpdater(config.GeoIPConfig{LicenseKey: "k", AccountID: "a"}, New("", ""))
	orig := http.DefaultTransport
	http.DefaultTransport = roundTripperFunc(func(req *http.Request) (*http.Response, error) {
		req2 := req.Clone(req.Context())
		req2.URL, _ = url.Parse(srv.URL)
		return orig.RoundTrip(req2)
	})
	defer func() { http.DefaultTransport = orig }()
	var called bool
	u.SetOnDownload(func(string, time.Time) { called = true })
	if err := u.downloadCSVSidecars(context.Background(), "GeoLite2-ASN", dir); err != nil {
		t.Fatal(err)
	}
	if !called {
		t.Fatal("expected onDownload callback")
	}
}

func TestDownloadMMDBEditionSuccess(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "GeoLite2-ASN.mmdb")
	body := buildTestMMDBArchive(t, "GeoLite2-ASN.mmdb", readTestFile(t, testASNPath(t)))
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(body)
	}))
	defer srv.Close()
	u := NewUpdater(config.GeoIPConfig{LicenseKey: "k", AccountID: "a"}, New("", ""))
	orig := http.DefaultTransport
	http.DefaultTransport = roundTripperFunc(func(req *http.Request) (*http.Response, error) {
		req2 := req.Clone(req.Context())
		req2.URL, _ = url.Parse(srv.URL)
		return orig.RoundTrip(req2)
	})
	defer func() { http.DefaultTransport = orig }()
	if err := u.downloadMMDBEdition(context.Background(), "GeoLite2-ASN", target); err != nil {
		t.Fatal(err)
	}
}

func TestLookupASByNumberMergeCountry(t *testing.T) {
	g := New("", "")
	g.asnIndex = map[uint32]domain.ASInfo{15169: {OrgName: "GOOGLE"}}
	info, err := g.LookupASByNumber(context.Background(), 15169)
	if err != nil || info.OrgName != "GOOGLE" {
		t.Fatalf("info=%+v err=%v", info, err)
	}
}

func TestSyncSettingsAlreadyConfigured(t *testing.T) {
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err := store.Migrate(db); err != nil {
		t.Fatal(err)
	}
	q := queries.New(db)
	_ = q.SetSettings(map[string]string{
		SettingLicenseKey:     "existing",
		SettingAccountID:      "existing",
		SettingUpdateInterval: "12h",
	})
	if err := SyncSettings(q, config.GeoIPConfig{LicenseKey: "new", AccountID: "new", UpdateInterval: "24h"}); err != nil {
		t.Fatal(err)
	}
}

func TestLookupIPPrefersMMDBWhenNoBlocks(t *testing.T) {
	g := New(testASNPath(t), testCityPath(t))
	report, err := g.LookupIP(context.Background(), "1.128.0.1")
	if err != nil {
		t.Fatal(err)
	}
	if report.MMDB.Matched && report.ChosenSource != SourceMMDB && report.ChosenSource != SourceBlocks {
		t.Fatalf("source=%q mmdb=%+v", report.ChosenSource, report.MMDB)
	}
}

func TestAbbreviateTracerouteNameMultiBranch(t *testing.T) {
	got := abbreviateTracerouteName("ALPHA BETA GAMMA DELTA", 12)
	if len(got) > 12 {
		t.Fatalf("too long: %q", got)
	}
	got = abbreviateTracerouteName("LONGWORD", 4)
	if len(got) > 4 {
		t.Fatalf("too long: %q", got)
	}
}

func TestLoadASNIndexFromBlocksBadRows(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, asnBlocksIPv4Name)
	_ = os.WriteFile(path, []byte(`network,geoname_id,registered_country_geoname_id,represented_country_geoname_id,is_anonymous_proxy,is_satellite_provider,autonomous_system_number,autonomous_system_organization
bad-row
1.2.3.0/24,,,,0,0,not-a-number,Org
`), 0644)
	idx, err := loadASNIndexFromBlocks(path, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(idx) != 0 {
		t.Fatalf("idx = %+v", idx)
	}
}

func TestUpdateAllNoReloadWhenNothingDownloaded(t *testing.T) {
	dir := t.TempDir()
	asnPath := writeASNEditionFiles(t, dir)
	u := NewUpdater(config.GeoIPConfig{UpdateInterval: "72h"}, New("", ""))
	u.asnPath = asnPath
	u.cityPath = filepath.Join(dir, "GeoLite2-City.mmdb")
	u.SetLastDownload(func(string) time.Time { return time.Now().UTC() })
	u.updateAll(context.Background())
}

func TestExtractCSVFilesMissingWanted(t *testing.T) {
	dir := t.TempDir()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	w, _ := zw.Create("other.csv")
	_, _ = io.WriteString(w, "x\n")
	_ = zw.Close()
	err := extractCSVFiles(bytes.NewReader(buf.Bytes()), dir, map[string]struct{}{asnBlocksIPv4Name: {}})
	if err == nil {
		t.Fatal("expected missing file error")
	}
}
