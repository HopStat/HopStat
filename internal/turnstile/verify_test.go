package turnstile

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strings"
	"testing"

	"github.com/HopStat/HopStat/internal/config"
)

func TestVerifierWithKeepsClient(t *testing.T) {
	var unset *Verifier
	got := unset.With(config.TurnstileConfig{Secret: "s", SiteKey: "k", Hostnames: []string{"h"}})
	if !got.Enabled() || got.httpClient == nil {
		t.Fatal("nil source should still build a verifier")
	}

	base := New(config.TurnstileConfig{})
	base.endpoint = "http://example.test/siteverify"
	base.httpClient = nil
	kept := base.With(config.TurnstileConfig{Secret: "secret", Hostnames: []string{"lg.example"}})
	if kept.endpoint != base.endpoint || kept.httpClient == nil {
		t.Fatalf("endpoint = %s client set = %v", kept.endpoint, kept.httpClient != nil)
	}

	client := &http.Client{}
	base.httpClient = client
	kept = base.With(config.TurnstileConfig{Secret: "secret", Hostnames: []string{"lg.example"}})
	if kept.httpClient != client || kept.secret != "secret" {
		t.Fatal("expected the existing siteverify client and the new secret")
	}
}

func TestVerifierDisabled(t *testing.T) {
	var unset *Verifier
	if unset.Enabled() {
		t.Fatal("nil verifier should be disabled")
	}
	v := New(config.TurnstileConfig{})
	if v.Enabled() {
		t.Fatal("empty secret should be disabled")
	}
	v.UseHTTPClient(nil)
	if v.httpClient == nil {
		t.Fatal("nil client should keep the existing client")
	}
}

func TestNewDropsBlankHostnames(t *testing.T) {
	v := New(config.TurnstileConfig{
		Secret:    " secret ",
		SiteKey:   "site",
		Hostnames: []string{" ", "lg.example"},
	})
	if !v.Enabled() || v.secret != "secret" {
		t.Fatalf("secret = %q enabled = %v", v.secret, v.Enabled())
	}
	if _, ok := v.hostnames["lg.example"]; !ok || len(v.hostnames) != 1 {
		t.Fatalf("hostnames = %#v", v.hostnames)
	}
}

func TestVerifyRejectsTokenShape(t *testing.T) {
	v := New(config.TurnstileConfig{Secret: "secret", Hostnames: []string{"lg.example"}})
	if err := v.Verify(context.Background(), "  ", "203.0.113.5"); err == nil {
		t.Fatal("expected empty token rejection")
	}
	if err := v.Verify(context.Background(), strings.Repeat("a", maxTokenLen+1), "203.0.113.5"); err == nil {
		t.Fatal("expected long token rejection")
	}
	open := New(config.TurnstileConfig{Secret: "secret"})
	if err := open.Verify(context.Background(), "token", "203.0.113.5"); err == nil {
		t.Fatal("expected missing hostname rejection")
	}
}

func TestVerifyRequestBuildFailure(t *testing.T) {
	v := New(config.TurnstileConfig{Secret: "secret", Hostnames: []string{"lg.example"}})
	v.endpoint = "http://["
	if err := v.Verify(context.Background(), "token", "203.0.113.5"); err == nil {
		t.Fatal("expected request build failure")
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func verifierWith(t *testing.T, rt roundTripFunc) *Verifier {
	t.Helper()
	v := New(config.TurnstileConfig{Secret: "secret", Hostnames: []string{"lg.example"}})
	v.UseHTTPClient(&http.Client{Transport: rt})
	return v
}

func TestVerifyTransportAndBodyFailures(t *testing.T) {
	v := verifierWith(t, func(r *http.Request) (*http.Response, error) {
		return nil, errors.New("dial")
	})
	if err := v.Verify(context.Background(), "token", "203.0.113.5"); err == nil {
		t.Fatal("expected transport error")
	}

	v = verifierWith(t, func(r *http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(errReader{}),
			Header:     make(http.Header),
			Request:    r,
		}, nil
	})
	if err := v.Verify(context.Background(), "token", "203.0.113.5"); err == nil {
		t.Fatal("expected body read error")
	}

	v = verifierWith(t, func(r *http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusBadGateway,
			Body:       io.NopCloser(strings.NewReader(`{"success":true}`)),
			Header:     make(http.Header),
			Request:    r,
		}, nil
	})
	if err := v.Verify(context.Background(), "token", "203.0.113.5"); err == nil {
		t.Fatal("expected non-200 rejection")
	}

	v = verifierWith(t, func(r *http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(strings.NewReader(`{`)),
			Header:     make(http.Header),
			Request:    r,
		}, nil
	})
	if err := v.Verify(context.Background(), "token", "203.0.113.5"); err == nil {
		t.Fatal("expected json rejection")
	}
}

func TestVerifySiteverifyDecision(t *testing.T) {
	cases := []struct {
		name string
		body string
		ok   bool
	}{
		{name: "success", body: `{"success":true,"action":"query","hostname":"lg.example"}`, ok: true},
		{name: "not success", body: `{"success":false,"action":"query","hostname":"lg.example","error-codes":["invalid-input-response"]}`},
		{name: "wrong action", body: `{"success":true,"action":"login","hostname":"lg.example"}`},
		{name: "wrong host", body: `{"success":true,"action":"query","hostname":"other.example"}`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			v := verifierWith(t, func(r *http.Request) (*http.Response, error) {
				if r.URL.String() != siteverifyURL {
					t.Fatalf("url = %s", r.URL.String())
				}
				if r.Header.Get("Content-Type") != "application/x-www-form-urlencoded" {
					t.Fatalf("content-type = %s", r.Header.Get("Content-Type"))
				}
				payload, err := io.ReadAll(r.Body)
				if err != nil {
					t.Fatalf("read request: %v", err)
				}
				values, err := url.ParseQuery(string(payload))
				if err != nil {
					t.Fatalf("parse form: %v", err)
				}
				if values.Get("secret") != "secret" || values.Get("response") != "token" || values.Get("remoteip") != "203.0.113.5" {
					t.Fatalf("form = %#v", values)
				}
				return &http.Response{
					StatusCode: http.StatusOK,
					Body:       io.NopCloser(strings.NewReader(tc.body)),
					Header:     make(http.Header),
					Request:    r,
				}, nil
			})
			err := v.Verify(context.Background(), " token ", "203.0.113.5")
			if tc.ok && err != nil {
				t.Fatalf("verify: %v", err)
			}
			if !tc.ok && err == nil {
				t.Fatal("expected rejection")
			}
		})
	}
}

type errReader struct{}

func (errReader) Read([]byte) (int, error) { return 0, errors.New("read") }
