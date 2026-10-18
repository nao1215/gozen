// Package event runs the commands of events (see data.Command): it changes the game state
// itself and asks the screen for what needs one (a line to read, a choice, a battle, a
// shop). The screen calls Next, shows the Request it gets, and calls Answer (for a choice
// or a battle) before the next Next.
package event

import (
	"fmt"
	"math"
	"path"
	"slices"
	"strings"

	lang "github.com/nao1215/gozen/text"

	"github.com/nao1215/gozen/expr"
	data "github.com/nao1215/gozen/internal/layer2/gamedata"
	"github.com/nao1215/gozen/internal/layer3/dungeon"
	"github.com/nao1215/gozen/rpg"
)

// Host applies immediate effects for a game frontend or a simulation.
// Handle is called synchronously; the host owns any resources it uses.
type Host interface{ Handle(Effect) }

// HostFunc adapts a host callback without passing an upper-layer concrete type
// into the event runtime. A nil callback discards immediate effects.
type HostFunc func(Effect)

// Handle delivers the effect synchronously.
func (f HostFunc) Handle(e Effect) {
	if f != nil {
		f(e)
	}
}

// Effect is an immediate presentation or movement effect. It does not pause Next.
type Effect interface{ isEffect() }

// MoveEffect asks the host to update the location after dungeon placement.
type MoveEffect struct{ Location string }

// MusicEffect selects a music cue without requiring an audio backend.
type MusicEffect struct{ Cue data.MusicCue }

// MoodEffect changes the music mood.
type MoodEffect struct{ Name string }

// ImageEffect sets the current picture; an empty file clears it.
type ImageEffect struct{ Picture data.CG }

// SoundEffect requests a named sound.
type SoundEffect struct{ Name string }

// VisualEffect requests a named visual effect.
type VisualEffect struct{ Name string }

func (MoveEffect) isEffect()   {}
func (MusicEffect) isEffect()  {}
func (MoodEffect) isEffect()   {}
func (ImageEffect) isEffect()  {}
func (SoundEffect) isEffect()  {}
func (VisualEffect) isEffect() {}

// Request is something the screen must show before the event goes on.
type Request interface{ isRequest() }

// Say is a line to read.
type Say struct {
	Name  string // "" for narration
	Face  string // a picture under assets/ ("" for none)
	Text  string
	Right bool // the picture stands on the right
	Actor string
	// Instead は戦闘中に話し手が倒れているときの言い替え（data.Say の instead）。
	Instead []Say
}

// Choice is a question: the screen answers with the index of the option taken.
type Choice struct {
	IDs     []string // 表示中の選択肢と同じ順の識別子（未指定は空）
	Prompt  string
	Options []string
}

// Battle is a battle to fight: the screen answers with its battle.Result.
type Battle struct {
	Troop   string
	CanLose bool // losing does not end the game
	// Fighters fight instead of the party (none: the party).
	Fighters []string
	// NoRetry: losing does not offer to fight again from just before (retry: false).
	NoRetry bool
}

// Shop opens a shop.
type Shop struct{ ID string }

// Wait pauses.
type Wait struct{ Seconds float64 }

// Save opens the save screen.
type Save struct{}

// Open opens one of the screens of the game (formation, skills, fusion).
type Open struct{ Screen string }

// Notice は止めずに画面の上に短く出す知らせ（notice 命令）。画面は出して、すぐ次へ進む（答えは要らない）。
type Notice struct{ Text string }

// Tap waits for a tap with nothing to read (between the pictures of an image's files).
// Auto が 0 より 大きければ、その 秒数で 押さなくても 次へ 進む（image の auto。スライドショー）。
type Tap struct{ Auto float64 }

// Clear ends the game happily.
type Clear struct{}

// GameOver ends the game.
type GameOver struct{}

func (Say) isRequest()      {}
func (Choice) isRequest()   {}
func (Battle) isRequest()   {}
func (Shop) isRequest()     {}
func (Wait) isRequest()     {}
func (Save) isRequest()     {}
func (Open) isRequest()     {}
func (Clear) isRequest()    {}
func (Tap) isRequest()      {}
func (Notice) isRequest()   {}
func (GameOver) isRequest() {}

// The answers of a battle.
const (
	BattleWon  = 1
	BattleLost = 2
	BattleFled = 3
)

type frame struct {
	cmds         []data.Command
	pc           int
	prepared     bool
	location     string
	checkedAfter bool
}

// Runner runs events.
type Runner struct {
	db    *data.DB
	st    *rpg.State
	host  Host
	stack []frame
	notes []string
	// ups are the notes that tell of a level gained: a sound goes with them when they are shown
	ups map[string]bool
	// waiting for an answer
	options []data.ChoiceOption
	// pick answers a choice made by the engine (stone_choose, stone_wearer) instead of options' do
	pick   func(n int)
	asked  *data.Command // the choice whose answer is waited for
	battle *data.Command
	// Extra is looked up before the state's names (a battle's scope, for troop events).
	Extra        expr.Env
	gameOver     bool
	afterPending bool // 勝敗の分岐を確定した後、次の選択・戦闘より前に後処理を行う
	// pages are the pictures of an image's files still to show, a tap before each
	pages []data.CG
	// later は今のイベントが終わってから独立に走る命令（勝利後の after_win）。end では消えない
	later [][]data.Command
	// shown は 最後に 出した 絵（asides の 後に 戻す。立ち絵を 隠す 絵の 間は 挟まない。画面が AsideBlocked を 持てば そちらで 見る）
	shown data.CG
	// Log receives the errors of expressions (nil: they are ignored).
	Log func(error)
	// BlockAside lets a screen suppress inserted events during replay or overlays.
	// When non-nil it takes precedence over Host's optional AsideBlocker.
	// The runner calls it synchronously; it must not advance this runner.
	BlockAside func() bool
	// OnStart is told of an event started when nothing was running (before any of it runs):
	// a test player's note keeps the state there, to begin the event again from it.
	OnStart func(cmds []data.Command)
}

// New is a runner on the state.
func New(db *data.DB, st *rpg.State, host Host) *Runner {
	return &Runner{db: db, st: st, host: host}
}

// Start begins running cmds (after whatever is running).
func (r *Runner) Start(cmds []data.Command) {
	if len(cmds) > 0 {
		if r.OnStart != nil && !r.Running() {
			r.OnStart(cmds)
		}
		r.stack = append([]frame{{cmds: cmds}}, r.stack...)
	}
}

// Running reports whether there is anything left to run.
func (r *Runner) Running() bool {
	return r.gameOver || len(r.stack) > 0 || len(r.notes) > 0 || r.options != nil || r.battle != nil || len(r.later) > 0 || r.afterPending
}

// Asking is the command whose answer is waited for: the choice or the battle just asked
// (nil when none). Its slices are the data's own (shared by copies of the command), so
// they tell one place in the data from another that reads the same.
func (r *Runner) Asking() *data.Command {
	if r.options != nil {
		return r.asked
	}
	return r.battle
}

// Stop drops everything left.
func (r *Runner) Stop() {
	r.afterPending = false
	r.stack, r.notes, r.options, r.battle, r.later, r.pick, r.ups = nil, nil, nil, nil, nil, nil, nil
}

// Later は cmds を今のイベント（とその end）とは別に、今のイベントが終わってから走らせる。
func (r *Runner) Later(cmds []data.Command) {
	if len(cmds) > 0 {
		r.later = append(r.later, cmds)
	}
}

// Won は戦闘に勝ったときに呼ぶ。game.yaml の battle.after_win のイベントを、
// 今のイベントの後に独立した処理として予約する（画面と headless で共通）。
func (r *Runner) Won() {
	if ev := r.db.Events[r.db.Game.Battle.AfterWin]; ev != nil {
		r.Later(ev.Do)
	}
}

func (r *Runner) env() expr.Env {
	if r.Extra != nil {
		return expr.Chain{r.Extra, r.st.Env()}
	}
	return r.st.Env()
}

func (r *Runner) logf(err error) {
	if err != nil && r.Log != nil {
		r.Log(err)
	}
}

// Answer answers the last Choice (the option's index) or Battle (BattleWon ...).
func (r *Runner) Answer(n int) {
	switch {
	case r.options != nil:
		if n >= 0 && n < len(r.options) {
			if r.pick != nil {
				r.pick(n)
			} else {
				r.push(r.options[n].Do)
			}
		}
		r.options, r.pick = nil, nil
	case r.battle != nil:
		c := r.battle
		r.battle = nil
		r.afterPending = r.db.Events[r.db.Game.Battle.After] != nil
		switch n {
		case BattleWon:
			r.push(c.OnWin)
		case BattleLost:
			if c.OnLose == nil {
				r.Stop()
				r.gameOver = true
				return
			}
			r.push(c.OnLose)
		case BattleFled:
			r.push(c.OnRun)
		}
	}
}

func (r *Runner) push(cmds []data.Command) {
	if len(cmds) > 0 {
		r.stack = append(r.stack, frame{cmds: cmds})
	}
}

// Frames are the command lists being run, the outermost first (a call adds one), and the
// index of the next command of each (for a note of where the play is: which event).
func (r *Runner) Frames() (cmds [][]data.Command, pc []int) {
	for _, f := range r.stack {
		cmds = append(cmds, f.cmds)
		pc = append(pc, f.pc)
	}
	return cmds, pc
}

// Next runs commands until one needs the screen, and returns it (nil when the events are
// over).
func (r *Runner) Next() Request {
	if r.gameOver {
		r.gameOver = false
		r.Stop()
		return GameOver{}
	}
	if len(r.pages) > 0 {
		shown := r.pages[0]
		r.show(shown)
		r.pages = r.pages[1:]
		if len(r.pages) > 0 {
			return Tap{Auto: shown.Auto}
		}
	}
	for {
		if len(r.notes) > 0 {
			n := r.notes[0]
			r.notes = r.notes[1:]
			if r.ups[n] {
				delete(r.ups, n)
				r.emit(SoundEffect{"levelup"})
			}
			return Say{Text: n}
		}
		if len(r.stack) == 0 {
			if len(r.later) == 0 {
				if r.afterPending {
					r.afterPending = false
					r.push(r.db.Events[r.db.Game.Battle.After].Do)
					continue
				}
				return nil
			}
			r.stack = []frame{{cmds: r.later[0]}}
			r.later = r.later[1:]
			continue
		}
		if len(r.stack) > 64 {
			r.logf(fmt.Errorf("events nested too deep (a call loop?)"))
			r.Stop()
			return nil
		}
		top := &r.stack[len(r.stack)-1]
		if top.pc >= len(top.cmds) {
			r.stack = r.stack[:len(r.stack)-1]
			continue
		}
		c := &top.cmds[top.pc]
		if r.afterPending && (c.Battle != "" || len(c.Choice) > 0) {
			r.afterPending = false
			top.checkedAfter, top.location = true, r.st.Location
			r.push(r.db.Events[r.db.Game.Battle.After].Do)
			continue
		}
		if (top.prepared || top.checkedAfter) && top.location != r.st.Location {
			r.Stop()
			return nil
		}
		if c.Battle != "" && !top.prepared {
			if ev := r.db.Events[r.db.Game.Battle.Before]; ev != nil {
				top.prepared, top.location = true, r.st.Location
				r.push(ev.Do)
				continue
			}
		}
		top.prepared, top.checkedAfter = false, false
		top.pc++
		if req := r.run(c); req != nil {
			return req
		}
	}
}

func (r *Runner) run(c *data.Command) Request {
	verb, err := c.Verb()
	if err != nil {
		r.logf(err)
		return nil
	}
	env := r.env()
	switch verb {
	case "say":
		req := r.say(c.Say, env)
		if line, ok := req.(Say); ok {
			r.aside(line, env)
		}
		return req
	case "choice":
		var labels, ids []string
		r.options = nil
		for _, o := range c.Choice {
			ok, err := o.When.Bool(env)
			r.logf(err)
			if ok {
				r.options = append(r.options, o)
				labels = append(labels, data.Render(o.Label, env))
				ids = append(ids, o.ID)
			}
		}
		if len(labels) == 0 {
			r.options = nil
			return nil
		}
		r.asked = c
		return Choice{Prompt: data.Render(c.Prompt, env), Options: labels, IDs: ids}
	case "done":
		r.st.Done[c.Done] = true
	case "if":
		ok, err := c.If.Bool(env)
		r.logf(err)
		if ok {
			r.push(c.Then)
		} else {
			r.push(c.Else)
		}
	case "set":
		// 旧新版・回想とも辞書順の逐次代入。右辺は１回だけ評価し、先の代入を後の右辺へ見せる。
		keys := make([]string, 0, len(c.SetV))
		for k := range c.SetV {
			keys = append(keys, k)
		}
		slices.Sort(keys)
		for _, k := range keys {
			v, err := c.SetV[k].Eval(env)
			r.logf(err)
			if err == nil {
				r.logf(r.st.SetPath(k, v))
			}
		}
	case "give":
		if n := c.Give.N(); n != 0 {
			r.st.AddItem(c.Give.Item, n)
		}
	case "take":
		if n := c.Take.N(); n != 0 {
			r.st.Take(c.Take.Item, n)
		}
	case "gold":
		n, err := c.Gold.Number(env)
		r.logf(err)
		r.st.Gold = max(0, r.st.Gold+int(n))
	case "join":
		// game.yaml の join_exp: 加わる前のパーティで下限を見て、遅れて加わる人を追いつかせる
		floor := -1.0
		if r.db.Game.JoinExp.Set() && !slices.Contains(r.st.Party, c.Join) {
			n, err := r.db.Game.JoinExp.Number(env)
			r.logf(err)
			floor = n
		}
		if r.st.Join(c.Join) {
			r.notes = append(r.notes, r.db.Actors[c.Join].Name+lang.T("が なかまに くわわった！"))
			if a := r.st.Actors[c.Join]; floor > float64(a.Exp) {
				a.GainExp(int(floor) - a.Exp)
				a.Recover()
			}
		}
	case "leave":
		r.st.Leave(c.Leave)
	case "heal":
		for _, a := range r.st.Members() {
			if c.Heal == "all" || a.ID == c.Heal {
				a.Recover()
			}
		}
	case "hurt":
		for _, a := range r.st.Members() {
			if (c.Hurt.Who == "all" || a.ID == c.Hurt.Who) && a.HP > 0 {
				v, err := c.Hurt.Amount.Number(expr.Chain{expr.Map{"self": rpg.ActorView{A: a}}, env})
				r.logf(err)
				a.HP = max(1, a.HP-max(0, int(v)))
			}
		}
	case "affect":
		for _, a := range r.st.Members() {
			if c.Affect.Who != "all" && a.ID != c.Affect.Who {
				continue
			}
			if s := c.Affect.Add; s != "" && r.db.States[s] != nil && (!c.Affect.Resist || r.st.Rand.Float64() < a.StateResist(s)) {
				turns := c.Affect.Turns
				if turns <= 0 {
					turns = r.db.States[s].Turns
				}
				if turns <= 0 { // 戦闘中の付与と同じく、期限なしは StateForever で持つ
					turns = rpg.StateForever
				}
				a.States[s] = turns
			}
			for s := range a.States {
				if s == c.Affect.Remove || (c.Affect.Remove == "bad" && r.db.States[s] != nil && r.db.States[s].Bad) {
					delete(a.States, s)
				}
			}
		}
	case "battle":
		r.battle = c
		return Battle{Troop: c.Battle, CanLose: c.OnLose != nil, Fighters: c.Fighters, NoRetry: c.Retry != nil && !*c.Retry}
	case "move":
		dungeon.Place(r.db, r.st, c.Move, c.At, c.Facing)
		r.emit(MoveEffect{c.Move})
	case "music":
		r.emit(MusicEffect{*c.Music})
	case "mood":
		r.emit(MoodEffect{c.Mood})
	case "image":
		if len(c.Image.Files) == 0 {
			r.show(*c.Image)
			break
		}
		pages := c.Image.Pages(r.db.FS)
		if len(pages) == 0 {
			break
		}
		n := c.Image.Page
		if c.Image.Pick.Set() { // 式で 選ぶ（値を 枚数で 割った 余り + 1 枚目）
			v, err := c.Image.Pick.Number(r.env())
			if err != nil {
				v = 0
			}
			k := int(math.Floor(v)) % len(pages)
			if k < 0 {
				k += len(pages)
			}
			n = k + 1
		}
		if n > 0 { // 段階を 一枚だけ 出す（文章の 間に 段階ごとに 書く）
			cg := *c.Image
			cg.File, cg.Files, cg.Page, cg.Pick = pages[min(n, len(pages))-1], nil, 0, data.Expr{}
			r.show(cg)
			break
		}
		r.pages = r.pages[:0]
		for _, f := range pages {
			cg := *c.Image
			cg.File, cg.Files = f, nil
			r.pages = append(r.pages, cg)
		}
		r.show(r.pages[0])
		r.pages = r.pages[1:]
		if len(r.pages) > 0 {
			return Tap{Auto: c.Image.Auto}
		}
	case "shop":
		return Shop{ID: c.Shop}
	case "call":
		r.push(r.db.Events[c.Call].Do)
	case "wait":
		return Wait{Seconds: c.Wait}
	case "notice":
		return Notice{Text: data.Render(c.Notice, env)}
	case "sfx":
		r.emit(SoundEffect{c.SFX})
	case "effect":
		r.emit(VisualEffect{c.Effect})
	case "exp":
		n, err := c.Exp.Number(env)
		r.logf(err)
		// 同じ レベルに 上がった 人は 1 行に まとめる（行為の 後に 5 行 続けて 送らされた）
		var levels []int
		names := map[int][]string{}
		for _, a := range r.st.Members() {
			if !a.Alive() {
				continue
			}
			for _, up := range a.GainExp(int(n)) {
				if names[up.Level] == nil {
					levels = append(levels, up.Level)
				}
				names[up.Level] = append(names[up.Level], a.Name())
			}
		}
		alive := 0
		for _, a := range r.st.Members() {
			if a.Alive() {
				alive++
			}
		}
		for _, lv := range levels {
			line := rpg.LevelUpLine(names[lv], alive, r.db.Game.Terms.Level, lv)
			r.notes = append(r.notes, line)
			if r.ups == nil {
				r.ups = map[string]bool{}
			}
			r.ups[line] = true
		}
	case "learn":
		if a := r.st.Actors[c.Learn.Actor]; a != nil && !slices.Contains(a.Skills(), c.Learn.Skill) {
			a.Learnt = append(a.Learnt, c.Learn.Skill)
			r.notes = append(r.notes, fmt.Sprintf(lang.T("%sは %sを おぼえた！"), a.Name(), r.db.Skills[c.Learn.Skill].Name))
		}
	case "reset_tree":
		for id, a := range r.st.Actors {
			if c.ResetTree == "all" || c.ResetTree == id {
				a.ResetTree()
			}
		}
	case "end":
		r.stack = nil
	case "clear":
		r.stack = nil
		return Clear{}
	case "save":
		return Save{}
	case "open":
		return Open{Screen: c.Open}
	case "dive": // a made dungeon: a new dive from outside, the next floor from inside
		floor := 1.0
		if c.Floor.Set() {
			n, err := c.Floor.Number(env)
			r.logf(err)
			floor = n
		}
		dungeon.DiveAt(r.db, r.st, c.Dive, int(floor), c.At, c.Facing)
		r.emit(MoveEffect{c.Dive})
	case "open_wall": // a wall by the party becomes the tile (stairs that appear)
		dungeon.OpenWall(r.db, r.st, c.OpenWall)
	case "stone_suggest", "stone_apply", "stone_choose", "stone_wearer", "stone_decline", "equip_stones":
		return r.stones(verb, c, env)
	case "sp_suggest", "sp_apply", "sp_decline":
		r.sp(verb, c)
	}
	return nil
}

// say builds the line: the speaker's name and face come from the actor when who is one.
func (r *Runner) say(s *data.Say, env expr.Env) Request {
	out, ok := r.line(s, env)
	if !ok {
		return nil
	}
	for i := range s.Instead {
		if alt, ok := r.line(&s.Instead[i], env); ok {
			out.Instead = append(out.Instead, alt)
		}
	}
	return out
}

// line builds one line (false when it has nothing to show).
func (r *Runner) line(s *data.Say, env expr.Env) (Say, bool) {
	out := Say{Text: data.Render(s.Text, env), Name: s.Who}
	if a, ok := r.db.Actors[s.Who]; ok {
		out.Name = a.Name
		out.Actor = s.Who
		face := s.Face
		if face == "" {
			face = a.Face
		}
		if a.Faces != "" && !a.NoStanding {
			outfit := ""
			if loc := r.db.Locations[r.st.Location]; loc != nil {
				outfit = loc.Outfit
			}
			out.Face = r.db.FacePicture(a, face, outfit)
		}
		out.Right = s.Side == "right"
	} else {
		if s.Face != "" {
			out.Face = s.Face
		}
		out.Right = s.Side != "left"
	}
	out.Name = data.Render(out.Name, env)
	if strings.TrimSpace(out.Text) == "" && out.Face == "" {
		return Say{}, false
	}
	return out, true
}

// AsideBlocker は 話し手の 後に 挟む イベント（game.yaml の asides）を 今は 挟まない かを 答える 画面
// （立ち絵を 隠す 一枚絵が 出て いる、回想の 再生 など）。持たない Host では、ランナーが 出した 一枚絵で 見る。
type AsideBlocker interface {
	AsideBlocked() bool
}

// aside は 話し手の 台詞の 後に、game.yaml の asides の 当たる 物を 一つ 積む（台詞の 次に 走り、終われば 元に 戻る）。
func (r *Runner) aside(s Say, env expr.Env) {
	if len(r.db.Game.Asides) == 0 || r.Extra != nil { // 戦闘の 中では 挟まない
		return
	}
	if r.BlockAside != nil {
		if r.BlockAside() {
			return
		}
	} else if b, ok := r.host.(AsideBlocker); ok {
		if b.AsideBlocked() {
			return
		}
	} else if r.shown.File != "" && !r.shown.Faces && !r.db.StandIn(r.shown.File) {
		return
	}
	for _, a := range r.db.Game.Asides {
		if !asideSpeaker(a, s) || r.asideInside(a) {
			continue
		}
		ok, err := a.When.Bool(env)
		r.logf(err)
		if !ok {
			continue
		}
		if ev := r.db.Events[a.Call]; ev != nil && len(ev.Do) > 0 {
			// 挟んだ イベントの 後は、挟む 前の 絵（場所の 背景・faces の 背景、無ければ 消す）に 戻す
			back := r.shown
			r.push([]data.Command{{Image: &back}})
			r.push(ev.Do)
			return
		}
	}
}

func asideSpeaker(a data.Aside, s Say) bool {
	switch {
	case a.Actor != "":
		return s.Actor == a.Actor
	case a.Face != "":
		ok, err := path.Match(a.Face, s.Face)
		return err == nil && ok && s.Face != ""
	default:
		return s.Name == a.Name
	}
}

// asideInside は 今 走って いる イベントの どれかが、挟む イベント 自身か skip の パターンに 当たるか。
func (r *Runner) asideInside(a data.Aside) bool {
	for _, f := range r.stack {
		if len(f.cmds) == 0 {
			continue
		}
		for name, ev := range r.db.Events {
			if len(ev.Do) == 0 || &ev.Do[0] != &f.cmds[0] {
				continue
			}
			if name == a.Call {
				return true
			}
			for _, p := range a.Skip {
				if ok, err := path.Match(p, name); err == nil && ok {
					return true
				}
			}
		}
	}
	return false
}

// show は 絵を 出す（最後に 出した 絵を 覚える）。
func (r *Runner) show(c data.CG) {
	// 仮背景の立ち絵表示を、画面と画面なしの実行へ同じ値で通知する。
	if r.db.StandIn(c.File) {
		c.Faces = true
	}
	r.shown = c
	r.emit(ImageEffect{c})
}

func (r *Runner) emit(effect Effect) {
	if r.host != nil {
		r.host.Handle(effect)
	}
}
