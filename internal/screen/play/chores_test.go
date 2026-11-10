package play

import (
	"fmt"
	"image"
	"testing"
)

// gearOffer is the notice on the screen that offers the strongest equipment.
func gearOffer(t *testing.T, g *Game) notice {
	t.Helper()
	for _, n := range g.notices.list {
		if n.act != nil {
			return n
		}
	}
	t.Fatalf("no offer among the notices: %+v", g.notices.list)
	return notice{}
}

// TestOfferGearDoesNotStopThePlay: stronger equipment in the bag puts no
// question over the field; the party can walk on.
func TestOfferGearDoesNotStopThePlay(t *testing.T) {
	g := sysOpen(t, Options{Location: "camp", Items: []string{"blade"}, SaveDir: t.TempDir()}, nil)
	sysFrames(t, g, 12)
	if len(g.stack) != 1 || g.busy() {
		t.Fatalf("the offer stopped the play with %T", g.stack[len(g.stack)-1])
	}
}

// TestOfferGearIsATappableNotice: a stronger item in the bag is offered once by
// a notice that asks nothing (the field stays the scene on top and goes on
// taking input); a tap on it equips the best, and it is not offered again.
func TestOfferGearIsATappableNotice(t *testing.T) {
	g := sysOpen(t, Options{Location: "camp", Items: []string{"blade"}, SaveDir: t.TempDir()}, nil)
	sysFrames(t, g, 12)
	offer := gearOffer(t, g)
	if offer.at[2] == 0 {
		t.Fatalf("the offer was not drawn: %+v", offer)
	}
	sc, err := parseScript(fmt.Sprintf("click %d %d\nquit", int(offer.at[0]+offer.at[2]/2), int(offer.at[1]+offer.at[3]/2)))
	if err != nil {
		t.Fatal(err)
	}
	g.in.script = sc
	sysFrames(t, g, 2)
	g.in.script = nil
	if got := g.st.Members()[0].Equip["weapon"]; got != "blade" {
		t.Fatalf("weapon %q after a tap on the offer", got)
	}
	sysFrames(t, g, 25)
	for _, n := range g.notices.list {
		if n.act != nil {
			t.Fatal("offered again after equipping")
		}
	}
	if n := g.equipBestAll(); n != 0 {
		t.Fatalf("nothing left to equip, got %d", n)
	}
}

// TestOfferGearPassesByItself: an offer left alone goes away, equips nothing
// and is not made again for the same bag.
func TestOfferGearPassesByItself(t *testing.T) {
	g := sysOpen(t, Options{Location: "camp", Items: []string{"blade"}, SaveDir: t.TempDir()}, nil)
	sysFrames(t, g, 12)
	offer := gearOffer(t, g)
	sysFrames(t, g, offer.total+40)
	if len(g.notices.list) != 0 {
		t.Fatalf("the offer stays: %+v", g.notices.list)
	}
	if got := g.st.Members()[0].Equip["weapon"]; got == "blade" {
		t.Fatal("equipped although the offer was left alone")
	}
}

// TestNoticeTapNeedsTheOfferOnTop: a tap takes an offer only where it shows
// over the scene alone; a notice that only tells takes none.
func TestNoticeTapNeedsTheOfferOnTop(t *testing.T) {
	g := sysOpen(t, Options{Location: "camp", SaveDir: t.TempDir()}, nil)
	sysFrames(t, g, 12)
	done := 0
	g.notices.add("Found a coin")
	g.notices.offer("Press me", func() { done++ })
	sysSafeDraw(g)
	told, offer := g.notices.list[0], g.notices.list[1]
	g.in = input{click: true, mouse: image.Pt(int(told.at[0])+4, int(offer.at[1])-20)}
	if g.tapNotice() || done != 0 {
		t.Fatal("a tap on a plain notice was taken")
	}
	g.in = input{click: true, mouse: image.Pt(int(offer.at[0])+4, int(offer.at[1])+4)}
	g.push(newConfirm("Sure?", func() {}))
	if g.tapNotice() || done != 0 {
		t.Fatal("an offer under a question took the tap")
	}
	g.pop()
	if !g.tapNotice() || done != 1 || len(g.notices.list) != 1 {
		t.Fatalf("a tap on the offer: done %d, notices %+v", done, g.notices.list)
	}
}
