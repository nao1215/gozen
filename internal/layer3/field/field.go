// Package world is the rules of the field without a screen: what can be done at the
// location the party is at, the events that start on arriving or by themselves, and the
// random battles. The screen (play) and the headless driver (headless)
// both use it.
package field

import (
	"github.com/nao1215/gozen/internal/layer1/optional"
	data "github.com/nao1215/gozen/internal/layer2/gamedata"
	"github.com/nao1215/gozen/internal/layer3/atlas"
	"github.com/nao1215/gozen/internal/rpg"
	lang "github.com/nao1215/gozen/internal/text"
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
	New    bool // there is a new story (the new of a person or item holds); not shown on screen
	Marked bool // the mark of a person or item holds; a marker is attached at the end of Label
	Person *data.Person
	Action *data.Action
	Exit   *data.Exit
	// Spot is where one stands to take the entry at a map or a stage (a point or a spot);
	// taking it puts the party there (State.Spot).
	Spot string
	// Thing is a thing of the stage looked at (its own do), SpotDo a spot with do.
	Thing  *data.Thing
	SpotDo string
	// Choice is a point of the map gone to (its roads, its do and its move).
	Choice *atlas.Choice
}

func holds(st *rpg.State, e data.Expr) bool {
	ok, err := e.Bool(st.Env())
	return err == nil && ok
}

// Entries are what can be done at the party's location now: talk to the people there,
// the location's actions, the things and the spots with do of its stage, search (when it
// has battles), the points of its map, the exits, and the menu. At a map or a stage each
// entry names where one stands to take it (Spot).
func Entries(db *data.DB, st *rpg.State) []Entry {
	loc := db.Locations[st.Location]
	env := st.Env()
	stage := loc.Stage
	var out []Entry
	for i := range loc.People {
		p := &loc.People[i]
		if loc.TalkOnEnter { // played on entering (Arrive)
			break
		}
		if holds(st, p.When) {
			e := Entry{Kind: Talk, Label: lang.T("話す：") + data.Render(p.Name, env), Person: p}
			if sp := stagePerson(stage, p.ID); sp != nil {
				e.Spot = stage.NearestSpot(sp.Feet())
			}
			mark(st, &e, p.New, p.Mark)
			out = append(out, e)
		}
	}
	for i := range loc.Actions {
		a := &loc.Actions[i]
		if holds(st, a.When) {
			e := Entry{Kind: Do, Label: data.Render(a.Label, env), Action: a}
			if stage != nil {
				if t := stage.ThingOf(a.ID); t != nil {
					e.Spot = stage.NearestSpot(data.Point{t.Area.Poly().Center().X, t.Area.Poly().Center().Y})
				}
			}
			mark(st, &e, a.New, a.Mark)
			out = append(out, e)
		}
	}
	if stage != nil {
		for i := range stage.Things {
			t := &stage.Things[i]
			if t.Action != "" || !holds(st, t.When) || t.Once && st.Done[atlas.DoneKey(st.Location, t.ID)] {
				continue
			}
			c := t.Area.Poly().Center()
			out = append(out, Entry{Kind: Do, Label: data.Render(t.Name, env), Thing: t, Spot: stage.NearestSpot(data.Point{c.X, c.Y})})
		}
		for _, id := range stage.SpotIDs() {
			sp := stage.Spots[id]
			if len(sp.Do) == 0 || sp.Once && st.Done[atlas.DoneKey(st.Location, id)] {
				continue
			}
			label := id
			if sp.Name != "" {
				label = data.Render(sp.Name, env)
			}
			out = append(out, Entry{Kind: Do, Label: label, SpotDo: id, Spot: id})
		}
	}
	if loc.Encounters.Search != "" && len(loc.Encounters.Troops) > 0 {
		out = append(out, Entry{Kind: Search, Label: loc.Encounters.Search})
	}
	for _, c := range atlas.Choices(db, st) {
		p := loc.Map.Points[c.Point]
		label := p.Label
		if label == "" && p.To != "" {
			label = db.Locations[p.To].Name
		}
		if label == "" {
			label = c.Point
		}
		e := Entry{Kind: Exit, Label: "→ " + data.Render(label, env), Spot: c.Point, Choice: &c, Exit: &data.Exit{To: p.To, Label: p.Label, At: p.AtPoint}}
		if c.Locked {
			e.Exit.To = ""
		}
		if p.To == "" {
			e.Kind, e.Label, e.Exit = Do, data.Render(label, env), nil
		}
		out = append(out, e)
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

// mark sets New if the new expression holds, and marks the item if a mark expression is written and holds.
// The mark character is lang.T("　！") (changeable via the game's game.yaml text). What the mark signals is up to the game.
func mark(st *rpg.State, e *Entry, n, m data.Expr) {
	e.New = n.Set() && holds(st, n)
	if m.Set() && holds(st, m) {
		e.Marked = true
		e.Label += lang.T("　！")
	}
}

// stagePerson is the stage's place of a person, or nil.
func stagePerson(s *data.Stage, id string) *data.StagePerson {
	if s == nil || id == "" {
		return nil
	}
	return s.Person(id)
}

// Commands are the commands an entry runs (nil for the menu). At a map or a stage taking
// an entry puts the party where it is taken (State.Spot) and marks a thing or a spot with
// once done; at a map it goes along the roads to the point (atlas.Go).
func (e Entry) Commands(db *data.DB, st *rpg.State) []data.Command {
	if e.Choice != nil {
		return atlas.Go(db, st, *e.Choice)
	}
	if e.Spot != "" {
		st.Spot = e.Spot
	}
	switch {
	case e.Thing != nil:
		if e.Thing.Once {
			st.Done[atlas.DoneKey(st.Location, e.Thing.ID)] = true
		}
		return e.Thing.Do
	case e.SpotDo != "":
		return Step(db, st, e.SpotDo)
	}
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
		if len(ev.Do) > 0 { // a copy is invisible to the event count, so put a counting command first (ran.<ID>)
			cmds = append(cmds, data.Command{Ran: id})
		}
		cmds = append(cmds, ev.Do...)
	}
	if loc.TalkOnEnter { // after the events on entering, one person's talk
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
	return atlas.PickTroop(st, db.Locations[st.Location].Encounters.Troops)
}

// Step is stepping on a spot of the party's stage: the party stands there, and the spot's
// do runs (once: the first time only, then nil).
func Step(db *data.DB, st *rpg.State, spot string) []data.Command {
	l := db.Locations[st.Location]
	if l == nil || l.Stage == nil || l.Stage.Spots[spot] == nil {
		return nil
	}
	st.Spot = spot
	sp := l.Stage.Spots[spot]
	key := atlas.DoneKey(st.Location, spot)
	if len(sp.Do) == 0 || sp.Once && st.Done[key] {
		return nil
	}
	if sp.Once {
		st.Done[key] = true
	}
	return sp.Do
}

// talkOnEnter is the talk when entering a talk_on_enter location (one person: people with a new story first, otherwise in turn from the continuation of the previous one).
// The turn order is kept in var.talk_turn_<location>.
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
	n := optional.Value[float64](st.Vars[key])
	st.Vars[key] = n + 1
	return loc.People[here[int(n)%len(here)]].Talk
}
