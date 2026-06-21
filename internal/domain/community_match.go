package domain

import "strings"

// MatchCommunityRules returns active rules whose community string appears in communities.
func MatchCommunityRules(rules []*CommunityRule, communities []string) []*CommunityRule {
	if len(rules) == 0 || len(communities) == 0 {
		return nil
	}
	set := make(map[string]struct{}, len(communities))
	for _, c := range communities {
		if norm := normalizeCommunity(c); norm != "" {
			set[norm] = struct{}{}
		}
	}
	var matched []*CommunityRule
	for _, rule := range rules {
		if rule == nil || !rule.Active {
			continue
		}
		if _, ok := set[normalizeCommunity(rule.Community)]; ok {
			matched = append(matched, rule)
		}
	}
	return matched
}

func normalizeCommunity(c string) string {
	return strings.TrimSpace(strings.ToLower(c))
}
