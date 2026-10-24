package session_test

import (
	"bytes"
	"context"
	"testing"
	"testing/fstest"

	gozen "github.com/nao1215/gozen/internal/session"
	"github.com/nao1215/gozen/internal/testgame"
)

// storyGame is the test expedition with a short story: lines, a choice with an id, and
// a random number drawn in the middle.
func storyGame(t *testing.T) *gozen.Game {
	t.Helper()
	fsys := fstest.MapFS{
		"game.yaml":      {Data: []byte("schema_version: 4\nid: story\n" + testgame.Game)},
		"data/main.yaml": {Data: []byte(testgame.Data)},
		"assets/x.png":   {Data: []byte("a stub")},
		"data/story.yaml": {Data: []byte(`events:
  story:
    do:
      - say: "One."
      - set: {var.roll: "rand(1000)"}
      - say: "Rolled {var.roll}."
      - choice:
          - {id: left, label: Left, do: [{set: {var.way: 1}}]}
          - {id: right, label: Right, do: [{set: {var.way: 2}}]}
        id: way
      - say: "Went {chose.way}."
      - set: {var.after: "rand(1000)"}
      - say: "Then {var.after}."
`)},
	}
	g, err := gozen.Load(fsys, ".")
	if err != nil {
		t.Fatal(err)
	}
	return g
}

// TestSaveInTheMiddleReplays is the contract of ADR 0025: a save made at a line of an
// event, restored, goes on at that line with the same state and the same random numbers
// as the play it was made in.
func TestSaveInTheMiddleReplays(t *testing.T) {
	g := storyGame(t)
	for line := 1; line <= 5; line++ {
		s, err := gozen.New(g, gozen.Config{Seed: 3})
		if err != nil {
			t.Fatal(err)
		}
		if err := s.Start("- call: story"); err != nil {
			t.Fatal(err)
		}
		var q gozen.Request
		for range line {
			if q.Kind == "choice" {
				s.Answer(1)
			}
			q = s.Next()
		}
		save, err := s.Save()
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Contains(save, []byte(`"cursor"`)) {
			t.Fatalf("line %d: no cursor in %s", line, save)
		}
		r, err := gozen.Restore(g, save, gozen.Config{Seed: 99})
		if err != nil {
			t.Fatalf("line %d: %v", line, err)
		}
		if got := r.Next(); got.Kind != q.Kind || got.Text != q.Text {
			t.Fatalf("line %d: restored at %+v, want %+v", line, got, q)
		}
		if a, b := stateNow(t, s), stateNow(t, r); !bytes.Equal(a, b) {
			t.Fatalf("line %d: the states differ\n%s\n%s", line, a, b)
		}
		for _, name := range []string{"var.roll", "var.way", "chose.way"} {
			var a, b any
			errA, errB := s.Query(name, &a), r.Query(name, &b)
			if errA != nil || errB != nil || a != b {
				t.Fatalf("line %d: %s is %v (%v) and %v (%v)", line, name, a, errA, b, errB)
			}
		}
		if q.Kind == "choice" {
			s.Answer(1)
			r.Answer(1)
		}
		for {
			a, b := s.Next(), r.Next()
			if a.Kind != b.Kind || a.Text != b.Text {
				t.Fatalf("line %d: went on to %+v and %+v", line, a, b)
			}
			if a.Kind == "" {
				break
			}
			if a.Kind == "choice" {
				s.Answer(0)
				r.Answer(0)
			}
		}
	}
}

// TestSaveAfterTheEventHasNoCursor: a save between events holds the state only, and a
// save past a boundary too.
func TestSaveAfterTheEventHasNoCursor(t *testing.T) {
	g := storyGame(t)
	s, err := gozen.New(g, gozen.Config{Seed: 3})
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Do(context.Background(), "- call: story"); err != nil {
		t.Fatal(err)
	}
	save, err := s.Save()
	if err != nil || bytes.Contains(save, []byte(`"cursor"`)) || !bytes.Contains(save, []byte(`"chose"`)) {
		t.Fatalf("a save after the event: %s (%v)", save, err)
	}
	if err := s.Start("- say: Before.\n- battle: one\n- say: After."); err != nil {
		t.Fatal(err)
	}
	for q := s.Next(); q.Kind != "" && q.Text != "After."; q = s.Next() {
		if q.Kind == "battle" {
			s.Answer(gozen.Won)
		}
	}
	if save, err := s.Save(); err != nil || bytes.Contains(save, []byte(`"cursor"`)) {
		t.Fatalf("a save past a battle has a cursor: %v", err)
	}
}

// stateNow is the state of a session as it is now (a clone runs nothing, so it saves no cursor).
func stateNow(t *testing.T, s *gozen.Session) []byte {
	t.Helper()
	c, err := s.Clone()
	if err != nil {
		t.Fatal(err)
	}
	b, err := c.Save()
	if err != nil {
		t.Fatal(err)
	}
	return b
}
