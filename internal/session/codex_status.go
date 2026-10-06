package session

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/hinshun/vt10x"
)

// CodexHook preserves turn and invocation identity. PermissionRequest can fire
// before automatic approval, so it is evidence of a possible prompt, not waiting.
type CodexHook struct {
	Event     string
	SessionID string
	Source    string
	TurnID    string
	ToolUseID string
	ToolName  string
	ToolInput json.RawMessage
}

type codexStatus struct {
	terminal     vt10x.Terminal
	utf8Tail     []byte
	active       bool
	lifecycle    bool
	turn         string
	sessionID    string
	completed    []string
	pending      map[string]bool
	screenStatus string
	ignoreScreen string
	endedAt      time.Time
}

func (s *Session) codexStateLocked() *codexStatus {
	if s.codexState == nil {
		s.codexState = &codexStatus{}
	}
	return s.codexState
}

func (c *codexStatus) ignoreCurrentScreen() {
	c.screenStatus = ""
	if c.terminal != nil {
		c.ignoreScreen = c.terminal.String()
	}
}

func codexToolKey(h CodexHook) string {
	// PermissionRequest does not currently include tool_use_id. Match its input
	// to PostToolUse rather than clearing another parallel tool's approval.
	if len(h.ToolInput) > 0 {
		var value any
		if json.Unmarshal(h.ToolInput, &value) == nil {
			// Codex adds an approval description only to PermissionRequest.
			// It is telemetry, not part of the Bash/apply_patch invocation identity.
			if h.ToolName == "Bash" || h.ToolName == "apply_patch" {
				if object, ok := value.(map[string]any); ok {
					delete(object, "description")
				}
			}
			canonical, _ := json.Marshal(value)
			return fmt.Sprintf("%s:%x", h.ToolName, sha256.Sum256(canonical))
		}
	}
	if h.ToolUseID != "" {
		return h.ToolUseID
	}
	return h.ToolName
}

// ApplyCodexHook is atomic with PTY output and input. Completed turns cannot
// be resurrected by delayed hook deliveries. Other engines never use it.
func (s *Session) ApplyCodexHook(h CodexHook) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.engine != "codex" || s.status == StatusExited {
		return
	}
	switch h.Event {
	case "SessionStart", "UserPromptSubmit", "PreToolUse", "PermissionRequest", "PostToolUse", "Stop", "Interrupt", "SessionEnd":
	default:
		return
	}
	c := s.codexStateLocked()
	if h.SessionID != "" {
		if c.sessionID != "" && c.sessionID != h.SessionID {
			// Child threads inherit Kamacu's environment. Their hooks must not end
			// or replace the root turn. An explicit root /resume or /clear may switch.
			if h.Event != "SessionStart" || (h.Source != "resume" && h.Source != "clear") {
				return
			}
			c.turn = ""
			c.completed = nil
			c.pending = nil
			c.active, c.lifecycle = false, true
			c.ignoreCurrentScreen()
			s.waiting = false
		}
		c.sessionID = h.SessionID
	}
	s.hooksAlive = true
	if h.Event == "SessionStart" {
		return
	}
	for _, id := range c.completed {
		if h.TurnID != "" && h.TurnID == id {
			return
		}
	}
	if c.turn != "" && h.TurnID != "" && c.turn != h.TurnID && h.Event != "UserPromptSubmit" {
		return
	}
	if h.TurnID != "" {
		c.turn = h.TurnID
	}
	switch h.Event {
	case "UserPromptSubmit":
		c.lifecycle, c.active = true, true
		c.pending = make(map[string]bool)
		c.endedAt = time.Time{}
		c.ignoreCurrentScreen()
		s.waiting = false
	case "PreToolUse", "PostToolUse":
		c.lifecycle, c.active = true, true
		c.endedAt = time.Time{}
		if h.Event == "PostToolUse" {
			delete(c.pending, codexToolKey(h))
		}
		// A different tool can run while an approval is pending. Only a matching
		// result may invalidate the visible prompt; PreToolUse alone cannot.
		if len(c.pending) == 0 {
			c.ignoreCurrentScreen()
			s.waiting = false
		}
	case "PermissionRequest":
		c.lifecycle, c.active = true, true
		if c.pending == nil {
			c.pending = make(map[string]bool)
		}
		c.pending[codexToolKey(h)] = true
	case "Stop", "Interrupt", "SessionEnd":
		c.lifecycle, c.active = true, false
		c.pending = nil
		c.endedAt = time.Now()
		c.ignoreCurrentScreen()
		s.waiting = false
		if c.turn != "" {
			c.completed = append(c.completed, c.turn)
			if len(c.completed) > 32 {
				c.completed = c.completed[1:]
			}
		}
		c.turn = ""
	}
}

var codexChoice = regexp.MustCompile(`^[›❯>]\s*[1-9][.]\s+`)
var codexRunning = regexp.MustCompile(`(?i)^(?:[•◦⠁-⣿]\s*)?(?:working|thinking|processing|generating|running command|starting mcp servers)\b.*(?:esc|ctrl\+c) to interrupt`)
var codexInterrupt = regexp.MustCompile(`(?i)^(?:•\s*)?(?:esc|ctrl\+c) to interrupt\s*$`)

// Read only the visible screen, bottom-up: a new running footer below a
// historical menu wins. Require menu chrome, not prose mentioning approval.
func codexScreenStatus(screen string) string {
	lines := strings.Split(strings.ToLower(screen), "\n")
	hint := false
	parked := false
	for i := len(lines) - 1; i >= 0; i-- {
		line := strings.TrimSpace(lines[i])
		if codexRunning.MatchString(line) || codexInterrupt.MatchString(line) {
			return "working"
		}
		if !parked && (strings.Contains(line, "enter to select") || strings.Contains(line, "enter to confirm") || strings.Contains(line, "enter to submit answer")) {
			hint = true
		}
		if (strings.HasPrefix(line, "›") || strings.HasPrefix(line, "❯")) && !codexChoice.MatchString(line) {
			parked = true
		}
		if hint && !parked && codexChoice.MatchString(line) {
			return "waiting"
		}
		if strings.HasPrefix(line, "■ conversation interrupted") || strings.HasPrefix(strings.TrimLeft(line, "─ "), "worked for ") || line == "<< code review finished >>" {
			return "idle"
		}
	}
	if parked && !hint {
		return "idle"
	}
	return ""
}

func codexObserverSize(cols, rows int) (int, int) {
	if cols <= 0 {
		cols = 80
	}
	if rows <= 0 {
		rows = 24
	}
	// Bound observer allocation even for a malformed or enormous resize.
	return min(cols, 500), min(rows, 200)
}

func (s *Session) noteCodexOutputLocked(chunk []byte) {
	c := s.codexStateLocked()
	if c.terminal == nil {
		cols, rows := codexObserverSize(int(s.lastWinsize.Cols), int(s.lastWinsize.Rows))
		// This observer must never answer terminal queries; xterm owns that path.
		c.terminal = vt10x.New(vt10x.WithSize(cols, rows), vt10x.WithWriter(io.Discard))
	}
	// vt10x.Write retains escape state, but not partial UTF-8 runes.
	data := append(c.utf8Tail, chunk...)
	end := 0
	for end < len(data) {
		if !utf8.FullRune(data[end:]) {
			break
		}
		_, size := utf8.DecodeRune(data[end:])
		end += size
	}
	_, _ = c.terminal.Write(data[:end])
	c.utf8Tail = append([]byte(nil), data[end:]...)
	screen := c.terminal.String()
	if screen == c.ignoreScreen {
		return
	}
	evidence := codexScreenStatus(screen)
	if !c.endedAt.IsZero() && time.Since(c.endedAt) < agentSettleWindow && evidence == "working" {
		return
	}
	c.screenStatus = evidence
	switch evidence {
	case "working":
		c.active = true
		s.waiting = false
	case "waiting":
		s.waiting = true
	case "idle":
		c.lifecycle = true
		c.active = false
		s.waiting = false
	}
}

func (s *Session) codexStatusLocked() string {
	c := s.codexStateLocked()
	if c.screenStatus == "waiting" || s.waiting {
		return "waiting"
	}
	if c.active {
		return "working"
	}
	if c.lifecycle || c.screenStatus == "idle" {
		return "idle"
	}
	// Before the first lifecycle event, keep the existing output estimate.
	if !s.lastActivity.IsZero() && time.Since(s.lastActivity) < agentQuietThreshold {
		return "working"
	}
	return "idle"
}
