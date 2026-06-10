package session

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"sort"
	"sync"
	"time"

	"github.com/armon/circbuf"
	"github.com/creack/pty"
	"github.com/google/uuid"
)

// ErrStillRunning is returned by Remove for sessions that have not exited.
var ErrStillRunning = errors.New("session still running")

// ErrNotFound is returned by Remove for unknown session IDs.
var ErrNotFound = errors.New("session not found")

// Manager owns every live Session. Lifecycle transitions (spawn / exit /
// remove) each live in a single method so Phase 4 can add DB writes inside
// them without restructuring.
type Manager struct {
	mu       sync.Mutex
	sessions map[string]*Session
	counter  int // monotonic "bash #N" label counter; never reused
}

// NewManager returns an empty Manager.
func NewManager() *Manager {
	return &Manager{sessions: make(map[string]*Session)}
}

// Spawn starts a new interactive shell ($SHELL, fallback /bin/bash) on its
// own PTY, cwd the user's home directory, with an explicit environment
// (never inherited blindly). pty.StartWithSize starts the child in a new
// session with the PTY as controlling terminal, so the child is session
// leader and PGID == child PID — no SysProcAttr needed.
func (m *Manager) Spawn() (*Session, error) {
	shell := os.Getenv("SHELL")
	if shell == "" {
		shell = "/bin/bash"
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("resolve home dir: %w", err)
	}

	cmd := exec.Command(shell)
	cmd.Dir = home
	cmd.Env = []string{
		"TERM=xterm-256color",
		"COLORTERM=truecolor",
		"HOME=" + home,
		"PATH=" + os.Getenv("PATH"),
		"LANG=" + os.Getenv("LANG"),
		"USER=" + os.Getenv("USER"),
		"SHELL=" + shell,
	}

	initial := pty.Winsize{Rows: 24, Cols: 80}
	ptmx, err := pty.StartWithSize(cmd, &initial)
	if err != nil {
		return nil, fmt.Errorf("start pty: %w", err)
	}

	ring, err := circbuf.NewBuffer(1 << 20) // 1 MiB replay ring
	if err != nil {
		_ = ptmx.Close()
		return nil, fmt.Errorf("create ring: %w", err)
	}

	m.mu.Lock()
	m.counter++
	n := m.counter
	m.mu.Unlock()

	s := &Session{
		id:        uuid.NewString(),
		label:     fmt.Sprintf("bash #%d", n),
		seq:       n,
		createdAt: time.Now(),
		cmd:       cmd,
		ptmx:      ptmx,
		ring:      ring,
		conns:     make(map[string]chan []byte),
		status:    StatusRunning,
		done:      make(chan struct{}),
		termGrace: 5 * time.Second, // D-14
	}

	go s.pump()
	go s.waitExit()

	m.mu.Lock()
	m.sessions[s.id] = s
	m.mu.Unlock()
	return s, nil
}

// Get returns the session with the given ID.
func (m *Manager) Get(id string) (*Session, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	s, ok := m.sessions[id]
	return s, ok
}

// List returns Info snapshots for every session, newest first (CreatedAt
// descending, spawn order as tiebreak).
func (m *Manager) List() []Info {
	m.mu.Lock()
	sessions := make([]*Session, 0, len(m.sessions))
	for _, s := range m.sessions {
		sessions = append(sessions, s)
	}
	m.mu.Unlock()

	sort.Slice(sessions, func(i, j int) bool {
		if sessions[i].createdAt.Equal(sessions[j].createdAt) {
			return sessions[i].seq > sessions[j].seq
		}
		return sessions[i].createdAt.After(sessions[j].createdAt)
	})
	out := make([]Info, 0, len(sessions))
	for _, s := range sessions {
		out = append(out, s.Info())
	}
	return out
}

// Remove deletes an exited session (freeing its ring). Running sessions are
// rejected with ErrStillRunning; unknown IDs with ErrNotFound.
func (m *Manager) Remove(id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	s, ok := m.sessions[id]
	if !ok {
		return ErrNotFound
	}
	if s.Info().Status == StatusRunning {
		return ErrStillRunning
	}
	delete(m.sessions, id)
	return nil
}
