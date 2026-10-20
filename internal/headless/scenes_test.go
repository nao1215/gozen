package headless_test

import (
	"strings"
	"testing"

	"github.com/nao1215/gozen/internal/headless"
	"github.com/nao1215/gozen/internal/testgame"
	"github.com/nao1215/gozen/internal/world"
)

func labels(d *headless.Driver) string {
	var out []string
	for _, e := range world.Entries(d.DB, d.St) {
		out = append(out, e.Label)
	}
	return strings.Join(out, " | ")
}

// TestWalkTheTown walks the maps and the stage by their menu entries (ADR 0022): the
// points of a map and the people, things and spots of a stage are entries like any other.
func TestWalkTheTown(t *testing.T) {
	db := testgame.LoadScenes(t, nil)
	st := testgame.State(db, 1)
	st.Calm = true
	d := headless.New(db, st)
	if err := d.Start(); err != nil {
		t.Fatal(err)
	}
	if st.Location != "world" {
		t.Fatalf("start at %s", st.Location)
	}
	// the ruins are closed: offered with their locked line, which says why and moves nowhere
	if got := labels(d); !strings.Contains(got, "→ Town") || !strings.Contains(got, "→ Ruins") {
		t.Fatalf("the plain's entries: %s", got)
	}
	if err := d.Do("→ Ruins"); err != nil {
		t.Fatal(err)
	}
	if st.Location != "world" || st.Spot != "" || st.Steps != 0 { // "" is the start point
		t.Fatalf("a locked point moved the party: %s/%s steps %d", st.Location, st.Spot, st.Steps)
	}
	if err := d.Do("→ Town"); err != nil {
		t.Fatal(err)
	}
	// coming from the plain, the party stands on the point leading back to it
	if st.Location != "town" || st.Spot != "out" {
		t.Fatalf("arrived at %s/%s, want town/out", st.Location, st.Spot)
	}
	got := labels(d)
	for _, want := range []string{"→ Plaza", "→ Shop", "Signpost", "→ Gate", "→ Leave town"} {
		if !strings.Contains(got, want) {
			t.Fatalf("the town's entries lack %q: %s", want, got)
		}
	}
	steps := st.Steps
	if err := d.Do("Signpost"); err != nil {
		t.Fatal(err)
	}
	// out -> square -> fork -> sign: three roads of one step
	if !st.Flags["read_sign"] || st.Spot != "sign" || st.Steps != steps+3 {
		t.Fatalf("the signpost: flag %v spot %s steps %d (was %d)", st.Flags["read_sign"], st.Spot, st.Steps, steps)
	}
	if strings.Contains(labels(d), "Signpost") {
		t.Fatalf("a point with once is offered again: %s", labels(d))
	}
	if err := d.Do("→ Plaza"); err != nil {
		t.Fatal(err)
	}
	if st.Location != "plaza" || st.Spot != "gate" {
		t.Fatalf("arrived at %s/%s, want plaza/gate (the first spot)", st.Location, st.Spot)
	}
	got = labels(d)
	for _, want := range []string{"Keeper", "Walker", "Rest", "Notice board", "Sit on the bench", "→ Shop", "→ Back to town"} {
		if !strings.Contains(got, want) {
			t.Fatalf("the plaza's entries lack %q: %s", want, got)
		}
	}
	if err := d.Do("Keeper"); err != nil {
		t.Fatal(err)
	}
	if st.Spot != "gate" { // the keeper stands at [30, 40]: the gate spot [48, 50] is nearest
		t.Fatalf("talking to the keeper stands on %s", st.Spot)
	}
	if err := d.Do("Sit on the bench"); err != nil {
		t.Fatal(err)
	}
	if st.Spot != "bench" || !st.Done["plaza/bench"] || strings.Contains(labels(d), "bench") {
		t.Fatalf("the bench: spot %s done %v entries %s", st.Spot, st.Done, labels(d))
	}
	if err := d.Do("→ Shop"); err != nil {
		t.Fatal(err)
	}
	if st.Location != "shop" || st.Spot != "" {
		t.Fatalf("in the shop: %s/%q", st.Location, st.Spot)
	}
	if err := d.Do("→ Back to the street"); err != nil {
		t.Fatal(err)
	}
	if st.Location != "town" || st.Spot != "shop" {
		t.Fatalf("back on the street at %s/%s, want town/shop", st.Location, st.Spot)
	}
	// the gate opens with the key; GoTo finds the way through the map's point
	if err := d.GoTo("gate"); err == nil {
		t.Fatal("reached the shut gate")
	}
	st.Items["key"] = 1
	if err := d.GoTo("gate"); err != nil {
		t.Fatalf("%v (entries %s)", err, labels(d))
	}
	if st.Location != "gate" {
		t.Fatalf("at %s", st.Location)
	}
}

// TestRoadEncounters meets a battle on each step of a road whose rate is 1, not when calm.
func TestRoadEncounters(t *testing.T) {
	db := testgame.LoadScenes(t, nil)
	for _, calm := range []bool{false, true} {
		st := testgame.State(db, 1)
		st.Calm = calm
		st.Flags["heard"] = true
		d := headless.New(db, st)
		if err := d.Start(); err != nil {
			t.Fatal(err)
		}
		before := d.Battles
		if err := d.Do("→ Ruins"); err != nil {
			t.Fatal(err)
		}
		fought := d.Battles - before
		if st.Location != "ruins" || st.Steps < 4 {
			t.Fatalf("calm %v: at %s after %d steps", calm, st.Location, st.Steps)
		}
		if want := map[bool]int{false: 4, true: 0}[calm]; fought != want {
			t.Fatalf("calm %v: %d battles on the road, want %d", calm, fought, want)
		}
		if st.Spot != "" {
			t.Fatalf("the ruins have no spots, the party stands on %q", st.Spot)
		}
	}
}
