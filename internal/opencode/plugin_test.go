package opencode

import (
	"os"
	"path/filepath"
	"testing"
)

// stubConfigDir returns a configDirFunc that points at dir (never errors).
func stubConfigDir(t *testing.T, dir string) configDirFunc {
	t.Helper()
	return func() (string, error) { return dir, nil }
}

// mkTempHome makes a throwaway base config dir for the test and registers cleanup.
func mkTempHome(t *testing.T) string {
	t.Helper()
	d := t.TempDir()
	return d
}

// --- InstallPlugin: no-pollution gate --------------------------------------

// TestInstallPluginNoopWhenOpencodeAbsent proves the load-bearing no-pollution
// gate: if the opencode config dir does not exist, InstallPlugin writes
// NOTHING — not the dir, not plugin/, not the file. This is what keeps a
// non-opencode user's home directory clean.
func TestInstallPluginNoopWhenOpencodeAbsent(t *testing.T) {
	home := mkTempHome(t)
	err := installPlugin(stubConfigDir(t, home))
	if err != nil {
		t.Fatalf("InstallPlugin with absent opencode dir: %v", err)
	}
	// Nothing under <home>/opencode should exist.
	if _, err := os.Stat(filepath.Join(home, "opencode")); !os.IsNotExist(err) {
		t.Fatalf("opencode dir should not exist; stat err=%v", err)
	}
}

// TestInstallPluginNoopWhenTargetIsFile proves the not-a-dir safety branch:
// if something named "opencode" exists but is a regular file, the installer
// leaves it alone and writes nothing.
func TestInstallPluginNoopWhenTargetIsFile(t *testing.T) {
	home := mkTempHome(t)
	opencodeAsFile := filepath.Join(home, "opencode")
	if err := os.WriteFile(opencodeAsFile, []byte("not a dir"), 0o644); err != nil {
		t.Fatalf("seed: %v", err)
	}
	if err := installPlugin(stubConfigDir(t, home)); err != nil {
		t.Fatalf("InstallPlugin with file-at-opencode: %v", err)
	}
	got, err := os.ReadFile(opencodeAsFile)
	if err != nil {
		t.Fatalf("opencode file should be untouched: %v", err)
	}
	if string(got) != "not a dir" {
		t.Fatalf("opencode file mutated: %q", got)
	}
	// And no plugin dir created. Because "opencode" is a regular file, stat'ing
	// a path through it returns ENOTDIR ("not a directory") rather than
	// ENOENT — both are proof Kamacu wrote nothing. Any stat error is the pass
	// condition; a nil error would mean something got created.
	if _, err := os.Stat(filepath.Join(home, "opencode", "plugin")); err == nil {
		t.Fatalf("plugin dir should not exist (opencode is a file); stat succeeded unexpectedly")
	}
}

// --- InstallPlugin: happy path + idempotency --------------------------------

// TestInstallPluginWritesWhenOpencodePresent proves that once opencode's config
// dir exists, the installer creates plugin/ and writes the exact embedded
// bytes (content-correct: header, env gate, event names).
func TestInstallPluginWritesWhenOpencodePresent(t *testing.T) {
	home := mkTempHome(t)
	opencodeDir := filepath.Join(home, "opencode")
	if err := os.MkdirAll(opencodeDir, 0o755); err != nil {
		t.Fatalf("seed opencode dir: %v", err)
	}

	if err := installPlugin(stubConfigDir(t, home)); err != nil {
		t.Fatalf("InstallPlugin: %v", err)
	}
	target := filepath.Join(opencodeDir, "plugin", PluginFilename)
	got, err := os.ReadFile(target)
	if err != nil {
		t.Fatalf("plugin not written: %v", err)
	}
	if !equalBytes(got, pluginSource) {
		t.Fatalf("installed plugin bytes != embedded source (got %d, want %d)", len(got), len(pluginSource))
	}
	assertPluginContent(t, got)
}

// TestInstallPluginIdempotentSkip proves the skip-on-match no-op: a second
// InstallPlugin against byte-identical content performs no write (verified
// via mtime preservation).
func TestInstallPluginIdempotentSkip(t *testing.T) {
	home := mkTempHome(t)
	opencodeDir := filepath.Join(home, "opencode")
	_ = os.MkdirAll(opencodeDir, 0o755)

	if err := installPlugin(stubConfigDir(t, home)); err != nil {
		t.Fatalf("first install: %v", err)
	}
	target := filepath.Join(opencodeDir, "plugin", PluginFilename)
	info1, err := os.Stat(target)
	if err != nil {
		t.Fatalf("stat after first install: %v", err)
	}

	if err := installPlugin(stubConfigDir(t, home)); err != nil {
		t.Fatalf("second install: %v", err)
	}
	info2, err := os.Stat(target)
	if err != nil {
		t.Fatalf("stat after second install: %v", err)
	}
	// File still present and byte-identical.
	got, _ := os.ReadFile(target)
	if !equalBytes(got, pluginSource) {
		t.Fatalf("content drifted after second install")
	}
	// mtime unchanged: the identical-content branch skipped the write entirely.
	if !info1.ModTime().Equal(info2.ModTime()) {
		t.Fatalf("idempotent skip should preserve mtime: %v vs %v", info1.ModTime(), info2.ModTime())
	}
}

// TestInstallPluginOverwritesStale proves the upgrade path: a drifted/stale
// plugin file (e.g. an older Kamacu shipped a different plugin) is overwritten
// with the current embedded source on the next boot.
func TestInstallPluginOverwritesStale(t *testing.T) {
	home := mkTempHome(t)
	opencodeDir := filepath.Join(home, "opencode")
	pluginDir := filepath.Join(opencodeDir, "plugin")
	_ = os.MkdirAll(pluginDir, 0o755)
	target := filepath.Join(pluginDir, PluginFilename)
	if err := os.WriteFile(target, []byte("// old kamacu plugin, stale"), 0o644); err != nil {
		t.Fatalf("seed stale: %v", err)
	}

	if err := installPlugin(stubConfigDir(t, home)); err != nil {
		t.Fatalf("InstallPlugin overwrite: %v", err)
	}
	got, err := os.ReadFile(target)
	if err != nil {
		t.Fatalf("stat after overwrite: %v", err)
	}
	if !equalBytes(got, pluginSource) {
		t.Fatalf("stale plugin was not overwritten")
	}
}

// TestInstallPluginConfigDirError proves an error resolving the base config
// dir bubbles (never silently writes to the wrong place).
func TestInstallPluginConfigDirError(t *testing.T) {
	errFn := func() (string, error) { return "", os.ErrInvalid }
	err := installPlugin(errFn)
	if err == nil {
		t.Fatalf("expected error from config dir resolution")
	}
}

// --- PluginSource: embedded bytes match disk content ------------------------

// TestPluginSourceMatchesFile proves the go:embed slice is the same bytes as
// the on-disk kamacu-status.js (guards against a stale embed after a hand-edit
// that forgets `go generate`/a rebuild).
func TestPluginSourceMatchesFile(t *testing.T) {
	src := PluginSource()
	if len(src) == 0 {
		t.Fatal("PluginSource returned empty bytes")
	}
	assertPluginContent(t, src)
}

// --- helpers ----------------------------------------------------------------

// assertPluginContent pins the load-bearing properties of the shipped plugin so
// a future edit can't silently break the env gate, the claude-compatible event
// mapping, or the managed-file header. These are the invariants the slice plan
// names as Must-Haves.
func assertPluginContent(t *testing.T, b []byte) {
	t.Helper()
	s := string(b)
	mustContain := []string{
		"kamacu-managed",           // managed-file header (overwritten at startup)
		"KAMACU_SESSION_ID",        // env gate reads it
		"KAMACU_HOOK_TOKEN",        // env gate reads it
		"KAMACU_HOOK_BASE",         // env gate reads it
		"hook_event_name",          // claude-compatible wire contract key
		"'SessionStart'",           // session.created -> SessionStart -> MarkHooksAlive
		"'Stop'",                   // idle -> Stop -> SetIdle
		"'Notification'",           // permission.ask -> Notification -> SetWaiting
		"X-Kamacu-Token",          // same header as claude's overlay
		"/api/hooks/sessions/",     // UNCHANGED hook receiver path
		"__kamacuOpencodePluginV1", // singleton guard
		"parentID",                 // child-session suppression
		"fetch",                    // D014: POST via fetch(), not curl
		"AbortController",          // 3s timeout matches claude overlay -m 3
	}
	for _, want := range mustContain {
		if !contains(s, want) {
			t.Errorf("plugin source missing required token %q (env gate / wire contract / child-suppression invariant)", want)
		}
	}
	// Negative assertion: the plugin must NOT depend on curl. The original S01
	// notify() shelled out to `curl -s -m 3` via Bun $, which silently no-ops
	// when curl is absent from PATH — leaving hooksAlive stuck false and
	// opencode tasks pinned on "working". fetch() + AbortController removes
	// the PATH dependency (D014). If this fires, notify() regressed to curl.
	if contains(s, "curl") {
		t.Errorf("plugin source must not depend on curl (silent no-op when curl is absent from PATH); use fetch + AbortController. Found 'curl' substring.")
	}
}

func contains(s, sub string) bool {
	return len(sub) == 0 || (len(s) >= len(sub) && indexOf(s, sub) >= 0)
}

func indexOf(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}

func equalBytes(a, b []byte) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
