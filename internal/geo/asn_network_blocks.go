package geo

import (
	"encoding/csv"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/HopStat/HopStat/internal/domain"
)

type asnNetworkBlock struct {
	network  *net.IPNet
	maskOnes int
	info     domain.ASInfo
}

func loadASNNetworkBlocks(blocksPath string, countryByGeoname map[int64]string) ([]asnNetworkBlock, error) {
	f, err := os.Open(blocksPath)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	r := csv.NewReader(f)
	header, err := r.Read()
	if err != nil {
		return nil, fmt.Errorf("read blocks header: %w", err)
	}

	networkIdx, asnIdx, orgIdx, geonameIdx, regCountryIdx := -1, -1, -1, -1, -1
	for i, col := range header {
		switch strings.TrimSpace(col) {
		case "network":
			networkIdx = i
		case "autonomous_system_number":
			asnIdx = i
		case "autonomous_system_organization":
			orgIdx = i
		case "geoname_id":
			geonameIdx = i
		case "registered_country_geoname_id":
			regCountryIdx = i
		}
	}
	if networkIdx < 0 || asnIdx < 0 {
		return nil, fmt.Errorf("blocks csv missing required columns")
	}

	out := make([]asnNetworkBlock, 0, 4096)
	for {
		row, err := r.Read()
		if err == io.EOF {
			break
		}
		if err != nil || len(row) <= networkIdx || len(row) <= asnIdx {
			continue
		}
		networkStr := strings.TrimSpace(row[networkIdx])
		if networkStr == "" {
			continue
		}
		_, ipNet, err := net.ParseCIDR(networkStr)
		if err != nil || ipNet == nil {
			continue
		}
		asnStr := strings.TrimSpace(row[asnIdx])
		if asnStr == "" {
			continue
		}
		asn64, err := strconv.ParseUint(asnStr, 10, 32)
		if err != nil || asn64 == 0 {
			continue
		}
		org := ""
		if orgIdx >= 0 && len(row) > orgIdx {
			org = strings.TrimSpace(row[orgIdx])
		}
		info := domain.ASInfo{
			ASN:       uint32(asn64),
			OrgName:   org,
			ShortName: shortenOrgName(org),
		}
		if cc := countryFromGeonameRow(row, geonameIdx, regCountryIdx, countryByGeoname); cc != "" {
			info.CountryCode = cc
			info.FlagEmoji = CountryToFlag(cc)
		}
		maskOnes, _ := ipNet.Mask.Size()
		out = append(out, asnNetworkBlock{
			network:  ipNet,
			maskOnes: maskOnes,
			info:     info,
		})
	}

	sort.Slice(out, func(i, j int) bool {
		return out[i].maskOnes > out[j].maskOnes
	})
	return out, nil
}

func countryFromGeonameRow(row []string, geonameIdx, regCountryIdx int, countryByGeoname map[int64]string) string {
	if countryByGeoname == nil {
		return ""
	}
	if geonameIdx >= 0 && len(row) > geonameIdx {
		if geoname, err := strconv.ParseInt(strings.TrimSpace(row[geonameIdx]), 10, 64); err == nil && geoname > 0 {
			if cc := countryByGeoname[geoname]; cc != "" {
				return cc
			}
		}
	}
	if regCountryIdx >= 0 && len(row) > regCountryIdx {
		if geoname, err := strconv.ParseInt(strings.TrimSpace(row[regCountryIdx]), 10, 64); err == nil && geoname > 0 {
			if cc := countryByGeoname[geoname]; cc != "" {
				return cc
			}
		}
	}
	return ""
}

func lookupLongestASNBlock(blocks []asnNetworkBlock, ip net.IP) (domain.ASInfo, bool) {
	info, _, ok := lookupLongestASNBlockNetwork(blocks, ip)
	return info, ok
}

func lookupLongestASNBlockNetwork(blocks []asnNetworkBlock, ip net.IP) (domain.ASInfo, string, bool) {
	if len(blocks) == 0 || ip == nil {
		return domain.ASInfo{}, "", false
	}
	for _, block := range blocks {
		if block.network.Contains(ip) {
			return block.info, block.network.String(), true
		}
	}
	return domain.ASInfo{}, "", false
}

func buildASNNetworkBlocks(dbDir string) []asnNetworkBlock {
	if dbDir == "" {
		return nil
	}
	var countryByGeoname map[int64]string
	if locPath := filepath.Join(dbDir, cityLocationsName); locPath != "" {
		if m, err := loadCountryByGeoname(locPath); err == nil {
			countryByGeoname = m
		}
	}
	merged := make([]asnNetworkBlock, 0, 4096)
	for _, name := range []string{asnBlocksIPv4Name, asnBlocksIPv6Name} {
		path := filepath.Join(dbDir, name)
		part, err := loadASNNetworkBlocks(path, countryByGeoname)
		if err != nil {
			continue
		}
		merged = append(merged, part...)
	}
	if len(merged) == 0 {
		return nil
	}
	sort.Slice(merged, func(i, j int) bool {
		return merged[i].maskOnes > merged[j].maskOnes
	})
	return merged
}

func pickASNInfo(mm *domain.ASInfo, mmOk bool, block domain.ASInfo, blockOk bool) *domain.ASInfo {
	if blockOk && block.ASN > 0 {
		out := block
		return &out
	}
	if mmOk && mm != nil && mm.ASN > 0 {
		return mm
	}
	return nil
}

func mergeASInfoPreferExisting(base, fallback *domain.ASInfo) *domain.ASInfo {
	if base == nil {
		return fallback
	}
	if fallback == nil {
		return base
	}
	out := *base
	if out.ASN == 0 {
		out.ASN = fallback.ASN
	}
	if strings.TrimSpace(out.OrgName) == "" {
		out.OrgName = fallback.OrgName
	}
	if strings.TrimSpace(out.ShortName) == "" {
		out.ShortName = fallback.ShortName
	}
	if strings.TrimSpace(out.CountryCode) == "" {
		out.CountryCode = fallback.CountryCode
	}
	if out.FlagEmoji == "" {
		out.FlagEmoji = fallback.FlagEmoji
	}
	return &out
}
