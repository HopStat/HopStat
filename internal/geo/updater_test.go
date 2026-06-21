package geo

import (
	"archive/zip"
	"bytes"
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
	u := NewUpdater(config.GeoIPConfig{UpdateInterval: "72h"}, New("", ""))
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
	u := NewUpdater(config.GeoIPConfig{UpdateInterval: "72h"}, New("", ""))
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

	u := NewUpdater(config.GeoIPConfig{UpdateInterval: "72h"}, New("", ""))
	should, _, _ := u.needsDownload("GeoLite2-ASN", asnPath)
	if should {
		t.Fatal("expected download to be skipped using file mod time")
	}
}

func TestUpdaterResolveCredentialsPrefersSettings(t *testing.T) {
	u := NewUpdater(config.GeoIPConfig{
		LicenseKey: "cfg-key",
		AccountID:  "cfg-account",
	}, New("", ""))
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
	u := NewUpdater(config.GeoIPConfig{UpdateInterval: "72h"}, New("", ""))
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
	u := NewUpdater(config.GeoIPConfig{UpdateInterval: "72h"}, New("", ""))
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

	u := NewUpdater(config.GeoIPConfig{UpdateInterval: "72h"}, New("", ""))
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
