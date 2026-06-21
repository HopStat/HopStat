package queries

import (
	"context"
	"database/sql"
)

type QuickQuery struct {
	ID        int64
	Command   string
	Name      string
	Target    string
	NodeID    sql.NullInt64
	SortOrder int
	Active    int
	CreatedAt string
	UpdatedAt string
}

const quickQuerySelectCols = `id, command, name, target, node_id, sort_order, active, created_at, updated_at`

func scanQuickQuery(scanner interface {
	Scan(dest ...any) error
}) (QuickQuery, error) {
	var item QuickQuery
	err := scanner.Scan(
		&item.ID, &item.Command, &item.Name, &item.Target, &item.NodeID,
		&item.SortOrder, &item.Active, &item.CreatedAt, &item.UpdatedAt,
	)
	return item, err
}

func (q *Queries) GetAllQuickQueries(ctx context.Context) ([]QuickQuery, error) {
	rows, err := q.db.QueryContext(ctx, `
		SELECT `+quickQuerySelectCols+`
		FROM quick_queries
		ORDER BY sort_order ASC, id ASC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var items []QuickQuery
	for rows.Next() {
		item, err := scanQuickQuery(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (q *Queries) GetActiveQuickQueries(ctx context.Context) ([]QuickQuery, error) {
	rows, err := q.db.QueryContext(ctx, `
		SELECT `+quickQuerySelectCols+`
		FROM quick_queries
		WHERE active = 1
		ORDER BY sort_order ASC, id ASC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var items []QuickQuery
	for rows.Next() {
		item, err := scanQuickQuery(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (q *Queries) GetQuickQueryByID(ctx context.Context, id int64) (*QuickQuery, error) {
	row := q.db.QueryRowContext(ctx, `
		SELECT `+quickQuerySelectCols+`
		FROM quick_queries WHERE id = ?
	`, id)
	item, err := scanQuickQuery(row)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	return &item, nil
}

func (q *Queries) NextQuickQuerySortOrder(ctx context.Context) (int, error) {
	row := q.db.QueryRowContext(ctx, `SELECT COALESCE(MAX(sort_order), 0) + 1 FROM quick_queries`)
	var next int
	if err := row.Scan(&next); err != nil {
		return 1, err
	}
	return next, nil
}

func (q *Queries) CreateQuickQuery(ctx context.Context, arg *QuickQuery) (*QuickQuery, error) {
	res, err := q.db.ExecContext(ctx, `
		INSERT INTO quick_queries (command, name, target, node_id, sort_order, active)
		VALUES (?, ?, ?, ?, ?, ?)
	`, arg.Command, arg.Name, arg.Target, arg.NodeID, arg.SortOrder, arg.Active)
	if err != nil {
		return nil, err
	}
	id, err := res.LastInsertId()
	if err != nil {
		return nil, err
	}
	return q.GetQuickQueryByID(ctx, id)
}

func (q *Queries) UpdateQuickQuery(ctx context.Context, arg *QuickQuery) (*QuickQuery, error) {
	_, err := q.db.ExecContext(ctx, `
		UPDATE quick_queries
		SET command = ?, name = ?, target = ?, node_id = ?, sort_order = ?, active = ?, updated_at = CURRENT_TIMESTAMP
		WHERE id = ?
	`, arg.Command, arg.Name, arg.Target, arg.NodeID, arg.SortOrder, arg.Active, arg.ID)
	if err != nil {
		return nil, err
	}
	return q.GetQuickQueryByID(ctx, arg.ID)
}

func (q *Queries) DeleteQuickQuery(ctx context.Context, id int64) error {
	_, err := q.db.ExecContext(ctx, `DELETE FROM quick_queries WHERE id = ?`, id)
	return err
}

func (q *Queries) ToggleQuickQuery(ctx context.Context, id int64) error {
	_, err := q.db.ExecContext(ctx, `
		UPDATE quick_queries
		SET active = CASE WHEN active = 1 THEN 0 ELSE 1 END,
		    updated_at = CURRENT_TIMESTAMP
		WHERE id = ?
	`, id)
	return err
}
