package play

import (
	"testing"

	lang "github.com/nao1215/gozen/internal/text"
)

// Choosing a recipe that cannot be made explains why (the screen had a message for this but
// the list swallowed the key, so it never showed).
func TestFusionDisabledRecipeExplains(t *testing.T) {
	g := mnuExtraGame(t)
	g.st.Gold = 100
	g.st.AddItem("shard", 3)
	g.openScreen("fusion", func() {})
	f := mnuAs[*fusion](t, g)
	mnuPress(g, btnDown) // gem + shard: no gem yet
	mnuPress(g, btnOK)
	if f.msgT == 0 || f.msg != lang.T("ざいりょうか おかねが たりない。") {
		t.Fatalf("msg %q (%d)", f.msg, f.msgT)
	}
	if g.st.Gold != 100 || g.st.Items["shard"] != 3 {
		t.Fatalf("a recipe that cannot be made took something: gold %d bag %v", g.st.Gold, g.st.Items)
	}
}
