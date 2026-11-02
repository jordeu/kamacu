package session

import (
	"os"
	"strings"
	"testing"
	"time"

	"kamacu/internal/observation"
)

const choiceScreen = `☐ Approach
Which approach?
❯ 1. Keep it
  2. Replace it
  3. Type something.
Enter to select · ↑/↓ to navigate · Esc to cancel`

func choiceSession(t *testing.T) (*Session, *os.File) {
	t.Helper()
	f, err := os.CreateTemp(t.TempDir(), "input")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { f.Close() })
	observer := observation.New(100, 30)
	observer.Feed([]byte(strings.ReplaceAll(choiceScreen, "\n", "\r\n")))
	return &Session{status: StatusRunning, engine: "claude", ptmx: f, observer: observer}, f
}

func TestObservedChoiceWaitsForRenderedSelection(t *testing.T) {
	s, f := choiceSession(t)
	obs := s.Observation()
	done := make(chan error, 1)
	go func() { done <- s.WriteObservedChoice(obs.Revision, obs.Input, "Replace it") }()
	deadline := time.Now().Add(time.Second)
	for {
		b, _ := os.ReadFile(f.Name())
		if len(b) > 0 {
			if string(b) != "\x1b[B" {
				t.Fatalf("submitted before render: %q", b)
			}
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("no navigation")
		}
		time.Sleep(time.Millisecond)
	}
	moved := strings.Replace(strings.Replace(choiceScreen, "❯ 1.", "  1.", 1), "  2.", "❯ 2.", 1)
	s.mu.Lock()
	s.observer.Feed([]byte("\x1b[2J\x1b[H" + strings.ReplaceAll(moved, "\n", "\r\n")))
	s.mu.Unlock()
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(f.Name())
	if string(b) != "\x1b[B\r" {
		t.Fatalf("wrong keys: %q", b)
	}
}

func TestObservedChoiceRejectsStaleAndUnsupportedInput(t *testing.T) {
	for _, kind := range []string{"revision", "input", "engine", "answer", "changed question"} {
		t.Run(kind, func(t *testing.T) {
			s, f := choiceSession(t)
			obs := s.Observation()
			answer := "Keep it"
			switch kind {
			case "revision":
				obs.Revision++
			case "input":
				obs.Input++
			case "engine":
				s.engine = "custom"
			case "answer":
				answer = "Invented option"
			case "changed question":
				s.observer.Feed([]byte("\x1b[2J\x1b[HOrdinary prompt >"))
			}
			if err := s.WriteObservedChoice(obs.Revision, obs.Input, answer); err == nil {
				t.Fatal("unsafe choice accepted")
			}
			b, _ := os.ReadFile(f.Name())
			if len(b) != 0 {
				t.Fatalf("wrote %q", b)
			}
		})
	}
}

func TestObservedChoiceNavigationTimeoutDoesNotSubmit(t *testing.T) {
	s, f := choiceSession(t)
	obs := s.Observation()
	if err := s.WriteObservedChoice(obs.Revision, obs.Input, "Replace it"); err == nil {
		t.Fatal("unconfirmed navigation accepted")
	}
	b, _ := os.ReadFile(f.Name())
	if string(b) != "\x1b[B" {
		t.Fatalf("unexpected keys: %q", b)
	}
}

func TestObservedChoiceDoesNotSubmitAfterMenuBecomesInactive(t *testing.T) {
	s, f := choiceSession(t)
	obs := s.Observation()
	done := make(chan error, 1)
	go func() { done <- s.WriteObservedChoice(obs.Revision, obs.Input, "Replace it") }()
	deadline := time.Now().Add(time.Second)
	for {
		b, _ := os.ReadFile(f.Name())
		if len(b) > 0 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("no navigation")
		}
		time.Sleep(time.Millisecond)
	}
	// The old menu remains rendered with the expected selection, but a new
	// prompt now owns input. It must not receive the pending Enter key.
	moved := strings.Replace(strings.Replace(choiceScreen, "❯ 1.", "  1.", 1), "  2.", "❯ 2.", 1)
	s.mu.Lock()
	s.observer.Feed([]byte("\x1b[2J\x1b[H" + strings.ReplaceAll(moved+"\nQuestion answered.\n❯ ", "\n", "\r\n")))
	s.mu.Unlock()
	if err := <-done; err == nil {
		t.Fatal("inactive menu accepted")
	}
	b, _ := os.ReadFile(f.Name())
	if string(b) != "\x1b[B" {
		t.Fatalf("submitted to inactive menu: %q", b)
	}
}
