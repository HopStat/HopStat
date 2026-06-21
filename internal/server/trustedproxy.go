package server

import (
	"log/slog"

	"github.com/HopStat/HopStat/internal/config"
	"github.com/gin-gonic/gin"
)

// Cloudflare publishes these CIDRs at https://www.cloudflare.com/ips/
var cloudflareCIDRs = []string{
	"173.245.48.0/20",
	"103.21.244.0/22",
	"103.22.200.0/22",
	"103.31.4.0/22",
	"141.101.64.0/18",
	"108.162.192.0/18",
	"190.93.240.0/20",
	"188.114.96.0/20",
	"197.234.240.0/22",
	"198.41.128.0/17",
	"162.158.0.0/15",
	"104.16.0.0/13",
	"104.24.0.0/14",
	"172.64.0.0/13",
	"131.0.72.0/22",
	"2400:cb00::/32",
	"2606:4700::/32",
	"2803:f800::/32",
	"2405:b500::/32",
	"2405:8100::/32",
	"2a06:98c0::/29",
	"2c0f:f248::/32",
}

// ConfigureClientIP sets trusted proxy handling for reverse proxies such as Cloudflare.
func ConfigureClientIP(router *gin.Engine, cfg config.ServerConfig) {
	proxies := append([]string(nil), cfg.TrustedProxies...)
	if cfg.BehindCloudflare {
		proxies = append(cloudflareCIDRs, proxies...)
	}

	if len(proxies) == 0 {
		router.SetTrustedProxies(nil)
		return
	}

	headers := []string{"X-Forwarded-For", "X-Real-IP"}
	if cfg.BehindCloudflare {
		headers = append([]string{"CF-Connecting-IP"}, headers...)
	}
	router.RemoteIPHeaders = headers

	if err := router.SetTrustedProxies(proxies); err != nil {
		slog.Warn("invalid trusted proxy CIDRs", "error", err)
		router.SetTrustedProxies(nil)
	}
}
