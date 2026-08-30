package updater

import (
	"bufio"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) {
	return f(r)
}

func TestSetReleaseAPIURL(t *testing.T) {
	u := New("HopStat/HopStat", "v1.0.0", true)
	u.SetReleaseAPIURL("  http://example.com/releases  ")
	if u.releaseAPIURL != "http://example.com/releases" {
		t.Fatalf("releaseAPIURL = %q", u.releaseAPIURL)
	}
}

func mockReleaseServer(t *testing.T, tag string, assetBody []byte) *httptest.Server {
	t.Helper()
	assetName := fmt.Sprintf("hopstat-%s-%s", runtime.GOOS, runtime.GOARCH)
	sum := sha256.Sum256(assetBody)
	checksums := fmt.Sprintf("%s  %s\n", hex.EncodeToString(sum[:]), assetName)

	var srv *httptest.Server
	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/" || strings.HasSuffix(r.URL.Path, "/releases/latest"):
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprintf(w, `{"tag_name":%q,"html_url":"https://example.com/%s","assets":[`, tag, tag)
			fmt.Fprintf(w, `{"name":%q,"browser_download_url":"%s/binary"},`, assetName, srv.URL)
			fmt.Fprintf(w, `{"name":"checksums.txt","browser_download_url":"%s/checksums"}`, srv.URL)
			fmt.Fprint(w, `]}`)
		case strings.HasSuffix(r.URL.Path, "/binary"):
			w.Write(assetBody)
		case strings.HasSuffix(r.URL.Path, "/checksums"):
			w.Write([]byte(checksums))
		default:
			http.NotFound(w, r)
		}
	}))
	return srv
}

func TestFetchLatestUsesDefaultRepoURL(t *testing.T) {
	u := New("HopStat/HopStat", "v1.0.0", true)
	u.apiClient = &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		if !strings.Contains(r.URL.String(), "api.github.com/repos/HopStat/HopStat/releases/latest") {
			t.Fatalf("unexpected url: %s", r.URL)
		}
		return &http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(strings.NewReader(`{"tag_name":"v2.0.0","html_url":"https://example.com/v2.0.0","assets":[]}`)),
			Header:     make(http.Header),
		}, nil
	})}
	rel, err := u.fetchLatest(context.Background())
	if err != nil {
		t.Fatalf("fetchLatest error: %v", err)
	}
	if rel.TagName != "v2.0.0" {
		t.Fatalf("tag = %q", rel.TagName)
	}
}

func TestFetchLatestSuccess(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Accept") != "application/vnd.github.v3+json" {
			t.Errorf("unexpected Accept header: %q", r.Header.Get("Accept"))
		}
		fmt.Fprint(w, `{"tag_name":"v2.0.0","html_url":"https://example.com/v2.0.0","assets":[]}`)
	}))
	defer srv.Close()

	u := New("HopStat/HopStat", "v1.0.0", true)
	u.SetReleaseAPIURL(srv.URL)
	rel, err := u.fetchLatest(context.Background())
	if err != nil {
		t.Fatalf("fetchLatest error: %v", err)
	}
	if rel.TagName != "v2.0.0" {
		t.Fatalf("tag = %q", rel.TagName)
	}
}

func TestFetchLatestErrors(t *testing.T) {
	u := New("HopStat/HopStat", "v1.0.0", true)
	u.SetReleaseAPIURL("http://127.0.0.1:1")
	if _, err := u.fetchLatest(context.Background()); err == nil {
		t.Fatal("expected network error")
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()
	u.SetReleaseAPIURL(srv.URL)
	if _, err := u.fetchLatest(context.Background()); err == nil {
		t.Fatal("expected status error")
	}

	srv2 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, "not-json")
	}))
	defer srv2.Close()
	u.SetReleaseAPIURL(srv2.URL)
	if _, err := u.fetchLatest(context.Background()); err == nil {
		t.Fatal("expected decode error")
	}
}

func TestStatusCachesSuccessfulResult(t *testing.T) {
	var calls int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		fmt.Fprint(w, `{"tag_name":"v2.0.0","html_url":"https://example.com/v2.0.0","assets":[]}`)
	}))
	defer srv.Close()

	u := New("HopStat/HopStat", "v1.0.0", true)
	u.SetReleaseAPIURL(srv.URL)

	if _, err := u.Status(context.Background(), ""); err != nil {
		t.Fatalf("first Status: %v", err)
	}
	if _, err := u.Status(context.Background(), ""); err != nil {
		t.Fatalf("second Status: %v", err)
	}
	if calls != 1 {
		t.Fatalf("github calls = %d, want 1", calls)
	}
}

func TestStatus(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"tag_name":"v2.0.0","name":"HopStat v2.0.0","body":"- Fix ping\n- Fix BGP","html_url":"https://example.com/v2.0.0","assets":[]}`)
	}))
	defer srv.Close()

	u := New("HopStat/HopStat", "v1.0.0", true)
	u.SetReleaseAPIURL(srv.URL)
	st, err := u.Status(context.Background(), "")
	if err != nil {
		t.Fatalf("Status error: %v", err)
	}
	if !st.UpdateAvailable || st.Latest != "v2.0.0" {
		t.Fatalf("status = %+v", st)
	}
	if st.ReleaseURL != "https://example.com/v2.0.0" {
		t.Fatalf("release url = %q", st.ReleaseURL)
	}
	if st.ReleaseName != "HopStat v2.0.0" {
		t.Fatalf("release name = %q", st.ReleaseName)
	}
	if st.ReleaseNotes != "- Fix ping\n- Fix BGP" {
		t.Fatalf("release notes = %q", st.ReleaseNotes)
	}
	if runtime.GOOS != "linux" && st.SelfUpdateEnabled {
		t.Fatal("expected self update disabled off linux")
	}
	if st.SelfUpdateReason == "" && runtime.GOOS != "linux" {
		t.Fatal("expected self update reason off linux")
	}
}

func TestStatusDisabledInConfig(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"tag_name":"v2.0.0","html_url":"https://example.com/v2.0.0","assets":[]}`)
	}))
	defer srv.Close()

	u := New("HopStat/HopStat", "v1.0.0", false)
	u.SetReleaseAPIURL(srv.URL)
	st, err := u.Status(context.Background(), "")
	if err != nil {
		t.Fatalf("Status error: %v", err)
	}
	if st.SelfUpdateEnabled {
		t.Fatal("expected disabled")
	}
	if st.SelfUpdateReason != selfUpdateDisabledReason {
		t.Fatalf("reason = %q", st.SelfUpdateReason)
	}
}

func TestDownload(t *testing.T) {
	body := []byte("binary-data")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write(body)
	}))
	defer srv.Close()

	dest := filepath.Join(t.TempDir(), "download.bin")
	u := New("HopStat/HopStat", "v1.0.0", true)
	if err := u.download(context.Background(), srv.URL, dest); err != nil {
		t.Fatalf("download error: %v", err)
	}
	got, err := os.ReadFile(dest)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(body) {
		t.Fatalf("download body mismatch")
	}
}

func TestDownloadErrors(t *testing.T) {
	u := New("HopStat/HopStat", "v1.0.0", true)
	dest := filepath.Join(t.TempDir(), "download.bin")
	if err := u.download(context.Background(), "http://127.0.0.1:1", dest); err == nil {
		t.Fatal("expected network error")
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()
	if err := u.download(context.Background(), srv.URL, dest); err == nil {
		t.Fatal("expected status error")
	}
}

func TestVerifyChecksum(t *testing.T) {
	body := []byte("binary-data")
	assetName := fmt.Sprintf("hopstat-%s-%s", runtime.GOOS, runtime.GOARCH)
	sum := sha256.Sum256(body)
	checksums := fmt.Sprintf("%s  %s\n", hex.EncodeToString(sum[:]), assetName)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(checksums))
	}))
	defer srv.Close()

	filePath := filepath.Join(t.TempDir(), "bin")
	if err := os.WriteFile(filePath, body, 0o755); err != nil {
		t.Fatal(err)
	}

	u := New("HopStat/HopStat", "v1.0.0", true)
	if err := u.verifyChecksum(context.Background(), filePath, assetName, srv.URL); err != nil {
		t.Fatalf("verifyChecksum error: %v", err)
	}
}

func TestVerifyChecksumErrors(t *testing.T) {
	u := New("HopStat/HopStat", "v1.0.0", true)
	filePath := filepath.Join(t.TempDir(), "bin")
	if err := os.WriteFile(filePath, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := u.verifyChecksum(context.Background(), filePath, "missing", "http://127.0.0.1:1"); err == nil {
		t.Fatal("expected fetch error")
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()
	if err := u.verifyChecksum(context.Background(), filePath, "asset", srv.URL); err == nil {
		t.Fatal("expected status error")
	}

	srv2 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, "deadbeef  other-asset\n")
	}))
	defer srv2.Close()
	if err := u.verifyChecksum(context.Background(), filePath, "asset", srv2.URL); err == nil {
		t.Fatal("expected missing entry error")
	}

	body := []byte("binary-data")
	assetName := "asset"
	sum := sha256.Sum256([]byte("wrong"))
	checksums := fmt.Sprintf("%s  %s\n", hex.EncodeToString(sum[:]), assetName)
	srv3 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, checksums)
	}))
	defer srv3.Close()
	if err := u.verifyChecksum(context.Background(), filePath, assetName, srv3.URL); err == nil {
		t.Fatal("expected mismatch error")
	}
	_ = body
}

func TestApplyDisabled(t *testing.T) {
	u := New("HopStat/HopStat", "v1.0.0", false)
	if err := u.Apply(context.Background()); err == nil {
		t.Fatal("expected disabled error")
	}
}

func TestApplyNotSupported(t *testing.T) {
	if runtime.GOOS == "linux" {
		t.Setenv("LG_UPDATE_ENABLED", "false")
	}
	u := New("HopStat/HopStat", "v1.0.0", true)
	if err := u.Apply(context.Background()); err == nil {
		t.Fatal("expected unsupported error")
	}
}

func TestApplyFetchError(t *testing.T) {
	origGOOS := runtimeGOOS
	t.Cleanup(func() { runtimeGOOS = origGOOS })
	runtimeGOOS = "linux"
	t.Setenv("LG_UPDATE_ENABLED", "")

	u := New("HopStat/HopStat", "v1.0.0", true)
	u.SetReleaseAPIURL("http://127.0.0.1:1")
	if err := u.Apply(context.Background()); err == nil {
		t.Fatal("expected fetch error")
	}
}

func TestApplyMissingAssets(t *testing.T) {
	origGOOS := runtimeGOOS
	t.Cleanup(func() { runtimeGOOS = origGOOS })
	runtimeGOOS = "linux"
	t.Setenv("LG_UPDATE_ENABLED", "")

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"tag_name":"v2.0.0","html_url":"https://example.com/v2.0.0","assets":[]}`)
	}))
	defer srv.Close()

	u := New("HopStat/HopStat", "v1.0.0", true)
	u.SetReleaseAPIURL(srv.URL)
	if err := u.Apply(context.Background()); err == nil {
		t.Fatal("expected missing asset error")
	}
}

func TestApplyMissingChecksumAsset(t *testing.T) {
	origGOOS := runtimeGOOS
	t.Cleanup(func() { runtimeGOOS = origGOOS })
	runtimeGOOS = "linux"
	t.Setenv("LG_UPDATE_ENABLED", "")

	assetName := fmt.Sprintf("hopstat-%s-%s", runtime.GOOS, runtime.GOARCH)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, `{"tag_name":"v2.0.0","html_url":"https://example.com/v2.0.0","assets":[{"name":%q,"browser_download_url":"http://example.com/bin"}]}`, assetName)
	}))
	defer srv.Close()

	u := New("HopStat/HopStat", "v1.0.0", true)
	u.SetReleaseAPIURL(srv.URL)
	if err := u.Apply(context.Background()); err == nil {
		t.Fatal("expected missing checksum error")
	}
}

func TestApplySuccessWithoutExec(t *testing.T) {
	origGOOS := runtimeGOOS
	origExec := execProcess
	origExecutable := osExecutable
	origEval := evalSymlinks
	t.Cleanup(func() {
		runtimeGOOS = origGOOS
		execProcess = origExec
		osExecutable = origExecutable
		evalSymlinks = origEval
	})
	runtimeGOOS = "linux"
	t.Setenv("LG_UPDATE_ENABLED", "")

	execProcess = func(path string, argv []string, envv []string) error {
		return nil
	}

	binDir := t.TempDir()
	binPath := filepath.Join(binDir, "hopstat")
	if err := os.WriteFile(binPath, []byte("old-binary"), 0o755); err != nil {
		t.Fatal(err)
	}
	osExecutable = func() (string, error) { return binPath, nil }
	evalSymlinks = func(path string) (string, error) { return path, nil }

	body := []byte("updated-binary")
	srv := mockReleaseServer(t, "v2.0.0", body)
	defer srv.Close()

	u := New("HopStat/HopStat", "v1.0.0", true)
	u.SetReleaseAPIURL(srv.URL)
	if err := u.Apply(context.Background()); err != nil {
		t.Fatalf("Apply error: %v", err)
	}

	got, err := os.ReadFile(binPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(body) {
		t.Fatalf("binary = %q, want %q", got, body)
	}
}

func TestStatusFetchErrorFallsBackToEmbedded(t *testing.T) {
	u := New("HopStat/HopStat", "v2.1.63", true)
	u.SetReleaseAPIURL("http://127.0.0.1:1")
	st, err := u.Status(context.Background(), "")
	if err != nil {
		t.Fatalf("expected embedded fallback, got error: %v", err)
	}
	if !st.UpdateAvailable || !isNewer(st.Latest, "v2.1.63") {
		t.Fatalf("status = %+v", st)
	}
	if strings.TrimSpace(st.ReleaseNotes) == "" {
		t.Fatal("expected embedded release notes")
	}
}

func TestApplyExecutablePathErrorOnSecondCall(t *testing.T) {
	origGOOS := runtimeGOOS
	origExecutable := osExecutable
	origEval := evalSymlinks
	t.Cleanup(func() {
		runtimeGOOS = origGOOS
		osExecutable = origExecutable
		evalSymlinks = origEval
	})
	runtimeGOOS = "linux"
	t.Setenv("LG_UPDATE_ENABLED", "")

	binDir := t.TempDir()
	binPath := filepath.Join(binDir, "hopstat")
	calls := 0
	osExecutable = func() (string, error) {
		calls++
		if calls == 1 {
			return binPath, nil
		}
		return "", errors.New("boom")
	}
	evalSymlinks = func(path string) (string, error) { return path, nil }

	srv := mockReleaseServer(t, "v2.0.0", []byte("updated-binary"))
	defer srv.Close()

	u := New("HopStat/HopStat", "v1.0.0", true)
	u.SetReleaseAPIURL(srv.URL)
	if err := u.Apply(context.Background()); err == nil {
		t.Fatal("expected executable path error")
	}
}

func TestApplyEvalSymlinksErrorOnSecondCall(t *testing.T) {
	origGOOS := runtimeGOOS
	origExecutable := osExecutable
	origEval := evalSymlinks
	t.Cleanup(func() {
		runtimeGOOS = origGOOS
		osExecutable = origExecutable
		evalSymlinks = origEval
	})
	runtimeGOOS = "linux"
	t.Setenv("LG_UPDATE_ENABLED", "")

	binDir := t.TempDir()
	binPath := filepath.Join(binDir, "hopstat")
	calls := 0
	osExecutable = func() (string, error) { return binPath, nil }
	evalSymlinks = func(path string) (string, error) {
		calls++
		if calls == 1 {
			return path, nil
		}
		return "", errors.New("boom")
	}

	srv := mockReleaseServer(t, "v2.0.0", []byte("updated-binary"))
	defer srv.Close()

	u := New("HopStat/HopStat", "v1.0.0", true)
	u.SetReleaseAPIURL(srv.URL)
	if err := u.Apply(context.Background()); err == nil {
		t.Fatal("expected eval symlinks error")
	}
}

func TestApplyChmodError(t *testing.T) {
	origGOOS := runtimeGOOS
	origExec := execProcess
	origExecutable := osExecutable
	origEval := evalSymlinks
	origChmod := osChmodFn
	t.Cleanup(func() {
		runtimeGOOS = origGOOS
		execProcess = origExec
		osExecutable = origExecutable
		evalSymlinks = origEval
		osChmodFn = origChmod
	})
	runtimeGOOS = "linux"
	t.Setenv("LG_UPDATE_ENABLED", "")

	binDir := t.TempDir()
	binPath := filepath.Join(binDir, "hopstat")
	osExecutable = func() (string, error) { return binPath, nil }
	evalSymlinks = func(path string) (string, error) { return path, nil }
	osChmodFn = func(string, os.FileMode) error { return errors.New("chmod failed") }

	srv := mockReleaseServer(t, "v2.0.0", []byte("updated-binary"))
	defer srv.Close()

	u := New("HopStat/HopStat", "v1.0.0", true)
	u.SetReleaseAPIURL(srv.URL)
	if err := u.Apply(context.Background()); err == nil {
		t.Fatal("expected chmod error")
	}
}

func TestFetchLatestInvalidURL(t *testing.T) {
	u := New("HopStat/HopStat", "v1.0.0", true)
	u.SetReleaseAPIURL(string([]byte{0x7f}))
	if _, err := u.fetchLatest(context.Background()); err == nil {
		t.Fatal("expected invalid URL error")
	}
}

func TestDownloadInvalidURL(t *testing.T) {
	u := New("HopStat/HopStat", "v1.0.0", true)
	if err := u.download(context.Background(), string([]byte{0x7f}), filepath.Join(t.TempDir(), "bin")); err == nil {
		t.Fatal("expected invalid URL error")
	}
}

func TestVerifyChecksumInvalidURL(t *testing.T) {
	u := New("HopStat/HopStat", "v1.0.0", true)
	if err := u.verifyChecksum(context.Background(), filepath.Join(t.TempDir(), "bin"), "asset", string([]byte{0x7f})); err == nil {
		t.Fatal("expected invalid URL error")
	}
}

func TestVerifyChecksumScannerError(t *testing.T) {
	body := strings.Repeat("a", bufio.MaxScanTokenSize+1)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, body)
	}))
	defer srv.Close()

	filePath := filepath.Join(t.TempDir(), "bin")
	if err := os.WriteFile(filePath, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}

	u := New("HopStat/HopStat", "v1.0.0", true)
	if err := u.verifyChecksum(context.Background(), filePath, "asset", srv.URL); err == nil {
		t.Fatal("expected scanner error")
	}
}

func TestVerifyChecksumHashReadError(t *testing.T) {
	assetName := fmt.Sprintf("hopstat-%s-%s", runtime.GOOS, runtime.GOARCH)
	sum := sha256.Sum256([]byte("x"))
	checksums := fmt.Sprintf("%s  %s\n", hex.EncodeToString(sum[:]), assetName)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, checksums)
	}))
	defer srv.Close()

	u := New("HopStat/HopStat", "v1.0.0", true)
	if err := u.verifyChecksum(context.Background(), t.TempDir(), assetName, srv.URL); err == nil {
		t.Fatal("expected hash read error")
	}
}

func TestApplyDownloadError(t *testing.T) {
	origGOOS := runtimeGOOS
	origExecutable := osExecutable
	origEval := evalSymlinks
	t.Cleanup(func() {
		runtimeGOOS = origGOOS
		osExecutable = origExecutable
		evalSymlinks = origEval
	})
	runtimeGOOS = "linux"
	t.Setenv("LG_UPDATE_ENABLED", "")

	binDir := t.TempDir()
	binPath := filepath.Join(binDir, "hopstat")
	if err := os.WriteFile(binPath, []byte("old-binary"), 0o755); err != nil {
		t.Fatal(err)
	}
	osExecutable = func() (string, error) { return binPath, nil }
	evalSymlinks = func(path string) (string, error) { return path, nil }

	assetName := fmt.Sprintf("hopstat-%s-%s", runtime.GOOS, runtime.GOARCH)
	var srv *httptest.Server
	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/" || strings.HasSuffix(r.URL.Path, "/releases/latest"):
			fmt.Fprintf(w, `{"tag_name":"v2.0.0","html_url":"https://example.com/v2.0.0","assets":[{"name":%q,"browser_download_url":"http://127.0.0.1:1/binary"},{"name":"checksums.txt","browser_download_url":"%s/checksums"}]}`, assetName, srv.URL)
		case strings.HasSuffix(r.URL.Path, "/checksums"):
			fmt.Fprintf(w, "%s  %s\n", "deadbeef", assetName)
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	u := New("HopStat/HopStat", "v1.0.0", true)
	u.SetReleaseAPIURL(srv.URL)
	if err := u.Apply(context.Background()); err == nil {
		t.Fatal("expected download error")
	}
}

func TestApplyChecksumError(t *testing.T) {
	origGOOS := runtimeGOOS
	origExecutable := osExecutable
	origEval := evalSymlinks
	t.Cleanup(func() {
		runtimeGOOS = origGOOS
		osExecutable = origExecutable
		evalSymlinks = origEval
	})
	runtimeGOOS = "linux"
	t.Setenv("LG_UPDATE_ENABLED", "")

	binDir := t.TempDir()
	binPath := filepath.Join(binDir, "hopstat")
	if err := os.WriteFile(binPath, []byte("old-binary"), 0o755); err != nil {
		t.Fatal(err)
	}
	osExecutable = func() (string, error) { return binPath, nil }
	evalSymlinks = func(path string) (string, error) { return path, nil }

	body := []byte("updated-binary")
	assetName := fmt.Sprintf("hopstat-%s-%s", runtime.GOOS, runtime.GOARCH)
	var srv *httptest.Server
	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/" || strings.HasSuffix(r.URL.Path, "/releases/latest"):
			fmt.Fprintf(w, `{"tag_name":"v2.0.0","html_url":"https://example.com/v2.0.0","assets":[{"name":%q,"browser_download_url":"%s/binary"},{"name":"checksums.txt","browser_download_url":"%s/checksums"}]}`, assetName, srv.URL, srv.URL)
		case strings.HasSuffix(r.URL.Path, "/binary"):
			w.Write(body)
		case strings.HasSuffix(r.URL.Path, "/checksums"):
			fmt.Fprintf(w, "deadbeef  %s\n", assetName)
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	u := New("HopStat/HopStat", "v1.0.0", true)
	u.SetReleaseAPIURL(srv.URL)
	if err := u.Apply(context.Background()); err == nil {
		t.Fatal("expected checksum error")
	}
}

func TestDownloadReadOnlyDirectory(t *testing.T) {
	dir := t.TempDir()
	if err := os.Chmod(dir, 0o555); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(dir, 0o755) })

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("data"))
	}))
	defer srv.Close()

	u := New("HopStat/HopStat", "v1.0.0", true)
	dest := filepath.Join(dir, "nested", "download.bin")
	if err := u.download(context.Background(), srv.URL, dest); err == nil {
		t.Fatal("expected open file error")
	}
}

func TestVerifyChecksumOpenError(t *testing.T) {
	u := New("HopStat/HopStat", "v1.0.0", true)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, "deadbeef  asset\n")
	}))
	defer srv.Close()
	if err := u.verifyChecksum(context.Background(), filepath.Join(t.TempDir(), "missing"), "asset", srv.URL); err == nil {
		t.Fatal("expected open error")
	}
}

func TestApplyRenameError(t *testing.T) {
	origGOOS := runtimeGOOS
	origExec := execProcess
	origExecutable := osExecutable
	origEval := evalSymlinks
	t.Cleanup(func() {
		runtimeGOOS = origGOOS
		execProcess = origExec
		osExecutable = origExecutable
		evalSymlinks = origEval
	})
	runtimeGOOS = "linux"
	t.Setenv("LG_UPDATE_ENABLED", "")
	execProcess = func(string, []string, []string) error { return nil }

	binDir := t.TempDir()
	if err := os.Mkdir(filepath.Join(binDir, "hopstat"), 0o755); err != nil {
		t.Fatal(err)
	}
	execPath := filepath.Join(binDir, "hopstat")
	osExecutable = func() (string, error) { return execPath, nil }
	evalSymlinks = func(path string) (string, error) { return path, nil }

	srv := mockReleaseServer(t, "v2.0.0", []byte("updated-binary"))
	defer srv.Close()

	u := New("HopStat/HopStat", "v1.0.0", true)
	u.SetReleaseAPIURL(srv.URL)
	if err := u.Apply(context.Background()); err == nil {
		t.Fatal("expected replace binary error")
	}
}

func TestStatusEnabledOnLinux(t *testing.T) {
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
	binPath := filepath.Join(dir, "hopstat")
	osExecutable = func() (string, error) { return binPath, nil }
	evalSymlinks = func(path string) (string, error) { return path, nil }
	t.Setenv("LG_UPDATE_ENABLED", "")

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"tag_name":"v2.0.0","html_url":"https://example.com/v2.0.0","assets":[]}`)
	}))
	defer srv.Close()

	u := New("HopStat/HopStat", "v1.0.0", true)
	u.SetReleaseAPIURL(srv.URL)
	st, err := u.Status(context.Background(), "")
	if err != nil {
		t.Fatalf("Status error: %v", err)
	}
	if !st.SelfUpdateEnabled {
		t.Fatalf("expected enabled on linux, reason=%q", st.SelfUpdateReason)
	}
	if st.SelfUpdateReason != "" {
		t.Fatalf("expected empty reason, got %q", st.SelfUpdateReason)
	}
}
