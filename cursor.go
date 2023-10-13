package tea

import (
	"bytes"
	"strconv"
)

// The last-known position of the cursor on the screen
type CursorPositionMsg struct {
	X int
	Y int
}

func parseCursorPositionEvent(buf []byte) (CursorPositionMsg, int) {
	var ev CursorPositionMsg
	// scan ahead to look for the terminating 'R' in the buffer
	var endIdx int
	var sepIdx int = -1
	for i, c := range buf {
		if c == ';' {
			if sepIdx != -1 {
				// multiple semicolons -- not a cursor position event
				return ev, -1
			} else {
				sepIdx = i
			}
		} else if c == 'R' {
			endIdx = i
			break
		}
	}
	// if we found no terminating 'R', then this is not a cursor position event
	// similarly, if we found no semicolon separator, then this is not a cursor
	// position event
	if endIdx == 0 || sepIdx == -1 {
		return ev, -1
	}

	components := bytes.SplitN(buf, []byte{';'}, 2)
	if len(components) != 2 {
		return ev, -1
	}

	// parse the numbers
	var err error
	ev.Y, err = strconv.Atoi(string(buf[2:sepIdx]))
	if err != nil {
		return ev, -1
	}
	ev.X, err = strconv.Atoi(string(buf[sepIdx+1 : endIdx]))
	if err != nil {
		return ev, -1
	}
	return ev, endIdx + 1
}
