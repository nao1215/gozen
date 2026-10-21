package battle_test

import (
	"strings"
	"testing"

	"github.com/nao1215/gozen/internal/battle"
	"github.com/nao1215/gozen/internal/testgame"
)

const effectData = `
items:
  ether: {name: Ether, price: 5, use: {scope: one_ally, where: [battle, menu], effects: [{heal: "7", stat: mp}]}}
  tonic: {name: Big Tonic, price: 5, use: {scope: all_allies, where: [battle], effects: [{heal: "20"}]}}
  sleepy_blade: {name: Sleepy Blade, kind: sword, slot: weapon, stats: {atk: 1}, states: [{state: sleep, chance: "1"}]}
skills:
  siphon: {name: Siphon, scope: one_enemy, effects: [{damage: "9", stat: mp, variance: 0}]}
  siphon_big: {name: Siphon Big, scope: one_enemy, effects: [{damage: "9999", stat: mp, variance: 0}]}
  charge: {name: Charge, scope: one_ally, effects: [{heal: "4", stat: mp}]}
  charge_big: {name: Overcharge, scope: self, effects: [{heal: "999", stat: mp}]}
  ram: {name: Ram, scope: one_enemy, effects: [{damage: "20", recoil: 0.5, variance: 0}]}
  cleanse: {name: Cleanse, scope: one_ally, effects: [{remove_state: bad}]}
  unpoison: {name: Antidote, scope: one_ally, effects: [{remove_state: poison}]}
  rez: {name: Rez, scope: dead_ally, effects: [{revive: "b.mhp / 2"}]}
  rez_all: {name: Rez All, scope: all_dead_allies, effects: [{revive: "b.mhp"}]}
  flee: {name: Flee, scope: self, effects: [{escape: true}]}
  sure_miss: {name: Whiff, scope: one_enemy, hit: "0", effects: [{damage: "5"}]}
  say_hit: {name: Shout, scope: one_enemy, effects: [{damage: "1", variance: 0}]}
  all_ring: {name: Ring, scope: everyone, effects: [{damage: "1", variance: 0}]}
  pricey: {name: Pricey, mp: 99, scope: one_enemy, effects: [{damage: "1"}]}
  broken_formula: {name: Broken, scope: one_enemy, effects: [{damage: "len(a.hp)"}]}
  broken_heal: {name: Broken Heal, scope: one_ally, effects: [{heal: "len(a.hp)"}]}
  broken_revive: {name: Broken Rise, scope: dead_ally, effects: [{revive: "len(a.hp)"}]}
  conditional: {name: Only Poisoned, scope: one_enemy, effects: [{damage: "5", when: "b.states.poison > 0"}]}
  hurt_all: {name: Shock, scope: all_allies, effects: [{damage: "9999", variance: 0}]}
  poison_hit: {name: Poison Hit, scope: one_enemy, effects: [{damage: "1", variance: 0}, {add_state: poison, chance: "1"}]}
states:
  wake: {name: Light Sleep, turns: 9, restrict: skip, remove_on_damage: 1}
  thorny: {name: Thorns, turns: 3}
enemies:
  caster:
    name: Caster
    image: x.png
    stats: {mhp: 200, mmp: 20, atk: 1, def: 1, mat: 1, mdf: 1, agi: 1, luk: 1}
    ai: [{use: guard}]
troops:
  caster: {members: [caster]}
  pair: {members: [mote, mote]}
`

func effectSetup(t *testing.T, troop string) (*battle.Battle, int) {
	t.Helper()
	db := testgame.Load(t, map[string]string{"data/effects.yaml": effectData})
	st := testgame.State(db, 1)
	return battle.New(db, st, troop), 0
}

// effectDuo is the same battle with the guide also in the party; the guide holds still (guards) every turn.
func effectDuo(t *testing.T, troop string) *battle.Battle {
	t.Helper()
	db := testgame.Load(t, map[string]string{"data/effects.yaml": effectData})
	st := testgame.State(db, 1)
	st.Join("guide")
	return battle.New(db, st, troop)
}

// turn plays a turn in which the guide, if present, guards.
func turn(b *battle.Battle, actions ...battle.Action) []battle.Step {
	for _, u := range b.Party[1:] {
		actions = append(actions, battle.Action{User: u, Skill: "guard"})
	}
	return run(b, actions...)
}

func steps(kind battle.StepKind, all []battle.Step) []battle.Step {
	var out []battle.Step
	for _, s := range all {
		if s.Kind == kind {
			out = append(out, s)
		}
	}
	return out
}

// TestManaDamageAndHealingStayInRange: a skill with stat: mp takes at most the target's MP, and charging never passes the maximum.
func TestManaDamageAndHealingStayInRange(t *testing.T) {
	b, _ := effectSetup(t, "caster")
	scout, caster := b.Party[0], b.Foes[0]
	got := run(b, battle.Action{User: scout, Skill: "siphon", Target: caster})
	dmg := steps(battle.Damage, got)
	if len(dmg) != 1 || dmg[0].Stat != "mp" || dmg[0].Amount != 9 || caster.MP != 11 || caster.HP != caster.MHP() {
		t.Fatalf("mana damage: %+v mp %d hp %d", dmg, caster.MP, caster.HP)
	}
	run(b, battle.Action{User: scout, Skill: "siphon_big", Target: caster})
	if caster.MP != 0 {
		t.Errorf("mana cannot go below 0, got %d", caster.MP)
	}
	scout.MP = 1
	got = run(b, battle.Action{User: scout, Skill: "charge", Target: scout})
	heal := steps(battle.Heal, got)
	if len(heal) != 1 || heal[0].Stat != "mp" || heal[0].Amount != 4 || scout.MP != 5 {
		t.Fatalf("mana heal: %+v mp %d", heal, scout.MP)
	}
	run(b, battle.Action{User: scout, Skill: "charge_big"})
	if scout.MP != scout.MMP() {
		t.Errorf("mana cannot pass the maximum %d, got %d", scout.MMP(), scout.MP)
	}
	// a heal on someone already full does nothing and says nothing of the sort
	got = run(b, battle.Action{User: scout, Skill: "charge", Target: scout})
	if h := steps(battle.Heal, got); len(h) != 0 && h[0].Amount != 0 {
		t.Errorf("healing a full gauge healed %d", h[0].Amount)
	}
}

func TestRecoilNeverKills(t *testing.T) {
	b, _ := effectSetup(t, "caster")
	scout, caster := b.Party[0], b.Foes[0]
	scout.HP = 30
	run(b, battle.Action{User: scout, Skill: "ram", Target: caster})
	if scout.HP != 20 {
		t.Errorf("half of the 20 damage comes back: hp %d, want 20", scout.HP)
	}
	scout.HP = 6
	run(b, battle.Action{User: scout, Skill: "ram", Target: caster})
	if scout.HP != 1 {
		t.Errorf("recoil leaves at least 1 HP, got %d", scout.HP)
	}
	scout.HP = 1
	run(b, battle.Action{User: scout, Skill: "ram", Target: caster})
	if scout.HP != 1 {
		t.Errorf("recoil at 1 HP changes nothing, got %d", scout.HP)
	}
}

func TestRemoveStateAndRevive(t *testing.T) {
	b := effectDuo(t, "one")
	scout, guide := b.Party[0], b.Party[1]
	guide.States["poison"] = 9
	guide.States["sleep"] = 3
	got := turn(b, battle.Action{User: scout, Skill: "unpoison", Target: guide})
	if off := steps(battle.StateOff, got); len(off) != 1 || off[0].State != "poison" || off[0].User != scout {
		t.Fatalf("removing one named state: %+v", off)
	}
	if _, ok := guide.States["sleep"]; !ok {
		t.Error("only the named state goes")
	}
	guide.States["poison"] = 9
	turn(b, battle.Action{User: scout, Skill: "cleanse", Target: guide})
	if len(guide.States) != 0 {
		t.Errorf("remove_state: bad clears every bad state, left %v", guide.States)
	}
	// revive
	guide.HP = 0
	for k := range guide.States {
		delete(guide.States, k)
	}
	got = turn(b, battle.Action{User: scout, Skill: "rez", Target: guide})
	rev := steps(battle.Revive, got)
	if len(rev) != 1 || guide.HP != guide.MHP()/2 {
		t.Fatalf("revive to half: %+v hp %d of %d", rev, guide.HP, guide.MHP())
	}
	// reviving someone who stands does nothing
	hp := guide.HP
	got = turn(b, battle.Action{User: scout, Skill: "rez", Target: guide})
	if len(steps(battle.Revive, got)) != 0 || guide.HP > hp {
		t.Error("a living target cannot be revived")
	}
	// everyone who fell
	b2 := effectDuo(t, "one")
	s2, g2 := b2.Party[0], b2.Party[1]
	g2.HP = 0
	turn(b2, battle.Action{User: s2, Skill: "rez_all"})
	if g2.HP < g2.MHP()-1 { // the mote may slap once after
		t.Errorf("rez_all brings everyone back whole, hp %d of %d", g2.HP, g2.MHP())
	}
}

func TestEscapeSkill(t *testing.T) {
	b, _ := effectSetup(t, "one")
	run(b, battle.Action{User: b.Party[0], Skill: "flee"})
	if b.Result != battle.Fled {
		t.Errorf("result %v, want escape", b.Result)
	}
	bb, _ := effectSetup(t, "boss")
	got := run(bb, battle.Action{User: bb.Party[0], Skill: "flee"})
	if bb.Result == battle.Fled || len(steps(battle.Escape, got)) != 0 {
		t.Error("a battle that cannot be escaped is not escaped")
	}
}

func TestMissesAndShortages(t *testing.T) {
	b, _ := effectSetup(t, "one")
	scout, mote := b.Party[0], b.Foes[0]
	got := run(b, battle.Action{User: scout, Skill: "sure_miss", Target: mote})
	if len(steps(battle.Miss, got)) != 1 || mote.HP != mote.MHP() {
		t.Errorf("hit 0 always misses: %v hp %d", steps(battle.Miss, got), mote.HP)
	}
	mp := scout.MP
	run(b, battle.Action{User: scout, Skill: "pricey", Target: mote})
	if scout.MP != mp || mote.HP != mote.MHP() {
		t.Error("a skill that costs more than the MP held does nothing")
	}
	run(b, battle.Action{User: scout, Skill: "pricey", Target: mote, Say: "Take this"})
	if ok, why := b.CanUse(scout, "pricey"); ok || why == "" {
		t.Errorf("CanUse says %v %q", ok, why)
	}
	if ok, why := b.CanUse(scout, "nothing"); ok || why != "unknown skill" {
		t.Errorf("CanUse of an unknown skill: %v %q", ok, why)
	}
	if ok, _ := b.CanUse(scout, "slap"); !ok {
		t.Error("a skill that costs nothing can be used")
	}
	// an item the party does not hold, or an enemy using an item
	delete(b.St.Items, "ether")
	got = run(b, battle.Action{User: scout, Item: "ether", Target: scout})
	if len(got) == 0 {
		t.Error("using an item that is not held still says something")
	}
	// a skill that does not exist
	got = run(b, battle.Action{User: scout, Skill: "nothing"})
	if !strings.Contains(texts(got), scout.Name) {
		t.Errorf("an unknown skill leaves the user idle with a line: %q", texts(got))
	}
}

func TestItemsInBattle(t *testing.T) {
	b := effectDuo(t, "one")
	scout, guide := b.Party[0], b.Party[1]
	b.St.AddItem("ether", 2)
	b.St.AddItem("tonic", 1)
	scout.MP = 0
	run(b, battle.Action{User: scout, Item: "ether", Target: scout})
	if scout.MP != 7 || b.St.Items["ether"] != 1 {
		t.Errorf("an item restores mana and is spent: mp %d, ethers %d", scout.MP, b.St.Items["ether"])
	}
	scout.HP, guide.HP = 1, 1
	turn(b, battle.Action{User: scout, Item: "tonic"})
	if scout.HP != 21 || guide.HP < 20 || guide.HP > 21 || b.St.Items["tonic"] != 0 {
		t.Errorf("an all-allies item reaches everyone: %d %d, tonics %d", scout.HP, guide.HP, b.St.Items["tonic"])
	}
}

func TestEveryoneAndDeadScopes(t *testing.T) {
	b := effectDuo(t, "pair")
	scout, guide := b.Party[0], b.Party[1]
	hp := map[*battle.Unit]int{scout: scout.HP, guide: guide.HP, b.Foes[0]: b.Foes[0].HP, b.Foes[1]: b.Foes[1].HP}
	turn(b, battle.Action{User: scout, Skill: "all_ring"})
	for u, before := range hp {
		if u.HP >= before {
			t.Errorf("%s was not hit by a skill on everyone", u.Name)
		}
	}
}

func TestBrokenFormulasReportInsteadOfCrashing(t *testing.T) {
	b, _ := effectSetup(t, "one")
	scout, mote := b.Party[0], b.Foes[0]
	for _, skill := range []string{"broken_formula", "broken_heal"} {
		got := run(b, battle.Action{User: scout, Skill: skill, Target: scout})
		if !strings.Contains(texts(got), "len of") {
			t.Errorf("%s: the error is reported in the log: %q", skill, texts(got))
		}
	}
	if mote.HP != mote.MHP() {
		t.Error("a broken formula hurt someone")
	}
	bd := effectDuo(t, "one")
	b = bd
	scout = b.Party[0]
	b.Party[1].HP = 0
	got := turn(b, battle.Action{User: scout, Skill: "broken_revive", Target: b.Party[1]})
	if b.Party[1].HP != 0 || !strings.Contains(texts(got), "len of") {
		t.Errorf("a broken revive formula revives no one: %q", texts(got))
	}
}

func TestEffectWhenConditionAndStatusFromWeapon(t *testing.T) {
	b, _ := effectSetup(t, "one")
	scout, mote := b.Party[0], b.Foes[0]
	run(b, battle.Action{User: scout, Skill: "conditional", Target: mote})
	if mote.HP != mote.MHP() {
		t.Error("an effect whose when does not hold does nothing")
	}
	mote.States["poison"] = 9
	run(b, battle.Action{User: scout, Skill: "conditional", Target: mote})
	if mote.HP >= mote.MHP() {
		t.Error("the effect works once the condition holds")
	}
	// a weapon that puts a state on what the wearer hits
	b2, _ := effectSetup(t, "one")
	b2.St.AddItem("sleepy_blade", 1)
	if err := b2.St.Equip(b2.St.Actors["scout"], "weapon", "sleepy_blade"); err != nil {
		t.Fatal(err)
	}
	b3 := battle.New(b2.DB, b2.St, "one")
	run(b3, battle.Action{User: b3.Party[0], Skill: "attack", Target: b3.Foes[0]})
	if b3.Foes[0].States["sleep"] == 0 {
		t.Errorf("the weapon's state was put on the target: %v", b3.Foes[0].States)
	}
}

func TestPartyWipeAndLoss(t *testing.T) {
	b, _ := effectSetup(t, "one")
	scout := b.Party[0]
	run(b, battle.Action{User: scout, Skill: "hurt_all"})
	if b.Result != battle.Lost {
		t.Errorf("result %v, want lose", b.Result)
	}
	if b.Result.String() != "lose" || battle.Result(9).String() != "unknown" || battle.Won.String() != "win" || battle.Fled.String() != "escape" {
		t.Error("Result words")
	}
}
