package sitecache

import (
	"context"
	"database/sql"
	"sync"

	"github.com/HopStat/HopStat/internal/domain"
	"github.com/HopStat/HopStat/internal/store/repo"
)

// PublicCommunity is the public API shape for community rules.
type PublicCommunity struct {
	ID          int64  `json:"id"`
	Community   string `json:"community"`
	Severity    string `json:"severity"`
	MessageI18n string `json:"message_i18n"`
	Scope       string `json:"scope"`
}

var (
	communitiesMu         sync.RWMutex
	cachedCommunities     []PublicCommunity
	cachedCommunityRules  []*domain.CommunityRule
	loadActiveCommunityRules = func(db *sql.DB) ([]*domain.CommunityRule, error) {
		r := repo.NewCommunityRuleRepo(db)
		return r.GetActive(context.Background())
	}
)

func RefreshCommunities(db *sql.DB) error {
	rules, err := loadActiveCommunityRules(db)
	if err != nil {
		return err
	}
	out := make([]PublicCommunity, 0, len(rules))
	full := make([]*domain.CommunityRule, 0, len(rules))
	for _, rule := range rules {
		if rule == nil {
			continue
		}
		out = append(out, PublicCommunity{
			ID:          rule.ID,
			Community:   rule.Community,
			Severity:    string(rule.Severity),
			MessageI18n: rule.MessageI18n,
			Scope:       rule.Scope,
		})
		full = append(full, rule)
	}
	communitiesMu.Lock()
	cachedCommunities = out
	cachedCommunityRules = full
	communitiesMu.Unlock()
	return nil
}

// ActiveCommunityRulesForNode returns cached rules matching global scope or the node.
// The bool is false when the community cache has not been loaded yet.
func ActiveCommunityRulesForNode(nodeID int64) ([]*domain.CommunityRule, bool) {
	communitiesMu.RLock()
	defer communitiesMu.RUnlock()
	if cachedCommunityRules == nil {
		return nil, false
	}
	if len(cachedCommunityRules) == 0 {
		return nil, true
	}
	out := make([]*domain.CommunityRule, 0, len(cachedCommunityRules))
	for _, rule := range cachedCommunityRules {
		if rule == nil || !rule.Active {
			continue
		}
		if rule.Scope == "global" || (rule.NodeID != nil && *rule.NodeID == nodeID) {
			out = append(out, rule)
		}
	}
	return out, true
}

// ActiveCommunities returns cached active community rules.
func ActiveCommunities() []PublicCommunity {
	communitiesMu.RLock()
	defer communitiesMu.RUnlock()
	if len(cachedCommunities) == 0 {
		return []PublicCommunity{}
	}
	out := make([]PublicCommunity, len(cachedCommunities))
	copy(out, cachedCommunities)
	return out
}
