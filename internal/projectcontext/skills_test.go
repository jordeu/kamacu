package projectcontext

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"kamacu/internal/store"
)

func TestSharedSkillsOnStartupAndLaunch(t *testing.T) {
	home := t.TempDir()
	dbPath := filepath.Join(home, "custom.db")
	db, err := store.Open(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := db.Exec("CREATE TABLE projects (id INTEGER PRIMARY KEY); INSERT INTO projects VALUES (1), (2)"); err != nil {
		t.Fatal(err)
	}
	f, err := New(home, dbPath)
	if err != nil {
		t.Fatal(err)
	}
	library := filepath.Join(home, "skills")
	if err := os.Mkdir(library, 0700); err != nil {
		t.Fatal(err)
	}
	source := t.TempDir()
	original := filepath.Join(source, "SKILL.md")
	if err := os.WriteFile(original, []byte("first"), 0600); err != nil {
		t.Fatal(err)
	}
	shared := filepath.Join(library, "find-project")
	if err := os.Symlink(source, shared); err != nil {
		t.Fatal(err)
	}
	// Ignore ordinary files and incomplete or dangling entries in the library.
	if err := os.WriteFile(filepath.Join(library, "README"), nil, 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(source, "missing"), filepath.Join(library, "broken")); err != nil {
		t.Fatal(err)
	}

	ctx := context.Background()
	if err := f.PrepareExisting(ctx, db); err != nil {
		t.Fatal(err)
	}
	for _, id := range []int64{1, 2} {
		dir := filepath.Join(f.Directory(id), "skills")
		entries, err := os.ReadDir(dir)
		if err != nil || len(entries) != 1 {
			t.Fatalf("project %d skills: %v (%v)", id, entries, err)
		}
		target, err := os.Readlink(filepath.Join(dir, "find-project"))
		if err != nil || target != shared {
			t.Fatalf("project %d link = %q, %v", id, target, err)
		}
	}

	if err := os.WriteFile(original, []byte("updated"), 0600); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(f.Directory(1), "skills", "find-project")
	if got, err := os.ReadFile(filepath.Join(link, "SKILL.md")); err != nil || string(got) != "updated" {
		t.Fatalf("shared edit = %q, %v", got, err)
	}

	// A project can override one skill, while still picking up new shared skills.
	if err := os.Remove(link); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(link, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(source, filepath.Join(library, "new-task")); err != nil {
		t.Fatal(err)
	}
	if path, err := f.PrepareForLaunch(ctx, db, 1); err != nil || path != f.Directory(1) {
		t.Fatal(path, err)
	}
	if info, err := os.Lstat(link); err != nil || !info.IsDir() {
		t.Fatal("lost local override", err)
	}
	if _, err := os.Stat(filepath.Join(f.Directory(1), "skills", "new-task", "SKILL.md")); err != nil {
		t.Fatal(err)
	}

	if err := os.Remove(shared); err != nil {
		t.Fatal(err)
	}
	if _, err := f.PrepareForLaunch(ctx, db, 2); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Lstat(filepath.Join(f.Directory(2), "skills", "find-project")); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("retired link", err)
	}
	if err := os.RemoveAll(f.Directory(2)); err != nil {
		t.Fatal(err)
	}
	if _, err := f.PrepareForLaunch(ctx, db, 2); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(f.Directory(2), "skills", "new-task", "SKILL.md")); err != nil {
		t.Fatal(err)
	}
	if _, err := f.PrepareForLaunch(ctx, db, 3); !errors.Is(err, sql.ErrNoRows) {
		t.Fatal("nonexistent project", err)
	}
	if _, err := os.Stat(f.Directory(3)); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("created nonexistent project folder", err)
	}
}
