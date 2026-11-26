package projectcontext

import (
	"embed"
	"errors"
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/sys/unix"
)

//go:embed defaults/*/SKILL.md
var defaultSkills embed.FS

// seedDefaultSkills runs before project backfill, including on an empty database.
// Existing files and custom library links belong to the user.
func (f *Folders) seedDefaultSkills() error {
	// Custom database instances have distinct archive locks but share this library.
	unlock, err := lockDirectory(f.sharedSkills)
	if err != nil {
		return err
	}
	defer unlock()
	entries, err := defaultSkills.ReadDir("defaults")
	if err != nil {
		return err
	}
	for _, entry := range entries {
		dir := filepath.Join(f.sharedSkills, entry.Name())
		if err := unlinkDirectorSkill(dir, entry.Name()); err != nil {
			return err
		}
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

// Earlier installations linked the two bundled names to the director checkout.
// Match the previous layout without following the link: the checkout may have
// moved or disappeared. Other symlinks are intentional user overrides.
func unlinkDirectorSkill(dir, name string) error {
	target, err := os.Readlink(dir)
	if errors.Is(err, os.ErrNotExist) || errors.Is(err, unix.EINVAL) {
		return nil
	}
	if err != nil {
		return err
	}
	if !filepath.IsAbs(target) {
		target = filepath.Join(filepath.Dir(dir), target)
	}
	suffix := string(filepath.Separator) + filepath.Join("kamacu-director", ".claude", "skills", name)
	if !strings.HasSuffix(filepath.Clean(target), suffix) {
		return nil
	}
	return unix.Unlink(dir)
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
