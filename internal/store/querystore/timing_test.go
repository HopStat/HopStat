package querystore

import "time"

func init() {
	cleanupTickInterval = 5 * time.Millisecond
}
