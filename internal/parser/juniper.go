package parser

import (
	"strings"

	"github.com/HopStat/HopStat/internal/domain"
)

type JuniperParser struct{}

func (p *JuniperParser) ParseBGPRoute(raw string) (*domain.BGPResult, error) {
	result := &domain.BGPResult{Raw: raw}
	if containsNoRoute(raw) {
		return result, nil
	}

	lines := splitLines(raw)
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "*") && !strings.HasPrefix(line, " ") {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}
		route := &domain.BGPRoute{}
		for i, f := range fields {
			if strings.Contains(f, "/") && isPrefix(f) {
				route.Prefix = f
			}
			if i > 0 && fields[i-1] == "to" {
				route.NextHop = f
			}
			if f == "IGP" || f == "EGP" || f == "Incomplete" {
				route.Origin = f
			}
		}
		if route.Prefix != "" {
			result.Routes = append(result.Routes, *route)
		}
	}

	return result, nil
}

func (p *JuniperParser) ParsePing(raw string) (*domain.PingResult, error) {
	return parsePingGeneric(raw)
}

func (p *JuniperParser) ParseTraceroute(raw string) (*domain.TracerouteResult, error) {
	return parseTracerouteGeneric(raw)
}

func (p *JuniperParser) ParseMTR(raw string) (*domain.MTRResult, error) {
	return parseMTRGeneric(raw)
}

func isPrefix(s string) bool {
	return strings.Contains(s, "/") && strings.Count(s, ".") >= 2
}
