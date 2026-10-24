package session_test

import (
	"errors"
	"strings"
	"testing"

	gozen "github.com/nao1215/gozen/internal/session"
)

func TestQueryOutTypes(t *testing.T) {
	g := load(t)
	s := session(t, g, gozen.Config{Seed: 1, Dev: true})
	if err := s.Do(t.Context(), "- join: sela\n- set: {var.text: \"'hi'\", flag.on: true}\n"); err != nil {
		t.Fatal(err)
	}
	var f float64
	var str string
	var b bool
	var anyv any
	if err := s.Query("gold", &f); err != nil || f != 2 {
		t.Errorf("float64 %v %v", f, err)
	}
	if err := s.Query("location", &str); err != nil || str != "camp" {
		t.Errorf("string %q %v", str, err)
	}
	if err := s.Query("flag.on", &b); err != nil || !b {
		t.Errorf("bool %v %v", b, err)
	}
	if err := s.Query("party.members", &anyv); err != nil {
		t.Fatal(err)
	}
	if list, ok := anyv.([]any); !ok || len(list) != 2 {
		t.Errorf("a list of objects: %#v", anyv)
	}
	// a number into a string, a string into a bool: the type must fit
	if err := s.Query("gold", &str); err == nil {
		t.Error("a number into a string")
	}
	if err := s.Query("location", &b); err == nil {
		t.Error("a string into a bool")
	}
	var n int
	if err := s.Query("gold", &n); err != nil || n != 2 {
		t.Errorf("a number into an int %v %v", n, err)
	}
	var names []string
	if err := s.Query("party.members[0].equip", &names); err == nil {
		t.Log("equip is an object, not a list")
	}
	var obj struct{ Hp, Mhp float64 }
	if err := s.Query("party.members[0]", &obj); err != nil || obj.Mhp <= 0 {
		t.Errorf("an object into a struct: %+v %v", obj, err)
	}
	for _, bad := range []any{nil, 3, f, (*int)(nil)} {
		if err := s.Query("gold", bad); err == nil {
			t.Errorf("out %#v must be a non-nil pointer", bad)
		}
	}
	for _, src := range []string{"", "gold +", "unknown_name", "gold.x", "location.x", "rand(1, 2, 3)", "menu[99].kind"} {
		var v any
		err := s.Query(src, &v)
		if err == nil || !errors.Is(err, gozen.ErrInvalid) {
			t.Errorf("Query(%q) = %v, want ErrInvalid", src, err)
		}
	}
	// a function is not a value
	var v any
	if err := s.Query("rand", &v); err == nil || strings.Contains(err.Error(), "panic") {
		t.Errorf("a function value: %v", err)
	}
	if err := s.Query("flag", &v); err != nil {
		t.Errorf("a flag object lists its names: %v", err)
	}
}

func TestExploreRefusals(t *testing.T) {
	g := load(t)
	s := session(t, g, gozen.Config{Seed: 1})
	if _, err := gozen.Explore(nil, s, "- gold: 1\n", gozen.ExploreOptions{}); err == nil { //nolint:staticcheck // A nil context is the case under test.
		t.Error("a nil context")
	}
	if _, err := gozen.Explore(t.Context(), nil, "- gold: 1\n", gozen.ExploreOptions{}); err == nil {
		t.Error("a nil session")
	}
	ctx := t.Context()
	if _, err := gozen.Explore(ctx, s, "gold: 1", gozen.ExploreOptions{}); !errors.Is(err, gozen.ErrInvalid) {
		t.Errorf("a script that is not a list: %v", err)
	}
	if _, err := gozen.Explore(ctx, s, "- gold: 1\n", gozen.ExploreOptions{MaxPaths: -1}); err != nil {
		t.Logf("negative limit: %v", err)
	}
	// a session is not changed by exploring it
	before := query[int](t, s, "gold")
	paths, err := gozen.Explore(ctx, s, "- gold: 50\n", gozen.ExploreOptions{Seed: 3})
	if err != nil || len(paths) != 1 {
		t.Fatal(err, len(paths))
	}
	if query[int](t, s, "gold") != before {
		t.Error("Explore changed the session")
	}
	if got := query[int](t, paths[0].End, "gold"); got != before+50 {
		t.Errorf("the end state has the gold: %d", got)
	}
}

func TestGameYAMLPaths(t *testing.T) {
	g := load(t)
	for _, p := range []string{"start", "start.party", "locations", "locations.camp", "locations.camp.exits", "locations.camp.exits[0]", "actors.walker.name", "events.lantern.do[0].choice[1].label"} {
		if b, err := g.YAML(p); err != nil || len(b) == 0 {
			t.Errorf("%q: %q %v", p, b, err)
		}
	}
	for _, p := range []string{"", ".", "nothing", "locations.nothing", "locations.camp.nothing", "locations.camp.exits[x]", "locations.camp.exits[9]", "start.party[5]", "locations.camp.name.deeper", "events.lantern.do[0].choice[9]", "start..party", "[0]"} {
		if _, err := g.YAML(p); !errors.Is(err, gozen.ErrNotFound) {
			t.Errorf("%q: want ErrNotFound, got %v", p, err)
		}
	}
	if len(g.IDs("nothing")) != 0 {
		t.Error("an unknown section has no ids")
	}
	ids := g.IDs("locations")
	ids[0] = "changed"
	if g.IDs("locations")[0] == "changed" {
		t.Error("IDs returns a copy")
	}
}
