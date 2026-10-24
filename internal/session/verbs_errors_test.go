package session_test

import (
	"strings"
	"testing"

	gozen "github.com/nao1215/gozen/internal/session"
)

// TestDungeonVerbsOutsideADungeon: turn, step and walk say where they cannot work, and change nothing.
func TestDungeonVerbsOutsideADungeon(t *testing.T) {
	s := session(t, loadMore(t), gozen.Config{Seed: 1, Dev: true})
	for _, script := range []string{"- turn: left\n", "- step: 2\n", "- walk: G\n"} {
		err := s.Do(t.Context(), script)
		if err == nil || !strings.Contains(err.Error(), "camp is not a dungeon") {
			t.Errorf("%q: %v", script, err)
		}
	}
	if query[string](t, s, "location") != "camp" {
		t.Error("a refused verb moved the party")
	}
}

func TestWalkAndTurnInsideADungeon(t *testing.T) {
	s := session(t, loadMore(t), gozen.Config{Seed: 1, Dev: true})
	if err := s.Do(t.Context(), "- set: {location: cave}\n"); err != nil {
		t.Fatal(err)
	}
	// facing: east at the start; right turns to the south, back turns to the west
	for _, c := range []struct{ way, facing string }{{"right", "south"}, {"back", "north"}, {"left", "west"}, {"back", "east"}} {
		if err := s.Do(t.Context(), "- turn: "+c.way+"\n"); err != nil {
			t.Fatal(err)
		}
		if got := query[string](t, s, "pos.facing"); got != c.facing {
			t.Fatalf("turn %s: facing %s, want %s", c.way, got, c.facing)
		}
	}
	// walk to the goal tile and its commands run
	if err := s.Do(t.Context(), "- walk: G\n"); err != nil {
		t.Fatal(err)
	}
	if query[int](t, s, "var.found") != 1 {
		t.Errorf("walking onto the goal runs its do: found = %d", query[int](t, s, "var.found"))
	}
	if x, y := query[int](t, s, "pos.x"), query[int](t, s, "pos.y"); x != 3 || y != 2 {
		t.Errorf("stands at %d,%d, want 3,2", x, y)
	}
	// walking to where one already stands is nothing
	if err := s.Do(t.Context(), "- walk: G\n"); err != nil {
		t.Fatal(err)
	}
	if query[int](t, s, "var.found") != 1 {
		t.Error("walking to the cell one stands on ran its do again")
	}
	// a tile that is not on the map
	err := s.Do(t.Context(), "- walk: Q\n")
	if err == nil {
		t.Fatal("walking to nowhere must fail")
	}
}

func TestPickAndUseErrors(t *testing.T) {
	s := session(t, loadMore(t), gozen.Config{Seed: 1, Dev: true})
	err := s.Do(t.Context(), "- pick: Nowhere\n")
	if err == nil || !strings.Contains(err.Error(), `no "Nowhere"`) || !strings.Contains(err.Error(), "Climb") {
		t.Errorf("a missing menu entry says what there is: %v", err)
	}
	// a part of the label is enough when nothing matches exactly
	if err := s.Do(t.Context(), "- pick: Clim\n"); err != nil {
		t.Fatal(err)
	}
	if query[string](t, s, "location") != "ridge" {
		t.Error("a partial label was not taken")
	}
	if err := s.Do(t.Context(), "- use: herb\n"); err == nil || !strings.Contains(err.Error(), "herb") {
		t.Errorf("using an item that is not held: %v", err)
	}
	if err := s.Do(t.Context(), "- use: {item: herb, target: stranger}\n"); err == nil {
		t.Error("using an item on a stranger")
	}
	if err := s.Do(t.Context(), "- gold: 30\n- buy: {shop: stall, item: herb, count: 2}\n- set: {actor.walker.hp: 5}\n- use: herb\n"); err != nil {
		t.Fatal(err)
	}
	if n := query[int](t, s, "item.herb"); n != 1 {
		t.Errorf("one herb is used up, %d left", n)
	}
}

func TestEquipCommand(t *testing.T) {
	s := session(t, loadMore(t), gozen.Config{Seed: 1, Dev: true})
	for _, script := range []string{
		"- equip: {actor: stranger, item: herb}\n",
		"- equip: {actor: walker, item: herb}\n",
	} {
		if err := s.Do(t.Context(), script); err == nil {
			t.Errorf("%q must fail", script)
		}
	}
}
