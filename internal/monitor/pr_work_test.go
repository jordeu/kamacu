package monitor

import (
	"context"
	"fmt"
	"kamacu/internal/observation"
	"kamacu/internal/session"
	"kamacu/internal/store"
	"strings"
	"testing"
	"time"
)

func TestPRWorkRequiresRoleAndExactTerminalEvidence(t *testing.T) {
	screen := "Created https://github.com/acme/repo/pull/12 for the implementation"
	snap := observation.Snapshot{Screen: screen}
	link := WorkLink{Repository: "acme/repo", Number: 12, Role: "implementation", Evidence: screen}
	if !validWorkLink(link, snap) {
		t.Fatal("valid PR link rejected")
	}
	wrapped := "Created https://github.com/acme/\nrepo/pull/12 for the implementation"
	wrappedLink := link
	wrappedLink.Evidence = wrapped
	if !validWorkLink(wrappedLink, observation.Snapshot{Screen: wrapped}) {
		t.Fatal("wrapped PR URL rejected")
	}
	for _, mutate := range []func(*WorkLink){
		func(l *WorkLink) { l.Role = "incidental" },
		func(l *WorkLink) { l.Number = 1 },
		func(l *WorkLink) { l.Evidence = "Invented evidence" },
	} {
		invalid := link
		mutate(&invalid)
		if validWorkLink(invalid, snap) {
			t.Fatalf("invalid association accepted: %+v", invalid)
		}
	}
	d := Decision{State: "waiting", Kind: "question", InputMode: "text", Evidence: screen, Question: "Review?", WorkSummary: "Ready", WorkEvidence: "not on screen", PRLinks: []WorkLink{link, {Repository: "acme/repo", Number: 1, Role: "implementation", Evidence: screen}}}
	cleaned, work, links := cleanWorkContext(d, snap)
	if work != 1 || links != 1 || cleaned.WorkSummary != "" || cleaned.WorkEvidence != "" || cleaned.WorkUncertainty != "" || len(cleaned.PRLinks) != 1 {
		t.Fatalf("invalid work not independently removed: %+v, %d %d", cleaned, work, links)
	}
	if err := validateDecision(cleaned, snap); err != nil {
		t.Fatal("valid question lost", err)
	}
}

func TestMonitorPersistsTaskAndScratchpadPRContext(t *testing.T) {
	for _, taskID := range []int64{0, 1} {
		t.Run(fmt.Sprint(taskID), func(t *testing.T) {
			svc, _, p, _ := reviewFixture(t, classifierFunc(func(_ context.Context, _ Profile, s observation.Snapshot) (Decision, error) {
				evidence := "Reviewing https://github.com/acme/repo/pull/7"
				return Decision{State: "working", Kind: "none", InputMode: "none", Evidence: evidence, WorkSummary: "Review underway; checking tests next", WorkEvidence: evidence, PRLinks: []WorkLink{{Repository: "acme/repo", Number: 7, Role: "review", Evidence: evidence}}}, nil
			}))
			ctx := context.Background()
			if taskID > 0 {
				for _, q := range []string{`INSERT INTO projects(id,name,repo_path) VALUES(1,'repo','/repo')`, `INSERT INTO tasks(id,project_id,title,position) VALUES(1,1,'Review',0)`} {
					if _, err := svc.DB.Exec(q); err != nil {
						t.Fatal(err)
					}
				}
			}
			sess, err := svc.Sessions.Spawn(session.SpawnOpts{Kind: session.KindAgent, AgentEngine: "custom", AgentArgs: []string{"sh", "-c", "printf 'Reviewing https://github.com/acme/repo/pull/7\\n'; read answer"}, Cwd: t.TempDir(), TaskID: taskID, Global: taskID == 0})
			if err != nil {
				t.Fatal(err)
			}
			defer sess.Stop()
			eventually(t, func() bool { return strings.Contains(sess.Observation().Screen, "Reviewing") })
			svc.states[sess.Info().ID] = &watch{}
			task, err := svc.taskContext(ctx, sess.Info())
			if err != nil {
				t.Fatal(err)
			}
			svc.workers <- struct{}{}
			svc.classify(ctx, sess, p, sess.Observation(), task)
			sessions, err := store.RelatedPRSessions(ctx, svc.DB, "acme/repo", 7)
			if err != nil || len(sessions) != 1 || sessions[0].Summary != "Review underway; checking tests next" {
				t.Fatalf("session context: %+v %v", sessions, err)
			}
			if taskID > 0 {
				links, err := store.PRLinks(ctx, svc.DB, taskID, "", 0)
				if err != nil || len(links) != 1 || links[0].Role != "review" {
					t.Fatalf("task association: %+v %v", links, err)
				}
			}
		})
	}
}

func TestMonitorLinksScrolledPRWithoutSummaryAndTracksIdleExit(t *testing.T) {
	state := "working"
	evidence := "Created https://github.com/acme/repo/pull/7 for implementation"
	svc, fixture, p, _ := reviewFixture(t, classifierFunc(func(_ context.Context, _ Profile, _ observation.Snapshot) (Decision, error) {
		current := "Running tests"
		if state == "idle" {
			current = "Ready>"
		}
		d := Decision{State: state, Kind: "none", InputMode: "none", Evidence: current}
		if state == "working" {
			d.PRLinks = []WorkLink{{Repository: "acme/repo", Number: 7, Role: "implementation", Evidence: evidence}}
		}
		return d, nil
	}))
	// Stop the fixture's unused process so scheduler reconciliation needs no other
	// asynchronous classification during the exit check.
	fixture.Stop()
	sess, err := svc.Sessions.Spawn(session.SpawnOpts{Kind: session.KindAgent, AgentEngine: "custom", AgentArgs: []string{"sh", "-c", "printf 'Created https://github.com/acme/repo/pull/7 for implementation\\r\\n'; printf '\\033[2J\\033[HRunning tests'; read answer; printf '\\033[2J\\033[HReady>'; read next"}, Cwd: t.TempDir(), Global: true})
	if err != nil {
		t.Fatal(err)
	}
	defer sess.Stop()
	ctx := context.Background()
	eventually(t, func() bool { return strings.Contains(sess.Observation().Screen, "Running tests") })
	snap := sess.Observation()
	if strings.Contains(snap.Screen, evidence) || !snap.ContainsEvidence(evidence) {
		t.Fatal("expected historical PR creation evidence")
	}
	id := sess.Info().ID
	svc.states[id] = &watch{}
	task, err := svc.taskContext(ctx, sess.Info())
	if err != nil {
		t.Fatal(err)
	}
	svc.workers <- struct{}{}
	svc.classify(ctx, sess, p, snap, task)
	sessions, err := store.RelatedPRSessions(ctx, svc.DB, "acme/repo", 7)
	if err != nil || len(sessions) != 1 || sessions[0].State != "working" || sessions[0].Summary != "" {
		t.Fatalf("missing summaryless session: %+v %v", sessions, err)
	}
	links, _ := store.SessionPRLinks(ctx, svc.DB, id)
	if len(links) != 1 || !links[0].ObservedAt.Equal(snap.EvidenceAt(evidence)) {
		t.Fatal("historical link acquired a fresh timestamp", links)
	}
	// A newer association cannot be replaced by rediscovered historical output.
	if err = store.PutSessionPRLink(ctx, svc.DB, id, store.PRLink{Repository: "acme/repo", Number: 7, Role: "review", Source: "terminal", Evidence: "Newer evidence", ObservedAt: time.Now()}); err != nil {
		t.Fatal(err)
	}
	svc.workers <- struct{}{}
	svc.classify(ctx, sess, p, snap, task)
	links, _ = store.SessionPRLinks(ctx, svc.DB, id)
	if links[0].Role != "review" {
		t.Fatal("history replaced newer association", links)
	}
	state = "idle"
	if err = sess.WriteInput([]byte("continue\n")); err != nil {
		t.Fatal(err)
	}
	eventually(t, func() bool { return strings.Contains(sess.Observation().Screen, "Ready>") })
	svc.workers <- struct{}{}
	svc.classify(ctx, sess, p, sess.Observation(), task)
	work, _ := store.WorkForSession(ctx, svc.DB, id)
	if work.State != "idle" {
		t.Fatal("idle state not persisted without summary", work)
	}
	sess.Stop()
	svc.tick(ctx)
	work, _ = store.WorkForSession(ctx, svc.DB, id)
	if work.State != "exited" {
		t.Fatal("exit not persisted", work)
	}
}
