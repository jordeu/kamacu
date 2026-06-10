package session

import (
	"encoding/json"
	"fmt"
)

// Kind discriminates session families: plain bash tabs vs the per-task
// Claude Code agent session. The zero value is treated as KindBash everywhere
// so Phase 2/3 callers are untouched.
type Kind string

const (
	KindBash  Kind = "bash"
	KindAgent Kind = "agent"
)

// AgentConfig is the per-server-instance configuration agent spawns read.
// Set once at startup via Manager.SetAgentConfig; Spawn reads a copy under
// the manager mutex.
type AgentConfig struct {
	BaseURL   string // hook receiver origin, e.g. "http://127.0.0.1:7333"
	Token     string // per-instance X-Kangent-Token value
	ClaudeBin string // "" -> exec.LookPath("claude") at spawn time
}

// overlayHook is one hook command entry in the injected settings overlay.
type overlayHook struct {
	Type    string `json:"type"`
	Command string `json:"command"`
	Timeout int    `json:"timeout"`
	Async   bool   `json:"async"`
}

// overlayEntry is a matcher group. Matcher is omitted entirely when empty:
// Stop and SessionStart have no matchers in v2.1.170 (research finding 15).
type overlayEntry struct {
	Matcher string        `json:"matcher,omitempty"`
	Hooks   []overlayHook `json:"hooks"`
}

// overlaySettings is the full --settings overlay document.
type overlaySettings struct {
	Hooks                 map[string][]overlayEntry `json:"hooks"`
	PreferredNotifChannel string                    `json:"preferredNotifChannel"`
}

// buildOverlayJSON constructs the inline --settings overlay injected at agent
// spawn (STAT-02, D-53). The shape is the empirically verified v2.1.170
// overlay (04-RESEARCH.md Pattern 2) plus a SessionStart entry — the
// hooks-alive canary that gates the BEL fallback.
//
// Built via encoding/json (never string concatenation) so escaping is always
// valid. Nothing is written to disk: the whole document travels as one argv.
//
// The Notification matcher deliberately excludes the idle-prompt event —
// that event is D-46's *idle*, not *waiting*; including it would make every
// quiet session go loud.
func buildOverlayJSON(baseURL, token, kangentSessionID string) string {
	hook := overlayHook{
		Type: "command",
		Command: fmt.Sprintf(
			"curl -s -m 3 -H 'X-Kangent-Token: %s' --data-binary @- %s/api/hooks/sessions/%s",
			token, baseURL, kangentSessionID,
		),
		Timeout: 5,
		Async:   true,
	}
	overlay := overlaySettings{
		Hooks: map[string][]overlayEntry{
			"Notification": {{
				Matcher: "permission_prompt|elicitation_dialog",
				Hooks:   []overlayHook{hook},
			}},
			"Stop":         {{Hooks: []overlayHook{hook}}},
			"SessionStart": {{Hooks: []overlayHook{hook}}},
		},
		PreferredNotifChannel: "terminal_bell",
	}
	b, err := json.Marshal(overlay)
	if err != nil {
		// Marshaling a static struct of strings/ints/bools cannot fail.
		panic(fmt.Sprintf("marshal settings overlay: %v", err))
	}
	return string(b)
}

// belScanner counts bare BEL (0x07) bytes in PTY output, treating BELs that
// terminate an OSC string (ESC ] ... BEL) as part of the sequence, with state
// carried across chunks. A naive 0x07 scan false-positives on title-set
// sequences (research anti-pattern).
type belScanner struct{ inOSC bool }

func (b *belScanner) scan(chunk []byte) (bare int) {
	for i := 0; i < len(chunk); i++ {
		c := chunk[i]
		switch {
		case b.inOSC:
			if c == 0x07 { // OSC terminator — not a bell
				b.inOSC = false
			} else if c == 0x1b && i+1 < len(chunk) && chunk[i+1] == '\\' {
				b.inOSC = false // ST terminator
				i++
			}
		case c == 0x1b && i+1 < len(chunk) && chunk[i+1] == ']':
			b.inOSC = true
			i++
		case c == 0x07:
			bare++
		}
	}
	return
}
