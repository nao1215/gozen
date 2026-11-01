package gamedata

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"reflect"
)

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

// CommandsFingerprint identifies a replay root, its reachable events and authored
// screens, including a screen-entry root's owning definition.
// Save consumers use this to reject edited actions before replay. JSON canonicalizes
// map order, while command order, expression source and parameter types remain significant.
func (db *DB) CommandsFingerprint(name string) (string, error) {
	cmds := db.NamedCommands(name)
	if cmds == nil {
		return "", fmt.Errorf("unknown replay root %q", name)
	}
	events := map[string]*Event{}
	screens := map[string]*AuthorScreen{}
	var visitEvent, visitScreen func(string)
	walk := func(cmds []Command) {
		eachCommand(reflect.ValueOf(cmds), func(cmd Command) {
			if cmd.Call.Event != "" {
				visitEvent(cmd.Call.Event)
			}
			if cmd.Open != "" {
				visitScreen(cmd.Open)
			}
		})
	}
	visitEvent = func(id string) {
		if _, seen := events[id]; seen {
			return
		}
		ev := db.Events[id]
		events[id] = ev
		if ev != nil {
			walk(ev.Do)
		}
	}
	visitScreen = func(id string) {
		if _, seen := screens[id]; seen {
			return
		}
		screen := db.Game.Screens[id]
		if screen == nil {
			return // Built-in screens have no authored definition.
		}
		screens[id] = screen
		for _, entry := range screen.Entries {
			walk(entry.Do)
		}
	}
	if db.Events[name] != nil {
		visitEvent(name)
	} else {
		walk(cmds)
		for id, screen := range db.Game.Screens {
			if screen == nil {
				continue
			}
			for i := range screen.Entries {
				if name == fmt.Sprintf("screen:%s:%d", id, i) {
					visitScreen(id)
				}
			}
		}
	}
	b, err := json.Marshal(struct {
		Commands []Command
		Events   map[string]*Event
		Screens  map[string]*AuthorScreen `json:",omitempty"`
	}{cmds, events, screens})
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:]), nil
}
