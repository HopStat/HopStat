package parser

import "testing"

func TestBirdTabularNoNextHop(t *testing.T) {
	route := parseBirdTabularFields([]string{"65000", "15169", "I"}, "10.0.0.0/24", true)
	if route == nil || len(route.ASPath) != 2 {
		t.Fatalf("route=%+v", route)
	}
}

func TestBirdVerboseNoMatch(t *testing.T) {
	if route := parseBirdVerboseRoute("just text", "10.0.0.0/24", false); route != nil {
		t.Fatalf("route=%+v", route)
	}
}

func TestCiscoBGPNoASPath(t *testing.T) {
	p := &CiscoParser{}
	got, err := p.ParseBGPRoute(`*> 10.0.0.0/24 10.0.0.1 100 0 i`)
	if err != nil || len(got.Routes) != 1 {
		t.Fatalf("routes=%+v err=%v", got.Routes, err)
	}
}

func TestGenericBGPMultiline(t *testing.T) {
	got, err := parseBGPRouteGeneric(`*>192.168.1.0/24 10.0.0.1
*>192.168.2.0/24 10.0.0.2`)
	if err != nil || len(got.Routes) != 2 {
		t.Fatalf("routes=%d err=%v", len(got.Routes), err)
	}
}

func TestParseTracerouteGenericHeaderSkip(t *testing.T) {
	got, err := parseTracerouteGeneric(`traceroute to 8.8.8.8 (8.8.8.8)
 1  10.0.0.1 (10.0.0.1)  1.000 ms`)
	if err != nil || len(got.Hops) != 1 {
		t.Fatalf("hops=%d err=%v", len(got.Hops), err)
	}
}

func TestParseBGPLineOriginE(t *testing.T) {
	route := parseBGPLine("*e 10.0.0.0/8 10.0.0.1 100 0 e")
	if route == nil || route.Origin != "e" {
		t.Fatalf("route=%+v", route)
	}
}

func TestMikroTikComputedLoss(t *testing.T) {
	p := &MikroTikParser{}
	got, err := p.ParsePing(`seq=1 no reply
seq=2 time=1.0 ms`)
	if err != nil || got.PacketLoss != 50 {
		t.Fatalf("ping=%+v", got)
	}
}

func TestMikroTikParsePingAllBranches(t *testing.T) {
	p := &MikroTikParser{}
	got, err := p.ParsePing(`seq=1 time=1.0 ms
1 packets transmitted, 1 received, 0% loss
min/avg/max = 1.0/1.5/2.0`)
	if err != nil || got.MinRTT != 1.0 || got.AvgRTT != 1.5 || got.MaxRTT != 2.0 {
		t.Fatalf("ping=%+v err=%v", got, err)
	}
}

func TestBirdParseNoRoute(t *testing.T) {
	p := &BirdParser{}
	got, err := p.ParseBGPRoute("no route found")
	if err != nil || len(got.Routes) != 0 {
		t.Fatalf("routes=%+v err=%v", got.Routes, err)
	}
}

func TestBirdParseBestFlagAndContinuation(t *testing.T) {
	p := &BirdParser{}
	got, err := p.ParseBGPRoute(`*> 10.0.0.0/24 10.0.0.1 100 65000 I
                        10.0.0.2 100 15169 I`)
	if err != nil || len(got.Routes) != 2 {
		t.Fatalf("routes=%+v err=%v", got.Routes, err)
	}
}

func TestBirdTabularLocalPref(t *testing.T) {
	route := parseBirdTabularFields([]string{"10.0.0.1", "100", "65000", "15169", "I"}, "10.0.0.0/24", true)
	if route == nil || route.LocalPref != 100 {
		t.Fatalf("route=%+v", route)
	}
}

func TestBirdVerboseCommunitiesOnly(t *testing.T) {
	route := parseBirdVerboseRoute("10.0.0.0/24 via 10.0.0.1 (65000:100)", "10.0.0.0/24", false)
	if route == nil || len(route.Communities) != 1 {
		t.Fatalf("route=%+v", route)
	}
}

func TestCiscoParseNoRoute(t *testing.T) {
	p := &CiscoParser{}
	got, err := p.ParseBGPRoute("Network not in table")
	if err != nil || len(got.Routes) != 0 {
		t.Fatalf("routes=%+v err=%v", got.Routes, err)
	}
}

func TestJuniperParseNoRoute(t *testing.T) {
	p := &JuniperParser{}
	got, err := p.ParseBGPRoute("inet.0: no route found")
	if err != nil || len(got.Routes) != 0 {
		t.Fatalf("routes=%+v err=%v", got.Routes, err)
	}
}

func TestParseBGPLineQuestionOrigin(t *testing.T) {
	route := parseBGPLine("*? 10.0.0.0/8 10.0.0.1 100 0 ?")
	if route == nil || route.Origin != "?" {
		t.Fatalf("route=%+v", route)
	}
}

func TestParsePingGenericSeqBranch(t *testing.T) {
	got, err := parsePingGeneric(`seq=1 time=2.5 ms`)
	if err != nil || got.AvgRTT != 2.5 {
		t.Fatalf("ping=%+v err=%v", got, err)
	}
}

func TestParseTracerouteGenericEmptyLine(t *testing.T) {
	got, err := parseTracerouteGeneric(" 1  10.0.0.1  1.000 ms\n\n")
	if err != nil || len(got.Hops) != 1 {
		t.Fatalf("hops=%d err=%v", len(got.Hops), err)
	}
}

func TestParseBirdRouteLineEmptyFields(t *testing.T) {
	if route, prefix := parseBirdRouteLine("   ", "10.0.0.0/24"); route != nil || prefix != "10.0.0.0/24" {
		t.Fatalf("route=%+v prefix=%q", route, prefix)
	}
	if route, _ := parseBirdRouteLine("no-prefix", ""); route != nil {
		t.Fatal("expected nil without prefix")
	}
	if route, _ := parseBirdRouteLine("10.0.0.0/24 10.0.0.1 65000 I", ""); route == nil {
		t.Fatal("expected tabular route")
	}
}

func TestParseBirdRouteLineFieldStartBounds(t *testing.T) {
	if route, _ := parseBirdRouteLine("10.0.0.0/24 10.0.0.1 65000 I", "10.0.0.0/24"); route == nil {
		t.Fatal("expected route with duplicate prefix")
	}
}

func TestParseBirdVerboseRouteEmptyRoute(t *testing.T) {
	if route := parseBirdVerboseRoute("10.0.0.0/24 via 10.0.0.1", "10.0.0.0/24", false); route == nil {
		t.Fatal("expected verbose route")
	}
	if route := parseBirdVerboseRoute("10.0.0.0/24 unrelated", "10.0.0.0/24", false); route != nil {
		t.Fatalf("route=%+v", route)
	}
}

func TestParseBirdTabularEmptyFields(t *testing.T) {
	if route := parseBirdTabularFields(nil, "10.0.0.0/24", false); route != nil {
		t.Fatal("expected nil")
	}
}

func TestParseBGPLineBranches(t *testing.T) {
	if route := parseBGPLine("not-a-route"); route != nil {
		t.Fatal("expected nil")
	}
	if route := parseBGPLine("*i 10.0.0.0/8 10.0.0.1 i"); route == nil || route.Prefix != "10.0.0.0/8" {
		t.Fatalf("route=%+v", route)
	}
	if route := parseBGPLine("*i"); route != nil {
		t.Fatal("expected nil for short origin line")
	}
}

func TestCiscoBGPNoRegexMatch(t *testing.T) {
	p := &CiscoParser{}
	got, err := p.ParseBGPRoute("unrelated line without route data")
	if err != nil || len(got.Routes) != 0 {
		t.Fatalf("routes=%+v err=%v", got.Routes, err)
	}
}

func TestJuniperBGPSkipAndOrigin(t *testing.T) {
	p := &JuniperParser{}
	got, err := p.ParseBGPRoute(`header line
* 10.0.0.0/24 to 10.0.0.1 IGP
 short`)
	if err != nil || len(got.Routes) != 1 || got.Routes[0].Origin != "IGP" {
		t.Fatalf("routes=%+v err=%v", got.Routes, err)
	}
}

func TestParsePingGenericMikrotikLossBranch(t *testing.T) {
	got, err := parsePingGeneric("seq=1 time=1.0 ms loss= 25%")
	if err != nil || got.PacketLoss != 25 {
		t.Fatalf("loss=%v err=%v", got.PacketLoss, err)
	}
}

func TestParseTracerouteGenericTracerouteHeader(t *testing.T) {
	got, err := parseTracerouteGeneric("Traceroute to host\n 1  10.0.0.1  1.000 ms")
	if err != nil || len(got.Hops) != 1 {
		t.Fatalf("hops=%d err=%v", len(got.Hops), err)
	}
}

func TestParseTracerouteLineNoFields(t *testing.T) {
	if hop := parseTracerouteLine("1"); hop != nil {
		t.Fatalf("hop=%+v", hop)
	}
}

func TestMikroTikParsePingTransmittedReceived(t *testing.T) {
	p := &MikroTikParser{}
	got, err := p.ParsePing("5 packets transmitted, 4 packets received, 20% loss")
	if err != nil {
		t.Fatal(err)
	}
	if got.PacketLoss != 20 {
		t.Fatalf("ping=%+v", got)
	}
}

func TestParseBirdRouteLineNoRouteFields(t *testing.T) {
	if route, prefix := parseBirdRouteLine("* 10.0.0.0/24 10.0.0.1 100 65000 I", ""); route == nil || prefix != "10.0.0.0/24" {
		t.Fatalf("route=%+v prefix=%q", route, prefix)
	}
}

func TestParseBirdRouteLineBestMarkers(t *testing.T) {
	if route, _ := parseBirdRouteLine("*> 10.0.0.0/24 10.0.0.1 100 65000 I", ""); route == nil || !route.Best {
		t.Fatalf("route=%+v", route)
	}
}

func TestParsePingGenericBusyBoxPacketLoss(t *testing.T) {
	got, err := parsePingGeneric("5 packets transmitted, 3 packets received, 40% packet loss")
	if err != nil || got.PacketLoss != 40 {
		t.Fatalf("loss=%v err=%v", got.PacketLoss, err)
	}
}

func TestParsePingGenericRTTWithMdev(t *testing.T) {
	got, err := parsePingGeneric("rtt min/avg/max/mdev = 1.0/2.0/3.0/0.5 ms")
	if err != nil || got.MinRTT != 1.0 || got.MaxRTT != 3.0 {
		t.Fatalf("ping=%+v err=%v", got, err)
	}
}

func TestParseTracerouteLineHostInParens(t *testing.T) {
	hop := parseTracerouteLine(` 1  gw (10.0.0.1)  1.000 ms`)
	if hop == nil || hop.IP != "10.0.0.1" {
		t.Fatalf("hop=%+v", hop)
	}
}

func TestContainsNoRouteVariants(t *testing.T) {
	if !containsNoRoute("Network not in table") {
		t.Fatal("expected match")
	}
}

func TestParseBirdRouteLineNoFieldsAfterMarkers(t *testing.T) {
	if route, _ := parseBirdRouteLine("*", "10.0.0.0/24"); route != nil {
		t.Fatalf("route=%+v", route)
	}
}

func TestParseBirdRouteLineFieldStartAtEnd(t *testing.T) {
	if route, _ := parseBirdRouteLine("10.0.0.0/24", "10.0.0.0/24"); route != nil {
		t.Fatalf("route=%+v", route)
	}
}

func TestParseBirdVerboseRouteReturnsNil(t *testing.T) {
	if route := parseBirdVerboseRoute("10.0.0.0/24 plain text only", "10.0.0.0/24", false); route != nil {
		t.Fatalf("route=%+v", route)
	}
}

func TestParseBGPLineShortOriginPrefix(t *testing.T) {
	if route := parseBGPLine("*i"); route != nil {
		t.Fatal("expected nil")
	}
}

func TestParseTracerouteGenericSkipsBlankLines(t *testing.T) {
	got, err := parseTracerouteGeneric(" 1  10.0.0.1  1.000 ms\n\n 2  10.0.0.2  2.000 ms")
	if err != nil || len(got.Hops) != 2 {
		t.Fatalf("hops=%d err=%v", len(got.Hops), err)
	}
}

func TestParseTracerouteGenericSkipsOnlyWhitespaceLine(t *testing.T) {
	got, err := parseTracerouteGeneric(" 1  10.0.0.1  1.000 ms\n   \n")
	if err != nil || len(got.Hops) != 1 {
		t.Fatalf("hops=%d err=%v", len(got.Hops), err)
	}
}

func TestJuniperParseBGPSkipShortLine(t *testing.T) {
	p := &JuniperParser{}
	got, err := p.ParseBGPRoute("*")
	if err != nil || len(got.Routes) != 0 {
		t.Fatalf("routes=%+v err=%v", got.Routes, err)
	}
}

func TestParseBirdVerboseRouteEmptyFields(t *testing.T) {
	if route := parseBirdVerboseRoute("10.0.0.0/24 via ", "10.0.0.0/24", false); route != nil {
		t.Fatalf("route=%+v", route)
	}
}

func TestParseBGPLineShortOriginTwoFields(t *testing.T) {
	if route := parseBGPLine("*i 10.0.0.0/8"); route != nil {
		t.Fatalf("route=%+v", route)
	}
}

func TestParseTracerouteGenericHopNilSkipped(t *testing.T) {
	got, err := parseTracerouteGeneric("1\n 2  10.0.0.2  2.000 ms")
	if err != nil || len(got.Hops) != 1 || got.Hops[0].Number != 2 {
		t.Fatalf("hops=%+v err=%v", got.Hops, err)
	}
}

func TestMikroTikParsePingPacketCounts(t *testing.T) {
	p := &MikroTikParser{}
	got, err := p.ParsePing("summary packets transmitted count 5 transmitted 4 received 20% loss")
	if err != nil {
		t.Fatal(err)
	}
	if got.PacketsSent != 5 || got.PacketsRecv != 4 || got.PacketLoss != 20 {
		t.Fatalf("ping=%+v", got)
	}
}
