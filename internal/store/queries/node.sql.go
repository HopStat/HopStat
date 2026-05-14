package queries

import (
	"context"
	"database/sql"
)

type Node struct {
	ID           int64
	Name         string
	Description  string
	Type         string
	City         string
	Country      string
	Lat          sql.NullFloat64
	Lon          sql.NullFloat64
	CredentialID sql.NullInt64
	Active       int
	EnabledCmds  string
	BGPConfig    sql.NullString
	AgentURL     string
	AgentToken   string
	CreatedAt    string
	UpdatedAt    string
}

const nodeCols = `id, name, description, type, city, country, lat, lon, credential_id, active, enabled_cmds, bgp_config, agent_url, agent_token, created_at, updated_at`

func scanNode(scanner interface{ Scan(...interface{}) error }, n *Node) error {
	return scanner.Scan(&n.ID, &n.Name, &n.Description, &n.Type, &n.City, &n.Country,
		&n.Lat, &n.Lon, &n.CredentialID, &n.Active, &n.EnabledCmds, &n.BGPConfig,
		&n.AgentURL, &n.AgentToken, &n.CreatedAt, &n.UpdatedAt)
}

func (q *Queries) GetAllNodes(ctx context.Context) ([]Node, error) {
	rows, err := q.db.QueryContext(ctx, `SELECT `+nodeCols+` FROM nodes ORDER BY name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var nodes []Node
	for rows.Next() {
		var n Node
		if err := scanNode(rows, &n); err != nil {
			return nil, err
		}
		nodes = append(nodes, n)
	}
	return nodes, nil
}

func (q *Queries) GetActiveNodes(ctx context.Context) ([]Node, error) {
	rows, err := q.db.QueryContext(ctx, `SELECT `+nodeCols+` FROM nodes WHERE active = 1 ORDER BY name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var nodes []Node
	for rows.Next() {
		var n Node
		if err := scanNode(rows, &n); err != nil {
			return nil, err
		}
		nodes = append(nodes, n)
	}
	return nodes, nil
}

func (q *Queries) GetNodeByID(ctx context.Context, id int64) (*Node, error) {
	row := q.db.QueryRowContext(ctx, `SELECT `+nodeCols+` FROM nodes WHERE id = ?`, id)
	var n Node
	err := scanNode(row, &n)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &n, nil
}

func (q *Queries) CreateNode(ctx context.Context, arg *Node) (*Node, error) {
	result, err := q.db.ExecContext(ctx, `
		INSERT INTO nodes (name, description, type, city, country, lat, lon, credential_id, active, enabled_cmds,
		                   bgp_config, agent_url, agent_token, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)
	`, arg.Name, arg.Description, arg.Type, arg.City, arg.Country, arg.Lat, arg.Lon, arg.CredentialID,
		arg.Active, arg.EnabledCmds, arg.BGPConfig, arg.AgentURL, arg.AgentToken)
	if err != nil {
		return nil, err
	}
	id, err := result.LastInsertId()
	if err != nil {
		return nil, err
	}
	return q.GetNodeByID(ctx, id)
}

func (q *Queries) UpdateNode(ctx context.Context, arg *Node) (*Node, error) {
	_, err := q.db.ExecContext(ctx, `
		UPDATE nodes SET name = ?, description = ?, type = ?, city = ?, country = ?, lat = ?, lon = ?,
		                 credential_id = ?, active = ?, enabled_cmds = ?, bgp_config = ?, agent_url = ?,
		                 agent_token = ?, updated_at = CURRENT_TIMESTAMP
		WHERE id = ?
	`, arg.Name, arg.Description, arg.Type, arg.City, arg.Country, arg.Lat, arg.Lon,
		arg.CredentialID, arg.Active, arg.EnabledCmds, arg.BGPConfig, arg.AgentURL, arg.AgentToken, arg.ID)
	if err != nil {
		return nil, err
	}
	return q.GetNodeByID(ctx, arg.ID)
}

func (q *Queries) DeleteNode(ctx context.Context, id int64) error {
	_, err := q.db.ExecContext(ctx, `DELETE FROM nodes WHERE id = ?`, id)
	return err
}
