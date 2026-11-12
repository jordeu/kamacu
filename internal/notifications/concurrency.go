package notifications

import (
	"context"
	"log/slog"
	"sync/atomic"
	"time"

	"golang.org/x/sync/errgroup"
)

const (
	notificationProjectConcurrency = 3
	notificationSubjectConcurrency = 4
	notificationGitHubConcurrency  = 4
)

type notificationRunKey struct{}
type notificationProjectKey struct{}

type notificationRun struct {
	requests     atomic.Int64
	rounds       atomic.Int64
	fetched      atomic.Int64
	changed      atomic.Int64
	unchanged    atomic.Int64
	fetchedBytes atomic.Int64
	batchID      string
	subjects     chan struct{}
	github       chan struct{}
}

func withNotificationRun(ctx context.Context, batchID string) context.Context {
	return context.WithValue(ctx, notificationRunKey{}, &notificationRun{
		batchID:  batchID,
		subjects: make(chan struct{}, notificationSubjectConcurrency),
		github:   make(chan struct{}, notificationGitHubConcurrency),
	})
}

// Workers own separate result slots. Wait joins every worker, including on error,
// so no inference or retrieval survives a failed batch or its parent context.
func parallelNotifications(ctx context.Context, count, limit int, fn func(context.Context, int) error) error {
	parent := ctx
	group, ctx := errgroup.WithContext(ctx)
	var next atomic.Int64
	for range min(count, limit) {
		group.Go(func() error {
			for {
				if err := ctx.Err(); err != nil {
					return err
				}
				i := int(next.Add(1) - 1)
				if i >= count {
					return nil
				}
				if err := fn(ctx, i); err != nil {
					return err
				}
			}
		})
	}
	if err := group.Wait(); err != nil {
		return err
	}
	return parent.Err()
}

func acquireNotificationSlot(ctx context.Context, slots chan struct{}) (func(), error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	select {
	case slots <- struct{}{}:
		// Cancellation and an available slot can become ready together. Do not
		// start another read after cancellation won while this worker queued.
		if err := ctx.Err(); err != nil {
			<-slots
			return nil, err
		}
		return func() { <-slots }, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

func parallelNotificationSubjects(ctx context.Context, count int, fn func(context.Context, int) error) error {
	return parallelNotifications(ctx, count, notificationSubjectConcurrency, func(ctx context.Context, i int) error {
		if run, ok := ctx.Value(notificationRunKey{}).(*notificationRun); ok {
			release, err := acquireNotificationSlot(ctx, run.subjects)
			if err != nil {
				return err
			}
			defer release()
		}
		return fn(ctx, i)
	})
}

// Stage durations are wall time, including queueing, not the sum of overlapping
// child calls. Repository and batch fields allow concurrent work to be correlated.
func notificationStage(ctx context.Context, stage string, fn func() error) (err error) {
	start := time.Now()
	defer func() {
		attrs := []any{"stage", stage, "duration_ms", time.Since(start).Milliseconds(), "error", err}
		if run, ok := ctx.Value(notificationRunKey{}).(*notificationRun); ok {
			attrs = append(attrs, "batch", run.batchID)
		}
		if repo, ok := ctx.Value(notificationProjectKey{}).(string); ok {
			attrs = append(attrs, "repository", repo)
		}
		slog.Info("notification stage finished", attrs...)
	}()
	return fn()
}
