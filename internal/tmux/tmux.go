// Package tmux is a leaf package (stdlib-only imports) wrapping the three
// production tmux verbs Kamacu uses: new-session -A (attach-or-create),
// has-session (liveness probe), and kill-session (× kill). Every invocation
// carries the dedicated socket (-L) and the Kamacu-generated config (-f) so
// the user's default tmux server and config are never touched.
//
// All exit-code semantics below were verified empirically against host
// tmux 3.4 (08-RESEARCH.md, Empirical Findings 1-14).
package tmux

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"strings"
	"time"
)

// DefaultSocket is the dedicated tmux socket name (-L) for all Kamacu
// sessions — never the user's default server (settled roadmap decision).
const DefaultSocket = "kamacu"

// Config is the Kamacu-managed tmux configuration: status bar off (D-79),
// mouse on so wheel-scroll reaches tmux's real history (D-80), and a deep
// history-limit because tmux history IS the durable scrollback (default 2000
// is too shallow). Applied via -f on every invocation; tmux reads it exactly
// once, when an invocation starts the server.
const Config = `# kamacu-managed tmux config (regenerated at startup -- do not edit)
set -g status off
set -g mouse on
set -g history-limit 50000
`

// execTimeout bounds every tmux invocation: a hung tmux binary must never
// block Stop() or a cleanup HTTP request (Pitfall 5). 5s matches the D-14
// grace scale.
const execTimeout = 5 * time.Second

// WriteConfig writes Config to path (0o644), overwriting any previous file.
func WriteConfig(path string) error {
	return os.WriteFile(path, []byte(Config), 0o644)
}

// Client runs tmux verbs against one socket + config file.
type Client struct {
	Socket   string // -L socket name; DefaultSocket in production, per-test sockets in tests
	ConfPath string // -f config path; the generated file in production, "/dev/null" in tests
}

// BaseArgs prefixes EVERY tmux invocation: dedicated socket + Kamacu config.
// -f is read exactly once, by whichever invocation starts the server, so
// passing it on every call makes config application deterministic regardless
// of which call wins the server-start race.
func (c Client) BaseArgs() []string {
	return []string{"-L", c.Socket, "-f", c.ConfPath}
}

// NewSessionArgs builds the attach-or-create argv: exact name, worktree cwd.
// Deliberately NO -d: the caller (Manager.Spawn) runs this under creack/pty
// as the foreground attach client. -c applies only at creation — names are
// fresh per tab, so that is correct.
func (c Client) NewSessionArgs(name, dir string) []string {
	return append(c.BaseArgs(), "new-session", "-A", "-s", name, "-c", dir)
}

// run executes tmux with args under a bounded context, arg-array exec only
// (never via a shell, per project posture).
func (c Client) run(ctx context.Context, args ...string) error {
	ctx, cancel := context.WithTimeout(ctx, execTimeout)
	defer cancel()
	return exec.CommandContext(ctx, "tmux", args...).Run()
}

// HasSession probes liveness of the named session.
//
// Session targets use "="+name because WITHOUT "=", tmux 3.4 PREFIX-MATCHES
// session names: has-session -t kamacu-1 matches kamacu-1-10 (exit 0!) —
// Empirical Finding 2. Exact match is mandatory on every session target.
//
// Returns:
//   - (true, nil): session alive (exit 0)
//   - (false, nil): session dead — exit 1 covers BOTH "can't find session"
//     AND "no server running" (identical exit codes, verified)
//   - (false, err): tmux binary missing/broken or context timeout. This case
//     is load-bearing: callers must never misread "tmux missing/hung" as
//     "session dead" (Pitfall 6).
func (c Client) HasSession(ctx context.Context, name string) (bool, error) {
	err := c.run(ctx, append(c.BaseArgs(), "has-session", "-t", "="+name)...)
	if err == nil {
		return true, nil
	}
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) && exitErr.ExitCode() == 1 {
		return false, nil
	}
	return false, err
}

// KillSession kills the named session. Idempotent: exit 1 (already dead /
// no server running) is treated as success, so killing an already-dead
// session or a dead server returns nil. Uses "="+name exact match — killing
// kamacu-1-1 must never touch kamacu-1-10 (Finding 2).
func (c Client) KillSession(ctx context.Context, name string) error {
	err := c.run(ctx, append(c.BaseArgs(), "kill-session", "-t", "="+name)...)
	if err == nil {
		return nil
	}
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) && exitErr.ExitCode() == 1 {
		return nil // already dead / no server — idempotent success
	}
	return err
}

// KillServer kills the whole tmux server on this socket. Same idempotency
// as KillSession: exit 1 (no server running) returns nil. Used by test
// cleanup now; Phase 9's README escape hatch later.
func (c Client) KillServer(ctx context.Context) error {
	err := c.run(ctx, append(c.BaseArgs(), "kill-server")...)
	if err == nil {
		return nil
	}
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) && exitErr.ExitCode() == 1 {
		return nil
	}
	return err
}

// ListSessions enumerates live session names on this socket (Phase 9 orphan
// sweep input, D-94). Exit 1 (no server running) is the empty case — nil,
// nil — never an error.
//
// Does NOT use the run helper (which returns only an error): listing needs
// stdout. The name format emits one clean name per line (Empirical Finding
// 14). Output is split on "\n", each line trimmed, empties dropped.
func (c Client) ListSessions(ctx context.Context) ([]string, error) {
	ctx, cancel := context.WithTimeout(ctx, execTimeout)
	defer cancel()
	args := append(c.BaseArgs(), "list-sessions", "-F", "#{session_name}")
	out, err := exec.CommandContext(ctx, "tmux", args...).Output()
	if err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) && exitErr.ExitCode() == 1 {
			return nil, nil // no server running -> zero sessions, never an error
		}
		return nil, err
	}
	var names []string
	for _, line := range strings.Split(string(out), "\n") {
		if name := strings.TrimSpace(line); name != "" {
			names = append(names, name)
		}
	}
	return names, nil
}
