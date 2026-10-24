package session_test

import (
	"testing"
	"testing/fstest"

	gozen "github.com/nao1215/gozen/internal/session"
	"github.com/nao1215/gozen/internal/testgame"
)

const hookEvents = `
events:
  prepare: {do: [{set: {var.prepared: "var.prepared + 1"}}]}
  finish: {do: [{set: {var.finished: "var.finished + 1", var.seen: "var.branch"}}]}
  fight:
    do:
      - battle: one
        on_win: [{set: {var.branch: 1}}]
        on_lose: [{set: {var.branch: 2}}]
        on_escape: [{set: {var.branch: 3}}]
`

func hookGame(t *testing.T) *gozen.Game {
	t.Helper()
	fsys := fstest.MapFS{
		"game.yaml":       {Data: []byte("schema_version: 4\nid: hooks\n" + testgame.Game + "  before: prepare\n  after: finish\n")},
		"data/main.yaml":  {Data: []byte(testgame.Data)},
		"data/hooks.yaml": {Data: []byte(hookEvents)},
		"assets/x.png":    {Data: []byte("stub")},
	}
	g, err := gozen.Load(fsys, ".")
	if err != nil {
		t.Fatal(err)
	}
	return g
}

// TestBattleHooksThroughASession (ADR 0013): before runs once, just before the battle is shown, however often the request is asked for;
// after runs once the result branch has run; a result decided by the caller goes the same way as one fought.
func TestBattleHooksThroughASession(t *testing.T) {
	g := hookGame(t)
	for name, answer := range map[string]int{"won": gozen.Won, "fled": gozen.Fled, "auto": gozen.Auto} {
		t.Run(name, func(t *testing.T) {
			s := mustNew(t, g, gozen.Config{Seed: 1})
			if err := s.Start("- call: fight\n"); err != nil {
				t.Fatal(err)
			}
			for range 3 {
				if q := s.Next(); q.Kind != "battle" {
					t.Fatalf("%+v", q)
				}
			}
			if got := query[int](t, s, "var.prepared"); got != 1 {
				t.Errorf("before ran %d times, want once", got)
			}
			if got := query[int](t, s, "var.finished"); got != 0 {
				t.Errorf("after ran before the battle was decided: %d", got)
			}
			s.Answer(answer)
			for q := s.Next(); q.Kind != ""; q = s.Next() {
			}
			if query[int](t, s, "var.prepared") != 1 || query[int](t, s, "var.finished") != 1 {
				t.Errorf("prepared %d finished %d", query[int](t, s, "var.prepared"), query[int](t, s, "var.finished"))
			}
		})
	}
}
