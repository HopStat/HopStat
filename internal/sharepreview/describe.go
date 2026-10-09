package sharepreview

import (
	"strconv"
	"strings"
)

// maxPreviewASPath caps the AS path in a description; a preview line is not a route dump.
const maxPreviewASPath = 8

// Describe writes the share title and description for a query link in lang. Turkish and
// English are written out; any other language reads the English text.
func Describe(lang string, q Query, nodeName, siteName string, snap *Snapshot) (title, description string) {
	tr := lang == "tr"
	title = queryTitle(tr, q.Command, q.Target, nodeName)
	if snap == nil || snap.Command != q.Command {
		return title, invitation(tr, q.Command, q.Target, nodeName, siteName)
	}

	var outcome string
	switch q.Command {
	case "ping":
		outcome = describePing(tr, q.Target, snap)
	case "traceroute":
		outcome = describeTraceroute(tr, snap)
	case "bgp_route":
		outcome = describeBGP(tr, q.Target, nodeName, snap)
	}
	if outcome == "" {
		return title, invitation(tr, q.Command, q.Target, nodeName, siteName)
	}

	stamp := snap.At.UTC().Format("2006-01-02 15:04") + " UTC"
	if tr {
		return title, outcome + ". Son ölçüm: " + stamp + "."
	}
	return title, outcome + ". Last run " + stamp + "."
}

func queryTitle(tr bool, command, target, node string) string {
	if tr {
		from := ""
		if node != "" {
			from = node + " noktasından "
		}
		switch command {
		case "ping":
			return from + target + " ping"
		case "traceroute":
			return from + target + " traceroute"
		default:
			if node != "" {
				return node + " üzerinde " + target + " BGP rotası"
			}
			return target + " BGP rotası"
		}
	}
	from := ""
	if node != "" {
		from = " from " + node
	}
	switch command {
	case "ping":
		return "Ping " + target + from
	case "traceroute":
		return "Traceroute to " + target + from
	default:
		if node != "" {
			return "BGP route for " + target + " on " + node
		}
		return "BGP route for " + target
	}
}

// invitation is the description of a link no finished query has described yet.
func invitation(tr bool, command, target, node, site string) string {
	if tr {
		from := ""
		if node != "" {
			from = node + " noktasından "
		}
		switch command {
		case "ping":
			return site + " üzerinden " + from + target + " hedefine canlı ping atın."
		case "traceroute":
			return site + " üzerinden " + from + target + " hedefine canlı traceroute çalıştırın."
		default:
			return site + " üzerinden " + target + " için canlı BGP rotasını sorgulayın."
		}
	}
	from := ""
	if node != "" {
		from = " from " + node
	}
	switch command {
	case "ping":
		return "Run a live ping to " + target + from + " on " + site + "."
	case "traceroute":
		return "Run a live traceroute to " + target + from + " on " + site + "."
	default:
		return "Look up the live BGP route for " + target + " on " + site + "."
	}
}

func describePing(tr bool, target string, s *Snapshot) string {
	if s.Recv == 0 {
		if tr {
			return target + " yanıt vermedi (" + strconv.Itoa(s.Sent) + " paket gönderildi)"
		}
		return "No replies from " + target + " (" + strconv.Itoa(s.Sent) + " sent)"
	}
	replies := strconv.Itoa(s.Recv) + "/" + strconv.Itoa(s.Sent)
	loss := number(tr, s.LossPct, 0)
	if tr {
		return replies + " yanıt, %" + loss + " kayıp · ort. " + ms(tr, s.AvgRTT) +
			" (min " + number(tr, s.MinRTT, 1) + ", maks " + ms(tr, s.MaxRTT) + ")"
	}
	return replies + " replies, " + loss + "% loss · avg " + ms(tr, s.AvgRTT) +
		" (min " + number(tr, s.MinRTT, 1) + ", max " + ms(tr, s.MaxRTT) + ")"
}

func describeTraceroute(tr bool, s *Snapshot) string {
	hops := strconv.Itoa(s.Hops)
	if s.LastRTT == 0 {
		if tr {
			return hops + " hop boyunca yanıt gelmedi"
		}
		return "No hop answered within " + hops + " hops"
	}
	parts := []string{}
	switch {
	case s.Reached && tr:
		parts = append(parts, hops+" hopta ulaşıldı", ms(tr, s.LastRTT))
	case s.Reached:
		parts = append(parts, "Reached in "+hops+" hops", ms(tr, s.LastRTT))
	case tr:
		parts = append(parts, hops+" hop izlendi, hedef yanıt vermedi", "son yanıt "+ms(tr, s.LastRTT))
	default:
		parts = append(parts, hops+" hops traced, target did not answer", "last reply "+ms(tr, s.LastRTT))
	}
	if s.LastAS != "" {
		parts = append(parts, s.LastAS)
	}
	return strings.Join(parts, " · ")
}

func describeBGP(tr bool, target, node string, s *Snapshot) string {
	if s.NoRoute {
		if tr {
			if node != "" {
				return node + " üzerinde " + target + " için rota yok"
			}
			return target + " için rota yok"
		}
		if node != "" {
			return "No route for " + target + " on " + node
		}
		return "No route for " + target
	}
	parts := []string{}
	if s.Prefix != "" {
		parts = append(parts, s.Prefix)
	}
	if len(s.ASPath) > 0 {
		path := s.ASPath
		more := ""
		if len(path) > maxPreviewASPath {
			path, more = path[:maxPreviewASPath], " …"
		}
		asns := make([]string, len(path))
		for i, asn := range path {
			asns[i] = strconv.FormatUint(uint64(asn), 10)
		}
		label := "AS path "
		if tr {
			label = "AS yolu "
		}
		parts = append(parts, label+strings.Join(asns, " ")+more)
	}
	if s.OriginAS != "" {
		if tr {
			parts = append(parts, "köken "+s.OriginAS)
		} else {
			parts = append(parts, "origin "+s.OriginAS)
		}
	}
	return strings.Join(parts, " · ")
}

func ms(tr bool, v float64) string {
	return number(tr, v, 1) + " ms"
}

// number writes v with the decimal separator of the language: 11.9 in English, 11,9 in Turkish.
func number(tr bool, v float64, decimals int) string {
	s := strconv.FormatFloat(v, 'f', decimals, 64)
	if tr {
		s = strings.Replace(s, ".", ",", 1)
	}
	return s
}
