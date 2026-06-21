package sitecache

import (
	"context"
	"database/sql"

	"github.com/HopStat/HopStat/internal/domain"
	"github.com/HopStat/HopStat/internal/store/repo"
)

type cachedNodeRepo struct {
	inner domain.NodeRepository
}

func NewCachedNodeRepo(db *sql.DB, credKey string) domain.NodeRepository {
	return &cachedNodeRepo{inner: repo.NewNodeRepo(db, credKey)}
}

func (r *cachedNodeRepo) GetByID(ctx context.Context, id int64) (*domain.Node, error) {
	if node, ok := NodeByID(id); ok {
		return node, nil
	}
	return r.inner.GetByID(ctx, id)
}

func (r *cachedNodeRepo) GetAll(ctx context.Context) ([]*domain.Node, error) {
	return r.inner.GetAll(ctx)
}

func (r *cachedNodeRepo) GetActive(ctx context.Context) ([]*domain.Node, error) {
	return r.inner.GetActive(ctx)
}

func (r *cachedNodeRepo) Create(ctx context.Context, node *domain.Node) (*domain.Node, error) {
	return r.inner.Create(ctx, node)
}

func (r *cachedNodeRepo) Update(ctx context.Context, node *domain.Node) (*domain.Node, error) {
	return r.inner.Update(ctx, node)
}

func (r *cachedNodeRepo) SetDefault(ctx context.Context, id int64) error {
	return r.inner.SetDefault(ctx, id)
}

func (r *cachedNodeRepo) Delete(ctx context.Context, id int64) error {
	return r.inner.Delete(ctx, id)
}

func (r *cachedNodeRepo) UpdateEnabledCmds(ctx context.Context, id int64, cmds []domain.CommandType) error {
	return r.inner.UpdateEnabledCmds(ctx, id, cmds)
}

type cachedCommunityRuleRepo struct {
	inner domain.CommunityRuleRepository
}

func NewCachedCommunityRuleRepo(db *sql.DB) domain.CommunityRuleRepository {
	return &cachedCommunityRuleRepo{inner: repo.NewCommunityRuleRepo(db)}
}

func (r *cachedCommunityRuleRepo) GetActiveRulesForNode(ctx context.Context, nodeID int64) ([]*domain.CommunityRule, error) {
	if rules, ok := ActiveCommunityRulesForNode(nodeID); ok {
		return rules, nil
	}
	return r.inner.GetActiveRulesForNode(ctx, nodeID)
}

func (r *cachedCommunityRuleRepo) GetAll(ctx context.Context) ([]*domain.CommunityRule, error) {
	return r.inner.GetAll(ctx)
}

func (r *cachedCommunityRuleRepo) GetActive(ctx context.Context) ([]*domain.CommunityRule, error) {
	return r.inner.GetActive(ctx)
}

func (r *cachedCommunityRuleRepo) Create(ctx context.Context, rule *domain.CommunityRule) (*domain.CommunityRule, error) {
	return r.inner.Create(ctx, rule)
}

func (r *cachedCommunityRuleRepo) Update(ctx context.Context, rule *domain.CommunityRule) (*domain.CommunityRule, error) {
	return r.inner.Update(ctx, rule)
}

func (r *cachedCommunityRuleRepo) Delete(ctx context.Context, id int64) error {
	return r.inner.Delete(ctx, id)
}

func (r *cachedCommunityRuleRepo) Toggle(ctx context.Context, id int64) error {
	return r.inner.Toggle(ctx, id)
}
