package i18n

import (
	"os"
	"testing"
)

func TestDetectLanguage(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"en-US,en;q=0.9", "en"},
		{"tr-TR,tr;q=0.9", "tr"},
		{"de", "de"},
		{"", "en"},
		{"fr-FR,fr;q=0.8,en-US;q=0.5", "fr"},
	}

	for _, tt := range tests {
		result := DetectLanguage(tt.input)
		if result != tt.expected {
			t.Errorf("DetectLanguage(%q) = %q, want %q", tt.input, result, tt.expected)
		}
	}
}

func TestLoadTranslations(t *testing.T) {
	dir, err := os.MkdirTemp("", "i18n-test")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(dir)

	os.WriteFile(dir+"/en.json", []byte(`{"app.title":"Looking Glass","nav.home":"Home"}`), 0644)
	os.WriteFile(dir+"/tr.json", []byte(`{"app.title":"Looking Glass","nav.home":"Ana Sayfa"}`), 0644)

	if err := LoadTranslations(dir); err != nil {
		t.Fatalf("LoadTranslations error: %v", err)
	}

	if v := T("en", "app.title"); v != "Looking Glass" {
		t.Errorf("T(en, app.title) = %q", v)
	}
	if v := T("tr", "nav.home"); v != "Ana Sayfa" {
		t.Errorf("T(tr, nav.home) = %q", v)
	}
	if v := T("en", "nonexistent"); v != "nonexistent" {
		t.Errorf("T(en, nonexistent) = %q, should return key", v)
	}
	if v := T("xx", "app.title"); v != "Looking Glass" {
		t.Errorf("T(xx, app.title) should fallback to en, got %q", v)
	}
}

func TestLoadTranslationsNonexistentDir(t *testing.T) {
	err := LoadTranslations("/nonexistent/path")
	if err != nil {
		t.Errorf("expected nil for nonexistent dir, got %v", err)
	}
}

func TestNormalizeLang(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"en-US", "en"},
		{"EN", "en"},
		{"tr-TR", "tr"},
		{" de ", "de"},
	}

	for _, tt := range tests {
		result := normalizeLang(tt.input)
		if result != tt.expected {
			t.Errorf("normalizeLang(%q) = %q, want %q", tt.input, result, tt.expected)
		}
	}
}
