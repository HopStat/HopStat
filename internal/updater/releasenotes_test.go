package updater

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestVersionsBetweenExclusive(t *testing.T) {
	known := []string{"v2.1.64", "v2.1.65", "v2.1.66", "v2.2.0"}
	got := versionsBetweenExclusive("v2.1.63", "v2.1.66", known)
	want := []string{"v2.1.64", "v2.1.65", "v2.1.66"}
	if len(got) != len(want) {
		t.Fatalf("got %v want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("got %v want %v", got, want)
		}
	}
}

func TestAggregateReleaseNotesEmbedded(t *testing.T) {
	u := New("HopStat/HopStat", "v2.1.63", true)
	agg, versions, err := u.AggregateReleaseNotes(context.Background(), "v2.1.63", "v2.1.66")
	if err != nil {
		t.Fatalf("AggregateReleaseNotes: %v", err)
	}
	if len(versions) != 3 {
		t.Fatalf("versions = %v", versions)
	}
	if !strings.Contains(agg, "v2.1.64") || !strings.Contains(agg, "v2.1.66") {
		t.Fatalf("aggregated notes missing versions: %q", agg)
	}
	if !strings.Contains(agg, "---") {
		t.Fatalf("expected section separators in aggregated notes")
	}
}

func httptestNewReleaseServer(t *testing.T, tag, body string) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, `{"tag_name":%q,"name":"HopStat %s","body":%q,"html_url":"https://example.com/%s","assets":[]}`,
			tag, tag, body, tag)
	}))
}

func TestStatusAggregatesSinceQuery(t *testing.T) {
	srv := httptestNewReleaseServer(t, "v2.1.66", "latest-only body")
	defer srv.Close()

	u := New("HopStat/HopStat", "v2.1.63", true)
	u.SetReleaseAPIURL(srv.URL)
	st, err := u.Status(context.Background(), "v2.1.63")
	if err != nil {
		t.Fatalf("Status: %v", err)
	}
	if len(st.ReleaseVersions) < 3 {
		t.Fatalf("release_versions = %v", st.ReleaseVersions)
	}
	if strings.TrimSpace(st.ReleaseNotes) == "latest-only body" {
		t.Fatal("expected aggregated embedded notes, got latest release body only")
	}
	if st.ReleaseName != "HopStat v2.1.64 – v2.1.66" {
		t.Fatalf("release_name = %q", st.ReleaseName)
	}
}

func TestAggregatedReleaseName(t *testing.T) {
	if got := aggregatedReleaseName([]string{"v2.1.66"}); got != "HopStat v2.1.66" {
		t.Fatalf("single = %q", got)
	}
	if got := aggregatedReleaseName([]string{"v2.1.64", "v2.1.66"}); got != "HopStat v2.1.64 – v2.1.66" {
		t.Fatalf("range = %q", got)
	}
}
