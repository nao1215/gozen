package play

import data "github.com/nao1215/gozen/internal/model/gamedata"

// derived is a value worked out from the game's data and kept for as long as
// that data is the game's: a live reload replaces the DB, and the value is then
// made again on its next use. The Game holds one for each such value instead of
// a value and the DB it was made for side by side.
type derived[T any] struct {
	db    *data.DB
	value T
}

// of is the value for db, made by build when db is not the one it was last made for.
func (d *derived[T]) of(db *data.DB, build func() T) T {
	if d.db != db {
		d.value, d.db = build(), db
	}
	return d.value
}
