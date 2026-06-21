package geo

import (
	"context"
	"fmt"
	"net"
	"strings"

	"github.com/HopStat/HopStat/internal/domain"
)

type ResolveSource string

const (
	SourceNone   ResolveSource = "none"
	SourceBlocks ResolveSource = "blocks"
	SourceMMDB   ResolveSource = "mmdb"
	SourceDNS    ResolveSource = "dns"
)

type SourceCandidate struct {
	Available bool           `json:"available"`
	Matched   bool           `json:"matched"`
	Network   string         `json:"network,omitempty"`
	Info      *domain.ASInfo `json:"info,omitempty"`
	Error     string         `json:"error,omitempty"`
}

type IPLookupCity struct {
	CountryISO  string  `json:"country_iso,omitempty"`
	Country     string  `json:"country,omitempty"`
	CountryFlag string  `json:"country_flag,omitempty"`
	City        string  `json:"city,omitempty"`
	Latitude    float64 `json:"latitude,omitempty"`
	Longitude   float64 `json:"longitude,omitempty"`
	TimeZone    string  `json:"time_zone,omitempty"`
}

type IPLookupReport struct {
	IP               string          `json:"ip"`
	ChosenSource     ResolveSource   `json:"chosen_source"`
	Result           *domain.ASInfo  `json:"result,omitempty"`
	Blocks           SourceCandidate `json:"blocks"`
	MMDB             SourceCandidate `json:"mmdb"`
	DNS              SourceCandidate `json:"dns"`
	City             *IPLookupCity   `json:"city,omitempty"`
	ASNIndexEnriched bool            `json:"asn_index_enriched"`
	BlocksCSVLoaded  bool            `json:"blocks_csv_loaded"`
	BlocksCSVCount   int             `json:"blocks_csv_count"`
	ASNDBPath        string          `json:"asn_db_path,omitempty"`
	CityDBPath       string          `json:"city_db_path,omitempty"`
	CacheNote        string          `json:"cache_note"`
}

const ipLookupCacheNote = "No per-IP cache. ASN org metadata is stored in memory by ASN number after a successful lookup (not by IP)."

func cloneASInfo(info *domain.ASInfo) *domain.ASInfo {
	if info == nil {
		return nil
	}
	out := *info
	return &out
}

func cityToLookupCity(c *GeoCityInfo) *IPLookupCity {
	if c == nil {
		return nil
	}
	return &IPLookupCity{
		CountryISO:  c.CountryISO,
		Country:     c.Country,
		CountryFlag: c.CountryFlag,
		City:        c.City,
		Latitude:    c.Latitude,
		Longitude:   c.Longitude,
		TimeZone:    c.TimeZone,
	}
}

func chosenSource(blockOk, mmOk bool, picked, block, mm *domain.ASInfo) ResolveSource {
	if picked == nil || picked.ASN == 0 {
		return SourceNone
	}
	if blockOk && block != nil && picked.ASN == block.ASN {
		return SourceBlocks
	}
	if mmOk && mm != nil && picked.ASN == mm.ASN {
		return SourceMMDB
	}
	return inferChosenSource(blockOk, mmOk, false)
}

var ipLookupChosenSource = chosenSource

func inferChosenSource(blockOk, mmOk, dnsMatched bool) ResolveSource {
	switch {
	case blockOk:
		return SourceBlocks
	case mmOk:
		return SourceMMDB
	case dnsMatched:
		return SourceDNS
	default:
		return SourceNone
	}
}

func (g *GeoIPDB) enrichASInfo(parsed net.IP, info *domain.ASInfo) (*domain.ASInfo, bool) {
	if info == nil || info.ASN == 0 {
		return info, false
	}

	g.mu.RLock()
	cityDB := g.cityDB
	g.mu.RUnlock()

	out := *info
	if cityDB != nil {
		applyCityRecord(cityDB, parsed, &out)
	}

	indexEnriched := false
	if strings.TrimSpace(out.OrgName) == "" && strings.TrimSpace(out.ShortName) == "" {
		if idx, ok := g.lookupASNFromIndex(out.ASN); ok {
			out = *mergeASInfoPreferExisting(&out, idx)
			indexEnriched = true
		}
	}
	return &out, indexEnriched
}

// LookupIP resolves an IP using the same pipeline as traceroute enrichment and
// returns per-source candidates for admin debugging. Does not write to rememberASN.
func (g *GeoIPDB) LookupIP(ctx context.Context, ip string) (*IPLookupReport, error) {
	parsed := net.ParseIP(strings.TrimSpace(ip))
	if parsed == nil {
		return nil, fmt.Errorf("invalid IP: %s", ip)
	}

	g.mu.RLock()
	asnDB := g.asnDB
	cityDB := g.cityDB
	asnBlocks := g.asnNetworkBlocks
	asnPath := g.asnPath
	cityPath := g.cityPath
	blocksCount := len(asnBlocks)
	g.mu.RUnlock()

	report := &IPLookupReport{
		IP:              parsed.String(),
		ChosenSource:    SourceNone,
		BlocksCSVLoaded: blocksCount > 0,
		BlocksCSVCount:  blocksCount,
		ASNDBPath:       asnPath,
		CityDBPath:      cityPath,
		CacheNote:       ipLookupCacheNote,
	}

	report.Blocks.Available = blocksCount > 0
	if blockInfo, network, blockOk := lookupLongestASNBlockNetwork(asnBlocks, parsed); blockOk {
		report.Blocks.Matched = true
		report.Blocks.Network = network
		report.Blocks.Info = cloneASInfo(&blockInfo)
	}

	report.MMDB.Available = asnDB != nil
	if asnDB != nil {
		record, err := lookupASNRecord(asnDB, parsed)
		if err != nil {
			report.MMDB.Error = err.Error()
		} else if mm, ok := lookupMMDBFromRecord(record, cityDB, parsed); ok && mm.ASN > 0 {
			report.MMDB.Matched = true
			report.MMDB.Info = cloneASInfo(mm)
		}
	}

	dnsInfo, dnsErr := g.resolveDNS(ctx, parsed, parsed.String())
	report.DNS.Available = parsed.To4() != nil
	if dnsErr != nil {
		report.DNS.Error = dnsErr.Error()
	} else if dnsInfo != nil && dnsInfo.ASN > 0 {
		report.DNS.Matched = true
		report.DNS.Info = cloneASInfo(dnsInfo)
	}

	blockOk := report.Blocks.Matched && report.Blocks.Info != nil && report.Blocks.Info.ASN > 0
	mmOk := report.MMDB.Matched && report.MMDB.Info != nil && report.MMDB.Info.ASN > 0

	var blockInfo domain.ASInfo
	if blockOk {
		blockInfo = *report.Blocks.Info
	}
	picked := pickASNInfo(report.MMDB.Info, mmOk, blockInfo, blockOk)

	if picked == nil && report.DNS.Matched && report.DNS.Info != nil {
		picked = cloneASInfo(report.DNS.Info)
		if cityDB != nil {
			applyCityRecord(cityDB, parsed, picked)
		}
		report.ChosenSource = SourceDNS
	} else if picked != nil && picked.ASN > 0 {
		report.ChosenSource = ipLookupChosenSource(blockOk, mmOk, picked, report.Blocks.Info, report.MMDB.Info)
		if report.ChosenSource == SourceNone {
			report.ChosenSource = inferChosenSource(blockOk, mmOk, false)
		}
	}

	if picked != nil && picked.ASN > 0 {
		enriched, indexUsed := g.enrichASInfo(parsed, picked)
		report.Result = enriched
		report.ASNIndexEnriched = indexUsed
	}

	if cityDB != nil {
		if city, err := g.LookupCity(parsed.String()); err == nil {
			report.City = cityToLookupCity(city)
		}
	}

	return report, nil
}
