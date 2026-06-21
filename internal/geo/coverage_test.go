package geo

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"context"
	"database/sql"
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

func testASNPath(t *testing.T) string {
	t.Helper()
	return filepath.Join("testdata", "GeoLite2-ASN-Test.mmdb")
}

func testCityPath(t *testing.T) string {
	t.Helper()
	return filepath.Join("testdata", "GeoLite2-City-Test.mmdb")
}

func testGeoDB(t *testing.T) *GeoIPDB {
	t.Helper()
	g := New(testASNPath(t), testCityPath(t))
	if !g.Enabled() {
		t.Fatal("expected test geo databases to load")
	}
	return g
}

func TestGeoPathsAndBuildInfo(t *testing.T) {
	g := testGeoDB(t)
	asn, city := g.Paths()
	if asn == "" || city == "" {
		t.Fatal("expected paths")
	}
	g.SetPaths("/tmp/asn.mmdb", "/tmp/city.mmdb")
	if a, c := g.Paths(); a != "/tmp/asn.mmdb" || c != "/tmp/city.mmdb" {
		t.Fatalf("paths = %q %q", a, c)
	}
	info := g.BuildInfo()
	if !info.ASNLoaded || !info.CityLoaded || info.ASNBuild <= 0 || info.CityBuild <= 0 {
		t.Fatalf("build info = %+v", info)
	}
}

func TestLookupMMDBAndCity(t *testing.T) {
	g := testGeoDB(t)
	info, err := g.ResolveASN(context.Background(), "1.128.0.1")
	if err != nil || info == nil || info.ASN != 1221 {
		t.Fatalf("ResolveASN = %+v err=%v", info, err)
	}
	city, err := g.LookupCity("2.125.160.216")
	if err != nil || city == nil || city.CountryISO != "GB" {
		t.Fatalf("LookupCity = %+v err=%v", city, err)
	}
}

func TestLookupMMDBHelpers(t *testing.T) {
	g := testGeoDB(t)
	g.mu.RLock()
	asnDB := g.asnDB
	cityDB := g.cityDB
	g.mu.RUnlock()

	ip := parseIP("1.128.0.1")
	info, ok := lookupMMDB(asnDB, cityDB, ip)
	if !ok || info.ASN != 1221 {
		t.Fatalf("lookupMMDB = %+v ok=%v", info, ok)
	}
	if !applyCityRecord(cityDB, parseIP("2.125.160.216"), info) {
		t.Fatal("expected city record applied")
	}
	if _, ok2 := lookupMMDB(asnDB, cityDB, parseIP("240.0.0.1")); ok2 {
		t.Fatal("expected miss for unknown IP")
	}
}

func parseIP(s string) net.IP {
	return net.ParseIP(s)
}

func TestResolveCacheOverflow(t *testing.T) {
	g := New("", "")
	g.resolveCache = make(map[string]domain.ASInfo, maxResolveCacheEntries)
	for i := 0; i < maxResolveCacheEntries+1; i++ {
		g.storeResolve("1.2.3.4", &domain.ASInfo{ASN: uint32(i + 1)})
	}
	if len(g.resolveCache) > maxResolveCacheEntries {
		t.Fatalf("cache size = %d", len(g.resolveCache))
	}
}

func TestRememberAndLookupASNIndex(t *testing.T) {
	g := New("", "")
	g.rememberASN(nil)
	g.rememberASN(&domain.ASInfo{ASN: 0})
	g.rememberASN(&domain.ASInfo{ASN: 15169, OrgName: "GOOGLE"})
	if info, ok := g.lookupASNFromIndex(15169); !ok || info.OrgName != "GOOGLE" {
		t.Fatalf("lookup = %+v ok=%v", info, ok)
	}
	if _, ok := g.lookupASNFromIndex(999); ok {
		t.Fatal("expected miss")
	}
}

func TestLookupASByNumberMerge(t *testing.T) {
	g := New("", "")
	g.asnIndex = map[uint32]domain.ASInfo{15169: {OrgName: "GOOGLE"}}
	info, err := g.LookupASByNumber(context.Background(), 15169)
	if err != nil || info.OrgName != "GOOGLE" {
		t.Fatalf("info = %+v err=%v", info, err)
	}
	if out := g.mergeASByNumberDNS(context.Background(), nil); out != nil {
		t.Fatal("expected nil")
	}
	if out := g.mergeASByNumberDNS(context.Background(), &domain.ASInfo{ASN: 0}); out.ASN != 0 {
		t.Fatal("expected zero asn passthrough")
	}
	if info, err := g.LookupASByNumber(context.Background(), 0); err != nil || info.ASN != 0 {
		t.Fatalf("zero asn = %+v err=%v", info, err)
	}
}

func TestParseCymruShortRecord(t *testing.T) {
	country, org := parseCymruASNRecord("15169 | US")
	if country != "US" || org != "" {
		t.Fatalf("country=%q org=%q", country, org)
	}
	if cc := countryFromOrgSuffix("Example Corp, X1"); cc != "" {
		t.Fatalf("unexpected country %q", cc)
	}
	if cc := countryFromOrgSuffix("Example Corp, US"); cc != "US" {
		t.Fatalf("got %q", cc)
	}
}

func TestReloadSuccess(t *testing.T) {
	g := testGeoDB(t)
	if err := g.Reload(); err != nil {
		t.Fatalf("Reload: %v", err)
	}
	if !g.Enabled() {
		t.Fatal("expected enabled after reload")
	}
}

func TestReloadPartialFailure(t *testing.T) {
	dir := t.TempDir()
	asnPath := filepath.Join(dir, "GeoLite2-ASN.mmdb")
	cityPath := filepath.Join(dir, "GeoLite2-City.mmdb")
	if err := os.WriteFile(cityPath, readTestFile(t, testCityPath(t)), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(asnPath, []byte("bad"), 0644); err != nil {
		t.Fatal(err)
	}
	g := New("", "")
	g.asnPath = asnPath
	g.cityPath = cityPath
	if err := g.Reload(); err == nil {
		t.Fatal("expected reload error for bad asn db")
	}
	if !g.Enabled() {
		t.Fatal("expected city db to keep geo enabled")
	}
}

func readTestFile(t *testing.T, path string) []byte {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func TestTrimToLenAndAbbreviate(t *testing.T) {
	if got := trimToLen("hello", 3); got != "hel" {
		t.Fatalf("trimToLen = %q", got)
	}
	if got := trimToLen("hi", 5); got != "hi" {
		t.Fatalf("trimToLen = %q", got)
	}
	if got := abbreviateTracerouteName("INTERNATIONAL BUSINESS MACHINES CORPORATION", 20); len(got) > 20 {
		t.Fatalf("too long: %q", got)
	}
	if got := abbreviateTracerouteName("AB", 20); got != "AB" {
		t.Fatalf("short name = %q", got)
	}
}

func TestLookupIPFullReport(t *testing.T) {
	dir := t.TempDir()
	blocksPath := filepath.Join(dir, asnBlocksIPv4Name)
	if err := os.WriteFile(blocksPath, []byte(`network,geoname_id,registered_country_geoname_id,represented_country_geoname_id,is_anonymous_proxy,is_satellite_provider,autonomous_system_number,autonomous_system_organization
1.128.0.0/17,,,,0,0,1221,TestOrg
`), 0644); err != nil {
		t.Fatal(err)
	}
	g := New(testASNPath(t), testCityPath(t))
	g.asnPath = testASNPath(t)
	g.reloadASNIndex()

	report, err := g.LookupIP(context.Background(), "1.128.0.1")
	if err != nil {
		t.Fatal(err)
	}
	if report.Result == nil || report.Result.ASN == 0 {
		t.Fatalf("report = %+v", report)
	}
	if report.City == nil {
		t.Fatal("expected city info when city db loaded")
	}
}

func TestCityToLookupCityAndClone(t *testing.T) {
	if cloneASInfo(nil) != nil {
		t.Fatal("expected nil clone")
	}
	in := &domain.ASInfo{ASN: 1, OrgName: "x"}
	out := cloneASInfo(in)
	if out == in || out.ASN != 1 {
		t.Fatal("expected copy")
	}
	c := cityToLookupCity(&GeoCityInfo{CountryISO: "US", City: "NYC"})
	if c == nil || c.CountryISO != "US" {
		t.Fatalf("city = %+v", c)
	}
	if cityToLookupCity(nil) != nil {
		t.Fatal("expected nil")
	}
}

func TestChosenSourceBranches(t *testing.T) {
	if got := chosenSource(false, false, &domain.ASInfo{ASN: 1}, nil, nil); got != SourceNone {
		t.Fatalf("got %q", got)
	}
	if got := inferChosenSource(false, false, true); got != SourceDNS {
		t.Fatalf("got %q", got)
	}
}

func TestResolveUpdateInterval(t *testing.T) {
	got := ResolveUpdateInterval(map[string]string{SettingUpdateInterval: "24h"}, config.GeoIPConfig{})
	if got != 24*time.Hour {
		t.Fatalf("got %v", got)
	}
	got = ResolveUpdateInterval(map[string]string{}, config.GeoIPConfig{UpdateInterval: "48h"})
	if got != 48*time.Hour {
		t.Fatalf("got %v", got)
	}
}

func TestCollectStatusWithGeo(t *testing.T) {
	dir := t.TempDir()
	asnPath := filepath.Join(dir, "GeoLite2-ASN.mmdb")
	if err := os.WriteFile(asnPath, readTestFile(t, testASNPath(t)), 0644); err != nil {
		t.Fatal(err)
	}
	mod := time.Now().UTC().Add(-time.Hour)
	_ = os.Chtimes(asnPath, mod, mod)

	settings := map[string]string{
		SettingLicenseKey: "key",
		SettingAccountID:  "acc",
	}
	st := CollectStatus(settings, config.GeoIPConfig{ASNDBPath: asnPath}, testGeoDB(t))
	if !st.Configured || !st.ASNLoaded || st.ASNBuildDate == "" {
		t.Fatalf("status = %+v", st)
	}
	if got := epochToRFC3339(0); got != "" {
		t.Fatalf("epoch zero = %q", got)
	}
	if got := fileModTimeRFC3339("/nonexistent"); got != "" {
		t.Fatalf("missing file mod = %q", got)
	}
}

func TestSyncSettings(t *testing.T) {
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err := store.Migrate(db); err != nil {
		t.Fatal(err)
	}
	q := queries.New(db)
	if err := SyncSettings(q, config.GeoIPConfig{
		LicenseKey:     "cfg-key",
		AccountID:      "cfg-account",
		UpdateInterval: "24h",
	}); err != nil {
		t.Fatal(err)
	}
	settings, err := q.GetSettings()
	if err != nil {
		t.Fatal(err)
	}
	if settings[SettingLicenseKey] != "cfg-key" {
		t.Fatalf("settings = %+v", settings)
	}
	if err := SyncSettings(q, config.GeoIPConfig{}); err != nil {
		t.Fatal(err)
	}
}

func TestUpdaterRunAndDownload(t *testing.T) {
	dir := t.TempDir()
	asnPath := filepath.Join(dir, "GeoLite2-ASN.mmdb")
	cityPath := filepath.Join(dir, "GeoLite2-City.mmdb")

	mmdbBody := buildTestMMDBArchive(t, "GeoLite2-ASN.mmdb", readTestFile(t, testASNPath(t)))
	csvBody := buildTestCSVArchive(t)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if stringsContains(r.URL.RawQuery, "suffix=tar.gz") {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write(mmdbBody)
			return
		}
		if stringsContains(r.URL.RawQuery, "suffix=zip") {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write(csvBody)
			return
		}
		http.NotFound(w, r)
	}))
	defer srv.Close()

	origTransport := http.DefaultTransport
	http.DefaultTransport = roundTripperFunc(func(req *http.Request) (*http.Response, error) {
		u := req.URL.String()
		u = replaceHost(u, srv.URL)
		req2 := req.Clone(req.Context())
		req2.URL, _ = urlParse(u)
		return origTransport.RoundTrip(req2)
	})
	defer func() { http.DefaultTransport = origTransport }()

	g := New("", "")
	u := NewUpdater(config.GeoIPConfig{
		LicenseKey:     "key",
		AccountID:      "acc",
		UpdateInterval: "72h",
		DBDir:          dir,
	}, g)
	u.asnPath = asnPath
	u.cityPath = cityPath
	u.SetCredentials(func() (string, string) { return "key", "acc" })
	u.SetUpdateInterval(func() time.Duration { return time.Millisecond })
	var downloaded []string
	u.SetOnDownload(func(edition string, _ time.Time) { downloaded = append(downloaded, edition) })

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		u.Run(ctx)
		close(done)
	}()
	time.Sleep(20 * time.Millisecond)
	cancel()
	<-done

	if _, err := os.Stat(asnPath); err != nil {
		t.Fatalf("asn db missing: %v", err)
	}
	if len(downloaded) == 0 {
		t.Fatal("expected download callback")
	}
	if u.ASNPath() != asnPath || u.CityPath() != cityPath {
		t.Fatal("path accessors mismatch")
	}
}

func buildTestMMDBArchive(t *testing.T, name string, content []byte) []byte {
	t.Helper()
	var buf bytes.Buffer
	gw := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gw)
	if err := tw.WriteHeader(&tar.Header{Name: "dir/" + name, Mode: 0644, Size: int64(len(content))}); err != nil {
		t.Fatal(err)
	}
	if _, err := tw.Write(content); err != nil {
		t.Fatal(err)
	}
	_ = tw.Close()
	_ = gw.Close()
	return buf.Bytes()
}

func buildTestCSVArchive(t *testing.T) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for _, item := range []struct {
		name string
		body string
	}{
		{asnBlocksIPv4Name, "network,autonomous_system_number,autonomous_system_organization\n"},
		{asnBlocksIPv6Name, "network,autonomous_system_number,autonomous_system_organization\n"},
		{cityLocationsName, "geoname_id,locale_code,country_iso_code\n"},
	} {
		w, err := zw.Create("csv/" + item.name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := w.Write([]byte(item.body)); err != nil {
			t.Fatal(err)
		}
	}
	_ = zw.Close()
	return buf.Bytes()
}

func TestExtractMMDBAndCSVFailures(t *testing.T) {
	dir := t.TempDir()
	if err := extractMMDBFile(bytes.NewReader([]byte("not-gzip")), filepath.Join(dir, "x.mmdb")); err == nil {
		t.Fatal("expected gunzip error")
	}
	if err := extractCSVFiles(bytes.NewReader([]byte("bad")), dir, map[string]struct{}{"missing.csv": {}}); err == nil {
		t.Fatal("expected csv zip error")
	}
}

func TestWriteArchiveFileError(t *testing.T) {
	if err := writeArchiveFile(bytes.NewReader([]byte("x")), "/no/such/dir/file.csv"); err == nil {
		t.Fatal("expected write error")
	}
}

func TestUpdaterResolveIntervalFallback(t *testing.T) {
	u := NewUpdater(config.GeoIPConfig{UpdateInterval: "72h"}, New("", ""))
	if got := u.resolveInterval(); got != 72*time.Hour {
		t.Fatalf("got %v", got)
	}
	u.SetUpdateInterval(func() time.Duration { return 0 })
	if got := u.resolveInterval(); got != 72*time.Hour {
		t.Fatalf("fallback got %v", got)
	}
}

func TestUpdaterResolveCredentialsConfig(t *testing.T) {
	u := NewUpdater(config.GeoIPConfig{LicenseKey: "k", AccountID: "a"}, New("", ""))
	key, account := u.resolveCredentials()
	if key != "k" || account != "a" {
		t.Fatalf("got %q %q", key, account)
	}
}

func TestResolvePathsCustom(t *testing.T) {
	asn, city := ResolvePaths(config.GeoIPConfig{
		DBDir:      "/data",
		ASNDBPath:  "/custom/asn.mmdb",
		CityDBPath: "/custom/city.mmdb",
	})
	if asn != "/custom/asn.mmdb" || city != "/custom/city.mmdb" {
		t.Fatalf("paths = %q %q", asn, city)
	}
}

type roundTripperFunc func(*http.Request) (*http.Response, error)

func (f roundTripperFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func stringsContains(s, sub string) bool { return bytes.Contains([]byte(s), []byte(sub)) }

func replaceHost(raw, serverURL string) string {
	// Keep query string, swap scheme/host with test server.
	if i := indexByte(raw, '?'); i >= 0 {
		return serverURL + raw[i:]
	}
	return serverURL
}

func indexByte(s string, c byte) int {
	for i := 0; i < len(s); i++ {
		if s[i] == c {
			return i
		}
	}
	return -1
}

func urlParse(raw string) (*url.URL, error) {
	return url.Parse(raw)
}
