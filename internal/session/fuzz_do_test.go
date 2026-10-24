package session_test

import (
	"bytes"
	"context"
	"errors"
	"testing"
	"time"

	gozen "github.com/nao1215/gozen/internal/session"
)

// FuzzDo: any text given to Do is rejected as ErrInvalid or runs; it never panics, and what ran can be saved
// and restored (ADR 0021: the commands are the only way to change a session).
func FuzzDo(f *testing.F) {
	g, err := gozen.Load(trail, ".")
	if err != nil {
		f.Fatal(err)
	}
	for _, s := range []string{
		"- gold: 5\n",
		"- set: {flag.a: true, var.n: 'rand(10)'}\n",
		"- join: sela\n- leave: sela\n",
		"- pick: Climb\n- pick: Back\n",
		"- battle: motes\n",
		"- call: lantern\n",
		"- if: 'gold > 1'\n  then: [{gold: 1}]\n  else: [{gold: -1}]\n",
		"- choice: [{label: A, do: [{gold: 1}]}, {label: B, do: []}]\n",
		"- say: 'hello {gold}'\n",
		"- heal: all\n- hurt: {who: all, amount: '1000'}\n",
		"- move: ridge\n",
		"- gold: -1000000\n",
		"- exp: 99999999\n",
		"- wait: 1\n",
		"- act: {actor: walker, skill: attack}\n",
		"- []\n",
		"- null\n",
		"- 1\n",
		"- {unknown: 1}\n",
		"- gold: 9223372036854775807\n- gold: 9223372036854775807\n",
	} {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, script string) {
		if len(script) > 8192 {
			return
		}
		s, err := gozen.New(g, gozen.Config{Seed: 1, Dev: true})
		if err != nil {
			t.Fatal(err)
		}
		ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
		defer cancel()
		if err := s.Begin(ctx); err != nil {
			t.Fatalf("begin: %v", err)
		}
		err = s.Do(ctx, script)
		if err != nil && ctx.Err() != nil {
			return // a long run is not a crash
		}
		if err != nil && !errors.Is(err, gozen.ErrInvalid) {
			// run problems (an expression that fails) are reported joined; they are errors, not panics
			t.Logf("run problem: %v", err)
		}
		b, err := s.Save()
		if err != nil {
			t.Fatalf("a session cannot be saved after Do: %v", err)
		}
		r, err := gozen.Restore(g, b, gozen.Config{Seed: 1})
		if err != nil {
			t.Fatalf("what Do left cannot be restored: %v\n%s", err, b)
		}
		if again, err := r.Save(); err != nil || !bytes.Equal(again, b) {
			t.Fatalf("save -> restore -> save changed the bytes (%v):\n%s\n%s", err, b, again)
		}
		var loc string
		if err := r.Query("location", &loc); err != nil {
			t.Fatal(err)
		}
	})
}

// FuzzQuery: any expression given to Query is an error or a value, never a panic.
func FuzzQuery(f *testing.F) {
	g, err := gozen.Load(trail, ".")
	if err != nil {
		f.Fatal(err)
	}
	s, err := gozen.New(g, gozen.Config{Seed: 1})
	if err != nil {
		f.Fatal(err)
	}
	if err := s.Begin(f.Context()); err != nil {
		f.Fatal(err)
	}
	for _, q := range []string{"gold", "party", "party.members[0].hp", "menu", "map", "cell(0, 0)", "pos", "actor.walker", "actor['walker'].hp", "roamers", "result", "rand(5)", "pages('x')", "chose.a", "item.none", "flag[1]", "var['a'] + 1", "location + 1", "len(menu)"} {
		f.Add(q)
	}
	f.Fuzz(func(t *testing.T, q string) {
		if len(q) > 4096 {
			return
		}
		var v any
		err := s.Query(q, &v)
		if err != nil && !errors.Is(err, gozen.ErrInvalid) {
			t.Logf("query problem: %v", err)
		}
	})
}

// FuzzCheckAndExplore: checking a script never panics, and a script that passes the check can be explored on every branch
// without a panic, leaving the session it was given as it was.
func FuzzCheckAndExplore(f *testing.F) {
	g, err := gozen.Load(trail, ".")
	if err != nil {
		f.Fatal(err)
	}
	for _, s := range []string{
		"- gold: 5\n",
		"- choice: [{label: A, do: [{gold: 1}]}, {label: B, do: []}]\n",
		"- battle: motes\n  on_win: [{gold: 1}]\n  on_lose: [{gold: -1}]\n",
		"- call: lantern\n",
		"- move: ridge\n- move: camp\n",
		"gold + 1",
		"gld",
		"- if: 'gold > 1'\n  then: [{gold: 1}]\n",
		"",
		"- [",
		"- {gold: [1]}\n",
	} {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, script string) {
		if len(script) > 4096 {
			return
		}
		ds := g.Check(script)
		for _, d := range ds {
			if d.Message == "" {
				t.Fatalf("a diagnostic with no message: %+v", d)
			}
		}
		if len(ds) > 0 {
			return
		}
		s, err := gozen.New(g, gozen.Config{Seed: 1})
		if err != nil {
			t.Fatal(err)
		}
		before, err := s.Save()
		if err != nil {
			t.Fatal(err)
		}
		ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
		defer cancel()
		if _, err := gozen.Explore(ctx, s, script, gozen.ExploreOptions{MaxPaths: 20, MaxSteps: 500}); err != nil {
			t.Logf("explore: %v", err)
		}
		after, err := s.Save()
		if err != nil {
			t.Fatal(err)
		}
		if string(before) != string(after) {
			t.Fatalf("Explore changed the session it was given:\n%s\n%s", before, after)
		}
	})
}

// FuzzDoWithADungeonAndAShop: the same promise as FuzzDo on a game with a dungeon, a shop and menus to pick from.
func FuzzDoWithADungeonAndAShop(f *testing.F) {
	g, err := gozen.Load(trail, ".", more)
	if err != nil {
		f.Fatal(err)
	}
	for _, s := range []string{
		"- set: {location: cave}\n- step: 2\n- turn: left\n- step: 1\n",
		"- set: {location: cave}\n- walk: G\n",
		"- set: {location: cave, pos.x: 3, pos.y: 1, pos.facing: south, steps: 99}\n- step: 5\n",
		"- gold: 99\n- buy: {shop: stall, item: herb, count: 3}\n- sell: {shop: stall, item: herb}\n- use: herb\n",
		"- pick: Climb\n- pick: Back\n- pick: Nowhere\n",
		"- act: {actor: walker, skill: guard}\n- battle: motes\n",
		"- equip: {actor: walker, item: herb}\n",
		"- join: sela\n- set: {party: [sela, walker]}\n- leave: sela\n",
		"- call: show\n- call: found\n",
		"- set: {actor.walker.tactics: {mode: manual, rows: []}}\n",
		"- set: {actor.walker.level: 99, actor.walker.hp: -1}\n",
	} {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, script string) {
		if len(script) > 4096 {
			return
		}
		s, err := gozen.New(g, gozen.Config{Seed: 2, Dev: true})
		if err != nil {
			t.Fatal(err)
		}
		ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
		defer cancel()
		if err := s.Begin(ctx); err != nil {
			t.Fatalf("begin: %v", err)
		}
		if err := s.Do(ctx, script); err != nil && ctx.Err() != nil {
			return
		}
		b, err := s.Save()
		if err != nil {
			t.Fatalf("a session cannot be saved after Do: %v", err)
		}
		// the same seed and the same commands give the same state
		twin, err := gozen.New(g, gozen.Config{Seed: 2, Dev: true})
		if err != nil {
			t.Fatal(err)
		}
		if err := twin.Begin(ctx); err != nil {
			t.Fatalf("begin: %v", err)
		}
		if err := twin.Do(ctx, script); err != nil && ctx.Err() != nil {
			return
		}
		if tb, err := twin.Save(); err != nil || !bytes.Equal(tb, b) {
			t.Fatalf("the same seed and commands gave different states (%v):\n%s\n%s", err, b, tb)
		}
		r, err := gozen.Restore(g, b, gozen.Config{Seed: 2})
		if err != nil {
			t.Fatalf("what Do left cannot be restored: %v\n%s", err, b)
		}
		if again, err := r.Save(); err != nil || !bytes.Equal(again, b) {
			t.Fatalf("save -> restore -> save changed the bytes (%v):\n%s\n%s", err, b, again)
		}
		for _, q := range []string{"location", "gold", "party", "menu", "map", "pos", "roamers"} {
			var v any
			if err := s.Query(q, &v); err != nil {
				t.Fatalf("Query(%q) after Do: %v", q, err)
			}
		}
	})
}
