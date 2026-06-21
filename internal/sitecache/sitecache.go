package sitecache

import (
	"database/sql"
	"fmt"

	"github.com/HopStat/HopStat/internal/quickqueries"
)

// Load warms all public-read caches from the database at startup.
func Load(db *sql.DB, credKey string, localAS uint32) error {
	if err := quickqueries.Load(db); err != nil {
		return fmt.Errorf("quick queries: %w", err)
	}
	if err := RefreshSettings(db, localAS); err != nil {
		return fmt.Errorf("settings: %w", err)
	}
	if err := RefreshCommunities(db); err != nil {
		return fmt.Errorf("communities: %w", err)
	}
	if err := RefreshNodes(db, credKey); err != nil {
		return fmt.Errorf("nodes: %w", err)
	}
	return nil
}
