package gozen_test

import (
	"context"
	"errors"
	"slices"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/nao1215/gozen"
	"github.com/nao1215/gozen/internal/testgame"
)

const moreSession = `
enemies:
  wall:
    name: Wall
    image: x.png
    stats: {mhp: 99999, mmp: 0, atk: 1, def: 99999, mat: 1, mdf: 1, agi: 1, luk: 1}
    ai: [{use: guard}]
  brute:
    name: Brute
    image: x.png
    stats: {mhp: 600, mmp: 0, atk: 400, def: 30, mat: 1, mdf: 1, agi: 99, luk: 1}
    ai: [{use: attack}]
troops:
  wall: {members: [wall]}
  brute: {members: [brute]}
events:
  slides:
    do: [{image: {files: [x.png, y.png], auto: 2}}, {say: after}]
  flashy:
    do: [{effect: flash}, {sfx: ding}]
  failing:
    do: [{set: {var.bad: "len(gold)"}}, {say: still going}]
  waking:
    trigger: auto
    once: true
    when: "flag.go"
    do: [{set: {flag.woke: true}}]
  pick:
    do: [{choice: [{label: First, do: [{set: {var.pick: 1}}]}, {label: Second, do: [{set: {var.pick: 2}}]}]}]
`

func moreSessionGame(t *testing.T, extra map[string]string) *gozen.Game {
	t.Helper()
	fsys := fstest.MapFS{
		"game.yaml":      {Data: []byte("schema_version: 4\nid: more\n" + testgame.Game)},
		"data/main.yaml": {Data: []byte(testgame.Data)},
		"data/more.yaml": {Data: []byte(moreSession)},
		"assets/x.png":   {Data: []byte("a stub")},
		"assets/y.png":   {Data: []byte("a stub")},
	}
	for k, v := range extra {
		fsys[k] = &fstest.MapFile{Data: []byte(v)}
	}
	g, err := gozen.Load(fsys, ".")
	if err != nil {
		t.Fatal(err)
	}
	return g
}

// TestAnAutoBattleItCannotWinIsLost: a battle fought on Auto that the party cannot win is a loss, and one that never ends is a loss too;
// the party that lost is raised, not left dead.
func TestAnAutoBattleItCannotWinIsLost(t *testing.T) {
	g := moreSessionGame(t, nil)
	for _, troop := range []string{"brute", "wall"} {
		var steps int
		s := mustNew(t, g, gozen.Config{Seed: 1, OnRequest: func(q gozen.Request) {
			if q.Kind == "step" {
				steps++
			}
		}})
		err := s.Do(t.Context(), "- battle: "+troop+"\n")
		if err != nil {
			t.Fatalf("%s: %v", troop, err)
		}
		var result string
		if err := s.Query("result", &result); err != nil || result != "lose" {
			t.Errorf("%s: result %q %v", troop, result, err)
		}
		var over bool
		if err := s.Query("over", &over); err != nil || !over {
			t.Errorf("%s: a lost battle with no loss event is game over: %v %v", troop, over, err)
		}
		var hp int
		if err := s.Query("actor.scout.hp", &hp); err != nil || hp < 1 {
			t.Errorf("%s: scout hp %d %v", troop, hp, err)
		}
		if steps == 0 {
			t.Errorf("%s: the fight is reported line by line", troop)
		}
	}
}

func TestOnTapAndImmediateEffects(t *testing.T) {
	g := moreSessionGame(t, nil)
	var effects []string
	s := mustNew(t, g, gozen.Config{Seed: 1, OnEffect: func(e gozen.Effect) { effects = append(effects, e.Kind) }})
	if err := s.Start("- call: slides"); err != nil {
		t.Fatal(err)
	}
	var kinds []string
	for q := s.Next(); q.Kind != ""; q = s.Next() {
		kinds = append(kinds, q.Kind)
		if q.Kind == "tap" && q.Seconds != 2 {
			t.Errorf("a slideshow taps after its seconds: %+v", q)
		}
	}
	if !slices.Contains(kinds, "tap") || kinds[len(kinds)-1] != "say" {
		t.Errorf("requests %v", kinds)
	}
	if !slices.Contains(effects, "image") {
		t.Errorf("effects %v", effects)
	}
	effects = nil
	if err := s.Do(t.Context(), "- call: flashy\n"); err != nil {
		t.Fatal(err)
	}
	if !slices.Contains(effects, "visual") || !slices.Contains(effects, "sound") {
		t.Errorf("effects %v", effects)
	}
}

// TestProblemsDoNotStopThePlay: an expression that fails is reported to OnLog and joined into Do's error, and the events go on.
func TestProblemsDoNotStopThePlay(t *testing.T) {
	g := moreSessionGame(t, nil)
	var logged []error
	var said []string
	s := mustNew(t, g, gozen.Config{Seed: 1, OnLog: func(err error) { logged = append(logged, err) }, OnRequest: func(q gozen.Request) {
		if q.Kind == "say" {
			said = append(said, q.Text)
		}
	}})
	err := s.Do(t.Context(), "- call: failing\n")
	if err == nil || !strings.Contains(err.Error(), "len") {
		t.Errorf("Do returns the problem: %v", err)
	}
	if len(logged) != 1 || !strings.Contains(logged[0].Error(), "len") {
		t.Errorf("OnLog saw %v", logged)
	}
	if !slices.Equal(said, []string{"still going"}) {
		t.Errorf("the events went on: %v", said)
	}
}

func TestAutoEventsStartOnTheirOwn(t *testing.T) {
	g := moreSessionGame(t, nil)
	s := mustNew(t, g, gozen.Config{Seed: 1})
	if err := s.Do(t.Context(), "- set: {flag.go: true}\n"); err != nil {
		t.Fatal(err)
	}
	var woke bool
	if err := s.Query("flag.woke", &woke); err != nil || !woke {
		t.Errorf("an auto event whose condition came true ran: %v %v", woke, err)
	}
	// stepping by hand: Next starts it too
	s2 := mustNew(t, g, gozen.Config{Seed: 1})
	if err := s2.Start("- set: {flag.go: true}\n"); err != nil {
		t.Fatal(err)
	}
	for q := s2.Next(); q.Kind != ""; q = s2.Next() {
		t.Fatalf("nothing to show: %+v", q)
	}
	if err := s2.Query("flag.woke", &woke); err != nil || !woke {
		t.Errorf("Next runs the auto event: %v %v", woke, err)
	}
}

// branchFirst answers every question with Branch, the answer that means "try them all" in Explore.
type branchFirst struct{ gozen.DefaultPolicy }

func (branchFirst) Decide(context.Context, *gozen.Session, gozen.Request) int { return gozen.Branch }

func TestBranchInDoIsTheFirstOption(t *testing.T) {
	g := moreSessionGame(t, nil)
	s := mustNew(t, g, gozen.Config{Seed: 1, Policy: branchFirst{}})
	if err := s.Do(t.Context(), "- call: pick\n"); err != nil {
		t.Fatal(err)
	}
	var n int
	if err := s.Query("var.pick", &n); err != nil || n != 1 {
		t.Errorf("Branch in Do is the first option: %d %v", n, err)
	}
}

func TestEventsThatNeverEndAreCutOffInDo(t *testing.T) {
	g := moreSessionGame(t, map[string]string{"data/forever.yaml": "events: {forever: {trigger: auto, do: [{gold: 0}]}}\n"})
	s := mustNew(t, g, gozen.Config{Seed: 1})
	err := s.Do(t.Context(), "- gold: 1\n")
	if err == nil || !strings.Contains(err.Error(), "do not end") {
		t.Errorf("an auto event that keeps holding: %v", err)
	}
	s2 := mustNew(t, g, gozen.Config{Seed: 1})
	if err := s2.Start("- gold: 1\n"); err != nil {
		t.Fatal(err)
	}
	for range 3 {
		if q := s2.Next(); q.Kind != "" {
			t.Fatalf("%+v", q)
		}
	}
}

func TestExploreTheEffectsOfAPath(t *testing.T) {
	g := moreSessionGame(t, map[string]string{"data/tour.yaml": "events: {tour: {do: [{music: {song: theme}}, {say: hi}, {image: x.png}, {move: field}]}}\nmusic: {songs: {theme: {file: x.png}}}\n"})
	s := mustNew(t, g, gozen.Config{Seed: 1})
	paths, err := gozen.Explore(t.Context(), s, "- call: tour\n", gozen.ExploreOptions{NoArrive: true})
	if err != nil || len(paths) != 1 {
		t.Fatal(err, len(paths))
	}
	var kinds []string
	for _, e := range paths[0].Effects {
		kinds = append(kinds, e.Kind+":"+e.Name)
	}
	want := []string{"music:theme", "image:x.png", "move:field"}
	if !slices.Equal(kinds, want) {
		t.Errorf("effects %v, want %v", kinds, want)
	}
	var loc string
	if err := paths[0].End.Query("location", &loc); err != nil || loc != "field" {
		t.Errorf("the end session stands in %q (%v)", loc, err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := gozen.Explore(ctx, s, "- call: tour\n", gozen.ExploreOptions{}); !errors.Is(err, context.Canceled) {
		t.Errorf("a cancelled context: %v", err)
	}
}

// cancelAtChoice cancels the context the moment a choice is put to it.
type cancelAtChoice struct {
	gozen.DefaultPolicy
	cancel context.CancelFunc
}

func (p cancelAtChoice) Decide(ctx context.Context, s *gozen.Session, q gozen.Request) int {
	if q.Kind == "choice" {
		p.cancel()
	}
	return p.DefaultPolicy.Decide(ctx, s, q)
}

// TestCancelledMidwayLeavesAUsableSession: a context cancelled while commands run ends Do with the context's error;
// the session can be saved, restored and used again with a fresh context.
func TestCancelledMidwayLeavesAUsableSession(t *testing.T) {
	g := moreSessionGame(t, nil)
	ctx, cancel := context.WithCancel(t.Context())
	s := mustNew(t, g, gozen.Config{Seed: 1, Policy: cancelAtChoice{cancel: cancel}})
	err := s.Do(ctx, "- call: pick\n- gold: 50\n")
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("want context.Canceled, got %v", err)
	}
	if got := query[int](t, s, "gold"); got != 100 {
		t.Errorf("commands after the cancel ran: gold %d", got)
	}
	b, err := s.Save()
	if err != nil {
		t.Fatalf("a session cancelled midway cannot be saved: %v", err)
	}
	r, err := gozen.Restore(g, b, gozen.Config{Seed: 1})
	if err != nil {
		t.Fatalf("restore: %v", err)
	}
	if err := r.Do(t.Context(), "- gold: 50\n"); err != nil {
		t.Fatal(err)
	}
	if got := query[int](t, r, "gold"); got != 150 {
		t.Errorf("gold %d", got)
	}
}
