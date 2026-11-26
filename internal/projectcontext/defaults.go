package projectcontext

import (
	"embed"
	"errors"
	"os"
	"path/filepath"
)

//go:embed defaults/*/SKILL.md
var defaultSkills embed.FS

// seedDefaultSkills runs before project backfill, including on an empty database.
// Existing files and custom library links belong to the user.
func (f *Folders) seedDefaultSkills() error {
	if err := os.MkdirAll(f.sharedSkills, 0700); err != nil {
		return err
	}
	entries, err := defaultSkills.ReadDir("defaults")
	if err != nil {
		return err
	}
	for _, entry := range entries {
		dir := filepath.Join(f.sharedSkills, entry.Name())
		if err := os.Mkdir(dir, 0700); err != nil && !errors.Is(err, os.ErrExist) {
			return err
		}
		info, err := os.Lstat(dir)
		if err != nil {
			return err
		}
		if !info.IsDir() {
			continue
		}
		dest := filepath.Join(dir, "SKILL.md")
		if _, err := os.Lstat(dest); err == nil {
			continue
		} else if !errors.Is(err, os.ErrNotExist) {
			return err
		}
		content, err := defaultSkills.ReadFile("defaults/" + entry.Name() + "/SKILL.md")
		if err != nil {
			return err
		}
		if err := installDefault(dest, content); err != nil {
			return err
		}
	}
	return nil
}

// Publish a complete file without overwriting an edit or a concurrent boot
// using the same library with another database.
func installDefault(dest string, content []byte) error {
	tmp, err := os.CreateTemp(filepath.Dir(dest), ".kamacu-skill-*")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	_, err = tmp.Write(content)
	if err := errors.Join(err, tmp.Close()); err != nil {
		return err
	}
	if err := os.Link(tmp.Name(), dest); !errors.Is(err, os.ErrExist) {
		return err
	}
	return nil
}
