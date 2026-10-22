package battle_test

import (
	"strings"
	"testing"

	"github.com/nao1215/gozen/internal/battle"
	"github.com/nao1215/gozen/internal/expr"
	data "github.com/nao1215/gozen/internal/layer2/gamedata"
	"github.com/nao1215/gozen/internal/testgame"
)

const formData = `
skills:
  change: {name: Change, scope: self, effects: [{transform: second, transform_hp: ratio}]}
  fuse: {name: Fuse, scope: self, effects: [{merge: {into: second, from: [first], count: 2, hp: sum}}]}
enemies:
  first:
    name: First
    image: x.png
    images: [{below: 0.5, file: hurt.png}, {below: 0.2, file: critical.png}]
    stats: {mhp: 100, mmp: 0, atk: 1, def: 1, mat: 1, mdf: 1, agi: 99, luk: 1}
    exp: 7
    ai: [{use: change, when: "self.hp_rate < 0.5", priority: 5}, {use: attack}]
  second:
    name: Second
    image: x.png
    stats: {mhp: 200, mmp: 0, atk: 1, def: 1, mat: 1, mdf: 1, agi: 99, luk: 1}
    exp: 11
    ai: [{use: attack}]
troops:
  forms: {members: [first]}
  pair: {members: [first, first]}
`

func formBattle(t *testing.T, troop string) *battle.Battle {
	t.Helper()
	db := testgame.Load(t, map[string]string{"game.yaml": strings.Replace(testgame.Game, "title:", "schema_version: 7\nid: forms\ntitle:", 1), "data/forms.yaml": formData, "assets/hurt.png": "x", "assets/critical.png": "x"})
	return battle.New(db, testgame.State(db, 1), troop)
}

func TestTransformKeepsSlotAndPaysFinalFormOnce(t *testing.T) {
	b := formBattle(t, "forms")
	u := b.Foes[0]
	u.HP = 40
	steps := run(b, battle.Action{User: b.Party[0], Skill: "guard"})
	if b.Foes[0] != u || u.EID != "second" || u.Index != 0 || u.HP != 80 {
		t.Fatalf("form in slot: %+v", u)
	}
	if b.St.Vars["bestiary.second.seen"] != true {
		t.Fatal("new form was not recorded as seen")
	}
	found := false
	for _, st := range steps {
		if st.Kind == battle.Transform && st.Before == "hurt.png" && st.Target == u {
			found = true
		}
	}
	if !found {
		t.Fatalf("missing staged transform: %v", steps)
	}
	u.HP = 1
	run(b, battle.Action{User: b.Party[0], Skill: "big", Target: u})
	b.End()
	if b.Gains.Exp != 11 {
		t.Fatalf("rewards = %+v", b.Gains)
	}
}

func TestMergeAndAvailability(t *testing.T) {
	b := formBattle(t, "pair")
	b.DB.Enemies["first"].AI.Rules[0].Use = "fuse"
	b.DB.Enemies["first"].AI.Rules[0].When = data.Expr{}
	if got := b.Decide(b.Foes[0]).Skill; got != "fuse" {
		t.Fatalf("available merge: %s", got)
	}
	steps := run(b, battle.Action{User: b.Party[0], Skill: "guard"})
	if len(b.Foes) != 1 || b.Foes[0].EID != "second" || b.Foes[0].HP != 200 {
		t.Fatalf("merged foes: %+v", b.Foes)
	}
	found := false
	for _, st := range steps {
		if st.Kind == battle.Merge && len(st.Members) == 2 {
			found = true
		}
	}
	if !found {
		t.Fatalf("missing merge step: %v", steps)
	}
	one := formBattle(t, "forms")
	one.DB.Enemies["first"].AI.Rules[0].Use = "fuse"
	one.DB.Enemies["first"].AI.Rules[0].When = data.Expr{}
	if got := one.Decide(one.Foes[0]).Skill; got == "fuse" {
		t.Fatal("AI chose an impossible merge")
	}
}

func TestHPPicturesUseTheLowestMatchingThreshold(t *testing.T) {
	b := formBattle(t, "forms")
	u := b.Foes[0]
	for _, tc := range []struct {
		hp   int
		file string
	}{{100, "x.png"}, {50, "x.png"}, {49, "hurt.png"}, {20, "hurt.png"}, {19, "critical.png"}} {
		u.HP = tc.hp
		if got := u.Picture(); got != tc.file {
			t.Fatalf("HP %d: %q", tc.hp, got)
		}
	}
}

func TestTransformFullAndExpressionHP(t *testing.T) {
	for _, tc := range []struct {
		hp   string
		want int
	}{{"full", 200}, {"b.hp + 20", 60}} {
		t.Run(tc.hp, func(t *testing.T) {
			b := formBattle(t, "forms")
			u := b.Foes[0]
			u.HP = 40
			p, err := expr.Parse(tc.hp)
			if err != nil {
				t.Fatal(err)
			}
			b.DB.Skills["change"].Effects[0].TransformHP = data.Expr{Expr: p}
			b.DB.Skills["change"].Effects[0].TransformClear = true
			u.States["poison"] = 3
			run(b, battle.Action{User: b.Party[0], Skill: "guard"})
			if u.EID != "second" || u.HP != tc.want || len(u.States) != 0 {
				t.Fatalf("form=%s HP=%d states=%v", u.EID, u.HP, u.States)
			}
		})
	}
}

func TestMergeHPModes(t *testing.T) {
	for _, tc := range []struct {
		mode string
		want int
	}{{"sum", 60}, {"ratio", 60}, {"full", 200}} {
		t.Run(tc.mode, func(t *testing.T) {
			b := formBattle(t, "pair")
			b.DB.Enemies["first"].AI.Rules[0].Use = "fuse"
			b.DB.Enemies["first"].AI.Rules[0].When = data.Expr{}
			b.DB.Skills["fuse"].Effects[0].Merge.HP = tc.mode
			b.Foes[0].HP, b.Foes[1].HP = 40, 20
			run(b, battle.Action{User: b.Party[0], Skill: "guard"})
			if len(b.Foes) != 1 || b.Foes[0].HP != tc.want {
				t.Fatalf("mode=%s foes=%+v", tc.mode, b.Foes)
			}
		})
	}
}

func TestHPTriggerChangesFormOnCrossingAndFatalHit(t *testing.T) {
	for _, tc := range []struct {
		hp    int
		skill string
		want  int
	}{{60, "attack", 62}, {100, "big", 1}} {
		b := formBattle(t, "forms")
		u := b.Foes[0]
		u.Def.HPTriggers = []data.HPTrigger{{Below: 0.5, Transform: "second"}}
		u.HP = tc.hp
		steps := run(b, battle.Action{User: b.Party[0], Skill: tc.skill, Target: u})
		if u.EID != "second" || u.HP != tc.want || b.Result == battle.Won {
			t.Fatalf("HP %d skill %s: form=%s hp=%d result=%v", tc.hp, tc.skill, u.EID, u.HP, b.Result)
		}
		found := false
		for _, st := range steps {
			if st.Kind == battle.Transform {
				found = true
			}
		}
		if !found {
			t.Fatalf("missing trigger step: %v", steps)
		}
	}
}
