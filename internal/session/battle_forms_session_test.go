package session_test

import (
	"context"
	"testing"
	"testing/fstest"

	gozen "github.com/nao1215/gozen/internal/session"
	"github.com/nao1215/gozen/internal/testgame"
)

const sessionForms = `
skills:
  reform: {name: Reform, scope: self, effects: [{transform: second, transform_hp: ratio}]}
  join: {name: Join, scope: self, effects: [{merge: {into: second, from: [first], count: 2, hp: sum}}]}
enemies:
  first:
    name: First
    image: x.png
    stats: {mhp: 40, mmp: 0, atk: 1, def: 1, mat: 1, mdf: 1, agi: 99, luk: 1}
    ai: [{use: reform, when: "allies.alive == 1", priority: 5}, {use: join, when: "allies.kind_first >= 2", priority: 5}]
  second:
    name: Second
    image: x.png
    stats: {mhp: 80, mmp: 0, atk: 1, def: 1, mat: 1, mdf: 1, agi: 99, luk: 1}
    ai: [{use: attack}]
troops:
  form: {members: [first]}
  merge: {members: [first, first]}
`

func TestBattleFormsThroughSessionAndExplore(t *testing.T) {
	fsys := fstest.MapFS{
		"game.yaml":       {Data: []byte("schema_version: 7\nid: forms\n" + testgame.Game)},
		"data/main.yaml":  {Data: []byte(testgame.Data)},
		"data/forms.yaml": {Data: []byte(sessionForms)},
		"assets/x.png":    {Data: []byte("fixture")},
	}
	g, err := gozen.Load(fsys, ".")
	if err != nil {
		t.Fatal(err)
	}
	for _, troop := range []string{"form", "merge"} {
		t.Run(troop, func(t *testing.T) {
			var steps []string
			s, err := gozen.New(g, gozen.Config{Seed: 11, OnRequest: func(r gozen.Request) {
				if r.Kind == "step" {
					steps = append(steps, r.Text)
				}
			}})
			if err != nil {
				t.Fatal(err)
			}
			if err := s.Do(context.Background(), "- battle: "+troop+"\n"); err != nil {
				t.Fatal(err)
			}
			if len(steps) == 0 {
				t.Fatal("session did not report battle steps")
			}
			var seen bool
			if err := s.Query("bestiary.second.seen", &seen); err != nil || !seen {
				t.Fatalf("seen=%v err=%v", seen, err)
			}
			b, err := s.Save()
			if err != nil {
				t.Fatal(err)
			}
			restored, err := gozen.Restore(g, b, gozen.Config{Seed: 11})
			if err != nil {
				t.Fatal(err)
			}
			if err := restored.Query("bestiary.second.seen", &seen); err != nil || !seen {
				t.Fatalf("restored seen=%v err=%v", seen, err)
			}
			fresh, err := gozen.New(g, gozen.Config{Seed: 11})
			if err != nil {
				t.Fatal(err)
			}
			paths, err := gozen.Explore(t.Context(), fresh, "- battle: "+troop+"\n", gozen.ExploreOptions{NoArrive: true})
			if err != nil || len(paths) == 0 {
				t.Fatalf("explore paths=%d err=%v", len(paths), err)
			}
		})
	}
}
