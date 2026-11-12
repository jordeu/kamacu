package notifications

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
)

// Historical lookups are restricted to a subject in this batch and immutable
// versions existing when its scan receipt was created. Later edits cannot leak
// into an earlier decision through an optional context lookup.
func (s *Service) lookupActivity(ctx context.Context, snapshot AgentSnapshot, ref string) (SubjectEvidence, string, error) {
	for _, sub := range snapshot.Subjects {
		for _, e := range sub.Event.Evidence {
			if e.ActivityRef == ref && !e.Summarized {
				return e, sub.ID, nil
			}
		}
	}
	var id, revision int64
	if _, err := fmt.Sscanf(ref, "A%d/V%d", &id, &revision); err != nil || id <= 0 || revision <= 0 || ref != fmt.Sprintf("A%d/V%d", id, revision) {
		return SubjectEvidence{}, "", errors.New("invalid activity reference")
	}
	for _, sub := range snapshot.Subjects {
		d := sub.Event.ActivityDelta
		if d == nil {
			continue
		}
		var raw string
		err := s.DB.QueryRowContext(ctx, `SELECT v.payload FROM notification_activity_versions v JOIN notification_activities a ON a.id=v.activity_id JOIN notification_activity_scans scan ON scan.id=? AND scan.source_id=a.source_id AND scan.subject_url=a.subject_url WHERE v.activity_id=? AND v.revision=? AND v.rowid<=scan.history_ceiling AND a.source_id=? AND a.subject_url=?`, d.ScanID, id, revision, d.SourceID, sub.Event.URL).Scan(&raw)
		if errors.Is(err, sql.ErrNoRows) {
			continue
		}
		if err != nil {
			return SubjectEvidence{}, "", err
		}
		var e SubjectEvidence
		if err = json.Unmarshal([]byte(raw), &e); err != nil {
			return e, "", err
		}
		return e, sub.ID, nil
	}
	return SubjectEvidence{}, "", errors.New("activity is outside the frozen batch subjects")
}
func (s *Service) readThread(ctx context.Context, snapshot AgentSnapshot, args string) (string, error) {
	var req struct {
		ID       string `json:"id"`
		Offset   int    `json:"offset"`
		ReviewID int64  `json:"review_id"`
	}
	if err := strictRuleJSON([]byte(args), &req); err != nil {
		return "", err
	}
	if req.Offset < 0 || req.ReviewID < 0 {
		return "", errors.New("invalid thread offset or review identity")
	}
	var sub *AgentSubject
	for i := range snapshot.Subjects {
		if snapshot.Subjects[i].ID == req.ID {
			sub = &snapshot.Subjects[i]
			break
		}
	}
	if sub == nil || sub.Event.ActivityDelta == nil {
		return "", errors.New("thread is not available in the frozen activity cache")
	}
	d := sub.Event.ActivityDelta
	rows, err := s.DB.QueryContext(ctx, `SELECT v.payload FROM notification_activities a JOIN notification_activity_versions v ON v.activity_id=a.id JOIN notification_activity_scans scan ON scan.id=? AND scan.source_id=a.source_id AND scan.subject_url=a.subject_url WHERE a.source_id=? AND a.subject_url=? AND v.rowid<=scan.history_ceiling AND v.revision=(SELECT max(old.revision) FROM notification_activity_versions old WHERE old.activity_id=a.id AND old.rowid<=scan.history_ceiling) AND (?=0 OR json_extract(v.payload,'$.review_id')=? OR a.resource_url LIKE ?) ORDER BY json_extract(v.payload,'$.updated_at') DESC,a.id DESC LIMIT 6 OFFSET ?`, d.ScanID, d.SourceID, sub.Event.URL, req.ReviewID, req.ReviewID, "%#pullrequestreview-"+fmt.Sprint(req.ReviewID), req.Offset)
	if err != nil {
		return "", err
	}
	defer rows.Close()
	entries := []SubjectEvidence{}
	for rows.Next() {
		var raw string
		if err = rows.Scan(&raw); err != nil {
			return "", err
		}
		var e SubjectEvidence
		if err = json.Unmarshal([]byte(raw), &e); err != nil {
			return "", err
		}
		e.Current = false
		e.ContextOnly = true
		if len(e.Text) > 1600 {
			e.Text = strings.ToValidUTF8(e.Text[:1600], "") + "\n[Read read_activity with this activity_ref for the full saved text.]"
			e.Complete = false
			e.Summarized = true
		}
		entries = append(entries, e)
	}
	if err = rows.Err(); err != nil {
		return "", err
	}
	more := len(entries) > 5
	if more {
		entries = entries[:5]
	}
	for len(entries) > 1 && len(mustJSON(entries)) > 14000 {
		entries = entries[:len(entries)-1]
		more = true
	}
	next := -1
	if more {
		next = req.Offset + len(entries)
	}
	return mustJSON(map[string]any{"id": req.ID, "offset": req.Offset, "next_offset": next, "historical_context": true, "activities": entries}), nil
}
