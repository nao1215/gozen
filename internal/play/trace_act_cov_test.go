package play

import (
	"bytes"
	"testing"
)

// The direct actions a trace can carry (replay's "act" lines): the screen's menus do the same
// work by hand, a replay and the test player's notes do it by name.

const wldActData = `items:
  charm: {name: Charm, kind: key, event: ev_charm}
shops:
  store: {name: Store, goods: [potion, blade]}
events:
  ev_charm:
    do:
      - set: {flag.charmed: true}
`

func wldActGame(t *testing.T) (*Game, *bytes.Buffer) {
	t.Helper()
	var buf bytes.Buffer
	opts := Options{FS: wldWorldFiles(t, "", map[string]string{"data/act_cov.yaml": wldActData}), Location: "camp", Trace: &buf, Seed: 3}
	g := wldOpenWith(t, opts)
	wldFrames(t, g, 5)
	return g, &buf
}

func TestWldActBuyAndUseItem(t *testing.T) {
	g, buf := wldActGame(t)
	gold, potions := g.st.Gold, g.st.Items["potion"]
	if !g.act("buy", traceLine{"shop": "store", "item": "potion", "count": 2.0}) {
		t.Fatal("buying two potions with enough gold failed")
	}
	if g.st.Gold != gold-20 || g.st.Items["potion"] != potions+2 {
		t.Fatalf("gold %d items %d after buying two potions at 10", g.st.Gold, g.st.Items["potion"])
	}
	if g.act("buy", traceLine{"shop": "store", "item": "blade", "count": 5.0}) {
		t.Fatal("bought five blades with the gold of a few")
	}
	if g.st.Gold != gold-20 {
		t.Fatalf("a failed purchase changed the gold: %d", g.st.Gold)
	}
	// without a shop the price is taken as given
	if !g.act("buy", traceLine{"item": "potion", "count": 1.0, "price": 4.0}) || g.st.Gold != gold-24 {
		t.Fatalf("a priced purchase: gold %d", g.st.Gold)
	}
	g.st.Items["charm"] = 1
	if !g.act("use_item", traceLine{"item": "charm"}) {
		t.Fatal("using an item with an event failed")
	}
	wldFrames(t, g, 5)
	if !g.st.Flags["charmed"] {
		t.Fatal("the item's event did not run")
	}
	if g.act("use_item", traceLine{"item": "potion"}) {
		t.Fatal("an item without an event cannot be used by name")
	}
	if n := bytes.Count(buf.Bytes(), []byte(`"t":"act"`)); n != 5 {
		t.Fatalf("%d act lines in the trace, want one for each of the 5 actions", n)
	}
}

func TestWldActEquipRunAndRefusals(t *testing.T) {
	g, _ := wldActGame(t)
	g.st.AddItem("blade", 1)
	if !g.act("equip_best", traceLine{"actor": "scout"}) {
		t.Fatal("equip_best failed for a member")
	}
	if got := g.st.Actors["scout"].Equip["weapon"]; got != "blade" {
		t.Fatalf("the strongest weapon is the blade, the member holds %q", got)
	}
	if g.act("equip_best", traceLine{"actor": "nobody"}) || g.act("raise", traceLine{"actor": "nobody", "node": "x"}) {
		t.Fatal("an actor who has not joined was acted on")
	}
	if g.act("battle_auto", nil) {
		t.Fatal("battle_auto outside a battle did something")
	}
	if g.act("fly", nil) {
		t.Fatal("an unknown action succeeded")
	}
	g.st.Actors["scout"].HP = 1
	if !g.act("run", traceLine{"location": "camp", "action": 0.0}) {
		t.Fatal("running the camp's first action failed")
	}
	wldFrames(t, g, 10)
	if a := g.st.Actors["scout"]; a.HP != a.Stat("mhp") {
		t.Fatalf("Rest left %d of %d HP", a.HP, a.Stat("mhp"))
	}
	if g.act("run", traceLine{"location": "camp", "action": 99.0}) || g.act("run", traceLine{"location": "nowhere", "action": 0.0}) {
		t.Fatal("an action that does not exist ran")
	}
}
