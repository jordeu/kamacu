package session

import (
	"context"
	"errors"
	"testing"
	"time"
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
