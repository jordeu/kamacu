package integration

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func testManager(t *testing.T) *Manager {
	t.Helper()
	home := t.TempDir()
	m := New(home, filepath.Join(home, ".config"), "/usr/local/bin/kamacu")
	m.lookPath = func(name string) (string, error) { return "/bin/" + name, nil }
	var codex []byte
	m.runCodex = func(ctx context.Context, args ...string) ([]byte, error) {
		if strings.Join(args, " ") == "mcp list --json" {
			if codex == nil {
				return []byte("[]"), nil
			}
			return append(append([]byte("["), codex...), ']'), nil
		}
		if len(args) > 2 && args[1] == "remove" {
			codex = nil
			return nil, nil
		}
		if len(args) == 7 && args[1] == "add" {
			codex, _ = json.Marshal(map[string]any{"name": "kamacu", "enabled": true, "transport": map[string]any{"type": "stdio", "command": args[4], "args": args[5:]}})
			return nil, nil
		}
		return nil, fmt.Errorf("unexpected codex args: %v", args)
	}
	return m
}
func read(t *testing.T, p string) []byte {
	t.Helper()
	b, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	return b
}
func write(t *testing.T, p string, b []byte) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(p), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, b, 0600); err != nil {
		t.Fatal(err)
	}
}

func TestInstallUpdateRepairAndRemove(t *testing.T) {
	for _, engine := range engines {
		t.Run(engine, func(t *testing.T) {
			m := testManager(t)
			ctx := context.Background()
			out, err := m.Enable(ctx, engine)
			if err != nil || !out.Enabled || out.MCP != "registered" {
				t.Fatal(out, err)
			}
			original, err := m.readRegistration(ctx, engine)
			if err != nil {
				t.Fatal(err)
			}
			if _, err = m.Enable(ctx, engine); err != nil {
				t.Fatal(err)
			}
			next, err := m.readRegistration(ctx, engine)
			if err != nil || !bytes.Equal(original.entry, next.entry) {
				t.Fatal("unchanged registration rewritten", err)
			}
			m.binary = "/new/path/kamacu"
			if err = m.Sync(ctx); err != nil {
				t.Fatal(err)
			}
			updated, err := m.readRegistration(ctx, engine)
			if err != nil || !m.compatible(updated) {
				t.Fatal("binary upgrade not applied", err)
			}
			if err = m.writeRegistration(ctx, updated, true); err != nil {
				t.Fatal(err)
			}
			if out, err = m.Enable(ctx, engine); err != nil || out.MCP != "registered" {
				t.Fatal("missing registration not repaired", out, err)
			}
			out, err = m.Disable(ctx, engine)
			if err != nil || out.Enabled || out.Message != "" {
				t.Fatal(out, err)
			}
			if err = m.Sync(ctx); err != nil {
				t.Fatal(err)
			}
			r, err := m.readRegistration(ctx, engine)
			if err != nil || len(r.entry) != 0 {
				t.Fatal("registration retained or reinstalled", err)
			}
		})
	}
}
func TestPreserveUnownedFilesAndMalformedConfig(t *testing.T) {
	ctx := context.Background()
	for _, config := range []string{`{"mcpServers":{"kamacu":{"command":"someone-else"}}}`, `{broken`, `null`, `{"mcpServers":42}`} {
		t.Run(config, func(t *testing.T) {
			m := testManager(t)
			p := filepath.Join(m.home, ".claude.json")
			write(t, p, []byte(config))
			if _, err := m.Enable(ctx, "claude"); err == nil {
				t.Fatal("accepted invalid/conflicting config")
			}
			if string(read(t, p)) != config {
				t.Fatal("config changed")
			}
		})
	}
	t.Run("reuse compatible unowned registration", func(t *testing.T) {
		m := testManager(t)
		p := filepath.Join(m.home, ".claude.json")
		config := `{"mcpServers":{"kamacu":{"command":"/usr/local/bin/kamacu","args":["mcp","serve"]}}}`
		write(t, p, []byte(config))
		if _, err := m.Enable(ctx, "claude"); err != nil {
			t.Fatal(err)
		}
		if _, err := m.Disable(ctx, "claude"); err != nil {
			t.Fatal(err)
		}
		if string(read(t, p)) != config {
			t.Fatal("user registration deleted")
		}
	})
}
func TestIndependentRegistrationsAndConfigComments(t *testing.T) {
	m := testManager(t)
	ctx := context.Background()
	p := filepath.Join(m.configHome, "opencode", "opencode.jsonc")
	original := []byte("{\n// My comment\n\"model\":\"example/model\",\"mcp\":{\"other\":{\"type\":\"local\",\"command\":[\"other\"]},},\n}\n")
	write(t, p, original)
	for _, engine := range []string{"codex", "opencode"} {
		if _, err := m.Enable(ctx, engine); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := m.Disable(ctx, "codex"); err != nil {
		t.Fatal(err)
	}
	remaining, err := m.readRegistration(ctx, "opencode")
	if err != nil || !m.compatible(remaining) {
		t.Fatal("other registration removed", err)
	}
	if _, err := m.Disable(ctx, "opencode"); err != nil {
		t.Fatal(err)
	}
	after := read(t, p)
	if !bytes.Contains(after, []byte("// My comment")) || !bytes.Contains(after, []byte("example/model")) || !bytes.Contains(after, []byte("other")) {
		t.Fatal("unrelated config lost")
	}
}
func TestEditedRegistrationAndBinaryUpgrade(t *testing.T) {
	m := testManager(t)
	ctx := context.Background()
	if _, err := m.Enable(ctx, "claude"); err != nil {
		t.Fatal(err)
	}
	m.binary = "/new/path/kamacu"
	if err := m.Sync(ctx); err != nil {
		t.Fatal(err)
	}
	r, err := m.readRegistration(ctx, "claude")
	if err != nil || !m.compatible(r) {
		t.Fatal("managed binary path not updated", err)
	}
	p := filepath.Join(m.home, ".claude.json")
	edited := bytes.ReplaceAll(read(t, p), []byte("/new/path/kamacu"), []byte("/user/path/kamacu"))
	write(t, p, edited)
	if _, err = m.Enable(ctx, "claude"); err == nil {
		t.Fatal("edited MCP entry accepted")
	}
	out, err := m.Disable(ctx, "claude")
	if err != nil || !strings.Contains(out.Message, "preserved") {
		t.Fatal(out, err)
	}
	if !bytes.Equal(read(t, p), edited) {
		t.Fatal("edited MCP deleted")
	}
}
func TestNoOptInNoInstallAndSymlinkPreserved(t *testing.T) {
	m := testManager(t)
	ctx := context.Background()
	if err := m.Sync(ctx); err != nil {
		t.Fatal(err)
	}
	for _, engine := range engines {
		r, err := m.readRegistration(ctx, engine)
		if err != nil || len(r.entry) != 0 {
			t.Fatal("installed without opt-in", err)
		}
	}
	target := filepath.Join(m.home, "custom-config.json")
	original := []byte(`{"other":"preserve"}`)
	write(t, target, original)
	if err := os.Symlink(target, filepath.Join(m.home, ".claude.json")); err != nil {
		t.Fatal(err)
	}
	if _, err := m.Enable(ctx, "claude"); err == nil {
		t.Fatal("replaced symlink")
	}
	if !bytes.Equal(read(t, target), original) {
		t.Fatal("symlink target changed")
	}
}

func TestUnknownManifestMetadataDoesNotManageFiles(t *testing.T) {
	m := testManager(t)
	ctx := context.Background()
	if _, err := m.Enable(ctx, "claude"); err != nil {
		t.Fatal(err)
	}
	untouched := filepath.Join(m.home, "unrelated-resource")
	content := []byte("preserve this file")
	write(t, untouched, content)
	var state map[string]any
	if err := json.Unmarshal(read(t, m.statePath), &state); err != nil {
		t.Fatal(err)
	}
	state["files"] = map[string]string{untouched: digest(content)}
	encoded, err := json.Marshal(state)
	if err != nil {
		t.Fatal(err)
	}
	write(t, m.statePath, encoded)
	if err = m.Sync(ctx); err != nil {
		t.Fatal(err)
	}
	statuses, err := m.Status(ctx)
	if err != nil || !statuses[0].Enabled || statuses[0].MCP != "registered" {
		t.Fatal(statuses, err)
	}
	if _, err = m.Disable(ctx, "claude"); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(read(t, untouched), content) {
		t.Fatal("unrelated file changed")
	}
}
func TestConcurrentConfigChangeRejected(t *testing.T) {
	m := testManager(t)
	ctx := context.Background()
	r, err := m.readRegistration(ctx, "claude")
	if err != nil {
		t.Fatal(err)
	}
	write(t, r.path, []byte(`{"other":"new"}`))
	if err = m.writeRegistration(ctx, r, false); err == nil {
		t.Fatal("lost concurrent config edit")
	}
}

// Opt-in smoke test of the real Codex TOML adapter. CODEX_HOME is scoped to
// this manager's disposable directory; existing user configuration is untouched.
func TestRealCodexRegistration(t *testing.T) {
	if os.Getenv("KAMACU_CODEX_TEST") != "1" {
		t.Skip("set KAMACU_CODEX_TEST=1")
	}
	m := testManager(t)
	m.runCodex = nil
	m.lookPath = exec.LookPath
	fresh := testManager(t)
	fresh.runCodex = nil
	fresh.lookPath = exec.LookPath
	if _, err := fresh.Enable(context.Background(), "codex"); err != nil {
		t.Fatal("fresh Codex home:", err)
	}
	if _, err := fresh.Disable(context.Background(), "codex"); err != nil {
		t.Fatal(err)
	}
	if _, err := m.lookPath("codex"); err != nil {
		t.Skip("codex not installed")
	}
	p := filepath.Join(m.codexHome, "config.toml")
	write(t, p, []byte("# Preserve my comment\nmodel = \"gpt-6.1-sol\"\n[mcp_servers.other]\ncommand = \"other-server\"\n"))
	if _, err := m.Enable(context.Background(), "codex"); err != nil {
		t.Fatal(err)
	}
	if _, err := m.Disable(context.Background(), "codex"); err != nil {
		t.Fatal(err)
	}
	b := read(t, p)
	if !bytes.Contains(b, []byte("# Preserve my comment")) || !bytes.Contains(b, []byte("other-server")) || bytes.Contains(b, []byte("mcp_servers.kamacu")) {
		t.Fatal("unexpected config edits")
	}
}

func TestCodexPersistedEditsArePreserved(t *testing.T) {
	// Keep the CLI response fixed: these changes must be detected even when
	// omitted from `mcp list --json` or unknown to the current Codex release.
	for _, edit := range []string{"tool_timeout_sec = 99\n", "startup_timeout_sec = 20\n", "enabled_tools = [\"ask_feedback\"]\n", "future_setting = \"keep me\"\n"} {
		t.Run(strings.TrimSpace(edit), func(t *testing.T) {
			m := testManager(t)
			ctx := context.Background()
			path := filepath.Join(m.codexHome, "config.toml")
			original := []byte("[mcp_servers.kamacu]\ncommand = \"/usr/local/bin/kamacu\"\nargs = [\"mcp\",\"serve\"]\n")
			write(t, path, original)
			if _, err := m.Enable(ctx, "codex"); err != nil {
				t.Fatal(err)
			}
			edited := append(bytes.Clone(original), []byte(edit)...)
			write(t, path, edited)
			statuses, err := m.Status(ctx)
			if err != nil {
				t.Fatal(err)
			}
			for _, status := range statuses {
				if status.Engine == "codex" && status.MCP != "modified" {
					t.Fatalf("edit not detected: %+v", status)
				}
			}
			m.binary = "/new/location/kamacu"
			if err = m.Sync(ctx); err == nil {
				t.Fatal("upgrade accepted edited entry")
			}
			if _, err = m.Enable(ctx, "codex"); err == nil {
				t.Fatal("repair accepted edited entry")
			}
			out, err := m.Disable(ctx, "codex")
			if err != nil || !strings.Contains(out.Message, "edited MCP registration preserved") {
				t.Fatalf("removal: %+v %v", out, err)
			}
			if !bytes.Equal(read(t, path), edited) {
				t.Fatal("user edits overwritten")
			}
			r, err := m.readRegistration(ctx, "codex")
			if err != nil || len(r.entry) == 0 {
				t.Fatalf("registration removed: %v", err)
			}
		})
	}
}

func TestCodexUnrelatedConfigEditDoesNotChangeOwnership(t *testing.T) {
	m := testManager(t)
	ctx := context.Background()
	p := filepath.Join(m.codexHome, "config.toml")
	original := []byte("[mcp_servers.kamacu]\ncommand = \"/usr/local/bin/kamacu\"\nargs = [\"mcp\",\"serve\"]\n")
	write(t, p, original)
	if _, err := m.Enable(ctx, "codex"); err != nil {
		t.Fatal(err)
	}
	write(t, p, append([]byte("# Edited a comment and unrelated setting\nmodel = \"another-model\"\n"), original...))
	statuses, err := m.Status(ctx)
	if err != nil {
		t.Fatal(err)
	}
	for _, status := range statuses {
		if status.Engine == "codex" && status.MCP != "registered" {
			t.Fatalf("unrelated edit changed ownership: %+v", status)
		}
	}
}

func TestRealCodexEditedTimeoutPreserved(t *testing.T) {
	if os.Getenv("KAMACU_CODEX_TEST") != "1" {
		t.Skip("set KAMACU_CODEX_TEST=1")
	}
	if _, err := exec.LookPath("codex"); err != nil {
		t.Skip("codex not installed")
	}
	m := testManager(t)
	m.runCodex = nil
	m.lookPath = exec.LookPath
	ctx := context.Background()
	if _, err := m.Enable(ctx, "codex"); err != nil {
		t.Fatal(err)
	}
	p := filepath.Join(m.codexHome, "config.toml")
	edited := append(read(t, p), []byte("tool_timeout_sec = 99\n")...)
	write(t, p, edited)
	if _, err := m.Enable(ctx, "codex"); err == nil {
		t.Fatal("repair accepted timeout edit")
	}
	out, err := m.Disable(ctx, "codex")
	if err != nil || !strings.Contains(out.Message, "preserved") {
		t.Fatalf("%+v %v", out, err)
	}
	if !bytes.Equal(read(t, p), edited) {
		t.Fatal("edited Codex registration removed")
	}
}
