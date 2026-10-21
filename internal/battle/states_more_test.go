package battle_test

import (
	"strings"
	"testing"

	"github.com/nao1215/gozen/internal/battle"
	"github.com/nao1215/gozen/internal/testgame"
)

const statesData = `
states:
  muddle: {name: Muddle, turns: 9, restrict: confuse, messages: {stay: "{target} is muddled."}}
  silent_muddle: {name: Hush, turns: 9, restrict: confuse}
  guardian: {name: Guardian, turns: 9, cover: "b.hp_rate < 0.5", messages: {cover: "{target} shields {user}!"}}
  guardian2: {name: Guardian2, turns: 9, cover: "b.hp_rate < 0.5"}
  riposte: {name: Riposte, turns: 9, counter: "1", messages: {counter: "{target} strikes back!"}}
  riposte2: {name: Riposte2, turns: 9, counter: "1"}
  doze: {name: Doze, turns: 9, restrict: skip, remove_on_damage: 1}
  broken_counter: {name: Broken, turns: 9, counter: "len(a.hp)"}
skills:
  jab: {name: Jab, scope: one_enemy, effects: [{damage: "10", variance: 0}]}
  weak_spot: {name: Weak Spot, scope: one_enemy, effects: [{damage: "30", variance: 0}]}
enemies:
  dummy:
    name: Dummy
    image: x.png
    stats: {mhp: 5000, mmp: 0, atk: 1, def: 1, mat: 1, mdf: 1, agi: 1, luk: 1}
    ai: [{use: guard}]
  hunter:
    name: Hunter
    image: x.png
    stats: {mhp: 5000, mmp: 0, atk: 1, def: 1, mat: 1, mdf: 1, agi: 99, luk: 1}
    ai:
      - {use: jab, target: weakest, say: "Hunter picks the weakest."}
  sniper:
    name: Sniper
    image: x.png
    stats: {mhp: 5000, mmp: 0, atk: 1, def: 1, mat: 1, mdf: 1, agi: 99, luk: 1}
    ai: [{use: jab, target: strongest}]
  striker:
    name: Striker
    image: x.png
    stats: {mhp: 5000, mmp: 0, atk: 40, def: 1, mat: 1, mdf: 1, agi: 99, luk: 1}
    ai: [{use: attack, target: weakest}]
  loner:
    name: Loner
    image: x.png
    stats: {mhp: 5000, mmp: 0, atk: 1, def: 1, mat: 1, mdf: 1, agi: 99, luk: 1}
    ai: [{use: jab, target: lowest_hp}]
  bully:
    name: Bully
    image: x.png
    stats: {mhp: 5000, mmp: 0, atk: 1, def: 1, mat: 1, mdf: 1, agi: 99, luk: 1}
    ai: [{use: jab, target: highest_hp}]
troops:
  dummy: {members: [dummy]}
  hunter: {members: [hunter]}
  striker: {members: [striker]}
  sniper: {members: [sniper]}
  loner: {members: [loner]}
  bully: {members: [bully]}
`

func statesBattle(t *testing.T, troop string) *battle.Battle {
	t.Helper()
	db := testgame.Load(t, map[string]string{"data/states.yaml": statesData})
	st := testgame.State(db, 1)
	st.Join("guide")
	return battle.New(db, st, troop)
}

func damagedTargets(steps []battle.Step) map[string]int {
	out := map[string]int{}
	for _, s := range steps {
		if s.Kind == battle.Damage && s.Target != nil && s.Target.Actor != nil {
			out[s.Target.Actor.ID]++
		}
	}
	return out
}

// TestConfusedBattlersHitAtRandom: a confused battler strikes anyone, friend or foe; its state may say so first.
func TestConfusedBattlersHitAtRandom(t *testing.T) {
	hit := map[string]int{}
	said := false
	for seed := uint64(1); seed <= 120; seed++ {
		b := statesBattle(t, "dummy")
		b.Rand = testgame.State(b.DB, seed).Rand
		scout := b.Party[0]
		scout.States["muddle"] = 9
		steps := turn(b, battle.Action{User: scout, Skill: "slap", Target: b.Foes[0]})
		for _, s := range steps {
			if s.Kind == battle.Damage && s.User == scout && s.Target != nil {
				if s.Target.Enemy {
					hit["foe"]++
				} else {
					hit[s.Target.Actor.ID]++
				}
			}
			if strings.Contains(s.Text, "is muddled") {
				said = true
			}
		}
	}
	if hit["foe"] == 0 || hit["guide"] == 0 || hit["scout"] == 0 {
		t.Errorf("a confused battler hits foe, friend and itself: %v", hit)
	}
	if !said {
		t.Error("the state's own message was not shown")
	}
	b := statesBattle(t, "dummy")
	b.Party[0].States["silent_muddle"] = 9
	if got := turn(b, battle.Action{User: b.Party[0], Skill: "slap", Target: b.Foes[0]}); strings.Contains(texts(got), "muddled") {
		t.Error("a state without a message shows none")
	}
}

func TestCoverAndCounterMessages(t *testing.T) {
	for _, withMsg := range []bool{true, false} {
		b := statesBattle(t, "striker")
		scout, guide := b.Party[0], b.Party[1]
		state := "guardian"
		if !withMsg {
			state = "guardian2"
		}
		guide.States[state] = 9
		scout.HP = scout.MHP() / 4
		got := turn(b, battle.Action{User: scout, Skill: "guard"}, battle.Action{User: guide, Skill: "guard"})
		took := damagedTargets(got)
		if took["scout"] != 0 || took["guide"] == 0 {
			t.Errorf("withMsg %v: the guardian takes the blow meant for the weak: %v\n%s", withMsg, took, texts(got))
		}
		if withMsg && !strings.Contains(texts(got), "shields") {
			t.Errorf("the cover message: %s", texts(got))
		}
	}
	for _, withMsg := range []bool{true, false} {
		b := statesBattle(t, "striker")
		scout := b.Party[0]
		state := "riposte"
		if !withMsg {
			state = "riposte2"
		}
		scout.States[state] = 9
		hunterHP := b.Foes[0].HP
		got := turn(b, battle.Action{User: scout, Skill: "guard"}, battle.Action{User: b.Party[1], Skill: "guard"})
		if b.Foes[0].HP >= hunterHP {
			t.Errorf("withMsg %v: the counter struck back: hp %d", withMsg, b.Foes[0].HP)
		}
		if withMsg && !strings.Contains(texts(got), "strikes back") {
			t.Errorf("the counter message: %s", texts(got))
		}
	}
	// a counter chance that fails to evaluate counters nothing
	b := statesBattle(t, "striker")
	b.Party[0].States["broken_counter"] = 9
	hp := b.Foes[0].HP
	turn(b, battle.Action{User: b.Party[0], Skill: "guard"}, battle.Action{User: b.Party[1], Skill: "guard"})
	if b.Foes[0].HP != hp {
		t.Error("a broken counter struck back")
	}
}

func TestDamageWakesASleeperAndLinesAreSaid(t *testing.T) {
	b := statesBattle(t, "dummy")
	scout, guide := b.Party[0], b.Party[1]
	b.Foes[0].States["doze"] = 9
	steps := turn(b, battle.Action{User: scout, Skill: "weak_spot", Target: b.Foes[0], Say: "Take this!"}, battle.Action{User: guide, Skill: "guard"})
	if b.Foes[0].States["doze"] != 0 {
		t.Errorf("a hit wakes the dozing foe: %v", b.Foes[0].States)
	}
	if !strings.Contains(texts(steps), "Take this!") {
		t.Errorf("the line said first: %s", texts(steps))
	}
	var wokeBy *battle.Unit
	for _, s := range steps {
		if s.Kind == battle.StateOff && s.State == "doze" {
			wokeBy = s.User
		}
	}
	if wokeBy != nil {
		t.Errorf("a state that wears off by damage is not credited to a skill that removed it: %v", wokeBy)
	}
}

// TestFoeTargetPresets: the named targets of an enemy's AI choose the party member they say.
func TestFoeTargetPresets(t *testing.T) {
	cases := []struct {
		troop string
		want  string
	}{
		{"hunter", "guide"}, // weakest: the lowest hp rate
		{"loner", "guide"},  // lowest hp
		{"bully", "scout"},  // highest hp
		{"sniper", "scout"}, // strongest: atk + mat
	}
	for _, c := range cases {
		b := statesBattle(t, c.troop)
		scout, guide := b.Party[0], b.Party[1]
		guide.HP = 10
		scout.HP = scout.MHP()
		if scout.Stat("atk")+scout.Stat("mat") < guide.Stat("atk")+guide.Stat("mat") {
			t.Fatal("the fixture needs the scout to be the stronger")
		}
		got := turn(b, battle.Action{User: scout, Skill: "guard"}, battle.Action{User: guide, Skill: "guard"})
		took := damagedTargets(got)
		if took[c.want] == 0 {
			t.Errorf("%s should hit %s, hit %v", c.troop, c.want, took)
		}
		if c.troop == "hunter" && !strings.Contains(texts(got), "Hunter picks the weakest.") {
			t.Errorf("the rule's line: %s", texts(got))
		}
	}
}
