package sharepreview

import (
	"html"
	"net/url"
	"strconv"
	"strings"
	"unicode"

	"golang.org/x/text/unicode/norm"

	"github.com/HopStat/HopStat/internal/domain"
)

// commandSlugs mirrors COMMAND_SLUGS in web/frontend/src/lib/query-share.ts.
var commandSlugs = map[string]string{
	"ping":       "ping",
	"traceroute": "traceroute",
	"bgp":        "bgp_route",
}

const maxTargetLength = 255

// Query is a shared query link read back off its path.
type Query struct {
	Command string
	Target  string
	// Node is the node slug or id written in the link; empty means the default node.
	Node string
}

// ParseQueryPath mirrors parseQueryLocation in the frontend: /<cmd>/<target...> or
// /<node>/<cmd>/<target...>, with the v2.1.76 ?node=<id> form still honoured.
func ParseQueryPath(path string, query url.Values) (Query, bool) {
	var segments []string
	for _, s := range strings.Split(path, "/") {
		if s != "" {
			segments = append(segments, s)
		}
	}
	if len(segments) < 2 {
		return Query{}, false
	}

	var q Query
	rest := segments[1:]
	if cmd, ok := commandSlugs[segments[0]]; ok {
		q.Command = cmd
	} else if cmd, ok := commandSlugs[segments[1]]; ok && len(segments) > 2 {
		node, err := url.PathUnescape(segments[0])
		if err != nil {
			return Query{}, false
		}
		q.Command, q.Node, rest = cmd, node, segments[2:]
	} else {
		return Query{}, false
	}

	parts := make([]string, 0, len(rest))
	for _, s := range rest {
		decoded, err := url.PathUnescape(s)
		if err != nil {
			return Query{}, false
		}
		parts = append(parts, decoded)
	}
	q.Target = strings.TrimSpace(strings.Join(parts, "/"))
	if q.Target == "" || len(q.Target) > maxTargetLength {
		return Query{}, false
	}
	if q.Node == "" {
		q.Node = strings.TrimSpace(query.Get("node"))
	}
	return q, true
}

// SlugifyNodeName mirrors slugifyNodeName in the frontend: "ŞİŞLİ" → "sisli".
func SlugifyNodeName(name string) string {
	var b strings.Builder
	dash := false
	for _, r := range norm.NFD.String(name) {
		switch {
		case unicode.Is(unicode.Mn, r):
			continue
		case r == 'ı' || r == 'İ':
			r = 'i'
		}
		r = unicode.ToLower(r)
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			if dash && b.Len() > 0 {
				b.WriteByte('-')
			}
			dash = false
			b.WriteRune(r)
			continue
		}
		dash = true
	}
	return b.String()
}

// ResolveNode mirrors resolveLinkedNodeId: an id, then a slug, then the default node.
func ResolveNode(nodes []*domain.Node, token string) *domain.Node {
	var first, marked *domain.Node
	for _, n := range nodes {
		if n == nil {
			continue
		}
		if first == nil {
			first = n
		}
		if marked == nil && n.IsDefault {
			marked = n
		}
	}
	fallback := marked
	if fallback == nil {
		fallback = first
	}
	if token == "" {
		return fallback
	}
	for _, n := range nodes {
		if n != nil && strconv.FormatInt(n.ID, 10) == token {
			return n
		}
	}
	slug := SlugifyNodeName(token)
	for _, n := range nodes {
		if n != nil && SlugifyNodeName(n.Name) == slug {
			return n
		}
	}
	return fallback
}

// Page is the set of tags written into index.html for one request.
type Page struct {
	Lang        string
	Title       string // the document title
	ShareTitle  string // og:title / twitter:title, without the site name
	Description string
	URL         string // canonical, absolute
	SiteName    string
	Image       string // absolute; empty for none
	NoIndex     bool
	JSONLD      string // a ready JSON-LD document, or empty
}

// Tags renders the page's head tags. Every value is escaped; none is trusted.
func (p Page) Tags() string {
	var b strings.Builder
	meta := func(attr, key, value string) {
		if strings.TrimSpace(value) == "" {
			return
		}
		b.WriteString(`<meta ` + attr + `="` + key + `" content="` + html.EscapeString(value) + `" data-ssr />` + "\n    ")
	}
	meta("name", "description", p.Description)
	if p.NoIndex {
		// Results are live and unbounded in number: worth sharing, not worth indexing.
		meta("name", "robots", "noindex, follow")
	}
	if p.URL != "" {
		b.WriteString(`<link rel="canonical" href="` + html.EscapeString(p.URL) + `" />` + "\n    ")
	}
	meta("property", "og:type", "website")
	meta("property", "og:site_name", p.SiteName)
	meta("property", "og:title", p.ShareTitle)
	meta("property", "og:description", p.Description)
	meta("property", "og:url", p.URL)
	meta("property", "og:locale", ogLocale(p.Lang))
	meta("property", "og:image", p.Image)
	meta("name", "twitter:card", "summary")
	meta("name", "twitter:title", p.ShareTitle)
	meta("name", "twitter:description", p.Description)
	meta("name", "twitter:image", p.Image)
	if p.JSONLD != "" {
		// "</" cannot appear inside the script; json.Marshal already escapes "<" as <.
		// The id is the one SiteDocumentHead upserts, so the SPA replaces it rather than adding a second.
		b.WriteString(`<script type="application/ld+json" id="hopstat-site-jsonld" data-ssr>` + p.JSONLD + `</script>` + "\n    ")
	}
	return b.String()
}

func ogLocale(lang string) string {
	switch lang {
	case "tr":
		return "tr_TR"
	case "de":
		return "de_DE"
	case "fr":
		return "fr_FR"
	case "ru":
		return "ru_RU"
	case "en":
		return "en_US"
	}
	return ""
}
