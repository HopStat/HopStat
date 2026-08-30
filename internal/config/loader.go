package config

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/viper"
)

var randReader = rand.Read

func Load(configPath string) (*Config, error) {
	v := viper.New()
	v.SetConfigFile(configPath)
	v.SetConfigType("yaml")
	v.SetEnvPrefix("LG")
	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	v.AutomaticEnv()
	setDefaults(v)

	if err := v.ReadInConfig(); err != nil {
		return nil, fmt.Errorf("read config: %w", err)
	}

	var cfg Config
	if err := v.Unmarshal(&cfg); err != nil {
		return nil, fmt.Errorf("unmarshal config: %w", err)
	}

	normalizeFloodControl(&cfg)

	if err := validate(&cfg); err != nil {
		return nil, fmt.Errorf("validate config: %w", err)
	}

	return &cfg, nil
}

// Generate creates a new config file at path with random secrets.
// mode is "server" or "agent" — controls which template is written.
func Generate(path, mode string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return fmt.Errorf("create config dir: %w", err)
	}

	jwtSecret, err := randomHex(32)
	if err != nil {
		return err
	}
	credKey, err := randomHex(32)
	if err != nil {
		return err
	}
	agentToken, err := randomHex(16)
	if err != nil {
		return err
	}

	var content string
	if mode == "agent" {
		content = fmt.Sprintf(`# HopStat agent configuration (auto-generated)
server:
  host: "0.0.0.0"
  mode: "agent"

agent:
  port: 9090
  token: "%s"

database:
  path: "./lg.db"

query:
  max_concurrent: 50
  default_timeout_sec: 30
  traceroute_timeout_sec: 60

`, agentToken)
	} else {
		content = fmt.Sprintf(`# HopStat configuration (auto-generated)
server:
  host: "0.0.0.0"
  port: 8080
  mode: "server"
  tls_cert: ""
  tls_key: ""
  autocert_domain: ""
  # Set true when HopStat is behind Cloudflare (uses CF-Connecting-IP for visitor IP).
  behind_cloudflare: false
  # Optional extra reverse-proxy CIDRs, e.g. ["10.0.0.0/8"]
  trusted_proxies: []

database:
  path: "./lg.db"

security:
  jwt_secret: "%s"
  credential_key: "%s"

flood_control:
  enabled: true
  http_rate_limit_per_min: 100
  query_rate_limit_per_min: 100
  brute_force_max: 5
  brute_force_ban_min: 15

audit:
  retention_days: 90
  async_write: true

query:
  max_concurrent: 50
  default_timeout_sec: 30
  traceroute_timeout_sec: 60

geoip:
  asn_db_path: ""
  city_db_path: ""
  license_key: ""
  account_id: ""
  update_interval: "72h"
  db_dir: "./data/geoip"

update:
  enabled: true

bgp:
  listen_port: 11790
  router_id: ""
  local_as: 0
  listen_addresses: []
  add_path_receive: true
`, jwtSecret, credKey)
	}

	return os.WriteFile(path, []byte(content), 0600)
}

func setDefaults(v *viper.Viper) {
	v.SetDefault("server.host", "0.0.0.0")
	v.SetDefault("server.port", 8080)
	v.SetDefault("server.mode", "server")
	v.SetDefault("agent.port", 9090)
	v.SetDefault("database.path", "./lg.db")
	v.SetDefault("security.rate_limit_per_min", 10)
	v.SetDefault("security.brute_force_max", 5)
	v.SetDefault("security.brute_force_ban_min", 15)
	v.SetDefault("flood_control.enabled", true)
	v.SetDefault("audit.retention_days", 90)
	v.SetDefault("audit.async_write", true)
	v.SetDefault("query.max_concurrent", 50)
	v.SetDefault("query.default_timeout_sec", 30)
	v.SetDefault("query.traceroute_timeout_sec", 60)
	v.SetDefault("bgp.listen_port", 11790)
	v.SetDefault("bgp.add_path_receive", true)
}

// isLowEntropy returns true if the string consists of a single repeated character
// or is an obvious low-entropy value (all same chars, sequential digits, etc.)
func isLowEntropy(s string) bool {
	if len(s) == 0 {
		return true
	}
	first := s[0]
	allSame := true
	for i := 1; i < len(s); i++ {
		if s[i] != first {
			allSame = false
			break
		}
	}
	return allSame
}

// normalizeFloodControl merges legacy security.* limits into flood_control and applies defaults.
func normalizeFloodControl(cfg *Config) {
	if cfg.FloodControl.HTTPRateLimitPerMin == 0 && cfg.Security.RateLimitPerMin > 0 {
		cfg.FloodControl.HTTPRateLimitPerMin = cfg.Security.RateLimitPerMin
	}
	if cfg.FloodControl.BruteForceMax == 0 && cfg.Security.BruteForceMax > 0 {
		cfg.FloodControl.BruteForceMax = cfg.Security.BruteForceMax
	}
	if cfg.FloodControl.BruteForceBanMin == 0 && cfg.Security.BruteForceBanMin > 0 {
		cfg.FloodControl.BruteForceBanMin = cfg.Security.BruteForceBanMin
	}
	if cfg.FloodControl.HTTPRateLimitPerMin == 0 {
		cfg.FloodControl.HTTPRateLimitPerMin = 100
	}
	if cfg.FloodControl.QueryRateLimitPerMin == 0 {
		cfg.FloodControl.QueryRateLimitPerMin = 100
	}
	if cfg.FloodControl.BruteForceMax == 0 {
		cfg.FloodControl.BruteForceMax = 5
	}
	if cfg.FloodControl.BruteForceBanMin == 0 {
		cfg.FloodControl.BruteForceBanMin = 15
	}

	// Keep security in sync for any code still reading legacy fields.
	cfg.Security.RateLimitPerMin = cfg.FloodControl.HTTPRateLimitPerMin
	cfg.Security.BruteForceMax = cfg.FloodControl.BruteForceMax
	cfg.Security.BruteForceBanMin = cfg.FloodControl.BruteForceBanMin
}

func validate(cfg *Config) error {
	if cfg.Server.Mode != "server" && cfg.Server.Mode != "agent" {
		return fmt.Errorf("invalid server.mode: %q (must be \"server\" or \"agent\")", cfg.Server.Mode)
	}
	if cfg.Server.Port < 1 || cfg.Server.Port > 65535 {
		return fmt.Errorf("invalid server.port: %d", cfg.Server.Port)
	}
	if cfg.IsServer() {
		if strings.TrimSpace(cfg.Security.JWTSecret) == "" {
			return fmt.Errorf("security.jwt_secret is required in server mode")
		}
		if len(cfg.Security.JWTSecret) < 32 {
			return fmt.Errorf("security.jwt_secret must be at least 32 characters")
		}
		if isLowEntropy(cfg.Security.JWTSecret) {
			return fmt.Errorf("security.jwt_secret appears to have low entropy; use a randomly generated value (e.g. 'openssl rand -hex 32')")
		}
	}
	if cfg.IsAgent() && strings.TrimSpace(cfg.Agent.Token) == "" {
		return fmt.Errorf("agent.token is required in agent mode")
	}
	if cfg.Security.CredentialKey != "" {
		if len(cfg.Security.CredentialKey) != 64 {
			return fmt.Errorf("security.credential_key must be 64 hex characters (32 bytes)")
		}
		isHexDigit := func(c rune) bool {
			return (c >= '0' && c <= '9') || (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F')
		}
		for _, c := range cfg.Security.CredentialKey {
			if !isHexDigit(c) {
				return fmt.Errorf("security.credential_key must be valid hex characters only")
			}
		}
	}
	if cfg.FloodControl.Enabled {
		if cfg.FloodControl.HTTPRateLimitPerMin < 0 {
			return fmt.Errorf("flood_control.http_rate_limit_per_min must be >= 0")
		}
		if cfg.FloodControl.QueryRateLimitPerMin < 0 {
			return fmt.Errorf("flood_control.query_rate_limit_per_min must be >= 0")
		}
		if cfg.FloodControl.BruteForceMax < 0 {
			return fmt.Errorf("flood_control.brute_force_max must be >= 0")
		}
		if cfg.FloodControl.BruteForceBanMin < 0 {
			return fmt.Errorf("flood_control.brute_force_ban_min must be >= 0")
		}
	}
	return nil
}

func randomHex(n int) (string, error) {
	b := make([]byte, n)
	if _, err := randReader(b); err != nil {
		return "", fmt.Errorf("generate random bytes: %w", err)
	}
	return hex.EncodeToString(b), nil
}
