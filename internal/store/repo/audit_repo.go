package repo

import (
	"context"
	"database/sql"

	"github.com/yourorg/lg-looking-glass/internal/domain"
	"github.com/yourorg/lg-looking-glass/internal/store/queries"
)

type auditRepo struct {
	q *queries.Queries
}

func NewAuditRepo(db *sql.DB) domain.AuditRepository {
	return &auditRepo{q: queries.New(db)}
}

func (r *auditRepo) Log(ctx context.Context, entry *domain.AuditEntry) error {
	success := 1
	if !entry.Success {
		success = 0
	}

	return r.q.CreateAuditLog(ctx, &queries.AuditLog{
		SourceIP:   entry.SourceIP,
		UserID:     nullInt64(entry.UserID),
		NodeID:     nullInt64(entry.NodeID),
		Command:    entry.Command,
		Params:     entry.Params,
		DurationMS: entry.DurationMS,
		Success:    success,
		ErrorMsg:   entry.ErrorMsg,
	})
}

func (r *auditRepo) List(ctx context.Context, filter domain.AuditFilter) ([]*domain.AuditEntry, int, error) {
	logs, total, err := r.q.ListAuditLogs(ctx, &queries.AuditFilter{
		From:     filter.From,
		To:       filter.To,
		NodeID:   filter.NodeID,
		Command:  filter.Command,
		SourceIP: filter.SourceIP,
		Page:     filter.Page,
		Limit:    filter.Limit,
	})
	if err != nil {
		return nil, 0, err
	}

	entries := make([]*domain.AuditEntry, len(logs))
	for i, l := range logs {
		entries[i] = mapAuditLog(&l)
	}
	return entries, total, nil
}

func (r *auditRepo) Cleanup(ctx context.Context, olderThan string) (int64, error) {
	return r.q.CleanupAuditLogs(ctx, olderThan)
}

func mapAuditLog(l *queries.AuditLog) *domain.AuditEntry {
	e := &domain.AuditEntry{
		ID:         l.ID,
		CreatedAt:  parseTime(l.CreatedAt),
		SourceIP:   l.SourceIP,
		Command:    l.Command,
		Params:     l.Params,
		DurationMS: l.DurationMS,
		Success:    l.Success == 1,
		ErrorMsg:   l.ErrorMsg,
	}
	if l.UserID.Valid {
		e.UserID = &l.UserID.Int64
	}
	if l.NodeID.Valid {
		e.NodeID = &l.NodeID.Int64
	}
	return e
}

func nullInt64(v *int64) sql.NullInt64 {
	if v == nil {
		return sql.NullInt64{}
	}
	return sql.NullInt64{Int64: *v, Valid: true}
}