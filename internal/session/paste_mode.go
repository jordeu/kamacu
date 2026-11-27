package session

import "bytes"

const enableBracketedPaste = "\x1b[?2004h"

type pasteModeState uint8

const (
	pasteGround pasteModeState = iota
	pasteEscape
	pasteCSI
	pastePrivateCSI
	pasteSoftReset
)

// pasteMode tracks DEC private mode 2004 across PTY reads. Browser replay
// can outlive the startup escape sequence that enabled it.
type pasteMode struct {
	state     pasteModeState
	parameter int
	sawMode   bool
	enabled   bool
	escapeAt  int64
	enabledAt int64
}

func (m *pasteMode) feed(chunk []byte, offset int64) {
	for len(chunk) > 0 {
		if m.state == pasteGround {
			i := bytes.IndexByte(chunk, 0x1b)
			if i < 0 {
				return
			}
			m.escapeAt = offset + int64(i)
			m.state = pasteEscape
			offset += int64(i + 1)
			chunk = chunk[i+1:]
			continue
		}
		c := chunk[0]
		chunk = chunk[1:]
		offset++
		switch {
		case c == 0x1b:
			m.escapeAt = offset - 1
			m.state = pasteEscape
			continue
		case c == 0x18 || c == 0x1a: // CAN or SUB cancels the sequence.
			m.state = pasteGround
			continue
		case c < 0x18 || c >= 0x1c && c < 0x20 || c == 0x7f:
			// xterm handles these control bytes without ending ESC or CSI.
			continue
		}
		switch m.state {
		case pasteEscape:
			m.state = pasteGround
			switch c {
			case '[':
				m.state = pasteCSI
			case 'c': // RIS
				m.enabled = false
			}
		case pasteCSI:
			m.state = pasteGround
			switch c {
			case '?':
				m.state = pastePrivateCSI
				m.parameter = 0
				m.sawMode = false
			case '!':
				m.state = pasteSoftReset
			}
		case pastePrivateCSI:
			switch {
			case c >= '0' && c <= '9':
				m.parameter = min(2005, 10*m.parameter+int(c-'0'))
			case c == ';':
				m.sawMode = m.sawMode || m.parameter == 2004
				m.parameter = 0
			default:
				if (c == 'h' || c == 'l') && (m.sawMode || m.parameter == 2004) {
					m.enabled = c == 'h'
					m.enabledAt = m.escapeAt
				}
				m.state = pasteGround
			}
		case pasteSoftReset:
			if c == 'p' { // DECSTR
				m.enabled = false
			}
			m.state = pasteGround
		}
	}
}
