package updater

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// ── releasenotes.go gaps ──────────────────────────────────────────────────────

func TestReadEmbeddedNoteMissing(t *testing.T) {
	note, ok := readEmbeddedNote("v999.999.999")
	if ok || note != "" {
		t.Fatalf("expected false for non-existent version, got ok=%v note=%q", ok, note)
	}
}

func TestRawNotesURL(t *testing.T) {
	url := rawNotesURL("HopStat/HopStat", "v2.1.64")
	if !strings.Contains(url, "v2.1.64") || !strings.Contains(url, "HopStat/HopStat") {
		t.Fatalf("rawNotesURL = %q", url)
	}
}

func TestFetchRawNoteHookUsed(t *testing.T) {
	orig := fetchRawNoteHook
	t.Cleanup(func() { fetchRawNoteHook = orig })
	fetchRawNoteHook = func(_ context.Context, repo, version string) (string, error) {
		return "hook note", nil
	}
	u := New("HopStat/HopStat", "v1.0.0", true)
	note, err := u.fetchRawNote(context.Background(), "v2.0.0")
	if err != nil || note != "hook note" {
		t.Fatalf("fetchRawNote hook: err=%v note=%q", err, note)
	}
}

func TestFetchRawNoteNonOKStatus(t *testing.T) {
	orig := fetchRawNoteHook
	t.Cleanup(func() { fetchRawNoteHook = orig })
	fetchRawNoteHook = nil // exercise real HTTP path

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	u := New("HopStat/HopStat", "v1.0.0", true)
	u.apiClient = &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		req, _ := http.NewRequestWithContext(r.Context(), http.MethodGet, srv.URL, nil)
		return http.DefaultTransport.RoundTrip(req)
	})}
	_, err := u.fetchRawNote(context.Background(), "v999.0.0")
	if err == nil {
		t.Fatal("expected error for non-OK status")
	}
}

func TestFetchRawNoteNetworkError(t *testing.T) {
	orig := fetchRawNoteHook
	t.Cleanup(func() { fetchRawNoteHook = orig })
	fetchRawNoteHook = nil

	u := New("HopStat/HopStat", "v1.0.0", true)
	u.apiClient = &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		return nil, errors.New("network down")
	})}
	_, err := u.fetchRawNote(context.Background(), "v2.0.0")
	if err == nil {
		t.Fatal("expected network error")
	}
}

func TestFetchRawNoteHTTPSuccess(t *testing.T) {
	orig := fetchRawNoteHook
	t.Cleanup(func() { fetchRawNoteHook = orig })
	fetchRawNoteHook = nil

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, "# Release notes\nSome content")
	}))
	defer srv.Close()

	u := New("HopStat/HopStat", "v1.0.0", true)
	u.apiClient = &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		req, _ := http.NewRequest(http.MethodGet, srv.URL, nil)
		return http.DefaultTransport.RoundTrip(req)
	})}
	note, err := u.fetchRawNote(context.Background(), "v999.0.0")
	if err != nil || !strings.Contains(note, "Release notes") {
		t.Fatalf("fetchRawNote HTTP: err=%v note=%q", err, note)
	}
}

func TestFetchGitTagsHookNonOK(t *testing.T) {
	orig := fetchGitTagsHook
	t.Cleanup(func() { fetchGitTagsHook = orig })
	fetchGitTagsHook = func(_ context.Context, repo string) ([]string, error) {
		return nil, errors.New("tags error")
	}
	u := New("HopStat/HopStat", "v1.0.0", true)
	_, err := u.fetchGitTags(context.Background())
	if err == nil {
		t.Fatal("expected hook error")
	}
}

func TestFetchGitTagsNetworkError(t *testing.T) {
	orig := fetchGitTagsHook
	t.Cleanup(func() { fetchGitTagsHook = orig })
	fetchGitTagsHook = nil

	u := New("HopStat/HopStat", "v1.0.0", true)
	u.apiClient = &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		return nil, errors.New("network down")
	})}
	_, err := u.fetchGitTags(context.Background())
	if err == nil {
		t.Fatal("expected network error")
	}
}

func TestFetchGitTagsNonOKStatus(t *testing.T) {
	orig := fetchGitTagsHook
	t.Cleanup(func() { fetchGitTagsHook = orig })
	fetchGitTagsHook = nil

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	}))
	defer srv.Close()

	u := New("HopStat/HopStat", "v1.0.0", true)
	u.apiClient = &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		req, _ := http.NewRequestWithContext(r.Context(), http.MethodGet, srv.URL, nil)
		return http.DefaultTransport.RoundTrip(req)
	})}
	_, err := u.fetchGitTags(context.Background())
	if err == nil {
		t.Fatal("expected non-OK status error")
	}
}

func TestFetchGitTagsDecodeError(t *testing.T) {
	orig := fetchGitTagsHook
	t.Cleanup(func() { fetchGitTagsHook = orig })
	fetchGitTagsHook = nil

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, "not-json")
	}))
	defer srv.Close()

	u := New("HopStat/HopStat", "v1.0.0", true)
	u.apiClient = &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		req, _ := http.NewRequestWithContext(r.Context(), http.MethodGet, srv.URL, nil)
		return http.DefaultTransport.RoundTrip(req)
	})}
	_, err := u.fetchGitTags(context.Background())
	if err == nil {
		t.Fatal("expected decode error")
	}
}

func TestFetchGitTagsFiltersNonVersionTags(t *testing.T) {
	orig := fetchGitTagsHook
	t.Cleanup(func() { fetchGitTagsHook = orig })
	fetchGitTagsHook = nil

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `[{"name":"v2.0.0"},{"name":"not-a-version"},{"name":"v2.1.0"}]`)
	}))
	defer srv.Close()

	u := New("HopStat/HopStat", "v1.0.0", true)
	u.apiClient = &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		req, _ := http.NewRequestWithContext(r.Context(), http.MethodGet, srv.URL, nil)
		return http.DefaultTransport.RoundTrip(req)
	})}
	tags, err := u.fetchGitTags(context.Background())
	if err != nil {
		t.Fatalf("fetchGitTags: %v", err)
	}
	// "not-a-version" is filtered; v2.0.0 and v2.1.0 are kept
	if len(tags) != 2 {
		t.Fatalf("expected 2 tags, got %v", tags)
	}
}

func TestAllKnownVersionsMergesGitHub(t *testing.T) {
	orig := fetchGitTagsHook
	t.Cleanup(func() { fetchGitTagsHook = orig })
	fetchGitTagsHook = func(_ context.Context, repo string) ([]string, error) {
		return []string{"v2.1.64", "v2.1.65", "v99.0.0"}, nil
	}
	u := New("HopStat/HopStat", "v1.0.0", true)
	all := u.allKnownVersions(context.Background())
	found99 := false
	for _, v := range all {
		if v == "v99.0.0" {
			found99 = true
		}
	}
	if !found99 {
		t.Fatal("expected v99.0.0 from GitHub tags")
	}
}

func TestAllKnownVersionsFetchTagsError(t *testing.T) {
	orig := fetchGitTagsHook
	t.Cleanup(func() { fetchGitTagsHook = orig })
	fetchGitTagsHook = func(_ context.Context, repo string) ([]string, error) {
		return nil, errors.New("github down")
	}
	u := New("HopStat/HopStat", "v1.0.0", true)
	all := u.allKnownVersions(context.Background())
	if len(all) == 0 {
		t.Fatal("expected embedded versions as fallback")
	}
}

func TestVersionsBetweenExclusiveEdgeCases(t *testing.T) {
	known := []string{"v2.0.0", "v2.1.0", "v2.2.0"}

	if got := versionsBetweenExclusive("", "v2.2.0", known); got != nil {
		t.Fatalf("empty since should return nil, got %v", got)
	}
	if got := versionsBetweenExclusive("v2.0.0", "", known); got != nil {
		t.Fatalf("empty latest should return nil, got %v", got)
	}
	if got := versionsBetweenExclusive("v2.2.0", "v2.0.0", known); got != nil {
		t.Fatalf("latest <= since should return nil, got %v", got)
	}

	// latest not in known → appended to output
	got := versionsBetweenExclusive("v2.0.0", "v3.0.0", known)
	last := got[len(got)-1]
	if last != "v3.0.0" {
		t.Fatalf("expected v3.0.0 appended, got %v", got)
	}

	// duplicate entries in known → dedup continue branch
	gotDedup := versionsBetweenExclusive("v1.0.0", "v2.0.0", []string{"v1.5.0", "v1.5.0"})
	if len(gotDedup) != 2 {
		t.Fatalf("expected 2 entries after dedup, got %v", gotDedup)
	}
}

func TestLoadReleaseNoteFetchesWhenEmbeddedMissing(t *testing.T) {
	orig := fetchRawNoteHook
	t.Cleanup(func() { fetchRawNoteHook = orig })
	fetchRawNoteHook = func(_ context.Context, repo, version string) (string, error) {
		return "fetched note", nil
	}
	u := New("HopStat/HopStat", "v1.0.0", true)
	note, err := u.loadReleaseNote(context.Background(), "v999.999.999")
	if err != nil || note != "fetched note" {
		t.Fatalf("loadReleaseNote fallback: err=%v note=%q", err, note)
	}
}

func TestAggregateReleaseNotesEmptyVersions(t *testing.T) {
	u := New("HopStat/HopStat", "v2.2.0", true)
	agg, versions, err := u.AggregateReleaseNotes(context.Background(), "v2.2.0", "v2.1.0")
	if err != nil || agg != "" || versions != nil {
		t.Fatalf("expected empty result when since >= latest: agg=%q versions=%v err=%v", agg, versions, err)
	}
}

func TestAggregateReleaseNotesAllLoadErrors(t *testing.T) {
	orig := fetchRawNoteHook
	t.Cleanup(func() { fetchRawNoteHook = orig })
	fetchRawNoteHook = func(_ context.Context, repo, version string) (string, error) {
		return "", errors.New("not found")
	}
	u := New("HopStat/HopStat", "v1.0.0", true)
	agg, versions, err := u.AggregateReleaseNotes(context.Background(), "v99.0.0", "v99.1.0")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if agg != "" || len(versions) == 0 {
		t.Fatalf("expected empty agg with versions list: agg=%q versions=%v", agg, versions)
	}
}

func TestAggregatedReleaseNameEmpty(t *testing.T) {
	if got := aggregatedReleaseName(nil); got != "" {
		t.Fatalf("expected empty string for nil versions, got %q", got)
	}
	if got := aggregatedReleaseName([]string{}); got != "" {
		t.Fatalf("expected empty string for empty versions, got %q", got)
	}
}

// ── updater.go gaps ───────────────────────────────────────────────────────────

func TestStatusReturnsCachedError(t *testing.T) {
	u := New("HopStat/HopStat", "v1.0.0", true)
	injected := errors.New("injected cache error")
	u.storeStatusCache("", nil, injected)
	_, err := u.Status(context.Background(), "")
	if !errors.Is(err, injected) {
		t.Fatalf("expected cached error, got %v", err)
	}
}

func TestStoreStatusCacheErrorTTL(t *testing.T) {
	u := New("HopStat/HopStat", "v1.0.0", true)
	u.storeStatusCache("", nil, errors.New("boom"))
	entry := u.statusCache[""]
	if entry.until.IsZero() {
		t.Fatal("expected non-zero TTL for error cache")
	}
	if entry.until.After(time.Now().Add(statusCacheTTL)) {
		t.Fatal("error TTL should be shorter than success TTL")
	}
}

func TestBuildStatusFromEmbeddedNoEmbeddedVersions(t *testing.T) {
	orig := listEmbeddedVersionsHook
	t.Cleanup(func() { listEmbeddedVersionsHook = orig })
	listEmbeddedVersionsHook = func() []string { return nil }

	u := New("HopStat/HopStat", "v1.5.0", false)
	st := u.buildStatusFromEmbedded(context.Background(), "")
	if st.Latest != "v1.5.0" {
		t.Fatalf("expected current version as fallback, got %q", st.Latest)
	}
	if st.SelfUpdateReason != "disabled in config (update.enabled: false)" {
		t.Fatalf("reason = %q", st.SelfUpdateReason)
	}
}

func TestBuildStatusFromEmbeddedWithSince(t *testing.T) {
	u := New("HopStat/HopStat", "v2.1.63", true)
	st := u.buildStatusFromEmbedded(context.Background(), "v2.1.63")
	if st == nil {
		t.Fatal("expected status")
	}
	if st.ReleaseNotes == "" {
		t.Fatal("expected aggregated release notes for since < latest")
	}
}

func TestBuildStatusFromEmbeddedSupportedEnabled(t *testing.T) {
	origGOOS := runtimeGOOS
	origExecutable := osExecutable
	origEval := evalSymlinks
	t.Cleanup(func() {
		runtimeGOOS = origGOOS
		osExecutable = origExecutable
		evalSymlinks = origEval
	})
	runtimeGOOS = "linux"
	dir := t.TempDir()
	osExecutable = func() (string, error) { return dir + "/hopstat", nil }
	evalSymlinks = func(path string) (string, error) { return path, nil }
	t.Setenv("LG_UPDATE_ENABLED", "")

	u := New("HopStat/HopStat", "v2.1.63", true)
	st := u.buildStatusFromEmbedded(context.Background(), "")
	if !st.SelfUpdateEnabled {
		t.Fatalf("expected self-update enabled on linux with writable dir, reason=%q", st.SelfUpdateReason)
	}
	if st.SelfUpdateReason != "" {
		t.Fatalf("expected empty reason, got %q", st.SelfUpdateReason)
	}
}

func TestStatusFetchErrorWithSinceFallsBackToEmbedded(t *testing.T) {
	u := New("HopStat/HopStat", "v2.1.63", true)
	u.SetReleaseAPIURL("http://127.0.0.1:1")
	st, err := u.Status(context.Background(), "v2.1.63")
	if err != nil {
		t.Fatalf("expected embedded fallback with since, got error: %v", err)
	}
	if st == nil {
		t.Fatal("expected status")
	}
}

func TestSetGitHubRequestHeadersWithToken(t *testing.T) {
	t.Setenv("GITHUB_TOKEN", "test-token-abc")
	req, _ := http.NewRequest(http.MethodGet, "https://example.com", nil)
	setGitHubRequestHeaders(req)
	if req.Header.Get("Authorization") != "Bearer test-token-abc" {
		t.Fatalf("expected bearer token, got %q", req.Header.Get("Authorization"))
	}
}

func TestSetGitHubRequestHeadersWithLGToken(t *testing.T) {
	t.Setenv("GITHUB_TOKEN", "")
	t.Setenv("LG_GITHUB_TOKEN", "lg-token-xyz")
	req, _ := http.NewRequest(http.MethodGet, "https://example.com", nil)
	setGitHubRequestHeaders(req)
	if req.Header.Get("Authorization") != "Bearer lg-token-xyz" {
		t.Fatalf("expected LG bearer token, got %q", req.Header.Get("Authorization"))
	}
}
