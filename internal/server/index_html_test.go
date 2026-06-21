package server

import (
	"errors"
	"strings"
	"testing"

	"github.com/HopStat/HopStat/internal/sitecache"
	"github.com/HopStat/HopStat/internal/store/queries"
)

func TestInjectIndexHTML(t *testing.T) {
	t.Parallel()

	db := setupTestDB(t)
	if _, err := db.Exec(`CREATE TABLE IF NOT EXISTS settings (key TEXT PRIMARY KEY, value TEXT NOT NULL)`); err != nil {
		t.Fatal(err)
	}
	if err := queries.New(db).SetSettings(map[string]string{
		"site_name":    "Test LG",
		"header_color": "#336699",
	}); err != nil {
		t.Fatal(err)
	}
	if err := sitecache.RefreshSettings(db, 0); err != nil {
		t.Fatal(err)
	}

	indexHTML := []byte(`<!doctype html><html><head><!-- hopstat:bootstrap --><title>Looking Glass</title></head><body></body></html>`)
	out := injectIndexHTML(indexHTML)

	html := string(out)
	if !strings.Contains(html, `<title>Test LG</title>`) {
		t.Fatalf("expected injected title, got %q", html)
	}
	if !strings.Contains(html, `"header_color":"#336699"`) {
		t.Fatalf("expected injected header color, got %q", html)
	}
	if !strings.Contains(html, `"site_name":"Test LG"`) {
		t.Fatalf("expected injected site name, got %q", html)
	}
}

func TestInjectIndexHTML_EmptySettings(t *testing.T) {
	indexHTML := []byte(`<!doctype html><html><head><!-- hopstat:bootstrap --><title>Looking Glass</title></head></html>`)
	out := injectIndexHTML(indexHTML)
	if string(out) != string(indexHTML) {
		t.Fatalf("expected unchanged html")
	}
}

func TestInjectIndexHTML_Defaults(t *testing.T) {
	t.Parallel()
	db := setupTestDB(t)
	if _, err := db.Exec(`CREATE TABLE IF NOT EXISTS settings (key TEXT PRIMARY KEY, value TEXT NOT NULL)`); err != nil {
		t.Fatal(err)
	}
	if err := queries.New(db).SetSettings(map[string]string{"site_name": "", "header_color": ""}); err != nil {
		t.Fatal(err)
	}
	if err := sitecache.RefreshSettings(db, 0); err != nil {
		t.Fatal(err)
	}

	indexHTML := []byte(`<!doctype html><html><head><!-- hopstat:bootstrap --><title>Looking Glass</title></head></html>`)
	out := injectIndexHTML(indexHTML)
	html := string(out)
	if !strings.Contains(html, `<title>Looking Glass</title>`) {
		t.Fatalf("expected default title, got %q", html)
	}
	if !strings.Contains(html, `"header_color":"#1e293b"`) {
		t.Fatalf("expected default header color, got %q", html)
	}
}

func TestInjectIndexHTML_MarshalError(t *testing.T) {
	prev := indexHTMLMarshal
	indexHTMLMarshal = func(v any) ([]byte, error) {
		return nil, errors.New("marshal fail")
	}
	t.Cleanup(func() { indexHTMLMarshal = prev })

	db := setupTestDB(t)
	if _, err := db.Exec(`CREATE TABLE IF NOT EXISTS settings (key TEXT PRIMARY KEY, value TEXT NOT NULL)`); err != nil {
		t.Fatal(err)
	}
	if err := queries.New(db).SetSettings(map[string]string{"site_name": "X"}); err != nil {
		t.Fatal(err)
	}
	if err := sitecache.RefreshSettings(db, 0); err != nil {
		t.Fatal(err)
	}

	indexHTML := []byte(`<!doctype html><html><head><!-- hopstat:bootstrap --><title>Looking Glass</title></head></html>`)
	out := injectIndexHTML(indexHTML)
	if !strings.Contains(string(out), `<title>X</title>`) {
		t.Fatalf("expected title replacement even on marshal error")
	}
}
