package session

import (
	"errors"
	"time"

	"kamacu/internal/observation"
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
