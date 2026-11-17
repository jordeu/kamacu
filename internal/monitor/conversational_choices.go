package monitor

import (
	"errors"
	"regexp"
	"strconv"
	"strings"
)

var errConversationalChoices = errors.New("visible conversational choices were dropped or misclassified")
var conversationalOption = regexp.MustCompile(`^(\s*)(?:[-*] )?([1-9][0-9]*[.)])\s+(.+)$`)

func normalizedChoice(text string) string {
	return strings.Join(strings.Fields(strings.TrimPrefix(strings.TrimPrefix(strings.TrimSpace(text), "- "), "* ")), " ")
}

// conversationalChoices recognizes a deliberately narrow ready-input profile,
// not arbitrary numbered text. The model must already identify an unanswered
// question. Its exact question must precede the choices, followed by Claude's
// empty conversational prompt and status footer. This only validates a model
// result; it never manufactures options or terminal keystrokes.
func conversationalChoices(screen, question string) []string {
	question = strings.Join(strings.Fields(question), " ")
	if question == "" {
		return nil
	}
	lines := strings.Split(screen, "\n")
	prompt := -1
	for i, line := range lines {
		if strings.TrimSpace(line) == "❯" {
			prompt = i
		}
	}
	rule := func(line string) bool {
		line = strings.TrimSpace(line)
		return len([]rune(line)) >= 8 && strings.Trim(line, "─") == ""
	}
	if prompt < 1 || prompt+2 >= len(lines) || !rule(lines[prompt-1]) || !rule(lines[prompt+1]) {
		return nil
	}
	footer := false
	for _, line := range lines[prompt+2:] {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if !strings.HasPrefix(line, "⏵") && !strings.HasPrefix(line, "? for shortcuts") {
			return nil
		}
		footer = true
	}
	if !footer {
		return nil
	}
	body := lines[:prompt-1]
	// This right-aligned Claude notification is screen chrome, not a wrapped
	// continuation of the final answer choice. Only strip the exact known toast
	// at the end of the response, never an arbitrary line inside its content.
	for len(body) > 0 {
		last := strings.TrimSpace(body[len(body)-1])
		if last != "" && last != "✔ Update installed · Restart to update" {
			break
		}
		body = body[:len(body)-1]
	}
	start := -1
	for i := range body {
		if strings.HasSuffix(strings.Join(strings.Fields(strings.Join(body[:i+1], " ")), " "), question) {
			start = i + 1
		}
	}
	if start < 0 {
		return nil
	}
	options := []string{}
	indent := 0
	for _, line := range body[start:] {
		if strings.TrimSpace(line) == "" {
			continue
		}
		if match := conversationalOption.FindStringSubmatch(line); match != nil {
			n, _ := strconv.Atoi(strings.TrimRight(match[2], ".)"))
			if n != len(options)+1 || n > 10 {
				return nil
			}
			indent = len(match[1])
			options = append(options, match[2]+" "+strings.TrimSpace(match[3]))
		} else {
			// Wrapped labels must be indented beyond their list marker. Any
			// unrelated paragraph, quoted block or new UI makes this uncertain.
			if len(options) == 0 || len(line)-len(strings.TrimLeft(line, " \t")) <= indent || strings.HasPrefix(strings.TrimSpace(line), "▎") {
				return nil
			}
			options[len(options)-1] += " " + strings.TrimSpace(line)
		}
		if len(options[len(options)-1]) > 500 {
			return nil
		}
	}
	if len(options) < 2 {
		return nil
	}
	return options
}
