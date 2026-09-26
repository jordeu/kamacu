// Package codex ships and installs the Kamacu status hook profile for the
// codex agent engine.
//
// codex loads lifecycle hooks (SessionStart / Stop / PermissionRequest, one
// JSON object on stdin with hook_event_name — the same wire contract as
// claude's hooks) from config layers next to its active config files. Kamacu
// therefore ships an env-gated profile (kamacu.config.toml) that no-ops
// outside a Kamacu PTY and POSTs to the UNCHANGED claude hook receiver.
//
// The profile is an EXCLUSIVE layer kamacu owns (selected at spawn via
// `codex -p kamacu`): unlike a shared ~/.codex/hooks.json it never merges
// with, reformats, or clobbers the user's own hook definitions. Content is
// byte-stable across boots (skip-on-match idempotency) because codex records
// hook trust against the hook's hash — churning the file would silently
// untrust the hooks on every startup.
//
// InstallHookConfig is the single public entry point, wired into startup
// (cmd/kamacu/serve.go). It is a silent no-op when codex is not installed
// (the codex home dir is absent) so it never pollutes a non-codex user's
// home directory.
package codex

import (
	"bytes"
	_ "embed"
	"fmt"
	"os"
	"path/filepath"
)

// ProfileFilename is the on-disk profile name codex resolves for
// `-p kamacu` as $CODEX_HOME/kamacu.config.toml.
const ProfileFilename = "kamacu.config.toml"

//go:embed kamacu.config.toml
var hookConfigSource []byte

// codexHomeFunc resolves the codex home directory (CODEX_HOME when set —
// codex's own precedence — else ~/.codex). Overridable in tests so the
// installer targets a temp tree without touching the real home dir.
type codexHomeFunc func() (string, error)

func defaultCodexHome() (string, error) {
	if v := os.Getenv("CODEX_HOME"); v != "" {
		return v, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolve home dir: %w", err)
	}
	return filepath.Join(home, ".codex"), nil
}

// InstallHookConfig idempotently writes the env-gated codex status profile to
// <codex home>/kamacu.config.toml.
//
// Gating: it writes ONLY when the codex home already exists — a user who has
// never run codex sees no file written (no pollution). codex creates
// ~/.codex on first run, so its presence is a reliable "codex is in use"
// signal.
//
// Idempotency: when the target already holds byte-identical content the write
// is skipped — not just cheap: codex records hook trust against the hook
// definition's hash, so a stable file keeps the user's one-time /hooks trust
// valid across boots. A stale/drifted file is overwritten so an upgrade is a
// drop-in refresh (re-trust once via /hooks after a kamacu hook-definition
// change — documented in the profile header).
//
// The profile carries a kamacu-managed header and is safe to overwrite.
func InstallHookConfig() error {
	return installHookConfig(defaultCodexHome)
}

func installHookConfig(codexHome codexHomeFunc) error {
	base, err := codexHome()
	if err != nil {
		return fmt.Errorf("resolving codex home: %w", err)
	}

	// No-op when codex is not installed: do not create the codex home, write
	// nothing. This is the no-pollution gate.
	info, err := os.Stat(base)
	if err != nil {
		if os.IsNotExist(err) {
			return nil // codex absent — silently skip
		}
		return fmt.Errorf("checking codex home: %w", err)
	}
	if !info.IsDir() {
		return nil // something occupies the path but it isn't a dir — leave it
	}

	target := filepath.Join(base, ProfileFilename)
	if existing, rerr := os.ReadFile(target); rerr == nil {
		if bytes.Equal(existing, hookConfigSource) {
			return nil // identical — skip the write (byte-stable = trust-stable)
		}
		// else: drifted/stale — fall through and overwrite.
	} else if !os.IsNotExist(rerr) {
		return fmt.Errorf("reading existing codex hook config: %w", rerr)
	}

	if err := os.WriteFile(target, hookConfigSource, 0o644); err != nil {
		return fmt.Errorf("writing codex hook config: %w", err)
	}
	return nil
}

// HookConfigSource returns the embedded profile bytes. Exposed so callers and
// tests can assert content (env gate, event names, sync Stop) without a
// round-trip to disk, mirroring opencode.PluginSource.
func HookConfigSource() []byte {
	// Defensive copy so callers cannot mutate the embedded slice.
	out := make([]byte, len(hookConfigSource))
	copy(out, hookConfigSource)
	return out
}
