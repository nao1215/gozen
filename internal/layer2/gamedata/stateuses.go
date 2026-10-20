package gamedata

import (
	"fmt"
	"maps"
	"slices"
	"strings"
)

// NameUse is where the game writes and where it reads one name of the state.
// The places are paths in the form of the check's messages (events.dinner.do[2].set).
type NameUse struct {
	Sets  []string
	Reads []string
}

// StateUses indexes the names of the state that events and places write and read:
// flag.<name>, var.<name>, item.<id>, ran.<event>, gold and the other names that set writes.
// A name read with fields after it is indexed by its first two parts (flag.met, item.key).
// It walks events (their when and commands, into the branches of if, choice and battle)
// and the people, actions, exits and on-arrival commands of places.
//
// One index is shared by the checks that warn of a flag that is set but never read (or read
// but never set), by gozen book's flow of events and by the --dev panel (ADR 0029).
func (db *DB) StateUses() map[string]*NameUse {
	u := stateUses{out: map[string]*NameUse{}}
	for _, name := range slices.Sorted(maps.Keys(db.Events)) {
		e := db.Events[name]
		w := "events." + name
		u.expr(w+".when", e.When)
		u.commands(w+".do", e.Do)
	}
	for _, name := range slices.Sorted(maps.Keys(db.Locations)) {
		l := db.Locations[name]
		w := "locations." + name
		u.commands(w+".on_enter", l.OnEnter)
		for i, p := range l.People {
			pw := fmt.Sprintf("%s.people[%d]", w, i)
			u.expr(pw+".when", p.When)
			u.expr(pw+".new", p.New)
			u.expr(pw+".mark", p.Mark)
			u.commands(pw+".talk", p.Talk)
		}
		for i, a := range l.Actions {
			aw := fmt.Sprintf("%s.actions[%d]", w, i)
			u.expr(aw+".when", a.When)
			u.expr(aw+".new", a.New)
			u.expr(aw+".mark", a.Mark)
			u.commands(aw+".do", a.Do)
		}
		for i, x := range l.Exits {
			xw := fmt.Sprintf("%s.exits[%d]", w, i)
			u.expr(xw+".when", x.When)
			u.commands(xw+".do", x.Do)
		}
	}
	return u.out
}

type stateUses struct {
	out map[string]*NameUse
}

func (u *stateUses) use(name string) *NameUse {
	n := u.out[name]
	if n == nil {
		n = &NameUse{}
		u.out[name] = n
	}
	return n
}

func (u *stateUses) set(name, where string) {
	n := u.use(name)
	if !slices.Contains(n.Sets, where) {
		n.Sets = append(n.Sets, where)
	}
}

func (u *stateUses) read(name, where string) {
	n := u.use(name)
	if !slices.Contains(n.Reads, where) {
		n.Reads = append(n.Reads, where)
	}
}

// expr indexes the names an expression reads.
func (u *stateUses) expr(where string, e Expr) {
	if !e.Set() {
		return
	}
	for _, r := range e.Refs() {
		name := r.Name
		if len(r.Path) > 0 && r.Path[0].Name != "" && stateTable(name) {
			name += "." + r.Path[0].Name
		}
		u.read(name, where)
	}
}

// stateTable reports whether a name is a table of the state whose entries are indexed one by one.
func stateTable(name string) bool {
	switch name {
	case "flag", "var", "item", "ran", "chose":
		return true
	}
	return false
}

// text indexes the names read by the {expression} parts of a line.
func (u *stateUses) text(where, s string) {
	for _, src := range Interpolations(s) {
		if e, err := parseExpr(src); err == nil {
			u.expr(where, e)
		}
	}
}

func (u *stateUses) commands(where string, cs []Command) {
	for i := range cs {
		c := &cs[i]
		w := fmt.Sprintf("%s[%d]", where, i)
		if c.Say != nil {
			u.text(w+".say", c.Say.Text)
		}
		u.expr(w+".if", c.If)
		for _, k := range slices.Sorted(maps.Keys(c.SetV)) {
			u.set(setName(k), w+".set")
			u.expr(w+".set."+k, c.SetV[k])
		}
		if c.Give != nil {
			u.set("item."+c.Give.Item, w+".give")
		}
		if c.Take != nil {
			u.set("item."+c.Take.Item, w+".take")
		}
		if c.Gold.Set() {
			u.set("gold", w+".gold")
			u.expr(w+".gold", c.Gold)
		}
		if c.Join != "" || c.Leave != "" {
			u.set("party", w)
		}
		if c.Move != "" {
			u.set("location", w+".move")
		}
		if c.Call != "" {
			u.set("ran."+c.Call, w+".call")
		}
		u.commands(w+".then", c.Then)
		u.commands(w+".else", c.Else)
		u.commands(w+".on_win", c.OnWin)
		u.commands(w+".on_lose", c.OnLose)
		u.commands(w+".on_escape", c.OnRun)
		for j, o := range c.Choice {
			ow := fmt.Sprintf("%s.choice[%d]", w, j)
			u.expr(ow+".when", o.When)
			u.commands(ow+".do", o.Do)
		}
	}
}

// setName is the indexed name of a left-hand side of set (actor.scout.hp is actor.scout).
func setName(key string) string {
	parts := strings.SplitN(key, ".", 3)
	if len(parts) > 2 {
		return parts[0] + "." + parts[1]
	}
	return key
}
