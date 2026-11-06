package notifications

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"kamacu/internal/monitor"
)

// Barriers establish overlap without relying on performance or sleep thresholds.
func awaitNotificationSignal(t *testing.T, signal <-chan struct{}) {
	t.Helper()
	select {
	case <-signal:
	case <-time.After(5 * time.Second):
		t.Fatal("concurrent work did not reach barrier")
	}
}

func TestNotificationWorkersBoundAndJoinOnFailure(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	entered := make(chan struct{}, 3)
	var active, peak atomic.Int64
	boom := errors.New("retrieval failed")
	err := parallelNotifications(ctx, 20, 3, func(ctx context.Context, i int) error {
		n := active.Add(1)
		defer active.Add(-1)
		for old := peak.Load(); n > old; old = peak.Load() {
			if peak.CompareAndSwap(old, n) {
				break
			}
		}
		entered <- struct{}{}
		if i == 0 {
			for range 3 {
				select {
				case <-entered:
				case <-ctx.Done():
					return ctx.Err()
				}
			}
			return boom
		}
		<-ctx.Done()
		return ctx.Err()
	})
	if !errors.Is(err, boom) || peak.Load() != 3 || active.Load() != 0 {
		t.Fatalf("err=%v peak=%d active=%d", err, peak.Load(), active.Load())
	}
}

func TestNotificationSubjectLimitIsSharedAcrossProjects(t *testing.T) {
	ctx, cancel := context.WithTimeout(withNotificationRun(context.Background(), "test"), 5*time.Second)
	defer cancel()
	entered := make(chan struct{}, 4)
	release := make(chan struct{})
	var active, peak atomic.Int64
	done := make(chan error, 1)
	go func() {
		done <- parallelNotifications(ctx, 3, 3, func(ctx context.Context, _ int) error {
			return parallelNotificationSubjects(ctx, 6, func(ctx context.Context, _ int) error {
				n := active.Add(1)
				defer active.Add(-1)
				for old := peak.Load(); n > old; old = peak.Load() {
					if peak.CompareAndSwap(old, n) {
						break
					}
				}
				select {
				case entered <- struct{}{}:
				default:
				}
				select {
				case <-release:
					return nil
				case <-ctx.Done():
					return ctx.Err()
				}
			})
		})
	}()
	for range 4 {
		awaitNotificationSignal(t, entered)
	}
	close(release)
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	if peak.Load() != 4 || active.Load() != 0 {
		t.Fatalf("peak=%d active=%d", peak.Load(), active.Load())
	}
}

func TestNotificationGitHubRequestLimitAndCancellation(t *testing.T) {
	ctx, cancel := context.WithTimeout(withNotificationRun(context.Background(), "test"), 5*time.Second)
	defer cancel()
	entered := make(chan struct{}, 4)
	var active, peak atomic.Int64
	g := GitHub{Run: func(ctx context.Context, _ ...string) ([]byte, error) {
		n := active.Add(1)
		defer active.Add(-1)
		for old := peak.Load(); n > old; old = peak.Load() {
			if peak.CompareAndSwap(old, n) {
				break
			}
		}
		entered <- struct{}{}
		<-ctx.Done()
		return nil, ctx.Err()
	}}
	done := make(chan error, 1)
	go func() {
		done <- parallelNotifications(ctx, 12, 12, func(ctx context.Context, _ int) error { _, _, err := g.api(ctx, "github.com", "user"); return err })
	}()
	for range 4 {
		awaitNotificationSignal(t, entered)
	}
	cancel()
	if err := <-done; !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	if peak.Load() != 4 || active.Load() != 0 {
		t.Fatalf("peak=%d active=%d", peak.Load(), active.Load())
	}
}

type concurrentNotificationModel struct {
	draftEntered chan struct{}
	draftRelease <-chan struct{}
	entered      chan struct{}
	release      <-chan struct{}
	failRepo     string
	failStage    string
}

func (m *concurrentNotificationModel) GenerateValidatedJSON(ctx context.Context, _ monitor.Profile, prompt string, _ any, validate func([]byte) error) ([]byte, error) {
	if raw, ok, err := fixtureOwnershipResponse(prompt); ok {
		if err == nil {
			err = validate(raw)
		}
		return raw, err
	}
	if strings.Contains(prompt, projectEvidenceMarker) {
		if m.entered != nil {
			select {
			case m.entered <- struct{}{}:
			case <-ctx.Done():
				return nil, ctx.Err()
			}
		}
		if m.release != nil {
			select {
			case <-m.release:
			case <-ctx.Done():
				return nil, ctx.Err()
			}
		}
		if m.failStage != "drafting" && m.failRepo != "" && strings.Contains(prompt, `"repository":"`+m.failRepo+`"`) {
			return nil, errors.New("classification failed")
		}
		raw, err := marshalFixtureResponse(prompt, intentResponse{Findings: []intentFinding{{Intent: "direct_question", EvidenceID: "context", Quote: "Review requested"}}})
		if err == nil {
			err = validate(raw)
		}
		return raw, err
	}
	if m.draftEntered != nil {
		select {
		case m.draftEntered <- struct{}{}:
		case <-ctx.Done():
			return nil, ctx.Err()
		}
		select {
		case <-m.draftRelease:
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	if m.failStage == "drafting" && strings.Contains(prompt, `"repository":"`+m.failRepo+`"`) {
		return nil, errors.New("drafting failed")
	}
	raw, err := fixtureProjectCardsJSON(prompt, nil)
	if err == nil {
		err = validate(raw)
	}
	return raw, err
}

func TestConcurrentProjectsKeepOrderAndPublishAtomically(t *testing.T) {
	for _, failStage := range []string{"", "classification", "drafting"} {
		t.Run("failure="+failStage, func(t *testing.T) {
			fail := failStage != ""
			s, _, _, source := fixture(t)
			// Include more projects than workers, each with its own notification scope.
			for i := range 5 {
				e := event()
				e.ExternalID = fmt.Sprintf("thread-%d", i)
				e.Repository = fmt.Sprintf("acme/project-%d", i)
				e.URL = fmt.Sprintf("https://github.com/%s/pull/1", e.Repository)
				seed(t, s, source, e)
			}
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			if _, err := s.RequestProcessing(ctx); err != nil {
				t.Fatal(err)
			}
			entered := make(chan struct{}, 5)
			release := make(chan struct{})
			model := &concurrentNotificationModel{entered: entered, release: release}
			draftEntered := make(chan struct{}, 5)
			draftRelease := make(chan struct{})
			if failStage != "classification" {
				model.draftEntered = draftEntered
				model.draftRelease = draftRelease
			}
			if fail {
				model.failRepo = "acme/project-4"
				model.failStage = failStage
			}
			s.Model = model
			done := make(chan error, 1)
			go func() { done <- s.processNext(ctx) }()
			for range 3 {
				awaitNotificationSignal(t, entered)
			}
			var before int
			if err := s.DB.QueryRow(`SELECT count(*) FROM inbox_questions`).Scan(&before); err != nil || before != 0 {
				t.Fatalf("partial publication before classification completed: %d %v", before, err)
			}
			close(release)
			if model.draftEntered != nil {
				for range 3 {
					awaitNotificationSignal(t, draftEntered)
				}
				if err := s.DB.QueryRow(`SELECT count(*) FROM inbox_questions`).Scan(&before); err != nil || before != 0 {
					t.Fatalf("partial publication during drafting: %d %v", before, err)
				}
				close(draftRelease)
			}
			if err := <-done; err != nil {
				t.Fatal(err)
			}
			status, err := s.Status(ctx)
			if err != nil {
				t.Fatal(err)
			}
			var count int
			if err = s.DB.QueryRow(`SELECT count(*) FROM inbox_questions`).Scan(&count); err != nil {
				t.Fatal(err)
			}
			if fail {
				if status.Processing.Status != "failed" || count != 0 {
					t.Fatalf("failed project partially published: %+v count=%d", status.Processing, count)
				}
				var archived int
				if err = s.DB.QueryRow(`SELECT count(*) FROM notification_events WHERE raw_status='archived'`).Scan(&archived); err != nil || archived != 0 {
					t.Fatalf("failed batch archived notifications: %d %v", archived, err)
				}
				return
			}
			if status.Processing.Status != "completed" || count != 5 {
				t.Fatalf("%+v count=%d", status.Processing, count)
			}
			rows, err := s.DB.Query(`SELECT source_url FROM inbox_questions ORDER BY rowid`)
			if err != nil {
				t.Fatal(err)
			}
			defer rows.Close()
			i := 0
			for rows.Next() {
				var url string
				if err = rows.Scan(&url); err != nil {
					t.Fatal(err)
				}
				want := fmt.Sprintf("https://github.com/acme/project-%d/pull/1", i)
				if url != want {
					t.Fatalf("publication order=%s, want %s", url, want)
				}
				i++
			}
			if err = rows.Err(); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestNotificationStageTimingIncludesFailuresAndScope(t *testing.T) {
	var buf bytes.Buffer
	old := slog.Default()
	slog.SetDefault(slog.New(slog.NewJSONHandler(&buf, nil)))
	defer slog.SetDefault(old)
	ctx := withNotificationRun(context.Background(), "timed-batch")
	ctx = context.WithValue(ctx, notificationProjectKey{}, "acme/repo")
	boom := errors.New("failed")
	if err := notificationStage(ctx, "classification", func() error { return boom }); !errors.Is(err, boom) {
		t.Fatal(err)
	}
	var entry map[string]any
	if err := json.Unmarshal(bytes.TrimSpace(buf.Bytes()), &entry); err != nil {
		t.Fatal(err)
	}
	if entry["stage"] != "classification" || entry["batch"] != "timed-batch" || entry["repository"] != "acme/repo" || entry["error"] != "failed" || entry["duration_ms"] == nil {
		t.Fatal(entry)
	}
}

// A provider can be entered concurrently even though SQLite access is serialized.
type overlappingNotificationProvider struct {
	*fakeProvider
	entered        chan struct{}
	release        <-chan struct{}
	versions       chan struct{}
	versionRelease <-chan struct{}
}

func (p *overlappingNotificationProvider) Enrich(ctx context.Context, source Source, e Event) (Event, error) {
	select {
	case p.entered <- struct{}{}:
	case <-ctx.Done():
		return e, ctx.Err()
	}
	select {
	case <-p.release:
	case <-ctx.Done():
		return e, ctx.Err()
	}
	fresh, err := p.fakeProvider.Enrich(ctx, source, e)
	fresh.SubjectUpdatedAt = "2026-11-06T01:00:00Z"
	return fresh, err
}

func (p *overlappingNotificationProvider) SubjectVersion(ctx context.Context, _ Source, e Event) (string, error) {
	select {
	case p.versions <- struct{}{}:
	case <-ctx.Done():
		return "", ctx.Err()
	}
	select {
	case <-p.versionRelease:
		return e.SubjectUpdatedAt, nil
	case <-ctx.Done():
		return "", ctx.Err()
	}
}

func TestSubjectRetrievalAndRevalidationOverlapInProcessing(t *testing.T) {
	s, model, _, source := fixture(t)
	entered := make(chan struct{}, 6)
	release := make(chan struct{})
	versions := make(chan struct{}, 6)
	versionRelease := make(chan struct{})
	s.Providers["github"] = &overlappingNotificationProvider{fakeProvider: &fakeProvider{}, entered: entered, release: release, versions: versions, versionRelease: versionRelease}
	for i := range 6 {
		e := event()
		e.ExternalID = fmt.Sprintf("thread-%d", i)
		e.URL = fmt.Sprintf("https://github.com/acme/repo/pull/%d", i+1)
		seed(t, s, source, e)
	}
	respondWithIntent(model, "direct_question")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if _, err := s.RequestProcessing(ctx); err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { done <- s.processNext(ctx) }()
	for range 4 {
		awaitNotificationSignal(t, entered)
	}
	close(release)
	for range 4 {
		awaitNotificationSignal(t, versions)
	}
	close(versionRelease)
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	status, err := s.Status(ctx)
	if err != nil || status.Processing.Status != "completed" {
		t.Fatalf("%+v %v", status, err)
	}
}

func TestNotificationSlotCancellationDoesNotLeakCapacity(t *testing.T) {
	for range 100 {
		ctx, cancel := context.WithCancel(context.Background())
		slots := make(chan struct{}, 1)
		slots <- struct{}{}
		done := make(chan error, 1)
		go func() {
			release, err := acquireNotificationSlot(ctx, slots)
			if release != nil {
				release()
			}
			done <- err
		}()
		cancel()
		<-slots
		if err := <-done; !errors.Is(err, context.Canceled) {
			t.Fatalf("queued worker ignored cancellation: %v", err)
		}
		if len(slots) != 0 {
			t.Fatal("cancelled acquisition leaked a slot")
		}
	}
}

type finalCheckNotificationProvider struct {
	*fakeProvider
	checks    atomic.Int64
	finalized chan struct{}
}

func (p *finalCheckNotificationProvider) CheckIdentity(context.Context, Source) error {
	if p.checks.Add(1) == 2 {
		close(p.finalized)
	}
	return nil
}

type sourcePauseNotificationModel struct {
	finalized <-chan struct{}
	waiting   chan struct{}
	release   <-chan struct{}
}

func (m *sourcePauseNotificationModel) GenerateValidatedJSON(ctx context.Context, _ monitor.Profile, prompt string, _ any, validate func([]byte) error) ([]byte, error) {
	if raw, ok, err := fixtureOwnershipResponse(prompt); ok {
		if err == nil {
			err = validate(raw)
		}
		return raw, err
	}
	if strings.Contains(prompt, projectEvidenceMarker) {
		raw, err := marshalFixtureResponse(prompt, intentResponse{Findings: []intentFinding{{Intent: "direct_question", EvidenceID: "context", Quote: "Review requested"}}})
		if err == nil {
			err = validate(raw)
		}
		return raw, err
	}
	if strings.Contains(prompt, `"repository":"acme/other"`) {
		select {
		case <-m.finalized:
		case <-ctx.Done():
			return nil, ctx.Err()
		}
		close(m.waiting)
		select {
		case <-m.release:
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	raw, err := fixtureProjectCardsJSON(prompt, nil)
	if err == nil {
		err = validate(raw)
	}
	return raw, err
}

func TestSourcePausedAfterItsProjectFinishesPreventsPublication(t *testing.T) {
	s, _, _, source := fixture(t)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	provider := &finalCheckNotificationProvider{fakeProvider: &fakeProvider{}, finalized: make(chan struct{})}
	s.Providers["github"] = provider
	s.Providers["second"] = &fakeProvider{}
	otherSource, err := s.AddSource(ctx, "second", "Second source", json.RawMessage(`{}`))
	if err != nil {
		t.Fatal(err)
	}
	seed(t, s, source, event())
	e := event()
	e.ExternalID = "second"
	e.Repository = "acme/other"
	e.URL = "https://github.com/acme/other/pull/1"
	seed(t, s, otherSource, e)
	release := make(chan struct{})
	model := &sourcePauseNotificationModel{finalized: provider.finalized, waiting: make(chan struct{}), release: release}
	s.Model = model
	if _, err = s.RequestProcessing(ctx); err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { done <- s.processNext(ctx) }()
	// The first project's post-draft identity check has succeeded. The other
	// source's project is still drafting, so only the final barrier can catch it.
	awaitNotificationSignal(t, model.waiting)
	if err = s.EnableSource(ctx, source.ID, false); err != nil {
		t.Fatal(err)
	}
	close(release)
	if err = <-done; err != nil {
		t.Fatal(err)
	}
	status, err := s.Status(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if status.Processing.Status != "failed" || !strings.Contains(status.Processing.Error, "source is paused") {
		t.Fatalf("paused source was published: %+v", status.Processing)
	}
	var cards, archived int
	if err = s.DB.QueryRow(`SELECT count(*) FROM inbox_questions`).Scan(&cards); err != nil {
		t.Fatal(err)
	}
	if err = s.DB.QueryRow(`SELECT count(*) FROM notification_events WHERE raw_status='archived'`).Scan(&archived); err != nil {
		t.Fatal(err)
	}
	if cards != 0 || archived != 0 {
		t.Fatalf("partial publication: cards=%d archived=%d", cards, archived)
	}
}
