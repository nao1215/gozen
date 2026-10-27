package react

import (
	"errors"
	"io/fs"
	"math/rand/v2"
	"strings"
	"testing"
	"testing/fstest"
	"time"
)

const host = `
lexicon:
  cat: [ねこ, 猫, {にゃ: 0.4}]
  dog: [いぬ, 犬, わんこ]
  rude: [ばか, アホ]
  love: [すき, 好き]
tags: [gift]
people:
  - id: host
    mood: 50
    faces: [normal, smile, angry, think]
    likes: {cat: 10, rude: -20}
    asks:
      - id: pet
        say: [犬派？ 猫派？]
        face: think
        expect: [cat, dog]
    rules:
      - id: answer_cat
        when: asked == "pet" && answer == "cat"
        say: [わかってるじゃん]
        face: smile
        remember: [{key: pet, set: "{slot.cat}"}]
      - id: answer_other
        when: asked == "pet"
        say: [ちゃんと答えて]
        face: angry
      - id: secret
        when: tag.acrostic_love > 0
        say: [縦読みバレてるよ]
      - id: gift
        kind: event
        when: tag.gift > 0 && value.amount >= 1000
        say: ["{from}、ありがと！"]
        delta: 15
      - id: callback
        when: tag.recall > 0
        say: ["{mem.pet}の話、覚えてるよ"]
      - id: cat
        when: tag.cat >= 0.5
        say: [猫いいよね]
        face: smile
        remember: [{key: cats, add: 1, turns: 3}]
        ask: pet
        cooldown: 2
    dodge:
      repeat: [それさっきも言った]
      unsure: [ふーん。で？]
      unknown: [日本語でおｋ]
      face: normal
`

func mind(t *testing.T) *Mind {
	t.Helper()
	b, err := Load(fstest.MapFS{"b.yaml": {Data: []byte(host)}}, "b.yaml")
	if err != nil {
		t.Fatal(err)
	}
	m, err := New(b, "host", Config{})
	if err != nil {
		t.Fatal(err)
	}
	return m
}

func do(t *testing.T, m *Mind, s Stimulus) Reaction {
	t.Helper()
	r, err := m.React(s)
	if err != nil {
		t.Fatal(err)
	}
	return r
}

func say(text string) Stimulus { return Stimulus{Kind: KindSay, From: "viewer", Text: text} }

// A conversation, turn by turn: the rule (or dodge) chosen, the first line and the mood.
func TestConversation(t *testing.T) {
	m := mind(t)
	steps := []struct {
		in   Stimulus
		rule string
		line string
		mood float64
	}{
		{say("ネコ飼ってます"), "cat", "猫いいよね", 60},
		{say("いぬ！"), "answer_other", "ちゃんと答えて", 60},
		{say("今日は雨"), "dodge.unknown", "日本語でおｋ", 60},
		{say("今日は雨"), "dodge.repeat", "それさっきも言った", 60},
		{say("にゃー"), "dodge.unsure", "ふーん。で？", 64},
		{say("ばーーか"), "dodge.unsure", "ふーん。で？", 44},
		{say("すごい\nきれい"), "secret", "縦読みバレてるよ", 44},
		{Stimulus{Kind: "event", From: "rich", Tags: []string{"gift"}, Values: map[string]float64{"amount": 5000}}, "gift", "rich、ありがと！", 59},
		{Stimulus{Kind: "event", From: "poor", Tags: []string{"gift"}, Values: map[string]float64{"amount": 100}}, "dodge.unsure", "ふーん。で？", 59},
	}
	for i, s := range steps {
		r, err := m.React(s.in)
		if err != nil {
			t.Fatalf("%d: %v", i, err)
		}
		if r.Rule != s.rule || len(r.Lines) == 0 || r.Lines[0] != s.line || r.Mood != s.mood {
			t.Fatalf("%d %q: got %s %q mood %v, want %s %q mood %v", i, s.in.Text, r.Rule, r.Lines, r.Mood, s.rule, s.line, s.mood)
		}
	}
}

// A question narrows the answer and the answer is remembered and brought back later.
func TestAskAndRecall(t *testing.T) {
	m := mind(t)
	r, err := m.Ask("pet")
	if err != nil || r.Ask != "pet" || r.Face != "think" || r.Lines[0] != "犬派？ 猫派？" {
		t.Fatalf("ask: %+v %v", r, err)
	}
	// "にゃ" alone is weak (0.4), but it is an expected answer, so it counts fully.
	r = do(t, m, say("にゃん派"))
	if r.Rule != "answer_cat" || r.Confidence != 1 || r.Ask != "" {
		t.Fatalf("answer: %+v", r)
	}
	if v, _ := m.Recall("pet"); v != "にゃ" {
		t.Fatalf("remembered %v", v)
	}
	// The question is no longer pending.
	if r = do(t, m, say("いぬ")); r.Rule != "dodge.unsure" {
		t.Fatalf("after the question: %s", r.Rule)
	}
	m2 := mind(t)
	if _, err := m2.Ask("pet"); err != nil {
		t.Fatal(err)
	}
	do(t, m2, say("猫"))
	if r = do(t, m2, say("そういえば猫って")); r.Rule != "callback" || r.Lines[0] != "猫の話、覚えてるよ" {
		t.Fatalf("callback: %s %q", r.Rule, r.Lines)
	}
}

// A rule's question follows its line, and the cooldown keeps the rule quiet.
func TestRuleAsksAndCools(t *testing.T) {
	m := mind(t)
	r := do(t, m, say("猫"))
	if len(r.Lines) != 2 || r.Lines[1] != "犬派？ 猫派？" || r.Face != "think" || r.Ask != "pet" {
		t.Fatalf("%+v", r)
	}
	do(t, m, say("天気"))
	if r = do(t, m, say("猫かわいい")); r.Rule == "cat" {
		t.Fatal("the cooldown did not hold")
	}
	do(t, m, say("a"))
	if r = do(t, m, say("ねこ")); r.Rule != "cat" {
		t.Fatalf("after the cooldown: %s", r.Rule)
	}
}

// Memories expire by turns and by game time.
func TestMemoryLimits(t *testing.T) {
	m := mind(t)
	do(t, m, say("猫"))
	if v, ok := m.Recall("cats"); !ok || v != 1.0 {
		t.Fatalf("cats %v", v)
	}
	for range 3 {
		do(t, m, say("x"))
	}
	if _, ok := m.Recall("cats"); !ok {
		t.Fatal("forgot within its turns")
	}
	do(t, m, say("x"))
	if _, ok := m.Recall("cats"); ok {
		t.Fatal("kept after its turns")
	}
	b, err := Parse([]byte(strings.Replace(host, "{key: cats, add: 1, turns: 3}", "{key: cats, add: 1, time: 10}", 1)))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := New(b, "host", Config{}); err == nil {
		t.Fatal("a time limit without a clock was accepted")
	}
	now := time.Duration(0)
	if m, err = New(b, "host", Config{Clock: func() time.Duration { return now }}); err != nil {
		t.Fatal(err)
	}
	do(t, m, say("猫"))
	now = 9 * time.Second
	do(t, m, say("x"))
	if _, ok := m.Recall("cats"); !ok {
		t.Fatal("forgot too early")
	}
	now = 11 * time.Second
	do(t, m, say("x"))
	if _, ok := m.Recall("cats"); ok {
		t.Fatal("kept after its time")
	}
}

// Save and Restore carry the state across.
func TestSaveRestore(t *testing.T) {
	m := mind(t)
	do(t, m, say("猫"))
	b, err := m.Save()
	if err != nil {
		t.Fatal(err)
	}
	m2 := mind(t)
	if err := m2.Restore(b); err != nil {
		t.Fatal(err)
	}
	if m2.Mood() != m.Mood() {
		t.Fatalf("mood %v != %v", m2.Mood(), m.Mood())
	}
	r := do(t, m2, say("ねこ"))
	if r.Rule != "answer_cat" {
		t.Fatalf("the pending question was not restored: %s", r.Rule)
	}
	if err := m2.Restore([]byte(`{"version":1,"person":"other"}`)); err == nil {
		t.Fatal("restored another person's state")
	}
}

func TestNormalize(t *testing.T) {
	for in, want := range map[string]string{
		"ネコ":      "ねこ",
		"ﾈｺ":      "ねこ",
		"ＡＢＣ ｄｅｆ": "abcdef",
		"すごーーーい！": "すごい",
		"かわいいいいい": "かわいい",
		"w w w w": "ww",
	} {
		if got := Normalize(in); got != want {
			t.Errorf("Normalize(%q) = %q, want %q", in, got, want)
		}
	}
}

// Validation finds broken references.
func TestValidate(t *testing.T) {
	for name, edit := range map[string][2]string{
		"unknown tag":    {"when: tag.cat >= 0.5", "when: tag.kat >= 0.5"},
		"unknown name":   {"when: tag.cat >= 0.5", "when: mod >= 0.5"},
		"unknown face":   {"face: smile\n        remember: [{key: cats", "face: grin\n        remember: [{key: cats"},
		"unknown ask":    {"ask: pet\n", "ask: food\n"},
		"bad template":   {"{from}、ありがと！", "{who}、ありがと！"},
		"bad weight":     {"{にゃ: 0.4}", "{にゃ: 2}"},
		"reserved tag":   {"love: [すき, 好き]", "repeat: [すき]"},
		"empty rule":     {"say: [縦読みバレてるよ]", "face: normal"},
		"set and add":    {"{key: cats, add: 1, turns: 3}", "{key: cats, add: 1, set: x}"},
		"unknown expect": {"expect: [cat, dog]", "expect: [cat, bird]"},
		"bad expression": {"when: tag.cat >= 0.5", "when: tag.cat >="},
		"duplicate rule": {"id: callback", "id: cat"},
		"unknown field":  {"cooldown: 2", "cooldwn: 2"},
	} {
		src := strings.Replace(host, edit[0], edit[1], 1)
		if src == host {
			t.Fatalf("%s: the edit did not apply", name)
		}
		if _, err := Parse([]byte(src)); err == nil {
			t.Errorf("%s: accepted", name)
		}
	}
}

// A long or hostile text is cut and does not break the analysis.
func FuzzReact(f *testing.F) {
	for _, s := range []string{"猫", "すごい\nきれい", strings.Repeat("あ", 5000), "{mem.pet}", "\x00\xff"} {
		f.Add(s)
	}
	b, err := Parse([]byte(host))
	if err != nil {
		f.Fatal(err)
	}
	f.Fuzz(func(t *testing.T, s string) {
		m, err := New(b, "host", Config{})
		if err != nil {
			t.Fatal(err)
		}
		for range 3 {
			r, err := m.React(say(s))
			if err != nil {
				t.Fatal(err)
			}
			if r.Mood < 0 || r.Mood > 100 || r.Confidence < 0 || r.Confidence > 1 {
				t.Fatalf("%+v", r)
			}
		}
	})
}

// A new session forgets repeats and the pending question but keeps mood and memory.
func TestReset(t *testing.T) {
	m := mind(t)
	do(t, m, say("猫"))
	do(t, m, say("今日は雨"))
	mood := m.Mood()
	m.Reset()
	if r := do(t, m, say("今日は雨")); r.Rule == "dodge.repeat" || r.Ask != "" {
		t.Fatalf("after a new session: %s ask %q", r.Rule, r.Ask)
	}
	if v, ok := m.Recall("cats"); !ok || v != 1.0 || m.Mood() != mood {
		t.Fatalf("memory %v %v, mood %v want %v", v, ok, m.Mood(), mood)
	}
}

// MaxRunes bounds the text used by built-in analysis and repeat detection.
func TestMaxRunesBoundsAnalysis(t *testing.T) {
	b, err := Parse([]byte(host))
	if err != nil {
		t.Fatal(err)
	}
	m, err := New(b, "host", Config{MaxRunes: 10})
	if err != nil {
		t.Fatal(err)
	}
	prefix := strings.Repeat("あ", 10)
	do(t, m, say(prefix+"first"))
	r := do(t, m, say(prefix+"different suffix"))
	if r.Rule != "dodge.repeat" {
		t.Fatalf("truncated repeat: %+v", r)
	}
}

// A failed condition advances the turn and drops expired memories, and changes nothing
// else: a retry of the same text is not counted as a repeat.
func TestReactErrorKeepsState(t *testing.T) {
	src := strings.Replace(host, "    rules:\n", "    rules:\n      - id: boom\n        kind: event.boom\n        when: from > 1\n        say: [x]\n", 1)
	if src == host {
		t.Fatal("the edit did not apply")
	}
	b, err := Parse([]byte(src))
	if err != nil {
		t.Fatal(err)
	}
	m, err := New(b, "host", Config{})
	if err != nil {
		t.Fatal(err)
	}
	do(t, m, say("猫"))
	before, err := m.Save()
	if err != nil {
		t.Fatal(err)
	}
	boom := Stimulus{Kind: "event.boom", From: "viewer", Text: "今日は雨"}
	for range 2 {
		if _, err := m.React(boom); err == nil {
			t.Fatal("comparing a name with a number succeeded")
		}
	}
	after, err := m.Save()
	if err != nil {
		t.Fatal(err)
	}
	want := strings.Replace(string(before), `"turn":1,`, `"turn":3,`, 1)
	if string(after) != want {
		t.Fatalf("state changed beyond the turn:\nbefore %s\nafter  %s", before, after)
	}
	if r := do(t, m, say("今日は雨")); r.Rule == "dodge.repeat" {
		t.Fatalf("a failed text was remembered: %+v", r)
	}
}

// Parse never panics, and a book it accepts makes a Mind for each person that can react.
func FuzzParse(f *testing.F) {
	f.Add([]byte(host))
	f.Add([]byte("tags: !x 0\n"))
	f.Add([]byte("people: [{id: a, rules: [{id: r, when: '(((('}]}]\n"))
	f.Add([]byte{})
	f.Fuzz(func(t *testing.T, b []byte) {
		if len(b) > 64*1024 {
			return
		}
		bk, err := Parse(b)
		if err != nil {
			return
		}
		for _, p := range bk.doc.People {
			m, err := New(bk, p.ID, Config{Clock: func() time.Duration { return 0 }})
			if err != nil {
				continue
			}
			if r, err := m.React(say("猫")); err == nil && (r.Mood < 0 || r.Mood > 100) {
				t.Fatalf("mood out of range: %+v", r)
			}
		}
	})
}

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
	m, err := New(b, "clerk", Config{Rand: rand.New(rand.NewPCG(seed, seed))}) //nolint:gosec // A fixed seed for a reproducible test.
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
	if v, ok := m.Recall("count"); !ok || !atLeast(v, 40) {
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
	b, err := Parse([]byte(clerk))
	if err != nil {
		t.Fatal(err)
	}
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
	good, err := m.Save()
	if err != nil {
		t.Fatal(err)
	}
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
	after, err := m.Save()
	if err != nil {
		t.Fatal(err)
	}
	if string(after) != string(good) {
		t.Errorf("a refused restore changed the state: %s", after)
	}
}

func TestNewAndAskErrors(t *testing.T) {
	b, err := Parse([]byte(clerk))
	if err != nil {
		t.Fatal(err)
	}
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
		"except of an unknown tag":       strings.Replace(clerk, "people:\n", "except: {nothing: [x]}\npeople:\n", 1),
		"except of an empty word":        strings.Replace(clerk, "people:\n", "except: {greet: [\"!\"]}\npeople:\n", 1),
		"an empty say entry":             strings.Replace(clerk, "say: [\"Goodbye\"]", "say: [[]]", 1),
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

// atLeast reports whether v is a number of at least n.
func atLeast(v any, n float64) bool {
	f, ok := v.(float64)
	return ok && f >= n
}

// TestWholeWordsExceptsAndLineLists checks a lexicon word matched only as a whole word
// of the text as written, a tag cancelled by an except word, a rule saying a list of
// lines in order, and {text}.
func TestWholeWordsExceptsAndLineLists(t *testing.T) {
	book, err := Parse([]byte(`
lexicon:
  cat: [{word: cat, whole: true}]
  sun: [sun]
except:
  sun: [sunday]
people:
  - id: host
    rules:
      - id: cat
        when: tag.cat > 0
        say: [[Cats!, "You said: {text}"]]
      - id: sun
        when: tag.sun > 0
        say: [Sunny.]
    dodge:
      unknown: [Hm.]
`))
	if err != nil {
		t.Fatal(err)
	}
	m, err := New(book, "host", Config{})
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		text string
		want []string
	}{
		{"I like CATS", []string{"Hm."}},
		{"a category", []string{"Hm."}},
		{"my Cat, again", []string{"Cats!", "You said: my Cat, again"}},
		{"the sun is out", []string{"Sunny."}},
		{"see you sunday", []string{"Hm."}},
	} {
		r, err := m.React(Stimulus{Kind: KindSay, From: "viewer", Text: tc.text})
		if err != nil {
			t.Fatal(err)
		}
		if strings.Join(r.Lines, "|") != strings.Join(tc.want, "|") {
			t.Errorf("%q: lines %q, want %q", tc.text, r.Lines, tc.want)
		}
	}
}
