package repo

import (
	"context"
	"database/sql"

	"github.com/yourorg/lg-looking-glass/internal/domain"
	"github.com/yourorg/lg-looking-glass/internal/store/queries"
)

type communityRuleRepo struct {
	q *queries.Queries
}

func NewCommunityRuleRepo(db *sql.DB) domain.CommunityRuleRepository {
	return &communityRuleRepo{q: queries.New(db)}
}

func (r *communityRuleRepo) GetAll(ctx context.Context) ([]*domain.CommunityRule, error) {
	rules, err := r.q.GetAllCommunityRules(ctx)
	if err != nil {
		return nil, err
	}
	result := make([]*domain.CommunityRule, len(rules))
	for i, rule := range rules {
		result[i] = mapCommunityRule(&rule)
	}
	return result, nil
}

func (r *communityRuleRepo) GetActiveRulesForNode(ctx context.Context, nodeID int64) ([]*domain.CommunityRule, error) {
	rules, err := r.q.GetActiveCommunityRulesForNode(ctx, nodeID)
	if err != nil {
		return nil, err
	}
	result := make([]*domain.CommunityRule, len(rules))
	for i, rule := range rules {
		result[i] = mapCommunityRule(&rule)
	}
	return result, nil
}

func (r *communityRuleRepo) Create(ctx context.Context, rule *domain.CommunityRule) (*domain.CommunityRule, error) {
	created, err := r.q.CreateCommunityRule(ctx, &queries.CommunityRule{
		Community:   rule.Community,
		Severity:    string(rule.Severity),
		MessageI18n: rule.MessageI18n,
		Scope:       rule.Scope,
		NodeID:      nullInt64(rule.NodeID),
		Active:      boolToInt(rule.Active),
	})
	if err != nil {
		return nil, err
	}
	return mapCommunityRule(created), nil
}

func (r *communityRuleRepo) Update(ctx context.Context, rule *domain.CommunityRule) (*domain.CommunityRule, error) {
	updated, err := r.q.UpdateCommunityRule(ctx, &queries.CommunityRule{
		ID:          rule.ID,
		Community:   rule.Community,
		Severity:    string(rule.Severity),
		MessageI18n: rule.MessageI18n,
		Scope:       rule.Scope,
		NodeID:      nullInt64(rule.NodeID),
		Active:      boolToInt(rule.Active),
	})
	if err != nil {
		return nil, err
	}
	return mapCommunityRule(updated), nil
}

func (r *communityRuleRepo) Delete(ctx context.Context, id int64) error {
	return r.q.DeleteCommunityRule(ctx, id)
}

func (r *communityRuleRepo) Toggle(ctx context.Context, id int64) error {
	return r.q.ToggleCommunityRule(ctx, id)
}

func mapCommunityRule(r *queries.CommunityRule) *domain.CommunityRule {
	rule := &domain.CommunityRule{
		ID:          r.ID,
		Community:   r.Community,
		Severity:    domain.Severity(r.Severity),
		MessageI18n: r.MessageI18n,
		Scope:       r.Scope,
		Active:      r.Active == 1,
	}
	if r.NodeID.Valid {
		rule.NodeID = &r.NodeID.Int64
	}
	return rule
}
