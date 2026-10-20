package main

import (
	"context"
	"slices"
	"strings"
	"testing"

	"github.com/nao1215/gozen"
)

// journey is a session of Wayfinder read through expressions and moved by YAML commands.
type journey struct {
	t       *testing.T
	g       *gozen.Game
	s       *gozen.Session
	cfg     gozen.Config
	lines   []string
	battles int
}

func (j *journey) do(script string) error { return j.s.Do(context.Background(), script) }

func (j *journey) must(script string) {
	j.t.Helper()
	if err := j.do(script); err != nil {
		j.t.Fatal(err)
	}
}

func (j *journey) has(label string) bool {
	j.t.Helper()
	var menu []struct{ Label string }
	if err := j.s.Query("menu", &menu); err != nil {
		j.t.Fatal(err)
	}
	return slices.ContainsFunc(menu, func(e struct{ Label string }) bool { return strings.Contains(e.Label, label) })
}

func (j *journey) flag(name string) bool { return query[bool](j.t, j.s, "flag."+name) }
func (j *journey) gold() int             { return query[int](j.t, j.s, "gold") }
func (j *journey) at() string            { return query[string](j.t, j.s, "location") }

func query[T any](t *testing.T, s *gozen.Session, src string) T {
	t.Helper()
	var v T
	if err := s.Query(src, &v); err != nil {
		t.Fatal(err)
	}
	return v
}

// reload saves the session into a store and goes on from what was read back.
func (j *journey) reload(store gozen.Store) {
	j.t.Helper()
	b, err := j.s.Save()
	if err != nil {
		j.t.Fatal(err)
	}
	if err := store.Write("slot", b); err != nil {
		j.t.Fatal(err)
	}
	if b, err = store.Read("slot"); err != nil {
		j.t.Fatal(err)
	}
	if j.s, err = gozen.Restore(j.g, b, j.cfg); err != nil {
		j.t.Fatal(err)
	}
}

func TestJourneyAndSaveRoundTrip(t *testing.T) {
	fsys, err := gameFS()
	if err != nil {
		t.Fatal(err)
	}
	g, err := gozen.Load(fsys, ".")
	if err != nil {
		t.Fatal(err)
	}
	j := &journey{t: t, g: g}
	j.cfg = gozen.Config{Seed: 7, OnRequest: func(q gozen.Request) {
		switch q.Kind {
		case "say":
			j.lines = append(j.lines, q.Text)
		case "battle":
			j.battles++
		}
	}}
	if j.s, err = gozen.New(g, j.cfg); err != nil {
		t.Fatal(err)
	}
	if err := j.s.Begin(context.Background()); err != nil {
		t.Fatal(err)
	}
	if j.has("Walk to the ridge") || j.has("Light the marker") {
		t.Fatal("progression opened before practice")
	}
	j.must("- pick: Courier\n")
	if !j.flag("briefed") || !strings.Contains(strings.Join(j.lines, "\n"), "A little light") {
		t.Fatal("conversation did not run")
	}
	j.must("- pick: Practice\n")
	if j.battles != 1 || query[string](t, j.s, "result") != "win" || !j.flag("trained") || j.gold() != 3 {
		t.Fatalf("practice: battles=%d gold=%d", j.battles, j.gold())
	}
	j.must("- pick: Light the marker\n")
	if j.has("Light the marker") || !j.has("Walk to the ridge") {
		t.Fatal("lighting the marker did not change available actions")
	}
	j.must("- pick: Walk to the ridge\n")
	if !j.flag("complete") || j.gold() != 7 {
		t.Fatalf("arrival reward: gold=%d", j.gold())
	}
	type member struct{ ID string }
	if got := query[[]member](t, j.s, "party.members"); len(got) != 2 || got[1].ID != "sela" {
		t.Fatalf("Sela did not join at the ridge: party=%v", got)
	}
	// The same versioned save bytes as the screen, in isolated disk storage.
	j.reload(gozen.DirStore(t.TempDir()))
	if j.at() != "ridge" || !j.flag("complete") || j.gold() != 7 || query[int](t, j.s, "members") != 2 {
		t.Fatalf("restored progress: location=%s gold=%d", j.at(), j.gold())
	}
	j.must("- pick: Return to camp\n- pick: Visit the village\n- pick: Bram\n")
	if got := query[[]member](t, j.s, "party.members"); len(got) != 3 || got[2].ID != "bram" || j.has("Bram") {
		t.Fatalf("Bram did not join in the village: party=%v", got)
	}
	j.must("- pick: Return to camp\n- pick: Explore chamber\n")
	if j.gold() != 7 {
		t.Fatal("arrival reward repeated after loading")
	}
	// The halls: a chest in a dead end, a sentry fight, a wheel that opens the gate to the lantern.
	j.must("- walk: T\n")
	if !j.flag("chest_opened") || j.gold() != 12 {
		t.Fatalf("chest: gold=%d", j.gold())
	}
	if err := j.do("- walk: L\n"); err == nil {
		t.Fatal("the lantern was reachable before the sentries and the gate")
	}
	battles := j.battles
	j.must("- walk: P\n")
	if j.battles != battles+1 || !j.flag("sentries_passed") || j.gold() != 18 {
		t.Fatalf("sentries: battles=%d gold=%d", j.battles, j.gold())
	}
	if err := j.do("- walk: L\n"); err == nil {
		t.Fatal("the lantern was reachable before the wheel")
	}
	j.must("- walk: K\n")
	if !j.flag("gate_open") {
		t.Fatal("the wheel did not open the gate")
	}
	// Saving in the halls keeps the spent chest, the passed sentries and the open gate.
	j.reload(gozen.MemStore())
	if j.at() != "chamber" || j.gold() != 18 || !j.flag("gate_open") {
		t.Fatalf("restored halls: location=%s gold=%d", j.at(), j.gold())
	}
	if err := j.do("- walk: T\n"); err == nil {
		t.Fatal("the opened chest was found again after loading")
	}
	j.must("- walk: L\n")
	if !j.flag("window_seen") || query[int](t, j.s, "pos.x") != 13 || query[int](t, j.s, "pos.y") != 9 || j.gold() != 18 {
		t.Fatalf("halls walk did not reach the lantern: pos=%d,%d gold=%d", query[int](t, j.s, "pos.x"), query[int](t, j.s, "pos.y"), j.gold())
	}
	j.must("- walk: C\n")
	if j.at() != "camp" {
		t.Fatal("the halls have no working return route")
	}
}
