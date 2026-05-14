package repo

import (
	"context"
	"database/sql"
	"encoding/json"

	"github.com/HopStat/HopStat/internal/domain"
	"github.com/HopStat/HopStat/internal/store/queries"
)

type nodeRepo struct {
	q *queries.Queries
}

func NewNodeRepo(db *sql.DB) domain.NodeRepository {
	return &nodeRepo{q: queries.New(db)}
}

func (r *nodeRepo) GetAll(ctx context.Context) ([]*domain.Node, error) {
	nodes, err := r.q.GetAllNodes(ctx)
	if err != nil {
		return nil, err
	}
	return mapNodes(nodes), nil
}

func (r *nodeRepo) GetActive(ctx context.Context) ([]*domain.Node, error) {
	nodes, err := r.q.GetActiveNodes(ctx)
	if err != nil {
		return nil, err
	}
	return mapNodes(nodes), nil
}

func (r *nodeRepo) GetByID(ctx context.Context, id int64) (*domain.Node, error) {
	node, err := r.q.GetNodeByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if node == nil {
		return nil, domain.ErrNodeNotFound
	}
	return mapNode(node), nil
}

func (r *nodeRepo) Create(ctx context.Context, node *domain.Node) (*domain.Node, error) {
	enabledCmds, _ := json.Marshal(node.EnabledCmds)
	created, err := r.q.CreateNode(ctx, &queries.Node{
		Name:        node.Name,
		Description: node.Description,
		Type:        string(node.Type),
		City:        node.City,
		Country:     node.Country,
		Lat:         nullFloat64(node.Lat),
		Active:      boolToInt(node.Active),
		EnabledCmds: string(enabledCmds),
		AgentURL:    node.AgentURL,
		AgentToken:  node.AgentToken,
	})
	if err != nil {
		return nil, err
	}
	return mapNode(created), nil
}

func (r *nodeRepo) Update(ctx context.Context, node *domain.Node) (*domain.Node, error) {
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
		EnabledCmds: string(enabledCmds),
		AgentURL:    node.AgentURL,
		AgentToken:  node.AgentToken,
	})
	if err != nil {
		return nil, err
	}
	return mapNode(updated), nil
}

func (r *nodeRepo) Delete(ctx context.Context, id int64) error {
	return r.q.DeleteNode(ctx, id)
}

func (r *nodeRepo) UpdateEnabledCmds(ctx context.Context, id int64, cmds []domain.CommandType) error {
	enabledCmds, _ := json.Marshal(cmds)
	node, err := r.q.GetNodeByID(ctx, id)
	if err != nil {
		return err
	}
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
	json.Unmarshal([]byte(r.EnabledCmds), &n.EnabledCmds)
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
