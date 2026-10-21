package play

import (
	"testing"

	lang "github.com/nao1215/gozen/internal/text"
)

// A node already at its highest level says so, even when no skill points are left (it used
// to say that the points were short, which sends the player off to earn points for nothing).
func TestSkillTreeMaxedNodeMessage(t *testing.T) {
	g := mnuExtraGame(t)
	g.st.Join("sage")
	sage := g.st.Actors["sage"]
	g.openScreen("skills", func() {})
	tv := mnuAs[*treeView](t, g)
	mnuPress(g, btnOK)
	mnuPress(g, btnOK) // spark is at 2 of 2 and the points are gone
	if sage.Tree["spark"] != 2 || sage.SP != 0 {
		t.Fatalf("tree %v sp %d", sage.Tree, sage.SP)
	}
	mnuPress(g, btnOK)
	if tv.msg != lang.T("これいじょう あげられない。") {
		t.Fatalf("msg %q for a maxed node with no points", tv.msg)
	}
}
