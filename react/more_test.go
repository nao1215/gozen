package react

import (
	"errors"
	"io/fs"
	"math/rand/v2"
	"strings"
	"testing"
	"testing/fstest"
)

const clerk = `
lexicon:
  greet: [hello, hi, "good morning"]
  rude: [idiot, jerk]
tags: [gift]
people:
  - id: clerk
    mood: 40
    mood_min: 10
    mood_max: 90
    faces: [normal, glad]
    likes: {greet: 5, rude: -50}
    asks:
      - id: color
        say: ["Which color?"]
        face: normal
        expect: [greet]
    rules:
      - id: probe
        kind: probe
        when: mood >= 0 && turn >= 0 && length >= 0 && repeats >= 0 && confidence >= 0 && kind == "probe" && slot.greet == "" && mem.nothing == 0 && value.size == 0 && from == "" && asked == "" && answer == ""
        say: ["probe {mood}"]
      - id: hello
        when: tag.greet > 0
        say: ["Hello {from}, mood {mood}", "Welcome {from}"]
        face: glad
        remember: [{key: seen, set: "{slot.greet}", turns: 2}, {key: count, add: 1}]
        forget: [stale]
      - id: bye
        when: tag.rude > 0
        say: ["Goodbye"]
    dodge:
      repeat: ["You said that"]
      unsure: ["Hmm"]
      unknown: ["What?"]
`

func clerkMind(t *testing.T, seed uint64) *Mind {
	t.Helper()
	b, err := Parse([]byte(clerk))
	if err != nil {
		t.Fatal(err)
	}
	m, err := New(b, "clerk", Config{Rand: rand.New(rand.NewPCG(seed, seed))})
	if err != nil {
		t.Fatal(err)
	}
	return m
}

func hear(t *testing.T, m *Mind, from, text string) Reaction {
	t.Helper()
	r, err := m.React(Stimulus{Kind: KindSay, From: from, Text: text})
	if err != nil {
		t.Fatal(err)
	}
	return r
}

func TestEveryNameAMotiveRuleCanRead(t *testing.T) {
	m := clerkMind(t, 1)
	r, err := m.React(Stimulus{Kind: "probe"})
	if err != nil {
		t.Fatal(err)
	}
	if r.Rule != "probe" || r.Lines[0] != "probe 40" {
		t.Errorf("%+v", r)
	}
}

func TestMoodIsClampedAndMemoriesAreWritten(t *testing.T) {
	m := clerkMind(t, 1)
	for range 40 {
		hear(t, m, "a", "hello hello")
	}
	if m.Mood() != 90 {
		t.Errorf("mood %v, want the upper bound 90", m.Mood())
	}
	for range 5 {
		hear(t, m, "a", "you idiot")
	}
	if m.Mood() != 10 {
		t.Errorf("mood %v, want the lower bound 10", m.Mood())
	}
	if v, ok := m.Recall("count"); !ok || v.(float64) < 40 {
		t.Errorf("an add keeps adding: %v %v", v, ok)
	}
	if v, ok := m.Recall("seen"); ok && v != "hello" {
		t.Errorf("seen = %v", v)
	}
}

func TestRepeatsArePerSender(t *testing.T) {
	m := clerkMind(t, 1)
	hear(t, m, "ann", "weather")
	if r := hear(t, m, "bob", "weather"); r.Rule == "dodge.repeat" {
		t.Error("a different sender saying the same is not a repeat")
	}
	if r := hear(t, m, "ann", "weather"); r.Rule != "dodge.repeat" {
		t.Errorf("the same sender is a repeat, got %s", r.Rule)
	}
}

func TestHistoryIsBounded(t *testing.T) {
	b, _ := Parse([]byte(clerk))
	m, err := New(b, "clerk", Config{History: 2})
	if err != nil {
		t.Fatal(err)
	}
	hear(t, m, "a", "first")
	hear(t, m, "a", "second")
	hear(t, m, "a", "third")
	if r := hear(t, m, "a", "first"); r.Rule == "dodge.repeat" {
		t.Error("a text older than the history is forgotten")
	}
}

func TestFullWidthAndCombiningInputsMatchTheDictionary(t *testing.T) {
	m := clerkMind(t, 1)
	for _, text := range []string{"ＨＥＬＬＯ", "Hello!!!", "h e l l o", "HELLO", "  hello  ", "good\tmorning"} {
		mm := clerkMind(t, 1)
		if r := hear(t, mm, "a", text); r.Rule != "hello" {
			t.Errorf("%q reacted with %s", text, r.Rule)
		}
	}
	for _, text := range []string{"", "   ", "🙂", "é", "あ゙"} {
		r := hear(t, m, "a", text)
		if r.Rule == "" && len(r.Lines) != 0 {
			t.Errorf("%q: %+v", text, r)
		}
		if r.Mood < 10 || r.Mood > 90 {
			t.Errorf("%q: mood %v out of its bounds", text, r.Mood)
		}
	}
}

func TestSameSeedSameLines(t *testing.T) {
	a, b := clerkMind(t, 7), clerkMind(t, 7)
	for _, text := range []string{"hello", "hi", "weather", "hello", "jerk", "hello there"} {
		ra, rb := hear(t, a, "x", text), hear(t, b, "x", text)
		if ra.Rule != rb.Rule || strings.Join(ra.Lines, "|") != strings.Join(rb.Lines, "|") || ra.Mood != rb.Mood {
			t.Fatalf("%q: %+v vs %+v", text, ra, rb)
		}
	}
}

func TestRestoreReproducesTheFollowingReactions(t *testing.T) {
	a := clerkMind(t, 3)
	hear(t, a, "x", "hello")
	hear(t, a, "x", "weather")
	if _, err := a.Ask("color"); err != nil {
		t.Fatal(err)
	}
	saved, err := a.Save()
	if err != nil {
		t.Fatal(err)
	}
	b := clerkMind(t, 3)
	if err := b.Restore(saved); err != nil {
		t.Fatal(err)
	}
	again, err := b.Save()
	if err != nil {
		t.Fatal(err)
	}
	if string(saved) != string(again) {
		t.Fatalf("save -> restore -> save changed:\n%s\n%s", saved, again)
	}
	for _, text := range []string{"hi", "weather", "weather", "idiot"} {
		ra, rb := hear(t, a, "x", text), hear(t, b, "x", text)
		if ra.Rule != rb.Rule || ra.Mood != rb.Mood || ra.Ask != rb.Ask {
			t.Fatalf("%q after a restore: %+v vs %+v", text, ra, rb)
		}
	}
}

func TestRestoreRefusesDamagedState(t *testing.T) {
	m := clerkMind(t, 1)
	good, _ := m.Save()
	for name, b := range map[string]string{
		"not json":         "{",
		"empty":            "",
		"another":          `{"version":1,"person":"other"}`,
		"unknown ask":      `{"version":1,"person":"clerk","asked":"nothing"}`,
		"memory of a list": `{"version":1,"person":"clerk","mem":{"k":{"v":[1]}}}`,
		"wrong version":    strings.Replace(string(good), `"version":1`, `"version":99`, 1),
	} {
		if err := m.Restore([]byte(b)); err == nil {
			t.Errorf("%s was accepted", name)
		}
	}
	// a refusal leaves the state as it was
	after, _ := m.Save()
	if string(after) != string(good) {
		t.Errorf("a refused restore changed the state: %s", after)
	}
}

func TestNewAndAskErrors(t *testing.T) {
	b, _ := Parse([]byte(clerk))
	if _, err := New(b, "nobody", Config{}); err == nil || !strings.Contains(err.Error(), `no person "nobody"`) {
		t.Errorf("%v", err)
	}
	m := clerkMind(t, 1)
	if _, err := m.Ask("nothing"); err == nil || !strings.Contains(err.Error(), `no ask "nothing"`) {
		t.Errorf("%v", err)
	}
}

func TestBookErrors(t *testing.T) {
	cases := map[string]string{
		"two people with one id":         strings.Replace(clerk, "people:\n", "people:\n  - {id: clerk}\n", 1),
		"person without an id":           strings.Replace(clerk, "people:\n", "people:\n  - {mood: 3}\n", 1),
		"mood bounds reversed":           strings.Replace(clerk, "mood_min: 10\n    mood_max: 90", "mood_min: 90\n    mood_max: 10", 1),
		"likes an unknown tag":           strings.Replace(clerk, "likes: {greet: 5, rude: -50}", "likes: {nothing: 5}", 1),
		"ask without lines":              strings.Replace(clerk, `say: ["Which color?"]`, "say: []", 1),
		"ask with an unknown face":       strings.Replace(clerk, "face: normal\n        expect", "face: wild\n        expect", 1),
		"ask with a bad template":        strings.Replace(clerk, `say: ["Which color?"]`, `say: ["{open"]`, 1),
		"dodge with an unknown face":     strings.Replace(clerk, `repeat: ["You said that"]`, "face: wild\n      repeat: [\"You said that\"]", 1),
		"dodge with a bad template":      strings.Replace(clerk, `unknown: ["What?"]`, `unknown: ["{nope.x}"]`, 1),
		"negative cooldown":              strings.Replace(clerk, "say: [\"Goodbye\"]", "say: [\"Goodbye\"]\n        cooldown: -1", 1),
		"memory with a bad template":     strings.Replace(clerk, `set: "{slot.greet}"`, `set: "{slot"`, 1),
		"tag with a field":               strings.Replace(clerk, "when: tag.greet > 0\n        say: [\"Hello", "when: tag > 0\n        say: [\"Hello", 1),
		"mood with a field":              strings.Replace(clerk, "when: tag.rude > 0", "when: mood.x > 0", 1),
		"a lexicon word of a map of two": strings.Replace(clerk, "greet: [hello, hi,", "greet: [{a: 1, b: 2}, hi,", 1),
	}
	for name, src := range cases {
		if src == clerk {
			t.Errorf("%s: the edit did not apply", name)
			continue
		}
		if _, err := Parse([]byte(src)); err == nil {
			t.Errorf("%s was accepted", name)
		}
	}
	if _, err := Parse([]byte("people: [")); err == nil {
		t.Error("broken YAML")
	}
	if _, err := Load(fstest.MapFS{}, "missing.yaml"); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("a missing file: %v", err)
	}
	if _, err := Load(fstest.MapFS{"b.yaml": {Data: []byte("people: [")}}, "b.yaml"); err == nil || !strings.Contains(err.Error(), "b.yaml") {
		t.Errorf("a broken file names itself: %v", err)
	}
}

func TestBookYAMLPaths(t *testing.T) {
	b, err := Parse([]byte(clerk))
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range []string{"", "people", "people.clerk", "people.clerk.rules", "people.clerk.rules.hello.say", "people[0].mood", "lexicon.greet", "lexicon.greet[1]", "tags"} {
		if _, err := b.YAML(p); err != nil {
			t.Errorf("%q: %v", p, err)
		}
	}
	for _, p := range []string{"nothing", "people.nobody", "people[9]", "people.clerk.rules.nothing", "lexicon.greet[9]", "lexicon.greet[x]", "tags.x", "people.clerk.mood.deep"} {
		if _, err := b.YAML(p); !errors.Is(err, fs.ErrNotExist) {
			t.Errorf("%q: want not-exist, got %v", p, err)
		}
	}
}
