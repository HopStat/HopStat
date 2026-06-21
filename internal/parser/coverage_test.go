package parser

import (
	"testing"
)

func TestBirdParseVerboseRoute(t *testing.T) {
	p := &BirdParser{}
	raw := `8.8.8.0/24 via 10.0.0.1 [AS15169] (65000:100)`

	got, err := p.ParseBGPRoute(raw)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Routes) != 1 {
		t.Fatalf("routes = %d", len(got.Routes))
	}
	r := got.Routes[0]
	if r.NextHop != "10.0.0.1" || len(r.ASPath) != 1 || r.ASPath[0] != 15169 {
		t.Fatalf("route = %+v", r)
	}
	if len(r.Communities) != 1 || r.Communities[0] != "65000:100" {
		t.Fatalf("communities = %v", r.Communities)
	}
}

func TestBirdParsePingTraceroute(t *testing.T) {
	p := &BirdParser{}
	pingRaw := `PING 8.8.8.8 (8.8.8.8) 56(84) bytes of data.
--- 8.8.8.8 ping statistics ---
1 packets transmitted, 1 received, 0% packet loss`
	if _, err := p.ParsePing(pingRaw); err != nil {
		t.Fatalf("ParsePing: %v", err)
	}
	trRaw := `traceroute to 8.8.8.8 (8.8.8.8), 30 hops max
 1  10.0.0.1 (10.0.0.1)  1.000 ms`
	if _, err := p.ParseTraceroute(trRaw); err != nil {
		t.Fatalf("ParseTraceroute: %v", err)
	}
}

func TestNormalizeBGPOrigin(t *testing.T) {
	tests := []struct {
		in, want string
	}{
		{"I", "IGP"},
		{"E", "EGP"},
		{"?", "incomplete"},
		{"IGP", "IGP"},
		{"EGP", "EGP"},
		{"INCOMPLETE", "incomplete"},
		{"custom", "custom"},
	}
	for _, tc := range tests {
		if got := normalizeBGPOrigin(tc.in); got != tc.want {
			t.Errorf("normalizeBGPOrigin(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestCiscoParsePingTraceroute(t *testing.T) {
	p := &CiscoParser{}
	if _, err := p.ParsePing(`--- 8.8.8.8 ping statistics ---
1 packets transmitted, 1 received, 0% packet loss`); err != nil {
		t.Fatal(err)
	}
	if _, err := p.ParseTraceroute(` 1  10.0.0.1 (10.0.0.1)  1.000 ms`); err != nil {
		t.Fatal(err)
	}
}

func TestJuniperParser(t *testing.T) {
	p := &JuniperParser{}
	raw := `*> 192.168.1.0/24 to 10.0.0.1 IGP
no route found`
	got, err := p.ParseBGPRoute(raw)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Routes) != 0 {
		t.Fatalf("expected no routes for no-route output, got %d", len(got.Routes))
	}

	raw = `*> 10.0.0.0/24 to 10.0.0.1 IGP`
	got, err = p.ParseBGPRoute(raw)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Routes) != 1 || got.Routes[0].Prefix != "10.0.0.0/24" {
		t.Fatalf("routes = %+v", got.Routes)
	}
	if _, err := p.ParsePing(`1 packets transmitted, 1 received`); err != nil {
		t.Fatal(err)
	}
	if _, err := p.ParseTraceroute(` 1  10.0.0.1 (10.0.0.1)  1.000 ms`); err != nil {
		t.Fatal(err)
	}
}

func TestMikroTikParser(t *testing.T) {
	p := &MikroTikParser{}
	bgpRaw := `*> 10.0.0.0/24 10.0.0.1`
	if _, err := p.ParseBGPRoute(bgpRaw); err != nil {
		t.Fatal(err)
	}
	pingRaw := `seq=1 time=1.2 ms
min/avg/max = 1.0/1.5/2.0`
	got, err := p.ParsePing(pingRaw)
	if err != nil {
		t.Fatal(err)
	}
	if got.PacketsSent != 1 || got.PacketsRecv != 1 || got.MinRTT != 1.0 {
		t.Fatalf("ping = %+v", got)
	}
	if _, err := p.ParseTraceroute(` 1  10.0.0.1  1.000 ms`); err != nil {
		t.Fatal(err)
	}
}

func TestParseBGPLine(t *testing.T) {
	tests := []struct {
		line string
		want bool
	}{
		{"", false},
		{"not a route", false},
		{"*>192.168.1.0/24 10.0.0.1", true},
		{"*> invalid-prefix 10.0.0.1", false},
	}
	for _, tc := range tests {
		got := parseBGPLine(tc.line)
		if (got != nil) != tc.want {
			t.Errorf("parseBGPLine(%q) nil=%v want %v", tc.line, got == nil, tc.want)
		}
	}
}

func TestParseBirdTabularOrigin(t *testing.T) {
	p := &BirdParser{}
	raw := `* 10.0.0.0/24 10.0.0.1 100 65000 15169 I`
	got, err := p.ParseBGPRoute(raw)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Routes) != 1 || got.Routes[0].Origin != "IGP" {
		t.Fatalf("routes = %+v", got.Routes)
	}
}

func TestParseTracerouteLineHostOnly(t *testing.T) {
	hop := parseTracerouteLine(` 3  * * *`)
	if hop == nil || hop.Number != 3 {
		t.Fatalf("hop = %+v", hop)
	}
}

func TestGenericParsePingBusyBox(t *testing.T) {
	raw := `PING 8.8.8.8 (8.8.8.8): 56 data bytes
--- 8.8.8.8 ping statistics ---
5 packets transmitted, 3 packets received, 40% packet loss`
	got, err := parsePingGeneric(raw)
	if err != nil {
		t.Fatal(err)
	}
	if got.PacketsRecv != 3 {
		t.Fatalf("recv = %d", got.PacketsRecv)
	}
}

func TestParseBGPLineOriginFlag(t *testing.T) {
	route := parseBGPLine("*i 10.0.0.0/8 10.0.0.1 100 0 i")
	if route == nil || route.Prefix != "10.0.0.0/8" || route.Origin != "i" {
		t.Fatalf("route = %+v", route)
	}
}

func TestBirdContinuationRoute(t *testing.T) {
	p := &BirdParser{}
	raw := `* 10.0.0.0/24 10.0.0.1 100 65000 15169 I
                        172.16.0.1 100 6453 15169 I`
	got, err := p.ParseBGPRoute(raw)
	if err != nil || len(got.Routes) != 2 {
		t.Fatalf("routes = %+v err=%v", got.Routes, err)
	}
}

func TestBirdVerboseASParen(t *testing.T) {
	p := &BirdParser{}
	raw := `10.0.0.0/24 (AS65000) via 10.0.0.1`
	got, err := p.ParseBGPRoute(raw)
	if err != nil || len(got.Routes) != 1 {
		t.Fatalf("routes = %+v", got.Routes)
	}
}

func TestCiscoBGPWithASPath(t *testing.T) {
	p := &CiscoParser{}
	raw := `*> 10.0.0.0/24 10.0.0.1 100 0 65000 15169 i 65000:100`
	got, err := p.ParseBGPRoute(raw)
	if err != nil || len(got.Routes) != 1 {
		t.Fatalf("routes = %+v", got.Routes)
	}
	if len(got.Routes[0].ASPath) == 0 || len(got.Routes[0].Communities) == 0 {
		t.Fatalf("route = %+v", got.Routes[0])
	}
}


func TestGenericParsePingMikrotikLoss(t *testing.T) {
	raw := `seq=1 host=8.8.8.8 time=1.0 loss=50%`
	got, err := parsePingGeneric(raw)
	if err != nil {
		t.Fatal(err)
	}
	if got.AvgRTT != 1.0 {
		t.Fatalf("avg = %v", got.AvgRTT)
	}
}

func TestParseTracerouteLineMSStar(t *testing.T) {
	hop := parseTracerouteLine(` 2  10.0.0.1  1.234ms*`)
	if hop == nil || len(hop.RTT) != 1 {
		t.Fatalf("hop = %+v", hop)
	}
}


func TestBirdParseEmptyAndSkipLines(t *testing.T) {
	p := &BirdParser{}
	raw := `
* 10.0.0.0/24 10.0.0.1 100 65000 I
garbage-line`
	got, err := p.ParseBGPRoute(raw)
	if err != nil || len(got.Routes) != 1 {
		t.Fatalf("routes = %+v err=%v", got.Routes, err)
	}
}


func TestGenericParsePingPacketLossBranch(t *testing.T) {
	raw := `1 packets transmitted, 0 packets received, 100% packet loss`
	got, err := parsePingGeneric(raw)
	if err != nil || got.PacketLoss != 100 {
		t.Fatalf("loss=%v err=%v", got.PacketLoss, err)
	}
}

func TestJuniperIncompleteOrigin(t *testing.T) {
	p := &JuniperParser{}
	raw := `* 10.0.0.0/24 to 10.0.0.1 Incomplete`
	got, err := p.ParseBGPRoute(raw)
	if err != nil || len(got.Routes) != 1 || got.Routes[0].Origin != "Incomplete" {
		t.Fatalf("routes = %+v", got.Routes)
	}
}
