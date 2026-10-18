package battle_test

import (
	"strings"
	"testing"

	"github.com/nao1215/gozen/battle"
	"github.com/nao1215/gozen/internal/testgame"
)

// 戦い方を変える敵のテスト: 即死（fatal の状態）と その防ぎ方、固い敵（hard）、逃げる敵、
// 自爆（sacrifice）、奪う技（on_user）。
const foes = `
items:
  ward: {name: Ward, kind: hat, slot: armor, state_resist: {doom: 0}}
skills:
  death_hand: {name: Death Hand, scope: one_enemy, effects: [{add_state: doom, chance: "1"}]}
  run_off: {name: Run, scope: self, effects: [{escape: true}]}
  blow_up: {name: Blow Up, scope: all_enemies, sacrifice: true, effects: [{damage: "10", variance: 0}]}
  steal_atk: {name: Steal, scope: one_enemy, effects: [{add_state: weak}, {add_state: strong, on_user: true}]}
  poke: {name: Poke, scope: one_enemy, effects: [{damage: "40", variance: 0}]}
states:
  doom: {name: Doom, fatal: true, bad: true}
  warded: {name: Warded, turns: 3, state_resist: {doom: 0}}
  weak: {name: Weak, turns: 3, stats: {atk: 0.5}}
  strong: {name: Strong, turns: 3, stats: {atk: 1.5}}
enemies:
  reaper:
    name: Reaper
    image: x.png
    stats: {mhp: 999, mmp: 0, atk: 1, def: 1, mat: 1, mdf: 1, agi: 99, luk: 1}
    ai: [{use: death_hand}]
  metal:
    name: Metal
    image: x.png
    hard: 1
    stats: {mhp: 60, mmp: 0, atk: 1, def: 0, mat: 1, mdf: 1, agi: 1, luk: 1}
    exp: 500
    ai: [{when: "turn >= 2", use: run_off}, {use: guard}]
  bomb:
    name: Bomb
    image: x.png
    stats: {mhp: 99, mmp: 0, atk: 1, def: 1, mat: 1, mdf: 1, agi: 99, luk: 1}
    ai: [{use: blow_up}]
  thief:
    name: Thief
    image: x.png
    stats: {mhp: 999, mmp: 0, atk: 10, def: 1, mat: 1, mdf: 1, agi: 99, luk: 1}
    ai: [{use: steal_atk}]
troops:
  reaper: {members: [reaper]}
  metal: {members: [metal]}
  bomb: {members: [bomb]}
  thief: {members: [thief]}
`

func foeSetup(t *testing.T, troop string) *battle.Battle {
	db := testgame.Load(t, map[string]string{"data/foes.yaml": foes})
	st := testgame.State(db, 1)
	return battle.New(db, st, troop)
}

func TestAnInstantDeathTakesALife(t *testing.T) {
	b := foeSetup(t, "reaper")
	scout := b.Party[0]
	steps := run(b, battle.Action{User: scout, Skill: "guard"})
	if scout.HP != 0 || b.Result != battle.Lost {
		t.Fatalf("the death hand did not take the scout (hp %d)\n%s", scout.HP, texts(steps))
	}
	if scout.States["doom"] != 0 {
		t.Error("the fatal state is kept on the fallen")
	}
}

func TestGearAndStatesWardOffAnInstantDeath(t *testing.T) {
	b := foeSetup(t, "reaper")
	scout := b.Party[0]
	scout.Actor.Equip["armor"] = "ward"
	steps := run(b, battle.Action{User: scout, Skill: "guard"})
	if scout.HP == 0 || !strings.Contains(texts(steps), "きかなかった") {
		t.Fatalf("the ward did not hold (hp %d)\n%s", scout.HP, texts(steps))
	}
	b = foeSetup(t, "reaper")
	scout = b.Party[0]
	scout.States["warded"] = 3
	run(b, battle.Action{User: scout, Skill: "guard"})
	if scout.HP == 0 {
		t.Fatal("a warding state did not hold")
	}
}

func TestAHardFoeTakesLittleFromThePlainAttack(t *testing.T) {
	b := foeSetup(t, "metal")
	scout, metal := b.Party[0], b.Foes[0]
	steps := run(b, battle.Action{User: scout, Skill: "attack", Target: metal})
	if metal.HP != 59 {
		t.Fatalf("the plain attack did %d (want 1)\n%s", 60-metal.HP, texts(steps))
	}
	run(b, battle.Action{User: scout, Skill: "fire", Target: metal}) // a skill: full damage (20)
	if metal.HP != 39 {
		t.Fatalf("a skill did %d to the hard foe (want 20)", 59-metal.HP)
	}
}

func TestAFoeRunsAway(t *testing.T) {
	b := foeSetup(t, "metal")
	scout := b.Party[0]
	run(b, battle.Action{User: scout, Skill: "guard"})
	if b.Result != battle.Ongoing {
		t.Fatal("over after a turn")
	}
	steps := run(b, battle.Action{User: scout, Skill: "guard"})
	if !b.Foes[0].Fled || !strings.Contains(texts(steps), "にげだした") {
		t.Fatalf("the metal did not run on the second turn\n%s", texts(steps))
	}
	if b.Result != battle.Won {
		t.Errorf("result %v after the last foe ran", b.Result)
	}
	if msgs := b.End(); b.St.Members()[0].Exp != 0 {
		t.Errorf("a foe that ran away gave exp: %v", msgs)
	}
}

func TestABombBlowsItselfUp(t *testing.T) {
	b := foeSetup(t, "bomb")
	scout, bomb := b.Party[0], b.Foes[0]
	steps := run(b, battle.Action{User: scout, Skill: "guard"})
	if bomb.HP != 0 || scout.HP == scout.MHP() {
		t.Fatalf("bomb hp %d, scout hp %d/%d\n%s", bomb.HP, scout.HP, scout.MHP(), texts(steps))
	}
	if b.Result != battle.Won {
		t.Errorf("result %v after the bomb blew up", b.Result)
	}
}

func TestATheftGoesToTheThief(t *testing.T) {
	b := foeSetup(t, "thief")
	scout, thief := b.Party[0], b.Foes[0]
	atk, tatk := scout.Stat("atk"), thief.Stat("atk")
	run(b, battle.Action{User: scout, Skill: "guard"})
	if scout.States["weak"] == 0 || thief.States["strong"] == 0 || thief.States["weak"] != 0 {
		t.Fatalf("states: scout %v thief %v", scout.States, thief.States)
	}
	if scout.Stat("atk") >= atk || thief.Stat("atk") <= tatk {
		t.Errorf("atk: scout %d→%d thief %d→%d", atk, scout.Stat("atk"), tatk, thief.Stat("atk"))
	}
}
