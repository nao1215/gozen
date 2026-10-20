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

// run plays a turn with the given party actions and returns its steps.
func run(b *battle.Battle, actions ...battle.Action) []battle.Step {
	b.StartTurn(actions)
	var all []battle.Step
	for {
		steps, done := b.Next()
		all = append(all, steps...)
		if done {
			return all
		}
	}
}

func texts(steps []battle.Step) string {
	var s []string
	for _, st := range steps {
		s = append(s, st.Text)
	}
	return strings.Join(s, "\n")
}

func setup(t *testing.T, troop string) (*battle.Battle, *rpg.State) {
	db := testgame.Load(t, nil)
	st := testgame.State(db, 1)
	return battle.New(db, st, troop), st
}

func TestDamageFormulaAndElement(t *testing.T) {
	b, _ := setup(t, "one")
	scout, mote := b.Party[0], b.Foes[0]
	steps := run(b, battle.Action{User: scout, Skill: "attack", Target: mote})
	// atk 15 * 2 - def 2 = 28
	var dmg *battle.Step
	for i := range steps {
		if steps[i].Kind == battle.Damage && steps[i].Target == mote {
			dmg = &steps[i]
		}
	}
	if dmg == nil || dmg.Amount != 28 || mote.HP != 2 {
		t.Fatalf("attack: %+v, mote hp %d\n%s", dmg, mote.HP, texts(steps))
	}
	// fire: 20, doubled by the mote's resistance -> it falls
	steps = run(b, battle.Action{User: scout, Skill: "fire", Target: mote})
	if b.Result != battle.Won {
		t.Fatalf("fire did not finish the mote\n%s", texts(steps))
	}
	if scout.MP != scout.MMP()-3 {
		t.Errorf("fire cost %d MP", scout.MMP()-scout.MP)
	}
}

func TestEnemyRulesAndRewards(t *testing.T) {
	b, st := setup(t, "one")
	scout := b.Party[0]
	steps := run(b, battle.Action{User: scout, Skill: "guard"})
	if !strings.Contains(texts(steps), "Mote") || scout.HP != scout.MHP()-1 {
		t.Fatalf("the mote should slap for 1 (guarding halves it to 0.5, rounded)\n%s\nhp %d", texts(steps), scout.HP)
	}
	run(b, battle.Action{User: scout, Skill: "big"})
	if b.Result != battle.Won {
		t.Fatal("not won")
	}
	msgs := b.End()
	if st.Gold != 107 || st.Items["key"] != 1 || st.Members()[0].Exp != 25 {
		t.Errorf("rewards: gold %d key %d exp %d\n%v", st.Gold, st.Items["key"], st.Members()[0].Exp, msgs)
	}
}

func TestRulePriorityAndStates(t *testing.T) {
	b, _ := setup(t, "boss")
	scout, boss := b.Party[0], b.Foes[0]
	// turn 1: the boss's rule "turn == 1" (priority 3) puts the scout to sleep
	steps := run(b, battle.Action{User: scout, Skill: "guard"})
	if scout.States["sleep"] == 0 {
		t.Fatalf("the scout is not asleep\n%s", texts(steps))
	}
	// turn 2: the troop event fires, the scout cannot act
	b.StartTurn(nil)
	var all []battle.Step
	for {
		steps, done := b.Next()
		all = append(all, steps...)
		if done {
			break
		}
	}
	events := 0
	for _, s := range append(steps, all...) { // the start of turn 2 comes with the end of turn 1
		if s.Kind == battle.Event {
			events++
		}
	}
	if events != 1 {
		t.Errorf("the turn-2 event fired %d times", events)
	}
	if !strings.Contains(texts(all), "うごけない") {
		t.Errorf("a sleeping scout acted\n%s", texts(all))
	}
	// the sleep lasts 2 turns
	run(b)
	if scout.States["sleep"] != 0 {
		t.Errorf("sleep still on after its turns: %v", scout.States)
	}
	// below half HP the boss heals itself (priority 5)
	boss.HP = 100
	act := b.Decide(boss)
	if act.Skill != "mend" || act.Target != boss {
		t.Errorf("the boss chose %s on %v", act.Skill, act.Target)
	}
}

func TestTroopEventFiresOnce(t *testing.T) {
	b, _ := setup(t, "boss")
	scout := b.Party[0]
	count := 0
	for range 4 {
		for _, s := range run(b, battle.Action{User: scout, Skill: "guard"}) {
			if s.Kind == battle.Event {
				count++
			}
		}
	}
	if count != 1 {
		t.Errorf("the turn-2 event fired %d times", count)
	}
}

func TestPoisonTicksAndPersists(t *testing.T) {
	b, st := setup(t, "two")
	scout := b.Party[0]
	b.Foes[0].HP, b.Foes[1].HP = 1000, 1000
	// put poison on the scout by hand: the venom skill is the enemies' own
	scout.States["poison"] = 999
	before := scout.HP
	run(b, battle.Action{User: scout, Skill: "guard"})
	if scout.HP > before-5 {
		t.Errorf("poison did not hurt: %d -> %d", before, scout.HP)
	}
	b.Result = battle.Fled
	b.End()
	if st.Members()[0].States["poison"] == 0 {
		t.Error("poison (persist) was cured by the end of the battle")
	}
}

func TestEscape(t *testing.T) {
	b, _ := setup(t, "one")
	if _, ok := b.TryEscape(); !ok || b.Result != battle.Fled {
		t.Error("escape chance 1 failed")
	}
	b2, _ := setup(t, "boss")
	if _, ok := b2.TryEscape(); ok {
		t.Error("escaped a troop that cannot be escaped")
	}
}

func TestUseInMenu(t *testing.T) {
	db := testgame.Load(t, nil)
	st := testgame.State(db, 1)
	a := st.Members()[0]
	a.HP = 5
	msgs, err := battle.UseInMenu(db, st, a, "", "potion", a)
	if err != nil {
		t.Fatal(err)
	}
	if a.HP != 35 || st.Items["potion"] != 1 {
		t.Errorf("hp %d potions %d (%v)", a.HP, st.Items["potion"], msgs)
	}
	if _, err := battle.UseInMenu(db, st, a, "fire", "", nil); err == nil {
		t.Error("fire (battle only) was used from the menu")
	}
}

func TestGambitsTakeTheFirstRuleThatHolds(t *testing.T) {
	b, _ := setup(t, "boss")
	b.DB.Game.Battle.AI = "gambit"
	defer func() { b.DB.Game.Battle.AI = "" }()
	boss := b.Foes[0]
	// the rules from the top: mend when hurt, nap on turn 1, slap
	if a := b.Decide(boss); a.Skill != "nap" {
		t.Fatalf("turn 1: %q, want nap (the first that holds)", a.Skill)
	}
	b.Turn = 2
	if a := b.Decide(boss); a.Skill != "slap" {
		t.Fatalf("turn 2: %q, want slap", a.Skill)
	}
	boss.HP = boss.MHP() / 3
	for range 10 {
		if a := b.Decide(boss); a.Skill != "mend" {
			t.Fatalf("hurt: %q, want mend", a.Skill)
		}
	}
}

func TestCoverAndCounter(t *testing.T) {
	db := testgame.Load(t, nil)
	st := testgame.State(db, 1)
	st.Join("guide")
	b := battle.New(db, st, "one")
	db.States["guardian"] = &data.State{Name: "guardian", Cover: data.Expr{Expr: testgame.Expr("b.hp_rate < 0.5")},
		Counter: data.Expr{Expr: testgame.Expr("1")}}
	db.Order["states"] = append(db.Order["states"], "guardian")
	if len(b.Party) < 2 {
		t.Fatal("fixture must supply two party members")
	}
	knight, weak := b.Party[0], b.Party[1]
	knight.States["guardian"] = 99
	weak.HP = 1
	db.Skills["slap"].Anim = "slash" // a physical attack on one
	hp := knight.HP
	var steps []battle.Step
	for range 5 { // the mote slaps someone: the weak one is covered whenever it is aimed at
		steps = append(steps, run(b, battle.Action{User: knight, Skill: "guard"}, battle.Action{User: weak, Skill: "guard"})...)
	}
	if weak.HP != 1 || knight.HP >= hp && !strings.Contains(texts(steps), "かばった") {
		t.Fatalf("not covered: weak %d, knight %d->%d\n%s", weak.HP, hp, knight.HP, texts(steps))
	}
	if !strings.Contains(texts(steps), "カウンター") {
		t.Fatalf("no counter\n%s", texts(steps))
	}
}

// TestSummon: a foe calls another into the battle, in the place of a fallen one first,
// and no more than summon_max stand.
func TestSummon(t *testing.T) {
	extra := map[string]string{"data/s.yaml": `
skills:
  call: {name: Call, scope: self, effects: [{summon: mote, summon_max: 2}]}
enemies:
  caller:
    name: Caller
    image: x.png
    stats: {mhp: 999, mmp: 0, atk: 1, def: 99, mat: 1, mdf: 1, agi: 99, luk: 1}
    ai: [{use: call}]
troops:
  calling: {members: [caller]}
`}
	db := testgame.Load(t, extra)
	st := testgame.State(db, 1)
	b := battle.New(db, st, "calling")
	scout := b.Party[0]
	steps := run(b, battle.Action{User: scout, Skill: "guard"})
	if len(b.Foes) != 2 || b.Foes[1].EID != "mote" || !strings.Contains(texts(steps), "Mote") {
		t.Fatalf("after one turn: %d foes\n%s", len(b.Foes), texts(steps))
	}
	steps = run(b, battle.Action{User: scout, Skill: "guard"})
	if len(b.Foes) != 2 || !strings.Contains(texts(steps), "なにも") {
		t.Fatalf("summon_max 2 let more in: %d foes\n%s", len(b.Foes), texts(steps))
	}
	b.Foes[1].HP = 0
	run(b, battle.Action{User: scout, Skill: "guard"})
	if len(b.Foes) != 2 || !b.Foes[1].Alive() {
		t.Fatalf("the fallen place was not taken: %d foes", len(b.Foes))
	}
}

// TestSummonedNames: when two of the same summoned kind exist, the earlier one is A and the later one B
// (the A is not dropped, as in "Mote/MoteB"). An absorb of 0 does not print "absorbed 0".
func TestSummonedNames(t *testing.T) {
	db := testgame.Load(t, map[string]string{"data/s.yaml": `
skills:
  call: {name: Call, scope: self, effects: [{summon: mote, summon_max: 3}]}
  suck: {name: Suck, scope: one_enemy, effects: [{damage: "5", variance: 0, drain: 1}]}
enemies:
  caller:
    name: Caller
    image: x.png
    stats: {mhp: 999, mmp: 0, atk: 1, def: 99, mat: 1, mdf: 1, agi: 99, luk: 1}
    ai: [{use: call}]
troops:
  calling: {members: [caller]}
`})
	st := testgame.State(db, 1)
	b := battle.New(db, st, "calling")
	scout := b.Party[0]
	run(b, battle.Action{User: scout, Skill: "guard"})
	run(b, battle.Action{User: scout, Skill: "guard"})
	if len(b.Foes) != 3 || b.Foes[1].Name != "MoteA" || b.Foes[2].Name != "MoteB" {
		var names []string
		for _, f := range b.Foes {
			names = append(names, f.Name)
		}
		t.Fatalf("names %v", names)
	}
	steps := run(b, battle.Action{User: scout, Skill: "suck", Target: b.Foes[1]})
	if strings.Contains(texts(steps), "0 すいとった") {
		t.Fatalf("a drain of 0 told:\n%s", texts(steps))
	}
}

// TestIntroOfAMixedTroop: a troop of mixed kinds does not say "X and the others".
func TestIntroOfAMixedTroop(t *testing.T) {
	db := testgame.Load(t, map[string]string{"data/m.yaml": `
enemies:
  bat:
    name: Bat
    image: x.png
    stats: {mhp: 10, mmp: 0, atk: 1, def: 1, mat: 1, mdf: 1, agi: 1, luk: 1}
troops:
  mixed: {members: [mote, bat, mote]}
  same: {members: [mote, mote]}
`})
	st := testgame.State(db, 1)
	if got := battle.New(db, st, "mixed").Begin()[0].Text; got != "Moteと Batが あらわれた！" {
		t.Errorf("mixed: %q", got)
	}
	if got := battle.New(db, st, "same").Begin()[0].Text; got != "Moteたちが あらわれた！" {
		t.Errorf("same: %q", got)
	}
}

// HP absorb is capped at the target's remaining HP (the amount actually lost), and the display shows the amount actually gained.
func TestDrainIsCappedByWhatWasLost(t *testing.T) {
	db := testgame.Load(t, map[string]string{"data/d.yaml": `
skills:
  suck: {name: Suck, scope: one_enemy, effects: [{damage: "100", variance: 0, drain: 1}]}
`})
	st := testgame.State(db, 1)
	b := battle.New(db, st, "one")
	scout, mote := b.Party[0], b.Foes[0]
	scout.HP = 10
	steps := run(b, battle.Action{User: scout, Skill: "suck", Target: mote})
	var heal *battle.Step
	for i := range steps {
		if steps[i].Kind == battle.Heal && steps[i].Target == scout {
			heal = &steps[i]
		}
	}
	// The slime has 30 HP. Even with 100 damage only up to 30 can be absorbed.
	if heal == nil || heal.Amount != 30 || scout.HP != 40 {
		t.Fatalf("吸収 %+v、勇者の HP %d（欲しいのは 30 吸って 40）\n%s", heal, scout.HP, texts(steps))
	}
}

// game.yaml battle back_weight: if 0, enemies do not target the back row.
func TestRowWeightsFromTheData(t *testing.T) {
	g := strings.Replace(testgame.Game, "battle:\n", "battle:\n  front_weight: 1\n  back_weight: 0\n", 1)
	db := testgame.Load(t, map[string]string{"game.yaml": g})
	st := testgame.State(db, 1)
	st.Join("guide")
	st.Actors["guide"].Back = true
	b := battle.New(db, st, "two")
	scout, guide := b.Party[0], b.Party[1]
	full := guide.HP
	for range 10 {
		run(b, battle.Action{User: scout, Skill: "guard"}, battle.Action{User: guide, Skill: "guard"})
	}
	if guide.HP != full || scout.HP == scout.MHP() {
		t.Errorf("後列の HP %d/%d、前列の HP %d/%d", guide.HP, full, scout.HP, scout.MHP())
	}
}

// From the menu a healing potion cannot be used on a downed party member (and does not move to another member), and an inflicted state lasts for its remaining turns.
func TestUseInMenuTargetsAndStates(t *testing.T) {
	db := testgame.Load(t, map[string]string{"data/more.yaml": `
skills:
  bless: {name: Bless, mp: 1, scope: all_allies, where: [menu], effects: [{heal: "1"}, {add_state: regen, turns: "1 + a.lv"}]}
states:
  regen: {name: Regen, turns: 9}
`})
	st := testgame.State(db, 1)
	if !st.Join("guide") {
		t.Fatal("対象を分けて試す仲間を加えられない")
	}
	ms := st.Members()
	if len(ms) < 2 {
		t.Fatal("対象を分けて試す仲間が足りない")
	}
	scout, guide := ms[0], ms[1]
	guide.HP = 0
	n := st.Items["potion"]
	if _, err := battle.UseInMenu(db, st, scout, "", "potion", guide); err == nil {
		t.Error("a potion was used on a fallen member")
	}
	if st.Items["potion"] != n || guide.HP != 0 {
		t.Errorf("potions %d (want %d), guide hp %d", st.Items["potion"], n, guide.HP)
	}
	if battle.CanTarget(db, "", "potion", guide) || !battle.CanTarget(db, "", "potion", scout) {
		t.Error("CanTarget: the fallen one is no target of a potion, the living one is")
	}
	if _, err := battle.UseInMenu(db, st, scout, "bless", "", nil); err != nil {
		t.Fatal(err)
	}
	if scout.States["regen"] != 2 {
		t.Errorf("regen %d turns after the menu, want 2 (1 + a.lv)", scout.States["regen"])
	}
}

// TestSummonKeepsTheFallen: even if summoned into a fallen enemy's slot, the earlier fallen enemy's kill count and reward remain.
func TestSummonKeepsTheFallen(t *testing.T) {
	db := testgame.Load(t, map[string]string{"data/s.yaml": `
skills:
  call: {name: Call, scope: self, effects: [{summon: mote, summon_max: 2}]}
enemies:
  caller:
    name: Caller
    image: x.png
    stats: {mhp: 999, mmp: 0, atk: 1, def: 99, mat: 1, mdf: 1, agi: 99, luk: 1}
    ai: [{use: call}]
troops:
  calling: {members: [caller]}
`})
	st := testgame.State(db, 1)
	b := battle.New(db, st, "calling")
	scout := b.Party[0]
	run(b, battle.Action{User: scout, Skill: "guard"})
	first := b.Foes[1]
	first.HP = 0
	run(b, battle.Action{User: scout, Skill: "guard"})
	if b.Foes[1] == first || !b.Foes[1].Alive() {
		t.Fatal("the fallen place was not taken")
	}
	for _, f := range b.Foes {
		f.HP = 0
	}
	b.Result = battle.Won
	b.End()
	if n := optional.Value[float64](st.Vars["kill_mote"]); n != 2 {
		t.Fatalf("kill_mote %v, want 2", st.Vars["kill_mote"])
	}
	want := db.Enemies["caller"].Exp + 2*db.Enemies["mote"].Exp
	if b.Gains.Exp != want {
		t.Fatalf("exp %d, want %d", b.Gains.Exp, want)
	}
}

// When a story continues after a loss, only the party members who fought get up (those downed outside the duel stay down).
func TestRiseOnlyTheFighters(t *testing.T) {
	db := testgame.Load(t, nil)
	st := testgame.State(db, 1)
	st.Join("guide")
	guide := st.Actors["guide"]
	guide.HP = 0
	st.Fighters = []string{"scout"}
	b := battle.New(db, st, "one")
	st.Fighters = nil
	b.Party[0].HP = 0
	b.Result = battle.Lost
	b.End()
	b.Rise()
	if h := st.Actors["scout"].HP; h != 1 {
		t.Errorf("戦った scout の HP %d、want 1", h)
	}
	if guide.HP != 0 {
		t.Errorf("戦っていない仲間が起きた: HP %d", guide.HP)
	}
}

// The menu's heal-all cannot be used if everyone is at full and MP is not spent. It can be used if someone is hurt.
func TestUseInMenuHealAllAtFull(t *testing.T) {
	db := testgame.Load(t, map[string]string{"data/more.yaml": `
skills:
  healall: {name: HealAll, mp: 5, scope: all_allies, where: [menu], effects: [{heal: "50"}]}
`})
	st := testgame.State(db, 1)
	st.Join("guide")
	scout := st.Actors["scout"]
	mp := scout.MP
	if _, err := battle.UseInMenu(db, st, scout, "healall", "", nil); err == nil {
		t.Error("全員満タンで全体回復を使えた")
	}
	if scout.MP != mp {
		t.Errorf("MP %d、want %d", scout.MP, mp)
	}
	st.Actors["guide"].HP--
	if _, err := battle.UseInMenu(db, st, scout, "healall", "", nil); err != nil {
		t.Fatal(err)
	}
	if scout.MP != mp-5 {
		t.Errorf("MP %d、want %d", scout.MP, mp-5)
	}
}

// Even on auto, a move with speed (a shield skill that prepares for a telegraphed attack) is decided at the start of the turn and acts before faster enemies.
func TestAutoFastSkillGoesFirst(t *testing.T) {
	db := testgame.Load(t, map[string]string{"data/more.yaml": `
skills:
  wall: {name: Wall, scope: all_allies, speed: "1000", effects: []}
enemies:
  quick:
    name: Quick
    image: x.png
    stats: {mhp: 999, mmp: 0, atk: 1, def: 99, mat: 1, mdf: 1, agi: 500, luk: 1}
troops:
  quick: {members: [quick]}
`})
	db.Actors["scout"].AI = data.AIRef{Rules: []data.Rule{{Use: "wall"}}}
	st := testgame.State(db, 1)
	st.Actors["scout"].Learnt = append(st.Actors["scout"].Learnt, "wall")
	b := battle.New(db, st, "quick")
	b.Party[0].Auto = true
	for _, s := range run(b) {
		if s.Kind == battle.Act {
			if s.User != b.Party[0] || s.Skill != "wall" {
				t.Fatalf("最初に動いたのは %s の %s", s.User.Name, s.Skill)
			}
			return
		}
	}
	t.Fatal("誰も動かなかった")
}

// The kill count of the previous fight is rewritten even for a fight that was fled, and the count from the earlier fight does not remain.
func TestLastKillIsTheLastBattle(t *testing.T) {
	db := testgame.Load(t, nil)
	st := testgame.State(db, 1)
	b := battle.New(db, st, "two")
	b.Foes[0].HP = 0
	b.Result = battle.Won
	b.End()
	if v := optional.Value[float64](st.Vars[battle.VarLastKill+"mote"]); v != 1 {
		t.Fatalf("last_kill_mote %v、want 1", v)
	}
	b = battle.New(db, st, "two")
	b.Result = battle.Fled
	b.End()
	if v, ok := st.Vars[battle.VarLastKill+"mote"]; ok {
		t.Errorf("逃げた戦いの後に前の数 %v が残った", v)
	}
	if v := optional.Value[float64](st.Vars["kill_mote"]); v != 1 {
		t.Errorf("kill_mote %v、want 1", v)
	}
}

// restrict_chance: a state that only sometimes loses the action. The command can be chosen, and the roll happens at action time.
func TestRestrictChance(t *testing.T) {
	db := testgame.Load(t, map[string]string{"data/daze.yaml": `
states:
  daze: {name: Daze, turns: 0, restrict: skip, restrict_chance: 0.5, messages: {stay: "{target}は ぼんやりしている"}}
`})
	st := testgame.State(db, 3)
	b := battle.New(db, st, "one")
	scout, mote := b.Party[0], b.Foes[0]
	scout.States["daze"] = rpg.StateForever
	if r, _ := scout.Restrict(); r != "" {
		t.Fatalf("a state with restrict_chance takes the command away: %q", r)
	}
	lost, acted := 0, 0
	for range 40 {
		mote.HP = mote.MHP()
		scout.HP = scout.MHP()
		steps := run(b, battle.Action{User: scout, Skill: "guard"})
		if strings.Contains(texts(steps), "ぼんやり") {
			lost++
		} else {
			acted++
		}
		if b.Result != battle.Ongoing {
			break
		}
	}
	if lost == 0 || acted == 0 {
		t.Errorf("with restrict_chance 0.5, lost %d and acted %d of the turns", lost, acted)
	}
}

// level_scale: enemies get stronger up to a cap by as much as the party's average level exceeds the expected one (no change at or below it).
func TestLevelScale(t *testing.T) {
	for _, c := range []struct {
		level   int
		mhp, df int
	}{{1, 30, 0}, {2, 33, 0}, {3, 36, 0}, {20, 36, 0}} {
		db := testgame.Load(t, nil)
		db.Troops["one"].LevelScale = &data.LevelScale{Level: 1, Rate: 0.1, Max: 0.2}
		st := testgame.State(db, 1)
		for _, a := range st.Members() {
			a.Level = c.level
		}
		b := battle.New(db, st, "one")
		mote := b.Foes[0]
		if mote.MHP() != c.mhp || mote.HP != c.mhp {
			t.Errorf("Lv%d: the mote has %d/%d HP (want %d)", c.level, mote.HP, mote.MHP(), c.mhp)
		}
		if d := mote.Stat("def") - db.Enemies[mote.EID].Stats["def"]; d != c.df {
			t.Errorf("Lv%d: def grew by %d (only mhp, atk and mat grow by default)", c.level, d)
		}
	}
}

// level_scale unbounded: with no cap, it keeps growing with the party's level (a dedicated enemy that is always above you).
func TestLevelScaleUnbounded(t *testing.T) {
	for _, c := range []struct{ level, mhp int }{{1, 30}, {3, 36}, {11, 60}, {21, 90}} {
		db := testgame.Load(t, nil)
		db.Troops["one"].LevelScale = &data.LevelScale{Level: 1, Rate: 0.1, Unbounded: true}
		st := testgame.State(db, 1)
		for _, a := range st.Members() {
			a.Level = c.level
		}
		if got := battle.New(db, st, "one").Foes[0].MHP(); got != c.mhp {
			t.Errorf("Lv%d: the mote has %d HP (want %d)", c.level, got, c.mhp)
		}
	}
}

// If max is written with unbounded, a cap above 0.5 stops it there.
func TestLevelScaleUnboundedCap(t *testing.T) {
	db := testgame.Load(t, nil)
	db.Troops["one"].LevelScale = &data.LevelScale{Level: 1, Rate: 0.1, Max: 1, Unbounded: true}
	st := testgame.State(db, 1)
	for _, a := range st.Members() {
		a.Level = 31
	}
	if got := battle.New(db, st, "one").Foes[0].MHP(); got != 60 {
		t.Errorf("the mote has %d HP at Lv31 (want 60: the cap +100%%)", got)
	}
}
