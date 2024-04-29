package tea

import (
	"bytes"
	"strconv"
	"strings"
	"sync"
	"testing"

	"github.com/muesli/termenv"
)

func niceByteStringRepr(b []byte, hl int) string {
	x := strings.Builder{}
	for i, c := range b {
		if i == hl {
			x.WriteString(
				termenv.
					String(strconv.Quote(string(c))).
					Foreground(termenv.ANSIRed).String(),
			)
		} else {
			x.WriteString(strconv.Quote(string(c)))
		}
		if i != len(b)-1 {
			x.WriteString(", ")
		}
	}
	return x.String()
}

func compareBuffers(
	t *testing.T,
	actual []byte,
	expected []byte,
) {
	if len(actual) != len(expected) || !bytes.Equal(actual, expected) {
		m := len(actual)
		if x := len(expected); x < m {
			m = x
		}

		i := 0
		for ; i < m; i++ {
			if actual[i] != expected[i] {
				t.Errorf("first mismatch at idx=%d c=%s", i, strconv.Quote(string(actual[i])))
				break
			}
		}

		t.Errorf("expected buffer to be:\n%s\ngot:\n%s", niceByteStringRepr(expected, i), niceByteStringRepr(actual, i))
	}
}

func TestFlush(t *testing.T) {
	t.Run("simple flush", func(t *testing.T) {
		buffer := bytes.Buffer{}
		r := standardRenderer{
			mtx: &sync.Mutex{},
			out: termenv.NewOutput(&buffer),
			buf: *bytes.NewBuffer([]byte(
				"Thing to render\n" +
					"that is multiple\n" +
					"lines")),
			width: 20,
		}

		r.flush()

		expectedBuffer := &bytes.Buffer{}
		eO := termenv.NewOutput(expectedBuffer)
		eO.WriteString("Thing to render\r\n")
		eO.WriteString("that is multiple\r\n")
		eO.WriteString("lines")
		eO.CursorBack(20)

		compareBuffers(t,
			buffer.Bytes(),
			expectedBuffer.Bytes(),
		)
	})

	t.Run("truncated flush", func(t *testing.T) {
		buffer := bytes.Buffer{}
		r := standardRenderer{
			mtx: &sync.Mutex{},
			out: termenv.NewOutput(&buffer),
			buf: *bytes.NewBuffer([]byte(
				"Thing to render\n" +
					"that overflows the renderer width\n")),
			width: 20,
		}

		r.flush()

		expectedBuffer := &bytes.Buffer{}
		eO := termenv.NewOutput(expectedBuffer)
		eO.WriteString("Thing to render\r\n")
		eO.WriteString("that overflows the r\r\n")
		eO.CursorBack(20)

		compareBuffers(t,
			buffer.Bytes(),
			expectedBuffer.Bytes(),
		)
	})

	t.Run("truncated flush with ansi escape sequences", func(t *testing.T) {
		buffer := bytes.Buffer{}
		r := standardRenderer{
			mtx: &sync.Mutex{},
			out: termenv.NewOutput(&buffer),
			buf: *bytes.NewBuffer([]byte(
				"Thing to render\n" +
					termenv.String("that overflows the renderer width").
						Foreground(termenv.ANSIRed).
						String() +
					"\n")),
			width: 20,
		}

		r.flush()

		expectedBuffer := &bytes.Buffer{}
		eO := termenv.NewOutput(expectedBuffer)
		eO.WriteString("Thing to render\r\n")
		eO.WriteString(
			termenv.String("that overflows the r").
				Foreground(termenv.ANSIRed).
				String(),
		)
		eO.WriteString("\r\n")
		eO.CursorBack(20)

		compareBuffers(t,
			buffer.Bytes(),
			expectedBuffer.Bytes(),
		)
	})

	t.Run("truncated flush with hyperlink", func(t *testing.T) {
		buffer := bytes.Buffer{}
		r := standardRenderer{
			mtx: &sync.Mutex{},
			out: termenv.NewOutput(&buffer),
			buf: *bytes.NewBuffer([]byte(
				termenv.Hyperlink(
					"http://www.contoso.com",
					"this overflows the renderer width") +
					"\n")),
			width: 20,
		}

		r.flush()

		expectedBuffer := &bytes.Buffer{}
		eO := termenv.NewOutput(expectedBuffer)
		eO.WriteString(
			termenv.Hyperlink(
				"http://www.contoso.com",
				"this overflows the r"))
		eO.WriteString("\r\n")
		eO.CursorBack(20)

		compareBuffers(t,
			buffer.Bytes(),
			expectedBuffer.Bytes(),
		)
	})

	t.Run("Only renders changed lines -->> Single Line", func(t *testing.T) {
		origRender := "Line 1\n" +
			"Line 2\n" +
			"Line 3\n"

		nextRender := "Line One\n" +
			"Line 2\n" +
			"Line 3\n"

		buffer := bytes.Buffer{}
		r := standardRenderer{
			mtx:           &sync.Mutex{},
			out:           termenv.NewOutput(&buffer),
			lastRender:    origRender,
			linesRendered: 4,
			buf:           *bytes.NewBuffer([]byte(nextRender)),
			width:         20,
			renderingHead: 0,
		}

		r.flush()

		expectedBuffer := &bytes.Buffer{}
		eO := termenv.NewOutput(expectedBuffer)
		eO.ClearLine()
		eO.WriteString("Line One\r")
		eO.CursorBack(20)

		compareBuffers(t,
			buffer.Bytes(),
			expectedBuffer.Bytes(),
		)

		if r.lastRender != nextRender {
			t.Errorf("expected lastRender to be:\n%s\ngot:\n%s",
				nextRender,
				r.lastRender)
		}

		if r.renderingHead != 0 {
			t.Errorf("expected renderingHead to be 0, got %d", r.renderingHead)
		}
	})

	t.Run("Only renders changed lines -->>  2 Lines", func(t *testing.T) {
		origRender := "Line 1\n" +
			"Line 2\n" +
			"Line 3\n"

		nextRender := "Line One\n" +
			"Line 2\n" +
			"Line Three\n"

		buffer := bytes.Buffer{}
		r := standardRenderer{
			mtx:           &sync.Mutex{},
			out:           termenv.NewOutput(&buffer),
			lastRender:    origRender,
			linesRendered: 4,
			buf:           *bytes.NewBuffer([]byte(nextRender)),
			width:         20,
			renderingHead: 0,
		}

		r.flush()

		expectedBuffer := &bytes.Buffer{}
		eO := termenv.NewOutput(expectedBuffer)
		// Jumping down to line 2
		eO.CursorDown(2)
		// Clearing lines 2 and 0 in order
		eO.ClearLine()
		eO.CursorUp(2)
		eO.ClearLine()
		// Writing line 0
		eO.WriteString("Line One\r")
		// Jumping down to line 2 and writing line 2
		eO.CursorDown(2)
		eO.WriteString("Line Three\r")
		// reset cursor to back
		eO.CursorBack(20)

		compareBuffers(t,
			buffer.Bytes(),
			expectedBuffer.Bytes(),
		)

		if r.lastRender != nextRender {
			t.Errorf("expected lastRender to be:\n%s\ngot:\n%s",
				nextRender,
				r.lastRender)
		}

		if r.renderingHead != 2 {
			t.Errorf("expected renderingHead to be 0, got %d", r.renderingHead)
		}
	})

	t.Run("Only renders changed lines -->> 3 Lines", func(t *testing.T) {
		origRender := "Line 1\n" +
			"Line 2\n" +
			"Line 3\n"

		nextRender := "Line One\n" +
			"Line Two\n" +
			"Line Three\n"

		buffer := bytes.Buffer{}
		r := standardRenderer{
			mtx:           &sync.Mutex{},
			out:           termenv.NewOutput(&buffer),
			lastRender:    origRender,
			linesRendered: 4,
			buf:           *bytes.NewBuffer([]byte(nextRender)),
			width:         20,
			renderingHead: 0,
		}

		r.flush()

		expectedBuffer := &bytes.Buffer{}
		eO := termenv.NewOutput(expectedBuffer)
		// Jumping down to line 2
		eO.CursorDown(2)
		// Clearing lines 2, 1, 0 in order
		eO.ClearLine()
		eO.CursorUp(1)
		eO.ClearLine()
		eO.CursorUp(1)
		eO.ClearLine()
		// Writing line 0
		eO.WriteString("Line One\r")
		// Jumping down to line 2 and writing line 2
		eO.WriteString("\n")
		eO.WriteString("Line Two\r")
		eO.WriteString("\n")
		eO.WriteString("Line Three\r")
		// reset cursor to back
		eO.CursorBack(20)

		compareBuffers(t,
			buffer.Bytes(),
			expectedBuffer.Bytes(),
		)

		if r.lastRender != nextRender {
			t.Errorf("expected lastRender to be:\n%s\ngot:\n%s",
				nextRender,
				r.lastRender)
		}

		if r.renderingHead != 2 {
			t.Errorf("expected renderingHead to be 0, got %d", r.renderingHead)
		}
	})

	t.Run("Only renders changed lines -->> Can render on lines not including the renderingHead", func(t *testing.T) {
		origRender := "Line 1\n" +
			"Line 2\n" +
			"Line 3\n"

		nextRender := "Line One\n" +
			"Line Two\n" +
			"Line Three\n"

		buffer := bytes.Buffer{}
		r := standardRenderer{
			mtx:           &sync.Mutex{},
			out:           termenv.NewOutput(&buffer),
			lastRender:    origRender,
			linesRendered: 4,
			buf:           *bytes.NewBuffer([]byte(nextRender)),
			width:         20,
			renderingHead: 3,
		}

		r.flush()

		expectedBuffer := &bytes.Buffer{}
		eO := termenv.NewOutput(expectedBuffer)
		// Clearing lines 2, 1, 0 in order
		eO.ClearLine()
		eO.CursorUp(1)
		eO.ClearLine()
		eO.CursorUp(1)
		eO.ClearLine()
		// Writing line 0
		eO.WriteString("Line One\r")
		// Jumping down to line 2 and writing line 2
		eO.WriteString("\n")
		eO.WriteString("Line Two\r")
		eO.WriteString("\n")
		eO.WriteString("Line Three\r")
		// reset cursor to back
		eO.CursorBack(20)

		compareBuffers(t,
			buffer.Bytes(),
			expectedBuffer.Bytes(),
		)

		if r.lastRender != nextRender {
			t.Errorf("expected lastRender to be:\n%s\ngot:\n%s",
				nextRender,
				r.lastRender)
		}

		if r.renderingHead != 2 {
			t.Errorf("expected renderingHead to be 0, got %d", r.renderingHead)
		}
	})

	t.Run("Partial Repaint + posting scrollback messages causes full re-render", func(t *testing.T) {
		origRender := "Line 1\n" +
			"Line 2\n" +
			"Line 3\n" +
			"Line 4\n"

		nextRender := "Line 1\n" +
			"Line 2\n" +
			"Line Three\n" +
			"Line Four\n"

		buffer := bytes.Buffer{}
		r := standardRenderer{
			mtx:           &sync.Mutex{},
			out:           termenv.NewOutput(&buffer),
			lastRender:    origRender,
			linesRendered: 4,
			buf:           *bytes.NewBuffer([]byte(nextRender)),
			width:         20,
			renderingHead: 1,
			queuedMessageLines: []string{
				"Queued Message 1",
				"Queued Message Two",
			},
		}

		r.flush()

		expectedBuffer := &bytes.Buffer{}
		eO := termenv.NewOutput(expectedBuffer)
		// move down from line 1 to line 3
		eO.CursorDown(2)
		// Clearing lines 3, 2, 1, 0 in order
		eO.ClearLine()
		eO.CursorUp(1)
		eO.ClearLine()
		eO.CursorUp(1)
		eO.ClearLine()
		eO.CursorUp(1)
		eO.ClearLine()
		// Dumping the message queue
		eO.WriteString("Queued Message 1")
		eO.WriteString("\r\n")
		eO.WriteString("Queued Message Two")
		eO.WriteString("\r\n")
		// Writing the full content of the buffer, even though some lines are the same
		// as they were before
		eO.WriteString("Line 1\r")
		eO.WriteString("\n")
		eO.WriteString("Line 2\r")
		eO.WriteString("\n")
		eO.WriteString("Line Three\r")
		eO.WriteString("\n")
		eO.WriteString("Line Four\r")
		// reset cursor to back
		eO.CursorBack(20)

		compareBuffers(t,
			buffer.Bytes(),
			expectedBuffer.Bytes(),
		)

		if r.lastRender != nextRender {
			t.Errorf("expected lastRender to be:\n%s\ngot:\n%s",
				nextRender,
				r.lastRender)
		}

		if r.renderingHead != 2 {
			t.Errorf("expected renderingHead to be 0, got %d", r.renderingHead)
		}
	})
}
