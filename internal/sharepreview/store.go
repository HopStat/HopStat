package sharepreview

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"time"
)

// Retention is how long a snapshot can describe a link. Past that the preview falls back to
// naming the query, because a months-old latency figure misleads more than it informs.
const Retention = 30 * 24 * time.Hour

// pruneEvery spaces out the delete of expired snapshots; Save runs it at most this often.
const pruneEvery = 6 * time.Hour

// Store keeps one snapshot per node, command and target in the share_snapshots table.
type Store struct {
	db  *sql.DB
	now func() time.Time

	pruneMu   sync.Mutex
	lastPrune time.Time
}

func NewStore(db *sql.DB) *Store {
	return &Store{db: db, now: time.Now}
}

// NormalizeTarget folds a target the way a share link spells it — buildQueryPath in the
// frontend drops empty path segments — so the link and the query land on the same row.
func NormalizeTarget(target string) string {
	parts := strings.Split(strings.TrimSpace(target), "/")
	kept := parts[:0]
	for _, p := range parts {
		if p != "" {
			kept = append(kept, p)
		}
	}
	return strings.ToLower(strings.Join(kept, "/"))
}

// Save records the latest outcome for a node, command and target, replacing the last one.
func (s *Store) Save(ctx context.Context, nodeID int64, target string, snap *Snapshot) error {
	if s == nil || s.db == nil || snap == nil {
		return nil
	}
	// A struct of plain fields cannot fail to marshal.
	payload, _ := json.Marshal(snap)
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO share_snapshots (node_id, command, target, snapshot, updated_at)
		VALUES (?, ?, ?, ?, ?)
		ON CONFLICT (node_id, command, target)
		DO UPDATE SET snapshot = excluded.snapshot, updated_at = excluded.updated_at`,
		nodeID, snap.Command, NormalizeTarget(target), string(payload), s.now().UTC(),
	)
	if err != nil {
		return err
	}
	s.pruneIfDue(ctx)
	return nil
}

// Load returns the snapshot for a node, command and target, or nil when there is none
// recent enough to describe the link.
func (s *Store) Load(ctx context.Context, nodeID int64, command, target string) (*Snapshot, error) {
	if s == nil || s.db == nil {
		return nil, nil
	}
	var raw string
	err := s.db.QueryRowContext(ctx,
		`SELECT snapshot FROM share_snapshots WHERE node_id = ? AND command = ? AND target = ?`,
		nodeID, command, NormalizeTarget(target),
	).Scan(&raw)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var snap Snapshot
	if err := json.Unmarshal([]byte(raw), &snap); err != nil {
		return nil, err
	}
	if s.now().Sub(snap.At) > Retention {
		return nil, nil
	}
	return &snap, nil
}

func (s *Store) pruneIfDue(ctx context.Context) {
	s.pruneMu.Lock()
	now := s.now()
	if now.Sub(s.lastPrune) < pruneEvery {
		s.pruneMu.Unlock()
		return
	}
	s.lastPrune = now
	s.pruneMu.Unlock()
	_, _ = s.db.ExecContext(ctx, `DELETE FROM share_snapshots WHERE updated_at < ?`, now.Add(-Retention).UTC())
}
