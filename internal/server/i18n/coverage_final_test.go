package i18n

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestLoadTranslationsReadDirError(t *testing.T) {
	if err := LoadTranslations(string([]byte{0})); err == nil {
		t.Fatal("expected read dir error")
	}
}

func TestLoadTranslationsSkipsNonJSONAndBadFiles(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "notes.txt"), []byte("skip"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "bad.json"), []byte("{"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := LoadTranslations(dir); err != nil {
		t.Fatalf("LoadTranslations: %v", err)
	}
}

func TestLoadTranslationsReadFileError(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "en.json"), []byte(`{"k":"v"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	old := readTranslationFile
	readTranslationFile = func(string) ([]byte, error) { return nil, errors.New("read failed") }
	t.Cleanup(func() { readTranslationFile = old })
	if err := LoadTranslations(dir); err != nil {
		t.Fatalf("LoadTranslations: %v", err)
	}
}

func TestDetectLanguageEmptyParts(t *testing.T) {
	old := splitAcceptLanguage
	splitAcceptLanguage = func(string, string) []string { return nil }
	t.Cleanup(func() { splitAcceptLanguage = old })
	if got := DetectLanguage("x"); got != defaultLang {
		t.Fatalf("DetectLanguage = %q", got)
	}
}
