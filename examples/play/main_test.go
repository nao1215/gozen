package main

import (
	"testing"
	"testing/fstest"

	"golang.org/x/image/font/gofont/goregular"

	"github.com/nao1215/gozen"
	"github.com/nao1215/gozen/gfx"
	"github.com/nao1215/gozen/play"
	data "github.com/nao1215/gozen/schema"
)

func TestExampleLoadsThroughPublicAPI(t *testing.T) {
	fsys, err := gameFS()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := data.Load(fsys, "."); err != nil {
		t.Fatal(err)
	}
	font, err := gfx.Load(fstest.MapFS{"regular.ttf": {Data: goregular.TTF}}, "regular.ttf")
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
	g, err := gozen.Load(fsys, ".")
	if err != nil {
		t.Fatal(err)
	}
	s, err := gozen.New(g, gozen.Config{Seed: 1})
	if err != nil {
		t.Fatal(err)
	}
	sc, err := play.New(s, play.Config{Mute: true, Store: func(string) (gozen.Store, error) { return gozen.MemStore(), nil }})
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := sc.Close(); err != nil {
			t.Error(err)
		}
	}()
	if w, h := sc.Layout(1, 1); w != 960 || h != 540 {
		t.Fatalf("layout: %d %d", w, h)
	}
	for range 3 {
		if err := sc.Update(); err != nil {
			t.Fatal(err)
		}
	}
	var loc string
	if err := s.Query("location", &loc); err != nil || loc != "camp" {
		t.Fatalf("the screen does not play the session: %q %v", loc, err)
	}
}
