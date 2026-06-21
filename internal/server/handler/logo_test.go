package handler

import (
	"os"
	"path/filepath"
	"strconv"
	"testing"
)

func TestResolveUploadsDir(t *testing.T) {
	tmp := t.TempDir()
	dbPath := filepath.Join(tmp, "lg.db")
	got := ResolveUploadsDir(dbPath)
	want := filepath.Join(tmp, "data", "uploads")
	if got != want {
		t.Fatalf("ResolveUploadsDir() = %q, want %q", got, want)
	}
	if got := ResolveUploadsDir(""); !stringsHasPrefix(got, string(filepath.Separator)) && got == "" {
		t.Fatalf("unexpected empty default")
	}
}

func TestLogoUploadsDirAndHelpers(t *testing.T) {
	if LogoUploadsDir() == "" {
		t.Fatal("expected uploads dir")
	}
	SetLogoUploadsDir("")
	if LogoUploadsDir() == "" {
		t.Fatal("empty set should not clear dir")
	}

	dir := t.TempDir()
	SetLogoUploadsDir(dir)
	removeLogoFiles()

	if got := logoFilePath("/not-logo.png"); got != "" {
		t.Fatalf("got %q", got)
	}

	enrichSettingsLogoPath(nil)
	settings := map[string]string{"logo_path": "/logo.png"}
	enrichSettingsLogoPath(settings)
}

func TestLogoPathWithCacheBuster(t *testing.T) {
	uploadDir := filepath.Join(t.TempDir(), "uploads")
	if err := os.MkdirAll(uploadDir, 0o755); err != nil {
		t.Fatal(err)
	}
	SetLogoUploadsDir(uploadDir)

	logoFile := filepath.Join(uploadDir, "logo.png")
	if err := os.WriteFile(logoFile, []byte("png"), 0o644); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(logoFile)
	if err != nil {
		t.Fatal(err)
	}

	got := logoPathWithCacheBuster("/logo.png")
	want := "/logo.png?v=" + strconv.FormatInt(info.ModTime().Unix(), 10)
	if got != want {
		t.Fatalf("logoPathWithCacheBuster() = %q, want %q", got, want)
	}

	if got := logoPathWithCacheBuster("/logo.png?v=1"); got != want {
		t.Fatalf("stale query param = %q, want %q", got, want)
	}

	if got := logoPathWithCacheBuster(""); got != "" {
		t.Fatalf("empty path = %q, want empty", got)
	}

	if err := os.Remove(logoFile); err != nil {
		t.Fatal(err)
	}
	if got := logoPathWithCacheBuster("/logo.png"); got != "/logo.png" {
		t.Fatalf("missing file = %q, want /logo.png", got)
	}
}

func stringsHasPrefix(s, prefix string) bool {
	return len(s) >= len(prefix) && s[:len(prefix)] == prefix
}
