package notifications

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
)

type activityPage struct{ start, end int }

// Receipt tracks bytes actually supplied to the model, not merely fetched.
type activityReceipts struct {
	visible map[string]bool
	pages   map[string][]activityPage
	lengths map[string]int
}

// requiredReads points at the first unread byte, including gaps left by
// out-of-order pages. It is a work list, never a receipt for unseen content.
func (r *activityReceipts) requiredReads(snapshot AgentSnapshot) []AgentRead {
	reads := []AgentRead{}
	for _, sub := range snapshot.Subjects {
		if sub.Event.ActivityDelta == nil {
			continue
		}
		for _, ref := range sub.Event.ActivityDelta.Activities {
			if r.visible[ref.Ref] {
				continue
			}
			pages := append([]activityPage(nil), r.pages["read_activity:"+ref.Ref]...)
			sort.Slice(pages, func(i, j int) bool { return pages[i].start < pages[j].start })
			offset := 0
			for _, page := range pages {
				if page.start > offset {
					break
				}
				offset = max(offset, page.end)
			}
			reads = append(reads, AgentRead{Name: "read_activity", Arguments: mustJSON(map[string]any{"id": ref.Ref, "offset": offset})})
		}
	}
	return reads
}

func newActivityReceipts(snapshot AgentSnapshot, initial string) *activityReceipts {
	r := &activityReceipts{visible: map[string]bool{}, pages: map[string][]activityPage{}, lengths: map[string]int{}}
	r.addVisible(snapshot, initial)
	return r
}
func (r *activityReceipts) addVisible(snapshot AgentSnapshot, initial string) {
	full := map[string]string{}
	for _, sub := range snapshot.Subjects {
		for _, e := range sub.Event.Evidence {
			if e.ActivityRef != "" {
				full[e.ActivityRef] = mustJSON(e)
			}
		}
	}
	// Both full snapshots and compact indexes carry complete evidence objects
	// when their source text fits. Manifests alone are never read receipts.
	var input struct {
		Subjects []struct {
			Source struct {
				Evidence []SubjectEvidence `json:"evidence"`
			} `json:"source"`
			ActivityEvidence []SubjectEvidence `json:"activity_evidence"`
		} `json:"subjects"`
	}
	if json.Unmarshal([]byte(initial), &input) != nil {
		return
	}
	for _, sub := range input.Subjects {
		for _, e := range append(sub.Source.Evidence, sub.ActivityEvidence...) {
			if raw, ok := full[e.ActivityRef]; ok && raw == mustJSON(e) {
				r.visible[e.ActivityRef] = true
			}
		}
	}
}
func (r *activityReceipts) addRead(snapshot AgentSnapshot, read AgentRead, result string) {
	if read.Name == "read_thread" {
		var page struct {
			Activities []SubjectEvidence `json:"activities"`
		}
		if json.Unmarshal([]byte(result), &page) == nil {
			for _, item := range page.Activities {
				if !item.Complete {
					continue
				}
				for _, sub := range snapshot.Subjects {
					for _, e := range sub.Event.Evidence {
						if e.ActivityRef == item.ActivityRef && e.Text == item.Text {
							r.visible[e.ActivityRef] = true
						}
					}
				}
			}
		}
		return
	}
	if read.Name != "read_subject" && read.Name != "read_activity" {
		return
	}
	var page struct {
		ID     string `json:"id"`
		Offset int    `json:"offset"`
		Next   int    `json:"next_offset"`
		Data   string `json:"data"`
	}
	if json.Unmarshal([]byte(result), &page) != nil {
		return
	}
	key := read.Name + ":" + page.ID
	r.pages[key] = append(r.pages[key], activityPage{page.Offset, page.Offset + len(page.Data)})
	if page.Next == -1 {
		r.lengths[key] = page.Offset + len(page.Data)
	}
	ranges := r.pages[key]
	sort.Slice(ranges, func(i, j int) bool { return ranges[i].start < ranges[j].start })
	end := 0
	for _, p := range ranges {
		if p.start > end {
			break
		}
		if p.end > end {
			end = p.end
		}
	}
	total, complete := r.lengths[key]
	if !complete || end < total {
		return
	}
	if read.Name == "read_activity" {
		r.visible[page.ID] = true
		return
	}
	for _, sub := range snapshot.Subjects {
		if sub.ID == page.ID && end >= len(mustJSON(sub)) {
			for _, e := range sub.Event.Evidence {
				r.visible[e.ActivityRef] = true
			}
		}
	}
}
func (r *activityReceipts) validate(snapshot AgentSnapshot) error {
	for _, sub := range snapshot.Subjects {
		d := sub.Event.ActivityDelta
		if d == nil {
			continue
		}
		if !d.Complete {
			return errors.New("required notification activity retrieval is incomplete; keep the notification pending")
		}
		for _, ref := range d.Activities {
			if !r.visible[ref.Ref] {
				return fmt.Errorf("new activity %s was not fully supplied to the model; call read_activity with id=%s and offset=0, and read its continuation pages", ref.Ref, ref.Ref)
			}
		}
	}
	return nil
}
func validateActivityDecisions(snapshot AgentSnapshot, turn AgentTurn) error {
	allowed := map[string]map[int64]bool{}
	for _, sub := range snapshot.Subjects {
		if sub.Event.ActivityDelta == nil {
			continue
		}
		if !sub.Event.ActivityDelta.Complete {
			return errors.New("cannot decide incomplete activity")
		}
		for _, a := range sub.Event.ActivityDelta.Activities {
			allowed[a.Ref] = map[int64]bool{}
			for _, id := range sub.EventIDs {
				allowed[a.Ref][id] = true
			}
		}
	}
	seen := map[string]bool{}
	for _, d := range turn.Decisions {
		for _, ref := range d.ActivityRefs {
			if seen[ref] || allowed[ref] == nil {
				return fmt.Errorf("unknown or repeated activity reference %s", ref)
			}
			bound := false
			for _, id := range d.EventIDs {
				bound = bound || allowed[ref][id]
			}
			if !bound {
				return errors.New("activity reference belongs to another notification")
			}
			seen[ref] = true
		}
	}
	for ref := range allowed {
		if !seen[ref] {
			return fmt.Errorf("decision must account for new activity %s in activity_refs", ref)
		}
	}
	return nil
}
func activityFingerprint(e Event) string {
	// Stable revision identities and current PR facts; retrieval timestamps and
	// receipt IDs do not turn a repeated fetch into new activity.
	refs := []string{}
	if e.ActivityDelta != nil {
		for _, a := range e.ActivityDelta.Activities {
			refs = append(refs, a.Ref)
		}
	}
	state := ""
	if e.PRFacts != nil {
		f := *e.PRFacts
		state = mustJSON(struct {
			State     string
			Draft     *bool
			Head      string
			Reviewers []string
			Approval  *bool
			Reviews   []ViewerReview
			Complete  bool
		}{f.State, f.Draft, f.HeadCommit, f.RequestedReviewers, f.ViewerEverApproved, f.ViewerReviews, f.ReviewsComplete})
	}
	return strings.Join(refs, ",") + "\n" + e.SubjectState + "\n" + state
}

func compactActivityIndex(sub AgentSubject, budget int) []SubjectEvidence {
	entries := []SubjectEvidence{}
	for _, e := range sub.Event.Evidence {
		if e.ActivityRef != "" {
			entries = append(entries, e)
		}
	}
	if len(mustJSON(entries)) <= budget {
		return entries
	}
	// Remove the largest bodies first, retaining short approvals and comments
	// even when another activity makes the subject exceed its share of space.
	order := make([]int, len(entries))
	for i := range order {
		order[i] = i
	}
	sort.SliceStable(order, func(i, j int) bool {
		return len(mustJSON(entries[order[i]].Text)) > len(mustJSON(entries[order[j]].Text))
	})
	size := len(mustJSON(entries))
	for _, i := range order {
		before := len(mustJSON(entries[i]))
		entries[i].Text = ""
		entries[i].Complete = false
		size += len(mustJSON(entries[i])) - before
		if size <= budget {
			break
		}
	}
	return entries
}

func notificationActivityMetrics(ctx context.Context, snapshot AgentSnapshot, trace []AgentReadResult) map[string]int64 {
	out := map[string]int64{"new_versions": 0, "evidence_bytes": 0, "lookups": 0}
	for _, sub := range snapshot.Subjects {
		if sub.Event.ActivityDelta != nil {
			out["new_versions"] += int64(len(sub.Event.ActivityDelta.Activities))
		}
		for _, e := range sub.Event.Evidence {
			out["evidence_bytes"] += int64(len(e.Text))
		}
	}
	for _, r := range trace {
		if r.Name != "source_updates" {
			out["lookups"]++
		}
	}
	if run, ok := ctx.Value(notificationRunKey{}).(*notificationRun); ok {
		out["github_requests"] = run.requests.Load()
		out["decision_rounds"] = run.rounds.Load()
		out["fetched"] = run.fetched.Load()
		out["changed"] = run.changed.Load()
		out["unchanged"] = run.unchanged.Load()
		out["fetched_bytes"] = run.fetchedBytes.Load()
	}
	return out
}
