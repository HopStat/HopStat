package sharepreview

import (
	"context"
	"database/sql"
	"net/url"
	"strings"
	"testing"
	"time"

	_ "modernc.org/sqlite"

	"github.com/HopStat/HopStat/internal/domain"
	"github.com/HopStat/HopStat/internal/store"
)

var at = time.Date(2026, 10, 9, 18, 23, 0, 0, time.UTC)

func TestParseQueryPath(t *testing.T) {
	cases := []struct {
		path, search string
		want         Query
		ok           bool
	}{
		{"/traceroute/1.1.1.1", "", Query{Command: "traceroute", Target: "1.1.1.1"}, true},
		{"/bgp/1.1.1.0/24", "", Query{Command: "bgp_route", Target: "1.1.1.0/24"}, true},
		{"/sofia/ping/8.8.8.8", "", Query{Command: "ping", Target: "8.8.8.8", Node: "sofia"}, true},
		{"/ping/8.8.8.8", "node=3", Query{Command: "ping", Target: "8.8.8.8", Node: "3"}, true},
		{"/ping", "", Query{}, false},
		{"/sofia/ping", "", Query{}, false},
		{"/foo/bar", "", Query{}, false},
		{"/ping/%zz", "", Query{}, false},
	}
	for _, tc := range cases {
		values, _ := url.ParseQuery(tc.search)
		got, ok := ParseQueryPath(tc.path, values)
		if ok != tc.ok || got != tc.want {
			t.Errorf("ParseQueryPath(%q, %q) = %+v, %v; want %+v, %v", tc.path, tc.search, got, ok, tc.want, tc.ok)
		}
	}
}

// Must agree with slugifyNodeName in web/frontend/src/lib/query-share.ts.
func TestSlugifyNodeName(t *testing.T) {
	cases := map[string]string{
		"ŞİŞLİ":         "sisli",
		"Smoke Node":    "smoke-node",
		"  Istanbul-1 ": "istanbul-1",
		"Çağlayan/Ümit": "caglayan-umit",
		"ıI":            "ii",
		"--":            "",
	}
	for in, want := range cases {
		if got := SlugifyNodeName(in); got != want {
			t.Errorf("SlugifyNodeName(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestResolveNode(t *testing.T) {
	nodes := []*domain.Node{
		{ID: 1, Name: "Bursa"},
		{ID: 2, Name: "Esenyurt", IsDefault: true},
	}
	if n := ResolveNode(nodes, ""); n == nil || n.ID != 2 {
		t.Fatalf("empty token should pick the default node, got %+v", n)
	}
	if n := ResolveNode(nodes, "1"); n == nil || n.ID != 1 {
		t.Fatalf("id token, got %+v", n)
	}
	if n := ResolveNode(nodes, "bursa"); n == nil || n.ID != 1 {
		t.Fatalf("slug token, got %+v", n)
	}
	if n := ResolveNode(nodes, "nowhere"); n == nil || n.ID != 2 {
		t.Fatalf("unknown token should fall back to the default node, got %+v", n)
	}
	if n := ResolveNode(nil, ""); n != nil {
		t.Fatalf("no nodes, got %+v", n)
	}
}

func TestFromResultTraceroute(t *testing.T) {
	result := &domain.QueryResult{Status: domain.StatusDone, Parsed: &domain.TracerouteResult{Hops: []domain.Hop{
		{Number: 1, IP: "192.168.0.1", RTT: []float64{0.6}},
		{Number: 2},
		{Number: 3, IP: "1.1.1.1", RTT: []float64{11, 12, 13}, ASInfo: &domain.ASInfo{ASN: 13335, ShortName: "CLOUDFLARENET"}},
	}}}
	snap, ok := FromResult("traceroute", "1.1.1.1", result, at)
	if !ok {
		t.Fatal("expected a snapshot")
	}
	if snap.Hops != 3 || !snap.Reached || snap.LastRTT != 12 || snap.LastAS != "AS13335 CLOUDFLARENET" {
		t.Fatalf("unexpected snapshot %+v", snap)
	}
	_, desc := Describe("en", Query{Command: "traceroute", Target: "1.1.1.1"}, "local", "LG", snap)
	if desc != "Reached in 3 hops · 12.0 ms · AS13335 CLOUDFLARENET. Last run 2026-10-09 18:23 UTC." {
		t.Fatalf("description %q", desc)
	}
	_, desc = Describe("tr", Query{Command: "traceroute", Target: "1.1.1.1"}, "local", "LG", snap)
	if !strings.HasPrefix(desc, "3 hopta ulaşıldı · 12,0 ms") {
		t.Fatalf("turkish description %q", desc)
	}
}

func TestFromResultPing(t *testing.T) {
	result := &domain.QueryResult{Status: domain.StatusDone, Parsed: &domain.PingResult{
		PacketsSent: 5, PacketsRecv: 5, AvgRTT: 11.94, MinRTT: 11.2, MaxRTT: 12.61,
	}}
	snap, ok := FromResult("ping", "8.8.8.8", result, at)
	if !ok {
		t.Fatal("expected a snapshot")
	}
	title, desc := Describe("en", Query{Command: "ping", Target: "8.8.8.8"}, "Bursa", "LG", snap)
	if title != "Ping 8.8.8.8 from Bursa" {
		t.Fatalf("title %q", title)
	}
	if !strings.HasPrefix(desc, "5/5 replies, 0% loss · avg 11.9 ms (min 11.2, max 12.6 ms)") {
		t.Fatalf("description %q", desc)
	}
}

func TestFromResultBGP(t *testing.T) {
	result := &domain.QueryResult{Status: domain.StatusDone, Parsed: &domain.BGPResult{
		Routes: []domain.BGPRoute{
			{Prefix: "1.1.1.0/24", ASPath: []uint32{9121, 1299, 13335}},
			{Prefix: "1.1.1.0/24", ASPath: []uint32{9121, 13335}, Best: true},
		},
		TargetAS: &domain.ASInfo{ASN: 13335, ShortName: "CLOUDFLARENET"},
	}}
	snap, ok := FromResult("bgp_route", "1.1.1.1", result, at)
	if !ok {
		t.Fatal("expected a snapshot")
	}
	_, desc := Describe("en", Query{Command: "bgp_route", Target: "1.1.1.1"}, "local", "LG", snap)
	if !strings.HasPrefix(desc, "1.1.1.0/24 · AS path 9121 13335 · origin AS13335 CLOUDFLARENET") {
		t.Fatalf("description %q", desc)
	}

	empty := &domain.QueryResult{Status: domain.StatusDone, Parsed: &domain.BGPResult{}}
	snap, ok = FromResult("bgp_route", "10.0.0.1", empty, at)
	if !ok || !snap.NoRoute {
		t.Fatalf("an empty table is still an outcome, got %+v %v", snap, ok)
	}
}

func TestFromResultSkipsFailures(t *testing.T) {
	if _, ok := FromResult("ping", "x", &domain.QueryResult{Status: domain.StatusError}, at); ok {
		t.Fatal("a failed query must not be previewed")
	}
	if _, ok := FromResult("ping", "x", nil, at); ok {
		t.Fatal("nil result")
	}
}

func TestDescribeWithoutSnapshot(t *testing.T) {
	title, desc := Describe("en", Query{Command: "traceroute", Target: "1.1.1.1"}, "local", "LG", nil)
	if title != "Traceroute to 1.1.1.1 from local" || desc != "Run a live traceroute to 1.1.1.1 from local on LG." {
		t.Fatalf("got %q / %q", title, desc)
	}
}

func TestTagsEscape(t *testing.T) {
	tags := Page{ShareTitle: `"><script>`, Description: "a & b", URL: "https://lg.example/ping/x", NoIndex: true}.Tags()
	if strings.Contains(tags, "<script>") {
		t.Fatalf("unescaped value in %q", tags)
	}
	for _, want := range []string{`content="a &amp; b"`, `rel="canonical"`, `content="noindex, follow"`} {
		if !strings.Contains(tags, want) {
			t.Errorf("missing %s in %q", want, tags)
		}
	}
}

func TestStoreRoundTrip(t *testing.T) {
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	db.SetMaxOpenConns(1)
	t.Cleanup(func() { db.Close() })
	if err := store.Migrate(db); err != nil {
		t.Fatal(err)
	}

	s := NewStore(db)
	s.now = func() time.Time { return at }
	ctx := context.Background()

	first := &Snapshot{Command: "ping", At: at, Sent: 5, Recv: 4}
	if err := s.Save(ctx, 7, " 8.8.8.8 ", first); err != nil {
		t.Fatal(err)
	}
	second := &Snapshot{Command: "ping", At: at, Sent: 5, Recv: 5}
	if err := s.Save(ctx, 7, "8.8.8.8", second); err != nil {
		t.Fatal(err)
	}
	got, err := s.Load(ctx, 7, "ping", "8.8.8.8")
	if err != nil || got == nil || got.Recv != 5 {
		t.Fatalf("expected the latest snapshot, got %+v, %v", got, err)
	}
	if got, _ := s.Load(ctx, 8, "ping", "8.8.8.8"); got != nil {
		t.Fatalf("another node must not see it, got %+v", got)
	}

	s.now = func() time.Time { return at.Add(Retention + time.Hour) }
	if got, _ := s.Load(ctx, 7, "ping", "8.8.8.8"); got != nil {
		t.Fatalf("an expired snapshot must not describe the link, got %+v", got)
	}
}

func TestParseQueryPathRejects(t *testing.T) {
	for _, path := range []string{"/%zz/ping/8.8.8.8", "/ping/" + strings.Repeat("a", maxTargetLength+1), "/ping/%20"} {
		if q, ok := ParseQueryPath(path, nil); ok {
			t.Errorf("ParseQueryPath(%q) = %+v, want rejected", path, q)
		}
	}
}

func TestResolveNodeSkipsNil(t *testing.T) {
	nodes := []*domain.Node{nil, {ID: 5, Name: "Ankara"}}
	if n := ResolveNode(nodes, ""); n == nil || n.ID != 5 {
		t.Fatalf("first non-nil node is the fallback, got %+v", n)
	}
}

func TestFromResultEdgeCases(t *testing.T) {
	done := func(parsed any) *domain.QueryResult {
		return &domain.QueryResult{Status: domain.StatusDone, Parsed: parsed}
	}
	for name, result := range map[string]*domain.QueryResult{
		"ping without packets":    done(&domain.PingResult{}),
		"nil ping":                done((*domain.PingResult)(nil)),
		"traceroute without hops": done(&domain.TracerouteResult{}),
		"nil traceroute":          done((*domain.TracerouteResult)(nil)),
		"unknown parsed type":     done("raw text"),
	} {
		if _, ok := FromResult("ping", "x", result, at); ok {
			t.Errorf("%s: expected no snapshot", name)
		}
	}

	// Unnumbered hops fall back to the hop count; silent hops are skipped.
	snap, ok := FromResult("traceroute", "9.9.9.9", done(&domain.TracerouteResult{Hops: []domain.Hop{
		{IP: "10.0.0.1", RTT: []float64{1}, ASInfo: &domain.ASInfo{OrgName: "Example Org"}},
		{IP: "10.0.0.2"},
	}}), at)
	if !ok || snap.Hops != 2 || snap.Reached || snap.LastAS != "Example Org" {
		t.Fatalf("unexpected snapshot %+v", snap)
	}

	// No TargetAS: the origin is named by its number alone.
	snap, ok = FromResult("bgp_route", "x", done(&domain.BGPResult{Routes: []domain.BGPRoute{{Prefix: "p", ASPath: []uint32{1, 2}}}}), at)
	if !ok || snap.OriginAS != "AS2" {
		t.Fatalf("unexpected snapshot %+v", snap)
	}
	// An empty AS path names no origin.
	snap, ok = FromResult("bgp_route", "x", done(&domain.BGPResult{Routes: []domain.BGPRoute{{Prefix: "p"}}}), at)
	if !ok || snap.OriginAS != "" {
		t.Fatalf("unexpected snapshot %+v", snap)
	}
}

func TestDescribeAllOutcomes(t *testing.T) {
	q := func(cmd string) Query { return Query{Command: cmd, Target: "1.1.1.1"} }
	longPath := []uint32{1, 2, 3, 4, 5, 6, 7, 8, 9, 10}
	cases := []struct {
		lang, cmd, node string
		snap            *Snapshot
		title, prefix   string
	}{
		{"tr", "ping", "Bursa", nil, "Bursa noktasından 1.1.1.1 ping", "LG üzerinden Bursa noktasından 1.1.1.1 hedefine canlı ping atın."},
		{"tr", "traceroute", "", nil, "1.1.1.1 traceroute", "LG üzerinden 1.1.1.1 hedefine canlı traceroute çalıştırın."},
		{"tr", "bgp_route", "Bursa", nil, "Bursa üzerinde 1.1.1.1 BGP rotası", "LG üzerinden 1.1.1.1 için canlı BGP rotasını sorgulayın."},
		{"tr", "bgp_route", "", nil, "1.1.1.1 BGP rotası", "LG üzerinden"},
		{"en", "ping", "", nil, "Ping 1.1.1.1", "Run a live ping to 1.1.1.1 on LG."},
		{"en", "bgp_route", "", nil, "BGP route for 1.1.1.1", "Look up the live BGP route for 1.1.1.1 on LG."},
		{"en", "ping", "", &Snapshot{Command: "traceroute"}, "Ping 1.1.1.1", "Run a live ping"},

		{"tr", "ping", "", &Snapshot{Command: "ping", Sent: 5}, "", "1.1.1.1 yanıt vermedi (5 paket gönderildi)"},
		{"en", "ping", "", &Snapshot{Command: "ping", Sent: 5}, "", "No replies from 1.1.1.1 (5 sent)"},
		{"tr", "ping", "", &Snapshot{Command: "ping", Sent: 5, Recv: 4, LossPct: 20, AvgRTT: 1, MinRTT: 1, MaxRTT: 1.5}, "", "4/5 yanıt, %20 kayıp · ort. 1,0 ms (min 1,0, maks 1,5 ms)"},

		{"tr", "traceroute", "", &Snapshot{Command: "traceroute", Hops: 30}, "", "30 hop boyunca yanıt gelmedi"},
		{"en", "traceroute", "", &Snapshot{Command: "traceroute", Hops: 30}, "", "No hop answered within 30 hops"},
		{"tr", "traceroute", "", &Snapshot{Command: "traceroute", Hops: 12, LastRTT: 40}, "", "12 hop izlendi, hedef yanıt vermedi · son yanıt 40,0 ms"},
		{"en", "traceroute", "", &Snapshot{Command: "traceroute", Hops: 12, LastRTT: 40}, "", "12 hops traced, target did not answer · last reply 40.0 ms."},

		{"tr", "bgp_route", "Bursa", &Snapshot{Command: "bgp_route", NoRoute: true}, "", "Bursa üzerinde 1.1.1.1 için rota yok"},
		{"tr", "bgp_route", "", &Snapshot{Command: "bgp_route", NoRoute: true}, "", "1.1.1.1 için rota yok"},
		{"en", "bgp_route", "", &Snapshot{Command: "bgp_route", NoRoute: true}, "", "No route for 1.1.1.1."},
		{"en", "bgp_route", "Bursa", &Snapshot{Command: "bgp_route", NoRoute: true}, "", "No route for 1.1.1.1 on Bursa."},
		{"tr", "bgp_route", "", &Snapshot{Command: "bgp_route", Prefix: "1.1.1.0/24", ASPath: longPath, OriginAS: "AS10"}, "", "1.1.1.0/24 · AS yolu 1 2 3 4 5 6 7 8 … · köken AS10"},
		{"en", "bgp_route", "", &Snapshot{Command: "bgp_route"}, "", "Look up the live BGP route"},
	}
	for _, tc := range cases {
		title, desc := Describe(tc.lang, q(tc.cmd), tc.node, "LG", tc.snap)
		if tc.title != "" && title != tc.title {
			t.Errorf("%s %s %+v: title %q, want %q", tc.lang, tc.cmd, tc.snap, title, tc.title)
		}
		if !strings.HasPrefix(desc, tc.prefix) {
			t.Errorf("%s %s %+v: description %q, want prefix %q", tc.lang, tc.cmd, tc.snap, desc, tc.prefix)
		}
	}
}

func TestOGLocale(t *testing.T) {
	for lang, want := range map[string]string{"tr": "tr_TR", "en": "en_US", "de": "de_DE", "fr": "fr_FR", "ru": "ru_RU", "xx": ""} {
		if got := ogLocale(lang); got != want {
			t.Errorf("ogLocale(%q) = %q, want %q", lang, got, want)
		}
	}
}

func TestStoreNilAndErrors(t *testing.T) {
	ctx := context.Background()
	var nilStore *Store
	if err := nilStore.Save(ctx, 1, "x", &Snapshot{}); err != nil {
		t.Fatal(err)
	}
	if snap, err := nilStore.Load(ctx, 1, "ping", "x"); snap != nil || err != nil {
		t.Fatalf("nil store: %+v %v", snap, err)
	}

	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	db.SetMaxOpenConns(1)
	t.Cleanup(func() { db.Close() })
	s := NewStore(db)

	// No table yet: both directions report the error.
	if err := s.Save(ctx, 1, "x", &Snapshot{Command: "ping"}); err == nil {
		t.Fatal("expected a save error without the table")
	}
	if _, err := s.Load(ctx, 1, "ping", "x"); err == nil {
		t.Fatal("expected a load error without the table")
	}

	if err := store.Migrate(db); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO share_snapshots (node_id, command, target, snapshot) VALUES (1, 'ping', 'x', 'not json')`); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Load(ctx, 1, "ping", "x"); err == nil {
		t.Fatal("expected a decode error")
	}
}
