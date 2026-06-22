package geo

import (
	"context"
	"fmt"
	"net"
	"path/filepath"
	"strings"
	"sync"

	"github.com/oschwald/geoip2-golang"
	"github.com/HopStat/HopStat/internal/domain"
)

type GeoIPDB struct {
	mu               sync.RWMutex
	asnDB            *geoip2.Reader
	cityDB           *geoip2.Reader
	asnIndex         map[uint32]domain.ASInfo
	asnNetworkBlocks []asnNetworkBlock
	asnPath          string
	cityPath         string
	enabled          bool
	resolveMu        sync.Mutex
	resolveCache     map[string]domain.ASInfo
}

const maxResolveCacheEntries = 4096

var lookupASNTXT = func(ctx context.Context, name string) ([]string, error) {
	return (&net.Resolver{}).LookupTXT(ctx, name)
}

var lookupOriginTXT = lookupASNTXT

var lookupCityRecord = func(db *geoip2.Reader, ip net.IP) (*geoip2.City, error) {
	return db.City(ip)
}

var lookupASNRecord = func(db *geoip2.Reader, ip net.IP) (*geoip2.ASN, error) {
	return db.ASN(ip)
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
	g.reloadASNIndex()

	return g
}

func (g *GeoIPDB) Enabled() bool {
	g.mu.RLock()
	defer g.mu.RUnlock()
	return g.enabled
}

func (g *GeoIPDB) SetPaths(asnPath, cityPath string) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.asnPath = asnPath
	g.cityPath = cityPath
}

func (g *GeoIPDB) Paths() (asnPath, cityPath string) {
	g.mu.RLock()
	defer g.mu.RUnlock()
	return g.asnPath, g.cityPath
}

type DBBuildInfo struct {
	ASNLoaded  bool
	CityLoaded bool
	ASNBuild   int64
	CityBuild  int64
}

func (g *GeoIPDB) BuildInfo() DBBuildInfo {
	g.mu.RLock()
	defer g.mu.RUnlock()
	info := DBBuildInfo{}
	if g.asnDB != nil {
		info.ASNLoaded = true
		info.ASNBuild = int64(g.asnDB.Metadata().BuildEpoch)
	}
	if g.cityDB != nil {
		info.CityLoaded = true
		info.CityBuild = int64(g.cityDB.Metadata().BuildEpoch)
	}
	return info
}

func (g *GeoIPDB) Close() {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.asnDB != nil {
		_ = g.asnDB.Close()
	}
	if g.cityDB != nil {
		_ = g.cityDB.Close()
	}
}

func (g *GeoIPDB) ResolveASN(ctx context.Context, ip string) (*domain.ASInfo, error) {
	parsed := net.ParseIP(ip)
	if parsed == nil {
		return nil, fmt.Errorf("invalid IP: %s", ip)
	}

	if cached, ok := g.cachedResolve(ip); ok {
		if cached.ASN == 0 {
			return nil, nil
		}
		copyInfo := cached
		return &copyInfo, nil
	}

	g.mu.RLock()
	asnDB := g.asnDB
	cityDB := g.cityDB
	asnBlocks := g.asnNetworkBlocks
	g.mu.RUnlock()

	var mmInfo *domain.ASInfo
	var mmOk bool
	if asnDB != nil {
		mmInfo, mmOk = lookupMMDB(asnDB, cityDB, parsed)
	}

	block, blockOk := lookupLongestASNBlock(asnBlocks, parsed)
	info := pickASNInfo(mmInfo, mmOk, block, blockOk)

	if info != nil && info.ASN > 0 {
		info, _ = g.enrichASInfo(parsed, info)
		g.rememberASN(info)
		g.storeResolve(ip, info)
		return info, nil
	}

	dnsInfo, err := g.resolveDNS(ctx, parsed, ip)
	if err != nil || dnsInfo == nil || dnsInfo.ASN == 0 {
		g.storeResolve(ip, nil)
		return dnsInfo, err
	}
	if cityDB != nil {
		applyCityRecord(cityDB, parsed, dnsInfo)
	}
	g.rememberASN(dnsInfo)
	g.storeResolve(ip, dnsInfo)
	return dnsInfo, nil
}

func (g *GeoIPDB) cachedResolve(ip string) (domain.ASInfo, bool) {
	g.resolveMu.Lock()
	defer g.resolveMu.Unlock()
	info, ok := g.resolveCache[ip]
	return info, ok
}

func (g *GeoIPDB) storeResolve(ip string, info *domain.ASInfo) {
	g.resolveMu.Lock()
	defer g.resolveMu.Unlock()
	if g.resolveCache == nil {
		g.resolveCache = make(map[string]domain.ASInfo, 256)
	}
	if len(g.resolveCache) >= maxResolveCacheEntries {
		g.resolveCache = make(map[string]domain.ASInfo, 256)
	}
	if info == nil || info.ASN == 0 {
		g.resolveCache[ip] = domain.ASInfo{}
		return
	}
	g.resolveCache[ip] = *info
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

	record, err := lookupCityRecord(g.cityDB, parsed)
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

func lookupMMDB(asnDB, cityDB *geoip2.Reader, ip net.IP) (*domain.ASInfo, bool) {
	record, err := asnDB.ASN(ip)
	if err != nil || record == nil || record.AutonomousSystemNumber == 0 {
		return nil, false
	}
	return lookupMMDBFromRecord(record, cityDB, ip)
}

func lookupMMDBFromRecord(record *geoip2.ASN, cityDB *geoip2.Reader, ip net.IP) (*domain.ASInfo, bool) {
	if record == nil || record.AutonomousSystemNumber == 0 {
		return nil, false
	}

	org := record.AutonomousSystemOrganization
	info := &domain.ASInfo{
		ASN:       uint32(record.AutonomousSystemNumber),
		ShortName: shortenOrgName(org),
		OrgName:   org,
	}

	if cityDB != nil {
		applyCityRecord(cityDB, ip, info)
	}

	return info, true
}

func applyCityRecord(cityDB *geoip2.Reader, ip net.IP, info *domain.ASInfo) bool {
	if cityDB == nil || info == nil {
		return false
	}
	city, err := lookupCityRecord(cityDB, ip)
	if err != nil {
		return false
	}
	cc := strings.TrimSpace(city.Country.IsoCode)
	if cc == "" {
		return false
	}
	info.CountryCode = cc
	info.FlagEmoji = CountryToFlag(cc)
	return true
}

func (g *GeoIPDB) rememberASN(info *domain.ASInfo) {
	if info == nil || info.ASN == 0 {
		return
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.asnIndex == nil {
		g.asnIndex = make(map[uint32]domain.ASInfo)
	}
	if _, exists := g.asnIndex[info.ASN]; exists {
		return
	}
	g.asnIndex[info.ASN] = *info
}

func (g *GeoIPDB) lookupASNFromIndex(asn uint32) (*domain.ASInfo, bool) {
	g.mu.RLock()
	defer g.mu.RUnlock()
	if g.asnIndex == nil {
		return nil, false
	}
	info, ok := g.asnIndex[asn]
	if !ok || (info.OrgName == "" && info.ShortName == "" && info.CountryCode == "") {
		return nil, false
	}
	out := info
	out.ASN = asn
	return &out, true
}

func (g *GeoIPDB) resolveDNS(ctx context.Context, parsed net.IP, ip string) (*domain.ASInfo, error) {
	if parsed.To4() == nil {
		return &domain.ASInfo{}, nil
	}

	parts := strings.Split(ip, ".")
	if len(parts) != 4 {
		return &domain.ASInfo{}, nil
	}

	reversed := fmt.Sprintf("%s.%s.%s.%s.origin.asn.cymru.com", parts[3], parts[2], parts[1], parts[0])
	txtRecords, err := lookupOriginTXT(ctx, reversed)
	if err != nil {
		return nil, err
	}
	if len(txtRecords) == 0 {
		return &domain.ASInfo{}, nil
	}

	fields := strings.Split(txtRecords[0], " | ")
	info := &domain.ASInfo{}
	if len(fields) >= 1 {
		var asn uint32
		_, _ = fmt.Sscanf(strings.TrimSpace(fields[0]), "%d", &asn)
		info.ASN = asn
	}
	if len(fields) >= 3 {
		info.CountryCode = strings.TrimSpace(fields[2])
		info.FlagEmoji = CountryToFlag(info.CountryCode)
	}

	// Step 2: Resolve org name from ASN via asn.cymru.com
	if info.ASN > 0 {
		asnQuery := fmt.Sprintf("AS%d.asn.cymru.com", info.ASN)
		if asnTXT, err := lookupASNTXT(ctx, asnQuery); err == nil && len(asnTXT) > 0 {
			country, org := parseCymruASNRecord(asnTXT[0])
			if country != "" {
				info.CountryCode = country
				info.FlagEmoji = CountryToFlag(country)
			}
			if org != "" {
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

func (g *GeoIPDB) LookupASByNumber(ctx context.Context, asn uint32) (*domain.ASInfo, error) {
	if asn == 0 {
		return &domain.ASInfo{}, nil
	}
	if info, ok := g.lookupASNFromIndex(asn); ok {
		if info.OrgName != "" && info.CountryCode != "" {
			return info, nil
		}
		return g.mergeASByNumberDNS(ctx, info), nil
	}
	return g.lookupASByNumberDNS(ctx, asn)
}

func (g *GeoIPDB) mergeASByNumberDNS(ctx context.Context, info *domain.ASInfo) *domain.ASInfo {
	if info == nil || info.ASN == 0 {
		return info
	}
	dnsInfo, err := g.lookupASByNumberDNS(ctx, info.ASN)
	if err != nil {
		return info
	}
	if dnsInfo.CountryCode != "" {
		info.CountryCode = dnsInfo.CountryCode
		info.FlagEmoji = dnsInfo.FlagEmoji
	}
	if info.OrgName == "" && dnsInfo.OrgName != "" {
		info.OrgName = dnsInfo.OrgName
		info.ShortName = dnsInfo.ShortName
	}
	return info
}

func (g *GeoIPDB) lookupASByNumberDNS(ctx context.Context, asn uint32) (*domain.ASInfo, error) {
	info := &domain.ASInfo{ASN: asn}
	asnQuery := fmt.Sprintf("AS%d.asn.cymru.com", asn)
	txt, err := lookupASNTXT(ctx, asnQuery)
	if err != nil {
		return info, err
	}
	if len(txt) == 0 {
		return info, nil
	}
	country, org := parseCymruASNRecord(txt[0])
	if country == "" {
		country = countryFromOrgSuffix(org)
	}
	if country != "" {
		info.CountryCode = country
		info.FlagEmoji = CountryToFlag(country)
	}
	if org != "" {
		info.OrgName = org
		info.ShortName = shortenOrgName(org)
	}
	return info, nil
}

// parseCymruASNRecord parses Team Cymru ASN TXT records.
// ARIN format:  "ASN | Prefix | CC | Registry | Date | Name"
// RIPE format:  "ASN | CC | Registry | Date | Name"
func parseCymruASNRecord(record string) (country, org string) {
	record = strings.Trim(strings.TrimSpace(record), "\"")
	fields := strings.Split(record, " | ")
	if len(fields) < 2 {
		return "", ""
	}
	// ARIN-style when field 1 looks like a BGP prefix.
	if len(fields) >= 6 && strings.Contains(fields[1], "/") {
		country = strings.TrimSpace(fields[2])
		org = strings.TrimSpace(fields[5])
		return country, org
	}
	// RIPE-style (and other RIRs without embedded prefix).
	if len(fields) >= 5 {
		country = strings.TrimSpace(fields[1])
		org = strings.TrimSpace(fields[4])
		return country, org
	}
	if len(fields) >= 2 {
		country = strings.TrimSpace(fields[1])
	}
	return country, org
}

func countryFromOrgSuffix(org string) string {
	if idx := strings.LastIndex(org, ", "); idx >= 0 {
		cc := strings.ToUpper(strings.TrimSpace(org[idx+2:]))
		if len(cc) == 2 && cc[0] >= 'A' && cc[0] <= 'Z' && cc[1] >= 'A' && cc[1] <= 'Z' {
			return cc
		}
	}
	return ""
}

func (g *GeoIPDB) Reload() error {
	g.mu.Lock()
	defer g.mu.Unlock()

	if g.asnDB != nil {
		_ = g.asnDB.Close()
		g.asnDB = nil
	}
	if g.cityDB != nil {
		_ = g.cityDB.Close()
		g.cityDB = nil
	}

	g.enabled = false

	var reloadErr error
	if g.asnPath != "" {
		db, err := geoip2.Open(g.asnPath)
		if err != nil {
			reloadErr = fmt.Errorf("open ASN db: %w", err)
		} else {
			g.asnDB = db
			g.enabled = true
		}
	}
	if g.cityPath != "" {
		db, err := geoip2.Open(g.cityPath)
		if err != nil {
			cityErr := fmt.Errorf("open city db: %w", err)
			if reloadErr != nil {
				reloadErr = fmt.Errorf("%v; %w", reloadErr, cityErr)
			} else {
				reloadErr = cityErr
			}
		} else {
			g.cityDB = db
			g.enabled = true
		}
	}

	g.reloadASNAssetsLocked()
	return reloadErr
}

func asnIndexDir(asnPath, cityPath string) string {
	dbDir := filepath.Dir(asnPath)
	if dbDir == "" || dbDir == "." {
		dbDir = filepath.Dir(cityPath)
	}
	return dbDir
}

func (g *GeoIPDB) reloadASNAssetsLocked() {
	dir := asnIndexDir(g.asnPath, g.cityPath)
	g.asnIndex = buildASNIndex(dir)
	g.asnNetworkBlocks = buildASNNetworkBlocks(dir)
}

func (g *GeoIPDB) reloadASNIndex() {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.reloadASNAssetsLocked()
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
		flag = append(flag, 0x1F1E6+(c-'A'))
	}
	return string(flag)
}

// shortenOrgName extracts a display-friendly AS organization name.
func shortenOrgName(org string) string {
	org = baseOrgLabel(org)
	if org == "" {
		return org
	}
	if words := strings.Fields(org); len(words) > 1 {
		return words[0]
	}
	return org
}

const tracerouteOrgMaxLen = 20

// FormatTracerouteOrgName returns a compact ISP label for traceroute hop lines:
// up to two name parts, abbreviated when the result would be too long.
func FormatTracerouteOrgName(org string) string {
	org = baseOrgLabel(org)
	if org == "" {
		return org
	}
	words := trimLegalSuffixWords(strings.Fields(org))
	if len(words) == 0 {
		return ""
	}
	if len(words) == 1 {
		return abbreviateTracerouteName(words[0], tracerouteOrgMaxLen)
	}
	name := words[0] + " " + words[1]
	return abbreviateTracerouteName(name, tracerouteOrgMaxLen)
}

func baseOrgLabel(org string) string {
	org = strings.TrimSpace(org)
	if org == "" {
		return org
	}
	if idx := strings.Index(org, " - "); idx > 0 {
		before := strings.TrimSpace(org[:idx])
		after := strings.TrimSpace(org[idx+3:])
		if strings.HasPrefix(strings.ToUpper(before), "AS") && after != "" {
			org = after
		} else {
			org = before
		}
	}
	if idx := strings.Index(org, ", "); idx > 0 {
		org = strings.TrimSpace(org[:idx])
	}
	return org
}

var legalOrgSuffixes = map[string]struct{}{
	"inc": {}, "llc": {}, "ltd": {}, "corp": {}, "corporation": {},
	"co": {}, "as": {}, "sa": {}, "gmbh": {}, "ag": {},
	"plc": {}, "limited": {}, "bv": {}, "nv": {}, "spa": {}, "pty": {},
}

func trimLegalSuffixWords(words []string) []string {
	for len(words) > 0 {
		last := strings.ReplaceAll(strings.ToLower(words[len(words)-1]), ".", "")
		if _, ok := legalOrgSuffixes[last]; ok {
			words = words[:len(words)-1]
			continue
		}
		break
	}
	return words
}

func abbreviateTracerouteName(name string, maxLen int) string {
	if len(name) <= maxLen {
		return name
	}
	parts := strings.Fields(name)
	if len(parts) >= 2 {
		first, second := parts[0], parts[1]
		if len(first)+1+4 <= maxLen {
			remain := maxLen - len(first) - 1
			if remain > len(second) {
				remain = len(second)
			}
			return first + " " + second[:remain]
		}
		firstMax := maxLen / 2
		secondMax := maxLen - firstMax - 1
		if secondMax < 3 {
			secondMax = 3
			firstMax = maxLen - 4
		}
		return trimToLen(first, firstMax) + " " + trimToLen(second, secondMax)
	}
	return trimToLen(name, maxLen)
}

func trimToLen(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	if maxLen <= 0 {
		return ""
	}
	return s[:maxLen]
}
