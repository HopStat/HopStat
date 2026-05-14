package config

import (
	"os"
	"testing"
)

func TestLoadValidConfig(t *testing.T) {
	content := `
server:
  host: "0.0.0.0"
  port: 8080
  mode: "server"
  as_number: "AS65000"
  default_route_as: "9121"
database:
  path: "./test.db"
security:
  jwt_secret: "this-is-a-very-long-secret-key-for-testing"
  credential_key: ""
  rate_limit_per_min: 10
audit:
  retention_days: 90
  async_write: true
query:
  max_concurrent: 50
  default_timeout_sec: 30
`
	tmpFile, err := os.CreateTemp("", "config-*.yaml")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(tmpFile.Name())
	tmpFile.WriteString(content)
	tmpFile.Close()

	cfg, err := Load(tmpFile.Name())
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if cfg.Server.Port != 8080 {
		t.Errorf("expected port 8080, got %d", cfg.Server.Port)
	}
	if !cfg.IsServer() {
		t.Error("expected server mode")
	}
	if cfg.IsAgent() {
		t.Error("should not be agent mode")
	}
}

func TestLoadMissingJWTSecret(t *testing.T) {
	content := `
server:
  mode: "server"
  port: 8080
database:
  path: "./test.db"
security:
  jwt_secret: ""
`
	tmpFile, err := os.CreateTemp("", "config-*.yaml")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(tmpFile.Name())
	tmpFile.WriteString(content)
	tmpFile.Close()

	_, err = Load(tmpFile.Name())
	if err == nil {
		t.Error("expected error for missing jwt_secret")
	}
}

func TestInvalidMode(t *testing.T) {
	content := `
server:
  mode: "invalid"
  port: 8080
database:
  path: "./test.db"
security:
  jwt_secret: "test"
`
	tmpFile, err := os.CreateTemp("", "config-*.yaml")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(tmpFile.Name())
	tmpFile.WriteString(content)
	tmpFile.Close()

	_, err = Load(tmpFile.Name())
	if err == nil {
		t.Error("expected error for invalid mode")
	}
}
