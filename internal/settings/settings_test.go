package settings_test

import (
	"database/sql"
	"path/filepath"
	"testing"

	"kamacu/internal/settings"
	"kamacu/internal/store"
)

// testDB opens a temp SQLite database with all migrations applied.
func testDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := store.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	if err := store.Migrate(db); err != nil {
		t.Fatalf("Migrate: %v", err)
	}
	return db
}

func TestDefaultsValues(t *testing.T) {
	want := map[string]string{
		settings.KeyAgentExtraParams:  "--dangerously-skip-permissions",
		settings.KeyWorktreeBase:      "~/.kangent/worktrees/",
		settings.KeyShell:             "bash",
		settings.KeyBranchTemplate:    "task/{slug}-{id}",
		settings.KeyDoneSessionTTL:    "24h", // REAP-01/D-91
		settings.KeyGithubIntegration: "on",  // GHSET-01/D-01: absent row = on
		settings.KeyPRReviewSeed:      `Review PR #<n> "<title>". Summarize the changes, then flag bugs, risky changes, and missing tests.`, // GHREV/12-07
	}
	for k, v := range want {
		if got := settings.Defaults[k]; got != v {
			t.Errorf("Defaults[%q] = %q, want %q", k, got, v)
		}
	}
	if len(settings.Defaults) != len(want) {
		t.Errorf("Defaults has %d keys, want %d", len(settings.Defaults), len(want))
	}
}

func TestGetReturnsDefaultOnFreshDB(t *testing.T) {
	db := testDB(t)
	for key, def := range settings.Defaults {
		got, err := settings.Get(db, key)
		if err != nil {
			t.Fatalf("Get(%q): %v", key, err)
		}
		if got != def {
			t.Errorf("Get(%q) on fresh DB = %q, want default %q", key, got, def)
		}
	}
}

func TestSetGetRoundTripAndUpsert(t *testing.T) {
	db := testDB(t)

	if err := settings.Set(db, settings.KeyBranchTemplate, "wip/{slug}-{id}"); err != nil {
		t.Fatalf("Set: %v", err)
	}
	got, err := settings.Get(db, settings.KeyBranchTemplate)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got != "wip/{slug}-{id}" {
		t.Errorf("Get after Set = %q, want %q", got, "wip/{slug}-{id}")
	}

	// Set again overwrites (upsert).
	if err := settings.Set(db, settings.KeyBranchTemplate, "feat/{slug}-{id}"); err != nil {
		t.Fatalf("Set (overwrite): %v", err)
	}
	got, err = settings.Get(db, settings.KeyBranchTemplate)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got != "feat/{slug}-{id}" {
		t.Errorf("Get after second Set = %q, want %q", got, "feat/{slug}-{id}")
	}

	// Exactly one row for the key (upsert, not insert-duplicate).
	var n int
	if err := db.QueryRow(`SELECT COUNT(*) FROM settings WHERE key = ?`, settings.KeyBranchTemplate).Scan(&n); err != nil {
		t.Fatalf("count rows: %v", err)
	}
	if n != 1 {
		t.Errorf("rows for key = %d, want 1", n)
	}
}

func TestSetEmptyStringIsPreservedNotDefaulted(t *testing.T) {
	// Pitfall 1: stored "" means "no extra parameters" — only an absent row
	// (sql.ErrNoRows) falls back to the default.
	db := testDB(t)

	if err := settings.Set(db, settings.KeyAgentExtraParams, ""); err != nil {
		t.Fatalf("Set empty: %v", err)
	}
	got, err := settings.Get(db, settings.KeyAgentExtraParams)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got != "" {
		t.Errorf("Get after storing empty string = %q, want \"\" (NOT the default %q)",
			got, settings.Defaults[settings.KeyAgentExtraParams])
	}
}

func TestGetAllMergesStoredOverDefaults(t *testing.T) {
	db := testDB(t)

	// Fresh DB: every known key present, all defaults.
	all, err := settings.GetAll(db)
	if err != nil {
		t.Fatalf("GetAll: %v", err)
	}
	if len(all) != len(settings.Defaults) {
		t.Errorf("GetAll returned %d keys, want %d", len(all), len(settings.Defaults))
	}
	for k, def := range settings.Defaults {
		if all[k] != def {
			t.Errorf("GetAll[%q] = %q, want default %q", k, all[k], def)
		}
	}

	// Stored values overlay defaults; untouched keys keep defaults.
	if err := settings.Set(db, settings.KeyShell, "bash"); err != nil {
		t.Fatalf("Set shell: %v", err)
	}
	if err := settings.Set(db, settings.KeyAgentExtraParams, ""); err != nil {
		t.Fatalf("Set extra params: %v", err)
	}
	all, err = settings.GetAll(db)
	if err != nil {
		t.Fatalf("GetAll: %v", err)
	}
	if all[settings.KeyAgentExtraParams] != "" {
		t.Errorf("GetAll[agent_extra_params] = %q, want \"\"", all[settings.KeyAgentExtraParams])
	}
	if all[settings.KeyBranchTemplate] != settings.Defaults[settings.KeyBranchTemplate] {
		t.Errorf("GetAll[branch_template] = %q, want untouched default", all[settings.KeyBranchTemplate])
	}
}

func TestSetRejectsUnknownKey(t *testing.T) {
	db := testDB(t)
	if err := settings.Set(db, "nope", "x"); err == nil {
		t.Error("Set with unknown key succeeded, want error")
	}
}

func TestSettingsSurviveReopen(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "test.db")
	db, err := store.Open(dbPath)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	if err := store.Migrate(db); err != nil {
		t.Fatalf("Migrate: %v", err)
	}
	if err := settings.Set(db, settings.KeyWorktreeBase, "/srv/worktrees"); err != nil {
		t.Fatalf("Set: %v", err)
	}
	if err := db.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	db2, err := store.Open(dbPath)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	defer db2.Close()
	got, err := settings.Get(db2, settings.KeyWorktreeBase)
	if err != nil {
		t.Fatalf("Get after reopen: %v", err)
	}
	if got != "/srv/worktrees" {
		t.Errorf("Get after reopen = %q, want %q", got, "/srv/worktrees")
	}
}

func TestExpandHome(t *testing.T) {
	home, err := settings.ExpandHome("~/x")
	if err != nil {
		t.Fatalf("ExpandHome: %v", err)
	}
	if home == "~/x" || home == "" {
		t.Errorf("ExpandHome(~/x) = %q, want expanded path", home)
	}
	abs, err := settings.ExpandHome("/abs/path")
	if err != nil {
		t.Fatalf("ExpandHome: %v", err)
	}
	if abs != "/abs/path" {
		t.Errorf("ExpandHome(/abs/path) = %q, want unchanged", abs)
	}
}
