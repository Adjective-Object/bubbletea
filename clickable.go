package tea

import (
	"strings"

	"github.com/mattn/go-runewidth"
)

type cell struct {
	x int
	y int
}

type clickableBounds struct {
	start cell
	end   cell
	// Sequence position of the start of the clickable sequence.
	// Used to disambiguate between multiple overlapping clickables
	// (e.g. from cursor control characters like \r)
	sequencePosition int
}

func (cb *clickableBounds) containsPoint(p cell) bool {
	return ((cb.start.y < p.y || (cb.start.y == p.y && cb.start.x <= p.x)) &&
		(cb.end.y > p.y || (cb.end.y == p.y && cb.end.x >= p.x)))
}

type clickable struct {
	data   interface{}
	bounds clickableBounds
	// To avoid redundant allocations and heap fragmentation,
	// we do not clear and reconstruct the clickable map each frame.
	//
	// Instead, we overwrite existing clickables on each frame, since
	// most clickable elements will be persist between frames
	generation int
}

// Tracks clickable regions and wraps text in interlinear annotations
//
// See https://www.unicode.org/reports/tr20/tr20-1.html
// (section 3.2 Interlinear Annotation Characters, U+FFF9-U+FFFB)
//
// Also see https://www.unicode.org/charts/nameslist/n_FFF0.html
type clickableState struct {
	currentGeneration int
	idCounter         int
	stableKeyMap      map[string]int
	// double-buffer the clickable map so the
	// current map can be used for click-determination while the
	// next map is built
	currentRegistered map[int]clickable
	nextRegistered    map[int]clickable
}

func makeClickableState() clickableState {
	return clickableState{
		currentGeneration: 0,
		idCounter:         0,
		stableKeyMap:      map[string]int{},
		currentRegistered: map[int]clickable{},
		nextRegistered:    map[int]clickable{},
	}
}

// Strips any clickable sequences from the frame, and
// registers their bounds within the frame against the next set of
// registered handlers
//
// after stripFrame() -> swapDoubleBuffer() is called, getClicked() can
// called to translate a position in current frame into a clicked object
func (cr *clickableState) stripClickableSequencesFromFrame(frame string) string {
	var prev cell
	var current cell

	parsingClickableStack := []clickableBounds{}
	currentParsingId := -1

	strippedFrameBuilder := strings.Builder{}

	for i, r := range frame {
		if r == '\uFFF9' {
			currentParsingId = 0
			parsingClickableStack = append(parsingClickableStack, clickableBounds{
				start:            current,
				sequencePosition: i,
			})
		} else if r == '\uFFFA' {
			if currentParsingId == -1 {
				// parse error: hit \uFFFA without \uFFF9 starting the sequence.
				// abort the sequence.
				cr.nextRegistered = map[int]clickable{}
				return frame
			}

			currentParsingId++
		} else if r == '\uFFFB' {
			// terminate the parse of the current clickable
			last := len(parsingClickableStack) - 1
			if last < 0 {
				// We tried popping a clickable off an empty stack
				//
				// abort clickable parsing and delete the current, probably mis-parsed map
				cr.nextRegistered = map[int]clickable{}
				return frame
			}
			existing, has := cr.nextRegistered[currentParsingId]
			// check that this was registered for the next generation
			if has && existing.generation == cr.currentGeneration+1 {
				// update the bounds and render generation
				// to the current render generation
				existing.bounds = parsingClickableStack[last]
				existing.bounds.end = prev
				cr.nextRegistered[currentParsingId] = existing
			} else {
				// Unexpected state:
				// This sequence that references a clickable that was
				// not registered in the current frame
				//
				// abort clickable parsing and delete the current, probably mis-parsed map
				cr.nextRegistered = map[int]clickable{}
				return frame

			}

			parsingClickableStack = parsingClickableStack[:last]
			if last == 0 {
				// clear parsing IDs
				currentParsingId = -1
			} else {
				// continue parsing IDs
				currentParsingId = 0
			}
		} else {
			if currentParsingId > 0 {
				// This is an invalid state: it means we hit the sequence:
				// \uFFF9 ... \uFFFA\uFFFA... (non-control-character),
				// but we expect the sequence to have a contiguous sequence of \uFFFA
				// terminated by \uFFFB
				//
				// abort clickable parsing and delete the current, probably mis-parsed map
				cr.currentRegistered = map[int]clickable{}
				return frame
			}

			prev = current

			if r == '\r' {
				current.x = 0
			} else if r == '\n' {
				current.x = 0
				current.y += 1
			} else {
				current.x += runewidth.RuneWidth(r)
			}

			strippedFrameBuilder.WriteRune(r)
		}
	}

	if currentParsingId != -1 {
		// This is an invalid state: it means we had an unterminated sequence
		cr.currentRegistered = map[int]clickable{}
		return frame
	}

	return strippedFrameBuilder.String()
}

// Swaps the double buffer and increments the generation count.
//
// Call this after the next frame is flushed to the display
func (cr *clickableState) swapDoubleBuffer() {
	cr.currentRegistered, cr.nextRegistered = cr.nextRegistered, cr.currentRegistered
	cr.currentGeneration += 1
}

// after stripFrame() is called, getClicked() can be called to
// translate the a position in current frame into a clicked object
//
// If no object is clicked, nil will be returned
func (cs *clickableState) getClicked(x int, y int) interface{} {
	var bestClicked clickable
	for _, clickable := range cs.currentRegistered {
		if clickable.generation == cs.currentGeneration &&
			clickable.bounds.containsPoint(cell{x, y}) &&
			clickable.bounds.sequencePosition >= bestClicked.bounds.sequencePosition {
			bestClicked = clickable
		}
	}

	return bestClicked.data
}

// registers a clickable to the clickable state,
// wrapping the text
func (cr *clickableState) registerAndWrap(
	wrapped string,
	// stable key to identify the object between renders
	key string,
	data interface{},
) string {
	id := cr.stableId(key)
	cr.nextRegistered[id] = clickable{
		data:       data,
		generation: cr.currentGeneration + 1,
	}

	builder := strings.Builder{}
	// start the annotated text
	builder.WriteRune('\uFFF9')
	builder.WriteString(wrapped)
	// end the annotated text and start the annotation
	// hack: we need our annotation to be non-printable
	// characters with no specified meaning, so represent the ID of the
	// string by repeating the 'annotation start' character <ID> times
	for i := 0; i < id; i++ {
		builder.WriteRune('\uFFFA')
	}

	// end the annotation
	builder.WriteRune('\uFFFB')

	return builder.String()
}

func (cr *clickableState) stableId(key string) int {
	if existingId, hasExistingId := cr.stableKeyMap[key]; hasExistingId {
		return existingId
	}

	// allocate and save a new Id
	id := cr.idCounter
	cr.idCounter += 1
	cr.stableKeyMap[key] = id
	return id
}
