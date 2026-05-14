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
	agentToken, err := randomHex(24)
	if err != nil {
		return err
	}

	var content string
	if mode == "agent" {
		content = fmt.Sprintf(`# HopStat agent configuration (auto-generated)
server:
  mode: "agent"

agent:
  port: 9090
  token: "%s"

`, agentToken)
	} else {
		content = fmt.Sprintf(`# HopStat configuration (auto-generated)
server:
  host: "0.0.0.0"
  port: 8080

security:
  jwt_secret: "%s"
  credential_key: "%s"
  rate_limit_per_min: 10
  brute_force_max: 5
  brute_force_ban_min: 15

database:
  path: "./lg.db"

geoip:
  license_key: ""
  account_id: ""
  db_dir: "./data/geoip"
  update_interval: "72h"

update:
  enabled: true
`, jwtSecret, credKey)
	}

	return os.WriteFile(path, []byte(content), 0600)
}

func setDefaults(v *viper.Viper) {
	v.SetDefault("server.host", "0.0.0.0")
	v.SetDefault("server.port", 8080)
	v.SetDefault("server.mode", "server")
	v.SetDefault("server.default_route_as", "9121")
	v.SetDefault("agent.port", 9090)
	v.SetDefault("database.path", "./lg.db")
	v.SetDefault("security.rate_limit_per_min", 10)
	v.SetDefault("security.brute_force_max", 5)
	v.SetDefault("security.brute_force_ban_min", 15)
	v.SetDefault("audit.retention_days", 90)
	v.SetDefault("audit.async_write", true)
	v.SetDefault("query.max_concurrent", 50)
	v.SetDefault("query.default_timeout_sec", 30)
	v.SetDefault("query.mtr_timeout_sec", 120)
	v.SetDefault("query.traceroute_timeout_sec", 60)
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
	}
	if cfg.IsAgent() && strings.TrimSpace(cfg.Agent.Token) == "" {
		return fmt.Errorf("agent.token is required in agent mode")
	}
	if cfg.Security.CredentialKey != "" {
		if len(cfg.Security.CredentialKey) != 64 {
			return fmt.Errorf("security.credential_key must be 64 hex characters (32 bytes)")
		}
		for _, c := range cfg.Security.CredentialKey {
			if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F')) {
				return fmt.Errorf("security.credential_key must be valid hex characters only")
			}
		}
	}
	return nil
}

func randomHex(n int) (string, error) {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("generate random bytes: %w", err)
	}
	return hex.EncodeToString(b), nil
}
