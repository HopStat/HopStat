package domain

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
)

// FormatQueryAuditParams builds a human-readable audit params string for a query.
func FormatQueryAuditParams(command, target, protocol string, pingCount, maxHops int) string {
	s := strings.TrimSpace(target)
	if s == "" {
		return "—"
	}

	var extras []string
	switch command {
	case "ping":
		if pingCount > 0 && pingCount != 5 {
			extras = append(extras, fmt.Sprintf("count %d", pingCount))
		}
	case "traceroute":
		if maxHops > 0 && maxHops != 30 {
			extras = append(extras, fmt.Sprintf("max hops %d", maxHops))
		}
	}
	if p := strings.TrimSpace(protocol); p != "" {
		extras = append(extras, p)
	}
	if len(extras) > 0 {
		return s + " · " + strings.Join(extras, " · ")
	}
	return s
}

// DisplayAuditParams normalizes stored audit params for display (handles legacy JSON).
func DisplayAuditParams(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "—"
	}
	if !strings.HasPrefix(raw, "{") {
		return raw
	}

	var obj map[string]any
	if err := json.Unmarshal([]byte(raw), &obj); err != nil {
		if target := legacyAuditTargetFromJSON(raw); target != "" {
			return target
		}
		return raw
	}

	target := legacyAuditTargetFromObject(obj)
	if target == "" {
		return raw
	}

	var extras []string
	for k, v := range obj {
		if k == "target" || k == "Target" {
			continue
		}
		if extra := formatLegacyAuditExtra(k, v); extra != "" {
			extras = append(extras, extra)
		}
	}
	if len(extras) > 0 {
		return target + " · " + strings.Join(extras, " · ")
	}
	return target
}

func legacyAuditTargetFromObject(obj map[string]any) string {
	if target, ok := obj["target"].(string); ok {
		return strings.TrimSpace(target)
	}
	if target, ok := obj["Target"].(string); ok {
		return strings.TrimSpace(target)
	}
	return ""
}

func legacyAuditTargetFromJSON(raw string) string {
	re := regexp.MustCompile(`"target"\s*:\s*"([^"]+)"`)
	if match := re.FindStringSubmatch(raw); len(match) > 1 {
		return strings.TrimSpace(match[1])
	}
	return ""
}

func formatLegacyAuditExtra(key string, v any) string {
	switch key {
	case "ping_count":
		if n, ok := asInt(v); ok {
			return fmt.Sprintf("count %d", n)
		}
	case "max_hops":
		if n, ok := asInt(v); ok {
			return fmt.Sprintf("max hops %d", n)
		}
	case "protocol":
		if s := strings.TrimSpace(fmt.Sprint(v)); s != "" {
			return s
		}
		return ""
	}
	if sv := strings.TrimSpace(fmt.Sprint(v)); sv != "" {
		return fmt.Sprintf("%s %s", key, sv)
	}
	return ""
}

func asInt(v any) (int, bool) {
	switch n := v.(type) {
	case float64:
		return int(n), true
	case int:
		return n, true
	case int64:
		return int(n), true
	case json.Number:
		i, err := n.Int64()
		if err != nil {
			return 0, false
		}
		return int(i), true
	default:
		return 0, false
	}
}
