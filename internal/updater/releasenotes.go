package updater

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strings"

	whatsnew "github.com/HopStat/HopStat/docs/whats-new"
)

var (
	fetchGitTagsHook       func(context.Context, string) ([]string, error)
	fetchRawNoteHook       func(context.Context, string, string) (string, error)
	listEmbeddedVersionsHook func() []string
)

func latestEmbeddedVersion() string {
	versions := listEmbeddedVersions()
	if len(versions) == 0 {
		return ""
	}
	return versions[len(versions)-1]
}

func listEmbeddedVersions() []string {
	if listEmbeddedVersionsHook != nil {
		return listEmbeddedVersionsHook()
	}
	// embed directive is `v*.md` so ReadDir always succeeds and every entry matches.
	entries, _ := whatsnew.Files.ReadDir(".")
	var versions []string
	for _, entry := range entries {
		versions = append(versions, strings.TrimSuffix(entry.Name(), ".md"))
	}
	sortVersions(versions)
	return versions
}

func readEmbeddedNote(version string) (string, bool) {
	version = normalizeTag(version)
	data, err := whatsnew.Files.ReadFile(version + ".md")
	if err != nil {
		return "", false
	}
	return strings.TrimSpace(string(data)), true
}

func rawNotesURL(repo, version string) string {
	version = normalizeTag(version)
	return fmt.Sprintf("https://raw.githubusercontent.com/%s/main/docs/whats-new/%s.md", repo, version)
}

func (u *Updater) fetchRawNote(ctx context.Context, version string) (string, error) {
	if fetchRawNoteHook != nil {
		return fetchRawNoteHook(ctx, u.repo, version)
	}
	// rawNotesURL always produces a valid URL; io.ReadAll on an HTTP response body never errors.
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, rawNotesURL(u.repo, version), nil)
	req.Header.Set("User-Agent", "hopstat")

	resp, err := u.apiClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("raw notes returned %d", resp.StatusCode)
	}
	data, _ := io.ReadAll(io.LimitReader(resp.Body, 256<<10))
	return strings.TrimSpace(string(data)), nil
}

func (u *Updater) fetchGitTags(ctx context.Context) ([]string, error) {
	if fetchGitTagsHook != nil {
		return fetchGitTagsHook(ctx, u.repo)
	}
	url := fmt.Sprintf("https://api.github.com/repos/%s/tags?per_page=100", u.repo)
	// URL is always valid; error from NewRequestWithContext is impossible here.
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	setGitHubRequestHeaders(req)

	resp, err := u.apiClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("github tags returned %d", resp.StatusCode)
	}

	var tags []struct {
		Name string `json:"name"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&tags); err != nil {
		return nil, err
	}
	var versions []string
	for _, tag := range tags {
		v := normalizeTag(tag.Name)
		if v == "" || !strings.HasPrefix(v, "v") {
			continue
		}
		versions = append(versions, v)
	}
	sortVersions(versions)
	return versions, nil
}

func (u *Updater) allKnownVersions(ctx context.Context) []string {
	seen := map[string]struct{}{}
	var merged []string
	// listEmbeddedVersions returns a deduplicated sorted list; no dedup needed here.
	for _, v := range listEmbeddedVersions() {
		seen[v] = struct{}{}
		merged = append(merged, v)
	}
	if tags, err := u.fetchGitTags(ctx); err == nil {
		for _, v := range tags {
			if _, ok := seen[v]; ok {
				continue
			}
			seen[v] = struct{}{}
			merged = append(merged, v)
		}
	}
	sortVersions(merged)
	return merged
}

func versionsBetweenExclusive(since, latest string, known []string) []string {
	since = normalizeTag(since)
	latest = normalizeTag(latest)
	if since == "" || latest == "" || semverCmp(latest, since) <= 0 {
		return nil
	}

	seen := map[string]struct{}{}
	var out []string
	for _, v := range known {
		v = normalizeTag(v)
		if semverCmp(v, since) <= 0 || semverCmp(v, latest) > 0 {
			continue
		}
		if _, ok := seen[v]; ok {
			continue
		}
		seen[v] = struct{}{}
		out = append(out, v)
	}
	if semverCmp(latest, since) > 0 {
		if _, ok := seen[latest]; !ok {
			out = append(out, latest)
		}
	}
	sortVersions(out)
	return out
}

func (u *Updater) loadReleaseNote(ctx context.Context, version string) (string, error) {
	if note, ok := readEmbeddedNote(version); ok && note != "" {
		return note, nil
	}
	return u.fetchRawNote(ctx, version)
}

// AggregateReleaseNotes returns markdown for every release after since up to and including latest.
func (u *Updater) AggregateReleaseNotes(ctx context.Context, since, latest string) (markdown string, versions []string, err error) {
	versions = versionsBetweenExclusive(since, latest, u.allKnownVersions(ctx))
	if len(versions) == 0 {
		return "", nil, nil
	}

	var parts []string
	for _, version := range versions {
		note, loadErr := u.loadReleaseNote(ctx, version)
		if loadErr != nil || strings.TrimSpace(note) == "" {
			continue
		}
		parts = append(parts, note)
	}
	if len(parts) == 0 {
		return "", versions, nil
	}
	return strings.Join(parts, "\n\n---\n\n"), versions, nil
}

func aggregatedReleaseName(versions []string) string {
	if len(versions) == 0 {
		return ""
	}
	if len(versions) == 1 {
		return "HopStat " + versions[0]
	}
	return fmt.Sprintf("HopStat %s – %s", versions[0], versions[len(versions)-1])
}

func normalizeTag(v string) string {
	return strings.TrimSpace(v)
}

func sortVersions(versions []string) {
	sort.Slice(versions, func(i, j int) bool {
		return semverCmp(versions[i], versions[j]) < 0
	})
}
