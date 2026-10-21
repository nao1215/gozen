package play

import (
	"image"
	"testing"

	lang "github.com/nao1215/gozen/internal/text"
)

// The field menu offers the character book when the game asks for it, and the entries
// after it keep working (Settings, Title, Close are numbered past the extra row).
func TestMenuCharactersEntryKeepsOtherEntries(t *testing.T) {
	g := mnuBookGame(t, Options{})
	g.push(newMenu(g))
	m := mnuAs[*menu](t, g)
	mnuTab(t, g, lang.T("キャラクター"))
	if _, ok := mnuTop(g).(*profileScreen); !ok {
		t.Fatalf("scene %T", mnuTop(g))
	}
	mnuPress(g, btnCancel)
	if mnuTop(g) != scene(m) {
		t.Fatalf("scene %T", mnuTop(g))
	}
	mnuTab(t, g, lang.T("せってい"))
	if _, ok := mnuTop(g).(*configScreen); !ok {
		t.Fatalf("scene %T", mnuTop(g))
	}
	g.stack = g.stack[:2]
	mnuTab(t, g, lang.T("とじる"))
	if len(g.stack) != 1 {
		t.Fatalf("Close left %d scenes", len(g.stack))
	}
}

// The status page lists the states a member has that describe themselves, and the stats.
func TestMenuStatusShowsStates(t *testing.T) {
	g, m := mnuMenu(t)
	g.db.States["poison"].Desc = "Loses a little each turn."
	g.st.Members()[0].States["poison"] = 3
	mnuTab(t, g, lang.T("ステータス"))
	mnuDrawAll(g)
	if m.mode != "status" {
		t.Fatalf("mode %q", m.mode)
	}
	// a click leaves the status page
	g.in.click, g.in.mouse = true, image.Pt(5, 5)
	mnuTop(g).update(g)
	g.in.click = false
	if m.mode != "" {
		t.Fatalf("mode %q", m.mode)
	}
}

// With the mouse the item list follows the pointer and a click chooses.
func TestMenuMouseChoosesItemAndMember(t *testing.T) {
	g, m := mnuMenu(t)
	g.st.Members()[1].HP = 3
	mnuTab(t, g, lang.T("どうぐ"))
	row := m.items.rowRect(0)
	g.in.click, g.in.mouse = true, image.Pt(row.Min.X+5, row.Min.Y+5)
	mnuTop(g).update(g)
	g.in.click = false
	if !m.targetOn {
		t.Fatal("a click on the item did not ask for a target")
	}
	card := m.cardRect(g, 1)
	g.in.moved, g.in.mouse = true, image.Pt(card.Min.X+5, card.Min.Y+5)
	mnuTop(g).update(g)
	g.in.moved = false
	if m.tsel != 1 {
		t.Fatalf("pointer over the second card: tsel %d", m.tsel)
	}
	g.in.click = true
	mnuTop(g).update(g)
	g.in.click = false
	if g.st.Members()[1].HP <= 3 {
		t.Fatalf("hp %d: the click did not heal the member under it", g.st.Members()[1].HP)
	}
}
