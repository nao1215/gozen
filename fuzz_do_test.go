package gozen_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/nao1215/gozen"
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
