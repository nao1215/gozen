package gozen_test

import (
	"testing"

	"github.com/nao1215/gozen"
	"github.com/nao1215/gozen/internal/layer4/sessionlink"
)

// TestSessionLinkHandsOverASession: the screen continues from what a session holds, and a session that has run is not fresh.
func TestSessionLinkHandsOverASession(t *testing.T) {
	g := load(t)
	s := mustNew(t, g, gozen.Config{Seed: 77, Dev: true})
	parts, ok := sessionlink.Of(s)
	if !ok || parts.FS == nil || parts.DB == nil || parts.State == nil || !parts.Dev || parts.Seed != 77 || !parts.Fresh {
		t.Fatalf("a new session: %+v %v", parts, ok)
	}
	if parts.State.Location != "camp" {
		t.Errorf("the state is the session's own: %q", parts.State.Location)
	}
	if err := s.Do(t.Context(), "- gold: 5\n"); err != nil {
		t.Fatal(err)
	}
	if parts, _ := sessionlink.Of(s); parts.Fresh {
		t.Error("a session that has run is no longer fresh")
	}
	for _, x := range []any{nil, "not a session", 3, (*gozen.Session)(nil), g} {
		if _, ok := sessionlink.Of(x); ok {
			t.Errorf("%T is not a session", x)
		}
	}
}
