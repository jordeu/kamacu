package codex

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// stubCodexHome returns a codexHomeFunc that points at dir (never errors).
func stubCodexHome(dir string) codexHomeFunc {
	return func() (string, error) { return dir, nil }
}

// --- InstallHookConfig: no-pollution gate -----------------------------------

// TestInstallHookConfigNoopWhenCodexAbsent proves the no-pollution gate: if
// the codex home (~/.codex, CODEX_HOME override honored) does not exist, the
// installer writes NOTHING — a user who never ran codex sees no file.
func TestInstallHookConfigNoopWhenCodexAbsent(t *testing.T) {
	home := t.TempDir()
	if err := installHookConfig(stubCodexHome(filepath.Join(home, ".codex"))); err != nil {
		t.Fatalf("InstallHookConfig with absent codex home: %v", err)
	}
	if _, err := os.Stat(filepath.Join(home, ".codex")); !os.IsNotExist(err) {
		t.Fatalf("codex home should not exist; stat err=%v", err)
	}
}

// TestInstallHookConfigNoopWhenCodexHomeIsFile proves the not-a-dir safety
// branch: if the codex home path is occupied by a regular file, the installer
// leaves it untouched and writes nothing.
func TestInstallHookConfigNoopWhenCodexHomeIsFile(t *testing.T) {
	home := t.TempDir()
	asFile := filepath.Join(home, ".codex")
	if err := os.WriteFile(asFile, []byte("not a dir"), 0o644); err != nil {
		t.Fatalf("seed: %v", err)
	}
	if err := installHookConfig(stubCodexHome(asFile)); err != nil {
		t.Fatalf("InstallHookConfig with file-at-codex-home: %v", err)
	}
	got, err := os.ReadFile(asFile)
	if err != nil {
		t.Fatalf("codex home file should be untouched: %v", err)
	}
	if string(got) != "not a dir" {
		t.Fatalf("codex home file mutated: %q", got)
	}
}

// --- InstallHookConfig: happy path + idempotency ----------------------------

// TestInstallHookConfigWritesWhenCodexPresent proves that once the codex home
// exists, the installer writes the exact embedded bytes to kamacu.config.toml
// (the exclusive profile layer selected by `-p kamacu` at spawn).
func TestInstallHookConfigWritesWhenCodexPresent(t *testing.T) {
	codexDir := t.TempDir()
	if err := installHookConfig(stubCodexHome(codexDir)); err != nil {
		t.Fatalf("InstallHookConfig: %v", err)
	}
	got, err := os.ReadFile(filepath.Join(codexDir, ProfileFilename))
	if err != nil {
		t.Fatalf("hook config not written: %v", err)
	}
	assertHookConfigContent(t, got)
}

// TestInstallHookConfigIdempotentSkip proves the skip-on-match no-op: a second
// install against byte-identical content performs no write (mtime preserved)
// — load-bearing because codex records hook trust against the hook's hash:
// churning the file on every boot would silently untrust the hooks.
func TestInstallHookConfigIdempotentSkip(t *testing.T) {
	codexDir := t.TempDir()
	if err := installHookConfig(stubCodexHome(codexDir)); err != nil {
		t.Fatalf("first install: %v", err)
	}
	target := filepath.Join(codexDir, ProfileFilename)
	info1, err := os.Stat(target)
	if err != nil {
		t.Fatalf("stat after first install: %v", err)
	}

	if err := installHookConfig(stubCodexHome(codexDir)); err != nil {
		t.Fatalf("second install: %v", err)
	}
	info2, err := os.Stat(target)
	if err != nil {
		t.Fatalf("stat after second install: %v", err)
	}
	if !info1.ModTime().Equal(info2.ModTime()) {
		t.Fatalf("idempotent skip should preserve mtime: %v vs %v", info1.ModTime(), info2.ModTime())
	}
}

// TestInstallHookConfigOverwritesStale proves the upgrade path: a drifted
// kamacu.config.toml (older kamacu shipped a different definition) is
// overwritten with the current embedded source on the next boot.
func TestInstallHookConfigOverwritesStale(t *testing.T) {
	codexDir := t.TempDir()
	target := filepath.Join(codexDir, ProfileFilename)
	if err := os.WriteFile(target, []byte("# stale kamacu codex profile"), 0o644); err != nil {
		t.Fatalf("seed stale: %v", err)
	}
	if err := installHookConfig(stubCodexHome(codexDir)); err != nil {
		t.Fatalf("InstallHookConfig overwrite: %v", err)
	}
	got, err := os.ReadFile(target)
	if err != nil {
		t.Fatalf("read after overwrite: %v", err)
	}
	assertHookConfigContent(t, got)
}

// TestInstallHookConfigHomeError proves an error resolving the codex home
// bubbles (never silently writes to the wrong place).
func TestInstallHookConfigHomeError(t *testing.T) {
	err := installHookConfig(func() (string, error) { return "", errors.New("boom") })
	if err == nil {
		t.Fatal("expected error from codex home resolution")
	}
}

// --- HookConfigSource: embedded bytes carry the wire contract ----------------

// TestHookConfigSourceMatchesFile proves the go:embed slice is non-empty and
// carries the load-bearing invariants (env gate, event names, sync Stop).
func TestHookConfigSourceMatchesFile(t *testing.T) {
	src := HookConfigSource()
	if len(src) == 0 {
		t.Fatal("HookConfigSource returned empty bytes")
	}
	assertHookConfigContent(t, src)
}

// --- helpers -----------------------------------------------------------------

// sectionOf returns the slice of s spanning one top-level hook section: from
// its header (e.g. "[[hooks.Stop]]") up to the NEXT top-level header or EOF.
// The `[[hooks.X.hooks]]` handler sub-tables are skipped by requiring no '.'
// between "[[hooks." and "]]". Used for section-scoped assertions
// (sync-vs-async) without a TOML parser dep.
func sectionOf(s, header string) string {
	start := strings.Index(s, header)
	if start < 0 {
		return ""
	}
	rest := s[start+len(header):]
	for from := 0; from < len(rest); {
		i := strings.Index(rest[from:], "[[hooks.")
		if i < 0 {
			return s[start:]
		}
		i += from
		end := strings.Index(rest[i:], "]]")
		if end < 0 {
			return s[start:]
		}
		name := rest[i+len("[[hooks.") : i+end]
		if !strings.Contains(name, ".") {
			return s[start : start+len(header)+i]
		}
		from = i + end
	}
	return s[start:]
}

// assertHookConfigContent pins the load-bearing properties of the shipped
// profile so a future edit can't silently break the env gate, the
// claude-compatible event mapping, or the verified sync-Stop requirement
// (an async Stop hook is cancelled at codex session close — verified against
// codex 0.157.0 — and would never reach the receiver).
func assertHookConfigContent(t *testing.T, b []byte) {
	t.Helper()
	s := string(b)
	mustContain := []string{
		"kamacu-managed",       // managed-file header (overwritten at startup)
		"KAMACU_SESSION_ID",    // env gate + receiver URL segment
		"KAMACU_HOOK_TOKEN",    // X-Kamacu-Token value
		"KAMACU_HOOK_BASE",     // receiver origin
		"X-Kamacu-Token",       // same header as claude's overlay
		"/api/hooks/sessions/", // UNCHANGED hook receiver path
		`[ -n \"$KAMACU_SESSION_ID\" ]`, // env gate (TOML-escaped quotes = file bytes): complete no-op outside kamacu
		"[[hooks.SessionStart]]",      // -> MarkHooksAlive (BEL fallback off)
		"[[hooks.Stop]]",              // -> SetIdle (turn end)
		"[[hooks.PermissionRequest]]", // -> SetWaiting (approval prompt)
	}
	for _, want := range mustContain {
		if !strings.Contains(s, want) {
			t.Errorf("hook config missing required token %q (env gate / wire contract invariant)", want)
		}
	}
	// Stop MUST be synchronous: codex cancels unfinished background hooks at
	// session close (verified), so an async Stop would be lost on the final
	// turn. SessionStart/PermissionRequest are async (verified to fire; never
	// block the turn).
	for _, sec := range []struct{ header, name string }{
		{"[[hooks.SessionStart]]", "SessionStart"},
		{"[[hooks.PermissionRequest]]", "PermissionRequest"},
	} {
		body := sectionOf(s, sec.header)
		if body == "" {
			t.Fatalf("section %s not found", sec.name)
		}
		if !strings.Contains(body, "async = true") {
			t.Errorf("%s hook must set async = true (never block the turn)", sec.name)
		}
	}
	stopBody := sectionOf(s, "[[hooks.Stop]]")
	if stopBody == "" {
		t.Fatal("section Stop not found")
	}
	if strings.Contains(stopBody, "async = true") {
		t.Errorf("Stop hook must NOT be async (cancelled at session close — verified codex 0.157.0); use a sync hook")
	}
	if !strings.Contains(stopBody, "timeout") {
		t.Errorf("Stop hook must carry an explicit timeout")
	}
}
