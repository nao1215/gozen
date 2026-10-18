package headless

// Explore: イベントを画面なしで「全部の枝」流す道具（テスト用）。
// 状態の写しから命令を流し、選択肢の各項目・戦闘の勝ち負け（逃げ）を枝として試す。
// 同じ問い（データの同じ場所の、問いの文と項目の並び。戦闘なら敵グループと勝ち負けの後の命令）は
// 一度だけ枝分かれさせるので、道の数は「問いの項目の合計」ほどで済む（全部の組み合わせは試さない）。

import (
	"errors"
	"fmt"
	"math/rand/v2"
	"runtime/debug"
	"strings"

	"github.com/nao1215/gozen/event"
	data "github.com/nao1215/gozen/internal/layer2/gamedata"
	"github.com/nao1215/gozen/internal/layer3/dungeon"
	"github.com/nao1215/gozen/rpg"
	"github.com/nao1215/gozen/world"
)

// Config は Explore の決まり。
type Config struct {
	// MaxPaths は 1 つの根から試す道の上限（0 なら 300）。
	MaxPaths int
	// MaxSteps は 1 本の道で受け取る要求（台詞・選択など）と着いたときの処理の上限
	// （0 なら 20000）。超えたら終わらないイベントとして Errs に書く。
	MaxSteps int
	// Seed は道ごとの乱数の種（同じ道は同じ乱数で流れ直す）。
	Seed uint64
	// NoArrive なら、場所を移ったときの到着のイベント（enter）と auto を走らせない。
	NoArrive bool
	// Outcomes は戦闘で試す結果（event.BattleWon など）。nil なら勝ち・負け、逃げられる群れなら逃げも。
	Outcomes func(st *rpg.State, b event.Battle) []int
	// Fight が nil でなければ、戦闘を枝分かれさせずに Fight の結果で進める（実際に戦う）。
	Fight func(st *rpg.State, b event.Battle) int
	// Choose が nil でなければ、選択肢を枝分かれさせずに Choose の答えで進める（でたらめに遊ぶ）。
	Choose func(st *rpg.State, q event.Choice) int
	// Check は道の終わり（CheckEvery なら要求のたびにも）に呼ばれ、返した誤りを Errs に足す（不変条件の検査）。
	Check      func(st *rpg.State) error
	CheckEvery bool
	// Prepare は道を流す前に、写した状態に手を入れる（nil なら何もしない）。
	Prepare func(st *rpg.State)
}

// Path は試した 1 本の道。
type Path struct {
	Answers []int // 選んだ項目と戦闘の結果の並び
	Taken   []string
	St      *rpg.State // 道の終わりの状態
	Lines   []string   // 読んだ台詞（「名前「文」」）と選んだ項目（「> 項目」）
	Notices []string   // 画面の上の短い知らせ（notice。会話の窓を出さない）
	Images  []string   // 出した一枚絵（files は 1 枚ずつ。消したときは ""）
	Moves   []string
	Battles []string // 「敵グループ:won」など
	Errs    []error
	Over    bool // 全滅した（game_over.event があればそれも流した）
	// Left は最後の道にだけ付く、MaxPaths で打ち切ってまだ試していない枝の数（0 なら全部試した）
	Left    int
	Cleared bool
	// Steps は台詞・一枚絵・曲・移動・選択を起きた順に並べた物（台詞と絵の前後を見るテスト用）。
	Steps []Step
}

// Step は道の中の 1 つの出来事（Kind は say・image・music・move・choice・notice・sfx）。
type Step struct {
	Kind  string
	Who   string // say: 話し手の名前（地の文は空）
	Actor string // say: 話し手の actor ID（仲間でなければ空）
	Face  string // say: 顔の絵
	Text  string // say・notice・choice: 文（choice は選んだ項目）。sfx: 効果音の名前
	File  string // image: 出した絵（消したときは空）
	Faces bool   // image: 立ち絵を隠さない
	Song  string // music: 曲
	Loc   string // move: 行き先
}

// Log is the path's lines, choices and battles, for an error message.
func (p *Path) Log() string {
	return strings.Join(p.Taken, " / ")
}

// Explore は st の写しから cmds を流し、枝を全部試した道を返す（st は変えない）。
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
				var n int
				if opt.Choose != nil {
					n = max(0, min(len(q.Options)-1, opt.Choose(st, q)))
					p.Taken = append(p.Taken, "> "+q.Options[n])
				} else {
					key := "choice:" + place(r.Asking()) + q.Prompt + "\x00" + strings.Join(q.Options, "\x00")
					n = decide(key, len(q.Options), func(a int) string { return "> " + q.Options[a] })
				}
				p.Lines = append(p.Lines, "> "+q.Options[n])
				p.Steps = append(p.Steps, Step{Kind: "choice", Text: q.Options[n]})
				r.Answer(n)
			case event.Battle:
				if db.Troops[q.Troop] == nil {
					p.Errs = append(p.Errs, fmt.Errorf("battle with a troop that does not exist: %q", q.Troop))
				}
				var res int
				if opt.Fight != nil {
					st.Fighters = q.Fighters
					res = opt.Fight(st, q)
					st.Fighters = nil
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
						// 負けを受け止めるイベントは画面と同じく、戦った仲間だけ HP 1 で続く
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
				r.Answer(res)
				if res == event.BattleWon {
					r.Won()
				}
			case event.GameOver:
				p.Over = true
				p.Taken = append(p.Taken, "game over")
				ev := db.Events[db.Game.GameOver.Event]
				if ev == nil || overRun {
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
		fmt.Fprintf(&b, "%p", &c.Choice[0]) //nolint:errcheck // strings.Builder writes always succeed.
	}
	for _, s := range [][]data.Command{c.OnWin, c.OnLose, c.OnRun} {
		if len(s) > 0 {
			fmt.Fprintf(&b, "%p", &s[0]) //nolint:errcheck // strings.Builder writes always succeed.
		} else {
			b.WriteString("-") //nolint:errcheck // strings.Builder writes always succeed.
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
	return fmt.Sprint(res)
}

func (h *explorer) handle(e event.Effect) {
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
