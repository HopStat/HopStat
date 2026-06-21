package domain

import (
	"encoding/json"
	"testing"
)

func TestFormatQueryAuditParams(t *testing.T) {
	tests := []struct {
		name      string
		command   string
		target    string
		protocol  string
		pingCount int
		maxHops   int
		want      string
	}{
		{"ping target only", "ping", "8.8.8.8", "", 5, 0, "8.8.8.8"},
		{"ping custom count", "ping", "8.8.8.8", "", 10, 0, "8.8.8.8 · count 10"},
		{"traceroute target only", "traceroute", "1.1.1.1", "", 0, 30, "1.1.1.1"},
		{"traceroute custom hops", "traceroute", "1.1.1.1", "", 0, 64, "1.1.1.1 · max hops 64"},
		{"bgp with protocol", "bgp_route", "8.8.8.0/24", "ipv4", 0, 0, "8.8.8.0/24 · ipv4"},
		{"empty target", "ping", "  ", "", 5, 0, "—"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := FormatQueryAuditParams(tt.command, tt.target, tt.protocol, tt.pingCount, tt.maxHops)
			if got != tt.want {
				t.Fatalf("FormatQueryAuditParams() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestDisplayAuditParams(t *testing.T) {
	tests := []struct {
		raw  string
		want string
	}{
		{`{"target":"8.8.8.8"}`, "8.8.8.8"},
		{"8.8.8.8", "8.8.8.8"},
		{"", "—"},
		{"  ", "—"},
		{`{"target":"1.1.1.1","ping_count":10}`, "1.1.1.1 · count 10"},
		{`{"Target":"8.8.4.4"}`, "8.8.4.4"},
		{`not json`, "not json"},
		{`{"target":"1.1.1.1","max_hops":64}`, "1.1.1.1 · max hops 64"},
		{`{"target":"8.8.8.8","protocol":"ipv4"}`, "8.8.8.8 · ipv4"},
		{`{"target":"8.8.8.8","custom":"value"}`, "8.8.8.8 · custom value"},
		{`{"target":"8.8.8.8","protocol":""}`, "8.8.8.8"},
		{`{"target":"8.8.8.8","ping_count":"bad"}`, "8.8.8.8 · ping_count bad"},
		{`{broken json`, `{broken json`},
		{`{broken "target":"9.9.9.9"`, "9.9.9.9"},
		{`{"ping_count":10}`, `{"ping_count":10}`},
		{`{"Target":"10.0.0.1"}`, "10.0.0.1"},
	}

	for _, tt := range tests {
		got := DisplayAuditParams(tt.raw)
		if got != tt.want {
			t.Fatalf("DisplayAuditParams(%q) = %q, want %q", tt.raw, got, tt.want)
		}
	}
}

func TestLegacyAuditHelpers(t *testing.T) {
	if got := legacyAuditTargetFromObject(map[string]any{"target": " 8.8.8.8 "}); got != "8.8.8.8" {
		t.Fatalf("target trim = %q", got)
	}
	if got := legacyAuditTargetFromObject(map[string]any{"Target": "1.1.1.1"}); got != "1.1.1.1" {
		t.Fatalf("Target key = %q", got)
	}
	if got := legacyAuditTargetFromObject(map[string]any{"other": "x"}); got != "" {
		t.Fatalf("missing target = %q", got)
	}
	if got := legacyAuditTargetFromJSON(`{"target":"9.9.9.9"}`); got != "9.9.9.9" {
		t.Fatalf("json regex = %q", got)
	}
	if got := legacyAuditTargetFromJSON(`no target here`); got != "" {
		t.Fatalf("no match = %q", got)
	}
	if got := formatLegacyAuditExtra("ping_count", float64(3)); got != "count 3" {
		t.Fatalf("ping_count = %q", got)
	}
	if got := formatLegacyAuditExtra("max_hops", int64(64)); got != "max hops 64" {
		t.Fatalf("max_hops = %q", got)
	}
	if got := formatLegacyAuditExtra("protocol", "ipv6"); got != "ipv6" {
		t.Fatalf("protocol = %q", got)
	}
	if got := formatLegacyAuditExtra("protocol", "  "); got != "" {
		t.Fatalf("blank protocol = %q", got)
	}
	if got := formatLegacyAuditExtra("foo", ""); got != "" {
		t.Fatalf("blank default = %q", got)
	}
	if got := formatLegacyAuditExtra("foo", 42); got != "foo 42" {
		t.Fatalf("default extra = %q", got)
	}
}

func TestAsInt(t *testing.T) {
	if n, ok := asInt(float64(5)); !ok || n != 5 {
		t.Fatalf("float64: n=%d ok=%v", n, ok)
	}
	if n, ok := asInt(int(7)); !ok || n != 7 {
		t.Fatalf("int: n=%d ok=%v", n, ok)
	}
	if n, ok := asInt(int64(9)); !ok || n != 9 {
		t.Fatalf("int64: n=%d ok=%v", n, ok)
	}
	if n, ok := asInt(json.Number("12")); !ok || n != 12 {
		t.Fatalf("json.Number: n=%d ok=%v", n, ok)
	}
	if _, ok := asInt(json.Number("nope")); ok {
		t.Fatal("invalid json.Number should fail")
	}
	if _, ok := asInt("string"); ok {
		t.Fatal("string should fail")
	}
}
