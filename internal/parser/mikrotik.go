package parser

import (
	"strconv"
	"strings"

	"github.com/HopStat/HopStat/internal/domain"
)

type MikroTikParser struct{}

func (p *MikroTikParser) ParseBGPRoute(raw string) (*domain.BGPResult, error) {
	return parseBGPRouteGeneric(raw)
}

func (p *MikroTikParser) ParsePing(raw string) (*domain.PingResult, error) {
	result := &domain.PingResult{Raw: raw}
	lines := splitLines(raw)

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if strings.Contains(line, "seq=") {
			result.PacketsSent++
			var rttSum float64
			var rttCount int
			fields := strings.Fields(line)
			for _, f := range fields {
				if strings.HasPrefix(f, "time=") {
					if t, err := strconv.ParseFloat(strings.TrimPrefix(f, "time="), 64); err == nil {
						rttSum += t
						rttCount++
						result.PacketsRecv++
					}
				}
			}
			if rttCount > 0 {
				result.AvgRTT = rttSum / float64(rttCount)
			}
		}
		if strings.Contains(line, "packets transmitted") {
			fields := strings.Fields(line)
			for i, f := range fields {
				if f == "transmitted" && i > 0 {
					result.PacketsSent, _ = strconv.Atoi(fields[i-1])
				}
				if f == "received" && i > 0 {
					result.PacketsRecv, _ = strconv.Atoi(fields[i-1])
				}
				if f == "loss" && i > 0 {
					loss := strings.TrimSuffix(fields[i-1], "%")
					result.PacketLoss, _ = strconv.ParseFloat(loss, 64)
				}
			}
		}
		if strings.Contains(line, "min/avg/max") {
			if idx := strings.Index(line, "="); idx != -1 {
				stats := strings.TrimSpace(line[idx+1:])
				parts := strings.Split(stats, "/")
				if len(parts) >= 3 {
					result.MinRTT, _ = strconv.ParseFloat(parts[0], 64)
					result.AvgRTT, _ = strconv.ParseFloat(parts[1], 64)
					result.MaxRTT, _ = strconv.ParseFloat(parts[2], 64)
				}
			}
		}
	}

	if result.PacketsSent > 0 && result.PacketLoss == 0 {
		result.PacketLoss = float64(result.PacketsSent-result.PacketsRecv) / float64(result.PacketsSent) * 100
	}

	return result, nil
}

func (p *MikroTikParser) ParseTraceroute(raw string) (*domain.TracerouteResult, error) {
	return parseTracerouteGeneric(raw)
}

func (p *MikroTikParser) ParseMTR(raw string) (*domain.MTRResult, error) {
	return parseMTRGeneric(raw)
}
