package config

import (
	"crypto/rand"
	"errors"
	"os"
	"strings"
	"testing"
)

func TestLoadValidConfig(t *testing.T) {
	content := `
server:
  host: "0.0.0.0"
  port: 8080
  mode: "server"
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
	if !cfg.FloodControl.Enabled {
		t.Error("expected flood control enabled by default")
	}
	if cfg.FloodControl.HTTPRateLimitPerMin != 10 {
		t.Errorf("expected http rate limit 10, got %d", cfg.FloodControl.HTTPRateLimitPerMin)
	}
}

func TestLoadFloodControlDisabled(t *testing.T) {
	content := `
server:
  mode: "server"
  port: 8080
database:
  path: "./test.db"
security:
  jwt_secret: "this-is-a-very-long-secret-key-for-testing"
flood_control:
  enabled: false
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
	if cfg.FloodControl.Enabled {
		t.Error("expected flood control disabled")
	}
}

func TestLoadLegacySecurityFloodLimits(t *testing.T) {
	content := `
server:
  mode: "server"
  port: 8080
database:
  path: "./test.db"
security:
  jwt_secret: "this-is-a-very-long-secret-key-for-testing"
  rate_limit_per_min: 25
  brute_force_max: 7
  brute_force_ban_min: 20
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
	if cfg.FloodControl.HTTPRateLimitPerMin != 25 {
		t.Errorf("expected legacy http limit 25, got %d", cfg.FloodControl.HTTPRateLimitPerMin)
	}
	if cfg.FloodControl.BruteForceMax != 7 {
		t.Errorf("expected legacy brute force max 7, got %d", cfg.FloodControl.BruteForceMax)
	}
}

func TestGenerateServerConfigIncludesFloodControl(t *testing.T) {
	tmpFile, err := os.CreateTemp("", "config-*.yaml")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(tmpFile.Name())
	tmpFile.Close()

	if err := Generate(tmpFile.Name(), "server"); err != nil {
		t.Fatalf("Generate: %v", err)
	}

	data, err := os.ReadFile(tmpFile.Name())
	if err != nil {
		t.Fatal(err)
	}
	content := string(data)
	for _, want := range []string{
		"flood_control:",
		"enabled: true",
		"http_rate_limit_per_min: 100",
		"query_rate_limit_per_min: 100",
		"brute_force_max: 5",
		"brute_force_ban_min: 15",
		"behind_cloudflare: false",
		"trusted_proxies: []",
		"geoip:",
		"update_interval: \"72h\"",
		"bgp:",
		"local_as: 0",
		"listen_port: 11790",
		"listen_addresses: []",
		"tls_cert:",
		"autocert_domain:",
	} {
		if !strings.Contains(content, want) {
			t.Errorf("generated config missing %q", want)
		}
	}

	cfg, err := Load(tmpFile.Name())
	if err != nil {
		t.Fatalf("Load generated config: %v", err)
	}
	if !cfg.FloodControl.Enabled {
		t.Error("expected flood control enabled in generated config")
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

func writeTempConfig(t *testing.T, content string) string {
	t.Helper()
	tmpFile, err := os.CreateTemp("", "config-*.yaml")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := tmpFile.WriteString(content); err != nil {
		t.Fatal(err)
	}
	if err := tmpFile.Close(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Remove(tmpFile.Name()) })
	return tmpFile.Name()
}

const validJWT = "this-is-a-very-long-secret-key-for-testing"

func TestLoadMissingConfigFile(t *testing.T) {
	_, err := Load("/nonexistent/path/config.yaml")
	if err == nil {
		t.Fatal("expected error for missing config file")
	}
}

func TestLoadInvalidPort(t *testing.T) {
	path := writeTempConfig(t, `
server:
  mode: "server"
  port: 70000
database:
  path: "./test.db"
security:
  jwt_secret: "`+validJWT+`"
`)
	_, err := Load(path)
	if err == nil {
		t.Fatal("expected error for invalid port")
	}
}

func TestLoadShortJWTSecret(t *testing.T) {
	path := writeTempConfig(t, `
server:
  mode: "server"
  port: 8080
database:
  path: "./test.db"
security:
  jwt_secret: "short"
`)
	_, err := Load(path)
	if err == nil {
		t.Fatal("expected error for short jwt_secret")
	}
}

func TestLoadLowEntropyJWTSecret(t *testing.T) {
	path := writeTempConfig(t, `
server:
  mode: "server"
  port: 8080
database:
  path: "./test.db"
security:
  jwt_secret: "`+strings.Repeat("a", 32)+`"
`)
	_, err := Load(path)
	if err == nil {
		t.Fatal("expected error for low entropy jwt_secret")
	}
}

func TestLoadAgentModeMissingToken(t *testing.T) {
	path := writeTempConfig(t, `
server:
  mode: "agent"
  port: 8080
database:
  path: "./test.db"
agent:
  token: ""
`)
	_, err := Load(path)
	if err == nil {
		t.Fatal("expected error for missing agent token")
	}
}

func TestLoadAgentModeValid(t *testing.T) {
	path := writeTempConfig(t, `
server:
  mode: "agent"
  port: 8080
database:
  path: "./test.db"
agent:
  token: "secret-agent-token"
`)
	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load agent config: %v", err)
	}
	if !cfg.IsAgent() {
		t.Fatal("expected agent mode")
	}
}

func TestLoadInvalidCredentialKeyLength(t *testing.T) {
	path := writeTempConfig(t, `
server:
  mode: "server"
  port: 8080
database:
  path: "./test.db"
security:
  jwt_secret: "`+validJWT+`"
  credential_key: "abc"
`)
	_, err := Load(path)
	if err == nil {
		t.Fatal("expected error for invalid credential_key length")
	}
}

func TestLoadInvalidCredentialKeyHex(t *testing.T) {
	path := writeTempConfig(t, `
server:
  mode: "server"
  port: 8080
database:
  path: "./test.db"
security:
  jwt_secret: "`+validJWT+`"
  credential_key: "`+strings.Repeat("g", 64)+`"
`)
	_, err := Load(path)
	if err == nil {
		t.Fatal("expected error for invalid credential_key hex")
	}
}

func TestLoadValidCredentialKey(t *testing.T) {
	path := writeTempConfig(t, `
server:
  mode: "server"
  port: 8080
database:
  path: "./test.db"
security:
  jwt_secret: "`+validJWT+`"
  credential_key: "`+strings.Repeat("a", 64)+`"
`)
	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load with credential_key: %v", err)
	}
	if cfg.Security.CredentialKey == "" {
		t.Fatal("expected credential_key to be loaded")
	}
}

func TestLoadFloodControlNegativeLimits(t *testing.T) {
	tests := []struct {
		name    string
		content string
	}{
		{"http rate", `
server:
  mode: "server"
  port: 8080
database:
  path: "./test.db"
security:
  jwt_secret: "` + validJWT + `"
flood_control:
  enabled: true
  http_rate_limit_per_min: -1
`},
		{"query rate", `
server:
  mode: "server"
  port: 8080
database:
  path: "./test.db"
security:
  jwt_secret: "` + validJWT + `"
flood_control:
  enabled: true
  query_rate_limit_per_min: -1
`},
		{"brute force max", `
server:
  mode: "server"
  port: 8080
database:
  path: "./test.db"
security:
  jwt_secret: "` + validJWT + `"
flood_control:
  enabled: true
  brute_force_max: -1
`},
		{"brute force ban", `
server:
  mode: "server"
  port: 8080
database:
  path: "./test.db"
security:
  jwt_secret: "` + validJWT + `"
flood_control:
  enabled: true
  brute_force_ban_min: -1
`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := writeTempConfig(t, tt.content)
			if _, err := Load(path); err == nil {
				t.Fatal("expected validation error")
			}
		})
	}
}

func TestLoadExplicitFloodControlDefaults(t *testing.T) {
	path := writeTempConfig(t, `
server:
  mode: "server"
  port: 8080
database:
  path: "./test.db"
security:
  jwt_secret: "`+validJWT+`"
flood_control:
  http_rate_limit_per_min: 50
  query_rate_limit_per_min: 60
  brute_force_max: 3
  brute_force_ban_min: 10
`)
	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.FloodControl.HTTPRateLimitPerMin != 50 {
		t.Fatalf("http limit = %d", cfg.FloodControl.HTTPRateLimitPerMin)
	}
	if cfg.FloodControl.QueryRateLimitPerMin != 60 {
		t.Fatalf("query limit = %d", cfg.FloodControl.QueryRateLimitPerMin)
	}
	if cfg.Security.RateLimitPerMin != 50 {
		t.Fatalf("legacy sync = %d", cfg.Security.RateLimitPerMin)
	}
}

func TestGenerateAgentConfig(t *testing.T) {
	tmpFile, err := os.CreateTemp("", "agent-config-*.yaml")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(tmpFile.Name())
	tmpFile.Close()

	if err := Generate(tmpFile.Name(), "agent"); err != nil {
		t.Fatalf("Generate agent: %v", err)
	}

	data, err := os.ReadFile(tmpFile.Name())
	if err != nil {
		t.Fatal(err)
	}
	content := string(data)
	for _, want := range []string{
		`mode: "agent"`,
		"agent:",
		"token:",
	} {
		if !strings.Contains(content, want) {
			t.Errorf("generated agent config missing %q", want)
		}
	}

	cfg, err := Load(tmpFile.Name())
	if err != nil {
		t.Fatalf("Load generated agent config: %v", err)
	}
	if !cfg.IsAgent() {
		t.Fatal("expected agent mode")
	}
}

func TestIsLowEntropy(t *testing.T) {
	if !isLowEntropy("") {
		t.Fatal("empty string should be low entropy")
	}
	if !isLowEntropy("aaaaaaaaaa") {
		t.Fatal("repeated chars should be low entropy")
	}
	if isLowEntropy(validJWT) {
		t.Fatal("varied secret should not be low entropy")
	}
}

func TestLoadUnmarshalError(t *testing.T) {
	path := writeTempConfig(t, `
server:
  mode: "server"
  port: not-a-number
database:
  path: "./test.db"
security:
  jwt_secret: "`+validJWT+`"
`)
	_, err := Load(path)
	if err == nil {
		t.Fatal("expected unmarshal error")
	}
}

func TestLoadLegacyBruteForceBanMinOnly(t *testing.T) {
	path := writeTempConfig(t, `
server:
  mode: "server"
  port: 8080
database:
  path: "./test.db"
security:
  jwt_secret: "`+validJWT+`"
  brute_force_ban_min: 20
`)
	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.FloodControl.BruteForceBanMin != 20 {
		t.Fatalf("brute_force_ban_min = %d, want 20", cfg.FloodControl.BruteForceBanMin)
	}
}

func TestLoadDefaultFloodControlWhenUnset(t *testing.T) {
	cfg := &Config{
		FloodControl: FloodControlConfig{Enabled: true},
	}
	normalizeFloodControl(cfg)
	if cfg.FloodControl.HTTPRateLimitPerMin != 100 {
		t.Fatalf("default http limit = %d", cfg.FloodControl.HTTPRateLimitPerMin)
	}
	if cfg.FloodControl.QueryRateLimitPerMin != 100 {
		t.Fatalf("default query limit = %d", cfg.FloodControl.QueryRateLimitPerMin)
	}
	if cfg.FloodControl.BruteForceMax != 5 {
		t.Fatalf("default brute force max = %d", cfg.FloodControl.BruteForceMax)
	}
	if cfg.FloodControl.BruteForceBanMin != 15 {
		t.Fatalf("default brute force ban = %d", cfg.FloodControl.BruteForceBanMin)
	}
}

func TestNormalizeFloodControlLegacyFields(t *testing.T) {
	cfg := &Config{
		Security: SecurityConfig{
			RateLimitPerMin:  25,
			BruteForceMax:    7,
			BruteForceBanMin: 20,
		},
	}
	normalizeFloodControl(cfg)
	if cfg.FloodControl.HTTPRateLimitPerMin != 25 {
		t.Fatalf("http limit = %d", cfg.FloodControl.HTTPRateLimitPerMin)
	}
	if cfg.FloodControl.BruteForceMax != 7 {
		t.Fatalf("brute force max = %d", cfg.FloodControl.BruteForceMax)
	}
	if cfg.FloodControl.BruteForceBanMin != 20 {
		t.Fatalf("brute force ban = %d", cfg.FloodControl.BruteForceBanMin)
	}
	if cfg.Security.RateLimitPerMin != 25 {
		t.Fatalf("legacy sync = %d", cfg.Security.RateLimitPerMin)
	}
}

func TestNormalizeFloodControlPreservesExplicitValues(t *testing.T) {
	cfg := &Config{
		FloodControl: FloodControlConfig{
			HTTPRateLimitPerMin:  50,
			QueryRateLimitPerMin: 60,
			BruteForceMax:        3,
			BruteForceBanMin:     10,
		},
		Security: SecurityConfig{RateLimitPerMin: 99},
	}
	normalizeFloodControl(cfg)
	if cfg.FloodControl.HTTPRateLimitPerMin != 50 {
		t.Fatalf("explicit http limit overwritten: %d", cfg.FloodControl.HTTPRateLimitPerMin)
	}
}

func TestGenerateMkdirError(t *testing.T) {
	tmpFile, err := os.CreateTemp("", "config-parent-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(tmpFile.Name())
	if err := tmpFile.Close(); err != nil {
		t.Fatal(err)
	}

	err = Generate(tmpFile.Name()+"/nested/config.yaml", "server")
	if err == nil {
		t.Fatal("expected error when parent path is a file")
	}
}

func TestGenerateAgentMkdirError(t *testing.T) {
	tmpFile, err := os.CreateTemp("", "agent-parent-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(tmpFile.Name())
	if err := tmpFile.Close(); err != nil {
		t.Fatal(err)
	}

	err = Generate(tmpFile.Name()+"/nested/config.yaml", "agent")
	if err == nil {
		t.Fatal("expected error when parent path is a file")
	}
}

func TestRandomHex(t *testing.T) {
	got, err := randomHex(16)
	if err != nil {
		t.Fatalf("randomHex: %v", err)
	}
	if len(got) != 32 {
		t.Fatalf("randomHex length = %d, want 32 hex chars", len(got))
	}
}

func TestRandomHexError(t *testing.T) {
	old := randReader
	randReader = func([]byte) (int, error) { return 0, errors.New("rand fail") }
	t.Cleanup(func() { randReader = old })

	if _, err := randomHex(4); err == nil {
		t.Fatal("expected randomHex error")
	}
}

func TestGenerateRandomHexError(t *testing.T) {
	old := randReader
	randReader = func([]byte) (int, error) { return 0, errors.New("rand fail") }
	t.Cleanup(func() { randReader = old })

	tmpFile, err := os.CreateTemp("", "config-gen-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(tmpFile.Name())
	tmpFile.Close()

	if err := Generate(tmpFile.Name(), "server"); err == nil {
		t.Fatal("expected Generate error when randomHex fails")
	}
}

func TestGenerateAgentRandomHexError(t *testing.T) {
	old := randReader
	randReader = func([]byte) (int, error) { return 0, errors.New("rand fail") }
	t.Cleanup(func() { randReader = old })

	tmpFile, err := os.CreateTemp("", "agent-gen-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(tmpFile.Name())
	tmpFile.Close()

	if err := Generate(tmpFile.Name(), "agent"); err == nil {
		t.Fatal("expected agent Generate error when randomHex fails")
	}
}

func TestGenerateRandomHexFailsOnSecondCall(t *testing.T) {
	old := randReader
	calls := 0
	randReader = func(b []byte) (int, error) {
		calls++
		if calls == 2 {
			return 0, errors.New("rand fail")
		}
		return rand.Read(b)
	}
	t.Cleanup(func() { randReader = old })

	tmpFile, err := os.CreateTemp("", "config-gen2-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(tmpFile.Name())
	tmpFile.Close()

	if err := Generate(tmpFile.Name(), "server"); err == nil {
		t.Fatal("expected Generate error on second randomHex call")
	}
}

func TestGenerateRandomHexFailsOnThirdCall(t *testing.T) {
	old := randReader
	calls := 0
	randReader = func(b []byte) (int, error) {
		calls++
		if calls == 3 {
			return 0, errors.New("rand fail")
		}
		return rand.Read(b)
	}
	t.Cleanup(func() { randReader = old })

	tmpFile, err := os.CreateTemp("", "config-gen3-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(tmpFile.Name())
	tmpFile.Close()

	if err := Generate(tmpFile.Name(), "server"); err == nil {
		t.Fatal("expected Generate error on third randomHex call")
	}
}

func TestGenerateWriteFileError(t *testing.T) {
	dir, err := os.MkdirTemp("", "config-readonly-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(dir)
	if err := os.Chmod(dir, 0500); err != nil {
		t.Fatal(err)
	}

	err = Generate(dir+"/cfg.yaml", "server")
	if err == nil {
		t.Fatal("expected write error in read-only directory")
	}
}
