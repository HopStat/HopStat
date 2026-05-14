package parser

import (
	"github.com/HopStat/HopStat/internal/domain"
)

type GenericParser struct{}

func (p *GenericParser) ParseBGPRoute(raw string) (*domain.BGPResult, error) {
	return parseBGPRouteGeneric(raw)
}

func (p *GenericParser) ParsePing(raw string) (*domain.PingResult, error) {
	return parsePingGeneric(raw)
}

func (p *GenericParser) ParseTraceroute(raw string) (*domain.TracerouteResult, error) {
	return parseTracerouteGeneric(raw)
}

func (p *GenericParser) ParseMTR(raw string) (*domain.MTRResult, error) {
	return parseMTRGeneric(raw)
}
