package terminalchoice

import (
	"strings"
	"testing"
)

// Plain list layout transcribed from the terminal example in
// https://github.com/anthropics/claude-code/issues/34540.
const screen = `────────────────────────────────────────
 ☐ Clarification

Which approach should I use?

❯ 1. Keep it
     Preserve the current behavior.
  2. Replace it
     Use the new implementation.
  3. Type something.
────────────────────────────────────────
  4. Chat about this
  5. Skip interview and plan immediately

Enter to select · ↑/↓ to navigate · Esc to cancel`

func TestClaudeDialog(t *testing.T) {
	d, ok := Claude(screen)
	if !ok || d.Question != "Which approach should I use?" || d.Selected != 0 || len(d.Options) != 2 || d.Options[1] != "Replace it — Use the new implementation." {
		t.Fatalf("%+v %v", d, ok)
	}
	moved := strings.Replace(strings.Replace(screen, "❯ 1.", "  1.", 1), "  2.", "❯ 2.", 1)
	if d, ok = Claude(moved); !ok || d.Selected != 1 {
		t.Fatalf("%+v %v", d, ok)
	}
}

func TestClaudeRejectsUnsupportedDialogs(t *testing.T) {
	for name, s := range map[string]string{
		"permission":     strings.Replace(screen, "☐ Clarification", "Permission", 1),
		"multiselect":    strings.Replace(screen, "Enter to select", "Space to select", 1),
		"preview":        strings.Replace(screen, "Keep it", "Keep it │ preview", 1),
		"clipped":        strings.Replace(screen, "❯ 1. Keep it", "❯ 2. Keep it", 1),
		"no focus":       strings.ReplaceAll(screen, "❯", " "),
		"double focus":   strings.Replace(screen, "  2.", "❯ 2.", 1),
		"input focused":  strings.Replace(strings.Replace(screen, "❯ 1.", "  1.", 1), "  3.", "❯ 3.", 1),
		"missing footer": strings.Replace(screen, "Enter to select", "", 1),
	} {
		t.Run(name, func(t *testing.T) {
			if _, ok := Claude(s); ok {
				t.Fatal("unsupported dialog accepted")
			}
		})
	}
}

func TestClaudeRejectsInactiveMenu(t *testing.T) {
	for _, tail := range []string{
		"\n\nQuestion answered.\nShould I deploy now?\n❯ ",
		"\n❯ ",
		"\nWorking…",
	} {
		if _, ok := Claude(screen + tail); ok {
			t.Fatalf("inactive menu accepted with subsequent output %q", tail)
		}
	}
	if _, ok := Claude(screen + "\n   \n\t"); !ok {
		t.Fatal("blank terminal rows should not invalidate an active menu")
	}
}

func TestClaudeAllowsBracketsInOptionText(t *testing.T) {
	for _, text := range []string{"Return [] when empty.", "See [the guide](https://example.com).", "Use items[index]."} {
		input := strings.Replace(screen, "Preserve the current behavior.", text, 1)
		d, ok := Claude(input)
		if !ok || d.Options[0] != "Keep it — "+text {
			t.Fatalf("ordinary bracketed text rejected: %q: %+v %v", text, d, ok)
		}
	}
	input := strings.Replace(screen, "Keep it", "Keep [existing] behavior", 1)
	if _, ok := Claude(input); !ok {
		t.Fatal("brackets in option label rejected")
	}
}

func TestClaudeRejectsCheckboxRows(t *testing.T) {
	for _, row := range []string{"❯ 1. [ ] Keep it", "❯ 1. [x] Keep it", "❯ [ ] Keep it", "     [X] Keep it"} {
		input := strings.Replace(screen, "❯ 1. Keep it", row, 1)
		if _, ok := Claude(input); ok {
			t.Fatalf("checkbox row accepted: %q", row)
		}
	}
}
