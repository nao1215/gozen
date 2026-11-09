package gamedata

import (
	"testing"

	"github.com/nao1215/gozen/internal/base/expr"
)

func TestUsesFindsVerbInBranches(t *testing.T) {
	db := &DB{
		Events:    map[string]*Event{"a": {Do: []Command{{If: mustIf(t), Then: []Command{{Choice: []ChoiceOption{{Do: []Command{{SPSuggest: "sp"}}}}}}}}}},
		Locations: map[string]*Location{"camp": {}},
	}
	if !db.Uses("sp_suggest") {
		t.Error("if の then の choice の 中の sp_suggest を 見つけない")
	}
	if db.Uses("stone_suggest") {
		t.Error("無い stone_suggest を 見つけた")
	}
}

func mustIf(t *testing.T) Expr {
	t.Helper()
	p, err := expr.Parse("true")
	if err != nil {
		t.Fatal(err)
	}
	return Expr{p}
}

func TestEachCommandVisitsAuthoredScreenActions(t *testing.T) {
	db := &DB{Game: Game{Screens: map[string]*AuthorScreen{
		"missing": nil,
		"desk":    {Entries: []ScreenEntry{{Do: []Command{{Choice: []ChoiceOption{{Do: []Command{{Clear: true}}}}}}}}},
	}}}
	if !db.Uses("clear") {
		t.Fatal("clear in an authored screen was omitted")
	}
	visited := 0
	db.EachCommand(func(c *Command) {
		visited++
		if c.Clear {
			c.Clear = false
		}
	})
	if visited != 2 || db.Uses("clear") {
		t.Fatalf("command walk did not visit mutable screen actions once: %d", visited)
	}
}

func TestEachCommandVisitsRoamerTransitions(t *testing.T) {
	for _, phase := range []string{"appear", "leave", "meet"} {
		t.Run(phase, func(t *testing.T) {
			commands := []Command{{Choice: []ChoiceOption{{Do: []Command{{Clear: true}}}}}}
			roamer := Roamer{}
			switch phase {
			case "appear":
				roamer.OnAppear = commands
			case "leave":
				roamer.OnLeave = commands
			case "meet":
				roamer.Meet = commands
			}
			db := &DB{Locations: map[string]*Location{"cave": {Dungeon: &Dungeon{Roamers: []Roamer{roamer}}}}}
			if !db.Uses("clear") {
				t.Fatal("roamer transition was omitted from feature inspection")
			}
			visits := 0
			db.EachCommand(func(c *Command) {
				visits++
				if c.Clear {
					c.Clear = false
				}
			})
			if visits != 2 || db.Uses("clear") {
				t.Fatalf("roamer commands were not mutable or visited once: %d", visits)
			}
		})
	}
}
