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
	mu           sync.Mutex
	sessions     map[string]*Session
	counter      int           // monotonic global "bash #N" label counter; never reused
	taskCounters map[int64]int // per-task "Bash N" label counters; never reused or reset
	seq          int           // global spawn order, List sort tiebreak
}

// NewManager returns an empty Manager.
func NewManager() *Manager {
	return &Manager{
		sessions:     make(map[string]*Session),
		taskCounters: make(map[int64]int),
	}
}

// SpawnOpts configures a new session.
type SpawnOpts struct {
	Cwd    string // "" -> user home (preserves Phase 2 /terminal dev behavior)
	TaskID int64  // 0 -> unscoped dev session, label "bash #N" (global counter)
}

// Spawn starts a new interactive shell ($SHELL, fallback /bin/bash) on its
// own PTY, cwd opts.Cwd (the user's home directory when empty), with an
// explicit environment (never inherited blindly). pty.StartWithSize starts
// the child in a new session with the PTY as controlling terminal, so the
// child is session leader and PGID == child PID — no SysProcAttr needed.
//
// A non-empty Cwd is validated BEFORE any PTY allocation: a deleted worktree
// must produce the clean "couldn't start a session" path, not a confusing
// shell error, and must never register a session.
func (m *Manager) Spawn(opts SpawnOpts) (*Session, error) {
	shell := os.Getenv("SHELL")
	if shell == "" {
		shell = "/bin/bash"
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("resolve home dir: %w", err)
	}
	dir := home
	if opts.Cwd != "" {
		fi, err := os.Stat(opts.Cwd)
		if err != nil || !fi.IsDir() {
			return nil, fmt.Errorf("working directory does not exist: %s", opts.Cwd)
		}
		dir = opts.Cwd
	}

	cmd := exec.Command(shell)
	cmd.Dir = dir
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
	m.seq++
	seq := m.seq
	var label string
	if opts.TaskID == 0 {
		m.counter++
		label = fmt.Sprintf("bash #%d", m.counter)
	} else {
		m.taskCounters[opts.TaskID]++
		label = fmt.Sprintf("Bash %d", m.taskCounters[opts.TaskID])
	}
	m.mu.Unlock()

	s := &Session{
		id:        uuid.NewString(),
		label:     label,
		taskID:    opts.TaskID,
		seq:       seq,
		createdAt: time.Now(),
		cmd:       cmd,
		ptmx:      ptmx,
		ring:      ring,
		conns:       make(map[string]chan []byte),
		status:      StatusRunning,
		lastWinsize: initial,
		done:        make(chan struct{}),
		termGrace:   5 * time.Second, // D-14
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
	return m.listWhere(func(*Session) bool { return true })
}

// ListByTask returns Info snapshots for sessions with the given TaskID,
// newest first (same ordering contract as List). taskID 0 selects unscoped
// dev sessions.
func (m *Manager) ListByTask(taskID int64) []Info {
	return m.listWhere(func(s *Session) bool { return s.taskID == taskID })
}

// listWhere snapshots sessions matching keep, newest first. The comparator
// lives only here — List and ListByTask share it.
func (m *Manager) listWhere(keep func(*Session) bool) []Info {
	m.mu.Lock()
	sessions := make([]*Session, 0, len(m.sessions))
	for _, s := range m.sessions {
		if keep(s) {
			sessions = append(sessions, s)
		}
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

// StopAllForTask stops every RUNNING session of the task concurrently and
// blocks until all have fully exited. Each Stop already blocks through the
// SIGTERM grace (D-14) and is idempotent, so concurrent and repeated calls
// are safe; stopping concurrently collapses the worst case to a single grace
// window — required because the DELETE /worktree handler calls this inline
// in the request (D-32 cleanup gate).
func (m *Manager) StopAllForTask(taskID int64) {
	m.mu.Lock()
	targets := make([]*Session, 0, len(m.sessions))
	for _, s := range m.sessions {
		if s.taskID == taskID && s.Info().Status == StatusRunning {
			targets = append(targets, s)
		}
	}
	m.mu.Unlock()

	var wg sync.WaitGroup
	for _, s := range targets {
		wg.Add(1)
		go func(s *Session) {
			defer wg.Done()
			s.Stop()
		}(s)
	}
	wg.Wait()
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
