package queries

import (
	"context"
	"database/sql"
)

type BGPNeighbor struct {
	ID             int64
	NodeID         int64
	LocalAS        uint32
	RemoteAS       uint32
	PeeringIP      string
	NeighborIP     string
	IPv6PeeringIP  string
	IPv6NeighborIP string
	Multihop       int
	CreatedAt      string
	UpdatedAt      string
}

const selectCols = `id, node_id, local_as, remote_as, peering_ip, neighbor_ip, ipv6_peering_ip, ipv6_neighbor_ip, multihop, created_at, updated_at`

func scanNeighbor(rows interface {
	Scan(...any) error
}, n *BGPNeighbor) error {
	return rows.Scan(&n.ID, &n.NodeID, &n.LocalAS, &n.RemoteAS, &n.PeeringIP, &n.NeighborIP, &n.IPv6PeeringIP, &n.IPv6NeighborIP, &n.Multihop, &n.CreatedAt, &n.UpdatedAt)
}

func (q *Queries) GetAllBGPNeighbors(ctx context.Context) ([]BGPNeighbor, error) {
	rows, err := q.db.QueryContext(ctx, `SELECT `+selectCols+` FROM bgp_neighbors ORDER BY created_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var neighbors []BGPNeighbor
	for rows.Next() {
		var n BGPNeighbor
		if err := scanNeighbor(rows, &n); err != nil {
			return nil, err
		}
		neighbors = append(neighbors, n)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return neighbors, nil
}

func (q *Queries) GetBGPNeighborsByNodeID(ctx context.Context, nodeID int64) ([]BGPNeighbor, error) {
	rows, err := q.db.QueryContext(ctx, `SELECT `+selectCols+` FROM bgp_neighbors WHERE node_id = ?`, nodeID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var neighbors []BGPNeighbor
	for rows.Next() {
		var n BGPNeighbor
		if err := scanNeighbor(rows, &n); err != nil {
			return nil, err
		}
		neighbors = append(neighbors, n)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return neighbors, nil
}

func (q *Queries) GetBGPNeighborByID(ctx context.Context, id int64) (*BGPNeighbor, error) {
	row := q.db.QueryRowContext(ctx, `SELECT `+selectCols+` FROM bgp_neighbors WHERE id = ?`, id)
	var n BGPNeighbor
	err := scanNeighbor(row, &n)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &n, nil
}

func (q *Queries) CreateBGPNeighbor(ctx context.Context, arg *BGPNeighbor) (*BGPNeighbor, error) {
	result, err := q.db.ExecContext(ctx, `
		INSERT INTO bgp_neighbors (node_id, local_as, remote_as, peering_ip, neighbor_ip, ipv6_peering_ip, ipv6_neighbor_ip, multihop, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)
	`, arg.NodeID, arg.LocalAS, arg.RemoteAS, arg.PeeringIP, arg.NeighborIP, arg.IPv6PeeringIP, arg.IPv6NeighborIP, arg.Multihop)
	if err != nil {
		return nil, err
	}
	id, err := result.LastInsertId()
	if err != nil {
		return nil, err
	}
	return q.GetBGPNeighborByID(ctx, id)
}

func (q *Queries) UpdateBGPNeighbor(ctx context.Context, arg *BGPNeighbor) (*BGPNeighbor, error) {
	_, err := q.db.ExecContext(ctx, `
		UPDATE bgp_neighbors SET node_id = ?, local_as = ?, remote_as = ?, peering_ip = ?, neighbor_ip = ?,
		ipv6_peering_ip = ?, ipv6_neighbor_ip = ?, multihop = ?, updated_at = CURRENT_TIMESTAMP
		WHERE id = ?
	`, arg.NodeID, arg.LocalAS, arg.RemoteAS, arg.PeeringIP, arg.NeighborIP, arg.IPv6PeeringIP, arg.IPv6NeighborIP, arg.Multihop, arg.ID)
	if err != nil {
		return nil, err
	}
	return q.GetBGPNeighborByID(ctx, arg.ID)
}

func (q *Queries) DeleteBGPNeighbor(ctx context.Context, id int64) error {
	_, err := q.db.ExecContext(ctx, `DELETE FROM bgp_neighbors WHERE id = ?`, id)
	return err
}
