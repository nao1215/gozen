// Package scope builds the names usable only in expressions inside event commands (menu, map, cell, roamers, result, over, cleared, completed, moved).
// It is shared so that event.Runner and gozen's Session expose the same names with the same values (ADR 0021).
package scope

import (
	"errors"
	"strings"

	"github.com/nao1215/gozen/internal/expr"
	data "github.com/nao1215/gozen/internal/layer2/gamedata"
	"github.com/nao1215/gozen/internal/layer3/dungeon"
	"github.com/nao1215/gozen/internal/layer3/field"
	"github.com/nao1215/gozen/internal/rpg"
)

// Last is the result of the previous battle and whether play has ended. The owner writes it and Env reads it.
type Last struct {
	Result        string // win, lose, escape ("" if none)
	Over, Cleared bool
}

// Env is the names on top of db, st and last. last is read on every call (writes are visible).
func Env(db *data.DB, st *rpg.State, last *Last) expr.Env { return env{db, st, last} }

type env struct {
	db   *data.DB
	st   *rpg.State
	last *Last
}

func (e env) Lookup(name string) (any, bool) {
	switch name {
	case "menu":
		var out []any
		for _, en := range field.Entries(e.db, e.st) {
			out = append(out, expr.Map{"kind": KindName(en.Kind), "label": en.Label, "new": en.New, "marked": en.Marked})
		}
		return out, true
	case "map":
		var out []any
		if d := dungeon.Of(e.db, e.st); d != nil {
			w, h := d.Size()
			for y := range h {
				var b strings.Builder
				for x := range w {
					b.WriteRune(dungeon.CellAt(d, e.st, x, y).Char)
				}
				out = append(out, b.String())
			}
		}
		return out, true
	case "cell":
		return expr.Func(e.cell), true
	case "roamers":
		var out []any
		d := dungeon.Of(e.db, e.st)
		for _, r := range e.st.Roamers {
			name := ""
			if d != nil && r.Kind >= 0 && r.Kind < len(d.Roamers) {
				name = d.Roamers[r.Kind].Name
			}
			out = append(out, expr.Map{"kind": float64(r.Kind), "name": name, "troop": r.Troop, "x": float64(r.X), "y": float64(r.Y)})
		}
		return out, true
	case "result":
		return e.last.Result, true
	case "over":
		return e.last.Over, true
	case "cleared":
		return e.last.Cleared, true
	case "completed": // finished walking the current floor (filled the map)
		return dungeon.Of(e.db, e.st) != nil && dungeon.Completed(e.st, e.st.Location), true
	case "moved": // the last step moved (false if it bumped into something)
		return e.st.Moved, true
	case "profile": // the character book (ADR 0031): profile.<id>.unlocked by the state now
		return e.profiles(), true
	}
	return nil, false
}

// cell is cell(x, y): a cell of the dungeon's current floor (nil outside the dungeon).
func (e env) cell(args []any) (any, error) {
	if len(args) != 2 {
		return nil, errors.New("cell takes 2 arguments (x, y)")
	}
	x, ok1 := expr.ToNumber(args[0])
	y, ok2 := expr.ToNumber(args[1])
	if !ok1 || !ok2 {
		return nil, errors.New("cell takes numbers (x, y)")
	}
	d := dungeon.Of(e.db, e.st)
	if d == nil {
		return nil, nil
	}
	c := dungeon.CellAt(d, e.st, int(x), int(y))
	w, h := d.Size()
	shown := c.X >= 0 && c.Y >= 0 && c.X < w && c.Y < h && dungeon.Shown(e.db, e.st)[c.Y*w+c.X]
	return expr.Map{
		"raw": string(d.At(c.X, c.Y)), "shown": shown,
		"char": string(c.Char), "passable": c.Passable(), "solid": c.Solid, "look": c.Look,
		"seen":  dungeon.Seen(e.db, e.st, e.st.Location, c.X, c.Y),
		"spent": dungeon.Spent(e.st, e.st.Location, c.X, c.Y),
	}, nil
}

// KindName is the word for the kind of a menu item (talk, do, search, exit, menu).
func KindName(k field.Kind) string {
	switch k {
	case field.Talk:
		return "talk"
	case field.Do:
		return "do"
	case field.Search:
		return "search"
	case field.Exit:
		return "exit"
	case field.Menu:
		return "menu"
	}
	return ""
}

// ResultName turns a battle answer (1 win, 2 lose, 3 escape) into a result word.
func ResultName(n int) string {
	switch n {
	case 1:
		return "win"
	case 2:
		return "lose"
	case 3:
		return "escape"
	}
	return ""
}

// profiles are the pages of the character book as the state now opens them: whether
// each is unlocked (its when holds), and its name.
func (e env) profiles() expr.Map {
	out := expr.Map{}
	env := e.st.Env()
	for id := range e.db.Profiles {
		out[id] = expr.Map{"unlocked": e.db.ProfileHolds(id, env), "name": e.db.ProfileName(id)}
	}
	return out
}
