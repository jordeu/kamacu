package session

import (
	"context"
	"errors"
	"regexp"
	"strings"
	"time"

	"kamacu/internal/observation"
)

// ErrObservedChanged means no input was written: a saved action's guard failed.
var ErrObservedChanged = errors.New("terminal changed before the action")
var emptyAgentPrompt = regexp.MustCompile(`(?m)^[\t ]*[❯›>][\t ]*$`)

// WaitInboxReady uses session lifecycle and terminal evidence, not an LLM. An
// empty newborn PTY is never considered ready just because its status is idle.
func (s *Session) WaitInboxReady(ctx context.Context, allowWorking ...bool) (observation.Snapshot, error) {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()
	for {
		info := s.Info()
		obs := s.Observation()
		if info.Kind != KindAgent || info.Status != StatusRunning || info.StopRequested {
			return obs, ErrObservedChanged
		}
		if info.AgentStatus == "waiting" || info.AgentStatus == "error" {
			return obs, errors.New("agent needs attention in its terminal before an Inbox action can submit input")
		}
		quiet := obs.OutputAt.IsZero() || time.Since(obs.OutputAt) >= 250*time.Millisecond
		text := strings.ToLower(obs.Screen)
		busy := strings.Contains(text, "esc to interrupt") || strings.Contains(text, "esc to stop")
		// A verified empty input prompt permits first-turn delivery immediately,
		// without waiting for the generic activity heuristic's 15-second cooldown.
		hasPrompt := inboxInputPrompt(info.Engine, obs.Screen)
		sendNow := len(allowWorking) > 0 && allowWorking[0]
		if sendNow && info.Engine == "codex" {
			hasPrompt = false
			if codexScreenStatus(obs.Screen) != "waiting" {
				for _, line := range strings.Split(obs.Screen, "\n") {
					text := strings.TrimSpace(line)
					if (strings.HasPrefix(text, "›") || strings.HasPrefix(text, "❯")) && !codexChoice.MatchString(text) {
						hasPrompt = true
					}
				}
			}
		}
		if sendNow && hasPrompt {
			busy = false
		}
		firstPrompt := obs.Input == 0 && hasPrompt && !busy
		if strings.TrimSpace(obs.Screen) != "" && quiet && hasPrompt && !busy && (info.AgentStatus == "idle" || firstPrompt || sendNow) {
			return obs, nil
		}
		select {
		case <-ctx.Done():
			return obs, errors.New("agent did not become ready within the action deadline; open its terminal")
		case <-s.Done():
			return obs, ErrObservedChanged
		case <-ticker.C:
		}
	}
}

// StopForInbox serializes the input guard and shutdown with keyboard/other
// writers. Only the saved session is stopped; it never resolves a replacement.
func (s *Session) StopForInbox(ctx context.Context, input uint64) error {
	s.inputMu.Lock()
	defer s.inputMu.Unlock()
	if s.Observation().Input != input {
		return ErrObservedChanged
	}
	s.Stop()
	select {
	case <-s.Done():
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func inboxInputPrompt(engine, screen string) bool {
	if engine == "codex" {
		return codexScreenStatus(screen) == "idle"
	}

	lines := strings.Split(screen, "\n")
	last := -1
	hasPrompt := false
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		// The latest input row wins. An older empty prompt does not authorize
		// submitting into a newer row that already contains a user's draft.
		if strings.HasPrefix(trimmed, "❯") || strings.HasPrefix(trimmed, "›") || strings.HasPrefix(trimmed, ">") {
			last = i
			hasPrompt = emptyAgentPrompt.MatchString(trimmed) || (engine == "claude" || engine == "") && strings.HasPrefix(trimmed, "❯ Try ")
		}
		if engine == "opencode" && strings.Contains(trimmed, "Ask anything") {
			last = i
			hasPrompt = true
		}
	}
	if !hasPrompt {
		return false
	}
	// Dialog hints can precede the cursor row. Inspect its surrounding UI rather
	// than treating phrases in old conversation output as a current dialog.
	nearby := strings.ToLower(strings.Join(lines[max(0, last-3):], "\n"))
	for _, hint := range []string{"enter to select", "enter to confirm", "enter to submit answer", "trust this folder", "trust this workspace"} {
		if strings.Contains(nearby, hint) {
			return false
		}
	}

	return hasPrompt
}
