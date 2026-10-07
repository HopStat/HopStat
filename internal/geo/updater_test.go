package geo

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/HopStat/HopStat/internal/config"
)

func TestParseUpdateInterval(t *testing.T) {
	if got := ParseUpdateInterval("24h", 72*time.Hour); got != 24*time.Hour {
		t.Fatalf("got %v", got)
	}
	if got := ParseUpdateInterval("bad", 72*time.Hour); got != 72*time.Hour {
		t.Fatalf("fallback got %v", got)
	}
}

func TestLastDownloadFromSettings(t *testing.T) {
	settings := map[string]string{
		SettingASNLastDownload:  "2026-06-17T10:00:00Z",
		SettingCityLastDownload: "2026-06-16T10:00:00Z",
	}
	got := LastDownloadFromSettings(settings, "GeoLite2-ASN")
	if got.Format(time.RFC3339) != "2026-06-17T10:00:00Z" {
		t.Fatalf("asn=%v", got)
	}
	got = LastDownloadFromSettings(settings, "GeoLite2-City")
	if got.Format(time.RFC3339) != "2026-06-16T10:00:00Z" {
		t.Fatalf("city=%v", got)
	}
}

func writeASNEditionFiles(t *testing.T, dir string) string {
	t.Helper()
	asnPath := filepath.Join(dir, "GeoLite2-ASN.mmdb")
	for _, name := range []string{"GeoLite2-ASN.mmdb", asnBlocksIPv4Name, asnBlocksIPv6Name} {
		path := filepath.Join(dir, name)
		if err := os.WriteFile(path, []byte("data"), 0644); err != nil {
			t.Fatal(err)
		}
	}
	return asnPath
}

func TestUpdaterNeedsDownloadUsesStoredTime(t *testing.T) {
	dir := t.TempDir()
	asnPath := writeASNEditionFiles(t, dir)

	last := time.Now().UTC().Add(-1 * time.Hour)
	u := NewUpdater(config.GeoIPConfig{}, New("", ""))
	u.SetLastDownload(func(edition string) time.Time {
		if edition == "GeoLite2-ASN" {
			return last
		}
		return time.Time{}
	})

	should, gotLast, next := u.needsDownload("GeoLite2-ASN", asnPath)
	if should {
		t.Fatal("expected download to be skipped")
	}
	if !gotLast.Equal(last) {
		t.Fatalf("last=%v want %v", gotLast, last)
	}
	if !next.Equal(last.Add(72 * time.Hour)) {
		t.Fatalf("next=%v", next)
	}
}

func TestUpdaterNeedsDownloadWhenIntervalElapsed(t *testing.T) {
	dir := t.TempDir()
	asnPath := writeASNEditionFiles(t, dir)

	last := time.Now().UTC().Add(-80 * time.Hour)
	u := NewUpdater(config.GeoIPConfig{}, New("", ""))
	u.SetLastDownload(func(edition string) time.Time {
		return last
	})

	should, _, _ := u.needsDownload("GeoLite2-ASN", asnPath)
	if !should {
		t.Fatal("expected download to be allowed")
	}
}

func TestUpdaterNeedsDownloadWithoutStoredTimeUsesFileModTime(t *testing.T) {
	dir := t.TempDir()
	asnPath := writeASNEditionFiles(t, dir)
	mod := time.Now().UTC().Add(-2 * time.Hour)
	if err := os.Chtimes(asnPath, mod, mod); err != nil {
		t.Fatal(err)
	}

	u := NewUpdater(config.GeoIPConfig{}, New("", ""))
	should, _, _ := u.needsDownload("GeoLite2-ASN", asnPath)
	if should {
		t.Fatal("expected download to be skipped using file mod time")
	}
}

func TestUpdaterResolveCredentialsPrefersSettings(t *testing.T) {
	u := NewUpdater(config.GeoIPConfig{}, New("", ""))
	u.SetCredentials(func() (string, string) {
		return "db-key", "db-account"
	})

	key, account := u.resolveCredentials()
	if key != "db-key" || account != "db-account" {
		t.Fatalf("got key=%q account=%q, want db credentials", key, account)
	}
}

func TestUpdaterNeedsDownloadWhenFileMissingIgnoresStoredTime(t *testing.T) {
	dir := t.TempDir()
	asnPath := filepath.Join(dir, "GeoLite2-ASN.mmdb")

	last := time.Now().UTC().Add(-1 * time.Hour)
	u := NewUpdater(config.GeoIPConfig{}, New("", ""))
	u.asnPath = asnPath
	u.SetLastDownload(func(edition string) time.Time {
		if edition == "GeoLite2-ASN" {
			return last
		}
		return time.Time{}
	})

	should, _, _ := u.needsDownload("GeoLite2-ASN", asnPath)
	if !should {
		t.Fatal("expected download when mmdb file is missing even if last download is recent")
	}
}

func TestUpdaterNeedsDownloadWhenSidecarsMissing(t *testing.T) {
	dir := t.TempDir()
	asnPath := filepath.Join(dir, "GeoLite2-ASN.mmdb")
	if err := os.WriteFile(asnPath, []byte("db"), 0644); err != nil {
		t.Fatal(err)
	}

	last := time.Now().UTC().Add(-1 * time.Hour)
	u := NewUpdater(config.GeoIPConfig{}, New("", ""))
	u.asnPath = asnPath
	u.SetLastDownload(func(edition string) time.Time {
		return last
	})

	should, _, _ := u.needsDownload("GeoLite2-ASN", asnPath)
	if !should {
		t.Fatal("expected download when ASN blocks CSV sidecars are missing")
	}
}

func TestUpdaterNeedsDownloadWhenDirEmpty(t *testing.T) {
	dir := t.TempDir()
	asnPath := filepath.Join(dir, "GeoLite2-ASN.mmdb")
	cityPath := filepath.Join(dir, "GeoLite2-City.mmdb")

	u := NewUpdater(config.GeoIPConfig{}, New("", ""))
	u.asnPath = asnPath
	u.cityPath = cityPath
	u.SetLastDownload(func(edition string) time.Time {
		return time.Now().UTC().Add(-1 * time.Hour)
	})

	for _, edition := range []string{"GeoLite2-ASN", "GeoLite2-City"} {
		target := asnPath
		if edition == "GeoLite2-City" {
			target = cityPath
		}
		should, _, _ := u.needsDownload(edition, target)
		if !should {
			t.Fatalf("expected download for %s when db dir is empty", edition)
		}
	}
}

func TestCSVEditionSidecars(t *testing.T) {
	edition, files := csvEditionSidecars("GeoLite2-ASN")
	if edition != "GeoLite2-ASN-CSV" || len(files) != 2 {
		t.Fatalf("asn csv edition=%q files=%v", edition, files)
	}
	edition, files = csvEditionSidecars("GeoLite2-City")
	if edition != "GeoLite2-City-CSV" || len(files) != 1 || files[0] != cityLocationsName {
		t.Fatalf("city csv edition=%q files=%v", edition, files)
	}
}

func TestExtractCSVFiles(t *testing.T) {
	dir := t.TempDir()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	w, err := zw.Create("GeoLite2-ASN-CSV_20260101/" + asnBlocksIPv4Name)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := w.Write([]byte("network,autonomous_system_number,autonomous_system_organization\n")); err != nil {
		t.Fatal(err)
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}

	wanted := map[string]struct{}{asnBlocksIPv4Name: {}}
	if err := extractCSVFiles(bytes.NewReader(buf.Bytes()), dir, wanted); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, asnBlocksIPv4Name)); err != nil {
		t.Fatalf("expected extracted csv: %v", err)
	}
}

// extractMMDBFile copies the entry through io.LimitReader(tr, maxMMDBSize). io.Copy stops
// at the cap with a nil error, so an entry over the cap used to be written out truncated
// and reported as a *successful* extraction — and the caller then renames that file over
// the live GeoIP database. An oversized entry must be refused instead.
func TestExtractMMDBFileRejectsOversizedEntry(t *testing.T) {
	withCap := func(t *testing.T, limit int64) {
		t.Helper()
		old := maxMMDBSize
		maxMMDBSize = limit
		t.Cleanup(func() { maxMMDBSize = old })
	}

	t.Run("oversized entry is refused", func(t *testing.T) {
		withCap(t, 8)
		target := filepath.Join(t.TempDir(), "out.mmdb")
		body := buildTestMMDBArchive(t, "GeoLite2-ASN.mmdb", bytes.Repeat([]byte("x"), 64))
		if err := extractMMDBFile(bytes.NewReader(body), target); err == nil {
			t.Fatalf("extract accepted a 64 byte entry under an 8 byte cap")
		}
	})

	t.Run("entry exactly at the cap is accepted", func(t *testing.T) {
		withCap(t, 64)
		target := filepath.Join(t.TempDir(), "out.mmdb")
		content := bytes.Repeat([]byte("x"), 64)
		body := buildTestMMDBArchive(t, "GeoLite2-ASN.mmdb", content)
		if err := extractMMDBFile(bytes.NewReader(body), target); err != nil {
			t.Fatalf("extract rejected an entry exactly at the cap: %v", err)
		}
		got, err := os.ReadFile(target)
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(got, content) {
			t.Fatalf("extracted %d bytes, want %d", len(got), len(content))
		}
	})

	t.Run("entry shorter than its header claims is refused", func(t *testing.T) {
		withCap(t, 4096)
		var buf bytes.Buffer
		gw := gzip.NewWriter(&buf)
		tw := tar.NewWriter(gw)
		// Header claims 1024 bytes; only 4 follow before the archive ends.
		if err := tw.WriteHeader(&tar.Header{Name: "GeoLite2-ASN.mmdb", Mode: 0644, Size: 1024}); err != nil {
			t.Fatal(err)
		}
		if _, err := tw.Write([]byte("data")); err != nil {
			t.Fatal(err)
		}
		// Close reports the deliberate under-write; the archive is meant to be short.
		_ = tw.Close()
		if err := gw.Close(); err != nil {
			t.Fatal(err)
		}
		target := filepath.Join(t.TempDir(), "out.mmdb")
		if err := extractMMDBFile(bytes.NewReader(buf.Bytes()), target); err == nil {
			t.Fatal("extract accepted an entry shorter than its header declares")
		}
	})
}
