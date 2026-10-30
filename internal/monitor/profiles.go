package monitor

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/url"
	"regexp"
	"strings"
)

type Profile struct {
	ID            int64  `json:"id"`
	Name          string `json:"name"`
	Backend       string `json:"backend"`
	Model         string `json:"model"`
	Endpoint      string `json:"endpoint"`
	Executable    string `json:"executable"`
	CredentialEnv string `json:"credential_env"`
	Revision      int64  `json:"revision"`
}

const profileCols = "id,name,backend,model,endpoint,executable,credential_env,revision"

func scanProfile(row interface{ Scan(...any) error }) (p Profile, err error) {
	err = row.Scan(&p.ID, &p.Name, &p.Backend, &p.Model, &p.Endpoint, &p.Executable, &p.CredentialEnv, &p.Revision)
	return
}
func ValidateProfile(p Profile) error {
	if strings.TrimSpace(p.Name) == "" || len(p.Name) > 100 {
		return errors.New("profile name is required (up to 100 characters)")
	}
	switch p.Backend {
	case "claude_cli", "codex_cli":
	case "openai_compatible", "anthropic_api":
		u, e := url.Parse(p.Endpoint)
		if e != nil || u.Host == "" || (u.Scheme != "http" && u.Scheme != "https") || u.User != nil || u.RawQuery != "" || u.Fragment != "" {
			return errors.New("endpoint must be an HTTP(S) base URL without credentials or query parameters")
		}
		if p.Model == "" {
			return errors.New("model is required for API endpoints")
		}
	default:
		return errors.New("unknown monitoring backend")
	}
	if p.CredentialEnv != "" && !regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`).MatchString(p.CredentialEnv) {
		return errors.New("credential must be an environment variable name, not a secret value")
	}
	if len(p.Model) > 200 || len(p.Endpoint) > 2000 || len(p.Executable) > 2000 {
		return errors.New("profile field too long")
	}
	return nil
}
func ListProfiles(ctx context.Context, db *sql.DB) ([]Profile, error) {
	rows, err := db.QueryContext(ctx, "SELECT "+profileCols+" FROM monitor_profiles ORDER BY name,id")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	ps := []Profile{}
	for rows.Next() {
		p, e := scanProfile(rows)
		if e != nil {
			return nil, e
		}
		ps = append(ps, p)
	}
	return ps, rows.Err()
}
func SaveProfile(ctx context.Context, db *sql.DB, p Profile) (Profile, error) {
	if err := ValidateProfile(p); err != nil {
		return p, err
	}
	if p.ID == 0 {
		r, e := db.ExecContext(ctx, `INSERT INTO monitor_profiles(name,backend,model,endpoint,executable,credential_env) VALUES(?,?,?,?,?,?)`, p.Name, p.Backend, p.Model, p.Endpoint, p.Executable, p.CredentialEnv)
		if e != nil {
			return p, e
		}
		p.ID, _ = r.LastInsertId()
	} else {
		r, e := db.ExecContext(ctx, `UPDATE monitor_profiles SET name=?,backend=?,model=?,endpoint=?,executable=?,credential_env=?,revision=revision+1 WHERE id=?`, p.Name, p.Backend, p.Model, p.Endpoint, p.Executable, p.CredentialEnv, p.ID)
		if e != nil {
			return p, e
		}
		n, _ := r.RowsAffected()
		if n == 0 {
			return p, sql.ErrNoRows
		}
	}
	return scanProfile(db.QueryRowContext(ctx, "SELECT "+profileCols+" FROM monitor_profiles WHERE id=?", p.ID))
}

// Resolve never falls back from an explicitly configured but failing override.
func Resolve(ctx context.Context, db *sql.DB, taskID int64) (Profile, error) {
	var id sql.NullInt64
	if taskID == 0 {
		err := db.QueryRowContext(ctx, `SELECT default_profile_id FROM monitor_settings WHERE id=1`).Scan(&id)
		if err != nil {
			return Profile{}, err
		}
	} else {
		err := db.QueryRowContext(ctx, `SELECT COALESCE(w.monitor_profile_id,m.default_profile_id) FROM tasks t JOIN projects p ON p.id=t.project_id JOIN workspaces w ON w.id=p.workspace_id CROSS JOIN monitor_settings m WHERE t.id=? AND m.id=1`, taskID).Scan(&id)
		if err != nil {
			return Profile{}, err
		}
	}
	if !id.Valid {
		return Profile{}, errors.New("configure a monitoring model in Settings before starting an agent")
	}
	return scanProfile(db.QueryRowContext(ctx, "SELECT "+profileCols+" FROM monitor_profiles WHERE id=?", id.Int64))
}
func (p Profile) Key() string { return fmt.Sprintf("%d:%d", p.ID, p.Revision) }
