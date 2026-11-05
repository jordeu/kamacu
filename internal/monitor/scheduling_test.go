package monitor

import (
	"strings"
	"testing"
	"time"

	"kamacu/internal/observation"
)

func TestGenerationRecognitionAndNormalization(t *testing.T) {
	for _, tc := range []struct{ engine, before, after string }{
		{"codex", "• Working (4s • esc to interrupt)", "• Working (1m 4s • esc to interrupt)"},
		{"claude", "✢ Scampering… (8s · ↓ 109 tokens)", "✻ Scampering… (3m 11s · ↓ 2.9k tokens)"},
		{"claude", "✢ Hatching… (5s · thinking with medium effort)", "✶ Hatching… (27s · ↓ 2.5k tokens · thinking with medium effort)"},
	} {
		if !activeGeneration(tc.before, tc.engine) || !activeGeneration(tc.after, tc.engine) {
			t.Fatalf("unrecognized: %s", tc.before)
		}
		if normalizedScreen(tc.before, tc.engine) != normalizedScreen(tc.after, tc.engine) {
			t.Fatalf("cosmetic progress changed content: %s", tc.before)
		}
	}
	for _, screen := range []string{"✻ Worked for 12s · done 2:24 PM", "• Working (4s • esc to interrupt)\n" + strings.Repeat("new output\n", 9), "Quoted: ✢ Scampering… (8s · ↓ 109 tokens)", "Tests passed: 4s", "✢ Unknown layout (8s)"} {
		if activeGeneration(screen, "claude") || activeGeneration(screen, "codex") {
			t.Fatalf("false generation: %q", screen)
		}
	}
	if activeGeneration("• Working (4s • esc to interrupt)", "custom") {
		t.Fatal("custom engine incorrectly deferred")
	}
	if normalizedScreen("Total 109 tokens", "codex") == normalizedScreen("Total 110 tokens", "codex") {
		t.Fatal("arbitrary counters normalized")
	}
}

func TestGenerationSchedulingSequence(t *testing.T) {
	now := time.Now()
	task := TaskContext{Engine: "codex"}
	s := observation.Snapshot{Revision: 1, Screen: "Testing\n• Working (4s • esc to interrupt)"}
	w := &watch{}
	observeScheduling(w, s, "p", task, now)
	if got := inferenceSchedule(w, true, s, now); got != "generation" {
		t.Fatal(got)
	}
	initialChange := w.lastChange
	s.Revision++
	s.Screen = strings.Replace(s.Screen, "4s", "14s", 1)
	observeScheduling(w, s, "p", task, now.Add(10*time.Second))
	if w.lastChange != initialChange {
		t.Fatal("cosmetic frame reset quiet clock")
	}
	if got := inferenceSchedule(w, true, s, now.Add(29*time.Second)); got != "generation" {
		t.Fatal(got)
	}
	if got := inferenceSchedule(w, true, s, now.Add(30*time.Second)); got != "fallback" {
		t.Fatal(got)
	}
	w.lastAttempt = now.Add(30 * time.Second)
	if got := inferenceSchedule(w, true, s, now.Add(31*time.Second)); got != "generation" {
		t.Fatal(got)
	}
	s.Revision++
	s.Screen = "All done\n> "
	observeScheduling(w, s, "p", task, now.Add(32*time.Second))
	if got := inferenceSchedule(w, true, s, now.Add(32*time.Second)); got != "urgent" {
		t.Fatal("completion delayed", got)
	}
}

func TestGenerationUrgentInvalidations(t *testing.T) {
	now := time.Now()
	task := TaskContext{Engine: "claude"}
	base := observation.Snapshot{Revision: 1, Screen: "Testing\n✢ Hatching… (5s · thinking with medium effort)"}
	for _, tc := range []struct {
		name   string
		change func(*observation.Snapshot, *string, *TaskContext)
	}{
		{"question", func(s *observation.Snapshot, _ *string, _ *TaskContext) { s.Screen = "Continue?\n" + s.Screen }},
		{"permission", func(s *observation.Snapshot, _ *string, _ *TaskContext) {
			s.Screen = "Allow access\n1. Yes\n2. No\n" + s.Screen
		}},
		{"native menu", func(s *observation.Snapshot, _ *string, _ *TaskContext) {
			s.Screen = "Enter to select · ↑/↓ to navigate · Esc to cancel\n" + s.Screen
		}},
		{"error", func(s *observation.Snapshot, _ *string, _ *TaskContext) {
			s.Screen = "Error: authentication failed\n" + s.Screen
		}},
		{"scrolled question", func(s *observation.Snapshot, _ *string, _ *TaskContext) { s.History = []string{"Continue?"} }},
		{"input", func(s *observation.Snapshot, _ *string, _ *TaskContext) { s.Input++ }},
		{"profile", func(_ *observation.Snapshot, p *string, _ *TaskContext) { *p = "changed" }},
		{"task", func(_ *observation.Snapshot, _ *string, t *TaskContext) { t.Description = "changed" }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			w := &watch{}
			s := base
			p := "p"
			task := task
			observeScheduling(w, s, p, task, now)
			w.busy = true
			w.lastAttempt = now
			tc.change(&s, &p, &task)
			s.Revision++
			observeScheduling(w, s, p, task, now.Add(time.Second))
			if got := inferenceSchedule(w, true, s, now.Add(time.Second)); got != "" {
				t.Fatal("scheduled while busy", got)
			}
			w.busy = false
			if got := inferenceSchedule(w, true, s, now.Add(2*time.Second)); got != "urgent" {
				t.Fatal("lost urgent event", got)
			}
			w.retryAfter = now.Add(20 * time.Second)
			if got := inferenceSchedule(w, true, s, now.Add(3*time.Second)); got != "" {
				t.Fatal("ignored failure backoff", got)
			}
		})
	}
}

func TestUnrecognizedGenerationRetainsScheduler(t *testing.T) {
	now := time.Now()
	s := observation.Snapshot{Revision: 2, Screen: "arbitrary streaming output"}
	w := &watch{lastAttempt: now.Add(-6 * time.Second), lastChange: now.Add(-time.Second)}
	if got := inferenceSchedule(w, true, s, now); got != "" {
		t.Fatal(got)
	}
	w.lastChange = now.Add(-3 * time.Second)
	if got := inferenceSchedule(w, true, s, now); got != "settled" {
		t.Fatal(got)
	}
	w.lastAttempt = now.Add(-11 * time.Second)
	w.lastChange = now
	if got := inferenceSchedule(w, true, s, now); got != "settled" {
		t.Fatal(got)
	}
}

func TestCosmeticResultsNeverRebaseAttention(t *testing.T) {
	old := observation.Snapshot{Screen: "Continue?\n• Working (4s • esc to interrupt)"}
	current := old
	current.Screen = strings.Replace(old.Screen, "4s", "14s", 1)
	question := Decision{State: "waiting", Kind: "question", Evidence: "Continue?", Question: "Continue?", InputMode: "text"}
	if _, ok := rebaseRoutineDecision(question, old, current, "codex"); ok {
		t.Fatal("attention reused against new revision")
	}
	routine := Decision{State: "working", Kind: "none", Evidence: "• Working (4s • esc to interrupt)", InputMode: "none"}
	got, ok := rebaseRoutineDecision(routine, old, current, "codex")
	if !ok || !strings.Contains(current.Screen, got.Evidence) {
		t.Fatal("footer evidence not rebased", got, ok)
	}
	current.History = []string{"Permission required"}
	if _, ok := rebaseRoutineDecision(routine, old, current, "codex"); ok {
		t.Fatal("new history accepted")
	}
}

func TestAttentionSignalsIgnoreRepaintHistory(t *testing.T) {
	old := observation.Snapshot{Screen: "Continue?\n• Working (4s • esc to interrupt)"}
	current := observation.Snapshot{Screen: "Continue?\n• Working (14s • esc to interrupt)", History: []string{old.Screen, old.Screen}}
	if attentionFingerprint(old) != attentionFingerprint(current) {
		t.Fatal("repaint history created new attention")
	}
	current.History = append(current.History, "Permission required")
	if attentionFingerprint(old) == attentionFingerprint(current) {
		t.Fatal("new historical attention missed")
	}
}

func TestUncertainAndExistingAttentionFallbacks(t *testing.T) {
	now := time.Now()
	s := observation.Snapshot{Screen: "Continue?\n• Working (4s • esc to interrupt)"}
	w := &watch{generating: true, lastAttempt: now.Add(-6 * time.Second), lastChange: now.Add(-3 * time.Second), decision: Decision{State: "waiting", Kind: "question", Evidence: "Continue?"}}
	if got := inferenceSchedule(w, true, s, now); got != "settled" {
		t.Fatal("existing dialog delayed by footer", got)
	}
	w.generating = false
	w.decision = Decision{State: "unknown", Kind: "none"}
	w.lastAttempt = now.Add(-generationFallback)
	if got := inferenceSchedule(w, false, s, now); got != "fallback" {
		t.Fatal("uncertain stable screen never retried", got)
	}
}

func TestProfileChangeClearsProviderBackoff(t *testing.T) {
	now := time.Now()
	s := observation.Snapshot{Revision: 1, Screen: "Testing\n• Working (4s • esc to interrupt)"}
	task := TaskContext{Engine: "codex"}
	w := &watch{}
	observeScheduling(w, s, "old", task, now)
	w.retryAfter = now.Add(20 * time.Second)
	observeScheduling(w, s, "replacement", task, now.Add(time.Second))
	if got := inferenceSchedule(w, true, s, now.Add(time.Second)); got != "urgent" {
		t.Fatal("new profile inherited old provider backoff", got)
	}
}

func TestOldAttentionDoesNotDisableNewGenerationDeferral(t *testing.T) {
	now := time.Now()
	task := TaskContext{ID: 1, Engine: "codex"}
	screen := observation.Snapshot{Input: 1, Screen: "Continue?\n• Working (4s • esc to interrupt)"}
	for _, tc := range []struct {
		name   string
		change func(*watch, *observation.Snapshot)
	}{
		{"new input", func(w *watch, s *observation.Snapshot) { s.Input++ }},
		{"new profile", func(w *watch, _ *observation.Snapshot) { w.observedProfile = "replacement" }},
		{"new task context", func(w *watch, _ *observation.Snapshot) { w.observedTask.Title = "new objective" }},
		{"old question scrolled away", func(_ *watch, s *observation.Snapshot) { s.Screen = "Testing\n• Working (4s • esc to interrupt)" }},
		{"previous completion", func(w *watch, _ *observation.Snapshot) { w.decision.Kind = "completion"; w.decision.State = "idle" }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := screen
			w := &watch{generating: true, input: 1, profile: "p", observedProfile: "p", taskContext: task, observedTask: task, lastAttempt: now.Add(-6 * time.Second), lastChange: now.Add(-3 * time.Second), decision: Decision{State: "waiting", Kind: "question", Evidence: "Continue?"}}
			tc.change(w, &s)
			if got := inferenceSchedule(w, true, s, now); got != "generation" {
				t.Fatal("obsolete attention bypassed generation deferral", got)
			}
		})
	}
}

func TestHistoryExpiryDoesNotCreateUrgencyOrPreemptInference(t *testing.T) {
	now := time.Now()
	task := TaskContext{Engine: "codex"}
	old := observation.Snapshot{Revision: 1, Screen: "Testing\n• Working (4s • esc to interrupt)", History: []string{"Error: old attempt", "Permission from earlier"}}
	w := &watch{}
	observeScheduling(w, old, "p", task, now)
	w.urgent = false
	w.inflightSignals = w.observedSignals
	current := old
	current.Revision++
	current.Screen = strings.Replace(old.Screen, "4s", "14s", 1)
	current.History = []string{"Permission from earlier"}
	observeScheduling(w, current, "p", task, now.Add(time.Second))
	if w.urgent || newAttention(w.inflightSignals, w.observedSignals) {
		t.Fatal("history expiry preempted inference")
	}
	current.Revision++
	current.History = append(current.History, "New permission required")
	observeScheduling(w, current, "p", task, now.Add(2*time.Second))
	if !w.urgent || !newAttention(w.inflightSignals, w.observedSignals) {
		t.Fatal("new historical attention was missed")
	}
	if !newAttention(attentionEvidence{history: "Continue?"}, attentionEvidence{current: "Continue?"}) {
		t.Fatal("new visible question was missed")
	}
}

func TestNormalizationPreservesStatusExamples(t *testing.T) {
	d := Decision{State: "working", Kind: "none", Evidence: "Testing", InputMode: "none"}
	for _, tc := range []struct{ name, engine, before, after string }{
		{"other engine", "codex", "✢ Hatching… (5s · thinking with medium effort)\nTesting\n• Working (4s • esc to interrupt)", "✢ Hatching… (15s · thinking with medium effort)\nTesting\n• Working (14s • esc to interrupt)"},
		{"earlier status example", "codex", "• Working (4s • esc to interrupt)\nTesting\n• Working (14s • esc to interrupt)", "• Working (8s • esc to interrupt)\nTesting\n• Working (24s • esc to interrupt)"},
		{"fenced example", "codex", "Testing\n```text\n• Working (4s • esc to interrupt)\n```", "Testing\n```text\n• Working (14s • esc to interrupt)\n```"},
		{"unknown engine", "custom", "Testing\n• Working (4s • esc to interrupt)", "Testing\n• Working (14s • esc to interrupt)"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			old, current := observation.Snapshot{Screen: tc.before}, observation.Snapshot{Screen: tc.after}
			if meaningfulFingerprint(old, tc.engine) == meaningfulFingerprint(current, tc.engine) {
				t.Fatal("substantive status example was normalized")
			}
			if _, ok := rebaseRoutineDecision(d, old, current, tc.engine); ok {
				t.Fatal("substantive change reused a routine decision")
			}
		})
	}
	if activeGeneration("```text\n• Working (4s • esc to interrupt)\n```", "codex") {
		t.Fatal("fenced example treated as generation")
	}
}

func TestRebasedEvidenceUsesActualFooter(t *testing.T) {
	old := observation.Snapshot{Screen: "• Working (8s • esc to interrupt)\nTesting\n• Working (4s • esc to interrupt)"}
	current := old
	current.Screen = strings.Replace(old.Screen, "Working (4s", "Working (14s", 1)
	d := Decision{State: "working", Kind: "none", Evidence: "• Working (4s • esc to interrupt)", InputMode: "none"}
	got, ok := rebaseRoutineDecision(d, old, current, "codex")
	if !ok || got.Evidence != "• Working (14s • esc to interrupt)" {
		t.Fatal("quoted progress became detection evidence", got, ok)
	}
}

func TestCompletionRepaintKeepsOrdinaryRetryCadence(t *testing.T) {
	now := time.Now()
	task := TaskContext{Engine: "codex"}
	old := observation.Snapshot{Screen: "All done\n• Working (4s • esc to interrupt)"}
	current := old
	current.Screen = strings.Replace(old.Screen, "4s", "14s", 1)
	w := &watch{generating: true, taskContext: task, observedTask: task, evidence: old, lastAttempt: now.Add(-6 * time.Second), lastChange: now.Add(-3 * time.Second), decision: Decision{State: "idle", Kind: "completion", Evidence: "All done"}}
	if got := inferenceSchedule(w, true, current, now); got != "settled" {
		t.Fatal("unread completion delayed by cosmetic repaint", got)
	}
	current.Screen = "All done\nStarting next step\n• Working (14s • esc to interrupt)"
	if got := inferenceSchedule(w, true, current, now); got != "generation" {
		t.Fatal("old completion disabled deferral of new work", got)
	}
}
