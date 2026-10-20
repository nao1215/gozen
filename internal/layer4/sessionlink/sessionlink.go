// Package sessionlink is an internal port for handing the contents of gozen's Session to play (it
// does not grow the public API).
// gozen sets Of in init.
package sessionlink

import (
	"io/fs"

	data "github.com/nao1215/gozen/internal/layer2/gamedata"
	"github.com/nao1215/gozen/internal/rpg"
)

// Parts is what the screen needs to continue from a Session.
type Parts struct {
	FS    fs.FS
	DB    *data.DB
	State *rpg.State
	Dev   bool
	Seed  uint64
	// Fresh means nothing has been run since New (no Begin, Do or Start).
	Fresh bool
}

// Of is the contents of s if s is gozen's Session. gozen's init sets it.
var Of = func(any) (Parts, bool) { return Parts{}, false }
