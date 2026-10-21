package chat

import (
	"errors"
	"io/fs"
	"strings"
	"testing"
	"testing/fstest"
	"time"
)

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

func (s fixedSource) Poll(now time.Duration) []Message { return s }

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
	whole, _ := s.YAML("")
	if string(whole) != script {
		t.Error("the whole script is returned as written")
	}
	for _, p := range []string{"nothing", "lines.none", "names[9]", "names[x]", "names[-1]", "names[0", "rate.x", "lines.idle[0][0]", "names[0]x"} {
		if _, err := s.YAML(p); !errors.Is(err, fs.ErrNotExist) {
			t.Errorf("%q: want not-exist, got %v", p, err)
		}
	}
	// within a sequence of objects, select by id
	got, err := yamlAt([]byte("people:\n  - id: a\n    n: 1\n  - id: b\n    n: 2\n  - 5\n"), "people.b.n")
	if err != nil || strings.TrimSpace(string(got)) != "2" {
		t.Errorf("by id: %q %v", got, err)
	}
	if _, err := yamlAt([]byte("people:\n  - id: a\n"), "people.zzz"); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("no such id: %v", err)
	}
	if _, err := yamlAt([]byte("a: [\n"), "a"); err == nil {
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
	s, _ := Parse([]byte(script))
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
