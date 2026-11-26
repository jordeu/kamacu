package integration

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSharedInstructionsPreserveUserTextAndLastOwner(t *testing.T) {
	m := testManager(t)
	ctx := context.Background()
	path := filepath.Join(m.claudeHome, "CLAUDE.md")
	original := []byte("My original guidance.\n")
	write(t, path, original)
	for _, engine := range []string{"opencode", "claude", "opencode"} {
		if _, err := m.Enable(ctx, engine); err != nil {
			t.Fatal(err)
		}
	}
	got := read(t, path)
	if bytes.Count(got, []byte(instructionBlock)) != 1 {
		t.Fatalf("unexpected instructions: %q", got)
	}
	write(t, path, append(got, []byte("My later rule.\n")...))
	if _, err := m.Disable(ctx, "opencode"); err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(read(t, path), []byte(instructionBlock)) {
		t.Fatal("removed shared block")
	}
	if _, err := m.Disable(ctx, "claude"); err != nil {
		t.Fatal(err)
	}
	if string(read(t, path)) != "My original guidance.\nMy later rule.\n" {
		t.Fatal("changed user text")
	}
}

func TestInstructionOverrideUpgradeAndEditedBlock(t *testing.T) {
	m := testManager(t)
	ctx := context.Background()
	if _, err := m.Enable(ctx, "codex"); err != nil {
		t.Fatal(err)
	}
	base := filepath.Join(m.codexHome, "AGENTS.md")
	override := filepath.Join(m.codexHome, "AGENTS.override.md")
	write(t, override, []byte("Active override"))
	if err := m.Sync(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(base); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("old managed file retained", err)
	}
	data := read(t, override)
	if !bytes.Contains(data, []byte(instructionBlock)) {
		t.Fatal("missing on override")
	}
	edited := bytes.Replace(data, []byte("its current"), []byte("my current"), 1)
	write(t, override, edited)
	if err := m.Sync(ctx); err == nil {
		t.Fatal("overwrote edited instruction")
	}
	out, err := m.Disable(ctx, "codex")
	if err != nil || out.Message == "" || !bytes.Equal(read(t, override), edited) {
		t.Fatal(out, err)
	}
}

func TestInstructionUpgradeFromMCPOnlyAndUnownedBlock(t *testing.T) {
	m := testManager(t)
	ctx := context.Background()
	state := &manifest{Engines: map[string]engineState{"claude": {Enabled: true}}}
	if err := m.save(state); err != nil {
		t.Fatal(err)
	}
	if err := m.Sync(ctx); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(m.claudeHome, "CLAUDE.md")
	if !bytes.Contains(read(t, path), []byte(instructionBlock)) {
		t.Fatal("missing on upgrade")
	}
	if _, err := m.Disable(ctx, "claude"); err != nil {
		t.Fatal(err)
	}
	write(t, path, []byte(instructionBlock))
	if _, err := m.Enable(ctx, "claude"); err != nil {
		t.Fatal(err)
	}
	if _, err := m.Disable(ctx, "claude"); err != nil {
		t.Fatal(err)
	}
	if string(read(t, path)) != instructionBlock {
		t.Fatal("removed user-owned block")
	}
}

func TestRepairPreservesReplacedSharedInstructionFile(t *testing.T) {
	m := testManager(t)
	ctx := context.Background()
	path := filepath.Join(m.claudeHome, "CLAUDE.md")
	for _, engine := range []string{"claude", "opencode"} {
		if _, err := m.Enable(ctx, engine); err != nil {
			t.Fatal(err)
		}
	}
	// The user replaces the entire managed file with an empty one.
	write(t, path, nil)
	if _, err := m.Enable(ctx, "claude"); err != nil {
		t.Fatal(err)
	}
	for _, engine := range []string{"claude", "opencode"} {
		if _, err := m.Disable(ctx, engine); err != nil {
			t.Fatal(err)
		}
	}
	if got := read(t, path); len(got) != 0 {
		t.Fatalf("replacement changed: %q", got)
	}
}

func TestInvalidOverridePreservesPreviousInstructions(t *testing.T) {
	m := testManager(t)
	ctx := context.Background()
	if _, err := m.Enable(ctx, "codex"); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(m.codexHome, "AGENTS.md")
	before := read(t, path)
	write(t, filepath.Join(m.codexHome, "AGENTS.override.md"), []byte(blockStart+"partial block"))
	if err := m.Sync(ctx); err == nil {
		t.Fatal("accepted malformed target")
	}
	if !bytes.Equal(before, read(t, path)) {
		t.Fatal("lost previous instructions")
	}
}

func TestOpenCodeWithClaudeFallbackDisabled(t *testing.T) {
	t.Setenv("OPENCODE_DISABLE_CLAUDE_CODE_PROMPT", "true")
	m := testManager(t)
	fallback := filepath.Join(m.claudeHome, "CLAUDE.md")
	write(t, fallback, []byte("personal guidance"))
	if _, err := m.Enable(context.Background(), "opencode"); err != nil {
		t.Fatal(err)
	}
	if got := string(read(t, fallback)); got != "personal guidance" {
		t.Fatalf("changed ignored fallback: %q", got)
	}
	if !bytes.Contains(read(t, filepath.Join(m.configHome, "opencode", "AGENTS.md")), []byte(instructionBlock)) {
		t.Fatal("missing OpenCode instructions")
	}
}

func TestRepairDistinguishesOlderManagedBlockFromUserEdit(t *testing.T) {
	m := testManager(t)
	ctx := context.Background()
	if _, err := m.Enable(ctx, "codex"); err != nil {
		t.Fatal(err)
	}
	s, err := m.load()
	if err != nil {
		t.Fatal(err)
	}
	e := s.Engines["codex"]
	previous := []byte("\n" + blockStart + "\nRead project instructions.\n" + blockEnd + "\n")
	e.InstructionHash = digest(previous)
	s.Engines["codex"] = e
	if err := m.save(s); err != nil {
		t.Fatal(err)
	}
	write(t, e.InstructionPath, append([]byte("User guidance.\n"), previous...))
	if msg := m.instructionStatus(e, "codex"); !strings.Contains(msg, "outdated") {
		t.Fatal(msg)
	}
	if err := m.Sync(ctx); err != nil {
		t.Fatal(err)
	}
	if got := string(read(t, e.InstructionPath)); got != "User guidance.\n"+instructionBlock {
		t.Fatal(got)
	}
}
