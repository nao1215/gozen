package gamedata

import (
	"fmt"
	"maps"
	"math"
	"slices"

	"github.com/nao1215/gozen/internal/base/diag"

	"github.com/goccy/go-yaml"
)

// sectionMap returns the items before type conversion. It separates omitted from 0 without changing the numeric Go API.
func sectionMap(raw yaml.MapSlice, name string) yaml.MapSlice {
	for _, item := range raw {
		if item.Key == name {
			m, ok := item.Value.(yaml.MapSlice)
			if !ok {
				return nil
			}
			return m
		}
	}
	return nil
}

// numberDefault makes an explicit 0 the default too, for the old spec only. Out-of-range values are diagnosed by Check.
func (db *DB) numberDefault(raw yaml.MapSlice, name string, value *float64, fallback float64) {
	for _, item := range raw {
		if item.Key == name {
			if db.Game.SchemaVersion == LegacySchemaVersion && *value == 0 {
				*value = fallback
			}
			return
		}
	}
	*value = fallback
}

// finiteNumber is shared by schema validators before range comparisons, which
// alone cannot reject NaN.
func finiteNumber(v float64) bool { return !math.IsNaN(v) && !math.IsInf(v, 0) }

func (c *checker) numberRange(where string, value, lower, upper float64) {
	if !finiteNumber(value) || value < lower || value > upper {
		c.errorf(diag.OutOfRange, where, "want a finite number in [%g, %g], got %g", lower, upper, value)
	}
}

// amounts reports the amounts the economy and the battle add and take that are below
// zero, in every schema version: a negative price pays the buyer, negative experience
// or gold takes from the party, a negative cost restores MP, a negative drop chance
// never holds and a negative loop start is before the track.
func (c *checker) amounts() {
	db := c.db
	amount := func(where string, n float64) {
		if !finiteNumber(n) || n < 0 {
			c.errorf(diag.OutOfRange, where, "want a finite number of 0 or more, got %g", n)
		}
	}
	for _, id := range slices.Sorted(maps.Keys(db.Items)) {
		if it := db.Items[id]; it != nil {
			amount("items."+id+".price", float64(it.Price))
		}
	}
	for i, f := range db.Fusions {
		amount(fmt.Sprintf("fusions[%d].price", i), float64(f.Price))
	}
	for _, id := range slices.Sorted(maps.Keys(db.Skills)) {
		if sk := db.Skills[id]; sk != nil {
			amount("skills."+id+".mp", float64(sk.MP))
		}
	}
	for _, id := range slices.Sorted(maps.Keys(db.Enemies)) {
		e := db.Enemies[id]
		if e == nil {
			continue
		}
		amount("enemies."+id+".exp", float64(e.Exp))
		amount("enemies."+id+".gold", float64(e.Gold))
		for i, d := range e.Drops {
			amount(fmt.Sprintf("enemies.%s.drops[%d].chance", id, i), d.Chance)
		}
	}
	amount("music.bars", float64(db.Music.Bars))
	amount("music.beats_per_bar", float64(db.Music.Beats))
	for _, name := range slices.Sorted(maps.Keys(db.Music.Songs)) {
		amount("music.songs."+name+".loop_start", db.Music.Songs[name].LoopStart)
		amount("music.songs."+name+".bpm", float64(db.Music.Songs[name].BPM))
	}
}
