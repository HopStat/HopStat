package parser

import (
	"regexp"
	"strings"

	"github.com/HopStat/HopStat/internal/domain"
)

type CiscoParser struct{}

func (p *CiscoParser) ParseBGPRoute(raw string) (*domain.BGPResult, error) {
	result := &domain.BGPResult{Raw: raw}
	if containsNoRoute(raw) {
		return result, nil
	}

	lines := splitLines(raw)
	re := regexp.MustCompile(`^([*>e ]+)\s+(\S+)\s+(\S+)\s+(\d+)\s+(\d+)\s+(\S+)`)

	for _, line := range lines {
		line = strings.TrimSpace(line)
		matches := re.FindStringSubmatch(line)
		if matches == nil {
			continue
		}
		route := &domain.BGPRoute{
			Prefix:    matches[2],
			NextHop:   matches[3],
			LocalPref: parseUint32(matches[4]),
			MED:       parseUint32(matches[5]),
			Origin:    matches[6],
		}

		asPathRe := regexp.MustCompile(`\s(\d+(?:\s\d+)*)\s`)
		asMatches := asPathRe.FindStringSubmatch(line)
		if asMatches != nil {
			for _, s := range strings.Fields(asMatches[1]) {
				route.ASPath = append(route.ASPath, parseUint32(s))
			}
		}

		result.Routes = append(result.Routes, *route)
	}

	return result, nil
}

func (p *CiscoParser) ParsePing(raw string) (*domain.PingResult, error) {
	return parsePingGeneric(raw)
}

func (p *CiscoParser) ParseTraceroute(raw string) (*domain.TracerouteResult, error) {
	return parseTracerouteGeneric(raw)
}

func (p *CiscoParser) ParseMTR(raw string) (*domain.MTRResult, error) {
	return parseMTRGeneric(raw)
}
