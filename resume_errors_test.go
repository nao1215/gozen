package gozen_test

import (
	"encoding/json"
	"errors"
	"testing"

	"github.com/nao1215/gozen"
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
	for k, v := range env {
		m[k] = v
	}
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
	for k, v := range env {
		bad[k] = v
	}
	bad["rand"] = json.RawMessage(`"AAAA"`)
	b, _ := json.Marshal(bad)
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
