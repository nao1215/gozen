//go:build js

package main

import (
	"context"
	"errors"
	"log"
	"syscall/js"

	"github.com/hajimehoshi/ebiten/v2"

	"github.com/nao1215/gozen/internal/layer2/gamedata"
	"github.com/nao1215/gozen/internal/play"
	"github.com/nao1215/gozen/internal/webfs"
)

func main() {
	if err := run(); err != nil {
		page("goZenFail", err.Error())
	}
}

func run() (err error) {
	fsys, err := webfs.Load("game/", func(done, total int64) { page("goZenProgress", "files", done, total) })
	if err != nil {
		return err
	}
	db, err := gamedata.Load(fsys)
	if err != nil {
		return err
	}
	g, err := play.New(play.Options{FS: fsys, DB: db, Locales: play.DeviceLocales(context.Background())})
	if err != nil {
		return err
	}
	defer func() { err = errors.Join(err, g.Close()) }()
	ebiten.SetWindowTitle(db.Game.Title)
	page("goZenStarting", db.Game.Title)
	g.StartRendering()
	return ebiten.RunGame(&shown{Game: g})
}

type shown struct {
	*play.Game
	done bool
}

func (g *shown) Draw(screen *ebiten.Image) {
	g.Game.Draw(screen)
	if !g.done {
		g.done = true
		page("goZenReady")
	}
}

// page calls the page's function name, when it has one. The page only shows the loading:
// an error it throws is logged and does not stop the game (a file coming in the background
// reports its bytes after the game has started).
func page(name string, args ...any) {
	defer func() {
		if r := recover(); r != nil {
			log.Printf("%s: %v", name, r)
		}
	}()
	if f := js.Global().Get(name); f.Type() == js.TypeFunction {
		f.Invoke(args...)
	}
}
