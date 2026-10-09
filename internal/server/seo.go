package server

import (
	"bytes"
	"context"
	"encoding/json"
	"html"
	"log/slog"
	"net/http"
	"strings"

	"github.com/HopStat/HopStat/internal/domain"
	"github.com/HopStat/HopStat/internal/sharepreview"
	"github.com/HopStat/HopStat/internal/sitecache"
)

// Test seams, the same way index_html.go stubs the settings cache.
var (
	seoActiveNodes    = sitecache.ActiveNodes
	seoPublicSettings = sitecache.PublicSettings
)

// snapshotLoader is what the page needs from sharepreview.Store.
type snapshotLoader interface {
	Load(ctx context.Context, nodeID int64, command, target string) (*sharepreview.Snapshot, error)
}

// isAppPath reports whether the SPA has a page at path. Anything else is answered with a
// 404 status, so a mistyped link is not indexed as a copy of the home page.
func isAppPath(path string) bool {
	if path == "/" || path == "/communities" || path == "/admin" || strings.HasPrefix(path, "/admin/") {
		return true
	}
	segments := strings.Split(strings.Trim(path, "/"), "/")
	for i, s := range segments {
		if i > 1 {
			break
		}
		if s == "ping" || s == "traceroute" || s == "bgp" {
			return true
		}
	}
	return false
}

// requestOrigin is the scheme and host the visitor reached, for absolute URLs in the tags.
// X-Forwarded-Proto only ever upgrades to https, so a forged header cannot downgrade links.
func requestOrigin(r *http.Request) string {
	scheme := "http"
	if r.TLS != nil || strings.EqualFold(r.Header.Get("X-Forwarded-Proto"), "https") {
		scheme = "https"
	}
	return scheme + "://" + r.Host
}

// siteLanguage is the first language the admin switched on; the frontend falls back to
// English, then Turkish, when none is set.
func siteLanguage(settings map[string]string) string {
	for _, lang := range strings.Split(settings["active_languages"], ",") {
		lang = strings.ToLower(strings.TrimSpace(lang))
		switch lang {
		case "tr", "en", "de", "fr", "ru":
			return lang
		}
	}
	return "en"
}

// shareImage is the logo as an absolute URL. SVG is left out: link-preview crawlers do not
// render it, and a broken image is worse than none.
func shareImage(origin string, settings map[string]string) string {
	logo := strings.TrimSpace(settings["logo_path"])
	if logo == "" || strings.HasPrefix(strings.Split(logo, "?")[0], "/logo.svg") {
		return ""
	}
	if strings.HasPrefix(logo, "http://") || strings.HasPrefix(logo, "https://") {
		return logo
	}
	if !strings.HasPrefix(logo, "/") {
		logo = "/" + logo
	}
	return origin + logo
}

// buildPage works out the head tags for one request to the SPA.
func buildPage(ctx context.Context, r *http.Request, shares snapshotLoader) sharepreview.Page {
	settings := seoPublicSettings()
	origin := requestOrigin(r)
	siteName := strings.TrimSpace(settings["site_name"])
	if siteName == "" {
		siteName = "Looking Glass"
	}
	description := strings.TrimSpace(settings["site_description"])

	page := sharepreview.Page{
		Lang:        siteLanguage(settings),
		Title:       siteName,
		ShareTitle:  siteName,
		Description: description,
		URL:         origin + "/",
		SiteName:    siteName,
		Image:       shareImage(origin, settings),
	}
	if description != "" {
		page.Title = siteName + " — " + description
	}

	path := r.URL.Path
	switch {
	case path == "/":
		page.JSONLD = websiteJSONLD(siteName, description, origin, page.Image)
		return page
	case path == "/communities":
		page.URL = origin + path
		return page
	case path == "/admin" || strings.HasPrefix(path, "/admin/"):
		page.URL = ""
		page.NoIndex = true
		return page
	}

	q, ok := sharepreview.ParseQueryPath(path, r.URL.Query())
	if !ok {
		page.URL = ""
		page.NoIndex = true
		return page
	}

	nodeName := ""
	var snap *sharepreview.Snapshot
	if node := sharepreview.ResolveNode(seoActiveNodes(), q.Node); node != nil {
		nodeName = node.Name
		enabled := len(node.EnabledCmds) == 0 || node.CanExecute(domain.CommandType(q.Command))
		if enabled && shares != nil {
			loaded, err := shares.Load(ctx, node.ID, q.Command, q.Target)
			if err != nil {
				slog.Warn("failed to load share snapshot", "error", err)
			}
			snap = loaded
		}
	}

	title, desc := sharepreview.Describe(page.Lang, q, nodeName, siteName, snap)
	page.Title = title + " | " + siteName
	page.ShareTitle = title
	page.Description = desc
	page.URL = origin + path
	page.NoIndex = true
	return page
}

func websiteJSONLD(name, description, origin, logo string) string {
	publisher := map[string]any{"@type": "Organization", "name": name}
	if logo != "" {
		publisher["logo"] = map[string]any{"@type": "ImageObject", "url": logo}
	}
	doc := map[string]any{
		"@context":  "https://schema.org",
		"@type":     "WebSite",
		"name":      name,
		"url":       origin + "/",
		"publisher": publisher,
	}
	if description != "" {
		doc["description"] = description
	}
	// A map of strings and maps cannot fail to marshal.
	out, _ := json.Marshal(doc)
	return string(out)
}

// applyPage writes the page into index.html: the language, the title and the head tags.
func applyPage(indexHTML []byte, page sharepreview.Page) []byte {
	out := indexHTML
	if page.Lang != "" {
		out = bytes.Replace(out, []byte(`<html lang="en">`), []byte(`<html lang="`+page.Lang+`">`), 1)
	}
	if start := bytes.Index(out, []byte("<title>")); start >= 0 {
		if end := bytes.Index(out[start:], []byte("</title>")); end >= 0 {
			end += start + len("</title>")
			title := []byte("<title>" + html.EscapeString(page.Title) + "</title>\n    " + page.Tags())
			out = append(append(append([]byte{}, out[:start]...), title...), out[end:]...)
		}
	}
	return out
}

func robotsTxt(origin string) string {
	return "User-agent: *\n" +
		"Allow: /\n" +
		"Disallow: /admin\n" +
		"Disallow: /api/\n" +
		"\n" +
		"Sitemap: " + origin + "/sitemap.xml\n"
}

// sitemapXML lists the one page that is the same for every visitor. Query links are left
// out: there is one per target, they describe live results, and each is marked noindex.
func sitemapXML(origin string) string {
	return `<?xml version="1.0" encoding="UTF-8"?>` + "\n" +
		`<urlset xmlns="http://www.sitemaps.org/schemas/sitemap/0.9">` + "\n" +
		"  <url><loc>" + html.EscapeString(origin+"/") + "</loc></url>\n" +
		"</urlset>\n"
}
