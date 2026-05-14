package repo

import (
	"context"
	"database/sql"

	"github.com/HopStat/HopStat/internal/domain"
	"github.com/HopStat/HopStat/internal/store/queries"
)

type bgpNeighborRepo struct {
	q *queries.Queries
}

func NewBGPNeighborRepo(db *sql.DB) domain.BGPNeighborRepository {
	return &bgpNeighborRepo{q: queries.New(db)}
}

func (r *bgpNeighborRepo) GetAll(ctx context.Context) ([]*domain.BGPNeighbor, error) {
	neighbors, err := r.q.GetAllBGPNeighbors(ctx)
	if err != nil {
		return nil, err
	}
	result := make([]*domain.BGPNeighbor, len(neighbors))
	for i, n := range neighbors {
		result[i] = mapBGPNeighbor(&n)
	}
	return result, nil
}

func (r *bgpNeighborRepo) GetByNodeID(ctx context.Context, nodeID int64) ([]*domain.BGPNeighbor, error) {
	neighbors, err := r.q.GetBGPNeighborsByNodeID(ctx, nodeID)
	if err != nil {
		return nil, err
	}
	result := make([]*domain.BGPNeighbor, len(neighbors))
	for i, n := range neighbors {
		result[i] = mapBGPNeighbor(&n)
	}
	return result, nil
}

func (r *bgpNeighborRepo) GetByID(ctx context.Context, id int64) (*domain.BGPNeighbor, error) {
	n, err := r.q.GetBGPNeighborByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if n == nil {
		return nil, nil
	}
	return mapBGPNeighbor(n), nil
}

func (r *bgpNeighborRepo) Create(ctx context.Context, neighbor *domain.BGPNeighbor) (*domain.BGPNeighbor, error) {
	created, err := r.q.CreateBGPNeighbor(ctx, &queries.BGPNeighbor{
		NodeID:     neighbor.NodeID,
		LocalAS:    neighbor.LocalAS,
		RemoteAS:   neighbor.RemoteAS,
		PeeringIP:  neighbor.PeeringIP,
		NeighborIP: neighbor.NeighborIP,
		Multihop:   boolToInt(neighbor.Multihop),
	})
	if err != nil {
		return nil, err
	}
	return mapBGPNeighbor(created), nil
}

func (r *bgpNeighborRepo) Update(ctx context.Context, neighbor *domain.BGPNeighbor) (*domain.BGPNeighbor, error) {
	updated, err := r.q.UpdateBGPNeighbor(ctx, &queries.BGPNeighbor{
		ID:         neighbor.ID,
		NodeID:     neighbor.NodeID,
		LocalAS:    neighbor.LocalAS,
		RemoteAS:   neighbor.RemoteAS,
		PeeringIP:  neighbor.PeeringIP,
		NeighborIP: neighbor.NeighborIP,
		Multihop:   boolToInt(neighbor.Multihop),
	})
	if err != nil {
		return nil, err
	}
	return mapBGPNeighbor(updated), nil
}

func (r *bgpNeighborRepo) Delete(ctx context.Context, id int64) error {
	return r.q.DeleteBGPNeighbor(ctx, id)
}

func mapBGPNeighbor(n *queries.BGPNeighbor) *domain.BGPNeighbor {
	return &domain.BGPNeighbor{
		ID:         n.ID,
		NodeID:     n.NodeID,
		LocalAS:    n.LocalAS,
		RemoteAS:   n.RemoteAS,
		PeeringIP:  n.PeeringIP,
		NeighborIP: n.NeighborIP,
		Multihop:   n.Multihop == 1,
	}
}
