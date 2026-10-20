package battle_test

import (
	"strings"
	"testing"

	"github.com/nao1215/gozen/internal/battle"
	"github.com/nao1215/gozen/internal/layer1/optional"
	data "github.com/nao1215/gozen/internal/layer2/gamedata"
	"github.com/nao1215/gozen/internal/rpg"
	"github.com/nao1215/gozen/internal/testgame"
)

func TestTacticConditionInBattleLog(t *testing.T) {
	db := testgame.Load(t, map[string]string{"data/tactics.yaml": testgame.Tactics})
	st := testgame.State(db, 4)
	percent := 80
	off := tacticRow("guard", "self")
	off.Enabled = false
	heal := tacticRow("potion", "ally")
	heal.Condition, heal.Percent = "low_hp", &percent
	setActorTactics(t, st.Actors["scout"], off, heal)
	b := battle.New(db, st, "one")
	b.Party[0].HP = b.Party[0].Stat("mhp") / 2
	steps := run(b)
	found := false
	for _, step := range steps {
		if step.Kind != battle.Act || step.User != b.Party[0] {
			continue
		}
		for _, part := range []string{"作戦2", "Health below threshold（80%）", b.Party[0].Name, db.Items["potion"].Name} {
			if !strings.Contains(step.Text, part) {
				t.Errorf("戦闘ログに %q がない %q", part, step.Text)
			}
		}
		found = true
	}
	if !found {
		t.Fatal("自動行動の記録がない")
	}
	t.Run("手動入力", func(t *testing.T) {
		b := battle.New(db, st, "one")
		for _, step := range run(b, battle.Action{User: b.Party[0], Skill: "attack", Target: b.Foes[0]}) {
			if step.Kind == battle.Act && step.User == b.Party[0] && strings.Contains(step.Text, "作戦") {
				t.Fatal("手動入力に作戦の条件を付けた")
			}
		}
	})
	t.Run("おまかせの行番号", func(t *testing.T) {
		db.Game.Battle.AutoTactics = map[string][]data.TacticRow{"support": {tacticRow("heal", "ally"), tacticRow("fire", "enemy")}}
		db.Actors["scout"].AI = data.AIRef{Name: "support"}
		st := testgame.State(db, 8)
		b := battle.New(db, st, "one")
		for _, step := range run(b) {
			if step.Kind == battle.Act && step.User == b.Party[0] && !strings.Contains(step.Text, "作戦1") {
				t.Fatalf("未習得行を数え、編集画面と違う番号になった %s", step.Text)
			}
		}
	})
	t.Run("全体技", func(t *testing.T) {
		st := testgame.State(db, 2)
		st.Join("guide")
		st.Actors["scout"].Learnt = []string{"group_heal"}
		setActorTactics(t, st.Actors["scout"], tacticRow("group_heal", "allies"))
		b := battle.New(db, st, "one")
		for _, step := range run(b) {
			if step.Kind == battle.Act && step.User == b.Party[0] {
				for _, u := range b.Party {
					if !strings.Contains(step.Text, u.Name) {
						t.Fatalf("全体技の相手がない %s", step.Text)
					}
				}
			}
		}
	})
}

func TestTacticLogUsesActualTargets(t *testing.T) {
	t.Run("先決めした対象が倒れた後の選び直し", func(t *testing.T) {
		b, _ := setup(t, "two")
		b.Party[0].Actor.Def().AI = data.AIRef{Rules: []data.Rule{{Use: "fire", Target: data.Target{Preset: "first"}}}}
		b.DB.Skills["fire"].Speed = decisionExpr("1000")
		b.StartTurn(nil)
		b.Foes[0].HP = 0
		steps, _ := b.Next()
		for _, s := range steps {
			if s.Kind == battle.Act && s.User == b.Party[0] {
				if !strings.Contains(s.Text, "→ "+b.Foes[1].Name+"・") {
					t.Fatalf("古い対象を記録した %s", s.Text)
				}
				return
			}
		}
		t.Fatal("先決めした技を実行しない")
	})
	t.Run("かばった相手", func(t *testing.T) {
		b, _ := setup(t, "two")
		b.Party[0].Actor.Def().AI = data.AIRef{Rules: []data.Rule{{Use: "attack", Target: data.Target{Preset: "first"}}}}
		b.DB.States["cover_test"] = &data.State{Name: "盾", Cover: decisionExpr("true")}
		b.DB.Order["states"] = append(b.DB.Order["states"], "cover_test")
		b.Foes[1].States["cover_test"] = rpg.StateForever
		for _, s := range run(b) {
			if s.Kind == battle.Act && s.User == b.Party[0] {
				if !strings.Contains(s.Text, "→ "+b.Foes[1].Name+"・") {
					t.Fatalf("かばう前の相手を記録した %s", s.Text)
				}
				return
			}
		}
		t.Fatal("自動攻撃の記録がない")
	})
}

func TestBattleAdviceCauses(t *testing.T) {
	attack := data.Rule{Use: "attack"}
	fire := data.Rule{Use: "fire"}
	potion := data.Rule{Item: "potion", Target: data.Target{Preset: "self"}}
	for _, tc := range []struct {
		name  string
		rules []data.Rule
		mp    int
		items int
		hurt  bool
		want  string
		row   float64
	}{
		{"魔力不足", []data.Rule{fire, attack}, 0, 1, false, "mp_shortage", 1},
		{"条件が不成立なら魔力不足ではない", []data.Rule{{Use: "fire", When: decisionExpr("false")}, attack}, 1, 1, false, "", 0},
		{"抽選が不成立なら魔力不足ではない", []data.Rule{{Use: "fire", Chance: decisionExpr("0")}, attack}, 1, 1, false, "", 0},
		{"未習得の魔力不足を除外", []data.Rule{{Use: "heal"}, attack}, 1, 1, false, "", 0},
		{"採用行より後の魔力不足を除外", []data.Rule{attack, fire}, 1, 1, false, "", 0},
		{"終了時に魔力切れ", []data.Rule{attack, fire}, 0, 1, false, "mp_empty", 2},
		{"品不足", []data.Rule{potion, attack}, 10, 0, false, "item_shortage", 1},
		{"回復の順番が遅い", []data.Rule{attack, potion}, 10, 1, true, "healing_delayed", 2},
		{"無傷の回復は遅れではない", []data.Rule{attack, potion}, 10, 1, false, "", 0},
		{"回復を採用済み", []data.Rule{potion, potion}, 10, 2, true, "", 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			b, st := setup(t, "one")
			b.DB.Game.Battle.AI = "gambit"
			b.Party[0].Actor.Def().AI = data.AIRef{Rules: tc.rules}
			b.Foes[0].Def.AI = data.AIRef{Rules: []data.Rule{{Use: "guard"}}}
			b.Foes[0].HP = 10000
			b.Party[0].MP = tc.mp
			st.Items["potion"] = tc.items
			if tc.hurt {
				b.Party[0].HP = b.Party[0].Stat("mhp") / 2
			}
			run(b)
			b.Result = battle.Fled
			b.End()
			got := optional.Value[string](st.Vars["battle_advice_reason"])
			if got != tc.want {
				t.Fatalf("助言の原因 %q、期待 %q", got, tc.want)
			}
			if got != "" && (st.Vars["battle_advice_row"] != tc.row || st.Vars["battle_advice_actor"] != b.Party[0].Name) {
				t.Fatalf("作戦行や仲間が違う %v", st.Vars)
			}
		})
	}
	t.Run("候補を調べただけでは不足を記録しない", func(t *testing.T) {
		b, st := setup(t, "one")
		b.Party[0].Actor.Def().AI = data.AIRef{Rules: []data.Rule{fire}}
		b.Party[0].MP = 0
		b.Decide(b.Party[0])
		b.Party[0].MP = 1
		b.Result = battle.Fled
		b.End()
		if st.Vars["battle_advice_reason"] != nil {
			t.Fatal("実行していない候補を問題にした")
		}
	})
	t.Run("敗北と倒れた仲間", func(t *testing.T) {
		for _, result := range []battle.Result{battle.Lost, battle.Won, battle.Fled} {
			b, st := setup(t, "one")
			b.Party[0].HP = 0
			b.Result = result
			b.End()
			want := "fallen"
			if result == battle.Lost {
				want = "lost"
			}
			if st.Vars["battle_advice_reason"] != want {
				t.Fatalf("結果 %v の原因 %v", result, st.Vars)
			}
		}
	})
}

func TestBattleAdviceRemembersRevivedAlly(t *testing.T) {
	db := testgame.Load(t, nil)
	st := testgame.State(db, 9)
	st.Join("guide")
	db.Enemies["mote"].AI = data.AIRef{Rules: []data.Rule{{Use: "slap", Target: data.Target{Preset: "first"}}}}
	db.Skills["slap"].Effects = []data.Effect{{Damage: decisionExpr("9999")}}
	b := battle.New(db, st, "one")
	scout, guide := b.Party[0], b.Party[1]
	steps := run(b, battle.Action{User: scout, Skill: "guard"}, battle.Action{User: guide, Skill: "guard"})
	fell := false
	for _, s := range steps {
		fell = fell || s.Kind == battle.Fall && s.Target == scout
	}
	if !fell {
		t.Fatal("倒れる戦闘を再現できない")
	}
	// After being raised within a battle, the fact of falling in that battle remains.
	scout.HP = 1
	b.Result = battle.Won
	b.End()
	if st.Vars["battle_advice_reason"] != "fallen" || st.Vars["battle_advice_actor"] != scout.Name {
		t.Fatalf("起こした仲間の記録が消えた %v", st.Vars)
	}
}

func TestBattleAdviceWithStrictState(t *testing.T) {
	db := testgame.Load(t, nil)
	db.Game.State.Strict = true
	st := testgame.State(db, 9)
	b := battle.New(db, st, "one")
	b.Party[0].HP = 0
	b.Result = battle.Lost
	b.End()
	encoded, err := st.Save()
	if err != nil {
		t.Fatal(err)
	}
	loaded, err := rpg.Parse(db, encoded, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := loaded.SetPath("var.battle_advice_reason", ""); err != nil {
		t.Fatal(err)
	}
}

func TestBattleAdviceSurvivesSaveAndCleanBattle(t *testing.T) {
	b, st := setup(t, "one")
	b.Party[0].HP = 0
	b.Result = battle.Lost
	b.End()
	encoded, err := st.Save()
	if err != nil {
		t.Fatal(err)
	}
	loaded, err := rpg.Parse(b.DB, encoded, nil)
	if err != nil {
		t.Fatal(err)
	}
	loaded.Actors["scout"].Recover()
	clean := battle.New(b.DB, loaded, "one")
	clean.Result = battle.Won
	clean.End()
	if loaded.Vars["battle_advice_reason"] != "lost" {
		t.Fatal("保存再開や無事な戦闘で未読の助言が消えた")
	}
}

func TestBattleAdviceObserverIsReadOnly(t *testing.T) {
	for _, observe := range []bool{false, true} {
		b, st := setup(t, "one")
		b.DB.Game.Battle.AI = "gambit"
		b.Party[0].HP = b.Party[0].Stat("mhp") / 2
		b.Party[0].Actor.Def().AI = data.AIRef{Rules: []data.Rule{{Use: "attack"}, {Item: "potion", Target: data.Target{Preset: "self"}}}}
		b.Foes[0].HP = 10000
		b.Foes[0].Def.AI = data.AIRef{Rules: []data.Rule{{Use: "guard"}}}
		if observe {
			b.OnDecision = func(d battle.Decision) {
				for i := range d.Rules {
					d.Rules[i].Reason = battle.RuleConditionFalse
					d.Rules[i].Index = 777
					if d.Rules[i].Target != nil {
						d.Rules[i].Target.Index = 999
					}
				}
			}
		}
		run(b)
		b.Result = battle.Fled
		b.End()
		if st.Vars["battle_advice_reason"] != "healing_delayed" || st.Vars["battle_advice_row"] != float64(2) {
			t.Fatalf("観測%tで助言が変わった %v", observe, st.Vars)
		}
	}
}
