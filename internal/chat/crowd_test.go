package chat

import (
	"errors"
	"io/fs"
	"strings"
	"testing"
	"time"

	"github.com/nao1215/gozen/internal/layer1/yamlpath"
)

const script = `
names: [a, b, c]
rate: 2
start: idle
lines:
  idle: [こんにちは, 初見です]
  hype: [すごい, 888]
`

func TestCrowd(t *testing.T) {
	s, err := Parse([]byte(script))
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

func TestParseErrors(t *testing.T) {
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

func TestCrowdTinyRateDoesNotFlood(t *testing.T) {
	s, err := Parse([]byte(strings.Replace(script, "rate: 2", "rate: 1e-300", 1)))
	if err != nil {
		t.Fatal(err)
	}
	c := NewCrowd(s, CrowdConfig{})
	for _, now := range []time.Duration{0, time.Second, time.Hour} {
		if got := len(c.Poll(now)); got != 0 {
			t.Fatalf("%d messages at %v for a tiny rate", got, now)
		}
	}
}
