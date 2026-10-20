package event

import (
	"errors"
	"fmt"
	"strings"

	data "github.com/nao1215/gozen/internal/layer2/gamedata"
	"github.com/nao1215/gozen/internal/layer3/dungeon"
	"github.com/nao1215/gozen/internal/layer3/field"
)

// Commands of spec version 2 and the names usable only in event expressions (ADR 0021).

// maxWalk is the upper bound of steps one walk takes (so it cannot loop forever when there is no path).
const maxWalk = 5000

// walking is the continuation of step and walk (per pushed sequence of one command): the starting place and the remaining count.
type walking struct {
	from string
	left int
}

// cont pushes the continuation commands. If the place changes, the continuation does not run.
func (r *Runner) cont(c data.Command, left int, from string) {
	cmds := []data.Command{c}
	if r.walks == nil {
		r.walks = map[*data.Command]walking{}
	}
	r.walks[&cmds[0]] = walking{from: from, left: left}
	// The continuation is a tail call: the finished frame is dropped first (so it does not get deeper even if the step count exceeds the nesting limit of 64).
	for n := len(r.stack); n > 0 && r.stack[n-1].pc >= len(r.stack[n-1].cmds); n = len(r.stack) {
		r.stack = r.stack[:n-1]
	}
	r.push(cmds)
}

// walkState returns c's state if c is a continuation and removes it from the table.
func (r *Runner) walkState(c *data.Command, left int) (walking, bool) {
	if w, ok := r.walks[c]; ok {
		delete(r.walks, c)
		return w, w.from == r.st.Location
	}
	return walking{from: r.st.Location, left: left}, true
}

func (r *Runner) inDungeon(verb string) bool {
	if dungeon.Of(r.db, r.st) == nil {
		r.logf(fmt.Errorf("%s: %s is not a dungeon", verb, r.st.Location))
		return false
	}
	return true
}

// turn changes the facing direction.
func (r *Runner) turn(way string) {
	if !r.inDungeon("turn") {
		return
	}
	switch way {
	case "left":
		dungeon.Turn(r.st, -1)
	case "right":
		dungeon.Turn(r.st, 1)
	case "back":
		dungeon.Turn(r.st, 2)
	default:
		r.logf(fmt.Errorf("turn: want left, right or back, not %q", way))
	}
}

// step advances one step, pushes the continuation if any remain, then pushes the commands of the floor stepped on (the floor's commands run first).
func (r *Runner) step(c *data.Command) {
	w, ok := r.walkState(c, c.Step)
	if !ok || !r.inDungeon("step") {
		return
	}
	moved, cmds := dungeon.Forward(r.db, r.st)
	if moved && w.left > 1 {
		r.cont(data.Command{Step: c.Step}, w.left-1, w.from)
	}
	r.push(cmds)
}

// walk advances one move toward the tile of that character, pushing the continuation until it arrives.
func (r *Runner) walk(c *data.Command) {
	w, ok := r.walkState(c, maxWalk)
	if !ok || !r.inDungeon("walk") {
		return
	}
	goal := []rune(c.Walk)
	if len(goal) != 1 {
		r.logf(fmt.Errorf("walk: want one character, not %q", c.Walk))
		return
	}
	if w.left <= 0 {
		r.logf(fmt.Errorf("walk: could not reach %q in %s", c.Walk, r.st.Location))
		return
	}
	move, ok := dungeon.Next(r.db, r.st, goal[0])
	if !ok {
		r.logf(fmt.Errorf("walk: no way to %q in %s from %d,%d", c.Walk, r.st.Location, r.st.Pos.X, r.st.Pos.Y))
		return
	}
	again := func() { r.cont(data.Command{Walk: c.Walk}, w.left-1, w.from) }
	switch move {
	case dungeon.MoveHere:
	case dungeon.MoveLeft:
		dungeon.Turn(r.st, -1)
		again()
	case dungeon.MoveRight:
		dungeon.Turn(r.st, 1)
		again()
	case dungeon.MoveLook:
		r.push(dungeon.Look(r.db, r.st))
	case dungeon.MoveForward:
		there := dungeon.Front(r.db, r.st).Char == goal[0]
		_, cmds := dungeon.Forward(r.db, r.st)
		if !there {
			again()
		}
		r.push(cmds)
	}
}

// pickEntry picks an entry of the current place's menu by display name (the same display name, otherwise the first that contains it).
func (r *Runner) pickEntry(label string) {
	entries := field.Entries(r.db, r.st)
	for _, exact := range []bool{true, false} {
		for _, e := range entries {
			if exact && e.Label == label || !exact && strings.Contains(e.Label, label) {
				r.push(e.Commands(r.db, r.st))
				return
			}
		}
	}
	labels := make([]string, 0, len(entries))
	for _, e := range entries {
		labels = append(labels, e.Label)
	}
	r.logf(fmt.Errorf("pick: at %s there is no %q (there is %s)", r.st.Location, label, strings.Join(labels, ", ")))
}

// use uses an item from the menu (the screen or the mock actually uses it via UseItem).
func (r *Runner) use(c *data.Command) {
	if r.UseItem == nil {
		r.logf(errors.New("use: this runner cannot use items (UseItem is nil)"))
		return
	}
	lines, err := r.UseItem(c.Use, c.Target)
	if err != nil {
		r.logf(fmt.Errorf("use %s: %w", c.Use, err))
		return
	}
	r.notes = append(r.notes, lines...)
}

// equip equips a bag item to a party member (if slot is omitted, the item's free slot, otherwise the first slot).
// With neither item nor slot, it is the menu's "さいきょうそうび" (equip the better items from the bag than the current ones).
func (r *Runner) equip(e *data.EquipItem) {
	a := r.st.Actors[e.Actor]
	if a == nil {
		r.logf(fmt.Errorf("equip: %s has not joined", e.Actor))
		return
	}
	if e.Item == "" && e.Slot == "" {
		r.st.EquipUpgrades(a)
		return
	}
	slot := e.Slot
	if slot == "" && e.Item != "" {
		if it := r.db.Items[e.Item]; it != nil {
			for _, s := range r.db.Game.Slots {
				if it.Fits(s.ID) && (slot == "" || a.Equip[slot] != "" && a.Equip[s.ID] == "") {
					slot = s.ID
				}
			}
		}
	}
	if slot == "" {
		r.logf(fmt.Errorf("equip: %s has no slot", e.Item))
		return
	}
	r.logf(r.st.Equip(a, slot, e.Item))
}

// setAllowed reports whether the left side of set may be written now (stat values only in Dev).
func (r *Runner) setAllowed(key string) bool {
	if data.DevSet(key) && !r.Dev {
		r.logf(fmt.Errorf("set %s: stats and levels are set only in the development mode (--dev)", key))
		return false
	}
	return true
}

// countRun counts the event as passed (ran.<ID>) when cmds is the event's own do (call, auto, before/after battle, start).
func (r *Runner) countRun(cmds []data.Command) {
	if len(cmds) == 0 {
		return
	}
	if r.events == nil {
		r.events = map[*data.Command]string{}
		for id, ev := range r.db.Events {
			if ev != nil && len(ev.Do) > 0 {
				r.events[&ev.Do[0]] = id
			}
		}
	}
	if id, ok := r.events[&cmds[0]]; ok {
		r.st.MarkRan(id)
	}
}

// trade buys and sells at a shop (same prices as the shop screen; selling is at half price).
func (r *Runner) trade(verb string, c *data.Command) {
	t := c.Buy
	if verb == "sell" {
		t = c.Sell
	}
	n := max(1, t.Count)
	var ok bool
	if verb == "buy" {
		ok = r.st.Buy(t.Shop, t.Item, n)
	} else {
		ok = r.st.Sell(t.Shop, t.Item, n)
	}
	if !ok {
		r.logf(fmt.Errorf("%s %d %s at %s: cannot (gold %d, in the bag %d)", verb, n, t.Item, t.Shop, r.st.Gold, r.st.Items[t.Item]))
	}
}
