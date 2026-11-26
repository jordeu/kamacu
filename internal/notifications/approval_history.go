package notifications

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"kamacu/internal/store"
)

// WithRecordedApprovalHistory restores the historical fact that a live,
// dismissed review was once APPROVED. Its current state remains DISMISSED.
// A dismissal alone never establishes an approval. Only immutable GitHub
// versions for the same account, source, PR and live review ID can do that.
func (g GitHub) WithRecordedApprovalHistory(ctx context.Context, source Source, f PRFacts) PRFacts {
	if source.Account != "" && !strings.EqualFold(source.Account, f.Viewer) {
		f.ViewerEverApproved = nil
		f.Available = false
		f.Stale = true
		f.ViewerReviews = append([]ViewerReview(nil), f.ViewerReviews...)
		for i := range f.ViewerReviews {
			f.ViewerReviews[i].ApprovalPreviouslyObserved = false
			f.ViewerReviews[i].ApprovalEvidenceRef = ""
			f.ViewerReviews[i].ApprovalObservedAt = ""
		}
		f.ApprovalHistoryError = "approval history account mismatch"
		return f
	}
	if !f.Available || f.Stale || !f.ReviewsComplete || f.Viewer == "" || f.ViewerEverApproved != nil && *f.ViewerEverApproved {
		return f
	}
	hasDismissal := false
	for _, r := range f.ViewerReviews {
		hasDismissal = hasDismissal || r.State == "DISMISSED"
	}
	if !hasDismissal {
		return f
	}
	// The original verdict is unknown even if malformed resource identity or
	// missing local history prevents a safe lookup.
	f.ViewerEverApproved = nil
	urls := []string{}
	indexes := map[string]int{}
	repo, number, err := store.PRIdentity(f.Repository, f.Number)
	if err != nil {
		return f
	}
	subject := fmt.Sprintf("https://github.com/%s/pull/%d", repo, number)
	for i, r := range f.ViewerReviews {
		if r.State != "DISMISSED" {
			continue
		}
		if r.ID <= 0 {
			continue
		}
		url := fmt.Sprintf("%s#pullrequestreview-%d", subject, r.ID)
		if r.URL != "" && !strings.EqualFold(r.URL, url) {
			continue
		}
		urls = append(urls, url)
		indexes[url] = i
	}
	if len(urls) == 0 {
		return f
	}
	if g.DB == nil {
		return f
	}
	raw, _ := json.Marshal(urls)
	query := `SELECT a.resource_url,a.id,v.revision,v.fetched_at FROM notification_activities a
 JOIN notification_activity_versions v ON v.activity_id=a.id
 JOIN notification_activity_subjects subject ON subject.source_id=a.source_id AND subject.subject_url=a.subject_url
 JOIN notification_sources source ON source.id=a.source_id
 WHERE lower(a.subject_url)=? AND a.kind='review' AND source.provider='github' AND source.retired=0
 AND lower(source.account)=lower(?) AND lower(subject.account)=lower(?)
 AND lower(a.resource_url) IN (SELECT value FROM json_each(?))
 AND json_extract(v.payload,'$.kind')='review'
 AND lower(json_extract(v.payload,'$.url'))=lower(a.resource_url)
 AND lower(json_extract(v.payload,'$.actor'))=lower(?)
 AND json_extract(v.payload,'$.review_state')='APPROVED'
 AND COALESCE(json_extract(v.payload,'$.unavailable'),0)=0
`
	args := []any{subject, f.Viewer, f.Viewer, string(raw), f.Viewer}
	if source.ID != "" {
		query += " AND a.source_id=?"
		args = append(args, source.ID)
	}
	query += " ORDER BY v.fetched_at,a.id,v.revision"
	rows, err := g.DB.QueryContext(ctx, query, args...)
	if err != nil {
		f.ApprovalHistoryError = "historical approval evidence unavailable: " + err.Error()
		return f
	}
	defer rows.Close()
	f.ViewerReviews = append([]ViewerReview(nil), f.ViewerReviews...)
	for rows.Next() {
		var url string
		var id, revision, fetched int64
		if err = rows.Scan(&url, &id, &revision, &fetched); err != nil {
			break
		}
		i, ok := indexes[strings.ToLower(url)]
		if !ok {
			continue
		}
		if f.ViewerReviews[i].ApprovalPreviouslyObserved {
			continue
		}
		f.ViewerReviews[i].ApprovalPreviouslyObserved = true
		f.ViewerReviews[i].ApprovalEvidenceRef = fmt.Sprintf("A%d/V%d", id, revision)
		f.ViewerReviews[i].ApprovalObservedAt = time.UnixMilli(fetched).UTC().Format(time.RFC3339Nano)
		yes := true
		f.ViewerEverApproved = &yes
	}
	if err == nil {
		err = rows.Err()
	}
	if err != nil {
		f.ApprovalHistoryError = "historical approval evidence incomplete: " + err.Error()
	}
	return f
}
