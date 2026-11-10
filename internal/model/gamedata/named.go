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
// The label and the person's name are the text as written, not as shown in the language
// chosen now, and a second entry of the same name in a place is <name>#2
// (DB.eachLocationList), so a name finds one list in every language.

// eachNamed passes every named command list to f until it returns true.
func (db *DB) eachNamed(f func(l commandList) bool) {
	db.eachCommandList(func(l commandList) bool { return l.name != "" && f(l) })
}

// CommandsName is the name of a command list of the data ("" when it is none of them, such
// as the commands of arriving at a place, made on the spot).
func (db *DB) CommandsName(cmds []Command) string {
	name := ""
	db.eachNamed(func(l commandList) bool {
		if len(l.cmds) > 0 && len(cmds) > 0 && &l.cmds[0] == &cmds[0] {
			name = l.name
			return true
		}
		return false
	})
	return name
}

// NamedCommands is the command list of that name (nil when there is none). A name kept
// before names came from the written text holds the label as it was shown then: it still
// finds its list while the game is shown in that language and no list has it as its name.
func (db *DB) NamedCommands(name string) []Command {
	var out, old []Command
	db.eachNamed(func(l commandList) bool {
		if l.name == name {
			out = l.cmds
			return true
		}
		if l.shown == name && old == nil {
			old = l.cmds
		}
		return false
	})
	if out == nil {
		return old
	}
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
			if cmd.Open.Screen != "" {
				visitScreen(cmd.Open.Screen)
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
