package server

import (
	"context"
	"errors"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/HopStat/HopStat/internal/domain"
	"github.com/HopStat/HopStat/internal/sharepreview"
)

type fakeShares map[string]*sharepreview.Snapshot

func (f fakeShares) Load(_ context.Context, _ int64, command, target string) (*sharepreview.Snapshot, error) {
	return f[command+" "+target], nil
}

func stubSEO(t *testing.T, settings map[string]string, nodes []*domain.Node) {
	t.Helper()
	prevSettings, prevNodes := seoPublicSettings, seoActiveNodes
	seoPublicSettings = func() map[string]string { return settings }
	seoActiveNodes = func() []*domain.Node { return nodes }
	t.Cleanup(func() { seoPublicSettings, seoActiveNodes = prevSettings, prevNodes })
}

func TestIsAppPath(t *testing.T) {
	for path, want := range map[string]bool{
		"/":                     true,
		"/communities":          true,
		"/admin/settings":       true,
		"/traceroute/1.1.1.1":   true,
		"/sofia/bgp/1.1.1.0/24": true,
		"/ping":                 true,
		"/favicon.ico":          false,
		"/foo/bar":              false,
		"/a/b/ping/x":           false,
	} {
		if got := isAppPath(path); got != want {
			t.Errorf("isAppPath(%q) = %v, want %v", path, got, want)
		}
	}
}

func TestBuildPageQueryLinkUsesSnapshot(t *testing.T) {
	stubSEO(t,
		map[string]string{"site_name": "Edge LG", "active_languages": "tr,en", "logo_path": "/logo.png?v=2"},
		[]*domain.Node{{ID: 4, Name: "Bursa", IsDefault: true}},
	)
	shares := fakeShares{"traceroute 1.1.1.1": {
		Command: "traceroute", At: time.Date(2026, 10, 9, 18, 23, 0, 0, time.UTC),
		Hops: 8, Reached: true, LastRTT: 11.9, LastAS: "AS13335 CLOUDFLARENET",
	}}
	req := httptest.NewRequest("GET", "https://lg.example/traceroute/1.1.1.1", nil)
	req.Header.Set("X-Forwarded-Proto", "https")

	page := buildPage(context.Background(), req, shares)
	if page.ShareTitle != "Bursa noktasından 1.1.1.1 traceroute" {
		t.Fatalf("share title %q", page.ShareTitle)
	}
	if !strings.HasPrefix(page.Description, "8 hopta ulaşıldı · 11,9 ms · AS13335 CLOUDFLARENET") {
		t.Fatalf("description %q", page.Description)
	}
	if page.URL != "https://lg.example/traceroute/1.1.1.1" || page.Image != "https://lg.example/logo.png?v=2" {
		t.Fatalf("url %q image %q", page.URL, page.Image)
	}
	if !page.NoIndex {
		t.Fatal("query links must be noindex")
	}

	index := []byte(`<!doctype html><html lang="en"><head><title>Edge LG</title></head><body></body></html>`)
	out := string(applyPage(index, page))
	for _, want := range []string{
		`<html lang="tr">`,
		`<title>Bursa noktasından 1.1.1.1 traceroute | Edge LG</title>`,
		`<meta property="og:title" content="Bursa noktasından 1.1.1.1 traceroute"`,
		`<meta property="og:locale" content="tr_TR"`,
	} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %s in %s", want, out)
		}
	}
}

func TestBuildPageHome(t *testing.T) {
	stubSEO(t, map[string]string{"site_name": "Edge LG", "site_description": "Network tools", "logo_path": "/logo.svg"}, nil)
	page := buildPage(context.Background(), httptest.NewRequest("GET", "http://lg.example/", nil), nil)
	if page.Title != "Edge LG — Network tools" || page.NoIndex || page.Image != "" {
		t.Fatalf("unexpected home page %+v", page)
	}
	if !strings.Contains(page.JSONLD, `"@type":"WebSite"`) {
		t.Fatalf("home needs WebSite JSON-LD, got %q", page.JSONLD)
	}
}

func TestRobotsAndSitemap(t *testing.T) {
	robots := robotsTxt("https://lg.example")
	if !strings.Contains(robots, "Disallow: /admin") || !strings.Contains(robots, "Sitemap: https://lg.example/sitemap.xml") {
		t.Fatalf("robots.txt %q", robots)
	}
	if !strings.Contains(sitemapXML("https://lg.example"), "<loc>https://lg.example/</loc>") {
		t.Fatal("sitemap must list the home page")
	}
}

type failingShares struct{}

func (failingShares) Load(context.Context, int64, string, string) (*sharepreview.Snapshot, error) {
	return nil, errors.New("db down")
}

func TestBuildPageRoutes(t *testing.T) {
	stubSEO(t, map[string]string{"logo_path": "logo.png"}, []*domain.Node{{ID: 1, Name: "Bursa"}})
	get := func(path string, shares snapshotLoader) sharepreview.Page {
		return buildPage(context.Background(), httptest.NewRequest("GET", "http://lg.example"+path, nil), shares)
	}

	home := get("/", nil)
	if home.SiteName != "Looking Glass" || home.Image != "http://lg.example/logo.png" || !strings.Contains(home.JSONLD, `"logo"`) {
		t.Fatalf("home %+v", home)
	}
	if p := get("/communities", nil); p.URL != "http://lg.example/communities" || p.NoIndex {
		t.Fatalf("communities %+v", p)
	}
	if p := get("/admin/nodes", nil); p.URL != "" || !p.NoIndex {
		t.Fatalf("admin %+v", p)
	}
	if p := get("/nowhere", nil); p.URL != "" || !p.NoIndex {
		t.Fatalf("unknown path %+v", p)
	}
	// A failing snapshot store still yields a preview, from the link alone.
	if p := get("/ping/8.8.8.8", failingShares{}); p.ShareTitle != "Ping 8.8.8.8 from Bursa" {
		t.Fatalf("query link %+v", p)
	}
}

func TestShareImage(t *testing.T) {
	for logo, want := range map[string]string{
		"":                          "",
		"/logo.svg?v=1":             "",
		"https://cdn.example/l.png": "https://cdn.example/l.png",
		"/logo.webp":                "https://lg.example/logo.webp",
	} {
		if got := shareImage("https://lg.example", map[string]string{"logo_path": logo}); got != want {
			t.Errorf("shareImage(%q) = %q, want %q", logo, got, want)
		}
	}
}
