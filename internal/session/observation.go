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
	if err := s.writeInput(body); err != nil {
		return err
	}
	time.Sleep(100 * time.Millisecond)
	return s.writeInput(submit)
}
