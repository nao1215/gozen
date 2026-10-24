package session_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"maps"
	"testing"

	gozen "github.com/nao1215/gozen/internal/session"
)

// midSave is a save made at the second line of the story event (after a choice has an answer pending at line 3).
func midSave(t *testing.T, line int) (*gozen.Game, map[string]json.RawMessage) {
	t.Helper()
	g := storyGame(t)
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
	b, err := s.Save()
	if err != nil {
		t.Fatal(err)
	}
	var env map[string]json.RawMessage
	if err := json.Unmarshal(b, &env); err != nil {
		t.Fatal(err)
	}
	if _, ok := env["cursor"]; !ok {
		t.Fatalf("no cursor: %s", b)
	}
	return g, env
}

func withCursor(t *testing.T, env map[string]json.RawMessage, cursor string) []byte {
	t.Helper()
	m := map[string]json.RawMessage{}
	maps.Copy(m, env)
	m["cursor"] = json.RawMessage(cursor)
	b, err := json.Marshal(m)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

// TestRestoreRefusesCursorsThatCannotBeReplayed: a cursor that names a missing event, a line the event never reaches, or lacks the
// answers it needs, is an ErrIncompatibleSave, not a half-resumed game.
func TestRestoreRefusesCursorsThatCannotBeReplayed(t *testing.T) {
	g, env := midSave(t, 4)
	cases := map[string]string{
		"an event that is gone":        `{"event":"gone","line":1}`,
		"a line the event never shows": `{"event":"story","line":99}`,
		"a choice with no answer":      `{"event":"story","line":4}`,
		"the cursor of nothing":        `{"line":2}`,
	}
	for name, cursor := range cases {
		t.Run(name, func(t *testing.T) {
			r, err := gozen.Restore(g, withCursor(t, env, cursor), gozen.Config{})
			if !errors.Is(err, gozen.ErrIncompatibleSave) || r != nil {
				t.Fatalf("want ErrIncompatibleSave, got %v", err)
			}
		})
	}
	// the same cursor with its answer is fine
	r, err := gozen.Restore(g, withCursor(t, env, `{"event":"story","line":4,"answers":[1]}`), gozen.Config{})
	if err != nil {
		t.Fatalf("a cursor with its answers: %v", err)
	}
	if q := r.Next(); q.Kind != "say" || q.Text != "Went right." {
		t.Errorf("resumed at %+v", q)
	}
	// damaged random bytes
	bad := map[string]json.RawMessage{}
	maps.Copy(bad, env)
	bad["rand"] = json.RawMessage(`"AAAA"`)
	b, err := json.Marshal(bad)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := gozen.Restore(g, b, gozen.Config{}); !errors.Is(err, gozen.ErrIncompatibleSave) {
		t.Errorf("damaged random bytes: %v", err)
	}
}

// TestRestoreInTheMiddleOfArrival: a save made while the arrival at a place is running goes on from that line.
func TestRestoreInTheMiddleOfArrival(t *testing.T) {
	g := load(t)
	s2, err := gozen.New(g, gozen.Config{Seed: 1})
	if err != nil {
		t.Fatal(err)
	}
	if err := s2.Start("- move: camp\n"); err != nil {
		t.Fatal(err)
	}
	q := s2.Next()
	for q.Kind != "say" && q.Kind != "" {
		q = s2.Next()
	}
	b, err := s2.Save()
	if err != nil {
		t.Fatal(err)
	}
	r, err := gozen.Restore(g, b, gozen.Config{Seed: 1})
	if err != nil {
		t.Fatalf("a save in the middle of an arrival: %v", err)
	}
	if q.Kind == "say" {
		if got := r.Next(); got.Kind != "say" || got.Text != q.Text {
			t.Errorf("restored at %+v, want the line %q", got, q.Text)
		}
	}
}

// FuzzSaveInTheMiddle (ADR 0025): wherever in an event a save is made, restoring it goes on with the same requests,
// the same answers' effects and the same random numbers as the play it was made in.
func FuzzSaveInTheMiddle(f *testing.F) {
	f.Add(uint64(3), []byte{0, 0, 0})
	f.Add(uint64(3), []byte{0, 0, 0, 1, 0, 0})
	f.Add(uint64(9), []byte{0, 0, 0, 2, 0, 0, 0, 0})
	f.Add(uint64(1), []byte{})
	f.Fuzz(func(t *testing.T, seed uint64, ops []byte) {
		if len(ops) > 24 {
			return
		}
		g := storyGame(t)
		s, err := gozen.New(g, gozen.Config{Seed: seed})
		if err != nil {
			t.Fatal(err)
		}
		if err := s.Start("- call: story"); err != nil {
			t.Fatal(err)
		}
		var q gozen.Request
		for _, op := range ops {
			if q.Kind == "choice" {
				s.Answer(int(op) % 2)
			}
			q = s.Next()
			if q.Kind == "" {
				break
			}
		}
		save, err := s.Save()
		if err != nil {
			t.Fatal(err)
		}
		r, err := gozen.Restore(g, save, gozen.Config{Seed: seed + 1})
		if err != nil {
			t.Fatalf("a save made by a session cannot be restored: %v\n%s", err, save)
		}
		if !bytes.Contains(save, []byte(`"cursor"`)) {
			return // a save between events holds the state only: nothing is shown after loading it
		}
		again, err := r.Save()
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(stateOf(t, save), stateOf(t, again)) {
			t.Fatalf("restoring changed the state:\n%s\n%s", save, again)
		}
		// a restored session shows the line the save was made at first
		if q.Kind != "" {
			if got := r.Next(); got.Kind != q.Kind || got.Text != q.Text {
				t.Fatalf("restored at %+v, want %+v", got, q)
			}
		}
		for range 12 {
			if q.Kind == "choice" {
				s.Answer(0)
				r.Answer(0)
			}
			a, b := s.Next(), r.Next()
			if a.Kind != b.Kind || a.Text != b.Text {
				t.Fatalf("went on to %+v and %+v", a, b)
			}
			q = a
			if a.Kind == "" {
				break
			}
		}
		for _, name := range []string{"var.roll", "var.way", "var.after", "chose.way"} {
			var x, y any
			if errX, errY := s.Query(name, &x), r.Query(name, &y); (errX == nil) != (errY == nil) || x != y {
				t.Fatalf("%s is %v (%v) and %v (%v)", name, x, errX, y, errY)
			}
		}
	})
}

// stateOf is the state part of a save (the cursor and random bytes of a save in the middle of an event differ in form
// from those of the save made after restoring it, but the state they hold is the same).
func stateOf(t *testing.T, save []byte) []byte {
	t.Helper()
	var env struct {
		State json.RawMessage `json:"state"`
	}
	if err := json.Unmarshal(save, &env); err != nil {
		t.Fatal(err)
	}
	return env.State
}

// TestRunCountsSurviveRepeatedSaveAndLoad: loading a save made in the middle of an event, saving again and loading again
// does not count the event more often (ran.<event>.count is the number of times the event was run).
func TestRunCountsSurviveRepeatedSaveAndLoad(t *testing.T) {
	g := storyGame(t)
	s := mustNew(t, g, gozen.Config{Seed: 3})
	if err := s.Start("- call: story"); err != nil {
		t.Fatal(err)
	}
	if q := s.Next(); q.Kind != "say" {
		t.Fatalf("%+v", q)
	}
	cur := s
	for generation := range 4 {
		var count int
		if err := cur.Query("ran.story.count", &count); err != nil || count != 1 {
			t.Fatalf("generation %d: the story ran %d times (%v), want 1", generation, count, err)
		}
		b, err := cur.Save()
		if err != nil {
			t.Fatal(err)
		}
		if cur, err = gozen.Restore(g, b, gozen.Config{Seed: 3}); err != nil {
			t.Fatal(err)
		}
	}
}
