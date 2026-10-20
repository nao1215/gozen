package gamedata

import (
	"math"

	"github.com/nao1215/gozen/internal/diag"

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

func (c *checker) numberRange(where string, value, lower, upper float64) {
	if math.IsNaN(value) || math.IsInf(value, 0) || value < lower || value > upper {
		c.errorf(diag.OutOfRange, where, "want a finite number in [%g, %g], got %g", lower, upper, value)
	}
}
