package monitor

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"kamacu/internal/inbox"
	"kamacu/internal/observation"
	"kamacu/internal/session"
	"kamacu/internal/store"
)

func generationFixture(t *testing.T, classifier Classifier) (*Service, *session.Session, Profile, string) {
	t.Helper()
	svc, old, p, _ := reviewFixture(t, classifier)
	old.Stop()
	dir := t.TempDir()
	bin := filepath.Join(dir, "agent")
	script := `#!/bin/sh
printf 'Testing\r\n• Working (4s • esc to interrupt)'
while [ ! -f repaint ]; do
 if [ -f noisy ] && [ ! -f logged ]; then
  printf '\033[2J\033[HTesting\r\nerror: retrying; done=0; value?.name ?? fallback\r\n• Working (4s • esc to interrupt)'
  touch logged
 fi
 sleep 0.01
done
printf '\033[2J\033[HTesting\r\n• Working (14s • esc to interrupt)'
while [ ! -f dialog ]; do sleep 0.01; done
printf '\033[2J\033[HAllow access?\r\n1. Yes\r\n2. No\r\nPress enter to confirm or Esc to cancel\r\n• Working (14s • esc to interrupt)'
while [ ! -f complete ]; do sleep 0.01; done
printf '\033[2J\033[HAll done\r\n> '
exec sleep 60
`
	if err := os.WriteFile(bin, []byte(script), 0700); err != nil {
		t.Fatal(err)
	}
	sess, err := svc.Sessions.Spawn(session.SpawnOpts{Kind: session.KindAgent, AgentEngine: "codex", AgentArgs: []string{bin}, Cwd: dir, Global: true})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { sess.Stop() })
	eventually(t, func() bool { return strings.Contains(sess.Observation().Screen, "Working (4s") })
	svc.Inbox.Close()
	svc.Inbox = inbox.New(svc.DB, func(id string) inbox.Session {
		return inbox.Session{Global: true, Live: id == sess.Info().ID && sess.Info().Status == session.StatusRunning, Done: sess.Done()}
	})
	t.Cleanup(svc.Inbox.Close)
	svc.Metrics = &Metrics{}
	return svc, sess, p, dir
}

func advanceGeneration(t *testing.T, sess *session.Session, dir, file, screen string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, file), nil, 0600); err != nil {
		t.Fatal(err)
	}
	eventually(t, func() bool { return strings.Contains(sess.Observation().Screen, screen) })
}

func TestServiceDefersGenerationSkipsCosmeticsAndHandlesDialogs(t *testing.T) {
	var calls atomic.Int32
	svc, sess, p, dir := generationFixture(t, classifierFunc(func(_ context.Context, _ Profile, s observation.Snapshot) (Decision, error) {
		calls.Add(1)
		if strings.Contains(s.Screen, "Allow access?") {
			return Decision{State: "waiting", Kind: "permission", Evidence: "Allow access?", Question: "Allow access?", InputMode: "terminal", WorkSummary: "Fabricated update", WorkEvidence: "not on screen", PRLinks: []WorkLink{{Repository: "acme/repo", Number: 1, Role: "implementation", Evidence: "invented"}}}, nil
		}
		if strings.Contains(s.Screen, "All done") {
			return Decision{State: "idle", Kind: "completion", Evidence: "All done", Summary: "All done", InputMode: "none"}, nil
		}
		return Decision{State: "working", Kind: "none", Evidence: "Testing", InputMode: "none", WorkSummary: "Running checks", WorkEvidence: "Testing"}, nil
	}))
	ctx := context.Background()
	id := sess.Info().ID
	svc.tick(ctx)
	svc.jobs.Wait()
	if calls.Load() != 0 {
		t.Fatal("inference ran during initial generation")
	}
	svc.mu.Lock()
	svc.states[id].observationStarted = time.Now().Add(-generationFallback)
	svc.mu.Unlock()
	svc.tick(ctx)
	svc.jobs.Wait()
	if calls.Load() != 1 {
		t.Fatal("periodic fallback missing")
	}
	advanceGeneration(t, sess, dir, "repaint", "Working (14s")
	svc.tick(ctx)
	svc.jobs.Wait()
	if calls.Load() != 1 || svc.Metrics.Snapshot()[p.ID].CosmeticReuses != 1 {
		t.Fatal("cosmetic repaint invoked inference", calls.Load(), svc.Metrics.Snapshot())
	}
	// Even after cosmetic reuse, the periodic fallback must still run.
	svc.mu.Lock()
	svc.states[id].lastAttempt = time.Now().Add(-generationFallback)
	svc.mu.Unlock()
	svc.tick(ctx)
	svc.jobs.Wait()
	if calls.Load() != 2 {
		t.Fatal("cosmetic skip starved fallback")
	}
	advanceGeneration(t, sess, dir, "dialog", "Allow access?")
	svc.tick(ctx)
	svc.jobs.Wait()
	if calls.Load() != 3 {
		t.Fatal("dialog delayed during generation")
	}
	page, err := svc.Inbox.List(ctx, nil, false, 0)
	if err != nil || len(page.Items) != 1 || page.Items[0].Question != "Allow access?" {
		t.Fatalf("valid permission lost: %+v %v", page, err)
	}
	work, err := store.WorkForSession(ctx, svc.DB, id)
	if err != nil || work == nil || work.Summary != "Running checks" {
		t.Fatal("invalid update replaced previous valid work", work, err)
	}
	links, err := store.SessionPRLinks(ctx, svc.DB, id)
	if err != nil || len(links) != 0 {
		t.Fatal("invented PR persisted", links, err)
	}
	got := svc.Metrics.Snapshot()[p.ID]
	if got.FallbackCalls != 2 || got.DroppedWorkUpdates != 1 || got.DroppedPRLinks != 1 || got.AcceptedDecisions != 3 {
		t.Fatalf("missing telemetry: %+v", got)
	}
	advanceGeneration(t, sess, dir, "complete", "All done")
	svc.tick(ctx)
	svc.jobs.Wait()
	if calls.Load() != 4 || sess.Info().AgentStatus != "idle" {
		t.Fatal("generation stop did not classify completion", calls.Load(), sess.Info())
	}
}

func TestServiceAcceptsRoutineResultAcrossCosmeticRepaint(t *testing.T) {
	started, release := make(chan struct{}), make(chan struct{})
	svc, sess, p, dir := generationFixture(t, classifierFunc(func(ctx context.Context, _ Profile, s observation.Snapshot) (Decision, error) {
		close(started)
		select {
		case <-release:
		case <-ctx.Done():
			return Decision{}, ctx.Err()
		}
		return Decision{State: "working", Kind: "none", Evidence: "• Working (4s • esc to interrupt)", InputMode: "none"}, nil
	}))
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	svc.tick(ctx)
	svc.mu.Lock()
	svc.states[sess.Info().ID].observationStarted = time.Now().Add(-generationFallback)
	svc.mu.Unlock()
	svc.tick(ctx)
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("fallback not started")
	}
	advanceGeneration(t, sess, dir, "repaint", "Working (14s")
	svc.tick(ctx)
	close(release)
	svc.jobs.Wait()
	got := svc.Metrics.Snapshot()[p.ID]
	if got.AcceptedDecisions != 1 || got.StaleResults != 0 {
		t.Fatalf("cosmetic repaint discarded result: %+v", got)
	}
	svc.mu.Lock()
	w := svc.states[sess.Info().ID]
	revision, evidence := w.revision, w.decision.Evidence
	svc.mu.Unlock()
	if revision != sess.Observation().Revision || !strings.Contains(sess.Observation().Screen, evidence) {
		t.Fatal("decision not rebased to actual evidence", revision, evidence)
	}
}

func TestServiceDialogPreemptsGenerationInference(t *testing.T) {
	var calls atomic.Int32
	started := make(chan struct{})
	svc, sess, p, dir := generationFixture(t, classifierFunc(func(ctx context.Context, _ Profile, s observation.Snapshot) (Decision, error) {
		if calls.Add(1) == 1 {
			close(started)
			<-ctx.Done()
			return Decision{}, ctx.Err()
		}
		return Decision{State: "waiting", Kind: "question", Evidence: "Allow access?", Question: "Allow access?", InputMode: "terminal"}, nil
	}))
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	svc.tick(ctx)
	svc.mu.Lock()
	svc.states[sess.Info().ID].observationStarted = time.Now().Add(-generationFallback)
	svc.mu.Unlock()
	svc.tick(ctx)
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("fallback not started")
	}
	advanceGeneration(t, sess, dir, "repaint", "Working (14s")
	advanceGeneration(t, sess, dir, "dialog", "Allow access?")
	svc.tick(ctx)
	svc.jobs.Wait()
	svc.tick(ctx)
	svc.jobs.Wait()
	if got := svc.Metrics.Snapshot()[p.ID]; got.ObsoleteResults != 1 || got.AcceptedDecisions != 1 {
		t.Fatalf("preemption telemetry: %+v", got)
	}
	if calls.Load() != 2 || sess.Info().AgentStatus != "waiting" {
		t.Fatal("dialog did not preempt inference", calls.Load(), sess.Info())
	}
}

func TestServiceRejectsAttentionAcrossCosmeticRepaint(t *testing.T) {
	started, release := make(chan struct{}), make(chan struct{})
	svc, sess, p, dir := generationFixture(t, classifierFunc(func(ctx context.Context, _ Profile, s observation.Snapshot) (Decision, error) {
		close(started)
		select {
		case <-release:
		case <-ctx.Done():
			return Decision{}, ctx.Err()
		}
		return Decision{State: "waiting", Kind: "question", Evidence: "Testing", Question: "Continue?", InputMode: "text"}, nil
	}))
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	svc.tick(ctx)
	svc.mu.Lock()
	svc.states[sess.Info().ID].observationStarted = time.Now().Add(-generationFallback)
	svc.mu.Unlock()
	svc.tick(ctx)
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("fallback not started")
	}
	advanceGeneration(t, sess, dir, "repaint", "Working (14s")
	close(release)
	svc.jobs.Wait()
	got := svc.Metrics.Snapshot()[p.ID]
	page, err := svc.Inbox.List(ctx, nil, false, 0)
	if got.StaleResults != 1 || got.AcceptedDecisions != 0 || err != nil || len(page.Items) != 0 {
		t.Fatalf("stale question accepted: %+v %+v %v", got, page, err)
	}
	svc.mu.Lock()
	reason := inferenceSchedule(svc.states[sess.Info().ID], true, sess.Observation(), time.Now())
	svc.mu.Unlock()
	if reason != "urgent" {
		t.Fatal("repainted question delayed by generation fallback", reason)
	}
}

func TestServiceRejectsInvalidAttentionBeforeSavingValidWork(t *testing.T) {
	svc, sess, _, _ := reviewFixture(t, classifierFunc(func(_ context.Context, _ Profile, _ observation.Snapshot) (Decision, error) {
		return Decision{State: "waiting", Kind: "question", Evidence: "Invented", Question: "Continue?", InputMode: "text", WorkSummary: "Supported work", WorkEvidence: "Continue?"}, nil
	}))
	ctx := context.Background()
	svc.tick(ctx)
	svc.jobs.Wait()
	work, err := store.WorkForSession(ctx, svc.DB, sess.Info().ID)
	if err != nil || work != nil {
		t.Fatal("invalid attention persisted work", work, err)
	}
	page, err := svc.Inbox.List(ctx, nil, false, 0)
	if err != nil || len(page.Items) != 0 {
		t.Fatal("invalid attention published", page, err)
	}
}

func TestTypingCancelsImmediatelyAndDebouncesReplacement(t *testing.T) {
	var calls atomic.Int32
	started := make(chan struct{})
	svc, sess, p, _ := generationFixture(t, classifierFunc(func(ctx context.Context, _ Profile, s observation.Snapshot) (Decision, error) {
		if calls.Add(1) == 1 {
			close(started)
			<-ctx.Done()
			return Decision{}, ctx.Err()
		}
		return Decision{State: "working", Kind: "none", Evidence: "Testing", InputMode: "none"}, nil
	}))
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	svc.tick(ctx)
	svc.mu.Lock()
	svc.states[sess.Info().ID].observationStarted = time.Now().Add(-generationFallback)
	svc.mu.Unlock()
	svc.tick(ctx)
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("initial inference not started")
	}
	if err := sess.WriteInput([]byte("typing")); err != nil {
		t.Fatal(err)
	}
	svc.tick(ctx)
	svc.jobs.Wait()
	// The old result must be canceled immediately; recent input must not start
	// another call, even when generation stopped or the old screen remains.
	svc.tick(ctx)
	svc.jobs.Wait()
	if calls.Load() != 1 || svc.Metrics.Snapshot()[p.ID].ObsoleteResults != 1 || svc.Metrics.Snapshot()[p.ID].TypingDeferrals != 1 {
		t.Fatal("typing caused replacement churn", calls.Load(), svc.Metrics.Snapshot())
	}
	svc.mu.Lock()
	w := svc.states[sess.Info().ID]
	reason := w.cancelReason
	w.lastInputAt = time.Now().Add(-typingDebounce)
	svc.mu.Unlock()
	if reason != "input" {
		t.Fatal("cancellation not attributed to input", reason)
	}
	svc.tick(ctx)
	svc.jobs.Wait()
	if calls.Load() != 2 || svc.Metrics.Snapshot()[p.ID].InputSettledCalls != 1 {
		t.Fatal("replacement missing after quiet input", calls.Load(), svc.Metrics.Snapshot())
	}
}

func TestTypingHidesPreviousQuestionImmediately(t *testing.T) {
	svc, sess, _, _ := reviewFixture(t, fixtureClassifier{})
	ctx := context.Background()
	svc.tick(ctx)
	svc.jobs.Wait()
	page, err := svc.Inbox.List(ctx, nil, false, 0)
	if err != nil || len(page.Items) != 1 {
		t.Fatal("initial question missing", page, err)
	}
	if err = sess.WriteInput([]byte("t")); err != nil {
		t.Fatal(err)
	}
	svc.tick(ctx)
	svc.jobs.Wait()
	page, err = svc.Inbox.List(ctx, nil, false, 0)
	if err != nil || len(page.Items) != 0 {
		t.Fatal("obsolete question remained visible during debounce", page, err)
	}
}

func TestOrdinaryLogsDoNotCancelOrRestartGenerationInference(t *testing.T) {
	var calls atomic.Int32
	started, release := make(chan struct{}), make(chan struct{})
	svc, sess, p, dir := generationFixture(t, classifierFunc(func(ctx context.Context, _ Profile, s observation.Snapshot) (Decision, error) {
		calls.Add(1)
		close(started)
		select {
		case <-release:
		case <-ctx.Done():
			return Decision{}, ctx.Err()
		}
		return Decision{State: "working", Kind: "none", Evidence: "Testing", InputMode: "none"}, nil
	}))
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	svc.tick(ctx)
	svc.mu.Lock()
	svc.states[sess.Info().ID].observationStarted = time.Now().Add(-generationFallback)
	svc.mu.Unlock()
	svc.tick(ctx)
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("fallback not started")
	}
	advanceGeneration(t, sess, dir, "noisy", "value?.name ?? fallback")
	svc.tick(ctx)
	svc.mu.Lock()
	reason := svc.states[sess.Info().ID].cancelReason
	svc.mu.Unlock()
	if reason != "" {
		t.Fatal("ordinary logs preempted inference", reason)
	}
	close(release)
	svc.jobs.Wait()
	svc.tick(ctx)
	svc.jobs.Wait()
	got := svc.Metrics.Snapshot()[p.ID]
	if calls.Load() != 1 || got.ObsoleteResults != 0 || got.StaleResults != 1 {
		t.Fatal("log keywords caused urgent replacement calls", calls.Load(), got)
	}
}
