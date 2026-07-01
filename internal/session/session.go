// Package session owns shell PTY lifetimes independent of any network
// connection. Sessions keep producing and consuming output with zero clients
// attached (TERM-05), and teardown kills the entire process tree with zero
// orphans (TERM-06).
//
// Dependency direction is enforced: this package imports neither the API nor
// the WS packages — the manager owns PTYs, never sockets.
package session

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/armon/circbuf"
	"github.com/creack/pty"

	"kamacu/internal/tmux"
)

// Status is a session lifecycle state.
type Status string

const (
	StatusRunning Status = "running"
	StatusExited  Status = "exited"
)

// Agent status tuning (D-47). Both windows are deliberate constants, not
// config: the settle window absorbs claude's final-response paint around the
// Stop hook (Pitfall 1), and the quiet threshold tolerates slow tool-output
// gaps mid-turn (the idle prompt is verified byte-silent, so this is generous).
const (
	agentSettleWindow   = 2 * time.Second
	agentQuietThreshold = 10 * time.Second
)

// Info is a JSON-ready snapshot of a session for the REST layer.
type Info struct {
	ID        string    `json:"id"`
	Label     string    `json:"label"`
	Status    Status    `json:"status"`
	ExitCode  *int      `json:"exitCode,omitempty"`
	CreatedAt time.Time `json:"createdAt"`
	TaskID    int64     `json:"taskId,omitempty"` // 0 omitted for dev sessions
	Kind      Kind      `json:"kind,omitempty"`   // "bash" or "agent"

	// Agent-only fields (kind == "agent").
	AgentStatus   string `json:"agentStatus,omitempty"`   // working | idle | waiting | exited
	StopRequested bool   `json:"stopRequested,omitempty"` // server-initiated Stop (exit 143 renders gray, not red)

	// Orphaned restored-tmux fields (TMUX-05, D-88). These are NOT produced by
	// any live Session — the sessions REST handler synthesizes them from
	// surviving tmux_sessions rows after a Kamacu restart (ID is ""). Orphaned
	// marks "this row needs a one-shot reattach spawn"; TmuxName carries the
	// persisted session name the frontend reattaches against. Both stay zero on
	// every real in-memory session, so the wire shape is unchanged for live
	// sessions and tmux stays invisible (D-77).
	Orphaned bool   `json:"orphaned,omitempty"`
	TmuxName string `json:"tmuxName,omitempty"`
}

// Session is a single shell running on its own PTY. The PTY's lifetime is
// owned here: connections attach and detach freely while the process keeps
// running; only Stop or the process exiting ends a session.
//
// Exit notification contract (consumed by the WS layer, plan 02-03): select
// on Done(); when it fires, the session has fully exited and Info().ExitCode
// is final — send the 'x' frame then. Attached queues are closed by
// markExited just before Done() fires; replay via Attach still works after
// exit so late attachers can render the final output.
type Session struct {
	id              string
	label           string
	taskID          int64        // 0 = unscoped dev session; immutable after Spawn
	kind            Kind         // KindBash or KindAgent; immutable after Spawn
	claudeSessionID string       // agent only ("" for bash); the --session-id uuid, immutable after Spawn
	tmuxName        string       // tmux-backed bash tab: the kangent-<task>-<n> session name ("" = not tmux)
	tmuxClient      *tmux.Client // socket/config for lifecycle probes; nil unless tmuxName != ""
	killer          func() error // non-nil: how Stop terminates the underlying work (assigned ONCE
	//                              in Spawn — the per-session lifecycle property; nil = default
	//                              signal path, byte-identical pre-Phase-8 behavior)
	seq       int
	createdAt time.Time

	cmd  *exec.Cmd
	ptmx *os.File

	mu           sync.Mutex
	ring         *circbuf.Buffer        // 1 MiB raw output ring (replay source)
	conns        map[string]chan []byte // per-conn output queues, buffered 256
	status       Status
	exitCode     int
	lastWinsize  pty.Winsize // last size applied to the PTY (jiggle detection)
	setsizeCalls int         // recorded pty.Setsize invocations (test observability)

	// Agent status state machine (D-47); all zero/unused for KindBash.
	lastActivity  time.Time  // effective output/input activity for working-vs-idle
	waiting       bool       // sticky until attach/stdin/Stop-hook/exit
	hooksAlive    bool       // set by SessionStart hook POST; gates the BEL fallback
	stopHookAt    time.Time  // last Stop hook — settle window anchor (Pitfall 1)
	stopRequested bool       // Stop() was called server-side (exit-143-is-gray)
	bel           belScanner // OSC-aware bare-BEL scanner, state across chunks
	detachedAlive bool       // tmux only: attach client exited but has-session said alive
	//                          (manual Ctrl+B d) — recorded for Phase 9's resume reconcile

	done      chan struct{} // closed after the exit watcher finishes
	termGrace time.Duration // D-14 grace between SIGTERM and SIGKILL
	stopOnce  sync.Once
}

// pump reads the PTY master and fans every chunk out to the ring and to all
// attached connection queues. Any read error (EIO on Linux once the child
// exits — Pitfall 5) is treated as end-of-stream: the pump returns and NEVER
// decides exit status; the exit watcher is the single authoritative exit
// event.
func (s *Session) pump() {
	buf := make([]byte, 4096)
	for {
		n, err := s.ptmx.Read(buf)
		if n > 0 {
			chunk := append([]byte(nil), buf[:n]...)
			s.mu.Lock()
			s.noteAgentOutputLocked(chunk)
			_, _ = s.ring.Write(chunk)
			for id, q := range s.conns {
				select {
				case q <- chunk:
				default:
					// Slow consumer (stalled browser): drop the connection so
					// the pump never blocks. It reconnects with fresh replay.
					delete(s.conns, id)
					close(q)
				}
			}
			s.mu.Unlock()
		}
		if err != nil {
			return
		}
	}
}

// waitExit reaps the child. cmd.Wait() is the single authoritative exit
// event: it captures the exit code (signaled exits reported as 128+signal,
// shell convention), transitions the session, and only then closes the PTY
// master and Done().
func (s *Session) waitExit() {
	_ = s.cmd.Wait()
	code := s.cmd.ProcessState.ExitCode()
	if ws, ok := s.cmd.ProcessState.Sys().(syscall.WaitStatus); ok && ws.Signaled() {
		code = 128 + int(ws.Signal())
	}
	if s.tmuxName != "" && s.tmuxClient != nil {
		// Exit vs detach (TMUX-06): the attach client exits 0 in every case —
		// inner exit, kill-session, AND detach — so has-session is the only
		// discriminator. alive ⇒ manual detach (Ctrl+B d, D-81 keeps the
		// prefix): record it for Phase 9's resume reconcile and fall through
		// to the honest exited banner (Open Q1: no auto-reattach in Phase 8).
		// A probe ERROR (tmux binary broken/hung) also falls through to
		// exited — the documented Pitfall-6 choice, never a silent state.
		if alive, err := s.tmuxClient.HasSession(context.Background(), s.tmuxName); err == nil && alive {
			s.mu.Lock()
			s.detachedAlive = true
			s.mu.Unlock()
		}
	}
	s.markExited(code)
	_ = s.ptmx.Close() // AFTER Wait — the pump's Read already returned EIO
	close(s.done)
}

// markExited transitions the session to exited and closes every attached
// queue. Queues are removed from the map under the same mutex the pump uses,
// so the pump can never send on a closed channel.
func (s *Session) markExited(code int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.status == StatusExited {
		return
	}
	s.status = StatusExited
	s.exitCode = code
	for id, q := range s.conns {
		delete(s.conns, id)
		close(q)
	}
}

// Info returns a JSON-ready snapshot. ExitCode is nil while running.
func (s *Session) Info() Info {
	s.mu.Lock()
	defer s.mu.Unlock()
	kind := s.kind
	if kind == "" {
		kind = KindBash // legacy zero value: bash
	}
	info := Info{
		ID:        s.id,
		Label:     s.label,
		Status:    s.status,
		CreatedAt: s.createdAt,
		TaskID:    s.taskID,
		Kind:      kind,
	}
	if s.kind == KindAgent {
		info.AgentStatus = s.agentStatusLocked()
	}
	info.StopRequested = s.stopRequested
	if s.status == StatusExited {
		code := s.exitCode
		info.ExitCode = &code
	}
	return info
}

// ClaudeSessionID returns the uuid passed to claude as --session-id at spawn
// (the Phase 5 --resume key). Empty for bash sessions.
func (s *Session) ClaudeSessionID() string {
	return s.claudeSessionID
}

// TmuxName returns the tmux session name for tmux-backed tabs ("" otherwise).
func (s *Session) TmuxName() string { return s.tmuxName }

// DetachedAlive reports whether the attach client exited while the tmux
// session stayed alive (manual detach). Phase 9 reads this for resume.
func (s *Session) DetachedAlive() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.detachedAlive
}

// noteAgentOutputLocked applies an output chunk's agent-status effects:
// the OSC-aware BEL fallback (hooks-dead mode only — Pitfall 2) and the
// settle-gated activity timestamp. Output NEVER clears waiting, and never
// counts as activity inside the post-Stop settle window (Pitfall 1).
// Caller holds s.mu. No-op for bash sessions.
func (s *Session) noteAgentOutputLocked(chunk []byte) {
	if s.kind != KindAgent {
		return
	}
	if bare := s.bel.scan(chunk); bare > 0 && !s.hooksAlive {
		s.waiting = true // BEL fallback ONLY while hooks are not confirmed alive
	}
	if !s.waiting && time.Since(s.stopHookAt) > agentSettleWindow {
		s.lastActivity = time.Now() // output -> working, post-settle, never from waiting
	}
}

// SetWaiting marks the agent as needing input (Notification hook:
// permission_prompt / elicitation_dialog). Sticky against output.
func (s *Session) SetWaiting() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.waiting = true
}

// SetIdle marks turn end (Stop hook): waiting clears, the status computes
// idle IMMEDIATELY (activity zeroed), and the settle window opens so the
// final response paint cannot flip idle back to working (Pitfall 1).
func (s *Session) SetIdle() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.waiting = false
	s.stopHookAt = time.Now()
	s.lastActivity = time.Time{}
}

// MarkHooksAlive records positive confirmation that the hook pipeline works
// (SessionStart hook POST). From then on the BEL fallback is ignored.
func (s *Session) MarkHooksAlive() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.hooksAlive = true
}

// ClearWaitingOnAttach clears a waiting agent to idle when a client attaches
// (D-45 — opening the task's agent tab acknowledges the prompt). Strict
// no-op for bash sessions and non-waiting agents.
func (s *Session) ClearWaitingOnAttach() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.kind == KindAgent && s.waiting {
		s.waiting = false
		s.lastActivity = time.Time{} // -> idle
	}
}

// agentStatusLocked lazily computes the D-47 status. No ticker goroutine —
// the 5s poll is the only consumer. Caller holds s.mu.
func (s *Session) agentStatusLocked() string {
	if s.status == StatusExited {
		return "exited"
	}
	if s.waiting {
		return "waiting"
	}
	if !s.lastActivity.IsZero() && time.Since(s.lastActivity) < agentQuietThreshold {
		return "working"
	}
	return "idle"
}

// Attach subscribes a connection to the session's output. The ring-buffer
// replay is queued as the channel's FIRST item inside the same critical
// section that registers the queue, so there is no gap or duplication versus
// the live stream (Pitfall 6). If the session already exited, the returned
// channel carries the replay and is then closed, letting a late attacher
// render the final output.
func (s *Session) Attach(connID string) <-chan []byte {
	s.mu.Lock()
	defer s.mu.Unlock()
	q := make(chan []byte, 256)
	q <- append([]byte(nil), s.ring.Bytes()...)
	if s.status == StatusExited {
		close(q)
		return q
	}
	s.conns[connID] = q
	return q
}

// Detach unsubscribes a connection. The PTY is untouched — that IS TERM-05.
func (s *Session) Detach(connID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if q, ok := s.conns[connID]; ok {
		delete(s.conns, connID)
		close(q)
	}
}

// WriteInput writes raw input bytes to the PTY. Errors if the session exited.
// For agents, stdin means the user typed/answered the prompt: waiting clears
// and the session is working (D-47 state machine).
func (s *Session) WriteInput(p []byte) error {
	s.mu.Lock()
	if s.status == StatusExited {
		s.mu.Unlock()
		return errors.New("session exited")
	}
	if s.kind == KindAgent {
		s.waiting = false
		s.lastActivity = time.Now()
	}
	s.mu.Unlock()
	_, err := s.ptmx.Write(p)
	return err
}

// Snapshot returns a copy of the current ring contents. This method is the
// Phase 4 seam: the WS layer must only ever consume Attach/Snapshot — never
// reach into the ring — so a headless-VT-emulator snapshot can be swapped in
// behind the same signature.
func (s *Session) Snapshot() []byte {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]byte(nil), s.ring.Bytes()...)
}

// Done returns a channel closed after the exit watcher finishes. Once it
// fires, Info().ExitCode is final.
func (s *Session) Done() <-chan struct{} {
	return s.done
}

// Resize applies a new PTY size.
//
// Contract for the WS handler (plan 02-03): pass forceRedraw=true exactly
// ONCE per attach — on the first resize frame after each attach — and false
// for every later resize. The kernel sends SIGWINCH only when the winsize
// actually CHANGES (man7 TIOCSWINSZ), so when a reattaching client's fitted
// size equals the PTY's current size, the only way to make full-screen
// programs repaint the replayed scrollback is a "jiggle": two real size
// changes (rows-1, then rows). The once-per-attach debounce matters because
// resize storms duplicate TUI output (claude-code resize-storm bug #49086).
func (s *Session) Resize(cols, rows uint16, forceRedraw bool) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.status == StatusExited {
		return errors.New("session exited")
	}
	want := pty.Winsize{Rows: rows, Cols: cols}
	if want != s.lastWinsize {
		// A real change delivers SIGWINCH by itself.
		return s.setsizeLocked(want)
	}
	if !forceRedraw {
		return nil
	}
	// Same size + forceRedraw: jiggle through rows-1 so two real changes
	// fire SIGWINCH and the foreground program performs a full repaint.
	if err := s.setsizeLocked(pty.Winsize{Rows: rows - 1, Cols: cols}); err != nil {
		return err
	}
	return s.setsizeLocked(want)
}

// setsizeLocked applies ws to the PTY and records it. Caller holds s.mu.
func (s *Session) setsizeLocked(ws pty.Winsize) error {
	s.setsizeCalls++
	if err := pty.Setsize(s.ptmx, &ws); err != nil {
		return err
	}
	s.lastWinsize = ws
	return nil
}

// Stop tears the session down per D-14: SIGTERM to every process group in
// the shell's session, a grace window (default 5s), then SIGKILL to anything
// remaining. Idempotent — concurrent and repeated calls are safe — and
// blocks until the exit watcher has finished.
//
// The whole SESSION is signaled, not just the leader's process group:
// interactive bash enables job control, so children like `sleep 300 &` live
// in their own process groups and would survive a kill(-leaderPGID) (TERM-06
// would silently break). Known limit (shared with every terminal
// multiplexer): a child that itself calls setsid() escapes the session and
// cannot be caught.
func (s *Session) Stop() {
	select {
	case <-s.done:
		return // already exited — nothing to do
	default:
	}
	s.stopOnce.Do(func() {
		// Server-initiated stop: the resulting exit (143 after SIGTERM) was
		// asked for — the UI renders it gray, never red (research OQ4).
		s.mu.Lock()
		s.stopRequested = true
		s.mu.Unlock()
		if s.killer != nil {
			// Per-session stop strategy (assigned at Spawn): for tmux tabs this is
			// kill-session, which ends the inner shell, the tmux session, and (when
			// last) the server atomically — signals can only ever reach the attach
			// client because the tmux server daemonizes out of the /proc session
			// sweep's reach (the proven "Stop doesn't stop" trap).
			if err := s.killer(); err == nil {
				select {
				case <-s.done: // server drops the client → attach client exits → waitExit fires
					return
				case <-time.After(s.termGrace):
				}
			}
			// tmux CLI failed or the client didn't die: fall through to signals —
			// at minimum the attach client dies and the tab shows exited.
		}
		s.signalSession(syscall.SIGTERM)
		select {
		case <-s.done:
		case <-time.After(s.termGrace):
			s.signalSession(syscall.SIGKILL)
		}
	})
	<-s.done
	// Defense in depth: the leader is reaped, but a group forked at the exact
	// moment of the kill sweep could linger. Sweep until the session is empty
	// (bounded; normally zero iterations).
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		groups := sessionPGIDs(s.cmd.Process.Pid)
		if len(groups) == 0 {
			return
		}
		for _, pgid := range groups {
			_ = syscall.Kill(-pgid, syscall.SIGKILL)
		}
		time.Sleep(20 * time.Millisecond)
	}
}

// signalSession sends sig to every process group in the child's session,
// always including the leader's own group (PGID == PID, since pty.Start made
// the child a session leader). ESRCH is ignored.
func (s *Session) signalSession(sig syscall.Signal) {
	pid := s.cmd.Process.Pid
	leaderSignaled := false
	for _, pgid := range sessionPGIDs(pid) {
		if pgid == pid {
			leaderSignaled = true
		}
		_ = syscall.Kill(-pgid, sig)
	}
	if !leaderSignaled {
		_ = syscall.Kill(-pid, sig)
	}
}

// sessionPGIDs returns the distinct process-group IDs of every live process
// whose session ID equals sid, scanned from /proc.
//
// Why this exists: the spawned shell is interactive (its stdio is a PTY), so
// bash enables job control and every pipeline — foreground or background —
// runs in its OWN process group within the shell's session. Signaling only
// -leaderPGID would orphan background children like `sleep 300 &`. Teardown
// therefore signals every group in the session.
func sessionPGIDs(sid int) []int {
	entries, err := os.ReadDir("/proc")
	if err != nil {
		return nil
	}
	seen := make(map[int]struct{})
	for _, e := range entries {
		pid, err := strconv.Atoi(e.Name())
		if err != nil || pid <= 0 {
			continue
		}
		raw, err := os.ReadFile("/proc/" + e.Name() + "/stat")
		if err != nil {
			continue // process vanished mid-scan
		}
		// Format: pid (comm) state ppid pgrp session ... — comm may contain
		// spaces or parens, so parse from the LAST ')'.
		stat := string(raw)
		i := strings.LastIndexByte(stat, ')')
		if i < 0 {
			continue
		}
		fields := strings.Fields(stat[i+1:])
		if len(fields) < 4 {
			continue
		}
		// fields (after the comm field): [0]=state [1]=ppid [2]=pgrp [3]=session
		pgrp, err1 := strconv.Atoi(fields[2])
		sess, err2 := strconv.Atoi(fields[3])
		if err1 != nil || err2 != nil {
			continue
		}
		if sess == sid {
			seen[pgrp] = struct{}{}
		}
	}
	out := make([]int, 0, len(seen))
	for pgid := range seen {
		out = append(out, pgid)
	}
	return out
}
