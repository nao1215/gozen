package event_test

import (
	"maps"
	"strings"
	"testing"

	"github.com/nao1215/gozen/event"
	data "github.com/nao1215/gozen/internal/layer2/gamedata"
	"github.com/nao1215/gozen/internal/testgame"
	"github.com/nao1215/gozen/rpg"
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

// TestSPSuggestIsWhatApplySpends は勧めた計画（sp_suggest の頁）が振らずに作られ、
// sp_apply がおまかせ（SpendSP）と同じ振り方をすることを確かめる。
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
	// 同じ振り方: jab と spirit を交互に、jab が 2 になれば kick が開く
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
	// 振った後は勧めない
	runAll(t, db, st, &host{}, []data.Command{{SPSuggest: "o"}})
	if st.Vars["o"] != 0.0 {
		t.Errorf("suggested with no points: %v", st.Vars["o"])
	}
}

// TestSPDecline は断ったら SP の数が変わるまで勧めず、変われば勧め直すことを確かめる。
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

// TestSPSuggestManual は「自分で振る」設定（SPHost）では勧めないことを確かめる。
func TestSPSuggestManual(t *testing.T) {
	db, st := spGame(t)
	runAll(t, db, st, &manualHost{}, []data.Command{{SPSuggest: "o"}})
	if st.Vars["o"] != 0.0 {
		t.Fatalf("suggested to a manual player: %v", st.Vars["o"])
	}
}
