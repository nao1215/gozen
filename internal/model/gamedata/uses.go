package gamedata

import (
	"fmt"
	"maps"
	"slices"
)

// Uses reports whether any command in the game has the verb (such as sp_suggest).
// It walks every command list of the game (EachCommand). It is used to show a screen
// field only when the game uses it.
func (db *DB) Uses(verb string) bool {
	found := false
	db.EachCommand(func(c *Command) {
		if v, err := c.Verb(); err == nil && v == verb {
			found = true
		}
	})
	return found
}

// EachCommand walks every command in the game, including authored screen actions,
// descending into if, choice and battle branches. Loaders use the mutable command
// pointers to resolve defaults; presentation and CLI consumers inspect feature use.
func (db *DB) EachCommand(fn func(*Command)) {
	var in func(cs []Command)
	in = func(cs []Command) {
		for i := range cs {
			c := &cs[i]
			fn(c)
			c.branches(func(_ string, _ int, cs []Command) { in(cs) })
		}
	}
	db.eachCommandList(func(l commandList) bool {
		in(l.cmds)
		return false
	})
}

// commandList is one command list written at the top of a definition: an event's do, a
// person's talk, a dungeon tile's do. The lists nested in its commands are reached with
// Command.branches.
type commandList struct {
	// where is the list's place in the form of the check's messages (events.dinner.do).
	where string
	// name is the name a save or a note keeps to begin the list again (named.go); "" for a
	// list that is not begun again by name. shown is the name the list had before names
	// came from the written text: with the label or the person's name as it is shown now.
	name, shown string
	cmds        []Command
}

// eachCommandList passes every top-level command list of the game to f, in a fixed order,
// until f returns true. It is the one walk over the definitions that hold commands:
// EachCommand, StateUses, SourceTexts and the named lists all read it, so a list added
// here is resolved, indexed, translated and named alike.
func (db *DB) eachCommandList(f func(commandList) bool) {
	stop := false
	list := func(where, name string, cmds []Command) {
		stop = stop || f(commandList{where: where, name: name, cmds: cmds})
	}
	for _, id := range slices.Sorted(maps.Keys(db.Events)) {
		if e := db.Events[id]; e != nil {
			list("events."+id+".do", id, e.Do)
		}
	}
	for _, id := range slices.Sorted(maps.Keys(db.Troops)) {
		if t := db.Troops[id]; t != nil {
			for i := range t.Events {
				list(fmt.Sprintf("troops.%s.events[%d].do", id, i), "", t.Events[i].Do)
			}
		}
	}
	for _, id := range slices.Sorted(maps.Keys(db.Locations)) {
		if l := db.Locations[id]; l != nil && !stop {
			stop = db.eachLocationList(id, l, f)
		}
	}
	for _, id := range slices.Sorted(maps.Keys(db.Game.Screens)) {
		if screen := db.Game.Screens[id]; screen != nil {
			for i := range screen.Entries {
				list(fmt.Sprintf("game.yaml.screens.%s.entries[%d].do", id, i), fmt.Sprintf("screen:%s:%d", id, i), screen.Entries[i].Do)
			}
		}
	}
	if cp := db.Game.Campaign; cp != nil {
		for _, id := range cp.OrderIDs() {
			if o := cp.Orders[id]; o != nil {
				w := "game.yaml campaign.orders." + id
				list(w+".do", "", o.Do)
				list(w+".on_win", "", o.OnWin)
				list(w+".on_lose", "", o.OnLose)
				list(w+".on_draw", "", o.OnDraw)
			}
		}
	}
}

// eachLocationList passes the command lists of one place to f and reports whether f
// stopped the walk. An action and a person are named <place>.<label or name> from the
// text as written, which a change of language leaves alone; a second entry of the same
// name in the place is <name>#2, so no two lists share a name.
func (db *DB) eachLocationList(id string, l *Location, f func(commandList) bool) bool {
	w := "locations." + id
	stop := false
	list := func(where, name, shown string, cmds []Command) {
		stop = stop || f(commandList{where: where, name: name, shown: shown, cmds: cmds})
	}
	list(w+".on_enter", "", "", l.OnEnter)
	taken := map[string]int{}
	named := func(text *string) (name, shown string) {
		name = id + "." + db.written(text)
		taken[name]++
		if n := taken[name]; n > 1 {
			return fmt.Sprintf("%s#%d", name, n), ""
		}
		return name, id + "." + *text
	}
	for i := range l.Actions {
		name, shown := named(&l.Actions[i].Label)
		list(fmt.Sprintf("%s.actions[%d].do", w, i), name, shown, l.Actions[i].Do)
	}
	for i := range l.People {
		name, shown := named(&l.People[i].Name)
		list(fmt.Sprintf("%s.people[%d].talk", w, i), name, shown, l.People[i].Talk)
	}
	for i := range l.Exits {
		list(fmt.Sprintf("%s.exits[%d].do", w, i), "", "", l.Exits[i].Do)
	}
	if s := l.Stage; s != nil {
		for _, sid := range s.SpotIDs() {
			if sp := s.Spots[sid]; sp != nil {
				list(w+".stage.spots."+sid+".do", "", "", sp.Do)
			}
		}
		for i := range s.Things {
			list(fmt.Sprintf("%s.stage.things[%d].do", w, i), "", "", s.Things[i].Do)
		}
	}
	if a := l.Map; a != nil {
		for _, pid := range a.PointIDs() {
			if p := a.Points[pid]; p != nil {
				list(w+".map.points."+pid+".do", "", "", p.Do)
			}
		}
		for i := range a.Roads {
			list(fmt.Sprintf("%s.map.roads[%d].do", w, i), "", "", a.Roads[i].Do)
		}
	}
	if d := l.Dungeon; d != nil {
		for _, c := range slices.Sorted(maps.Keys(d.Tiles)) {
			if t := d.Tiles[c]; t != nil {
				list(w+".dungeon.tiles."+c+".do", id+".tile."+c, "", t.Do)
			}
		}
		for i := range d.Roamers {
			rw := fmt.Sprintf("%s.dungeon.roamers[%d]", w, i)
			list(rw+".on_appear", "", "", d.Roamers[i].OnAppear)
			list(rw+".on_leave", "", "", d.Roamers[i].OnLeave)
			list(rw+".meet", "", "", d.Roamers[i].Meet)
		}
		list(w+".dungeon.complete", "", "", d.Complete)
		list(w+".dungeon.walked", "", "", d.Walked)
	}
	return stop
}

// branches passes the command lists nested in c to f: field is the list's place after the
// command's own (.then, .muted.do), or .choice with the index of its option (-1 otherwise).
func (c *Command) branches(f func(field string, option int, cs []Command)) {
	f(".then", -1, c.Then)
	f(".else", -1, c.Else)
	f(".on_win", -1, c.OnWin)
	f(".on_lose", -1, c.OnLose)
	f(".on_escape", -1, c.OnRun)
	for j := range c.Choice {
		f(".choice", j, c.Choice[j].Do)
	}
	if c.Muted != nil {
		f(".muted.do", -1, c.Muted.Do)
	}
}

// branchPath is the place of a list Command.branches passed, under the command at w.
func branchPath(w, field string, option int) string {
	if option < 0 {
		return w + field
	}
	return fmt.Sprintf("%s%s[%d].do", w, field, option)
}
