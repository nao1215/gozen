// Package headless plays a game without a screen: it reads every line, answers the
// choices from a list (the first option by default), fights every battle on AUTO, and
// walks the party where it is told. It is for testing that a game can be played through
// (a game's story test drives it) and for balancing (gozen sim battle and sim dive).
package headless

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/nao1215/gozen/internal/battle"
	"github.com/nao1215/gozen/internal/event"
	data "github.com/nao1215/gozen/internal/layer2/gamedata"
	"github.com/nao1215/gozen/internal/layer3/dungeon"
	"github.com/nao1215/gozen/internal/layer4/itemuse"
	"github.com/nao1215/gozen/internal/rpg"
	"github.com/nao1215/gozen/internal/world"
)

// Driver plays a game.
type Driver struct {
	DB *data.DB
	St *rpg.State

	// Answers are the options to take at the next choices, in order (then 0).
	Answers []int
	// Transcript is every line read and every battle fought.
	Transcript []string
	// MaxTurns ends a battle as a loss when it lasts longer.
	MaxTurns int

	// OnDecision receives the record of reasons the battle AI actually evaluated. Nothing is recorded if nil.
	OnDecision func(battle.Decision)

	Battles, Wins, Losses int
	Cleared, Over         bool

	// If Record is set, dialogue, choices, notices, CGs, music, movement and battles are kept in Steps in the order
	// they happened (for tools that write out text in play order; same shape as Explore's Path.Steps).
	Record bool
	Steps  []Step
	// OnStep observes the state at recording time. It must not change the state.
	OnStep func(Step)
	// If Choose is not nil, choices are advanced by Choose's answers instead of Answers.
	Choose func(st *rpg.State, q event.Choice) int
	// If Result is not nil and returns ok, the battle is not fought and takes that result (event.BattleWon, etc.).
	Result func(st *rpg.State, troop string) (res int, ok bool)
	// If OnShop is not nil, it is called for an event's shop (the store screen), to make purchases. If nil, shops do nothing and close.
	OnShop func(st *rpg.State, shop string)

	ctx     context.Context
	runner  *event.Runner
	arrived bool
	quiet   bool // the next arrival has no random battle (the start of a game)
	errs    []error
}

// New constructs a driver borrowing db and st. Call Start for a new game, or
// Restore for a loaded save; construction does not run events.
func New(db *data.DB, st *rpg.State) *Driver {
	d := &Driver{DB: db, St: st, MaxTurns: 80}
	d.runner = event.New(db, st, event.HostFunc(d.handle))
	d.runner.Log = func(err error) { d.errs = append(d.errs, err) }
	// headless is a tool for checking and simulation, so set may write stats and levels (ADR 0021, author decision 5).
	d.runner.Dev, d.runner.UseItem = true, itemuse.For(db, st)
	return d
}

// Start runs the game's start event and what happens on arriving at the start location.
func (d *Driver) Start() error {
	if ev := d.DB.Game.Start.Event; ev != "" {
		d.runner.Start(d.DB.Events[ev].Do)
	}
	d.arrived, d.quiet = true, true
	dungeon.Ensure(d.DB, d.St)
	return d.settle()
}

// ---- event.Host

// move goes to a location.
func (d *Driver) move(loc string) {
	d.St.Location = loc
	d.St.Steps++
	// Place is called with the pre-move location, so the entrance and surroundings are recorded after arrival.
	dungeon.Reveal(d.DB, d.St)
	d.arrived = true
	d.record(Step{Kind: "move", Loc: loc})
}

// music is ignored (but recorded).
func (d *Driver) music(c data.MusicCue) { d.record(Step{Kind: "music", Song: c.Song}) }

// mood is ignored.
func (d *Driver) mood(string) {}

// image does not draw; it records the portrait display state the same as on screen.
func (d *Driver) image(cg data.CG) { d.record(imageStep(d.DB, cg)) }

// With a temporary background the screen keeps the portrait, so the actual display state is used for the verification record too.
func imageStep(db *data.DB, cg data.CG) Step {
	return Step{Kind: "image", File: cg.File, Faces: cg.Faces || db.StandIn(cg.File)}
}

func (d *Driver) record(s Step) {
	if d.OnStep != nil {
		d.OnStep(s)
	}
	if d.Record {
		d.Steps = append(d.Steps, s)
	}
}

// SFX is ignored.
func (d *Driver) sFX(string) {}

// effect is ignored.
func (d *Driver) effect(string) {}

// settle runs the events until nothing is left to run: the requests are answered, the
// arrivals and the auto events handled.
func (d *Driver) settle() error {
	for range 10000 {
		if d.ctx != nil {
			if err := d.ctx.Err(); err != nil {
				return err
			}
		}
		if d.Over || d.Cleared {
			return errors.Join(d.errs...)
		}
		if d.runner.Running() {
			d.answer(d.runner.Next())
			continue
		}
		if d.arrived {
			d.arrived = false
			d.runner.Start(world.Arrive(d.DB, d.St, !d.quiet))
			d.quiet = false
			continue
		}
		if cmds := world.Auto(d.DB, d.St); cmds != nil {
			d.runner.Start(cmds)
			continue
		}
		if cmds := dungeon.Hunters(d.DB, d.St); cmds != nil { // tough enemies that appear or vanish depending on conditions
			d.runner.Start(cmds)
			continue
		}
		return errors.Join(d.errs...)
	}
	return errors.New("the events do not end (an auto event that keeps holding?)")
}

func (d *Driver) answer(r event.Request) {
	switch r := r.(type) {
	case event.Say:
		if r.Name != "" {
			d.Transcript = append(d.Transcript, r.Name+"「"+r.Text+"」")
		} else {
			d.Transcript = append(d.Transcript, r.Text)
		}
		d.record(Step{Kind: "say", Who: r.Name, Actor: r.Actor, Face: r.Face, Text: r.Text})
	case event.Notice:
		d.record(Step{Kind: "notice", Text: r.Text})
	case event.Choice:
		n := 0
		if d.Choose != nil {
			n = d.Choose(d.St, r)
		} else if len(d.Answers) > 0 {
			n, d.Answers = d.Answers[0], d.Answers[1:]
		}
		n = max(0, min(n, len(r.Options)-1))
		d.Transcript = append(d.Transcript, "> "+r.Options[n])
		d.record(Step{Kind: "choice", Text: r.Options[n]})
		d.runner.Answer(n)
	case event.Battle:
		d.St.Fighters = r.Fighters
		res, ok := 0, false
		if d.Result != nil {
			res, ok = d.Result(d.St, r.Troop)
		}
		if !ok {
			res = d.fight(r.Troop)
		} else if res == event.BattleLost && r.CanLose {
			for _, m := range d.St.Members() {
				m.HP = max(1, m.HP)
			}
		}
		d.St.Fighters = nil
		d.record(Step{Kind: "battle", Text: r.Troop + ":" + resultName(res)})
		d.runner.Answer(res)
		if res == event.BattleWon {
			d.runner.Won() // as on screen, battle.after_win runs afterwards
		}
	case event.Shop:
		if d.OnShop != nil {
			d.OnShop(d.St, r.ID)
		}
	case event.Clear:
		d.Cleared = true
	case event.GameOver:
		d.Over = true
	}
}

// fight fights a battle with the whole party on AUTO and reports its result as an
// event answer (event.BattleWon ...).
func (d *Driver) fight(troop string) int {
	d.Battles++
	b := battle.New(d.DB, d.St, troop)
	b.OnDecision = d.OnDecision
	AutoBattle(d.DB, d.St, b, event.HostFunc(d.handle), d.MaxTurns, nil)
	if b.Result == battle.Ongoing {
		b.Result = battle.Lost
	}
	d.Transcript = append(d.Transcript, b.End()...)
	d.Transcript = append(d.Transcript, fmt.Sprintf("[battle %s: %s in %d turns]", troop, b.Result, b.Turn))
	switch b.Result {
	case battle.Ongoing, battle.Lost: // A turn limit is treated as a loss.
	case battle.Won:
		d.Wins++
		return event.BattleWon
	case battle.Fled:
		return event.BattleFled
	}
	d.Losses++
	b.Rise() // as on screen, only the party members who fought get up with 1 HP
	return event.BattleLost
}

// AutoBattle fights b on AUTO until it is decided (or maxTurns), and also runs the commands of in-battle events
// (troops' events) without a screen (the first choice is picked, and a battle inside a battle counts as a win).
// Fight, gozen sim battle and balance (gozen sim dive) use the same path. If host is nil, location moves, sounds and pictures are ignored.
// The record of decision reasons is passed to b.OnDecision.
// If seen is not nil, each emitted Step is passed to it in order. The result is left in b.Result (a draw if Ongoing).
func AutoBattle(db *data.DB, st *rpg.State, b *battle.Battle, host event.Host, maxTurns int, seen func([]battle.Step)) {
	for _, u := range b.Party {
		u.Auto = true
	}
	if host == nil {
		host = quietHost{}
	}
	sub := event.New(db, st, host)
	play := func(steps []battle.Step) {
		if seen != nil {
			seen(steps)
		}
		for _, s := range steps {
			if s.Kind == battle.Event {
				sub.Extra = b.Env(nil)
				sub.Start(s.Commands)
				for sub.Running() {
					switch sub.Next().(type) {
					case event.Choice:
						sub.Answer(0)
					case event.Battle:
						sub.Answer(event.BattleWon) // no battle inside a battle
					}
				}
			}
		}
	}
	play(b.Begin())
	for b.Result == battle.Ongoing && b.Turn <= maxTurns {
		b.StartTurn(nil)
		for {
			steps, done := b.Next()
			play(steps)
			if done {
				break
			}
		}
	}
}

// quietHost is an event.Host that does nothing (for using AutoBattle without a screen or driver).
type quietHost struct{}

func (quietHost) Handle(event.Effect) {}

// Do takes the entry of the field's menu whose label contains label, and settles.
func (d *Driver) Do(label string) error {
	for _, e := range world.Entries(d.DB, d.St) {
		if strings.Contains(e.Label, label) {
			d.runner.Start(e.Commands(d.DB, d.St))
			return d.settle()
		}
	}
	var labels []string
	for _, e := range world.Entries(d.DB, d.St) {
		labels = append(labels, e.Label)
	}
	return fmt.Errorf("at %s there is no %q (there is %s)", d.St.Location, label, strings.Join(labels, ", "))
}

// Has reports whether the field's menu has an entry whose label contains label.
func (d *Driver) Has(label string) bool {
	for _, e := range world.Entries(d.DB, d.St) {
		if strings.Contains(e.Label, label) {
			return true
		}
	}
	return false
}

// GoTo walks to a location by the shortest way through the exits open now (fighting
// whatever comes on the way).
func (d *Driver) GoTo(loc string) error {
	for range 50 {
		if d.Over {
			return errors.New("game over on the way")
		}
		if d.St.Location == loc {
			return nil
		}
		next, ok := d.route(loc)
		if !ok {
			return fmt.Errorf("no way from %s to %s", d.St.Location, loc)
		}
		d.runner.Start(next.Commands(d.DB, d.St))
		if err := d.settle(); err != nil {
			return err
		}
	}
	return fmt.Errorf("could not reach %s", loc)
}

// route is the first entry (an exit, or a point of a map) on the shortest way to loc. From
// the party's location it takes the entries open now; beyond, the exits whose when holds
// and the open points of maps (the roads between them are not followed: a map is one hop).
func (d *Driver) route(loc string) (world.Entry, bool) {
	type node struct {
		at    string
		first world.Entry
	}
	seen := map[string]bool{d.St.Location: true}
	var queue []node
	for _, e := range world.Entries(d.DB, d.St) {
		if e.Kind != world.Exit || e.Exit == nil || e.Exit.To == "" || seen[e.Exit.To] {
			continue
		}
		if e.Exit.To == loc {
			return e, true
		}
		seen[e.Exit.To] = true
		queue = append(queue, node{at: e.Exit.To, first: e})
	}
	env := d.St.Env()
	open := func(w data.Expr) bool {
		ok, err := w.Bool(env)
		return err == nil && ok
	}
	for len(queue) > 0 {
		n := queue[0]
		queue = queue[1:]
		l := d.DB.Locations[n.at]
		var tos []string
		for i := range l.Exits {
			if e := &l.Exits[i]; e.To != "" && open(e.When) {
				tos = append(tos, e.To)
			}
		}
		if l.Map != nil {
			for _, id := range l.Map.PointIDs() {
				if p := l.Map.Points[id]; p.To != "" && open(p.When) {
					tos = append(tos, p.To)
				}
			}
		}
		for _, to := range tos {
			if seen[to] {
				continue
			}
			if to == loc {
				return n.first, true
			}
			seen[to] = true
			queue = append(queue, node{at: to, first: n.first})
		}
	}
	return world.Entry{}, false
}

// Fights searches the location for battles n times.
func (d *Driver) Fights(n int) error {
	for range n {
		if d.Over {
			return errors.New("game over")
		}
		t := world.PickTroop(d.DB, d.St)
		if t == "" {
			return fmt.Errorf("no battles at %s", d.St.Location)
		}
		d.runner.Start([]data.Command{{Battle: t}})
		if err := d.settle(); err != nil {
			return err
		}
	}
	return nil
}

// EquipBest gives every member the best equipment in the bag (by the sum of its stats).
func (d *Driver) EquipBest() {
	score := func(id string) int {
		if id == "" {
			return 0
		}
		n := 0
		for _, v := range d.DB.Items[id].Stats {
			n += v
		}
		return n
	}
	for _, a := range d.St.Members() {
		for _, s := range d.DB.Game.Slots {
			best := a.Equip[s.ID]
			for _, id := range d.St.Bag() {
				it := d.DB.Items[id]
				if it.Fits(s.ID) && a.CanEquip(id) && score(id) > score(best) {
					best = id
				}
			}
			if best != a.Equip[s.ID] {
				if err := d.St.Equip(a, s.ID, best); err != nil {
					d.errs = append(d.errs, err)
				}
			}
		}
	}
}

// Shop buys from the shop what makes a member's equipment better, best first, while the
// money lasts, then up to n of each consumable in buy.
func (d *Driver) Shop(id string, buy map[string]int) {
	sh := d.DB.Shops[id]
	goods := slices.Clone(sh.Goods)
	slices.SortFunc(goods, func(a, b string) int { return d.DB.Items[b].Price - d.DB.Items[a].Price })
	for _, a := range d.St.Members() {
		for _, it := range goods {
			item := d.DB.Items[it]
			if !item.Equipment() || !a.CanEquip(it) || item.Price > d.St.Gold {
				continue
			}
			cur := d.DB.Items[a.Equip[item.Slot]]
			if cur != nil && cur.Price >= item.Price {
				continue
			}
			d.St.Buy(id, it, 1)
			if err := d.St.Equip(a, item.Slot, it); err != nil {
				d.errs = append(d.errs, err)
			}
		}
	}
	for it, n := range buy {
		for d.St.ItemCount(it, false) < n && slices.Contains(sh.Goods, it) && d.St.Buy(id, it, 1) {
		}
	}
}

// Restore continues from st (a game loaded from a save), after a game over.
func (d *Driver) Restore(st *rpg.State) {
	d.St = st
	d.Over, d.Cleared, d.arrived = false, false, false
	d.errs = nil
	d.quiet = false
	d.runner = event.New(d.DB, st, event.HostFunc(d.handle))
	d.runner.Log = func(err error) { d.errs = append(d.errs, err) }
	d.runner.Dev, d.runner.UseItem = true, itemuse.For(d.DB, st)
}

// Walk goes through the dungeon the party is in to the nearest cell with the character
// goal by the shortest way, fighting what it meets, and steps on it, or (for a solid one)
// looks at it. It stops early when the party leaves the dungeon.
func (d *Driver) Walk(goal rune) error {
	loc := d.St.Location
	for range 5000 {
		if d.Over {
			return errors.New("game over on the way")
		}
		if d.St.Location != loc {
			return nil
		}
		move, ok := dungeon.Next(d.DB, d.St, goal)
		if !ok {
			return fmt.Errorf("no way to %q in %s from %d,%d", goal, loc, d.St.Pos.X, d.St.Pos.Y)
		}
		switch move {
		case dungeon.MoveHere:
			return nil
		case dungeon.MoveLeft:
			dungeon.Turn(d.St, -1)
		case dungeon.MoveRight:
			dungeon.Turn(d.St, 1)
		case dungeon.MoveLook:
			d.runner.Start(dungeon.Look(d.DB, d.St))
			return d.settle()
		case dungeon.MoveForward:
			there := dungeon.Front(d.DB, d.St).Char == goal
			_, cmds := dungeon.Forward(d.DB, d.St)
			d.runner.Start(cmds)
			if err := d.settle(); err != nil {
				return err
			}
			if there {
				return nil
			}
		}
	}
	return fmt.Errorf("could not reach %q in %s", goal, loc)
}

// Run runs commands (an event's) to their end, as Settle does, and reports the events'
// errors (a test that every scene plays through).
func (d *Driver) Run(ctx context.Context, cmds []data.Command) error {
	if ctx == nil {
		return errors.New("run: nil context")
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	d.ctx = ctx
	defer func() { d.ctx = nil }()
	d.errs = nil
	d.runner.Start(cmds)
	return d.settle()
}

func (d *Driver) handle(e event.Effect) {
	switch e := e.(type) {
	case event.MoveEffect:
		d.move(e.Location)
	case event.MusicEffect:
		d.music(e.Cue)
	case event.MoodEffect:
		d.mood(e.Name)
	case event.ImageEffect:
		d.image(e.Picture)
	case event.SoundEffect:
		d.sFX(e.Name)
	case event.VisualEffect:
		d.effect(e.Name)
	}
}
