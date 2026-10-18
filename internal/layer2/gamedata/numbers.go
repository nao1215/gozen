package gamedata

import (
	"math"

	"github.com/goccy/go-yaml"
)

// sectionMap は型変換前の項目を返す。数値の Go API を変えずに省略と０を分ける。
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

// numberDefault は旧仕様だけ明示した０も既定値にする。範囲外は Check で診断する。
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
		c.errorf(where, "want a finite number in [%g, %g], got %g", lower, upper, value)
	}
}
