package gozen

import (
	"context"
	"errors"
	"fmt"
	"math/rand/v2"
	"strings"

	"github.com/nao1215/gozen/internal/battle"
	"github.com/nao1215/gozen/internal/event"
	"github.com/nao1215/gozen/internal/layer2/gamedata"
	"github.com/nao1215/gozen/internal/layer3/dungeon"
	"github.com/nao1215/gozen/internal/layer3/field"
	"github.com/nao1215/gozen/internal/layer3/scope"
	"github.com/nao1215/gozen/internal/layer4/itemuse"
	"github.com/nao1215/gozen/internal/rpg"
	"github.com/nao1215/gozen/internal/world"
	data "github.com/nao1215/gozen/schema"
)

// Config is how a session answers and reports. The zero value works: choices take the
// first option, battles are fought on AUTO, and nothing is reported.
type Config struct {
	// Seed is the random seed; 0 picks a new one.
	Seed uint64
	// Dev lets set change an actor's stats and level (actor.ID.hp, mp, exp, level, sp):
	// development and tests. The zero value is the game as played: Do and Explore refuse
	// such a set (ErrInvalid), and the game's own events skip it.
	Dev bool
	// Policy answers the choices and battles that Do and Explore meet; nil is DefaultPolicy.
	Policy Policy
	// OnRequest sees every request shown, answered by the Policy or by the caller of Next.
	OnRequest func(Request)
	// OnEffect sees the immediate effects: move, music, mood, image, sound, visual.
	OnEffect func(Effect)
	// OnLog sees the problems that do not stop the play (an expression that fails).
	OnLog func(error)
}

// maxSteps ends a command whose events do not end (an auto event that keeps holding).
const maxSteps = 20000

// Session is one play of a game: its state, random numbers and running events. Use it
// from one goroutine.
type Session struct {
	g       *Game
	db      *data.DB
	st      *rpg.State
	pcg     *rand.PCG
	cfg     Config
	runner  *event.Runner
	last    scope.Last
	arrived bool
	quiet   bool // the next arrival has no random battle (the start of a game)
	pending *Request
	battle  event.Battle
	acts    []act // moves decided by the act command for the next battle
	errs    []error
	fresh   bool // nothing has been fed since New (play.New starts on screen from the opening event)
}

// New starts a session at the game's start (game.yaml start), running nothing yet;
// Begin runs the start event.
func New(g *Game, cfg Config) (*Session, error) {
	if g == nil {
		return nil, fmt.Errorf("%w: nil game", ErrInvalid)
	}
	s := newSession(g, cfg)
	s.st = rpg.New(s.db, rand.New(s.pcg)) //nolint:gosec // Gameplay randomness, not security.
	s.start()
	s.fresh = true
	return s, nil
}

// Restore continues a saved game (Session.Save). A save of another game or of an
// unknown format is ErrIncompatibleSave.
func Restore(g *Game, save []byte, cfg Config) (*Session, error) {
	if g == nil {
		return nil, fmt.Errorf("%w: nil game", ErrInvalid)
	}
	s := newSession(g, cfg)
	st, err := rpg.Parse(s.db, save, rand.New(s.pcg)) //nolint:gosec // Gameplay randomness, not security.
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrIncompatibleSave, err)
	}
	s.st = st
	s.start()
	dungeon.Remake(s.db, s.st)
	return s, nil
}

func newSession(g *Game, cfg Config) *Session {
	seed := cfg.Seed
	if seed == 0 {
		seed = rand.Uint64() //nolint:gosec // Gameplay randomness, not security.
	}
	if cfg.Policy == nil {
		cfg.Policy = DefaultPolicy{}
	}
	return &Session{g: g, db: gamedata.Fork(g.db), pcg: rand.NewPCG(seed, seed^0x9e3779b97f4a7c15), cfg: cfg}
}

// start makes the event runner of the state.
func (s *Session) start() {
	s.runner = event.New(s.db, s.st, event.HostFunc(s.effect))
	s.runner.Log = func(err error) {
		s.errs = append(s.errs, err)
		if s.cfg.OnLog != nil {
			s.cfg.OnLog(err)
		}
	}
	// Setting party stats or levels via set is allowed only during development (Config.Dev) (ADR 0021, author decision 5).
	s.runner.Dev, s.runner.UseItem = s.cfg.Dev, itemuse.For(s.db, s.st)
	s.runner.Act = func(actor, skill, item, target string) {
		s.acts = append(s.acts, act{actor, skill, item, target})
	}
}

// Begin starts a new game: the start event, then what happens on arriving at the start.
func (s *Session) Begin(ctx context.Context) error {
	s.fresh = false
	if ev := s.db.Events[s.db.Game.Start.Event]; ev != nil {
		s.runner.Start(ev.Do)
	}
	s.arrived, s.quiet = true, true
	dungeon.Ensure(s.db, s.st)
	return s.settle(ctx)
}

// Do runs a list of YAML commands to the end, one command at a time: after each one,
// what follows it (arriving at a place, the events that start by themselves) runs too,
// as if the player did it. The Policy answers the choices and battles. Invalid YAML or a
// command that does not fit the game is ErrInvalid with the Diagnostics; the problems
// met while running are returned joined, after everything ran.
func (s *Session) Do(ctx context.Context, script string) error {
	cmds, err := s.parse(script)
	if err != nil {
		return err
	}
	s.fresh = false
	var errs []error
	for i := range cmds {
		if s.last.Over || s.last.Cleared {
			break
		}
		s.runner.Start(cmds[i : i+1])
		if err := s.settle(ctx); err != nil {
			if ctx != nil && ctx.Err() != nil {
				return err
			}
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

// Start queues a list of YAML commands for a screen that shows them with Next and Answer.
func (s *Session) Start(script string) error {
	cmds, err := s.parse(script)
	if err != nil {
		return err
	}
	s.fresh = false
	s.runner.Start(cmds)
	return nil
}

func (s *Session) parse(script string) ([]gamedata.Command, error) {
	if !gamedata.IsScript(script) {
		return nil, invalid{Diagnostics{{File: gamedata.ScriptFile, Line: 1, Col: 1, Message: "want a list of commands (YAML starting with \"- \")", Severity: "error"}}}
	}
	if ds := gamedata.CheckScript(s.db, script, s.cfg.Dev); len(ds) > 0 {
		return nil, invalid{ds}
	}
	cmds, ds := gamedata.ParseScript(script)
	if len(ds) > 0 {
		return nil, invalid{ds}
	}
	return cmds, nil
}

// settle runs until nothing waits, answering with the Policy.
func (s *Session) settle(ctx context.Context) error {
	if ctx == nil {
		return errors.New("gozen: nil context")
	}
	defer func() { s.errs = nil }()
	for range maxSteps {
		if err := ctx.Err(); err != nil {
			return err
		}
		q := s.Next()
		switch q.Kind {
		case "":
			return errors.Join(s.errs...)
		case "choice", "battle":
			n := s.cfg.Policy.Decide(ctx, s, q)
			if n == Branch {
				n = 0
			}
			s.Answer(n)
		}
	}
	return errors.Join(append(s.errs, errors.New("gozen: the events do not end (an auto event that keeps holding?)"))...)
}

// Next runs until something must be shown and returns it; Kind "" means nothing is
// waiting. A choice or a battle waits for Answer (Next returns it again until then).
func (s *Session) Next() Request {
	if s.pending != nil {
		return *s.pending
	}
	for range maxSteps {
		if s.runner.Running() {
			r := s.runner.Next()
			if r == nil {
				continue
			}
			q := s.request(r)
			if s.cfg.OnRequest != nil {
				s.cfg.OnRequest(q)
			}
			if q.Kind == "choice" || q.Kind == "battle" {
				s.pending = &q
			}
			return q
		}
		if s.last.Over || s.last.Cleared {
			return Request{}
		}
		if s.arrived {
			s.arrived = false
			s.runner.Start(world.Arrive(s.db, s.st, !s.quiet))
			s.quiet = false
			continue
		}
		if cmds := field.Auto(s.db, s.st); cmds != nil {
			s.runner.Start(cmds)
			continue
		}
		if cmds := dungeon.Hunters(s.db, s.st); cmds != nil {
			s.runner.Start(cmds)
			continue
		}
		return Request{}
	}
	s.runner.Log(errors.New("gozen: the events do not end"))
	s.runner.Stop()
	return Request{}
}

// Answer answers the choice (the option's index) or the battle (Auto, Won, Lost or
// Fled) that Next returned. Other requests need no answer.
func (s *Session) Answer(n int) {
	q := s.pending
	if q == nil {
		return
	}
	s.pending = nil
	if q.Kind == "choice" {
		s.runner.Answer(n)
		return
	}
	b := s.battle
	res := n
	switch n {
	case Won, Lost, Fled:
		if n == Lost && b.CanLose {
			for _, m := range s.st.Members() {
				m.HP = max(1, m.HP)
			}
		}
	default:
		res = s.fight(b)
	}
	s.last.Result = scope.ResultName(res)
	s.runner.Answer(res)
	if res == Won {
		s.runner.Won()
	}
}

// fight fights the battle on AUTO, reporting each line as a "step" request.
func (s *Session) fight(q event.Battle) int {
	s.st.Fighters = q.Fighters
	defer func() { s.st.Fighters = nil }()
	b := battle.New(s.db, s.st, q.Troop)
	if s.cfg.OnRequest != nil {
		b.OnDecision = func(d battle.Decision) {
			s.cfg.OnRequest(Request{Kind: "decision", Who: d.User.ID, Text: d.Skill + d.Item})
		}
	}
	s.autoBattle(b, 80)
	if b.Result == battle.Ongoing { // hitting the move limit is a loss (same as headless)
		b.Result = battle.Lost
	}
	for _, line := range b.End() { // return HP, MP and states to the party; on a win, experience, money and items
		if s.cfg.OnRequest != nil {
			s.cfg.OnRequest(Request{Kind: "step", Text: line})
		}
	}
	switch b.Result {
	case battle.Won:
		return Won
	case battle.Fled:
		return Fled
	case battle.Ongoing, battle.Lost:
	}
	b.Rise() // as on screen, only the party members who fought get up with 1 HP
	return Lost
}

// request turns the runner's request into a Request.
func (s *Session) request(r event.Request) Request {
	switch r := r.(type) {
	case event.Say:
		return Request{Kind: "say", Text: r.Text, Who: r.Name, Face: r.Face, ID: r.Actor}
	case event.Choice:
		return Request{Kind: "choice", Text: r.Prompt, Options: r.Options}
	case event.Battle:
		s.battle = r
		var flags []string
		if r.CanLose {
			flags = append(flags, "can_lose")
		}
		if r.NoRetry {
			flags = append(flags, "no_retry")
		}
		return Request{Kind: "battle", ID: r.Troop, Flags: strings.Join(flags, ",")}
	case event.Shop:
		return Request{Kind: "shop", ID: r.ID}
	case event.Wait:
		return Request{Kind: "wait", Seconds: r.Seconds}
	case event.Save:
		return Request{Kind: "save"}
	case event.Open:
		return Request{Kind: "open", ID: r.Screen}
	case event.Notice:
		return Request{Kind: "notice", Text: r.Text}
	case event.Tap:
		return Request{Kind: "tap", Seconds: r.Auto}
	case event.Clear:
		s.last.Cleared = true
		return Request{Kind: "clear"}
	case event.GameOver:
		s.last.Over = true
		return Request{Kind: "game_over"}
	}
	return Request{Kind: fmt.Sprintf("%T", r)}
}

// effect applies and reports an immediate effect.
func (s *Session) effect(e event.Effect) {
	var out Effect
	switch e := e.(type) {
	case event.MoveEffect:
		s.st.Location = e.Location
		s.st.Steps++
		dungeon.Reveal(s.db, s.st)
		s.arrived = true
		out = Effect{Kind: "move", Name: e.Location}
	case event.MusicEffect:
		out = Effect{Kind: "music", Name: e.Cue.Song}
	case event.MoodEffect:
		out = Effect{Kind: "mood", Name: e.Name}
	case event.ImageEffect:
		out = Effect{Kind: "image", Name: e.Picture.File, Faces: e.Picture.Faces}
	case event.SoundEffect:
		out = Effect{Kind: "sound", Name: e.Name}
	case event.VisualEffect:
		out = Effect{Kind: "visual", Name: e.Name}
	default:
		return
	}
	if s.cfg.OnEffect != nil {
		s.cfg.OnEffect(out)
	}
}

// Save is the state as JSON (Restore reads it). The events being run are not saved.
func (s *Session) Save() ([]byte, error) { return s.st.Save() }

// Clone copies the state and the random numbers between commands (for trying a branch).
// The commands being run are not copied.
func (s *Session) Clone() (*Session, error) {
	b, err := s.st.Save()
	if err != nil {
		return nil, err
	}
	c := &Session{g: s.g, db: gamedata.Fork(s.db), cfg: s.cfg, last: s.last}
	pcg := *s.pcg
	c.pcg = &pcg
	st, err := rpg.Parse(c.db, b, rand.New(c.pcg)) //nolint:gosec // Gameplay randomness, not security.
	if err != nil {
		return nil, err
	}
	st.Calm = s.st.Calm
	c.st = st
	c.start()
	dungeon.Remake(c.db, c.st)
	return c, nil
}

// act is one move of the act command.
type act struct{ actor, skill, item, target string }

// autoBattle fights a battle on AUTO (same as headless.AutoBattle). Moves decided by the act command are used in written
// order on the turns when their user can act. Each line is reported as a "step".
func (s *Session) autoBattle(b *battle.Battle, maxTurns int) {
	for _, u := range b.Party {
		u.Auto = true
	}
	sub := event.New(s.db, s.st, event.HostFunc(s.effect))
	sub.Log = s.runner.Log
	play := func(steps []battle.Step) {
		for _, st := range steps {
			if st.Text != "" && s.cfg.OnRequest != nil {
				s.cfg.OnRequest(Request{Kind: "step", Text: st.Text})
			}
			if st.Kind != battle.Event {
				continue
			}
			sub.Extra = b.Env(nil)
			sub.Start(st.Commands)
			for sub.Running() {
				switch sub.Next().(type) {
				case event.Choice:
					sub.Answer(0)
				case event.Battle:
					sub.Answer(event.BattleWon) // there is no battle inside a battle
				}
			}
		}
	}
	play(b.Begin())
	for b.Result == battle.Ongoing && b.Turn <= maxTurns {
		b.StartTurn(s.forced(b))
		for {
			steps, done := b.Next()
			play(steps)
			if done {
				break
			}
		}
	}
	if len(s.acts) > 0 {
		s.runner.Log(fmt.Errorf("act: %d moves were not taken (%s could not act)", len(s.acts), s.acts[0].actor))
		s.acts = nil
	}
}

// forced is the act move to use this turn (the first unused move for each user).
func (s *Session) forced(b *battle.Battle) []battle.Action {
	var out []battle.Action
	used := map[string]bool{}
	rest := s.acts[:0]
	for _, a := range s.acts {
		u := unitOf(b.Party, a.actor)
		if used[a.actor] || u == nil || !u.Alive() {
			rest = append(rest, a)
			continue
		}
		used[a.actor] = true
		out = append(out, battle.Action{User: u, Skill: a.skill, Item: a.item, Target: targetOf(b, a.target)})
	}
	s.acts = rest
	return out
}

func unitOf(units []*battle.Unit, actor string) *battle.Unit {
	for _, u := range units {
		if u.Actor != nil && u.Actor.ID == actor {
			return u
		}
	}
	return nil
}

// targetOf is the target (a party member's ID, or the first living enemy of that kind). nil if none (chosen by the skill's rules).
func targetOf(b *battle.Battle, id string) *battle.Unit {
	if id == "" {
		return nil
	}
	if u := unitOf(b.Party, id); u != nil {
		return u
	}
	for _, u := range b.Foes {
		if u.EID == id && u.Alive() {
			return u
		}
	}
	return nil
}
