package balance

import (
	"fmt"
	"math/rand/v2"
	"sort"

	"github.com/nao1215/gozen/battle"
	"github.com/nao1215/gozen/data"
	"github.com/nao1215/gozen/headless"
	"github.com/nao1215/gozen/rpg"
)

// Odds は同じ始まりから一つの敵の群れと何度も戦った集計。gozen sim と開発用の
// バランス表示が同じ値を出すよう、両方がこの型と Try・Add を使う。
type Odds struct {
	Troop    string
	Battles  int
	Won      int
	Lost     int
	Fled     int
	Draw     int
	Turns    int // 全戦闘のターンの合計
	Standing int // 勝った戦闘で立っていた仲間の合計
	// HP は勝った戦闘ごとの一行の残り HP の割合（0..1）。並びは戦った順。
	HP []float64
}

// WinRate は勝った割合（0..1）。
func (o Odds) WinRate() float64 { return ratio(o.Won, o.Battles) }

// AvgTurns は 1 戦の平均ターン。
func (o Odds) AvgTurns() float64 {
	if o.Battles == 0 {
		return 0
	}
	return float64(o.Turns) / float64(o.Battles)
}

// AvgStanding は勝った戦闘で立っていた仲間の平均。
func (o Odds) AvgStanding() float64 {
	return float64(o.Standing) / float64(max(1, o.Won))
}

// HPBand は勝った戦闘の残り HP の 10%・50%・90% 点。勝ちが無ければ ok は false。
func (o Odds) HPBand() (low, mid, high float64, ok bool) {
	if len(o.HP) == 0 {
		return 0, 0, 0, false
	}
	s := append([]float64(nil), o.HP...)
	sort.Float64s(s)
	at := func(p float64) float64 { return s[int(p*float64(len(s)-1)+0.5)] }
	return at(0.1), at(0.5), at(0.9), true
}

func ratio(a, b int) float64 {
	if b == 0 {
		return 0
	}
	return float64(a) / float64(b)
}

// Try は st の一行で troop と AUTO 戦闘を 1 回戦う（gozen sim の 1 戦）。st は変わる。
// seen が nil でなければ、出た Step を順に渡す。maxTurns を超えたら引き分け。
func Try(db *data.DB, st *rpg.State, troop string, maxTurns int, seen func(*battle.Battle, []battle.Step)) *battle.Battle {
	b := battle.New(db, st, troop)
	var f func([]battle.Step)
	if seen != nil {
		f = func(steps []battle.Step) { seen(b, steps) }
	}
	headless.AutoBattle(db, st, b, nil, maxTurns, f)
	return b
}

// Add は終わった戦闘 b を集計に足す。
func (o *Odds) Add(b *battle.Battle) {
	o.Battles++
	o.Turns += b.Turn
	switch b.Result {
	case battle.Won:
		o.Won++
		hp, mhp := 0, 0
		for _, u := range b.Party {
			if u.Alive() {
				o.Standing++
			}
			hp += max(0, u.HP)
			mhp += u.Stat("mhp")
		}
		o.HP = append(o.HP, ratio(hp, mhp))
	case battle.Lost:
		o.Lost++
	case battle.Fled:
		o.Fled++
	case battle.Ongoing:
		o.Draw++
	}
}

// Start は i 戦目（0 から）の始まりの状態を作る。毎回新しい状態を返す。
type Start func(i int) (*rpg.State, error)

// Prep は始まりの状態への手直し（gozen sim の -level・-tree・-gear・-as-is）。ゼロ値は何もしない。
type Prep struct {
	AsIs  bool     // SavedStart で全回復せず、保存された HP・MP・状態のまま始める（-level の Lv 上げは回復する）
	Level int      // 0 より大きければ、各仲間をこの Lv の経験値にして全回復する
	Tree  bool     // スキルの点を振る
	Gear  []string // 一行の人数ぶん袋に入れ、一番合う所に着ける
}

func (p Prep) apply(db *data.DB, st *rpg.State) error {
	if p.Level > 0 {
		for _, a := range st.Members() {
			a.GainExp(a.ExpFor(p.Level) - a.Exp)
			a.Recover()
		}
	}
	if p.Tree {
		for _, a := range st.Members() {
			a.SpendSP()
		}
	}
	for _, id := range p.Gear {
		if db.Items[id] == nil {
			return fmt.Errorf("unknown item %q", id)
		}
		st.AddItem(id, len(st.Members()))
	}
	if len(p.Gear) > 0 {
		for _, a := range st.Members() {
			st.EquipBest(a)
			a.Recover()
		}
	}
	return nil
}

// FreshStart は新しく始めたゲームの一行（party が空なら始まりの一行）。i 戦目の乱数は PCG(seed, i)。
func FreshStart(db *data.DB, seed uint64, party []string, p Prep) Start {
	return func(i int) (*rpg.State, error) {
		st := rpg.New(db, rand.New(rand.NewPCG(seed, uint64(i)))) //nolint:gosec // Reproducible gameplay simulation, not security randomness.
		if len(party) > 0 {
			st.Party = nil
			for _, id := range party {
				if db.Actors[id] == nil {
					return nil, fmt.Errorf("unknown actor %q", id)
				}
				st.Join(id)
			}
		}
		if err := p.apply(db, st); err != nil {
			return nil, err
		}
		return st, nil
	}
}

// SavedStart は保存データ save（rpg.State.Save の形、F9 のメモの .save）の一行を全回復した状態
// （p.AsIs なら保存された HP・MP・状態のまま）。i 戦目の乱数は PCG(seed, i)。手直し p は回復の後に当てる。
func SavedStart(db *data.DB, save []byte, seed uint64, p Prep) Start {
	return func(i int) (*rpg.State, error) {
		st, err := rpg.Parse(db, save, rand.New(rand.NewPCG(seed, uint64(i)))) //nolint:gosec // Reproducible gameplay simulation, not security randomness.
		if err != nil {
			return nil, err
		}
		if !p.AsIs {
			recoverAll(st)
		}
		if err := p.apply(db, st); err != nil {
			return nil, err
		}
		return st, nil
	}
}

// StreakOne は st の一行が回復せずに troop と続けて戦い、負けるか引き分けるか逃げるまでに
// 勝った回数（最大 limit）。戦闘の間は battle.End で HP・MP・状態・経験値を一行に戻す。st は変わる。
func StreakOne(db *data.DB, st *rpg.State, troop string, maxTurns, limit int) int {
	wins := 0
	for wins < limit {
		b := Try(db, st, troop, maxTurns, nil)
		b.End()
		if b.Result != battle.Won {
			break
		}
		wins++
	}
	return wins
}

// Streak は start の i 番目の始まりごとの StreakOne（gozen sim -streak）。
func Streak(db *data.DB, troop string, n, maxTurns, limit int, start Start) ([]int, error) {
	out := make([]int, 0, n)
	for i := range n {
		st, err := start(i)
		if err != nil {
			return out, err
		}
		out = append(out, StreakOne(db, st, troop, maxTurns, limit))
	}
	return out, nil
}

// Percentile は並べた値の p 点（0..1、最も近い順位）。空なら 0。
func Percentile(v []int, p float64) int {
	if len(v) == 0 {
		return 0
	}
	s := append([]int(nil), v...)
	sort.Ints(s)
	return s[int(p*float64(len(s)-1)+0.5)]
}

// Sim は start の始まりから troop と n 回戦った集計（gozen sim）。seen は戦闘番号と Step を受ける。
func Sim(db *data.DB, troop string, n, maxTurns int, start Start, seen func(i int, b *battle.Battle, steps []battle.Step)) (Odds, error) {
	o := Odds{Troop: troop}
	for i := range n {
		st, err := start(i)
		if err != nil {
			return o, err
		}
		var f func(*battle.Battle, []battle.Step)
		if seen != nil {
			f = func(b *battle.Battle, steps []battle.Step) { seen(i, b, steps) }
		}
		o.Add(Try(db, st, troop, maxTurns, f))
	}
	return o, nil
}

// Here は今いる場所で出会う敵の群れ。場所の遭遇（when の成り立つ物、重みの割合付き）、
// 場所の行動の battle、extra（戦闘中の群れなど）の順に、重なりを除いて返す。
func Here(db *data.DB, st *rpg.State, extra ...string) []Encounter {
	var out []Encounter
	seen := map[string]bool{}
	add := func(e Encounter) {
		if e.Troop == "" || seen[e.Troop] || db.Troops[e.Troop] == nil {
			return
		}
		seen[e.Troop] = true
		out = append(out, e)
	}
	loc := db.Locations[st.Location]
	if loc != nil {
		env := st.Env()
		total := 0.0
		var picks []Encounter
		for _, t := range loc.Encounters.Troops {
			if ok, err := t.When.Bool(env); err != nil || !ok {
				continue
			}
			w := t.Weight
			if w <= 0 {
				w = 1
			}
			total += w
			picks = append(picks, Encounter{Troop: t.Troop, Share: w})
		}
		for _, p := range picks {
			p.Share /= total
			add(p)
		}
		for _, a := range loc.Actions {
			if ok, err := a.When.Bool(env); err == nil && ok {
				for _, id := range battlesIn(a.Do) {
					add(Encounter{Troop: id})
				}
			}
		}
	}
	for _, id := range extra {
		add(Encounter{Troop: id})
	}
	return out
}

// Encounter は場所で出会う群れ。Share は遭遇の重みの割合（行動の戦いなら 0）。
type Encounter struct {
	Troop string
	Share float64
}

func battlesIn(cmds []data.Command) []string {
	var out []string
	for _, c := range cmds {
		if c.Battle != "" {
			out = append(out, c.Battle)
		}
	}
	return out
}
