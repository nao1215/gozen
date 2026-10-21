package gozen

import (
	"context"
	"errors"
	"strings"

	"github.com/nao1215/gozen/internal/event"
	"github.com/nao1215/gozen/internal/rpg"

	"github.com/nao1215/gozen/internal/headless"
	"github.com/nao1215/gozen/internal/layer2/gamedata"
	"github.com/nao1215/gozen/internal/layer3/dungeon"
)

// ExploreOptions limit Explore. The zero value tries 300 paths of 20000 requests each,
// with the session's seed, branching at every choice and battle.
type ExploreOptions struct {
	MaxPaths, MaxSteps int
	Seed               uint64
	// Policy answers the choices and battles that should not branch: it returns an
	// answer as for Answer, or Branch to try them all there. Nil branches everywhere.
	Policy Policy
	// NoArrive runs only the commands: moving to a place does not run its arrival (its
	// enter events, random battle) nor the auto events.
	NoArrive bool
}

// Branch is the answer of an Explore policy that tries every answer of a choice or a
// battle. In Do it is the first option, or Auto.
const Branch = -1

// Path is one way through the commands: the answers taken in order, in the words of
// Answer (a choice's option; a battle's Won, Lost or Fled: a battle is decided, not
// fought), what was shown and done, the problems met, and the state at its end.
type Path struct {
	Answers  []int
	Requests []Request // say, choice (Text the option taken), notice, battle (ID the troop, Text win, lose or escape)
	Effects  []Effect  // image, music, move; At places each among the Requests
	Errs     []error
	End      *Session // read it with Query; it is a session of its own
}

// Explore runs the commands on copies of the session's state, taking every option of
// every choice and every result of every battle (win, lose, and escape where it can)
// once, and returns each path. It is not every combination: a choice met again is
// tried once. The session itself does not change.
func Explore(ctx context.Context, s *Session, script string, opt ExploreOptions) ([]Path, error) {
	if ctx == nil || s == nil {
		return nil, errors.New("gozen: explore needs a context and a session")
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	cmds, err := s.parse(script)
	if err != nil {
		return nil, err
	}
	seed := opt.Seed
	if seed == 0 {
		// the seed is read from a copy, so exploring leaves the session's random numbers alone
		cp := *s.pcg
		seed = cp.Uint64()
	}
	db := gamedata.Fork(s.db)
	cfg := headless.Config{MaxPaths: opt.MaxPaths, MaxSteps: opt.MaxSteps, Seed: seed, NoArrive: opt.NoArrive}
	if pol := opt.Policy; pol != nil {
		view := func(st *rpg.State) *Session { // the policy reads the path's state through a session
			v := &Session{g: s.g, db: db, st: st, cfg: s.cfg, last: s.last}
			v.start()
			return v
		}
		cfg.Choose = func(st *rpg.State, q event.Choice) int {
			return pol.Decide(ctx, view(st), Request{Kind: "choice", Text: q.Prompt, Options: q.Options})
		}
		cfg.Fight = func(st *rpg.State, b event.Battle) int {
			v := view(st)
			switch n := pol.Decide(ctx, v, v.request(b)); n {
			case Branch, Won, Lost, Fled:
				return n
			default:
				return v.fight(b)
			}
		}
	}
	got := headless.Explore(db, s.st, cmds, cfg)
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	out := make([]Path, 0, len(got))
	for _, p := range got {
		path := Path{Answers: p.Picks, Errs: p.Errs}
		for _, st := range p.Steps {
			at := len(path.Requests)
			switch st.Kind {
			case "say":
				path.Requests = append(path.Requests, Request{Kind: "say", Text: st.Text, Who: st.Who, Face: st.Face, ID: st.Actor})
			case "choice", "notice":
				path.Requests = append(path.Requests, Request{Kind: st.Kind, Text: st.Text})
			case "battle": // herd and result (win, lose, escape)
				troop, res, _ := strings.Cut(st.Text, ":")
				path.Requests = append(path.Requests, Request{Kind: "battle", ID: troop, Text: map[string]string{"won": "win", "lost": "lose", "fled": "escape"}[res]})
			case "image":
				path.Effects = append(path.Effects, Effect{Kind: "image", Name: st.File, Faces: st.Faces, At: at})
			case "music":
				path.Effects = append(path.Effects, Effect{Kind: "music", Name: st.Song, At: at})
			case "move":
				path.Effects = append(path.Effects, Effect{Kind: "move", Name: st.Loc, At: at})
			case "stand":
				path.Effects = append(path.Effects, Effect{Kind: "stand", Name: st.Who, At: at})
			case "voice":
				path.Effects = append(path.Effects, Effect{Kind: "voice", Name: st.File, At: at})
			}
		}
		if p.St != nil {
			end := newSession(s.g, s.cfg)
			end.db = gamedata.Fork(db)
			p.St.DB = end.db
			end.st = p.St
			end.last.Over, end.last.Cleared = p.Over, p.Cleared
			end.start()
			dungeon.Remake(end.db, end.st)
			path.End = end
		}
		out = append(out, path)
	}
	return out, nil
}
