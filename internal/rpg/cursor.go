package rpg

import (
	"encoding/hex"
	"encoding/json"
	"errors"
	"maps"
	"slices"
	"strings"

	data "github.com/nao1215/gozen/internal/layer2/gamedata"
	lang "github.com/nao1215/gozen/internal/text"
)

// Cursor is where a save made in the middle of an event goes on (save format version 2,
// ADR 0025). The save's state is the one when the event began; a loader starts the event
// again (Event, or the commands of arriving at the state's location when Arrival) and reads
// on to Line with Answers, the same replay as a note of F9 and F5. Session screen
// continuations additionally replay Inputs after that optional dialogue prefix.
type Cursor struct {
	// Inputs is a bounded session continuation transcript from the named root's
	// snapshot. Session replays it on private state; other frontends must reject it
	// until they implement the same input contract. Line/Answers optionally describe
	// an earlier dialogue cursor to replay before these inputs.
	Inputs []ReplayInput `json:"inputs,omitempty"`
	// Args are frozen root bindings, validated against the current declaration.
	Args map[string]any `json:"args,omitempty"`
	// Fingerprint protects schema-10 named replay roots against changed commands.
	Fingerprint string `json:"fingerprint,omitempty"`
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

// ReplayInput records either a request digest or a visible choice/screen answer.
// Session owns request encoding and replay; rpg validates the stored value shape.
// Request is a SHA-256 hex digest. Answer is nil for requests, and may be -1 to
// close an authored screen. No runner frames or UI pointers are serialized.
type ReplayInput struct {
	Request string `json:"request,omitempty"`
	Answer  *int   `json:"answer,omitempty"`
}

// Resume is the cursor of a loaded save and the random generator's bytes when the event
// began (nil bytes: none were saved).
type Resume struct {
	Cursor Cursor
	Rand   []byte
}

// valid checks the dialogue cursor or bounded input transcript shape. The frontend
// must also check reachability against the named root before installing the state.
func (c Cursor) valid() bool {
	if len(c.Inputs) > 0 {
		if len(c.Inputs) > 20000 || c.Event == "" || c.Arrival || c.Line < 0 {
			return false
		}
		if c.Line == 0 && (len(c.Answers) != 0 || c.Inputs[0].Request == "") {
			return false
		}
		if slices.ContainsFunc(c.Answers, func(n int) bool { return n < 0 }) {
			return false
		}
		for _, input := range c.Inputs {
			if input.Answer != nil {
				if input.Request != "" || *input.Answer < -1 {
					return false
				}
			} else if digest, err := hex.DecodeString(input.Request); err != nil || len(digest) != 32 {
				return false
			}
		}
		return true
	}
	// Line 0 begins a named event from its start (a save at a checkpoint).
	return (c.Line >= 1 || c.Line == 0 && c.Event != "" && len(c.Answers) == 0) && (c.Event == "") == c.Arrival && !slices.ContainsFunc(c.Answers, func(n int) bool { return n < 0 })
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
	c.Args = maps.Clone(c.Args)
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

// Seal records the current definition for session/play saves. Legacy games keep
// their original cursor contract; arrival reconstruction is still owned by world.
func (c *Cursor) Seal(db *data.DB) error {
	if db.Game.SchemaVersion < data.SchemaVersion10 || c.Event == "" {
		return nil
	}
	fingerprint, err := db.CommandsFingerprint(c.Event)
	if err != nil {
		return err
	}
	c.Fingerprint = fingerprint
	return nil
}

func (c Cursor) validateBindings(db *data.DB, version int) error {
	if len(c.Inputs) > 0 && (version < 3 || db.Game.SchemaVersion < data.SchemaVersion10 || c.Fingerprint == "") {
		return errors.New("screen replay requires format 3, schema 10 and a sealed named root")
	}
	if len(c.Args) > 0 || c.Fingerprint != "" {
		if version < 3 || c.Arrival {
			return errors.New("bound cursor requires save format 3 and a named root")
		}
	}
	if db.Events[c.Event] != nil || strings.HasPrefix(c.Event, "screen:") {
		if _, err := db.ValidateArguments(c.Event, c.Args); err != nil {
			return err
		}
	} else if len(c.Args) > 0 {
		return errors.New("arguments require a named event")
	}
	if version >= 3 && db.Game.SchemaVersion >= data.SchemaVersion10 && c.Event != "" {
		fingerprint, err := db.CommandsFingerprint(c.Event)
		if err != nil {
			return err
		}
		if fingerprint != c.Fingerprint {
			return errors.New("replay action changed; restart the event instead of resuming this save")
		}
	}
	return nil
}
