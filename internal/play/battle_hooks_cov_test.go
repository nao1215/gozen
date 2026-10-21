package play

import (
	"testing"
)

const btlHookEvents = `
events:
  pre: {do: [{set: {var.pre_runs: "var.pre_runs + 1"}}]}
  post: {do: [{set: {var.post_runs: "var.post_runs + 1"}}]}
  leave: {do: [{set: {flag.left: true}}, {move: vault}]}
  fight_one: {do: [{battle: one, on_win: [{set: {flag.won: true}}]}, {set: {flag.after_fight: true}}]}
  fight_doom: {do: [{battle: doom}, {set: {flag.after_fight: true}}]}
`

func btlHooks(t *testing.T, before, after string) *Game {
	t.Helper()
	g, _ := btlOpen(t, Options{Location: "field"}, "", btlHookEvents)
	g.st.Vars["pre_runs"], g.st.Vars["post_runs"] = 0.0, 0.0
	g.db.Game.Battle.Before, g.db.Game.Battle.After = before, after
	return g
}

// btlFightEvent starts an event and fights its battle with the attack command until the
// event is over.
func btlFightEvent(t *testing.T, g *Game, name string) {
	t.Helper()
	g.events.Start(g.db.Events[name].Do)
	for range 20000 {
		if !g.events.Running() && g.req == nil && g.currentBattle() == nil {
			return
		}
		if s := g.currentBattle(); s != nil && s.phase == bpInput && s.target == nil && s.sub == nil {
			btlKey(g, btnOK)
			btlKey(g, btnOK)
			continue
		}
		if s := g.currentBattle(); s != nil && s.phase == bpResult {
			g.in.pressed[btnOK] = true
			g.tick++
			s.update(g)
			g.in.pressed = [numButtons]bool{}
		}
		if _, over := g.stack[len(g.stack)-1].(*gameOver); over {
			return
		}
		if g.talk.active && (g.talk.choice != nil || g.talk.shown >= g.talk.total) {
			btlKey(g, btnOK)
		}
		btlFrame(t, g)
	}
	t.Fatal("the event did not finish")
}

// TestBattleHooksOnAWin: the event named by battle.before runs once before the battle starts and the
// one named by battle.after when the result has been dealt with; the win branch runs between.
func TestBattleHooksOnAWin(t *testing.T) {
	g := btlHooks(t, "pre", "post")
	btlFightEvent(t, g, "fight_one")
	if g.st.Vars["pre_runs"] != 1.0 || g.st.Vars["post_runs"] != 1.0 {
		t.Fatalf("before ran %v times, after %v times", g.st.Vars["pre_runs"], g.st.Vars["post_runs"])
	}
	if !g.st.Flags["won"] || !g.st.Flags["after_fight"] {
		t.Fatalf("flags %v", g.st.Flags)
	}
}

// TestBattleHooksOnAnOrdinaryDefeat: a defeat with no defeat branch ends in the game-over screen and the
// post-processing does not run; the pre-processing ran once.
func TestBattleHooksOnAnOrdinaryDefeat(t *testing.T) {
	g := btlHooks(t, "pre", "post")
	g.events.Start(g.db.Events["fight_doom"].Do)
	for range 20000 {
		if _, over := g.stack[len(g.stack)-1].(*gameOver); over {
			break
		}
		s := g.currentBattle()
		switch {
		case s != nil && s.phase == bpInput && s.target == nil:
			btlChoose(g, s, 2)
			btlChoose(g, s, 2)
		case s != nil && s.phase == bpResult:
			g.in.pressed[btnOK] = true
			g.tick++
			s.update(g)
			g.in.pressed = [numButtons]bool{}
		}
		btlFrame(t, g)
	}
	if _, over := g.stack[len(g.stack)-1].(*gameOver); !over {
		t.Fatalf("top is %T, want the game-over screen", g.stack[len(g.stack)-1])
	}
	if g.st.Vars["pre_runs"] != 1.0 || g.st.Vars["post_runs"] != 0.0 {
		t.Fatalf("before ran %v times, after %v times", g.st.Vars["pre_runs"], g.st.Vars["post_runs"])
	}
	if g.st.Flags["after_fight"] {
		t.Fatal("the rest of the event ran after the party fell")
	}
}

// TestBattleHooksThatMoveTheParty: when the pre-processing moves the party, the battle is not
// fought and the rest of the event is dropped.
func TestBattleHooksThatMoveTheParty(t *testing.T) {
	g := btlHooks(t, "leave", "post")
	g.events.Start(g.db.Events["fight_one"].Do)
	for range 300 {
		btlFrame(t, g)
	}
	if g.currentBattle() != nil {
		t.Fatal("a battle was fought after the party was moved away")
	}
	if g.st.Location != "vault" || !g.st.Flags["left"] {
		t.Fatalf("location %q flags %v", g.st.Location, g.st.Flags)
	}
	if g.st.Flags["after_fight"] || g.st.Flags["won"] {
		t.Fatal("the rest of the event ran")
	}
}
