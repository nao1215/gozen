package play

import (
	"slices"
	"testing"
)

const sysRequests = `shops:
  general:
    name: General store
    greeting: Take a look around.
    goods: [potion]
enemies:
  doom:
    name: Doom
    image: x.png
    stats: {mhp: 99999, mmp: 0, atk: 9999, def: 9999, mat: 1, mdf: 1, agi: 99, luk: 1}
    ai: [{use: attack}]
troops:
  doom: {members: [doom]}
events:
  fight:
    do:
      - say: "Ahead."
      - battle: one
        on_win: [{set: {flag.won: true}}]
      - say: "After."
  doomed:
    do:
      - say: "Doom."
      - battle: doom
      - say: "Never shown."
  browse:
    do:
      - shop: general
      - set: {flag.left_shop: true}
  keep:
    do:
      - save: true
      - set: {flag.kept: true}
  pause:
    do:
      - wait: 0.5
      - wait: {seconds: 30, skip: false}
      - set: {flag.waited: true}
`

// TestEventBattleWinContinuesTheEvent: a battle started by an event, won, runs on_win and
// the lines after it; the script films the battle.
func TestEventBattleWinContinuesTheEvent(t *testing.T) {
	g := sysOpen4(t, Options{Seed: 1, Location: "camp", Event: "fight", Line: 1, Script: "play fight 10\nquit"},
		map[string]string{"data/req.yaml": sysRequests})
	if !sysFrames(t, g, 20000) {
		t.Fatal("the script did not end")
	}
	if !g.st.Flags["won"] {
		t.Fatal("on_win did not run")
	}
	filmed := slices.ContainsFunc(sysShotNames, func(n string) bool { return len(n) > 7 && n[:7] == "fight_b" })
	if !filmed {
		t.Fatalf("the battle was not filmed: %v", sysShotNames)
	}
	if g.st.Fighters != nil {
		t.Fatal("the fighters of the duel were not cleared")
	}
}

// TestEventBattleLossWithoutOnLoseIsGameOver: losing a battle that has no on_lose ends in the
// game over screen and the lines after the battle never show.
func TestEventBattleLossWithoutOnLoseIsGameOver(t *testing.T) {
	g := sysOpen4(t, Options{Seed: 1, Location: "camp", Event: "doomed", Line: 1, Script: "play doom 50\nwait 5\nquit"},
		map[string]string{"data/req.yaml": sysRequests})
	for range 30000 {
		if err := g.Update(); err != nil {
			t.Fatal(err)
		}
		sysSafeDraw(g)
		if _, ok := g.stack[0].(*gameOver); ok {
			break
		}
	}
	if _, ok := g.stack[0].(*gameOver); !ok {
		t.Fatalf("scene %T, want the game over screen", g.stack[0])
	}
	if g.talk.say.Text == "Never shown." {
		t.Fatal("the line after a lost battle was shown")
	}
	if g.events.Running() {
		t.Fatal("the event kept running after the game over")
	}
}

// TestEventShopOpensAndReturns: the shop command opens the shop screen and the event goes on
// when it is closed.
func TestEventShopOpensAndReturns(t *testing.T) {
	g := sysOpen4(t, Options{Seed: 1, Location: "camp", Event: "browse", Line: 1}, map[string]string{"data/req.yaml": sysRequests})
	for range 200 {
		sysFrames(t, g, 1)
		if len(g.stack) > 1 {
			break
		}
	}
	if len(g.stack) < 2 {
		t.Fatal("the shop did not open")
	}
	g.fast = nil // --event reads on by itself; the player takes over now
	if g.st.Flags["left_shop"] {
		t.Fatal("the event went on while the shop was open")
	}
	if !g.busy() {
		t.Fatal("an open shop should make the game busy")
	}
	for range 300 {
		sysFramesWithKey(t, g, btnCancel)
		if g.st.Flags["left_shop"] {
			break
		}
	}
	if !g.st.Flags["left_shop"] {
		t.Fatal("closing the shop did not continue the event")
	}
}

// TestEventSaveCommandOpensTheSaveScreen: the save command opens the save screen; choosing a
// slot writes it and the event goes on.
func TestEventSaveCommandOpensTheSaveScreen(t *testing.T) {
	g := sysInnWith(t, Options{Seed: 1}, map[string]string{"data/req.yaml": sysRequests})
	g.events.Start(g.db.Events["keep"].Do)
	for range 100 {
		sysFrames(t, g, 1)
		if _, ok := g.stack[len(g.stack)-1].(*saveMenu); ok {
			break
		}
	}
	sm, ok := g.stack[len(g.stack)-1].(*saveMenu)
	if !ok {
		t.Fatalf("top is %T", g.stack[len(g.stack)-1])
	}
	sysKey(g, btnOK)
	sm.update(g)
	for sm.msgT > 0 {
		sysKey(g)
		sm.update(g)
	}
	sysFrames(t, g, 5)
	if !g.st.Flags["kept"] {
		t.Fatal("the event did not go on after the save")
	}
	if !g.storeHas(g.slotPath(0)) {
		t.Fatal("slot 1 was not written")
	}
}

// TestWaitCommandCountsTicksAndSkipRespectsNoSkip: wait holds the event for its seconds; in skip
// mode a skippable wait is cut short and one marked skip: false is not.
func TestWaitCommandCountsTicksAndSkipRespectsNoSkip(t *testing.T) {
	g := sysOpen4(t, Options{Seed: 1, Location: "camp"}, map[string]string{"data/req.yaml": sysRequests})
	g.events.Start(g.db.Events["pause"].Do)
	n := 0
	for ; !g.st.Flags["waited"] && n < 2000; n++ {
		sysFrames(t, g, 1)
		if g.waitT > 0 && g.req != nil && n == 1 {
			g.skipping = true
		}
	}
	if !g.st.Flags["waited"] {
		t.Fatal("the event did not finish")
	}
	// 0.5 s is cut short by skipping, the 30 s one that forbids skipping is not: it takes the
	// full 1800 ticks.
	if n < 1700 {
		t.Fatalf("finished after %d ticks, the unskippable wait was cut", n)
	}
}
