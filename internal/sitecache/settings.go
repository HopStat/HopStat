package sitecache

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"

	"github.com/HopStat/HopStat/internal/store/queries"
)

var (
	settingsMu     sync.RWMutex
	allSettings    map[string]string
	publicSettings map[string]string
	localAS        uint32
	logoUploadsDir = filepath.Join("data", "uploads")
)

var publicSettingKeys = map[string]bool{
	"site_name": true, "site_description": true, "logo_path": true,
	"header_color": true, "url_website": true, "url_peeringdb": true,
	"url_contact": true, "url_terms": true, "url_privacy": true,
	"active_languages": true,
}

// SetLogoUploadsDir configures where uploaded logos are stored for cache-busting.
func SetLogoUploadsDir(dir string) {
	if strings.TrimSpace(dir) != "" {
		logoUploadsDir = dir
	}
}

func RefreshSettings(db *sql.DB, bgpLocalAS uint32) error {
	q := queries.New(db)
	settings, err := q.GetSettings()
	if err != nil {
		return err
	}
	enrichLogoPath(settings)
	public := make(map[string]string, len(publicSettingKeys)+1)
	for k, v := range settings {
		if publicSettingKeys[k] {
			public[k] = v
		}
	}
	if bgpLocalAS == 0 {
		settingsMu.RLock()
		bgpLocalAS = localAS
		settingsMu.RUnlock()
	}
	allCopy := make(map[string]string, len(settings))
	for k, v := range settings {
		allCopy[k] = v
	}
	if bgpLocalAS > 0 {
		public["local_as"] = strconv.FormatUint(uint64(bgpLocalAS), 10)
	}
	settingsMu.Lock()
	allSettings = allCopy
	publicSettings = public
	localAS = bgpLocalAS
	settingsMu.Unlock()
	return nil
}

// AllSettings returns a copy of all site settings (for query engine and admin reads after refresh).
func AllSettings() map[string]string {
	settingsMu.RLock()
	defer settingsMu.RUnlock()
	if len(allSettings) == 0 {
		return map[string]string{}
	}
	out := make(map[string]string, len(allSettings))
	for k, v := range allSettings {
		out[k] = v
	}
	return out
}

// PublicSettings returns cached public-facing settings.
func PublicSettings() map[string]string {
	settingsMu.RLock()
	defer settingsMu.RUnlock()
	if len(publicSettings) == 0 {
		return map[string]string{}
	}
	out := make(map[string]string, len(publicSettings))
	for k, v := range publicSettings {
		out[k] = v
	}
	return out
}

func enrichLogoPath(settings map[string]string) {
	if settings == nil {
		return
	}
	path, ok := settings["logo_path"]
	if !ok || strings.TrimSpace(path) == "" {
		return
	}
	base := strings.Split(path, "?")[0]
	if !strings.HasPrefix(base, "/logo.") {
		return
	}
	diskPath := filepath.Join(logoUploadsDir, "logo"+strings.TrimPrefix(base, "/logo"))
	info, err := os.Stat(diskPath)
	if err != nil {
		settings["logo_path"] = base
		return
	}
	settings["logo_path"] = fmt.Sprintf("%s?v=%d", base, info.ModTime().Unix())
}
