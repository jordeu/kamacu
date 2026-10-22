package integration

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"github.com/pelletier/go-toml/v2"
	"github.com/tailscale/hujson"
)

// registration edits only the global Kamacu entry. JSONC edits retain comments;
// Codex owns its TOML edits through its CLI. No config contents enter diagnostics.
type registration struct {
	engine, path, key string
	data              []byte
	entry             json.RawMessage
	tree              hujson.Value
}

func (m *Manager) command(ctx context.Context, args ...string) ([]byte, error) {
	if m.runCodex != nil {
		return m.runCodex(ctx, args...)
	}
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	bin, err := m.lookPath("codex")
	if err != nil {
		return nil, fmt.Errorf("Codex is not installed")
	}
	cmd := exec.CommandContext(ctx, bin, args...)
	cmd.Dir = m.home
	cmd.Env = append(os.Environ(), "CODEX_HOME="+m.codexHome)
	cmd.WaitDelay = 100 * time.Millisecond
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("Codex MCP configuration command failed; check codex mcp list in your terminal")
	}
	return out, nil
}

func (m *Manager) readRegistration(ctx context.Context, engine string) (*registration, error) {
	r := &registration{engine: engine}
	if engine == "codex" {
		// The CLI list is an effective runtime view and omits persisted fields
		// such as tool restrictions. Include the entire stored registration in
		// the ownership fingerprint so updates and removal preserve all edits.
		r.path = filepath.Join(m.codexHome, "config.toml")
		data, err := os.ReadFile(r.path)
		if err != nil && !errors.Is(err, os.ErrNotExist) {
			return nil, err
		}
		r.data = data
		var config struct {
			Servers map[string]map[string]any `toml:"mcp_servers"`
		}
		if err := toml.Unmarshal(data, &config); err != nil {
			return nil, fmt.Errorf("invalid Codex configuration file: %s", r.path)
		}
		persisted := config.Servers["kamacu"]
		if m.runCodex == nil {
			if _, err := os.Stat(m.codexHome); errors.Is(err, os.ErrNotExist) {
				return r, nil
			} else if err != nil {
				return nil, err
			}
		}
		data, err = m.command(ctx, "mcp", "list", "--json")
		if err != nil {
			return nil, err
		}
		var entries []struct {
			Name      string          `json:"name"`
			Enabled   bool            `json:"enabled"`
			Transport json.RawMessage `json:"transport"`
		}
		if json.Unmarshal(data, &entries) != nil {
			return nil, fmt.Errorf("could not read Codex MCP registrations")
		}
		for _, e := range entries {
			if e.Name == "kamacu" {
				r.entry, err = json.Marshal(struct {
					Enabled       bool            `json:"enabled"`
					Transport     json.RawMessage `json:"transport"`
					Configuration map[string]any  `json:"configuration"`
				}{e.Enabled, e.Transport, persisted})
				if err != nil {
					return nil, fmt.Errorf("could not fingerprint Codex registration")
				}
			}
		}
		return r, nil
	}
	r.path = filepath.Join(m.home, ".claude.json")
	r.key = "mcpServers"
	if m.claudeHome != filepath.Join(m.home, ".claude") {
		r.path = filepath.Join(m.claudeHome, ".claude.json")
	}
	if engine == "opencode" {
		r.key = "mcp"
		r.path = filepath.Join(m.configHome, "opencode", "opencode.json")
		jsonc := filepath.Join(m.configHome, "opencode", "opencode.jsonc")
		if _, err := os.Stat(jsonc); err == nil {
			if _, err := os.Stat(r.path); err == nil {
				return nil, fmt.Errorf("both opencode.json and opencode.jsonc exist; consolidate them before enabling integration")
			}
			r.path = jsonc
		}
	}
	data, err := os.ReadFile(r.path)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}
	r.data = data
	if len(data) == 0 {
		if err == nil {
			return nil, fmt.Errorf("empty configuration file: %s", r.path)
		}
		data = []byte("{}\n")
	}
	r.tree, err = hujson.Parse(data)
	if err != nil {
		return nil, fmt.Errorf("invalid configuration file: %s", r.path)
	}
	if _, ok := r.tree.Value.(*hujson.Object); !ok {
		return nil, fmt.Errorf("expected an object in %s", r.path)
	}
	if v := r.tree.Find("/" + r.key); v != nil {
		if _, ok := v.Value.(*hujson.Object); !ok {
			return nil, fmt.Errorf("expected %s to be an object in %s", r.key, r.path)
		}
	}
	if v := r.tree.Find("/" + r.key + "/kamacu"); v != nil {
		copy := v.Clone()
		copy.Standardize()
		r.entry = copy.Pack()
	}
	return r, nil
}

func canonical(data []byte) []byte {
	var v any
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.UseNumber()
	if decoder.Decode(&v) != nil {
		return data
	}
	out, _ := json.Marshal(v)
	return out
}

func (m *Manager) compatible(r *registration) bool {
	var e struct {
		Command   json.RawMessage `json:"command"`
		Args      []string        `json:"args"`
		Type      string          `json:"type"`
		Enabled   *bool           `json:"enabled"`
		Transport struct {
			Type    string   `json:"type"`
			Command string   `json:"command"`
			Args    []string `json:"args"`
		} `json:"transport"`
	}
	if json.Unmarshal(r.entry, &e) != nil || (e.Enabled != nil && !*e.Enabled) {
		return false
	}
	var command string
	args := e.Args
	switch r.engine {
	case "codex":
		if e.Transport.Type != "stdio" {
			return false
		}
		command = e.Transport.Command
		args = e.Transport.Args
	case "opencode":
		if e.Type != "local" {
			return false
		}
		var cmd []string
		if json.Unmarshal(e.Command, &cmd) != nil || len(cmd) == 0 {
			return false
		}
		command = cmd[0]
		args = cmd[1:]
	default:
		if e.Type != "" && e.Type != "stdio" {
			return false
		}
		if json.Unmarshal(e.Command, &command) != nil {
			return false
		}
	}
	sameBinary := command == m.binary
	if !sameBinary {
		resolved := command
		if !filepath.IsAbs(resolved) {
			resolved, _ = m.lookPath(resolved)
		}
		left, err1 := filepath.EvalSymlinks(resolved)
		right, err2 := filepath.EvalSymlinks(m.binary)
		sameBinary = err1 == nil && err2 == nil && left == right
	}
	return sameBinary && len(args) == 2 && args[0] == "mcp" && args[1] == "serve"
}

func (m *Manager) writeRegistration(ctx context.Context, r *registration, remove bool) error {
	if r.engine == "codex" {
		current, err := os.ReadFile(r.path)
		if err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
		if !bytes.Equal(current, r.data) {
			return fmt.Errorf("configuration changed; retry the operation")
		}
		if remove {
			_, err := m.command(ctx, "mcp", "remove", "kamacu")
			return err
		}
		if err := os.MkdirAll(m.codexHome, 0700); err != nil {
			return err
		}
		_, err = m.command(ctx, "mcp", "add", "kamacu", "--", m.binary, "mcp", "serve")
		return err
	}
	// Refuse to replace a config concurrently changed by another application.
	current, err := os.ReadFile(r.path)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if !bytes.Equal(current, r.data) {
		return fmt.Errorf("configuration changed; retry the operation")
	}
	var patches []map[string]any
	if remove {
		patches = append(patches, map[string]any{"op": "remove", "path": "/" + r.key + "/kamacu"})
	} else {
		if r.tree.Find("/"+r.key) == nil {
			patches = append(patches, map[string]any{"op": "add", "path": "/" + r.key, "value": map[string]any{}})
		}
		var entry any = map[string]any{"command": m.binary, "args": []string{"mcp", "serve"}, "type": "stdio"}
		if r.engine == "opencode" {
			entry = map[string]any{"type": "local", "command": []string{m.binary, "mcp", "serve"}, "enabled": true}
		}
		patches = append(patches, map[string]any{"op": "add", "path": "/" + r.key + "/kamacu", "value": entry})
	}
	patch, _ := json.Marshal(patches)
	if err := r.tree.Patch(patch); err != nil {
		return fmt.Errorf("could not edit MCP configuration")
	}
	return atomicWrite(r.path, r.tree.Pack(), 0600)
}
