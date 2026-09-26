package presence

import (
	"slices"
	"testing"
	"time"
)

// newTestTracker returns a Tracker whose clock starts at a fixed base and is
// advanced only via the returned closure (deterministic time tests).
func newTestTracker() (*Tracker, *time.Time) {
	tr := NewTracker()
	cur := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	tr.now = func() time.Time { return cur }
	return tr, &cur
}

func advance(cur *time.Time, d time.Duration) { *cur = cur.Add(d) }

// TestHeartbeatFreshVisibleActive: one fresh visible beat makes the user
// active with zero away time and a populated viewing context.
func TestHeartbeatFreshVisibleActive(t *testing.T) {
	tr, _ := newTestTracker()
	tr.Heartbeat(Beat{
		ClientID:  "tab-1",
		Route:     "/projects/3/tasks/7",
		ProjectID: 3,
		TaskID:    7,
		SessionID: "sess-a",
		Visible:   true,
	})

	snap := tr.Snapshot()
	if !snap.Active {
		t.Errorf("Active = false, want true after a fresh visible beat")
	}
	if snap.AwayForSeconds != 0 {
		t.Errorf("AwayForSeconds = %d, want 0 while active", snap.AwayForSeconds)
	}
	if snap.Viewing == nil {
		t.Fatalf("Viewing = nil, want the visible client's context")
	}
	if snap.Viewing.ClientID != "tab-1" {
		t.Errorf("Viewing.ClientID = %q, want tab-1", snap.Viewing.ClientID)
	}
	if snap.Viewing.Route != "/projects/3/tasks/7" {
		t.Errorf("Viewing.Route = %q, want the beat's route", snap.Viewing.Route)
	}
	if snap.Viewing.ProjectID != 3 || snap.Viewing.TaskID != 7 {
		t.Errorf("Viewing ids = %d/%d, want 3/7", snap.Viewing.ProjectID, snap.Viewing.TaskID)
	}
	if snap.Viewing.SessionID != "sess-a" {
		t.Errorf("Viewing.SessionID = %q, want sess-a", snap.Viewing.SessionID)
	}
	if len(snap.Clients) != 1 {
		t.Errorf("Clients = %d, want 1: %+v", len(snap.Clients), snap.Clients)
	}
	if !slices.Contains(snap.TypingIn, "sess-a") {
		// not asserted — typing is stdin-driven, not viewing-driven; placeholder no-op
		_ = snap.TypingIn
	}
}

// TestNoBeatsInactiveEmpty: a tracker with no heartbeats reports inactive,
// empty clients, no viewing, and a zero (omitted) last-active timestamp.
func TestNoBeatsInactiveEmpty(t *testing.T) {
	tr, _ := newTestTracker()
	snap := tr.Snapshot()
	if snap.Active {
		t.Errorf("Active = true, want false with no beats")
	}
	if snap.Viewing != nil {
		t.Errorf("Viewing = %+v, want nil", snap.Viewing)
	}
	if len(snap.Clients) != 0 {
		t.Errorf("Clients = %+v, want empty", snap.Clients)
	}
	if !snap.LastActiveAt.IsZero() {
		t.Errorf("LastActiveAt = %v, want zero (never seen)", snap.LastActiveAt)
	}
	if len(snap.TypingIn) != 0 {
		t.Errorf("TypingIn = %v, want empty", snap.TypingIn)
	}
}

// TestStaleClientsPrunedAway: clients silent past ActiveWindow disappear and
// the user counts as away for the elapsed time since the last visible beat.
func TestStaleClientsPrunedAway(t *testing.T) {
	tr, cur := newTestTracker()
	tr.Heartbeat(Beat{ClientID: "tab-1", Route: "/projects/3", Visible: true})
	advance(cur, ActiveWindow+time.Second)

	snap := tr.Snapshot()
	if snap.Active {
		t.Errorf("Active = true past the active window, want false")
	}
	if len(snap.Clients) != 0 {
		t.Errorf("Clients = %+v, want pruned", snap.Clients)
	}
	if snap.Viewing != nil {
		t.Errorf("Viewing = %+v, want nil after prune", snap.Viewing)
	}
	if snap.LastActiveAt.IsZero() {
		t.Errorf("LastActiveAt = zero, want the remembered last visible beat")
	}
	if want := int64((ActiveWindow + time.Second) / time.Second); snap.AwayForSeconds != want {
		t.Errorf("AwayForSeconds = %d, want %d", snap.AwayForSeconds, want)
	}
}

// TestHiddenFinalBeatAwayImmediately: the UI's final visible:false beat marks
// the user away at once (no 15s wait) while the client row stays listed
// until the prune window passes.
func TestHiddenFinalBeatAwayImmediately(t *testing.T) {
	tr, cur := newTestTracker()
	tr.Heartbeat(Beat{ClientID: "tab-1", Route: "/projects/3", Visible: true})
	advance(cur, 5*time.Second)
	tr.Heartbeat(Beat{ClientID: "tab-1", Route: "/projects/3", Visible: false})

	snap := tr.Snapshot()
	if snap.Active {
		t.Errorf("Active = true after a hidden final beat, want false immediately")
	}
	if len(snap.Clients) != 1 {
		t.Errorf("Clients = %d, want 1 (fresh-but-hidden stays listed): %+v", len(snap.Clients), snap.Clients)
	}
	if snap.Clients[0].Visible {
		t.Errorf("client Visible = true, want false")
	}
	if snap.Viewing != nil {
		t.Errorf("Viewing = %+v, want nil (no visible client)", snap.Viewing)
	}
	if snap.AwayForSeconds != 5 {
		t.Errorf("AwayForSeconds = %d, want 5 (since the last VISIBLE beat)", snap.AwayForSeconds)
	}
}

// TestHeartbeatUpsertsSameClient: repeat beats from one client update the
// row in place — never duplicate.
func TestHeartbeatUpsertsSameClient(t *testing.T) {
	tr, cur := newTestTracker()
	tr.Heartbeat(Beat{ClientID: "tab-1", Route: "/projects/3", Visible: true})
	advance(cur, 2*time.Second)
	tr.Heartbeat(Beat{ClientID: "tab-1", Route: "/projects/4/tasks/9", ProjectID: 4, TaskID: 9, Visible: true})

	snap := tr.Snapshot()
	if len(snap.Clients) != 1 {
		t.Fatalf("Clients = %d, want 1 (upsert): %+v", len(snap.Clients), snap.Clients)
	}
	if snap.Viewing == nil || snap.Viewing.Route != "/projects/4/tasks/9" {
		t.Errorf("Viewing = %+v, want the updated route", snap.Viewing)
	}
}

// TestMultiTabUnionMostRecentViewing: active = ANY visible client (union);
// viewing = the most recently seen VISIBLE client; when it hides, viewing
// falls back to the still-visible peer.
func TestMultiTabUnionMostRecentViewing(t *testing.T) {
	tr, cur := newTestTracker()
	tr.Heartbeat(Beat{ClientID: "tab-a", Route: "/projects/3", Visible: true})
	advance(cur, 2*time.Second)
	tr.Heartbeat(Beat{ClientID: "tab-b", Route: "/global", Visible: true})

	snap := tr.Snapshot()
	if !snap.Active {
		t.Errorf("Active = false, want true (union of visible tabs)")
	}
	if snap.Viewing == nil || snap.Viewing.ClientID != "tab-b" {
		t.Errorf("Viewing = %+v, want tab-b (most recent visible)", snap.Viewing)
	}

	// tab-b hides: still active via tab-a, viewing falls back to tab-a.
	advance(cur, 2*time.Second)
	tr.Heartbeat(Beat{ClientID: "tab-b", Route: "/global", Visible: false})
	snap = tr.Snapshot()
	if !snap.Active {
		t.Errorf("Active = false after one tab hid, want true (tab-a remains)")
	}
	if snap.Viewing == nil || snap.Viewing.ClientID != "tab-a" {
		t.Errorf("Viewing = %+v, want fallback to tab-a", snap.Viewing)
	}
}

// TestClientsSortedByLastSeenDesc: the per-client list is deterministic —
// newest beat first.
func TestClientsSortedByLastSeenDesc(t *testing.T) {
	tr, cur := newTestTracker()
	tr.Heartbeat(Beat{ClientID: "old", Route: "/a", Visible: true})
	advance(cur, 3*time.Second)
	tr.Heartbeat(Beat{ClientID: "new", Route: "/b", Visible: true})

	snap := tr.Snapshot()
	if len(snap.Clients) != 2 {
		t.Fatalf("Clients = %d, want 2", len(snap.Clients))
	}
	if snap.Clients[0].ClientID != "new" || snap.Clients[1].ClientID != "old" {
		t.Errorf("order = [%s %s], want [new old]", snap.Clients[0].ClientID, snap.Clients[1].ClientID)
	}
}

// TestTypingWindow: NoteUserInput feeds typing_in for TypingWindow seconds
// only, and the list is sorted for determinism.
func TestTypingWindow(t *testing.T) {
	tr, cur := newTestTracker()
	tr.NoteUserInput("sess-b")
	tr.NoteUserInput("sess-a")

	snap := tr.Snapshot()
	if !slices.Equal(snap.TypingIn, []string{"sess-a", "sess-b"}) {
		t.Errorf("TypingIn = %v, want [sess-a sess-b] (sorted)", snap.TypingIn)
	}

	advance(cur, TypingWindow-time.Second)
	snap = tr.Snapshot()
	if !slices.Equal(snap.TypingIn, []string{"sess-a", "sess-b"}) {
		t.Errorf("TypingIn = %v, want both still inside the window", snap.TypingIn)
	}

	advance(cur, 2*time.Second)
	snap = tr.Snapshot()
	if len(snap.TypingIn) != 0 {
		t.Errorf("TypingIn = %v, want empty past the window", snap.TypingIn)
	}
}

// TestNoteUserInputRefreshes: a fresh keystroke restarts that session's
// typing window without affecting others.
func TestNoteUserInputRefreshes(t *testing.T) {
	tr, cur := newTestTracker()
	tr.NoteUserInput("sess-a")
	advance(cur, 8*time.Second)
	tr.NoteUserInput("sess-a")
	advance(cur, 8*time.Second) // 16s since nothing, 8s since sess-a's refresh

	snap := tr.Snapshot()
	if !slices.Equal(snap.TypingIn, []string{"sess-a"}) {
		t.Errorf("TypingIn = %v, want [sess-a] (refreshed)", snap.TypingIn)
	}
}
