// Command headless demonstrates a complete in-memory game and save round trip.
package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"io"
	"os"
	"testing/fstest"

	"github.com/nao1215/gozen"
)

const game = `schema_version: 2
id: trail-example
title: Trail lesson
start: {location: camp, party: [walker]}
battle: {variance: 0, critical: 0, escape: 1}
`
const definitions = `
classes:
  walker:
    name: Walker
    stats: {mhp: 24, mmp: 0, atk: 6, def: 2, mat: 0, mdf: 0, agi: 8, luk: 1}
    exp_for: level * 20
actors: {walker: {name: Walker, class: walker}}
skills:
  attack: {name: Tap, effects: [{damage: 8, variance: 0}]}
  guard: {name: Wait, scope: self}
enemies:
  pebble:
    name: Pebble
    image: pebble.png
    stats: {mhp: 8, mmp: 0, atk: 1, def: 0, mat: 0, mdf: 0, agi: 1, luk: 1}
    gold: 3
    ai: [{use: attack}]
troops: {lesson: {members: [pebble]}}
locations:
  camp:
    name: Camp
    exits: [{to: trail}]
  trail:
    name: Trail
    on_enter:
      - say: The next marker is ahead.
      - choice:
          - label: Practice
            do: [{battle: lesson}]
          - label: Rest
            do: [{heal: all}]
events: {}
`

func run(out io.Writer) error {
	// The only picture is an original colored square generated with the Go library.
	sprite := image.NewNRGBA(image.Rect(0, 0, 8, 8))
	for y := range 8 {
		for x := range 8 {
			sprite.SetNRGBA(x, y, color.NRGBA{R: 80, G: 140, B: 180, A: 255})
		}
	}
	var asset bytes.Buffer
	if err := png.Encode(&asset, sprite); err != nil {
		return err
	}
	fsys := fstest.MapFS{"game.yaml": {Data: []byte(game)}, "data/world.yaml": {Data: []byte(definitions)}, "assets/pebble.png": {Data: asset.Bytes()}}
	g, err := gozen.Load(fsys, ".")
	if err != nil {
		return err
	}
	battles, wins := 0, 0
	cfg := gozen.Config{Seed: 4, OnRequest: func(q gozen.Request) {
		if q.Kind == "battle" {
			battles++
		}
	}}
	s, err := gozen.New(g, cfg)
	if err != nil {
		return err
	}
	ctx := context.Background()
	// The default policy takes the first option (Practice) and fights the battle on AUTO.
	if err := errors.Join(s.Begin(ctx), s.Do(ctx, "- move: trail\n")); err != nil {
		return err
	}
	var result string
	if err := s.Query("result", &result); err != nil {
		return err
	}
	if result == "win" {
		wins++
	}
	encoded, err := s.Save()
	if err != nil {
		return err
	}
	// The host chooses the save storage. No user directory is touched by this example.
	store := gozen.MemStore()
	if err := store.Write("slot", encoded); err != nil {
		return err
	}
	if encoded, err = store.Read("slot"); err != nil {
		return err
	}
	restored, err := gozen.Restore(g, encoded, cfg)
	if err != nil {
		return err
	}
	var location string
	var gold int
	if err := errors.Join(restored.Query("location", &location), restored.Query("gold", &gold)); err != nil {
		return err
	}
	_, err = fmt.Fprintf(out, "location=%s battles=%d wins=%d gold=%d\n", location, battles, wins, gold)
	return err
}

func main() {
	if err := run(os.Stdout); err != nil {
		if _, writeErr := fmt.Fprintln(os.Stderr, err); writeErr != nil {
			os.Exit(2)
		}
		os.Exit(1)
	}
}
