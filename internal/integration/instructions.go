package integration

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Keep the visible instruction outside the comments: some CLIs strip HTML comments.
const blockStart = "<!-- kamacu:project-context -->"
const blockEnd = "<!-- /kamacu:project-context -->"
const instructionBlock = "\n" + blockStart + `
When starting or resuming a task, check the environment variable KAMACU_PROJECT_CONTEXT.
If it names an existing directory, read instructions.md there when present. Inspect the
names and descriptions in skills/*/SKILL.md. Whenever a skill matches the task, read
its current SKILL.md and resolve supporting files relative to its directory. If the
variable is unset or the folder is absent, proceed normally. Report unreadable files.
` + blockEnd + "\n"

func optionalFile(path string) ([]byte, error) {
	info, err := os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() {
		return nil, fmt.Errorf("preserving non-regular instruction file %s", path)
	}
	return os.ReadFile(path)
}

func (m *Manager) instructionPath(engine string) (string, error) {
	claude := filepath.Join(m.claudeHome, "CLAUDE.md")
	switch engine {
	case "codex":
		override := filepath.Join(m.codexHome, "AGENTS.override.md")
		data, err := optionalFile(override)
		if err != nil || len(bytes.TrimSpace(data)) > 0 {
			return override, err
		}
		return filepath.Join(m.codexHome, "AGENTS.md"), nil
	case "opencode":
		path := filepath.Join(m.configHome, "opencode", "AGENTS.md")
		if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
			return path, err
		}
		// Creating AGENTS.md would shadow an existing Claude fallback.
		for _, flag := range []string{"OPENCODE_DISABLE_CLAUDE_CODE", "OPENCODE_DISABLE_CLAUDE_CODE_PROMPT"} {
			value := os.Getenv(flag)
			if value == "1" || strings.EqualFold(value, "true") {
				return path, nil
			}
		}
		fallback := filepath.Join(m.home, ".claude", "CLAUDE.md")
		if _, err := os.Stat(fallback); !errors.Is(err, os.ErrNotExist) {
			return fallback, err
		}
		return path, nil
	default:
		return claude, nil
	}
}

func instructionBounds(data []byte) (int, int, error) {
	start := bytes.Index(data, []byte(blockStart))
	end := bytes.Index(data, []byte(blockEnd))
	if start < 0 && end < 0 {
		return -1, -1, nil
	}
	if start < 0 || end < start || bytes.Count(data, []byte(blockStart)) != 1 || bytes.Count(data, []byte(blockEnd)) != 1 {
		return 0, 0, errors.New("ambiguous Kamacu instruction markers")
	}
	// Include only the two newlines installed as part of the block.
	if start > 0 && data[start-1] == '\n' {
		start--
	}
	end += len(blockEnd)
	if end < len(data) && data[end] == '\n' {
		end++
	}
	return start, end, nil
}

func writeInstructions(path string, original, next []byte) error {
	current, err := optionalFile(path)
	if err != nil {
		return err
	}
	if (current == nil) != (original == nil) || !bytes.Equal(current, original) {
		return fmt.Errorf("instructions in %s changed concurrently; retry", path)
	}
	if next == nil {
		return os.Remove(path)
	}
	return atomicWrite(path, next, 0600)
}

func (m *Manager) installInstructions(s *manifest, engine string) error {
	path, err := m.instructionPath(engine)
	if err != nil {
		return err
	}
	data, err := optionalFile(path)
	if err != nil {
		return err
	}
	start, end, err := instructionBounds(data)
	if err != nil {
		return fmt.Errorf("%s: %w", path, err)
	}
	want := []byte(instructionBlock)
	// Ownership belongs to a particular block on disk. All engines sharing
	// that file must retain the same fingerprint and file ownership.
	var hash string
	var created bool
	for _, other := range s.Engines {
		if other.InstructionPath == path && other.InstructionHash != "" {
			hash, created = other.InstructionHash, other.InstructionFileCreated
			break
		}
	}
	upgrade := start >= 0 && !bytes.Equal(data[start:end], want)
	if upgrade && digest(data[start:end]) != hash {
		return fmt.Errorf("edited instructions in %s preserved", path)
	}
	// Validate the replacement before detaching from the previous location.
	e := s.Engines[engine]
	if e.InstructionPath != "" && e.InstructionPath != path {
		if err := m.removeInstructions(s, engine); err != nil {
			return err
		}
	}
	switch {
	case start < 0:
		// A file whose block was removed may be a user-created replacement.
		created = data == nil
		hash = digest(want)
	case upgrade:
		if err := writeInstructions(path, data, bytes.Replace(data, data[start:end], want, 1)); err != nil {
			return err
		}
	}
	// A previous update may have written the new block before its manifest save.
	if hash != "" {
		hash = digest(want)
	}
	for name, owner := range s.Engines {
		if name == engine || owner.InstructionPath == path {
			owner.InstructionPath, owner.InstructionHash, owner.InstructionFileCreated = path, hash, created
			s.Engines[name] = owner
		}
	}
	// Journal new ownership before creating the block so repair can finish a partial install.
	if err := m.save(s); err != nil {
		return err
	}
	if start < 0 {
		return writeInstructions(path, data, append(bytes.Clone(data), want...))
	}
	return nil
}

func (m *Manager) removeInstructions(s *manifest, engine string) error {
	e := s.Engines[engine]
	if e.InstructionPath == "" {
		return nil
	}
	shared := false
	for name, other := range s.Engines {
		if name != engine && other.InstructionPath == e.InstructionPath {
			shared = true
			if e.InstructionHash != "" && other.InstructionHash == "" {
				other.InstructionHash = e.InstructionHash
				other.InstructionFileCreated = e.InstructionFileCreated
				s.Engines[name] = other
			}
		}
	}
	if !shared && e.InstructionHash != "" {
		data, err := optionalFile(e.InstructionPath)
		if err != nil {
			return err
		}
		start, end, err := instructionBounds(data)
		if err != nil || (start >= 0 && digest(data[start:end]) != e.InstructionHash) {
			return fmt.Errorf("edited instructions in %s preserved", e.InstructionPath)
		}
		if start >= 0 {
			next := append(append([]byte{}, data[:start]...), data[end:]...)
			if e.InstructionFileCreated && len(next) == 0 {
				next = nil
			}
			if err := writeInstructions(e.InstructionPath, data, next); err != nil {
				return err
			}
		}
	}
	e.InstructionPath, e.InstructionHash = "", ""
	e.InstructionFileCreated = false
	s.Engines[engine] = e
	return m.save(s)
}

func (m *Manager) instructionStatus(e engineState, engine string) string {
	if !e.Enabled {
		return ""
	}
	path, err := m.instructionPath(engine)
	if err != nil || path != e.InstructionPath {
		return "Project context instructions need repair."
	}
	data, err := optionalFile(path)
	if err != nil {
		return err.Error()
	}
	start, end, err := instructionBounds(data)
	if err != nil || (start >= 0 && !bytes.Equal(data[start:end], []byte(instructionBlock))) {
		return fmt.Sprintf("Edited instructions in %s preserved.", path)
	}
	if start < 0 {
		return "Project context instructions are missing. Repair the integration to restore them."
	}
	return ""
}

// Enabled reads the manifest at launch so changes made via the CLI take effect immediately.
func (m *Manager) Enabled(engine string) (bool, error) {
	s, err := m.load()
	if err != nil {
		return false, err
	}
	return s.Engines[engine].Enabled, nil
}
