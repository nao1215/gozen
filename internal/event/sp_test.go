package event_test

import (
	"maps"
	"strings"
	"testing"

	"github.com/nao1215/gozen/internal/event"
	data "github.com/nao1215/gozen/internal/layer2/gamedata"
	"github.com/nao1215/gozen/internal/rpg"
	"github.com/nao1215/gozen/internal/testgame"
)

const spData = `
classes:
  monk:
    name: Monk
    stats: {mhp: "30", mmp: "10", atk: "5", def: "5", mat: "10", mdf: "5", agi: "10", luk: "5"}
    exp_for: "level * level * 10"
    sp: 1
    start_sp: 5
    sp_pitch: "Hit first."
    tree:
      - {id: jab, skill: slap}
      - {id: spirit, name: Spirit, stats: {atk: 1}}
      - {id: kick, skill: mend, requires: [{node: jab, level: 2}]}
actors:
  monk: {name: Monk, class: monk}
`

// spGame is the test game with the monk (5 points left) in the party.
func spGame(t *testing.T) (*data.DB, *rpg.State) {
	t.Helper()
	db := testgame.Load(t, map[string]string{"data/sp.yaml": spData})
	st := testgame.State(db, 1)
	st.Join("monk")
	return db, st
}

// manualHost is a host whose player spends the points by hand.
type manualHost struct{ host }

func (manualHost) ManualSP() bool { return true }

// runAll runs cmds to the end and gives the lines shown.
func runAll(t *testing.T, db *data.DB, st *rpg.State, h event.Host, cmds []data.Command) []string {
	t.Helper()
	r := event.New(db, st, h)
	r.Log = func(err error) { t.Error(err) }
	r.Start(cmds)
	var out []string
	for req := r.Next(); req != nil; req = r.Next() {
		if s, ok := req.(event.Say); ok {
			out = append(out, s.Text)
		}
	}
	return out
}

// TestSPSuggestIsWhatApplySpends checks that the suggested plan (the sp_suggest page) is built without spending,
// and that sp_apply spends the same way as the auto setting (SpendSP).
func TestSPSuggestIsWhatApplySpends(t *testing.T) {
	db, st := spGame(t)
	monk := st.Actors["monk"]
	runAll(t, db, st, &host{}, []data.Command{{SPSuggest: "o"}})
	if monk.SP != 5 || len(monk.Tree) != 0 {
		t.Fatalf("sp_suggest spent: SP %d tree %v", monk.SP, monk.Tree)
	}
	if st.Vars["o"] != 1.0 || st.Vars["o_pages"] != 1.0 {
		t.Fatalf("o = %v, pages %v", st.Vars["o"], st.Vars["o_pages"])
	}
	// same spending: alternate jab and spirit, and kick opens once jab reaches 2
	want := "Monk　Spirit Lv2、Nudge Lv2・Repair Lv1 を覚える\nHit first."
	if got := st.Vars["o_page1"]; got != want {
		t.Errorf("page = %q, want %q", got, want)
	}
	plain := rpg.NewActor(db, "monk")
	plain.SpendSP()
	notes := runAll(t, db, st, &host{}, []data.Command{{SPApply: "o"}})
	if monk.SP != 0 || !maps.Equal(monk.Tree, plain.Tree) {
		t.Errorf("sp_apply: SP %d tree %v, SpendSP %v", monk.SP, monk.Tree, plain.Tree)
	}
	if len(notes) != 1 || !strings.Contains(notes[0], "Nudge") || !strings.Contains(notes[0], "Repair") {
		t.Errorf("notes %q", notes)
	}
	// no suggestion after spending
	runAll(t, db, st, &host{}, []data.Command{{SPSuggest: "o"}})
	if st.Vars["o"] != 0.0 {
		t.Errorf("suggested with no points: %v", st.Vars["o"])
	}
}

// TestSPDecline checks that after declining, no suggestion is made until the SP count changes, and it is suggested again once it does.
func TestSPDecline(t *testing.T) {
	db, st := spGame(t)
	runAll(t, db, st, &host{}, []data.Command{{SPSuggest: "o"}, {SPDecline: "o"}, {SPSuggest: "o"}})
	if st.Vars["o"] != 0.0 {
		t.Fatalf("suggested again after a no: %v", st.Vars["o"])
	}
	st.Actors["monk"].SP++
	runAll(t, db, st, &host{}, []data.Command{{SPSuggest: "o"}})
	if st.Vars["o"] != 1.0 {
		t.Fatalf("not suggested after the points changed: %v", st.Vars["o"])
	}
}

// TestSPSuggestManual checks that nothing is suggested under the "spend myself" setting (SPHost).
func TestSPSuggestManual(t *testing.T) {
	db, st := spGame(t)
	runAll(t, db, st, &manualHost{}, []data.Command{{SPSuggest: "o"}})
	if st.Vars["o"] != 0.0 {
		t.Fatalf("suggested to a manual player: %v", st.Vars["o"])
	}
}
