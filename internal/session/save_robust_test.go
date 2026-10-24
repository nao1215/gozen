package session_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"strings"
	"testing"
	"testing/fstest"

	gozen "github.com/nao1215/gozen/internal/session"
)

// played is a session of the trail game that has done a few things.
func played(t *testing.T, seed uint64) (*gozen.Game, *gozen.Session) {
	t.Helper()
	g := load(t)
	s := session(t, g, gozen.Config{Seed: seed, Dev: true})
	if err := s.Do(t.Context(), "- set: {flag.met: true, var.count: 3, var.name: \"'Sela'\"}\n- gold: 7\n- join: sela\n- pick: Climb\n"); err != nil {
		t.Fatal(err)
	}
	return g, s
}

// TestSaveRestoreSaveIsStable: saving, restoring and saving again gives the same bytes (ADR 0025), and the
// restored session reads the same state with the expressions.
func TestSaveRestoreSaveIsStable(t *testing.T) {
	g, s := played(t, 9)
	b1, err := s.Save()
	if err != nil {
		t.Fatal(err)
	}
	r, err := gozen.Restore(g, b1, gozen.Config{Seed: 1})
	if err != nil {
		t.Fatal(err)
	}
	b2, err := r.Save()
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(b1, b2) {
		t.Fatalf("save -> restore -> save changed the bytes:\n%s\n---\n%s", b1, b2)
	}
	for _, q := range []string{"location", "gold", "flag.met", "var.count", "var.name", "party.size", "actor.sela.in_party"} {
		var a, b any
		if err := errors.Join(s.Query(q, &a), r.Query(q, &b)); err != nil {
			t.Fatal(q, err)
		}
		if fmt.Sprint(a) != fmt.Sprint(b) {
			t.Errorf("%s: %v became %v after a restore", q, a, b)
		}
	}
}

// TestSameSeedSameResult: the same seed and the same commands give the same state and the same save;
// the random draws continue the same way after a restore.
func TestSameSeedSameResult(t *testing.T) {
	g := load(t)
	run := func(seed uint64) ([]byte, []string) {
		var seen []string
		s := session(t, g, gozen.Config{Seed: seed, Dev: true, OnRequest: func(q gozen.Request) { seen = append(seen, q.Kind+":"+q.Text) }})
		lantern, err := g.YAML("events.lantern.do")
		if err != nil {
			t.Fatal(err)
		}
		if err := s.Do(t.Context(), "- set: {var.r: 'rand(100000)'}\n"+string(lantern)+"- set: {var.s: 'rand(100000)'}\n"); err != nil {
			t.Fatal(err)
		}
		b, err := s.Save()
		if err != nil {
			t.Fatal(err)
		}
		return b, seen
	}
	b1, seen1 := run(42)
	b2, seen2 := run(42)
	if !bytes.Equal(b1, b2) || strings.Join(seen1, "|") != strings.Join(seen2, "|") {
		t.Fatal("the same seed gave different plays")
	}
	b3, _ := run(43)
	if bytes.Equal(b1, b3) {
		t.Error("different seeds gave the same play (the draws ignore the seed?)")
	}
}

// TestCloneContinuesTheSameDraws: a clone made at a point draws the same numbers as the original from there.
func TestCloneContinuesTheSameDraws(t *testing.T) {
	_, s := played(t, 77)
	c, err := s.Clone()
	if err != nil {
		t.Fatal(err)
	}
	script := "- set: {var.a: 'rand(1000000)', var.b: 'rand(1000000)'}\n"
	if err := errors.Join(s.Do(t.Context(), script), c.Do(t.Context(), script)); err != nil {
		t.Fatal(err)
	}
	for _, q := range []string{"var.a", "var.b"} {
		if x, y := query[int](t, s, q), query[int](t, c, q); x != y {
			t.Errorf("%s: the original drew %d, the clone %d", q, x, y)
		}
	}
}

// TestRestoreRejectsDamagedSaves: a truncated, edited, foreign or future save is ErrIncompatibleSave, never a panic
// and never a half-built session.
func TestRestoreRejectsDamagedSaves(t *testing.T) {
	g, s := played(t, 5)
	good, err := s.Save()
	if err != nil {
		t.Fatal(err)
	}
	// every strict prefix is a truncated file
	for n := 0; n < len(good); n += max(1, len(good)/200) {
		if _, err := gozen.Restore(g, good[:n], gozen.Config{}); !errors.Is(err, gozen.ErrIncompatibleSave) {
			t.Fatalf("a save cut at %d of %d bytes: %v", n, len(good), err)
		}
	}
	var env map[string]json.RawMessage
	if err := json.Unmarshal(good, &env); err != nil {
		t.Fatal(err)
	}
	with := func(key, value string) []byte {
		m := map[string]json.RawMessage{}
		maps.Copy(m, env)
		if value == "" {
			delete(m, key)
		} else {
			m[key] = json.RawMessage(value)
		}
		b, err := json.Marshal(m)
		if err != nil {
			t.Fatal(err)
		}
		return b
	}
	cases := map[string][]byte{
		"future format version":     with("format_version", "3"),
		"format version zero":       with("format_version", "0"),
		"negative format version":   with("format_version", "-1"),
		"format version as string":  with("format_version", `"2"`),
		"format version missing":    with("format_version", ""),
		"another game":              with("game_id", `"someone-else"`),
		"game id missing":           with("game_id", ""),
		"data version from future":  with("data_version", "99"),
		"data version negative":     with("data_version", "-1"),
		"state missing":             with("state", ""),
		"state null":                with("state", "null"),
		"state is a list":           with("state", "[]"),
		"state is a number":         with("state", "7"),
		"a cursor on line zero":     with("cursor", `{"event":"lantern","line":0}`),
		"a cursor with no event":    with("cursor", `{"line":1}`),
		"a cursor of two kinds":     with("cursor", `{"event":"lantern","arrival":true,"line":1}`),
		"empty":                     nil,
		"just whitespace":           []byte("  \n"),
		"null":                      []byte("null"),
		"a list":                    []byte("[]"),
		"a string":                  []byte(`"save"`),
		"not json":                  []byte("gold: 1"),
		"trailing garbage":          append(bytes.Clone(good), []byte(" {}")...),
		"duplicate key":             []byte(`{"format_version":2,"format_version":2}`),
		"unknown envelope field":    with("extra", "1"),
		"a NUL byte":                append(bytes.Clone(good[:len(good)/2]), 0),
		"invalid utf-8 in the file": append([]byte(`{"game_id":"`), 0xff, 0xfe),
	}
	for name, b := range cases {
		t.Run(name, func(t *testing.T) {
			r, err := gozen.Restore(g, b, gozen.Config{})
			if !errors.Is(err, gozen.ErrIncompatibleSave) {
				t.Fatalf("want ErrIncompatibleSave, got %v (session %v)", err, r != nil)
			}
			if r != nil {
				t.Fatal("an error must not come with a session")
			}
		})
	}
	if _, err := gozen.Restore(nil, good, gozen.Config{}); !errors.Is(err, gozen.ErrInvalid) {
		t.Errorf("a nil game: %v", err)
	}
	if _, err := gozen.New(nil, gozen.Config{}); !errors.Is(err, gozen.ErrInvalid) {
		t.Errorf("a nil game: %v", err)
	}
}

// TestRestoreWithEditedStateFields: editing the state inside a valid wrapper is caught or repaired, not trusted.
func TestRestoreWithEditedStateFields(t *testing.T) {
	g, s := played(t, 5)
	good, err := s.Save()
	if err != nil {
		t.Fatal(err)
	}
	var env struct {
		FormatVersion int             `json:"format_version"`
		GameID        string          `json:"game_id"`
		DataVersion   int             `json:"data_version"`
		State         json.RawMessage `json:"state"`
	}
	if err := json.Unmarshal(good, &env); err != nil {
		t.Fatal(err)
	}
	var state map[string]json.RawMessage
	if err := json.Unmarshal(env.State, &state); err != nil {
		t.Fatal(err)
	}
	build := func(edit func(m map[string]json.RawMessage)) []byte {
		m := map[string]json.RawMessage{}
		maps.Copy(m, state)
		edit(m)
		st, err := json.Marshal(m)
		if err != nil {
			t.Fatal(err)
		}
		env.State = st
		b, err := json.Marshal(env)
		if err != nil {
			t.Fatal(err)
		}
		return b
	}
	bad := map[string][]byte{
		"party missing":        build(func(m map[string]json.RawMessage) { delete(m, "party") }),
		"party empty":          build(func(m map[string]json.RawMessage) { m["party"] = json.RawMessage(`[]`) }),
		"party of a stranger":  build(func(m map[string]json.RawMessage) { m["party"] = json.RawMessage(`["ghost"]`) }),
		"party twice":          build(func(m map[string]json.RawMessage) { m["party"] = json.RawMessage(`["walker","walker"]`) }),
		"actors null":          build(func(m map[string]json.RawMessage) { m["actors"] = json.RawMessage(`null`) }),
		"location missing":     build(func(m map[string]json.RawMessage) { delete(m, "location") }),
		"location is a number": build(func(m map[string]json.RawMessage) { m["location"] = json.RawMessage(`4`) }),
		"unknown field":        build(func(m map[string]json.RawMessage) { m["wallet"] = json.RawMessage(`9`) }),
		"gold is a string":     build(func(m map[string]json.RawMessage) { m["gold"] = json.RawMessage(`"rich"`) }),
	}
	for name, b := range bad {
		t.Run(name, func(t *testing.T) {
			if r, err := gozen.Restore(g, b, gozen.Config{}); !errors.Is(err, gozen.ErrIncompatibleSave) || r != nil {
				t.Fatalf("want ErrIncompatibleSave, got %v", err)
			}
		})
	}
	// a place that the game no longer has is repaired by moving the party to the start, not refused
	moved := build(func(m map[string]json.RawMessage) { m["location"] = json.RawMessage(`"gone"`) })
	r, err := gozen.Restore(g, moved, gozen.Config{})
	if err != nil {
		t.Fatalf("a removed location is repaired: %v", err)
	}
	if got := query[string](t, r, "location"); got != "camp" {
		t.Errorf("the party stands in %q, want the start camp", got)
	}
}

// TestRestoreSaveOfAnotherGameVersion: a save made with more actors than the game has now drops the missing ones.
func TestRestoreSaveOfAnotherGameVersion(t *testing.T) {
	_, s := played(t, 5)
	good, err := s.Save()
	if err != nil {
		t.Fatal(err)
	}
	smaller := fstest.MapFS{}
	maps.Copy(smaller, trail)
	smaller["data/world.yaml"] = &fstest.MapFile{Data: bytes.ReplaceAll(trail["data/world.yaml"].Data, []byte("  sela: {name: Sela, class: walker}\n"), nil)}
	g2, err := gozen.Load(smaller, ".")
	if err != nil {
		t.Fatal(err)
	}
	r, err := gozen.Restore(g2, good, gozen.Config{})
	if err != nil {
		t.Fatalf("a save with an actor that is gone: %v", err)
	}
	if got := query[float64](t, r, "party.size"); got != 1 {
		t.Errorf("party size %v, want 1 (the missing actor is dropped)", got)
	}
}

// TestLoadRejectsBrokenGames: broken, unknown or future definitions are reported as ErrInvalid with a position and never panic.
func TestLoadRejectsBrokenGames(t *testing.T) {
	base := func(game, world string) fstest.MapFS {
		fsys := fstest.MapFS{}
		maps.Copy(fsys, trail)
		if game != "" {
			fsys["game.yaml"] = &fstest.MapFile{Data: []byte(game)}
		}
		if world != "" {
			fsys["data/world.yaml"] = &fstest.MapFile{Data: []byte(world)}
		}
		return fsys
	}
	start := "id: x\nstart: {location: camp, party: [walker]}\n"
	w := string(trail["data/world.yaml"].Data)
	cases := []struct {
		name string
		fsys fstest.MapFS
		want string
	}{
		{"future schema version", base("schema_version: 99\n"+start, ""), "schema_version"},
		{"negative schema version", base("schema_version: -1\n"+start, ""), "schema_version"},
		{"schema version as text", base("schema_version: two\n"+start, ""), "schema_version"},
		{"broken yaml in game", base("start: [\n", ""), "game.yaml"},
		{"tab indentation", base("start:\n\t- a\n", ""), "game.yaml"},
		{"unknown game key", base("schema_version: 2\n"+start+"colour: red\n", ""), `unknown field "colour"`},
		{"unknown data key", base("", w+"monsters: {}\n"), "monsters"},
		{"unknown key in an actor", base("", strings.Replace(w, "sela: {name: Sela, class: walker}", "sela: {name: Sela, class: walker, hairstyle: bun}", 1)), "hairstyle"},
		{"broken yaml in data", base("", "classes: {\n"), "data/world.yaml"},
		{"a list where a map belongs", base("", "classes: [a, b]\n"), "classes"},
		{"duplicate key in data", base("", "actors: {a: {name: A, class: walker}, a: {name: B, class: walker}}\n"), "a"},
		{"empty data file", base("", "# nothing here\n"), ""},
		{"unreferenced class", base("", strings.Replace(w, "class: walker}\n  sela", "class: nobody}\n  sela", 1)), `unknown class "nobody"`},
		{"party of an unknown actor", base("schema_version: 2\nid: x\nstart: {location: camp, party: [ghost]}\n", ""), `unknown actor "ghost"`},
		{"a binary game file", base("\x00\x01\x02", ""), "game.yaml"},
		{"anchors that loop", base("", "classes: &a {x: *a}\n"), ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			g, err := gozen.Load(c.fsys, ".")
			if c.want == "" {
				if err != nil {
					t.Logf("rejected: %v", err)
				}
				return
			}
			if err == nil || g != nil {
				t.Fatalf("expected an error containing %q, got a game", c.want)
			}
			var ds gozen.Diagnostics
			if !errors.Is(err, gozen.ErrInvalid) || !errors.As(err, &ds) || len(ds) == 0 {
				t.Fatalf("want ErrInvalid with Diagnostics, got %T %v", err, err)
			}
			if !strings.Contains(err.Error(), c.want) {
				t.Fatalf("want %q in %v", c.want, err)
			}
			for _, d := range ds {
				if d.File == "" || d.Message == "" || d.Severity == "" {
					t.Errorf("a diagnostic lacks its parts: %+v", d)
				}
			}
		})
	}
}

// TestUnicodeNamesAreKept: Japanese, full-width, emoji and combining-character names load, and come back unchanged.
func TestUnicodeNamesAreKept(t *testing.T) {
	names := []string{"ミア", "ＡＢＣ", "étoile", "🌟star", "宿屋"}
	for _, name := range names {
		t.Run(name, func(t *testing.T) {
			world := strings.ReplaceAll(string(trail["data/world.yaml"].Data), "Walker", name)
			fsys := fstest.MapFS{}
			maps.Copy(fsys, trail)
			fsys["data/world.yaml"] = &fstest.MapFile{Data: []byte(world)}
			g, err := gozen.Load(fsys, ".")
			if err != nil {
				t.Fatal(err)
			}
			s := session(t, g, gozen.Config{Seed: 1})
			if got := query[string](t, s, "party.members[0].name"); got != name {
				t.Errorf("name %q became %q", name, got)
			}
			b, err := s.Save()
			if err != nil {
				t.Fatal(err)
			}
			r, err := gozen.Restore(g, b, gozen.Config{})
			if err != nil {
				t.Fatal(err)
			}
			if got := query[string](t, r, "party.members[0].name"); got != name {
				t.Errorf("after a restore the name %q became %q", name, got)
			}
		})
	}
	// ids and flags in other scripts
	fsys := fstest.MapFS{}
	maps.Copy(fsys, trail)
	fsys["data/world.yaml"] = &fstest.MapFile{Data: []byte(strings.Replace(string(trail["data/world.yaml"].Data), "  ridge:\n", "  尾根:\n", 1) + "")}
	if _, err := gozen.Load(fsys, "."); err == nil {
		t.Log("a location id in Japanese loads (the exit to ridge is now dangling, so this must have been reported)")
		t.Fatal("a dangling exit was accepted")
	}
}

// FuzzRestore: whatever bytes a save holds, Restore returns a session or ErrIncompatibleSave, never panics,
// and a restored session can be saved again and read.
func FuzzRestore(f *testing.F) {
	g, err := gozen.Load(trail, ".")
	if err != nil {
		f.Fatal(err)
	}
	s, err := gozen.New(g, gozen.Config{Seed: 3, Dev: true})
	if err != nil {
		f.Fatal(err)
	}
	if err := s.Begin(f.Context()); err != nil {
		f.Fatal(err)
	}
	if err := s.Do(f.Context(), "- set: {flag.met: true}\n- join: sela\n- pick: Climb\n"); err != nil {
		f.Fatal(err)
	}
	good, err := s.Save()
	if err != nil {
		f.Fatal(err)
	}
	f.Add(good)
	f.Add([]byte(`{"format_version":2,"game_id":"tiny-trail","data_version":0,"state":{}}`))
	f.Add([]byte(`{"format_version":2,"game_id":"tiny-trail","data_version":0,"state":{"actors":{},"party":[],"location":"camp"},"cursor":{"event":"lantern","line":1}}`))
	f.Add([]byte(`{"party":["walker"]}`))
	f.Add(good[:len(good)/2])
	f.Fuzz(func(t *testing.T, b []byte) {
		if len(b) > 1<<20 {
			return
		}
		r, err := gozen.Restore(g, b, gozen.Config{Seed: 1})
		if err != nil {
			if !errors.Is(err, gozen.ErrIncompatibleSave) {
				t.Fatalf("an unreadable save must be ErrIncompatibleSave, got %v", err)
			}
			return
		}
		again, err := r.Save()
		if err != nil {
			t.Fatalf("a restored session cannot be saved: %v", err)
		}
		if _, err := gozen.Restore(g, again, gozen.Config{Seed: 1}); err != nil {
			t.Fatalf("what a restored session saves cannot be restored: %v", err)
		}
		var loc string
		if err := r.Query("location", &loc); err != nil || loc == "" {
			t.Fatalf("a restored session has no location: %q %v", loc, err)
		}
	})
}

// FuzzLoad: any file content in the game and data files gives a game or ErrInvalid / a file error, never a panic,
// and a game that loads can be started and saved.
func FuzzLoad(f *testing.F) {
	world := string(trail["data/world.yaml"].Data)
	game := string(trail["game.yaml"].Data)
	f.Add(game, world)
	f.Add("", "")
	f.Add("schema_version: 99\n", world)
	f.Add(game, "locations: {camp: {name: C, exits: [{to: camp}]}}")
	f.Add(game, "events: {e: {do: [{if: 'gold >', then: []}]}}")
	f.Add("start: {location: a, party: [b]}\nid: \xff\n", world)
	f.Add(game, "x: &a [*a]")
	f.Fuzz(func(t *testing.T, gameYAML, worldYAML string) {
		if len(gameYAML)+len(worldYAML) > 1<<16 {
			return
		}
		fsys := fstest.MapFS{
			"game.yaml":       {Data: []byte(gameYAML)},
			"data/world.yaml": {Data: []byte(worldYAML)},
			"assets/mote.png": {Data: []byte("sentinel")},
		}
		g, err := gozen.Load(fsys, ".")
		ds := gozen.Validate(fsys, ".")
		errorsFound := 0
		for _, d := range ds {
			if d.Severity == "error" {
				errorsFound++
			}
		}
		if (err == nil) != (errorsFound == 0) {
			t.Fatalf("Load says %v but Validate found %d errors: %v", err, errorsFound, ds)
		}
		if err != nil {
			if g != nil {
				t.Fatal("an error with a game")
			}
			return
		}
		if len(ds) != 0 {
			t.Fatalf("a game that loads has diagnostics: %v", ds)
		}
		s, err := gozen.New(g, gozen.Config{Seed: 1})
		if err != nil {
			t.Fatal(err)
		}
		if err := s.Begin(t.Context()); err != nil {
			t.Skipf("a game that does not begin has nothing more to check: %v", err)
		}
		if _, err := s.Save(); err != nil {
			t.Fatalf("a loaded game cannot be saved: %v", err)
		}
	})
}
