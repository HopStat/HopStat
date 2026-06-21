package lgnode

import (
	"net"
	"testing"
)

func TestResolveLocalAgentURL(t *testing.T) {
	localIP := ""
	ifaces, err := net.InterfaceAddrs()
	if err == nil {
		for _, a := range ifaces {
			ipnet, ok := a.(*net.IPNet)
			if !ok || ipnet.IP.IsLoopback() {
				continue
			}
			if v4 := ipnet.IP.To4(); v4 != nil {
				localIP = v4.String()
				break
			}
		}
	}

	tests := []struct {
		in   string
		want string
	}{
		{"http://localhost:8080", "http://localhost:8080"},
		{"http://127.0.0.1:8080", "http://127.0.0.1:8080"},
	}
	if localIP != "" {
		tests = append(tests, struct {
			in   string
			want string
		}{"http://" + localIP + ":8080", "http://127.0.0.1:8080"})
	}

	for _, tt := range tests {
		got := resolveLocalAgentURL(tt.in)
		if got != tt.want {
			t.Errorf("resolveLocalAgentURL(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}
