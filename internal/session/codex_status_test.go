package session

import (
	"encoding/json"
	"os"
	"testing"
	"time"
)

func codexStatusSession() *Session {
	return &Session{kind: KindAgent, engine: "codex", status: StatusRunning}
}
func codexPaint(s *Session, screen string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.noteAgentOutputLocked([]byte("\x1b[2J\x1b[H" + screen))
}
func expectCodexStatus(t *testing.T, s *Session, want string) {
	t.Helper()
	if got := s.Info().AgentStatus; got != want {
		t.Fatalf("status %q, want %q", got, want)
	}
}

const approvalScreen = "Would you like to run the following command?\r\n\r\n› 1. Yes, proceed (y)\r\n  2. No (esc)\r\n\r\nPress enter to select or esc to cancel"

func TestCodexAutomaticApprovalAndQuietTurn(t *testing.T) {
	s := codexStatusSession()
	s.ApplyCodexHook(CodexHook{Event: "UserPromptSubmit", TurnID: "t1"})
	s.ApplyCodexHook(CodexHook{Event: "PermissionRequest", TurnID: "t1", ToolName: "Bash"})
	expectCodexStatus(t, s, "working")
	s.ApplyCodexHook(CodexHook{Event: "PostToolUse", TurnID: "t1", ToolName: "Bash"})
	s.lastActivity = time.Now().Add(-time.Hour)
	expectCodexStatus(t, s, "working")
	s.ApplyCodexHook(CodexHook{Event: "Stop", TurnID: "t1"})
	// Delayed events for a completed turn cannot reactivate it.
	for _, event := range []string{"PermissionRequest", "PostToolUse", "PreToolUse"} {
		s.ApplyCodexHook(CodexHook{Event: event, TurnID: "t1"})
		expectCodexStatus(t, s, "idle")
	}
}

func TestCodexVisiblePromptAndParallelTools(t *testing.T) {
	s := codexStatusSession()
	a := json.RawMessage(`{"command":"protected"}`)
	b := json.RawMessage(`{"command":"other"}`)
	s.ApplyCodexHook(CodexHook{Event: "PermissionRequest", TurnID: "t1", ToolName: "Bash", ToolInput: a})
	codexPaint(s, approvalScreen)
	expectCodexStatus(t, s, "waiting")
	s.ClearWaitingOnAttach()
	s.ApplyCodexHook(CodexHook{Event: "PreToolUse", TurnID: "t1", ToolName: "Bash", ToolInput: b})
	s.ApplyCodexHook(CodexHook{Event: "PostToolUse", TurnID: "t1", ToolName: "Bash", ToolInput: b})
	expectCodexStatus(t, s, "waiting")
	s.ApplyCodexHook(CodexHook{Event: "PostToolUse", TurnID: "t1", ToolName: "Bash", ToolInput: a})
	expectCodexStatus(t, s, "working")
	// Identical redraw of the resolved dialog must not reassert waiting.
	codexPaint(s, approvalScreen)
	expectCodexStatus(t, s, "working")
	codexPaint(s, "• Working (esc to interrupt)\r\n› ")
	expectCodexStatus(t, s, "working")
}

func TestCodexScreenReconciliationWithoutHooks(t *testing.T) {
	s := codexStatusSession()
	codexPaint(s, approvalScreen)
	expectCodexStatus(t, s, "waiting")
	codexPaint(s, "• Working (esc to interrupt)\r\n› ")
	s.lastActivity = time.Now().Add(-time.Hour)
	expectCodexStatus(t, s, "working")
	codexPaint(s, "■ Conversation interrupted - tell the model what to do differently\r\n› ")
	expectCodexStatus(t, s, "idle")
	codexPaint(s, "──── Worked for 1m 3s ────\r\nDone.\r\n› ")
	expectCodexStatus(t, s, "idle")
	// Completion bells alone are never approval evidence.
	codexPaint(s, "\aDone")
	expectCodexStatus(t, s, "idle")
}

func TestCodexScreenPromptShapes(t *testing.T) {
	cases := []struct{ screen, want string }{
		{approvalScreen, "waiting"},
		{"Question 1/1 (1 unanswered)\n› 1. Yes\nEnter to submit answer", "waiting"},
		{approvalScreen + "\n• Working (esc to interrupt)", "working"},
		{approvalScreen + "\n› Ask Codex to do anything", "idle"},
		{"◦ Working (2s • esc to interrupt)\n› Ask Codex to do anything", "working"},
		{"Documentation: enter to select or esc to cancel\nAn approval may be required.", ""},
		{"Done.\nRunning commands is described above.", ""},
	}
	for _, tc := range cases {
		if got := codexScreenStatus(tc.screen); got != tc.want {
			t.Errorf("%q: got %q, want %q", tc.screen, got, tc.want)
		}
	}
}

func TestCodexChunkedScreenAndResize(t *testing.T) {
	s := codexStatusSession()
	// Feed one byte at a time, splitting both escape sequences and Unicode.
	for _, b := range []byte("\x1b[2J\x1b[H" + approvalScreen) {
		s.noteAgentOutputLocked([]byte{b})
	}
	expectCodexStatus(t, s, "waiting")
	s.codexState.terminal.Resize(100, 30)
	codexPaint(s, "• Working (esc to interrupt)")
	expectCodexStatus(t, s, "working")
}

func TestCodexTerminalReportsAndLatePermission(t *testing.T) {
	s := codexStatusSession()
	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	defer reader.Close()
	defer writer.Close()
	s.ptmx = writer
	s.ApplyCodexHook(CodexHook{Event: "PermissionRequest", TurnID: "t1"})
	codexPaint(s, approvalScreen)
	for _, report := range []string{"\x1b[I", "\x1b[24;80R", "\x1b[?1;2c"} {
		if err := s.WriteInput([]byte(report)); err != nil {
			t.Fatal(err)
		}
		expectCodexStatus(t, s, "waiting")
	}
	if err := s.WriteInput([]byte("y")); err != nil {
		t.Fatal(err)
	}
	s.ApplyCodexHook(CodexHook{Event: "PermissionRequest", TurnID: "t1"})
	expectCodexStatus(t, s, "working")
	codexPaint(s, approvalScreen)
	expectCodexStatus(t, s, "working")
}

func TestCodexCapturedTerminalLifecycle(t *testing.T) {
	data, err := os.ReadFile("testdata/codex/0.160.1-approval.ansi")
	if err != nil {
		t.Fatal(err)
	}
	metadata, err := os.ReadFile("testdata/codex/0.160.1-approval.json")
	if err != nil {
		t.Fatal(err)
	}
	var fixture struct {
		Cols, Rows uint16
		Stages     []struct {
			End    int
			Status string
		}
	}
	if err := json.Unmarshal(metadata, &fixture); err != nil {
		t.Fatal(err)
	}
	s := codexStatusSession()
	s.lastWinsize.Cols = fixture.Cols
	s.lastWinsize.Rows = fixture.Rows
	offset := 0
	for _, stage := range fixture.Stages {
		for offset < stage.End {
			end := min(offset+7, stage.End)
			s.noteAgentOutputLocked(data[offset:end])
			offset = end
		}
		expectCodexStatus(t, s, stage.Status)
	}
}

func TestCodexPermissionDescriptionMatchesToolResult(t *testing.T) {
	s := codexStatusSession()
	s.ApplyCodexHook(CodexHook{Event: "PermissionRequest", TurnID: "t1", ToolName: "Bash", ToolInput: json.RawMessage(`{"command":"echo status-probe","description":"Status detection test"}`)})
	codexPaint(s, approvalScreen)
	expectCodexStatus(t, s, "waiting")
	s.ApplyCodexHook(CodexHook{Event: "PostToolUse", TurnID: "t1", ToolName: "Bash", ToolUseID: "call_test", ToolInput: json.RawMessage(`{"command":"echo status-probe"}`)})
	expectCodexStatus(t, s, "working")
}

func TestCodexCancelledDialogWithoutStopHook(t *testing.T) {
	s := codexStatusSession()
	s.ApplyCodexHook(CodexHook{Event: "PermissionRequest", TurnID: "t1"})
	codexPaint(s, approvalScreen)
	expectCodexStatus(t, s, "waiting")
	codexPaint(s, "› Ask Codex to do anything\r\n  gpt-5.4 default · /tmp")
	expectCodexStatus(t, s, "idle")
}

func TestCodexChildHooksCannotChangeRootStatus(t *testing.T) {
	s := codexStatusSession()
	s.ApplyCodexHook(CodexHook{Event: "SessionStart", SessionID: "root", Source: "startup"})
	s.ApplyCodexHook(CodexHook{Event: "UserPromptSubmit", SessionID: "root", TurnID: "root-turn"})
	for _, event := range []string{"SessionStart", "UserPromptSubmit", "PermissionRequest", "Stop", "SessionEnd"} {
		s.ApplyCodexHook(CodexHook{Event: event, SessionID: "child", TurnID: "child-turn", Source: "startup"})
		expectCodexStatus(t, s, "working")
	}
	s.ApplyCodexHook(CodexHook{Event: "Unknown", SessionID: "root", TurnID: "other"})
	s.ApplyCodexHook(CodexHook{Event: "Stop", SessionID: "root", TurnID: "root-turn"})
	expectCodexStatus(t, s, "idle")
	s.ApplyCodexHook(CodexHook{Event: "SessionStart", SessionID: "resumed", Source: "resume"})
	s.ApplyCodexHook(CodexHook{Event: "UserPromptSubmit", SessionID: "resumed", TurnID: "next"})
	s.ApplyCodexHook(CodexHook{Event: "Stop", SessionID: "root"})
	expectCodexStatus(t, s, "working")
}
