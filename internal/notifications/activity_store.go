package notifications

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"kamacu/internal/store"
)

const activityOverlap = 5 * time.Minute
const activityReconcileInterval = 24 * time.Hour

type activityReplayKey struct{}

type activityScope struct {
	replay    bool
	repair    bool
	started   time.Time
	since     string
	floor     time.Time
	reconcile bool
	bootstrap bool
	legacy    map[string]legacyActivity
}
type legacyActivity struct {
	Batch    string
	Evidence SubjectEvidence
}

func activityDigest(e SubjectEvidence) string {
	// Timestamps may change without new content (especially bot reports).
	// Compare against the latest content, so restoring an older body still
	// creates a new monotonically numbered revision.
	raw := mustJSON(struct {
		URL, Actor, Text, State string
		Empty, Unavailable      bool
	}{e.URL, strings.ToLower(e.Actor), e.Text, e.ReviewState, e.ReviewBodyEmpty, e.Unavailable})
	return fmt.Sprintf("%x", sha256.Sum256([]byte(raw)))
}
func (g GitHub) activityScope(ctx context.Context, s Source, e Event) (*activityScope, error) {
	if g.DB == nil || s.ID == "" {
		return nil, nil
	}
	now := time.Now().UTC()
	if g.Now != nil {
		now = g.Now().UTC()
	}
	scope := &activityScope{started: now, legacy: map[string]legacyActivity{}}
	replay, _ := ctx.Value(activityReplayKey{}).(bool)
	scope.replay = replay || e.ActivityReplay
	floor := e.ActivityFloor
	if floor == "" {
		floor = e.Version
	}
	scope.floor, _ = time.Parse(time.RFC3339, floor)
	scope.floor = scope.floor.Add(-activityOverlap)
	var account, since, repairFrom string
	var reconciled int64
	err := g.DB.QueryRowContext(ctx, `SELECT account,fetched_through,reconciled_at,repair_from FROM notification_activity_subjects WHERE source_id=? AND subject_url=?`, s.ID, e.URL).Scan(&account, &since, &reconciled, &repairFrom)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return nil, err
	}
	if err == nil && !strings.EqualFold(account, s.Account) {
		return nil, errors.New("notification activity account changed")
	}
	scope.bootstrap = errors.Is(err, sql.ErrNoRows) || since == ""
	scope.repair = repairFrom != ""
	// A notification timestamp is the latest thread change, not the start of
	// unseen activity. On first contact use the source registration boundary.
	if scope.bootstrap && !scope.replay {
		scope.floor = time.UnixMilli(s.CreatedAt).UTC()
	}
	if repairFrom != "" {
		if at, e := time.Parse(time.RFC3339Nano, repairFrom); e == nil {
			scope.floor = at
		}
	}
	scope.reconcile = scope.replay || err != nil || now.Sub(time.UnixMilli(reconciled)) >= activityReconcileInterval
	if !scope.reconcile && since != "" {
		at, err := time.Parse(time.RFC3339Nano, since)
		if err != nil {
			return nil, err
		}
		scope.since = at.Add(-activityOverlap).UTC().Format(time.RFC3339)
	}
	if scope.bootstrap {
		var gap sql.NullString
		if err := g.DB.QueryRowContext(ctx, `SELECT min(json_extract(ev.payload,'$.version')) FROM notification_batches b JOIN notification_batch_events be ON be.batch_id=b.id JOIN notification_events ev ON ev.id=be.event_id,json_each(b.processor_snapshot,'$.subjects') j WHERE b.status='completed' AND b.created_at>=? AND ev.source_id=? AND json_extract(ev.payload,'$.url')=? AND json_extract(j.value,'$.source.url')=? AND COALESCE(json_extract(j.value,'$.source.evidence_complete'),0)=0`, time.Now().Add(-7*24*time.Hour).UnixMilli(), s.ID, e.URL, e.URL).Scan(&gap); err != nil {
			return nil, err
		}
		if gap.Valid {
			if at, err := time.Parse(time.RFC3339, gap.String); err == nil && at.Add(-activityOverlap).Before(scope.floor) {
				scope.floor = at.Add(-activityOverlap)
			}
		}
		rows, err := g.DB.QueryContext(ctx, `SELECT b.id,j.value FROM notification_batches b,json_each(b.processor_snapshot,'$.subjects') j WHERE b.status='completed' AND json_extract(j.value,'$.source.url')=? AND json_extract(j.value,'$.source.evidence_complete')=1 AND EXISTS(SELECT 1 FROM notification_batch_events be JOIN notification_events ev ON ev.id=be.event_id WHERE be.batch_id=b.id AND ev.source_id=?) ORDER BY b.created_at DESC LIMIT 100`, e.URL, s.ID)
		if err != nil {
			return nil, err
		}
		for rows.Next() {
			var id, raw string
			if err = rows.Scan(&id, &raw); err != nil {
				break
			}
			var sub AgentSubject
			if json.Unmarshal([]byte(raw), &sub) != nil {
				continue
			}
			for _, ev := range sub.Event.Evidence {
				if !ev.Complete || ev.URL == "" {
					continue
				}
				// Snapshots are newest first: only the latest acknowledged body
				// can seed this resource, even if older content is restored.
				if _, exists := scope.legacy[ev.URL]; !exists {
					scope.legacy[ev.URL] = legacyActivity{id, ev}
				}
			}
		}
		rowErr := rows.Err()
		rows.Close()
		if err != nil {
			return nil, err
		}
		if rowErr != nil {
			return nil, rowErr
		}
	}
	return scope, nil
}
func (g GitHub) reviewIsNew(ctx context.Context, s Source, event Event, scope *activityScope, e SubjectEvidence) (bool, error) {
	if e.ReviewState == "PENDING" {
		return false, nil
	}
	if scope == nil {
		return false, nil
	}
	if scope.replay || scope.repair {
		at, err := time.Parse(time.RFC3339, e.UpdatedAt)
		if err != nil || !at.Before(scope.floor) {
			return true, nil
		}
	}
	var digest string
	var contextOnly bool
	var done sql.NullString
	err := g.DB.QueryRowContext(ctx, `SELECT a.digest,v.processed_batch_id,v.context_only FROM notification_activities a JOIN notification_activity_versions v ON v.activity_id=a.id AND v.revision=a.revision WHERE a.source_id=? AND a.subject_url=? AND a.resource_url=?`, s.ID, event.URL, e.URL).Scan(&digest, &done, &contextOnly)
	if err == nil {
		return digest != activityDigest(e) || (!done.Valid && !contextOnly), nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return false, err
	}
	if !scope.bootstrap {
		return true, nil
	}
	at, err := time.Parse(time.RFC3339, e.UpdatedAt)
	return err != nil || !at.Before(scope.floor), nil
}
func (g GitHub) finishActivityScan(ctx context.Context, s Source, event Event, scope *activityScope) (Event, error) {
	if scope == nil {
		return event, nil
	}
	d := &store.NotificationDelta{SourceID: s.ID, SubjectURL: event.URL, Since: scope.since, Replay: scope.replay, Complete: event.EvidenceComplete, Activities: []store.NotificationActivityRef{}}
	err := func() error {
		tx, err := g.DB.BeginTx(ctx, nil)
		if err != nil {
			return err
		}
		defer tx.Rollback()
		if _, err = tx.ExecContext(ctx, `INSERT OR IGNORE INTO notification_activity_subjects(source_id,subject_url,account) VALUES(?,?,?)`, s.ID, event.URL, s.Account); err != nil {
			return err
		}
		for _, e := range event.Evidence {
			d.Fetched++
			d.FetchedBytes += len(e.Text)
			digest := activityDigest(e)
			var id, revision int64
			var previous string
			err = tx.QueryRowContext(ctx, `SELECT id,revision,digest FROM notification_activities WHERE source_id=? AND subject_url=? AND resource_url=?`, s.ID, event.URL, e.URL).Scan(&id, &revision, &previous)
			if err != nil && !errors.Is(err, sql.ErrNoRows) {
				return err
			}
			if err == nil && previous == digest {
				d.Unchanged++
				at, dateErr := time.Parse(time.RFC3339, e.UpdatedAt)
				if (scope.repair || e.Current) && e.Kind != "subject" && (dateErr != nil || !at.Before(scope.floor) || e.Current) {
					e.ActivityRef = fmt.Sprintf("A%d/V%d", id, revision)
					e.ContextOnly = false
					if _, err = tx.ExecContext(ctx, `UPDATE notification_activity_versions SET context_only=0,payload=? WHERE activity_id=? AND revision=? AND context_only=1 AND processed_batch_id IS NULL`, mustJSON(e), id, revision); err != nil {
						return err
					}
				}
				continue
			}
			first := errors.Is(err, sql.ErrNoRows)
			revision++
			d.Changed++
			if first {
				if err = tx.QueryRowContext(ctx, `INSERT INTO notification_activities(source_id,subject_url,resource_url,kind,digest,revision) VALUES(?,?,?,?,?,?) RETURNING id`, s.ID, event.URL, e.URL, e.Kind, digest, revision).Scan(&id); err != nil {
					return err
				}
			} else {
				if _, err = tx.ExecContext(ctx, `UPDATE notification_activities SET digest=?,revision=? WHERE id=?`, digest, revision, id); err != nil {
					return err
				}
			}
			contextOnly := false
			if first && scope.bootstrap {
				if e.Kind == "subject" {
					if event.SubjectType == "PullRequest" || event.SubjectType == "Issue" {
						created, err := time.Parse(time.RFC3339, e.CreatedAt)
						contextOnly = err != nil || created.Before(scope.floor)
					}
				} else {
					at, err := time.Parse(time.RFC3339, e.UpdatedAt)
					contextOnly = err == nil && at.Before(scope.floor) && !e.Current
				}
			}
			var processed any
			if legacy, ok := scope.legacy[e.URL]; first && ok && activityDigest(legacy.Evidence) == digest {
				processed = legacy.Batch
			}
			e.ActivityRef = fmt.Sprintf("A%d/V%d", id, revision)
			e.ContextOnly = contextOnly
			if _, err = tx.ExecContext(ctx, `INSERT INTO notification_activity_versions(activity_id,revision,digest,payload,context_only,fetched_at,processed_batch_id) VALUES(?,?,?,?,?,?,?)`, id, revision, digest, mustJSON(e), contextOnly, scope.started.UnixMilli(), processed); err != nil {
				return err
			}
		}
		// Previously fetched but undecided versions survive model and publication
		// failures, even when subsequent requests use the advanced fetch cursor.
		rows, err := tx.QueryContext(ctx, `SELECT a.id,a.revision,v.payload,v.processed_batch_id,v.context_only FROM notification_activities a JOIN notification_activity_versions v ON v.activity_id=a.id AND v.revision=a.revision WHERE a.source_id=? AND a.subject_url=? AND ((v.processed_batch_id IS NULL AND v.context_only=0) OR ?) ORDER BY CASE a.kind WHEN 'review' THEN 0 WHEN 'review_comment' THEN 1 WHEN 'comment' THEN 2 ELSE 3 END,a.id`, s.ID, event.URL, scope.replay)
		if err != nil {
			return err
		}
		current := map[string]bool{}
		for _, e := range event.Evidence {
			current[e.URL] = e.Current || (e.Kind == "subject" && event.SubjectType != "PullRequest" && event.SubjectType != "Issue")
		}
		var evidence []SubjectEvidence
		seen := map[string]bool{}
		for rows.Next() {
			var id, rev int64
			var raw string
			var done sql.NullString
			var contextOnly bool
			if err = rows.Scan(&id, &rev, &raw, &done, &contextOnly); err != nil {
				break
			}
			var e SubjectEvidence
			if err = json.Unmarshal([]byte(raw), &e); err != nil {
				break
			}
			if done.Valid || contextOnly {
				at, dateErr := time.Parse(time.RFC3339, e.UpdatedAt)
				if e.Kind == "subject" && (event.SubjectType == "PullRequest" || event.SubjectType == "Issue") && rev == 1 {
					created, err := time.Parse(time.RFC3339, e.CreatedAt)
					if err != nil || created.Before(scope.floor) {
						continue
					}
				}
				if !current[e.URL] && dateErr == nil && at.Before(scope.floor) {
					continue
				}
			}
			e.Current = true
			e.ContextOnly = false
			evidence = append(evidence, compactActivityEvidence(e))
			seen[e.URL] = true
			d.Activities = append(d.Activities, store.NotificationActivityRef{ID: id, Revision: rev, Ref: e.ActivityRef})
		}
		re := rows.Err()
		rows.Close()
		if err != nil {
			return err
		}
		if re != nil {
			return re
		}
		// The current description remains context; unchanged history is available
		// through precise, immutable activity lookups rather than copied wholesale.
		rows, err = tx.QueryContext(ctx, `SELECT v.payload FROM notification_activities a JOIN notification_activity_versions v ON v.activity_id=a.id AND v.revision=a.revision WHERE a.source_id=? AND a.subject_url=? AND a.kind='subject'`, s.ID, event.URL)
		if err != nil {
			return err
		}
		for rows.Next() {
			var raw string
			if err = rows.Scan(&raw); err != nil {
				break
			}
			var e SubjectEvidence
			if err = json.Unmarshal([]byte(raw), &e); err != nil {
				break
			}
			if !seen[e.URL] {
				e.ContextOnly = true
				e.Current = false
				if len(e.Text) > 12000 {
					e.Text = strings.ToValidUTF8(e.Text[:12000], "") + "\n[Description excerpt; read_activity retrieves the full saved version.]"
					e.Summarized = true
					e.Complete = false
				}
				evidence = append(evidence, e)
				seen[e.URL] = true
			}
		}
		re = rows.Err()
		rows.Close()
		if err != nil {
			return err
		}
		if re != nil {
			return re
		}
		// Parent comments already cached by the baseline or an earlier delta.
		parents := []int64{}
		for _, e := range evidence {
			if e.ParentID > 0 {
				parents = append(parents, e.ParentID)
			}
		}
		for _, parent := range parents {
			var raw string
			err = tx.QueryRowContext(ctx, `SELECT v.payload FROM notification_activities a JOIN notification_activity_versions v ON v.activity_id=a.id AND v.revision=a.revision WHERE a.source_id=? AND a.subject_url=? AND a.resource_url LIKE ? LIMIT 1`, s.ID, event.URL, "%#discussion_r"+fmt.Sprint(parent)).Scan(&raw)
			if errors.Is(err, sql.ErrNoRows) {
				continue
			}
			if err != nil {
				return err
			}
			var e SubjectEvidence
			if err = json.Unmarshal([]byte(raw), &e); err != nil {
				return err
			}
			if !seen[e.URL] {
				e.ContextOnly = true
				e.Current = false
				evidence = append(evidence, compactActivityEvidence(e))
				seen[e.URL] = true
			}
		}
		event.Evidence = evidence
		if err = tx.QueryRowContext(ctx, `INSERT INTO notification_activity_scans(source_id,subject_url,started_at,complete,manifest,history_ceiling) VALUES(?,?,?,?,?,(SELECT COALESCE(max(rowid),0) FROM notification_activity_versions)) RETURNING id`, s.ID, event.URL, scope.started.Format("2006-01-02T15:04:05.000000000Z"), d.Complete, mustJSON(d.Activities)).Scan(&d.ScanID); err != nil {
			return err
		}
		if d.Complete {
			reconciled := int64(0)
			if scope.reconcile {
				reconciled = scope.started.UnixMilli()
			}
			if _, err = tx.ExecContext(ctx, `UPDATE notification_activity_subjects SET fetched_through=max(fetched_through,?),reconciled_at=max(reconciled_at,?) WHERE source_id=? AND subject_url=?`, scope.started.Format("2006-01-02T15:04:05.000000000Z"), reconciled, s.ID, event.URL); err != nil {
				return err
			}
		}
		return tx.Commit()
	}()
	if err != nil {
		return event, err
	}
	if run, ok := ctx.Value(notificationRunKey{}).(*notificationRun); ok {
		run.fetched.Add(int64(d.Fetched))
		run.changed.Add(int64(d.Changed))
		run.unchanged.Add(int64(d.Unchanged))
		run.fetchedBytes.Add(int64(d.FetchedBytes))
	}
	event.ActivityDelta = d
	event.Context = "" // Avoid repeating full API bodies in prompts.
	slog.Info("notification activity retrieved", "source", s.ID, "subject", event.URL, "fetched", d.Fetched, "changed", d.Changed, "unchanged", d.Unchanged, "bytes", d.FetchedBytes, "pending_versions", len(d.Activities), "complete", d.Complete)
	return event, nil
}
func compactActivityEvidence(e SubjectEvidence) SubjectEvidence {
	if e.Actor != "github-actions[bot]" || e.ViewerMention == nil || *e.ViewerMention || !strings.HasPrefix(e.Text, "🧪 Test coverage:") || !strings.Contains(e.Text, "## Code Coverage Summary") {
		return e
	}
	// Replace balanced blocks individually; preserve all prose between them.
	// Malformed markup is kept intact rather than guessing its boundaries.
	var out strings.Builder
	rest := e.Text
	changed := false
	for {
		start := strings.Index(rest, "<details>")
		if start < 0 {
			out.WriteString(rest)
			break
		}
		depth, pos := 1, start+len("<details>")
		for depth > 0 {
			open := strings.Index(rest[pos:], "<details>")
			close := strings.Index(rest[pos:], "</details>")
			if close < 0 {
				return e
			}
			if open >= 0 && open < close {
				depth++
				pos += open + len("<details>")
			} else {
				depth--
				pos += close + len("</details>")
			}
		}
		out.WriteString(rest[:start])
		out.WriteString("[Generated coverage table omitted; read_activity retrieves the full saved report.]")
		rest = rest[pos:]
		changed = true
	}
	if changed {
		e.Text = out.String()
		e.Summarized = true
		e.Complete = false
	}
	return e
}
