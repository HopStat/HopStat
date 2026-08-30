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
	"sync"
	"syscall"
	"time"
)

const githubAPIURL = "https://api.github.com/repos/%s/releases/latest"

// StatusErrHook, when non-nil, makes Status return this error immediately (tests only).
var StatusErrHook error

var execProcess = syscall.Exec

var (
	osExecutable = os.Executable
	evalSymlinks = filepath.EvalSymlinks
	osChmodFn    = os.Chmod
)

type Release struct {
	TagName string  `json:"tag_name"`
	Name    string  `json:"name"`
	Body    string  `json:"body"`
	HTMLURL string  `json:"html_url"`
	Assets  []Asset `json:"assets"`
}

type Asset struct {
	Name               string `json:"name"`
	BrowserDownloadURL string `json:"browser_download_url"`
}

type Status struct {
	Current           string   `json:"current"`
	Latest            string   `json:"latest"`
	UpdateAvailable   bool     `json:"update_available"`
	ReleaseURL        string   `json:"release_url"`
	ReleaseName       string   `json:"release_name,omitempty"`
	ReleaseNotes      string   `json:"release_notes,omitempty"`
	ReleaseVersions   []string `json:"release_versions,omitempty"`
	SelfUpdateEnabled bool     `json:"self_update_enabled"`
	SelfUpdateReason  string   `json:"self_update_reason,omitempty"`
}

// SettingSelfUpdateEnabled is the settings key the admin panel writes. Stored as "true" or
// "false"; absent means the config value still decides.
const SettingSelfUpdateEnabled = "self_update_enabled"

// The switch now lives in two places — config.yaml seeds it, the admin panel owns it — so
// the reason names neither.
const selfUpdateDisabledReason = "self-update is disabled"

type Updater struct {
	repo          string
	current       string
	enabled       bool
	enabledLive   func() (bool, bool)
	releaseAPIURL string
	apiClient     *http.Client
	dlClient      *http.Client
	statusCacheMu sync.Mutex
	statusCache   map[string]cachedStatus
}

type cachedStatus struct {
	status *Status
	err    error
	until  time.Time
}

const statusCacheTTL = 60 * time.Second
const statusErrorCacheTTL = 15 * time.Second

func New(repo, currentVersion string, enabled bool) *Updater {
	return &Updater{
		repo:      repo,
		current:   currentVersion,
		enabled:   enabled,
		apiClient: &http.Client{Timeout: 15 * time.Second},
		dlClient:  &http.Client{Timeout: 10 * time.Minute},
	}
}

// SetEnabledSource supplies a live answer for whether self-update is allowed, so the
// switch in the admin panel applies without a restart. The callback's second return says
// whether it has an answer; when it does not, the config value stands.
func (u *Updater) SetEnabledSource(fn func() (bool, bool)) {
	u.enabledLive = fn
}

// selfUpdateAllowed resolves the live setting first, then the config value.
func (u *Updater) selfUpdateAllowed() bool {
	if u.enabledLive != nil {
		if enabled, ok := u.enabledLive(); ok {
			return enabled
		}
	}
	return u.enabled
}

// SetReleaseAPIURL overrides the GitHub releases/latest URL (for tests).
func (u *Updater) SetReleaseAPIURL(url string) {
	u.releaseAPIURL = strings.TrimSpace(url)
}

func (u *Updater) Status(ctx context.Context, since string) (*Status, error) {
	if StatusErrHook != nil {
		return nil, StatusErrHook
	}
	since = normalizeTag(since)
	if cached, ok := u.cachedStatus(since); ok {
		if cached.err != nil {
			return nil, cached.err
		}
		return cached.status, nil
	}

	// buildStatus always falls back to embedded on GitHub failure; it never returns an error.
	status, _ := u.buildStatus(ctx, since)
	u.storeStatusCache(since, status, nil)
	return status, nil
}

func (u *Updater) cachedStatus(since string) (cachedStatus, bool) {
	u.statusCacheMu.Lock()
	defer u.statusCacheMu.Unlock()
	entry, ok := u.statusCache[since]
	if !ok || time.Now().After(entry.until) {
		return cachedStatus{}, false
	}
	return entry, true
}

func (u *Updater) storeStatusCache(since string, status *Status, err error) {
	u.statusCacheMu.Lock()
	defer u.statusCacheMu.Unlock()
	if u.statusCache == nil {
		u.statusCache = make(map[string]cachedStatus)
	}
	ttl := statusCacheTTL
	if err != nil {
		ttl = statusErrorCacheTTL
	}
	u.statusCache[since] = cachedStatus{status: status, err: err, until: time.Now().Add(ttl)}
}

func (u *Updater) buildStatus(ctx context.Context, since string) (*Status, error) {
	rel, err := u.fetchLatest(ctx)
	if err != nil {
		slog.Warn("github release check failed, using embedded release notes", "error", err)
		return u.buildStatusFromEmbedded(ctx, since), nil
	}
	allowed := u.selfUpdateAllowed()
	supported, reason := SelfUpdateSupported()
	selfUpdateEnabled := allowed && supported
	selfUpdateReason := reason
	if !allowed {
		selfUpdateReason = selfUpdateDisabledReason
	} else if supported {
		selfUpdateReason = ""
	}

	releaseName := rel.Name
	releaseNotes := rel.Body
	var releaseVersions []string
	since = normalizeTag(since)
	if since != "" && isNewer(rel.TagName, since) {
		if agg, versions, aggErr := u.AggregateReleaseNotes(ctx, since, rel.TagName); aggErr == nil && strings.TrimSpace(agg) != "" {
			releaseNotes = agg
			releaseVersions = versions
			if name := aggregatedReleaseName(versions); name != "" {
				releaseName = name
			}
		}
	}

	return &Status{
		Current:           u.current,
		Latest:            rel.TagName,
		UpdateAvailable:   isNewer(rel.TagName, u.current),
		ReleaseURL:        rel.HTMLURL,
		ReleaseName:       releaseName,
		ReleaseNotes:      releaseNotes,
		ReleaseVersions:   releaseVersions,
		SelfUpdateEnabled: selfUpdateEnabled,
		SelfUpdateReason:  selfUpdateReason,
	}, nil
}

func (u *Updater) buildStatusFromEmbedded(ctx context.Context, since string) *Status {
	latest := normalizeTag(latestEmbeddedVersion())
	if latest == "" {
		latest = normalizeTag(u.current)
	}
	since = normalizeTag(since)

	releaseName := "HopStat " + latest
	releaseNotes := ""
	var releaseVersions []string
	if since != "" && isNewer(latest, since) {
		if agg, versions, aggErr := u.AggregateReleaseNotes(ctx, since, latest); aggErr == nil && strings.TrimSpace(agg) != "" {
			releaseNotes = agg
			releaseVersions = versions
			if name := aggregatedReleaseName(versions); name != "" {
				releaseName = name
			}
		}
	} else if note, ok := readEmbeddedNote(latest); ok {
		releaseNotes = note
	}

	allowed := u.selfUpdateAllowed()
	supported, reason := SelfUpdateSupported()
	selfUpdateEnabled := allowed && supported
	selfUpdateReason := reason
	if !allowed {
		selfUpdateReason = selfUpdateDisabledReason
	} else if supported {
		selfUpdateReason = ""
	}

	return &Status{
		Current:           u.current,
		Latest:            latest,
		UpdateAvailable:   isNewer(latest, u.current),
		ReleaseURL:        fmt.Sprintf("https://github.com/%s/releases/tag/%s", u.repo, latest),
		ReleaseName:       releaseName,
		ReleaseNotes:      releaseNotes,
		ReleaseVersions:   releaseVersions,
		SelfUpdateEnabled: selfUpdateEnabled,
		SelfUpdateReason:  selfUpdateReason,
	}
}

func setGitHubRequestHeaders(req *http.Request) {
	req.Header.Set("Accept", "application/vnd.github.v3+json")
	req.Header.Set("User-Agent", "hopstat")
	token := strings.TrimSpace(os.Getenv("GITHUB_TOKEN"))
	if token == "" {
		token = strings.TrimSpace(os.Getenv("LG_GITHUB_TOKEN"))
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
}

// Apply downloads the latest binary, replaces the executable, and execs the new process.
// On success syscall.Exec never returns — the current process is replaced.
func (u *Updater) Apply(ctx context.Context) error {
	if !u.selfUpdateAllowed() {
		return fmt.Errorf("self-update is disabled")
	}
	if supported, reason := SelfUpdateSupported(); !supported {
		return fmt.Errorf("self-update not supported: %s", reason)
	}
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

	execPath, err := osExecutable()
	if err != nil {
		return fmt.Errorf("executable path: %w", err)
	}
	execPath, err = evalSymlinks(execPath)
	if err != nil {
		return fmt.Errorf("eval symlinks: %w", err)
	}

	slog.Info("downloading update", "version", rel.TagName, "asset", assetName)

	tmp := execPath + ".new"
	if err := u.download(ctx, dlURL, tmp); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("download: %w", err)
	}

	if err := u.verifyChecksum(ctx, tmp, assetName, checksumURL); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("checksum verification failed: %w", err)
	}

	if err := osChmodFn(tmp, 0755); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("chmod: %w", err)
	}

	if err := os.Rename(tmp, execPath); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("replace binary: %w", err)
	}

	slog.Info("restarting with new binary", "path", execPath)
	return execProcess(execPath, os.Args, os.Environ())
}

func (u *Updater) fetchLatest(ctx context.Context) (*Release, error) {
	apiURL := u.releaseAPIURL
	if apiURL == "" {
		apiURL = fmt.Sprintf(githubAPIURL, u.repo)
	}
	req, err := http.NewRequestWithContext(ctx, "GET", apiURL, nil)
	if err != nil {
		return nil, err
	}
	setGitHubRequestHeaders(req)

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
