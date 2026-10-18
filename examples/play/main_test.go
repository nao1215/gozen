package main

import (
	"errors"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"

	"github.com/nao1215/gozen/data"
	"github.com/nao1215/gozen/gfx"
	"github.com/nao1215/gozen/play"
)

func TestExampleLoadsThroughPublicAPI(t *testing.T) {
	fsys, err := gameFS()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := data.Load(fsys, "."); err != nil {
		t.Fatal(err)
	}
	font, err := gfx.Load(fsys, "assets/regular.ttf")
	if err != nil {
		t.Fatal(err)
	}
	w, h := font.Measure("Wayfinder", 24)
	if w <= 0 || h <= 0 {
		t.Fatalf("font dimensions %v %v", w, h)
	}
}

func TestComposeScreenThroughPublicAPI(t *testing.T) {
	fsys, err := gameFS()
	if err != nil {
		t.Fatal(err)
	}
	g, err := play.New(fsys, play.Config{Mute: true, SaveDir: t.TempDir(), Development: &play.Development{Start: "camp", Script: "quit", Shots: t.TempDir()}})
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := g.Close(); err != nil {
			t.Error(err)
		}
	}()
	if w, h := g.Layout(1, 1); w != 960 || h != 540 {
		t.Fatalf("layout: %d %d", w, h)
	}
	for range 3 {
		if err := g.Update(); errors.Is(err, ebiten.Termination) {
			return
		} else if err != nil {
			t.Fatal(err)
		}
	}
	t.Fatal("script did not terminate")
}
