package gamedata

import (
	"fmt"
	"io/fs"
	"maps"
	"slices"
	"time"

	"github.com/nao1215/gozen/internal/react"
)

func (db *DB) loadReactions() error {
	db.ReactionBooks = map[string]*react.Book{}
	for _, who := range slices.Sorted(maps.Keys(db.Game.Reactions)) {
		name := db.Game.Reactions[who]
		where := "game.yaml.reactions." + who
		if db.Actors[who] == nil || !fs.ValidPath(name) || name == "." {
			return fmt.Errorf("%s: needs a known speaker and a relative asset filename", where)
		}
		book, err := react.Load(db.FS, "assets/"+name)
		if err != nil {
			return fmt.Errorf("%s: %w", where, err)
		}
		if _, err := react.New(book, who, react.Config{Clock: func() time.Duration { return 0 }}); err != nil {
			return fmt.Errorf("%s: %w", where, err)
		}
		db.ReactionBooks[who] = book
	}
	return nil
}
