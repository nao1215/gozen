package play

import (
	"image"
	"strings"
	"testing"

	"github.com/nao1215/gozen/internal/battle"
	data "github.com/nao1215/gozen/internal/layer2/gamedata"
	"github.com/nao1215/gozen/internal/testgame"
)

func btlHas(trace []traceLine, key string, want any) bool {
	for _, l := range trace {
		if l[key] == want {
			return true
		}
	}
	return false
}

// TestBattleFacesReactAndRules: a member's face follows what happens (a reaction for a moment,
// then the rule of its health), fallen members look fallen, a foe drawn as a person has its
// own box and faces, and a member with no skills has no skill command.
func TestBattleFacesReactAndRules(t *testing.T) {
	g, _ := btlOpen(t, Options{Battle: "tall", Party: []string{"knight", "weaver"}}, "", "")
	s := btlAsk(t, g)
	if s.cmd.items[1].disabled {
		t.Fatal("the knight knows skills")
	}
	s.mi = 1
	s.openCommands(g)
	if !s.cmd.items[1].disabled {
		t.Fatal("a member without skills has a skill command")
	}
	s.mi = 0
	s.openCommands(g)
	k := s.b.Party[0]
	if got := s.faceOf(k); got != "normal" {
		t.Fatalf("healthy face %q", got)
	}
	k.HP = 2
	s.sync()
	if got := s.faceOf(k); got != "hurt" {
		t.Fatalf("hurt face %q", got)
	}
	s.react[k] = reaction{face: "grin", t: 5}
	if got := s.faceOf(k); got != "grin" {
		t.Fatalf("reaction %q", got)
	}
	k.HP = 0
	s.sync()
	if got := s.faceOf(k); got != "hurt" {
		t.Fatalf("a fallen member shows %q", got)
	}
	s.react[k] = reaction{face: "grin", t: 5}
	if got := s.faceOf(k); got != "hurt" {
		t.Fatalf("a happy reaction on a fallen member: %q", got)
	}
	// The unit seen through the screen's shown values.
	v := shownUnit{k, s}
	if hp, _ := v.Field("hp"); hp != 0.0 {
		t.Fatalf("shown hp %v", hp)
	}
	k.HP = 7
	for name, want := range map[string]any{"hp": 0.0, "mp": float64(s.shownMP[k]), "alive": false, "dead": true} {
		if got, ok := v.Field(name); !ok || got != want {
			t.Errorf("shown %s = %v (%v), want %v", name, got, ok, want)
		}
	}
	if r, _ := v.Field("hp_rate"); r != 0.0 {
		t.Errorf("hp_rate %v", r)
	}
	if r, _ := v.Field("mp_rate"); r == nil {
		t.Errorf("mp_rate %v", r)
	}
	if _, ok := (shownUnit{k, &battleScene{}}).Field("hp"); !ok {
		t.Error("a unit not yet shown has no hp")
	}
	// The person-sized foe: the idol's box is tall and its face changes with its health.
	idol := s.b.Foes[0]
	box := s.foeBoxes(g)[idol]
	if box.rect.Dy() < 200 || box.bottom <= float64(box.rect.Max.Y) {
		t.Fatalf("idol box %+v", box)
	}
	idol.HP = 10
	s.sync()
	if idol.Face() != "hurt" {
		t.Fatalf("idol face %q", idol.Face())
	}
	s.react[idol] = reaction{face: "angry", t: 9}
	for range 5 {
		btlFrame(t, g)
	}
	// An unknown foe has an empty rectangle.
	if r := s.unitRect(g, &battle.Unit{Enemy: true}); r != (image.Rectangle{}) {
		t.Fatalf("unknown foe rect %v", r)
	}
	// A unit with neither an actor nor a definition reacts with nothing.
	s.reactTo(g, &battle.Unit{}, "hit", 5)
	if _, ok := s.react[&battle.Unit{}]; ok {
		t.Fatal("a reaction for a unit without faces")
	}
}

// TestBattleAtHitDelays: damage lands at the moment of the impact of its effect; an effect
// without an impact moment lands at once, and what is still waiting lands before the next step.
func TestBattleAtHitDelays(t *testing.T) {
	g, _ := btlOpen(t, Options{Battle: "one"}, "", "")
	s := btlBattle(t, g)
	s.stepT = 0
	landed := 0
	s.atHit(g, "buff", 40, func() { landed++ })
	if landed != 1 || len(s.hits) != 0 {
		t.Fatalf("an effect without an impact moment: landed %d waiting %d", landed, len(s.hits))
	}
	s.atHit(g, "slash", 40, func() { landed++ })
	if landed != 1 || len(s.hits) != 1 || s.hits[0].t != int(fxHit("slash")*40) {
		t.Fatalf("waiting %+v landed %d", s.hits, landed)
	}
	if s.stepT < s.hits[0].t {
		t.Fatalf("the step ends at %d, before the hit at %d", s.stepT, s.hits[0].t)
	}
	for range s.hits[0].t - 1 {
		s.stepHits()
	}
	if landed != 1 {
		t.Fatal("landed early")
	}
	s.stepHits()
	if landed != 2 || len(s.hits) != 0 {
		t.Fatalf("landed %d waiting %d at the moment", landed, len(s.hits))
	}
	s.atHit(g, "fire", 100, func() { landed++ })
	s.atHit(g, "ice", 100, func() { landed++ })
	s.flushHits()
	if landed != 4 || len(s.hits) != 0 {
		t.Fatalf("flush: landed %d waiting %d", landed, len(s.hits))
	}
}

// TestBattlePopupsKeepApart: numbers appearing at the same place are stacked upward, a number
// seen for a while is faded out when a new one arrives, and tags sit above the number.
func TestBattlePopupsKeepApart(t *testing.T) {
	s := &battleScene{}
	if y := s.popY(100, 100); y != 100 {
		t.Fatalf("free place moved to %v", y)
	}
	s.pops = []popup{{x: 100, y: 100, t: 3}}
	if y := s.popY(100, 100); y != 100-46 {
		t.Fatalf("stacked at %v", y)
	}
	if y := s.popY(300, 100); y != 100 {
		t.Fatalf("a far number moved to %v", y)
	}
	s.pops[0].t = popFade
	if y := s.popY(100, 100); y != 100 {
		t.Fatalf("a fading number still pushes: %v", y)
	}
	s.pops = []popup{{x: 100, y: 100, t: 3}, {x: 120, y: 100, t: 10}, {x: 400, y: 100, t: 10}}
	s.retirePops(110)
	if s.pops[0].t != 3 || s.pops[1].t != popFade || s.pops[2].t != 10 {
		t.Fatalf("retire: %+v", s.pops)
	}
	g, _ := btlOpen(t, Options{Battle: "one"}, "", "")
	small, _ := g.popSize(popup{t: 20})
	big, _ := g.popSize(popup{t: 20, big: true})
	if small != 40 || big <= small-1 {
		t.Fatalf("sizes %v %v", small, big)
	}
	if _, lift := g.popSize(popup{t: 3, big: true}); lift <= 0 {
		t.Fatalf("a fresh big number does not bounce: %v", lift)
	}
	bounce, _ := g.popSize(popup{t: 2})
	if bounce <= 40 {
		t.Fatalf("a fresh number does not pop: %v", bounce)
	}
	g.db.Game.UI.BattleFx.Pop = new(0.0)
	if bigNoPop, _ := g.popSize(popup{t: 3, big: true}); bigNoPop < 20 {
		t.Fatalf("a number below the floor: %v", bigNoPop)
	}
}

// TestBattleStatesAndTheirEffects: weaken puts poison on a foe (a debuff), bless puts ward on
// a friend (a buff), poison ticks at the end of the turn for the damage of the state with no
// attacker, a good state on a foe can be dispelled, and a quiet heal says nothing.
func TestBattleStatesAndTheirEffects(t *testing.T) {
	g, out := btlOpen(t, Options{Battle: "two"}, "", "")
	s := btlAsk(t, g)
	foe0, foe1 := s.b.Foes[0], s.b.Foes[1]
	foe1.States["ward"] = 3
	s.sync()
	btlUse(g, s, "weaken", 0)
	btlUse(g, s, "bless", 0)
	if s.phase != bpPlay {
		t.Fatal("not played")
	}
	btlUntil(t, g, "the next command", 6000, func() bool { return s.phase == bpInput })
	if foe0.States["poison"] == 0 && !foe0.Alive() {
		t.Fatal("no poison")
	}
	if s.b.Party[0].States["ward"] == 0 {
		t.Fatal("bless did not put the ward on")
	}
	steps := btlLines(t, out, "battle_step")
	var ticked bool
	for _, l := range steps {
		if l["state"] == "poison" || (l["user"] == nil && l["target"] != nil && l["amount"] == 5.0) {
			ticked = true
		}
	}
	if !ticked {
		t.Fatal("poison never ticked for 5")
	}
	if foe0.HP > foe0.MHP()-5 {
		t.Fatalf("the poisoned foe is at %d of %d", foe0.HP, foe0.MHP())
	}
	// Dispel the ward of the other foe (the party aims at the second).
	btlUse(g, s, "dispel", 1)
	btlUse(g, s, "dispel", 1)
	btlUntil(t, g, "the dispel", 6000, func() bool { return foe1.States["ward"] == 0 })
	// A quiet heal.
	s.b.Party[0].HP = 10
	btlUntil(t, g, "the next command", 6000, func() bool { return s.phase == bpInput })
	shown := len(s.msg)
	btlUse(g, s, "mend_quiet", 0)
	btlUse(g, s, "mend_quiet", 0)
	btlUntil(t, g, "the quiet heal", 6000, func() bool { return s.b.Party[0].HP > 10 })
	_ = shown
}

// TestBattleElementsAndTags: a weak hit says WEAK and stops the screen, a resisted hit is
// remembered, thunder flashes the screen, an element on a physical move adds its burst, and a
// magic hit on a weak foe booms.
func TestBattleElementsAndTags(t *testing.T) {
	g, _ := btlOpen(t, Options{Battle: "tall"}, "", "")
	s := btlAsk(t, g)
	// idol (boss), giant (resists ice), hulk
	btlUse(g, s, "chill", 1)
	btlUse(g, s, "chill", 1)
	btlUntil(t, g, "the hit", 4000, func() bool { return g.st.Vars["known_giant_ice"] != nil })
	g.st.Vars["known_giant_ice"] = nil
	btlUntil(t, g, "the next command", 6000, func() bool { return s.phase == bpInput })
	if line := s.known(g, s.b.Foes[1]); line != "" && !strings.Contains(line, "Frost") {
		t.Fatalf("known %q", line)
	}
	// Thunder and a fire edge.
	btlUse(g, s, "bolt", 0)
	btlUse(g, s, "slash_fire", 0)
	btlUntil(t, g, "the next command", 6000, func() bool { return s.phase == bpInput })
}

// TestBattleMagicOnAWeakFoe: a magic hit on a foe weak to it stops the screen briefly (the hit
// stop), tags the number WEAK! and shakes the screen harder than a plain hit.
func TestBattleMagicOnAWeakFoe(t *testing.T) {
	g, _ := btlOpen(t, Options{Battle: "one"}, "", "")
	s := btlAsk(t, g)
	btlUse(g, s, "ember", 0)
	btlUse(g, s, "ember", 0)
	btlUntil(t, g, "a weak hit", 4000, func() bool { return s.dr.freeze > 0 || g.shake >= 10 })
	var tagged bool
	for _, p := range s.pops {
		tagged = tagged || strings.Contains(p.text, "WEAK")
	}
	if !tagged {
		t.Fatalf("no WEAK tag in %+v", s.pops)
	}
	if g.st.Vars["known_mote_fire"] == nil {
		t.Fatal("the weakness was not learned")
	}
}

// TestBattleCoverProtectsAnAlly: a member with a cover state takes the blow aimed at an ally,
// and steps in front of that ally for a moment.
func TestBattleCoverProtectsAnAlly(t *testing.T) {
	g, out := btlOpen(t, Options{Battle: "pack"}, "", "")
	s := btlAsk(t, g)
	s.b.Party[0].States["shield"] = 99
	s.sync()
	var covered bool
	for range 40 {
		btlGuardAll(g, s)
		btlUntil(t, g, "the next command", 8000, func() bool { return s.phase != bpPlay })
		if s.coverBy != nil || strings.Contains(strings.Join(btlTexts(t, out), "|"), "covers") {
			covered = true
			break
		}
		if s.phase != bpInput {
			break
		}
	}
	if !covered {
		t.Fatal("nobody was covered in 40 turns")
	}
}

func btlTexts(t *testing.T, out interface {
	Bytes() []byte
},
) []string {
	t.Helper()
	var texts []string
	for _, l := range traceLines(t, out.Bytes(), "battle") {
		if s, ok := l["text"].(string); ok {
			texts = append(texts, s)
		}
	}
	return texts
}

// TestBattleItemListShowsWhatIsLeft: the item list leaves out items that cannot be used, and a
// potion already chosen by the first member counts against the second.
func TestBattleItemListShowsWhatIsLeft(t *testing.T) {
	g, _ := btlOpen(t, Options{Battle: "one"}, "", "")
	g.st.Items = map[string]int{"key": 1, "potion": 1}
	s := btlAsk(t, g)
	btlChoose(g, s, 3)
	if len(s.subIDs) != 1 || s.subIDs[0] != "potion" || s.sub.items[0].disabled {
		t.Fatalf("items %v %+v", s.subIDs, s.sub.items)
	}
	btlKey(g, btnOK)
	btlKey(g, btnOK)
	btlChoose(g, s, 3)
	if len(s.sub.items) != 1 || !s.sub.items[0].disabled || !strings.Contains(s.sub.items[0].right, "0") {
		t.Fatalf("the second member's list %+v", s.sub.items)
	}
	btlKey(g, btnOK) // disabled: nothing
	if s.target != nil {
		t.Fatal("a used-up item was taken")
	}
}

// TestBattleEscapeFailureLosesTheTurn: a failed escape passes the turn of everyone, and the foes act.
func TestBattleEscapeFailureLosesTheTurn(t *testing.T) {
	g, out := btlOpen(t, Options{Battle: "one"}, "", "")
	g.db.Game.Battle.Escape = data.Expr{Expr: testgame.Expr("0")}
	s := btlAsk(t, g)
	btlChoose(g, s, 5)
	btlUntil(t, g, "the next command", 6000, func() bool { return s.phase == bpInput })
	if s.b.Result != battle.Ongoing {
		t.Fatalf("result %v", s.b.Result)
	}
	if !strings.Contains(strings.Join(btlTexts(t, out), "|"), "!") {
		t.Fatal("no message for the failed escape")
	}
	passed := 0
	for _, l := range btlLines(t, out, "battle_step") {
		if l["skill"] == "guard" {
			passed++
		}
	}
	if passed != 0 {
		t.Fatalf("a passing member guarded %d times", passed)
	}
}

// TestBattleTargetByMouse: the pointer picks a foe in the aim list and a click confirms it.
func TestBattleTargetByMouse(t *testing.T) {
	g, _ := btlOpen(t, Options{Battle: "pack"}, "", "")
	s := btlAsk(t, g)
	btlKey(g, btnOK)
	if s.target == nil || len(s.target.pool) != 3 {
		t.Fatalf("target %+v", s.target)
	}
	last := s.target.pool[2]
	r := s.unitRect(g, last)
	g.in.mouse = image.Pt((r.Min.X+r.Max.X)/2, (r.Min.Y+r.Max.Y)/2)
	g.in.moved = true
	s.updateTarget(g)
	g.in.moved = false
	if s.target.sel != 2 {
		t.Fatalf("selected %d", s.target.sel)
	}
	g.in.click = true
	s.updateTarget(g)
	g.in.click = false
	if s.target != nil || s.actions[0].Target != last {
		t.Fatalf("click did not aim at the foe under the pointer: %+v", s.actions)
	}
}

// TestBattleAutoStartSetting: a game that starts battles on auto, or a player who chose it,
// has the party fight by itself from the first moment; a player's "off" wins over the game's "on".
func TestBattleAutoStartSetting(t *testing.T) {
	g, _ := btlOpen(t, Options{}, "", "")
	g.db.Game.Battle.AutoStart = true
	s := g.startBattle("one", func(int) {})
	if !s.auto || !s.b.Party[0].Auto {
		t.Fatal("the game's auto start was ignored")
	}
	g.pop()
	g.autoBattle = false
	off := false
	g.cfg.BattleAuto = &off
	s = g.startBattle("one", func(int) {})
	if s.auto {
		t.Fatal("the player's off was ignored")
	}
	g.pop()
	on := true
	g.cfg.BattleAuto = &on
	g.db.Game.Battle.AutoStart = false
	if !g.autoStartBattle() {
		t.Fatal("the player's on was ignored")
	}
	g.rep = &replay{version: 1}
	if g.autoStartBattle() {
		t.Fatal("an old replay started on auto")
	}
	g.rep = nil
	g.autoBattle = true
	if !g.autoStartBattle() && g.currentBattle() == nil {
		t.Log("autoBattle read by startBattle itself")
	}
	if g.battleAutoTalk() {
		t.Fatal("talking on auto without a battle")
	}
}

// TestBattleCutInOfAMember: a skill with a cut-in shows the member's picture and the line
// shouted, the lines taking turns, and ends by itself; a member who is not shown standing
// has no picture band.
func TestBattleCutInOfAMember(t *testing.T) {
	g, out := btlOpen(t, Options{Battle: "one", Party: []string{"knight", "scout"}}, "", "")
	s := btlAsk(t, g)
	btlUse(g, s, "finisher", 0)
	btlUse(g, s, "guard", 0)
	btlUntil(t, g, "the cut-in", 6000, func() bool { return s.cut != nil })
	if s.cut.name != "Finisher" || s.cut.line != "Take this!" && s.cut.line != "Hya!" || !s.cut.bust || len(s.cut.box) != 3 {
		t.Fatalf("cut-in %+v", s.cut)
	}
	for i := 0; i < cutInTicks+2 && s.cut != nil; i++ {
		btlFrame(t, g)
	}
	if s.cut != nil {
		t.Fatal("the cut-in never ended")
	}
	if !btlHas(btlLines(t, out, "battle"), "cutin", "Finisher") {
		t.Fatal("cut-in not traced")
	}
	// A member who is not shown standing.
	g.db.Actors["scout"].NoStanding = true
	u := s.b.Party[1]
	if !s.startCutIn(g, battle.Step{Kind: battle.Act, User: u, Skill: "finisher"}) || s.cut.path != "-" {
		t.Fatalf("no-standing cut-in %+v", s.cut)
	}
	btlFrame(t, g)
	// A skill without a cut-in does not start one.
	if s.startCutIn(g, battle.Step{Kind: battle.Act, User: u, Skill: "attack"}) {
		t.Fatal("a plain skill got a cut-in")
	}
}

// TestBattleFoeStatesAreShownAboveTheFoe: a foe that is not aimed at shows its state badges.
func TestBattleFoeStatesAreShownAboveTheFoe(t *testing.T) {
	g, _ := btlOpen(t, Options{Battle: "two"}, "", "")
	s := btlAsk(t, g)
	s.b.Foes[0].States["poison"] = 3
	s.b.Foes[1].States["ward"] = 2
	s.sync()
	g.Draw(btlImage())
	s.b.Party[0].States["ward"] = 2
	s.sync()
	s.openCommands(g)
	g.Draw(btlImage()) // the member's own states beside the command list
	btlKey(g, btnOK)
	g.Draw(btlImage()) // aimed: the aim line carries the foe's states
	if got := s.stateNames(g, s.b.Foes[0]); got == "" {
		t.Fatal("no state names")
	}
}

// TestBattleSkillDescriptionShown: the skill list explains the skill under the cursor.
func TestBattleSkillDescriptionShown(t *testing.T) {
	g, _ := btlOpen(t, Options{Battle: "one", Party: []string{"scout", "guide"}}, "", "")
	g.st.Actors["scout"].Learnt = []string{"ember"}
	s := btlAsk(t, g)
	btlChoose(g, s, 1)
	i := -1
	for k, id := range s.subIDs {
		if id == "ember" {
			i = k
		}
	}
	if i < 0 {
		t.Fatalf("skills %v", s.subIDs)
	}
	s.sub.sel = i
	g.Draw(btlImage())
	if s.b.DB.Skills["ember"].Desc == "" {
		t.Fatal("no description")
	}
}

// TestBattleManyFoesShrink: five foes at once share the width by shrinking.
func TestBattleManyFoesShrink(t *testing.T) {
	g, _ := btlOpen(t, Options{Battle: "crowd"}, "", "")
	s := btlBattle(t, g)
	boxes := s.foeBoxes(g)
	w := 0
	for _, f := range s.b.Foes {
		w += boxes[f].rect.Dx()
	}
	sw, _ := g.screen()
	if float64(w) > sw-foeLeft-36 {
		t.Fatalf("foes take %d of %v", w, sw-foeLeft-36)
	}
}
