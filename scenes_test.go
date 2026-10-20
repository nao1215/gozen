package gozen_test

import (
	"strings"
	"testing"

	"github.com/nao1215/gozen"
	"github.com/nao1215/gozen/internal/testgame"
)

// TestScenesByCommands walks the maps and the stage with pick, reads where the party
// stands with the expression spot, and keeps it through a save (ADR 0022).
func TestScenesByCommands(t *testing.T) {
	g, err := gozen.Load(testgame.ScenesFS(nil), ".")
	if err != nil {
		t.Fatal(err)
	}
	s := session(t, g, gozen.Config{Seed: 1, Dev: true})
	if err := s.Do(t.Context(), "- set: {calm: true}\n- pick: → Town\n- pick: → Plaza\n- pick: Notice board"); err != nil {
		t.Fatal(err)
	}
	if loc, spot := query[string](t, s, "location"), query[string](t, s, "spot"); loc != "plaza" || spot != "gate" {
		t.Fatalf("at %s/%s, want plaza/gate (the spot nearest the board)", loc, spot)
	}
	labels := query[[]map[string]any](t, s, "menu")
	for _, e := range labels {
		if label, ok := e["label"].(string); ok && strings.Contains(label, "Notice board") {
			t.Fatalf("a thing with once is offered again: %v", labels)
		}
	}
	if err := s.Do(t.Context(), "- set: {spot: north}"); err != nil {
		t.Fatal(err)
	}
	save, err := s.Save()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(save), `"spot": "north"`) {
		t.Fatalf("the save has no spot: %s", save)
	}
	r, err := gozen.Restore(g, save, gozen.Config{Seed: 1})
	if err != nil {
		t.Fatal(err)
	}
	if spot := query[string](t, r, "spot"); spot != "north" {
		t.Fatalf("restored spot %q", spot)
	}
	// a spot the game no longer has: back at the location's start, reported
	g2, err := gozen.Load(testgame.ScenesFS(map[string]string{"data/town.yaml": strings.Replace(testgame.ScenesData, "north: {at: [48, 12], facing: south}", "", 1)}), ".")
	if err != nil {
		t.Fatal(err)
	}
	r2, err := gozen.Restore(g2, save, gozen.Config{Seed: 1})
	if err != nil {
		t.Fatal(err)
	}
	if spot := query[string](t, r2, "spot"); spot != "" {
		t.Fatalf("an unknown spot was kept: %q", spot)
	}
	if n := query[float64](t, r2, "len(repairs)"); n == 0 {
		t.Fatal("the repair is not reported")
	}
	// set spot is for --dev and tests only
	p := session(t, g, gozen.Config{Seed: 1})
	if err := p.Do(t.Context(), "- set: {spot: north}"); err == nil {
		t.Fatal("set spot without Dev")
	}
}

// TestExploreScenes: the maps and the stage add entries, not branches.
func TestExploreScenes(t *testing.T) {
	g, err := gozen.Load(testgame.ScenesFS(nil), ".")
	if err != nil {
		t.Fatal(err)
	}
	s := session(t, g, gozen.Config{Seed: 1})
	paths, err := gozen.Explore(t.Context(), s, "- pick: → Town\n- pick: Signpost\n- pick: → Plaza\n- pick: Walker", gozen.ExploreOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if len(paths) != 1 {
		t.Fatalf("%d paths, want 1", len(paths))
	}
	var read bool
	if err := paths[0].End.Query("flag.read_sign && flag.heard && location == 'plaza'", &read); err != nil || !read {
		t.Fatalf("the path did not walk the town: %v %v", read, paths[0].Errs)
	}
}

// TestRoadBattleLost: the rules settle road by road (ADR 0022). A battle lost on the road
// to the ruins (a battle in the road's do: Explore branches on it) leaves the party where it
// set out, not at the ruins; a battle fled goes on to the ruins.
func TestRoadBattleLost(t *testing.T) {
	src := strings.Replace(testgame.ScenesData, "steps: 4, encounters: {rate: \"1\", troops: [{troop: one}]}", "steps: 4, do: [{battle: one}]", 1)
	if src == testgame.ScenesData {
		t.Fatal("the fixture's road changed")
	}
	g, err := gozen.Load(testgame.ScenesFS(map[string]string{"data/town.yaml": src}), ".")
	if err != nil {
		t.Fatal(err)
	}
	s := session(t, g, gozen.Config{Seed: 1, Dev: true})
	if err := s.Do(t.Context(), "- set: {flag.heard: true}"); err != nil {
		t.Fatal(err)
	}
	paths, err := gozen.Explore(t.Context(), s, "- pick: → Ruins", gozen.ExploreOptions{NoArrive: true})
	if err != nil {
		t.Fatal(err)
	}
	got := map[int]string{}
	for _, p := range paths {
		if len(p.Answers) != 1 {
			t.Fatalf("answers %v", p.Answers)
		}
		var loc, spot string
		if err := p.End.Query("location", &loc); err != nil {
			t.Fatal(err)
		}
		if err := p.End.Query("spot", &spot); err != nil {
			t.Fatal(err)
		}
		got[p.Answers[0]] = loc + "/" + spot
	}
	if got[gozen.Lost] != "world/" || got[gozen.Fled] != "ruins/" || got[gozen.Won] != "ruins/" {
		t.Fatalf("where each branch ends: %v", got)
	}
}
