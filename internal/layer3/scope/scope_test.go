package scope_test

import (
	"fmt"
	"strings"
	"testing"

	"github.com/nao1215/gozen/internal/expr"
	"github.com/nao1215/gozen/internal/layer3/dungeon"
	"github.com/nao1215/gozen/internal/layer3/field"
	"github.com/nao1215/gozen/internal/layer3/scope"
	"github.com/nao1215/gozen/internal/rpg"
	"github.com/nao1215/gozen/internal/testgame"
)

const dungeonData = `
locations:
  vault2:
    name: Vault Two
    dungeon:
      generate:
        width: 11
        height: 11
        rooms: 2
        start: S
        place: [{char: ">", far: true}]
      tiles:
        S: {name: Up}
        ">": {name: Stairs, solid: "true", do: [{say: down}]}
      roamers:
        - {name: Brute, picture: x.png, count: "1", move: guard, far: 1, troops: [{troop: one}]}
`

func eval(t *testing.T, env expr.Env, src string) any {
	t.Helper()
	e, err := expr.Parse(src)
	if err != nil {
		t.Fatal(err)
	}
	v, err := e.Eval(env)
	if err != nil {
		t.Fatalf("%s: %v", src, err)
	}
	return v
}

// TestEnvNamesInTheField: menu lists the field's entries as objects, the other names are read live from the last result and the state.
func TestEnvNamesInTheField(t *testing.T) {
	db := testgame.Load(t, nil)
	st := testgame.State(db, 1)
	last := &scope.Last{}
	env := expr.Chain{scope.Env(db, st, last), st.Env()}
	menu := testgame.As[[]any](t, eval(t, env, "menu"))
	if len(menu) != len(field.Entries(db, st)) {
		t.Fatalf("menu %#v", menu)
	}
	first := testgame.As[expr.Map](t, menu[0])
	if first["kind"] != "talk" || !strings.Contains(testgame.As[string](t, first["label"]), "Courier") || first["new"] != false {
		t.Errorf("first entry %v", first)
	}
	if got := eval(t, env, "menu[len(menu) - 1].kind"); got != "menu" {
		t.Errorf("the last entry is the menu, got %v", got)
	}
	// result, over, cleared are read when asked, so the owner's later writes show
	if eval(t, env, "result") != "" || eval(t, env, "over") != false || eval(t, env, "cleared") != false {
		t.Error("nothing has happened yet")
	}
	last.Result, last.Over, last.Cleared = "win", true, true
	if eval(t, env, "result") != "win" || eval(t, env, "over") != true || eval(t, env, "cleared") != true {
		t.Error("the names must follow the last result as it changes")
	}
	st.Moved = true
	if eval(t, env, "moved") != true {
		t.Error("moved follows the state")
	}
	if eval(t, env, "completed") != false {
		t.Error("a place without a dungeon is never completed")
	}
	if got := testgame.As[[]any](t, eval(t, env, "map")); len(got) != 0 {
		t.Errorf("a place without a dungeon has an empty map, got %v", got)
	}
	if got := testgame.As[[]any](t, eval(t, env, "roamers")); len(got) != 0 {
		t.Errorf("roamers %v", got)
	}
	if got := eval(t, env, "cell(0, 0)"); got != nil {
		t.Errorf("a cell outside a dungeon is nil, got %v", got)
	}
	if _, ok := scope.Env(db, st, last).Lookup("nonsense"); ok {
		t.Error("an unknown name must not resolve")
	}
}

func TestEnvNamesInADungeon(t *testing.T) {
	db := testgame.Load(t, map[string]string{"data/v.yaml": dungeonData})
	st := testgame.State(db, 3)
	st.Location = "vault2"
	dungeon.DiveAt(db, st, "vault2", 1, "", "")
	last := &scope.Last{}
	env := expr.Chain{scope.Env(db, st, last), st.Env()}
	rows := testgame.As[[]any](t, eval(t, env, "map"))
	if len(rows) != 11 || len([]rune(testgame.As[string](t, rows[0]))) != 11 {
		t.Fatalf("map %v", rows)
	}
	sx, sy := -1, -1
	for y, r := range rows {
		if x := strings.IndexRune(testgame.As[string](t, r), '>'); x >= 0 {
			sx, sy = x, y
		}
	}
	if sx < 0 {
		t.Fatalf("the stairs are on the map: %v", rows)
	}
	c := testgame.As[expr.Map](t, eval(t, env, fmt.Sprintf("cell(%d, %d)", sx, sy)))
	if c["char"] != ">" || c["solid"] != true || c["passable"] != false || c["raw"] != ">" {
		t.Errorf("stairs cell %v", c)
	}
	wall := testgame.As[expr.Map](t, eval(t, env, "cell(0, 0)"))
	if wall["char"] != "#" || wall["passable"] != false {
		t.Errorf("wall %v", wall)
	}
	outside := testgame.As[expr.Map](t, eval(t, env, "cell(-5, 99)"))
	if outside["shown"] != false || outside["passable"] != false {
		t.Errorf("outside the map: %v", outside)
	}
	if eval(t, env, fmt.Sprintf("cell(%d, %d).seen", st.Pos.X, st.Pos.Y)) != true {
		t.Error("the start cell has been seen")
	}
	// a cell function needs two numbers
	for _, src := range []string{"cell(1)", "cell('a', 1)", "cell(1, nil)"} {
		e := expr.Chain{scope.Env(db, st, last)}
		p, perr := expr.Parse(src)
		if perr != nil {
			t.Fatal(perr)
		}
		v, err := p.Eval(e)
		if src == "cell(1, nil)" {
			// nil converts to 0
			if err != nil || v == nil {
				t.Errorf("%s: %v %v", src, v, err)
			}
			continue
		}
		if err == nil {
			t.Errorf("%s must be an error, got %v", src, v)
		}
	}
	// roamers carry their kind, name, troop and place
	st.Roamers = []rpg.Roamer{{Kind: 0, Troop: "one", X: 2, Y: 1}, {Kind: 9, Troop: "one", X: 1, Y: 1}, {Kind: -1}}
	rs := testgame.As[[]any](t, eval(t, env, "roamers"))
	if len(rs) != 3 {
		t.Fatalf("roamers %v", rs)
	}
	r0, r1 := testgame.As[expr.Map](t, rs[0]), testgame.As[expr.Map](t, rs[1])
	if r0["name"] != "Brute" || r0["troop"] != "one" || r0["x"] != 2.0 || r0["y"] != 1.0 {
		t.Errorf("roamer 0 %v", r0)
	}
	if r1["name"] != "" {
		t.Errorf("a roamer of an unknown kind has no name: %v", r1)
	}
	if eval(t, env, "completed") != false {
		t.Error("an unwalked floor is not completed")
	}
}

func TestKindAndResultNames(t *testing.T) {
	kinds := map[field.Kind]string{field.Talk: "talk", field.Do: "do", field.Search: "search", field.Exit: "exit", field.Menu: "menu", field.Kind(99): ""}
	for k, want := range kinds {
		if got := scope.KindName(k); got != want {
			t.Errorf("KindName(%d) = %q, want %q", k, got, want)
		}
	}
	results := map[int]string{0: "", 1: "win", 2: "lose", 3: "escape", 4: "", -1: ""}
	for n, want := range results {
		if got := scope.ResultName(n); got != want {
			t.Errorf("ResultName(%d) = %q, want %q", n, got, want)
		}
	}
}

func TestProfileNamesFollowTheState(t *testing.T) {
	db := testgame.Load(t, map[string]string{"game.yaml": "schema_version: 4\nid: p\n" + testgame.Game, "data/p.yaml": `
profiles:
  keeper:
    name: Keeper page
    picture: x.png
    when: "flag.met"
`})
	st := testgame.State(db, 1)
	env := scope.Env(db, st, &scope.Last{})
	p := testgame.As[expr.Map](t, eval(t, env, "profile"))
	page, ok := p["keeper"].(expr.Map)
	if !ok || page["unlocked"] != false || page["name"] != "Keeper page" {
		t.Fatalf("profile %v", p)
	}
	st.Flags["met"] = true
	if eval(t, env, "profile.keeper.unlocked") != true {
		t.Error("the page opens when its when holds")
	}
}
