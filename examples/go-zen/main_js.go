//go:build js

// Command go-zen runs Go Zen in the browser: the game files are fetched next to
// the page and the saves go into localStorage.
package main

import (
	"log"
	"syscall/js"

	"github.com/hajimehoshi/ebiten/v2"

	"github.com/nao1215/gozen"
	"github.com/nao1215/gozen/internal/webfs"
	"github.com/nao1215/gozen/play"
)

// In a browser the game's files are not in the program: the page serves them next to it
// (game/files.json lists them, written by web/assets.py with the pictures as WebP), and
// files are fetched when opened so unused pictures do not delay the first screen.
// The game runs as a gozen.Session on a play.Screen; the saves go to the page's
// localStorage.
func main() {
	if err := run(); err != nil {
		page("goZenFail", err.Error())
		log.Fatal(err)
	}
}

func run() error {
	fsys, err := webfs.Load("game/")
	if err != nil {
		return err
	}
	g, err := gozen.Load(fsys, ".")
	if err != nil {
		return err
	}
	s, err := gozen.New(g, gozen.Config{})
	if err != nil {
		return err
	}
	sc, err := play.New(s, play.Config{})
	if err != nil {
		return err
	}
	defer func() {
		if err := sc.Close(); err != nil {
			log.Print(err)
		}
	}()
	ebiten.SetWindowTitle("Go Zen")
	page("goZenStarting")
	return ebiten.RunGame(&shown{Screen: sc})
}

// shown is the game's screen that takes the page's loading note away once the first
// frame is drawn: the first frames read the pictures and can take some seconds, and
// until then the canvas is black.
type shown struct {
	*play.Screen
	done bool
}

func (s *shown) Draw(dst *ebiten.Image) {
	s.Screen.Draw(dst)
	if !s.done {
		s.done = true
		page("goZenReady")
	}
}

// page calls a function of index.html when the page has it.
func page(name string, args ...any) {
	if f := js.Global().Get(name); f.Type() == js.TypeFunction {
		f.Invoke(args...)
	}
}
