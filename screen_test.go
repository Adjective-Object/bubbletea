package tea

import (
	"bytes"
	"testing"
)

type clearMsgTest = struct {
	cmds     sequenceMsg
	expected string
}

func runClearMsgTest(t *testing.T, test clearMsgTest) {
	t.Helper()
	var buf bytes.Buffer
	var in bytes.Buffer

	m := &testModel{}
	p := NewProgram(m, WithInput(&in), WithOutput(&buf))

	test.cmds = append(test.cmds, Quit)
	go p.Send(test.cmds)

	if _, err := p.Run(); err != nil {
		t.Fatal(err)
	}

	compareBuffers(t, buf.Bytes(), []byte(test.expected))
}

func TestClearMsg(t *testing.T) {
	t.Parallel()
	t.Run("clear_screen", func(t *testing.T) {
		t.Parallel()
		runClearMsgTest(t, clearMsgTest{
			cmds:     []Cmd{ClearScreen},
			expected: "\x1b[?25l\x1b[?2004h\x1b[2J\x1b[1;1H\x1b[1;1Hsuccess\r\n\x1b[0D\x1b[2K\x1b[?2004l\x1b[?25h\x1b[?1002l\x1b[?1003l\x1b[?1006l",
		})
	})

	t.Run("altscreen", func(t *testing.T) {
		t.Parallel()
		runClearMsgTest(t, clearMsgTest{
			cmds:     []Cmd{EnterAltScreen, ExitAltScreen},
			expected: "\x1b[?25l\x1b[?2004h\x1b[?1049h\x1b[2J\x1b[1;1H\x1b[1;1H\x1b[?25l\x1b[?1049l\x1b[?25lsuccess\r\n\x1b[0D\x1b[2K\x1b[?2004l\x1b[?25h\x1b[?1002l\x1b[?1003l\x1b[?1006l",
		})
	})

	t.Run("altscreen_autoexit", func(t *testing.T) {
		t.Parallel()
		runClearMsgTest(t, clearMsgTest{
			cmds:     []Cmd{EnterAltScreen},
			expected: "\x1b[?25l\x1b[?2004h\x1b[?1049h\x1b[2J\x1b[1;1H\x1b[1;1H\x1b[?25lsuccess\r\n\x1b[2;0H\x1b[2K\x1b[?2004l\x1b[?25h\x1b[?1002l\x1b[?1003l\x1b[?1006l\x1b[?1049l\x1b[?25h",
		})
	})

	t.Run("mouse_cellmotion", func(t *testing.T) {
		t.Parallel()
		runClearMsgTest(t, clearMsgTest{
			cmds:     []Cmd{EnableMouseCellMotion},
			expected: "\x1b[?25l\x1b[?2004h\x1b[?1002h\x1b[?1006hsuccess\r\n\x1b[0D\x1b[2K\x1b[?2004l\x1b[?25h\x1b[?1002l\x1b[?1003l\x1b[?1006l",
		})
	})

	t.Run("mouse_allmotion", func(t *testing.T) {
		t.Parallel()
		runClearMsgTest(t, clearMsgTest{
			cmds:     []Cmd{EnableMouseAllMotion},
			expected: "\x1b[?25l\x1b[?2004h\x1b[?1003h\x1b[?1006hsuccess\r\n\x1b[0D\x1b[2K\x1b[?2004l\x1b[?25h\x1b[?1002l\x1b[?1003l\x1b[?1006l",
		})
	})

	t.Run("mouse_disable", func(t *testing.T) {
		t.Parallel()
		runClearMsgTest(t, clearMsgTest{
			cmds:     []Cmd{EnableMouseAllMotion, DisableMouse},
			expected: "\x1b[?25l\x1b[?2004h\x1b[?1003h\x1b[?1006h\x1b[?1002l\x1b[?1003l\x1b[?1006lsuccess\r\n\x1b[0D\x1b[2K\x1b[?2004l\x1b[?25h\x1b[?1002l\x1b[?1003l\x1b[?1006l",
		})
	})

	t.Run("cursor_hide", func(t *testing.T) {
		t.Parallel()
		runClearMsgTest(t, clearMsgTest{
			cmds:     []Cmd{HideCursor},
			expected: "\x1b[?25l\x1b[?2004h\x1b[?25lsuccess\r\n\x1b[0D\x1b[2K\x1b[?2004l\x1b[?25h\x1b[?1002l\x1b[?1003l\x1b[?1006l",
		})
	})

	t.Run("cursor_hideshow", func(t *testing.T) {
		t.Parallel()
		runClearMsgTest(t, clearMsgTest{
			cmds:     []Cmd{HideCursor, ShowCursor},
			expected: "\x1b[?25l\x1b[?2004h\x1b[?25l\x1b[?25hsuccess\r\n\x1b[0D\x1b[2K\x1b[?2004l\x1b[?25h\x1b[?1002l\x1b[?1003l\x1b[?1006l",
		})
	})

	t.Run("bp_stop_start", func(t *testing.T) {
		t.Parallel()
		runClearMsgTest(t, clearMsgTest{
			cmds:     []Cmd{DisableBracketedPaste, EnableBracketedPaste},
			expected: "\x1b[?25l\x1b[?2004h\x1b[?2004l\x1b[?2004hsuccess\r\n\x1b[0D\x1b[2K\x1b[?2004l\x1b[?25h\x1b[?1002l\x1b[?1003l\x1b[?1006l",
		})
	})

}
