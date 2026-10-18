package battle_test

import (
	"strings"
	"testing"

	"github.com/nao1215/gozen/battle"
	data "github.com/nao1215/gozen/internal/layer2/gamedata"
	"github.com/nao1215/gozen/internal/testgame"
)

func ex(s string) data.Expr { return data.Expr{Expr: testgame.Expr(s)} }

// TestPassives: battle.passives は when が 成り立つ ときだけ 戦いの 始めに 付き、stats_by の 倍率は var.* で 変わる。
// 戦いの 後には 残らない。
func TestPassives(t *testing.T) {
	db := testgame.Load(t, nil)
	db.States["strong"] = &data.State{Name: "strong", StatsBy: map[string]data.Expr{"atk": ex("1 + 0.5 * var.n")}}
	db.Order["states"] = append(db.Order["states"], "strong")
	db.Game.Battle.Passives = []data.Passive{{Actor: "scout", State: "strong", When: ex("var.n > 0")}}
	st := testgame.State(db, 1)
	base := battle.New(db, st, "one").Party[0].Stat("atk")
	st.Vars["n"] = 2.0
	b := battle.New(db, st, "one")
	scout := b.Party[0]
	if scout.States["strong"] == 0 || scout.Stat("atk") != base*2 {
		t.Fatalf("passive: states %v, atk %d (want %d)", scout.States, scout.Stat("atk"), base*2)
	}
	b.End()
	if _, ok := st.Actors["scout"].States["strong"]; ok {
		t.Fatal("the passive stayed after the battle")
	}
}

// TestEndure: endure の 回数だけ、倒れる 一撃を HP 1 で 耐える。
func TestEndure(t *testing.T) {
	extra := map[string]string{"data/e.yaml": `
skills:
  smash: {name: Smash, scope: one_enemy, effects: [{damage: "999", variance: 0}]}
enemies:
  brute:
    name: Brute
    image: x.png
    stats: {mhp: 999, mmp: 0, atk: 1, def: 99, mat: 1, mdf: 1, agi: 99, luk: 1}
    ai: [{use: smash}]
troops:
  brute: {members: [brute]}
`}
	db := testgame.Load(t, extra)
	db.States["tough"] = &data.State{Name: "tough", Endure: ex("1")}
	db.Order["states"] = append(db.Order["states"], "tough")
	st := testgame.State(db, 1)
	b := battle.New(db, st, "brute")
	scout := b.Party[0]
	scout.States["tough"] = 99
	steps := run(b, battle.Action{User: scout, Skill: "guard"})
	if scout.HP != 1 || !strings.Contains(texts(steps), "ふみとどまった") {
		t.Fatalf("first blow: hp %d\n%s", scout.HP, texts(steps))
	}
	run(b, battle.Action{User: scout, Skill: "guard"})
	if scout.HP != 0 {
		t.Fatalf("endured twice with endure 1: hp %d", scout.HP)
	}
}

// TestAgain: again の 確率 1 なら、ふつうの 攻撃が 続けて 4 回（もう一度は 3 回まで）。
func TestAgain(t *testing.T) {
	extra := map[string]string{"data/a.yaml": `
enemies:
  wall:
    name: Wall
    image: x.png
    stats: {mhp: 9999, mmp: 0, atk: 1, def: 0, mat: 1, mdf: 1, agi: 1, luk: 1}
    ai: [{use: slap}]
troops:
  wall: {members: [wall]}
`}
	db := testgame.Load(t, extra)
	db.States["chain"] = &data.State{Name: "chain", Again: ex("1")}
	db.Order["states"] = append(db.Order["states"], "chain")
	st := testgame.State(db, 1)
	b := battle.New(db, st, "wall")
	scout, wall := b.Party[0], b.Foes[0]
	scout.States["chain"] = 99
	steps := run(b, battle.Action{User: scout, Skill: "attack", Target: wall})
	n := 0
	for _, s := range steps {
		if s.Kind == battle.Damage && s.Target == wall {
			n++
		}
	}
	if n != 4 {
		t.Fatalf("hits %d, want 4\n%s", n, texts(steps))
	}
}

// TestTickAllies: on_turn_end_allies は 立って いる 味方 全員に 効く。
func TestTickAllies(t *testing.T) {
	db := testgame.Load(t, nil)
	db.States["prayer"] = &data.State{Name: "prayer", TickAllies: []data.Effect{{Heal: ex("5"), Quiet: true}}}
	db.Order["states"] = append(db.Order["states"], "prayer")
	st := testgame.State(db, 1)
	st.Join("guide")
	b := battle.New(db, st, "one")
	if len(b.Party) < 2 {
		t.Fatal("fixture must supply two party members")
	}
	scout, guide := b.Party[0], b.Party[1]
	scout.States["prayer"] = 99
	guide.HP = 3 // スライムの 平手（1）を 受けても 立って いる
	scout.HP = scout.MHP() - 10
	before := scout.HP
	steps := run(b, battle.Action{User: scout, Skill: "guard"}, battle.Action{User: guide, Skill: "guard"})
	for _, s := range steps {
		if s.Kind == battle.Heal && s.Text != "" {
			t.Errorf("a quiet heal has a text %q", s.Text)
		}
	}
	if guide.HP < 7 || scout.HP <= before-2 {
		t.Fatalf("guide %d, scout %d (before %d)", guide.HP, scout.HP, before)
	}
}
