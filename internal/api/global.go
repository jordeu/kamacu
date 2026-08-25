package api

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"strings"

	"kamacu/internal/session"
	"kamacu/internal/settings"
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
// GET reads config + derived live state (D-17); PUT is the single partial
// config update (D-20) — folder root, managed clone, clear, default agent —
// behind the live-session 409 gate (GCONF-04).
func GlobalRoutes(mux *http.ServeMux, db *sql.DB, mgr *session.Manager, tmuxClient tmux.Client) {
	g := &globalHandlers{db: db, mgr: mgr, tmuxClient: tmuxClient}
	mux.HandleFunc("GET /api/global", g.get)
	mux.HandleFunc("PUT /api/global", g.put)
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

// globalLiveBlockers returns why a root change must be refused (GCONF-04 /
// D-13..D-15): one {kind, target} blocker per LIVE global session surface,
// reusing the app-wide gated-delete grammar (deleteBlocker, projects.go)
// verbatim — the locked reading of D-15 (14-RESEARCH Open Question 1; the
// 13-CONTEXT string sketch was illustrative).
//
// Today ONLY the tmux half can be non-empty: no global PTY can exist until
// Phase 15's spawn path lands Info.Global + ListGlobal(), which widens this
// helper in place (the mgr field is already carried for that seam, D-19).
// Until then the manager half is a documented zero-count seam. The gate
// only COUNTS — it never stops anything (stopping is the user's job; the
// 409 copy says so).
func (h *globalHandlers) globalLiveBlockers(ctx context.Context) []deleteBlocker {
	var blockers []deleteBlocker
	for _, name := range h.liveGlobalTmuxNames(ctx) {
		blockers = append(blockers, deleteBlocker{Kind: "sessions", Target: name})
	}
	return blockers
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

// put handles PUT /api/global — the single partial config endpoint (D-20).
// Pointer fields: omitted = untouched. root_path:"" is THE CLEAR (D-21 —
// reset both root columns), a non-empty root_path is the folder variant,
// agent_id sets the default agent (D-24). The live-session 409 gate guards
// every root change AND the clear (GCONF-04), and any successful root
// change clears the resume ids in the SAME single UPDATE (D-16).
func (h *globalHandlers) put(w http.ResponseWriter, r *http.Request) {
	var req struct {
		RootPath *string `json:"root_path"`
		AgentID  *int64  `json:"agent_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	// Branch FIRST, validate AFTER the branch knows what it holds (14-RESEARCH
	// Pitfall 1 — a naive validate-then-branch sends root_path:"" into
	// validateRepoPath and 400s "path must be absolute" in the wrong branch).
	if req.RootPath == nil && req.AgentID == nil {
		writeError(w, http.StatusBadRequest, "nothing to update")
		return
	}
	rootSupplied := req.RootPath != nil

	// GCONF-04 gate: any live global session blocks a root change or clear —
	// NOTHING is mutated, and the check runs before any validation or disk
	// work (D-13: any live global PTY blocks; D-14: exited sessions and
	// persisted resume ids never block — the probe is live-only by
	// construction). The error copy uses the Phase-13-locked "Scratchpad"
	// label (D-09).
	if rootSupplied {
		if blockers := h.globalLiveBlockers(r.Context()); len(blockers) > 0 {
			writeJSON(w, http.StatusConflict, map[string]any{
				"error":   "the Scratchpad root can't be changed while sessions are running",
				"reasons": blockers,
			})
			return
		}
	}

	// Resolve the root variant into newRoot/newRepo. Validation rejects
	// BEFORE anything is appended to sets/args, so a reject leaves the row
	// untouched (the projects.go update idiom).
	var newRoot string
	var newRepo sql.NullString
	if rootSupplied {
		if *req.RootPath == "" {
			// THE CLEAR (D-21): explicit "" resets root_path='' AND
			// github_repo=NULL. No path validation — "" is not a folder.
			newRoot = ""
			newRepo = sql.NullString{}
		} else {
			// FOLDER variant (GCONF-01): git repo required (D-05,
			// validateRepoPath verbatim — no project-overlap guard beyond
			// this, D-26).
			abs, err := validateRepoPath(*req.RootPath)
			if err != nil {
				writeError(w, http.StatusBadRequest, err.Error())
				return
			}
			// D-07 footguns on the POST-expansion absolute value (Pitfall 6):
			// "~" and "~/" expand to $HOME inside validateRepoPath, so one
			// equality catches every home spelling; "/" catches the fs root.
			home, herr := os.UserHomeDir()
			if herr != nil {
				writeError(w, http.StatusInternalServerError, herr.Error())
				return
			}
			if abs == home || abs == "/" {
				writeError(w, http.StatusBadRequest,
					"the root can't be your home directory or the filesystem root")
				return
			}
			// D-27 block: everything under ~/.kamacu is Kamacu-managed
			// machinery — a task worktree gets gated-removed by cleanup, a
			// managed clone belongs to its project — so a global root there
			// is always a trap, never a choice. Reject equality AND the
			// child prefix (Pitfall 5: separator-suffixed, expanded base).
			base, berr := settings.ExpandHome("~/.kamacu")
			if berr != nil {
				writeError(w, http.StatusInternalServerError, berr.Error())
				return
			}
			if abs == base || strings.HasPrefix(abs, base+string(os.PathSeparator)) {
				writeError(w, http.StatusBadRequest,
					"the root can't live inside ~/.kamacu — everything there is Kamacu-managed machinery (a task worktree is gated-removed by cleanup, a managed clone belongs to its project), so a root there would be cleaned up by its owner")
				return
			}
			newRoot = abs
			newRepo = sql.NullString{} // folder ⇒ github_repo NULL (Pitfall 8 — never a stale managed marker)
		}
	}

	// Agent field (D-24/D-25): existence-validated 400, NO session gate (a
	// live global session keeps the agent it was spawned with; the change
	// applies at the next Start), NO resume-id touching (ids are
	// engine-keyed, not agent-keyed — D-16 stays the only clearing path).
	if req.AgentID != nil {
		var exists int
		err := h.db.QueryRow(`SELECT 1 FROM agents WHERE id = ?`, *req.AgentID).Scan(&exists)
		if errors.Is(err, sql.ErrNoRows) {
			writeError(w, http.StatusBadRequest, "agent not found")
			return
		}
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
	}

	// Single final UPDATE — never split (Pitfall 2 anti-pattern: a second
	// write is a partial-failure window). Root changes clear the resume ids
	// unconditionally in the SAME UPDATE (D-16 — no no-op-re-PUT exception);
	// agent-only PUTs never touch them (D-25).
	var sets []string
	var args []any
	if rootSupplied {
		sets = append(sets, "root_path = ?", "github_repo = ?")
		args = append(args, newRoot, newRepo)
		sets = append(sets, "claude_session_id = NULL", "opencode_session_id = NULL")
	}
	if req.AgentID != nil {
		sets = append(sets, "agent_id = ?")
		args = append(args, *req.AgentID)
	}
	sets = append(sets, `updated_at = strftime('%Y-%m-%dT%H:%M:%fZ','now')`)
	args = append(args, 1)
	if _, err := h.db.Exec(`UPDATE global_task SET `+strings.Join(sets, ", ")+` WHERE id = 1`, args...); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	// Re-read through the GET load path so the response IS the GET shape
	// (D-23) — 200, not 201: this is an update of a guaranteed row.
	g, err := h.loadGlobalConfig()
	if errors.Is(err, sql.ErrNoRows) {
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
