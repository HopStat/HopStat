package parser

import (
	"github.com/yourorg/lg-looking-glass/internal/domain"
)

type OutputParser interface {
	ParseBGPRoute(raw string) (*domain.BGPResult, error)
	ParsePing(raw string) (*domain.PingResult, error)
	ParseTraceroute(raw string) (*domain.TracerouteResult, error)
	ParseMTR(raw string) (*domain.MTRResult, error)
}

func GetParser(vendor string) OutputParser {
	switch vendor {
	case "cisco":
		return &CiscoParser{}
	case "juniper":
		return &JuniperParser{}
	case "mikrotik":
		return &MikroTikParser{}
	case "bird":
		return &BirdParser{}
	default:
		return &GenericParser{}
	}
}