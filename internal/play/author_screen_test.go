package play

import (
	"testing"

	data "github.com/nao1215/gozen/internal/layer2/gamedata"
	"github.com/nao1215/gozen/internal/testgame"
)

func TestAuthorScreenVisibilityAndReload(t *testing.T) {
	db := testgame.Load(t, nil)
	db.Game.Screens = map[string]*data.AuthorScreen{"notes": {
		Title: "Visits {var.visits}", Entries: []data.ScreenEntry{
			{Label: "Arrival", Text: "Welcome"},
			{Label: "Secret", Text: "Found", When: data.Expr{Expr: testgame.Expr("flag.found")}},
		},
	}}
	g := &Game{db: db, st: testgame.State(db, 1)}
	g.st.Vars["visits"] = float64(3)
	s := newAuthorScreen(g, "notes", nil)
	if s.title != "Visits 3" || len(s.entries) != 1 || s.entries[0].Label != "Arrival" {
		t.Fatalf("unexpected screen: %+v", s)
	}
	g.st.Flags["found"] = true
	n := sysAs[*authorScreen](t, s.liveKeep(g))
	if len(n.entries) != 2 || n.entries[1].Text != "Found" {
		t.Fatalf("reload did not reevaluate visibility: %+v", n)
	}
	db.Game.Screens["notes"].Entries = db.Game.Screens["notes"].Entries[1:]
	g.st.Flags["found"] = false
	n = sysAs[*authorScreen](t, n.liveKeep(g))
	if len(n.entries) != 0 || n.list.sel != 0 {
		t.Fatalf("empty screen selection: %+v", n)
	}
}
