package repo

import (
	"context"
	"database/sql"
	"encoding/json"
	"log/slog"

	"github.com/HopStat/HopStat/internal/domain"
	"github.com/HopStat/HopStat/internal/store/queries"
)

type nodeRepo struct {
	q            *queries.Queries
	credentialKey string
}

func NewNodeRepo(db *sql.DB, credentialKey string) domain.NodeRepository {
	return &nodeRepo{q: queries.New(db), credentialKey: credentialKey}
}

func (r *nodeRepo) encryptToken(token string) string {
	if token == "" || r.credentialKey == "" {
		return token
	}
	enc, err := Encrypt(token, r.credentialKey)
	if err != nil {
		return token
	}
	return enc
}

func (r *nodeRepo) decryptToken(token string) string {
	if token == "" || r.credentialKey == "" {
		return token
	}
	dec, err := Decrypt(token, r.credentialKey)
	if err != nil {
		// Not yet encrypted (plaintext legacy value) — return as-is
		return token
	}
	return dec
}

func (r *nodeRepo) GetAll(ctx context.Context) ([]*domain.Node, error) {
	nodes, err := r.q.GetAllNodes(ctx)
	if err != nil {
		return nil, err
	}
	result := mapNodes(nodes)
	for _, n := range result {
		n.AgentToken = r.decryptToken(n.AgentToken)
	}
	return result, nil
}

func (r *nodeRepo) GetActive(ctx context.Context) ([]*domain.Node, error) {
	if err := r.ensureDefault(ctx); err != nil {
		return nil, err
	}
	nodes, err := r.q.GetActiveNodes(ctx)
	if err != nil {
		return nil, err
	}
	result := mapNodes(nodes)
	for _, n := range result {
		n.AgentToken = r.decryptToken(n.AgentToken)
	}
	return result, nil
}

func (r *nodeRepo) GetByID(ctx context.Context, id int64) (*domain.Node, error) {
	node, err := r.q.GetNodeByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if node == nil {
		return nil, domain.ErrNodeNotFound
	}
	n := mapNode(node)
	n.AgentToken = r.decryptToken(n.AgentToken)
	return n, nil
}

func (r *nodeRepo) Create(ctx context.Context, node *domain.Node) (*domain.Node, error) {
	node.EnabledCmds = domain.NormalizeEnabledCmds(node.EnabledCmds)
	enabledCmds, _ := json.Marshal(node.EnabledCmds)
	created, err := r.q.CreateNode(ctx, &queries.Node{
		Name:        node.Name,
		Description: node.Description,
		Type:        string(node.Type),
		City:        node.City,
		Country:     node.Country,
		Lat:         nullFloat64(node.Lat),
		Active:      boolToInt(node.Active),
		IsDefault:   boolToInt(node.IsDefault),
		EnabledCmds: string(enabledCmds),
		AgentURL:    node.AgentURL,
		AgentToken:  r.encryptToken(node.AgentToken),
	})
	if err != nil {
		return nil, err
	}
	if node.IsDefault {
		if err := r.q.SetDefaultNode(ctx, created.ID); err != nil {
			return nil, err
		}
	} else if err := r.ensureDefaultAfterCreate(ctx, created.ID); err != nil {
		return nil, err
	}
	refreshed, err := r.q.GetNodeByID(ctx, created.ID)
	if err == nil && refreshed != nil {
		created = refreshed
	}
	n := mapNode(created)
	n.AgentToken = r.decryptToken(n.AgentToken)
	return n, nil
}

func (r *nodeRepo) Update(ctx context.Context, node *domain.Node) (*domain.Node, error) {
	node.EnabledCmds = domain.NormalizeEnabledCmds(node.EnabledCmds)
	enabledCmds, _ := json.Marshal(node.EnabledCmds)
	updated, err := r.q.UpdateNode(ctx, &queries.Node{
		ID:          node.ID,
		Name:        node.Name,
		Description: node.Description,
		Type:        string(node.Type),
		City:        node.City,
		Country:     node.Country,
		Lat:         nullFloat64(node.Lat),
		Lon:         nullFloat64(node.Lon),
		Active:      boolToInt(node.Active),
		IsDefault:   boolToInt(node.IsDefault),
		EnabledCmds: string(enabledCmds),
		AgentURL:    node.AgentURL,
		AgentToken:  r.encryptToken(node.AgentToken),
	})
	if err != nil {
		return nil, err
	}
	n := mapNode(updated)
	n.AgentToken = r.decryptToken(n.AgentToken)
	return n, nil
}

func (r *nodeRepo) SetDefault(ctx context.Context, id int64) error {
	return r.q.SetDefaultNode(ctx, id)
}

func (r *nodeRepo) Delete(ctx context.Context, id int64) error {
	if err := r.q.DeleteNode(ctx, id); err != nil {
		return err
	}
	return r.ensureDefault(ctx)
}

func (r *nodeRepo) ensureDefaultAfterCreate(ctx context.Context, newID int64) error {
	return r.ensureDefault(ctx)
}

func (r *nodeRepo) ensureDefault(ctx context.Context) error {
	nodes, err := r.q.GetAllNodes(ctx)
	if err != nil {
		return err
	}
	if len(nodes) == 0 {
		return nil
	}
	for _, n := range nodes {
		if n.IsDefault == 1 {
			return nil
		}
	}
	minID := nodes[0].ID
	for _, n := range nodes {
		if n.ID < minID {
			minID = n.ID
		}
	}
	return r.q.SetDefaultNode(ctx, minID)
}

func (r *nodeRepo) UpdateEnabledCmds(ctx context.Context, id int64, cmds []domain.CommandType) error {
	cmds = domain.NormalizeEnabledCmds(cmds)
	enabledCmds, _ := json.Marshal(cmds)
	node, err := r.q.GetNodeByID(ctx, id)
	if err != nil {
		return err
	}
	// AgentToken is already encrypted in the DB; pass it through unchanged.
	node.EnabledCmds = string(enabledCmds)
	_, err = r.q.UpdateNode(ctx, node)
	return err
}

func mapNodes(rows []queries.Node) []*domain.Node {
	nodes := make([]*domain.Node, len(rows))
	for i, r := range rows {
		nodes[i] = mapNode(&r)
	}
	return nodes
}

func mapNode(r *queries.Node) *domain.Node {
	n := &domain.Node{
		ID:          r.ID,
		Name:        r.Name,
		Description: r.Description,
		Type:        domain.NodeType(r.Type),
		City:        r.City,
		Country:     r.Country,
		Active:      r.Active == 1,
		IsDefault:   r.IsDefault == 1,
		AgentURL:    r.AgentURL,
		AgentToken:  r.AgentToken,
	}
	if r.Lat.Valid {
		n.Lat = &r.Lat.Float64
	}
	if r.Lon.Valid {
		n.Lon = &r.Lon.Float64
	}
	if r.CredentialID.Valid {
		n.CredentialID = &r.CredentialID.Int64
	}
	if err := json.Unmarshal([]byte(r.EnabledCmds), &n.EnabledCmds); err != nil {
		slog.Warn("failed to parse enabled_cmds for node", "node_id", r.ID, "error", err)
	}
	n.EnabledCmds = domain.NormalizeEnabledCmds(n.EnabledCmds)
	n.CreatedAt = parseTime(r.CreatedAt)
	n.UpdatedAt = parseTime(r.UpdatedAt)
	return n
}

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

func nullFloat64(v *float64) sql.NullFloat64 {
	if v == nil {
		return sql.NullFloat64{}
	}
	return sql.NullFloat64{Float64: *v, Valid: true}
}
