package gozen

import (
	"context"
	"errors"
	"fmt"
	"math/rand/v2"
	"strconv"
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
	seed    uint64
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
	at      cursor
	held    *Request // the line a restored save goes on at, shown by the next Next
}

// cursor is where the running event is, for a save made in its middle (ADR 0025): the state
// and random bytes when it began (or before the arrival that made it), its name, the lines
// shown and the answers given, and whether it passed a boundary (a battle, a shop, a save,
// a screen, a move, the end), after which it cannot be begun again.
type cursor struct {
	snap, rnd   []byte
	name        string
	arrival     bool
	lines       int
	answers     []int
	noBack      bool
	arriving    bool
	before, rbf []byte
}

// New starts a session at the game's start (game.yaml start), running nothing yet;
// Begin runs the start event.
func New(g *Game, cfg Config) (*Session, error) {
	if g == nil {
		return nil, fmt.Errorf("%w: nil game", ErrInvalid)
	}
	s := newSession(g, cfg)
	s.st = rpg.New(s.db, rand.New(s.pcg)) //nolint:gosec // Gameplay randomness, not security.
	s.st.SetRecruitSeed(s.seed)
	s.start()
	s.fresh = true
	return s, nil
}

// Restore continues a saved game (Session.Save). A save of another game or of an
// unknown format is ErrIncompatibleSave. A save made in the middle of an event begins the
// event again from its start and reads on, with the answers given, to the line it was
// made at, which the next Next returns (ADR 0025); the requests read on to are not reported.
func Restore(g *Game, save []byte, cfg Config) (*Session, error) {
	if g == nil {
		return nil, fmt.Errorf("%w: nil game", ErrInvalid)
	}
	s := newSession(g, cfg)
	st, err := rpg.Parse(s.db, save, rand.New(s.pcg)) //nolint:gosec // Gameplay randomness, not security.
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrIncompatibleSave, err)
	}
	if r := st.Resume; r != nil && len(r.Rand) > 0 {
		if err := s.pcg.UnmarshalBinary(r.Rand); err != nil {
			return nil, fmt.Errorf("%w: %w", ErrIncompatibleSave, err)
		}
	}
	if st.Resume == nil && len(st.SavedRand) > 0 {
		if err := s.pcg.UnmarshalBinary(st.SavedRand); err != nil {
			return nil, fmt.Errorf("%w: %w", ErrIncompatibleSave, err)
		}
	}
	s.st = st
	s.db = st.DB
	s.start()
	dungeon.Remake(s.db, s.st)
	if r := st.Resume; r != nil {
		if err := s.resume(r.Cursor); err != nil {
			return nil, fmt.Errorf("%w: %w", ErrIncompatibleSave, err)
		}
	}
	return s, nil
}

// resume begins the event of a cursor again and reads on to its line.
func (s *Session) resume(c rpg.Cursor) error {
	if c.Arrival {
		s.arrived = true
	} else {
		cmds := s.db.NamedCommands(c.Event)
		if cmds == nil {
			return fmt.Errorf("no event %q to go on with", c.Event)
		}
		s.runner.Start(cmds)
	}
	onRequest, onEffect := s.cfg.OnRequest, s.cfg.OnEffect
	s.cfg.OnRequest, s.cfg.OnEffect = nil, nil
	defer func() { s.cfg.OnRequest, s.cfg.OnEffect = onRequest, onEffect }()
	for range maxSteps {
		q := s.Next()
		switch q.Kind {
		case "":
			return fmt.Errorf("the event ended before line %d", c.Line)
		case "battle", "shop", "save", "open", "clear", "game_over":
			return fmt.Errorf("line %d is past a %s", c.Line, q.Kind)
		case "say", "choice":
			if s.at.lines >= c.Line {
				if q.Kind == "say" {
					s.held = &q
				}
				return nil
			}
			if q.Kind == "choice" {
				k := len(s.at.answers)
				if k >= len(c.Answers) {
					return fmt.Errorf("no answer for the choice before line %d", c.Line)
				}
				s.Answer(c.Answers[k])
			}
		}
	}
	return fmt.Errorf("line %d was not reached", c.Line)
}

func newSession(g *Game, cfg Config) *Session {
	seed := cfg.Seed
	if seed == 0 {
		seed = rand.Uint64() //nolint:gosec // Gameplay randomness, not security.
	}
	if cfg.Policy == nil {
		cfg.Policy = DefaultPolicy{}
	}
	return &Session{g: g, db: gamedata.Fork(g.db), pcg: rand.NewPCG(seed, seed^0x9e3779b97f4a7c15), seed: seed, cfg: cfg}
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
	s.runner.OnStart = s.onStart
}

// onStart keeps the state when an event begins, for a save made in its middle.
func (s *Session) onStart(cmds []gamedata.Command) {
	name := s.db.CommandsName(cmds)
	if name == "" && len(cmds) == 1 && cmds[0].Call != "" && s.db.Events[cmds[0].Call] != nil {
		name = cmds[0].Call // - call: NAME from Do or Start
	}
	c := cursor{name: name, arrival: s.at.arriving && name == "", before: s.at.before, rbf: s.at.rbf}
	var err error
	if c.snap, err = s.st.Save(); err != nil {
		c.noBack = true
	}
	if c.rnd, err = s.pcg.MarshalBinary(); err != nil {
		c.noBack = true
	}
	s.at = c
}

// arrive starts the commands of arriving at the location, keeping the state before them.
func (s *Session) arrive() {
	before, err := s.st.Save()
	if err != nil {
		s.runner.Log(err)
	}
	rbf, err := s.pcg.MarshalBinary()
	if err != nil {
		s.runner.Log(err)
	}
	s.at.arriving, s.at.before, s.at.rbf = true, before, rbf
	s.runner.Start(world.Arrive(s.db, s.st, !s.quiet))
	s.at.arriving = false
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
		return cloneRequest(*s.pending)
	}
	if s.held != nil {
		q := *s.held
		s.held = nil
		return q
	}
	for range maxSteps {
		if s.runner.Running() {
			r := s.runner.Next()
			if r == nil {
				continue
			}
			q := s.request(r)
			switch q.Kind {
			case "say", "choice":
				s.at.lines++
			case "battle", "shop", "save", "open", "clear", "game_over":
				s.at.noBack = true
			}
			if s.cfg.OnRequest != nil {
				s.cfg.OnRequest(cloneRequest(q))
			}
			if q.Kind == "choice" || q.Kind == "battle" {
				s.pending = &q
			}
			return cloneRequest(q)
		}
		if s.last.Over || s.last.Cleared {
			return Request{}
		}
		if s.arrived {
			s.arrived = false
			s.arrive()
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
		s.at.answers = append(s.at.answers, n)
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
	case event.ClashScene:
		return Request{Kind: "clash", ID: r.Clash.Region, Text: r.Clash.Result}
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
		s.at.noBack = true
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
	case event.StandEffect:
		out = Effect{Kind: "stand", Name: e.Who}
	case event.VoiceEffect:
		out = Effect{Kind: "voice", Name: e.File}
	case event.CampaignEffect:
		out = Effect{Kind: e.Kind, Name: e.Order}
		if e.Kind == "end_turn" {
			out.Name = strconv.Itoa(e.Turn)
		}
	default:
		return
	}
	if s.cfg.OnEffect != nil {
		s.cfg.OnEffect(out)
	}
}

// Save is the state as JSON (Restore reads it). In the middle of an event that can be begun
// again (it passed no battle, shop, save, screen or move), it is the state when the event
// began with a cursor to the line shown last (save format version 2, ADR 0025); otherwise
// the events being run are not saved.
func (s *Session) Save() ([]byte, error) {
	c := s.at
	if !s.runner.Running() || c.noBack || c.lines == 0 || c.snap == nil || c.name == "" && !c.arrival {
		return s.saveBetween()
	}
	cur := rpg.Cursor{Event: c.name, Line: c.lines, Answers: c.answers}
	if c.arrival {
		cur.Arrival = true
		if c.before == nil {
			return s.saveBetween()
		}
		return rpg.WithCursor(c.before, cur, c.rbf)
	}
	return rpg.WithCursor(c.snap, cur, c.rnd)
}

// saveBetween is the state as it is with the random generator's bytes, so that Restore
// draws the numbers this session would have (ADR 0030).
func (s *Session) saveBetween() ([]byte, error) {
	b, err := s.st.Save()
	if err != nil {
		return nil, err
	}
	rnd, err := s.pcg.MarshalBinary()
	if err != nil {
		return nil, err
	}
	return rpg.WithRand(b, rnd)
}

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
