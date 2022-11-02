package tea

import (
	"strconv"
	"testing"
)

func TestSingleClickableSingleLine(t *testing.T) {
	cs := makeClickableState()
	frame := "Click me " + cs.registerAndWrap(
		"here",   // text
		"link-1", // key
		"DATA-1", // data
	) + " please"
	strippedFrame := cs.stripClickableSequencesFromFrame(frame)
	if strippedFrame != "Click me here please" {
		t.Errorf("Stripped text did not match (got '%s')",
			strconv.Quote(strippedFrame),
		)
	}

	cs.swapDoubleBuffer()

	if cs.getClicked(8, 0) != nil {
		t.Errorf("clicking before the clickable should return nil")
	}

	if cs.getClicked(9, 0) != "DATA-1" {
		t.Errorf("clicking on the leftmost bound of the link should get the link data")
	}

	if cs.getClicked(12, 0) != "DATA-1" {
		t.Errorf("clicking on the rightmost bound of the link should get the link data")
	}

	if cs.getClicked(13, 0) != nil {
		t.Errorf("clicking after the clickable should return nil (%s)", strippedFrame[:13])
	}
}

func TestSingleClickableInMultiLine(t *testing.T) {
	cs := makeClickableState()
	frame := "Click me\nRight " + cs.registerAndWrap(
		"here",   // text
		"link-1", // key
		"DATA-1", // data
	) + "\n please"
	strippedFrame := cs.stripClickableSequencesFromFrame(frame)
	if strippedFrame != "Click me\nRight here\n please" {
		t.Errorf("Stripped text did not match (got '%s')",
			strconv.Quote(strippedFrame),
		)
	}

	cs.swapDoubleBuffer()

	if cs.getClicked(5, 1) != nil {
		t.Errorf("clicking before the clickable should return nil")
	}

	if cs.getClicked(6, 1) != "DATA-1" {
		t.Errorf("clicking on the leftmost bound of the link should get the link data")
	}

	if cs.getClicked(9, 1) != "DATA-1" {
		t.Errorf("clicking on the rightmost bound of the link should get the link data")
	}

	if cs.getClicked(0, 2) != nil {
		t.Errorf("clicking after the clickable should return nil")
	}
}

func TestSingleClickableInWindowsMultiLine(t *testing.T) {
	cs := makeClickableState()
	frame := "Click me\r\nRight " + cs.registerAndWrap(
		"here",   // text
		"link-1", // key
		"DATA-1", // data
	) + "\r\n please"
	strippedFrame := cs.stripClickableSequencesFromFrame(frame)
	if strippedFrame != "Click me\r\nRight here\r\n please" {
		t.Errorf("Stripped text did not match (got '%s')",
			strconv.Quote(strippedFrame),
		)
	}

	cs.swapDoubleBuffer()

	if cs.getClicked(5, 1) != nil {
		t.Errorf("clicking before the clickable should return nil")
	}

	if cs.getClicked(6, 1) != "DATA-1" {
		t.Errorf("clicking on the leftmost bound of the link should get the link data")
	}

	if cs.getClicked(9, 1) != "DATA-1" {
		t.Errorf("clicking on the rightmost bound of the link should get the link data")
	}

	if cs.getClicked(0, 2) != nil {
		t.Errorf("clicking after the clickable should return nil")
	}
}

func TestOverlappingClickableFromControlCharactersPriority(t *testing.T) {
	cs := makeClickableState()
	frame := cs.registerAndWrap(
		"First Clickable goes here", // text
		"link-1",                    // key
		"DATA-1",                    // data
	) + "\r" + cs.registerAndWrap(
		"Second-Clickable", // text
		"link-2",           // key
		"DATA-2",           // data
	)
	strippedFrame := cs.stripClickableSequencesFromFrame(frame)
	if strippedFrame != "First Clickable goes here\rSecond-Clickable" {
		t.Errorf("Stripped text did not match (got '%s')",
			strconv.Quote(strippedFrame),
		)
	}

	cs.swapDoubleBuffer()

	if cs.getClicked(0, 0) != "DATA-2" {
		t.Errorf("When multiple clickables overlap, the one later in the sequence should take precedence")
	}
}

func TestClickPreviousFrameBeforeSwap(t *testing.T) {
	cs := makeClickableState()
	cs.stripClickableSequencesFromFrame(cs.registerAndWrap(
		"Click here", // text
		"link-1",     // key
		"DATA-1",     // data
	) + "\n not here")
	cs.swapDoubleBuffer()
	cs.stripClickableSequencesFromFrame("not here\n" + cs.registerAndWrap(
		"Click here", // text
		"link-1",     // key
		"DATA-2",     // data
	))

	// before double-buffer swap, should be clicking on previous frame

	if cs.getClicked(0, 0) != "DATA-1" {
		t.Errorf("clicking on the previous frame's clickable before double-buffer swap should give previous frame's data")
	}

	if cs.getClicked(0, 1) != nil {
		t.Errorf("clicking off the previous frame's clickable before double-buffer swap should give nil")
	}

	cs.swapDoubleBuffer()

	if cs.getClicked(0, 0) != nil {
		t.Errorf("clicking off the next frame's clickable after double-buffer swap should give nil")
	}

	if cs.getClicked(0, 1) != "DATA-2" {
		t.Errorf("clicking on the next frame's clickable after double-buffer swap should give previous frame's data")
	}
}

func TestClickMultilineClickable(t *testing.T) {
	cs := makeClickableState()
	cs.stripClickableSequencesFromFrame(
		"Don't click here, but " + cs.registerAndWrap(
			"click\nhere", // text
			"link-1",      // key
			"DATA-1",      // data
		) + "\n not here")
	cs.swapDoubleBuffer()

	if cs.getClicked(0, 0) != nil {
		t.Errorf("Clicking off the clickable should be nil")
	}

	if cs.getClicked(22, 0) != "DATA-1" {
		t.Errorf("Clicking on the clickable should give the clickable's data")
	}

	if cs.getClicked(0, 1) != "DATA-1" {
		t.Errorf("Clicking on the clickable should give the clickable's data")
	}

	if cs.getClicked(0, 2) != nil {
		t.Errorf("Clicking off the clickable should be nil")
	}
}

func TestParseInvalidSequenceMissingTerminatorThenEOF(t *testing.T) {
	cs := makeClickableState()
	invalidSequence := "Hello!\uFFF9sequence\uFFFA"
	stripped := cs.stripClickableSequencesFromFrame(
		invalidSequence,
	)

	if stripped != invalidSequence {
		t.Errorf("Invalid sequences should be passed through unmodified")
	}

	if len(cs.nextRegistered) != 0 {
		t.Errorf("nextRegistered set should be cleared on invalid sequence")
	}
}

func TestParseInvalidSequenceMissingTerminatorFollowedByText(t *testing.T) {
	cs := makeClickableState()
	invalidSequence := "Hello!\uFFF9sequence\uFFFAabc"
	stripped := cs.stripClickableSequencesFromFrame(
		invalidSequence,
	)

	if stripped != invalidSequence {
		t.Errorf("Invalid sequences should be passed through unmodified")
	}

	if len(cs.nextRegistered) != 0 {
		t.Errorf("nextRegistered set should be cleared on invalid sequence")
	}
}

func TestParseInvalidSequenceMissingTerminatorAndCount(t *testing.T) {
	cs := makeClickableState()
	invalidSequence := "Hello!\uFFF9sequence"
	stripped := cs.stripClickableSequencesFromFrame(
		invalidSequence,
	)

	if stripped != invalidSequence {
		t.Errorf("Invalid sequences should be passed through unmodified")
	}

	if len(cs.nextRegistered) != 0 {
		t.Errorf("nextRegistered set should be cleared on invalid sequence")
	}
}

func TestParseInvalidSequenceMissingStart(t *testing.T) {
	cs := makeClickableState()
	invalidSequence := "Hello!sequence\uFFFA\uFFFB"
	stripped := cs.stripClickableSequencesFromFrame(
		invalidSequence,
	)

	if stripped != invalidSequence {
		t.Errorf("Invalid sequences should be passed through unmodified")
	}

	if len(cs.nextRegistered) != 0 {
		t.Errorf("nextRegistered set should be cleared on invalid sequence")
	}
}

func TestParseInvalidSequenceMissingReference(t *testing.T) {
	cs := makeClickableState()
	invalidSequence := "Hello!\uFFF9sequence\uFFFA\uFFFA\uFFFA\uFFFA\uFFFB"
	stripped := cs.stripClickableSequencesFromFrame(
		invalidSequence,
	)

	if stripped != invalidSequence {
		t.Errorf("Invalid sequences should be passed through unmodified")
	}

	if len(cs.nextRegistered) != 0 {
		t.Errorf("nextRegistered set should be cleared on invalid sequence")
	}
}

func TestParseInvalidSequenceMissingStartAndCount(t *testing.T) {
	cs := makeClickableState()
	invalidSequence := "Hello!sequence\uFFFB"
	stripped := cs.stripClickableSequencesFromFrame(
		invalidSequence,
	)

	if stripped != invalidSequence {
		t.Errorf("Invalid sequences should be passed through unmodified")
	}

	if len(cs.nextRegistered) != 0 {
		t.Errorf("nextRegistered set should be cleared on invalid sequence")
	}
}

func TestNestedClickableSingleLine(t *testing.T) {
	cs := makeClickableState()
	frame := "Click me " + cs.registerAndWrap(
		"here or "+cs.registerAndWrap(
			"here",       // text
			"inner",      // key
			"inner-data", //data
		), // text
		"outer",      // key
		"outer-data", // data
	) + " please"
	strippedFrame := cs.stripClickableSequencesFromFrame(frame)
	if strippedFrame != "Click me here or here please" {
		t.Errorf("Stripped text did not match (got '%s')",
			strconv.Quote(strippedFrame),
		)
	}

	cs.swapDoubleBuffer()

	if cs.getClicked(8, 0) != nil {
		t.Errorf("clicking before either clickable should return nil")
	}

	if cs.getClicked(10, 0) != "outer-data" {
		t.Errorf("clicking on the outer clickable should give outer-data")
	}

	if cs.getClicked(17, 0) != "inner-data" {
		t.Errorf("clicking on the inner clickable should give inner-data")
	}

	if cs.getClicked(20, 0) != "inner-data" {
		t.Errorf("clicking on the end of the inner clickable (also the end of the outer clickable) should give inner-data")
	}
}
