package rpg

import (
	"encoding/json"
	"errors"
	"maps"
	"slices"

	lang "github.com/nao1215/gozen/internal/text"
)

// Cursor is where a save made in the middle of an event goes on (save format version 2,
// ADR 0025). The save's state is the one when the event began; a loader starts the event
// again (Event, or the commands of arriving at the state's location when Arrival) and reads
// on to Line with Answers, the same replay as a note of F9 and F5.
type Cursor struct {
	// Event is the name of the commands (an event id, or a place's action or person as the
	// screen names them); empty with Arrival.
	Event string `json:"event,omitempty"`
	// Arrival: the event is what happens on arriving at the state's location.
	Arrival bool `json:"arrival,omitempty"`
	// Line is the count of lines and choices shown, the one on the screen included.
	Line int `json:"line"`
	// Answers are the options taken in the event, in order.
	Answers []int `json:"answers,omitempty"`
}

// Resume is the cursor of a loaded save and the random generator's bytes when the event
// began (nil bytes: none were saved).
type Resume struct {
	Cursor Cursor
	Rand   []byte
}

// valid reports whether the cursor can be read on: a line, an event or the arrival (one
// of them), and answers that are options (none below 0).
func (c Cursor) valid() bool {
	return c.Line >= 1 && (c.Event == "") == c.Arrival && !slices.ContainsFunc(c.Answers, func(n int) bool { return n < 0 })
}

// WithCursor adds a cursor and the random bytes to a save made by Save at the start of the
// event (the state of the save is the event's start).
func WithCursor(save []byte, c Cursor, rnd []byte) ([]byte, error) {
	var e saveEnvelope
	if err := json.Unmarshal(save, &e); err != nil {
		return nil, err
	}
	if e.FormatVersion != saveFormatVersion {
		return nil, errors.New(lang.T("カーソルは現在の保存形式のセーブにだけ足せる"))
	}
	if !c.valid() {
		return nil, errors.New(lang.T("カーソルの行またはイベントが不正"))
	}
	c.Answers = slices.Clone(c.Answers)
	e.Cursor, e.Rand = &c, slices.Clone(rnd)
	return json.MarshalIndent(e, "", "  ")
}

// WithRand adds the random generator's bytes to a save made by Save between events, so
// that a loader draws the same numbers as the game saved would have.
func WithRand(save, rnd []byte) ([]byte, error) {
	if len(rnd) == 0 {
		return save, nil
	}
	var e saveEnvelope
	if err := json.Unmarshal(save, &e); err != nil {
		return nil, err
	}
	if e.FormatVersion != saveFormatVersion || e.Cursor != nil {
		return nil, errors.New(lang.T("乱数はカーソルのない現在の保存形式のセーブにだけ足せる"))
	}
	e.Rand = slices.Clone(rnd)
	return json.MarshalIndent(e, "", "  ")
}

// choseView is chose.<choice id>: the id of the option last taken at that choice (its number
// from 0 when the option has no id), "" before it was answered.
type choseView struct{ s *State }

func (v choseView) Field(id string) (any, bool) { return v.s.Chose[id], true }

func (v choseView) Keys() []string { return slices.Sorted(maps.Keys(v.s.Chose)) }
