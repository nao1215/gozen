package react

import (
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
