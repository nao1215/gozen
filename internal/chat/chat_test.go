package chat

import (
	"errors"
	"io/fs"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	"github.com/nao1215/gozen/internal/layer1/yamlpath"
)

// The log keeps the last Max messages with increasing IDs and cuts long texts.
func TestLog(t *testing.T) {
	l := New(Config{Max: 3, MaxRunes: 4})
	for _, s := range []string{"a", "b", "c", "d", "あいうえお"} {
		l.Post(Message{From: "p", Text: s})
	}
	got := l.Recent(10)
	if len(got) != 3 || got[0].Text != "c" || got[2].Text != "あいうえ" || got[2].ID != 5 {
		t.Fatalf("%+v", got)
	}
	if s := l.Since(4); len(s) != 1 || s[0].ID != 5 {
		t.Fatalf("since: %+v", s)
	}
	if s := l.Since(0); len(s) != 3 {
		t.Fatalf("since 0: %+v", s)
	}
	if r := l.Recent(-1); len(r) != 0 {
		t.Fatalf("recent -1: %+v", r)
	}
}

type drop struct{}

func (drop) Filter(m Message) (Message, bool) { return m, m.From != "spam" }

// Filters rewrite or drop messages; Mask hides words however they are spelled.
func TestFilters(t *testing.T) {
	l := New(Config{Filters: []Filter{drop{}, NewMask([]string{"ばか", "BAD"})}})
	if _, ok := l.Post(Message{From: "spam", Text: "x"}); ok {
		t.Fatal("not dropped")
	}
	for in, want := range map[string]string{
		"お前ばかだな":  "お前**だな",
		"バーカ":     "***",
		"ば か":     "* *",
		"ﾊﾞｶ以外":   "***以外", // half-width voiced marks are normalized
		"ＢＡＤ end": "*** end",
		"so bad.": "so ***.",
		"ばかばか":    "****",
		"たばこ":     "たばこ",
	} {
		m, ok := l.Post(Message{From: "p", Text: in})
		if !ok || m.Text != want {
			t.Errorf("%q: got %q, want %q", in, m.Text, want)
		}
	}
}

const script = `
names: [a, b, c]
rate: 2
start: idle
lines:
  idle: [こんにちは, 初見です]
  hype: [すごい, 888]
`

// A crowd talks at about its rate, follows its mood and bursts on demand.
func TestCrowd(t *testing.T) {
	s, err := Load(fstest.MapFS{"c.yaml": {Data: []byte(script)}}, "c.yaml")
	if err != nil {
		t.Fatal(err)
	}
	c := NewCrowd(s, CrowdConfig{})
	l := New(Config{Max: 1000})
	n := 0
	for sec := 1; sec <= 100; sec++ {
		n += len(l.Pull(time.Duration(sec)*time.Second, c))
	}
	if n < 150 || n > 250 {
		t.Fatalf("%d messages in 100s at rate 2", n)
	}
	if err := c.Mood("hype"); err != nil {
		t.Fatal(err)
	}
	if err := c.Burst("hype", 5); err != nil {
		t.Fatal(err)
	}
	for _, m := range c.Poll(101 * time.Second) {
		if m.Text != "すごい" && m.Text != "888" {
			t.Fatalf("not hype: %q", m.Text)
		}
	}
	// A long pause does not flood.
	if got := len(c.Poll(10000 * time.Second)); got > maxPerPoll {
		t.Fatalf("%d after a pause", got)
	}
	if c.Mood("none") == nil || c.Burst("none", 1) == nil {
		t.Fatal("an unknown mood was accepted")
	}
	for _, bad := range []string{
		strings.Replace(script, "rate: 2", "rate: 0", 1),
		strings.Replace(script, "start: idle", "start: calm", 1),
		strings.Replace(script, "names: [a, b, c]", "names: []", 1),
		strings.Replace(script, "rate: 2", "rat: 2", 1),
		strings.Replace(script, "rate: 2", "rate: .nan", 1),
	} {
		if _, err := Parse([]byte(bad)); err == nil {
			t.Errorf("accepted:\n%s", bad)
		}
	}
}

func FuzzMask(f *testing.F) {
	for _, s := range []string{"ばか", "ﾊﾞｶ", "\xff", "", strings.Repeat("ば か", 100)} {
		f.Add(s)
	}
	m := NewMask([]string{"ばか", "bad"})
	f.Fuzz(func(t *testing.T, s string) {
		out, ok := m.Filter(Message{Text: s})
		if !ok || len([]rune(out.Text)) != len([]rune(s)) {
			t.Fatalf("%q -> %q", s, out.Text)
		}
	})
}

// The log never shares a Values map with its callers: changing the posted map, the
// returned message or a Recent/Since copy leaves the logged message as it was.
func TestValuesAreCopied(t *testing.T) {
	l := New(Config{})
	in := map[string]float64{"tip": 100}
	out, ok := l.Post(Message{From: "p", Text: "x", Values: in})
	if !ok {
		t.Fatal("dropped")
	}
	in["tip"] = 1
	out.Values["tip"] = 2
	l.Recent(1)[0].Values["tip"] = 3
	l.Since(0)[0].Values["tip"] = 4
	if got := l.Recent(1)[0].Values["tip"]; got != 100 {
		t.Fatalf("logged tip changed to %v", got)
	}
	if m, _ := l.Post(Message{From: "p", Text: "y"}); m.Values != nil {
		t.Fatalf("nil Values became %v", m.Values)
	}
}

// parse never panics, and whatever it accepts is a script NewCrowd can run.
func FuzzParse(f *testing.F) {
	f.Add([]byte(script))
	f.Add([]byte("names: [a]\nrate: .nan\nstart: x\nlines: {x: [y]}\n"))
	f.Add([]byte("names: [a]\nrate: 1e309\nstart: x\nlines: {x: [y]}\n"))
	f.Add([]byte("names: [a]\nrate: 1\nstart: x\nlines: {x: [\"\"]}\n"))
	f.Add([]byte("&a [*a]"))
	f.Add([]byte{})
	f.Fuzz(func(t *testing.T, b []byte) {
		s, err := Parse(b)
		if err != nil {
			return
		}
		if !(s.doc.Rate > 0 && s.doc.Rate <= 100) || len(s.doc.Names) == 0 || len(s.doc.Lines[s.doc.Start]) == 0 {
			t.Fatalf("accepted an invalid script: %+v", s)
		}
		c := NewCrowd(s, CrowdConfig{})
		for sec := 1; sec <= 3; sec++ {
			if got := len(c.Poll(time.Duration(sec) * time.Second)); got > maxPerPoll {
				t.Fatalf("%d messages in one poll", got)
			}
		}
	})
}

func TestMaskCatchesTheDisguises(t *testing.T) {
	m := NewMask([]string{"bad", "ばか", "", "---"})
	cases := map[string]string{
		"bad":          "***",
		"BAD":          "***",
		"ｂａｄ":          "***",
		"b a d":        "* * *",
		"b-a_d":        "*****",
		"you are bad":  "you are ***",
		"ばか":           "**",
		"バカ":           "**",
		"ば-か":          "***",
		"ばーか":          "***",
		"ＢＡＤ！！":        "***！！",
		"nothing here": "nothing here",
		"":             "",
		"🙂bad🙂":        "🙂***🙂",
		"bbad":         "b***",
	}
	for in, want := range cases {
		got, ok := m.Filter(Message{Text: in})
		if !ok || got.Text != want {
			t.Errorf("%q -> %q (ok %v), want %q", in, got.Text, ok, want)
		}
	}
}

// Half-width kana and a voiced mark written as its own character spell the same word
// as the composed kana; the original characters that formed it are hidden.
func TestMaskNormalizesBeforeMatching(t *testing.T) {
	m := NewMask([]string{"ばか", "ガキ", "bad"})
	cases := map[string]string{
		"ﾊﾞｶ":             "***",
		"は\u3099か":        "***",
		"ﾊﾞｶ!":            "***!",
		"x\u30ab\u3099ｷy": "x***y",
		"ｶﾞｷ":             "***",
		"ﾊﾟｶ":             "ﾊﾟｶ",
		"ｂａｄ":             "***",
		"か\u3099き":        "***",
	}
	for in, want := range cases {
		got, _ := m.Filter(Message{Text: in})
		if got.Text != want {
			t.Errorf("%q -> %q, want %q", in, got.Text, want)
		}
	}
}

func TestMaskKeepsTheRestOfTheMessage(t *testing.T) {
	m := NewMask([]string{"bad"})
	in := Message{ID: 7, From: "x", Text: "so bad", At: time.Second, Values: map[string]float64{"tip": 3}}
	out, ok := m.Filter(in)
	if !ok || out.ID != 7 || out.From != "x" || out.At != time.Second || out.Values["tip"] != 3 || out.Text != "so ***" {
		t.Errorf("%+v", out)
	}
	if in.Text != "so bad" {
		t.Error("the input message was changed")
	}
}

func TestLogBoundsAndOrder(t *testing.T) {
	l := New(Config{Max: 3, MaxRunes: 4})
	for _, text := range []string{"a", "b", "c", "dddddddd", "e"} {
		l.Post(Message{From: "x", Text: text})
	}
	recent := l.Recent(10)
	if len(recent) != 3 || recent[0].Text != "c" || recent[1].Text != "dddd" || recent[2].Text != "e" {
		t.Fatalf("%+v", recent)
	}
	if recent[0].ID != 3 || recent[2].ID != 5 {
		t.Errorf("ids keep counting after old messages are dropped: %d %d", recent[0].ID, recent[2].ID)
	}
	if got := l.Recent(0); len(got) != 0 {
		t.Errorf("Recent(0) = %v", got)
	}
	if got := l.Recent(-3); len(got) != 0 {
		t.Errorf("Recent(-3) = %v", got)
	}
	if got := l.Recent(2); len(got) != 2 || got[0].Text != "dddd" {
		t.Errorf("Recent(2) = %v", got)
	}
	if got := l.Since(3); len(got) != 2 || got[0].ID != 4 {
		t.Errorf("Since(3) = %v", got)
	}
	if got := l.Since(0); len(got) != 3 {
		t.Errorf("Since(0) = %v (only what is kept)", got)
	}
	if got := l.Since(99); got != nil {
		t.Errorf("Since(99) = %v", got)
	}
	// a long text of multi-byte characters is cut on characters
	l2 := New(Config{MaxRunes: 3})
	out, _ := l2.Post(Message{Text: "あいうえお"})
	if out.Text != "あいう" {
		t.Errorf("cut %q", out.Text)
	}
	out, _ = l2.Post(Message{Text: "é́😀😀x"})
	if len([]rune(out.Text)) != 3 {
		t.Errorf("cut %q", out.Text)
	}
}

type fixedSource []Message

func (s fixedSource) Poll(time.Duration) []Message { return s }

func TestPullSetsTheTimeAndAppliesFilters(t *testing.T) {
	l := New(Config{Filters: []Filter{drop{}, NewMask([]string{"bad"})}})
	got := l.Pull(5*time.Second,
		fixedSource{{From: "a", Text: "bad day"}, {From: "spam", Text: "buy"}, {From: "b", Text: "kept", At: time.Second}},
		fixedSource{{From: "c", Text: "late"}},
	)
	if len(got) != 3 {
		t.Fatalf("%+v", got)
	}
	if got[0].Text != "*** day" || got[0].At != 5*time.Second {
		t.Errorf("a message with no time gets now, and is masked: %+v", got[0])
	}
	if got[1].At != time.Second {
		t.Errorf("a message with a time keeps it: %+v", got[1])
	}
	if got[2].ID != 3 {
		t.Errorf("sources are polled in order: %+v", got[2])
	}
	if len(l.Recent(10)) != 3 {
		t.Error("dropped messages are not logged")
	}
	if l.Pull(time.Second) != nil {
		t.Error("no sources, no messages")
	}
}

func TestYAMLPaths(t *testing.T) {
	s, err := Parse([]byte(script))
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range []string{"", "names", "names[1]", "rate", "start", "lines", "lines.idle", "lines.idle[0]", "lines.hype[1]"} {
		if _, err := s.YAML(p); err != nil {
			t.Errorf("%q: %v", p, err)
		}
	}
	whole, err := s.YAML("")
	if err != nil || string(whole) != script {
		t.Error("the whole script is returned as written")
	}
	for _, p := range []string{"nothing", "lines.none", "names[9]", "names[x]", "names[-1]", "names[0", "rate.x", "lines.idle[0][0]", "names[0]x"} {
		if _, err := s.YAML(p); !errors.Is(err, fs.ErrNotExist) {
			t.Errorf("%q: want not-exist, got %v", p, err)
		}
	}
	// within a sequence of objects, select by id
	got, err := yamlpath.At([]byte("people:\n  - id: a\n    n: 1\n  - id: b\n    n: 2\n  - 5\n"), "people.b.n")
	if err != nil || strings.TrimSpace(string(got)) != "2" {
		t.Errorf("by id: %q %v", got, err)
	}
	if _, err := yamlpath.At([]byte("people:\n  - id: a\n"), "people.zzz"); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("no such id: %v", err)
	}
	if _, err := yamlpath.At([]byte("a: [\n"), "a"); err == nil {
		t.Error("broken YAML")
	}
}

func TestLoadErrors(t *testing.T) {
	if _, err := Load(fstest.MapFS{}, "none.yaml"); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("%v", err)
	}
	if _, err := Load(fstest.MapFS{"c.yaml": {Data: []byte("rate: 0\n")}}, "c.yaml"); err == nil || !strings.Contains(err.Error(), "c.yaml") {
		t.Errorf("a bad script names its file: %v", err)
	}
	if _, err := Parse([]byte("names: [a]\nrate: 101\nstart: idle\nlines: {idle: [x]}\n")); err == nil {
		t.Error("a rate above 100")
	}
	if _, err := Parse([]byte("names: ['']\nrate: 1\nstart: idle\nlines: {idle: [x]}\n")); err == nil {
		t.Error("an empty name")
	}
}

func TestCrowdIsDeterministicAndBurstsOnce(t *testing.T) {
	s, err := Parse([]byte(script))
	if err != nil {
		t.Fatal(err)
	}
	a, b := NewCrowd(s, CrowdConfig{}), NewCrowd(s, CrowdConfig{})
	for sec := 1; sec <= 30; sec++ {
		x, y := a.Poll(time.Duration(sec)*time.Second), b.Poll(time.Duration(sec)*time.Second)
		if len(x) != len(y) {
			t.Fatalf("second %d: %d vs %d", sec, len(x), len(y))
		}
		for i := range x {
			if x[i].From != y[i].From || x[i].Text != y[i].Text || x[i].At != y[i].At {
				t.Fatalf("second %d: %+v vs %+v", sec, x[i], y[i])
			}
		}
	}
	if err := a.Burst("hype", 4); err != nil {
		t.Fatal(err)
	}
	if got := a.Poll(31 * time.Second); len(got) < 4 {
		t.Errorf("a burst of 4 shows at the next poll: %d", len(got))
	}
	if err := a.Burst("hype", 0); err != nil {
		t.Fatal(err)
	}
	if got := a.Poll(31 * time.Second); len(got) > 1 {
		t.Errorf("a burst is spent once: %d", len(got))
	}
	if err := a.Burst("hype", 500); err != nil {
		t.Fatal(err)
	}
	if got := a.Poll(32 * time.Second); len(got) > maxPerPoll {
		t.Errorf("a huge burst is capped: %d", len(got))
	}
}
