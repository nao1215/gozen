package expr

import (
	"strings"
	"testing"

	"github.com/goccy/go-yaml"
)

type actor struct{ hp, mhp float64 }

func (a *actor) Field(name string) (any, bool) {
	switch name {
	case "hp":
		return a.hp, true
	case "mhp":
		return a.mhp, true
	case "hp_rate":
		return a.hp / a.mhp, true
	}
	return nil, false
}

func TestEval(t *testing.T) {
	env := Map{
		"self":    &actor{hp: 20, mhp: 80},
		"turn":    3.0,
		"name":    "ヒカリ",
		"flag":    Map{"met": true},
		"enemies": []any{&actor{hp: 5, mhp: 10}},
		"double":  Func(func(a []any) (any, error) { f, _ := ToNumber(a[0]); return f * 2, nil }),
	}
	tests := []struct {
		src  string
		want any
	}{
		{"1 + 2 * 3", 7.0},
		{"(1 + 2) * 3", 9.0},
		{"-2 + 5", 3.0},
		{"7 % 3", 1.0},
		{"-1 % 3", 2.0},
		{"10 / 0", 0.0},
		{"self.hp_rate < 0.3", true},
		{"self.hp_rate < 0.3 && turn % 3 == 0", true},
		{"self.hp > 50 or turn == 3", true},
		{"not flag.met", false},
		{"!flag.met || 1", true},
		{"turn >= 3 ? 'late' : 'early'", "late"},
		{"name + 'は' + turn", "ヒカリは3"},
		{"max(1, self.hp, 4)", 20.0},
		{"min(3, 1)", 1.0},
		{"clamp(150, 0, 99)", 99.0},
		{"floor(2.7) + ceil(0.2)", 3.0},
		{"double(4)", 8.0},
		{"enemies[0].hp", 5.0},
		{"enemies[5] == nil", true},
		{"len(enemies)", 1.0},
		{"has(flag, 'met')", true},
		{"has(flag, 'nope')", false},
		{"'a' < 'b'", true},
		{"true == 1", false},
		{"nil == nil", true},
		{"self == self", true},
		{"0.5 * 4", 2.0},
	}
	for _, tt := range tests {
		e, err := Parse(tt.src)
		if err != nil {
			t.Errorf("Parse(%q): %v", tt.src, err)
			continue
		}
		got, err := e.Eval(env)
		if err != nil {
			t.Errorf("Eval(%q): %v", tt.src, err)
			continue
		}
		if got != tt.want {
			t.Errorf("Eval(%q) = %#v, want %#v", tt.src, got, tt.want)
		}
	}
}

func TestShortCircuit(t *testing.T) {
	// the right side would fail (unknown name) if it were evaluated
	for _, src := range []string{"false && nope", "true || nope", "false ? nope : 1"} {
		if _, err := mustParse(src).Eval(Map{}); err != nil {
			t.Errorf("%s: %v", src, err)
		}
	}
}

func TestErrors(t *testing.T) {
	for _, src := range []string{"1 +", "(1", "a..b", "'open", "1 $ 2", "f(1,", "a ? b", ""} {
		if _, err := Parse(src); err == nil {
			t.Errorf("Parse(%q) succeeded", src)
		}
	}
	for _, src := range []string{"nope", "self.nope", "1 - 'a'", "turn(1)", "self.hp.x"} {
		if _, err := mustParse(src).Eval(Map{"self": &actor{1, 1}, "turn": 1.0}); err == nil {
			t.Errorf("Eval(%q) succeeded", src)
		}
	}
}

func TestNames(t *testing.T) {
	got := mustParse("max(self.hp, b.def) + rand(3) * self.mp").Names()
	want := []string{"self", "b", "rand"}
	if len(got) != len(want) {
		t.Fatalf("Names = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("Names = %v, want %v", got, want)
		}
	}
}

func TestRefs(t *testing.T) {
	got := mustParse(`max(party.members[var.i].hp, 1) + flag["x"] + rand(2) + floor(a.b.c)`).Refs()
	var s []string
	for _, r := range got {
		p := r.Name
		for _, st := range r.Path {
			switch {
			case st.Index && st.Name == "":
				p += "[]"
			case st.Index:
				p += "[" + st.Name + "]"
			default:
				p += "." + st.Name
			}
		}
		s = append(s, p)
	}
	want := "party.members[].hp var.i flag[x] rand a.b.c"
	if g := strings.Join(s, " "); g != want {
		t.Fatalf("Refs = %q, want %q", g, want)
	}
}

func TestNilExpr(t *testing.T) {
	var e *Expr
	if ok, err := e.Bool(nil); err != nil || !ok {
		t.Error("a nil condition should hold")
	}
	if n, err := e.Number(nil); err != nil || n != 0 {
		t.Error("a nil formula should be 0")
	}
}

// Deep nesting is an error, not a stack overflow; ordinary nesting still parses.
func TestNestingLimit(t *testing.T) {
	for _, n := range []int{1, 50, 100} {
		src := strings.Repeat("(", n) + "1" + strings.Repeat(")", n)
		if _, err := Parse(src); err != nil {
			t.Errorf("%d parentheses: %v", n, err)
		}
	}
	for _, src := range []string{
		strings.Repeat("(", 1000) + "1" + strings.Repeat(")", 1000),
		strings.Repeat("(", 1_000_000),
		strings.Repeat("!", 1000) + "true",
		strings.Repeat("a[", 1000) + "0" + strings.Repeat("]", 1000),
		strings.Repeat("true ? 1 : ", 1000) + "0",
	} {
		if _, err := Parse(src); err == nil || !strings.Contains(err.Error(), "nests too deeply") {
			t.Errorf("%.20q...: %v", src, err)
		}
	}
}

// A condition in YAML is any scalar, read as its text; a list or map is an error.
func TestUnmarshalYAML(t *testing.T) {
	for src, want := range map[string]string{
		"when: hp > 3": "hp > 3", "when: 1": "1", "when: true": "true", "when: \"a == 'x'\"": "a == 'x'",
	} {
		var doc struct {
			When *Expr `yaml:"when"`
		}
		if err := yaml.UnmarshalWithOptions([]byte(src), &doc, yaml.Strict()); err != nil || doc.When == nil || doc.When.String() != want {
			t.Errorf("%s: got %v, %v; want %q", src, doc.When, err, want)
		}
	}
	for _, src := range []string{"when: [1]", "when: {a: 1}", "when: 1 +"} {
		var doc struct {
			When *Expr `yaml:"when"`
		}
		if err := yaml.UnmarshalWithOptions([]byte(src), &doc, yaml.Strict()); err == nil {
			t.Errorf("%s: want an error", src)
		}
	}
}
