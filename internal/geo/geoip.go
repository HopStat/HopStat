package geo

import (
	"context"
	"fmt"
	"net"
	"strings"
	"sync"

	"github.com/oschwald/geoip2-golang"
	"github.com/HopStat/HopStat/internal/domain"
)

type GeoIPDB struct {
	mu       sync.RWMutex
	asnDB    *geoip2.Reader
	cityDB   *geoip2.Reader
	asnPath  string
	cityPath string
	enabled  bool
}

func New(asnPath, cityPath string) *GeoIPDB {
	g := &GeoIPDB{
		asnPath:  asnPath,
		cityPath: cityPath,
	}

	if asnPath != "" {
		if db, err := geoip2.Open(asnPath); err == nil {
			g.asnDB = db
			g.enabled = true
		}
	}
	if cityPath != "" {
		if db, err := geoip2.Open(cityPath); err == nil {
			g.cityDB = db
			g.enabled = true
		}
	}

	return g
}

func (g *GeoIPDB) Enabled() bool {
	return g.enabled
}

func (g *GeoIPDB) SetPaths(asnPath, cityPath string) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.asnPath = asnPath
	g.cityPath = cityPath
}

func (g *GeoIPDB) Close() {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.asnDB != nil {
		g.asnDB.Close()
	}
	if g.cityDB != nil {
		g.cityDB.Close()
	}
}

func (g *GeoIPDB) ResolveASN(ctx context.Context, ip string) (*domain.ASInfo, error) {
	parsed := net.ParseIP(ip)
	if parsed == nil {
		return nil, fmt.Errorf("invalid IP: %s", ip)
	}

	g.mu.RLock()
	defer g.mu.RUnlock()

	if g.asnDB != nil {
		return g.resolveMMDB(parsed)
	}
	return g.resolveDNS(ctx, parsed, ip)
}

func (g *GeoIPDB) LookupCity(ip string) (*GeoCityInfo, error) {
	parsed := net.ParseIP(ip)
	if parsed == nil {
		return nil, fmt.Errorf("invalid IP: %s", ip)
	}

	g.mu.RLock()
	defer g.mu.RUnlock()

	if g.cityDB == nil {
		return nil, fmt.Errorf("city database not loaded")
	}

	record, err := g.cityDB.City(parsed)
	if err != nil {
		return nil, err
	}

	info := &GeoCityInfo{
		CountryISO: record.Country.IsoCode,
		Country:    record.Country.Names["en"],
		City:       record.City.Names["en"],
		Latitude:   record.Location.Latitude,
		Longitude:  record.Location.Longitude,
		TimeZone:   record.Location.TimeZone,
	}
	info.CountryFlag = CountryToFlag(info.CountryISO)

	return info, nil
}

func (g *GeoIPDB) resolveMMDB(ip net.IP) (*domain.ASInfo, error) {
	record, err := g.asnDB.ASN(ip)
	if err != nil {
		return nil, err
	}

	org := record.AutonomousSystemOrganization
	info := &domain.ASInfo{
		ASN:       uint32(record.AutonomousSystemNumber),
		ShortName: shortenOrgName(org),
		OrgName:   org,
	}

	// Enrich with country from city DB
	if g.cityDB != nil {
		if city, err := g.cityDB.City(ip); err == nil {
			info.CountryCode = city.Country.IsoCode
			info.FlagEmoji = CountryToFlag(info.CountryCode)
		}
	}

	return info, nil
}

func (g *GeoIPDB) resolveDNS(ctx context.Context, parsed net.IP, ip string) (*domain.ASInfo, error) {
	if parsed.To4() == nil {
		return &domain.ASInfo{}, nil
	}

	parts := strings.Split(ip, ".")
	if len(parts) != 4 {
		return &domain.ASInfo{}, nil
	}

	// Step 1: Resolve ASN from IP via origin.asn.cymru.com
	reversed := fmt.Sprintf("%s.%s.%s.%s.origin.asn.cymru.com", parts[3], parts[2], parts[1], parts[0])
	resolver := net.Resolver{}
	txtRecords, err := resolver.LookupTXT(ctx, reversed)
	if err != nil || len(txtRecords) == 0 {
		return &domain.ASInfo{}, nil
	}

	fields := strings.Split(txtRecords[0], " | ")
	info := &domain.ASInfo{}
	if len(fields) >= 1 {
		var asn uint32
		fmt.Sscanf(strings.TrimSpace(fields[0]), "%d", &asn)
		info.ASN = asn
	}
	if len(fields) >= 3 {
		info.CountryCode = strings.TrimSpace(fields[2])
		info.FlagEmoji = CountryToFlag(info.CountryCode)
	}

	// Step 2: Resolve org name from ASN via asn.cymru.com
	if info.ASN > 0 {
		asnQuery := fmt.Sprintf("AS%d.asn.cymru.com", info.ASN)
		if asnTXT, err := resolver.LookupTXT(ctx, asnQuery); err == nil && len(asnTXT) > 0 {
			asnFields := strings.Split(asnTXT[0], " | ")
			if len(asnFields) >= 5 {
				org := strings.TrimSpace(asnFields[4])
				info.OrgName = org
				info.ShortName = shortenOrgName(org)
			}
		}
	}

	if info.ASN == 0 {
		return &domain.ASInfo{}, nil
	}

	return info, nil
}

func (g *GeoIPDB) Reload() error {
	g.mu.Lock()
	defer g.mu.Unlock()

	if g.asnDB != nil {
		g.asnDB.Close()
		g.asnDB = nil
	}
	if g.cityDB != nil {
		g.cityDB.Close()
		g.cityDB = nil
	}

	g.enabled = false

	if g.asnPath != "" {
		db, err := geoip2.Open(g.asnPath)
		if err != nil {
			return fmt.Errorf("open ASN db: %w", err)
		}
		g.asnDB = db
		g.enabled = true
	}
	if g.cityPath != "" {
		db, err := geoip2.Open(g.cityPath)
		if err != nil {
			return fmt.Errorf("open city db: %w", err)
		}
		g.cityDB = db
		g.enabled = true
	}

	return nil
}

type GeoCityInfo struct {
	CountryISO string
	Country    string
	CountryFlag string
	City       string
	Latitude   float64
	Longitude  float64
	TimeZone   string
}

func CountryToFlag(code string) string {
	if len(code) != 2 {
		return ""
	}
	code = strings.ToUpper(code)
	flag := make([]rune, 0, 2)
	for _, c := range code {
		flag = append(flag, c+'U'-'A'+0x1F1E5)
	}
	return string(flag)
}

// shortenOrgName extracts the first part of an AS organization name.
// "CLOUDFLARENET - Cloudflare, Inc., US" → "CLOUDFLARENET"
// "EURONET, TR" → "EURONET"
func shortenOrgName(org string) string {
	org = strings.TrimSpace(org)
	if org == "" {
		return org
	}
	// Split on " - " or ", " and take the first part
	if idx := strings.Index(org, " - "); idx > 0 {
		return strings.TrimSpace(org[:idx])
	}
	if idx := strings.Index(org, ", "); idx > 0 {
		return strings.TrimSpace(org[:idx])
	}
	if len(org) > 25 {
		words := strings.Fields(org)
		if len(words) > 1 {
			return words[0]
		}
	}
	return org
}
