// Package opencode ships and installs the Kamacu status plugin for the
// opencode agent engine (M002, D013/D014).
//
// opencode cannot receive a per-instance hook command via argv the way claude
// does (--settings overlay). Instead it loads status plugins from static .js
// files in ${XDG_CONFIG_HOME:-~/.config}/opencode/plugin/ (verified
// empirically: the dir is the SINGULAR `plugin/`, mirroring the reference
// slayzone-notify.js that already lives there). Kamacu therefore ships an
// env-gated plugin (kamacu-status.js) that no-ops outside a Kamacu PTY and
// curls the UNCHANGED claude hook receiver with claude-compatible event names
// when Kamacu spawned the process.
//
// InstallPlugin is the single public entry point and is wired into startup
// (cmd/kamacu/main.go). It is a silent no-op when opencode is not installed
// (the opencode config dir is absent) so it never pollutes a non-opencode
// user's home directory.
package opencode

import (
	"bytes"
	_ "embed"
	"fmt"
	"os"
	"path/filepath"
)

// PluginFilename is the on-disk plugin name. opencode imports every .js file
// in plugin/, so this stable name also lets a future uninstaller remove it.
const PluginFilename = "kamacu-status.js"

//go:embed kamacu-status.js
var pluginSource []byte

// configDirFunc resolves the base per-user config directory
// ($XDG_CONFIG_HOME or $HOME/.config on Linux, the platform equivalent
// elsewhere) — exactly what os.UserConfigDir returns. Overridable in tests so
// the installer can target a temp tree without touching the real home dir.
type configDirFunc func() (string, error)

func defaultConfigDir() (string, error) { return os.UserConfigDir() }

// InstallPlugin idempotently writes the env-gated opencode status plugin to
// <configDir>/opencode/plugin/kamacu-status.js.
//
// Gating: it writes ONLY when <configDir>/opencode already exists — a user
// who has never run opencode sees no file written (no pollution). opencode
// creates ~/.config/opencode on first run, so its presence is a reliable
// "opencode is in use" signal.
//
// Idempotency: when the target already holds byte-identical content the write
// is skipped (a cheap no-op on healthy boots that also avoids mtime churn).
// A stale/drifted file is overwritten so an upgrade is a drop-in refresh — the
// same regenerate-at-startup posture as tmux.WriteConfig, but skip-on-match.
//
// The plugin carries a kamacu-managed header and is safe to overwrite.
func InstallPlugin() error {
	return installPlugin(defaultConfigDir)
}

func installPlugin(configDir configDirFunc) error {
	base, err := configDir()
	if err != nil {
		return fmt.Errorf("resolving user config dir: %w", err)
	}

	// No-op when opencode is not installed: do not create the opencode dir,
	// do not create plugin/, write nothing. This is the no-pollution gate.
	opencodeDir := filepath.Join(base, "opencode")
	info, err := os.Stat(opencodeDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil // opencode absent — silently skip
		}
		return fmt.Errorf("checking opencode config dir: %w", err)
	}
	if !info.IsDir() {
		return nil // something occupies the name but it isn't a dir — leave it
	}

	// opencode IS installed: ensure plugin/ exists and write our plugin.
	pluginDir := filepath.Join(opencodeDir, "plugin")
	if err := os.MkdirAll(pluginDir, 0o755); err != nil {
		return fmt.Errorf("creating opencode plugin dir: %w", err)
	}

	target := filepath.Join(pluginDir, PluginFilename)
	if existing, rerr := os.ReadFile(target); rerr == nil {
		if bytes.Equal(existing, pluginSource) {
			return nil // identical — skip the write (idempotent no-op)
		}
		// else: drifted/stale — fall through and overwrite.
	} else if !os.IsNotExist(rerr) {
		return fmt.Errorf("reading existing opencode plugin: %w", rerr)
	}

	if err := os.WriteFile(target, pluginSource, 0o644); err != nil {
		return fmt.Errorf("writing opencode plugin: %w", err)
	}
	return nil
}

// PluginSource returns the embedded plugin bytes. Exposed so callers/tests can
// assert content (env gate, event names, kamacu-managed header) without a
// round-trip to disk, and so a future diagnostics surface can print what
// Kamacu intends to install.
func PluginSource() []byte {
	// Defensive copy so callers cannot mutate the embedded slice.
	out := make([]byte, len(pluginSource))
	copy(out, pluginSource)
	return out
}
