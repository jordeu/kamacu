package monitor

import (
	"encoding/json"
	"regexp"
	"strconv"
	"strings"

	"kamacu/internal/terminalchoice"
)

// These layouts are scheduling hints only. The classifier still validates the
// state and evidence; this recognizer never supplies an answer or keystrokes.
var dialogFooter = regexp.MustCompile(`(?i)^(?:Enter to select · (?:↑/↓|Tab/Arrow keys) to navigate(?: · ctrl\+g to edit in [^·]+)? · Esc to cancel|(?:Press )?enter to confirm or esc to cancel)$`)
var dialogChoice = regexp.MustCompile(`^\s*(?:[❯›>]\s*)?([1-9][0-9]*)[.)]\s+\S.*$`)
var permissionHeading = regexp.MustCompile(`(?i)^(?:would you like to |do you want to |allow\b|permission\b|approval\b)`)
var dialogHeading = regexp.MustCompile(`(?i)^(?:(?:choose|select)\b.*|(?:which|what|how|when|where|why|who|should|would|could|can|do|does|is|are|will|shall|may|continue|proceed)\b.*\?)$`)
var dialogSelection = regexp.MustCompile(`^\s*[❯›>]\s*([1-9][0-9]*[.)]\s+)`)

func dialogFingerprint(screen, engine string) string {
	if engine != "codex" && engine != "claude" {
		return ""
	}
	lines := strings.Split(screen, "\n")
	inCode := make([]bool, len(lines))
	fenced := false
	for i, line := range lines {
		line = strings.TrimSpace(line)
		boundary := strings.HasPrefix(line, "```") || strings.HasPrefix(line, "~~~")
		inCode[i] = fenced || boundary
		if boundary {
			fenced = !fenced
		}
	}
	for i, line := range lines {
		line = strings.TrimSpace(line)
		if inCode[i] {
			continue
		}
		// Both word-boundary wrapping and wrapping in the middle of a word occur.
		footerEnd := -1
		joined, spaced := "", ""
		for j := i; j < len(lines) && j < i+3; j++ {
			joined += strings.TrimSpace(lines[j])
			spaced += strings.TrimSpace(lines[j]) + " "
			if dialogFooter.MatchString(joined) || dialogFooter.MatchString(strings.TrimSpace(spaced)) {
				footerEnd = j
				break
			}
		}
		if footerEnd < 0 {
			continue
		}
		active := true
		for _, tail := range lines[footerEnd+1:] {
			tail = strings.TrimSpace(tail)
			// A stuck generation footer must not hide a real dialog. A new input
			// prompt or arbitrary response below the menu makes that menu inactive.
			if tail != "" && !activeGeneration(tail, engine) {
				active = false
				break
			}
		}
		if !active {
			continue
		}
		first, count := -1, 0
		for j := 0; j < i; j++ {
			if inCode[j] {
				continue
			}
			m := dialogChoice.FindStringSubmatch(lines[j])
			if m == nil {
				continue
			}
			n, _ := strconv.Atoi(m[1])
			if n == 1 {
				first = j
				count = 1
			} else if first >= 0 && n == count+1 {
				count++
			} else {
				first = -1
				count = 0
			}
		}
		if first < 0 || count < 2 {
			continue
		}
		// Reuse the tested Claude question parser, excluding background output
		// and focus/selection changes from this scheduling identity.
		if engine == "claude" {
			// Keyboard eligibility depends on selection (e.g. "Type something")
			// but scheduling identity must not. Canonicalize focus on a copy;
			// real input handling always validates the unmodified screen.
			neutral := append([]string(nil), lines[:footerEnd+1]...)
			for j := range neutral {
				neutral[j] = dialogSelection.ReplaceAllString(neutral[j], "${1}")
			}
			neutral[first] = "❯ " + strings.TrimSpace(neutral[first])
			if d, ok := terminalchoice.Claude(strings.Join(neutral, "\n")); ok {
				raw, _ := json.Marshal(struct {
					Question string
					Options  []string
				}{d.Question, d.Options})
				return "claude_question:" + string(raw)
			}
		}

		// Start at the panel heading, not an arbitrary lookback. Include the
		// entire command, however long, without unrelated preceding logs.
		start := -1
		for j := first - 1; j >= 0; j-- {
			text := strings.TrimSpace(lines[j])
			if !inCode[j] && (permissionHeading.MatchString(text) || dialogHeading.MatchString(text)) {
				start = j
				break
			}
		}
		if start < 0 {
			continue
		}
		var block []string
		for _, text := range lines[start : footerEnd+1] {
			if activeGeneration(text, engine) {
				continue
			}
			text = dialogSelection.ReplaceAllString(text, "${1}")
			block = append(block, strings.Join(strings.Fields(text), " "))
		}
		return strings.Join(block, "\n")
	}
	return ""
}
