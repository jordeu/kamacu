// Package session owns shell PTY lifetimes independent of any network
// connection. Sessions keep producing and consuming output with zero clients
// attached (TERM-05), and teardown kills the entire process tree with zero
// orphans (TERM-06).
//
// Dependency direction is enforced: this package imports neither the API nor
// the WS packages — the manager owns PTYs, never sockets.
package session

import (
	"errors"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/armon/circbuf"
)

// Status is a session lifecycle state.
type Status string

const (
	StatusRunning Status = "running"
	StatusExited  Status = "exited"
)

// Info is a JSON-ready snapshot of a session for the REST layer.
type Info struct {
	ID        string    `json:"id"`
	Label     string    `json:"label"`
	Status    Status    `json:"status"`
	ExitCode  *int      `json:"exitCode,omitempty"`
	CreatedAt time.Time `json:"createdAt"`
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
	id        string
	label     string
	seq       int
	createdAt time.Time

	cmd  *exec.Cmd
	ptmx *os.File

	mu       sync.Mutex
	ring     *circbuf.Buffer        // 1 MiB raw output ring (replay source)
	conns    map[string]chan []byte // per-conn output queues, buffered 256
	status   Status
	exitCode int

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
	info := Info{
		ID:        s.id,
		Label:     s.label,
		Status:    s.status,
		CreatedAt: s.createdAt,
	}
	if s.status == StatusExited {
		code := s.exitCode
		info.ExitCode = &code
	}
	return info
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
func (s *Session) WriteInput(p []byte) error {
	s.mu.Lock()
	if s.status == StatusExited {
		s.mu.Unlock()
		return errors.New("session exited")
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
