package config

type Config struct {
	Server   ServerConfig   `mapstructure:"server"`
	Agent    AgentConfig    `mapstructure:"agent"`
	Database DatabaseConfig `mapstructure:"database"`
	Security SecurityConfig `mapstructure:"security"`
	Audit    AuditConfig    `mapstructure:"audit"`
	Query    QueryConfig    `mapstructure:"query"`
	GeoIP    GeoIPConfig    `mapstructure:"geoip"`
	Footer   FooterConfig   `mapstructure:"footer"`
	BGP      BGPConfig      `mapstructure:"bgp"`
	Update   UpdateConfig   `mapstructure:"update"`
}

type ServerConfig struct {
	Host           string `mapstructure:"host"`
	Port           int    `mapstructure:"port"`
	Mode           string `mapstructure:"mode"`
	TLSCert        string `mapstructure:"tls_cert"`
	TLSKey         string `mapstructure:"tls_key"`
	AutocertDomain string `mapstructure:"autocert_domain"`
	DefaultRouteAS string `mapstructure:"default_route_as"`
}

type AgentConfig struct {
	Port        int    `mapstructure:"port"`
	Token       string `mapstructure:"token"`
	BGPRouterID string `mapstructure:"bgp_router_id"`
	BGPLocalAS  uint32 `mapstructure:"bgp_local_as"`
	BGPPeerAS   uint32 `mapstructure:"bgp_peer_as"`
	BGPPeerAddr string `mapstructure:"bgp_peer_addr"`
}

type DatabaseConfig struct {
	Path string `mapstructure:"path"`
}

type SecurityConfig struct {
	JWTSecret        string `mapstructure:"jwt_secret"`
	CredentialKey    string `mapstructure:"credential_key"`
	RateLimitPerMin  int    `mapstructure:"rate_limit_per_min"`
	BruteForceMax    int    `mapstructure:"brute_force_max"`
	BruteForceBanMin int    `mapstructure:"brute_force_ban_min"`
}

type AuditConfig struct {
	RetentionDays int  `mapstructure:"retention_days"`
	AsyncWrite    bool `mapstructure:"async_write"`
}

type QueryConfig struct {
	MaxConcurrent        int `mapstructure:"max_concurrent"`
	DefaultTimeoutSec    int `mapstructure:"default_timeout_sec"`
	MTRTimeoutSec        int `mapstructure:"mtr_timeout_sec"`
	TracerouteTimeoutSec int `mapstructure:"traceroute_timeout_sec"`
}

type GeoIPConfig struct {
	ASNDBPath      string `mapstructure:"asn_db_path"`
	CityDBPath     string `mapstructure:"city_db_path"`
	LicenseKey     string `mapstructure:"license_key"`
	AccountID      string `mapstructure:"account_id"`
	UpdateInterval string `mapstructure:"update_interval"`
	DBDir          string `mapstructure:"db_dir"`
}

type FooterConfig struct {
	PeeringDBASN int    `mapstructure:"peeringdb_asn"`
	IPv4URL      string `mapstructure:"ipv4_url"`
	Email        string `mapstructure:"email"`
	BGPURL       string `mapstructure:"bgp_url"`
	NOCURL       string `mapstructure:"noc_url"`
}

type BGPConfig struct {
	ListenPort      int      `mapstructure:"listen_port"`
	RouterID        string   `mapstructure:"router_id"`
	LocalAS         uint32   `mapstructure:"local_as"`
	ListenAddresses []string `mapstructure:"listen_addresses"`
}

type UpdateConfig struct {
	Enabled bool `mapstructure:"enabled"`
}

func (c *Config) IsServer() bool {
	return c.Server.Mode == "server"
}

func (c *Config) IsAgent() bool {
	return c.Server.Mode == "agent"
}
