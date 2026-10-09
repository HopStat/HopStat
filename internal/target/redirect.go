package target

import (
	"fmt"
	"net/http"
)

// maxRedirects bounds redirect chains independently of Go's default of 10, so a
// vendor endpoint cannot bounce a request around the policy below.
const maxRedirects = 3

// CheckSameHostRedirect is the http.Client CheckRedirect policy for clients whose
// destination host is fixed by a literal in the source.
//
// Those clients are not operator-controlled — only a path or query segment varies —
// so a redirect off that host is never legitimate. Following one would hand the
// request to whatever host the response named, which is exactly the hop an internal
// address (cloud metadata, a loopback admin port, a link-local service) needs in
// order to be reached from a process that is otherwise careful about where it dials.
//
// Redirects that stay on the original host are still followed: a vendor may serve an
// asset from a sibling path on the same host, and that does not move the destination.
func CheckSameHostRedirect(req *http.Request, via []*http.Request) error {
	if len(via) == 0 {
		return nil
	}
	if len(via) > maxRedirects {
		return fmt.Errorf("stopped after %d redirects", len(via))
	}
	from := via[0].URL.Host
	if req.URL.Host == from {
		return nil
	}
	return fmt.Errorf("refusing cross-host redirect from %s to %s", from, req.URL.Host)
}

// GitHub serves release assets from github.com through a 302 to its asset CDN.
// objects.githubusercontent.com is the older name for the same store.
var GitHubReleaseAssetHosts = []string{
	"release-assets.githubusercontent.com",
	"objects.githubusercontent.com",
}

// MaxMind answers download.maxmind.com with a 302 to a presigned URL on its R2 bucket,
// and documents this host as the one firewalls must allow.
var MaxMindDownloadHosts = []string{
	"mm-prod-geoip-databases.a2649acb697e2c09b632799562c076f2.r2.cloudflarestorage.com",
}

// CheckRedirectToHosts is CheckSameHostRedirect plus a fixed list of hosts the vendor is
// known to hand its downloads to. Those hops are part of the vendor's own delivery, so
// refusing them breaks the download; any other host is still refused, and a listed host
// is only accepted over HTTPS.
func CheckRedirectToHosts(hosts ...string) func(*http.Request, []*http.Request) error {
	allowed := make(map[string]struct{}, len(hosts))
	for _, h := range hosts {
		allowed[h] = struct{}{}
	}
	return func(req *http.Request, via []*http.Request) error {
		err := CheckSameHostRedirect(req, via)
		if err == nil || len(via) > maxRedirects {
			return err
		}
		if _, ok := allowed[req.URL.Hostname()]; ok && req.URL.Scheme == "https" && req.URL.Port() == "" {
			return nil
		}
		return err
	}
}
