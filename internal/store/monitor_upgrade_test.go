package store

import (
	"path/filepath"
	"testing"

	"github.com/pressly/goose/v3"
)

func TestMonitorEpisodeInputUpgrade(t *testing.T) {
	db, err := Open(filepath.Join(t.TempDir(), "upgrade.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	goose.SetBaseFS(migrationsFS)
	if err = goose.SetDialect("sqlite3"); err != nil {
		t.Fatal(err)
	}
	if err = goose.UpTo(db, "migrations", 28); err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec(`INSERT INTO monitor_episodes(session_id,episode_id,fingerprint,decision,seen,active) VALUES('session','episode','attention','{}',1,1)`); err != nil {
		t.Fatal(err)
	}
	if err = Migrate(db); err != nil {
		t.Fatal(err)
	}
	var id string
	var seen bool
	var input uint64
	if err = db.QueryRow(`SELECT episode_id,seen,input_generation FROM monitor_episodes WHERE session_id='session'`).Scan(&id, &seen, &input); err != nil {
		t.Fatal(err)
	}
	if id != "episode" || !seen || input != 0 {
		t.Fatal("upgrade lost existing episode", id, seen, input)
	}
}
