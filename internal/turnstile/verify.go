package turnstile

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/HopStat/HopStat/internal/target"
)

const (
	ActionQuery   = "query"
	siteverifyURL = "https://challenges.cloudflare.com/turnstile/v0/siteverify"
	maxTokenLen   = 2048
	verifyTimeout = 10 * time.Second
)

var errRejected = errors.New("turnstile rejected")

// Verifier calls Cloudflare siteverify. An empty secret leaves the check disabled.
type Verifier struct {
	secret     string
	hostnames  map[string]struct{}
	endpoint   string
	httpClient *http.Client
}

type siteverifyResult struct {
	Success  bool   `json:"success"`
	Action   string `json:"action"`
	Hostname string `json:"hostname"`
}

// New builds a verifier. An empty secret leaves the public query API open.
func New(cfg Config) *Verifier {
	hosts := make(map[string]struct{}, len(cfg.Hostnames))
	for _, hostname := range cfg.Hostnames {
		hostname = strings.TrimSpace(hostname)
		if hostname == "" {
			continue
		}
		hosts[hostname] = struct{}{}
	}
	return &Verifier{
		secret:    strings.TrimSpace(cfg.Secret),
		hostnames: hosts,
		endpoint:  siteverifyURL,
		httpClient: &http.Client{
			Timeout:       verifyTimeout,
			CheckRedirect: target.CheckSameHostRedirect,
		},
	}
}

func (v *Verifier) Enabled() bool {
	return v != nil && v.secret != ""
}

// With returns a verifier for cfg that keeps this verifier's siteverify client.
func (v *Verifier) With(cfg Config) *Verifier {
	next := New(cfg)
	if v == nil {
		return next
	}
	next.endpoint = v.endpoint
	if v.httpClient != nil {
		next.httpClient = v.httpClient
	}
	return next
}

// UseHTTPClient replaces the siteverify client. Tests use it to avoid the network.
func (v *Verifier) UseHTTPClient(client *http.Client) {
	if client == nil {
		return
	}
	v.httpClient = client
}

// Verify checks a single-use Turnstile token. Tokens are not accepted twice by Cloudflare;
// callers still treat any failed siteverify as a rejection.
func (v *Verifier) Verify(ctx context.Context, token, remoteIP string) error {
	token = strings.TrimSpace(token)
	if token == "" || len(token) > maxTokenLen || len(v.hostnames) == 0 {
		return errRejected
	}

	ctx, cancel := context.WithTimeout(ctx, verifyTimeout)
	defer cancel()

	form := url.Values{}
	form.Set("secret", v.secret)
	form.Set("response", token)
	form.Set("remoteip", remoteIP)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, v.endpoint, strings.NewReader(form.Encode()))
	if err != nil {
		return errRejected
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := v.httpClient.Do(req)
	if err != nil {
		return errRejected
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return errRejected
	}
	if resp.StatusCode != http.StatusOK {
		return errRejected
	}

	var result siteverifyResult
	if err := json.Unmarshal(body, &result); err != nil {
		return errRejected
	}
	if _, ok := v.hostnames[result.Hostname]; !ok || !result.Success || result.Action != ActionQuery {
		return errRejected
	}
	return nil
}
