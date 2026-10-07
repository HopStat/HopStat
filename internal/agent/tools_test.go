package agent

import (
	"context"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/HopStat/HopStat/internal/target"
)

func prependFakeBin(t *testing.T, scripts map[string]string) {
	t.Helper()
	dir := t.TempDir()
	for name, script := range scripts {
		path := filepath.Join(dir, name)
		body := "#!/bin/sh\n" + script + "\n"
		if err := os.WriteFile(path, []byte(body), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
}

const samplePingOutput = `PING 8.8.8.8 (8.8.8.8) 56(84) bytes of data.
64 bytes from 8.8.8.8: icmp_seq=1 ttl=118 time=4.23 ms
--- 8.8.8.8 ping statistics ---
2 packets transmitted, 2 received, 0% packet loss, time 1002ms
rtt min/avg/max/mdev = 4.234/4.397/4.560/0.163 ms`

const sampleTracerouteOutput = `traceroute to 8.8.8.8 (8.8.8.8), 30 hops max, 60 byte packets
 1  192.168.1.1 (192.168.1.1)  0.534 ms  0.521 ms  0.507 ms
 2  8.8.8.8 (8.8.8.8)  4.321 ms  4.310 ms  4.298 ms`

const sampleBirdBGPOutput = `* 8.8.8.0/24              10.183.1.25                  100        204457 15169 I`

const sampleCiscoBGPOutput = `*> 192.168.1.0/24  10.0.0.1  100  0  i`

func TestIsBlockedIP(t *testing.T) {
	tests := []struct {
		ip   string
		want bool
	}{
		{"127.0.0.1", true},
		{"0.0.0.0", true},
		{"10.0.0.1", true},
		{"169.254.1.1", true},
		{"100.127.0.1", true},
		{"100.63.0.1", false},
		{"0.1.2.3", true},
		{"224.0.0.1", true},
		{"255.255.255.255", true},
		{"fe80::1", true},
		{"2001:4860:4860::8888", false},
		{"8.8.8.8", false},
	}
	for _, tc := range tests {
		if got := isBlockedIP(net.ParseIP(tc.ip)); got != tc.want {
			t.Errorf("isBlockedIP(%q) = %v, want %v", tc.ip, got, tc.want)
		}
	}
}

func TestIsValidTarget(t *testing.T) {
	if isValidTarget("127.0.0.1") {
		t.Fatal("loopback should be invalid")
	}
	if isValidTarget("8.8.8.8") {
		// ok
	} else {
		t.Fatal("public IP should be valid")
	}
	if isValidTarget("example.com") {
		// ok
	} else {
		t.Fatal("resolvable public hostname should be valid")
	}
	if isValidTarget("hopstat-invalid-host.example") {
		t.Fatal("unknown hostname should be invalid")
	}
	if isValidTarget("localhost") {
		t.Fatal("localhost should resolve to blocked address")
	}
	if isValidTarget("bad;host") {
		t.Fatal("unsafe chars should be invalid")
	}
	longName := strings.Repeat("a", 254)
	if isValidTarget(longName) {
		t.Fatal("overlong target should be invalid")
	}
}

func TestRunPingInvalidTarget(t *testing.T) {
	_, err := runPing(context.Background(), "127.0.0.1", 3)
	if err == nil {
		t.Fatal("expected error for blocked target")
	}
}

func TestRunPingSuccess(t *testing.T) {
	prependFakeBin(t, map[string]string{
		"ping": `cat <<'EOF'
` + samplePingOutput + `
EOF
exit 0`,
	})
	result, err := runPing(context.Background(), "8.8.8.8", 2)
	if err != nil {
		t.Fatalf("runPing error: %v", err)
	}
	if result.PacketsRecv != 2 {
		t.Fatalf("expected 2 received, got %d", result.PacketsRecv)
	}
}

func TestRunPingEmptyOutputError(t *testing.T) {
	prependFakeBin(t, map[string]string{
		"ping": "exit 1",
	})
	result, err := runPing(context.Background(), "8.8.8.8", 3)
	if err == nil {
		t.Fatal("expected error")
	}
	if result == nil || result.PacketLoss != 100 {
		t.Fatalf("expected loss result, got %#v", result)
	}
}

func TestRunPingParseableErrorOutput(t *testing.T) {
	prependFakeBin(t, map[string]string{
		"ping": `cat <<'EOF'
` + samplePingOutput + `
EOF
exit 1`,
	})
	result, err := runPing(context.Background(), "8.8.8.8", 2)
	if err != nil {
		t.Fatalf("expected parsed success despite exit code, got err=%v", err)
	}
	if result.PacketsRecv != 2 {
		t.Fatalf("expected 2 received, got %d", result.PacketsRecv)
	}
}

func TestRunPingUnparseableErrorOutput(t *testing.T) {
	prependFakeBin(t, map[string]string{
		"ping": `echo '%%%'; exit 1`,
	})
	result, err := runPing(context.Background(), "8.8.8.8", 4)
	if err != nil {
		t.Fatalf("expected nil error with fallback result, got %v", err)
	}
	if result.PacketLoss != 100 || result.PacketsSent != 4 {
		t.Fatalf("unexpected result: %#v", result)
	}
}

func TestRunTracerouteInvalidTarget(t *testing.T) {
	_, err := runTraceroute(context.Background(), "127.0.0.1", 10)
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestRunTracerouteSuccess(t *testing.T) {
	prependFakeBin(t, map[string]string{
		"traceroute": `cat <<'EOF'
` + sampleTracerouteOutput + `
EOF
exit 0`,
	})
	result, err := runTraceroute(context.Background(), "8.8.8.8", 30)
	if err != nil {
		t.Fatalf("runTraceroute error: %v", err)
	}
	if len(result.Hops) != 2 {
		t.Fatalf("expected 2 hops, got %d", len(result.Hops))
	}
}

func TestRunTracerouteEmptyOutputError(t *testing.T) {
	prependFakeBin(t, map[string]string{
		"traceroute": "exit 1",
	})
	result, err := runTraceroute(context.Background(), "8.8.8.8", 5)
	if err == nil {
		t.Fatal("expected error")
	}
	if result == nil {
		t.Fatal("expected result wrapper")
	}
}

func TestRunBGPRouteBird(t *testing.T) {
	prependFakeBin(t, map[string]string{
		"birdc": `cat <<'EOF'
` + sampleBirdBGPOutput + `
EOF
exit 0`,
		"vtysh": "exit 1",
	})
	result, err := runBGPRoute(context.Background(), "8.8.8.8")
	if err != nil {
		t.Fatalf("runBGPRoute error: %v", err)
	}
	if len(result.Routes) == 0 {
		t.Fatal("expected bird routes")
	}
}

func TestRunBGPRouteVtysh(t *testing.T) {
	prependFakeBin(t, map[string]string{
		"birdc": "exit 1",
		"vtysh": `cat <<'EOF'
` + sampleCiscoBGPOutput + `
EOF
exit 0`,
	})
	result, err := runBGPRoute(context.Background(), "192.168.1.0/24")
	if err != nil {
		t.Fatalf("runBGPRoute error: %v", err)
	}
	if len(result.Routes) == 0 {
		t.Fatal("expected cisco routes")
	}
}

func TestRunBGPRouteNoData(t *testing.T) {
	prependFakeBin(t, map[string]string{
		"birdc": "exit 1",
		"vtysh": "exit 1",
	})
	result, err := runBGPRoute(context.Background(), "1.1.1.1")
	if err != nil {
		t.Fatalf("runBGPRoute error: %v", err)
	}
	if !strings.Contains(result.Raw, "no BGP data") {
		t.Fatalf("unexpected raw: %q", result.Raw)
	}
}

func TestRunPingZeroParsedStats(t *testing.T) {
	prependFakeBin(t, map[string]string{
		"ping": `cat <<'EOF'
--- 8.8.8.8 ping statistics ---
0 packets transmitted, 0 received, 0% packet loss
EOF
exit 1`,
	})
	result, err := runPing(context.Background(), "8.8.8.8", 9)
	if err != nil {
		t.Fatalf("expected parsed fallback without error, got %v", err)
	}
	if result.PacketsSent != 9 || result.PacketLoss != 100 {
		t.Fatalf("unexpected result: %#v", result)
	}
}

func TestRunBGPRouteInvalidPrefix(t *testing.T) {
	_, err := runBGPRoute(context.Background(), "127.0.0.1")
	if err == nil {
		t.Fatal("expected blocked target error")
	}
}

// Regression: isBlockedIP used to let 255.255.255.255 through. IsMulticast covers
// only 224.0.0.0/4, so the broadcast address fell through every branch and runPing
// would have exec'd ping against it.
func TestIsBlockedIPRejectsLimitedBroadcast(t *testing.T) {
	if !isBlockedIP(net.ParseIP("255.255.255.255")) {
		t.Fatal("isBlockedIP(255.255.255.255) = false; the limited broadcast address must be blocked")
	}
	if isValidTarget("255.255.255.255") {
		t.Fatal("isValidTarget accepted 255.255.255.255")
	}
	for _, s := range []string{"8.8.8.8", "1.1.1.1"} {
		if isBlockedIP(net.ParseIP(s)) {
			t.Errorf("isBlockedIP(%q) = true; public addresses must stay dialable", s)
		}
	}
}

// The agent's blocklist and the server's must agree on every address. They gate
// the same commands, so a drift means one path accepts a target the other
// rejects. This pins that contract across the address families both must
// handle, including the v4-mapped forms where the two implementations could
// most easily diverge.
func TestIsBlockedIPMatchesTargetBlocklist(t *testing.T) {
	addresses := []string{
		"127.0.0.1", "::1", "0.0.0.0", "::",
		"10.0.0.1", "172.16.0.1", "192.168.1.1", "fd00::1",
		"169.254.1.1", "fe80::1", "::ffff:169.254.1.1",
		"224.0.0.1", "ff02::1",
		"100.64.0.1", "100.127.255.254", "100.63.0.1", "100.128.0.1",
		"0.1.2.3", "255.255.255.255",
		"8.8.8.8", "1.1.1.1", "93.184.216.34", "2001:4860:4860::8888",
		"::ffff:8.8.8.8",
	}
	for _, s := range addresses {
		ip := net.ParseIP(s)
		if ip == nil {
			t.Fatalf("unparsable address %q", s)
		}
		if got, want := isBlockedIP(ip), target.IsBlockedIP(ip); got != want {
			t.Errorf("isBlockedIP(%q) = %v but target.IsBlockedIP = %v; the two blocklists must not drift", s, got, want)
		}
	}
}
