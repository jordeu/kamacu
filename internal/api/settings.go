package api

import (
	"database/sql"
	"encoding/json"
	"net/http"

	"kamacu/internal/settings"
)

// settingEntry is the per-key payload shape (research Pattern 2). Returning
// the default alongside the value lets the client render "Reset to default"
// without hardcoding defaults; options (shell only) is the SHELL-FUT-01
// seam — future shells are server data, zero frontend changes.
type settingEntry struct {
	Value   string   `json:"value"`
	Default string   `json:"default"`
	Options []string `json:"options,omitempty"`
}

// entryFor builds the response entry for key with the given effective value.
func entryFor(key, value string) settingEntry {
	e := settingEntry{Value: value, Default: settings.Defaults[key]}
	if key == settings.KeyShell {
		// Same function as save-time validation — one source of truth.
		e.Options = settings.AllowedShells()
	}
	return e
}

// SettingsRoutes registers the global settings endpoints.
func SettingsRoutes(mux *http.ServeMux, db *sql.DB) {
	h := &settingsHandlers{db: db}
	mux.HandleFunc("GET /api/settings", h.getAll)
	mux.HandleFunc("PUT /api/settings/{key}", h.put)
}

type settingsHandlers struct{ db *sql.DB }

// getAll handles GET /api/settings.
func (h *settingsHandlers) getAll(w http.ResponseWriter, r *http.Request) {
	all, err := settings.GetAll(h.db)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	out := make(map[string]settingEntry, len(all))
	for key, value := range all {
		out[key] = entryFor(key, value)
	}
	writeJSON(w, http.StatusOK, out)
}

// put handles PUT /api/settings/{key}. Validation errors surface verbatim
// through the {"error": msg} pipe — the canonical UI-SPEC copy reaches the
// frontend's inline error untouched (client.ts extracts body.error).
func (h *settingsHandlers) put(w http.ResponseWriter, r *http.Request) {
	key := r.PathValue("key")
	if _, ok := settings.Defaults[key]; !ok {
		writeError(w, http.StatusNotFound, "unknown setting")
		return
	}
	var req struct {
		Value string `json:"value"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	// Validate in the handler so a failure is unambiguously a 400; Set
	// re-validates before writing, preserving its validate-before-write
	// guarantee for any other caller (BRANCH-02).
	if err := settings.Validate(key, req.Value); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := settings.Set(h.db, key, req.Value); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, entryFor(key, req.Value))
}
