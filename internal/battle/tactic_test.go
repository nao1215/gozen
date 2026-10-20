package battle_test

import (
	"reflect"
	"testing"

	"github.com/nao1215/gozen/internal/battle"
	data "github.com/nao1215/gozen/internal/layer2/gamedata"
	"github.com/nao1215/gozen/internal/rpg"
	"github.com/nao1215/gozen/internal/testgame"
)

func TestTacticCompileBattle(t *testing.T) {
	setup := func(t *testing.T) *battle.Battle {
		db := testgame.Load(t, map[string]string{"data/tactics.yaml": testgame.Tactics})
		db.Game.Battle.AI = "gambit"
		st := testgame.State(db, 19)
		st.Join("guide")
		st.Actors["scout"].Learnt = []string{"heal", "group_heal", "raise"}
		return battle.New(db, st, "two")
	}
	install := func(t *testing.T, b *battle.Battle, row data.TacticRow) {
		t.Helper()
		rule, errs := b.DB.CompileTactic(row, b.Party[0].Actor.Skills())
		if len(errs) != 0 || rule == nil {
			t.Fatalf("変換 %v", errs)
		}
		b.DB.Actors["scout"].AI = data.AIRef{Rules: []data.Rule{*rule, {Use: "attack"}}}
	}
	t.Run("全体の発動条件と効果範囲", func(t *testing.T) {
		b := setup(t)
		scout, guide := b.Party[0], b.Party[1]
		scout.HP, guide.HP = 10, guide.MHP()-5
		install(t, b, data.TacticRow{Enabled: true, Condition: "low_hp", Percent: new(50), Target: "allies", Action: "group_heal"})
		a := b.Decide(scout)
		if a.Skill != "group_heal" || a.Target != nil {
			t.Fatalf("全体回復を選ばない %+v", a)
		}
		steps := run(b, a, battle.Action{User: guide, Skill: "guard"})
		healed := map[*battle.Unit]int{}
		for _, s := range steps {
			if s.Kind == battle.Heal {
				healed[s.Target] += s.Amount
			}
		}
		if healed[scout] != 10 || healed[guide] != 5 {
			t.Fatalf("条件に当たらない仲間にも効果が必要 %v", healed)
		}
		scout.HP, guide.HP = scout.MHP(), guide.MHP()
		if a := b.Decide(scout); a.Skill != "attack" {
			t.Fatalf("条件不成立でも全体回復 %+v", a)
		}
	})
	t.Run("単体の状態と対象不在", func(t *testing.T) {
		b := setup(t)
		scout, guide := b.Party[0], b.Party[1]
		guide.States["poison"] = 2
		install(t, b, data.TacticRow{Enabled: true, Condition: "poisoned", Target: "ally", Action: "heal"})
		if a := b.Decide(scout); a.Skill != "heal" || a.Target != guide {
			t.Fatalf("状態で対象を絞らない %+v", a)
		}
		delete(guide.States, "poison")
		if a := b.Decide(scout); a.Skill != "attack" {
			t.Fatalf("対象不在で次へ進まない %+v", a)
		}
	})
	t.Run("自己と蘇生", func(t *testing.T) {
		b := setup(t)
		scout, guide := b.Party[0], b.Party[1]
		guide.HP = 0
		install(t, b, data.TacticRow{Enabled: true, Condition: "fallen", Target: "fallen", Action: "raise"})
		if a := b.Decide(scout); a.Skill != "raise" || a.Target != guide {
			t.Fatalf("倒れた仲間を選ばない %+v", a)
		}
		install(t, b, data.TacticRow{Enabled: true, Condition: "any", Target: "self", Action: "heal"})
		if a := b.Decide(scout); a.Target != scout {
			t.Fatalf("自己を選ばない %+v", a)
		}
	})
	t.Run("変換は乱数と状態を消費しない", func(t *testing.T) {
		b, untouched := setup(t), setup(t)
		before, err := b.St.Save()
		if err != nil {
			t.Fatal(err)
		}
		for range 10 {
			if _, errs := b.DB.CompileTactic(data.TacticRow{Enabled: true, Condition: "complex", Target: "complex", Action: "fire"}, b.Party[0].Actor.Skills()); len(errs) != 0 {
				t.Fatal(errs)
			}
		}
		after, err := b.St.Save()
		if err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(before, after) || b.Rand.Uint64() != untouched.Rand.Uint64() {
			t.Fatal("変換が状態か乱数を消費した")
		}
	})
}

func TestSimpleTacticsSkipAndTarget(t *testing.T) {
	db := testgame.Load(t, map[string]string{"data/tactics.yaml": testgame.Tactics})
	db.Game.Battle.SimpleTactics, db.Game.Battle.AI = true, "gambit"
	db.TacticActions["potion"].Target = "ally"
	db.TacticActions["heal"].Target = "ally"
	db.TacticActions["attack"].Target = "enemy"
	for _, tc := range []struct {
		name        string
		items, mp   int
		item, skill string
	}{
		{"薬あり", 1, 10, "potion", ""}, {"薬切れ", 0, 10, "", "heal"}, {"薬とMP切れ", 0, 0, "", "attack"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			st := testgame.State(db, 19)
			st.Join("guide")
			a := st.Actors["scout"]
			a.Learnt = []string{"heal"}
			a.MP = tc.mp
			st.Items["potion"] = tc.items
			plan := rpg.ActorTactics{Version: rpg.TacticsVersion, Mode: rpg.TacticsManual, Rows: []data.TacticRow{
				{Enabled: true, Condition: "any", Action: "potion"}, {Enabled: true, Condition: "any", Action: "heal"}, {Enabled: true, Condition: "any", Action: "attack"},
			}}
			if err := a.SetTactics(plan); err != nil {
				t.Fatal(err)
			}
			b := battle.New(db, st, "two")
			b.Party[1].HP = 1
			got := b.Decide(b.Party[0])
			if got.Item != tc.item || got.Skill != tc.skill {
				t.Fatalf("不足行を飛ばさない %+v", got)
			}
			if tc.skill != "attack" && got.Target != b.Party[1] {
				t.Fatal("最も傷の深い仲間を選ばない")
			}
			if st.Items["potion"] != tc.items || a.MP != tc.mp {
				t.Fatal("判断だけで消費した")
			}
		})
	}
}
