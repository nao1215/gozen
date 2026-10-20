package event_test

import (
	"testing"

	data "github.com/nao1215/gozen/internal/layer2/gamedata"
	"github.com/nao1215/gozen/internal/testgame"
)

// TestJoinCatchesUpToJoinExp verifies that with join_exp in game.yaml, the experience of a
// late-joining member rises to the
// value of the expression seen from the party before joining, and they join fully healed (never
// lowered; not raised if there is no expression).
func TestJoinCatchesUpToJoinExp(t *testing.T) {
	db := testgame.Load(t, nil)
	st := testgame.State(db, 1)
	st.Actors["scout"].GainExp(400) // around Lv6
	join := []data.Command{{Join: "guide"}}

	// If there is no expression, the starting experience is kept
	runAll(t, db, st, &host{}, join)
	if got := st.Actors["guide"].Exp; got > 40 {
		t.Fatalf("join_exp が無いのに 経験値 %d", got)
	}
	st.Leave("guide")
	delete(st.Actors, "guide")

	db.Game.JoinExp = data.Expr{Expr: testgame.Expr("party.avg_exp")}
	runAll(t, db, st, &host{}, join)
	f := st.Actors["guide"]
	if f.Exp != 400 || f.Level != st.Actors["scout"].Level {
		t.Fatalf("追いついていない: 経験値 %d Lv%d（勇者 Lv%d）", f.Exp, f.Level, st.Actors["scout"].Level)
	}
	if f.HP != f.Stat("mhp") || f.MP != f.Stat("mmp") {
		t.Errorf("全快していない: HP %d/%d MP %d/%d", f.HP, f.Stat("mhp"), f.MP, f.Stat("mmp"))
	}

	// Joining someone already in the party changes nothing. Someone with more experience is not
	// lowered
	f.GainExp(1000)
	st.Leave("guide")
	runAll(t, db, st, &host{}, join)
	if f.Exp != 1400 {
		t.Errorf("経験値が 下がった: %d", f.Exp)
	}
}
