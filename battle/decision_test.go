package battle_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"reflect"
	"testing"

	"github.com/nao1215/gozen/battle"
	data "github.com/nao1215/gozen/internal/layer2/gamedata"
	"github.com/nao1215/gozen/internal/testgame"
	"github.com/nao1215/gozen/rpg"
)

func decisionExpr(s string) data.Expr { return data.Expr{Expr: testgame.Expr(s)} }

func TestRuleApplicability(t *testing.T) {
	for _, tc := range []struct {
		name   string
		rule   data.Rule
		enemy  bool
		setup  func(*battle.Battle, *battle.Unit)
		reason battle.RuleReason
	}{
		{name: "成立", rule: data.Rule{Use: "fire"}, reason: battle.RuleApplicable},
		{name: "条件不成立", rule: data.Rule{Use: "fire", When: decisionExpr("false")}, reason: battle.RuleConditionFalse},
		{name: "条件の評価エラー", rule: data.Rule{Use: "fire", When: decisionExpr("missing()")}, reason: battle.RuleConditionError},
		{name: "抽選不成立", rule: data.Rule{Use: "fire", Chance: decisionExpr("0")}, reason: battle.RuleChanceFailed},
		{name: "確率の評価エラー", rule: data.Rule{Use: "fire", Chance: decisionExpr("missing()")}, reason: battle.RuleChanceError},
		{name: "MP不足", rule: data.Rule{Use: "fire"}, setup: func(_ *battle.Battle, u *battle.Unit) { u.MP = 0 }, reason: battle.RuleMPShortage},
		{name: "未習得", rule: data.Rule{Use: "heal"}, reason: battle.RuleSkillUnlearned},
		{name: "品不足", rule: data.Rule{Item: "potion"}, setup: func(b *battle.Battle, _ *battle.Unit) { b.St.Items["potion"] = 0 }, reason: battle.RuleItemShortage},
		{name: "敵は品を使わない", rule: data.Rule{Item: "potion"}, enemy: true, reason: battle.RuleEnemyItem},
		{name: "対象不在", rule: data.Rule{Use: "fire", Target: data.Target{Where: decisionExpr("false")}}, reason: battle.RuleNoTarget},
		{name: "対象の式が失敗", rule: data.Rule{Use: "fire", Target: data.Target{Where: decisionExpr("missing()")}}, reason: battle.RuleNoTarget},
		{name: "使用済み", rule: data.Rule{Use: "fire", Once: true}, setup: func(b *battle.Battle, u *battle.Unit) { b.Decide(u) }, reason: battle.RuleOnceUsed},
		{name: "敵の使用済み", rule: data.Rule{Use: "slap", Once: true}, enemy: true, setup: func(b *battle.Battle, u *battle.Unit) { b.Decide(u) }, reason: battle.RuleOnceUsed},
		{name: "削除された技", rule: data.Rule{Use: "gone"}, reason: battle.RuleInvalidAction},
		{name: "削除された品", rule: data.Rule{Item: "gone"}, reason: battle.RuleInvalidAction},
		{name: "使えない品", rule: data.Rule{Item: "stick"}, reason: battle.RuleInvalidAction},
		{name: "二重の行動", rule: data.Rule{Use: "fire", Item: "potion"}, reason: battle.RuleInvalidAction},
		{name: "MP判定が習得より先", rule: data.Rule{Use: "heal"}, setup: func(_ *battle.Battle, u *battle.Unit) { u.MP = 0 }, reason: battle.RuleMPShortage},
		{name: "条件が参照解決より先", rule: data.Rule{Use: "gone", When: decisionExpr("false")}, reason: battle.RuleConditionFalse},
	} {
		t.Run(tc.name, func(t *testing.T) {
			b, _ := setup(t, "one")
			u := b.Party[0]
			if tc.enemy {
				u = b.Foes[0]
				u.Def.AI = data.AIRef{Rules: []data.Rule{tc.rule}}
			} else {
				u.Actor.Def().AI = data.AIRef{Rules: []data.Rule{tc.rule}}
			}
			if tc.setup != nil {
				tc.setup(b, u)
			}
			cands, results := b.EvaluateRules(u)
			if len(results) != 1 || results[0].Reason != tc.reason {
				t.Fatalf("評価結果 %v、期待 %s", results, tc.reason)
			}
			if (len(cands) == 1) != (tc.reason == battle.RuleApplicable) {
				t.Fatalf("候補と理由が不一致 %v / %v", cands, results)
			}
			if (tc.reason == battle.RuleConditionError || tc.reason == battle.RuleChanceError) && results[0].Detail == "" {
				t.Fatal("式の失敗の詳細がない")
			}
			var recorded *battle.Decision
			b.OnDecision = func(d battle.Decision) { recorded = &d }
			b.Decide(u)
			if recorded == nil || len(recorded.Rules) != 1 || recorded.Rules[0].Reason != tc.reason {
				t.Fatalf("実際の判断に理由が残らない %v", recorded)
			}
		})
	}
	t.Run("対象の無視と発動条件を区別", func(t *testing.T) {
		for _, scope := range []string{"all_enemies", "all_allies", "all_dead_allies", "everyone", "none", "random_enemy"} {
			t.Run(scope, func(t *testing.T) {
				b, _ := setup(t, "two")
				b.DB.Skills["fire"].Scope = scope
				b.DB.Actors["scout"].AI = data.AIRef{Rules: []data.Rule{{Use: "fire", Target: data.Target{Where: decisionExpr("false"), Pick: "min", By: decisionExpr("rand()")}}}}
				_, results := b.EvaluateRules(b.Party[0])
				want := battle.RuleApplicable
				if scope == "random_enemy" || scope == "all_dead_allies" {
					want = battle.RuleNoTarget
				}
				if results[0].Reason != want || results[0].Target != nil {
					t.Fatalf("%s の対象評価 %v", scope, results)
				}
			})
		}
	})
	t.Run("候補なしは通常攻撃", func(t *testing.T) {
		b, _ := setup(t, "one")
		b.Foes[0].HP = 0
		var records []battle.Decision
		b.OnDecision = func(d battle.Decision) { records = append(records, d) }
		got := b.Decide(b.Party[0])
		if got.Skill != b.DB.Game.Battle.Attack || got.Target != nil || records[0].Selected != -1 {
			t.Fatalf("通常攻撃への復帰が変わった %v / %v", got, records)
		}
		// 相手がいなくても対象を探し続けず、既存の勝敗判定へ進む。
		run(b)
		if b.Result != battle.Won {
			t.Fatalf("相手がいない戦闘が終わらない %v", b.Result)
		}
	})
}

// decisionScenario は式・対象選択にも乱数を使い、防御・高速技・品・once を混在させる。
func decisionScenario(t *testing.T, mode string, seed uint64) (*battle.Battle, *rpg.State) {
	t.Helper()
	db := testgame.Load(t, map[string]string{"data/decision.yaml": `
skills:
  wall: {name: Wall, scope: all_allies, speed: "1000", effects: [{heal: "2"}]}
  rain: {name: Rain, scope: random_enemy, hits: 2, effects: [{damage: "4"}]}
`})
	db.Game.Battle.AI = mode
	db.Game.Battle.Speed = decisionExpr("a.agi + rand(8)")
	db.Actors["scout"].AI = data.AIRef{Rules: []data.Rule{
		{Use: "guard", When: decisionExpr("turn == 1"), Once: true, Priority: 20},
		{Use: "wall", When: decisionExpr("turn == 2"), Priority: 20},
		{Item: "potion", When: decisionExpr("self.hp_rate < 0.8"), Chance: decisionExpr("0.65"), Priority: 10, Target: data.Target{Pick: "first"}},
		{Use: "fire", When: decisionExpr("rand() < 0.8"), Chance: decisionExpr("0.7"), Weight: 3, Target: data.Target{Where: decisionExpr("rand() < 0.8"), Pick: "min", By: decisionExpr("t.hp + rand(5)")}},
		{Use: "rain", Weight: 2, Target: data.Target{Where: decisionExpr("t.alive")}},
		{Use: "attack", Weight: 1},
	}}
	db.Actors["guide"].AI = data.AIRef{Rules: []data.Rule{
		{Use: "fire", Once: true, Chance: decisionExpr("0.8")},
		{Use: "attack", Target: data.Target{Preset: "random"}},
	}}
	db.Enemies["mote"].Stats["mhp"] = 250
	db.Enemies["mote"].Actions = 2
	db.Enemies["mote"].AI = data.AIRef{Rules: []data.Rule{
		{Use: "venom", Once: true, Chance: decisionExpr("0.6"), Priority: 5},
		{Use: "mend", When: decisionExpr("self.hp_rate < 0.7"), Weight: 2, Target: data.Target{Preset: "self"}},
		{Use: "slap", Weight: 3},
	}}
	st := testgame.State(db, seed)
	st.Join("guide")
	st.Actors["scout"].Learnt = []string{"wall", "rain"}
	return battle.New(db, st, "two"), st
}

// decisionRun はポインタを残さず、各ステップ・ターン末の状態・最後の乱数を比較できる形にする。
func decisionRun(t *testing.T, b *battle.Battle) []byte {
	t.Helper()
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	ref := func(u *battle.Unit) string {
		if u == nil {
			return ""
		}
		return fmt.Sprintf("%t/%d/%s", u.Enemy, u.Index, u.Name)
	}
	write := func(v any) {
		if err := enc.Encode(v); err != nil {
			t.Fatal(err)
		}
	}
	for b.Result == battle.Ongoing && b.Turn <= 12 {
		for _, s := range run(b) {
			write([]any{s.Kind, s.Text, ref(s.User), ref(s.Target), s.Amount, s.Stat, s.Crit, s.Weak, s.Resisted, s.Element, s.State, s.Skill, s.HP, s.MP})
		}
		for _, side := range [][]*battle.Unit{b.Party, b.Foes} {
			for _, u := range side {
				write([]any{ref(u), u.HP, u.MP, u.States, u.Guard, u.Fled})
			}
		}
		write([]any{b.Turn, b.Result, b.St.Items})
	}
	if b.Result != battle.Ongoing {
		write(b.End())
	}
	saved, err := b.St.Save()
	if err != nil {
		t.Fatal(err)
	}
	write(json.RawMessage(saved))
	write(b.Rand.Uint64())
	return buf.Bytes()
}

func TestDecisionTraceDoesNotChangeBattle(t *testing.T) {
	for _, mode := range []string{"gambit", ""} {
		for seed := uint64(1); seed <= 24; seed++ {
			t.Run(fmt.Sprintf("%s/%d", mode, seed), func(t *testing.T) {
				plain, _ := decisionScenario(t, mode, seed)
				want := decisionRun(t, plain)
				traced, _ := decisionScenario(t, mode, seed)
				var records []battle.Decision
				traced.OnDecision = func(d battle.Decision) { records = append(records, d) }
				got := decisionRun(t, traced)
				if !bytes.Equal(got, want) {
					t.Fatal("記録の有無で行動・状態・次の乱数が変わった")
				}
				seen := map[battle.DecisionTiming]bool{}
				for _, d := range records {
					seen[d.Timing] = true
					if d.Timing == battle.DecisionTurnStart && d.Committed != (d.Skill == "guard" || d.Skill == "wall") {
						t.Fatalf("先決めの採否が違う %v", d)
					}
					if d.User.Enemy && d.Timing != battle.DecisionAction {
						t.Fatalf("敵を先決めした %v", d)
					}
					if d.Turn == 1 && !d.User.Enemy && d.User.ID == "scout" && (!d.Committed || d.Skill != "guard") {
						t.Fatalf("防御の先決めが変わった %v", d)
					}
				}
				if !seen[battle.DecisionTurnStart] || !seen[battle.DecisionAction] {
					t.Fatalf("評価時点の記録が足りない %v", seen)
				}
				// 記録は DB や Unit の変更を受けない。
				before, err := json.Marshal(records)
				if err != nil {
					t.Fatal(err)
				}
				traced.Party[0].Name = "changed"
				traced.DB.Actors["scout"].AI.Rules[0].Use = "changed"
				after, err := json.Marshal(records)
				if err != nil {
					t.Fatal(err)
				}
				if !bytes.Equal(before, after) {
					t.Fatal("記録が戦況の参照を保持している")
				}
			})
		}
	}
}

func TestDecisionRecordCannotMutateBattle(t *testing.T) {
	plain, _ := decisionScenario(t, "gambit", 7)
	want := decisionRun(t, plain)
	traced, _ := decisionScenario(t, "gambit", 7)
	traced.OnDecision = func(d battle.Decision) {
		d.User.ID = "changed"
		if d.Target != nil {
			d.Target.ID = "changed"
		}
		for i := range d.Rules {
			d.Rules[i].Skill = "changed"
			if d.Rules[i].Target != nil {
				d.Rules[i].Target.Index = 99
			}
		}
	}
	if got := decisionRun(t, traced); !bytes.Equal(want, got) {
		t.Fatal("控えの編集が戦闘に届いた")
	}
}

func TestRuleEvaluationDoesNotEvaluateIgnoredExpressions(t *testing.T) {
	for _, scope := range []string{"all_enemies", "all_allies", "all_dead_allies", "everyone", "none", "random_enemy"} {
		t.Run(scope, func(t *testing.T) {
			a, _ := setup(t, "two")
			b, _ := setup(t, "two")
			for _, test := range []*battle.Battle{a, b} {
				test.DB.Skills["fire"].Scope = scope
				test.DB.Actors["scout"].AI = data.AIRef{Rules: []data.Rule{{Use: "fire"}}}
			}
			// 全体では where も評価せず、random_enemy では where だけを評価する。
			a.DB.Actors["scout"].AI.Rules[0].Target = data.Target{Where: decisionExpr("rand() < 1"), Pick: "min", By: decisionExpr("rand()")}
			if scope == "random_enemy" {
				b.DB.Actors["scout"].AI.Rules[0].Target.Where = decisionExpr("rand() < 1")
			}
			a.EvaluateRules(a.Party[0])
			b.Candidates(b.Party[0])
			if a.Rand.Uint64() != b.Rand.Uint64() {
				t.Fatal("無視する対象の式を評価した")
			}
		})
	}
	t.Run("条件不成立の後は評価しない", func(t *testing.T) {
		a, _ := setup(t, "one")
		b, _ := setup(t, "one")
		a.DB.Actors["scout"].AI = data.AIRef{Rules: []data.Rule{{Use: "fire", When: decisionExpr("false"), Chance: decisionExpr("rand()"), Target: data.Target{Where: decisionExpr("rand() < 0.5")}}}}
		a.EvaluateRules(a.Party[0])
		if a.Rand.Uint64() != b.Rand.Uint64() {
			t.Fatal("条件不成立後の式を評価した")
		}
	})
}

func TestDecisionSelectionAndOnce(t *testing.T) {
	for _, mode := range []string{"gambit", ""} {
		t.Run(mode, func(t *testing.T) {
			b, _ := setup(t, "two")
			b.DB.Game.Battle.AI = mode
			b.DB.Actors["scout"].AI = data.AIRef{Rules: []data.Rule{
				{Use: "fire", Once: true, Priority: 2}, {Use: "attack", Priority: 1}, {Use: "attack", Priority: 2},
			}}
			var records []battle.Decision
			b.OnDecision = func(d battle.Decision) { records = append(records, d) }
			run(b)
			first := records[0]
			if first.Committed || first.Timing != battle.DecisionTurnStart {
				t.Fatalf("通常技を先決めした %v", first)
			}
			for _, d := range records {
				if d.User.Enemy {
					continue
				}
				if d.Rules[0].Reason == battle.RuleOnceUsed {
					t.Fatal("先決めの候補で once を消費した")
				}
				if d.Rules[d.Selected].Selection != "selected" {
					t.Fatalf("選択結果がない %v", d)
				}
				if mode == "gambit" && (d.Selected != 0 || d.Rules[2].Selection != "later_rule") {
					t.Fatalf("先頭を使わない %v", d)
				}
				if mode == "" && d.Rules[1].Selection != "lower_priority" {
					t.Fatalf("優先度の理由がない %v", d)
				}
			}
			if mode == "gambit" {
				_, results := b.EvaluateRules(b.Party[0])
				if results[0].Reason != battle.RuleOnceUsed {
					t.Fatal("実行時の once が残らない")
				}
			}
		})
	}
}

func TestCandidateEvaluationUsesTheSameRandomSequence(t *testing.T) {
	a, _ := decisionScenario(t, "gambit", 9)
	b, _ := decisionScenario(t, "gambit", 9)
	a.Turn, b.Turn = 3, 3
	x := a.Candidates(a.Party[0])
	y, reasons := b.EvaluateRules(b.Party[0])
	var xi, yi []int
	for _, c := range x {
		xi = append(xi, c.Index)
	}
	for _, c := range y {
		yi = append(yi, c.Index)
	}
	if len(reasons) != len(b.Rules(b.Party[0])) || !reflect.DeepEqual(xi, yi) || a.Rand.Uint64() != b.Rand.Uint64() {
		t.Fatal("共通の候補評価で結果か乱数が変わった")
	}
}
