package session

import (
	"context"
	"errors"
	"os"
	"strings"
	"testing"
	"time"

	"kamacu/internal/observation"
)

func TestInboxReadinessWaitsForAnActualPrompt(t *testing.T) {
	m := NewManager()
	s, err := m.Spawn(SpawnOpts{Kind: KindAgent, TaskID: 1, Cwd: t.TempDir(), AgentEngine: "custom", AgentArgs: []string{"sh", "-c", "sleep 0.1; printf '❯\\n'; sleep 30"}})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Stop()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	obs, err := s.WaitInboxReady(ctx)
	if err != nil || obs.Screen == "" || obs.Input != 0 {
		t.Fatal(obs, err)
	}
	if err = s.WriteObserved(obs.Revision, obs.Input, []byte("message"), []byte("\r")); err != nil {
		t.Fatal(err)
	}
	if err = s.StopForInbox(ctx, obs.Input); !errors.Is(err, ErrObservedChanged) {
		t.Fatal("stale completion stopped a session with new input", err)
	}
	if s.Info().Status != StatusRunning {
		t.Fatal("stale stop exited the session")
	}
}
func TestInboxReadinessNeverTreatsEmptyStartupAsIdleInput(t *testing.T) {
	m := NewManager()
	s, err := m.Spawn(SpawnOpts{Kind: KindAgent, TaskID: 1, Cwd: t.TempDir(), AgentEngine: "custom", AgentArgs: []string{"sh", "-c", "sleep 30"}})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Stop()
	ctx, cancel := context.WithTimeout(context.Background(), 150*time.Millisecond)
	defer cancel()
	if _, err = s.WaitInboxReady(ctx); err == nil {
		t.Fatal("empty startup was considered ready")
	}
}

func TestInboxInputReadinessRequiresAnActualInputSurface(t *testing.T) {
	for _, tc := range []struct {
		engine, screen string
		want           bool
	}{
		{"claude", "Welcome to Claude\n❯ ", true},
		{"claude", "Welcome to Claude\n❯\u00a0", true},
		{"claude", "Welcome to Claude\n\u2003❯\u202f", true},
		{"claude", "❯\u00a0draft message", false},
		{"claude", "❯\u00a01. Yes, trust this folder\nEnter to select", false},
		{"claude", "❯\u00a0\nTrust this workspace?\nEnter to confirm", false},
		{"claude", "Welcome\n❯ Try \"fix a bug\"", true},
		{"claude", "❯ 1. Yes, trust this folder\n2. No\nEnter to select", false},
		{"claude", "❯ \nDo you trust this workspace?\nEnter to confirm", false},
		{"claude", "The terminal is quiet but not ready", false},
		{"claude", "❯\u00a0\nPrevious response\n❯\u00a0draft text", false},
		{"claude", "❯ old message\nResponse\n❯\u00a0", true},
		{"claude", "Enter to confirm\n❯\u00a0", false},
		{"claude", "Trust this workspace?\n❯\u00a0", false},
		{"claude", "Earlier docs mention trust this folder\nline 1\nline 2\nline 3\nline 4\n❯\u00a0", true},
		{"codex", "› Ask Codex to do anything", true},
		{"codex", "◦ Working (2s • esc to interrupt)\n› Ask Codex to do anything", false},
		{"custom", "Background task is idle", false},
	} {
		if got := inboxInputPrompt(tc.engine, tc.screen); got != tc.want {
			t.Fatalf("%s %q got %v want %v", tc.engine, tc.screen, got, tc.want)
		}
	}
}

func TestExplicitSendNowCanUseWorkingInputButNotAChoiceDialog(t *testing.T) {
	for _, tc := range []struct {
		screen string
		want   bool
	}{{"Working (esc to interrupt)\\n❯\\n", true}, {"❯ 1. Trust this folder\\n2. No\\nEnter to select", false}} {
		m := NewManager()
		s, err := m.Spawn(SpawnOpts{Kind: KindAgent, TaskID: 1, Cwd: t.TempDir(), AgentEngine: "custom", AgentArgs: []string{"sh", "-c", "printf '" + tc.screen + "'; sleep 30"}})
		if err != nil {
			t.Fatal(err)
		}
		ctx, cancel := context.WithTimeout(context.Background(), 700*time.Millisecond)
		_, err = s.WaitInboxReady(ctx, true)
		cancel()
		s.Stop()
		if (err == nil) != tc.want {
			t.Fatal(tc, err)
		}
	}
}

func TestInboxReadinessRecognizesClaudeNonBreakingSpacePrompt(t *testing.T) {
	m := NewManager()
	s, err := m.Spawn(SpawnOpts{Kind: KindAgent, TaskID: 1, Cwd: t.TempDir(), AgentEngine: "custom", AgentArgs: []string{"sh", "-c", "printf 'Welcome to Claude\\n❯\u00a0'; sleep 30"}})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Stop()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	obs, err := s.WaitInboxReady(ctx)
	if err != nil {
		t.Fatal("idle Unicode prompt missed", err)
	}
	if err = s.WriteObserved(obs.Revision, obs.Input, []byte("Review the PR"), []byte("\r")); err != nil {
		t.Fatal(err)
	}
	if s.Observation().Input != 1 {
		t.Fatal("prompt not submitted")
	}
}

func TestInboxReadinessDoesNotUseAnOldPromptOrDialogCursor(t *testing.T) {
	for _, screen := range []string{"❯\\nEarlier response\\n❯\u00a0draft", "Enter to confirm\\n❯\u00a0"} {
		t.Run(screen, func(t *testing.T) {
			m := NewManager()
			s, err := m.Spawn(SpawnOpts{Kind: KindAgent, TaskID: 1, Cwd: t.TempDir(), AgentEngine: "custom", AgentArgs: []string{"sh", "-c", "printf '" + screen + "'; sleep 30"}})
			if err != nil {
				t.Fatal(err)
			}
			defer s.Stop()
			ctx, cancel := context.WithTimeout(context.Background(), 600*time.Millisecond)
			defer cancel()
			if _, err = s.WaitInboxReady(ctx); err == nil {
				t.Fatal("unsafe input surface accepted", s.Observation().Screen)
			}
			if s.Observation().Input != 0 {
				t.Fatal("input written despite failed readiness")
			}
		})
	}
}

const piEditorRule = "────────────────────────────────────────────────────────────────────────────────"
const piDefaultFooter = "~/worktrees/project/pr-2108 (review)\n$0.000 (sub) 0.0%/500k (auto)                                    model • medium"
const piReadyScreen = "Pi v1.6.1\n[Context]\nAGENTS.md\n\n" + piEditorRule + "\n\n" + piEditorRule + "\n" + piDefaultFooter

func TestPiInboxInputRequiresAnEmptyEditor(t *testing.T) {
	for _, tc := range []struct {
		name, screen string
		want         bool
	}{
		{"startup from observed Pi layout", piReadyScreen, true},
		{"after a response and terminal padding", "Previous response\n" + piEditorRule + "\n     \n" + piEditorRule + "\n" + piDefaultFooter + "\n   \n", true},
		{"draft", piEditorRule + "\nDraft review text\n" + piEditorRule + "\n" + piDefaultFooter, false},
		{"multiline draft with a blank last line", piEditorRule + "\nDraft review text\n\n" + piEditorRule + "\n" + piDefaultFooter, false},
		{"working inside the top rule", "── ⠹ Working ───────────\n\n" + piEditorRule + "\n" + piDefaultFooter, false},
		{"completion selection above the footer", piEditorRule + "\n\n" + piEditorRule + "\n→ /model\n" + piDefaultFooter, false},
		{"project trust dialog", piEditorRule + "\nProject trust\n→ Trust this project\nEnter to save   Esc to cancel\n\n" + piEditorRule + "\n" + piDefaultFooter, false},
		{"partial startup without a footer", piEditorRule + "\n\n" + piEditorRule, false},
		{"unrelated footer", piEditorRule + "\n\n" + piEditorRule + "\n" + "~/tmp\nCommand finished", false},
		{"missing model", piEditorRule + "\n\n" + piEditorRule + "\n~/tmp\n0.0%/0 (auto)   no-model", false},
		{"old or generic prompt", piReadyScreen + "\nSelect a session\n>", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := inboxInputPrompt("pi", tc.screen); got != tc.want {
				t.Fatalf("Pi input ready = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestPiEmptyEditorDoesNotOverrideCurrentWorkingStatus(t *testing.T) {
	term := observation.New(80, 24)
	term.Feed([]byte(strings.ReplaceAll(piReadyScreen, "\n", "\r\n")))
	sess := &Session{kind: KindAgent, engine: "pi", status: StatusRunning, observer: term}
	sess.SetMonitorStatus(MonitorStatus{State: "working", ObservedAt: time.Now()})
	ctx, cancel := context.WithTimeout(context.Background(), 600*time.Millisecond)
	defer cancel()
	if _, err := sess.WaitInboxReady(ctx); err == nil {
		t.Fatal("Pi editor was treated as ready despite current working evidence")
	}
}

func TestPiInboxReadinessCanSubmitTheFirstPrompt(t *testing.T) {
	manager := NewManager()
	sess, err := manager.Spawn(SpawnOpts{
		Kind: KindAgent, TaskID: 1, Cwd: t.TempDir(), AgentEngine: "pi",
		AgentArgs: []string{"sh", "-c", `stty -echo
			printf '%s\n' "$1"
			IFS= read -r text
			printf 'accepted:%s\n' "$text"
			sleep 30`, "pi-test", piReadyScreen},
	})
	if err != nil {
		t.Fatal(err)
	}
	defer sess.Stop()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	obs, err := sess.WaitInboxReady(ctx)
	if err != nil || obs.Input != 0 {
		t.Fatalf("Pi first prompt not recognized: %v", err)
	}
	if err := sess.WriteObserved(obs.Revision, obs.Input, []byte("Review locally"), []byte("\r")); err != nil {
		t.Fatal(err)
	}
	eventually(t, time.Second, "review instruction received", func() bool {
		return strings.Contains(string(sess.Snapshot()), "accepted:Review locally")
	})
}

func TestPiInboxActionRechecksTheEditorAfterARepaint(t *testing.T) {
	banner := "Update Available\nNew version available. Run pi update\n\n"
	for _, tc := range []struct {
		name, screen                  string
		newInput, cancelled, wantSend bool
	}{
		{name: "empty editor after banner", screen: banner + piReadyScreen, wantSend: true},
		{name: "draft after repaint", screen: strings.Replace(piReadyScreen, piEditorRule+"\n\n"+piEditorRule, piEditorRule+"\nDraft\n"+piEditorRule, 1)},
		{name: "keyboard input after readiness", screen: banner + piReadyScreen, newInput: true},
		{name: "cancelled handoff", screen: banner + piReadyScreen, cancelled: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			output, err := os.CreateTemp(t.TempDir(), "terminal-input")
			if err != nil {
				t.Fatal(err)
			}
			defer output.Close()
			term := observation.New(80, 24)
			term.Feed([]byte(strings.ReplaceAll(piReadyScreen, "\n", "\r\n")))
			sess := &Session{kind: KindAgent, engine: "pi", status: StatusRunning, observer: term, ptmx: output}

			// An existing session has already received input, and the monitor
			// had classified it as idle before the new repaint.
			term.Input()
			sess.SetMonitorStatus(MonitorStatus{State: "idle", ObservedAt: time.Now()})
			ready := sess.Observation()
			term.Feed([]byte("\x1b[2J\x1b[H" + strings.ReplaceAll(tc.screen, "\n", "\r\n")))
			if tc.newInput {
				term.Input()
			}
			if sess.Observation().Revision == ready.Revision {
				t.Fatal("fixture must make the readiness revision stale")
			}

			ctx, cancel := context.WithTimeout(context.Background(), 700*time.Millisecond)
			defer cancel()
			if tc.cancelled {
				cancel()
			}
			err = sess.WriteInboxAction(ctx, ready.Revision, ready.Input, []byte("Review the PR"), []byte("\r"), false)
			raw, readErr := os.ReadFile(output.Name())
			if readErr != nil {
				t.Fatal(readErr)
			}
			if tc.wantSend {
				if err != nil || string(raw) != "Review the PR\r" || sess.Observation().Input != ready.Input+1 {
					t.Fatalf("instruction was not delivered exactly once: %q, %v", raw, err)
				}
			} else if !errors.Is(err, ErrObservedChanged) || len(raw) != 0 {
				t.Fatalf("stale handoff wrote bytes: %q, %v", raw, err)
			}
		})
	}
}
