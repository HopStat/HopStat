package target

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"runtime"
	"sort"
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
// repoRoot locates the repository from this file's own path rather than the process
// working directory. Deriving it from the cwd meant the guard resolved to whatever
// directory the binary happened to start in, walked an empty tree, found no clients,
// and passed vacuously — a guard that silently stops guarding.
func repoRoot(t *testing.T) string {
	t.Helper()
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("cannot locate this test file, so the repository root is unknown")
	}
	// thisFile is <root>/internal/target/redirect_test.go
	return filepath.Clean(filepath.Join(filepath.Dir(thisFile), "..", ".."))
}

func TestEveryProductionHTTPClientRefusesCrossHostRedirects(t *testing.T) {
	root := repoRoot(t)

	var offenders []string
	scanned := 0
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
		// Parse rather than scan text. Brace matching over raw source is fooled by a
		// "}" inside a string or comment, and a fixed line window lets one literal's
		// CheckRedirect vouch for its neighbour. The AST gives each client its exact
		// construct and ignores comments and string contents.
		scanned++
		fset := token.NewFileSet()
		file, parseErr := parser.ParseFile(fset, path, nil, 0)
		if parseErr != nil {
			return nil
		}
		rel, relErr := filepath.Rel(root, path)
		if relErr != nil {
			rel = path
		}
		flag := func(pos token.Pos) {
			offenders = append(offenders, fmt.Sprintf("%s:%d", rel, fset.Position(pos).Line))
		}
		ast.Inspect(file, func(n ast.Node) bool {
			switch node := n.(type) {
			case *ast.CompositeLit:
				// &http.Client{...} and http.Client{...}
				if !isHTTPClientType(node.Type) {
					return true
				}
				if clientLitIsProtected(node) {
					return true
				}
				flag(node.Type.Pos())
			case *ast.ValueSpec:
				// var c http.Client — a zero-value client with no policy at all.
				// Skipped when initialised, since that initialiser is itself a
				// composite literal and is checked above.
				if len(node.Values) > 0 {
					return true
				}
				if isHTTPClientType(node.Type) {
					flag(node.Type.Pos())
				}
			case *ast.CallExpr:
				// new(http.Client) and the package-level one-shots, none of which
				// can carry a CheckRedirect at all.
				if isNewHTTPClient(node) || isHTTPOneShot(node) {
					flag(node.Pos())
				}
			case *ast.SelectorExpr:
				// http.DefaultClient is the shared global and cannot be protected.
				if isHTTPDefaultClient(node) {
					flag(node.Pos())
				}
			}
			return true
		})
		return nil
	})
	if walkErr != nil {
		t.Fatalf("walk: %v", walkErr)
	}
	if scanned == 0 {
		t.Fatalf("no Go files found under %s; the walk root is wrong, so this guard would pass vacuously", root)
	}
	if len(offenders) > 0 {
		sort.Strings(offenders)
		t.Fatalf("production http.Client without a redirect policy: %v\n"+
			"A vendor response could redirect the request to an internal address.", offenders)
	}
}

func isHTTPClientType(e ast.Expr) bool {
	sel, ok := e.(*ast.SelectorExpr)
	if !ok {
		return false
	}
	x, ok := sel.X.(*ast.Ident)
	return ok && x.Name == "http" && sel.Sel.Name == "Client"
}

func isHTTPDefaultClient(sel *ast.SelectorExpr) bool {
	if sel.Sel.Name != "DefaultClient" {
		return false
	}
	x, ok := sel.X.(*ast.Ident)
	return ok && x.Name == "http"
}

// clientLitIsProtected reports whether the literal carries one of the two accepted
// protections. CheckRedirect refuses the hop before it is made. agentTransport()
// validates at dial time instead, which is strictly stronger for that client: every
// connection it opens, including one made for a redirect, re-checks the resolved
// address. The requirement is "protected", not "literally has CheckRedirect".
func clientLitIsProtected(lit *ast.CompositeLit) bool {
	for _, elt := range lit.Elts {
		kv, ok := elt.(*ast.KeyValueExpr)
		if !ok {
			continue
		}
		key, ok := kv.Key.(*ast.Ident)
		if !ok {
			continue
		}
		switch key.Name {
		case "CheckRedirect":
			return true
		case "Transport":
			if call, ok := kv.Value.(*ast.CallExpr); ok {
				if fn, ok := call.Fun.(*ast.Ident); ok && fn.Name == "agentTransport" {
					return true
				}
			}
		}
	}
	return false
}

func isNewHTTPClient(call *ast.CallExpr) bool {
	fn, ok := call.Fun.(*ast.Ident)
	if !ok || fn.Name != "new" || len(call.Args) != 1 {
		return false
	}
	return isHTTPClientType(call.Args[0])
}

func isHTTPOneShot(call *ast.CallExpr) bool {
	sel, ok := call.Fun.(*ast.SelectorExpr)
	if !ok {
		return false
	}
	x, ok := sel.X.(*ast.Ident)
	if !ok || x.Name != "http" {
		return false
	}
	switch sel.Sel.Name {
	case "Get", "Head", "Post", "PostForm":
		return true
	}
	return false
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

func redirectReq(t *testing.T, from, to string) (*http.Request, []*http.Request) {
	t.Helper()
	prev, err := http.NewRequest(http.MethodGet, from, nil)
	if err != nil {
		t.Fatal(err)
	}
	next, err := http.NewRequest(http.MethodGet, to, nil)
	if err != nil {
		t.Fatal(err)
	}
	return next, []*http.Request{prev}
}

// GitHub and MaxMind hand their downloads to a CDN host. Refusing that hop is what broke
// self-update and GeoIP downloads in v2.2.15.
func TestCheckRedirectToHostsAllowsVendorCDN(t *testing.T) {
	check := CheckRedirectToHosts(GitHubReleaseAssetHosts...)
	req, via := redirectReq(t, "https://github.com/HopStat/HopStat/releases/download/v1/x", "https://release-assets.githubusercontent.com/a?sig=1")
	if err := check(req, via); err != nil {
		t.Fatalf("GitHub asset redirect refused: %v", err)
	}

	geo := CheckRedirectToHosts(MaxMindDownloadHosts...)
	req, via = redirectReq(t, "https://download.maxmind.com/app/geoip_download", "https://"+MaxMindDownloadHosts[0]+"/x")
	if err := geo(req, via); err != nil {
		t.Fatalf("MaxMind R2 redirect refused: %v", err)
	}

	req, via = redirectReq(t, "https://github.com/a", "https://github.com/b")
	if err := check(req, via); err != nil {
		t.Fatalf("same-host redirect refused: %v", err)
	}
	if err := check(req, nil); err != nil {
		t.Fatalf("first request refused: %v", err)
	}
}

func TestCheckRedirectToHostsRefusesEverythingElse(t *testing.T) {
	check := CheckRedirectToHosts(GitHubReleaseAssetHosts...)
	for _, to := range []string{
		"https://169.254.169.254/latest/meta-data",
		"http://release-assets.githubusercontent.com/a",
		"https://release-assets.githubusercontent.com:8443/a",
		"https://evil.release-assets.githubusercontent.com.example/a",
		"https://" + MaxMindDownloadHosts[0] + "/x",
	} {
		req, via := redirectReq(t, "https://github.com/a", to)
		if err := check(req, via); err == nil || !strings.Contains(err.Error(), "refusing cross-host redirect") {
			t.Fatalf("redirect to %s: err = %v, want refusal", to, err)
		}
	}

	req, _ := redirectReq(t, "https://github.com/a", "https://release-assets.githubusercontent.com/a")
	via := make([]*http.Request, maxRedirects+1)
	for i := range via {
		via[i], _ = http.NewRequest(http.MethodGet, "https://github.com/a", nil)
	}
	if err := check(req, via); err == nil || !strings.Contains(err.Error(), "stopped after") {
		t.Fatalf("err = %v, want redirect cap", err)
	}
}
