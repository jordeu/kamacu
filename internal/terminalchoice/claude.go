// Package terminalchoice recognizes terminal dialogs with tested keyboard semantics.
// It never derives keystrokes from model-generated labels.
package terminalchoice

import (
	"regexp"
	"strconv"
	"strings"
)

const Mode = "claude_question"

type Dialog struct {
	Question string
	Options  []string
	Selected int
}

var optionLine = regexp.MustCompile(`^\s*(❯\s*)?([1-9][0-9]*)\. (.+)$`)
var checkboxLine = regexp.MustCompile(`^\s*(❯\s*)?([1-9][0-9]*\.\s*)?\[([ xX✓])\](\s|$)`)

// Claude recognizes the plain single-select AskUserQuestion layout. Partial,
// multi-select, preview, and free-text dialogs deliberately remain terminal-only.
func Claude(screen string) (Dialog, bool) {
	d := Dialog{Selected: -1}
	lines := strings.Split(screen, "\n")
	footer := -1
	for i, line := range lines {
		if strings.TrimSpace(line) == "Enter to select · ↑/↓ to navigate · Esc to cancel" {
			footer = i
		}
	}
	if footer < 0 {
		return d, false
	}
	// The navigation footer must end the active UI. A retained menu above
	// subsequent output or a new input prompt is not safe to navigate.
	for _, line := range lines[footer+1:] {
		if strings.TrimSpace(line) != "" {
			return d, false
		}
	}
	start := -1
	for i := footer - 1; i >= 0; i-- {
		if strings.Contains(lines[i], "☐") {
			start = i
			break
		}
	}
	if start < 0 {
		return d, false
	}
	// A second column, checkbox list, or different navigation mode is not this profile.
	block := strings.Join(lines[start:footer], "\n")
	if strings.ContainsAny(block, "│┃") || strings.Contains(block, "☑") {
		return d, false
	}
	first := -1
	count := 0
	for i := start + 1; i < footer; i++ {
		line := strings.TrimSpace(lines[i])
		if checkboxLine.MatchString(lines[i]) {
			return d, false
		}
		m := optionLine.FindStringSubmatch(lines[i])
		if m != nil {
			m[3] = strings.TrimSpace(m[3])
			n, _ := strconv.Atoi(m[2])
			if n != count+1 {
				return d, false
			}
			count++
			if first < 0 {
				first = i
			}
			if m[1] != "" {
				if d.Selected >= 0 {
					return d, false
				}
				d.Selected = count - 1
			}
			// These native controls are navigable but aren't answer buttons.
			if m[3] == "Type something." || m[3] == "Type something" || m[3] == "Chat about this" || m[3] == "Skip interview and plan immediately" {
				continue
			}
			if len(d.Options) != count-1 {
				return d, false
			}
			d.Options = append(d.Options, m[3])
		} else if first >= 0 && line != "" && strings.Trim(line, "─ ") != "" {
			if len(d.Options) == count && len(d.Options) > 0 {
				d.Options[len(d.Options)-1] += " — " + line
			} else {
				return d, false
			}
		}
	}
	if first < 0 || len(d.Options) < 2 || len(d.Options) > 4 || d.Selected < 0 || d.Selected >= len(d.Options) {
		return d, false
	}
	d.Question = strings.TrimSpace(strings.Join(lines[start+1:first], "\n"))
	if d.Question == "" {
		return d, false
	}
	seen := map[string]bool{}
	for _, label := range d.Options {
		if len(label) > 500 || seen[label] {
			return d, false
		}
		seen[label] = true
	}
	return d, true
}
