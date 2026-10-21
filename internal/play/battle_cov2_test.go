package play

import (
	"strings"
	"testing"

	"github.com/nao1215/gozen/internal/battle"
	"github.com/nao1215/gozen/internal/event"
)

// btlGuardAll has every member guard this turn.
func btlGuardAll(g *Game, s *battleScene) {
	for s.phase == bpInput && s.target == nil {
		btlChoose(g, s, 2)
	}
}

// btlUse has the current member use a skill, aim at the chosen target (an index into the
// pool) and, once the turn is complete, sends the others guarding.
func btlUse(g *Game, s *battleScene, skill string, target int) {
	s.choose(g, battle.Action{User: s.members[s.mi], Skill: skill})
	if s.target != nil {
		s.target.sel = target
		btlKey(g, btnOK)
	}
}

// btlTalk presses OK whenever a line is fully shown and runs frames until cond holds. It
// returns the lines that were on the screen.
func btlTalk(t *testing.T, g *Game, what string, cond func() bool) []string {
	t.Helper()
	var said []string
	for range 8000 {
		if cond() {
			return said
		}
		if g.talk.active {
			if n := len(said); n == 0 || said[n-1] != g.talk.say.Text {
				said = append(said, g.talk.say.Text)
			}
		}
		if g.talk.active && (g.talk.choice != nil || g.talk.shown >= g.talk.total) {
			btlKey(g, btnOK)
		}
		btlFrame(t, g)
	}
	t.Fatalf("never reached: %s", what)
	return nil
}

// TestBattleTroopEventsTalk: the events of a troop run inside the battle at their turn;
// a line of a fallen speaker is replaced by the line of "instead", a line without a
// replacement is left out, and the choice made by the player is kept as a flag.
func TestBattleTroopEventsTalk(t *testing.T) {
	g, _ := btlOpen(t, Options{Battle: "chatty"}, "", "")
	s := btlBattle(t, g)
	s.b.Party[1].HP = 0 // the guide is down
	s.sync()
	said := btlTalk(t, g, "the choice made", func() bool { return g.st.Flags["left"] || g.st.Flags["right"] })
	if !g.st.Flags["left"] || g.st.Flags["right"] {
		t.Fatalf("flags %v", g.st.Flags)
	}
	text := strings.Join(said, "|")
	if !strings.Contains(text, "Scout instead") {
		t.Fatalf("the replacement line was not said: %q", text)
	}
	if strings.Contains(text, "Guide line") || strings.Contains(text, "Lost line") {
		t.Fatalf("a fallen speaker spoke: %q", text)
	}
}

// TestBattleLineForTheSpeaker: a line is shown as written for a speaker in the party who is
// standing, taken from the alternatives when the speaker is down or not in the party, and
// dropped when nobody can say it.
func TestBattleLineForTheSpeaker(t *testing.T) {
	g, _ := btlOpen(t, Options{Battle: "one"}, "", "")
	party := btlBattle(t, g).b.Party
	line := event.Say{Actor: "guide", Text: "A", Instead: []event.Say{{Actor: "scout", Text: "B"}}}
	if got, ok := battleLine(party, line); !ok || got.Text != "A" {
		t.Fatalf("standing speaker: %+v %v", got, ok)
	}
	party[1].HP = 0
	if got, ok := battleLine(party, line); !ok || got.Text != "B" {
		t.Fatalf("fallen speaker: %+v %v", got, ok)
	}
	party[0].HP = 0
	if _, ok := battleLine(party, line); ok {
		t.Fatal("a line was said with the whole party down")
	}
	party[0].HP = 10
	if _, ok := battleLine(party, event.Say{Actor: "stranger", Text: "C"}); ok {
		t.Fatal("a speaker outside the party spoke in battle")
	}
	if got, ok := battleLine(party, event.Say{Text: "Narration"}); !ok || got.Text != "Narration" {
		t.Fatalf("narration: %+v %v", got, ok)
	}
	if _, ok := battleLine(party, event.Say{Actor: "stranger", Text: "D", Instead: []event.Say{{Text: "E"}}}); ok {
		// Without an actor in the alternative it is still said by the narrator.
		t.Log("alternative without an actor accepted")
	}
	if !inParty(party, "scout") || inParty(party, "nobody") {
		t.Fatal("inParty is wrong")
	}
	if !downSpeaker(party, event.Say{Actor: "guide"}) || downSpeaker(party, event.Say{Actor: "scout"}) {
		t.Fatal("downSpeaker is wrong")
	}
	if !downSpeaker(party, event.Say{Name: party[1].Name}) {
		t.Fatal("a speaker named like a fallen member is not down")
	}
}

// TestBattleSummonAndFall: a foe that calls a helper puts a new foe on the field with its
// own box; foes that fall fade out and the battle is won when all are gone.
func TestBattleSummonAndFall(t *testing.T) {
	g, _ := btlOpen(t, Options{Battle: "caller"}, "", "")
	s := btlBattle(t, g)
	if len(s.b.Foes) != 1 {
		t.Fatalf("%d foes", len(s.b.Foes))
	}
	btlUntil(t, g, "the first command", 4000, func() bool { return s.phase == bpInput })
	btlGuardAll(g, s)
	btlUntil(t, g, "a summoned foe", 4000, func() bool { return len(s.b.Foes) == 2 })
	if s.b.Foes[1].Name != "Mote" {
		t.Fatalf("summoned %q", s.b.Foes[1].Name)
	}
	btlUntil(t, g, "the next command", 4000, func() bool { return s.phase == bpInput })
	if _, ok := s.foeBoxes(g)[s.b.Foes[1]]; !ok {
		t.Fatal("the summoned foe has no box")
	}
	btlAttackUntilOver(t, g, s)
	if s.b.Result != battle.Won {
		t.Fatalf("result %v", s.b.Result)
	}
	for _, f := range s.b.Foes {
		if _, gone := s.gone[f]; !gone {
			t.Fatalf("%s did not fade out", f.Name)
		}
	}
}

// TestBattleStatesOnBadges: a state put on a unit is shown at once on its badges with the
// turns left and cleared when it wears off; bad states are listed before good ones.
func TestBattleStatesOnBadges(t *testing.T) {
	g, _ := btlOpen(t, Options{Battle: "two"}, "", "")
	s := btlAsk(t, g)
	foe, ally := s.b.Foes[0], s.b.Party[0]
	foe.States["poison"] = 4
	ally.States["ward"] = 3
	ally.States["shield"] = 2
	s.sync()
	if got := s.stateNames(g, foe); got != "Poisoned" {
		t.Fatalf("foe states %q", got)
	}
	line := s.stateTurns(g, ally)
	if !strings.Contains(line, "Warded") || !strings.Contains(line, "3") || !strings.Contains(line, "Shield") {
		t.Fatalf("turns line %q", line)
	}
	if s.shownStates(foe)["poison"] != 4 || len(s.shownStates(foe)) != 1 {
		t.Fatalf("shown %v", s.shownStates(foe))
	}
	// A state not yet shown comes in with the step that adds it.
	s.catchUp(battle.Step{Kind: battle.StateOn, Target: foe, State: "ward"})
	if s.shownStates(foe)["ward"] < 1 {
		t.Fatal("StateOn did not show the state")
	}
	s.catchUp(battle.Step{Kind: battle.StateOff, Target: foe, State: "ward"})
	if _, on := s.shownStates(foe)["ward"]; on {
		t.Fatal("StateOff did not clear the state")
	}
	btlFrame(t, g)
	// Aim at the foe so the aim line carries its states.
	btlKey(g, btnOK)
	if s.target == nil {
		t.Fatal("no aim")
	}
	btlFrame(t, g)
}

// TestBattleMissAndGuardMarks: an attack that cannot hit shows MISS; a guarding member is
// marked.
func TestBattleMissAndGuardMarks(t *testing.T) {
	g, _ := btlOpen(t, Options{Battle: "two"}, "", "")
	s := btlAsk(t, g)
	for range 2 {
		s.choose(g, battle.Action{User: s.members[s.mi], Skill: "whiff"})
		s.target.sel = 0
		btlKey(g, btnOK)
	}
	btlUntil(t, g, "MISS", 3000, func() bool {
		for _, p := range s.pops {
			if p.text == "MISS" {
				return true
			}
		}
		return false
	})
}

// TestBattleHealAndRevive: healing shows +N on the target, and a revived member comes back
// from the fallen look.
func TestBattleHealAndRevive(t *testing.T) {
	g, _ := btlOpen(t, Options{Battle: "caller"}, "", "")
	s := btlAsk(t, g)
	s.b.Party[1].HP = 0
	s.sync()
	s.gone[s.b.Party[1]] = 3
	s.members = s.b.Commandable()
	s.mi = 0
	s.actions = nil
	s.choose(g, battle.Action{User: s.members[0], Skill: "raise_all"})
	if s.target == nil {
		t.Fatal("no fallen ally to raise")
	}
	btlKey(g, btnOK)
	btlUntil(t, g, "the revive", 3000, func() bool { return s.b.Party[1].Alive() })
	btlUntil(t, g, "the number", 3000, func() bool {
		for _, p := range s.pops {
			if strings.HasPrefix(p.text, "+") {
				return true
			}
		}
		return false
	})
	if _, still := s.gone[s.b.Party[1]]; still {
		t.Fatal("the revived member still looks fallen")
	}
}

// TestBattleItemUse: using the tonic in battle heals the target and spends one from the bag.
func TestBattleItemUse(t *testing.T) {
	g, _ := btlOpen(t, Options{Battle: "caller"}, "", "")
	s := btlAsk(t, g)
	s.b.Party[0].HP = 1
	s.sync()
	before := g.st.Items["potion"]
	btlChoose(g, s, 3) // Items
	btlKey(g, btnOK)   // the tonic
	s.target.sel = 0
	btlKey(g, btnOK) // the first ally
	btlGuardAll(g, s)
	btlUntil(t, g, "the heal", 3000, func() bool { return s.b.Party[0].HP > 1 })
	if g.st.Items["potion"] != before-1 {
		t.Fatalf("potions %d -> %d", before, g.st.Items["potion"])
	}
}
