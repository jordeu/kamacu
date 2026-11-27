package session

import "testing"

func TestBracketedPasteModeTracksPTYOutput(t *testing.T) {
	var mode pasteMode
	var offset int64
	for _, step := range []struct {
		output  string
		enabled bool
	}{
		{"plain text [?2004h", false},
		{"\x1b", false},
		{"[?25;20", false},
		{"04h", true},
		{"\x1b[?20040l", true},
		{"\x1b[?200;4l", true},
		{"\x1b[?2004;1004l", false},
		{"\x1b[?2004h", true},
		{"\x1b[!", true},
		{"p", false},
		{"\x1b[?2004h\x1bc", false},
		{"\x1b[?20\x074h", false},
		{"\x1b[?20\x0704h", true},
		{"\x1b[?2004\x18l", true},
		{"\x1b[?2004\x7fl", false},
	} {
		mode.feed([]byte(step.output), offset)
		offset += int64(len(step.output))
		if mode.enabled != step.enabled {
			t.Fatalf("after %q: bracketed paste = %v, want %v", step.output, mode.enabled, step.enabled)
		}
	}
}
