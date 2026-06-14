package github

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

// TestPRStateNoGH forces gh to be unresolvable (an empty PATH) and asserts
// PRState degrades to a typed error + an empty string (degrade-don't-break,
// D-03) — so the reaper warn-and-skips a PR rather than crashing the tick.
func TestPRStateNoGH(t *testing.T) {
	t.Setenv("PATH", "")
	s := New(Config{})
	state, err := s.PRState(context.Background(), "cli/cli", 1)
	if err == nil {
		t.Fatal("PRState with gh absent returned nil error, want a typed error")
	}
	if state != "" {
		t.Errorf("PRState with gh absent returned %q, want an empty string", state)
	}
}

// TestPRStateFakeGH stubs `gh` with a tiny script that echoes a {"state":...}
// payload, so the full PRState path (exec + decode) is exercised end-to-end
// without the real gh CLI or network. UPPERCASE is the verified gh contract
// (RESEARCH §A: OPEN|CLOSED|MERGED).
func TestPRStateFakeGH(t *testing.T) {
	for _, want := range []string{"OPEN", "CLOSED", "MERGED"} {
		t.Run(want, func(t *testing.T) {
			dir := t.TempDir()
			fake := filepath.Join(dir, "gh")
			// Echo a single-line {"state":"<want>"} (only the `echo` builtin, so
			// it works under the restricted PATH this test sets).
			script := "#!/bin/sh\necho '{\"state\":\"" + want + "\"}'\n"
			if err := os.WriteFile(fake, []byte(script), 0o755); err != nil {
				t.Fatalf("write fake gh: %v", err)
			}
			t.Setenv("PATH", dir)

			s := New(Config{})
			got, err := s.PRState(context.Background(), "cli/cli", 1)
			if err != nil {
				t.Fatalf("PRState with fake gh: %v", err)
			}
			if got != want {
				t.Errorf("PRState = %q, want %q (UPPERCASE single source)", got, want)
			}
		})
	}
}
