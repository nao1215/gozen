package event_test

import (
	"testing"

	data "github.com/nao1215/gozen/internal/layer2/gamedata"
	"github.com/nao1215/gozen/internal/testgame"
)

// TestJoinCatchesUpToJoinExp は game.yaml の join_exp があると、遅れて加わる仲間の経験値が
// 加わる前のパーティで見た式の値まで上がり、全快して加わることを確かめる（下げない・式が無ければ上げない）。
func TestJoinCatchesUpToJoinExp(t *testing.T) {
	db := testgame.Load(t, nil)
	st := testgame.State(db, 1)
	st.Actors["scout"].GainExp(400) // Lv6 あたり
	join := []data.Command{{Join: "guide"}}

	// 式が無ければ、はじめの経験値のまま
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

	// もういる人に join しても何も変わらない。経験値が多い人は下げない
	f.GainExp(1000)
	st.Leave("guide")
	runAll(t, db, st, &host{}, join)
	if f.Exp != 1400 {
		t.Errorf("経験値が 下がった: %d", f.Exp)
	}
}
