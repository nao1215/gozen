package gamedata

import (
	"fmt"
	"reflect"
	"slices"
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

// TestCommandWalkerCoverage plants a marked command in every []Command field that can be
// reached from DB, found by reflection, and requires the one walk over the command lists
// (eachCommandList with Command.branches) and its readers to reach each of them: EachCommand
// visits the command, StateUses indexes its set and SourceTexts lists its line. A new
// definition that holds commands, or a new branch of a command, fails here until
// eachCommandList or Command.branches knows it. Dango runs this test (decision "walker").
func TestCommandWalkerCoverage(t *testing.T) {
	always := mustIf(t)
	listType := reflect.TypeFor[[]Command]()
	reaches := map[reflect.Type]bool{}
	var reach func(typ reflect.Type) bool
	reach = func(typ reflect.Type) bool {
		if typ == listType {
			return true
		}
		if known, ok := reaches[typ]; ok {
			return known
		}
		reaches[typ] = false // a type met again inside itself adds nothing
		found := false
		switch typ.Kind() { //nolint:exhaustive // Only the kinds that hold other values lead to commands.
		case reflect.Pointer, reflect.Slice, reflect.Array, reflect.Map:
			found = reach(typ.Elem())
		case reflect.Struct:
			for f := range typ.Fields() {
				found = found || f.IsExported() && reach(f.Type)
			}
		}
		reaches[typ] = found
		return found
	}
	var planted []string // the place of each list, by its marker's number
	var plant func(v reflect.Value, path string, nested bool)
	plant = func(v reflect.Value, path string, nested bool) {
		typ := v.Type()
		if !reach(typ) {
			return
		}
		if typ == listType {
			n := len(planted)
			planted = append(planted, path)
			cmds := []Command{{Say: &Say{Text: fmt.Sprintf("walker %d", n)}, SetV: Assignments{fmt.Sprintf("flag.walker%d", n): always}}}
			if !nested { // the lists inside a command, one level deep
				plant(reflect.ValueOf(&cmds[0]).Elem(), path+"[0]", true)
			}
			v.Set(reflect.ValueOf(cmds))
			return
		}
		switch typ.Kind() { //nolint:exhaustive // reach admits only these kinds.
		case reflect.Pointer:
			v.Set(reflect.New(typ.Elem()))
			plant(v.Elem(), path, nested)
		case reflect.Struct:
			for i := range typ.NumField() {
				if f := typ.Field(i); f.IsExported() {
					plant(v.Field(i), path+"."+f.Name, nested)
				}
			}
		case reflect.Slice:
			v.Set(reflect.MakeSlice(typ, 1, 1))
			plant(v.Index(0), path+"[0]", nested)
		case reflect.Map:
			if typ.Key().Kind() != reflect.String {
				t.Fatalf("%s: a map of commands keyed by %s; teach this test its key", path, typ.Key())
			}
			elem := reflect.New(typ.Elem()).Elem()
			plant(elem, path+".k", nested)
			v.Set(reflect.MakeMap(typ))
			v.SetMapIndex(reflect.ValueOf("k").Convert(typ.Key()), elem)
		default:
			t.Fatalf("%s: commands inside a %s; teach this test to fill it", path, typ.Kind())
		}
	}
	db := &DB{}
	plant(reflect.ValueOf(db).Elem(), "DB", false)
	if len(planted) < 20 {
		t.Fatalf("only %d command lists were found by reflection: %v", len(planted), planted)
	}
	marker := func(c *Command) int {
		n := -1
		if c.Say != nil {
			if _, err := fmt.Sscanf(c.Say.Text, "walker %d", &n); err != nil {
				return -1
			}
		}
		return n
	}
	walked := make([]int, len(planted))
	var walk func(cs []Command)
	walk = func(cs []Command) {
		for i := range cs {
			if n := marker(&cs[i]); n >= 0 {
				walked[n]++
			}
			cs[i].branches(func(_ string, _ int, cs []Command) { walk(cs) })
		}
	}
	names := map[string]bool{}
	db.eachCommandList(func(l commandList) bool {
		walk(l.cmds)
		if l.name != "" {
			if names[l.name] {
				t.Errorf("two command lists are named %q", l.name)
			}
			names[l.name] = true
			if got := db.NamedCommands(l.name); len(got) == 0 || &got[0] != &l.cmds[0] {
				t.Errorf("NamedCommands(%q) is not the list at %s", l.name, l.where)
			}
		}
		return false
	})
	visited := make([]int, len(planted))
	db.EachCommand(func(c *Command) {
		if n := marker(c); n >= 0 {
			visited[n]++
		}
	})
	uses := db.StateUses()
	var texts []string
	for _, st := range db.SourceTexts() {
		texts = append(texts, st.Text)
	}
	for n, path := range planted {
		if walked[n] != 1 {
			t.Errorf("%s: eachCommandList and Command.branches reach it %d times, want once", path, walked[n])
		}
		if visited[n] != 1 {
			t.Errorf("%s: EachCommand visits it %d times, want once", path, visited[n])
		}
		if u := uses[fmt.Sprintf("flag.walker%d", n)]; u == nil || len(u.Sets) != 1 {
			t.Errorf("%s: StateUses does not index its set once: %+v", path, u)
		}
		if !slices.Contains(texts, fmt.Sprintf("walker %d", n)) {
			t.Errorf("%s: SourceTexts does not list its line", path)
		}
	}
}
