package tea

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/muesli/ansi/compressor"
	"github.com/muesli/reflow/truncate"
	"github.com/muesli/termenv"
)

const (
	// defaultFramerate specifies the maximum interval at which we should
	// update the view.
	defaultFPS = 60
	maxFPS     = 120
)

// standardRenderer is a framerate-based terminal renderer, updating the view
// at a given framerate to avoid overloading the terminal emulator.
//
// In cases where very high performance is needed the renderer can be told
// to exclude ranges of lines, allowing them to be written to directly.
type standardRenderer struct {
	mtx *sync.Mutex
	out *termenv.Output

	buf                bytes.Buffer
	queuedMessageLines []string
	framerate          time.Duration
	ticker             *time.Ticker
	done               chan struct{}
	lastRender         string
	linesRendered      int
	useANSICompressor  bool
	once               sync.Once

	// cursor visibility state
	cursorHidden bool

	// essentially whether or not we're using the full size of the terminal
	altScreenActive bool

	// whether or not we're currently using bracketed paste
	bpActive bool

	// renderer dimensions; usually the size of the window
	width  int
	height int

	// lines explicitly set not to render
	ignoreLines map[int]struct{}

	// buffer of which lines to skip in the current render,
	// which is reused between renders as a performance optimization
	skipLines []bool

	// tracks the position of the cursor in the write-buffer over time
	renderingHead int
}

// newRenderer creates a new renderer. Normally you'll want to initialize it
// with os.Stdout as the first argument.
func newRenderer(out *termenv.Output, useANSICompressor bool, fps int) renderer {
	if fps < 1 {
		fps = defaultFPS
	} else if fps > maxFPS {
		fps = maxFPS
	}
	r := &standardRenderer{
		out:                out,
		mtx:                &sync.Mutex{},
		done:               make(chan struct{}),
		framerate:          time.Second / time.Duration(fps),
		useANSICompressor:  useANSICompressor,
		queuedMessageLines: []string{},
	}
	if r.useANSICompressor {
		r.out = termenv.NewOutput(&compressor.Writer{Forward: out})
	}
	return r
}

// start starts the renderer.
func (r *standardRenderer) start() {
	if r.ticker == nil {
		r.ticker = time.NewTicker(r.framerate)
	} else {
		// If the ticker already exists, it has been stopped and we need to
		// reset it.
		r.ticker.Reset(r.framerate)
	}

	// Since the renderer can be restarted after a stop, we need to reset
	// the done channel and its corresponding sync.Once.
	r.once = sync.Once{}

	go r.listen()
}

// stop permanently halts the renderer, rendering the final frame.
func (r *standardRenderer) stop() {
	// Stop the renderer before acquiring the mutex to avoid a deadlock.
	r.once.Do(func() {
		r.done <- struct{}{}
	})

	// flush locks the mutex
	r.flush()

	r.mtx.Lock()
	defer r.mtx.Unlock()

	r.out.ClearLine()

	if r.useANSICompressor {
		if w, ok := r.out.TTY().(io.WriteCloser); ok {
			_ = w.Close()
		}
	}
}

// kill halts the renderer. The final frame will not be rendered.
func (r *standardRenderer) kill() {
	// Stop the renderer before acquiring the mutex to avoid a deadlock.
	r.once.Do(func() {
		r.done <- struct{}{}
	})

	r.mtx.Lock()
	defer r.mtx.Unlock()

	r.out.ClearLine()
}

// listen waits for ticks on the ticker, or a signal to stop the renderer.
func (r *standardRenderer) listen() {
	for {
		select {
		case <-r.done:
			r.ticker.Stop()
			return

		case <-r.ticker.C:
			r.flush()
		}
	}
}

// flush renders the buffer.
func (r *standardRenderer) flush() {
	r.mtx.Lock()
	defer r.mtx.Unlock()

	if r.buf.Len() == 0 || r.buf.String() == r.lastRender {
		// Nothing to do
		return
	}

	// Output buffer
	buf := &bytes.Buffer{}
	out := termenv.NewOutput(buf)

	newLines := strings.Split(r.buf.String(), "\n")

	// If we know the output's height, we can use it to determine how many
	// lines we can render. We drop lines from the top of the render buffer if
	// necessary, as we can't navigate the cursor into the terminal's scrollback
	// buffer.
	if r.height > 0 && len(newLines) > r.height {
		newLines = newLines[len(newLines)-r.height:]
	}

	numLinesThisFlush := len(newLines)
	oldLines := strings.Split(r.lastRender, "\n")

	// get capacity for the skipLines buffer
	skipCap := r.linesRendered
	if numLinesThisFlush > skipCap {
		skipCap = numLinesThisFlush
	}

	// reset the skipLines buffer to the correct size
	if len(r.skipLines) < skipCap {
		if cap(r.skipLines) < skipCap {
			r.skipLines = make([]bool, skipCap)
		} else {
			// You can safely resize a slice to a larger capcity of its length
			// See: https://go.dev/tour/moretypes/11
			r.skipLines = r.skipLines[:skipCap]
		}
	}
	for i := 0; i < len(r.skipLines); i++ {
		r.skipLines[i] = false
	}

	flushQueuedMessages := len(r.queuedMessageLines) > 0 && !r.altScreenActive

	// TODO remove printf
	fmt.Fprintln(os.Stderr, "--------- start new render:", "r.linesRendered=", r.linesRendered, "flushQueuedMessages=", flushQueuedMessages, "r.renderingHead=", r.renderingHead, "numLinesThisFlush=", numLinesThisFlush, "---------")

	// Find all the lines we want to skip
	var lowestLineToClear = 0
	if flushQueuedMessages {
		// if wer'e flushing messages, we need to clear all lines
		lowestLineToClear = r.linesRendered - 1
	} else if r.linesRendered > 0 {
		for i := 0; i < r.linesRendered; i++ {
			if (len(newLines) > i && len(oldLines) > i) && (newLines[i] == oldLines[i]) {
				// If the number of lines we want to render hasn't increased and
				// new line is the same as the old line we can skip rendering for
				// this line as a performance optimization.
				r.skipLines[i] = true
			} else if _, shouldIgnore := r.ignoreLines[i]; shouldIgnore {
				r.skipLines[i] = true
			} else {
				lowestLineToClear = i
			}
		}
	}
	// Actually erase the lines we want to skip, tracking the relative position of
	// the cursor in the write-buffer over time to avoid unnecessary cursor movement.
	//
	// This is because unecessary cursor movement can cause "flickering" in the
	// terminal, where the cursor jumps around as the terminal is being updated.
	fmt.Fprintln(os.Stderr, "skipLines", r.skipLines)
	fmt.Fprintln(os.Stderr, "highestRenderedLine", lowestLineToClear)
	fmt.Fprintln(os.Stderr, "skipping")

	// If we have rendered anyting previously, we need to clear the lines that
	// were not skipped
	if r.linesRendered > 0 {
		// The rendering head is the index of the cursor in the current render.
		// If we have more lines to render, we need to move the cursor down to
		// the line we want to replace.
		if r.renderingHead < lowestLineToClear {
			fmt.Fprintln(os.Stderr, "  jump down to line", lowestLineToClear, "delta=", lowestLineToClear-r.renderingHead)
			out.CursorDown(lowestLineToClear - r.renderingHead)
		}
		r.renderingHead = lowestLineToClear

		// iterate backwards, starting from the last rendered line
		for i := lowestLineToClear; i >= 0; i-- {
			if !r.skipLines[i] {
				// jump to this position and clear it
				fmt.Fprintln(os.Stderr, "  up + clear to line ", i, "delta", r.renderingHead-i, "newLine", newLines[i])
				if r.renderingHead != i {
					out.CursorUp(r.renderingHead - i)
				}
				out.ClearLine()
				r.renderingHead = i
			}
		}
	}

	if flushQueuedMessages {
		// Dump the lines we've queued up for printing
		for _, line := range r.queuedMessageLines {
			_, _ = out.WriteString(line)
			_, _ = out.WriteString("\r\n")
		}
		// clear the queued message lines
		r.queuedMessageLines = r.queuedMessageLines[:0]
	}

	// Paint new lines, starting at the current position of the rendering head
	fmt.Fprintln(os.Stderr, "painting")
	for i := r.renderingHead; i < numLinesThisFlush; i++ {
		if skip := r.skipLines[i]; !skip {
			line := newLines[i]

			// Truncate lines wider than the width of the window to avoid
			// wrapping, which will mess up rendering. If we don't have the
			// width of the window this will be ignored.
			//
			// Note that on Windows we only get the width of the window on
			// program initialization, so after a resize this won't perform
			// correctly (signal SIGWINCH is not supported on Windows).
			if r.width > 0 {
				line = truncate.String(line, uint(r.width))
			}

			// move the rendering head down to the current line
			if r.renderingHead != i {
				fmt.Fprintln(os.Stderr, "skip down:", "renderingHead", r.renderingHead, "i", i, "down:", i-r.renderingHead)
				delta := i - r.renderingHead
				if delta == 1 {
					out.Write([]byte("\n"))
				} else {
					out.CursorDown(delta)
				}
				r.renderingHead = i
			}

			fmt.Fprintln(os.Stderr, "render:", "renderingHead", r.renderingHead, "i", i, " |||", line)
			_, _ = out.WriteString(line)
			if i < numLinesThisFlush-1 {
				_, _ = out.WriteString("\r")
			}
		}
	}
	r.linesRendered = numLinesThisFlush

	// Make sure the cursor is at the start of the last line to keep rendering
	// behavior consistent.
	if r.altScreenActive {
		// This case fixes a bug in macOS terminal. In other terminals the
		// other case seems to do the job regardless of whether or not we're
		// using the full terminal window.
		out.MoveCursor(r.linesRendered, 0)
	} else {
		out.CursorBack(r.width)
	}

	_, _ = r.out.Write(buf.Bytes())
	r.lastRender = r.buf.String()
	r.buf.Reset()
}

// write writes to the internal buffer. The buffer will be outputted via the
// ticker which calls flush().
func (r *standardRenderer) write(s string) {
	r.mtx.Lock()
	defer r.mtx.Unlock()
	r.buf.Reset()

	// If an empty string was passed we should clear existing output and
	// rendering nothing. Rather than introduce additional state to manage
	// this, we render a single space as a simple (albeit less correct)
	// solution.
	if s == "" {
		s = " "
	}

	_, _ = r.buf.WriteString(s)
}

func (r *standardRenderer) repaint() {
	r.lastRender = ""
}

func (r *standardRenderer) clearScreen() {
	r.mtx.Lock()
	defer r.mtx.Unlock()

	r.out.ClearScreen()
	r.out.MoveCursor(1, 1)

	r.repaint()
}

func (r *standardRenderer) altScreen() bool {
	r.mtx.Lock()
	defer r.mtx.Unlock()

	return r.altScreenActive
}

func (r *standardRenderer) enterAltScreen() {
	r.mtx.Lock()
	defer r.mtx.Unlock()

	if r.altScreenActive {
		return
	}

	r.altScreenActive = true
	r.out.AltScreen()

	// Ensure that the terminal is cleared, even when it doesn't support
	// alt screen (or alt screen support is disabled, like GNU screen by
	// default).
	//
	// Note: we can't use r.clearScreen() here because the mutex is already
	// locked.
	r.out.ClearScreen()
	r.out.MoveCursor(1, 1)

	// cmd.exe and other terminals keep separate cursor states for the AltScreen
	// and the main buffer. We have to explicitly reset the cursor visibility
	// whenever we enter AltScreen.
	if r.cursorHidden {
		r.out.HideCursor()
	} else {
		r.out.ShowCursor()
	}

	r.repaint()
}

func (r *standardRenderer) exitAltScreen() {
	r.mtx.Lock()
	defer r.mtx.Unlock()

	if !r.altScreenActive {
		return
	}

	r.altScreenActive = false
	r.out.ExitAltScreen()

	// cmd.exe and other terminals keep separate cursor states for the AltScreen
	// and the main buffer. We have to explicitly reset the cursor visibility
	// whenever we exit AltScreen.
	if r.cursorHidden {
		r.out.HideCursor()
	} else {
		r.out.ShowCursor()
	}

	r.repaint()
}

func (r *standardRenderer) showCursor() {
	r.mtx.Lock()
	defer r.mtx.Unlock()

	r.cursorHidden = false
	r.out.ShowCursor()
}

func (r *standardRenderer) hideCursor() {
	r.mtx.Lock()
	defer r.mtx.Unlock()

	r.cursorHidden = true
	r.out.HideCursor()
}

func (r *standardRenderer) enableMouseCellMotion() {
	r.mtx.Lock()
	defer r.mtx.Unlock()

	r.out.EnableMouseCellMotion()
}

func (r *standardRenderer) disableMouseCellMotion() {
	r.mtx.Lock()
	defer r.mtx.Unlock()

	r.out.DisableMouseCellMotion()
}

func (r *standardRenderer) enableMouseAllMotion() {
	r.mtx.Lock()
	defer r.mtx.Unlock()

	r.out.EnableMouseAllMotion()
}

func (r *standardRenderer) disableMouseAllMotion() {
	r.mtx.Lock()
	defer r.mtx.Unlock()

	r.out.DisableMouseAllMotion()
}

func (r *standardRenderer) enableMouseSGRMode() {
	r.mtx.Lock()
	defer r.mtx.Unlock()

	r.out.EnableMouseExtendedMode()
}

func (r *standardRenderer) disableMouseSGRMode() {
	r.mtx.Lock()
	defer r.mtx.Unlock()

	r.out.DisableMouseExtendedMode()
}

func (r *standardRenderer) enableBracketedPaste() {
	r.mtx.Lock()
	defer r.mtx.Unlock()

	r.out.EnableBracketedPaste()
	r.bpActive = true
}

func (r *standardRenderer) disableBracketedPaste() {
	r.mtx.Lock()
	defer r.mtx.Unlock()

	r.out.DisableBracketedPaste()
	r.bpActive = false
}

func (r *standardRenderer) bracketedPasteActive() bool {
	r.mtx.Lock()
	defer r.mtx.Unlock()

	return r.bpActive
}

// setIgnoredLines specifies lines not to be touched by the standard Bubble Tea
// renderer.
func (r *standardRenderer) setIgnoredLines(from int, to int) {
	// Lock if we're going to be clearing some lines since we don't want
	// anything jacking our cursor.
	if r.linesRendered > 0 {
		r.mtx.Lock()
		defer r.mtx.Unlock()
	}

	if r.ignoreLines == nil {
		r.ignoreLines = make(map[int]struct{})
	}
	for i := from; i < to; i++ {
		r.ignoreLines[i] = struct{}{}
	}

	// Erase ignored lines
	if r.linesRendered > 0 {
		buf := &bytes.Buffer{}
		out := termenv.NewOutput(buf)

		for i := r.linesRendered - 1; i >= 0; i-- {
			if _, exists := r.ignoreLines[i]; exists {
				out.ClearLine()
			}
			out.CursorUp(1)
		}
		out.MoveCursor(r.linesRendered, 0) // put cursor back
		_, _ = r.out.Write(buf.Bytes())
	}
}

// clearIgnoredLines returns control of any ignored lines to the standard
// Bubble Tea renderer. That is, any lines previously set to be ignored can be
// rendered to again.
func (r *standardRenderer) clearIgnoredLines() {
	r.ignoreLines = nil
}

// insertTop effectively scrolls up. It inserts lines at the top of a given
// area designated to be a scrollable region, pushing everything else down.
// This is roughly how ncurses does it.
//
// To call this function use command ScrollUp().
//
// For this to work renderer.ignoreLines must be set to ignore the scrollable
// region since we are bypassing the normal Bubble Tea renderer here.
//
// Because this method relies on the terminal dimensions, it's only valid for
// full-window applications (generally those that use the alternate screen
// buffer).
//
// This method bypasses the normal rendering buffer and is philosophically
// different than the normal way we approach rendering in Bubble Tea. It's for
// use in high-performance rendering, such as a pager that could potentially
// be rendering very complicated ansi. In cases where the content is simpler
// standard Bubble Tea rendering should suffice.
func (r *standardRenderer) insertTop(lines []string, topBoundary, bottomBoundary int) {
	r.mtx.Lock()
	defer r.mtx.Unlock()

	buf := &bytes.Buffer{}
	out := termenv.NewOutput(buf)

	out.ChangeScrollingRegion(topBoundary, bottomBoundary)
	out.MoveCursor(topBoundary, 0)
	out.InsertLines(len(lines))
	_, _ = out.WriteString(strings.Join(lines, "\r\n"))
	out.ChangeScrollingRegion(0, r.height)

	// Move cursor back to where the main rendering routine expects it to be
	out.MoveCursor(r.linesRendered, 0)

	_, _ = r.out.Write(buf.Bytes())
}

// insertBottom effectively scrolls down. It inserts lines at the bottom of
// a given area designated to be a scrollable region, pushing everything else
// up. This is roughly how ncurses does it.
//
// To call this function use the command ScrollDown().
//
// See note in insertTop() for caveats, how this function only makes sense for
// full-window applications, and how it differs from the normal way we do
// rendering in Bubble Tea.
func (r *standardRenderer) insertBottom(lines []string, topBoundary, bottomBoundary int) {
	r.mtx.Lock()
	defer r.mtx.Unlock()

	buf := &bytes.Buffer{}
	out := termenv.NewOutput(buf)

	out.ChangeScrollingRegion(topBoundary, bottomBoundary)
	out.MoveCursor(bottomBoundary, 0)
	_, _ = out.WriteString("\r\n" + strings.Join(lines, "\r\n"))
	out.ChangeScrollingRegion(0, r.height)

	// Move cursor back to where the main rendering routine expects it to be
	out.MoveCursor(r.linesRendered, 0)

	_, _ = r.out.Write(buf.Bytes())
}

// handleMessages handles internal messages for the renderer.
func (r *standardRenderer) handleMessages(msg Msg) {
	switch msg := msg.(type) {
	case repaintMsg:
		// Force a repaint by clearing the render cache as we slide into a
		// render.
		r.mtx.Lock()
		r.repaint()
		r.mtx.Unlock()

	case WindowSizeMsg:
		r.mtx.Lock()
		r.width = msg.Width
		r.height = msg.Height
		r.repaint()
		r.mtx.Unlock()

	case clearScrollAreaMsg:
		r.clearIgnoredLines()

		// Force a repaint on the area where the scrollable stuff was in this
		// update cycle
		r.mtx.Lock()
		r.repaint()
		r.mtx.Unlock()

	case syncScrollAreaMsg:
		// Re-render scrolling area
		r.clearIgnoredLines()
		r.setIgnoredLines(msg.topBoundary, msg.bottomBoundary)
		r.insertTop(msg.lines, msg.topBoundary, msg.bottomBoundary)

		// Force non-scrolling stuff to repaint in this update cycle
		r.mtx.Lock()
		r.repaint()
		r.mtx.Unlock()

	case scrollUpMsg:
		r.insertTop(msg.lines, msg.topBoundary, msg.bottomBoundary)

	case scrollDownMsg:
		r.insertBottom(msg.lines, msg.topBoundary, msg.bottomBoundary)

	case printLineMessage:
		if !r.altScreenActive {
			lines := strings.Split(msg.messageBody, "\n")
			r.mtx.Lock()
			r.queuedMessageLines = append(r.queuedMessageLines, lines...)
			r.repaint()
			r.mtx.Unlock()
		}
	}
}

// HIGH-PERFORMANCE RENDERING STUFF

type syncScrollAreaMsg struct {
	lines          []string
	topBoundary    int
	bottomBoundary int
}

// SyncScrollArea performs a paint of the entire region designated to be the
// scrollable area. This is required to initialize the scrollable region and
// should also be called on resize (WindowSizeMsg).
//
// For high-performance, scroll-based rendering only.
func SyncScrollArea(lines []string, topBoundary int, bottomBoundary int) Cmd {
	return func() Msg {
		return syncScrollAreaMsg{
			lines:          lines,
			topBoundary:    topBoundary,
			bottomBoundary: bottomBoundary,
		}
	}
}

type clearScrollAreaMsg struct{}

// ClearScrollArea deallocates the scrollable region and returns the control of
// those lines to the main rendering routine.
//
// For high-performance, scroll-based rendering only.
func ClearScrollArea() Msg {
	return clearScrollAreaMsg{}
}

type scrollUpMsg struct {
	lines          []string
	topBoundary    int
	bottomBoundary int
}

// ScrollUp adds lines to the top of the scrollable region, pushing existing
// lines below down. Lines that are pushed out the scrollable region disappear
// from view.
//
// For high-performance, scroll-based rendering only.
func ScrollUp(newLines []string, topBoundary, bottomBoundary int) Cmd {
	return func() Msg {
		return scrollUpMsg{
			lines:          newLines,
			topBoundary:    topBoundary,
			bottomBoundary: bottomBoundary,
		}
	}
}

type scrollDownMsg struct {
	lines          []string
	topBoundary    int
	bottomBoundary int
}

// ScrollDown adds lines to the bottom of the scrollable region, pushing
// existing lines above up. Lines that are pushed out of the scrollable region
// disappear from view.
//
// For high-performance, scroll-based rendering only.
func ScrollDown(newLines []string, topBoundary, bottomBoundary int) Cmd {
	return func() Msg {
		return scrollDownMsg{
			lines:          newLines,
			topBoundary:    topBoundary,
			bottomBoundary: bottomBoundary,
		}
	}
}

type printLineMessage struct {
	messageBody string
}

// Println prints above the Program. This output is unmanaged by the program and
// will persist across renders by the Program.
//
// Unlike fmt.Println (but similar to log.Println) the message will be print on
// its own line.
//
// If the altscreen is active no output will be printed.
func Println(args ...interface{}) Cmd {
	return func() Msg {
		return printLineMessage{
			messageBody: fmt.Sprint(args...),
		}
	}
}

// Printf prints above the Program. It takes a format template followed by
// values similar to fmt.Printf. This output is unmanaged by the program and
// will persist across renders by the Program.
//
// Unlike fmt.Printf (but similar to log.Printf) the message will be print on
// its own line.
//
// If the altscreen is active no output will be printed.
func Printf(template string, args ...interface{}) Cmd {
	return func() Msg {
		return printLineMessage{
			messageBody: fmt.Sprintf(template, args...),
		}
	}
}
