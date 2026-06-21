package config

import (
	"os"
	"testing"
)

func TestLoadFloodControlEnvOverride(t *testing.T) {
	content := `
server:
  mode: "server"
  port: 8080
database:
  path: "./test.db"
security:
  jwt_secret: "this-is-a-very-long-secret-key-for-testing"
flood_control:
  enabled: true
`
	tmpFile, err := os.CreateTemp("", "config-*.yaml")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(tmpFile.Name())
	tmpFile.WriteString(content)
	tmpFile.Close()

	t.Setenv("LG_FLOOD_CONTROL_ENABLED", "false")
	cfg, err := Load(tmpFile.Name())
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if cfg.FloodControl.Enabled {
		t.Error("expected flood control disabled via env override")
	}
}
