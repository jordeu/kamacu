package notifications

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"
)

// The delta may contain undecided cache entries fetched in an earlier attempt.
// Revalidate entries not present in this scan with narrow resource requests.
func (g GitHub) verifyPendingActivity(ctx context.Context, s Source, event Event, scope *activityScope) (Event, error) {
	seen := map[string]bool{}
	reviews := map[int64]bool{}
	for _, e := range event.Evidence {
		seen[e.URL] = true
		if e.Kind == "review" && e.Current {
			_, id, ok := strings.Cut(e.URL, "#pullrequestreview-")
			if ok {
				n, _ := strconv.ParseInt(id, 10, 64)
				reviews[n] = true
			}
		}
	}
	rows, err := g.DB.QueryContext(ctx, `SELECT v.payload FROM notification_activities a JOIN notification_activity_versions v ON v.activity_id=a.id AND v.revision=a.revision WHERE a.source_id=? AND a.subject_url=? AND ((v.processed_batch_id IS NULL AND v.context_only=0) OR ?) ORDER BY a.id`, s.ID, event.URL, scope.replay)
	if err != nil {
		return event, err
	}
	var pending []SubjectEvidence
	for rows.Next() {
		var raw string
		if err = rows.Scan(&raw); err != nil {
			break
		}
		var e SubjectEvidence
		if err = json.Unmarshal([]byte(raw), &e); err != nil {
			break
		}
		if seen[e.URL] {
			continue
		}
		if scope.replay {
			at, err := time.Parse(time.RFC3339, e.UpdatedAt)
			if err == nil && at.Before(scope.floor) && !reviews[e.ReviewID] {
				continue
			}
		}
		pending = append(pending, e)
	}
	rowErr := rows.Err()
	rows.Close()
	if err != nil {
		return event, err
	}
	if rowErr != nil {
		return event, rowErr
	}
	if len(pending) > 200 {
		event.EvidenceComplete = false
		event.MentionScanComplete = false
		event.ContextWarnings = append(event.ContextWarnings, "More than 200 cached undecided activities require verification; narrow the pending selection.")
		return event, nil
	}
	subjectPath, err := githubPath(g.config(s).Hostname, event.SubjectURL)
	if err != nil {
		return event, err
	}
	for _, e := range pending {
		if seen[e.URL] {
			continue
		}
		marker, ok := map[string]string{"comment": "#issuecomment-", "review_comment": "#discussion_r", "review": "#pullrequestreview-"}[e.Kind]
		if !ok {
			continue
		}
		_, id, ok := strings.Cut(e.URL, marker)
		if !ok {
			return event, errors.New("cached GitHub activity has no resource identity")
		}
		n, err := strconv.ParseInt(id, 10, 64)
		if err != nil || n <= 0 {
			return event, errors.New("invalid cached GitHub activity identity")
		}
		path := subjectPath
		switch e.Kind {
		case "comment":
			prefix, _, _ := strings.Cut(subjectPath, "/pulls/")
			if prefix == subjectPath {
				prefix, _, _ = strings.Cut(subjectPath, "/issues/")
			}
			path = prefix + "/issues/comments/" + id
		case "review_comment":
			prefix, _, _ := strings.Cut(subjectPath, "/pulls/")
			path = prefix + "/pulls/comments/" + id
		case "review":
			path = subjectPath + "/reviews/" + id
		}
		raw, _, err := g.api(ctx, g.config(s).Hostname, path)
		if err != nil {
			if !errors.Is(err, ErrContextUnavailable) {
				return event, err
			}
			e.Actor = ""
			e.Unavailable = true
			e.Text = "GitHub reports this activity is unavailable (HTTP 404/410). Earlier saved text is historical, not a current request."
			e.Complete = true
			e.Current = true
			e.ReviewState = ""
			e.ReviewBodyEmpty = false
			no := false
			e.ViewerMention = &no
			e.MentionViewer = s.Account
			event.Evidence = append(event.Evidence, e)
			continue
		}
		var v struct {
			Body      string `json:"body"`
			Updated   string `json:"updated_at"`
			Submitted string `json:"submitted_at"`
			Created   string `json:"created_at"`
			State     string `json:"state"`
			User      struct {
				Login string `json:"login"`
			} `json:"user"`
		}
		if err = json.Unmarshal(raw, &v); err != nil {
			return event, fmt.Errorf("cannot verify cached activity: %w", err)
		}
		e.Text = v.Body
		e.Actor = v.User.Login
		e.CreatedAt = v.Created
		e.UpdatedAt = v.Updated
		if e.UpdatedAt == "" {
			e.UpdatedAt = v.Submitted
		}
		e.Unavailable = false
		e.Complete = true
		e.Current = true
		if e.Kind == "review" {
			e.ReviewState = v.State
			e.ReviewBodyEmpty = strings.TrimSpace(v.Body) == ""
			if e.ReviewBodyEmpty {
				e.Text = "GitHub review state: " + v.State + "."
			}
		}
		mentioned := textMentionsViewer(e.Text, s.Account)
		e.ViewerMention = &mentioned
		e.MentionViewer = s.Account
		event.Evidence = append(event.Evidence, e)
	}
	return event, nil
}
