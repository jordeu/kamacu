package session

import "regexp"

// xterm's onData emits both user input and automatic terminal reports. The
// socket forwards each emission as one frame, so complete reports can be
// recognized without buffering input (especially a user's standalone Escape).
// Keep these bytes flowing to the PTY, but don't treat them as prompt answers.
// Covers focus, device attributes/status, cursor position, mode/window reports,
// color queries, and DECRQSS replies. Ordinary keys and paste remain input.
var terminalReports = regexp.MustCompile(`^(?:\x1b\[(?:[IO]|[?>][0-9;]*c|\??[0-9]+;[0-9]+R|0n|\??[0-9]+;[0-9]+\$y|[468];[0-9]+;[0-9]+t)|\x1b\](?:4;[0-9]+|1[012]);rgb:[0-9a-fA-F/]+(?:\x07|\x1b\\)|\x1bP[01]\$r[^\x1b]*\x1b\\)+$`)

func isUserInput(p []byte) bool {
	return len(p) > 0 && !terminalReports.Match(p)
}
