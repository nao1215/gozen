// Command headless demonstrates a complete in-memory game and save round trip.
package main

import (
	"bytes"
	"context"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"io"
	"math/rand/v2"
	"os"
	"testing/fstest"

	"github.com/nao1215/gozen/data"
	"github.com/nao1215/gozen/headless"
	"github.com/nao1215/gozen/rpg"
	"github.com/nao1215/gozen/savefile"
)

const game = `schema_version: 1
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
	db, err := data.Load(fsys, ".")
	if err != nil {
		return err
	}
	rng := rand.New(rand.NewPCG(4, 9)) //nolint:gosec // Reproducible game simulation.
	state := rpg.New(db, rng)
	driver := headless.New(db, state)
	if err := driver.Start(); err != nil {
		return err
	}
	if err := driver.Run(context.Background(), []data.Command{{Move: "trail"}}); err != nil {
		return err
	}
	encoded, err := state.Save()
	if err != nil {
		return err
	}
	// The host chooses the save storage. No user directory is touched by this example.
	saves := fstest.MapFS{"slot.json": {Data: encoded}}
	encoded, err = savefile.Load(saves, "slot.json")
	if err != nil {
		return err
	}
	restored, err := rpg.Parse(db, encoded, nil)
	if err != nil {
		return err
	}
	driver.Restore(restored)
	_, err = fmt.Fprintf(out, "location=%s battles=%d wins=%d gold=%d\n", restored.Location, driver.Battles, driver.Wins, restored.Gold)
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
