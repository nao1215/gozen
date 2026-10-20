package rpg

import (
	"maps"
	"slices"

	data "github.com/nao1215/gozen/internal/layer2/gamedata"
)

// Keys is the list of names an expression-side object holds (so that gozen's Query can map the object to a Go value; ADR 0021).
// Names looked up by key that cannot be enumerated (such as with_<state>) are not included.

// Keys are the names of an actor's fields.
func (v ActorView) Keys() []string {
	return append([]string{
		"id", "name", "level", "exp", "hp", "mp", "hp_rate", "mp_rate", "alive", "dead",
		"class", "equip", "states", "back", "sp", "wears", "tactics", "skills", "sets",
	}, data.StatNames...)
}

// Keys are the names of a group's fields.
func (g *Group) Keys() []string { return []string{"size", "alive", "dead", "hp_rate", "members"} }

// Keys are the items the actor wears.
func (w WearsView) Keys() []string {
	if w.A == nil {
		return nil
	}
	return slices.Sorted(maps.Values(w.A.Equip))
}

func (e equipView) Keys() []string  { return slices.Sorted(maps.Keys(e)) }
func (s statesView) Keys() []string { return slices.Sorted(maps.Keys(s)) }
func (f flagView) Keys() []string   { return slices.Sorted(maps.Keys(f.s.Flags)) }
func (v varView) Keys() []string    { return slices.Sorted(maps.Keys(v.s.Vars)) }
func (v itemView) Keys() []string   { return slices.Sorted(maps.Keys(v.s.Items)) }
func (v actorsView) Keys() []string { return slices.Sorted(maps.Keys(v.s.Actors)) }
func (p inParty) Keys() []string    { return append(p.ActorView.Keys(), "in_party") }
