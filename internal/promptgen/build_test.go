package promptgen

import (
	"bytes"
	"errors"
	"strings"
	"testing"
	"testing/fstest"
)

// base is a small original catalog: two scenes with a person layer and a place layer.
const base = `
version: 1
dialect: sdxl
emphasis: [soft, normal, strong, stronger, strongest]
default_emphasis: normal
priorities: {required: 0, subject: 1, detail: 2}
negative_limit: 4
layers:
  - id: place
    defaults: [field]
    items:
      field:
        positive: [{word: green field, priority: subject}, {word: wind, priority: detail}]
        negative: [{word: fog, priority: detail}]
      hill:
        positive: [{word: tall hill, priority: subject}]
        conflicts: [{layer: hero, id: knight}]
  - id: hero
    person: true
    max: 2
    items:
      knight:
        positive: [{word: knight, priority: subject, emphasis: strong}]
        negative: [{word: rust, priority: required}]
        requires: [{layer: place, id: field}]
      scout:
        positive: [{word: scout, priority: subject}, {word: wind, priority: subject, emphasis: soft}]
scenes:
  open: {select: {place: [field]}}
  duo: {select: {place: [field], hero: [knight, scout]}}
`

func parse(t *testing.T, y string, rules ...string) *Catalog {
	t.Helper()
	var rb [][]byte
	for _, r := range rules {
		rb = append(rb, []byte(r))
	}
	c, err := Parse([]byte(y), rb...)
	if err != nil {
		t.Fatalf("%v\n%s", err, y)
	}
	return c
}

func build(t *testing.T, c *Catalog, sel map[string][]string) Result {
	t.Helper()
	r, err := c.Build(Plan{Select: sel})
	if err != nil {
		t.Fatalf("build %v: %v", sel, err)
	}
	return r
}

func wordOf(r Result, side, word string) (Word, bool) {
	for _, w := range r.Words {
		if w.Side == side && w.Word == word {
			return w, true
		}
	}
	return Word{}, false
}

func TestBuildMergesSameWordsAcrossItems(t *testing.T) {
	c := parse(t, base)
	r := build(t, c, map[string][]string{"hero": {"scout"}})
	w, ok := wordOf(r, "positive", "wind")
	if !ok {
		t.Fatalf("wind missing: %+v", r.Words)
	}
	// One entry; the best priority (smaller number) and the strongest emphasis of the two sources.
	if w.Priority != 1 || w.Strength != 1 || len(w.Sources) != 2 {
		t.Fatalf("merged word %+v", w)
	}
	count := strings.Count(r.Positive, "wind")
	if count != 1 {
		t.Fatalf("%q lists wind %d times", r.Positive, count)
	}
	if !strings.HasSuffix(r.Positive, ", adult") || !strings.HasSuffix(r.Negative, "child, underage") {
		t.Fatalf("age guards missing: %q / %q", r.Positive, r.Negative)
	}
}

func TestBuildAppliesEmphasisToTheText(t *testing.T) {
	c := parse(t, base)
	r := build(t, c, map[string][]string{"hero": {"knight"}})
	if !strings.Contains(r.Positive, "(knight:1.1)") {
		t.Fatalf("strong emphasis not written as a weight: %q", r.Positive)
	}
	if strings.Contains(r.Positive, "(green field") {
		t.Fatalf("normal emphasis must be bare: %q", r.Positive)
	}
}

func TestPolarityResolvedByPriority(t *testing.T) {
	// "fog" is wanted (priority detail) and unwanted at the same time.
	withNeg := func(posPriority, negPriority string) string {
		y := strings.Replace(base, "negative: [{word: fog, priority: detail}]", "negative: [{word: fog, priority: "+negPriority+"}]", 1)
		return strings.Replace(y, "{word: wind, priority: detail}", "{word: fog, priority: "+posPriority+"}", 1)
	}
	// The positive is stronger (smaller number): the negative is dropped as a conflict.
	c := parse(t, withNeg("subject", "detail"))
	r := build(t, c, nil)
	if _, ok := wordOf(r, "negative", "fog"); ok {
		t.Fatal("the weaker negative stayed")
	}
	if _, ok := wordOf(r, "positive", "fog"); !ok {
		t.Fatal("the stronger positive was dropped")
	}
	if len(r.Dropped) != 1 || r.Dropped[0].Reason != "conflict" || r.Dropped[0].Side != "negative" || len(r.Dropped[0].By) == 0 {
		t.Fatalf("dropped %+v", r.Dropped)
	}
	// The negative is stronger: the positive goes.
	c = parse(t, withNeg("detail", "subject"))
	r = build(t, c, nil)
	if _, ok := wordOf(r, "positive", "fog"); ok {
		t.Fatal("the weaker positive stayed")
	}
	if _, ok := wordOf(r, "negative", "fog"); !ok {
		t.Fatal("the stronger negative was dropped")
	}
	// A tie is an error, not a coin toss.
	c = parse(t, withNeg("detail", "detail"))
	_, err := c.Build(Plan{})
	var ve *ValidationError
	if !errors.As(err, &ve) || len(ve.Diagnostics) != 1 || ve.Diagnostics[0].Rule != "core.polarity" {
		t.Fatalf("tie: %v", err)
	}
	if ve.Error() == "" || !strings.HasPrefix(ve.Error(), "core.polarity") {
		t.Fatalf("error text %q", ve.Error())
	}
}

func TestNegativeLimit(t *testing.T) {
	y := strings.Replace(base, "negative_limit: 4", "negative_limit: 1", 1)
	c := parse(t, y)
	r := build(t, c, map[string][]string{"hero": {"knight"}})
	// rust (required) is kept over fog (detail); the fixed guards do not count against the limit.
	if r.NegativeCount != 1 || r.FixedNegativeCount != 2 || !strings.HasPrefix(r.Negative, "rust, ") {
		t.Fatalf("limit result %+v / %q", r, r.Negative)
	}
	if len(r.Dropped) != 1 || r.Dropped[0].Reason != "limit" || r.Dropped[0].Word.Word != "fog" {
		t.Fatalf("dropped %+v", r.Dropped)
	}
	// More required negatives than the limit allows cannot be satisfied.
	y = strings.Replace(base, "negative_limit: 4", "negative_limit: 0", 1)
	c = parse(t, y)
	if _, err := c.Build(Plan{Select: map[string][]string{"hero": {"knight"}}}); err == nil {
		t.Fatal("required negatives over the limit accepted")
	}
}

func TestReplacements(t *testing.T) {
	y := strings.Replace(base, "requires: [{layer: place, id: field}]", "requires: [{layer: place, id: field}]\n        replaces: [{side: negative, word: fog, layer: place}]", 1)
	c := parse(t, y)
	r := build(t, c, map[string][]string{"hero": {"knight"}})
	if _, ok := wordOf(r, "negative", "fog"); ok {
		t.Fatal("replaced word stayed")
	}
	if len(r.Dropped) != 1 || r.Dropped[0].Reason != "replaced" || r.Dropped[0].By[0].ID != "knight" {
		t.Fatalf("dropped %+v", r.Dropped)
	}
	// Without the replacing item the word stays.
	if _, ok := wordOf(build(t, c, nil), "negative", "fog"); !ok {
		t.Fatal("fog lost without its replacer")
	}
	for _, bad := range []string{
		"replaces: [{side: sideways, word: fog}]",
		"replaces: [{side: negative, word: nothing here}]",
		"replaces: [{side: negative, word: fog, id: field}]", // an id needs a layer
		"replaces: [{side: negative, word: \"a,b\"}]",
	} {
		y := strings.Replace(base, "requires: [{layer: place, id: field}]", "requires: [{layer: place, id: field}]\n        "+bad, 1)
		if _, err := Parse([]byte(y)); err == nil {
			t.Errorf("accepted %s", bad)
		}
	}
	// A required (priority 0) word cannot be replaced.
	y = strings.Replace(base, "scout:", "scout:\n        replaces: [{side: negative, word: rust}]", 1)
	if _, err := Parse([]byte(y)); err == nil {
		t.Error("a word of stage 0 was made replaceable")
	}
}

func TestSelectionLimits(t *testing.T) {
	y := strings.Replace(base, "  - id: hero\n    person: true\n    max: 2", "  - id: hero\n    person: true\n    min: 1\n    max: 2", 1)
	c := parse(t, y)
	if _, err := c.Build(Plan{Select: map[string][]string{"hero": {}}}); err == nil {
		t.Fatal("below the minimum accepted")
	}
	if _, err := c.Build(Plan{Select: map[string][]string{"hero": {"scout", "scout"}}}); err == nil {
		t.Fatal("duplicate selection accepted")
	}
	three := strings.Replace(y, "scenes:", "scenes:\n  many: {select: {hero: [knight, scout, scout]}}", 1)
	if c := parse(t, three); c != nil {
		if _, err := c.Build(Plan{Select: map[string][]string{"hero": {"knight", "scout", "scout"}}}); err == nil {
			t.Fatal("above the maximum accepted")
		}
	}
	// A conflict of an item with a selected one, and a missing requirement.
	if _, err := c.Build(Plan{Select: map[string][]string{"place": {"hill"}, "hero": {"knight"}}}); err == nil {
		t.Fatal("conflicting items accepted")
	}
	if _, err := c.Build(Plan{Select: map[string][]string{"place": {"hill"}, "hero": {"scout"}}}); err != nil {
		t.Fatalf("scout does not need the field: %v", err)
	}
	// Lint builds an item alone partially, so it does not fail the minimum; the scene
	// "open" selects no hero and does.
	for _, d := range c.Lint() {
		if d.Context != "scene/open" {
			t.Fatalf("lint: %+v", d)
		}
	}
}

func TestBlockSeparatorsAndGroups(t *testing.T) {
	y := strings.Replace(base, "negative_limit: 4", "negative_limit: 4\nseparator: block", 1)
	y = strings.Replace(y, "  - id: hero\n    person: true", "  - id: hero\n    person: true\n    separate: true", 1)
	c := parse(t, y)
	r := build(t, c, map[string][]string{"hero": {"knight", "scout"}})
	// Each separated item is its own block; the fixed guard stays inline with the last.
	if got := strings.Count(r.Positive, " BREAK "); got < 2 {
		t.Fatalf("expected blocks between groups: %q", r.Positive)
	}
	w, ok := wordOf(r, "positive", "knight")
	if !ok || w.Group != "hero/knight" {
		t.Fatalf("group %+v", w)
	}
	// Words of a separated layer are not merged across groups.
	wind := 0
	for _, w := range r.Words {
		if w.Side == "positive" && w.Word == "wind" {
			wind++
		}
	}
	if wind != 2 {
		t.Fatalf("wind appears %d times, once per group expected: %q", wind, r.Positive)
	}
	// A layer may override the catalog separator.
	y2 := strings.Replace(y, "separate: true", "separate: true\n    separator: inline", 1)
	r2 := build(t, parse(t, y2), map[string][]string{"hero": {"knight", "scout"}})
	if strings.Contains(r2.Positive, " BREAK ") {
		t.Fatalf("inline layer produced a BREAK: %q", r2.Positive)
	}
}

func TestInvalidCatalogs(t *testing.T) {
	cases := map[string]string{
		"version":           strings.Replace(base, "version: 1", "version: 2", 1),
		"dialect":           strings.Replace(base, "dialect: sdxl", "dialect: other", 1),
		"no layers":         "version: 1\ndialect: sdxl\nemphasis: [normal]\ndefault_emphasis: normal\npriorities: {a: 0}\n",
		"default emphasis":  strings.Replace(base, "default_emphasis: normal", "default_emphasis: loud", 1),
		"emphasis name":     strings.Replace(base, "[soft, normal,", "[Soft, normal,", 1),
		"emphasis twice":    strings.Replace(base, "[soft, normal,", "[normal, normal,", 1),
		"priority name":     strings.Replace(base, "detail: 2", "Detail: 2", 1),
		"priority negative": strings.Replace(base, "detail: 2", "detail: -1", 1),
		"layer name":        strings.Replace(base, "id: place", "id: scene", 1),
		"layer twice":       strings.Replace(base, "id: hero", "id: place", 1),
		"min over max":      strings.Replace(base, "max: 2", "min: 3\n    max: 2", 1),
		"separator":         strings.Replace(base, "negative_limit: 4", "negative_limit: 4\nseparator: line", 1),
		"limit negative":    strings.Replace(base, "negative_limit: 4", "negative_limit: -1", 1),
		"default unknown":   strings.Replace(base, "defaults: [field]", "defaults: [ghost]", 1),
		"default twice":     strings.Replace(base, "defaults: [field]", "defaults: [field, field]", 1),
		"default over max":  strings.Replace(base, "id: place\n    defaults: [field]", "id: place\n    max: 1\n    defaults: [field, hill]", 1),
		"item id":           strings.Replace(base, "      hill:", "      Hill:", 1),
		"unknown priority":  strings.Replace(base, "priority: subject}, {word: wind", "priority: nope}, {word: wind", 1),
		"unknown emphasis":  strings.Replace(base, "emphasis: strong}", "emphasis: loud}", 1),
		"japanese tag":      strings.Replace(base, "word: knight", "word: 騎士", 1),
		"bracket in tag":    strings.Replace(base, "word: knight", "word: kni(ght", 1),
		"weight syntax":     strings.Replace(base, "word: knight", "word: \"knight:1.3\"", 1),
		"BREAK keyword":     strings.Replace(base, "word: knight", "word: BREAK", 1),
		"AND keyword":       strings.Replace(base, "word: knight", "word: AND", 1),
		"score tag":         strings.Replace(base, "word: knight", "word: score_9", 1),
		"empty tag":         strings.Replace(base, "word: knight", "word: \"  \"", 1),
		"age guard word":    strings.Replace(base, "word: knight", "word: adult", 1),
		"underage in plus":  strings.Replace(base, "word: knight", "word: underage knight", 1),
		"self requirement":  strings.Replace(base, "requires: [{layer: place, id: field}]", "requires: [{layer: hero, id: knight}]", 1),
		"unknown reference": strings.Replace(base, "id: field}]", "id: ghost}]", 1),
		"scene id":          strings.Replace(base, "  open:", "  Open:", 1),
		"unknown top key":   base + "\nmystery: 1\n",
	}
	for name, y := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := Parse([]byte(y)); err == nil {
				t.Fatal("invalid catalog accepted")
			}
		})
	}
	for _, y := range []string{"", "---\n---\n", "{", "- a\n- b\n"} {
		if _, err := Parse([]byte(y)); err == nil {
			t.Errorf("accepted %q", y)
		}
	}
	if _, err := ParsePlan([]byte("select: [unclosed")); err == nil {
		t.Fatal("broken plan accepted")
	}
}

func TestRuleValidation(t *testing.T) {
	good := "rules:\n- {id: r_one, kind: forbidden, severity: error, terms: [gold]}\n"
	if _, err := Parse([]byte(base), []byte(good)); err != nil {
		t.Fatal(err)
	}
	for name, rule := range map[string]string{
		"id":                 "{id: R1, kind: forbidden, severity: error, terms: [gold]}",
		"severity":           "{id: r1, kind: forbidden, severity: fatal, terms: [gold]}",
		"scope":              "{id: r1, kind: forbidden, severity: error, scope: planet, terms: [gold]}",
		"kind":               "{id: r1, kind: mystery, severity: error, terms: [gold]}",
		"forbidden no terms": "{id: r1, kind: forbidden, severity: error}",
		"conflict one term":  "{id: r1, kind: conflict, severity: error, terms: [gold]}",
		"color one color":    "{id: r1, kind: color, severity: error, target: hair, colors: {red: [rose]}}",
		"color no target":    "{id: r1, kind: color, severity: error, colors: {red: [rose], blue: [sky]}}",
		"emphasis no limit":  "{id: r1, kind: emphasis, severity: error}",
		"emphasis negative":  "{id: r1, kind: emphasis, severity: error, limit: -1}",
		"minimum over terms": "{id: r1, kind: risk, severity: error, terms: [gold], minimum: 2}",
		"minimum on forbid":  "{id: r1, kind: forbidden, severity: error, terms: [gold], minimum: 1}",
		"limit on forbidden": "{id: r1, kind: forbidden, severity: error, terms: [gold], limit: 1}",
		"target on forbid":   "{id: r1, kind: forbidden, severity: error, terms: [gold], target: hair}",
		"terms on emphasis":  "{id: r1, kind: emphasis, severity: error, terms: [gold], limit: 1}",
		"term twice":         "{id: r1, kind: forbidden, severity: error, terms: [gold, GOLD]}",
		"term syntax":        "{id: r1, kind: forbidden, severity: error, terms: [\"a(b\"]}",
		"color empty list":   "{id: r1, kind: color, severity: error, target: hair, colors: {red: [rose], blue: []}}",
		"color shared word":  "{id: r1, kind: color, severity: error, target: hair, colors: {red: [rose], blue: [rose]}}",
		"duplicate id":       "{id: r1, kind: forbidden, severity: error, terms: [gold]}\n- {id: r1, kind: forbidden, severity: error, terms: [tin]}",
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := Parse([]byte(base), []byte("rules:\n- "+rule+"\n")); err == nil {
				t.Fatal("invalid rule accepted")
			}
		})
	}
	if _, err := Parse([]byte(base), []byte("rules: [unclosed")); err == nil {
		t.Fatal("broken rule file accepted")
	}
	declared := strings.Replace(base, "negative_limit: 4", "negative_limit: 4\nvalidation: rules.yaml", 1)
	if _, err := Parse([]byte(declared)); err == nil {
		t.Fatal("a declared rule file that was not given must be an error")
	}
}

func TestRulesFireAsDocumented(t *testing.T) {
	// "wind" and "knight" are in the scene duo; scout adds wind at a stronger emphasis too.
	rule := func(r string) *Catalog { return parse(t, base, "rules:\n- "+r+"\n") }
	duo := map[string][]string{"hero": {"knight", "scout"}}
	diagOf := func(c *Catalog, id string) (Diagnostic, bool) {
		r, err := c.Build(Plan{Select: duo})
		for _, d := range r.Diagnostics {
			if d.Rule == id {
				return d, true
			}
		}
		_ = err
		return Diagnostic{}, false
	}
	// forbidden: any one of the terms.
	if d, ok := diagOf(rule("{id: no_wind, kind: forbidden, severity: warning, terms: [wind, rain]}"), "no_wind"); !ok || d.Severity != "warning" || len(d.Words) != 1 {
		t.Fatalf("forbidden: %+v %v", d, ok)
	}
	// A warning does not stop the build and leaves the text.
	c := rule("{id: no_wind, kind: forbidden, severity: warning, terms: [wind]}")
	if r, err := c.Build(Plan{Select: duo}); err != nil || r.Positive == "" {
		t.Fatalf("warning blocked the build: %v", err)
	}
	// An error does, and blanks the text.
	c = rule("{id: no_wind, kind: forbidden, severity: error, terms: [wind]}")
	if r, err := c.Build(Plan{Select: duo}); err == nil || r.Positive != "" || r.Negative != "" {
		t.Fatalf("error rule did not block: %v %+v", err, r)
	}
	// conflict: both terms present together.
	if _, ok := diagOf(rule("{id: pair, kind: conflict, severity: error, terms: [knight, scout]}"), "pair"); !ok {
		t.Fatal("conflict not reported")
	}
	if _, ok := diagOf(rule("{id: pair, kind: conflict, severity: error, terms: [knight, giant]}"), "pair"); ok {
		t.Fatal("conflict reported with one term absent")
	}
	// risk: all terms by default, or the minimum.
	if _, ok := diagOf(rule("{id: risky, kind: risk, severity: info, terms: [knight, scout, giant]}"), "risky"); ok {
		t.Fatal("risk fired without all its terms")
	}
	if d, ok := diagOf(rule("{id: risky, kind: risk, severity: info, terms: [knight, scout, giant], minimum: 2}"), "risky"); !ok || d.Severity != "info" {
		t.Fatal("risk with minimum 2 did not fire")
	}
	// color: two colors named together.
	colors := "{id: palette, kind: color, severity: warning, target: coat, colors: {green: [knight], red: [scout]}}"
	if d, ok := diagOf(rule(colors), "palette"); !ok || !strings.Contains(d.Message, "coat") {
		t.Fatalf("color: %+v %v", d, ok)
	}
	one := "{id: palette, kind: color, severity: warning, target: coat, colors: {green: [knight], red: [giant]}}"
	if _, ok := diagOf(rule(one), "palette"); ok {
		t.Fatal("color fired with only one color present")
	}
	// emphasis: a word stronger than the limit.
	if d, ok := diagOf(rule("{id: loud, kind: emphasis, severity: warning, limit: 1}"), "loud"); !ok || len(d.Words) != 1 || d.Words[0].Word != "knight" {
		t.Fatalf("emphasis: %+v %v", d, ok)
	}
	if _, ok := diagOf(rule("{id: loud, kind: emphasis, severity: warning, limit: 2}"), "loud"); ok {
		t.Fatal("emphasis fired within the limit")
	}
}

func TestRuleScopes(t *testing.T) {
	y := strings.Replace(base, "  - id: hero\n    person: true", "  - id: hero\n    person: true\n    separate: true", 1)
	duo := Plan{Select: map[string][]string{"hero": {"knight", "scout"}}}
	fired := func(scope string) bool {
		c := parse(t, y, "rules:\n- {id: pair, kind: conflict, severity: warning, scope: "+scope+", terms: [knight, scout]}\n")
		r, err := c.Build(duo)
		if err != nil {
			t.Fatal(err)
		}
		for _, d := range r.Diagnostics {
			if d.Rule == "pair" {
				return true
			}
		}
		return false
	}
	// Separated items are different groups: a group-scoped rule does not see them together.
	if fired("group") || fired("") {
		t.Fatal("group scope joined separate groups")
	}
	if !fired("global") {
		t.Fatal("global scope must see the whole prompt")
	}
}

func TestCoreDiagnostics(t *testing.T) {
	c := parse(t, base)
	// Duplicate and polarity problems in a result are reported by validateResult.
	w := Word{Word: "x", Side: "positive", Strength: 1}
	neg := Word{Word: "x", Side: "negative", Strength: 1}
	big := Word{Word: "y", Side: "positive", Strength: 5}
	ds := c.validateResult(Result{Positive: "x, y", Negative: "x", Words: []Word{w, w, neg, big}})
	rules := map[string]bool{}
	for _, d := range ds {
		rules[d.Rule] = true
	}
	for _, want := range []string{"core.duplicate", "core.polarity", "core.emphasis"} {
		if !rules[want] {
			t.Errorf("missing %s in %+v", want, ds)
		}
	}
	if !hasErrors(ds) || hasErrors([]Diagnostic{{Severity: "warning"}}) {
		t.Fatal("hasErrors")
	}
	if (&ValidationError{Diagnostics: []Diagnostic{{Severity: "warning", Rule: "a", Message: "m"}}}).Error() != "" {
		t.Fatal("warnings are not part of the error text")
	}
}

func TestLintFindsBrokenItemsAndScenes(t *testing.T) {
	c := parse(t, base, "rules:\n- {id: no_scout, kind: forbidden, severity: error, terms: [scout]}\n")
	ds := c.Lint()
	contexts := map[string]bool{}
	for _, d := range ds {
		contexts[d.Context] = true
		if d.Rule != "no_scout" {
			t.Errorf("unexpected diagnostic %+v", d)
		}
	}
	if !contexts["item/hero/scout"] || !contexts["scene/duo"] || contexts["item/hero/knight"] || contexts["scene/open"] {
		t.Fatalf("lint contexts %v", contexts)
	}
	var b bytes.Buffer
	if err := ds[0].WriteText(&b); err != nil || !strings.Contains(b.String(), ds[0].Context) || !strings.Contains(b.String(), "scout") {
		t.Fatalf("diagnostic text %q %v", b.String(), err)
	}
	// An item pulls in what it requires, so knight alone is linted with its place.
	if got := parse(t, base).Lint(); len(got) != 0 {
		t.Fatalf("clean catalog lints %+v", got)
	}
	// A scene whose plan cannot be built reports a build problem.
	broken := strings.Replace(base, "duo: {select: {place: [field], hero: [knight, scout]}}", "duo: {select: {place: [field], hero: [ghost]}}", 1)
	ds = parse(t, broken).Lint()
	if len(ds) != 1 || ds[0].Rule != "core.build" || ds[0].Context != "scene/duo" || ds[0].Severity != "error" {
		t.Fatalf("broken scene: %+v", ds)
	}
}

func TestLoadRelativeRules(t *testing.T) {
	catalog := strings.Replace(base, "negative_limit: 4", "negative_limit: 4\nvalidation: rules/main.yaml", 1)
	fsys := fstest.MapFS{
		"cat/catalog.yaml":    {Data: []byte(catalog)},
		"cat/rules/main.yaml": {Data: []byte("rules:\n- {id: no_wind, kind: forbidden, severity: warning, terms: [wind]}\n")},
	}
	c, err := Load(fsys, "cat/catalog.yaml")
	if err != nil {
		t.Fatal(err)
	}
	r, err := c.Build(Plan{})
	if err != nil || len(r.Diagnostics) != 1 || r.Diagnostics[0].Rule != "no_wind" {
		t.Fatalf("rules loaded relative to the catalog: %v %+v", err, r.Diagnostics)
	}
	for name, v := range map[string]string{
		"parent":    "../rules.yaml",
		"absolute":  "/etc/rules.yaml",
		"backslash": "rules\\\\main.yaml",
		"drive":     "c:rules.yaml",
		"missing":   "rules/none.yaml",
	} {
		bad := strings.Replace(base, "negative_limit: 4", "negative_limit: 4\nvalidation: \""+v+"\"", 1)
		fs := fstest.MapFS{"cat/catalog.yaml": {Data: []byte(bad)}, "cat/rules/main.yaml": {Data: []byte("rules: []\n")}}
		if _, err := Load(fs, "cat/catalog.yaml"); err == nil {
			t.Errorf("%s: validation path %q accepted", name, v)
		}
	}
	if _, err := Load(fsys, "cat/none.yaml"); err == nil {
		t.Fatal("missing catalog accepted")
	}
	if _, err := Load(nil, "cat/catalog.yaml"); err == nil {
		t.Fatal("nil FS accepted")
	}
	if _, err := Load(fstest.MapFS{"c.yaml": {Data: []byte("{")}}, "c.yaml"); err == nil {
		t.Fatal("broken catalog accepted")
	}
}

func TestSceneAndLayerAccessors(t *testing.T) {
	c := parse(t, base)
	if ids := c.LayerIDs(); len(ids) != 2 || ids[0] != "place" || ids[1] != "hero" {
		t.Fatalf("layer ids %v", ids)
	}
	if _, err := c.Scene("nowhere"); err == nil {
		t.Fatal("unknown scene accepted")
	}
	s, err := c.Scene("duo")
	if err != nil {
		t.Fatal(err)
	}
	s.Select["hero"][0] = "changed"
	again, err := c.Scene("duo")
	if err != nil || again.Select["hero"][0] != "knight" {
		t.Fatal("Scene returned the catalog's own slice")
	}
	if _, err := c.Build(Plan{Select: map[string][]string{"nolayer": {"x"}}}); err == nil {
		t.Fatal("unknown layer accepted")
	}
}

func TestResultWriteText(t *testing.T) {
	c := parse(t, base)
	r := build(t, c, map[string][]string{"hero": {"knight"}})
	var b bytes.Buffer
	if err := r.WriteText(&b); err != nil {
		t.Fatal(err)
	}
	out := b.String()
	for _, want := range []string{"positive: " + r.Positive, "negative: " + r.Negative, "positive knight", "hero/knight stage=subject", "negative rust"} {
		if !strings.Contains(out, want) {
			t.Errorf("text lacks %q:\n%s", want, out)
		}
	}
	y := strings.Replace(base, "negative_limit: 4", "negative_limit: 1", 1)
	r = build(t, parse(t, y), map[string][]string{"hero": {"knight"}})
	b.Reset()
	if err := r.WriteText(&b); err != nil || !strings.Contains(b.String(), "limit") {
		t.Fatalf("dropped words missing from the text: %q", b.String())
	}
}

func TestAgeGuardsAreFixed(t *testing.T) {
	for _, w := range []string{"adult", "Adult", "child", "UNDERAGE", " underage "} {
		if !protected(w) {
			t.Errorf("%q is not protected", w)
		}
	}
	for _, w := range []string{"children", "adulthood", "garden"} {
		if protected(w) {
			t.Errorf("%q is protected", w)
		}
	}
	if err := checkGuard("positive", "a child smiling"); err == nil {
		t.Error("a positive containing a guard exclusion was accepted")
	}
	if err := checkGuard("positive", "Under-age? no: underage"); err == nil {
		t.Error("punctuation hid a guard exclusion")
	}
	if err := checkGuard("negative", "a child smiling"); err != nil {
		t.Errorf("a negative may mention the excluded word inside a phrase: %v", err)
	}
	if err := checkGuard("negative", "child"); err == nil {
		t.Error("overriding a guard was accepted")
	}
	// A catalog without a person layer gets no positive guard, but always the negative ones.
	r := build(t, parse(t, base), nil)
	if strings.Contains(r.Positive, "adult") || !strings.HasSuffix(r.Negative, "child, underage") {
		t.Fatalf("guards: %q / %q", r.Positive, r.Negative)
	}
	for _, w := range r.Words {
		if w.Fixed && w.Priority != -1 {
			t.Errorf("fixed word %+v", w)
		}
	}
}
