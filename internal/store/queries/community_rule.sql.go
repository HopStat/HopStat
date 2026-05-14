package queries

import (
	"context"
	"database/sql"
)

type CommunityRule struct {
	ID          int64
	Community   string
	Severity    string
	MessageI18n string
	Scope       string
	NodeID      sql.NullInt64
	Active      int
	CreatedAt   string
	UpdatedAt   string
}

func (q *Queries) GetAllCommunityRules(ctx context.Context) ([]CommunityRule, error) {
	rows, err := q.db.QueryContext(ctx, `
		SELECT id, community, severity, message_i18n, scope, node_id, active, created_at, updated_at
		FROM community_rules ORDER BY created_at DESC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var rules []CommunityRule
	for rows.Next() {
		var r CommunityRule
		if err := rows.Scan(&r.ID, &r.Community, &r.Severity, &r.MessageI18n, &r.Scope, &r.NodeID, &r.Active, &r.CreatedAt, &r.UpdatedAt); err != nil {
			return nil, err
		}
		rules = append(rules, r)
	}
	return rules, nil
}

func (q *Queries) GetActiveCommunityRulesForNode(ctx context.Context, nodeID int64) ([]CommunityRule, error) {
	rows, err := q.db.QueryContext(ctx, `
		SELECT id, community, severity, message_i18n, scope, node_id, active, created_at, updated_at
		FROM community_rules
		WHERE active = 1 AND (scope = 'global' OR node_id = ?)
		ORDER BY created_at DESC
	`, nodeID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var rules []CommunityRule
	for rows.Next() {
		var r CommunityRule
		if err := rows.Scan(&r.ID, &r.Community, &r.Severity, &r.MessageI18n, &r.Scope, &r.NodeID, &r.Active, &r.CreatedAt, &r.UpdatedAt); err != nil {
			return nil, err
		}
		rules = append(rules, r)
	}
	return rules, nil
}

func (q *Queries) GetCommunityRuleByID(ctx context.Context, id int64) (*CommunityRule, error) {
	row := q.db.QueryRowContext(ctx, `
		SELECT id, community, severity, message_i18n, scope, node_id, active, created_at, updated_at
		FROM community_rules WHERE id = ?
	`, id)
	var r CommunityRule
	err := row.Scan(&r.ID, &r.Community, &r.Severity, &r.MessageI18n, &r.Scope, &r.NodeID, &r.Active, &r.CreatedAt, &r.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &r, nil
}

func (q *Queries) CreateCommunityRule(ctx context.Context, arg *CommunityRule) (*CommunityRule, error) {
	result, err := q.db.ExecContext(ctx, `
		INSERT INTO community_rules (community, severity, message_i18n, scope, node_id, active, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)
	`, arg.Community, arg.Severity, arg.MessageI18n, arg.Scope, arg.NodeID, arg.Active)
	if err != nil {
		return nil, err
	}
	id, err := result.LastInsertId()
	if err != nil {
		return nil, err
	}
	return q.GetCommunityRuleByID(ctx, id)
}

func (q *Queries) UpdateCommunityRule(ctx context.Context, arg *CommunityRule) (*CommunityRule, error) {
	_, err := q.db.ExecContext(ctx, `
		UPDATE community_rules SET community = ?, severity = ?, message_i18n = ?, scope = ?, node_id = ?, active = ?, updated_at = CURRENT_TIMESTAMP
		WHERE id = ?
	`, arg.Community, arg.Severity, arg.MessageI18n, arg.Scope, arg.NodeID, arg.Active, arg.ID)
	if err != nil {
		return nil, err
	}
	return q.GetCommunityRuleByID(ctx, arg.ID)
}

func (q *Queries) DeleteCommunityRule(ctx context.Context, id int64) error {
	_, err := q.db.ExecContext(ctx, `DELETE FROM community_rules WHERE id = ?`, id)
	return err
}

func (q *Queries) ToggleCommunityRule(ctx context.Context, id int64) error {
	_, err := q.db.ExecContext(ctx, `
		UPDATE community_rules SET active = CASE WHEN active = 1 THEN 0 ELSE 1 END, updated_at = CURRENT_TIMESTAMP
		WHERE id = ?
	`, id)
	return err
}
