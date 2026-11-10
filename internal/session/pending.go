package session

import (
	"strings"
	"time"

	"kamacu/internal/observation"
)

// WritePendingPrompt guards the input surface as well as its revision. A quiet
// terminal can still contain a user's draft, a permission menu or a busy turn.
// The check runs under the same locks as the observed paste/submit.
func (s *Session) WritePendingPrompt(revision, input uint64, body, submit []byte, resumed bool) error {
	return s.writeObserved(revision, input, body, submit, func(obs observation.Snapshot) bool {
		state := terminalAgentStatus(obs, s.monitor, time.Now())
		if s.monitor.State == "working" && !obs.InputAt.After(s.monitor.ObservedAt) && !obs.OutputAt.After(s.monitor.ObservedAt) {
			return false
		}
		firstPrompt := resumed && obs.Input == 0
		return (state == "idle" || state == "waiting" || firstPrompt) && pendingInputPrompt(s.engine, obs)
	})
}

func pendingInputPrompt(engine string, obs observation.Snapshot) bool {
	if obs.UnsupportedSize || !inboxInputPrompt(engine, obs.Screen) {
		return false
	}
	text := strings.ToLower(obs.Screen)
	if strings.Contains(text, "esc to interrupt") || strings.Contains(text, "esc to stop") {
		return false
	}
	if engine == "codex" {
		// Codex's status recognizer accepts both placeholders and typed text. Only
		// an empty composer or its default placeholder is safe for a bulk paste.
		prompt := ""
		for _, line := range strings.Split(obs.Screen, "\n") {
			line = strings.TrimSpace(line)
			if strings.HasPrefix(line, "›") || strings.HasPrefix(line, "❯") {
				prompt = line
			}
		}
		return prompt == "›" || prompt == "❯" || prompt == "› Ask Codex to do anything"
	}
	return true
}

// PendingCheckFinished requires affirmative completion evidence. The normal
// status dot may become idle after fifteen seconds of silence, even while a
// model or tool is still running; that heuristic must not release a bulk slot.
func (s *Session) PendingCheckFinished() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.status != StatusRunning || s.observer == nil {
		return false
	}
	return pendingCheckFinished(s.observer.Snapshot(), s.monitor, s.engine, time.Now())
}

func pendingCheckFinished(obs observation.Snapshot, monitor MonitorStatus, engine string, now time.Time) bool {
	activity := obs.InputAt
	if obs.OutputAt.After(activity) {
		activity = obs.OutputAt
	}
	if !monitor.ObservedAt.IsZero() && !activity.After(monitor.ObservedAt) {
		switch monitor.State {
		case "idle", "waiting":
			return true
		case "working", "error":
			return false
		}
	}
	// A rendered empty prompt after new output is also evidence, even when the
	// classifier is unavailable. Require quiet output so transient repainting
	// does not release a slot before the response starts.
	return obs.OutputAt.After(obs.InputAt) && now.Sub(obs.OutputAt) >= time.Second && pendingInputPrompt(engine, obs)
}
