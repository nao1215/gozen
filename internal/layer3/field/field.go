// Package world is the rules of the field without a screen: what can be done at the
// location the party is at, the events that start on arriving or by themselves, and the
// random battles. The screen (play) and the headless driver (headless)
// both use it.
package field

import (
	data "github.com/nao1215/gozen/internal/layer2/gamedata"
	"github.com/nao1215/gozen/rpg"
	lang "github.com/nao1215/gozen/text"
)

// Kind is what an entry of the field's menu does.
type Kind int

// The kinds of entries.
const (
	Talk Kind = iota
	Do
	Search
	Exit
	Menu
)

// Entry is a row of the field's menu.
type Entry struct {
	Kind   Kind
	Label  string
	New    bool // 新しい話がある（人・項目の new が成り立つ）。画面には出さない
	Marked bool // 人・項目の mark が成り立つ。Label の末尾に印が付いている
	Person *data.Person
	Action *data.Action
	Exit   *data.Exit
}

func holds(st *rpg.State, e data.Expr) bool {
	ok, err := e.Bool(st.Env())
	return err == nil && ok
}

// Entries are what can be done at the party's location now: talk to the people there,
// the location's actions, search (when it has battles), the exits, and the menu.
func Entries(db *data.DB, st *rpg.State) []Entry {
	loc := db.Locations[st.Location]
	env := st.Env()
	var out []Entry
	for i := range loc.People {
		p := &loc.People[i]
		if loc.TalkOnEnter { // 入った ときに 流す（Arrive）
			break
		}
		if holds(st, p.When) {
			e := Entry{Kind: Talk, Label: lang.T("話す：") + data.Render(p.Name, env), Person: p}
			mark(st, &e, p.New, p.Mark)
			out = append(out, e)
		}
	}
	for i := range loc.Actions {
		a := &loc.Actions[i]
		if holds(st, a.When) {
			e := Entry{Kind: Do, Label: data.Render(a.Label, env), Action: a}
			mark(st, &e, a.New, a.Mark)
			out = append(out, e)
		}
	}
	if loc.Encounters.Search != "" && len(loc.Encounters.Troops) > 0 {
		out = append(out, Entry{Kind: Search, Label: loc.Encounters.Search})
	}
	for i := range loc.Exits {
		e := &loc.Exits[i]
		if !holds(st, e.When) {
			continue
		}
		label := e.Label
		if label == "" && e.To != "" {
			label = db.Locations[e.To].Name
		}
		out = append(out, Entry{Kind: Exit, Label: "→ " + data.Render(label, env), Exit: e})
	}
	if loc.NoMenu {
		return out
	}
	return append(out, Entry{Kind: Menu, Label: lang.T("メニュー")})
}

// mark は、new の式が成り立てば New を立て、mark の式が書かれていて成り立てば項目に印を付ける。
// 印の文字は lang.T("　！")（ゲームの game.yaml の text で変えられる）。印が何を知らせるかはゲームが決める。
func mark(st *rpg.State, e *Entry, n, m data.Expr) {
	e.New = n.Set() && holds(st, n)
	if m.Set() && holds(st, m) {
		e.Marked = true
		e.Label += lang.T("　！")
	}
}

// Commands are the commands an entry runs (nil for the menu).
func (e Entry) Commands(db *data.DB, st *rpg.State) []data.Command {
	switch e.Kind {
	case Menu:
		return nil
	case Talk:
		return e.Person.Talk
	case Do:
		return e.Action.Do
	case Search:
		if t := PickTroop(db, st); t != "" {
			return []data.Command{{Battle: t}}
		}
	case Exit:
		cmds := append([]data.Command{}, e.Exit.Do...)
		if e.Exit.To != "" {
			cmds = append(cmds, data.Command{Move: e.Exit.To, At: e.Exit.At, Facing: e.Exit.Facing})
		}
		return cmds
	}
	return nil
}

// Arrive is what happens on arriving at the party's location: its on_enter and the enter
// events that hold (those with once are marked done), or, when none and battles is set,
// maybe a random battle.
func Arrive(db *data.DB, st *rpg.State, battles bool) []data.Command {
	loc := db.Locations[st.Location]
	var cmds []data.Command
	cmds = append(cmds, loc.OnEnter...)
	for _, id := range db.Ordered("events") {
		ev := db.Events[id]
		if ev.Trigger != "enter" || ev.Location != st.Location || (ev.Once && st.Done[id]) {
			continue
		}
		if !holds(st, ev.When) {
			continue
		}
		if ev.Once {
			st.Done[id] = true
		}
		cmds = append(cmds, ev.Do...)
	}
	if loc.TalkOnEnter { // 入った ときの 出来事の 後に、人の 話を 1 つ
		cmds = append(cmds, talkOnEnter(st, st.Location, loc)...)
	}
	if len(cmds) > 0 || !battles || loc.Dungeon != nil { // in a dungeon, battles come on the steps
		return cmds
	}
	if loc.Encounters.Rate.Set() && !st.Calm {
		p, err := loc.Encounters.Rate.Number(st.Env())
		if err == nil && st.Rand.Float64() < p {
			if t := PickTroop(db, st); t != "" {
				return []data.Command{{Battle: t}}
			}
		}
	}
	return nil
}

// Auto is the first auto event that holds now (marked done when it has once), or nil.
func Auto(db *data.DB, st *rpg.State) []data.Command {
	for _, id := range db.Ordered("events") {
		ev := db.Events[id]
		if ev.Trigger != "auto" || (ev.Location != "" && ev.Location != st.Location) || (ev.Once && st.Done[id]) {
			continue
		}
		if !holds(st, ev.When) {
			continue
		}
		if ev.Once {
			st.Done[id] = true
		}
		if len(ev.Do) > 0 {
			return ev.Do
		}
	}
	return nil
}

// PickTroop draws one of the location's troops by weight ("" when none holds).
func PickTroop(db *data.DB, st *rpg.State) string {
	var picks []data.TroopPick
	total := 0.0
	for _, t := range db.Locations[st.Location].Encounters.Troops {
		if !holds(st, t.When) {
			continue
		}
		if t.Weight <= 0 {
			t.Weight = 1
		}
		picks = append(picks, t)
		total += t.Weight
	}
	x := st.Rand.Float64() * total
	for _, t := range picks {
		if x -= t.Weight; x < 0 {
			return t.Troop
		}
	}
	return ""
}

// talkOnEnter は talk_on_enter の 場所に 入った ときの 話（新しい 話の ある 人を 先に、無ければ 前の 続きの 人から 順番に 1 人）。
// 順番は var.talk_turn_<場所> に 残す。
func talkOnEnter(st *rpg.State, id string, loc *data.Location) []data.Command {
	var here []int
	for i := range loc.People {
		if holds(st, loc.People[i].When) && len(loc.People[i].Talk) > 0 {
			here = append(here, i)
		}
	}
	if len(here) == 0 {
		return nil
	}
	for _, i := range here {
		if p := &loc.People[i]; p.New.Set() && holds(st, p.New) {
			return p.Talk
		}
	}
	key := "talk_turn_" + id
	n, _ := st.Vars[key].(float64) //nolint:errcheck // Optional state values intentionally default to zero when absent.
	st.Vars[key] = n + 1
	return loc.People[here[int(n)%len(here)]].Talk
}
