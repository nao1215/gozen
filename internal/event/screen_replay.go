package event

import (
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"maps"

	"github.com/nao1215/gozen/internal/expr"
	data "github.com/nao1215/gozen/internal/model/gamedata"
	"github.com/nao1215/gozen/internal/rpg"
)

// ReplayView is the presentation-independent request fingerprint used by session
// saves and player continuation probes. Field order and JSON names are part of
// the existing schema-10 transcript format. Options belong to the caller.
type ReplayView struct {
	Kind      string
	Text      string
	Who, Face string
	Options   []string
	ID        string
	Seconds   float64
	Flags     string
}

// Answers is how many answers the request takes: its options, and for a muted
// choice (Seconds) staying quiet, the index past them.
func (v ReplayView) Answers() int {
	if v.Kind == "choice" && v.Seconds > 0 {
		return len(v.Options) + 1
	}
	return len(v.Options)
}

// Digest identifies a presented request without storing its text in a save.
func (v ReplayView) Digest() (string, error) {
	b, err := json.Marshal(v)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%x", sha256.Sum256(b)), nil
}

// ScreenReplayFrame is a suspended opener reconstructed for session and play.
// It contains live runners, never serialized frames. Args and Indices are owned
// by the result; definitions remain read-only. Frames are outermost first.
type ScreenReplayFrame struct {
	Name, Origin string
	Destination  string
	Args         map[string]any
	Runner       *Runner
	Indices      []int
	Rows         []ScreenRow
}

// ScreenReplay is a validated continuation on the caller's private state.
// Request is nil immediately after an answer; View also identifies an idle
// authored screen, whose original Open request need not be fetched again.
type ScreenReplay struct {
	Runner  *Runner
	Frames  []ScreenReplayFrame
	Request Request
	View    ReplayView
}

// ReplayRequest describes requests that can be recorded by session and play.
// Authored opens need their evaluated title and visible entries from the frontend.
func ReplayRequest(q Request) (ReplayView, bool) {
	switch q := q.(type) {
	case Say:
		return ReplayView{Kind: "say", Text: q.Text, Who: q.Name, Face: q.Face, ID: q.Actor}, true
	case Choice:
		return ReplayView{Kind: "choice", Text: q.Prompt, Options: q.Options, Seconds: q.Muted}, true
	case Wait:
		return ReplayView{Kind: "wait", Seconds: q.Seconds}, true
	case Tap:
		return ReplayView{Kind: "tap", Seconds: q.Auto}, true
	case Notice:
		return ReplayView{Kind: "notice", Text: q.Text}, true
	}
	return ReplayView{}, false
}

func (s *ScreenReplay) screen() {
	f := &s.Frames[len(s.Frames)-1]
	env := expr.Chain{expr.Map{"arg": expr.Map(f.Args)}, s.Runner.st.Env()}
	s.Request = Open{Screen: f.Name, Args: maps.Clone(f.Args)}
	s.View = ReplayView{Kind: "open", ID: f.Name, Text: data.Render(s.Runner.db.Tr(s.Runner.db.Game.Screens[f.Name].Title), env)}
	f.Indices = nil
	f.Rows = nil
	for index, entry := range ScreenEntries(s.Runner.db, f.Name, env, s.Runner.Log) {
		f.Indices = append(f.Indices, index)
		f.Rows = append(f.Rows, entry)
		s.View.Options = append(s.View.Options, entry.Label)
	}
}

// ReplayScreens rebuilds a bounded screen transcript from an already started
// root (optionally advanced by ReplayTo). pending is that prefix's unanswered
// choice, or nil. Session restoration and player probes supply private state,
// RNG and a host that suppresses external effects. Failure can mutate only that
// private state. Successful runners retain the root's host/configuration; child
// starts do not invoke OnStart during reconstruction. Moves, logged execution
// errors and non-replayable requests are rejected before returning a result.
func (r *Runner) ReplayScreens(inputs []rpg.ReplayInput, pending Request) (ScreenReplay, error) {
	s := ScreenReplay{Runner: r, Request: pending}
	if pending != nil {
		s.View, _ = ReplayRequest(pending)
	}
	if len(inputs) == 0 || len(inputs) > 20000 {
		return s, errors.New("load: invalid screen replay length")
	}
	var failure error
	var runners []*Runner
	host, log := r.host, r.Log
	attach := func(runner *Runner) {
		runners = append(runners, runner)
		runner.host = HostFunc(func(e Effect) {
			if _, ok := e.(MoveEffect); ok {
				failure = errors.New("load: screen replay crossed a move")
			}
			if host != nil {
				host.Handle(e)
			}
		})
		runner.Log = func(err error) { failure = err }
	}
	attach(r)
	defer func() {
		for _, runner := range runners {
			runner.host, runner.Log = host, log
		}
	}()
	sawScreen := false
	for _, input := range inputs {
		if input.Answer != nil {
			n := *input.Answer
			if (s.View.Kind != "open" && s.View.Kind != "choice") || n < 0 && (n != -1 || s.View.Kind != "open") || n >= s.View.Answers() {
				return s, errors.New("load: invalid screen replay answer")
			}
			if s.View.Kind == "choice" {
				s.Runner.Answer(n)
				s.Request, s.View = nil, ReplayView{}
			} else {
				f := &s.Frames[len(s.Frames)-1]
				if n == -1 {
					s.Runner = f.Runner
					s.Frames = s.Frames[:len(s.Frames)-1]
					s.Request, s.View = nil, ReplayView{}
				} else {
					entry := r.db.Game.Screens[f.Name].Entries[f.Indices[n]]
					visible, err := f.Rows[n].Available(r.db, entry, expr.Chain{expr.Map{"arg": expr.Map(f.Args)}, r.st.Env()})
					if err != nil {
						return s, err
					}
					if !visible {
						s.screen()
					} else if len(entry.Do) > 0 || entry.Destination != "" {
						f.Destination = entry.Destination
						child := New(r.db, r.st, host)
						child.Sortie, child.Dev, child.UseItem, child.Act = r.Sortie, r.Dev, r.UseItem, r.Act
						child.BlockAside = r.BlockAside
						attach(child)
						if err := child.StartBound(fmt.Sprintf("screen:%s:%d", f.Name, f.Indices[n]), f.Rows[n].Args); err != nil {
							return s, err
						}
						child.OnStart = r.OnStart
						s.Runner, s.Request, s.View = child, nil, ReplayView{}
					}
				}
			}
		} else {
			// Pending choices and idle screens are stable across repeated Next calls.
			if s.View.Kind != "open" && s.View.Kind != "choice" {
				s.Request, s.View = nil, ReplayView{}
				for steps := 0; ; steps++ {
					if steps >= 20000 {
						return s, errors.New("load: screen replay exceeded the request limit")
					}
					q := s.Runner.Next()
					if failure != nil {
						return s, failure
					}
					if q == nil {
						if s.Runner.Running() {
							continue
						}
						if len(s.Frames) == 0 {
							return s, errors.New("load: missing screen continuation")
						}
						f := &s.Frames[len(s.Frames)-1]
						s.Runner = f.Runner
						if f.Destination != "" {
							f.Name, f.Args, f.Destination = f.Destination, nil, ""
						}
						s.screen()
						break
					}
					if opened, ok := q.(Open); ok {
						if r.db.Game.Screens[opened.Screen] == nil || len(s.Frames) >= MaxScreenDepth {
							return s, errors.New("load: unsupported screen replay open")
						}
						s.Frames = append(s.Frames, ScreenReplayFrame{Name: opened.Screen, Origin: r.st.Location, Args: maps.Clone(opened.Args), Runner: s.Runner})
						sawScreen = true
						s.screen()
					} else {
						var ok bool
						s.View, ok = ReplayRequest(q)
						if !ok {
							return s, fmt.Errorf("load: screen replay crossed an unsupported boundary (%T)", q)
						}
						s.Request = q
					}
					break
				}
			}
			digest, err := s.View.Digest()
			if err != nil {
				return s, err
			}
			if digest != input.Request {
				return s, errors.New("load: screen replay request changed")
			}
		}
		if failure != nil {
			return s, failure
		}
	}
	if !sawScreen || len(s.Frames) == 0 && !s.Runner.Running() {
		return s, errors.New("load: missing screen continuation")
	}
	return s, nil
}
