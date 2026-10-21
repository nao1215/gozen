package promptgen

import (
	"strings"
	"testing"
)

func TestSDXLWeightsAndRender(t *testing.T) {
	d := sdxl{}
	for strength, want := range map[int]float64{0: 0.9, 1: 1, 2: 1.1, 3: 1.2, 4: 1.5, 5: 1.6, 99: 1.6} {
		if got := d.Weight(strength); got != want {
			t.Errorf("Weight(%d) = %v, want %v", strength, got, want)
		}
	}
	got := d.Render([]Block{
		{Words: []Word{{Word: "a", Strength: 1}, {Word: "b", Strength: 0}}},
		{Words: nil, Boundary: "block"},
		{Words: []Word{{Word: "c", Strength: 4}}, Boundary: "block"},
		{Words: []Word{{Word: "d", Strength: 4, Fixed: true}}, Boundary: "inline"},
	})
	if got != "a, (b:0.9) BREAK (c:1.5), d" {
		t.Fatalf("render %q", got)
	}
	if d.Render(nil) != "" {
		t.Fatal("nothing renders as empty")
	}
}

func TestSDXLTermRules(t *testing.T) {
	d := sdxl{}
	for _, ok := range []string{"green field", "Soft Light", "8k", "a-b", "under_score"} {
		if err := d.ValidateTerm(ok); err != nil {
			t.Errorf("%q rejected: %v", ok, err)
		}
	}
	for _, bad := range []string{
		"", "   ", "score_9", "SCORE 7", "BREAK", "break", "AND", "a BREAK b",
		"a,b", "a:b", "(a)", "[a]", "{a}", "<a>", "a|b", "a\\b", "line\nbreak", "tab\there",
		"全角（かっこ）", "、読点", "日本語", "ひらがな", "カタカナ",
	} {
		if err := d.ValidateTerm(bad); err == nil {
			t.Errorf("%q accepted", bad)
		}
	}
	// "and" in lower case is an ordinary word; only the SDXL operator AND is reserved.
	if err := d.ValidateTerm("sword and shield"); err != nil {
		t.Errorf("lower-case and rejected: %v", err)
	}
	if err := checkDialect("sdxl"); err != nil {
		t.Fatal(err)
	}
	if err := checkDialect("flux"); err == nil {
		t.Fatal("unknown dialect accepted")
	}
}

func TestSDXLCountTokens(t *testing.T) {
	d := sdxl{}
	// Two for the start and end tokens; letters and digits in runs of four; each mark one.
	cases := map[string]int{
		"":                 2,
		"abcd":             3,
		"abcde":            4,
		"abcd, efgh":       2 + 1 + 1 + 1,
		"(word:1.2)":       2 + 1, // the weight is not a word
		"a b c":            2 + 3,
		"x" + "xxxxxxxx":   2 + 3,
		"(a:1.1), (b:0.9)": 2 + 1 + 1 + 1,
	}
	for in, want := range cases {
		if got := d.CountTokens(in); got != want {
			t.Errorf("CountTokens(%q) = %d, want %d", in, got, want)
		}
	}
}

func TestSDXLValidateOutput(t *testing.T) {
	d := sdxl{}
	ruleSet := func(ds []Diagnostic) map[string]string {
		m := map[string]string{}
		for _, x := range ds {
			m[x.Rule] = x.Severity
		}
		return m
	}
	if ds := d.ValidateOutput(Result{Positive: "a, (b:1.1)", Negative: "c"}); len(ds) != 0 {
		t.Fatalf("clean output: %+v", ds)
	}
	// An empty block between BREAKs.
	if got := ruleSet(d.ValidateOutput(Result{Positive: "a BREAK  BREAK b", Negative: "c"})); got["sdxl.break"] != "error" {
		t.Fatalf("empty block: %v", got)
	}
	// More than about 77 tokens in a block is only a warning.
	long := strings.Repeat("word, ", 80)
	if got := ruleSet(d.ValidateOutput(Result{Positive: long, Negative: "c"})); got["sdxl.tokens"] != "warning" {
		t.Fatalf("long block: %v", got)
	}
	// Each BREAK block has its own budget.
	split := strings.Repeat("word, ", 30) + " BREAK " + strings.Repeat("word, ", 30)
	if got := ruleSet(d.ValidateOutput(Result{Positive: split, Negative: "c"})); got["sdxl.tokens"] != "" {
		t.Fatalf("blocks share a budget: %v", got)
	}
	// Weights: not a number, zero, negative, NaN, infinite, too large, missing colon.
	for _, tag := range []string{"(a:x)", "(a:0)", "(a:-1)", "(a:NaN)", "(a:Inf)", "(a:1.6)", "(a)"} {
		if got := ruleSet(d.ValidateOutput(Result{Positive: tag, Negative: "c"})); got["sdxl.weight"] != "error" {
			t.Errorf("%s: %v", tag, got)
		}
	}
	// A tag the dialect cannot take.
	if got := ruleSet(d.ValidateOutput(Result{Positive: "a, 日本語", Negative: "c"})); got["sdxl.syntax"] != "error" {
		t.Fatalf("japanese tag: %v", got)
	}
	// An empty positive with no words is skipped (a catalog may select nothing).
	if ds := d.ValidateOutput(Result{Negative: "c"}); len(ds) != 0 {
		t.Fatalf("empty positive: %+v", ds)
	}
	// Stray commas and spaces between tags are ignored.
	if ds := d.ValidateOutput(Result{Positive: "a,, b ,", Negative: ", ,c"}); len(ds) != 0 {
		t.Fatalf("separators: %+v", ds)
	}
}

func TestRuleGroups(t *testing.T) {
	words := []Word{
		{Word: "common"},
		{Word: "a1", Group: "g1"},
		{Word: "b1", Group: "g2"},
		{Word: "a2", Group: "g1"},
	}
	groups := ruleGroups(words, "group")
	if len(groups) != 2 {
		t.Fatalf("%d groups", len(groups))
	}
	names := func(ws []Word) string {
		var out []string
		for _, w := range ws {
			out = append(out, w.Word)
		}
		return strings.Join(out, " ")
	}
	// Each group sees the common words too, in the order the groups first appear.
	if names(groups[0]) != "common a1 a2" || names(groups[1]) != "common b1" {
		t.Fatalf("groups %q / %q", names(groups[0]), names(groups[1]))
	}
	if g := ruleGroups(words, "global"); len(g) != 1 || len(g[0]) != 4 {
		t.Fatalf("global: %+v", g)
	}
	if g := ruleGroups([]Word{{Word: "x"}}, ""); len(g) != 1 || names(g[0]) != "x" {
		t.Fatalf("no groups: %+v", g)
	}
}
