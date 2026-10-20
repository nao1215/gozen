//go:build js

package main

import (
	"log"

	"github.com/hajimehoshi/ebiten/v2"

	"github.com/nao1215/gozen"
	"github.com/nao1215/gozen/play"
)

// In a browser the game runs as a gozen.Session on a play.Screen in ebiten's own loop;
// the saves go to the page's localStorage.
func main() {
	fsys, err := gameFS()
	if err != nil {
		log.Fatal(err)
	}
	g, err := gozen.Load(fsys, ".")
	if err != nil {
		log.Fatal(err)
	}
	s, err := gozen.New(g, gozen.Config{})
	if err != nil {
		log.Fatal(err)
	}
	sc, err := play.New(s, play.Config{})
	if err != nil {
		log.Fatal(err)
	}
	defer func() {
		if err := sc.Close(); err != nil {
			log.Print(err)
		}
	}()
	ebiten.SetWindowTitle("Wayfinder")
	if err := ebiten.RunGame(sc); err != nil {
		log.Print(err)
	}
}
