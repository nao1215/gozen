package chat

import (
	"strings"
	"testing"
	"testing/fstest"
	"time"
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
		"ﾊﾞｶ以外":   "ﾊﾞｶ以外", // half-width voiced marks are not folded together
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

// Parse never panics, and whatever it accepts is a script NewCrowd can run.
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
		if !(s.Rate > 0 && s.Rate <= 100) || len(s.Names) == 0 || len(s.Lines[s.Start]) == 0 {
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
