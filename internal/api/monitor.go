package api

import (
	"context"
	"database/sql"
	"encoding/json"
	"io"
	"net/http"
	"strconv"
	"time"

	"kamacu/internal/monitor"
	"kamacu/internal/observation"
)

func MonitorRoutes(mux *http.ServeMux, db *sql.DB, backend monitor.Classifier, metrics ...*monitor.Metrics) {
	mux.HandleFunc("GET /api/monitor", func(w http.ResponseWriter, r *http.Request) {
		ps, err := monitor.ListProfiles(r.Context(), db)
		if err != nil {
			writeError(w, 500, "cannot load monitor profiles")
			return
		}
		var global sql.NullInt64
		if err = db.QueryRowContext(r.Context(), `SELECT default_profile_id FROM monitor_settings WHERE id=1`).Scan(&global); err != nil {
			writeError(w, 500, "cannot load monitor default")
			return
		}
		type override struct {
			ID      int64  `json:"id"`
			Name    string `json:"name"`
			Profile *int64 `json:"profile_id"`
		}
		workspaces := []override{}
		rows, err := db.QueryContext(r.Context(), `SELECT id,name,monitor_profile_id FROM workspaces ORDER BY name`)
		if err != nil {
			writeError(w, 500, "cannot load workspace models")
			return
		}
		defer rows.Close()
		for rows.Next() {
			var x override
			var id sql.NullInt64
			if err = rows.Scan(&x.ID, &x.Name, &id); err != nil {
				writeError(w, 500, "cannot load workspace models")
				return
			}
			if id.Valid {
				x.Profile = &id.Int64
			}
			workspaces = append(workspaces, x)
		}
		if rows.Err() != nil {
			writeError(w, 500, "cannot load workspace models")
			return
		}
		var def *int64
		if global.Valid {
			def = &global.Int64
		}
		var usage map[int64]monitor.ProfileMetrics
		if len(metrics) > 0 {
			usage = metrics[0].Snapshot()
		}
		writeJSON(w, 200, map[string]any{"usage": usage, "profiles": ps, "default_profile_id": def, "workspaces": workspaces})
	})
	mux.HandleFunc("POST /api/monitor/profiles", func(w http.ResponseWriter, r *http.Request) {
		var p monitor.Profile
		if !monitorDecode(w, r, &p) {
			return
		}
		out, err := monitor.SaveProfile(r.Context(), db, p)
		if err != nil {
			writeError(w, 400, err.Error())
			return
		}
		writeJSON(w, 200, out)
	})
	mux.HandleFunc("DELETE /api/monitor/profiles/{id}", func(w http.ResponseWriter, r *http.Request) {
		id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
		if err != nil {
			writeError(w, 400, "invalid profile")
			return
		}
		result, err := db.ExecContext(r.Context(), `DELETE FROM monitor_profiles WHERE id=?`, id)
		if err != nil {
			writeError(w, 409, "profile is still used by a workspace or global default")
			return
		}
		n, _ := result.RowsAffected()
		if n == 0 {
			writeError(w, 404, "profile not found")
			return
		}
		w.WriteHeader(204)
	})
	mux.HandleFunc("PUT /api/monitor/selection", func(w http.ResponseWriter, r *http.Request) {
		var in struct {
			WorkspaceID int64  `json:"workspace_id"`
			ProfileID   *int64 `json:"profile_id"`
		}
		if !monitorDecode(w, r, &in) {
			return
		}
		var result sql.Result
		var err error
		if in.WorkspaceID == 0 {
			if in.ProfileID == nil {
				writeError(w, 400, "global default is required")
				return
			}
			result, err = db.ExecContext(r.Context(), `UPDATE monitor_settings SET default_profile_id=? WHERE id=1`, in.ProfileID)
		} else {
			result, err = db.ExecContext(r.Context(), `UPDATE workspaces SET monitor_profile_id=? WHERE id=?`, in.ProfileID, in.WorkspaceID)
		}
		if err != nil {
			writeError(w, 400, "profile does not exist")
			return
		}
		n, _ := result.RowsAffected()
		if n == 0 {
			writeError(w, 404, "workspace not found")
			return
		}
		w.WriteHeader(204)
	})
	mux.HandleFunc("POST /api/monitor/test", func(w http.ResponseWriter, r *http.Request) {
		var p monitor.Profile
		if !monitorDecode(w, r, &p) {
			return
		}
		if err := monitor.ValidateProfile(p); err != nil {
			writeError(w, 400, err.Error())
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
		defer cancel()
		start := time.Now()
		d, err := backend.Classify(ctx, p, observation.Snapshot{Screen: "Would you like me to continue?\n> ", At: time.Now()})
		if err != nil {
			writeError(w, 502, err.Error())
			return
		}
		if d.State != "waiting" || d.Kind != "question" {
			writeError(w, 502, "model did not recognize the sample question")
			return
		}
		writeJSON(w, 200, map[string]any{"ok": true, "latency_ms": time.Since(start).Milliseconds(), "authentication": "Uses the installed CLI's current authentication, or the configured API credential"})
	})
}
func monitorDecode(w http.ResponseWriter, r *http.Request, v any) bool {
	r.Body = http.MaxBytesReader(w, r.Body, 16000)
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if dec.Decode(v) != nil || dec.Decode(new(any)) != io.EOF {
		writeError(w, 400, "invalid monitor configuration")
		return false
	}
	return true
}
