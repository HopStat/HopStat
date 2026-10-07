package updater

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"net/url"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"testing"
)

// repoRoot locates the repository from this file's own path rather than the process
// working directory. Deriving it from the cwd meant the guard resolved to whatever
// directory the binary happened to start in, walked an empty tree, found no callers,
// and passed vacuously — a guard that silently stops guarding. runtime.Caller is
// unaffected by where the test is invoked from.
func repoRoot(t *testing.T) string {
	t.Helper()
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("cannot locate this test file, so the repository root is unknown")
	}
	// thisFile is <root>/internal/updater/releaseurl_guard_test.go
	return filepath.Clean(filepath.Join(filepath.Dir(thisFile), "..", ".."))
}

// SetReleaseAPIURL is a test-only override of the GitHub releases endpoint. It exists so
// tests can aim at a local httptest server. If production code ever calls it with a value
// read from settings, the release host becomes admin-controlled: the updater would then
// fetch from wherever an admin points it and, on Apply, exec whatever binary it downloads.
// That is the same SSRF-to-remote-code shape the agent driver was hardened against, so the
// seam is guarded rather than merely documented.
//
// The setter cannot be unexported — other packages' tests legitimately use it — so this
// test is the guard: any production reference fails the build.
//
// The scan is over the AST, not the raw text. A line-based scan reported a /* */ comment
// body and a raw-string body as production callers, which is the failure mode that gets a
// guard disabled: people delete the useful comment rather than the real caller.
func TestSetReleaseAPIURLHasNoProductionCallers(t *testing.T) {
	root := repoRoot(t)
	var offenders []string
	scanned := 0

	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
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
		// Any selector on the method is a production reference. That covers a call
		// (whose Fun is the selector) and a method value assigned for later use.
		// The declaration itself is an ast.FuncDecl, not a selector, so it is never
		// matched, and comments and string literals are not nodes at all.
		ast.Inspect(file, func(n ast.Node) bool {
			sel, ok := n.(*ast.SelectorExpr)
			if ok && sel.Sel.Name == "SetReleaseAPIURL" {
				offenders = append(offenders, fmt.Sprintf("%s:%d", rel, fset.Position(sel.Pos()).Line))
			}
			return true
		})
		return nil
	})
	if err != nil {
		t.Fatalf("walk: %v", err)
	}

	if scanned == 0 {
		t.Fatalf("no Go files found under %s; the walk root is wrong, so this guard would pass vacuously", root)
	}
	if len(offenders) > 0 {
		sort.Strings(offenders)
		t.Fatalf("SetReleaseAPIURL is a test-only seam but is referenced from production code: %v\n"+
			"Wiring it to a settings value would make the release host operator-controlled.", offenders)
	}
}

// The repo is the only configurable part of the releases URL, and it lands in the path.
// No spelling of it may move the request to a different host.
func TestReleaseURLHostIsNeverOperatorControlled(t *testing.T) {
	hostile := []string{
		"evil.example/x",
		"../../../etc",
		"@evil.example/x",
		"owner/name?next=https://evil.example",
		"owner/name#@evil.example",
		"https://evil.example",
		"owner\\name",
		"",
	}
	for _, repo := range hostile {
		t.Run(repo, func(t *testing.T) {
			u := New(repo, "v0.0.0", false)
			u.SetReleaseAPIURL("") // production default: no test override in play

			raw := u.releaseURL()
			parsed, err := url.Parse(raw)
			if err != nil {
				t.Fatalf("releaseURL() = %q: %v", raw, err)
			}
			if parsed.Host != "api.github.com" {
				t.Fatalf("repo %q moved the release host to %q: %s", repo, parsed.Host, raw)
			}
			if parsed.Scheme != "https" {
				t.Fatalf("repo %q downgraded the scheme to %q: %s", repo, parsed.Scheme, raw)
			}
		})
	}
}
