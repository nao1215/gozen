// Package headless plays a game without a screen: it reads every line, answers the
// choices from a list (the first option by default), fights every battle on AUTO, and
// walks the party where it is told. It is for testing that a game can be played through
// (a game's story test drives it) and for balancing (gozen sim and curve).
package headless

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/nao1215/gozen/battle"
	"github.com/nao1215/gozen/event"
	data "github.com/nao1215/gozen/internal/layer2/gamedata"
	"github.com/nao1215/gozen/internal/layer3/dungeon"
	"github.com/nao1215/gozen/rpg"
	"github.com/nao1215/gozen/world"
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

	// OnDecision は戦闘 AI が実評価した理由の控えを受け取る。nil なら記録しない。
	OnDecision func(battle.Decision)

	Battles, Wins, Losses int
	Cleared, Over         bool

	// Record なら、台詞・選択・知らせ・一枚絵・曲・移動・戦闘を起きた順に Steps に残す
	// （遊んだ順に読む文を書き出す道具用。Explore の Path.Steps と同じ形）。
	Record bool
	Steps  []Step
	// OnStep は記録時点の状態を観測する。状態を変更しないこと。
	OnStep func(Step)
	// Choose が nil でなければ、選択肢を Answers でなく Choose の答えで進める。
	Choose func(st *rpg.State, q event.Choice) int
	// Result が nil でなく ok を返せば、戦闘を戦わずにその結果（event.BattleWon など）にする。
	Result func(st *rpg.State, troop string) (res int, ok bool)
	// OnShop が nil でなければ、イベントの shop（お店の画面）で呼ぶ（買い物をさせる）。nil なら店は何もせず閉じる。
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
	return d
}

// Start runs the game's start event and what happens on arriving at the start location.
func (d *Driver) Start() error {
	if ev := d.DB.Game.Start.Event; ev != "" {
		d.runner.Start(d.DB.Events[ev].Do)
	}
	d.arrived, d.quiet = true, true
	dungeon.Ensure(d.DB, d.St)
	return d.Settle()
}

// ---- event.Host

// Move goes to a location.
func (d *Driver) move(loc string) {
	d.St.Location = loc
	d.St.Steps++
	// Place は移動前の場所で呼ばれるため、到着後に入口と周囲を記録する。
	dungeon.Reveal(d.DB, d.St)
	d.arrived = true
	d.record(Step{Kind: "move", Loc: loc})
}

// Music is ignored (but recorded).
func (d *Driver) music(c data.MusicCue) { d.record(Step{Kind: "music", Song: c.Song}) }

// Mood is ignored.
func (d *Driver) mood(string) {}

// Image は描画せず、画面と同じ立ち絵の表示状態を記録する。
func (d *Driver) image(cg data.CG) { d.record(imageStep(d.DB, cg)) }

// 仮背景では画面側が立ち絵を残すため、検証用の記録にも実際の表示状態を使う。
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

// Effect is ignored.
func (d *Driver) effect(string) {}

// Settle runs the events until nothing is left to run: the requests are answered, the
// arrivals and the auto events handled.
func (d *Driver) Settle() error {
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
		if cmds := dungeon.Hunters(d.DB, d.St); cmds != nil { // 条件で 出る・消える 強敵
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
			res = d.Fight(r.Troop)
		} else if res == event.BattleLost && r.CanLose {
			for _, m := range d.St.Members() {
				m.HP = max(1, m.HP)
			}
		}
		d.St.Fighters = nil
		d.record(Step{Kind: "battle", Text: r.Troop + ":" + resultName(res)})
		d.runner.Answer(res)
		if res == event.BattleWon {
			d.runner.Won() // 画面と同じく battle.after_win を後に走らせる
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

// Fight fights a battle with the whole party on AUTO and reports its result as an
// event answer (event.BattleWon ...).
func (d *Driver) Fight(troop string) int {
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
	b.Rise() // 画面と同じく、戦った仲間だけ HP 1 で起きる
	return event.BattleLost
}

// AutoBattle は b を AUTO で決着（または maxTurns）まで戦い、戦闘中イベント（troops の events）の
// 命令も画面なしで実行する（選択は最初の選択肢、戦闘の中の戦闘は勝ちとする）。Fight・gozen sim・
// balance（gozen curve）が同じ経路を使う。host が nil なら場所の移動・音・絵は無視する。
// 判断理由の記録は b.OnDecision に渡す。
// seen が nil でなければ、出た Step を順に渡す。結果は b.Result に残る（Ongoing なら引き分け）。
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

// quietHost は何もしない event.Host（AutoBattle を画面もドライバも無しで使うとき）。
type quietHost struct{}

func (quietHost) Handle(event.Effect) {}

// Do takes the entry of the field's menu whose label contains label, and settles.
func (d *Driver) Do(label string) error {
	for _, e := range world.Entries(d.DB, d.St) {
		if strings.Contains(e.Label, label) {
			d.runner.Start(e.Commands(d.DB, d.St))
			return d.Settle()
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
		next := d.route(loc)
		if next == nil {
			return fmt.Errorf("no way from %s to %s", d.St.Location, loc)
		}
		d.runner.Start(world.Entry{Kind: world.Exit, Exit: next}.Commands(d.DB, d.St))
		if err := d.Settle(); err != nil {
			return err
		}
	}
	return fmt.Errorf("could not reach %s", loc)
}

// route is the first exit on the shortest way to loc.
func (d *Driver) route(loc string) *data.Exit {
	type node struct {
		at    string
		first *data.Exit
	}
	seen := map[string]bool{d.St.Location: true}
	queue := []node{{at: d.St.Location}}
	env := d.St.Env()
	for len(queue) > 0 {
		n := queue[0]
		queue = queue[1:]
		l := d.DB.Locations[n.at]
		for i := range l.Exits {
			e := &l.Exits[i]
			if e.To == "" || seen[e.To] {
				continue
			}
			if ok, err := e.When.Bool(env); err != nil || !ok {
				continue
			}
			first := n.first
			if first == nil {
				first = e
			}
			if e.To == loc {
				return first
			}
			seen[e.To] = true
			queue = append(queue, node{at: e.To, first: first})
		}
	}
	return nil
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
		if err := d.Settle(); err != nil {
			return err
		}
	}
	return nil
}

// Level is the lowest level in the party.
func (d *Driver) Level() int {
	lv := 999
	for _, a := range d.St.Members() {
		lv = min(lv, a.Level)
	}
	return lv
}

// HPRate is the party's HP over its max HP.
func (d *Driver) HPRate() float64 {
	var hp, mhp int
	for _, a := range d.St.Members() {
		hp += a.HP
		mhp += a.Stat("mhp")
	}
	return float64(hp) / float64(max(1, mhp))
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
}

// Fallen is how many members have fallen.
func (d *Driver) Fallen() int {
	n := 0
	for _, a := range d.St.Members() {
		if !a.Alive() {
			n++
		}
	}
	return n
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
			return d.Settle()
		case dungeon.MoveForward:
			there := dungeon.Front(d.DB, d.St).Char == goal
			_, cmds := dungeon.Forward(d.DB, d.St)
			d.runner.Start(cmds)
			if err := d.Settle(); err != nil {
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
	return d.Settle()
}

func (h *Driver) handle(e event.Effect) {
	switch e := e.(type) {
	case event.MoveEffect:
		h.move(e.Location)
	case event.MusicEffect:
		h.music(e.Cue)
	case event.MoodEffect:
		h.mood(e.Name)
	case event.ImageEffect:
		h.image(e.Picture)
	case event.SoundEffect:
		h.sFX(e.Name)
	case event.VisualEffect:
		h.effect(e.Name)
	}
}
