package parser

import (
	"testing"

	"github.com/HopStat/HopStat/internal/domain"
)

func TestGetParser(t *testing.T) {
	tests := []struct {
		vendor   string
		expected string
	}{
		{"cisco", "*parser.CiscoParser"},
		{"juniper", "*parser.JuniperParser"},
		{"mikrotik", "*parser.MikroTikParser"},
		{"bird", "*parser.BirdParser"},
		{"generic", "*parser.GenericParser"},
		{"unknown", "*parser.GenericParser"},
		{"", "*parser.GenericParser"},
	}

	for _, tt := range tests {
		p := GetParser(tt.vendor)
		if p == nil {
			t.Errorf("GetParser(%q) returned nil", tt.vendor)
		}
	}
}

func TestGenericParsePing(t *testing.T) {
	p := &GenericParser{}
	raw := `PING 8.8.8.8 (8.8.8.8) 56(84) bytes of data.
64 bytes from 8.8.8.8: icmp_seq=1 ttl=118 time=4.23 ms
64 bytes from 8.8.8.8: icmp_seq=2 ttl=118 time=4.56 ms
--- 8.8.8.8 ping statistics ---
2 packets transmitted, 2 received, 0% packet loss, time 1002ms
rtt min/avg/max/mdev = 4.234/4.397/4.560/0.163 ms`

	result, err := p.ParsePing(raw)
	if err != nil {
		t.Fatalf("ParsePing error: %v", err)
	}
	if result.PacketsSent != 2 {
		t.Errorf("expected 2 sent, got %d", result.PacketsSent)
	}
	if result.PacketsRecv != 2 {
		t.Errorf("expected 2 received, got %d", result.PacketsRecv)
	}
	if result.PacketLoss != 0 {
		t.Errorf("expected 0%% loss, got %.1f%%", result.PacketLoss)
	}
	if result.MinRTT < 4.0 || result.MinRTT > 5.0 {
		t.Errorf("expected min RTT ~4.2, got %.2f", result.MinRTT)
	}
}

func TestGenericParsePingTotalLoss(t *testing.T) {
	p := &GenericParser{}
	raw := `PING 10.255.255.1 (10.255.255.1) 56(84) bytes of data.

--- 10.255.255.1 ping statistics ---
5 packets transmitted, 0 received, 100% packet loss, time 4094ms`

	result, err := p.ParsePing(raw)
	if err != nil {
		t.Fatalf("ParsePing error: %v", err)
	}
	if result.PacketsSent != 5 {
		t.Errorf("expected 5 sent, got %d", result.PacketsSent)
	}
	if result.PacketsRecv != 0 {
		t.Errorf("expected 0 received, got %d", result.PacketsRecv)
	}
	if result.PacketLoss != 100 {
		t.Errorf("expected 100%% loss, got %.1f%%", result.PacketLoss)
	}
	if result.Raw == "" {
		t.Error("expected raw output preserved")
	}
}

func TestGenericParsePingLoss(t *testing.T) {
	p := &GenericParser{}
	raw := `PING 10.0.0.1 (10.0.0.1) 56(84) bytes of data.

--- 10.0.0.1 ping statistics ---
5 packets transmitted, 3 received, 40% packet loss, time 4003ms
rtt min/avg/max/mdev = 1.234/2.345/3.456/1.111 ms`

	result, err := p.ParsePing(raw)
	if err != nil {
		t.Fatalf("ParsePing error: %v", err)
	}
	if result.PacketsSent != 5 {
		t.Errorf("expected 5 sent, got %d", result.PacketsSent)
	}
	if result.PacketsRecv != 3 {
		t.Errorf("expected 3 received, got %d", result.PacketsRecv)
	}
	if result.PacketLoss != 40 {
		t.Errorf("expected 40%% loss, got %.1f%%", result.PacketLoss)
	}
}

func TestGenericParseTraceroute(t *testing.T) {
	p := &GenericParser{}
	raw := `traceroute to 8.8.8.8 (8.8.8.8), 30 hops max, 60 byte packets
 1  192.168.1.1 (192.168.1.1)  0.534 ms  0.521 ms  0.507 ms
 2  10.0.0.1 (10.0.0.1)  1.234 ms  1.222 ms  1.210 ms
 3  8.8.8.8 (8.8.8.8)  4.321 ms  4.310 ms  4.298 ms`

	result, err := p.ParseTraceroute(raw)
	if err != nil {
		t.Fatalf("ParseTraceroute error: %v", err)
	}
	if len(result.Hops) != 3 {
		t.Fatalf("expected 3 hops, got %d", len(result.Hops))
	}
	if result.Hops[0].IP != "192.168.1.1" {
		t.Errorf("hop 1 IP mismatch: %s", result.Hops[0].IP)
	}
}

func TestGenericParseBGPNoRoute(t *testing.T) {
	p := &GenericParser{}
	raw := `Network not in table`

	result, err := p.ParseBGPRoute(raw)
	if err != nil {
		t.Fatalf("ParseBGPRoute error: %v", err)
	}
	if len(result.Routes) != 0 {
		t.Errorf("expected no routes, got %d", len(result.Routes))
	}
}

func TestContainsNoRoute(t *testing.T) {
	tests := []struct {
		input    string
		expected bool
	}{
		{"no route found", true},
		{"Network not in table", true},
		{"% Network not in table", true},
		{"*> 192.168.1.0/24 10.0.0.1", false},
	}
	for _, tt := range tests {
		if got := containsNoRoute(tt.input); got != tt.expected {
			t.Errorf("containsNoRoute(%q) = %v, want %v", tt.input, got, tt.expected)
		}
	}
}

func TestBirdParseBGPTabularMultipath(t *testing.T) {
	p := &BirdParser{}
	raw := `* 8.8.8.0/24              10.183.1.25                  100        204457 15169 I
                          172.16.16.33                 100        44901 15169 I`

	result, err := p.ParseBGPRoute(raw)
	if err != nil {
		t.Fatalf("ParseBGPRoute error: %v", err)
	}
	if len(result.Routes) != 2 {
		t.Fatalf("expected 2 routes, got %d", len(result.Routes))
	}
	first := result.Routes[0]
	second := result.Routes[1]
	if !first.Best {
		t.Fatal("expected first route to be marked best")
	}
	if second.Best {
		t.Fatal("expected second route not best")
	}
	if first.NextHop != "10.183.1.25" || first.LocalPref != 100 {
		t.Fatalf("first route = %+v", first)
	}
	if len(first.ASPath) != 2 || first.ASPath[0] != 204457 || first.ASPath[1] != 15169 {
		t.Fatalf("first as_path = %v", first.ASPath)
	}
	if second.NextHop != "172.16.16.33" || len(second.ASPath) != 2 || second.ASPath[0] != 44901 {
		t.Fatalf("second route = %+v", second)
	}
}

func TestCiscoParseBGP(t *testing.T) {
	p := &CiscoParser{}
	raw := `*> 192.168.1.0/24  10.0.0.1  100  0  i`

	result, err := p.ParseBGPRoute(raw)
	if err != nil {
		t.Fatalf("ParseBGPRoute error: %v", err)
	}
	if len(result.Routes) < 1 {
		t.Fatalf("expected at least 1 route, got %d", len(result.Routes))
	}
	r := result.Routes[0]
	if r.Prefix != "192.168.1.0/24" {
		t.Errorf("prefix: %s", r.Prefix)
	}
	if r.NextHop != "10.0.0.1" {
		t.Errorf("nexthop: %s", r.NextHop)
	}
}

func TestOutputParserInterface(t *testing.T) {
	var _ OutputParser = &GenericParser{}
	var _ OutputParser = &CiscoParser{}
	var _ OutputParser = &JuniperParser{}
	var _ OutputParser = &MikroTikParser{}
	var _ OutputParser = &BirdParser{}
}

func TestDomainTypes(t *testing.T) {
	if domain.CmdPing != "ping" {
		t.Error("CmdPing mismatch")
	}
	if domain.CmdTraceroute != "traceroute" {
		t.Error("CmdTraceroute mismatch")
	}
	if domain.CmdBGPRoute != "bgp_route" {
		t.Error("CmdBGPRoute mismatch")
	}
}

// A BGP table marks the selected path with "*" and prints every other path indented, so
// the indentation is part of the route syntax. parseBGPLine trims the line before
// inspecting it, which made the indented-line branch unreachable and silently dropped
// every non-selected route.
func TestParseBGPLineKeepsNonSelectedRoutes(t *testing.T) {
	tests := []struct {
		name       string
		line       string
		wantPrefix string
		wantNil    bool
	}{
		{name: "selected path", line: "*>10.0.0.0/24 10.0.0.1 100 0 65001 i", wantPrefix: "10.0.0.0/24"},
		{name: "selected path with origin marker", line: "*e 10.0.0.0/8 10.0.0.1 100 0 e", wantPrefix: "10.0.0.0/8"},
		{name: "non-selected path", line: "   192.168.0.0/24 10.0.0.2 100 0 65002 i", wantPrefix: "192.168.0.0/24"},
		{name: "non-selected path tab indented", line: "\t198.51.100.0/24 10.0.0.3 100 0 65003 i", wantPrefix: "198.51.100.0/24"},
		{name: "blank line", line: "   ", wantNil: true},
		{name: "unmarked non-route header", line: "BGP table version is 12", wantNil: true},
		{name: "indented non-route header", line: "   Network          Next Hop", wantNil: true},
		{name: "route without a next hop", line: "*>10.0.0.0/24", wantNil: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parseBGPLine(tt.line)
			if tt.wantNil {
				if got != nil {
					t.Fatalf("parseBGPLine(%q) = %+v, want nil", tt.line, got)
				}
				return
			}
			if got == nil {
				t.Fatalf("parseBGPLine(%q) = nil, want prefix %q", tt.line, tt.wantPrefix)
			}
			if got.Prefix != tt.wantPrefix {
				t.Fatalf("parseBGPLine(%q).Prefix = %q, want %q", tt.line, got.Prefix, tt.wantPrefix)
			}
		})
	}
}

// Through the package entry point: a table with one selected and one non-selected route
// must yield both.
func TestParseBGPRouteGenericKeepsNonSelectedRoutes(t *testing.T) {
	got, err := parseBGPRouteGeneric(`BGP table version is 12
   192.168.0.0/24       10.0.0.2               0      100      0 65002 i
*>10.0.0.0/24          10.0.0.1               0      100      0 65001 i
`)
	if err != nil {
		t.Fatalf("ParseBGPRoute: %v", err)
	}
	if len(got.Routes) != 2 {
		t.Fatalf("routes = %+v, want both the selected and the non-selected route", got.Routes)
	}
}
