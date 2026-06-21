package bgp

import (
	"context"
	"net"
	"strings"

	"github.com/HopStat/HopStat/internal/domain"
	"github.com/HopStat/HopStat/internal/geo"
	"github.com/HopStat/HopStat/internal/target"
)

type targetASNResolver interface {
	ResolveASN(context.Context, string) (*domain.ASInfo, error)
}

// EnrichResultTargetAS attaches ASN metadata for the queried prefix or IP.
func EnrichResultTargetAS(ctx context.Context, geoDB *geo.GeoIPDB, br *domain.BGPResult, prefix string) {
	if geoDB == nil {
		return
	}
	enrichResultTargetAS(ctx, geoDB, br, prefix)
}

func enrichResultTargetAS(ctx context.Context, resolver targetASNResolver, br *domain.BGPResult, prefix string) {
	if br == nil || resolver == nil || br.TargetAS != nil {
		return
	}

	normalized, err := target.NormalizeBGPLookup(ctx, prefix)
	if err != nil {
		return
	}

	ip := queryTargetIP(normalized)
	if ip == "" {
		return
	}

	info, err := resolver.ResolveASN(ctx, ip)
	if err != nil || info == nil || info.ASN == 0 {
		return
	}

	copyInfo := *info
	copyInfo.ASN = info.ASN
	if copyInfo.CountryCode != "" && copyInfo.FlagEmoji == "" {
		copyInfo.FlagEmoji = geo.CountryToFlag(copyInfo.CountryCode)
	}
	br.TargetAS = &copyInfo
}

// queryTargetIPHook, when set, overrides queryTargetIP (tests only).
var queryTargetIPHook func(string) string

func queryTargetIP(normalized string) string {
	if queryTargetIPHook != nil {
		return queryTargetIPHook(normalized)
	}
	if strings.Contains(normalized, "/") {
		ip, _, err := net.ParseCIDR(normalized)
		if err == nil {
			return ip.String()
		}
	}
	if ip := net.ParseIP(normalized); ip != nil {
		return ip.String()
	}
	return ""
}
