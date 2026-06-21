package handler

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

var logoUploadsDir = filepath.Join("data", "uploads")

var uploadsDirAbs = filepath.Abs

func SetLogoUploadsDir(dir string) {
	if strings.TrimSpace(dir) != "" {
		logoUploadsDir = dir
	}
}

func LogoUploadsDir() string {
	return logoUploadsDir
}

func ResolveUploadsDir(dbPath string) string {
	if strings.TrimSpace(dbPath) == "" {
		dbPath = "./lg.db"
	}
	abs, err := uploadsDirAbs(dbPath)
	if err != nil {
		return filepath.Join("data", "uploads")
	}
	return filepath.Join(filepath.Dir(abs), "data", "uploads")
}

func logoFilePath(logoURLPath string) string {
	base := strings.Split(strings.TrimSpace(logoURLPath), "?")[0]
	if !strings.HasPrefix(base, "/logo.") {
		return ""
	}
	return filepath.Join(logoUploadsDir, "logo"+strings.TrimPrefix(base, "/logo"))
}

func logoPathWithCacheBuster(logoPath string) string {
	if strings.TrimSpace(logoPath) == "" {
		return ""
	}

	base := strings.Split(logoPath, "?")[0]
	diskPath := logoFilePath(base)
	if diskPath == "" {
		return logoPath
	}

	info, err := os.Stat(diskPath)
	if err != nil {
		return base
	}

	return fmt.Sprintf("%s?v=%d", base, info.ModTime().Unix())
}

func enrichSettingsLogoPath(settings map[string]string) {
	if settings == nil {
		return
	}
	if path, ok := settings["logo_path"]; ok {
		settings["logo_path"] = logoPathWithCacheBuster(path)
	}
}

func removeLogoFiles() {
	for _, ext := range []string{".png", ".jpg", ".svg", ".webp"} {
		os.Remove(filepath.Join(logoUploadsDir, "logo"+ext))
	}
}
