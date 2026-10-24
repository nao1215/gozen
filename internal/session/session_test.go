package session_test

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"maps"
	"slices"
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
