package geo

import (
	"archive/tar"
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

	"github.com/yourorg/lg-looking-glass/internal/config"
)

type Updater struct {
	cfg    config.GeoIPConfig
	geoDB  *GeoIPDB
	asnPath  string
	cityPath string
}

func NewUpdater(cfg config.GeoIPConfig, geoDB *GeoIPDB) *Updater {
	u := &Updater{cfg: cfg, geoDB: geoDB}

	dbDir := cfg.DBDir
	if dbDir == "" {
		dbDir = "./data/geoip"
	}

	if cfg.ASNDBPath != "" {
		u.asnPath = cfg.ASNDBPath
	} else {
		u.asnPath = filepath.Join(dbDir, "GeoLite2-ASN.mmdb")
	}
	if cfg.CityDBPath != "" {
		u.cityPath = cfg.CityDBPath
	} else {
		u.cityPath = filepath.Join(dbDir, "GeoLite2-City.mmdb")
	}

	return u
}

func (u *Updater) ASNPath() string  { return u.asnPath }
func (u *Updater) CityPath() string { return u.cityPath }

func (u *Updater) Run(ctx context.Context) {
	interval := 72 * time.Hour
	if u.cfg.UpdateInterval != "" {
		if d, err := time.ParseDuration(u.cfg.UpdateInterval); err == nil {
			interval = d
		}
	}

	slog.Info("geoip updater starting", "asn_path", u.asnPath, "city_path", u.cityPath, "interval", interval)

	if err := os.MkdirAll(filepath.Dir(u.asnPath), 0755); err != nil {
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

func (u *Updater) updateAll(ctx context.Context) {
	needsReload := false

	if err := u.downloadAndExtract(ctx, "GeoLite2-ASN", u.asnPath); err != nil {
		slog.Error("geoip updater: download ASN db", "error", err)
	} else {
		needsReload = true
	}

	if err := u.downloadAndExtract(ctx, "GeoLite2-City", u.cityPath); err != nil {
		slog.Error("geoip updater: download City db", "error", err)
	} else {
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

func (u *Updater) downloadAndExtract(ctx context.Context, edition, targetPath string) error {
	dlURL := fmt.Sprintf("https://download.maxmind.com/app/geoip_download?edition_id=%s&license_key=%s&account_id=%s&suffix=tar.gz",
		url.QueryEscape(edition),
		url.QueryEscape(u.cfg.LicenseKey),
		url.QueryEscape(u.cfg.AccountID),
	)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, dlURL, nil)
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("download: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return fmt.Errorf("download %s: HTTP %d: %s", edition, resp.StatusCode, strings.TrimSpace(string(body)))
	}

	tmpPath := targetPath + ".tmp"
	mmdbFile, err := extractMMDB(resp.Body, edition, tmpPath)
	if err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("extract %s: %w", edition, err)
	}

	if mmdbFile != tmpPath {
		if err := os.Rename(mmdbFile, tmpPath); err != nil {
			os.Remove(mmdbFile)
			return fmt.Errorf("rename temp: %w", err)
		}
	}

	if err := os.Rename(tmpPath, targetPath); err != nil {
		return fmt.Errorf("rename to target: %w", err)
	}

	slog.Info("geoip updated", "edition", edition, "path", targetPath)
	return nil
}

func extractMMDB(r io.Reader, edition, tmpPath string) (string, error) {
	gzr, err := gzip.NewReader(r)
	if err != nil {
		return "", fmt.Errorf("gunzip: %w", err)
	}
	defer gzr.Close()

	tr := tar.NewReader(gzr)
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			return "", fmt.Errorf("no .mmdb file found in archive for %s", edition)
		}
		if err != nil {
			return "", fmt.Errorf("read tar: %w", err)
		}

		if strings.HasSuffix(hdr.Name, ".mmdb") {
			f, err := os.Create(tmpPath)
			if err != nil {
				return "", fmt.Errorf("create temp file: %w", err)
			}
			if _, err := io.Copy(f, tr); err != nil {
				f.Close()
				return "", fmt.Errorf("write mmdb: %w", err)
			}
			f.Close()
			return tmpPath, nil
		}
	}
}
