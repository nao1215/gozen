package trial

import (
	"slices"
	"strings"
	"testing"
	"testing/fstest"
)

// loopGame counts up without an end, uses engine variables and a computed dive floor.
func loopGame() fstest.MapFS {
	return fstest.MapFS{
		"assets/x.png":           {Data: []byte(pic)},
		"assets/faces/guide.png": {Data: []byte(pic)},
		"game.yaml": {Data: []byte(`schema_version: 1
id: loop
title: Loop
start: {location: camp, party: [scout]}
battle: {variance: 0, critical: 0, escape: 1}
asides:
  - {face: faces/guide*, call: guide_aside}
  - {actor: scout, when: "var.n > 100", call: far_aside}
`)},
		"data/a.yaml": {Data: []byte(`classes:
  walker: {name: Walker, stats: {mhp: 30, mmp: 5, atk: 6, def: 2, mat: 1, mdf: 1, agi: 5, luk: 1}, exp_for: level * 10}
actors: {scout: {name: Scout, class: walker}}
skills:
  attack: {name: Tap, effects: [{damage: 6, variance: 0}]}
  guard: {name: Wait, scope: self}
locations:
  camp:
    name: Camp
    people:
      - name: Guide
        talk:
          - say: {text: Hi, who: Guide, face: faces/guide.png}
          - set: {var.n: var.n + 1}
          - set: {var.deepest: 99, var.gauge: 5, var.kill_mote: 2, var.mystery: 1, var.step: 2}
          - choice:
              - {label: Never, when: "var.step < 0", do: [{say: unreachable}]}
          - if: "var.step == 2"
            then: [{say: Always two.}]
            else: [{say: Not two.}]
          - if: "var.n > 100"
            then: [{say: Far along.}]
          - call: tail
    actions:
      - {label: Descend, do: [{set: {var.target: 4}}, {dive: tower, floor: var.target}]}
      - {label: Low, do: [{dive: tower, floor: 2}]}
      - {label: Plain, do: [{dive: tower}]}
      - {label: Deep, do: [{dive: tower, floor: var.deepest + 1}]}
      - {label: Cutting, do: [{call: endgame}, {say: after}]}
  tower:
    name: Tower
    dungeon:
      generate:
        width: 11
        height: 11
        start: S
        place:
          - {char: '>', far: true}
          - {char: K, count: "var.keys"}
      tiles:
        S: {start: true}
        '>': {do: [{dive: tower, floor: var.floor + 1}]}
        K: {do: [{say: key}]}
events:
  tail: {do: [{say: tail}]}
  endgame: {do: [{say: bye}]}
  guide_aside: {do: [{say: Aside}]}
  far_aside: {do: [{say: Far}]}
`)},
	}
}

func TestBuildWidensCountersAndGuardsComputedDives(t *testing.T) {
	cfg, err := Parse([]byte(`
assume: {var.keys: 0, var.step: 2}
cut: [endgame]
unknown: [var.mystery]
dive_max: {tower: 3}
blocked: [{say: Enough.}]
`))
	if err != nil {
		t.Fatal(err)
	}
	res, err := Build(loopGame(), cfg)
	if err != nil {
		t.Fatal(err)
	}
	if err := res.Check(); err != nil {
		t.Fatal(err)
	}
	world := string(res.FS()["data/a.yaml"].Data)
	// The counter never settles: it widens to every number, so "n > 100" may hold.
	if !strings.Contains(world, "Far along.") || !slices.Contains(res.Kept["events"], "far_aside") {
		t.Errorf("a counter that grows without a bound can pass 100:\n%s", world)
	}
	if strings.Contains(world, "unreachable") {
		t.Errorf("a choice whose only option cannot be offered must go with the choice:\n%s", world)
	}
	// Engine variables and the ones declared unknown are not analysed (their values are not decided by commands).
	for _, k := range []string{"var.deepest", "var.gauge", "var.kill_mote", "var.mystery"} {
		if _, ok := res.Domains[k]; ok {
			t.Errorf("%s must not be analysed", k)
		}
	}
	if _, ok := res.Domains["var.n"]; !ok {
		t.Errorf("domains: %v", res.Domains)
	}
	// "var.step" is assumed to be 2, so the else branch of an exact test goes away.
	if strings.Contains(world, "Not two.") || !strings.Contains(world, "Always two.") {
		t.Errorf("an if that always holds keeps only its then:\n%s", world)
	}
	// Dives: a literal floor within the limit is not guarded, any other is, and
	// a guard goes right after the command that sets the variable the floor reads.
	if strings.Count(world, "> 3") != 3 {
		t.Errorf("want guards on the 3 computed dives (var.target, var.deepest + 1 and var.floor + 1):\n%s", world)
	}
	if strings.Contains(world, "(2) > 3") || strings.Contains(world, "(1) > 3") {
		t.Errorf("a floor that is within the limit needs no guard:\n%s", world)
	}
	idx := func(s string) int { return strings.Index(world, s) }
	if idx("var.target: 4") > idx("(var.target) > 3") {
		t.Errorf("the guard must come after the set of the floor variable:\n%s", world)
	}
	if strings.Contains(world, "call: endgame") || strings.Count(world, "Enough.") < 2 {
		t.Errorf("a cut call becomes the blocked commands:\n%s", world)
	}
	// An assumed floor makes the floor expression exact.
	cfg2, err := Parse([]byte("assume: {var.keys: 0, var.floor: 2}\ndive_max: {tower: 3}\nblocked: [{say: Enough.}]\n"))
	if err != nil {
		t.Fatal(err)
	}
	res2, err := Build(loopGame(), cfg2)
	if err != nil {
		t.Fatal(err)
	}
	if got := string(res2.FS()["data/a.yaml"].Data); strings.Contains(got, "(var.floor + 1) > 3") {
		t.Errorf("with the floor assumed to be 2 the stairs stay within 3 and need no guard:\n%s", got)
	}
}

func TestBuildAsidesFollowTheSpeakersFace(t *testing.T) {
	cfg, err := Parse([]byte("assume: {var.keys: 0}\n"))
	if err != nil {
		t.Fatal(err)
	}
	res, err := Build(loopGame(), cfg)
	if err != nil {
		t.Fatal(err)
	}
	game := string(res.FS()["game.yaml"].Data)
	if !strings.Contains(game, "guide_aside") || !slices.Contains(res.Kept["events"], "guide_aside") {
		t.Errorf("an aside on a face a kept line shows is kept:\n%s", game)
	}
}
