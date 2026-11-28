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
var piContextUsage = regexp.MustCompile(`(?:^|[\t ])(?:[0-9]+(?:\.[0-9]+)?%|\?)/[1-9][0-9]*(?:\.[0-9]+)?[kKmM]?(?:[\t ]|$)`)

// WaitInboxReady uses session lifecycle and terminal evidence, not an LLM. An
// empty newborn PTY is never considered ready just because its status is idle.
func (s *Session) WaitInboxReady(ctx context.Context, allowWorking ...bool) (observation.Snapshot, error) {
	return s.waitInboxReady(ctx, len(allowWorking) > 0 && allowWorking[0], nil)
}

func (s *Session) waitInboxReady(ctx context.Context, sendNow bool, expectedInput *uint64) (observation.Snapshot, error) {
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
		if expectedInput != nil && obs.Input != *expectedInput {
			return obs, ErrObservedChanged
		}
		if info.AgentStatus == "waiting" || info.AgentStatus == "error" {
			return obs, errors.New("agent needs attention in its terminal before an Inbox action can submit input")
		}
		quiet := obs.OutputAt.IsZero() || time.Since(obs.OutputAt) >= 250*time.Millisecond
		busy := info.Engine != "pi" && !sendNow && inboxScreenShowsWork(obs.Screen)
		// An empty prompt permits first-turn delivery or revalidation after
		// a repaint without waiting for the activity heuristic to cool down.
		var hasPrompt bool
		if info.Engine == "pi" {
			hasPrompt = piInboxReady(obs, info.Monitor, sendNow)
		} else {
			hasPrompt = inboxInputPrompt(info.Engine, obs.Screen)
		}
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
		promptVerified := obs.Input == 0 || expectedInput != nil
		if quiet && hasPrompt && !busy && (info.AgentStatus == "idle" || promptVerified || sendNow) {
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

// WriteInboxAction retries a Pi action once when an intervening repaint made
// its readiness observation stale. ErrObservedChanged is returned before any
// PTY write, and the second attempt still checks the original input generation.
func (s *Session) WriteInboxAction(ctx context.Context, revision, input uint64, body, submit []byte, allowWorking bool) error {
	// The monitor can change its decision without a PTY repaint. Recheck Pi
	// readiness and cancellation inside the same guard as the revision check.
	guard := func(obs observation.Snapshot) bool {
		return ctx.Err() == nil && (s.engine != "pi" || piInboxReady(obs, s.monitor, allowWorking))
	}
	err := s.writeObserved(revision, input, body, submit, guard)
	if s.engine != "pi" || !errors.Is(err, ErrObservedChanged) || ctx.Err() != nil || s.Observation().Revision == revision {
		return err
	}
	recheck, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	obs, err := s.waitInboxReady(recheck, allowWorking, &input)
	if err != nil {
		return ErrObservedChanged
	}
	return s.writeObserved(obs.Revision, input, body, submit, func(obs observation.Snapshot) bool {
		return recheck.Err() == nil && guard(obs)
	})
}

func inboxScreenShowsWork(screen string) bool {
	text := strings.ToLower(screen)
	return strings.Contains(text, "esc to interrupt") || strings.Contains(text, "esc to stop")
}

func piInboxReady(obs observation.Snapshot, monitor MonitorStatus, allowWorking bool) bool {
	if !monitor.ObservedAt.IsZero() && !obs.OutputAt.After(monitor.ObservedAt) && !obs.InputAt.After(monitor.ObservedAt) {
		switch monitor.State {
		case "waiting", "error":
			return false
		case "working":
			if !allowWorking {
				return false
			}
		}
	}
	return !obs.UnsupportedSize && (allowWorking || !inboxScreenShowsWork(obs.Screen)) && piEmptyEditor(obs.Screen)
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
	switch engine {
	case "codex":
		return codexScreenStatus(screen) == "idle"
	case "pi":
		return piEmptyEditor(screen)
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

// Pi's default editor renders a blank row between two matching horizontal
// rules, directly above the working directory and context usage. A draft,
// working indicator, or replacement editor changes that layout. A zero-size
// context window is Pi's placeholder when there is no model.
func piEmptyEditor(screen string) bool {
	lines := strings.Split(strings.TrimRight(screen, " \t\r\n"), "\n")
	if len(lines) < 5 {
		return false
	}
	tail := lines[len(lines)-5:]
	top, bottom := strings.TrimSpace(tail[0]), strings.TrimSpace(tail[2])
	if top == "" || top != bottom || strings.Trim(top, "─") != "" || strings.TrimSpace(tail[1]) != "" {
		return false
	}
	dir := strings.TrimSpace(tail[3])
	if dir != "~" && !strings.HasPrefix(dir, "~/") && !strings.HasPrefix(dir, "/") {
		return false
	}
	return piContextUsage.MatchString(tail[4])
}
