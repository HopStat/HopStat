package parser

import (
	"net"
	"regexp"
	"strconv"
	"strings"

	"github.com/HopStat/HopStat/internal/domain"
)

type BirdParser struct{}

func (p *BirdParser) ParseBGPRoute(raw string) (*domain.BGPResult, error) {
	result := &domain.BGPResult{Raw: raw}
	if containsNoRoute(raw) {
		return result, nil
	}

	var currentPrefix string
	for _, line := range splitLines(raw) {
		if route, prefix := parseBirdRouteLine(line, currentPrefix); route != nil {
			result.Routes = append(result.Routes, *route)
			currentPrefix = prefix
		}
	}

	return result, nil
}

func parseBirdRouteLine(line, currentPrefix string) (*domain.BGPRoute, string) {
	if strings.TrimSpace(line) == "" {
		return nil, currentPrefix
	}

	trimmed := strings.TrimLeft(line, " \t")
	best := false
	for len(trimmed) > 0 && (trimmed[0] == '*' || trimmed[0] == '>') {
		best = true
		trimmed = strings.TrimLeft(trimmed[1:], " \t")
	}

	fields := strings.Fields(trimmed)
	if len(fields) == 0 {
		return nil, currentPrefix
	}

	prefix := currentPrefix
	fieldStart := 0
	if strings.Contains(fields[0], "/") && isPrefix(fields[0]) {
		prefix = fields[0]
		fieldStart = 1
	}
	if prefix == "" {
		return nil, currentPrefix
	}

	// Standard BIRD verbose lines: "<prefix> via <nh> [AS...]"
	if route := parseBirdVerboseRoute(trimmed, prefix, best); route != nil {
		return route, prefix
	}

	if fieldStart >= len(fields) {
		return nil, prefix
	}

	route := parseBirdTabularFields(fields[fieldStart:], prefix, best)
	if route == nil {
		return nil, prefix
	}
	return route, prefix
}

func parseBirdVerboseRoute(line, prefix string, best bool) *domain.BGPRoute {
	asnRe := regexp.MustCompile(`\(AS(\d+)\)`)
	if !strings.Contains(line, " via ") && !asnRe.MatchString(line) {
		return nil
	}

	route := &domain.BGPRoute{Prefix: prefix, Best: best}
	fields := strings.Fields(line)
	for i, f := range fields {
		if f == "via" && i+1 < len(fields) {
			route.NextHop = strings.TrimSuffix(fields[i+1], ",")
		}
		if strings.HasPrefix(f, "[AS") {
			asn := strings.TrimSuffix(strings.TrimPrefix(f, "[AS"), "]")
			route.ASPath = append(route.ASPath, parseUint32(asn))
		}
	}
	for _, m := range asnRe.FindAllStringSubmatch(line, -1) {
		route.ASPath = append(route.ASPath, parseUint32(m[1]))
	}

	commRe := regexp.MustCompile(`\((\d+:\d+)\)`)
	for _, cm := range commRe.FindAllStringSubmatch(line, -1) {
		route.Communities = append(route.Communities, cm[1])
	}

	if route.NextHop == "" && len(route.ASPath) == 0 && len(route.Communities) == 0 {
		return nil
	}
	return route
}

func parseBirdTabularFields(fields []string, prefix string, best bool) *domain.BGPRoute {
	if len(fields) == 0 {
		return nil
	}

	route := &domain.BGPRoute{Prefix: prefix, Best: best}
	idx := 0

	if net.ParseIP(fields[0]) != nil {
		route.NextHop = fields[0]
		idx = 1
	}

	for idx < len(fields) {
		field := fields[idx]
		if isBGPOriginToken(field) {
			route.Origin = normalizeBGPOrigin(field)
			break
		}
		if n, err := strconv.ParseUint(field, 10, 32); err == nil {
			if route.NextHop != "" && route.LocalPref == 0 && idx == 1 {
				route.LocalPref = uint32(n)
				idx++
				continue
			}
			route.ASPath = append(route.ASPath, uint32(n))
			idx++
			continue
		}
		idx++
	}

	if route.NextHop == "" && len(route.ASPath) == 0 {
		return nil
	}
	return route
}

func isBGPOriginToken(s string) bool {
	switch strings.ToUpper(s) {
	case "I", "E", "?", "IGP", "EGP", "INCOMPLETE":
		return true
	default:
		return false
	}
}

func normalizeBGPOrigin(s string) string {
	switch strings.ToUpper(s) {
	case "I", "IGP":
		return "IGP"
	case "E", "EGP":
		return "EGP"
	case "?", "INCOMPLETE":
		return "incomplete"
	default:
		return s
	}
}

func (p *BirdParser) ParsePing(raw string) (*domain.PingResult, error) {
	return parsePingGeneric(raw)
}

func (p *BirdParser) ParseTraceroute(raw string) (*domain.TracerouteResult, error) {
	return parseTracerouteGeneric(raw)
}

func parseUint32(s string) uint32 {
	n, _ := strconv.ParseUint(strings.TrimSpace(s), 10, 32)
	return uint32(n)
}
