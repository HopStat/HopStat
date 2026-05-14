package parser

import (
	"net"
	"strconv"
	"strings"

	"github.com/HopStat/HopStat/internal/domain"
)

func splitLines(s string) []string {
	return strings.Split(strings.TrimSpace(s), "\n")
}

func containsNoRoute(s string) bool {
	noRouteIndicators := []string{
		"no route",
		"not in table",
		"network not in table",
		"no such network",
		"% Network not in",
	}
	s = strings.ToLower(s)
	for _, indicator := range noRouteIndicators {
		if strings.Contains(s, indicator) {
			return true
		}
	}
	return false
}

func parseBGPLine(line string) *domain.BGPRoute {
	line = strings.TrimSpace(line)
	if line == "" || strings.HasPrefix(line, "*") == false && strings.HasPrefix(line, " ") == false {
		return nil
	}

	route := &domain.BGPRoute{}

	fields := strings.Fields(line)
	if len(fields) < 2 {
		return nil
	}

	prefix := fields[0]
	if strings.HasPrefix(prefix, "*>") {
		prefix = strings.TrimPrefix(prefix, "*>")
	}
	if strings.HasPrefix(prefix, "*") {
		prefix = strings.TrimPrefix(prefix, "*")
	}
	if strings.HasPrefix(prefix, "i") || strings.HasPrefix(prefix, "e") || strings.HasPrefix(prefix, "?") {
		if len(fields) < 3 {
			return nil
		}
		prefix = fields[1]
		route.Origin = fields[len(fields)-1]
	}

	if _, _, err := net.ParseCIDR(prefix); err != nil {
		return nil
	}
	route.Prefix = prefix

	return route
}

func parseBGPRouteGeneric(raw string) (*domain.BGPResult, error) {
	result := &domain.BGPResult{Raw: raw}

	lines := splitLines(raw)
	for _, line := range lines {
		if route := parseBGPLine(line); route != nil {
			result.Routes = append(result.Routes, *route)
		}
	}

	return result, nil
}

func parsePingGeneric(raw string) (*domain.PingResult, error) {
	result := &domain.PingResult{Raw: raw}

	lines := splitLines(raw)
	for _, line := range lines {
		line = strings.TrimSpace(line)

		if strings.Contains(line, "packets transmitted") || strings.Contains(line, "packets transmitted,") {
			normalized := strings.ReplaceAll(line, ",", "")
			fields := strings.Fields(normalized)
			for i, f := range fields {
				switch f {
				case "transmitted":
					if i >= 2 {
						if n, err := strconv.Atoi(fields[i-2]); err == nil {
							result.PacketsSent = n
						}
					}
				case "received":
					// GNU: "N received", BusyBox: "N packets received"
					if i > 0 {
						val := fields[i-1]
						if val == "packets" && i >= 2 {
							val = fields[i-2]
						}
						if n, err := strconv.Atoi(val); err == nil {
							result.PacketsRecv = n
						}
					}
				case "loss":
					if i > 0 {
						if loss := strings.TrimSuffix(fields[i-1], "%"); loss != fields[i-1] {
							if n, err := strconv.ParseFloat(loss, 64); err == nil {
								result.PacketLoss = n
							}
						}
					}
				}
			}
		}

		if strings.Contains(line, "rtt") || strings.Contains(line, "min/avg/max") {
			if idx := strings.Index(line, "="); idx != -1 {
				stats := strings.TrimSpace(line[idx+1:])
				stats = strings.TrimPrefix(stats, "mdev=")
				stats = strings.TrimSuffix(strings.TrimSpace(stats), "ms")
				stats = strings.TrimSpace(stats)
				parts := strings.Split(stats, "/")
				if len(parts) >= 3 {
					if v, err := strconv.ParseFloat(parts[0], 64); err == nil {
						result.MinRTT = v
					}
					if v, err := strconv.ParseFloat(parts[1], 64); err == nil {
						result.AvgRTT = v
					}
					if v, err := strconv.ParseFloat(parts[2], 64); err == nil {
						result.MaxRTT = v
					}
				}
			}
		}

		// MikroTik format
		if strings.Contains(line, "seq=") {
			fields := strings.Fields(line)
			for i, f := range fields {
				if strings.HasPrefix(f, "time=") {
					if t, err := strconv.ParseFloat(strings.TrimPrefix(f, "time="), 64); err == nil {
						result.AvgRTT = t
					}
				}
				if f == "loss=" {
					if i+1 < len(fields) {
						if loss := strings.TrimSuffix(fields[i+1], "%"); loss != fields[i+1] {
							if n, err := strconv.ParseFloat(loss, 64); err == nil {
								result.PacketLoss = n
							}
						}
					}
				}
			}
		}
	}

	if result.PacketsSent > 0 && result.PacketsRecv > 0 && result.PacketLoss == 0 {
		result.PacketLoss = float64(result.PacketsSent-result.PacketsRecv) / float64(result.PacketsSent) * 100
	}

	return result, nil
}

func parseTracerouteGeneric(raw string) (*domain.TracerouteResult, error) {
	result := &domain.TracerouteResult{Raw: raw}
	lines := splitLines(raw)

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		if strings.HasPrefix(line, "traceroute") || strings.HasPrefix(line, "Traceroute") {
			continue
		}

		hop := parseTracerouteLine(line)
		if hop != nil {
			result.Hops = append(result.Hops, *hop)
		}
	}

	return result, nil
}

func parseTracerouteLine(line string) *domain.Hop {
	hop := &domain.Hop{}

	line = strings.TrimLeft(line, " \t")

	// Extract hop number from leading digits (before spaces/dots)
	rest := strings.TrimLeft(line, "0123456789")
	if n, err := strconv.Atoi(strings.TrimSpace(line[:len(line)-len(rest)])); err == nil {
		hop.Number = n
	}
	rest = strings.TrimLeft(rest, ". \t")

	fields := strings.Fields(rest)
	if len(fields) < 1 {
		return nil
	}

	for _, field := range fields {
		field = strings.Trim(field, "()")
		if ip := net.ParseIP(field); ip != nil {
			hop.IP = field
			break
		}
	}

	for _, field := range fields {
		field = strings.Trim(field, "()")
		if strings.HasSuffix(field, "ms") || strings.HasSuffix(field, "ms*") {
			rttStr := strings.TrimSuffix(field, "*")
			rttStr = strings.TrimSuffix(rttStr, "ms")
			if rtt, err := strconv.ParseFloat(rttStr, 64); err == nil {
				hop.RTT = append(hop.RTT, rtt)
			}
		}
	}

	if hop.IP == "" && len(fields) > 0 {
		hop.Host = fields[0]
	}

	return hop
}

func parseMTRGeneric(raw string) (*domain.MTRResult, error) {
	result := &domain.MTRResult{Raw: raw}
	lines := splitLines(raw)

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "Start:") || strings.HasPrefix(line, "HOST:") {
			continue
		}

		hop := parseMTRLine(line)
		if hop != nil {
			result.Hops = append(result.Hops, *hop)
		}
	}

	return result, nil
}

func parseMTRLine(line string) *domain.MTRHop {
	hop := &domain.MTRHop{}

	fields := strings.Fields(line)
	if len(fields) < 9 {
		return nil
	}

	first := strings.TrimSuffix(fields[0], ".")
	if n, err := strconv.Atoi(first); err == nil {
		hop.Number = n
	}

	hop.Host = fields[1]

	if v, err := strconv.ParseFloat(strings.TrimSuffix(fields[2], "%"), 64); err == nil {
		hop.Loss = v
	}
	if v, err := strconv.Atoi(fields[3]); err == nil {
		hop.Sent = v
	}
	if v, err := strconv.ParseFloat(fields[4], 64); err == nil {
		hop.Last = v
	}
	if v, err := strconv.ParseFloat(fields[5], 64); err == nil {
		hop.Avg = v
	}
	if v, err := strconv.ParseFloat(fields[6], 64); err == nil {
		hop.Best = v
	}
	if v, err := strconv.ParseFloat(fields[7], 64); err == nil {
		hop.Worst = v
	}
	if v, err := strconv.ParseFloat(fields[8], 64); err == nil {
		hop.StDev = v
	}

	hop.Recv = hop.Sent - int(float64(hop.Sent)*hop.Loss/100)

	return hop
}
