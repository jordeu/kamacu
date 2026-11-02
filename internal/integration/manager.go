// Package integration manages opt-in user-level MCP registrations. Hashes, not ownership comments, protect edits.
package integration

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"sync"

	"golang.org/x/sys/unix"
)

var engines = []string{"claude", "codex", "opencode"}

type engineState struct {
	Enabled bool   `json:"enabled"`
	MCPHash string `json:"mcp_hash,omitempty"`
}
type manifest struct {
	Engines map[string]engineState `json:"engines"`
}
type Status struct {
	Engine    string `json:"engine"`
	Available bool   `json:"available"`
	Enabled   bool   `json:"enabled"`
	Managed   bool   `json:"managed"`
	MCP       string `json:"mcp"`
	Message   string `json:"message,omitempty"`
}
type Manager struct {
	mu                                  sync.Mutex
	home, configHome, binary, statePath string
	codexHome                           string
	claudeHome                          string
	runCodex                            func(context.Context, ...string) ([]byte, error)
	lookPath                            func(string) (string, error)
}

func New(home, configHome, binary string) *Manager {
	return &Manager{home: home, configHome: configHome, binary: binary, codexHome: filepath.Join(home, ".codex"), claudeHome: filepath.Join(home, ".claude"), statePath: filepath.Join(home, ".kamacu", "integrations.json"), lookPath: exec.LookPath}
}
func Default() (*Manager, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, err
	}
	config := os.Getenv("XDG_CONFIG_HOME")
	if config == "" {
		config = filepath.Join(home, ".config")
	}
	binary, err := os.Executable()
	if err != nil {
		return nil, err
	}
	m := New(home, config, binary)
	if v := os.Getenv("CODEX_HOME"); v != "" {
		m.codexHome = v
	}
	if v := os.Getenv("CLAUDE_CONFIG_DIR"); v != "" {
		m.claudeHome = v
	}
	return m, nil
}
func digest(b []byte) string { h := sha256.Sum256(b); return hex.EncodeToString(h[:]) }
func (m *Manager) load() (*manifest, error) {
	s := &manifest{Engines: map[string]engineState{}}
	b, err := os.ReadFile(m.statePath)
	if errors.Is(err, os.ErrNotExist) {
		return s, nil
	}
	if err != nil {
		return nil, err
	}
	if json.Unmarshal(b, s) != nil || s.Engines == nil {
		return nil, fmt.Errorf("invalid Kamacu integration state; preserve %s and repair it before continuing", m.statePath)
	}
	return s, nil
}
func (m *Manager) save(s *manifest) error {
	b, _ := json.MarshalIndent(s, "", "  ")
	return atomicWrite(m.statePath, append(b, '\n'), 0600)
}
func atomicWrite(path string, b []byte, mode os.FileMode) error {
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	if st, err := os.Lstat(path); err == nil {
		if !st.Mode().IsRegular() {
			return fmt.Errorf("refusing to replace non-regular file %s", path)
		}
		mode = st.Mode().Perm()
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	f, err := os.CreateTemp(filepath.Dir(path), ".kamacu-*")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	if err = f.Chmod(mode); err == nil {
		_, err = f.Write(b)
	}
	if err == nil {
		err = f.Sync()
	}
	closeErr := f.Close()
	if err == nil {
		err = closeErr
	}
	if err != nil {
		return err
	}
	return os.Rename(f.Name(), path)
}

// Mutations share a lock with CLI invocations and other running Kamacu instances.
func (m *Manager) lock() (func(), error) {
	m.mu.Lock()
	if err := os.MkdirAll(filepath.Dir(m.statePath), 0700); err != nil {
		m.mu.Unlock()
		return nil, err
	}
	f, err := os.OpenFile(m.statePath+".lock", os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		m.mu.Unlock()
		return nil, err
	}
	if err = unix.Flock(int(f.Fd()), unix.LOCK_EX|unix.LOCK_NB); err != nil {
		f.Close()
		m.mu.Unlock()
		return nil, fmt.Errorf("another integration operation is running; retry shortly")
	}
	return func() { unix.Flock(int(f.Fd()), unix.LOCK_UN); f.Close(); m.mu.Unlock() }, nil
}
func (m *Manager) status(ctx context.Context, s *manifest, engine string) Status {
	out := Status{Engine: engine, Enabled: s.Engines[engine].Enabled, Managed: s.Engines[engine].Enabled || s.Engines[engine].MCPHash != ""}
	_, err := m.lookPath(engine)
	out.Available = err == nil
	if !out.Available {
		out.MCP = "unavailable"
		return out
	}
	r, err := m.readRegistration(ctx, engine)
	if err != nil {
		out.MCP = "error"
		out.Message = err.Error()
		return out
	}
	switch {
	case len(r.entry) == 0:
		out.MCP = "missing"
	case s.Engines[engine].MCPHash != "" && s.Engines[engine].MCPHash != digest(canonical(r.entry)):
		out.MCP = "modified"
	case m.compatible(r):
		out.MCP = "registered"
	case s.Engines[engine].MCPHash != "":
		out.MCP = "outdated"
	default:
		out.MCP = "conflict"
	}
	return out
}
func (m *Manager) Status(ctx context.Context) ([]Status, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	s, err := m.load()
	if err != nil {
		return nil, err
	}
	out := make([]Status, 0, 3)
	for _, e := range engines {
		out = append(out, m.status(ctx, s, e))
	}
	return out, nil
}

// Enable also repairs missing registrations. Modified or unowned entries are never replaced.
func (m *Manager) Enable(ctx context.Context, engine string) (Status, error) {
	if !slices.Contains(engines, engine) {
		return Status{}, fmt.Errorf("unsupported agent engine")
	}
	unlock, err := m.lock()
	if err != nil {
		return Status{}, err
	}
	defer unlock()
	s, err := m.load()
	if err != nil {
		return Status{}, err
	}
	if err = m.enable(ctx, s, engine); err != nil {
		return m.status(ctx, s, engine), err
	}
	return m.status(ctx, s, engine), nil
}
func (m *Manager) enable(ctx context.Context, s *manifest, engine string) error {
	if _, err := m.lookPath(engine); err != nil {
		return fmt.Errorf("install %s before enabling its integration", engine)
	}
	r, err := m.readRegistration(ctx, engine)
	if err != nil {
		return err
	}
	e := s.Engines[engine]
	if len(r.entry) > 0 {
		if e.MCPHash != "" && e.MCPHash != digest(canonical(r.entry)) {
			return fmt.Errorf("%s MCP registration was edited; it has been preserved", engine)
		}
		if !m.compatible(r) && e.MCPHash == "" {
			return fmt.Errorf("%s already has a different kamacu MCP registration; it has been preserved", engine)
		}
	}
	// Record opt-in before writing resources so a partial install can be repaired
	// or removed. Each completed mutation is journaled before the next action.
	e.Enabled = true
	s.Engines[engine] = e
	if err = m.save(s); err != nil {
		return err
	}
	if len(r.entry) == 0 || !m.compatible(r) {
		if err = m.writeRegistration(ctx, r, false); err != nil {
			return err
		}
		r, err = m.readRegistration(ctx, engine)
		if err != nil {
			return err
		}
		if !m.compatible(r) {
			return fmt.Errorf("could not verify %s MCP registration", engine)
		}
		e.MCPHash = digest(canonical(r.entry))
		s.Engines[engine] = e
		return m.save(s)
	}
	return nil
}

// Disable stops future updates first, then removes only unchanged managed parts.
func (m *Manager) Disable(ctx context.Context, engine string) (Status, error) {
	if !slices.Contains(engines, engine) {
		return Status{}, fmt.Errorf("unsupported agent engine")
	}
	unlock, err := m.lock()
	if err != nil {
		return Status{}, err
	}
	defer unlock()
	s, err := m.load()
	if err != nil {
		return Status{}, err
	}
	e := s.Engines[engine]
	e.Enabled = false
	s.Engines[engine] = e
	if err = m.save(s); err != nil {
		return Status{}, err
	}
	var notes []error
	if e.MCPHash != "" {
		r, err := m.readRegistration(ctx, engine)
		if err != nil {
			notes = append(notes, err)
		} else if len(r.entry) == 0 {
			e.MCPHash = ""
		} else if digest(canonical(r.entry)) != e.MCPHash {
			notes = append(notes, fmt.Errorf("edited MCP registration preserved"))
		} else if err = m.writeRegistration(ctx, r, true); err != nil {
			notes = append(notes, err)
		} else {
			e.MCPHash = ""
		}
		s.Engines[engine] = e
	}
	if err = m.save(s); err != nil {
		notes = append(notes, err)
	}
	out := m.status(ctx, s, engine)
	if len(notes) > 0 {
		out.Message = errors.Join(notes...).Error()
	}
	return out, nil
}

// Sync refreshes only opted-in integrations; a new user gets no installation.
func (m *Manager) Sync(ctx context.Context) error {
	unlock, err := m.lock()
	if err != nil {
		return err
	}
	defer unlock()
	s, err := m.load()
	if err != nil {
		return err
	}
	var errs []error
	for _, e := range engines {
		if s.Engines[e].Enabled {
			if err = m.enable(ctx, s, e); err != nil {
				errs = append(errs, fmt.Errorf("%s: %w", e, err))
			}
		}
	}
	return errors.Join(errs...)
}
