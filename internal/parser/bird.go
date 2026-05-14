package parser

import (
	"regexp"
	"strconv"
	"strings"

	"github.com/yourorg/lg-looking-glass/internal/domain"
)

type BirdParser struct{}

func (p *BirdParser) ParseBGPRoute(raw string) (*domain.BGPResult, error) {
	result := &domain.BGPResult{Raw: raw}
	if containsNoRoute(raw) {
		return result, nil
	}

	lines := splitLines(raw)
	asnRe := regexp.MustCompile(`\(AS(\d+)\)`)
	prefixRe := regexp.MustCompile(`^\s*(\d+\.\d+\.\d+\.\d+/\d+)\s+`)

	for _, line := range lines {
		line = strings.TrimSpace(line)

		if matches := prefixRe.FindStringSubmatch(line); matches != nil {
			route := &domain.BGPRoute{Prefix: matches[1]}
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

			result.Routes = append(result.Routes, *route)
		}
	}

	return result, nil
}

func (p *BirdParser) ParsePing(raw string) (*domain.PingResult, error) {
	return parsePingGeneric(raw)
}

func (p *BirdParser) ParseTraceroute(raw string) (*domain.TracerouteResult, error) {
	return parseTracerouteGeneric(raw)
}

func (p *BirdParser) ParseMTR(raw string) (*domain.MTRResult, error) {
	return parseMTRGeneric(raw)
}

func parseUint32(s string) uint32 {
	n, _ := strconv.ParseUint(strings.TrimSpace(s), 10, 32)
	return uint32(n)
}
