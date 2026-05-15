package updater

import (
	"bufio"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"time"
)

const githubAPIURL = "https://api.github.com/repos/%s/releases/latest"

type Release struct {
	TagName string  `json:"tag_name"`
	HTMLURL string  `json:"html_url"`
	Assets  []Asset `json:"assets"`
}

type Asset struct {
	Name               string `json:"name"`
	BrowserDownloadURL string `json:"browser_download_url"`
}

type Status struct {
	Current         string `json:"current"`
	Latest          string `json:"latest"`
	UpdateAvailable bool   `json:"update_available"`
	ReleaseURL      string `json:"release_url"`
}

type Updater struct {
	repo      string
	current   string
	apiClient *http.Client
	dlClient  *http.Client
}

func New(repo, currentVersion string) *Updater {
	return &Updater{
		repo:      repo,
		current:   currentVersion,
		apiClient: &http.Client{Timeout: 15 * time.Second},
		dlClient:  &http.Client{Timeout: 10 * time.Minute},
	}
}

func (u *Updater) Status(ctx context.Context) (*Status, error) {
	rel, err := u.fetchLatest(ctx)
	if err != nil {
		return nil, err
	}
	return &Status{
		Current:         u.current,
		Latest:          rel.TagName,
		UpdateAvailable: isNewer(rel.TagName, u.current),
		ReleaseURL:      rel.HTMLURL,
	}, nil
}

// Apply downloads the latest binary, replaces the executable, and execs the new process.
// On success syscall.Exec never returns — the current process is replaced.
func (u *Updater) Apply(ctx context.Context) error {
	rel, err := u.fetchLatest(ctx)
	if err != nil {
		return fmt.Errorf("fetch release: %w", err)
	}

	assetName := fmt.Sprintf("hopstat-%s-%s", runtime.GOOS, runtime.GOARCH)
	var dlURL, checksumURL string
	for _, a := range rel.Assets {
		switch a.Name {
		case assetName:
			dlURL = a.BrowserDownloadURL
		case "checksums.txt":
			checksumURL = a.BrowserDownloadURL
		}
	}
	if dlURL == "" {
		return fmt.Errorf("no release asset for %s/%s", runtime.GOOS, runtime.GOARCH)
	}
	if checksumURL == "" {
		return fmt.Errorf("checksums.txt not found in release assets")
	}

	execPath, err := os.Executable()
	if err != nil {
		return fmt.Errorf("executable path: %w", err)
	}
	execPath, err = filepath.EvalSymlinks(execPath)
	if err != nil {
		return fmt.Errorf("eval symlinks: %w", err)
	}

	slog.Info("downloading update", "version", rel.TagName, "asset", assetName)

	tmp := execPath + ".new"
	if err := u.download(ctx, dlURL, tmp); err != nil {
		os.Remove(tmp)
		return fmt.Errorf("download: %w", err)
	}

	if err := u.verifyChecksum(ctx, tmp, assetName, checksumURL); err != nil {
		os.Remove(tmp)
		return fmt.Errorf("checksum verification failed: %w", err)
	}

	if err := os.Chmod(tmp, 0755); err != nil {
		os.Remove(tmp)
		return fmt.Errorf("chmod: %w", err)
	}

	if err := os.Rename(tmp, execPath); err != nil {
		os.Remove(tmp)
		return fmt.Errorf("replace binary: %w", err)
	}

	slog.Info("restarting with new binary", "path", execPath)
	return syscall.Exec(execPath, os.Args, os.Environ())
}

func (u *Updater) fetchLatest(ctx context.Context) (*Release, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", fmt.Sprintf(githubAPIURL, u.repo), nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/vnd.github.v3+json")
	req.Header.Set("User-Agent", "hopstat")

	resp, err := u.apiClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("github api: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("github api returned %d", resp.StatusCode)
	}

	var rel Release
	if err := json.NewDecoder(resp.Body).Decode(&rel); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}
	return &rel, nil
}

func (u *Updater) download(ctx context.Context, url, dest string) error {
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return err
	}

	resp, err := u.dlClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("download returned %d", resp.StatusCode)
	}

	f, err := os.OpenFile(dest, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0755)
	if err != nil {
		return err
	}
	defer f.Close()

	_, err = io.Copy(f, io.LimitReader(resp.Body, 200<<20))
	return err
}

func (u *Updater) verifyChecksum(ctx context.Context, filePath, assetName, checksumURL string) error {
	req, err := http.NewRequestWithContext(ctx, "GET", checksumURL, nil)
	if err != nil {
		return err
	}
	resp, err := u.apiClient.Do(req)
	if err != nil {
		return fmt.Errorf("fetch checksums: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("checksums.txt returned %d", resp.StatusCode)
	}

	var expected string
	scanner := bufio.NewScanner(resp.Body)
	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		if len(fields) == 2 && fields[1] == assetName {
			expected = fields[0]
			break
		}
	}
	if err := scanner.Err(); err != nil {
		return fmt.Errorf("read checksums: %w", err)
	}
	if expected == "" {
		return fmt.Errorf("no checksum entry for %s", assetName)
	}

	f, err := os.Open(filePath)
	if err != nil {
		return err
	}
	defer f.Close()

	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return fmt.Errorf("hash binary: %w", err)
	}
	got := hex.EncodeToString(h.Sum(nil))
	if got != expected {
		return fmt.Errorf("SHA256 mismatch: got %s, want %s", got, expected)
	}
	slog.Info("checksum verified", "asset", assetName, "sha256", got)
	return nil
}

// isNewer returns true if latest tag is a newer semver than current.
func isNewer(latest, current string) bool {
	if current == "dev" || current == "" {
		return latest != ""
	}
	return semverCmp(latest, current) > 0
}

func semverCmp(a, b string) int {
	ap, bp := parseSemver(a), parseSemver(b)
	for i := range 3 {
		if ap[i] != bp[i] {
			if ap[i] > bp[i] {
				return 1
			}
			return -1
		}
	}
	return 0
}

func parseSemver(v string) [3]int {
	v = strings.TrimPrefix(v, "v")
	parts := strings.SplitN(v, ".", 3)
	var out [3]int
	for i, p := range parts {
		if i >= 3 {
			break
		}
		p = strings.SplitN(p, "-", 2)[0]
		n := 0
		for _, c := range p {
			if c >= '0' && c <= '9' {
				n = n*10 + int(c-'0')
			}
		}
		out[i] = n
	}
	return out
}
