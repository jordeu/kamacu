// Package settings stores global app settings in a SQLite KV table.
//
// Defaults live in code: an absent row reads as the default, so adding a
// setting or changing a default never needs a migration. Values are stored
// raw (e.g. worktree base keeps its "~" form); expansion happens at use.
package settings

import (
	"database/sql"
	"errors"
	"fmt"
)

// Setting keys. The set of valid keys is exactly the Defaults map's keys.
const (
	KeyAgentExtraParams  = "agent_extra_params"
	KeyWorktreeBase      = "worktree_base"
	KeyShell             = "shell"
	KeyBranchTemplate    = "branch_template"
	KeyDoneSessionTTL    = "done_session_ttl"
	KeyGithubIntegration = "github_integration"
)

// Defaults maps each setting key to its code default. Absent row = default.
var Defaults = map[string]string{
	// AGENT-02: intentionally defaults the skip-permissions flag ON — a
	// deliberate reversal of v1.0's D-51 interactive-by-default posture.
	// The user can clear the field to restore permission prompts.
	KeyAgentExtraParams: "--dangerously-skip-permissions",
	KeyWorktreeBase:     "~/.kangent/worktrees/", // WT-01; stored raw, expanded at use
	KeyShell:            "bash",                  // SHELL-01
	KeyBranchTemplate:   "task/{slug}-{id}",      // BRANCH-01
	// REAP-01/D-91: Done-TTL reaper window. Go-style duration; empty/"0"/
	// "never" disable reaping (see ParseDoneSessionTTL). Read-at-use by 09-05.
	KeyDoneSessionTTL: "24h",
	// GHSET-01/D-01: global GitHub integration toggle; absent row = on
	KeyGithubIntegration: "on",
}

// Get returns the stored value for key, or the code default when no row
// exists. Only sql.ErrNoRows falls back to the default — a stored empty
// string is a real value (Pitfall 1: "" for agent_extra_params means
// "no extra parameters", not "use the default").
func Get(db *sql.DB, key string) (string, error) {
	var v string
	err := db.QueryRow(`SELECT value FROM settings WHERE key = ?`, key).Scan(&v)
	if errors.Is(err, sql.ErrNoRows) {
		return Defaults[key], nil
	}
	if err != nil {
		return "", err
	}
	return v, nil
}

// GetAll returns every setting key mapped to its effective value:
// the stored row where one exists, the code default otherwise.
func GetAll(db *sql.DB) (map[string]string, error) {
	all := make(map[string]string, len(Defaults))
	for k, v := range Defaults {
		all[k] = v
	}
	rows, err := db.Query(`SELECT key, value FROM settings`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var k, v string
		if err := rows.Scan(&k, &v); err != nil {
			return nil, err
		}
		if _, ok := all[k]; ok { // ignore stale rows for keys we no longer know
			all[k] = v
		}
	}
	return all, rows.Err()
}

// Set validates then upserts value under key. Unknown keys are rejected,
// and validation runs BEFORE the write — an invalid value is never stored
// (BRANCH-02 save-time guarantee).
func Set(db *sql.DB, key, value string) error {
	if _, ok := Defaults[key]; !ok {
		return fmt.Errorf("unknown setting %q", key)
	}
	if err := Validate(key, value); err != nil {
		return err
	}
	_, err := db.Exec(
		`INSERT INTO settings(key, value) VALUES(?, ?)
		 ON CONFLICT(key) DO UPDATE SET
		   value = excluded.value,
		   updated_at = strftime('%Y-%m-%dT%H:%M:%fZ','now')`,
		key, value,
	)
	return err
}
