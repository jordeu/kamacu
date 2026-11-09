package session

import (
	"testing"
	"time"
)

func TestLifecycleChanges(t *testing.T) {
	m := NewManager()
	first, second := m.Changes(), m.Changes()
	s := spawnForTest(t, m)
	wait := func(ch <-chan struct{}) {
		t.Helper()
		select {
		case <-ch:
		case <-time.After(2 * time.Second):
			t.Fatal("lifecycle notification missing")
		}
	}
	wait(first)
	wait(second) // all listeners receive the same broadcast
	if len(m.List()) != 1 {
		t.Fatal("spawn notification preceded registration")
	}
	exited := m.Changes()
	if err := s.WriteInput([]byte("exit\n")); err != nil {
		t.Fatal(err)
	}
	wait(exited)
	if s.Info().Status != StatusExited {
		t.Fatal("exit notification preceded state transition")
	}
	removed := m.Changes()
	if err := m.Remove(s.id); err != nil {
		t.Fatal(err)
	}
	wait(removed)
	select {
	case <-m.Changes():
		t.Fatal("fresh signal should wait for the next transition")
	default:
	}
}
