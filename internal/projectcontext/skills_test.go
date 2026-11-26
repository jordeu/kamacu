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
	shared := filepath.Join(library, "review")
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
		if err != nil || len(entries) != 3 {
			t.Fatalf("project %d skills: %v (%v)", id, entries, err)
		}
		target, err := os.Readlink(filepath.Join(dir, "review"))
		if err != nil || target != shared {
			t.Fatalf("project %d link = %q, %v", id, target, err)
		}
	}

	if err := os.WriteFile(original, []byte("updated"), 0600); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(f.Directory(1), "skills", "review")
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
	if err := os.Symlink(source, filepath.Join(library, "format")); err != nil {
		t.Fatal(err)
	}
	if path, err := f.PrepareForLaunch(ctx, db, 1); err != nil || path != f.Directory(1) {
		t.Fatal(path, err)
	}
	if info, err := os.Lstat(link); err != nil || !info.IsDir() {
		t.Fatal("lost local override", err)
	}
	if _, err := os.Stat(filepath.Join(f.Directory(1), "skills", "format", "SKILL.md")); err != nil {
		t.Fatal(err)
	}

	if err := os.Remove(shared); err != nil {
		t.Fatal(err)
	}
	if _, err := f.PrepareForLaunch(ctx, db, 2); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Lstat(filepath.Join(f.Directory(2), "skills", "review")); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("retired link", err)
	}
	if err := os.RemoveAll(f.Directory(2)); err != nil {
		t.Fatal(err)
	}
	if _, err := f.PrepareForLaunch(ctx, db, 2); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(f.Directory(2), "skills", "format", "SKILL.md")); err != nil {
		t.Fatal(err)
	}
	if _, err := f.PrepareForLaunch(ctx, db, 3); !errors.Is(err, sql.ErrNoRows) {
		t.Fatal("nonexistent project", err)
	}
	if _, err := os.Stat(f.Directory(3)); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("created nonexistent project folder", err)
	}
}

func TestBootSeedsDefaultsWithoutProjectsAndPreservesEdits(t *testing.T) {
	home := t.TempDir()
	path := filepath.Join(home, "kamacu.db")
	db, err := store.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := db.Exec("CREATE TABLE projects (id INTEGER PRIMARY KEY)"); err != nil {
		t.Fatal(err)
	}
	f, err := New(home, path)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	if err := f.PrepareExisting(ctx, db); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"find-project", "new-task"} {
		dir := filepath.Join(home, "skills", name)
		info, err := os.Lstat(dir)
		if err != nil || !info.IsDir() {
			t.Fatalf("shared skill directory %s: %v, %v", name, info, err)
		}
		file := filepath.Join(dir, "SKILL.md")
		info, err = os.Lstat(file)
		if err != nil || !info.Mode().IsRegular() {
			t.Fatalf("installed skill %s: %v, %v", name, info, err)
		}
		want, err := defaultSkills.ReadFile("defaults/" + name + "/SKILL.md")
		if err != nil {
			t.Fatal(err)
		}
		if got, err := os.ReadFile(file); err != nil || string(got) != string(want) {
			t.Fatalf("incomplete installed skill %s: %v", name, err)
		}
	}

	edited := filepath.Join(home, "skills", "find-project", "SKILL.md")
	if err := os.WriteFile(edited, []byte("local instructions"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(filepath.Join(home, "skills", "new-task", "SKILL.md")); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec("INSERT INTO projects VALUES (1)"); err != nil {
		t.Fatal(err)
	}
	if err := f.PrepareExisting(ctx, db); err != nil {
		t.Fatal(err)
	}
	skills := filepath.Join(f.Directory(1), "skills")
	if got, err := os.ReadFile(filepath.Join(skills, "find-project", "SKILL.md")); err != nil || string(got) != "local instructions" {
		t.Fatalf("project lost shared edits: %q, %v", got, err)
	}
	target, err := os.Readlink(filepath.Join(skills, "new-task"))
	if err != nil || target != filepath.Join(home, "skills", "new-task") {
		t.Fatalf("link to restored skill = %q, %v", target, err)
	}
	if _, err := os.Stat(filepath.Join(target, "SKILL.md")); err != nil {
		t.Fatal(err)
	}
}
