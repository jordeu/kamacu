package notifications

import (
	"context"
	"encoding/json"
	"errors"
	"kamacu/internal/store"
	"strings"
	"testing"
	"time"
)

type activityGitHubFixture struct {
	s            *Service
	model        *fakeModel
	source       Source
	event        Event
	github       GitHub
	now          time.Time
	paths        []string
	reviews      []map[string]any
	comments     []map[string]any
	inline       []map[string]any
	failComments bool
}

func newActivityFixture(t *testing.T) *activityGitHubFixture {
	t.Helper()
	s, m, _, source := fixture(t)
	f := &activityGitHubFixture{s: s, model: m, source: source, now: time.Date(2026, 11, 12, 10, 0, 0, 0, time.UTC)}
	f.source.Account = "alice"
	f.source.CreatedAt = f.now.Add(-2 * time.Hour).UnixMilli()
	if _, err := s.DB.Exec(`UPDATE notification_sources SET created_at=? WHERE id=?`, f.source.CreatedAt, f.source.ID); err != nil {
		t.Fatal(err)
	}
	f.event = event()
	f.event.Version = f.now.Add(-time.Minute).Format(time.RFC3339)
	f.event.SubjectURL = "https://api.github.com/repos/acme/repo/pulls/7"
	f.event.Context = ""
	f.github = GitHub{DB: s.DB, Now: func() time.Time { return f.now }, Run: func(_ context.Context, args ...string) ([]byte, error) {
		path := args[len(args)-1]
		f.paths = append(f.paths, path)
		var data any = []any{}
		switch {
		case path == "user":
			data = map[string]any{"login": "alice"}
		case path == "repos/acme/repo/pulls/7":
			data = map[string]any{"title": "Change", "body": "PR background", "state": "open", "draft": false, "head": map[string]any{"sha": "head"}, "updated_at": f.event.Version, "created_at": "2026-11-01T00:00:00Z", "html_url": f.event.URL, "user": map[string]string{"login": "alice"}}
		case path == "repos/acme/repo/pulls/comments/101" && len(f.inline) > 0:
			data = f.inline[0]
		case strings.Contains(path, "/reviews/") && strings.Contains(path, "/comments"):
			data = f.inline
		case strings.Contains(path, "/reviews?"):
			data = f.reviews
		case strings.Contains(path, "/pulls/7/comments?"):
			data = f.inline
		case strings.Contains(path, "/issues/7/comments?"):
			if f.failComments {
				return errorResponse(404), errors.New("not available")
			}
			data = f.comments
		}
		if data == nil {
			data = []any{}
		}
		raw, err := json.Marshal(data)
		if err != nil {
			t.Fatal(err)
		}
		return response(string(raw), ""), nil
	}}
	s.Providers["github"] = f.github
	m.respond = func(prompt string) (any, error) {
		snapshot := inputSnapshot(t, prompt)
		turn := finalResponse(snapshot, false)
		for i, sub := range snapshot.Subjects {
			if sub.Event.ActivityDelta != nil {
				for _, ref := range sub.Event.ActivityDelta.Activities {
					turn.Decisions[i].ActivityRefs = append(turn.Decisions[i].ActivityRefs, ref.Ref)
				}
			}
		}
		return turn, nil
	}
	return f
}
func activityComment(id int64, text, actor, at string) map[string]any {
	return map[string]any{"id": id, "body": text, "html_url": "https://github.com/acme/repo/pull/7#discussion_r" + formatID(id), "user": map[string]string{"login": actor}, "created_at": at, "updated_at": at, "pull_request_review_id": int64(100)}
}
func formatID(id int64) string { raw, _ := json.Marshal(id); return string(raw) }
func (f *activityGitHubFixture) addReview() {
	f.reviews = []map[string]any{{"id": 100, "body": "", "state": "COMMENTED", "submitted_at": f.event.Version, "html_url": f.event.URL + "#pullrequestreview-100", "user": map[string]string{"login": "reviewer"}}}
	f.inline = []map[string]any{activityComment(101, "A new regression needs fixing", "reviewer", f.now.Add(-24*time.Hour).Format(time.RFC3339))}
}
func (f *activityGitHubFixture) process(t *testing.T) Processing {
	t.Helper()
	seed(t, f.s, f.source, f.event)
	return processAgent(t, f.s)
}
func TestIncrementalReviewSurvivesCoverageAndDraftTimestamps(t *testing.T) {
	f := newActivityFixture(t)
	f.addReview()
	for i, n := range []int{43000, 47000} {
		c := activityComment(int64(200+i), "🧪 Test coverage: 80%\n## Code Coverage Summary\n<details>"+strings.Repeat("x", n)+"</details>", "github-actions[bot]", f.event.Version)
		c["html_url"] = f.event.URL + "#issuecomment-" + formatID(int64(200+i))
		f.comments = append(f.comments, c)
	}
	p := f.process(t)
	if p.Status != "completed" {
		t.Fatal(p)
	}
	var raw string
	if err := f.s.DB.QueryRow(`SELECT processor_snapshot FROM notification_batches WHERE id=?`, p.ID).Scan(&raw); err != nil {
		t.Fatal(err)
	}
	var snap AgentSnapshot
	if err := json.Unmarshal([]byte(raw), &snap); err != nil {
		t.Fatal(err)
	}
	sub := snap.Subjects[0]
	found := false
	for _, e := range sub.Event.Evidence {
		if e.Actor == "reviewer" && strings.Contains(e.Text, "regression") {
			found = e.Current
		}
		if e.Actor == "github-actions[bot]" && len(e.Text) > 1000 {
			t.Fatal("coverage table was copied into prompt")
		}
	}
	if !found || sub.Event.ActivityDelta == nil || !sub.Event.ActivityDelta.Complete {
		t.Fatal(sub)
	}
	if len(f.model.prompts) != 1 {
		t.Fatal("unneeded inference", len(f.model.prompts))
	}
	var pending int
	f.s.DB.QueryRow(`SELECT count(*) FROM notification_activity_versions WHERE context_only=0 AND processed_batch_id IS NULL`).Scan(&pending)
	if pending != 0 {
		t.Fatal("checkpoint did not commit", pending)
	}
	// Subsequent scans retrieve incrementally and send no unchanged activity.
	f.paths = nil
	f.now = f.now.Add(10 * time.Minute)
	f.event.Version = f.now.Add(-time.Minute).Format(time.RFC3339)
	p = f.process(t)
	if p.Status != "completed" {
		t.Fatal(p)
	}
	if !strings.Contains(strings.Join(f.paths, "\n"), "&since=") {
		t.Fatal(f.paths)
	}
	var current AgentSnapshot
	f.s.DB.QueryRow(`SELECT processor_snapshot FROM notification_batches WHERE id=?`, p.ID).Scan(&raw)
	json.Unmarshal([]byte(raw), &current)
	if len(current.Subjects[0].Event.ActivityDelta.Activities) != 0 {
		t.Fatal("unchanged activity was repeated", current.Subjects[0])
	}
}
func TestFailedDecisionKeepsFetchedActivityForRetry(t *testing.T) {
	f := newActivityFixture(t)
	f.addReview()
	f.model.respond = func(string) (any, error) { return nil, errors.New("model offline") }
	p := f.process(t)
	if p.Status != "failed" {
		t.Fatal(p)
	}
	var fetched, processed string
	f.s.DB.QueryRow(`SELECT fetched_through,processed_through FROM notification_activity_subjects`).Scan(&fetched, &processed)
	if fetched == "" || processed != "" {
		t.Fatal(fetched, processed)
	}
	f.model.respond = func(prompt string) (any, error) {
		snap := inputSnapshot(t, prompt)
		turn := finalResponse(snap, false)
		for i, sub := range snap.Subjects {
			for _, a := range sub.Event.ActivityDelta.Activities {
				turn.Decisions[i].ActivityRefs = append(turn.Decisions[i].ActivityRefs, a.Ref)
			}
		}
		return turn, nil
	}
	p = processAgent(t, f.s)
	if p.Status != "completed" {
		t.Fatal(p)
	}
	var done int
	f.s.DB.QueryRow(`SELECT count(*) FROM notification_activity_versions WHERE context_only=0 AND processed_batch_id IS NOT NULL`).Scan(&done)
	if done != 2 {
		t.Fatal("cached review or inline comment lost", done)
	}
}
func TestPartialActivityCannotReachModelOrAdvanceCursor(t *testing.T) {
	f := newActivityFixture(t)
	f.addReview()
	f.failComments = true
	p := f.process(t)
	if p.Status != "failed" || len(f.model.prompts) != 0 {
		t.Fatal(p, f.model.prompts)
	}
	var fetched string
	f.s.DB.QueryRow(`SELECT fetched_through FROM notification_activity_subjects`).Scan(&fetched)
	if fetched != "" {
		t.Fatal("partial scan advanced cursor", fetched)
	}
	var pending int
	f.s.DB.QueryRow(`SELECT count(*) FROM notification_events WHERE raw_status='pending'`).Scan(&pending)
	if pending != 1 {
		t.Fatal(pending)
	}
}
func TestActivityEditReappearanceAndNoDuplicateVersion(t *testing.T) {
	f := newActivityFixture(t)
	f.addReview()
	if p := f.process(t); p.Status != "completed" {
		t.Fatal(p)
	}
	f.inline[0]["body"] = "Edited regression with a new direct request @alice"
	f.inline[0]["updated_at"] = f.now.Add(time.Minute).Format(time.RFC3339)
	f.now = f.now.Add(2 * time.Minute)
	f.event.Version = f.now.Add(-time.Minute).Format(time.RFC3339)
	p := f.process(t)
	if p.Status != "completed" {
		t.Fatal(p)
	}
	var maxRevision int
	f.s.DB.QueryRow(`SELECT max(revision) FROM notification_activity_versions`).Scan(&maxRevision)
	if maxRevision != 2 {
		t.Fatal(maxRevision)
	}
	var raw string
	f.s.DB.QueryRow(`SELECT processor_snapshot FROM notification_batches WHERE id=?`, p.ID).Scan(&raw)
	var snap AgentSnapshot
	json.Unmarshal([]byte(raw), &snap)
	if len(snap.Subjects[0].Event.ActivityDelta.Activities) != 1 || snap.Subjects[0].ViewerMentioned == nil || !*snap.Subjects[0].ViewerMentioned {
		t.Fatal(snap.Subjects[0])
	}
}

func TestActivityCheckpointRollsBackWithInboxPublication(t *testing.T) {
	f := newActivityFixture(t)
	f.addReview()
	f.model.respond = func(prompt string) (any, error) {
		snap := inputSnapshot(t, prompt)
		turn := finalResponse(snap, true)
		for i, sub := range snap.Subjects {
			for _, a := range sub.Event.ActivityDelta.Activities {
				turn.Decisions[i].ActivityRefs = append(turn.Decisions[i].ActivityRefs, a.Ref)
			}
		}
		return turn, nil
	}
	if _, err := f.s.DB.Exec(`CREATE TRIGGER fail_card BEFORE INSERT ON inbox_questions WHEN NEW.origin='notification' BEGIN SELECT RAISE(ABORT,'card failure'); END`); err != nil {
		t.Fatal(err)
	}
	p := f.process(t)
	if p.Status != "failed" {
		t.Fatal(p)
	}
	var done int
	f.s.DB.QueryRow(`SELECT count(*) FROM notification_activity_versions WHERE context_only=0 AND processed_batch_id IS NOT NULL`).Scan(&done)
	if done != 0 {
		t.Fatal("checkpoint escaped failed publication", done)
	}
	var cursor string
	f.s.DB.QueryRow(`SELECT processed_through FROM notification_activity_subjects`).Scan(&cursor)
	if cursor != "" {
		t.Fatal(cursor)
	}
	f.s.DB.Exec(`DROP TRIGGER fail_card`)
	p = processAgent(t, f.s)
	if p.Status != "completed" || p.ActionCount != 1 {
		t.Fatal(p)
	}
	f.s.DB.QueryRow(`SELECT count(*) FROM notification_activity_versions WHERE context_only=0 AND processed_batch_id IS NOT NULL`).Scan(&done)
	if done != 2 {
		t.Fatal(done)
	}
}
func TestIgnoringWithoutActivityCoverageIsRejected(t *testing.T) {
	f := newActivityFixture(t)
	f.addReview()
	f.model.respond = func(prompt string) (any, error) { return finalResponse(inputSnapshot(t, prompt), false), nil }
	p := f.process(t)
	if p.Status != "failed" {
		t.Fatal(p)
	}
	var archived int
	f.s.DB.QueryRow(`SELECT count(*) FROM notification_events WHERE raw_status='archived'`).Scan(&archived)
	if archived != 0 {
		t.Fatal("unaccounted review was archived", archived)
	}
}
func TestCoverageSummaryRetainsNonTableTextAndDirectMentions(t *testing.T) {
	no := false
	original := "🧪 Test coverage: 80%\n## Code Coverage Summary\n<details>large table</details>\nBuild failed: needs investigation."
	e := SubjectEvidence{Actor: "github-actions[bot]", Text: original, ViewerMention: &no, Complete: true}
	got := compactActivityEvidence(e)
	if strings.Contains(got.Text, "large table") || !strings.Contains(got.Text, "Build failed: needs investigation.") || !got.Summarized {
		t.Fatal(got)
	}
	yes := true
	e.ViewerMention = &yes
	e.Text += " @alice please investigate"
	if compactActivityEvidence(e).Text != e.Text {
		t.Fatal("direct mention was summarized away")
	}
}
func TestNewActivityPagesRequireCompleteReadCoverage(t *testing.T) {
	f := newActivityFixture(t)
	f.addReview()
	fresh, err := f.github.Enrich(context.Background(), f.source, f.event)
	if err != nil {
		t.Fatal(err)
	}
	sub := AgentSubject{ID: "S1", EventIDs: []int64{1}, Event: fresh}
	updateAgentSubject(&sub, fresh)
	snapshot := AgentSnapshot{Subjects: []AgentSubject{sub}}
	// Empty text in the index is a manifest, not a receipt for a full activity.
	entries := activityIndex(sub, false)
	for i := range entries {
		entries[i].Text = ""
		entries[i].Complete = false
	}
	initial := mustJSON(map[string]any{"subjects": []any{map[string]any{"activity_evidence": entries}}})
	receipts := newActivityReceipts(snapshot, initial)
	if receipts.validate(snapshot) == nil {
		t.Fatal("unread manifest counted as read evidence")
	}
	for _, e := range sub.Event.Evidence {
		if e.ActivityRef == "" || e.ContextOnly {
			continue
		}
		n := len(e.Text) / 2
		read := AgentRead{Name: "read_activity"}
		receipts.addRead(snapshot, read, mustJSON(map[string]any{"id": e.ActivityRef, "offset": n, "next_offset": -1, "data": e.Text[n:]}))
		if receipts.visible[e.ActivityRef] {
			t.Fatal("last page alone completed evidence")
		}
		receipts.addRead(snapshot, read, mustJSON(map[string]any{"id": e.ActivityRef, "offset": 0, "next_offset": n, "data": e.Text[:n]}))
	}
	if err = receipts.validate(snapshot); err != nil {
		t.Fatal(err)
	}
}

func TestRequiredActivityIndexKeepsLargeReviewAndShortApproval(t *testing.T) {
	large := SubjectEvidence{ID: "S1/E1", ActivityRef: "A1/V1", Text: strings.Repeat("Review finding. ", 5000), Complete: true}
	approval := SubjectEvidence{ID: "S1/E2", ActivityRef: "A2/V1", Kind: "review", ReviewState: "APPROVED", Text: "Looks good; a few style comments.", Complete: true}
	sub := AgentSubject{ID: "S1", Event: Event{Evidence: []SubjectEvidence{large, approval}, ActivityDelta: &store.NotificationDelta{Complete: true, Activities: []store.NotificationActivityRef{{Ref: large.ActivityRef}, {Ref: approval.ActivityRef}}}}}
	snapshot := AgentSnapshot{Subjects: []AgentSubject{sub}}
	entries := activityIndex(sub, true)
	if len(entries) != 2 || entries[0] != large || entries[1] != approval {
		t.Fatal("required activity was omitted", entries)
	}
	if sub.Event.Evidence[0].Text != large.Text {
		t.Fatal("compaction mutated frozen evidence")
	}
	initial := mustJSON(map[string]any{"subjects": []any{map[string]any{"activity_evidence": entries}}})
	receipts := newActivityReceipts(snapshot, initial)
	reads := receipts.requiredReads(snapshot)
	if len(reads) != 0 || !receipts.visible[approval.ActivityRef] {
		t.Fatal("supplied evidence incorrectly requires a lookup", reads)
	}
	if err := receipts.validate(snapshot); err != nil {
		t.Fatal(err)
	}
}

func TestRequiredActivityReadsResumeAtFirstMissingPage(t *testing.T) {
	snapshot := AgentSnapshot{Subjects: []AgentSubject{{Event: Event{ActivityDelta: &store.NotificationDelta{Complete: true, Activities: []store.NotificationActivityRef{{Ref: "A1/V1"}}}}}}}
	receipts := newActivityReceipts(snapshot, `{}`)
	read := AgentRead{Name: "read_activity"}
	add := func(start, next int, data string) {
		receipts.addRead(snapshot, read, mustJSON(map[string]any{"id": "A1/V1", "offset": start, "next_offset": next, "data": data}))
	}
	add(6, -1, "last")
	if got := receipts.requiredReads(snapshot); len(got) != 1 || got[0].Arguments != `{"id":"A1/V1","offset":0}` {
		t.Fatal("last page hid earlier unread bytes", got)
	}
	add(0, 3, "one")
	if got := receipts.requiredReads(snapshot); len(got) != 1 || got[0].Arguments != `{"id":"A1/V1","offset":3}` {
		t.Fatal("required read did not resume at the gap", got)
	}
	if receipts.validate(snapshot) == nil {
		t.Fatal("gap was accepted as fully supplied evidence")
	}
	add(3, 6, "two")
	if got := receipts.requiredReads(snapshot); len(got) != 0 {
		t.Fatal("fully supplied activity still requires reads", got)
	}
	if err := receipts.validate(snapshot); err != nil {
		t.Fatal(err)
	}
}

func TestAgentCanReadPaginatedFrozenHistoricalActivity(t *testing.T) {
	f := newActivityFixture(t)
	f.addReview()
	f.inline[0]["body"] = strings.Repeat("Review finding 🔎. ", 5000)
	fresh, err := f.github.Enrich(context.Background(), f.source, f.event)
	if err != nil {
		t.Fatal(err)
	}
	var historicalRef string
	for i := range fresh.Evidence {
		if len(fresh.Evidence[i].Text) > 10000 {
			historicalRef = fresh.Evidence[i].ActivityRef
			fresh.Evidence[i].ContextOnly = true
			fresh.Evidence[i].Current = false
		}
	}
	refs := []store.NotificationActivityRef{}
	for _, ref := range fresh.ActivityDelta.Activities {
		if ref.Ref != historicalRef {
			refs = append(refs, ref)
		}
	}
	fresh.ActivityDelta.Activities = refs
	sub := AgentSubject{ID: "S1", EventIDs: []int64{1}, Event: fresh}
	updateAgentSubject(&sub, fresh)
	snapshot := AgentSnapshot{Subjects: []AgentSubject{sub}}
	f.model.respond = func(prompt string) (any, error) {
		_, raw, ok := strings.Cut(prompt, "\nREAD RESULTS:\n")
		if !ok {
			t.Fatal("missing read results")
		}
		var reads []AgentReadResult
		if err := json.Unmarshal([]byte(raw), &reads); err != nil {
			t.Fatal(err)
		}
		next := 0
		if len(reads) > 0 {
			var page struct {
				Next int `json:"next_offset"`
			}
			if err := json.Unmarshal([]byte(reads[len(reads)-1].Result), &page); err != nil {
				t.Fatal(err)
			}
			next = page.Next
		}
		if next != -1 {
			return AgentTurn{Action: "read", Reads: []AgentRead{{Name: "read_activity", Arguments: mustJSON(map[string]any{"id": historicalRef, "offset": next})}}}, nil
		}
		turn := finalResponse(snapshot, false)
		for _, ref := range fresh.ActivityDelta.Activities {
			turn.Decisions[0].ActivityRefs = append(turn.Decisions[0].ActivityRefs, ref.Ref)
		}
		return turn, nil
	}
	_, trace, err := f.s.runNotificationAgent(context.Background(), snapshot, Config{ContextBytes: 128000, MaxReads: 12})
	if err != nil {
		t.Fatal(err)
	}
	if len(trace) < 2 {
		t.Fatal("large frozen activity did not require continuation reads", trace)
	}
	for _, read := range trace {
		if read.Unavailable || read.Name != "read_activity" {
			t.Fatal("frozen activity lookup failed", read)
		}
	}
}

func TestActivityVersionRestoredToOldTextIsANewRevision(t *testing.T) {
	f := newActivityFixture(t)
	f.addReview()
	if p := f.process(t); p.Status != "completed" {
		t.Fatal(p)
	}
	original := f.inline[0]["body"]
	for _, text := range []any{"Changed review request", original} {
		f.inline[0]["body"] = text
		f.now = f.now.Add(2 * time.Minute)
		f.inline[0]["updated_at"] = f.now.Format(time.RFC3339)
		f.event.Version = f.now.Format(time.RFC3339)
		if p := f.process(t); p.Status != "completed" {
			t.Fatal(p)
		}
	}
	var count int
	f.s.DB.QueryRow(`SELECT max(revision) FROM notification_activity_versions`).Scan(&count)
	if count != 3 {
		t.Fatal("restored edit was lost", count)
	}
}

func TestSubmittedReviewUpgradesAnOlderLatestCommentPointer(t *testing.T) {
	f := newActivityFixture(t)
	f.addReview()
	f.event.LatestCommentURL = "https://api.github.com/repos/acme/repo/pulls/comments/101"
	p := f.process(t)
	if p.Status != "completed" {
		t.Fatal(p)
	}
	var raw string
	f.s.DB.QueryRow(`SELECT processor_snapshot FROM notification_batches WHERE id=?`, p.ID).Scan(&raw)
	var snap AgentSnapshot
	json.Unmarshal([]byte(raw), &snap)
	if len(snap.Subjects[0].Event.ActivityDelta.Activities) != 2 {
		t.Fatal("drafted inline comment was treated as history", snap.Subjects[0])
	}
}
func TestNewReviewWithOldSubmissionTimestampStillLoadsItsInlineComments(t *testing.T) {
	f := newActivityFixture(t)
	if p := f.process(t); p.Status != "completed" {
		t.Fatal(p)
	}
	f.now = f.now.Add(time.Hour)
	f.event.Version = f.now.Format(time.RFC3339)
	f.addReview()
	f.reviews[0]["submitted_at"] = f.now.Add(-24 * time.Hour).Format(time.RFC3339)
	f.paths = nil
	p := f.process(t)
	if p.Status != "completed" {
		t.Fatal(p)
	}
	found := false
	for _, path := range f.paths {
		found = found || strings.Contains(path, "/reviews/100/comments?")
	}
	if !found {
		t.Fatal("new review was missed based on its timestamp", f.paths)
	}
}

func TestReviewArrivingDuringInferenceIsMergedEvenWithoutPRVersionChange(t *testing.T) {
	f := newActivityFixture(t)
	added := false
	f.model.onGenerate = func(string) {
		if !added {
			added = true
			f.addReview()
		}
	}
	p := f.process(t)
	if p.Status != "completed" {
		t.Fatal(p)
	}
	if len(f.model.prompts) != 2 || !strings.Contains(f.model.prompts[1], "FINAL SOURCE NOTES") {
		t.Fatal("new review was not included in bounded final pass", len(f.model.prompts))
	}
	var processed int
	f.s.DB.QueryRow(`SELECT count(*) FROM notification_activity_versions WHERE context_only=0 AND processed_batch_id IS NOT NULL`).Scan(&processed)
	if processed != 2 {
		t.Fatal("new review was not committed", processed)
	}
}
func TestTimestampOnlyCommentUpdateDoesNotCreateAnotherActivityVersion(t *testing.T) {
	f := newActivityFixture(t)
	f.addReview()
	if p := f.process(t); p.Status != "completed" {
		t.Fatal(p)
	}
	f.now = f.now.Add(2 * time.Minute)
	f.event.Version = f.now.Format(time.RFC3339)
	f.inline[0]["updated_at"] = f.event.Version
	p := f.process(t)
	if p.Status != "completed" {
		t.Fatal(p)
	}
	var maxRevision int
	f.s.DB.QueryRow(`SELECT max(revision) FROM notification_activity_versions`).Scan(&maxRevision)
	if maxRevision != 1 {
		t.Fatal("timestamp-only edit was treated as new feedback", maxRevision)
	}
}

func TestCachedUndecidedCommentIsVerifiedInsteadOfReplayingDeletedText(t *testing.T) {
	f := newActivityFixture(t)
	f.addReview()
	// The review itself is handled; an edited inline comment remains undecided.
	if p := f.process(t); p.Status != "completed" {
		t.Fatal(p)
	}
	f.inline[0]["body"] = "New request that is subsequently deleted"
	fresh, err := f.github.Enrich(context.Background(), f.source, f.event)
	if err != nil {
		t.Fatal(err)
	}
	if len(fresh.ActivityDelta.Activities) != 1 {
		t.Fatal(fresh.ActivityDelta)
	}
	f.inline = nil
	original := f.github.Run
	f.github.Run = func(ctx context.Context, args ...string) ([]byte, error) {
		if args[len(args)-1] == "repos/acme/repo/pulls/comments/101" {
			return errorResponse(404), errors.New("not found")
		}
		return original(ctx, args...)
	}
	fresh, err = f.github.Enrich(context.Background(), f.source, f.event)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, e := range fresh.Evidence {
		if e.Unavailable {
			found = true
		}
		if strings.Contains(e.Text, "New request that is subsequently deleted") {
			t.Fatal("deleted instruction was replayed", e)
		}
	}
	if !found || len(fresh.ActivityDelta.Activities) != 1 {
		t.Fatal(fresh)
	}
}

func TestActivityReadIsSupersededWhenItsSubjectChanges(t *testing.T) {
	trace := []AgentReadResult{{ID: "read:1", Name: "read_activity", SubjectID: "S1", Arguments: `{"id":"A1/V1","offset":0}`, Result: `{"data":"old instruction"}`}, {ID: "read:2", Name: "read_activity", SubjectID: "S2"}}
	supersedeSourceReads(trace, []AgentSubject{{ID: "S1"}})
	if !trace[0].Superseded || trace[1].Superseded {
		t.Fatal(trace)
	}
	promptReads := agentTraceForPrompt(trace, false)
	if strings.Contains(promptReads[0].Result, "old instruction") {
		t.Fatal("obsolete activity remained in prompt", promptReads)
	}
}

func TestExplicitReprocessingIncludesHandledVersionsWithoutChangingTheirReceipt(t *testing.T) {
	f := newActivityFixture(t)
	f.addReview()
	first := f.process(t)
	if first.Status != "completed" {
		t.Fatal(first)
	}
	var id int64
	f.s.DB.QueryRow(`SELECT id FROM notification_events LIMIT 1`).Scan(&id)
	if err := f.s.SetRawStatus(context.Background(), id, "pending"); err != nil {
		t.Fatal(err)
	}
	second := processAgent(t, f.s)
	if second.Status != "completed" {
		t.Fatal(second)
	}
	var raw string
	f.s.DB.QueryRow(`SELECT processor_snapshot FROM notification_batches WHERE id=?`, second.ID).Scan(&raw)
	var snap AgentSnapshot
	json.Unmarshal([]byte(raw), &snap)
	if !snap.Subjects[0].Event.ActivityDelta.Replay || len(snap.Subjects[0].Event.ActivityDelta.Activities) != 2 {
		t.Fatal(snap.Subjects[0])
	}
	var original int
	f.s.DB.QueryRow(`SELECT count(*) FROM notification_activity_versions WHERE processed_batch_id=?`, first.ID).Scan(&original)
	if original != 2 {
		t.Fatal("reprocessing rewrote original version receipts", original)
	}
	var replay bool
	f.s.DB.QueryRow(`SELECT force_replay FROM notification_events WHERE id=?`, id).Scan(&replay)
	if replay {
		t.Fatal("reprocessing flag was not cleared")
	}
}
func TestRulePreviewCanReevaluatePreviouslyHandledActivity(t *testing.T) {
	f := newActivityFixture(t)
	f.addReview()
	first := f.process(t)
	if first.Status != "completed" {
		t.Fatal(first)
	}
	var id int64
	var rules string
	f.s.DB.QueryRow(`SELECT id FROM notification_events LIMIT 1`).Scan(&id)
	f.s.DB.QueryRow(`SELECT active_set_id FROM notification_rule_state WHERE id=1`).Scan(&rules)
	preview, err := f.s.QueueRulePreview(context.Background(), rules, []int64{id})
	if err != nil {
		t.Fatal(err)
	}
	if err = f.s.processRulePreview(context.Background()); err != nil {
		t.Fatal(err)
	}
	got, err := f.s.RulePreview(context.Background(), preview.ID)
	if err != nil || got.Status != "completed" {
		t.Fatal(got, err)
	}
	var turn AgentTurn
	if err = json.Unmarshal(got.Result, &turn); err != nil {
		t.Fatal(err)
	}
	if len(turn.Decisions) != 1 || len(turn.Decisions[0].ActivityRefs) != 2 {
		t.Fatal("preview lost handled evidence", turn)
	}
	var count int
	f.s.DB.QueryRow(`SELECT count(*) FROM notification_activity_versions WHERE processed_batch_id=?`, first.ID).Scan(&count)
	if count != 2 {
		t.Fatal("preview changed processing receipts", count)
	}
}

func TestNewReleaseBodyIsActivityRatherThanHistoricalDescription(t *testing.T) {
	f := newActivityFixture(t)
	e := f.event
	e.URL = "https://github.com/acme/repo/releases/tag/v1"
	e.SubjectURL = "https://api.github.com/repos/acme/repo/releases/1"
	e.SubjectType = "Release"
	f.github.Run = func(context.Context, ...string) ([]byte, error) {
		return response(`{"body":"@alice please check this release","html_url":"https://github.com/acme/repo/releases/tag/v1","user":{"login":"maintainer"},"created_at":"2026-01-01T00:00:00Z","updated_at":"2026-01-01T00:00:00Z"}`, ""), nil
	}
	fresh, err := f.github.Enrich(context.Background(), f.source, e)
	if err != nil {
		t.Fatal(err)
	}
	mentioned := mentionStatus([]RawNotification{{Event: e}}, fresh, "alice")
	if len(fresh.ActivityDelta.Activities) != 1 || mentioned == nil || !*mentioned {
		t.Fatal("new release mention was lost", fresh, mentioned)
	}
}
func TestNewIssueBodyMentionsRemainCurrent(t *testing.T) {
	f := newActivityFixture(t)
	e := f.event
	e.URL = "https://github.com/acme/repo/issues/7"
	e.SubjectURL = "https://api.github.com/repos/acme/repo/issues/7"
	e.SubjectType = "Issue"
	f.github.Run = func(_ context.Context, args ...string) ([]byte, error) {
		if strings.Contains(args[len(args)-1], "/comments?") {
			return response(`[]`, ""), nil
		}
		return response(`{"body":"@alice investigate this issue","html_url":"https://github.com/acme/repo/issues/7","user":{"login":"maintainer"},"created_at":"2026-11-12T09:59:00Z","updated_at":"2026-11-12T09:59:00Z","state":"open"}`, ""), nil
	}
	fresh, err := f.github.Enrich(context.Background(), f.source, e)
	if err != nil {
		t.Fatal(err)
	}
	mentioned := mentionStatus([]RawNotification{{Event: e}}, fresh, "alice")
	if len(fresh.ActivityDelta.Activities) != 1 || mentioned == nil || !*mentioned {
		t.Fatal("new issue-body mention was lost", fresh, mentioned)
	}
}

func TestReleaseBodyChangeDuringInferenceDoesNotNeedUpdatedAt(t *testing.T) {
	f := newActivityFixture(t)
	f.event.URL = "https://github.com/acme/repo/releases/tag/v1"
	f.event.SubjectURL = "https://api.github.com/repos/acme/repo/releases/1"
	f.event.SubjectType = "Release"
	body := "Initial release announcement"
	f.github.Run = func(_ context.Context, args ...string) ([]byte, error) {
		if args[len(args)-1] == "user" {
			return response(`{"login":"alice"}`, ""), nil
		}
		raw, _ := json.Marshal(map[string]any{"body": body, "html_url": f.event.URL, "user": map[string]string{"login": "maintainer"}, "created_at": "2026-01-01T00:00:00Z"})
		return response(string(raw), ""), nil
	}
	f.s.Providers["github"] = f.github
	f.model.onGenerate = func(string) { body = "Updated release announcement requiring attention" }
	p := f.process(t)
	if p.Status != "completed" {
		t.Fatal(p)
	}
	if len(f.model.prompts) != 2 || !strings.Contains(f.model.prompts[1], "Updated release announcement requiring attention") {
		t.Fatal("release update was missed", len(f.model.prompts))
	}
}

func TestTimestampOnlySourceChangeDoesNotRequireAnotherDecisionRound(t *testing.T) {
	f := newActivityFixture(t)
	f.addReview()
	f.model.onGenerate = func(string) {
		f.event.Version = f.now.Format(time.RFC3339)
		f.inline[0]["updated_at"] = f.event.Version
	}
	p := f.process(t)
	if p.Status != "completed" {
		t.Fatal(p)
	}
	if len(f.model.prompts) != 1 {
		t.Fatal("timestamp-only change repeated inference", len(f.model.prompts))
	}
	var metrics map[string]int64
	var raw string
	f.s.DB.QueryRow(`SELECT activity_metrics FROM notification_batches WHERE id=?`, p.ID).Scan(&raw)
	if err := json.Unmarshal([]byte(raw), &metrics); err != nil {
		t.Fatal(err)
	}
	if metrics["decision_rounds"] != 1 || metrics["new_versions"] != 2 || metrics["github_requests"] == 0 {
		t.Fatal(metrics)
	}
}

func TestOlderDiscussionLookupIsScopedAndFrozenAtTheScan(t *testing.T) {
	f := newActivityFixture(t)
	f.addReview()
	if p := f.process(t); p.Status != "completed" {
		t.Fatal(p)
	}
	f.now = f.now.Add(10 * time.Minute)
	f.event.Version = f.now.Format(time.RFC3339)
	next := activityComment(300, "Same concern as before", "reviewer", f.event.Version)
	next["html_url"] = f.event.URL + "#issuecomment-300"
	f.comments = []map[string]any{next}
	fresh, err := f.github.Enrich(context.Background(), f.source, f.event)
	if err != nil {
		t.Fatal(err)
	}
	sub := AgentSubject{ID: "S1", Event: fresh}
	updateAgentSubject(&sub, fresh)
	snap := AgentSnapshot{Subjects: []AgentSubject{sub}}
	result, err := f.s.readThread(context.Background(), snap, `{"id":"S1","offset":0,"review_id":100}`)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(result, "A new regression needs fixing") || !strings.Contains(result, `"historical_context":true`) {
		t.Fatal(result)
	}
	var ref string
	f.s.DB.QueryRow(`SELECT json_extract(v.payload,'$.activity_ref') FROM notification_activities a JOIN notification_activity_versions v ON v.activity_id=a.id AND v.revision=a.revision WHERE a.resource_url LIKE '%#discussion_r101'`).Scan(&ref)
	f.inline[0]["body"] = "A later edit that must not enter the earlier snapshot"
	if _, err = f.github.Enrich(context.Background(), f.source, f.event); err != nil {
		t.Fatal(err)
	}
	result, err = f.s.readThread(context.Background(), snap, `{"id":"S1","offset":0,"review_id":100}`)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(result, "A later edit") || !strings.Contains(result, "A new regression needs fixing") {
		t.Fatal("history was not frozen", result)
	}
	page, err := f.s.agentRead(context.Background(), snap, AgentRead{Name: "read_activity", Arguments: mustJSON(map[string]any{"id": ref, "offset": 0})}, Config{ContextBytes: 1500000, MaxReads: 12})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(page, `"historical_context":true`) || !strings.Contains(page, `"subject_id":"S1"`) {
		t.Fatal(page)
	}
	var latest string
	f.s.DB.QueryRow(`SELECT json_extract(v.payload,'$.activity_ref') FROM notification_activities a JOIN notification_activity_versions v ON v.activity_id=a.id AND v.revision=a.revision WHERE a.resource_url LIKE '%#discussion_r101'`).Scan(&latest)
	if _, _, err = f.s.lookupActivity(context.Background(), snap, latest); err == nil {
		t.Fatal("later version leaked into frozen snapshot")
	}
	if _, err = f.s.readThread(context.Background(), snap, `{"id":"S2","offset":0}`); err == nil {
		t.Fatal("outside subject lookup accepted")
	}
}
