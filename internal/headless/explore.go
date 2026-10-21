package headless

// Explore is a tool (for tests) that runs an event through "all branches" without a screen.
// It runs commands from a copy of the state and tries each choice item and each battle outcome (win, lose, escape) as a branch.
// The same question (the question text and item order at the same place in the data; for a battle, the enemy group and the commands after win or lose) is
// branched only once, so the number of paths is about the sum of the question items (not every combination).

import (
	"errors"
	"fmt"
	"math/rand/v2"
	"runtime/debug"
	"strconv"
	"strings"

	"github.com/nao1215/gozen/internal/event"
	data "github.com/nao1215/gozen/internal/layer2/gamedata"
	"github.com/nao1215/gozen/internal/layer3/dungeon"
	"github.com/nao1215/gozen/internal/layer4/itemuse"
	"github.com/nao1215/gozen/internal/rpg"
	"github.com/nao1215/gozen/internal/world"
)

// Config holds the rules for Explore.
type Config struct {
	// MaxPaths is the upper bound of paths tried from one root (300 if 0).
	MaxPaths int
	// MaxSteps is the upper bound of requests received on one path (lines, choices, etc.) plus arrival handling
	// (20000 if 0). Exceeding it is recorded in Errs as an event that never ends.
	MaxSteps int
	// Seed is the random seed per path (the same path is rerun with the same random values).
	Seed uint64
	// With NoArrive, the arrival event (enter) and auto are not run when moving between places.
	NoArrive bool
	// Outcomes are the battle results to try (event.BattleWon and so on). If nil, win and lose, and escape too for a herd that can be escaped from.
	Outcomes func(st *rpg.State, b event.Battle) []int
	// If Fight is non-nil, battles do not branch and proceed with Fight's result (actually fighting). A negative return branches.
	Fight func(st *rpg.State, b event.Battle) int
	// If Choose is non-nil, choices do not branch and proceed with Choose's answer (random play). A negative return branches.
	Choose func(st *rpg.State, q event.Choice) int
	// Check is called at the end of a path (and at every request with CheckEvery), and adds the errors it returns to Errs (invariant checking).
	Check      func(st *rpg.State) error
	CheckEvery bool
	// Prepare adjusts the copied state before a path runs (does nothing if nil).
	Prepare func(st *rpg.State)
}

// Path is one path that was tried.
type Path struct {
	Answers []int // sequence of chosen items and battle results (only at branch points; for a battle, the number of the result tried)
	// Picks is the whole sequence of answers (including answers from Choose and Fight; choices are numbers, battles are event.BattleWon etc.).
	Picks   []int
	Taken   []string
	St      *rpg.State // state at the end of the path
	Lines   []string   // lines read ("name: text") and items chosen ("> item")
	Notices []string   // short notices at the top of the screen (notice; no conversation window shown)
	Images  []string   // CGs shown (files one at a time; "" when cleared)
	Moves   []string
	Battles []string // e.g. "enemy group:won"
	Errs    []error
	Over    bool // the party was wiped out (game_over.event was also run if present)
	// Left is set only on the last path: the number of branches cut off by MaxPaths and not yet tried (0 if all were tried).
	Left    int
	Cleared bool
	// Steps lists lines, CGs, music, moves and choices in the order they happened (for tests that look at what comes before and after lines and images).
	Steps []Step
}

// Step is one occurrence on a path (Kind is say, image, music, move, choice, notice or sfx).
type Step struct {
	Kind  string
	Who   string // say: speaker name (empty for narration). stand: the actor ("" for everyone off)
	Actor string // say: the speaker's actor ID (empty if not a party member)
	Face  string // say: face image
	Text  string // say, notice, choice: the text (for choice, the chosen item). sfx: the sound effect name
	File  string // image: the image shown (empty when cleared). voice: the voice's file
	Faces bool   // image: do not hide the portraits
	Song  string // music: the song
	Loc   string // move: the destination
}

// Log is the path's lines, choices and battles, for an error message.
func (p *Path) Log() string {
	return strings.Join(p.Taken, " / ")
}

// Explore runs cmds from a copy of st and returns the paths with every branch tried (st is not changed).
func Explore(db *data.DB, st *rpg.State, cmds []data.Command, opt Config) []*Path {
	if opt.MaxPaths < 0 || opt.MaxSteps < 0 {
		return []*Path{{Errs: []error{errors.New("explore: limits must not be negative")}}}
	}
	if opt.MaxPaths == 0 {
		opt.MaxPaths = 300
	}
	if opt.MaxSteps == 0 {
		opt.MaxSteps = 20000
	}
	base, err := st.Save()
	if err != nil {
		return []*Path{{Errs: []error{err}}}
	}
	queue := [][]int{nil}
	expanded := map[string]bool{}
	var out []*Path
	for len(queue) > 0 && len(out) < opt.MaxPaths {
		prefix := queue[0]
		queue = queue[1:]
		p, branches := explorePath(db, base, cmds, prefix, opt, expanded)
		out = append(out, p)
		queue = append(queue, branches...)
	}
	if len(out) > 0 {
		out[len(out)-1].Left = len(queue)
	}
	return out
}

type explorer struct {
	db    *data.DB
	st    *rpg.State
	opt   Config
	p     *Path
	moved bool
}

func (e *explorer) move(loc string) {
	if e.db.Locations[loc] == nil {
		e.p.Errs = append(e.p.Errs, fmt.Errorf("move to a location that does not exist: %q", loc))
		return
	}
	e.st.Location = loc
	e.st.Steps++
	e.moved = true
	e.p.Moves = append(e.p.Moves, loc)
	e.p.Steps = append(e.p.Steps, Step{Kind: "move", Loc: loc})
}

func (e *explorer) music(c data.MusicCue) {
	e.p.Steps = append(e.p.Steps, Step{Kind: "music", Song: c.Song})
}
func (e *explorer) mood(string) {}
func (e *explorer) sFX(name string) {
	e.p.Steps = append(e.p.Steps, Step{Kind: "sfx", Text: name})
}
func (e *explorer) effect(string) {}
func (e *explorer) image(cg data.CG) {
	if cg.File != "" && !e.db.Exists("assets/"+cg.File) {
		e.p.Errs = append(e.p.Errs, fmt.Errorf("image %q does not exist", cg.File))
	}
	e.p.Images = append(e.p.Images, cg.File)
	e.p.Steps = append(e.p.Steps, imageStep(e.db, cg))
}

func explorePath(db *data.DB, base []byte, cmds []data.Command, prefix []int, opt Config, expanded map[string]bool) (p *Path, branches [][]int) {
	p = &Path{}
	st, err := rpg.Parse(db, base, rand.New(rand.NewPCG(opt.Seed, 0x5eed))) //nolint:gosec // Deterministic gameplay randomness, not security-sensitive data.
	if err != nil {
		p.Errs = append(p.Errs, err)
		return p, nil
	}
	st.Calm = true
	if opt.Prepare != nil {
		opt.Prepare(st)
	}
	dungeon.Remake(db, st)
	p.St = st
	e := &explorer{db: db, st: st, opt: opt, p: p}
	r := event.New(db, st, event.HostFunc(e.handle))
	r.Log = func(err error) { p.Errs = append(p.Errs, err) }
	r.Dev, r.UseItem = true, itemuse.For(db, st)
	defer func() {
		if x := recover(); x != nil {
			p.Errs = append(p.Errs, fmt.Errorf("panic: %v (after %s)\n%s", x, p.Log(), debug.Stack()))
		}
	}()
	seen := map[string]bool{}
	check := func() {
		if opt.Check == nil {
			return
		}
		if err := opt.Check(st); err != nil && !seen[err.Error()] {
			seen[err.Error()] = true
			p.Errs = append(p.Errs, fmt.Errorf("%w (after %s)", err, p.Log()))
		}
	}
	decide := func(key string, n int, label func(int) string) int {
		i := len(p.Answers)
		a := 0
		if i < len(prefix) {
			a = min(prefix[i], n-1)
		} else if !expanded[key] {
			expanded[key] = true
			for alt := 1; alt < n; alt++ {
				b := append(append([]int{}, p.Answers...), alt)
				branches = append(branches, b)
			}
		}
		p.Answers = append(p.Answers, a)
		p.Taken = append(p.Taken, label(a))
		return a
	}
	r.Start(cmds)
	overRun := false
	steps := 0
	autos := 0
	for {
		steps++
		if steps > opt.MaxSteps {
			p.Errs = append(p.Errs, fmt.Errorf("the events do not end (%d steps; after %s)", steps, p.Log()))
			break
		}
		if r.Running() {
			req := r.Next()
			switch q := req.(type) {
			case event.Say:
				line := q.Text
				if q.Name != "" {
					line = q.Name + "「" + q.Text + "」"
				}
				p.Lines = append(p.Lines, line)
				p.Steps = append(p.Steps, Step{Kind: "say", Who: q.Name, Actor: q.Actor, Face: q.Face, Text: q.Text})
				if q.Face != "" && !db.Exists("assets/"+q.Face) {
					p.Errs = append(p.Errs, fmt.Errorf("face %q does not exist (%s)", q.Face, line))
				}
			case event.Notice:
				p.Notices = append(p.Notices, q.Text)
				p.Steps = append(p.Steps, Step{Kind: "notice", Text: q.Text})
			case event.Choice:
				n := -1
				if opt.Choose != nil {
					n = opt.Choose(st, q)
				}
				if n >= 0 {
					n = min(len(q.Options)-1, n)
					p.Taken = append(p.Taken, "> "+q.Options[n])
				} else {
					key := "choice:" + place(r.Asking()) + q.Prompt + "\x00" + strings.Join(q.Options, "\x00")
					n = decide(key, len(q.Options), func(a int) string { return "> " + q.Options[a] })
				}
				p.Lines = append(p.Lines, "> "+q.Options[n])
				p.Steps = append(p.Steps, Step{Kind: "choice", Text: q.Options[n]})
				p.Picks = append(p.Picks, n)
				r.Answer(n)
			case event.Battle:
				if db.Troops[q.Troop] == nil {
					p.Errs = append(p.Errs, fmt.Errorf("battle with a troop that does not exist: %q", q.Troop))
				}
				res := -1
				if opt.Fight != nil {
					st.Fighters = q.Fighters
					res = opt.Fight(st, q)
					st.Fighters = nil
				}
				if res >= 0 {
					p.Taken = append(p.Taken, "battle "+q.Troop+" "+resultName(res))
				} else {
					outs := []int{event.BattleWon, event.BattleLost}
					if t := db.Troops[q.Troop]; t != nil && t.Escapable() {
						outs = append(outs, event.BattleFled)
					}
					if opt.Outcomes != nil {
						outs = opt.Outcomes(st, q)
					}
					if len(outs) == 0 {
						p.Errs = append(p.Errs, errors.New("explore: no battle outcomes"))
						return p, branches
					}
					a := decide("battle:"+place(r.Asking())+q.Troop, len(outs), func(a int) string { return "battle " + q.Troop + " " + resultName(outs[a]) })
					res = outs[a]
					if res == event.BattleLost && q.CanLose {
						// The event that catches a defeat continues with only the members who fought at HP 1, same as the screen.
						fought := st.Members()
						if len(q.Fighters) > 0 {
							fought = nil
							for _, id := range q.Fighters {
								if a := st.Actors[id]; a != nil {
									fought = append(fought, a)
								}
							}
						}
						for _, m := range fought {
							m.HP = max(1, m.HP)
						}
					}
				}
				p.Battles = append(p.Battles, q.Troop+":"+resultName(res))
				p.Steps = append(p.Steps, Step{Kind: "battle", Text: q.Troop + ":" + resultName(res)})
				p.Picks = append(p.Picks, res)
				r.Answer(res)
				if res == event.BattleWon {
					r.Won()
				}
			case event.GameOver:
				p.Over = true
				p.Taken = append(p.Taken, "game over")
				ev := db.Events[db.Game.GameOver.Event]
				if ev == nil || overRun {
					check()
					return p, branches
				}
				overRun = true
				for _, m := range st.Members() {
					m.Recover()
				}
				r.Start(ev.Do)
			case event.Clear:
				p.Cleared = true
				p.Taken = append(p.Taken, "clear")
				check()
				return p, branches
			}
			if req != nil && opt.CheckEvery {
				check()
			}
			continue
		}
		if opt.NoArrive {
			break
		}
		if e.moved {
			e.moved = false
			r.Start(world.Arrive(db, st, false))
			continue
		}
		if c := world.Auto(db, st); c != nil {
			autos++
			if autos > 200 {
				p.Errs = append(p.Errs, errors.New("an auto event keeps starting (its when stays true)"))
				break
			}
			r.Start(c)
			continue
		}
		break
	}
	check()
	return p, branches
}

// place tells a choice or a battle of one place in the data from one of another that reads
// the same: by the data's own slices of its branches (shared by the copies of the command
// that world.Arrive makes, so the same place keeps the same key on every path).
func place(c *data.Command) string {
	if c == nil {
		return ""
	}
	var b strings.Builder
	if len(c.Choice) > 0 {
		fmt.Fprintf(&b, "%p", &c.Choice[0])
	}
	for _, s := range [][]data.Command{c.OnWin, c.OnLose, c.OnRun} {
		if len(s) > 0 {
			fmt.Fprintf(&b, "%p", &s[0])
		} else {
			b.WriteString("-")
		}
	}
	return b.String() + "\x00"
}

func resultName(res int) string {
	switch res {
	case event.BattleWon:
		return "won"
	case event.BattleLost:
		return "lost"
	case event.BattleFled:
		return "fled"
	}
	return strconv.Itoa(res)
}

func (e *explorer) handle(eff event.Effect) {
	switch eff := eff.(type) {
	case event.MoveEffect:
		e.move(eff.Location)
	case event.MusicEffect:
		e.music(eff.Cue)
	case event.MoodEffect:
		e.mood(eff.Name)
	case event.ImageEffect:
		e.image(eff.Picture)
	case event.SoundEffect:
		e.sFX(eff.Name)
	case event.VisualEffect:
		e.effect(eff.Name)
	case event.StandEffect:
		e.p.Steps = append(e.p.Steps, Step{Kind: "stand", Who: eff.Who, Face: eff.Face})
	case event.VoiceEffect:
		e.p.Steps = append(e.p.Steps, Step{Kind: "voice", Who: eff.Who, File: eff.File})
	}
}
