package session_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"maps"
	"os"
	"reflect"
	"slices"
	"strconv"
	"strings"
	"testing"
	"testing/fstest"

	gozen "github.com/nao1215/gozen/internal/session"
	"github.com/nao1215/gozen/internal/testgame"
)

func load(t *testing.T) *gozen.Game {
	t.Helper()
	g, err := gozen.Load(trail, ".")
	if err != nil {
		t.Fatal(err)
	}
	return g
}

func session(t *testing.T, g *gozen.Game, cfg gozen.Config) *gozen.Session {
	t.Helper()
	s, err := gozen.New(g, cfg)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Begin(t.Context()); err != nil {
		t.Fatal(err)
	}
	return s
}

func query[T any](t *testing.T, s *gozen.Session, src string) T {
	t.Helper()
	var v T
	if err := s.Query(src, &v); err != nil {
		t.Fatal(err)
	}
	return v
}

func TestLoadErrors(t *testing.T) {
	bad := fstest.MapFS{"game.yaml": {Data: []byte("schema_version: 2\nid: x\nstart: {location: nowhere, party: [walker]}\n")}, "data/world.yaml": trail["data/world.yaml"]}
	_, err := gozen.Load(bad, ".")
	var ds gozen.Diagnostics
	if !errors.Is(err, gozen.ErrInvalid) || !errors.As(err, &ds) || len(ds) == 0 {
		t.Fatalf("an invalid game: %v", err)
	}
	if _, err := gozen.Load(trail, "missing"); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("a missing directory: %v", err)
	}
	if _, err := gozen.Load(nil, "."); !errors.Is(err, gozen.ErrInvalid) {
		t.Fatal(err)
	}
	if ds := gozen.Validate(trail, "."); len(ds) != 0 {
		t.Fatalf("a sound game has problems: %v", ds)
	}
}

func TestGameReads(t *testing.T) {
	g := load(t)
	if g.ID() != "tiny-trail" || strings.Join(g.IDs("locations"), ",") != "camp,ridge" || g.FS() == nil {
		t.Fatal(g.ID(), g.IDs("locations"))
	}
	b, err := g.YAML("locations.camp.exits[0].to")
	if err != nil || strings.TrimSpace(string(b)) != "ridge" {
		t.Fatalf("%q %v", b, err)
	}
	if b, err := g.YAML("start.party"); err != nil || !strings.Contains(string(b), "walker") {
		t.Fatalf("%q %v", b, err)
	}
	for _, p := range []string{"", "events.none", "locations.camp.exits[5]", "nowhere.x"} {
		if _, err := g.YAML(p); !errors.Is(err, gozen.ErrNotFound) {
			t.Errorf("%q: %v", p, err)
		}
	}
	if ds := g.Check("gold > 1 && menu[0].label == 'Climb'"); ds != nil {
		t.Fatal(ds)
	}
	if ds := g.Check("gld > 1"); len(ds) != 1 || ds[0].File != "<script>" {
		t.Fatalf("%v", ds)
	}
}

func TestDoQueryAndErrors(t *testing.T) {
	g := load(t)
	s := session(t, g, gozen.Config{Seed: 1, Dev: true})
	if err := s.Do(t.Context(), "- set: {actor.walker.level: 3, item.none: 1}\n"); !errors.Is(err, gozen.ErrInvalid) {
		t.Fatalf("a set of an unknown item: %v", err)
	}
	if err := s.Do(t.Context(), "gold: 1"); !errors.Is(err, gozen.ErrInvalid) {
		t.Fatalf("not a list: %v", err)
	}
	if err := s.Do(t.Context(), "- set: {actor.walker.level: 3}\n- join: sela\n- pick: Climb\n"); err != nil {
		t.Fatal(err)
	}
	type entry struct {
		Kind, Label string
		New         bool
	}
	menu := query[[]entry](t, s, "menu")
	if query[int](t, s, "actor.walker.level") != 3 || query[string](t, s, "location") != "ridge" || len(menu) == 0 || menu[0].Kind != "exit" || !strings.Contains(menu[0].Label, "Back") {
		t.Fatalf("%v", menu)
	}
	if query[float64](t, s, "party.size") != 2 || query[bool](t, s, "over") {
		t.Fatal("party or over")
	}
	var party struct {
		Size    int
		Members []struct {
			ID    string
			Level int
			HP    int
		}
	}
	if err := s.Query("party", &party); err != nil || party.Size != 2 || len(party.Members) != 2 || party.Members[0].ID != "walker" || party.Members[0].Level != 3 {
		t.Fatalf("an object becomes its fields: %+v %v", party, err)
	}
	var v any
	if err := s.Query("gold +", &v); !errors.Is(err, gozen.ErrInvalid) {
		t.Fatal(err)
	}
	var n int
	if err := s.Query("location", &n); err == nil {
		t.Fatal("a string into an int")
	}
}

// Start, Next and Answer drive the events from a screen; a battle answered Auto is fought.
func TestStepByStep(t *testing.T) {
	g := load(t)
	var steps int
	s := session(t, g, gozen.Config{Seed: 2, OnRequest: func(q gozen.Request) {
		if q.Kind == "step" {
			steps++
		}
	}})
	lantern, err := g.YAML("events.lantern.do")
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Start(string(lantern)); err != nil {
		t.Fatal(err)
	}
	q := s.Next()
	if q.Kind != "choice" || len(q.Options) != 2 || s.Next().Kind != "choice" {
		t.Fatalf("%+v", q)
	}
	s.Answer(1)
	if q = s.Next(); q.Kind != "battle" || q.ID != "motes" {
		t.Fatalf("%+v", q)
	}
	s.Answer(gozen.Auto)
	for q = s.Next(); q.Kind != ""; q = s.Next() {
	}
	if steps == 0 || query[string](t, s, "result") != "win" || query[int](t, s, "gold") != 2+6+5 {
		t.Fatalf("steps %d result %s gold %d", steps, query[string](t, s, "result"), query[int](t, s, "gold"))
	}
}

// policy takes the last option, and otherwise answers as the default.
type policy struct{ gozen.DefaultPolicy }

func (p policy) Decide(ctx context.Context, s *gozen.Session, q gozen.Request) int {
	if q.Kind == "choice" {
		return len(q.Options) - 1
	}
	return p.DefaultPolicy.Decide(ctx, s, q)
}

func TestPolicySaveRestoreClone(t *testing.T) {
	g := load(t)
	s := session(t, g, gozen.Config{Seed: 4, Policy: policy{}, Dev: true})
	lantern, err := g.YAML("events.lantern.do")
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Do(t.Context(), string(lantern)); err != nil {
		t.Fatal(err)
	}
	if query[bool](t, s, "flag.lit") || query[string](t, s, "result") != "win" {
		t.Fatal("the policy did not take the last option")
	}
	c, err := s.Clone()
	if err != nil {
		t.Fatal(err)
	}
	if err := c.Do(t.Context(), "- gold: 50\n"); err != nil {
		t.Fatal(err)
	}
	if query[int](t, s, "gold")+50 != query[int](t, c, "gold") {
		t.Fatal("a clone shares the state")
	}
	b, err := s.Save()
	if err != nil {
		t.Fatal(err)
	}
	r, err := gozen.Restore(g, b, gozen.Config{})
	if err != nil || query[int](t, r, "gold") != query[int](t, s, "gold") {
		t.Fatalf("restore: %v", err)
	}
	if _, err := gozen.Restore(g, []byte(`{"format_version": 99}`), gozen.Config{}); !errors.Is(err, gozen.ErrIncompatibleSave) {
		t.Fatal(err)
	}
}

// Sessions of one game do not share what changes while playing.
func TestSessionsAreIndependent(t *testing.T) {
	g := load(t)
	a := session(t, g, gozen.Config{Seed: 5})
	b := session(t, g, gozen.Config{Seed: 5})
	if err := a.Do(t.Context(), "- gold: 10\n- pick: Climb\n"); err != nil {
		t.Fatal(err)
	}
	if query[int](t, b, "gold") != 2 || query[string](t, b, "location") != "camp" {
		t.Fatal("one session changed another")
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if err := a.Do(ctx, "- gold: 1\n"); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
}

// Author's decision 5: set on a party member's ability or level is allowed only with Config.Dev.
// The zero value is production, and it is an error.
func TestStatsNeedDev(t *testing.T) {
	g := load(t)
	s := session(t, g, gozen.Config{Seed: 1})
	err := s.Do(t.Context(), "- set: {actor.walker.level: 3}\n")
	var ds gozen.Diagnostics
	if !errors.Is(err, gozen.ErrInvalid) || !errors.As(err, &ds) || !strings.Contains(ds[0].Message, "Config.Dev") || query[int](t, s, "actor.walker.level") != 1 {
		t.Fatalf("a level set outside development: %v", err)
	}
	if _, err := gozen.Explore(t.Context(), s, "- set: {actor.walker.hp: 1}\n", gozen.ExploreOptions{}); !errors.Is(err, gozen.ErrInvalid) {
		t.Fatalf("explore: %v", err)
	}
	if err := s.Do(t.Context(), "- set: {gold: 9, actor.walker.tactics: auto}\n"); err != nil {
		t.Fatalf("other sets are allowed: %v", err)
	}
}

// Explore's answers use the same words as Answer: choices are numbers, battles are Won, Lost,
// Fled.
func TestExploreAnswersAreAnswerWords(t *testing.T) {
	g := load(t)
	s := session(t, g, gozen.Config{Seed: 1})
	lantern, err := g.YAML("events.lantern.do")
	if err != nil {
		t.Fatal(err)
	}
	paths, err := gozen.Explore(t.Context(), s, string(lantern), gozen.ExploreOptions{})
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, p := range paths {
		got = append(got, fmt.Sprint(p.Answers))
	}
	if last := paths[2].Requests[len(paths[2].Requests)-1]; last.Kind != "battle" || last.ID != "motes" || last.Text != "lose" {
		t.Fatalf("the path keeps the battle: %+v", last)
	}
	var skills, sets []string
	if err := paths[0].End.Query("actor.walker.skills", &skills); err != nil {
		t.Fatalf("skills: %v %v", skills, err)
	}
	if err := paths[0].End.Query("actor.walker.sets", &sets); err != nil {
		t.Fatal(err)
	}
	want := fmt.Sprint([][]int{{0}, {1, gozen.Won}, {1, gozen.Lost}, {1, gozen.Fled}})
	if "["+strings.Join(got, " ")+"]" != want {
		t.Fatalf("got %v, want %s", got, want)
	}
}

// fixed takes the second option of every choice and lets the battles branch.
type fixed struct{}

func (fixed) Decide(_ context.Context, _ *gozen.Session, q gozen.Request) int {
	if q.Kind == "choice" {
		return 1
	}
	return gozen.Branch
}

func TestExplorePolicy(t *testing.T) {
	g := load(t)
	s := session(t, g, gozen.Config{Seed: 1})
	lantern, err := g.YAML("events.lantern.do")
	if err != nil {
		t.Fatal(err)
	}
	paths, err := gozen.Explore(t.Context(), s, string(lantern), gozen.ExploreOptions{Policy: fixed{}})
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, p := range paths {
		got = append(got, fmt.Sprint(p.Answers))
	}
	if strings.Join(got, " ") != "[1 1] [1 2] [1 3]" {
		t.Fatalf("a fixed choice and branching battles: %v", got)
	}
	auto := gozen.ExploreOptions{Policy: policyFunc(func(q gozen.Request) int { return 1 - map[string]int{"choice": 0, "battle": 1}[q.Kind] })}
	paths, err = gozen.Explore(t.Context(), s, string(lantern), auto)
	if err != nil || len(paths) != 1 || fmt.Sprint(paths[0].Answers) != "[1 1]" {
		t.Fatalf("battles fought on AUTO: %v %v", paths, err)
	}
	var gold int
	if err := paths[0].End.Query("gold", &gold); err != nil || gold != 2+6+5 {
		t.Fatalf("the AUTO battle's reward: %d %v", gold, err)
	}
}

type policyFunc func(gozen.Request) int

func (f policyFunc) Decide(_ context.Context, _ *gozen.Session, q gozen.Request) int { return f(q) }

// Game.YAML also reads definitions in files in subdirectories of data/.
func TestYAMLInSubdirectories(t *testing.T) {
	fsys := fstest.MapFS{}
	maps.Copy(fsys, trail)
	fsys["data/scenes/more.yaml"] = &fstest.MapFile{Data: []byte("events:\n  deep: {do: [{gold: 1}]}\n")}
	g, err := gozen.Load(fsys, ".")
	if err != nil {
		t.Fatal(err)
	}
	if b, err := g.YAML("events.deep.do"); err != nil || !strings.Contains(string(b), "gold") {
		t.Fatalf("%q %v", b, err)
	}
	if b, err := g.YAML("events.lantern.do"); err != nil || !strings.Contains(string(b), "choice") {
		t.Fatalf("%q %v", b, err)
	}
}

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
		t.Errorf("a canceled context: %v", err)
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

// TestCancelledMidwayLeavesAUsableSession: a context canceled while commands run ends Do with the context's error;
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
		t.Fatalf("a session canceled midway cannot be saved: %v", err)
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

const flowData = `
shops:
  smith: {goods: [potion]}
events:
  tour:
    do:
      - say: {who: scout, text: Hello}
      - notice: A notice
      - wait: 0.5
      - shop: smith
      - save: true
      - open: formation
      - image: {file: x.png}
      - music: {song: theme}
      - mood: calm
      - sfx: {name: ding}
  fight_can_lose:
    do:
      - battle: boss
        on_lose: [{gold: 1}]
        retry: false
      - say: after
  fight_must_win:
    do:
      - battle: boss
  ending:
    do: [{say: bye}, {clear: true}, {say: never shown}]
  loop:
    do: [{if: "true", then: [{call: loop}]}]
music:
  songs: {theme: {file: x.png}}
`

func mustNew(t *testing.T, g *gozen.Game, cfg gozen.Config) *gozen.Session {
	t.Helper()
	s, err := gozen.New(g, cfg)
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func flowGame(t *testing.T) *gozen.Game {
	t.Helper()
	fsys := fstest.MapFS{
		"game.yaml":      {Data: []byte("schema_version: 4\nid: flow\n" + testgame.Game)},
		"data/main.yaml": {Data: []byte(testgame.Data)},
		"data/flow.yaml": {Data: []byte(flowData)},
		"assets/x.png":   {Data: []byte("a stub")},
	}
	g, err := gozen.Load(fsys, ".")
	if err != nil {
		t.Fatal(err)
	}
	return g
}

// TestEveryRequestKind: stepping through events shows each kind of request in order, and reports immediate effects separately.
func TestEveryRequestKind(t *testing.T) {
	g := flowGame(t)
	var effects []string
	s, err := gozen.New(g, gozen.Config{Seed: 1, OnEffect: func(e gozen.Effect) { effects = append(effects, e.Kind+":"+e.Name) }})
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Start("- call: tour"); err != nil {
		t.Fatal(err)
	}
	var kinds []string
	for q := s.Next(); q.Kind != ""; q = s.Next() {
		kinds = append(kinds, q.Kind)
		switch q.Kind {
		case "say":
			if q.Who != "Scout" || q.Text != "Hello" {
				t.Errorf("say %+v", q)
			}
		case "wait":
			if q.Seconds != 0.5 {
				t.Errorf("wait %+v", q)
			}
		case "shop":
			if q.ID != "smith" {
				t.Errorf("shop %+v", q)
			}
		case "open":
			if q.ID != "formation" {
				t.Errorf("open %+v", q)
			}
		case "notice":
			if q.Text != "A notice" {
				t.Errorf("notice %+v", q)
			}
		}
	}
	want := []string{"say", "notice", "wait", "shop", "save", "open"}
	if !slices.Equal(kinds[:len(want)], want) {
		t.Fatalf("requests %v, want to start with %v", kinds, want)
	}
	for _, e := range []string{"image:x.png", "music:theme", "mood:calm", "sound:ding"} {
		if !slices.Contains(effects, e) {
			t.Errorf("effect %s missing from %v", e, effects)
		}
	}
}

// TestBattleRequestFlags: a battle that may be lost says so, and one that cannot be retried says so.
func TestBattleRequestFlags(t *testing.T) {
	g := flowGame(t)
	s := mustNew(t, g, gozen.Config{Seed: 1})
	if err := s.Start("- call: fight_can_lose"); err != nil {
		t.Fatal(err)
	}
	q := s.Next()
	if q.Kind != "battle" || q.ID != "boss" || q.Flags != "can_lose,no_retry" {
		t.Fatalf("%+v", q)
	}
	// the same request until it is answered
	if again := s.Next(); again.Kind != "battle" {
		t.Errorf("a battle waits for its answer: %+v", again)
	}
	// losing a battle that may be lost leaves everyone standing and goes on
	s.Answer(gozen.Lost)
	if next := s.Next(); next.Kind != "say" || next.Text != "after" {
		t.Errorf("after losing: %+v", next)
	}
	var result string
	if err := s.Query("result", &result); err != nil || result != "lose" {
		t.Errorf("result %q %v", result, err)
	}
	for _, m := range []string{"scout"} {
		var hp int
		if err := s.Query("actor."+m+".hp", &hp); err != nil || hp < 1 {
			t.Errorf("%s hp %d %v", m, hp, err)
		}
	}
	// Answer with nothing pending changes nothing
	s.Answer(3)

	s2 := mustNew(t, g, gozen.Config{Seed: 1})
	if err := s2.Start("- call: fight_must_win"); err != nil {
		t.Fatal(err)
	}
	if q := s2.Next(); q.Flags != "" {
		t.Errorf("a plain battle has no flags: %+v", q)
	}
	s2.Answer(gozen.Fled)
	var res string
	if err := s2.Query("result", &res); err != nil {
		t.Fatal(err)
	}
	if res != "escape" {
		t.Errorf("fled result %q", res)
	}
}

func TestClearEndsTheRun(t *testing.T) {
	g := flowGame(t)
	s := mustNew(t, g, gozen.Config{Seed: 1})
	if err := s.Start("- call: ending"); err != nil {
		t.Fatal(err)
	}
	var kinds, texts []string
	for q := s.Next(); q.Kind != ""; q = s.Next() {
		kinds = append(kinds, q.Kind)
		texts = append(texts, q.Text)
	}
	if !slices.Equal(kinds, []string{"say", "clear"}) || texts[0] != "bye" {
		t.Errorf("%v %v (nothing after the end is shown)", kinds, texts)
	}
	var done bool
	if err := s.Query("cleared", &done); err != nil || !done {
		t.Errorf("cleared = %v %v", done, err)
	}
	// Do runs no more commands once the game has been cleared
	if err := s.Do(t.Context(), "- gold: 100\n"); err != nil {
		t.Fatal(err)
	}
	var gold int
	if err := s.Query("gold", &gold); err != nil || gold != 100 {
		t.Errorf("gold %d: a command after the end must not run", gold)
	}
}

// TestLosingWithoutOnLoseIsGameOver: a battle lost where losing is not allowed ends the game.
func TestLosingWithoutOnLoseIsGameOver(t *testing.T) {
	g := flowGame(t)
	s := mustNew(t, g, gozen.Config{Seed: 1})
	if err := s.Start("- call: fight_must_win\n"); err != nil {
		t.Fatal(err)
	}
	if q := s.Next(); q.Kind != "battle" {
		t.Fatalf("%+v", q)
	}
	s.Answer(gozen.Lost)
	if q := s.Next(); q.Kind != "game_over" {
		t.Fatalf("%+v", q)
	}
	var over bool
	if err := s.Query("over", &over); err != nil || !over {
		t.Errorf("over = %v %v", over, err)
	}
	if q := s.Next(); q.Kind != "" {
		t.Errorf("nothing follows a game over: %+v", q)
	}
}

// TestEventsThatNeverEnd: a call loop is cut off with an error, not a hang.
func TestEventsThatNeverEnd(t *testing.T) {
	g := flowGame(t)
	s := mustNew(t, g, gozen.Config{Seed: 1})
	err := s.Do(t.Context(), "- call: loop\n")
	if err == nil {
		t.Fatal("a call loop must be reported")
	}
	if !strings.Contains(err.Error(), "deep") && !strings.Contains(err.Error(), "do not end") {
		t.Logf("loop reported as: %v", err)
	}
	s2 := mustNew(t, g, gozen.Config{Seed: 1})
	if err := s2.Start("- call: loop"); err != nil {
		t.Fatal(err)
	}
	for range 5 {
		if q := s2.Next(); q.Kind != "" {
			t.Fatalf("a loop shows nothing: %+v", q)
		}
	}
}

// TestBeginRunsTheStartEventOnce: Begin runs the start event and arrives; Next on a fresh session shows the first line of the opening.
func TestBeginRunsTheStartEvent(t *testing.T) {
	fsys := fstest.MapFS{
		"game.yaml":      {Data: []byte("schema_version: 4\nid: opening\n" + strings.Replace(testgame.Game, "start: {", "start: {event: intro, ", 1))},
		"data/main.yaml": {Data: []byte(testgame.Data)},
		"data/open.yaml": {Data: []byte("events:\n  intro: {do: [{set: {flag.opened: true}}, {say: Welcome}]}\n")},
		"assets/x.png":   {Data: []byte("stub")},
	}
	g, err := gozen.Load(fsys, ".")
	if err != nil {
		t.Fatal(err)
	}
	var seen []string
	s := mustNew(t, g, gozen.Config{Seed: 1, OnRequest: func(q gozen.Request) { seen = append(seen, q.Kind+":"+q.Text) }})
	if err := s.Begin(t.Context()); err != nil {
		t.Fatal(err)
	}
	if len(seen) == 0 || seen[0] != "say:Welcome" {
		t.Errorf("requests %v", seen)
	}
	var opened bool
	if err := s.Query("flag.opened", &opened); err != nil || !opened {
		t.Errorf("opened %v %v", opened, err)
	}
	if err := s.Begin(context.TODO()); err != nil {
		t.Fatal(err)
	}
}

func TestNilContextIsAnError(t *testing.T) {
	g := load(t)
	s := mustNew(t, g, gozen.Config{Seed: 1})
	if err := s.Do(nil, "- gold: 1\n"); err == nil { //nolint:staticcheck // A nil context is the case under test.
		t.Error("a nil context must be refused")
	}
}

// Recovery is an explicit policy, separate from retry and result acknowledgement.
func TestBattlePreservesCasualties(t *testing.T) {
	for _, recover := range []bool{true, false} {
		for _, answer := range []int{gozen.Auto, gozen.Lost} {
			t.Run(fmt.Sprintf("recover=%v/answer=%d", recover, answer), func(t *testing.T) {
				g, err := gozen.Load(fstest.MapFS{
					"game.yaml":      {Data: []byte(testgame.Game)},
					"data/main.yaml": {Data: []byte(testgame.Data)},
					"data/doom.yaml": {Data: []byte(`enemies:
  storm: {name: Storm, image: x.png, stats: {mhp: 99999, mmp: 0, atk: 99999, def: 999, mat: 1, mdf: 1, agi: 999, luk: 1}, ai: [{use: slap}]}
troops:
  doom: {members: [storm]}
`)},
					"assets/x.png": {Data: []byte("stub")},
				}, ".")
				if err != nil {
					t.Fatal(err)
				}
				s := mustNew(t, g, gozen.Config{Seed: 1})
				src := fmt.Sprintf(`
- join: guide
- battle: doom
  fighters: [guide]
  retry: false
  recover: %v
  on_lose:
    - set: {var.losses: "var.losses + 1", var.final_hp: "last_battle.hp.guide"}
- say: acknowledged
`, recover)
				if err := s.Start(src); err != nil {
					t.Fatal(err)
				}
				if q := s.Next(); q.Kind != "say" {
					t.Fatalf("join: %+v", q)
				}
				if q := s.Next(); q.Kind != "battle" {
					t.Fatalf("request: %+v", q)
				}
				scout := query[int](t, s, "actor.scout.hp")
				s.Answer(answer)
				if q := s.Next(); q.Kind != "say" {
					t.Fatalf("callback: %+v", q)
				}
				if got := query[bool](t, s, "last_battle.participated.guide && !last_battle.participated.scout"); !got {
					t.Fatal("wrong participants")
				}
				hp := query[int](t, s, "last_battle.hp.guide")
				if answer == gozen.Auto && hp != 0 {
					t.Fatalf("expected casualty, hp %d", hp)
				}
				want := hp
				if recover {
					want = max(1, want)
				}
				if got := query[int](t, s, "actor.guide.hp"); got != want {
					t.Fatalf("hp %d want %d", got, want)
				}
				if got := query[int](t, s, "actor.scout.hp"); got != scout {
					t.Fatal("absent scout changed")
				}
				s.Answer(answer)
				if got := query[int](t, s, "var.losses"); got != 1 {
					t.Fatalf("callbacks %d", got)
				}
				saved, err := s.Save()
				if err != nil {
					t.Fatal(err)
				}
				restored, err := gozen.Restore(g, saved, gozen.Config{})
				if err != nil {
					t.Fatal(err)
				}
				if got := query[int](t, restored, "actor.guide.hp"); got != want {
					t.Fatalf("restored hp %d", got)
				}
				if got := query[int](t, restored, "last_battle.hp.guide"); got != hp {
					t.Fatalf("restored snapshot %d", got)
				}
				restored.Next()
				restored.Answer(answer)
				if got := query[int](t, restored, "var.losses"); got != 1 {
					t.Fatalf("restored callbacks %d", got)
				}
			})
		}
	}
}

func TestCheckpointDiscardsContinuationAndSurvivesSave(t *testing.T) {
	files := fstest.MapFS{}
	for k, v := range trail {
		fileCopy := *v
		files[k] = &fileCopy
	}
	files["game.yaml"] = &fstest.MapFile{Data: []byte("schema_version: 9\nid: retry-test\nstart: {location: camp, party: [walker]}\n")}
	files["data/retry.yaml"] = &fstest.MapFile{Data: []byte("events: {retry: {do: [{say: Ready}]}}")}
	g, err := gozen.Load(files, ".")
	if err != nil {
		t.Fatal(err)
	}
	s := session(t, g, gozen.Config{Seed: 7})
	if err := s.Do(t.Context(), "- set: {var.counter: 17}\n- checkpoint: retry\n- set: {var.draw: 'rand()', var.counter: 1}"); err != nil {
		t.Fatal(err)
	}
	want := query[float64](t, s, "var.draw")
	b, err := s.Save()
	if err != nil {
		t.Fatal(err)
	}
	s, err = gozen.Restore(g, b, gozen.Config{Seed: 8})
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Start("- restore_checkpoint: true\n- set: {var.counter: 999}"); err != nil {
		t.Fatal(err)
	}
	if q := s.Next(); q.Kind != "say" || q.Text != "Ready" {
		t.Fatal(q)
	}
	if q := s.Next(); q.Kind != "" {
		t.Fatal(q)
	}
	if got := query[float64](t, s, "var.counter"); got != 17 {
		t.Fatal(got)
	}
	if err := s.Do(t.Context(), "- set: {var.draw: 'rand()'}"); err != nil {
		t.Fatal(err)
	}
	if got := query[float64](t, s, "var.draw"); got != want {
		t.Fatalf("random draw %v want %v", got, want)
	}
}

// TestEventArgumentsResume observes a reused rule, a nested choice and caller restoration.
func TestEventArgumentsResume(t *testing.T) {
	fsys := os.DirFS("../../docs/examples/event-arguments")
	g, err := gozen.Load(fsys, ".")
	if err != nil {
		t.Fatal(err)
	}
	cfg := gozen.Config{Seed: 7, OnLog: func(err error) { t.Error(err) }}
	s, err := gozen.New(g, cfg)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Start("- call: visit"); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"Ready potter", "Returned potter/2", "Ready weaver", "Returned weaver/3", "Totals 2/3"} {
		q := s.Next()
		if q.Kind == "choice" {
			s.Answer(0)
			q = s.Next()
		}
		if strings.HasPrefix(q.Text, "Roll ") {
			q = s.Next()
		}
		if q.Text != want {
			t.Fatalf("got %+v, want %s", q, want)
		}
		saved, err := s.Save()
		if err != nil {
			t.Fatal(err)
		}
		restored, err := gozen.Restore(g, saved, cfg)
		if err != nil {
			t.Fatal(err)
		}
		if got := restored.Next(); got.Text != want {
			t.Fatalf("resume %+v, want %s", got, want)
		}
		// Save and restore twice at the same line; earlier effects must not accumulate.
		saved, err = restored.Save()
		if err != nil {
			t.Fatal(err)
		}
		restored, err = gozen.Restore(g, saved, cfg)
		if err != nil {
			t.Fatal(err)
		}
		if got := restored.Next(); got.Text != want {
			t.Fatalf("second resume %+v", got)
		}
		if strings.HasPrefix(want, "Ready ") {
			a, b := s.Next(), restored.Next()
			if a.Text != b.Text || !strings.HasPrefix(a.Text, "Roll ") {
				t.Fatalf("RNG %v / %v", a, b)
			}
		}
		s = restored
	}
}

func TestBoundEventRootAndChangedSave(t *testing.T) {
	fsys := os.DirFS("../../docs/examples/event-arguments")
	g, err := gozen.Load(fsys, ".")
	if err != nil {
		t.Fatal(err)
	}
	cfg := gozen.Config{Seed: 19, OnLog: func(err error) { t.Error(err) }}
	s, err := gozen.New(g, cfg)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Start("- call: {event: encourage, args: {resident: '\"potter\"', amount: '2'}}"); err != nil {
		t.Fatal(err)
	}
	if q := s.Next(); q.Kind != "choice" {
		t.Fatalf("choice %+v", q)
	}
	saved, err := s.Save()
	if err != nil {
		t.Fatal(err)
	}
	r, err := gozen.Restore(g, saved, cfg)
	if err != nil {
		t.Fatal(err)
	}
	// A pending choice is restored without applying the effect twice.
	r.Answer(0)
	if q := r.Next(); q.Text != "Ready potter" {
		t.Fatalf("ready %+v", q)
	}
	again, err := r.Save()
	if err != nil {
		t.Fatal(err)
	}
	r, err = gozen.Restore(g, again, cfg)
	if err != nil {
		t.Fatal(err)
	}
	if q := r.Next(); q.Text != "Ready potter" {
		t.Fatalf("ready %+v", q)
	}
	r.Next()
	if q := r.Next(); q.Text != "Returned potter/2" {
		t.Fatalf("bindings %+v", q)
	}

	for _, mutate := range []func(map[string]any){
		func(c map[string]any) { c["args"] = map[string]any{"resident": "missing", "amount": 2} },
		func(c map[string]any) { c["args"] = map[string]any{"resident": "potter", "amount": true} },
		func(c map[string]any) { delete(c, "args") },
		func(c map[string]any) { c["fingerprint"] = "changed" },
	} {
		var doc map[string]any
		if err := json.Unmarshal(saved, &doc); err != nil {
			t.Fatal(err)
		}
		cursor, ok := doc["cursor"].(map[string]any)
		if !ok {
			t.Fatal("missing cursor")
		}
		mutate(cursor)
		broken, err := json.Marshal(doc)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := gozen.Restore(g, broken, cfg); err == nil {
			t.Fatal("accepted damaged bindings")
		}
	}
	header, err := os.ReadFile("../../docs/examples/event-arguments/game.yaml")
	if err != nil {
		t.Fatal(err)
	}
	story, err := os.ReadFile("../../docs/examples/event-arguments/data/story.yaml")
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		from, to string
		reject   bool
	}{
		{"Ready {arg.resident}", "Changed {arg.resident}", true},
		{"params: {resident: entity, amount: number}", "params: {amount: number, resident: entity}", false},
	} {
		edited, err := gozen.Load(fstest.MapFS{"game.yaml": {Data: header}, "data/story.yaml": {Data: []byte(strings.Replace(string(story), tc.from, tc.to, 1))}}, ".")
		if err != nil {
			t.Fatal(err)
		}
		_, err = gozen.Restore(edited, saved, cfg)
		if (err != nil) != tc.reject {
			t.Fatalf("changed definition: %v, want reject=%v", err, tc.reject)
		}
	}
	// A refused restore has not damaged the original session.
	s.Answer(0)
	if q := s.Next(); q.Text != "Ready potter" {
		t.Fatalf("original %+v", q)
	}
}

// Restore must consume exactly the recorded answers, never select a default for
// a damaged cursor or accept answers belonging after the saved line.
func TestResumeRejectsDamagedAnswers(t *testing.T) {
	g, err := gozen.Load(os.DirFS("../../docs/examples/event-arguments"), ".")
	if err != nil {
		t.Fatal(err)
	}
	cfg := gozen.Config{Seed: 19}
	s, err := gozen.New(g, cfg)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Start(`- call: {event: encourage, args: {resident: '"potter"', amount: '2'}}`); err != nil {
		t.Fatal(err)
	}
	if q := s.Next(); q.Kind != "choice" {
		t.Fatal(q)
	}
	s.Answer(0)
	// Preserve the supported boundary between Answer and Next, including re-save.
	answered, err := s.Save()
	if err != nil {
		t.Fatal(err)
	}
	for range 2 {
		restored, err := gozen.Restore(g, answered, cfg)
		if err != nil {
			t.Fatal(err)
		}
		answered, err = restored.Save()
		if err != nil {
			t.Fatal(err)
		}
		if q := restored.Next(); q.Text != "Ready potter" {
			t.Fatalf("answered choice replayed: %+v", q)
		}
	}

	if q := s.Next(); q.Text != "Ready potter" {
		t.Fatal(q)
	}
	saved, err := s.Save()
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name    string
		answers []int
		line    int
	}{
		{"valid", []int{0}, 2},
		{"missing", nil, 2},
		{"out of range", []int{1}, 2},
		{"unused", []int{0, 0}, 2},
		{"future answer", []int{0, 0}, 1},
		{"unreachable line", []int{0}, 99},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var doc map[string]any
			if err := json.Unmarshal(saved, &doc); err != nil {
				t.Fatal(err)
			}
			c, ok := doc["cursor"].(map[string]any)
			if !ok {
				t.Fatal("missing cursor")
			}
			c["answers"], c["line"] = tc.answers, tc.line
			b, err := json.Marshal(doc)
			if err != nil {
				t.Fatal(err)
			}
			r, err := gozen.Restore(g, b, cfg)
			if tc.name != "valid" {
				if !errors.Is(err, gozen.ErrIncompatibleSave) || r != nil {
					t.Fatalf("accepted damaged cursor: %v", err)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if q := r.Next(); q.Text != "Ready potter" {
				t.Fatal(q)
			}
			if q, want := r.Next(), s.Next(); q.Text != want.Text {
				t.Fatalf("RNG %v / %v", q, want)
			}
		})
	}
	if got := query[float64](t, s, "entity.potter.vigor"); got != 2 {
		t.Fatalf("original state changed: %v", got)
	}
}

func TestLegacySaveRefusesScreenContinuation(t *testing.T) {
	files := fstest.MapFS{
		"game.yaml": {Data: []byte("schema_version: 8\nid: screen-save\nstart: {location: workshop}\nscreens: {stock: {title: Stock, entries: [{label: Boards}]}}\n")},
		"data/world.yaml": {Data: []byte(`locations: {workshop: {name: Workshop}}
events:
  visit:
    do:
      - set: {var.opened: 'var.opened + 1'}
      - open: stock
      - say: Returned.
      - set: {var.closed: 'var.closed + 1'}
`)},
	}
	g, err := gozen.Load(files, ".")
	if err != nil {
		t.Fatal(err)
	}
	s, err := gozen.New(g, gozen.Config{Seed: 17})
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Start("- call: visit"); err != nil {
		t.Fatal(err)
	}
	for _, kind := range []string{"open", "say"} {
		if q := s.Next(); q.Kind != kind {
			t.Fatalf("request: %+v, want %s", q, kind)
		}
		for range 2 {
			if b, err := s.Save(); err == nil || b != nil {
				t.Fatal("save silently discarded a screen continuation")
			}
		}
		if query[int](t, s, "var.opened") != 1 || query[int](t, s, "var.closed") != 0 {
			t.Fatal("rejection changed event state")
		}
		if kind == "open" {
			s.Answer(-1)
		}
	}
	if q := s.Next(); q.Kind != "" {
		t.Fatalf("event did not finish: %+v", q)
	}
	b, err := s.Save()
	if err != nil {
		t.Fatal(err)
	}
	r, err := gozen.Restore(g, b, gozen.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if query[int](t, r, "var.opened") != 1 || query[int](t, r, "var.closed") != 1 {
		t.Fatal("finished event was lost or repeated")
	}
}

func TestAuthorScreenLifecycle(t *testing.T) {
	files := fstest.MapFS{
		"game.yaml": {Data: []byte(`schema_version: 8
id: screen-actions
start: {location: workshop}
screens:
  stock:
    title: Stock
    entries:
    - {label: Hidden, when: 'false', do: [{set: {var.wrong: '1'}}]}
    - label: Boards
      when: 'var.boards == 0'
      do:
      - choice:
        - {label: Confirm, do: [{set: {var.boards: 'var.boards + 1'}}]}
        - {label: Cancel}
      - open: receipt
      - set: {var.finished: 'var.finished + 1'}
    - {label: Read only}
  receipt:
    title: Receipt
    entries:
    - {label: Read, do: [{say: Ready.}]}
`)},
		"data/world.yaml": {Data: []byte(`locations: {workshop: {name: Workshop}}
events:
  visit:
    do:
    - set: {var.opened: 'var.opened + 1'}
    - open: stock
    - set: {var.closed: 'var.closed + 1'}
`)},
	}
	g, err := gozen.Load(files, ".")
	if err != nil {
		t.Fatal(err)
	}
	s := session(t, g, gozen.Config{Seed: 17, OnRequest: func(q gozen.Request) {
		if q.Kind == "open" && len(q.Options) > 0 {
			q.Options[0] = "observer mutation"
		}
	}})
	control := session(t, g, gozen.Config{Seed: 17})
	if err := s.Start("- call: visit"); err != nil {
		t.Fatal(err)
	}
	expect := func(kind, id string) gozen.Request {
		t.Helper()
		q := s.Next()
		if q.Kind != kind || q.ID != id {
			t.Fatalf("request %+v, want %s/%s", q, kind, id)
		}
		if kind == "open" {
			if saved, err := s.Save(); err == nil || saved != nil {
				t.Fatal("open screen save lost continuation")
			}
		}
		return q
	}
	q := expect("open", "stock")
	if !slices.Equal(q.Options, []string{"Boards", "Read only"}) {
		t.Fatalf("options: %v", q.Options)
	}
	q.Options[0] = "tampered"
	for range 2 {
		if q := expect("open", "stock"); q.Options[0] != "Boards" {
			t.Fatal("request alias")
		}
	}
	if query[int](t, s, "var.closed") != 0 {
		t.Fatal("Next closed the screen")
	}
	s.Answer(99)
	expect("open", "stock")
	s.Answer(1) // A read-only entry leaves the screen open.
	expect("open", "stock")
	if err := s.Start("- set: {var.wrong: '1'}"); !errors.Is(err, gozen.ErrInvalid) {
		t.Fatalf("Start during screen: %v", err)
	}
	if err := s.Do(t.Context(), "- set: {var.wrong: '1'}"); !errors.Is(err, gozen.ErrInvalid) {
		t.Fatalf("Do during screen: %v", err)
	}
	s.Answer(0)
	s.Answer(0) // Repeated confirmation cannot queue another action.
	expect("choice", "")
	if b, err := s.Save(); err == nil || b != nil {
		t.Fatal("entry save lost parent")
	}
	s.Answer(1) // Cancel the authored confirmation.
	expect("open", "receipt")
	if query[int](t, s, "var.boards") != 0 {
		t.Fatal("cancel committed boards")
	}
	s.Answer(-1)
	expect("open", "stock")
	s.Answer(0)
	expect("choice", "")
	s.Answer(0)
	expect("open", "receipt")
	s.Answer(0)
	if q := expect("say", ""); q.Text != "Ready." {
		t.Fatalf("receipt: %+v", q)
	}
	expect("open", "receipt")
	s.Answer(-1)
	q = expect("open", "stock")
	if !slices.Equal(q.Options, []string{"Read only"}) {
		t.Fatalf("stale entries: %v", q.Options)
	}
	if query[int](t, s, "var.boards") != 1 || query[int](t, s, "var.finished") != 2 || query[int](t, s, "var.closed") != 0 {
		t.Fatal("entry effects or opener continuation")
	}
	s.Answer(-1)
	expect("", "")
	if query[int](t, s, "var.opened") != 1 || query[int](t, s, "var.closed") != 1 || query[int](t, s, "var.wrong") != 0 {
		t.Fatal("opener lost or repeated")
	}
	b, err := s.Save()
	if err != nil {
		t.Fatal(err)
	}
	r, err := gozen.Restore(g, b, gozen.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if query[int](t, r, "var.boards") != 1 || query[int](t, r, "var.closed") != 1 {
		t.Fatal("finished save lost effects")
	}
	for _, current := range []*gozen.Session{s, r, control} {
		if err := current.Do(t.Context(), "- set: {var.draw: 'rand(1000000)'}"); err != nil {
			t.Fatal(err)
		}
	}
	if query[int](t, s, "var.draw") != query[int](t, control, "var.draw") || query[int](t, r, "var.draw") != query[int](t, control, "var.draw") {
		t.Fatal("screen consumed randomness")
	}
	// Default unattended execution explicitly closes authored screens.
	if err := control.Do(t.Context(), "- call: visit"); err != nil {
		t.Fatal(err)
	}
	if query[int](t, control, "var.closed") != 1 || query[int](t, control, "var.boards") != 0 {
		t.Fatal("default policy performed a screen action")
	}
}

func TestAuthorScreenActionLeavesLocationOrRestoresCheckpoint(t *testing.T) {
	for _, tc := range []struct {
		name, action     string
		closed, finished int
	}{
		{"move", "- move: yard", 1, 1},
		{"checkpoint", "- restore_checkpoint: true", 0, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			files := fstest.MapFS{
				"game.yaml": {Data: []byte("schema_version: 9\nid: screen-exit\nstart: {location: workshop}\nscreens:\n  stock:\n    title: Stock\n    entries:\n    - label: Leave\n      do:\n      " + tc.action + "\n      - set: {var.finished: '1'}\n")},
				"data/world.yaml": {Data: []byte(`locations: {workshop: {name: Workshop}, yard: {name: Yard}}
events:
  retry:
    do: [{say: Retry.}]
  visit:
    do:
    - checkpoint: retry
    - open: stock
    - set: {var.closed: '1'}
`)},
			}
			g, err := gozen.Load(files, ".")
			if err != nil {
				t.Fatal(err)
			}
			s := session(t, g, gozen.Config{Seed: 17})
			if err := s.Start("- call: visit"); err != nil {
				t.Fatal(err)
			}
			if q := s.Next(); q.Kind != "open" {
				t.Fatalf("open: %+v", q)
			}
			s.Answer(0)
			if tc.name == "checkpoint" {
				if q := s.Next(); q.Text != "Retry." {
					t.Fatalf("checkpoint: %+v", q)
				}
			}
			if q := s.Next(); q.Kind != "" {
				t.Fatalf("screen survived exit: %+v", q)
			}
			if query[int](t, s, "var.closed") != tc.closed || query[int](t, s, "var.finished") != tc.finished {
				t.Fatal("wrong continuation after exit")
			}
			if _, err := s.Save(); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestAuthorScreenNestingIsBounded(t *testing.T) {
	files := fstest.MapFS{
		"game.yaml":       {Data: []byte("schema_version: 8\nid: screen-depth\nstart: {location: workshop}\nscreens: {stock: {title: Stock, entries: [{label: Again, do: [{open: stock}]}]}}\n")},
		"data/world.yaml": {Data: []byte("locations: {workshop: {name: Workshop}}\nevents: {visit: {do: [{open: stock}, {set: {var.closed: '1'}}]}}\n")},
	}
	g, err := gozen.Load(files, ".")
	if err != nil {
		t.Fatal(err)
	}
	var logs []error
	s := session(t, g, gozen.Config{OnLog: func(err error) { logs = append(logs, err) }})
	if err := s.Start("- call: visit"); err != nil {
		t.Fatal(err)
	}
	for range 65 {
		if q := s.Next(); q.Kind != "open" {
			t.Fatalf("open: %+v", q)
		}
		s.Answer(0)
	}
	if q := s.Next(); q.Kind != "open" {
		t.Fatalf("open after limit: %+v", q)
	}
	if len(logs) == 0 || !strings.Contains(logs[0].Error(), "nested too deep") {
		t.Fatalf("missing depth rejection: %v", logs)
	}
	for range 64 {
		s.Answer(-1)
		s.Next()
	}
	if q := s.Next(); q.Kind != "" || query[int](t, s, "var.closed") != 1 {
		t.Fatalf("could not close after rejection: %+v", q)
	}
}

// A cursor before open must reject changed reachable screen actions before replay,
// including the entries that a later screen continuation may replay.
func TestResumeChecksReachableScreens(t *testing.T) {
	const header = `schema_version: 10
id: screen-fingerprint
start: {location: workshop}
screens:
  stock:
    title: Stock
    entries:
    - label: Boards
      when: 'true'
      do:
      - choice:
        - {label: Inspect, do: [{open: receipt}]}
        - {label: Cancel}
  receipt:
    title: Receipt
    entries:
    - {label: Read, do: [{call: report}]}
  unused:
    title: Unused
    entries: [{label: Spare}]
`
	const story = `locations: {workshop: {name: Workshop}}
events:
  visit:
    do:
    - set: {var.opened: 'var.opened + 1'}
    - say: Ready.
    - open: stock
    - set: {var.closed: 'var.closed + 1'}
  report:
    do:
    - say: Checked.
    - if: 'false'
      then: [{call: visit}]
  unused:
    do: [{say: Unused.}]
`
	loadGame := func(head, body string) *gozen.Game {
		t.Helper()
		g, err := gozen.Load(fstest.MapFS{
			"game.yaml":       {Data: []byte(head)},
			"data/world.yaml": {Data: []byte(body)},
		}, ".")
		if err != nil {
			t.Fatal(err)
		}
		return g
	}
	cfg := gozen.Config{Seed: 17}
	g := loadGame(header, story)
	s := session(t, g, cfg)
	if err := s.Start("- call: visit"); err != nil {
		t.Fatal(err)
	}
	if q := s.Next(); q.Text != "Ready." {
		t.Fatalf("before open: %+v", q)
	}
	saved, err := s.Save()
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name, from, to   string
		inHeader, reject bool
	}{
		{"entry identity", "label: Boards", "label: Tools", true, true},
		{"entry guard", "when: 'true'", "when: 'false'", true, true},
		{"nested screen", "label: Read", "label: Inspect receipt", true, true},
		{"screen callee", "say: Checked.", "say: Changed.", false, true},
		{"unrelated screen", "title: Unused", "title: Elsewhere", true, false},
		{"unrelated event", "say: Unused.", "say: Elsewhere.", false, false},
		{"mapping order", "label: Read, do: [{call: report}]", "do: [{call: report}], label: Read", true, false},
		{"whitespace", "title: Stock", "title:   Stock # same title", true, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			head, body := header, story
			if tc.inHeader {
				head = strings.Replace(head, tc.from, tc.to, 1)
			} else {
				body = strings.Replace(body, tc.from, tc.to, 1)
			}
			r, err := gozen.Restore(loadGame(head, body), saved, cfg)
			if tc.reject {
				if !errors.Is(err, gozen.ErrIncompatibleSave) {
					t.Fatalf("changed action accepted: %v", err)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if q := r.Next(); q.Text != "Ready." {
				t.Fatalf("resume: %+v", q)
			}
			if q := r.Next(); q.Kind != "open" || q.ID != "stock" {
				t.Fatalf("open: %+v", q)
			}
			r.Answer(0)
			if q := r.Next(); q.Kind != "choice" {
				t.Fatalf("confirmation: %+v", q)
			}
			r.Answer(0)
			if q := r.Next(); q.ID != "receipt" {
				t.Fatalf("nested screen: %+v", q)
			}
			r.Answer(0)
			if q := r.Next(); q.Text != "Checked." {
				t.Fatalf("callee: %+v", q)
			}
			r.Next()
			r.Answer(-1)
			r.Next()
			r.Answer(-1)
			r.Next()
			if opened, closed := query[float64](t, r, "var.opened"), query[float64](t, r, "var.closed"); opened != 1 || closed != 1 {
				t.Fatalf("opener effects: %v/%v", opened, closed)
			}
		})
	}
	// Rejected restores leave the original session and its continuation usable.
	if q := s.Next(); q.ID != "stock" {
		t.Fatalf("original screen: %+v", q)
	}
	s.Answer(-1)
	s.Next()
	if query[float64](t, s, "var.opened") != 1 || query[float64](t, s, "var.closed") != 1 {
		t.Fatal("original continuation changed")
	}
}

func TestScreenArguments(t *testing.T) {
	g, err := gozen.Load(os.DirFS("../../docs/examples/event-arguments"), ".")
	if err != nil {
		t.Fatal(err)
	}
	s := session(t, g, gozen.Config{Seed: 17})
	expect := func(kind, text string) {
		t.Helper()
		q := s.Next()
		if q.Kind != kind || q.Text != text {
			t.Fatalf("got %+v, want %s %q", q, kind, text)
		}
	}
	for _, amount := range []string{"2", "3"} {
		if err := s.Start("- call: {event: stock_visit, args: {amount: " + amount + "}}"); err != nil {
			t.Fatal(err)
		}
		expect("open", "Stock "+amount)
		s.Answer(0)
		expect("choice", "")
		s.Answer(1) // Cancellation leaves the stock unchanged.
		expect("open", "Receipt 1"+amount)
		s.Answer(0)
		expect("say", "Received 1"+amount)
		expect("open", "Receipt 1"+amount)
		s.Answer(-1)
		expect("say", "Stock "+amount)
		expect("open", "Stock "+amount)
		s.Answer(0)
		expect("choice", "")
		s.Answer(0)
		s.Answer(0) // A duplicate answer cannot execute the pending action twice.
		expect("open", "Receipt 1"+amount)
		saved, err := s.Save()
		if err != nil {
			t.Fatal(err)
		}
		s, err = gozen.Restore(g, saved, gozen.Config{Seed: 999})
		if err != nil {
			t.Fatal(err)
		}
		expect("open", "Receipt 1"+amount)
		s.Answer(-1)
		expect("say", "Stock "+amount)
		expect("open", "Stock "+amount)
		s.Answer(-1)
		expect("say", "Returned "+amount)
		expect("", "")
	}
	if got := query[int](t, s, "var.total"); got != 5 {
		t.Fatalf("total = %d, want 5", got)
	}
	saved, err := s.Save()
	if err != nil {
		t.Fatal(err)
	}
	r, err := gozen.Restore(g, saved, gozen.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if got := query[int](t, r, "var.total"); got != 5 {
		t.Fatalf("restored total = %d", got)
	}
}

// One candidate screen must assign either activity without copying its commands.
func TestScreenActivityTargets(t *testing.T) {
	g, err := gozen.Load(os.DirFS("../../docs/examples/event-arguments"), ".")
	if err != nil {
		t.Fatal(err)
	}
	s := session(t, g, gozen.Config{Seed: 17})
	for _, task := range []string{"prepare", "shape"} {
		if err := s.Start(`- call: {event: assign_visit, args: {task: '"` + task + `"', resident: '"weaver"'}}`); err != nil {
			t.Fatal(err)
		}
		if q := s.Next(); q.Kind != "open" || q.Text != "Assign "+task {
			t.Fatalf("open: %+v", q)
		}
		s.Answer(0)
		if q := s.Next(); q.Kind != "choice" {
			t.Fatalf("choice: %+v", q)
		}
		if got := query[string](t, s, "activity."+task+".assignee"); got != "potter" {
			t.Fatal(got)
		}
		s.Answer(1)
		s.Next()
		if got := query[string](t, s, "activity."+task+".assignee"); got != "potter" {
			t.Fatal(got)
		}
		s.Next()
		s.Answer(0)
		s.Next()
		s.Answer(0)
		s.Answer(0)
		s.Next()
		if got := query[string](t, s, "activity."+task+".assignee"); got != "weaver" {
			t.Fatal(got)
		}
		s.Next()
		s.Answer(-1)
		s.Next()
		s.Next()
	}
	saved, err := s.Save()
	if err != nil {
		t.Fatal(err)
	}
	restored, err := gozen.Restore(g, saved, gozen.Config{})
	if err != nil {
		t.Fatal(err)
	}
	for _, task := range []string{"prepare", "shape"} {
		if got := query[string](t, restored, "activity."+task+".assignee"); got != "weaver" {
			t.Fatal(got)
		}
	}
}

// Saving at every input boundary must agree with uninterrupted play, including
// cancellation, nested screens, repeated opens, duplicate answers and RNG draws.
func TestScreenContinuationRoundTrip(t *testing.T) {
	files := fstest.MapFS{
		"game.yaml": {Data: []byte(`schema_version: 10
id: screen-round-trip
start: {location: workshop}
screens:
  stock:
    params: {amount: number}
    title: 'Stock {arg.amount}'
    entries:
    - label: Add
      do:
      - choice:
        - {label: Confirm, do: [{set: {var.total: 'var.total + arg.amount'}}]}
        - {label: Cancel}
      - open: receipt
      - say: Back.
  receipt:
    title: Receipt
    entries: [{label: Read, do: [{say: Receipt.}]}]
`)},
		"data/world.yaml": {Data: []byte(`locations: {workshop: {name: Workshop}}
events:
  visit:
    do:
    - set: {var.opened: 'var.opened + 1', var.roll: 'rand(1000)'}
    - say: Ready.
    - choice: [{label: Continue}]
    - open: {screen: stock, args: {amount: '2'}}
    - open: {screen: stock, args: {amount: '3'}}
    - set: {var.closed: 'var.closed + 1', var.next: 'rand(1000)'}
    - say: Finished.
`)},
	}
	g, err := gozen.Load(files, ".")
	if err != nil {
		t.Fatal(err)
	}
	for _, restoreFirst := range []bool{false, true} {
		t.Run(strconv.FormatBool(restoreFirst), func(t *testing.T) {
			control := session(t, g, gozen.Config{Seed: 17})
			live := session(t, g, gozen.Config{Seed: 17})
			for _, s := range []*gozen.Session{control, live} {
				if err := s.Start("- call: visit"); err != nil {
					t.Fatal(err)
				}
			}
			roundtrip := func() {
				t.Helper()
				b, err := live.Save()
				if err != nil {
					t.Fatal(err)
				}
				live, err = gozen.Restore(g, b, gozen.Config{Seed: 99, OnRequest: func(gozen.Request) { t.Error("replay reported a request") }, OnEffect: func(gozen.Effect) { t.Error("replay reported an effect") }})
				if err != nil {
					t.Fatal(err)
				}
				// Re-save before consuming the restored request as well.
				b, err = live.Save()
				if err != nil {
					t.Fatal(err)
				}
				live, err = gozen.Restore(g, b, gozen.Config{Seed: 101})
				if err != nil {
					t.Fatal(err)
				}
			}
			next := func(kind, text string, restore bool) {
				t.Helper()
				a, b := control.Next(), live.Next()
				if a.Kind != kind || a.Text != text || !reflect.DeepEqual(a, b) {
					t.Fatalf("requests: %+v / %+v, want %s %s", a, b, kind, text)
				}
				if restore {
					roundtrip()
					if q := live.Next(); !reflect.DeepEqual(a, q) {
						t.Fatalf("restored request: %+v / %+v", a, q)
					}
				}
			}
			answer := func(n int) { control.Answer(n); live.Answer(n); roundtrip() }
			next("say", "Ready.", restoreFirst)
			next("choice", "", restoreFirst)
			control.Answer(0)
			live.Answer(0)
			for _, amount := range []string{"2", "3"} {
				next("open", "Stock "+amount, true)
				for _, option := range []int{1, 0} {
					answer(0)
					next("choice", "", true)
					answer(option)
					control.Answer(option)
					live.Answer(option) // duplicate
					next("open", "Receipt", true)
					answer(0)
					next("say", "Receipt.", true)
					next("open", "Receipt", true)
					answer(-1)
					next("say", "Back.", true)
					next("open", "Stock "+amount, true)
				}
				answer(-1)
			}
			next("say", "Finished.", true)
			next("", "", false)
			for _, key := range []string{"opened", "closed", "total", "roll", "next"} {
				if a, b := query[float64](t, control, "var."+key), query[float64](t, live, "var."+key); a != b {
					t.Fatalf("%s: %v != %v", key, a, b)
				}
			}
			if query[float64](t, live, "var.total") != 5 || query[float64](t, live, "var.closed") != 1 {
				t.Fatal("lost or repeated effects")
			}
		})
	}
}

func TestScreenContinuationRejectsDamage(t *testing.T) {
	g, err := gozen.Load(os.DirFS("../../docs/examples/event-arguments"), ".")
	if err != nil {
		t.Fatal(err)
	}
	s := session(t, g, gozen.Config{Seed: 17})
	if err := s.Start("- call: {event: stock_visit, args: {amount: '2'}}"); err != nil {
		t.Fatal(err)
	}
	s.Next()
	s.Answer(0)
	s.Next()
	saved, err := s.Save()
	if err != nil {
		t.Fatal(err)
	}
	for name, mutate := range map[string]func(map[string]any){
		"invalid digest":  func(c map[string]any) { c["inputs"] = []any{map[string]any{"request": "broken"}} },
		"wrong request":   func(c map[string]any) { c["inputs"] = []any{map[string]any{"request": strings.Repeat("0", 64)}} },
		"missing request": func(c map[string]any) { c["inputs"] = []any{map[string]any{"answer": 0}} },
		"missing answer": func(c map[string]any) {
			in, ok := c["inputs"].([]any)
			if !ok {
				t.Fatal("missing inputs")
			}
			c["inputs"] = []any{in[0], in[2]}
		},
		"wrong answer": func(c map[string]any) {
			in, ok := c["inputs"].([]any)
			if !ok {
				t.Fatal("missing inputs")
			}
			in[1] = map[string]any{"answer": 99}
		},
		"both fields": func(c map[string]any) {
			in, ok := c["inputs"].([]any)
			if !ok {
				t.Fatal("missing inputs")
			}
			row, ok := in[0].(map[string]any)
			if !ok {
				t.Fatal("missing request")
			}
			row["answer"] = 0
		},
		"too many inputs":    func(c map[string]any) { c["inputs"] = make([]any, 20001) },
		"changed arguments":  func(c map[string]any) { c["args"] = map[string]any{"amount": 3} },
		"changed definition": func(c map[string]any) { c["fingerprint"] = "changed" },
	} {
		t.Run(name, func(t *testing.T) {
			var doc map[string]any
			if err := json.Unmarshal(saved, &doc); err != nil {
				t.Fatal(err)
			}
			c, ok := doc["cursor"].(map[string]any)
			if !ok {
				t.Fatal("missing cursor")
			}
			mutate(c)
			b, err := json.Marshal(doc)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := gozen.Restore(g, b, gozen.Config{}); !errors.Is(err, gozen.ErrIncompatibleSave) {
				t.Fatalf("accepted damaged continuation: %v", err)
			}
		})
	}
	// Failed restores neither answer nor discard the live pending confirmation.
	s.Answer(0)
	if q := s.Next(); q.Text != "Receipt 12" || query[int](t, s, "var.total") != 2 {
		t.Fatalf("live game changed: %+v", q)
	}
}

func TestScreenDepthBoundary(t *testing.T) {
	g, err := gozen.Load(fstest.MapFS{
		"game.yaml": {Data: []byte(`schema_version: 10
id: shelves
title: Shelves
start: {location: workshop}
screens:
  shelf:
    title: Shelf
    entries:
    - label: Inspect inner shelf
      do:
      - open: shelf
      - set: {var.returned: 'var.returned + 1'}
`)},
		"data/world.yaml": {Data: []byte(`locations: {workshop: {name: Workshop}}
events:
  inspect_shelves:
    do:
    - open: shelf
    - set: {var.closed: 'var.closed + 1'}
`)},
	}, ".")
	if err != nil {
		t.Fatal(err)
	}
	var diagnostics []string
	s := session(t, g, gozen.Config{Seed: 17, OnLog: func(err error) { diagnostics = append(diagnostics, err.Error()) }})
	if err := s.Start("- call: inspect_shelves"); err != nil {
		t.Fatal(err)
	}
	for range 64 {
		if q := s.Next(); q.Kind != "open" {
			t.Fatalf("open: %+v", q)
		}
		s.Answer(0)
	}
	if q := s.Next(); q.Kind != "open" {
		t.Fatalf("returned screen: %+v", q)
	}
	if len(diagnostics) != 1 || !strings.Contains(diagnostics[0], "nested too deep") {
		t.Fatalf("diagnostics: %v", diagnostics)
	}
	if got := query[float64](t, s, "var.returned"); got != 1 {
		t.Fatalf("rejected opener did not continue: %v", got)
	}
	for range 64 {
		s.Answer(-1)
		s.Next()
	}
	if got := query[float64](t, s, "var.returned"); got != 64 {
		t.Fatalf("unwinding repeated or lost an entry: %v", got)
	}
	if got := query[float64](t, s, "var.closed"); got != 1 {
		t.Fatalf("root continuation: %v", got)
	}
}
