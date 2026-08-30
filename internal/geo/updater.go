package geo

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/HopStat/HopStat/internal/config"
)

type Updater struct {
	cfg            config.GeoIPConfig
	geoDB          *GeoIPDB
	asnPath        string
	cityPath       string
	onDownload     func(edition string, downloadedAt time.Time)
	lastDownload   func(edition string) time.Time
	updateInterval func() time.Duration
	credentials    func() (licenseKey, accountID string)
}

func NewUpdater(cfg config.GeoIPConfig, geoDB *GeoIPDB) *Updater {
	u := &Updater{cfg: cfg, geoDB: geoDB}
	u.asnPath, u.cityPath = ResolvePaths(cfg)
	return u
}

func (u *Updater) SetOnDownload(fn func(edition string, downloadedAt time.Time)) {
	u.onDownload = fn
}

func (u *Updater) SetLastDownload(fn func(edition string) time.Time) {
	u.lastDownload = fn
}

func (u *Updater) SetUpdateInterval(fn func() time.Duration) {
	u.updateInterval = fn
}

func (u *Updater) SetCredentials(fn func() (licenseKey, accountID string)) {
	u.credentials = fn
}

func (u *Updater) resolveCredentials() (licenseKey, accountID string) {
	if u.credentials != nil {
		if key, account := u.credentials(); key != "" && account != "" {
			return key, account
		}
	}
	return u.cfg.LicenseKey, u.cfg.AccountID
}

func (u *Updater) ASNPath() string  { return u.asnPath }
func (u *Updater) CityPath() string { return u.cityPath }

func (u *Updater) Run(ctx context.Context) {
	interval := u.resolveInterval()
	slog.Info("geoip updater starting", "asn_path", u.asnPath, "city_path", u.cityPath, "interval", interval)

	if err := updaterMkdirAll(filepath.Dir(u.asnPath), 0755); err != nil {
		slog.Error("geoip updater: create db dir", "error", err)
		return
	}

	u.updateAll(ctx)

	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			slog.Info("geoip updater stopped")
			return
		case <-ticker.C:
			u.updateAll(ctx)
		}
	}
}

func (u *Updater) resolveInterval() time.Duration {
	if u.updateInterval != nil {
		if d := u.updateInterval(); d > 0 {
			return d
		}
	}
	return ParseUpdateInterval(u.cfg.UpdateInterval, 72*time.Hour)
}

func (u *Updater) lastDownloadAt(edition, targetPath string) time.Time {
	if _, err := updaterOsStat(targetPath); err != nil {
		return time.Time{}
	}
	if u.lastDownload != nil {
		if t := u.lastDownload(edition); !t.IsZero() {
			return t
		}
	}
	if info, err := updaterOsStat(targetPath); err == nil {
		return info.ModTime().UTC()
	}
	return time.Time{}
}

func editionSidecarsMissing(edition, dbDir string) bool {
	switch edition {
	case "GeoLite2-ASN":
		for _, name := range []string{asnBlocksIPv4Name, asnBlocksIPv6Name} {
			if _, err := os.Stat(filepath.Join(dbDir, name)); err != nil {
				return true
			}
		}
	case "GeoLite2-City":
		if _, err := os.Stat(filepath.Join(dbDir, cityLocationsName)); err != nil {
			return true
		}
	}
	return false
}

func (u *Updater) needsImmediateDownload(edition, targetPath string) bool {
	if _, err := updaterOsStat(targetPath); err != nil {
		return true
	}
	return editionSidecarsMissing(edition, filepath.Dir(targetPath))
}

func (u *Updater) needsDownload(edition, targetPath string) (bool, time.Time, time.Time) {
	if u.needsImmediateDownload(edition, targetPath) {
		return true, time.Time{}, time.Time{}
	}
	interval := u.resolveInterval()
	last := u.lastDownloadAt(edition, targetPath)
	if last.IsZero() {
		return true, time.Time{}, time.Time{}
	}
	next := last.Add(interval)
	if time.Now().UTC().Before(next) {
		return false, last, next
	}
	return true, last, next
}

func (u *Updater) updateAll(ctx context.Context) {
	// Checked per cycle rather than once at startup: credentials can be entered in the
	// admin panel while the process is running, and that must not need a restart.
	if key, account := u.resolveCredentials(); key == "" || account == "" {
		return
	}

	// Both editions are attempted every time, so these must not be folded into a single
	// short-circuiting expression.
	needsReload := u.tryDownloadEdition(ctx, "GeoLite2-ASN", u.asnPath)
	if u.tryDownloadEdition(ctx, "GeoLite2-City", u.cityPath) {
		needsReload = true
	}

	if needsReload {
		if err := u.geoDB.Reload(); err != nil {
			slog.Error("geoip updater: reload", "error", err)
		} else {
			slog.Info("geoip databases reloaded")
		}
	}
}

func (u *Updater) tryDownloadEdition(ctx context.Context, edition, targetPath string) bool {
	should, last, next := u.needsDownload(edition, targetPath)
	if !should {
		slog.Info("geoip download skipped", "edition", edition, "last_download", last.Format(time.RFC3339), "next_download", next.Format(time.RFC3339))
		return false
	}

	dbDir := filepath.Dir(targetPath)
	_, err := os.Stat(targetPath)
	mmdbMissing := err != nil
	csvMissing := editionSidecarsMissing(edition, dbDir)
	scheduled := !u.needsImmediateDownload(edition, targetPath)

	switch {
	case mmdbMissing || csvMissing:
		slog.Info("geoip download required", "edition", edition, "reason", "missing database files")
	case scheduled:
		slog.Info("geoip download required", "edition", edition, "reason", "update interval elapsed")
	}

	updated := false
	if mmdbMissing || scheduled {
		if err := u.downloadMMDBEdition(ctx, edition, targetPath); err != nil {
			slog.Error("geoip updater: download mmdb", "edition", edition, "error", err)
		} else {
			updated = true
		}
	}
	if csvMissing || scheduled || mmdbMissing {
		if err := u.downloadCSVSidecars(ctx, edition, dbDir); err != nil {
			slog.Error("geoip updater: download csv sidecars", "edition", edition, "error", err)
		} else {
			updated = true
		}
	}
	return updated
}

func csvEditionSidecars(edition string) (csvEdition string, files []string) {
	switch edition {
	case "GeoLite2-ASN":
		return "GeoLite2-ASN-CSV", []string{asnBlocksIPv4Name, asnBlocksIPv6Name}
	case "GeoLite2-City":
		return "GeoLite2-City-CSV", []string{cityLocationsName}
	default:
		return "", nil
	}
}

func (u *Updater) downloadMMDBEdition(ctx context.Context, edition, targetPath string) error {
	licenseKey, accountID := u.resolveCredentials()
	dlURL := fmt.Sprintf("https://download.maxmind.com/app/geoip_download?edition_id=%s&license_key=%s&account_id=%s&suffix=tar.gz",
		url.QueryEscape(edition),
		url.QueryEscape(licenseKey),
		url.QueryEscape(accountID),
	)

	resp, err := u.fetchMaxMind(ctx, dlURL, edition)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	tmpPath := targetPath + ".tmp"
	if err := extractMMDBFile(resp.Body, tmpPath); err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("extract %s: %w", edition, err)
	}

	if err := osRename(tmpPath, targetPath); err != nil {
		return fmt.Errorf("rename to target: %w", err)
	}

	if u.onDownload != nil {
		u.onDownload(edition, time.Now().UTC())
	}

	slog.Info("geoip updated", "edition", edition, "path", targetPath)
	return nil
}

func (u *Updater) downloadCSVSidecars(ctx context.Context, edition, dbDir string) error {
	csvEdition, wantedFiles := csvEditionSidecars(edition)
	if csvEdition == "" || len(wantedFiles) == 0 {
		return nil
	}

	licenseKey, accountID := u.resolveCredentials()
	dlURL := fmt.Sprintf("https://download.maxmind.com/app/geoip_download?edition_id=%s&license_key=%s&account_id=%s&suffix=zip",
		url.QueryEscape(csvEdition),
		url.QueryEscape(licenseKey),
		url.QueryEscape(accountID),
	)

	resp, err := u.fetchMaxMind(ctx, dlURL, csvEdition)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	wanted := make(map[string]struct{}, len(wantedFiles))
	for _, name := range wantedFiles {
		wanted[name] = struct{}{}
	}
	if err := extractCSVFiles(resp.Body, dbDir, wanted); err != nil {
		return fmt.Errorf("extract %s: %w", csvEdition, err)
	}

	if u.onDownload != nil {
		u.onDownload(edition, time.Now().UTC())
	}

	slog.Info("geoip csv sidecars updated", "edition", edition, "csv_edition", csvEdition, "dir", dbDir)
	return nil
}

func (u *Updater) fetchMaxMind(ctx context.Context, dlURL, edition string) (*http.Response, error) {
	req, err := updaterNewRequestWithCtx(ctx, http.MethodGet, dlURL, nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}

	dlClient := &http.Client{Timeout: 10 * time.Minute}
	resp, err := dlClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("download: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		resp.Body.Close()
		return nil, fmt.Errorf("download %s: HTTP %d: %s", edition, resp.StatusCode, strings.TrimSpace(string(body)))
	}
	return resp, nil
}

func extractMMDBFile(r io.Reader, mmdbTmpPath string) error {
	gzr, err := gzip.NewReader(r)
	if err != nil {
		return fmt.Errorf("gunzip: %w", err)
	}
	defer gzr.Close()

	tr := tar.NewReader(gzr)
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("read tar: %w", err)
		}

		base := filepath.Base(hdr.Name)
		if !strings.HasSuffix(base, ".mmdb") {
			continue
		}
		f, err := os.Create(mmdbTmpPath)
		if err != nil {
			return fmt.Errorf("create temp file: %w", err)
		}
		const maxMMDBSize = 200 << 20 // 200 MiB
		if _, err := io.Copy(f, io.LimitReader(tr, maxMMDBSize)); err != nil {
			f.Close()
			return fmt.Errorf("write mmdb: %w", err)
		}
		f.Close()
		return nil
	}
	return fmt.Errorf("no .mmdb file found in archive")
}

func extractCSVFiles(r io.Reader, dbDir string, wanted map[string]struct{}) error {
	body, err := io.ReadAll(r)
	if err != nil {
		return fmt.Errorf("read csv archive: %w", err)
	}
	zr, err := zip.NewReader(bytes.NewReader(body), int64(len(body)))
	if err != nil {
		return fmt.Errorf("open csv zip: %w", err)
	}

	found := make(map[string]struct{}, len(wanted))
	for _, f := range zr.File {
		if f.FileInfo().IsDir() {
			continue
		}
		base := filepath.Base(f.Name)
		if _, ok := wanted[base]; !ok {
			continue
		}
		rc, err := zipFileOpen(f)
		if err != nil {
			return fmt.Errorf("open csv %s: %w", base, err)
		}
		if err := writeArchiveFile(rc, filepath.Join(dbDir, base)); err != nil {
			rc.Close()
			return err
		}
		rc.Close()
		found[base] = struct{}{}
	}
	for name := range wanted {
		if _, ok := found[name]; !ok {
			return fmt.Errorf("csv file %s not found in archive", name)
		}
	}
	return nil
}

var (
	osCreate                 = os.Create
	osRename                 = os.Rename
	updaterNewRequestWithCtx = http.NewRequestWithContext
	updaterMkdirAll          = os.MkdirAll
	updaterOsStat            = os.Stat
	zipFileOpen              = func(f *zip.File) (io.ReadCloser, error) { return f.Open() }
)

func writeArchiveFile(r io.Reader, targetPath string) error {
	tmp := targetPath + ".tmp"
	f, err := osCreate(tmp)
	if err != nil {
		return fmt.Errorf("create csv temp: %w", err)
	}
	const maxCSVSize = 100 << 20 // 100 MiB
	if _, err := io.Copy(f, io.LimitReader(r, maxCSVSize)); err != nil {
		f.Close()
		os.Remove(tmp)
		return fmt.Errorf("write csv: %w", err)
	}
	if err := closeArchiveTempFile(f); err != nil {
		os.Remove(tmp)
		return err
	}
	return osRename(tmp, targetPath)
}

var closeArchiveTempFile = func(f *os.File) error { return f.Close() }
