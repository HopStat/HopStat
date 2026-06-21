package middleware

import "time"

func init() {
	denyListPurgeInterval = 5 * time.Millisecond
	bruteForceCleanupInterval = 5 * time.Millisecond
	rateLimitCleanupInterval = 5 * time.Millisecond
}
