package session_test

import (
	"slices"
	"strings"
	"testing"
	"testing/fstest"

	gozen "github.com/nao1215/gozen/internal/session"
)

// more is a definition layered on trail (Load's extra): a shop, a small dungeon, and an event with pictures.
var more = fstest.MapFS{"data/zz_more.yaml": {Data: []byte(`items:
  herb: {name: Herb, price: 10, use: {where: [menu, battle], scope: one_ally, effects: [{heal: 5}]}}
shops:
  stall: {goods: [herb]}
locations:
  cave:
    name: Cave
    dungeon:
      map: |
        #####
        #S..#
        ###G#
        #####
      tiles:
        S: {start: true, facing: east}
        G: {name: Goal, do: [{call: found}]}
events:
  found: {do: [{set: {var.found: "var.found + 1"}}]}
  show:
    do:
      - image: {file: mote.png, faces: true}
      - say: {who: walker, text: Look.}
      - move: ridge
      - say: Done.
`)}}

func loadMore(t *testing.T) *gozen.Game {
	t.Helper()
	g, err := gozen.Load(trail, ".", more)
	if err != nil {
		t.Fatal(err)
	}
	return g
}

func TestLoadExtraFiles(t *testing.T) {
	g := loadMore(t)
	if !slices.Contains(g.IDs("shops"), "stall") || !slices.Contains(g.IDs("locations"), "camp") {
		t.Fatalf("the extra files did not join the game: shops %v", g.IDs("shops"))
	}
	over := fstest.MapFS{"game.yaml": {Data: []byte("schema_version: 2\nid: tiny-trail\nstart: {location: ridge, party: [walker]}\n")}}
	g2, err := gozen.Load(trail, ".", over)
	if err != nil {
		t.Fatal(err)
	}
	if got := query[string](t, session(t, g2, gozen.Config{Seed: 1}), "location"); got != "ridge" {
		t.Errorf("a replaced game.yaml starts at %s", got)
	}
}

func TestBuySellAndRan(t *testing.T) {
	s := session(t, loadMore(t), gozen.Config{Seed: 1, Dev: true})
	if err := s.Do(t.Context(), "- gold: 30\n- buy: {shop: stall, item: herb, count: 3}\n- sell: {shop: stall, item: herb}\n"); err != nil {
		t.Fatal(err)
	}
	if gold, n := query[int](t, s, "gold"), query[int](t, s, "item.herb"); gold != 2+30-30+5 || n != 2 {
		t.Errorf("gold %d herbs %d (want 7 and 2)", gold, n)
	}
	if err := s.Do(t.Context(), "- buy: {shop: stall, item: herb, count: 9}\n"); err == nil {
		t.Error("buying more than the gold pays for did not fail")
	}
	if err := s.Do(t.Context(), "- call: found\n- call: show\n- call: found\n"); err != nil {
		t.Fatal(err)
	}
	if c, o, so := query[int](t, s, "ran.found.count"), query[int](t, s, "ran.found.order"), query[int](t, s, "ran.show.order"); c != 2 || o <= so || so == 0 {
		t.Errorf("ran.found %d/%d, ran.show.order %d", c, o, so)
	}
	if n := query[int](t, s, "ran.lantern.count"); n != 0 {
		t.Errorf("an event never run counts %d", n)
	}
}

func TestDungeonNames(t *testing.T) {
	s := session(t, loadMore(t), gozen.Config{Seed: 1, Dev: true})
	if err := s.Do(t.Context(), "- set: {location: cave}\n- step: 1\n"); err != nil {
		t.Fatal(err)
	}
	if !query[bool](t, s, "moved") || query[int](t, s, "pos.x") != 2 {
		t.Fatalf("the step did not move: x %d", query[int](t, s, "pos.x"))
	}
	if err := s.Do(t.Context(), "- turn: left\n- step: 1\n"); err != nil {
		t.Fatal(err)
	}
	if query[bool](t, s, "moved") {
		t.Error("a step into the wall moved")
	}
	if err := s.Do(t.Context(), "- set: {pos.x: 3, pos.y: 1, pos.facing: south, steps: 40}\n"); err != nil {
		t.Fatal(err)
	}
	if x, f, st := query[int](t, s, "pos.x"), query[string](t, s, "pos.facing"), query[int](t, s, "steps"); x != 3 || f != "south" || st != 40 {
		t.Errorf("set pos: x %d facing %s steps %d", x, f, st)
	}
	if raw, shown := query[string](t, s, "cell(1, 1).raw"), query[bool](t, s, "cell(1, 1).shown"); raw != "S" || !shown {
		t.Errorf("cell(1, 1): raw %q shown %v", raw, shown)
	}
	if query[bool](t, s, "completed") {
		t.Error("the floor is completed before walking it")
	}
}

func TestPositionNeedsDev(t *testing.T) {
	s := session(t, loadMore(t), gozen.Config{Seed: 1})
	if err := s.Do(t.Context(), "- set: {steps: 3}\n"); err == nil {
		t.Error("set steps without Dev passed")
	}
}

func TestPathOrderAndFaces(t *testing.T) {
	g := loadMore(t)
	s := session(t, g, gozen.Config{Seed: 1})
	show, err := g.YAML("events.show.do")
	if err != nil {
		t.Fatal(err)
	}
	paths, err := gozen.Explore(t.Context(), s, string(show), gozen.ExploreOptions{NoArrive: true})
	if err != nil || len(paths) != 1 {
		t.Fatal(err, len(paths))
	}
	p := paths[0]
	if len(p.Effects) != 2 || !p.Effects[0].Faces || p.Effects[0].At != 0 || p.Effects[1].At != 1 {
		t.Errorf("effects %+v", p.Effects)
	}
	if len(p.Requests) != 2 || p.Requests[0].ID != "walker" || p.Requests[1].ID != "" {
		t.Errorf("requests %+v", p.Requests)
	}
}

func TestPreferAndTactics(t *testing.T) {
	g := loadMore(t)
	s := session(t, g, gozen.Config{Seed: 1, Policy: gozen.DefaultPolicy{Prefer: []string{"Walk"}}})
	if err := s.Do(t.Context(), "- call: lantern\n"); err != nil {
		t.Fatal(err)
	}
	if query[bool](t, s, "flag.lit") {
		t.Error("Prefer Walk took Light it")
	}
	if err := s.Do(t.Context(), "- set: {actor.walker.tactics: {mode: manual, rows: [{enabled: true, condition: always, action: guard}]}}\n"); err != nil {
		t.Fatal(err)
	}
	if m, a := query[string](t, s, "actor.walker.tactics.mode"), query[string](t, s, "actor.walker.tactics.rows[0].action"); m != "manual" || a != "guard" {
		t.Errorf("tactics %s %s", m, a)
	}
}

func TestActTakesTheMove(t *testing.T) {
	var walker int
	s := session(t, loadMore(t), gozen.Config{Seed: 1, Dev: true, OnRequest: func(q gozen.Request) {
		if q.Kind == "decision" && q.Who == "walker" {
			walker++
		}
	}})
	if err := s.Do(t.Context(), "- act: {actor: walker, skill: guard}\n- battle: motes\n"); err != nil {
		t.Fatal(err)
	}
	if query[string](t, s, "result") != "win" {
		t.Fatalf("result %s", query[string](t, s, "result"))
	}
	walker = 0
	acts := strings.Repeat("- act: {actor: walker, skill: attack, target: mote}\n", 40)
	err := s.Do(t.Context(), acts+"- battle: motes\n")
	if err == nil || !strings.Contains(err.Error(), "were not taken") {
		t.Errorf("40 moves in a short battle: %v", err)
	}
	if walker != 0 {
		t.Errorf("the AI chose %d moves for walker while act gave them", walker)
	}
	if err := s.Do(t.Context(), "- act: {actor: walker, skill: nope}\n"); err == nil {
		t.Error("an unknown skill passed the check")
	}
}
