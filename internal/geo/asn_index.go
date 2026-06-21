package geo

import (
	"encoding/csv"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/HopStat/HopStat/internal/domain"
)

const (
	asnBlocksIPv4Name = "GeoLite2-ASN-Blocks-IPv4.csv"
	asnBlocksIPv6Name = "GeoLite2-ASN-Blocks-IPv6.csv"
	cityLocationsName = "GeoLite2-City-Locations-en.csv"
)

func loadCountryByGeoname(locationsPath string) (map[int64]string, error) {
	f, err := os.Open(locationsPath)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	r := csv.NewReader(f)
	header, err := r.Read()
	if err != nil {
		return nil, fmt.Errorf("read locations header: %w", err)
	}
	geonameIdx, countryIdx := -1, -1
	for i, col := range header {
		switch strings.TrimSpace(col) {
		case "geoname_id":
			geonameIdx = i
		case "country_iso_code":
			countryIdx = i
		}
	}
	if geonameIdx < 0 || countryIdx < 0 {
		return nil, fmt.Errorf("locations csv missing required columns")
	}

	out := make(map[int64]string)
	for {
		row, err := r.Read()
		if err == io.EOF {
			break
		}
		if err != nil || len(row) <= geonameIdx || len(row) <= countryIdx {
			continue
		}
		id, err := strconv.ParseInt(strings.TrimSpace(row[geonameIdx]), 10, 64)
		if err != nil || id == 0 {
			continue
		}
		cc := strings.ToUpper(strings.TrimSpace(row[countryIdx]))
		if len(cc) == 2 {
			out[id] = cc
		}
	}
	return out, nil
}

func loadASNIndexFromBlocks(blocksPath string, countryByGeoname map[int64]string) (map[uint32]domain.ASInfo, error) {
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
	asnIdx, orgIdx, regCountryIdx := -1, -1, -1
	for i, col := range header {
		switch strings.TrimSpace(col) {
		case "autonomous_system_number":
			asnIdx = i
		case "autonomous_system_organization":
			orgIdx = i
		case "registered_country_geoname_id":
			regCountryIdx = i
		}
	}
	if asnIdx < 0 || orgIdx < 0 {
		return nil, fmt.Errorf("blocks csv missing required columns")
	}

	out := make(map[uint32]domain.ASInfo)
	for {
		row, err := r.Read()
		if err == io.EOF {
			break
		}
		if err != nil || len(row) <= asnIdx || len(row) <= orgIdx {
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
		asn := uint32(asn64)
		if _, exists := out[asn]; exists {
			continue
		}
		org := strings.TrimSpace(row[orgIdx])
		info := domain.ASInfo{
			ASN:       asn,
			OrgName:   org,
			ShortName: shortenOrgName(org),
		}
		if regCountryIdx >= 0 && len(row) > regCountryIdx && countryByGeoname != nil {
			if geoname, err := strconv.ParseInt(strings.TrimSpace(row[regCountryIdx]), 10, 64); err == nil && geoname > 0 {
				if cc := countryByGeoname[geoname]; cc != "" {
					info.CountryCode = cc
					info.FlagEmoji = CountryToFlag(cc)
				}
			}
		}
		out[asn] = info
	}
	return out, nil
}

func buildASNIndex(dbDir string) map[uint32]domain.ASInfo {
	if dbDir == "" {
		return nil
	}
	var countryByGeoname map[int64]string
	if locPath := filepath.Join(dbDir, cityLocationsName); locPath != "" {
		if m, err := loadCountryByGeoname(locPath); err == nil {
			countryByGeoname = m
		}
	}

	merged := make(map[uint32]domain.ASInfo)
	for _, name := range []string{asnBlocksIPv4Name, asnBlocksIPv6Name} {
		path := filepath.Join(dbDir, name)
		part, err := loadASNIndexFromBlocks(path, countryByGeoname)
		if err != nil {
			continue
		}
		for asn, info := range part {
			if _, exists := merged[asn]; !exists {
				merged[asn] = info
			}
		}
	}
	if len(merged) == 0 {
		return nil
	}
	return merged
}
