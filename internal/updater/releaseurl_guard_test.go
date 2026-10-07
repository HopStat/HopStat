package updater

import (
	"fmt"
	"io/fs"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// repoRoot is two levels up from internal/updater.
func repoRoot(t *testing.T) string {
	t.Helper()
	root, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatalf("resolve repo root: %v", err)
	}
	return root
}

// SetReleaseAPIURL is a test-only override of the GitHub releases endpoint. It exists so
// tests can aim at a local httptest server. If production code ever calls it with a value
// read from settings, the release host becomes admin-controlled: the updater would then
// fetch from wherever an admin points it and, on Apply, exec whatever binary it downloads.
// That is the same SSRF-to-remote-code shape the agent driver was hardened against, so the
// seam is guarded rather than merely documented.
//
// The setter cannot be unexported — other packages' tests legitimately use it — so this
// test is the guard: any call from a non-test file fails the build.
func TestSetReleaseAPIURLHasNoProductionCallers(t *testing.T) {
	root := repoRoot(t)
	var offenders []string

	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		name := d.Name()
		if d.IsDir() {
			switch name {
			case "vendor", "node_modules", "dist", ".git", ".temp_files", "coverage":
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			return nil
		}
		rel, relErr := filepath.Rel(root, path)
		if relErr != nil {
			rel = path
		}
		body, readErr := os.ReadFile(path)
		if readErr != nil {
			return readErr
		}
		// Match call sites only: a mention in a comment or in the setter's own
		// signature is not a call. Skipping by line rather than by file means a
		// production call placed in updater.go itself is still caught.
		for i, line := range strings.Split(string(body), "\n") {
			trimmed := strings.TrimSpace(line)
			if strings.HasPrefix(trimmed, "//") {
				continue
			}
			if strings.Contains(line, "SetReleaseAPIURL") &&
				!strings.HasPrefix(trimmed, "func (u *Updater) SetReleaseAPIURL(") {
				offenders = append(offenders, fmt.Sprintf("%s:%d: %s", rel, i+1, trimmed))
			}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walk: %v", err)
	}
	if len(offenders) > 0 {
		t.Fatalf("SetReleaseAPIURL is a test-only seam but is called from production code: %v\n"+
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
