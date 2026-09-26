package session

import (
	"regexp"
	"strconv"
)

// xterm's onData emits both user input and automatic terminal reports. The
// socket forwards each emission as one frame, so complete reports can be
// recognized without buffering input (especially a user's standalone Escape).
// Keep these bytes flowing to the PTY, but don't treat them as prompt answers.
// Covers focus, device attributes/status, cursor position, mode/window reports,
// color queries, and DECRQSS replies. Ordinary keys and paste remain input.
var terminalReports = regexp.MustCompile(`^(?:\x1b\[(?:[IO]|[?>][0-9;]*c|\??[0-9]+;[0-9]+R|0n|\??[0-9]+;[0-9]+\$y|[468];[0-9]+;[0-9]+t)|\x1b\](?:4;[0-9]+|1[012]);rgb:[0-9a-fA-F/]+(?:\x07|\x1b\\)|\x1bP[01]\$r[^\x1b]*\x1b\\)+`)
var mouseReport = regexp.MustCompile(`^\x1b\[<([0-9]+);[0-9]+;[0-9]+([Mm])`)

func isUserInput(p []byte) bool {
	for len(p) > 0 {
		if report := terminalReports.Find(p); len(report) > 0 {
			p = p[len(report):]
			continue
		}
		if mouse := mouseReport.FindSubmatch(p); mouse != nil {
			button, err := strconv.Atoi(string(mouse[1]))
			// Pointer motion, scrolling, and releasing a mouse button do not
			// answer a permission prompt. A press remains actionable input.
			if err == nil && (button&96 != 0 || mouse[2][0] == 'm') {
				p = p[len(mouse[0]):]
				continue
			}
		}
		return true
	}
	return false
}
