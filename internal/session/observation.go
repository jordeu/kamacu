package session

import (
	"errors"
	"slices"
	"time"

	"kamacu/internal/observation"
	"kamacu/internal/terminalchoice"
)

type MonitorStatus struct {
	State      string    `json:"state"`
	Health     string    `json:"health"`
	Detail     string    `json:"detail,omitempty"`
	ObservedAt time.Time `json:"observed_at,omitempty"`
}

func (s *Session) Observation() observation.Snapshot {
	if s.observer == nil {
		return observation.Snapshot{}
	}
	return s.observer.Snapshot()
}
func (s *Session) SetMonitorStatus(v MonitorStatus) { s.mu.Lock(); s.monitor = v; s.mu.Unlock() }

// WriteObserved serializes an entire paste/submit with all browser input and
// checks the observation under the session mutex before the first byte.
func (s *Session) WriteObserved(revision, input uint64, body, submit []byte) error {
	s.inputMu.Lock()
	defer s.inputMu.Unlock()
	s.mu.Lock()
	if s.status != StatusRunning || s.stopRequested || s.observer == nil {
		s.mu.Unlock()
		return errors.New("session unavailable")
	}
	obs := s.observer.Snapshot()
	if obs.Revision != revision || obs.Input != input {
		s.mu.Unlock()
		return errors.New("terminal changed; review the current question before replying")
	}
	s.observer.Input()
	s.lastInputSource = "external"
	s.mu.Unlock()
	if _, err := s.ptmx.Write(body); err != nil {
		return err
	}
	time.Sleep(100 * time.Millisecond)
	_, err := s.ptmx.Write(submit)
	return err
}

// WriteMessage excludes keyboard input and other message writers between the
// paste and its separate submit key, just like an observed inbox reply.
func (s *Session) WriteMessage(body, submit []byte) error {
	s.inputMu.Lock()
	defer s.inputMu.Unlock()
	if err := s.writeInput(body, "external"); err != nil {
		return err
	}
	time.Sleep(100 * time.Millisecond)
	return s.writeInput(submit, "external")
}

// terminalAgentStatus is engine-independent. Quiet means no recent terminal
// activity, not completion. Model uncertainty/availability is diagnostic only.
func terminalAgentStatus(obs observation.Snapshot, monitor MonitorStatus, now time.Time) string {
	activity := obs.OutputAt
	if obs.InputAt.After(activity) {
		activity = obs.InputAt
	}
	// A decision describes only the screen/input that was classified. New
	// activity immediately invalidates it, without waiting for another LLM call.
	if !monitor.ObservedAt.IsZero() && !activity.After(monitor.ObservedAt) {
		switch monitor.State {
		case "waiting", "idle", "error":
			return monitor.State
		}
	}
	if !activity.IsZero() && now.Sub(activity) < 15*time.Second {
		return "working"
	}
	return "idle"
}

// WriteObservedChoice navigates a recognized Claude dialog one key at a time.
// Each movement must be reflected in the rendered screen before Enter is sent.
// Keyboard input and other inbox writers cannot interleave with this operation.
func (s *Session) WriteObservedChoice(revision, input uint64, answer string) error {
	s.inputMu.Lock()
	defer s.inputMu.Unlock()
	s.mu.Lock()
	if s.status != StatusRunning || s.stopRequested || s.observer == nil || s.engine != "claude" {
		s.mu.Unlock()
		return errors.New("session unavailable")
	}
	obs := s.observer.Snapshot()
	dialog, ok := terminalchoice.Claude(obs.Screen)
	if !ok || obs.UnsupportedSize || obs.Revision != revision || obs.Input != input {
		s.mu.Unlock()
		return errors.New("terminal changed; review the current question before replying")
	}
	target := slices.Index(dialog.Options, answer)
	if target < 0 {
		s.mu.Unlock()
		return errors.New("choice is no longer available")
	}
	s.observer.Input()
	generation := s.observer.Snapshot().Input
	s.lastInputSource = "external"
	s.mu.Unlock()
	selected := dialog.Selected
	for {
		s.mu.Lock()
		current := s.observer.Snapshot()
		next, valid := terminalchoice.Claude(current.Screen)
		if s.status != StatusRunning || s.stopRequested || current.UnsupportedSize || current.Input != generation || !valid || next.Question != dialog.Question || !slices.Equal(next.Options, dialog.Options) || next.Selected != selected {
			s.mu.Unlock()
			return errors.New("terminal dialog changed during selection")
		}
		if selected == target {
			_, err := s.ptmx.Write([]byte("\r"))
			s.mu.Unlock()
			return err
		}
		step, key := 1, "\x1b[B"
		if target < selected {
			step, key = -1, "\x1b[A"
		}
		_, err := s.ptmx.Write([]byte(key))
		s.mu.Unlock()
		if err != nil {
			return err
		}
		expected := selected + step
		deadline := time.Now().Add(time.Second)
		for {
			current = s.Observation()
			next, valid = terminalchoice.Claude(current.Screen)
			if current.Input != generation {
				return errors.New("terminal input changed during selection")
			}
			if valid && next.Question == dialog.Question && slices.Equal(next.Options, dialog.Options) && next.Selected == expected {
				selected = expected
				break
			}
			if current.Revision != obs.Revision && valid && (next.Question != dialog.Question || !slices.Equal(next.Options, dialog.Options)) {
				return errors.New("terminal dialog changed during selection")
			}
			if time.Now().After(deadline) {
				return errors.New("terminal did not confirm choice navigation")
			}
			time.Sleep(10 * time.Millisecond)
		}
	}
}
