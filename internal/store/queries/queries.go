package queries

import "database/sql"

type Queries struct {
	db *sql.DB
}

func New(db *sql.DB) *Queries {
	return &Queries{db: db}
}

// auditDayFormat is what SQLite's CURRENT_TIMESTAMP writes into audit_log.created_at.
const auditDayFormat = "2006-01-02"

// endOfDayBound makes a date-only upper bound inclusive of that whole day. created_at
// carries a time component, so comparing against a bare "2006-01-02" would exclude
// entries logged later on the day the caller asked for.
func endOfDayBound(v string) string {
	if len(v) == len(auditDayFormat) {
		return v + " 23:59:59"
	}
	return v
}
