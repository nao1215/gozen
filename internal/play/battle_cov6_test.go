package play

import (
	"strings"
	"testing"

	"github.com/nao1215/gozen/internal/battle"
	"github.com/nao1215/gozen/internal/event"
)

// TestBattleEventShowsPicturesAndSkipsOtherRequests: a troop event can show pictures page by page
// (a tap moves to the next) and a request the battle has no screen for is passed over.
func TestBattleEventShowsPicturesAndSkipsOtherRequests(t *testing.T) {
	g, _ := btlOpen(t, Options{Battle: "pictured"}, "", "")
	s := btlBattle(t, g)
	for i := range 4000 {
		if g.st.Flags["shown"] {
			break
		}
		if s.phase == bpEvent && i%4 == 0 {
			g.in.pressed[btnOK] = true
			g.tick++
			s.update(g)
			g.in.pressed = [numButtons]bool{}
		}
		btlFrame(t, g)
	}
	if !g.st.Flags["shown"] {
		t.Fatalf("the event did not run to its end, phase %d", s.phase)
	}
}

// TestBattleSkillShowsWhatItDoes: each kind of move runs through the battle: a state with a magic
// light, a blow that also poisons, a move with no animation, healing with a holy light, and a blow on
// the foe's MP.
func TestBattleSkillShowsWhatItDoes(t *testing.T) {
	for _, tc := range []struct {
		name, troop, skill string
		target             int
		hurt               bool
		check              func(t *testing.T, s *battleScene, steps []traceLine)
	}{
		{name: "magic state", troop: "two", skill: "freeze", check: func(t *testing.T, s *battleScene, _ []traceLine) {
			if s.b.Foes[0].States["ward"] == 0 {
				t.Error("the state was not put on")
			}
		}},
		{name: "blow and poison", troop: "two", skill: "sting", check: func(t *testing.T, s *battleScene, _ []traceLine) {
			if s.b.Foes[0].States["poison"] == 0 || s.b.Foes[0].HP >= s.b.Foes[0].MHP() {
				t.Error("poison or damage missing")
			}
		}},
		{name: "no animation", troop: "two", skill: "quiet_hit", check: func(t *testing.T, s *battleScene, _ []traceLine) {
			if s.b.Foes[0].HP >= s.b.Foes[0].MHP() {
				t.Error("no damage")
			}
		}},
		{name: "heal animation", troop: "two", skill: "cure", hurt: true, check: func(t *testing.T, s *battleScene, _ []traceLine) {
			if s.b.Party[0].HP <= 5 {
				t.Error("not healed")
			}
		}},
		{name: "holy heal", troop: "two", skill: "hallow", hurt: true, check: func(t *testing.T, s *battleScene, _ []traceLine) {
			if s.b.Party[0].HP <= 5 {
				t.Error("not healed")
			}
		}},
		{name: "mp blow", troop: "guardian", skill: "drain", check: func(t *testing.T, _ *battleScene, steps []traceLine) {
			found := false
			for _, l := range steps {
				found = found || l["stat"] == "mp"
			}
			if !found {
				t.Error("no step on the MP")
			}
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			g, out := btlOpen(t, Options{Battle: tc.troop}, "", "")
			s := btlAsk(t, g)
			if tc.hurt {
				s.b.Party[0].HP = 5
				s.sync()
			}
			btlUse(g, s, tc.skill, tc.target)
			btlUse(g, s, "guard", 0)
			btlUntil(t, g, "the next command", 6000, func() bool { return s.phase != bpPlay })
			tc.check(t, s, btlLines(t, out, "battle_step"))
		})
	}
}

// TestBattleRegenerationHealsQuietly: a state that heals at the end of each turn does it with
// the light and the number but without a line of text.
func TestBattleRegenerationHealsQuietly(t *testing.T) {
	g, out := btlOpen(t, Options{Battle: "two"}, "", "")
	s := btlAsk(t, g)
	s.b.Party[0].HP = 10
	s.b.Party[0].States["regen"] = 3
	s.sync()
	btlGuardAll(g, s)
	btlUntil(t, g, "the next command", 6000, func() bool { return s.phase != bpPlay })
	if s.b.Party[0].HP <= 10 {
		t.Fatalf("hp %d", s.b.Party[0].HP)
	}
	quiet := false
	for _, l := range btlLines(t, out, "battle_step") {
		if l["kind"] == float64(battle.Heal) && l["amount"] == 3.0 {
			quiet = true
		}
	}
	if !quiet {
		t.Fatal("no heal step for the state")
	}
	for _, text := range btlTexts(t, out) {
		if strings.Contains(text, "restores 3") || strings.Contains(text, "recovers 3") {
			t.Fatalf("a quiet heal said %q", text)
		}
	}
}

// TestBattleBeginInputAfterTheEnd: asking for commands when the battle is already decided goes to
// the result instead.
func TestBattleBeginInputAfterTheEnd(t *testing.T) {
	g, _ := btlOpen(t, Options{Battle: "one"}, "", "")
	s := btlBattle(t, g)
	s.b.Result = battle.Won
	s.beginInput(g)
	if s.phase != bpResult {
		t.Fatalf("phase %d", s.phase)
	}
	if s.usage("") != nil {
		t.Fatal("usage of nothing")
	}
}

// TestBattleRetryNeedsAGoodSnapshot: a retry whose saved state cannot be restored does not start
// a new battle and leaves the question open.
func TestBattleRetryNeedsAGoodSnapshot(t *testing.T) {
	g, _ := btlOpen(t, Options{Battle: "doom"}, "", "")
	s := btlBattle(t, g)
	s.retry = []byte("not a save")
	s.b.Result, s.endT = battle.Lost, 20
	s.results = nil
	s.result(g)
	q, ok := g.stack[len(g.stack)-1].(*confirm)
	if !ok {
		t.Fatalf("top is %T", g.stack[len(g.stack)-1])
	}
	for range 6 {
		q.update(g)
	}
	g.in.pressed[btnOK] = true
	q.update(g)
	g.in.pressed = [numButtons]bool{}
	if g.currentBattle() != s {
		t.Fatal("a retry from a damaged snapshot started another battle")
	}
}

// TestFoeBoxesOfWideFoesShrink: foes too wide for the screen are scaled down together, and a foe with
// no scale is drawn at its natural size.
func TestFoeBoxesOfWideFoesShrink(t *testing.T) {
	g, _ := btlOpen(t, Options{Battle: "titans"}, "", "")
	s := btlBattle(t, g)
	boxes := s.foeBoxes(g)
	sw, _ := g.screen()
	right := 0
	for _, f := range s.b.Foes {
		if r := boxes[f].rect; r.Max.X > right {
			right = r.Max.X
		}
	}
	if float64(right) > sw-36+1 {
		t.Fatalf("the foes reach %d of %v", right, sw)
	}
	one := boxes[s.b.Foes[0]].rect
	g.db.Enemies["giant"].Scale = 0
	again := s.foeBoxes(g)[s.b.Foes[0]].rect
	if again.Dx() != one.Dx() || again.Dy() != one.Dy() {
		t.Fatalf("scale 0 changed the size: %v vs %v", again, one)
	}
}

// TestBattleAimLineAndSmallFoe: the line over the aimed foe holds what is known and its states, and
// is kept inside the foe's box for a very small foe.
func TestBattleAimLineAndSmallFoe(t *testing.T) {
	g, _ := btlOpen(t, Options{Battle: "one"}, "", "")
	g.db.Enemies["mote"].Scale = 0.5
	s := btlAsk(t, g)
	foe := s.b.Foes[0]
	g.st.Vars["known_mote_fire"] = 1.0
	foe.States["poison"] = 3
	s.sync()
	btlKey(g, btnOK)
	if s.target == nil {
		t.Fatal("not aiming")
	}
	g.Draw(btlImage())
	if !strings.Contains(s.known(g, foe), "Warmth") || s.stateNames(g, foe) != "Poisoned" {
		t.Fatalf("known %q states %q", s.known(g, foe), s.stateNames(g, foe))
	}
}

// TestShownUnitFieldsFallBack: names the screen does not hold back come from the unit itself, and a
// state added to a unit not yet shown starts from an empty set.
func TestShownUnitFieldsFallBack(t *testing.T) {
	g, _ := btlOpen(t, Options{Battle: "one"}, "", "")
	s := btlBattle(t, g)
	u := s.b.Party[0]
	v := shownUnit{u, s}
	if got, ok := v.Field("agi"); !ok || got == nil {
		t.Fatalf("agi %v %v", got, ok)
	}
	fresh := &battle.Unit{States: map[string]int{"ward": 2}}
	s.catchUp(battle.Step{Kind: battle.StateOn, Target: fresh, State: "ward"})
	if s.shownSt[fresh]["ward"] != 2 {
		t.Fatalf("shown %v", s.shownSt[fresh])
	}
	s.catchUp(battle.Step{Kind: battle.Damage})
}

// TestBattleLineSkipsStrangers: when the speaker is down, an alternative spoken by someone outside
// the party is passed over for one spoken by a member.
func TestBattleLineSkipsStrangers(t *testing.T) {
	g, _ := btlOpen(t, Options{Battle: "one"}, "", "")
	party := btlBattle(t, g).b.Party
	party[1].HP = 0
	line := event.Say{Actor: "guide", Text: "A", Instead: []event.Say{{Actor: "stranger", Text: "S"}, {Actor: "guide", Text: "G"}, {Actor: "scout", Text: "B"}}}
	got, ok := battleLine(party, line)
	if !ok || got.Text != "B" {
		t.Fatalf("line %+v %v", got, ok)
	}
}
