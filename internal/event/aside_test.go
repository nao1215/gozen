package event_test

import (
	"testing"

	"github.com/nao1215/gozen/internal/event"
	"github.com/nao1215/gozen/internal/testgame"
)

const asideGame = `
asides:
  - {face: "npc/girl*.png", when: "var.on == 1", call: think, skip: [serious*]}
`

const asideEvents = `
events:
  think:
    do:
      - say: "……"
      - say: {who: 娘, face: npc/girl.png, text: inner}
  talk:
    do:
      - say: {who: 娘, face: npc/girl.png, text: hello}
      - say: {who: 男, face: npc/man.png, text: man}
      - say: {who: 娘, face: npc/girl_smile.png, text: bye}
  serious_talk:
    do:
      - call: talk
  covered:
    do:
      - image: cg/x.png
      - say: {who: 娘, face: npc/girl.png, text: hello}
`

func asideLines(t *testing.T, ev string, on float64) []string {
	t.Helper()
	db := testgame.Load(t, map[string]string{"data/z.yaml": asideEvents, "game.yaml": testgame.Game + asideGame,
		"assets/npc/girl.png": "x", "assets/npc/girl_smile.png": "x", "assets/npc/man.png": "x", "assets/cg/x.png": "x"})
	st := testgame.State(db, 1)
	st.Vars["on"] = on
	r := event.New(db, st, &host{})
	r.Log = func(err error) { t.Error(err) }
	r.Start(db.Events[ev].Do)
	var out []string
	for req := r.Next(); req != nil; req = r.Next() {
		if s, ok := req.(event.Say); ok {
			out = append(out, s.Text)
		}
	}
	return out
}

// asides inserts an event right after a matching speaker's line and returns to the original conversation when it ends.
// The same speaker inside an inserted event does not trigger another insert. No insert happens during when, skip or a CG.
func TestAsides(t *testing.T) {
	cases := []struct {
		ev   string
		on   float64
		want []string
	}{
		{"talk", 1, []string{"hello", "……", "inner", "man", "bye", "……", "inner"}},
		{"talk", 0, []string{"hello", "man", "bye"}},
		{"serious_talk", 1, []string{"hello", "man", "bye"}},
		{"covered", 1, []string{"hello"}},
	}
	for _, c := range cases {
		got := asideLines(t, c.ev, c.on)
		if len(got) != len(c.want) {
			t.Errorf("%s (on %v): %q, want %q", c.ev, c.on, got, c.want)
			continue
		}
		for i := range got {
			if got[i] != c.want[i] {
				t.Errorf("%s (on %v): %q, want %q", c.ev, c.on, got, c.want)
				break
			}
		}
	}
}

func TestAsideCallbackForPresentation(t *testing.T) {
	db := testgame.Load(t, map[string]string{"data/z.yaml": asideEvents, "game.yaml": testgame.Game + asideGame,
		"assets/npc/girl.png": "x", "assets/npc/girl_smile.png": "x", "assets/npc/man.png": "x", "assets/cg/x.png": "x"})
	st := testgame.State(db, 1)
	st.Vars["on"] = 1
	r := event.New(db, st, event.HostFunc(func(event.Effect) {}))
	calls := 0
	r.BlockAside = func() bool { calls++; return true }
	r.Start(db.Events["talk"].Do)
	lines := 0
	for req := r.Next(); req != nil; req = r.Next() {
		if _, ok := req.(event.Say); ok {
			lines++
		}
	}
	if calls == 0 || lines != 3 {
		t.Fatalf("callback calls=%d, lines=%d", calls, lines)
	}
}
