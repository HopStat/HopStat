package geo

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/HopStat/HopStat/internal/config"
	"github.com/HopStat/HopStat/internal/domain"
	"github.com/HopStat/HopStat/internal/store/queries"
	"github.com/oschwald/geoip2-golang"
)

func TestLoadCountryByGeonameHeaderReadError(t *testing.T) {
	path := filepath.Join(t.TempDir(), cityLocationsName)
	if err := os.WriteFile(path, []byte(""), 0644); err != nil {
		t.Fatal(err)
	}
	if _, err := loadCountryByGeoname(path); err == nil {
		t.Fatal("expected header read error")
	}
}

func TestLoadCountryByGeonameRowReadError(t *testing.T) {
	path := filepath.Join(t.TempDir(), cityLocationsName)
	content := "geoname_id,locale_code,country_iso_code\n1,en,US\n\"unclosed\n"
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
	if _, err := loadCountryByGeoname(path); err != nil {
		t.Fatal(err)
	}
}

func TestLoadASNIndexFromBlocksHeaderError(t *testing.T) {
	path := filepath.Join(t.TempDir(), asnBlocksIPv4Name)
	if err := os.WriteFile(path, []byte(""), 0644); err != nil {
		t.Fatal(err)
	}
	if _, err := loadASNIndexFromBlocks(path, nil); err == nil {
		t.Fatal("expected header error")
	}
}

func TestLoadASNIndexFromBlocksDuplicateASN(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, asnBlocksIPv4Name)
	_ = os.WriteFile(path, []byte(`network,geoname_id,registered_country_geoname_id,represented_country_geoname_id,is_anonymous_proxy,is_satellite_provider,autonomous_system_number,autonomous_system_organization
1.2.3.0/24,,,,0,0,99,First
2.0.0.0/8,,,,0,0,99,Second
`), 0644)
	idx, err := loadASNIndexFromBlocks(path, nil)
	if err != nil || idx[99].OrgName != "First" {
		t.Fatalf("idx=%+v err=%v", idx[99], err)
	}
}

func TestLoadASNIndexFromBlocksRegisteredCountry(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, asnBlocksIPv4Name)
	_ = os.WriteFile(path, []byte(`network,geoname_id,registered_country_geoname_id,represented_country_geoname_id,is_anonymous_proxy,is_satellite_provider,autonomous_system_number,autonomous_system_organization
1.2.3.0/24,,1,,0,0,99,Org
`), 0644)
	idx, err := loadASNIndexFromBlocks(path, map[int64]string{1: "US"})
	if err != nil || idx[99].CountryCode != "US" {
		t.Fatalf("idx=%+v err=%v", idx[99], err)
	}
}

func TestLoadASNNetworkBlocksHeaderError(t *testing.T) {
	path := filepath.Join(t.TempDir(), asnBlocksIPv4Name)
	if err := os.WriteFile(path, []byte(""), 0644); err != nil {
		t.Fatal(err)
	}
	if _, err := loadASNNetworkBlocks(path, nil); err == nil {
		t.Fatal("expected header error")
	}
}

func TestLoadASNNetworkBlocksSkipBadRows(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, asnBlocksIPv4Name)
	_ = os.WriteFile(path, []byte(`network,geoname_id,registered_country_geoname_id,represented_country_geoname_id,is_anonymous_proxy,is_satellite_provider,autonomous_system_number,autonomous_system_organization
,,,,0,0,1,Org
bad-cidr,,,,0,0,2,Org
1.2.3.0/24,,,,0,0,,Org
1.2.3.0/24,,,,0,0,notnum,Org
10.0.0.0/8,,,,0,0,3,Good
`), 0644)
	blocks, err := loadASNNetworkBlocks(path, nil)
	if err != nil || len(blocks) != 1 || blocks[0].info.ASN != 3 {
		t.Fatalf("blocks=%+v err=%v", blocks, err)
	}
}

func TestLookupLongestASNBlockNetworkNoMatch(t *testing.T) {
	dir := t.TempDir()
	_ = os.WriteFile(filepath.Join(dir, asnBlocksIPv4Name), []byte(`network,geoname_id,registered_country_geoname_id,represented_country_geoname_id,is_anonymous_proxy,is_satellite_provider,autonomous_system_number,autonomous_system_organization
10.0.0.0/8,,,,0,0,1,Test
`), 0644)
	blocks := buildASNNetworkBlocks(dir)
	if _, _, ok := lookupLongestASNBlockNetwork(blocks, net.ParseIP("192.168.1.1")); ok {
		t.Fatal("expected miss")
	}
}

func TestBuildASNNetworkBlocksSortsByMask(t *testing.T) {
	dir := t.TempDir()
	_ = os.WriteFile(filepath.Join(dir, asnBlocksIPv4Name), []byte(`network,geoname_id,registered_country_geoname_id,represented_country_geoname_id,is_anonymous_proxy,is_satellite_provider,autonomous_system_number,autonomous_system_organization
10.0.0.0/8,,,,0,0,1,Wide
10.0.0.0/24,,,,0,0,2,Narrow
`), 0644)
	blocks := buildASNNetworkBlocks(dir)
	if len(blocks) != 2 || blocks[0].maskOnes < blocks[1].maskOnes {
		t.Fatalf("blocks=%+v", blocks)
	}
}

func TestResolveASNDNSFallbackWithCity(t *testing.T) {
	g := New("", "")
	g.cityDB = testGeoDB(t).cityDB
	info, err := g.ResolveASN(context.Background(), "8.8.8.8")
	if err != nil {
		t.Fatal(err)
	}
	if info == nil || info.ASN == 0 {
		t.Fatalf("info=%+v", info)
	}
}

func TestResolveASNZeroASNDNSFallback(t *testing.T) {
	g := New("", "")
	info, err := g.ResolveASN(context.Background(), "::1")
	if err != nil || info == nil || info.ASN != 0 {
		t.Fatalf("info=%+v err=%v", info, err)
	}
}

func TestLookupCityRecordError(t *testing.T) {
	g := New(testASNPath(t), "")
	g.cityDB = testGeoDB(t).cityDB
	dir := t.TempDir()
	badCity := filepath.Join(dir, "bad-city.mmdb")
	_ = os.WriteFile(badCity, []byte("bad"), 0644)
	g2 := New("", badCity)
	if err := g2.Reload(); err == nil {
		t.Fatal("expected reload error")
	}
	if _, err := g.LookupCity("not-an-ip"); err == nil {
		t.Fatal("expected invalid ip")
	}
}

func TestApplyCityRecordError(t *testing.T) {
	g := testGeoDB(t)
	info := &domain.ASInfo{ASN: 1}
	if applyCityRecord(g.cityDB, net.ParseIP("240.0.0.1"), info) {
		t.Fatal("expected false for unknown city record")
	}
}

func TestResolveDNSZeroASN(t *testing.T) {
	g := New("", "")
	info, err := g.resolveDNS(context.Background(), net.ParseIP("::1"), "::1")
	if err != nil || info == nil || info.ASN != 0 {
		t.Fatalf("info=%+v err=%v", info, err)
	}
}

func TestMergeASByNumberDNSError(t *testing.T) {
	old := lookupASNTXT
	lookupASNTXT = func(context.Context, string) ([]string, error) {
		return nil, errors.New("dns failed")
	}
	t.Cleanup(func() { lookupASNTXT = old })

	g := New("", "")
	out := g.mergeASByNumberDNS(context.Background(), &domain.ASInfo{ASN: 15169, OrgName: "Base"})
	if out.OrgName != "Base" {
		t.Fatalf("out=%+v", out)
	}
}

func TestLookupASByNumberDNSCountryFromOrg(t *testing.T) {
	old := lookupASNTXT
	lookupASNTXT = func(_ context.Context, name string) ([]string, error) {
		return []string{`"15169 | US | arin | 2000 | GOOGLE, US"`}, nil
	}
	t.Cleanup(func() { lookupASNTXT = old })

	g := New("", "")
	info, err := g.lookupASByNumberDNS(context.Background(), 15169)
	if err != nil || info.CountryCode != "US" {
		t.Fatalf("info=%+v err=%v", info, err)
	}
}

func TestReloadBothDBErrors(t *testing.T) {
	dir := t.TempDir()
	g := New(filepath.Join(dir, "bad-asn.mmdb"), filepath.Join(dir, "bad-city.mmdb"))
	_ = os.WriteFile(g.asnPath, []byte("bad"), 0644)
	_ = os.WriteFile(g.cityPath, []byte("bad"), 0644)
	if err := g.Reload(); err == nil || !strings.Contains(err.Error(), "open city db") {
		t.Fatalf("err=%v", err)
	}
}

func TestFormatTracerouteOrgNameEmptyAfterSuffixTrim(t *testing.T) {
	if got := FormatTracerouteOrgName("LLC"); got != "" {
		t.Fatalf("got=%q", got)
	}
}

func TestChosenSourceNilPicked(t *testing.T) {
	if src := chosenSource(false, false, nil, nil, nil); src != SourceNone {
		t.Fatalf("src=%q", src)
	}
}

func TestEnrichASInfoNilASN(t *testing.T) {
	g := New("", "")
	info, enriched := g.enrichASInfo(net.ParseIP("1.1.1.1"), nil)
	if info != nil || enriched {
		t.Fatalf("info=%+v enriched=%v", info, enriched)
	}
}

func TestLookupIPMMDBErrorOnly(t *testing.T) {
	g := testGeoDB(t)
	g.mu.Lock()
	g.asnDB = nil
	g.mu.Unlock()
	report, err := g.LookupIP(context.Background(), "1.128.0.1")
	if err != nil {
		t.Fatal(err)
	}
	if report.MMDB.Available {
		t.Fatalf("report=%+v", report)
	}
}

func TestLookupIPDNSChosenWithCity(t *testing.T) {
	g := New("", "")
	g.cityDB = testGeoDB(t).cityDB
	old := lookupASNTXT
	lookupASNTXT = func(context.Context, string) ([]string, error) {
		return []string{`"15169 | 1.2.3.0/24 | US | arin | 2000 | GOOGLE"`}, nil
	}
	t.Cleanup(func() { lookupASNTXT = old })

	report, err := g.LookupIP(context.Background(), "8.8.8.8")
	if err != nil {
		t.Fatal(err)
	}
	if report.ChosenSource != SourceDNS || report.Result == nil {
		t.Fatalf("report=%+v", report)
	}
}

func TestLookupIPInferChosenSource(t *testing.T) {
	g := testGeoDB(t)
	report, err := g.LookupIP(context.Background(), "1.128.0.1")
	if err != nil || report.ChosenSource == SourceNone {
		t.Fatalf("report=%+v err=%v", report, err)
	}
}

func TestLatestRFC3339SecondLater(t *testing.T) {
	got := latestRFC3339("2024-01-01T00:00:00Z", "2024-06-01T00:00:00Z")
	if got != "2024-06-01T00:00:00Z" {
		t.Fatalf("got=%q", got)
	}
}

func TestSyncSettingsErrorsAndWrites(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	mock.ExpectQuery("SELECT key, value FROM settings").WillReturnError(errors.New("settings failed"))
	if err := SyncSettings(queries.New(db), config.GeoIPConfig{}); err == nil {
		t.Fatal("expected settings error")
	}

	db2, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db2.Close() })
	if _, err := db2.Exec(`CREATE TABLE settings (key TEXT PRIMARY KEY, value TEXT NOT NULL)`); err != nil {
		t.Fatal(err)
	}
	q := queries.New(db2)
	if err := SyncSettings(q, config.GeoIPConfig{LicenseKey: "k", AccountID: "a", UpdateInterval: "24h"}); err != nil {
		t.Fatal(err)
	}
}

func TestUpdaterRunMkdirError(t *testing.T) {
	u := NewUpdater(config.GeoIPConfig{}, New("", ""))
	u.asnPath = string([]byte{0})
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	u.Run(ctx)
}

func TestUpdaterLastDownloadModTimeFallback(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "GeoLite2-ASN.mmdb")
	_ = os.WriteFile(path, []byte("db"), 0644)
	u := NewUpdater(config.GeoIPConfig{}, New("", ""))
	u.SetLastDownload(func(string) time.Time { return time.Time{} })
	got := u.lastDownloadAt("GeoLite2-ASN", path)
	if got.IsZero() {
		t.Fatal("expected mod time fallback")
	}
}

func TestUpdaterNeedsDownloadLastZeroWithSidecars(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "GeoLite2-ASN.mmdb")
	_ = os.WriteFile(path, []byte("db"), 0644)
	u := NewUpdater(config.GeoIPConfig{UpdateInterval: "72h"}, New("", ""))
	u.SetLastDownload(func(string) time.Time { return time.Time{} })
	should, _, _ := u.needsDownload("GeoLite2-ASN", path)
	if !should {
		t.Fatal("expected download when last is zero")
	}
}

func TestUpdaterUpdateAllReloadError(t *testing.T) {
	dir := t.TempDir()
	asnPath := filepath.Join(dir, "GeoLite2-ASN.mmdb")
	_ = os.WriteFile(asnPath, []byte("bad"), 0644)
	g := New("", "")
	u := NewUpdater(config.GeoIPConfig{LicenseKey: "k", AccountID: "a"}, g)
	u.asnPath = asnPath
	u.cityPath = filepath.Join(dir, "GeoLite2-City.mmdb")
	body := buildTestMMDBArchive(t, "GeoLite2-ASN.mmdb", readTestFile(t, testASNPath(t)))
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(body)
	}))
	defer srv.Close()
	orig := http.DefaultTransport
	http.DefaultTransport = roundTripperFunc(func(req *http.Request) (*http.Response, error) {
		req2 := req.Clone(req.Context())
		req2.URL, _ = url.Parse(srv.URL)
		return orig.RoundTrip(req2)
	})
	t.Cleanup(func() { http.DefaultTransport = orig })
	u.updateAll(context.Background())
}

func TestDownloadMMDBEditionExtractError(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "GeoLite2-ASN.mmdb")
	u := NewUpdater(config.GeoIPConfig{LicenseKey: "k", AccountID: "a"}, New("", ""))
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("not-a-tar"))
	}))
	defer srv.Close()
	orig := http.DefaultTransport
	http.DefaultTransport = roundTripperFunc(func(req *http.Request) (*http.Response, error) {
		req2 := req.Clone(req.Context())
		req2.URL, _ = url.Parse(srv.URL)
		return orig.RoundTrip(req2)
	})
	t.Cleanup(func() { http.DefaultTransport = orig })
	if err := u.downloadMMDBEdition(context.Background(), "GeoLite2-ASN", target); err == nil {
		t.Fatal("expected extract error")
	}
}

func TestDownloadMMDBEditionRenameError(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "GeoLite2-ASN.mmdb")
	u := NewUpdater(config.GeoIPConfig{LicenseKey: "k", AccountID: "a"}, New("", ""))
	body := buildTestMMDBArchive(t, "GeoLite2-ASN.mmdb", []byte("mmdb"))
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write(body)
	}))
	defer srv.Close()
	orig := http.DefaultTransport
	http.DefaultTransport = roundTripperFunc(func(req *http.Request) (*http.Response, error) {
		req2 := req.Clone(req.Context())
		req2.URL, _ = url.Parse(srv.URL)
		return orig.RoundTrip(req2)
	})
	t.Cleanup(func() { http.DefaultTransport = orig })
	oldRename := osRename
	osRename = func(string, string) error { return errors.New("rename failed") }
	t.Cleanup(func() { osRename = oldRename })
	if err := u.downloadMMDBEdition(context.Background(), "GeoLite2-ASN", target); err == nil {
		t.Fatal("expected rename error")
	}
}

func TestDownloadCSVSidecarsUnknownEdition(t *testing.T) {
	u := NewUpdater(config.GeoIPConfig{}, New("", ""))
	if err := u.downloadCSVSidecars(context.Background(), "unknown", t.TempDir()); err != nil {
		t.Fatal(err)
	}
}

func TestFetchMaxMindNewRequestError(t *testing.T) {
	u := NewUpdater(config.GeoIPConfig{}, New("", ""))
	old := updaterNewRequestWithCtx
	updaterNewRequestWithCtx = func(context.Context, string, string, io.Reader) (*http.Request, error) {
		return nil, errors.New("new request failed")
	}
	t.Cleanup(func() { updaterNewRequestWithCtx = old })
	if _, err := u.fetchMaxMind(context.Background(), "http://example.com", "GeoLite2-ASN"); err == nil {
		t.Fatal("expected new request error")
	}
}

func TestExtractMMDBFileTarAndCreateErrors(t *testing.T) {
	var buf bytes.Buffer
	gw := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gw)
	_ = tw.WriteHeader(&tar.Header{Name: "bad.tar", Mode: 0644, Size: 1})
	_, _ = tw.Write([]byte("x"))
	_ = tw.Close()
	_ = gw.Close()
	if err := extractMMDBFile(bytes.NewReader(buf.Bytes()), filepath.Join(t.TempDir(), "out.mmdb")); err == nil {
		t.Fatal("expected tar read continuation error")
	}

	target := filepath.Join(string([]byte{0}), "out.mmdb")
	body := buildTestMMDBArchive(t, "GeoLite2-ASN.mmdb", []byte("mmdb"))
	if err := extractMMDBFile(bytes.NewReader(body), target); err == nil {
		t.Fatal("expected create error")
	}
}

func TestExtractMMDBFileCopyError(t *testing.T) {
	target := filepath.Join(t.TempDir(), "out.mmdb")
	var buf bytes.Buffer
	gw := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gw)
	_ = tw.WriteHeader(&tar.Header{Name: "GeoLite2-ASN.mmdb", Mode: 0644, Size: 1024})
	_, _ = tw.Write([]byte("short"))
	_ = tw.Close()
	_ = gw.Close()
	if err := extractMMDBFile(bytes.NewReader(buf.Bytes()), target); err == nil {
		t.Fatal("expected copy error")
	}
}

func TestExtractCSVFilesReadAndOpenErrors(t *testing.T) {
	if err := extractCSVFiles(badCopyReader{}, t.TempDir(), map[string]struct{}{asnBlocksIPv4Name: {}}); err == nil {
		t.Fatal("expected read error")
	}

	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	hdr := &zip.FileHeader{Name: "dir/", Method: zip.Store}
	w, _ := zw.CreateHeader(hdr)
	_ = w
	w2, _ := zw.Create(asnBlocksIPv4Name)
	_, _ = io.WriteString(w2, "a,b\n")
	_ = zw.Close()
	if err := extractCSVFiles(bytes.NewReader(buf.Bytes()), t.TempDir(), map[string]struct{}{asnBlocksIPv4Name: {}}); err != nil {
		t.Fatal(err)
	}

	corrupt := []byte("not-a-zip")
	if err := extractCSVFiles(bytes.NewReader(corrupt), t.TempDir(), map[string]struct{}{asnBlocksIPv4Name: {}}); err == nil {
		t.Fatal("expected zip open error")
	}
}

func TestWriteArchiveFileCloseError(t *testing.T) {
	oldClose := closeArchiveTempFile
	closeArchiveTempFile = func(*os.File) error { return errors.New("close failed") }
	t.Cleanup(func() { closeArchiveTempFile = oldClose })
	target := filepath.Join(t.TempDir(), "out.csv")
	if err := writeArchiveFile(bytes.NewReader([]byte("data")), target); err == nil {
		t.Fatal("expected close error")
	}
}

func TestDownloadCSVSidecarsExtractError(t *testing.T) {
	dir := t.TempDir()
	u := NewUpdater(config.GeoIPConfig{LicenseKey: "k", AccountID: "a"}, New("", ""))
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("not-a-zip"))
	}))
	defer srv.Close()
	orig := http.DefaultTransport
	http.DefaultTransport = roundTripperFunc(func(req *http.Request) (*http.Response, error) {
		req2 := req.Clone(req.Context())
		req2.URL, _ = url.Parse(srv.URL)
		return orig.RoundTrip(req2)
	})
	t.Cleanup(func() { http.DefaultTransport = orig })
	if err := u.downloadCSVSidecars(context.Background(), "GeoLite2-ASN", dir); err == nil {
		t.Fatal("expected extract error")
	}
}

func TestLoadASNIndexFromBlocksMissingColumns(t *testing.T) {
	path := filepath.Join(t.TempDir(), asnBlocksIPv4Name)
	_ = os.WriteFile(path, []byte("network,geoname_id\n1.2.3.0/24,1\n"), 0644)
	if _, err := loadASNIndexFromBlocks(path, nil); err == nil {
		t.Fatal("expected missing columns error")
	}
}

func TestLoadASNIndexFromBlocksEmptyASNField(t *testing.T) {
	path := filepath.Join(t.TempDir(), asnBlocksIPv4Name)
	_ = os.WriteFile(path, []byte(`network,geoname_id,registered_country_geoname_id,represented_country_geoname_id,is_anonymous_proxy,is_satellite_provider,autonomous_system_number,autonomous_system_organization
1.2.3.0/24,,,,0,0,,Org
`), 0644)
	idx, err := loadASNIndexFromBlocks(path, nil)
	if err != nil || len(idx) != 0 {
		t.Fatalf("idx=%v err=%v", idx, err)
	}
}

func TestLoadASNNetworkBlocksMalformedRow(t *testing.T) {
	path := filepath.Join(t.TempDir(), asnBlocksIPv4Name)
	_ = os.WriteFile(path, []byte("network,geoname_id,registered_country_geoname_id,represented_country_geoname_id,is_anonymous_proxy,is_satellite_provider,autonomous_system_number,autonomous_system_organization\n\"bad\n"), 0644)
	blocks, err := loadASNNetworkBlocks(path, nil)
	if err != nil || len(blocks) != 0 {
		t.Fatalf("blocks=%v err=%v", blocks, err)
	}
}

func TestStoreResolveCacheReset(t *testing.T) {
	g := New("", "")
	g.resolveCache = make(map[string]domain.ASInfo, maxResolveCacheEntries)
	for i := 0; i < maxResolveCacheEntries; i++ {
		g.resolveCache[fmt.Sprintf("10.0.0.%d", i)] = domain.ASInfo{ASN: uint32(i + 1)}
	}
	g.storeResolve("9.9.9.9", &domain.ASInfo{ASN: 99})
	if len(g.resolveCache) != 1 {
		t.Fatalf("cache len=%d", len(g.resolveCache))
	}
}

func TestLookupCityRecordHookError(t *testing.T) {
	g := testGeoDB(t)
	old := lookupCityRecord
	lookupCityRecord = func(*geoip2.Reader, net.IP) (*geoip2.City, error) {
		return nil, errors.New("city lookup failed")
	}
	t.Cleanup(func() { lookupCityRecord = old })
	if _, err := g.LookupCity("2.125.160.216"); err == nil {
		t.Fatal("expected city lookup error")
	}
}

func TestApplyCityRecordHookError(t *testing.T) {
	g := testGeoDB(t)
	old := lookupCityRecord
	lookupCityRecord = func(*geoip2.Reader, net.IP) (*geoip2.City, error) {
		return nil, errors.New("city lookup failed")
	}
	t.Cleanup(func() { lookupCityRecord = old })
	if applyCityRecord(g.cityDB, net.ParseIP("2.125.160.216"), &domain.ASInfo{ASN: 1}) {
		t.Fatal("expected false")
	}
}

func TestResolveDNSZeroASNFromTXT(t *testing.T) {
	old := lookupOriginTXT
	lookupOriginTXT = func(context.Context, string) ([]string, error) {
		return []string{"0 | 0 | US"}, nil
	}
	t.Cleanup(func() { lookupOriginTXT = old })
	g := New("", "")
	info, err := g.resolveDNS(context.Background(), net.ParseIP("8.8.8.8"), "8.8.8.8")
	if err != nil || info.ASN != 0 {
		t.Fatalf("info=%+v err=%v", info, err)
	}
}

func TestMergeASByNumberDNSNilInfo(t *testing.T) {
	g := New("", "")
	if g.mergeASByNumberDNS(context.Background(), nil) != nil {
		t.Fatal("expected nil")
	}
}

func TestLookupASByNumberDNSOrgSuffixCountry(t *testing.T) {
	old := lookupASNTXT
	lookupASNTXT = func(_ context.Context, name string) ([]string, error) {
		return []string{`"15169 | 10.0.0.0/8 |  | arin | 2000 | GOOGLE, US"`}, nil
	}
	t.Cleanup(func() { lookupASNTXT = old })
	g := New("", "")
	info, err := g.lookupASByNumberDNS(context.Background(), 15169)
	if err != nil || info.CountryCode != "US" {
		t.Fatalf("info=%+v err=%v", info, err)
	}
}

func TestFormatTracerouteOrgNameSuffixOnlyWords(t *testing.T) {
	if got := FormatTracerouteOrgName("Inc. LLC"); got != "" {
		t.Fatalf("got=%q", got)
	}
}

func TestLookupIPMMDBLookupError(t *testing.T) {
	g := testGeoDB(t)
	old := lookupASNRecord
	lookupASNRecord = func(*geoip2.Reader, net.IP) (*geoip2.ASN, error) {
		return nil, errors.New("asn lookup failed")
	}
	t.Cleanup(func() { lookupASNRecord = old })
	report, err := g.LookupIP(context.Background(), "1.128.0.1")
	if err != nil {
		t.Fatal(err)
	}
	if report.MMDB.Error == "" {
		t.Fatalf("report=%+v", report)
	}
}

func TestLookupIPDNSErrorPath(t *testing.T) {
	g := New("", "")
	old := lookupOriginTXT
	lookupOriginTXT = func(context.Context, string) ([]string, error) {
		return nil, errors.New("dns failed")
	}
	t.Cleanup(func() { lookupOriginTXT = old })
	report, err := g.LookupIP(context.Background(), "8.8.8.8")
	if err != nil {
		t.Fatal(err)
	}
	if report.DNS.Error == "" {
		t.Fatalf("report=%+v", report)
	}
}

func TestLookupIPChosenSourceInferNone(t *testing.T) {
	g := New("", "")
	report, err := g.LookupIP(context.Background(), "240.0.0.1")
	if err != nil {
		t.Fatal(err)
	}
	if report.ChosenSource != SourceNone {
		t.Fatalf("report=%+v", report)
	}
}

func TestUpdaterRunMkdirAllErrorReturns(t *testing.T) {
	u := NewUpdater(config.GeoIPConfig{}, New("", ""))
	old := updaterMkdirAll
	updaterMkdirAll = func(string, os.FileMode) error { return errors.New("mkdir failed") }
	t.Cleanup(func() { updaterMkdirAll = old })
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	u.Run(ctx)
}

func TestUpdaterLastDownloadAtFinalZero(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "GeoLite2-ASN.mmdb")
	_ = os.WriteFile(path, []byte("db"), 0644)
	calls := 0
	old := updaterOsStat
	updaterOsStat = func(name string) (os.FileInfo, error) {
		calls++
		if calls >= 2 {
			return nil, errors.New("stat failed")
		}
		return os.Stat(name)
	}
	t.Cleanup(func() { updaterOsStat = old })
	u := NewUpdater(config.GeoIPConfig{}, New("", ""))
	if got := u.lastDownloadAt("GeoLite2-ASN", path); !got.IsZero() {
		t.Fatalf("got=%v", got)
	}
}

func TestUpdaterNeedsDownloadLastZero(t *testing.T) {
	dir := t.TempDir()
	asnPath := writeASNEditionFiles(t, dir)
	calls := 0
	old := updaterOsStat
	updaterOsStat = func(name string) (os.FileInfo, error) {
		calls++
		if calls >= 3 {
			return nil, errors.New("stat failed")
		}
		return os.Stat(name)
	}
	t.Cleanup(func() { updaterOsStat = old })
	u := NewUpdater(config.GeoIPConfig{UpdateInterval: "72h"}, New("", ""))
	u.asnPath = asnPath
	should, _, _ := u.needsDownload("GeoLite2-ASN", asnPath)
	if !should {
		t.Fatal("expected download")
	}
}

func TestUpdaterUpdateAllReloadSuccessWithError(t *testing.T) {
	dir := t.TempDir()
	g := New("", "")
	u := NewUpdater(config.GeoIPConfig{LicenseKey: "k", AccountID: "a"}, g)
	u.asnPath = filepath.Join(dir, "GeoLite2-ASN.mmdb")
	u.cityPath = filepath.Join(dir, "GeoLite2-City.mmdb")
	body := buildTestMMDBArchive(t, "GeoLite2-ASN.mmdb", readTestFile(t, testASNPath(t)))
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write(body)
	}))
	defer srv.Close()
	orig := http.DefaultTransport
	http.DefaultTransport = roundTripperFunc(func(req *http.Request) (*http.Response, error) {
		req2 := req.Clone(req.Context())
		req2.URL, _ = url.Parse(srv.URL)
		return orig.RoundTrip(req2)
	})
	t.Cleanup(func() { http.DefaultTransport = orig })
	u.updateAll(context.Background())
	_ = os.WriteFile(u.asnPath, []byte("bad"), 0644)
	u.updateAll(context.Background())
}

func TestExtractMMDBFileTarHeaderError(t *testing.T) {
	var buf bytes.Buffer
	gw := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gw)
	_ = tw.WriteHeader(&tar.Header{Name: "broken", Mode: 0644, Size: 10, Typeflag: tar.TypeReg})
	_ = tw.Close()
	_ = gw.Close()
	if err := extractMMDBFile(bytes.NewReader(buf.Bytes()), filepath.Join(t.TempDir(), "out.mmdb")); err == nil {
		t.Fatal("expected tar read error")
	}
}

func TestExtractCSVFilesZipOpenErrorOnFile(t *testing.T) {
	dir := t.TempDir()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	w, _ := zw.Create(asnBlocksIPv4Name)
	_, _ = io.WriteString(w, "a,b\n")
	_ = zw.Close()
	oldCreate := osCreate
	osCreate = func(name string) (*os.File, error) { return nil, errors.New("create failed") }
	t.Cleanup(func() { osCreate = oldCreate })
	err := extractCSVFiles(bytes.NewReader(buf.Bytes()), dir, map[string]struct{}{asnBlocksIPv4Name: {}})
	if err == nil {
		t.Fatal("expected write error")
	}
}

func TestResolveDNSOriginTXTError(t *testing.T) {
	old := lookupOriginTXT
	lookupOriginTXT = func(context.Context, string) ([]string, error) {
		return nil, errors.New("txt failed")
	}
	t.Cleanup(func() { lookupOriginTXT = old })
	g := New("", "")
	if _, err := g.resolveDNS(context.Background(), net.ParseIP("8.8.8.8"), "8.8.8.8"); err == nil {
		t.Fatal("expected error")
	}
}

func TestResolveDNSOriginAndASNOrg(t *testing.T) {
	oldOrigin := lookupOriginTXT
	oldASN := lookupASNTXT
	lookupOriginTXT = func(context.Context, string) ([]string, error) {
		return []string{"15169 | 8.8.8.8 | US"}, nil
	}
	lookupASNTXT = func(context.Context, string) ([]string, error) {
		return []string{`"15169 | US | arin | 2000 | GOOGLE"`}, nil
	}
	t.Cleanup(func() {
		lookupOriginTXT = oldOrigin
		lookupASNTXT = oldASN
	})
	g := New("", "")
	info, err := g.resolveDNS(context.Background(), net.ParseIP("8.8.8.8"), "8.8.8.8")
	if err != nil || info.ASN != 15169 || info.OrgName == "" {
		t.Fatalf("info=%+v err=%v", info, err)
	}
}

func TestMergeASByNumberDNSZeroASN(t *testing.T) {
	g := New("", "")
	out := g.mergeASByNumberDNS(context.Background(), &domain.ASInfo{ASN: 0})
	if out.ASN != 0 {
		t.Fatalf("out=%+v", out)
	}
}

func TestMergeASByNumberDNSEnrichesFromDNS(t *testing.T) {
	old := lookupASNTXT
	lookupASNTXT = func(context.Context, string) ([]string, error) {
		return []string{`"15169 | US | arin | 2000 | GOOGLE"`}, nil
	}
	t.Cleanup(func() { lookupASNTXT = old })
	g := New("", "")
	out := g.mergeASByNumberDNS(context.Background(), &domain.ASInfo{ASN: 15169})
	if out.CountryCode != "US" || out.OrgName == "" {
		t.Fatalf("out=%+v", out)
	}
}

func TestFormatTracerouteOrgNameSingleWord(t *testing.T) {
	if got := FormatTracerouteOrgName("Google"); got != "Google" {
		t.Fatalf("got=%q", got)
	}
}

func TestLookupIPChosenSourceFallbackInfer(t *testing.T) {
	g := testGeoDB(t)
	old := ipLookupChosenSource
	ipLookupChosenSource = func(bool, bool, *domain.ASInfo, *domain.ASInfo, *domain.ASInfo) ResolveSource {
		return SourceNone
	}
	t.Cleanup(func() { ipLookupChosenSource = old })
	report, err := g.LookupIP(context.Background(), "1.128.0.1")
	if err != nil || report.ChosenSource == SourceNone || report.Result == nil {
		t.Fatalf("report=%+v err=%v", report, err)
	}
}

func TestLookupIPIncludesCity(t *testing.T) {
	g := testGeoDB(t)
	report, err := g.LookupIP(context.Background(), "2.125.160.216")
	if err != nil || report.City == nil {
		t.Fatalf("report=%+v err=%v", report, err)
	}
}

func TestUpdaterUpdateAllReloadSuccess(t *testing.T) {
	dir := t.TempDir()
	g := New(testASNPath(t), testCityPath(t))
	u := NewUpdater(config.GeoIPConfig{LicenseKey: "k", AccountID: "a"}, g)
	u.asnPath = filepath.Join(dir, "GeoLite2-ASN.mmdb")
	u.cityPath = filepath.Join(dir, "GeoLite2-City.mmdb")
	mmdbBody := buildTestMMDBArchive(t, "GeoLite2-ASN.mmdb", readTestFile(t, testASNPath(t)))
	cityBody := buildTestMMDBArchive(t, "GeoLite2-City.mmdb", readTestFile(t, testCityPath(t)))
	csvBody := buildTestCSVArchive(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.Contains(r.URL.RawQuery, "GeoLite2-City") && strings.Contains(r.URL.RawQuery, "suffix=zip"):
			_, _ = w.Write(csvBody)
		case strings.Contains(r.URL.RawQuery, "GeoLite2-City"):
			_, _ = w.Write(cityBody)
		case strings.Contains(r.URL.RawQuery, "suffix=zip"):
			_, _ = w.Write(csvBody)
		default:
			_, _ = w.Write(mmdbBody)
		}
	}))
	defer srv.Close()
	orig := http.DefaultTransport
	http.DefaultTransport = roundTripperFunc(func(req *http.Request) (*http.Response, error) {
		req2 := req.Clone(req.Context())
		req2.URL, _ = url.Parse(srv.URL)
		return orig.RoundTrip(req2)
	})
	t.Cleanup(func() { http.DefaultTransport = orig })
	u.updateAll(context.Background())
}

func TestFormatTracerouteOrgNameTwoWords(t *testing.T) {
	if got := FormatTracerouteOrgName("Foo Bar"); got != "Foo Bar" {
		t.Fatalf("got=%q", got)
	}
}

func TestResolveDNSWithoutASNOrgTXT(t *testing.T) {
	oldOrigin := lookupOriginTXT
	oldASN := lookupASNTXT
	lookupOriginTXT = func(context.Context, string) ([]string, error) {
		return []string{"15169 | 8.8.8.8 | US"}, nil
	}
	lookupASNTXT = func(context.Context, string) ([]string, error) {
		return nil, nil
	}
	t.Cleanup(func() {
		lookupOriginTXT = oldOrigin
		lookupASNTXT = oldASN
	})
	g := New("", "")
	info, err := g.resolveDNS(context.Background(), net.ParseIP("8.8.8.8"), "8.8.8.8")
	if err != nil || info.ASN != 15169 || info.CountryCode != "US" {
		t.Fatalf("info=%+v err=%v", info, err)
	}
}

func TestLookupASByNumberDNSEmptyTXT(t *testing.T) {
	old := lookupASNTXT
	lookupASNTXT = func(context.Context, string) ([]string, error) {
		return nil, nil
	}
	t.Cleanup(func() { lookupASNTXT = old })
	g := New("", "")
	info, err := g.lookupASByNumberDNS(context.Background(), 15169)
	if err != nil || info.ASN != 15169 || info.CountryCode != "" {
		t.Fatalf("info=%+v err=%v", info, err)
	}
}

func TestUpdaterUpdateAllNoDownload(t *testing.T) {
	dir := t.TempDir()
	asnPath := writeASNEditionFiles(t, dir)
	cityPath := filepath.Join(dir, "GeoLite2-City.mmdb")
	_ = os.WriteFile(cityPath, []byte("db"), 0644)
	_ = os.WriteFile(filepath.Join(dir, cityLocationsName), []byte("geoname_id,locale_code,country_iso_code\n"), 0644)
	g := New(testASNPath(t), testCityPath(t))
	u := NewUpdater(config.GeoIPConfig{UpdateInterval: "72h"}, g)
	u.asnPath = asnPath
	u.cityPath = cityPath
	u.SetLastDownload(func(string) time.Time { return time.Now().UTC() })
	u.updateAll(context.Background())
}

func TestResolveDNSASNTXTError(t *testing.T) {
	oldOrigin := lookupOriginTXT
	oldASN := lookupASNTXT
	lookupOriginTXT = func(context.Context, string) ([]string, error) {
		return []string{"15169 | 8.8.8.8 | US"}, nil
	}
	lookupASNTXT = func(context.Context, string) ([]string, error) {
		return nil, errors.New("asn txt failed")
	}
	t.Cleanup(func() {
		lookupOriginTXT = oldOrigin
		lookupASNTXT = oldASN
	})
	g := New("", "")
	info, err := g.resolveDNS(context.Background(), net.ParseIP("8.8.8.8"), "8.8.8.8")
	if err != nil || info.ASN != 15169 {
		t.Fatalf("info=%+v err=%v", info, err)
	}
}

func TestResolveDNSOriginTwoFields(t *testing.T) {
	oldOrigin := lookupOriginTXT
	oldASN := lookupASNTXT
	lookupOriginTXT = func(context.Context, string) ([]string, error) {
		return []string{"15169 | 8.8.8.8"}, nil
	}
	lookupASNTXT = func(context.Context, string) ([]string, error) {
		return []string{`"15169 | US | arin | 2000 | GOOGLE"`}, nil
	}
	t.Cleanup(func() {
		lookupOriginTXT = oldOrigin
		lookupASNTXT = oldASN
	})
	g := New("", "")
	info, err := g.resolveDNS(context.Background(), net.ParseIP("8.8.8.8"), "8.8.8.8")
	if err != nil || info.CountryCode != "US" || info.OrgName == "" {
		t.Fatalf("info=%+v err=%v", info, err)
	}
}

func TestUpdaterUpdateAllCityDownloadOnly(t *testing.T) {
	dir := t.TempDir()
	asnPath := writeASNEditionFiles(t, dir)
	cityPath := filepath.Join(dir, "GeoLite2-City.mmdb")
	g := New(testASNPath(t), testCityPath(t))
	u := NewUpdater(config.GeoIPConfig{LicenseKey: "k", AccountID: "a"}, g)
	u.asnPath = asnPath
	u.cityPath = cityPath
	u.SetLastDownload(func(edition string) time.Time {
		if edition == "GeoLite2-ASN" {
			return time.Now().UTC()
		}
		return time.Time{}
	})
	cityBody := buildTestMMDBArchive(t, "GeoLite2-City.mmdb", readTestFile(t, testCityPath(t)))
	csvBody := buildTestCSVArchive(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.RawQuery, "suffix=zip") {
			_, _ = w.Write(csvBody)
			return
		}
		_, _ = w.Write(cityBody)
	}))
	defer srv.Close()
	orig := http.DefaultTransport
	http.DefaultTransport = roundTripperFunc(func(req *http.Request) (*http.Response, error) {
		req2 := req.Clone(req.Context())
		req2.URL, _ = url.Parse(srv.URL)
		return orig.RoundTrip(req2)
	})
	t.Cleanup(func() { http.DefaultTransport = orig })
	u.updateAll(context.Background())
}

func TestResolveDNSEmptyOriginTXT(t *testing.T) {
	old := lookupOriginTXT
	lookupOriginTXT = func(context.Context, string) ([]string, error) {
		return nil, nil
	}
	t.Cleanup(func() { lookupOriginTXT = old })
	g := New("", "")
	info, err := g.resolveDNS(context.Background(), net.ParseIP("8.8.8.8"), "8.8.8.8")
	if err != nil || info == nil || info.ASN != 0 {
		t.Fatalf("info=%+v err=%v", info, err)
	}
}

func TestUpdaterUpdateAllASNDownloadOnly(t *testing.T) {
	dir := t.TempDir()
	cityPath := filepath.Join(dir, "GeoLite2-City.mmdb")
	_ = os.WriteFile(cityPath, []byte("db"), 0644)
	_ = os.WriteFile(filepath.Join(dir, cityLocationsName), []byte("geoname_id,locale_code,country_iso_code\n"), 0644)
	g := New(testASNPath(t), testCityPath(t))
	u := NewUpdater(config.GeoIPConfig{LicenseKey: "k", AccountID: "a"}, g)
	u.asnPath = filepath.Join(dir, "GeoLite2-ASN.mmdb")
	u.cityPath = cityPath
	u.SetLastDownload(func(edition string) time.Time {
		if edition == "GeoLite2-City" {
			return time.Now().UTC()
		}
		return time.Time{}
	})
	asnBody := buildTestMMDBArchive(t, "GeoLite2-ASN.mmdb", readTestFile(t, testASNPath(t)))
	csvBody := buildTestCSVArchive(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.RawQuery, "suffix=zip") {
			_, _ = w.Write(csvBody)
			return
		}
		_, _ = w.Write(asnBody)
	}))
	defer srv.Close()
	orig := http.DefaultTransport
	http.DefaultTransport = roundTripperFunc(func(req *http.Request) (*http.Response, error) {
		req2 := req.Clone(req.Context())
		req2.URL, _ = url.Parse(srv.URL)
		return orig.RoundTrip(req2)
	})
	t.Cleanup(func() { http.DefaultTransport = orig })
	u.updateAll(context.Background())
}

func TestUpdaterUpdateAllReloadErrorPath(t *testing.T) {
	dir := t.TempDir()
	asnPath := filepath.Join(dir, "GeoLite2-ASN.mmdb")
	cityPath := filepath.Join(dir, "GeoLite2-City.mmdb")
	_ = os.WriteFile(cityPath, []byte("bad"), 0644)
	g := New(asnPath, cityPath)
	u := NewUpdater(config.GeoIPConfig{LicenseKey: "k", AccountID: "a"}, g)
	u.asnPath = asnPath
	u.cityPath = cityPath
	u.SetLastDownload(func(edition string) time.Time {
		if edition == "GeoLite2-City" {
			return time.Now().UTC()
		}
		return time.Time{}
	})
	asnBody := buildTestMMDBArchive(t, "GeoLite2-ASN.mmdb", readTestFile(t, testASNPath(t)))
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write(asnBody)
	}))
	defer srv.Close()
	orig := http.DefaultTransport
	http.DefaultTransport = roundTripperFunc(func(req *http.Request) (*http.Response, error) {
		req2 := req.Clone(req.Context())
		req2.URL, _ = url.Parse(srv.URL)
		return orig.RoundTrip(req2)
	})
	t.Cleanup(func() { http.DefaultTransport = orig })
	u.updateAll(context.Background())
}

func TestFormatTracerouteOrgNameEmptyInput(t *testing.T) {
	if got := FormatTracerouteOrgName(""); got != "" {
		t.Fatalf("got=%q", got)
	}
}

func TestFormatTracerouteOrgNameLongSingleWord(t *testing.T) {
	got := FormatTracerouteOrgName("ABCDEFGHIJKLMNOPQRSTUVWXYZ")
	if len(got) > tracerouteOrgMaxLen {
		t.Fatalf("got=%q", got)
	}
}

func TestExtractCSVFilesZipEntryOpenError(t *testing.T) {
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	w, _ := zw.Create(asnBlocksIPv4Name)
	_, _ = io.WriteString(w, "a,b\n")
	_ = zw.Close()
	old := zipFileOpen
	zipFileOpen = func(*zip.File) (io.ReadCloser, error) {
		return nil, errors.New("open failed")
	}
	t.Cleanup(func() { zipFileOpen = old })
	if err := extractCSVFiles(bytes.NewReader(buf.Bytes()), t.TempDir(), map[string]struct{}{asnBlocksIPv4Name: {}}); err == nil {
		t.Fatal("expected open error")
	}
}
