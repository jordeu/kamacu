package store

import (
	"context"
	"database/sql"
	"log/slog"
	"time"
)

// ObservePool reports contention without running queries or changing the pool.
func ObservePool(ctx context.Context, db *sql.DB) {
	previous := db.Stats()
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			current := db.Stats()
			waits := current.WaitCount - previous.WaitCount
			duration := current.WaitDuration - previous.WaitDuration
			if waits > 0 || duration > 0 {
				slog.Debug("database pool contention", "wait_count", waits, "wait_duration_ms", duration.Milliseconds(), "in_use", current.InUse, "idle", current.Idle)
			}
			previous = current
		}
	}
}
