// Package presence tracks ephemeral user-presence beacons from the Kamacu
// UI so agents (via the MCP bridge) can learn where the user's attention
// is: which route/task/session the user is viewing, whether they are active
// or away (and for how long), and which sessions they are typing into.
//
// All state is in-memory and lazily computed on read — no goroutines, no
// persistence, no DB (the D-47 agentStatusLocked philosophy: the reader
// computes freshness, a ticker never runs). Server timestamps only; client
// clocks are never trusted.
package presence

import (
	"sort"
	"sync"
	"time"
)

// ActiveWindow is how long a client stays listed after its last heartbeat.
// Deliberately looser than the session package's agentQuietThreshold (10s):
// heartbeats are periodic (5s interval), not event-driven, so the prune
// window must absorb one missed beat without flapping the user to away.
const ActiveWindow = 15 * time.Second

// TypingWindow is how long a session counts as "being typed into" after the
// last browser-originated stdin byte (aligned with agentQuietThreshold —
// the same activity horizon the agent status state machine uses).
const TypingWindow = 10 * time.Second

// Beat is one UI heartbeat payload (POST /api/presence/heartbeat). ClientID
// identifies the browser tab (per-tab sessionStorage); one user may run
// several. Route is the SPA path ("/projects/3/tasks/7"); ProjectID/TaskID
// are parsed route params; SessionID is the terminal session whose tab is
// focused, when known. Visible mirrors document.visibilityState — the UI
// sends a final visible:false beat when the tab hides.
type Beat struct {
	ClientID  string `json:"client_id"`
	Route     string `json:"route"`
	ProjectID int64  `json:"project_id,omitempty"`
	TaskID    int64  `json:"task_id,omitempty"`
	SessionID string `json:"session_id,omitempty"`
	Visible   bool   `json:"visible"`
}

// Viewing is the user's current focus: the most recently seen VISIBLE
// client (hidden tabs never steal focus).
type Viewing struct {
	ClientID  string `json:"client_id"`
	Route     string `json:"route"`
	ProjectID int64  `json:"project_id,omitempty"`
	TaskID    int64  `json:"task_id,omitempty"`
	SessionID string `json:"session_id,omitempty"`
}

// ClientView is one listed UI client (browser tab), newest first.
type ClientView struct {
	ClientID  string    `json:"client_id"`
	Route     string    `json:"route"`
	ProjectID int64     `json:"project_id,omitempty"`
	TaskID    int64     `json:"task_id,omitempty"`
	SessionID string    `json:"session_id,omitempty"`
	Visible   bool      `json:"visible"`
	LastSeen  time.Time `json:"last_seen"`
}

// Snapshot is the read model served by GET /api/presence: active iff ANY
// client is both fresh and visible (union across tabs); away_for_seconds
// counts from the last VISIBLE beat, so the hide-time final beat makes the
// user away immediately instead of after the prune window.
type Snapshot struct {
	Active         bool         `json:"active"`
	AwayForSeconds int64        `json:"away_for_seconds"`
	LastActiveAt   time.Time    `json:"last_active_at,omitempty"`
	Viewing        *Viewing     `json:"viewing,omitempty"`
	Clients        []ClientView `json:"clients"`
	TypingIn       []string     `json:"typing_in"`
}

type clientState struct {
	beat     Beat
	lastSeen time.Time
}

// Tracker is the in-memory presence state. Safe for concurrent use; every
// method is a mutex-guarded flip, never blocking (the hooks.go contract).
type Tracker struct {
	now func() time.Time

	mu            sync.Mutex
	clients       map[string]*clientState
	typing        map[string]time.Time
	lastPresentAt time.Time // last VISIBLE beat, ever
}

// NewTracker returns a Tracker clocked at time.Now.
func NewTracker() *Tracker {
	return &Tracker{
		now:     time.Now,
		clients: make(map[string]*clientState),
		typing:  make(map[string]time.Time),
	}
}

// Heartbeat records one UI beat, upserting the client row and stamping the
// server's clock. Visible beats also advance lastPresentAt.
func (t *Tracker) Heartbeat(b Beat) {
	now := t.now()
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.clients == nil {
		t.clients = make(map[string]*clientState)
	}
	c, ok := t.clients[b.ClientID]
	if !ok {
		c = &clientState{}
		t.clients[b.ClientID] = c
	}
	c.beat = b
	c.lastSeen = now
	if b.Visible {
		t.lastPresentAt = now
	}
}

// NoteUserInput records browser-originated stdin for a session (the WS
// readLoop seam — agent-delegated input via the REST /input endpoint never
// reaches this method). Timestamps only; no content is captured.
func (t *Tracker) NoteUserInput(sessionID string) {
	now := t.now()
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.typing == nil {
		t.typing = make(map[string]time.Time)
	}
	t.typing[sessionID] = now
}

// Snapshot prunes stale state and returns the presence read model. Lists
// are deterministically ordered: clients newest-first, typing_in ascending.
func (t *Tracker) Snapshot() Snapshot {
	now := t.now()
	t.mu.Lock()
	defer t.mu.Unlock()

	for id, c := range t.clients {
		if now.Sub(c.lastSeen) > ActiveWindow {
			delete(t.clients, id)
		}
	}
	for id, at := range t.typing {
		if now.Sub(at) > TypingWindow {
			delete(t.typing, id)
		}
	}

	snap := Snapshot{
		Clients:  make([]ClientView, 0, len(t.clients)),
		TypingIn: make([]string, 0, len(t.typing)),
	}

	var newestVisible *clientState
	for _, c := range t.clients {
		snap.Clients = append(snap.Clients, ClientView{
			ClientID:  c.beat.ClientID,
			Route:     c.beat.Route,
			ProjectID: c.beat.ProjectID,
			TaskID:    c.beat.TaskID,
			SessionID: c.beat.SessionID,
			Visible:   c.beat.Visible,
			LastSeen:  c.lastSeen,
		})
		if c.beat.Visible && (newestVisible == nil || c.lastSeen.After(newestVisible.lastSeen)) {
			newestVisible = c
		}
	}
	sort.Slice(snap.Clients, func(i, j int) bool {
		return snap.Clients[i].LastSeen.After(snap.Clients[j].LastSeen)
	})

	for id := range t.typing {
		snap.TypingIn = append(snap.TypingIn, id)
	}
	sort.Strings(snap.TypingIn)

	if newestVisible != nil {
		snap.Active = true
		snap.Viewing = &Viewing{
			ClientID:  newestVisible.beat.ClientID,
			Route:     newestVisible.beat.Route,
			ProjectID: newestVisible.beat.ProjectID,
			TaskID:    newestVisible.beat.TaskID,
			SessionID: newestVisible.beat.SessionID,
		}
	}

	snap.LastActiveAt = t.lastPresentAt
	if !snap.Active && !t.lastPresentAt.IsZero() {
		snap.AwayForSeconds = int64(now.Sub(t.lastPresentAt) / time.Second)
	}
	return snap
}
