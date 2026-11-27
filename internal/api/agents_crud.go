package api

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"kamacu/internal/settings"
)

// Agent is the JSON shape of an agents row (migration 00013). Mirrors the
// Workspace struct's bool-from-INTEGER idiom. IsDefault marks the protected
// default agent (D-M001-3: identified by the flag, rename-proof). IsSystem
// marks the non-deletable Claude seed.
type Agent struct {
	Models                []string `json:"models"`
	ModelArgument         string   `json:"model_argument"`
	ModelDiscoveryCommand string   `json:"model_discovery_command"`
	ID                    int64    `json:"id"`
	Name                  string   `json:"name"`
	Command               string   `json:"command"`
	Engine                string   `json:"engine"`
	// ExtraParams is claude-engine only (M001 gate follow-up): extra argv flags
	// appended after the fixed claude flags at spawn. Custom agents ignore it.
	ExtraParams string `json:"extra_params"`
	IsDefault   bool   `json:"is_default"`
	IsSystem    bool   `json:"is_system"`
	CreatedAt   string `json:"created_at"`
	UpdatedAt   string `json:"updated_at"`
}

// agentColumns is the canonical SELECT/RETURNING column list; its ORDER must
// match the scanAgent Scan order.
const agentColumns = `id, name, command, engine, extra_params, is_default, is_system, created_at, updated_at, models, model_argument, model_discovery_command`

// scanAgent scans one agents row. is_default/is_system come back as INTEGER 0/1;
// scan into ints and map to bool (the managedInt != 0 idiom in scanWorkspace).
func scanAgent(row interface{ Scan(...any) error }) (Agent, error) {
	var a Agent
	var isDefaultInt, isSystemInt int
	var models string
	err := row.Scan(&a.ID, &a.Name, &a.Command, &a.Engine, &a.ExtraParams, &isDefaultInt, &isSystemInt, &a.CreatedAt, &a.UpdatedAt, &models, &a.ModelArgument, &a.ModelDiscoveryCommand)
	if err == nil {
		err = json.Unmarshal([]byte(models), &a.Models)
	}
	if a.Models == nil {
		a.Models = []string{}
	}
	a.IsDefault = isDefaultInt != 0
	a.IsSystem = isSystemInt != 0
	return a, err
}

// agentCRUDHandlers is the /api/agents CRUD surface, mirroring workspaceHandlers.
// The pre-existing agentHandlers (agents.go) owns the STATUS endpoint; this type
// owns the configuration CRUD -- distinct concerns, distinct types.
type agentCRUDHandlers struct{ db *sql.DB }

// list handles GET /api/agents -- every agent, default-first then name-sorted
// (COLLATE NOCASE). Default-first so the UI can surface the default at the top
// without a second sort. Returned as a JSON array, never nil (mirrors WSNAV-01).
func (h *agentCRUDHandlers) list(w http.ResponseWriter, r *http.Request) {
	rows, err := h.db.Query(`SELECT ` + agentColumns +
		` FROM agents ORDER BY is_default DESC, name COLLATE NOCASE`)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	defer rows.Close()
	agents := []Agent{}
	for rows.Next() {
		a, err := scanAgent(rows)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		agents = append(agents, a)
	}
	if err := rows.Err(); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, agents)
}

// create handles POST /api/agents. Creates a custom agent (name + command); a
// case-insensitive duplicate name is rejected 409. New agents are always
// engine='custom', is_default=0, is_system=0 -- the system/claude seed is only
// ever created by migration 00013 / BackfillAgents, never by this handler.
func (h *agentCRUDHandlers) create(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name                  string   `json:"name"`
		Command               string   `json:"command"`
		Models                []string `json:"models"`
		ModelArgument         string   `json:"model_argument"`
		ModelDiscoveryCommand string   `json:"model_discovery_command"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	name := strings.TrimSpace(req.Name)
	if name == "" {
		writeError(w, http.StatusBadRequest, "name is required")
		return
	}
	command := strings.TrimSpace(req.Command)
	if command == "" {
		writeError(w, http.StatusBadRequest, "command is required")
		return
	}
	// Case-insensitive duplicate pre-check (the COLLATE NOCASE unique index is
	// the ultimate backstop; the pre-check gives a clean message).
	var exists int
	if err := h.db.QueryRow(`SELECT 1 FROM agents WHERE name = ? COLLATE NOCASE`, name).Scan(&exists); err == nil {
		writeError(w, http.StatusConflict, "an agent with that name already exists")
		return
	} else if !errors.Is(err, sql.ErrNoRows) {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	models, err := validateModelConfig(req.Models, strings.TrimSpace(req.ModelArgument))
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	a, err := scanAgent(h.db.QueryRow(
		`INSERT INTO agents (name, command, engine, is_default, is_system, models, model_argument, model_discovery_command) VALUES (?, ?, 'custom', 0, 0, ?, ?, ?) RETURNING `+agentColumns,
		name, command, models, strings.TrimSpace(req.ModelArgument), strings.TrimSpace(req.ModelDiscoveryCommand)))
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, a)
}

// update handles PATCH /api/agents/{id}. For a system (claude) agent the engine
// is immutable -- only name + command are editable (the hook/resume plumbing is
// internal, not user-editable); for a custom agent name + command + engine are
// editable. A case-insensitive name collision with a DIFFERENT agent is 409.
func (h *agentCRUDHandlers) update(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	var req struct {
		Name                  *string   `json:"name"`
		Command               *string   `json:"command"`
		Engine                *string   `json:"engine"`
		ExtraParams           *string   `json:"extra_params"`
		Models                *[]string `json:"models"`
		ModelArgument         *string   `json:"model_argument"`
		ModelDiscoveryCommand *string   `json:"model_discovery_command"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	if req.Name == nil && req.Command == nil && req.Engine == nil && req.ExtraParams == nil && req.Models == nil && req.ModelArgument == nil && req.ModelDiscoveryCommand == nil {
		writeError(w, http.StatusBadRequest, "nothing to update")
		return
	}

	// Load the existing row to enforce the system-engine lock and build the
	// conditional UPDATE (mirrors the projects partial-PATCH pattern).
	var curName, curCommand, curEngine, curExtra, curModels, curModelArgument, curDiscovery string
	var isSystem int
	err := h.db.QueryRow(`SELECT name, command, engine, extra_params, is_system, models, model_argument, model_discovery_command FROM agents WHERE id = ?`, id).
		Scan(&curName, &curCommand, &curEngine, &curExtra, &isSystem, &curModels, &curModelArgument, &curDiscovery)
	if errors.Is(err, sql.ErrNoRows) {
		writeError(w, http.StatusNotFound, "agent not found")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	// System agents (the claude seed) lock the engine -- only name/command move.
	// An attempt to change a system agent's engine is rejected 400.
	if isSystem == 1 && req.Engine != nil && *req.Engine != curEngine {
		writeError(w, http.StatusBadRequest, "the system agent's engine can't be changed")
		return
	}
	// Reject any engine value other than 'claude' | 'custom' (forward-safe).
	if req.Engine != nil {
		e := strings.TrimSpace(*req.Engine)
		if e != curEngine && e != "claude" && e != "custom" {
			writeError(w, http.StatusBadRequest, "engine must be 'claude' or 'custom'")
			return
		}
	}

	name := curName
	if req.Name != nil {
		name = strings.TrimSpace(*req.Name)
		if name == "" {
			writeError(w, http.StatusBadRequest, "name is required")
			return
		}
		// Duplicate pre-check excluding self.
		var exists int
		if err := h.db.QueryRow(`SELECT 1 FROM agents WHERE name = ? COLLATE NOCASE AND id != ?`, name, id).Scan(&exists); err == nil {
			writeError(w, http.StatusConflict, "an agent with that name already exists")
			return
		} else if !errors.Is(err, sql.ErrNoRows) {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
	}
	command := curCommand
	if req.Command != nil {
		command = strings.TrimSpace(*req.Command)
		if command == "" {
			writeError(w, http.StatusBadRequest, "command is required")
			return
		}
	}
	engine := curEngine
	if req.Engine != nil {
		engine = strings.TrimSpace(*req.Engine)
	}
	// Extra params is claude-engine config; an explicit empty string clears it,
	// nil = untouched.
	extra := curExtra
	if req.ExtraParams != nil {
		extra = *req.ExtraParams
	}

	if req.ModelArgument != nil {
		curModelArgument = strings.TrimSpace(*req.ModelArgument)
	}
	if req.ModelDiscoveryCommand != nil {
		curDiscovery = strings.TrimSpace(*req.ModelDiscoveryCommand)
	}
	var modelList []string
	if err := json.Unmarshal([]byte(curModels), &modelList); err != nil {
		writeError(w, http.StatusInternalServerError, "invalid stored model list")
		return
	}
	if req.Models != nil {
		modelList = *req.Models
	}
	if engine == "pi" {
		for _, model := range modelList {
			if err := validatePiModel(model); err != nil {
				writeError(w, http.StatusBadRequest, err.Error())
				return
			}
		}
		curModelArgument = "--model {{model}}"
	}
	modelJSON, err := validateModelConfig(modelList, curModelArgument)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	a, err := scanAgent(h.db.QueryRow(
		`UPDATE agents SET name = ?, command = ?, engine = ?, extra_params = ?, models = ?, model_argument = ?, model_discovery_command = ?, updated_at = strftime('%Y-%m-%dT%H:%M:%fZ','now') WHERE id = ? RETURNING `+agentColumns,
		name, command, engine, extra, modelJSON, curModelArgument, curDiscovery, id))
	if errors.Is(err, sql.ErrNoRows) {
		writeError(w, http.StatusNotFound, "agent not found")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, a)
}

// delete removes an unused, non-system agent. Checks and removal share a
// transaction so a workspace or task cannot start using the agent in between.
func (h *agentCRUDHandlers) delete(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	ctx := r.Context()
	tx, err := h.db.BeginTx(ctx, nil)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	defer tx.Rollback()

	var isSystem int
	err = tx.QueryRowContext(ctx, `SELECT is_system FROM agents WHERE id = ?`, id).Scan(&isSystem)
	if errors.Is(err, sql.ErrNoRows) {
		writeError(w, http.StatusNotFound, "agent not found")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	// System guard: the claude seed is non-deletable.
	if isSystem == 1 {
		writeError(w, http.StatusConflict, "the system agent can't be deleted")
		return
	}
	// In-use guard: explicit count -> clean message (block-until-unassigned).
	var n int
	if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM projects WHERE agent_id = ?`, id).Scan(&n); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if n > 0 {
		writeError(w, http.StatusConflict, fmt.Sprintf("reassign its %d project(s) first", n))
		return
	}
	if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM workspaces WHERE default_agent_id = ?`, id).Scan(&n); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if n > 0 {
		writeError(w, http.StatusConflict, fmt.Sprintf("change the default agent in %d workspace(s) first", n))
		return
	}
	// Global task guard: the singleton references exactly one agent, always —
	// a count-free 409 with the locked D-09 user-facing string (UI-SPEC
	// Copywriting row 1) ahead of the 00017 ON DELETE RESTRICT backstop. The
	// FK is the invariant of last resort; this guard runs first for the clean
	// message (the projects.agent_id pattern).
	var g int
	if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM global_task WHERE agent_id = ?`, id).Scan(&g); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if g > 0 {
		writeError(w, http.StatusConflict, "reassign the Scratchpad agent first")
		return
	}
	if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM task_agents WHERE agent_id = ?`, id).Scan(&n); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if n > 0 {
		writeError(w, http.StatusConflict, fmt.Sprintf("agent is assigned to %d task(s)", n))
		return
	}
	// Remaining snapshots are masked by explicit task or project pins.
	if _, err := tx.ExecContext(ctx, `UPDATE tasks SET workspace_agent_id = NULL WHERE workspace_agent_id = ?`, id); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM agents WHERE id = ?`, id); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if err := tx.Commit(); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// readAgentID parses a supplied setting. A JSON null clears the setting.
func readAgentID(w http.ResponseWriter, db *sql.DB, raw json.RawMessage) (*int64, bool) {
	var id *int64
	if err := json.Unmarshal(raw, &id); err != nil {
		writeError(w, http.StatusBadRequest, "agent ID must be an integer or null")
		return nil, false
	}
	if id == nil {
		return nil, true
	}
	var exists int
	err := db.QueryRow(`SELECT 1 FROM agents WHERE id = ?`, *id).Scan(&exists)
	if errors.Is(err, sql.ErrNoRows) {
		writeError(w, http.StatusBadRequest, "agent not found")
		return nil, false
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return nil, false
	}
	return id, true
}

// setDefault handles POST /api/agents/{id}/default (R019). Flips the is_default
// flag to the target in a single transactional UPDATE so exactly one row holds
// the flag at all times (the exactly-one invariant, D-M001-3).
func (h *agentCRUDHandlers) setDefault(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	// Confirm the target exists (404 before any mutation) and isn't already the
	// default (cheap no-op short-circuit, keeps the invariant honest).
	var exists int
	err := h.db.QueryRow(`SELECT 1 FROM agents WHERE id = ?`, id).Scan(&exists)
	if errors.Is(err, sql.ErrNoRows) {
		writeError(w, http.StatusNotFound, "agent not found")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	tx, err := h.db.Begin()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	defer tx.Rollback() // safe: a committed tx rolls back as a no-op.
	// Clear all defaults, then set the target -- two statements, one tx. Under
	// SetMaxOpenConns(1) this is atomic with no concurrent writer.
	if _, err := tx.Exec(`UPDATE agents SET is_default = 0`); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	res, err := tx.Exec(`UPDATE agents SET is_default = 1, updated_at = strftime('%Y-%m-%dT%H:%M:%fZ','now') WHERE id = ?`, id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if rows, _ := res.RowsAffected(); rows == 0 {
		writeError(w, http.StatusNotFound, "agent not found")
		return
	}
	if err := tx.Commit(); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	// Return the full updated agent (default flag now set) for the UI.
	a, err := scanAgent(h.db.QueryRow(`SELECT `+agentColumns+` FROM agents WHERE id = ?`, id))
	if err != nil {
		// Invariant held; the row vanished only in a race -- surface as 500.
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, a)
}

// renderAgentCommand substitutes the {{worktree}} and {{session_id}} placeholders
// in a custom agent's command template by shell-splitting FIRST (settings.Tokenize)
// settings.Tokenize (reused -- no new deps, D007). Returns the full argv tokens
// (binary name at [0]). The worktree is ALWAYS the spawn cwd (set by the
// handler), so {{worktree}} lets a user pass an explicit path flag if their
// agent needs one; {{session_id}} is a stable uuid for users who wire resume-
// style flags (custom agents don't resume, but the placeholder is honored).
//
// Unknown placeholders (e.g. {{foo}}) are left literal -- safer than erroring on
// a user's valid-but-unsupported token (D005 degrade-don't-break at the config
// layer). An empty/whitespace template yields nil (the spawn handler rejects it
// with a clear error before reaching the session layer).
//
// NOTE: this lives in the api package (not session) so session stays a leaf with
// no settings import; the API handler owns template rendering + tokenization and
// hands session already-split tokens.
func renderAgentCommand(template, worktree, sessionID string) []string {
	// Tokenize FIRST, then substitute per token: a substituted value with
	// spaces (e.g. a worktree path under a directory with spaces) stays a
	// single token rather than re-splitting. --cwd={{worktree}} and a bare
	// {{worktree}} both survive intact. (Pre-substitution string replacement
	// would break paths with spaces; this is the robust order.)
	tokens := settings.Tokenize(template)
	for i, tok := range tokens {
		tok = strings.ReplaceAll(tok, "{{worktree}}", worktree)
		tok = strings.ReplaceAll(tok, "{{session_id}}", sessionID)
		tokens[i] = tok
	}
	return tokens
}
