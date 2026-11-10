package play

import "testing"

// TestSkillTreeMemberButtons: the tree's previous and next buttons change the
// member whose tree shows, as C and Tab do.
func TestSkillTreeMemberButtons(t *testing.T) {
	g := mnuExtraGame(t)
	g.st.Join("sage")
	g.openScreen("skills", func() {})
	tv := mnuAs[*treeView](t, g)
	tv.ids = []string{"sage", "sage", "sage"} // three pages of the one fixture with a tree
	prev, next := stepButtons(tv.steps(g))
	mnuTap(g, next)
	if tv.member != 1 {
		t.Fatalf("the next button: member %d", tv.member)
	}
	mnuTap(g, prev)
	mnuTap(g, prev)
	if tv.member != 2 {
		t.Fatalf("the previous button twice from the second: member %d, want the last", tv.member)
	}
}
