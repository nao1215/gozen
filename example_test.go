package gozen_test

import (
	"context"
	"errors"
	"fmt"
	"log"
	"maps"
	"testing/fstest"

	"github.com/nao1215/gozen"
)

// trail is a small original game: a camp, a ridge with a choice, and a field of motes.
var trail = fstest.MapFS{
	"game.yaml": {Data: []byte(`schema_version: 2
id: tiny-trail
start: {location: camp, party: [walker]}
`)},
	"assets/mote.png": {Data: []byte("sentinel")},
	"data/world.yaml": {Data: []byte(`classes:
  walker:
    stats: {mhp: "20 + level * 5", mmp: 0, atk: "4 + level", def: 2, mat: 0, mdf: 0, agi: 4, luk: 1}
    exp_for: level * 20
actors:
  walker: {name: Walker, class: walker}
  sela: {name: Sela, class: walker}
skills:
  attack: {effects: [{damage: a.atk}]}
  guard: {scope: self}
enemies:
  mote: {image: mote.png, stats: {mhp: 6, mmp: 0, atk: 1, def: 0, mat: 0, mdf: 0, agi: 1, luk: 1}, gold: 3}
troops: {motes: {members: [mote, mote]}}
locations:
  camp:
    name: Camp
    on_enter: [{say: A new trail begins here.}, {gold: 2}]
    exits: [{to: ridge, label: Climb}]
  ridge:
    name: Ridge
    exits: [{to: camp, label: Back}]
events:
  lantern:
    do:
      - choice:
          - label: Light it
            do: [{set: {flag.lit: true}}, {say: The lantern answers.}]
          - label: Walk on
            do: [{battle: motes, on_win: [{gold: 5}]}]
`)},
}

// A game is three calls: Load, New and Begin. Read the state with expressions.
func Example() {
	if err := tinyGame(); err != nil {
		log.Print(err)
	}
	// Output: location=camp gold=2
}

func tinyGame() error {
	g, err := gozen.Load(trail, ".")
	if err != nil {
		return err // every problem with file, line and path (errors.As gives the Diagnostics)
	}
	s, err := gozen.New(g, gozen.Config{Seed: 1, Dev: true})
	if err != nil {
		return err
	}
	if err := s.Begin(context.Background()); err != nil { // the start and the arrival at camp
		return err
	}
	var loc string
	var gold int
	if err := errors.Join(s.Query("location", &loc), s.Query("gold", &gold)); err != nil {
		return err
	}
	_, err = fmt.Printf("location=%s gold=%d\n", loc, gold)
	return err
}

// A test without a screen: set up a state with YAML commands, then try every branch of
// an event and read each ending with Query.
func ExampleExplore() {
	if err := everyBranch(context.Background()); err != nil {
		log.Print(err)
	}
	// Output:
	// answers=[0] lit=true gold=100 at ridge
	// answers=[1 1] lit=false gold=105 at ridge
	// answers=[1 2] lit=false gold=100 at ridge
	// answers=[1 3] lit=false gold=100 at ridge
}

func everyBranch(ctx context.Context) error {
	g, err := gozen.Load(trail, ".")
	if err != nil {
		return err
	}
	s, err := gozen.New(g, gozen.Config{Seed: 3, Dev: true}) // Dev: a test may set the level
	if err != nil {
		return err
	}
	if err := s.Do(ctx, `
- join: sela
- set: {actor.walker.level: 5, gold: 100}
- pick: Climb
`); err != nil {
		return err
	}
	lantern, err := g.YAML("events.lantern.do")
	if err != nil {
		return err
	}
	paths, err := gozen.Explore(ctx, s, string(lantern), gozen.ExploreOptions{})
	if err != nil {
		return err
	}
	for _, p := range paths {
		var lit bool
		var gold int
		var where string
		if err := errors.Join(append(p.Errs, p.End.Query("flag.lit", &lit), p.End.Query("gold", &gold), p.End.Query("location", &where))...); err != nil {
			return err
		}
		if _, err := fmt.Printf("answers=%v lit=%t gold=%d at %s\n", p.Answers, lit, gold, where); err != nil {
			return err
		}
	}
	return nil
}

// An agent writes YAML, then checks it: Validate for the files, Check for the commands
// it is about to run. The diagnostics are those of gozen check --format json.
func ExampleValidate() {
	if err := agentCheck(); err != nil {
		log.Print(err)
	}
	// Output:
	// data/more.yaml:4:7 events.ambush.do[0].battle: unknown troop "wolves" (error)
	// <script>:2 [1].sya: unknown field "sya" (did you mean "say"?)
}

func agentCheck() error {
	broken := fstest.MapFS{"data/more.yaml": {Data: []byte("events:\n  ambush:\n    do:\n    - battle: wolves\n")}}
	maps.Copy(broken, trail)
	for _, d := range gozen.Validate(broken, ".") {
		if _, err := fmt.Printf("%s:%d:%d %s: %s (%s)\n", d.File, d.Line, d.Col, d.Path, d.Message, d.Severity); err != nil {
			return err
		}
	}
	g, err := gozen.Load(trail, ".")
	if err != nil {
		return err
	}
	for _, d := range g.Check("- say: hi\n- sya: hello\n") {
		if _, err := fmt.Printf("%s:%d %s: %s\n", d.File, d.Line, d.Path, d.Message); err != nil {
			return err
		}
	}
	return nil
}
