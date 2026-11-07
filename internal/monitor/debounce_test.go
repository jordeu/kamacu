package monitor

import (
	"strings"
	"testing"
	"time"

	"kamacu/internal/observation"
)

const permissionDialog = "Allow access?\n1. Yes\n2. No\nPress enter to confirm or Esc to cancel"

func TestTypingBurstDefersReplacementUntilQuiet(t *testing.T) {
	now := time.Now()
	task := TaskContext{Engine: "codex"}
	s := observation.Snapshot{Revision: 1, Screen: "Testing\n• Working (4s • esc to interrupt)"}
	w := &watch{lastAttempt: now}
	observeScheduling(w, s, "p", task, now)
	for i := 1; i <= 30; i++ {
		at := now.Add(time.Duration(i) * 100 * time.Millisecond)
		s.Input++
		s.InputAt = at
		s.Revision++
		s.At = at
		observeScheduling(w, s, "p", task, at)
		if got := inferenceSchedule(w, true, s, at); got != "typing" {
			t.Fatal("inference restarted while typing", i, got)
		}
	}
	if got := inferenceSchedule(w, true, s, s.InputAt.Add(typingDebounce-time.Millisecond)); got != "typing" {
		t.Fatal("debounce ended early", got)
	}
	if got := inferenceSchedule(w, true, s, s.InputAt.Add(typingDebounce)); got != "input_settled" {
		t.Fatal("replacement was not scheduled after quiet input", got)
	}
}

func TestRecognizedDialogCanBypassTypingDebounce(t *testing.T) {
	now := time.Now()
	task := TaskContext{Engine: "codex"}
	s := observation.Snapshot{Revision: 1, Screen: "Testing\n• Working (4s • esc to interrupt)"}
	w := &watch{}
	observeScheduling(w, s, "p", task, now)
	s.Input = 1
	s.InputAt = now
	s.Revision++
	observeScheduling(w, s, "p", task, now)
	s.Screen = permissionDialog + "\n• Working (4s • esc to interrupt)"
	s.At = now.Add(100 * time.Millisecond)
	s.Revision++
	observeScheduling(w, s, "p", task, s.At)
	if got := inferenceSchedule(w, true, s, s.At); got != "urgent" {
		t.Fatal("fresh native dialog delayed by typing debounce", got)
	}
}

func TestOldDialogAndSelectionMovementDoNotBypassDebounce(t *testing.T) {
	now := time.Now()
	task := TaskContext{Engine: "codex"}
	s := observation.Snapshot{Revision: 1, At: now.Add(-time.Second), Screen: strings.Replace(permissionDialog, "1. Yes", "› 1. Yes", 1)}
	w := &watch{}
	observeScheduling(w, s, "p", task, now)
	s.Input = 1
	s.InputAt = now
	observeScheduling(w, s, "p", task, now)
	if got := inferenceSchedule(w, true, s, now); got != "typing" {
		t.Fatal("retained menu bypassed debounce", got)
	}
	s.Screen = strings.Replace(strings.Replace(s.Screen, "› 1. Yes", "1. Yes", 1), "2. No", "› 2. No", 1)
	s.At = now.Add(100 * time.Millisecond)
	s.Revision++
	observeScheduling(w, s, "p", task, s.At)
	if w.freshDialog || inferenceSchedule(w, true, s, s.At) != "typing" {
		t.Fatal("selection movement became a new dialog")
	}
}

func TestInputDebounceUsesTimestampOnStartup(t *testing.T) {
	now := time.Now()
	s := observation.Snapshot{Revision: 1, Input: 2, InputAt: now.Add(-900 * time.Millisecond), Screen: "Testing"}
	w := &watch{}
	observeScheduling(w, s, "p", TaskContext{}, now)
	if got := inferenceSchedule(w, true, s, now); got != "typing" {
		t.Fatal(got)
	}
	if got := inferenceSchedule(w, true, s, now.Add(100*time.Millisecond)); got != "input_settled" {
		t.Fatal("debounce restarted on observation instead of input", got)
	}
}

func TestOrdinaryOutputDoesNotCreateUrgency(t *testing.T) {
	now := time.Now()
	task := TaskContext{Engine: "codex"}
	base := observation.Snapshot{Revision: 1, Screen: "Testing\n• Working (4s • esc to interrupt)"}
	for _, text := range []string{
		"const fallback = value?.name ?? 'done';",
		"error: retrying transient request",
		"Tests completed: 120; failed: 0",
		"permission denied fixture passed",
		"Should I proceed?",
		"Enter to select · ↑/↓ to navigate · Esc to cancel",
		"```text\n" + permissionDialog + "\n```",
	} {
		t.Run(text, func(t *testing.T) {
			w := &watch{lastAttempt: now}
			observeScheduling(w, base, "p", task, now)
			old := w.observedSignals
			next := base
			next.Revision++
			next.Screen = text + "\n" + base.Screen
			observeScheduling(w, next, "p", task, now.Add(time.Second))
			if w.urgent || newAttention(old, w.observedSignals) {
				t.Fatal("ordinary output preempted inference")
			}
			if got := inferenceSchedule(w, true, next, now.Add(2*time.Second)); got != "generation" {
				t.Fatal("ordinary output bypassed generation deferral", got)
			}
		})
	}
	// A conversational question still reaches inference through the normal
	// quiet-period path when no recognized generation UI remains.
	s := observation.Snapshot{Screen: "Should I proceed?\n> "}
	w := &watch{lastAttempt: now.Add(-6 * time.Second), lastChange: now.Add(-3 * time.Second)}
	if got := inferenceSchedule(w, true, s, now); got != "settled" {
		t.Fatal("conversational question never analyzed", got)
	}
}

func TestDialogSchedulingHints(t *testing.T) {
	for _, screen := range []string{
		permissionDialog,
		"Would you like to run the following command?\n$ make test\n› 1. Yes, proceed (y)\n2. No (esc)\nPress enter to confirm or esc to cancel",
		"☐ Choice\nWhich route?\n❯ 1. Keep\n2. Replace\nEnter to select · ↑/↓ to navigate · Esc to cancel",
		"☐ Choice\nWhich route?\n❯ 1. Keep\n2. Replace\nEnter to select · Tab/Arrow keys to navigate · Esc to\n cancel",
	} {
		for _, engine := range []string{"codex", "claude"} {
			if dialogFingerprint(screen, engine) == "" {
				t.Fatalf("native dialog not recognized: %q", screen)
			}
		}
	}
	for _, screen := range []string{
		permissionDialog + "\nNew response",
		permissionDialog + "\n› ",
		"```text\n" + permissionDialog + "\n```",
		"Press enter to confirm or Esc to cancel",
		"Allow access?\n2. Yes\n3. No\nPress enter to confirm or Esc to cancel",
	} {
		if dialogFingerprint(screen, "codex") != "" {
			t.Fatalf("inactive or uncertain dialog recognized: %q", screen)
		}
	}
	if dialogFingerprint(permissionDialog, "custom") != "" {
		t.Fatal("unknown engine recognized as a native dialog")
	}
}

func TestDialogFingerprintIgnoresAdjacentGenerationClock(t *testing.T) {
	old := "• Working (4s • esc to interrupt)\n" + permissionDialog
	current := strings.Replace(old, "4s", "14s", 1)
	if dialogFingerprint(old, "codex") != dialogFingerprint(current, "codex") {
		t.Fatal("generation timer preempted dialog inference")
	}
}

func TestDialogFingerprintExcludesBackgroundOutput(t *testing.T) {
	for _, engine := range []string{"codex", "claude"} {
		old := "Permission from an unrelated earlier tool\nold background text\n" + permissionDialog
		current := "Permission from a different earlier tool\nnew background text\n" + permissionDialog
		if got := dialogFingerprint(old, engine); got == "" || got != dialogFingerprint(current, engine) {
			t.Fatal("background output created a new permission", engine)
		}
		question := "Which route?\n1. Keep\n2. Replace\nEnter to select · ↑/↓ to navigate · Esc to cancel"
		old = "Permission from an unrelated earlier tool\n" + question
		current = "Permission from a different earlier tool\n" + question
		if got := dialogFingerprint(old, engine); got == "" || got != dialogFingerprint(current, engine) {
			t.Fatal("old permission text overrode current question heading", engine)
		}
	}
}

func TestDialogFingerprintIncludesLongCommandsAndOptions(t *testing.T) {
	old := "Would you like to run the following command?\n$ important-operation\n" + strings.Repeat("argument\n", 12) + "1. Yes\n2. No\nPress enter to confirm or Esc to cancel"
	current := strings.Replace(old, "important-operation", "different-operation", 1)
	if a, b := dialogFingerprint(old, "codex"), dialogFingerprint(current, "codex"); a == "" || b == "" || a == b {
		t.Fatal("changed command omitted from dialog identity")
	}
	long := "☐ Choice\nWhich route?\n❯ 1. Keep\n" + strings.Repeat("  Description of the first option.\n", 35) + "2. Replace\nEnter to select · ↑/↓ to navigate · Esc to cancel"
	if dialogFingerprint(long, "claude") == "" {
		t.Fatal("long supported question lost its fast path")
	}
	longPermission := "Allow access?\n1. Yes\n" + strings.Repeat("  Long explanation.\n", 35) + "2. No\nPress enter to confirm or Esc to cancel"
	if dialogFingerprint(longPermission, "codex") == "" {
		t.Fatal("long permission lost its fast path")
	}
}

func TestDialogFingerprintDoesNotUseCodeExamplesAsChoices(t *testing.T) {
	for _, screen := range []string{
		"Allow access?\n```text\n1. Yes\n2. No\n```\nPress enter to confirm or Esc to cancel",
		"Choose a route\n~~~text\n1. Keep\n2. Replace\n~~~\nEnter to select · ↑/↓ to navigate · Esc to cancel",
		"☐ Choice\nWhich route?\n```text\n❯ 1. Keep\n2. Replace\n```\nEnter to select · ↑/↓ to navigate · Esc to cancel",
	} {
		for _, engine := range []string{"codex", "claude"} {
			if dialogFingerprint(screen, engine) != "" {
				t.Fatal("code examples became active options", engine)
			}
		}
	}
}

func TestFreshAcceptedDialogDoesNotCountAsTypingDeferral(t *testing.T) {
	now := time.Now()
	s := observation.Snapshot{Screen: permissionDialog}
	w := &watch{lastInputAt: now, lastAttempt: now, decision: Decision{State: "waiting", Kind: "permission"}}
	if got := inferenceSchedule(w, false, s, now.Add(100*time.Millisecond)); got != "" {
		t.Fatal("accepted unchanged dialog counted as deferred typing", got)
	}
}

func TestClaudeFreeTextSelectionKeepsDialogIdentity(t *testing.T) {
	screen := "☐ Choice\nWhich route?\n❯ 1. Keep\n2. Replace\n3. Type something.\nEnter to select · ↑/↓ to navigate · Esc to cancel"
	freeText := strings.Replace(strings.Replace(screen, "❯ 1.", "1.", 1), "3. Type something.", "❯ 3. Type something.", 1)
	if a, b := dialogFingerprint(screen, "claude"), dialogFingerprint(freeText, "claude"); a == "" || a != b {
		t.Fatal("free-text selection changed scheduling identity")
	}
	now := time.Now()
	task := TaskContext{Engine: "claude"}
	s := observation.Snapshot{Revision: 1, Screen: screen, At: now.Add(-time.Second)}
	w := &watch{}
	observeScheduling(w, s, "p", task, now)
	s.Input++
	s.InputAt = now
	s.Revision++
	s.Screen = freeText
	s.At = now.Add(100 * time.Millisecond)
	observeScheduling(w, s, "p", task, s.At)
	if got := inferenceSchedule(w, true, s, s.At); got != "typing" {
		t.Fatal("free-text selection bypassed typing debounce", got)
	}
}
