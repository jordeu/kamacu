package notifications

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"
)

type recordingError struct {
	key     string
	phase   string
	err     error
	outcome error
}

func (e *recordingError) Error() string { return fmt.Sprintf("cannot record %s: %v", e.phase, e.err) }
func (e *recordingError) Unwrap() error { return e.err }

func transientStorageError(err error) bool {
	if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
		return true
	}
	type sqliteError interface{ Code() int }
	var se sqliteError
	if errors.As(err, &se) {
		return se.Code()&255 == 5 || se.Code()&255 == 6
	}
	return false
}

// A single result is retried in memory; work is never repeated just because a
// write timed out. Connection wait and SQL time are measured independently.
func (s *Service) journalWrite(ctx context.Context, phase string, fn func(context.Context, *sql.Tx) error) error {
	var last error
	for attempt := 0; attempt < 3; attempt++ {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		call, cancel := context.WithTimeout(ctx, 5*time.Second)
		wait := time.Now()
		before := s.DB.Stats()
		conn, err := s.DB.Conn(call)
		acquired := time.Now()
		if err == nil {
			var tx *sql.Tx
			tx, err = conn.BeginTx(call, nil)
			if err == nil {
				err = fn(call, tx)
				if err == nil {
					err = tx.Commit()
				}
				_ = tx.Rollback()
			}
			_ = conn.Close()
		}
		if err != nil && call.Err() != nil {
			err = call.Err()
		}
		cancel()
		elapsed := time.Since(wait)
		if elapsed >= 250*time.Millisecond || err != nil {
			slog.Warn("notification database operation", "phase", phase, "attempt", attempt+1, "connection_wait_ms", acquired.Sub(wait).Milliseconds(), "sql_ms", time.Since(acquired).Milliseconds(), "pool_wait_count", s.DB.Stats().WaitCount-before.WaitCount, "error", err)
		}
		if err == nil {
			return nil
		}
		last = err
		if !transientStorageError(err) {
			return err
		}
		timer := time.NewTimer(time.Duration(attempt+1) * 100 * time.Millisecond)
		select {
		case <-ctx.Done():
			timer.Stop()
			return ctx.Err()
		case <-timer.C:
		}
	}
	return last
}

func (ex *actionExecutor) progress(ctx context.Context, text string) {
	call, cancel := context.WithTimeout(ctx, 500*time.Millisecond)
	defer cancel()
	if err := ex.s.Inbox.FinishNotification(call, ex.q.ID, "running", text); err != nil {
		slog.Debug("notification progress update deferred", "run", ex.q.ID, "error", err)
	}
}

func retryableRead(state, class string) bool {
	return state == "running" || (state == "blocked" || state == "failed") && (class == "read_result" || class == "interrupted_read" || class == "intent")
}

func recoveryProgress(phase string) string {
	if strings.Contains(phase, "read") {
		return "Waiting to record agent/source readiness. Retrying automatically; no reply is needed."
	}
	return "Waiting for the database. Retrying the saved action automatically; no reply is needed."
}
