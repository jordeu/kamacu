package integration

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestPiContextFollowsActiveInstructions(t *testing.T) {
	m := testManager(t)
	m.piHome = filepath.Join(m.home, "custom-pi")
	ctx := context.Background()
	fallback := filepath.Join(m.piHome, "CLAUDE.MD")
	override := filepath.Join(m.piHome, "AGENTS.override.md")
	guidance := []byte("User guidance\n")
	write(t, fallback, guidance)
	if _, err := m.Enable(ctx, "pi"); err != nil {
		t.Fatal(err)
	}
	if current := read(t, fallback); !bytes.Equal(current, append(bytes.Clone(guidance), instructionBlock...)) {
		t.Fatalf("active fallback was not updated: %q", current)
	}
	registration := read(t, filepath.Join(m.piHome, "mcp.json"))
	for _, value := range piMCPEnvironment() {
		if !bytes.Contains(registration, []byte(value)) {
			t.Errorf("Pi MCP configuration is missing %s", value)
		}
	}
	write(t, override, []byte{})
	if _, err := m.Enable(ctx, "pi"); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(read(t, fallback), guidance) || string(read(t, override)) != instructionBlock {
		t.Fatal("repair did not move instructions to the empty override")
	}
	edit := []byte(blockStart + "\nMy block\n" + blockEnd + "\n")
	write(t, override, edit)
	if _, err := m.Enable(ctx, "pi"); err == nil || !bytes.Equal(read(t, override), edit) {
		t.Fatal("repair overwrote edited instructions or failed to report the conflict")
	}
}

func TestPiContextPreservesInstructionSymlink(t *testing.T) {
	m := testManager(t)
	target := filepath.Join(m.home, "guidance.md")
	write(t, target, []byte("User guidance"))
	if err := os.MkdirAll(m.piHome, 0700); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(m.piHome, "AGENTS.md")
	if err := os.Symlink(target, link); err != nil {
		t.Fatal(err)
	}
	if _, err := m.Enable(context.Background(), "pi"); err == nil {
		t.Fatal("enable accepted an active symlink")
	}
	if got, err := os.Readlink(link); err != nil || got != target {
		t.Fatalf("symlink changed: %q, %v", got, err)
	}
}
