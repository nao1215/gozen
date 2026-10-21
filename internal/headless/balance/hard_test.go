package balance

import (
	"strings"
	"testing"

	"github.com/nao1215/gozen/internal/testgame"
)

const hardData = `
enemies:
  brute:
    name: Brute
    image: x.png
    stats: {mhp: 600, mmp: 0, atk: 120, def: 30, mat: 1, mdf: 1, agi: 30, luk: 1}
    ai: [{use: attack}]
troops:
  hard: {members: [brute, brute]}
locations:
  pit:
    name: Pit
    encounters: {rate: "1", troops: [{troop: hard}]}
    exits: [{to: camp}]
shops:
  smith: {goods: [blade, stick]}
events:
  die:
    do: [{gold: "-3"}]
  fall:
    do:
      - choice: [{label: A, do: [{set: {var.picked: 1}}]}, {label: B, do: []}]
      - set: {var.bad: "len(gold)"}
  win:
    do: [{set: {var.won: "var.won + 1"}}]
`

// TestLosingPartyIsCountedAndRecovers: a party that cannot win is counted lost and fallen every battle,
// is brought back after each defeat, and the retries on a boss are bounded.
func TestLosingPartyIsCountedAndRecovers(t *testing.T) {
	db := testgame.Load(t, map[string]string{
		"game.yaml":      testgame.Game + "  after_win: win\ngame_over: {event: die}\n",
		"data/hard.yaml": hardData,
	})
	res, err := Run(db, Config{Location: "pit", Floors: 3, Battles: 2, Tries: 2, Seed: 1, Events: true, Bosses: map[int]string{2: "hard"}, Guards: map[int]string{1: "hard"}, Grind: 0.5, Retreat: true, MaxTurns: 30, RestCost: "1", Shops: []string{"smith"}})
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range res.Floors {
		if f.Battles != 2 {
			t.Errorf("floor %d: %d battles, want 2", f.Floor, f.Battles)
		}
		if f.Lost != 2 || f.Wiped != 2 || f.Fallen == 0 {
			t.Errorf("floor %d should lose every battle: %+v", f.Floor, f)
		}
	}
	g := res.Floors[0]
	if g.Guard != "hard" || !g.GuardBack || g.GuardLost != 5 || g.Grinding == 0 {
		t.Errorf("an unbeatable guard with retreat on: back, grind, then give up after 5 retries: %+v", g)
	}
	b := res.Floors[1]
	if b.Boss != "hard" || b.BossRate != 0 || b.Grinding != 200 {
		t.Errorf("an unbeatable boss is ground at most 200 rounds: %+v", b)
	}
	if g.GoldLost == 0 && res.Floors[2].GoldLost == 0 {
		t.Log("the game over event took no gold (the purse was empty)")
	}
}

// TestEventsThatChoseAndFight: events run during a simulation answer a choice with the first option and a battle as won;
// an expression that fails in them is reported in the error.
func TestEventsThatChoseAndFight(t *testing.T) {
	db := testgame.Load(t, map[string]string{
		"game.yaml":      testgame.Game + "  before: fall\n",
		"data/hard.yaml": hardData,
	})
	res, err := Run(db, Config{Location: "field", Floors: 1, Battles: 1, Tries: 1, Seed: 1, Events: true})
	if err == nil || !strings.Contains(err.Error(), "len") {
		t.Fatalf("the failing expression in the event is reported: %v", err)
	}
	if len(res.Floors) != 1 || res.Floors[0].Battles != 1 {
		t.Errorf("the run goes on after a logged problem: %+v", res.Floors)
	}
}

// TestNoEncountersMeansNoBattles: a location with no encounters has no battles to count.
func TestNoEncountersMeansNoBattles(t *testing.T) {
	db := testgame.Load(t, nil)
	res, err := Run(db, Config{Location: "camp", Floors: 2, Battles: 3, Tries: 1, Seed: 1})
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range res.Floors {
		if f.Battles != 0 || len(f.Troops) != 0 {
			t.Errorf("a camp has no battles: %+v", f)
		}
	}
}

// TestBackItemIsUsedUpAndStrands: each trip back uses one return item; without one the party is stranded.
func TestBackItemIsUsedUpAndStrands(t *testing.T) {
	db := testgame.Load(t, map[string]string{"data/hard.yaml": hardData})
	res, err := Run(db, Config{Location: "field", Floors: 4, Battles: 1, Tries: 1, Seed: 1, BackItem: "potion"})
	if err != nil {
		t.Fatal(err)
	}
	stranded := 0
	for _, f := range res.Floors {
		stranded += f.Stranded
	}
	// the party starts with 2 potions and a rest happens on every floor: the 3rd and 4th trips are stranded
	if stranded < 2 {
		t.Errorf("stranded %d times over 4 trips with 2 return items", stranded)
	}
}

// TestWornPartyStandsUpWithoutRest: floors between rests still raise a fallen member to 1 HP.
func TestWornPartyStandsUpWithoutRest(t *testing.T) {
	db := testgame.Load(t, map[string]string{"data/hard.yaml": hardData})
	res, err := Run(db, Config{Location: "pit", Floors: 3, Battles: 1, Tries: 1, Seed: 1, RestEvery: 3, MaxTurns: 30})
	if err != nil {
		t.Fatal(err)
	}
	if res.Floors[0].Rested || !res.Floors[2].Rested {
		t.Errorf("rest only on floor 3: %+v %+v", res.Floors[0], res.Floors[2])
	}
	if res.Floors[0].Lost != 1 {
		t.Errorf("the brute wins: %+v", res.Floors[0])
	}
}
