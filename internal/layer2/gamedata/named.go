package gamedata

import "fmt"

// The names of command lists: an event's id, a place's action (<place>.<label>), a place's
// person (<place>.<name>) and a dungeon tile (<place>.tile.<char>). A note of F9, F5 and a
// save made in the middle of an event (ADR 0025) name the commands they begin again.
// Screen entry actions use screen:<id>:<authored index> for the same consumers.

// eachNamed passes every named command list to f until it returns true.
func (db *DB) eachNamed(f func(name string, cmds []Command) bool) {
	for id, screen := range db.Game.Screens {
		if screen == nil {
			continue
		}
		for i, entry := range screen.Entries {
			if f(fmt.Sprintf("screen:%s:%d", id, i), entry.Do) {
				return
			}
		}
	}
	for id, ev := range db.Events {
		if f(id, ev.Do) {
			return
		}
	}
	for id, loc := range db.Locations {
		for _, a := range loc.Actions {
			if f(id+"."+a.Label, a.Do) {
				return
			}
		}
		for _, p := range loc.People {
			if f(id+"."+p.Name, p.Talk) {
				return
			}
		}
		if loc.Dungeon != nil {
			for c, t := range loc.Dungeon.Tiles {
				if t != nil && f(id+".tile."+c, t.Do) {
					return
				}
			}
		}
	}
}

// CommandsName is the name of a command list of the data ("" when it is none of them, such
// as the commands of arriving at a place, made on the spot).
func (db *DB) CommandsName(cmds []Command) string {
	name := ""
	db.eachNamed(func(n string, c []Command) bool {
		if len(c) > 0 && len(cmds) > 0 && &c[0] == &cmds[0] {
			name = n
			return true
		}
		return false
	})
	return name
}

// NamedCommands is the command list of that name (nil when there is none).
func (db *DB) NamedCommands(name string) []Command {
	var out []Command
	db.eachNamed(func(n string, c []Command) bool {
		if n == name {
			out = c
			return true
		}
		return false
	})
	return out
}
