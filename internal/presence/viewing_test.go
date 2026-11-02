package presence

import (
	"testing"
	"time"
)

func TestViewingAtSurvivesLeaving(t *testing.T) {
	tr := NewTracker()
	now := time.Now()
	tr.now = func() time.Time { return now }
	tr.Heartbeat(Beat{ClientID: "tab", SessionID: "a", Visible: true})
	evidence := now.Add(time.Second)
	now = now.Add(3 * time.Second)
	tr.Heartbeat(Beat{ClientID: "tab", Visible: false})
	if tr.ViewingSession("a") {
		t.Fatal("still viewing")
	}
	if !tr.ViewingAt("a", evidence) {
		t.Fatal("lost evidence-time presence")
	}
	now = now.Add(time.Second)
	tr.Heartbeat(Beat{ClientID: "other", SessionID: "a", Visible: true})
	if !tr.ViewingSession("a") {
		t.Fatal("other tab ignored")
	}
}

func TestOutOfOrderBeatsCannotUndoViewing(t *testing.T) {
	tr := NewTracker()
	tr.Heartbeat(Beat{ClientID: "tab", Sequence: 2, SessionID: "a", Visible: true})
	if tr.Heartbeat(Beat{ClientID: "tab", Sequence: 1, Visible: false}) {
		t.Fatal("accepted older departure")
	}
	if !tr.ViewingSession("a") {
		t.Fatal("late departure overrode current viewing")
	}
	tr.Heartbeat(Beat{ClientID: "tab", Sequence: 3, Visible: false})
	if tr.Heartbeat(Beat{ClientID: "tab", Sequence: 2, SessionID: "a", Visible: true}) {
		t.Fatal("accepted older arrival")
	}
	if tr.ViewingSession("a") {
		t.Fatal("late arrival hid pending notification")
	}
}
