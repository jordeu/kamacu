package api

import (
	"context"
	"database/sql"
	"errors"
	"net/http"
	"os"

	"kamacu/internal/session"
	"kamacu/internal/tmux"
)

// globalAgent is the embedded agent summary on the GET wire (D-17): the
// singleton's current default agent, read via a JOIN. A value type, not a
// pointer — the INNER JOIN is guaranteed because global_task.agent_id is
// NOT NULL REFERENCES agents(id) ON DELETE RESTRICT (00017) and the Phase-13
// delete guard (agents_crud.go) refuses deleting the referenced agent.
type globalAgent struct {
	ID     int64  `json:"id"`
	Name   string `json:"name"`
	Engine string `json:"engine"`
}

// globalLive is the live-global-session count block (D-19): the wire
// contract ships complete from day one. Agent and Bash are literal zeros in
// Phase 14 — no global PTY can exist until Phase 15's spawn path lands
// Info.Global + ListGlobal(), which widens the derivation without a wire
// change.
type globalLive struct {
	Agent int `json:"agent"`
	Bash  int `json:"bash"`
	Tmux  int `json:"tmux"`
}

// globalConfig is the single wire type for GET /api/global and the PUT
// response (D-17/D-23): the stored singleton config PLUS derived state.
// There are deliberately NO claude_session_id / opencode_session_id fields
// anywhere — the resume ids are Phase-15 spawn-path internals that never
// serialize (D-18, enforced by TestGetGlobalUnconfigured). GithubRepo
// follows the 00017 marker contract: nil ⇒ folder root (or unconfigured),
// non-nil ⇒ managed clone. No managed bool — the pointer is the marker.
type globalConfig struct {
	RootPath   string      `json:"root_path"`
	GithubRepo *string     `json:"github_repo"`
	AgentID    int64       `json:"agent_id"`
	UpdatedAt  string      `json:"updated_at"`
	RootExists bool        `json:"root_exists"`
	Live       globalLive  `json:"live"`
	Agent      globalAgent `json:"agent"`
}

// globalHandlers carries the global-config dependencies (mirrors
// projectHandlers minus wt — no worktrees exist for the global scope). mgr
// is taken NOW even though the manager half of the live gate returns zeros:
// Phase 15's Info.Global + ListGlobal() widens globalLiveBlockers without a
// signature change (14-RESEARCH A2).
type globalHandlers struct {
	db         *sql.DB
	mgr        *session.Manager
	tmuxClient tmux.Client
}

// GlobalRoutes registers the global Scratchpad config endpoints (GCONF):
// GET reads config + derived live state (D-17). The PUT registration (the
// single partial config update, D-20) lands with the put handler in the
// next task.
func GlobalRoutes(mux *http.ServeMux, db *sql.DB, mgr *session.Manager, tmuxClient tmux.Client) {
	g := &globalHandlers{db: db, mgr: mgr, tmuxClient: tmuxClient}
	mux.HandleFunc("GET /api/global", g.get)
}

// liveGlobalTmuxNames returns the global-scope tmux session names alive on
// the dedicated socket. It is the liveTmuxNames analog (projects.go) with
// the predicate swapped to the 00018 scope discriminator. Each name is
// probed via h.tmuxClient.HasSession verbatim — the "="+name exact-match
// prefix is embedded in Client.HasSession and must never be hand-rolled
// (tmux 3.4 prefix-matching, 14-RESEARCH Pitfall 4). A name is live ONLY on
// (true, nil); (false, err) is inconclusive and NEVER read as alive (D-13
// probe contract). Degrade posture mirrors liveTmuxNames exactly: a query
// error returns nil, a scan error skips the row.
func (h *globalHandlers) liveGlobalTmuxNames(ctx context.Context) []string {
	rows, err := h.db.QueryContext(ctx, `SELECT name FROM tmux_sessions WHERE scope = 'global'`)
	if err != nil {
		return nil
	}
	defer rows.Close()
	var names []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			continue
		}
		names = append(names, name)
	}
	live := make([]string, 0, len(names))
	for _, name := range names {
		if alive, herr := h.tmuxClient.HasSession(ctx, name); alive && herr == nil {
			live = append(live, name)
		}
	}
	return live
}

// loadGlobalConfig reads the singleton row + agent summary — the stored
// half of the D-17 wire shape. get() and put() both build their response
// through this one loader so the endpoint has exactly one wire shape
// (D-23). ErrNoRows propagates to the caller, which fail-louds (the row is
// guaranteed by the 00017 seed + BackfillGlobalTask).
func (h *globalHandlers) loadGlobalConfig() (globalConfig, error) {
	var g globalConfig
	var repo sql.NullString
	err := h.db.QueryRow(`
		SELECT g.root_path, g.github_repo, g.agent_id, g.updated_at,
		       a.id, a.name, a.engine
		FROM global_task g JOIN agents a ON a.id = g.agent_id
		WHERE g.id = 1`).Scan(
		&g.RootPath, &repo, &g.AgentID, &g.UpdatedAt,
		&g.Agent.ID, &g.Agent.Name, &g.Agent.Engine)
	if err != nil {
		return g, err
	}
	if repo.Valid {
		g.GithubRepo = &repo.String
	}
	return g, nil
}

// deriveGlobalState fills the read-time half of the wire shape (D-17):
// root_exists via an inline os.Stat IsDir check (the api package has no
// dirExists helper — do not import internal/migrate for one) and the live
// counts. An empty RootPath never probes the filesystem. The stat is honest
// about a vanished root at GET time without violating D-08's no-boot-
// revalidation — nothing is revalidated or repaired here.
func (h *globalHandlers) deriveGlobalState(ctx context.Context, g *globalConfig) {
	if g.RootPath != "" {
		if info, err := os.Stat(g.RootPath); err == nil && info.IsDir() {
			g.RootExists = true
		}
	}
	// tmux half is REAL today (00018 rows + exact-match probe). The manager
	// half is a zero-count seam until Phase 15's ListGlobal() (D-19).
	g.Live.Tmux = len(h.liveGlobalTmuxNames(ctx))
	g.Live.Agent = 0 // Phase-15 ListGlobal() seam (D-19)
	g.Live.Bash = 0  // Phase-15 ListGlobal() seam (D-19)
}

// get handles GET /api/global: the config + derived-state read that serves
// Phase 16's Settings section and the /global view's honest unconfigured
// state in one round-trip (GVIEW-04).
func (h *globalHandlers) get(w http.ResponseWriter, r *http.Request) {
	g, err := h.loadGlobalConfig()
	if errors.Is(err, sql.ErrNoRows) {
		// Unreachable by design (00017 seed + BackfillGlobalTask); only a
		// hand-SQL DELETE mid-run can drop the row. That is a corrupted
		// invariant — fail loudly, never paper over it (14-RESEARCH Open
		// Question 3).
		writeError(w, http.StatusInternalServerError, "global task row missing")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	h.deriveGlobalState(r.Context(), &g)
	writeJSON(w, http.StatusOK, g)
}
