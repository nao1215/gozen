package play

import (
	"math"
	"reflect"
	"testing"

	"github.com/nao1215/gozen/internal/battle"
	data "github.com/nao1215/gozen/internal/layer2/gamedata"
)

// TestBattleTempo: the battle's tempo shortens the waits (a tempo of 2 halves them), the player's
// battle speed setting multiplies it, and no value makes a wait shorter than one tick or longer
// than it is at the default.
func TestBattleTempo(t *testing.T) {
	g, _ := btlOpen(t, Options{Battle: "one"}, "", "")
	g.db.Game.Battle.Tempo = 0
	if got := g.tempo(34); got != 34 {
		t.Fatalf("default tempo: %d", got)
	}
	g.db.Game.Battle.Tempo = 2
	if got := g.tempo(34); got != 17 {
		t.Fatalf("tempo 2: %d", got)
	}
	g.cfg.BattleSpeed = 1
	if got := g.tempo(30); got != 10 {
		t.Fatalf("tempo 2 at speed 1.5: %d", got)
	}
	g.cfg.BattleSpeed = 99 // out of range: the fastest setting
	if got := g.tempo(33); got != 7 {
		t.Fatalf("speed out of range: %d", got)
	}
	g.cfg.BattleSpeed = -3
	g.db.Game.Battle.Tempo = 1
	if got := g.tempo(34); got != 34 {
		t.Fatalf("speed below range: %d", got)
	}
	for _, tempo := range []float64{-5, 0, 1e-9, 1e9, math.Inf(1), math.NaN()} {
		g.db.Game.Battle.Tempo = tempo
		if n := g.tempo(34); n < 1 || n > 34 && tempo > 0 && !math.IsNaN(tempo) && tempo >= 1 {
			t.Errorf("tempo %v: wait %d", tempo, n)
		}
		if n := g.fxTicks(34); n < 1 {
			t.Errorf("tempo %v: effect %d", tempo, n)
		}
	}
	g.db.Game.Battle.AutoTempo = 0
	if g.autoTempo() != 1 {
		t.Fatal("auto tempo below 1")
	}
	g.db.Game.Battle.AutoTempo = 3
	if g.autoTempo() != 3 {
		t.Fatal("auto tempo 3")
	}
}

// TestBattleEndsAtAnyTempo: whatever the tempo, a fought battle ends.
func TestBattleEndsAtAnyTempo(t *testing.T) {
	for _, tempo := range []float64{-1, 0.5, 50} {
		g, _ := btlOpen(t, Options{Battle: "one"}, "", "")
		g.db.Game.Battle.Tempo = tempo
		s := btlBattle(t, g)
		btlAttackUntilOver(t, g, s)
		if s.b.Result != battle.Won {
			t.Errorf("tempo %v: result %v", tempo, s.b.Result)
		}
		btlClose(t, g)
	}
}

// TestTacticsWithoutACatalog: a game that defines no tactic catalog still opens the screen; the rows
// are empty, a row can be started but not accepted, and everything can be left again.
func TestTacticsWithoutACatalog(t *testing.T) {
	g, _ := btlOpen(t, Options{Location: "camp"}, "", "")
	done := false
	g.openScreen("tactics", func() { done = true })
	s, ok := g.stack[len(g.stack)-1].(*tacticsScreen)
	if !ok {
		t.Fatalf("top is %T", g.stack[len(g.stack)-1])
	}
	if len(s.rows.items) != 0 {
		t.Fatalf("%d rows without a catalog", len(s.rows.items))
	}
	g.Draw(btlImage())
	btlTacticsAct(g, s, 0, tacticAdd)
	e := btlEditor(t, g)
	if e.row.Condition != "" || len(e.errors(g)) == 0 {
		t.Fatalf("a row without a catalog is usable: %+v", e.row)
	}
	g.Draw(btlImage())
	for _, f := range []int{tacticCondition, tacticTarget, tacticAction} {
		e.fields.sel = f
		e.refresh(g)
		btlKey(g, btnOK)
		btlKey(g, btnOK) // nothing to choose
		btlKey(g, btnCancel)
	}
	e.fields.sel = tacticRowAccept
	e.refresh(g)
	btlKey(g, btnOK)
	if g.stack[len(g.stack)-1] != e {
		t.Fatal("a row that cannot work was accepted")
	}
	e.fields.sel = tacticRowDetail
	e.refresh(g)
	btlKey(g, btnOK)
	btlKey(g, btnCancel)
	btlKey(g, btnCancel) // out of the editor
	btlKey(g, btnCancel) // out of the screen
	if !done || g.stack[len(g.stack)-1] == s {
		t.Fatal("the screen did not close")
	}
	if tt := g.st.Actors["scout"].Tactics; tt != nil && tt.Mode != "auto" {
		t.Fatalf("tactics %+v", tt)
	}
}

// TestTacticsScreenIsNotOpenedInABattle: the open command from a battle does nothing but report done.
func TestTacticsScreenIsNotOpenedInABattle(t *testing.T) {
	g, _ := btlOpen(t, Options{Battle: "one"}, "", "")
	n := len(g.stack)
	done := false
	g.openScreen("tactics", func() { done = true })
	if !done || len(g.stack) != n {
		t.Fatalf("done %v, stack %d -> %d", done, n, len(g.stack))
	}
}

// TestBattleKeepsTheDataItWasGiven: a battle only reads the game's definitions; after a full fight
// the skills, enemies and troops are as they were.
func TestBattleKeepsTheDataItWasGiven(t *testing.T) {
	g, _ := btlOpen(t, Options{Battle: "pack"}, "", "")
	before := map[string]data.Enemy{}
	for id, e := range g.db.Enemies {
		before[id] = *e
	}
	troops := len(g.db.Troops["pack"].Members)
	s := btlBattle(t, g)
	btlAttackUntilOver(t, g, s)
	for id, e := range g.db.Enemies {
		if !reflect.DeepEqual(e.Stats, before[id].Stats) || e.Name != before[id].Name || e.Exp != before[id].Exp {
			t.Errorf("enemy %s changed", id)
		}
	}
	if len(g.db.Troops["pack"].Members) != troops {
		t.Fatal("the troop changed")
	}
}
