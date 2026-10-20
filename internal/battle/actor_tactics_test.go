package battle_test

import (
	"reflect"
	"strings"
	"testing"

	"github.com/nao1215/gozen/internal/battle"
	"github.com/nao1215/gozen/internal/headless"
	data "github.com/nao1215/gozen/internal/layer2/gamedata"
	"github.com/nao1215/gozen/internal/rpg"
	"github.com/nao1215/gozen/internal/testgame"
)

func tacticRow(action, target string) data.TacticRow {
	return data.TacticRow{Enabled: true, Condition: "any", Target: target, Action: action}
}

func setActorTactics(t *testing.T, a *rpg.Actor, rows ...data.TacticRow) {
	t.Helper()
	if err := a.SetTactics(rpg.ActorTactics{Version: rpg.TacticsVersion, Mode: rpg.TacticsManual, Rows: rows}); err != nil {
		t.Fatal(err)
	}
}

func TestTacticsIsolation(t *testing.T) {
	db := testgame.Load(t, map[string]string{"data/tactics.yaml": testgame.Tactics})
	db.Classes["ranger"].AI = data.AIRef{Rules: []data.Rule{{Use: "fire"}}}
	st := testgame.State(db, 19)
	st.Join("guide")
	setActorTactics(t, st.Actors["scout"], tacticRow("guard", "self"))
	other := testgame.State(db, 19)
	b := battle.New(db, st, "one")
	if b.Decide(b.Party[0]).Skill != "guard" || b.Decide(b.Party[1]).Skill != "fire" || b.Decide(b.Foes[0]).Skill != "slap" {
		t.Fatal("仲間別・職業・敵の作戦が混ざった")
	}
	c := battle.New(db, other, "one")
	if c.Decide(c.Party[0]).Skill != "fire" || len(db.Rules(db.Actors["scout"].AI)) != 0 || !reflect.DeepEqual(db.Rules(db.Classes["ranger"].AI), []data.Rule{{Use: "fire"}}) {
		t.Fatal("DB または別セーブへ波及した")
	}
	setActorTactics(t, st.Actors["guide"], tacticRow("attack", "enemy"))
	both := battle.New(db, st, "one")
	if both.Decide(both.Party[0]).Skill != "guard" || both.Decide(both.Party[1]).Skill != "attack" {
		t.Fatal("複数人の手動作戦が混ざった")
	}
	// Confirming, adding rows, or changing the catalog does not affect a manual tactic that was already imported.
	setActorTactics(t, st.Actors["scout"], tacticRow("attack", "enemy"))
	db.TacticActions["guard"].Use = "attack"
	if b.Decide(b.Party[0]).Skill != "guard" {
		t.Fatal("戦闘中に確定設定かカタログの変更が漏れた")
	}
	next := battle.New(db, st, "one")
	if next.Decide(next.Party[0]).Skill != "attack" {
		t.Fatal("次の戦闘に確定設定を取り込まない")
	}
	// Only the reset-to-auto operation restores the default rules.
	if err := st.Actors["scout"].SetTactics(rpg.ActorTactics{Version: rpg.TacticsVersion, Mode: rpg.TacticsAuto}); err != nil {
		t.Fatal(err)
	}
	back := battle.New(db, st, "one")
	if back.Decide(back.Party[0]).Skill != "fire" {
		t.Fatal("おまかせへ戻せない")
	}
}

func TestTacticsAvailability(t *testing.T) {
	db := testgame.Load(t, map[string]string{"data/tactics.yaml": testgame.Tactics})
	db.StoneSets["gift"] = &data.StoneSet{Name: "Gift", Stones: []string{"blade"}, Skill: "group_heal"}
	db.Order["stone_sets"] = append(db.Order["stone_sets"], "gift")
	st := testgame.State(db, 1)
	a := st.Actors["scout"]
	setActorTactics(t, a, tacticRow("group_heal", "allies"), tacticRow("fire", "enemy"))
	want := a.TacticsDraft()
	choose := func(wantSkill string) *battle.Battle {
		t.Helper()
		b := battle.New(db, st, "one")
		if got := b.Decide(b.Party[0]).Skill; got != wantSkill {
			t.Fatalf("選択 %s、必要 %s", got, wantSkill)
		}
		if !reflect.DeepEqual(a.Tactics, &want) {
			t.Fatal("利用できる技の変化で保存行を上書きした")
		}
		return b
	}
	before := choose("fire")
	a.Equip["weapon"] = "blade"
	choose("group_heal")
	if before.Decide(before.Party[0]).Skill != "fire" {
		t.Fatal("戦闘中に装備の技の追加で作戦を作り直した")
	}
	delete(a.Equip, "weapon")
	choose("fire")
	a.Learnt = []string{"group_heal"}
	choose("group_heal")
	a.Learnt = nil
	choose("fire")
	db.Classes["ranger"].Tree = []data.TreeNode{{ID: "care", Skill: "group_heal", Max: 1}}
	a.Tree["care"] = 1
	choose("group_heal")
	a.ResetTree()
	choose("fire")
}

func TestActorTacticsBattle(t *testing.T) {
	for _, tc := range []struct {
		name    string
		rows    []data.TacticRow
		want    string
		reasons []battle.RuleReason
	}{
		{"空", []data.TacticRow{}, "attack", nil},
		{"全行OFF", []data.TacticRow{{Condition: "any", Target: "self", Action: "guard"}}, "attack", []battle.RuleReason{battle.RuleDisabled}},
		{"未知IDの次へ", []data.TacticRow{tacticRow("gone", "enemy"), tacticRow("fire", "enemy")}, "fire", []battle.RuleReason{battle.RuleInvalidTactic, battle.RuleApplicable}},
		{"未習得の次へ", []data.TacticRow{tacticRow("group_heal", "allies"), tacticRow("fire", "enemy")}, "fire", []battle.RuleReason{battle.RuleInvalidTactic, battle.RuleApplicable}},
		{"所持数不足の次へ", []data.TacticRow{tacticRow("potion", "ally"), tacticRow("fire", "enemy")}, "fire", []battle.RuleReason{battle.RuleItemShortage, battle.RuleApplicable}},
		{"対象不在の次へ", []data.TacticRow{{Enabled: true, Condition: "poisoned", Target: "ally", Action: "potion"}, tacticRow("fire", "enemy")}, "fire", []battle.RuleReason{battle.RuleNoTarget, battle.RuleApplicable}},
		{"全部不正", []data.TacticRow{tacticRow("gone", "enemy")}, "attack", []battle.RuleReason{battle.RuleInvalidTactic}},
		{"優先度によらず先頭", []data.TacticRow{tacticRow("fire", "enemy"), {Enabled: true, Condition: "complex", Target: "complex", Action: "fire"}}, "fire", []battle.RuleReason{battle.RuleApplicable, battle.RuleApplicable}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			db := testgame.Load(t, map[string]string{"data/tactics.yaml": testgame.Tactics})
			// Even when the DB default is weighted lottery, manual order is used from the top.
			db.Classes["ranger"].AI = data.AIRef{Rules: []data.Rule{{Use: "guard"}}}
			st := testgame.State(db, 4)
			if strings.Contains(tc.name, "所持数") {
				st.Items["potion"] = 0
			}
			setActorTactics(t, st.Actors["scout"], tc.rows...)
			if strings.Contains(tc.name, "優先度") {
				db.TacticConditions["complex"].Priority = 999
				db.TacticConditions["complex"].Chance = data.Expr{}
			}
			b := battle.New(db, st, "one")
			if strings.Contains(tc.name, "優先度") {
				b.Foes[0].States["poison"] = 2
				st.Flags["ready"] = true
			}
			var d battle.Decision
			b.OnDecision = func(v battle.Decision) { d = v }
			if a := b.Decide(b.Party[0]); a.Skill != tc.want {
				t.Fatalf("選択 %s、必要 %s、理由 %+v", a.Skill, tc.want, d)
			}
			if d.Mode != "gambit" || len(d.Rules) != len(tc.rows) {
				t.Fatalf("行番号または評価モードが違う %+v", d)
			}
			for i, want := range tc.reasons {
				if d.Rules[i].Index != i || d.Rules[i].Reason != want {
					t.Fatalf("行 %d の理由 %+v、必要 %s", i, d.Rules[i], want)
				}
				if want == battle.RuleInvalidTactic && d.Rules[i].Detail == "" {
					t.Fatal("不正な行の理由がない")
				}
			}
			if !reflect.DeepEqual(st.Actors["scout"].Tactics.Rows, tc.rows) {
				t.Fatal("実行で無効行を消したか並べ替えた")
			}
		})
	}
	t.Run("MP不足と通常攻撃", func(t *testing.T) {
		db := testgame.Load(t, map[string]string{"data/tactics.yaml": testgame.Tactics})
		st := testgame.State(db, 1)
		setActorTactics(t, st.Actors["scout"], tacticRow("fire", "enemy"))
		b := battle.New(db, st, "one")
		b.Party[0].MP = 0
		c, reasons := b.EvaluateRules(b.Party[0])
		if len(c) != 0 || reasons[0].Reason != battle.RuleMPShortage || b.Decide(b.Party[0]).Skill != "attack" {
			t.Fatal("MP 不足で通常攻撃に戻らない")
		}
	})
}

func TestActorTacticsDecisionRecording(t *testing.T) {
	db := testgame.Load(t, map[string]string{"data/tactics.yaml": testgame.Tactics})
	makeBattle := func() *battle.Battle {
		st := testgame.State(db, 42)
		setActorTactics(t, st.Actors["scout"], tacticRow("gone", "enemy"), tacticRow("fire", "enemy"), tacticRow("attack", "enemy"))
		return battle.New(db, st, "boss")
	}
	quiet, traced := makeBattle(), makeBattle()
	var decisions []battle.Decision
	traced.OnDecision = func(d battle.Decision) { decisions = append(decisions, d) }
	headless.AutoBattle(db, quiet.St, quiet, nil, 3, nil)
	headless.AutoBattle(db, traced.St, traced, nil, 3, nil)
	if len(decisions) == 0 || quiet.Rand.Uint64() != traced.Rand.Uint64() || !reflect.DeepEqual(quiet.St.Actors, traced.St.Actors) || quiet.Result != traced.Result { //nolint:govet // deepequalerrors: the errors are inside the one *data.DB both sides point at
		t.Fatal("理由の記録で乱数・行動・結果が変わった")
	}
}

func TestTacticsOnceAndTurnTiming(t *testing.T) {
	db := testgame.Load(t, map[string]string{"data/tactics.yaml": testgame.Tactics})
	db.TacticConditions["once_guard"] = &data.TacticCondition{Kind: "template", Name: "一度だけ守る", When: data.Expr{Expr: testgame.Expr("true")}, Once: true}
	st := testgame.State(db, 1)
	setActorTactics(t, st.Actors["scout"], tacticRow("gone", "enemy"),
		data.TacticRow{Enabled: true, Condition: "once_guard", Target: "self", Action: "guard"}, tacticRow("fire", "enemy"))
	b := battle.New(db, st, "one")
	var decisions []battle.Decision
	b.OnDecision = func(d battle.Decision) { decisions = append(decisions, d) }
	headless.AutoBattle(db, st, b, nil, 2, nil)
	if len(decisions) == 0 || decisions[0].Timing != battle.DecisionTurnStart || !decisions[0].Committed || decisions[0].Selected != 1 || decisions[0].Skill != "guard" {
		t.Fatalf("防御を元の行番号で先決めしない %+v", decisions)
	}
	var next *battle.Decision
	for i := range decisions {
		d := &decisions[i]
		if d.User.ID == "scout" && d.Turn == 2 && d.Timing == battle.DecisionTurnStart {
			next = d
		}
	}
	if next == nil || next.Committed || next.Selected != 2 || next.Skill != "fire" || next.Rules[1].Reason != battle.RuleOnceUsed {
		t.Fatalf("once の消費または通常技の行動時判定が違う %+v", next)
	}
}
