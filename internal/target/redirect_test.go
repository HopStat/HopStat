package target

import (
	"fmt"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The threat these clients face is a response that names a different host: a redirect to
// a loopback admin port, a link-local metadata service, or an RFC1918 address. The vendor
// host is fixed in source, so any hop that changes it is one the code never intended.
func TestCheckSameHostRedirectRefusesInternalDestination(t *testing.T) {
	var internalHits int
	internal := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		internalHits++
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"success":true}`))
	}))
	defer internal.Close()

	vendor := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, internal.URL+"/latest", http.StatusFound)
	}))
	defer vendor.Close()

	client := &http.Client{CheckRedirect: CheckSameHostRedirect}
	resp, err := client.Get(vendor.URL + "/download")
	if err == nil {
		resp.Body.Close()
		t.Fatal("the redirect off the vendor host was followed")
	}
	if !strings.Contains(err.Error(), "refusing cross-host redirect") {
		t.Fatalf("err = %v, want a cross-host refusal", err)
	}
	if internalHits != 0 {
		t.Fatalf("the internal server was reached %d time(s); the redirect must not be followed", internalHits)
	}
}

// A redirect that stays on the vendor's own host is legitimate — an asset served from a
// sibling path — and must keep working.
func TestCheckSameHostRedirectAllowsSameHost(t *testing.T) {
	var final int
	mux := http.NewServeMux()
	mux.HandleFunc("/start", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/asset", http.StatusFound)
	})
	mux.HandleFunc("/asset", func(w http.ResponseWriter, r *http.Request) {
		final++
		w.WriteHeader(http.StatusOK)
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	client := &http.Client{CheckRedirect: CheckSameHostRedirect}
	resp, err := client.Get(srv.URL + "/start")
	if err != nil {
		t.Fatalf("same-host redirect was refused: %v", err)
	}
	defer resp.Body.Close()
	if final != 1 {
		t.Fatalf("same-host redirect did not reach the final handler (hits = %d)", final)
	}
}

// Every http.Client built in production code must carry the policy. The four vendor
// clients are easy to harden once and easy to regress by adding a fifth later, so the
// invariant is checked across the tree rather than per file.
func TestEveryProductionHTTPClientRefusesCrossHostRedirects(t *testing.T) {
	root, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatalf("resolve repo root: %v", err)
	}

	var offenders []string
	walkErr := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			switch d.Name() {
			case "vendor", "node_modules", "dist", ".git", ".temp_files", "coverage":
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(d.Name(), ".go") || strings.HasSuffix(d.Name(), "_test.go") {
			return nil
		}
		body, readErr := os.ReadFile(path)
		if readErr != nil {
			return readErr
		}
		src := string(body)
		for i, line := range strings.Split(src, "\n") {
			if !strings.Contains(line, "http.Client{") && !strings.Contains(line, "http.DefaultClient") {
				continue
			}
			// The literal may be spread over several lines; check the remainder of the
			// composite literal rather than only the opening line.
			tail := strings.Join(strings.Split(src, "\n")[i:min(i+8, len(strings.Split(src, "\n")))], "\n")
			// Two acceptable protections. CheckRedirect refuses the hop before it is
			// made. agentTransport() validates at dial time instead, which is strictly
			// stronger for that client: every connection it opens, including one made
			// for a redirect, re-checks the resolved address. So the requirement is
			// "protected", not "literally has CheckRedirect".
			if strings.Contains(tail, "CheckRedirect") || strings.Contains(tail, "agentTransport()") {
				continue
			}
			rel, _ := filepath.Rel(root, path)
			offenders = append(offenders, rel+":"+itoa(i+1))
		}
		return nil
	})
	if walkErr != nil {
		t.Fatalf("walk: %v", walkErr)
	}
	if len(offenders) > 0 {
		t.Fatalf("production http.Client without a CheckRedirect policy: %v\n"+
			"A vendor response could redirect the request to an internal address.", offenders)
	}
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b []byte
	for n > 0 {
		b = append([]byte{byte('0' + n%10)}, b...)
		n /= 10
	}
	return string(b)
}

// The first request has no redirect history at all. CheckRedirect is not consulted for
// it in practice, but the policy must not reject a request it is handed directly.
func TestCheckSameHostRedirectAllowsRequestWithNoHistory(t *testing.T) {
	req, err := http.NewRequest(http.MethodGet, "https://api.github.com/repos/x/releases/latest", nil)
	if err != nil {
		t.Fatalf("build request: %v", err)
	}
	if err := CheckSameHostRedirect(req, nil); err != nil {
		t.Fatalf("a request with no redirect history was rejected: %v", err)
	}
}

// A vendor that bounces the request around its own host is bounded, so a long same-host
// chain cannot be used to extend the client's reach or its runtime.
func TestCheckSameHostRedirectStopsLongChains(t *testing.T) {
	mux := http.NewServeMux()
	for i := 0; i < 12; i++ {
		from, to := fmt.Sprintf("/%d", i), fmt.Sprintf("/%d", i+1)
		mux.HandleFunc(from, func(w http.ResponseWriter, r *http.Request) {
			http.Redirect(w, r, to, http.StatusFound)
		})
	}
	srv := httptest.NewServer(mux)
	defer srv.Close()

	client := &http.Client{CheckRedirect: CheckSameHostRedirect}
	resp, err := client.Get(srv.URL + "/0")
	if err == nil {
		resp.Body.Close()
		t.Fatal("an unbounded same-host redirect chain was followed to the end")
	}
	if !strings.Contains(err.Error(), "stopped after") {
		t.Fatalf("err = %v, want the redirect chain to be cut short", err)
	}
}
