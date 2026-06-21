package target

import (
	"context"
	"testing"
)

func TestValidAgentURL(t *testing.T) {
	tests := []struct {
		raw  string
		want bool
	}{
		{"", true},
		{"http://8.8.8.8:9090", true},
		{"http://127.0.0.1:9090", true},
		{"http://localhost:9090", true},
		{"ftp://agent.example.com", false},
		{"http://", false},
		{"http://169.254.169.254/latest/meta-data/", false},
		{"http://10.0.0.5:9090", false},
		{"http://192.168.1.10:9090", false},
	}

	for _, tc := range tests {
		if got := ValidAgentURL(tc.raw); got != tc.want {
			t.Errorf("ValidAgentURL(%q) = %v, want %v", tc.raw, got, tc.want)
		}
	}
}

func TestAllowedAgentHostBlocksMetadata(t *testing.T) {
	if AllowedAgentHost(context.Background(), "169.254.169.254") {
		t.Fatal("expected metadata IP to be blocked")
	}
}
