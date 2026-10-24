//go:build js

package main

import (
	"errors"
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
	fsys, err := webfs.Load("game/")
	if err != nil {
		return err
	}
	db, err := gamedata.Load(fsys)
	if err != nil {
		return err
	}
	g, err := play.New(play.Options{FS: fsys, DB: db})
	if err != nil {
		return err
	}
	defer func() { err = errors.Join(err, g.Close()) }()
	ebiten.SetWindowTitle(db.Game.Title)
	page("goZenStarting", db.Game.Title)
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

func page(name string, args ...any) {
	if f := js.Global().Get(name); f.Type() == js.TypeFunction {
		f.Invoke(args...)
	}
}
