package play

import (
	"fmt"
	"image"
	"testing"

	lang "github.com/nao1215/gozen/internal/base/text"
)

// A free item can be bought without money: the count starts at one and never falls to
// zero, whatever the purse holds.
func TestShopFreeItemCountWithoutGold(t *testing.T) {
	g := mnuShopGame(t, 0)
	s, _ := mnuOpenShop(g, "bazaar")
	mnuPress(g, btnOK) // buy
	s.goods.sel = indexOf(s.ids, "hat")
	mnuPress(g, btnOK)
	if s.pick != "hat" || s.count != 1 {
		t.Fatalf("pick %q count %d", s.pick, s.count)
	}
	mnuPress(g, btnUp)
	if s.count < 1 {
		t.Fatalf("one more of a free item made the count %d", s.count)
	}
	mnuPress(g, btnOK)
	if g.st.Items["hat"] < 1 {
		t.Fatalf("the shop said %q but the bag holds %d", s.msg, g.st.Items["hat"])
	}
}

// TestShopCountButtons: the quantity box is worked by its buttons alone: plus
// and minus change the count, a tap on the box beside them trades nothing,
// Cancel and a tap outside go back to the goods, and the trade button buys.
func TestShopCountButtons(t *testing.T) {
	g := mnuShopGame(t, 100)
	s, _ := mnuOpenShop(g, "bazaar")
	mnuPress(g, btnOK) // buy
	s.goods.sel = indexOf(s.ids, "potion")
	pick := func() {
		t.Helper()
		mnuPress(g, btnOK)
		if s.pick != "potion" || s.count != 1 {
			t.Fatalf("pick %q count %d", s.pick, s.count)
		}
	}
	tap := func(r image.Rectangle) {
		g.in = input{click: true, pdown: true, mouse: r.Min.Add(image.Pt(r.Dx()/2, r.Dy()/2))}
		s.update(g)
	}
	pick()
	b := s.countRects(g)
	held := g.st.Items["potion"]
	tap(b.plus)
	tap(b.plus)
	tap(b.minus)
	if s.count != 2 {
		t.Fatalf("plus, plus, minus made the count %d", s.count)
	}
	tap(image.Rect(b.minus.Max.X, b.box.Min.Y, b.plus.Min.X, b.minus.Min.Y)) // the item's name
	if s.pick == "" || g.st.Items["potion"] != held {
		t.Fatal("a tap on the box beside its buttons traded or closed it")
	}
	tap(b.cancel)
	if s.pick != "" || g.st.Items["potion"] != held {
		t.Fatal("the cancel button did not go back without buying")
	}
	pick()
	tap(image.Rect(b.box.Min.X, b.box.Max.Y+10, b.box.Max.X, b.box.Max.Y+30))
	if s.pick != "" || g.st.Items["potion"] != held {
		t.Fatal("a tap outside the box did not go back without buying")
	}
	pick()
	tap(b.plus)
	tap(b.accept)
	if s.pick != "" || g.st.Items["potion"] != held+2 || g.st.Gold != 80 {
		t.Fatalf("the buy button: potions %d (were %d), gold %d", g.st.Items["potion"], held, g.st.Gold)
	}
}

// TestShopSaysWhenATradeIsRefused: the message follows what the rules did, not
// what was asked.
func TestShopSaysWhenATradeIsRefused(t *testing.T) {
	g := mnuShopGame(t, 100)
	s, _ := mnuOpenShop(g, "bazaar")
	mnuPress(g, btnOK) // buy
	s.goods.sel = indexOf(s.ids, "potion")
	mnuPress(g, btnOK)
	g.st.Gold = 0 // the purse empties while the box is open
	held := g.st.Items["potion"]
	mnuPress(g, btnOK)
	if want := fmt.Sprintf(lang.T("%sを %dこ かった！"), g.db.Items["potion"].Name, 1); g.st.Items["potion"] != held || s.msg == want || s.msg == "" {
		t.Fatalf("a refused purchase said %q with %d potions (were %d)", s.msg, g.st.Items["potion"], held)
	}
}
