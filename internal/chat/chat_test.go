package chat

import (
	"testing"
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
