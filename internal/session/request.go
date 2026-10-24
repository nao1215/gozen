package session

import (
	"context"
	"slices"
	"strings"
)

// Request is something a screen shows: the same words as the trace and the YAML
// (Kind "say" is the say command). Only the fields of its kind are set.
type Request struct {
	// Kind is say, choice, battle, shop, wait, open, notice, tap, save, clear,
	// game_over or clash (a campaign's march shown as units: ID the region, Text won,
	// lost or draw; it needs no answer); and, only seen by Config.OnRequest, step (a line of a battle fought on
	// AUTO) and decision (an AI's choice: Who the actor, Text the skill or item).
	Kind string
	// Text is the line of say, notice and step, and the prompt of choice.
	Text string
	// Who is the speaker of say ("" for narration); Face its picture under assets/.
	Who, Face string
	// Options are the options of choice, copied for each returned request and callback.
	Options []string
	// ID is the troop of battle, the shop of shop, the screen of open, and the speaker's
	// actor of say ("" when the speaker is not an actor).
	ID string
	// Seconds is the time of wait, and the time before tap goes on by itself (0: never).
	Seconds float64
	// Flags are the battle's can_lose and no_retry, comma-separated.
	Flags string
}

func cloneRequest(q Request) Request {
	q.Options = slices.Clone(q.Options)
	return q
}

// The answers to a battle (Answer, Policy.Decide).
const (
	// Auto fights the battle on AUTO.
	Auto = 0
	// Won, Lost and Fled skip the battle with that result.
	Won  = 1
	Lost = 2
	Fled = 3
)

// Effect is an immediate effect: Kind move (Name the location), music (the song), mood,
// image (the picture's file; "" hides it), sound or visual (the name), stand (the actor
// brought onto or taken off the stage of the novel window; "" for everyone off), voice
// (the file under assets/ of the voice of the line that comes next), order (the id of a
// campaign's order given) or end_turn (the turn that begins, as a number).
type Effect struct {
	Kind, Name string
	// Faces: the image keeps the speakers' portraits shown (image's faces).
	Faces bool
	// At is, in a Path, how many Requests came before it (its place in the order).
	At int
}

// Policy answers the choices (the option's index) and battles (Auto, Won, Lost, Fled)
// that Session.Do and Explore meet. Embed DefaultPolicy to answer only some yourself.
type Policy interface {
	Decide(ctx context.Context, s *Session, q Request) int
}

// DefaultPolicy takes the first option of a choice and fights a battle on AUTO.
type DefaultPolicy struct {
	// Prefer are labels to take when a choice shows them, in order: the first label
	// that an option equals, else the first that an option contains. A choice without
	// any of them takes the first option.
	Prefer []string
}

// Decide answers the preferred option, else 0: the first option, or Auto.
func (p DefaultPolicy) Decide(_ context.Context, _ *Session, q Request) int {
	if q.Kind != "choice" {
		return 0
	}
	for _, exact := range []bool{true, false} {
		for _, want := range p.Prefer {
			for i, o := range q.Options {
				if exact && o == want || !exact && strings.Contains(o, want) {
					return i
				}
			}
		}
	}
	return 0
}
