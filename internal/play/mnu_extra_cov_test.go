package play

import (
	"image"
	"slices"
	"strings"
	"testing"

	lang "github.com/nao1215/gozen/internal/text"
)

func mnuNodeAt(col, row int) image.Point {
	r := treeNodeRect(col, row)
	return image.Pt((r.Min.X+r.Max.X)/2, (r.Min.Y+r.Max.Y)/2)
}

func TestFormationScreen(t *testing.T) {
	g := mnuExtraGame(t)
	for _, id := range []string{"sage", "third", "fourth", "fifth"} {
		g.st.Join(id)
	}
	// the party holds four: the fourth steps out for the fifth
	g.st.Leave("fourth")
	g.st.Join("fifth")
	if len(g.st.Party) != g.db.Game.MaxParty || g.st.Actors["fourth"] == nil {
		t.Fatalf("party %v", g.st.Party)
	}
	done := 0
	g.openScreen("formation", func() { done++ })
	f, ok := mnuTop(g).(*formation)
	if !ok {
		t.Fatalf("scene %T", mnuTop(g))
	}
	mnuDrawAll(g)
	mnuPress(g, btnUp) // wraps to the last
	if f.sel != len(f.ids)-1 {
		t.Fatalf("sel %d of %d", f.sel, len(f.ids))
	}
	mnuPress(g, btnDown)
	mnuPress(g, btnRight) // swaps the row of the first
	first := g.st.Actors[f.ids[0]]
	if !first.Back {
		t.Fatal("right did not send the member to the back row")
	}
	mnuPress(g, btnLeft)
	if first.Back {
		t.Fatal("left did not bring the member back")
	}
	// the scout is the party's first: take the scout out (a party of 3 remains), put back
	n := len(g.st.Party)
	mnuPress(g, btnOK)
	if len(g.st.Party) != n-1 {
		t.Fatalf("party %v", g.st.Party)
	}
	mnuPress(g, btnOK)
	if len(g.st.Party) != n {
		t.Fatalf("party %v", g.st.Party)
	}
	// a fifth cannot join a full party: refused with a message
	f.sel = indexOf(f.ids, "fourth")
	mnuPress(g, btnOK)
	if f.msgT == 0 || len(g.st.Party) != g.db.Game.MaxParty || slices.Contains(g.st.Party, "fourth") {
		t.Fatalf("a party over the limit: %v msg %q", g.st.Party, f.msg)
	}
	mnuDrawAll(g)
	// the party may not be empty
	g.st.Party = nil
	mnuPress(g, btnCancel)
	if f.msgT == 0 || done != 0 {
		t.Fatalf("closed with an empty party (msgT %d, done %d)", f.msgT, done)
	}
	g.st.Party = []string{"sage"}
	mnuPress(g, btnCancel)
	if done != 1 {
		t.Fatalf("done %d", done)
	}
}

func TestSkillTreeScreen(t *testing.T) {
	g := mnuExtraGame(t)
	g.st.Join("sage")
	sage := g.st.Actors["sage"]
	g.openScreen("skills", func() {})
	tv, ok := mnuTop(g).(*treeView)
	if !ok {
		t.Fatalf("scene %T", mnuTop(g))
	}
	if len(tv.ids) != 1 || tv.ids[0] != "sage" {
		t.Fatalf("actors with a tree: %v", tv.ids)
	}
	if sage.SP != 2 {
		t.Fatalf("start SP %d", sage.SP)
	}
	mnuDrawAll(g)
	// node 0 is spark. glow needs spark: refuse, then raise spark twice
	mnuPress(g, btnRight)
	if tv.sel != 1 {
		t.Fatalf("right moved to %d", tv.sel)
	}
	mnuPress(g, btnOK)
	if sage.Tree["glow"] != 0 || !strings.Contains(tv.msg, "") || tv.msgT == 0 {
		t.Fatalf("raised a locked node: %v", sage.Tree)
	}
	if tv.msg != lang.T("まだ とれない。（さきに ひつような スキルを あげよう）") {
		t.Fatalf("msg %q", tv.msg)
	}
	mnuPress(g, btnLeft)
	mnuPress(g, btnOK)
	if sage.Tree["spark"] != 1 || sage.SP != 1 {
		t.Fatalf("tree %v sp %d", sage.Tree, sage.SP)
	}
	mnuDrawAll(g)
	mnuPress(g, btnOK)
	if sage.Tree["spark"] != 2 || sage.SP != 0 {
		t.Fatalf("tree %v sp %d", sage.Tree, sage.SP)
	}
	mnuPress(g, btnOK) // spark is at its max
	if tv.msg != lang.T("これいじょう あげられない。") {
		t.Fatalf("msg %q", tv.msg)
	}
	mnuPress(g, btnRight) // glow: reachable, but no SP
	mnuPress(g, btnOK)
	if tv.msg != lang.T("SPが たりない。") || sage.Tree["glow"] != 0 {
		t.Fatalf("msg %q tree %v", tv.msg, sage.Tree)
	}
	mnuDrawAll(g)
	sage.SP = 3
	mnuPress(g, btnDown) // mend
	mnuPress(g, btnUp)
	mnuPress(g, btnUp) // nothing above
	mnuPress(g, btnOK)
	if sage.Tree["glow"] != 1 {
		t.Fatalf("tree %v", sage.Tree)
	}
	mnuDrawAll(g)
	// the member switch with Tab (one member only: stays)
	mnuPress(g, btnNext)
	mnuShiftTab(g)
	if tv.member != 0 || tv.sel != 0 {
		t.Fatalf("member %d sel %d", tv.member, tv.sel)
	}
	// hidden passives are drawn "???" until their condition opens, then named
	mnuDrawAll(g)
	g.st.Flags["awake"] = true
	mnuDrawAll(g)
	// mouse: a click on a node selects it, a second click raises it
	g.in.click = true
	g.in.mouse = mnuNodeAt(1, 0)
	mnuTop(g).update(g)
	if tv.sel != 1 {
		t.Fatalf("click selected %d", tv.sel)
	}
	g.in.click = true
	mnuTop(g).update(g)
	if sage.Tree["glow"] != 2 {
		t.Fatalf("second click did not raise: %v", sage.Tree)
	}
	g.in.click = true
	g.in.mouse = mnuNodeAt(5, 5)
	g.in.mouse.Y = 50 // the header: next member
	mnuTop(g).update(g)
	g.in.click = false
	mnuPress(g, btnCancel)
	if _, still := mnuTop(g).(*treeView); still {
		t.Fatal("cancel did not close the tree")
	}
}

func TestSkillTreeWithoutTreesClosesAtOnce(t *testing.T) {
	g := openGame(t, Options{Location: "camp"})
	done := 0
	g.openScreen("skills", func() { done++ })
	mnuPress(g)
	if done != 1 || len(g.stack) != 1 {
		t.Fatalf("done %d stack %d", done, len(g.stack))
	}
}

func TestFusionScreen(t *testing.T) {
	g := mnuExtraGame(t)
	g.st.Gold = 100
	g.st.AddItem("shard", 3)
	done := 0
	g.openScreen("fusion", func() { done++ })
	f := mnuAs[*fusion](t, g)
	mnuDrawAll(g)
	if f.list.items[0].disabled || f.list.items[1].disabled == false {
		t.Fatalf("rows %+v", f.list.items)
	}
	mnuPress(g, btnDown) // second recipe needs a gem: refused
	mnuPress(g, btnOK)
	if f.msgT == 0 || g.st.Gold != 100 {
		t.Fatalf("msgT %d gold %d", f.msgT, g.st.Gold)
	}
	mnuPress(g, btnUp)
	mnuPress(g, btnOK)
	if g.st.Gold != 70 || g.st.Items["shard"] != 1 || g.st.Items["gem"] != 1 {
		t.Fatalf("gold %d bag %v", g.st.Gold, g.st.Items)
	}
	mnuDrawAll(g)
	// now the second recipe works: gem + shard
	mnuPress(g, btnDown)
	mnuPress(g, btnOK)
	if g.st.Items["crystal"] != 1 || g.st.Items["gem"] != 0 || g.st.Items["shard"] != 0 || g.st.Gold != 65 {
		t.Fatalf("gold %d bag %v", g.st.Gold, g.st.Items)
	}
	// money short: the first recipe is greyed
	g.st.AddItem("shard", 2)
	g.st.Gold = 29
	f.refresh(g)
	if !f.list.items[0].disabled {
		t.Fatal("a recipe with too little gold is on offer")
	}
	mnuPress(g, btnUp)
	mnuPress(g, btnOK)
	if g.st.Items["gem"] != 0 {
		t.Fatal("fused without the money")
	}
	f.list.sel = len(f.idx)
	mnuPress(g, btnOK)
	if done != 1 {
		t.Fatalf("done %d", done)
	}
}

func TestOpenScreenDispatch(t *testing.T) {
	g := mnuExtraGame(t)
	called := map[string]int{}
	for _, name := range []string{"status", "item", "skill", "equip", "save", "config", "title", "profiles", "nope"} {
		before := len(g.stack)
		g.openScreen(name, func() { called[name]++ })
		pushed := len(g.stack) - before
		if name == "nope" && (pushed != 0 || called[name] != 1) {
			t.Fatalf("unknown screen: pushed %d, done %d", pushed, called[name])
		}
		if name != "nope" && pushed != 1 {
			t.Fatalf("%s: pushed %d scenes", name, pushed)
		}
		mnuDrawAll(g)
		g.stack = g.stack[:before]
	}
	// load with no save at all is a no-op that finishes; with a save it opens the load screen
	done := 0
	g.openScreen("load", func() { done++ })
	if done != 1 || len(g.stack) != 1 {
		t.Fatalf("load without saves: done %d stack %d", done, len(g.stack))
	}
	if err := g.save(0); err != nil {
		t.Fatal(err)
	}
	g.openScreen("load", func() { done++ })
	sm, ok := mnuTop(g).(*saveMenu)
	if !ok || sm.saving || !sm.inPlay {
		t.Fatalf("scene %T", mnuTop(g))
	}
	// tactics outside a battle opens the editor
	g.stack = g.stack[:1]
	g.openScreen("tactics", func() {})
	if len(g.stack) != 2 {
		t.Fatalf("tactics pushed %d", len(g.stack)-1)
	}
}
