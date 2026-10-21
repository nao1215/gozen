package play

import (
	"strings"
	"testing"
	"testing/fstest"

	lang "github.com/nao1215/gozen/internal/text"
)

const mnuShopData = `
shops:
  bazaar: {name: Bazaar, goods: [potion, blade, stick, hat], greeting: Welcome in, spent: var.spent}
  stall: {name: Stall, goods: [potion], sell: false}
  keeper: {name: Keeper shop, goods: [potion, key], keeper: x.png}
`

// mnuShopGame is the camp game with three shops; the party is the scout and the guide.
func mnuShopGame(t *testing.T, gold int) *Game {
	t.Helper()
	fsys := gameFiles(t)
	fsys["data/shops.yaml"] = &fstest.MapFile{Data: []byte(mnuShopData)}
	g, err := New(Options{FS: fsys, NoSound: true, SaveDir: t.TempDir(), Location: "camp", Seed: 3})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := g.Close(); err != nil {
			t.Error(err)
		}
	})
	g.st.Join("guide")
	g.st.Gold = gold
	return g
}

// mnuOpenShop pushes the shop and returns it with a counter of its done callback.
func mnuOpenShop(g *Game, id string) (*shop, *int) {
	left := 0
	s := newShop(g, id, func() { left++ })
	g.push(s)
	return s, &left
}

func TestShopBuyAndSellRules(t *testing.T) {
	g := mnuShopGame(t, 100)
	s, left := mnuOpenShop(g, "bazaar")
	if s.msg != "Welcome in" {
		t.Fatalf("greeting %q", s.msg)
	}
	mnuDrawAll(g)
	mnuPress(g, btnOK) // buy
	if s.mode != "buy" || len(s.ids) != 4 {
		t.Fatalf("mode %q goods %v", s.mode, s.ids)
	}
	// the blade costs exactly the gold we have: it is on offer; after buying one the next is not
	for i, id := range s.ids {
		if id == "blade" {
			s.goods.sel = i
		}
	}
	mnuPress(g, btnOK)
	if s.pick != "blade" || s.count != 1 {
		t.Fatalf("pick %q count %d", s.pick, s.count)
	}
	mnuPress(g, btnUp) // only one is affordable
	if s.count != 1 {
		t.Fatalf("count %d beyond the money", s.count)
	}
	mnuDrawAll(g)
	mnuPress(g, btnOK)
	if g.st.Gold != 0 || g.st.Items["blade"] != 1 {
		t.Fatalf("gold %d blades %d", g.st.Gold, g.st.Items["blade"])
	}
	if got := g.st.Vars["spent"]; got != float64(100) && got != 100 {
		t.Fatalf("spent var %v", got)
	}
	if !strings.Contains(s.msg, "1") || s.msgT != 90 {
		t.Fatalf("msg %q", s.msg)
	}
	for i, row := range s.goods.items {
		if want := s.ids[i] != "hat"; row.disabled != want { // the cap is free
			t.Fatalf("row %s disabled %v with no gold", s.ids[i], row.disabled)
		}
	}
	s.goods.sel = 0
	mnuPress(g, btnOK) // disabled row: nothing happens
	if s.pick != "" {
		t.Fatal("chose an unaffordable item")
	}
	mnuPress(g, btnCancel)
	if s.mode != "" {
		t.Fatalf("mode %q", s.mode)
	}
	// sell: key items and the unpriced are not offered; the price is half
	mnuPress(g, btnDown)
	mnuPress(g, btnOK)
	if s.mode != "sell" {
		t.Fatalf("mode %q", s.mode)
	}
	for _, id := range s.ids {
		if id == "key" || id == "hat" {
			t.Fatalf("%s offered for sale", id)
		}
	}
	mnuDrawAll(g)
	for i, id := range s.ids {
		if id == "potion" {
			s.goods.sel = i
		}
	}
	mnuPress(g, btnOK)
	mnuPress(g, btnUp)
	mnuPress(g, btnUp) // two potions only
	if s.count != 2 {
		t.Fatalf("count %d", s.count)
	}
	mnuPress(g, btnDown)
	mnuPress(g, btnDown)
	if s.count != 1 {
		t.Fatalf("count %d", s.count)
	}
	mnuPress(g, btnCancel) // back to the list without selling
	if g.st.Items["potion"] != 2 || s.pick != "" {
		t.Fatal("cancel sold something")
	}
	mnuPress(g, btnOK)
	mnuPress(g, btnUp)
	mnuPress(g, btnOK)
	if g.st.Items["potion"] != 0 || g.st.Gold != 10 {
		t.Fatalf("potions %d gold %d", g.st.Items["potion"], g.st.Gold)
	}
	mnuDrawAll(g)
	mnuPress(g, btnCancel)
	mnuPress(g, btnCancel) // out of the shop
	if *left != 1 || mnuTop(g) == scene(s) {
		t.Fatalf("done called %d times, shop still open %v", *left, mnuTop(g) == scene(s))
	}
}

func TestShopExitEntryAndNoSelling(t *testing.T) {
	g := mnuShopGame(t, 25)
	s, left := mnuOpenShop(g, "stall")
	if !s.top.items[1].disabled || s.top.items[0].disabled {
		t.Fatalf("top menu %+v", s.top.items)
	}
	mnuPress(g, btnDown)
	mnuPress(g, btnOK) // selling is off here
	if s.mode != "" {
		t.Fatalf("sold at a shop that does not buy: %q", s.mode)
	}
	mnuPress(g, btnUp)
	mnuPress(g, btnOK) // buy
	// buy two potions of 25 gold: 10 each
	mnuPress(g, btnOK)
	if s.pick != "potion" {
		t.Fatalf("pick %q", s.pick)
	}
	mnuPress(g, btnRight)
	mnuPress(g, btnRight)
	if s.count != 2 {
		t.Fatalf("count %d", s.count)
	}
	mnuPress(g, btnOK)
	if g.st.Gold != 5 || g.st.Items["potion"] != 4 {
		t.Fatalf("gold %d potions %d", g.st.Gold, g.st.Items["potion"])
	}
	mnuPress(g, btnCancel)
	mnuPress(g, btnDown)
	mnuPress(g, btnDown)
	mnuPress(g, btnOK) // the exit entry
	if *left != 1 {
		t.Fatalf("done %d", *left)
	}
}

func TestShopBuyWithoutMoneyAndSellNothing(t *testing.T) {
	g := mnuShopGame(t, 0)
	g.st.Items = map[string]int{}
	s, _ := mnuOpenShop(g, "bazaar")
	mnuPress(g, btnOK)
	mnuPress(g, btnOK) // nothing is affordable
	if s.pick != "" || g.st.Items["potion"] != 0 {
		t.Fatal("bought with no gold")
	}
	if g.st.Buy("bazaar", "potion", 1) || g.st.Buy("bazaar", "potion", 0) || g.st.Buy("bazaar", "nope", 1) {
		t.Fatal("State.Buy accepted a purchase it cannot pay for")
	}
	mnuPress(g, btnCancel)
	mnuPress(g, btnDown)
	mnuPress(g, btnOK) // sell with an empty bag
	if s.mode != "sell" || len(s.ids) != 0 {
		t.Fatalf("mode %q ids %v", s.mode, s.ids)
	}
	mnuPress(g, btnOK) // an empty list: nothing to pick
	if s.pick != "" {
		t.Fatal("picked from an empty list")
	}
	mnuDrawAll(g)
	if g.st.Sell("bazaar", "key", 1) || g.st.Sell("bazaar", "potion", 1) {
		t.Fatal("sold something that is not in the bag")
	}
}

func TestShopKeeperAndGearNotes(t *testing.T) {
	g := mnuShopGame(t, 1000)
	s, _ := mnuOpenShop(g, "keeper")
	mnuDrawAll(g)
	if r := s.right(g); r >= 1280-36 {
		t.Fatalf("goods may run under the keeper: right edge %v", r)
	}
	if w := s.goodsW(g); w < 420 || w > 600 {
		t.Fatalf("goods width %v", w)
	}
	// gear notes: the blade is an upgrade for the scout (↑); the worn stick is "equipped"
	b := g
	if cs := gearCompare(b, "potion"); cs != nil {
		t.Fatalf("a potion compared as gear: %+v", cs)
	}
	note, _ := gearNote(b, gearCompare(b, "blade"))
	if note != "↑" {
		t.Fatalf("blade note %q", note)
	}
	b.st.AddItem("stick", 1)
	if err := b.st.Equip(b.st.Members()[1], "weapon", "stick"); err != nil {
		t.Fatal(err)
	}
	note, _ = gearNote(b, gearCompare(b, "stick"))
	if note != lang.T("装備中") {
		t.Fatalf("worn stick note %q", note)
	}
	if note, c := gearNote(b, nil); note != "" || c != nil {
		t.Fatal("note for nothing")
	}
	// a downgrade: wear the blade, then look at the stick
	sc := b.st.Members()[0]
	b.st.AddItem("blade", 1)
	if err := b.st.Equip(sc, "weapon", "blade"); err != nil {
		t.Fatal(err)
	}
	// the guide wears the stick still, the scout the blade: nobody gains
	b.st.AddItem("blade", 1)
	if err := b.st.Equip(b.st.Members()[1], "weapon", "blade"); err != nil {
		t.Fatal(err)
	}
	note, _ = gearNote(b, gearCompare(b, "stick"))
	if note != "↓" {
		t.Fatalf("stick note when worse for all: %q", note)
	}
	// the shop view draws the comparison panel for gear
	b2, _ := mnuOpenShop(b, "bazaar")
	mnuPress(b, btnOK)
	for i, id := range b2.ids {
		if id == "stick" {
			b2.goods.sel = i
		}
	}
	mnuDrawAll(b)
}
