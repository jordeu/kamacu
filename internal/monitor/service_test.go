package monitor

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"kamacu/internal/inbox"
	"kamacu/internal/observation"
	"kamacu/internal/presence"
	"kamacu/internal/session"
	"kamacu/internal/store"
)

type classifierFunc func(context.Context, Profile, observation.Snapshot) (Decision, error)

func (f classifierFunc) Classify(ctx context.Context, p Profile, s observation.Snapshot) (Decision, error) {
	return f(ctx, p, s)
}

type fixtureClassifier struct{}

func (fixtureClassifier) Classify(_ context.Context, _ Profile, s observation.Snapshot) (Decision, error) {
	if strings.Contains(s.Screen, "All done") {
		return Decision{State: "idle", Kind: "completion", Evidence: "All done", Summary: "All done", InputMode: "none"}, nil
	}
	if strings.Contains(s.Screen, "Continue?") {
		d := Decision{State: "waiting", Kind: "question", Evidence: "Continue?", Question: "Continue?", Summary: "Continue?", InputMode: "text"}
		if strings.Contains(s.Screen, "1. Yes") {
			d.Options = []string{"1. Yes", "2. No"}
		}
		return d, nil
	}
	return Decision{State: "unknown", Kind: "none", InputMode: "none"}, nil
}
func TestTerminalToInboxWithoutHooks(t *testing.T) {
	db, e := store.Open(filepath.Join(t.TempDir(), "monitor.db"))
	if e != nil {
		t.Fatal(e)
	}
	defer db.Close()
	if e = store.Migrate(db); e != nil {
		t.Fatal(e)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	p, e := SaveProfile(ctx, db, Profile{Name: "fixture", Backend: "codex_cli"})
	if e != nil {
		t.Fatal(e)
	}
	_, _ = db.Exec(`UPDATE monitor_settings SET default_profile_id=?`, p.ID)
	dir := t.TempDir()
	bin := filepath.Join(dir, "agent")
	if e = os.WriteFile(bin, []byte("#!/bin/sh\nstty sane\nprintf 'Continue?\\r\\n1. Yes\\r\\n2. No\\r\\n> '\nread answer\nprintf '\\033[2J\\033[HAll done\\r\\n> '\nread next\n"), 0700); e != nil {
		t.Fatal(e)
	}
	mgr := session.NewManager()
	sess, e := mgr.Spawn(session.SpawnOpts{Kind: session.KindAgent, AgentEngine: "custom", AgentArgs: []string{bin}, Cwd: dir, Global: true})
	if e != nil {
		t.Fatal(e)
	}
	defer sess.Stop()
	id := sess.Info().ID
	in := inbox.New(db, func(sid string) inbox.Session { return inbox.Session{Global: true, Live: sid == id, Done: sess.Done()} })
	defer in.Close()
	pr := presence.NewTracker()
	svc := New(db, mgr, in, pr, fixtureClassifier{})
	pr.Heartbeat(presence.Beat{ClientID: "tab", SessionID: id, Visible: true})
	wait := func(label string, check func() bool) {
		t.Helper()
		deadline := time.Now().Add(5 * time.Second)
		for time.Now().Before(deadline) {
			svc.mu.Lock()
			if w := svc.states[id]; w != nil {
				w.lastAttempt = time.Time{}
				w.lastChange = time.Time{}
			}
			svc.mu.Unlock()
			svc.tick(ctx)
			if check() {
				return
			}
			time.Sleep(20 * time.Millisecond)
		}
		t.Fatalf("%s: info=%+v observation=%+v", label, sess.Info(), sess.Observation())
	}
	wait("question classification", func() bool { return sess.Info().AgentStatus == "waiting" })
	page, e := in.List(ctx, nil, false, 0)
	if e != nil || len(page.Items) != 0 {
		t.Fatal("notification while viewed", page, e)
	}
	pr.Heartbeat(presence.Beat{ClientID: "tab", Visible: false})
	wait("question after departure", func() bool { page, _ = in.List(ctx, nil, false, 0); return len(page.Items) == 1 })
	original := page.Items[0]
	pr.Heartbeat(presence.Beat{ClientID: "tab", SessionID: id, Visible: true})
	svc.tick(ctx)
	page, _ = in.List(ctx, nil, false, 0)
	if len(page.Items) != 0 {
		t.Fatal("existing card did not hide")
	}
	pr.Heartbeat(presence.Beat{ClientID: "tab", Visible: false})
	svc.tick(ctx)
	page, _ = in.List(ctx, nil, false, 0)
	if len(page.Items) != 1 || page.Items[0].ID != original.ID {
		t.Fatal("card identity changed", page)
	}
	if len(original.Options) != 2 || original.Options[0].Label != "1. Yes" {
		t.Fatalf("missing observed choices: %+v", original)
	}
	answer, e := in.Answer(ctx, nil, original.ID, original.Revision, "", original.Options[0].ID)
	if e != nil {
		t.Fatal(e)
	}
	// Delay completion inference until after departure. Presence must use when
	// the evidence appeared, never when the model finished.
	started, release := make(chan struct{}), make(chan struct{})
	svc.mu.Lock()
	svc.Classifier = classifierFunc(func(ctx context.Context, p Profile, o observation.Snapshot) (Decision, error) {
		if strings.Contains(o.Screen, "All done") {
			close(started)
			select {
			case <-release:
			case <-ctx.Done():
				return Decision{}, ctx.Err()
			}
		}
		return (fixtureClassifier{}).Classify(ctx, p, o)
	})
	svc.mu.Unlock()
	// Ordinary PTY input, no acknowledge_feedback or agent-side tool.
	pr.Heartbeat(presence.Beat{ClientID: "tab", SessionID: id, Visible: true})
	if e = sess.WriteObserved(answer.Question.EvidenceRevision, answer.Question.InputGeneration, []byte(answer.Question.Answer), []byte("\r")); e != nil {
		t.Fatal(e)
	}
	wait("completion inference started", func() bool {
		select {
		case <-started:
			return true
		default:
			return false
		}
	})
	pr.Heartbeat(presence.Beat{ClientID: "tab", Visible: false})
	close(release)
	wait("completion classification", func() bool { return sess.Info().AgentStatus == "idle" })
	svc.tick(ctx)
	page, _ = in.List(ctx, nil, false, 0)
	if len(page.Items) != 0 {
		t.Fatal("viewed completion notified later", page)
	}
}

func reviewFixture(t *testing.T, c Classifier) (*Service, *session.Session, Profile, string) {
	t.Helper()
	db, err := store.Open(filepath.Join(t.TempDir(), "monitor.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	if err = store.Migrate(db); err != nil {
		t.Fatal(err)
	}
	p, err := SaveProfile(context.Background(), db, Profile{Name: "Test", Backend: "codex_cli"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec(`UPDATE monitor_settings SET default_profile_id=?`, p.ID); err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	bin := filepath.Join(dir, "agent")
	if err = os.WriteFile(bin, []byte("#!/bin/sh\nprintf 'Continue?\\r\\n> '\nwhile [ ! -f advance ]; do sleep 0.01; done\nprintf 'new output\\r\\n'\nexec sleep 60\n"), 0700); err != nil {
		t.Fatal(err)
	}
	mgr := session.NewManager()
	sess, err := mgr.Spawn(session.SpawnOpts{Kind: session.KindAgent, AgentEngine: "custom", AgentArgs: []string{bin}, Cwd: dir, Global: true})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { sess.Stop() })
	in := inbox.New(db, func(id string) inbox.Session {
		return inbox.Session{Global: true, Live: id == sess.Info().ID && sess.Info().Status == session.StatusRunning, Done: sess.Done()}
	})
	t.Cleanup(in.Close)
	svc := New(db, mgr, in, presence.NewTracker(), c)
	eventually(t, func() bool { return strings.Contains(sess.Observation().Screen, "Continue?") })
	return svc, sess, p, dir
}
func eventually(t *testing.T, check func() bool) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if check() {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatal("condition did not become true")
}
func TestProviderFailureBacksOffDespiteNewOutput(t *testing.T) {
	started, release := make(chan struct{}), make(chan struct{})
	svc, sess, _, dir := reviewFixture(t, classifierFunc(func(ctx context.Context, _ Profile, _ observation.Snapshot) (Decision, error) {
		close(started)
		select {
		case <-release:
		case <-ctx.Done():
		}
		return Decision{}, fmt.Errorf("provider unavailable")
	}))
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	svc.tick(ctx)
	<-started
	revision := sess.Observation().Revision
	if err := os.WriteFile(filepath.Join(dir, "advance"), nil, 0600); err != nil {
		t.Fatal(err)
	}
	eventually(t, func() bool { return sess.Observation().Revision != revision })
	close(release)
	svc.jobs.Wait()
	if sess.Info().Monitor.Health != "unavailable" {
		t.Fatal("failure lost after repaint", sess.Info())
	}
	svc.mu.Lock()
	retry := svc.states[sess.Info().ID].lastAttempt
	svc.mu.Unlock()
	if !retry.After(time.Now()) {
		t.Fatal("provider failure did not back off")
	}
}
func TestRestorationCannotUndoNewViewingHeartbeat(t *testing.T) {
	svc, sess, _, _ := reviewFixture(t, fixtureClassifier{})
	ctx := context.Background()
	svc.tick(ctx)
	svc.jobs.Wait()
	page, err := svc.Inbox.List(ctx, nil, false, 0)
	if err != nil || len(page.Items) != 1 {
		t.Fatal(page, err)
	}
	id := sess.Info().ID
	svc.Presence.Heartbeat(presence.Beat{ClientID: "tab", SessionID: id, Visible: true})
	if err = svc.Inbox.HideObserved(ctx, id, true); err != nil {
		t.Fatal(err)
	}
	// Simulate a scheduler that captured "away" before this heartbeat.
	if err = svc.visibility(ctx, id, false); err != nil {
		t.Fatal(err)
	}
	page, err = svc.Inbox.List(ctx, nil, false, 0)
	if err != nil || len(page.Items) != 0 {
		t.Fatal("restoration raced viewing", page, err)
	}
}
func TestStopCancelsClassifier(t *testing.T) {
	started := make(chan struct{})
	canceled := make(chan struct{})
	svc, sess, _, dir := reviewFixture(t, classifierFunc(func(ctx context.Context, _ Profile, _ observation.Snapshot) (Decision, error) {
		close(started)
		<-ctx.Done()
		close(canceled)
		return Decision{}, ctx.Err()
	}))
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	svc.tick(ctx)
	<-started
	if err := os.WriteFile(filepath.Join(dir, "advance"), nil, 0600); err != nil {
		t.Fatal(err)
	}
	eventually(t, func() bool { return strings.Contains(sess.Observation().Screen, "new output") })
	sess.Stop()
	svc.tick(ctx)
	select {
	case <-canceled:
	case <-time.After(time.Second):
		t.Fatal("classification continued after session exit")
	}
	svc.jobs.Wait()
}

func TestProfileChangeCancelsOldClassification(t *testing.T) {
	started := make(chan struct{})
	svc, sess, p, _ := reviewFixture(t, classifierFunc(func(ctx context.Context, p Profile, obs observation.Snapshot) (Decision, error) {
		close(started)
		<-ctx.Done()
		return (fixtureClassifier{}).Classify(context.Background(), p, obs)
	}))
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	svc.tick(ctx)
	<-started
	p.Name = "Changed model profile"
	if _, err := SaveProfile(ctx, svc.DB, p); err != nil {
		t.Fatal(err)
	}
	svc.tick(ctx)
	svc.jobs.Wait()
	page, err := svc.Inbox.List(ctx, nil, false, 0)
	if err != nil || len(page.Items) != 0 || sess.Info().AgentStatus != "working" {
		t.Fatal("accepted obsolete configuration", page, sess.Info(), err)
	}
}
func TestSupersedeFailureKeepsCardHiddenAndRetries(t *testing.T) {
	svc, sess, _, dir := reviewFixture(t, fixtureClassifier{})
	ctx := context.Background()
	svc.tick(ctx)
	svc.jobs.Wait()
	page, _ := svc.Inbox.List(ctx, nil, false, 0)
	if len(page.Items) != 1 {
		t.Fatal(page)
	}
	if _, err := svc.DB.Exec(`CREATE TRIGGER fail_supersede BEFORE UPDATE OF status ON inbox_questions WHEN NEW.status='unanswered' BEGIN SELECT RAISE(ABORT,'test failure'); END`); err != nil {
		t.Fatal(err)
	}
	svc.Classifier = classifierFunc(func(_ context.Context, _ Profile, _ observation.Snapshot) (Decision, error) {
		return Decision{State: "working", Kind: "none", Evidence: "new output", InputMode: "none"}, nil
	})
	if err := os.WriteFile(filepath.Join(dir, "advance"), nil, 0600); err != nil {
		t.Fatal(err)
	}
	eventually(t, func() bool { return strings.Contains(sess.Observation().Screen, "new output") })
	svc.mu.Lock()
	svc.states[sess.Info().ID].lastAttempt = time.Time{}
	svc.mu.Unlock()
	svc.tick(ctx)
	svc.jobs.Wait()
	if sess.Info().Monitor.Health != "unavailable" {
		t.Fatal("persistence failure ignored")
	}
	page, _ = svc.Inbox.List(ctx, nil, false, 0)
	if len(page.Items) != 0 {
		t.Fatal("obsolete card revealed after persistence failure", page)
	}
	if _, err := svc.DB.Exec(`DROP TRIGGER fail_supersede`); err != nil {
		t.Fatal(err)
	}
	svc.mu.Lock()
	svc.states[sess.Info().ID].lastAttempt = time.Time{}
	svc.mu.Unlock()
	svc.tick(ctx)
	svc.jobs.Wait()
	if sess.Info().AgentStatus != "working" {
		t.Fatal("failed to recover")
	}
	archived, _ := svc.Inbox.List(ctx, nil, true, 0)
	if len(archived.Items) != 1 || archived.Items[0].Reason != "superseded" {
		t.Fatal(archived)
	}
}

func TestRewordingDoesNotReopenDismissedEpisode(t *testing.T) {
	svc, sess, _, dir := reviewFixture(t, fixtureClassifier{})
	ctx := context.Background()
	svc.tick(ctx)
	svc.jobs.Wait()
	page, _ := svc.Inbox.List(ctx, nil, false, 0)
	if len(page.Items) != 1 {
		t.Fatal(page)
	}
	original := page.Items[0]
	if _, err := svc.Inbox.Dismiss(ctx, original.ID, original.Revision); err != nil {
		t.Fatal(err)
	}
	svc.Classifier = classifierFunc(func(_ context.Context, _ Profile, _ observation.Snapshot) (Decision, error) {
		return Decision{State: "waiting", Kind: "question", Evidence: "Continue?", Question: "Should I continue?", InputMode: "text"}, nil
	})
	if err := os.WriteFile(filepath.Join(dir, "advance"), nil, 0600); err != nil {
		t.Fatal(err)
	}
	eventually(t, func() bool { return strings.Contains(sess.Observation().Screen, "new output") })
	svc.mu.Lock()
	svc.states[sess.Info().ID].lastAttempt = time.Time{}
	svc.mu.Unlock()
	svc.tick(ctx)
	svc.jobs.Wait()
	page, _ = svc.Inbox.List(ctx, nil, false, 0)
	if len(page.Items) != 0 {
		t.Fatal("model rewording reopened dismissed question", page)
	}
}
func TestFreshEvidenceAfterInputCreatesNewEpisodeAndSurvivesReload(t *testing.T) {
	svc, sess, _, dir := reviewFixture(t, fixtureClassifier{})
	ctx := context.Background()
	svc.tick(ctx)
	svc.jobs.Wait()
	page, _ := svc.Inbox.List(ctx, nil, false, 0)
	if len(page.Items) != 1 {
		t.Fatal(page)
	}
	original := page.Items[0]
	if _, err := svc.Inbox.Answer(ctx, nil, original.ID, original.Revision, "yes", ""); err != nil {
		t.Fatal(err)
	}
	if err := sess.WriteInput([]byte("yes\r")); err != nil {
		t.Fatal(err)
	}
	// An uncertain intermediate observation must not consume the new-turn
	// boundary before fresh question evidence arrives.
	svc.Classifier = classifierFunc(func(_ context.Context, _ Profile, _ observation.Snapshot) (Decision, error) {
		return Decision{State: "unknown", Kind: "none", InputMode: "none"}, nil
	})
	svc.mu.Lock()
	svc.states[sess.Info().ID].lastAttempt = time.Time{}
	svc.mu.Unlock()
	svc.tick(ctx)
	svc.jobs.Wait()
	svc.Classifier = classifierFunc(func(_ context.Context, _ Profile, _ observation.Snapshot) (Decision, error) {
		return Decision{State: "waiting", Kind: "question", Evidence: "new output", Question: "Continue?", InputMode: "text"}, nil
	})
	if err := os.WriteFile(filepath.Join(dir, "advance"), nil, 0600); err != nil {
		t.Fatal(err)
	}
	eventually(t, func() bool { return strings.Contains(sess.Observation().Screen, "new output") })
	svc.mu.Lock()
	svc.states[sess.Info().ID].lastAttempt = time.Time{}
	svc.mu.Unlock()
	svc.tick(ctx)
	svc.jobs.Wait()
	page, _ = svc.Inbox.List(ctx, nil, false, 0)
	if len(page.Items) != 1 || page.Items[0].ID == original.ID {
		t.Fatal("fresh question lost to old episode", page)
	}
	current := page.Items[0]
	if _, err := svc.Inbox.Dismiss(ctx, current.ID, current.Revision); err != nil {
		t.Fatal(err)
	}
	restored := New(svc.DB, svc.Sessions, svc.Inbox, svc.Presence, svc.Classifier)
	restored.tick(ctx)
	restored.jobs.Wait()
	page, _ = svc.Inbox.List(ctx, nil, false, 0)
	if len(page.Items) != 0 {
		t.Fatal("reloaded episode reopened dismissal", page)
	}
}

func TestPublishedMessagePreservesAuthoredContext(t *testing.T) {
	message := "I restricted image uploads to workspace members.\n\n## Changes\n- Require authentication.\n- Limit uploads to 8 MiB.\n\n## Verification\nAll 12 tests passed.\n\nhttps://github.com/example/demo/pull/42"
	svc, sess, _, _ := reviewFixture(t, classifierFunc(func(context.Context, Profile, observation.Snapshot) (Decision, error) {
		return Decision{State: "waiting", Kind: "question", Evidence: "Continue?", Question: "Continue?", Summary: "I restricted image uploads to workspace members. All 12 tests passed.", Context: message, InputMode: "text"}, nil
	}))
	svc.tick(context.Background())
	svc.jobs.Wait()
	page, err := svc.Inbox.List(context.Background(), nil, false, 0)
	if err != nil || len(page.Items) != 1 {
		t.Fatal(page, err)
	}
	q := page.Items[0]
	if q.Context != message || q.Abstract != q.Summary || q.Question != "Continue?" {
		t.Fatalf("authored message replaced: %+v", q)
	}
	var raw string
	if err := svc.DB.QueryRow(`SELECT decision FROM monitor_episodes WHERE session_id=?`, sess.Info().ID).Scan(&raw); err != nil {
		t.Fatal(err)
	}
	var saved Decision
	if err := json.Unmarshal([]byte(raw), &saved); err != nil || saved.Context != message {
		t.Fatal("context was not persisted", saved, err)
	}
}

func TestMessageFallsBackToOriginalEvidence(t *testing.T) {
	svc, _, _, _ := reviewFixture(t, fixtureClassifier{})
	svc.tick(context.Background())
	svc.jobs.Wait()
	page, err := svc.Inbox.List(context.Background(), nil, false, 0)
	if err != nil || len(page.Items) != 1 || page.Items[0].Context != "Continue?" {
		t.Fatal(page, err)
	}
}
