package gozen_test

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/nao1215/gozen"
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

func TestStores(t *testing.T) {
	for name, st := range map[string]gozen.Store{"dir": gozen.DirStore(filepath.Join(t.TempDir(), "saves")), "mem": gozen.MemStore()} {
		t.Run(name, func(t *testing.T) {
			if _, err := st.Read("one"); !errors.Is(err, fs.ErrNotExist) {
				t.Fatal(err)
			}
			if names, err := st.List(); err != nil || len(names) != 0 {
				t.Fatal(names, err)
			}
			if err := st.Write("one", []byte("a")); err != nil {
				t.Fatal(err)
			}
			if err := st.Write("one", []byte("b")); err != nil {
				t.Fatal(err)
			}
			if b, err := st.Read("one"); err != nil || string(b) != "b" {
				t.Fatal(string(b), err)
			}
			if names, err := st.List(); err != nil || strings.Join(names, ",") != "one" {
				t.Fatal(names, err)
			}
			if err := st.Remove("one"); err != nil || st.Remove("one") != nil {
				t.Fatal(err)
			}
			for _, bad := range []string{"", "../x", "a/b", "."} {
				if err := st.Write(bad, nil); !errors.Is(err, gozen.ErrInvalid) {
					t.Errorf("%q: %v", bad, err)
				}
			}
		})
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
	for name, f := range trail {
		fsys[name] = f
	}
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
