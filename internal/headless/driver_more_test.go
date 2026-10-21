package headless_test

import (
	"context"
	"strings"
	"testing"

	"github.com/nao1215/gozen/internal/battle"
	"github.com/nao1215/gozen/internal/event"
	"github.com/nao1215/gozen/internal/headless"
	data "github.com/nao1215/gozen/internal/layer2/gamedata"
	"github.com/nao1215/gozen/internal/testgame"
)

const driverData = `
music:
  songs: {theme: {file: x.png}}
enemies:
  wall:
    name: Wall
    image: x.png
    stats: {mhp: 99999, mmp: 0, atk: 1, def: 99999, mat: 1, mdf: 1, agi: 1, luk: 1}
    ai: [{use: guard}]
  brute:
    name: Brute
    image: x.png
    stats: {mhp: 600, mmp: 0, atk: 400, def: 30, mat: 1, mdf: 1, agi: 99, luk: 1}
    ai: [{use: attack}]
troops:
  wall: {members: [wall]}
  brute: {members: [brute], can_escape: false}
  chatty:
    members: [wall]
    events:
      - when: "turn == 1"
        do:
          - choice: [{label: A, do: [{set: {var.chose: 1}}]}, {label: B, do: []}]
          - battle: one
          - mood: calm
          - sfx: ding
          - effect: flash
          - music: {song: theme}
          - set: {var.talked: 1}
locations:
  crypt:
    name: Crypt
    dungeon:
      map: |
        #######
        #S..G.#
        #######
      tiles:
        S: {start: true, facing: east}
        G: {name: Goal, do: [{set: {flag.goal: true}}]}
  pit:
    name: Pit
    encounters: {rate: "1", troops: [{troop: brute}]}
    exits: [{to: camp}]
events:
  opening: {do: [{say: Welcome}, {set: {flag.opened: true}}]}
  wake: {trigger: auto, once: true, when: "flag.go", do: [{set: {flag.woke: true}}]}
`

func TestStartRunsTheStartEvent(t *testing.T) {
	db := testgame.Load(t, map[string]string{"game.yaml": strings.Replace(testgame.Game, "start: {", "start: {event: opening, ", 1), "data/d.yaml": driverData})
	st := testgame.State(db, 1)
	d := headless.New(db, st)
	if err := d.Start(); err != nil {
		t.Fatal(err)
	}
	if !st.Flags["opened"] || len(d.Transcript) == 0 || d.Transcript[0] != "Welcome" {
		t.Errorf("flags %v transcript %v", st.Flags, d.Transcript)
	}
}

func TestAutoEventsRunWhenTheyHold(t *testing.T) {
	db := testgame.Load(t, map[string]string{"data/d.yaml": driverData})
	st := testgame.State(db, 1)
	d := headless.New(db, st)
	st.Flags["go"] = true
	if err := d.Run(context.Background(), db.Events["opening"].Do); err != nil {
		t.Fatal(err)
	}
	if !st.Flags["woke"] {
		t.Error("an auto event whose when holds ran after the commands")
	}
}

func TestRunNeedsAContextAndStopsWhenItIsDone(t *testing.T) {
	db := testgame.Load(t, map[string]string{"data/d.yaml": driverData})
	st := testgame.State(db, 1)
	d := headless.New(db, st)
	if err := d.Run(nil, db.Events["opening"].Do); err == nil { //nolint:staticcheck // A nil context is the case under test.
		t.Error("a nil context")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := d.Run(ctx, db.Events["opening"].Do); err == nil {
		t.Error("a cancelled context")
	}
	if st.Flags["opened"] {
		t.Error("a cancelled run changed the state")
	}
}

// TestBattleResults: a battle that cannot be finished within MaxTurns is a loss, a lost battle raises the party to 1 HP,
// and an escape is its own result.
func TestBattleResults(t *testing.T) {
	db := testgame.Load(t, map[string]string{"data/d.yaml": driverData})
	st := testgame.State(db, 1)
	d := headless.New(db, st)
	d.MaxTurns = 3
	if got := d.Fight("wall"); got != event.BattleLost || d.Losses != 1 || d.Wins != 0 {
		t.Errorf("a stalemate is a loss: result %d losses %d", got, d.Losses)
	}
	d.MaxTurns = 80
	for _, a := range st.Members() {
		a.HP = 0
	}
	if got := d.Fight("brute"); got != event.BattleLost {
		t.Errorf("result %d", got)
	}
	for _, a := range st.Members() {
		if a.HP < 1 {
			t.Errorf("%s was not raised after the loss", a.Name())
		}
	}
	if d.Battles != 2 {
		t.Errorf("battles %d", d.Battles)
	}
	if !strings.Contains(strings.Join(d.Transcript, "\n"), "[battle brute: lose in") {
		t.Errorf("the transcript says how it ended: %v", d.Transcript)
	}
}

func TestFightsAndGoToStopAtGameOver(t *testing.T) {
	db := testgame.Load(t, map[string]string{"data/d.yaml": driverData})
	st := testgame.State(db, 1)
	d := headless.New(db, st)
	if err := d.GoTo("pit"); err == nil {
		t.Log("no way to the pit from the camp")
	}
	st.Location = "pit"
	err := d.Fights(3)
	if err == nil && !d.Over {
		t.Fatal("losing a battle that cannot be lost ends the game")
	}
	if err := d.Fights(1); err == nil || !strings.Contains(err.Error(), "game over") {
		t.Errorf("after the game is over: %v", err)
	}
	if err := d.GoTo("camp"); err == nil || !strings.Contains(err.Error(), "game over") {
		t.Errorf("walking after the game is over: %v", err)
	}
	// Restore starts again
	fresh := testgame.State(db, 2)
	d.Restore(fresh)
	if d.Over || d.St != fresh {
		t.Error("Restore forgets the game over and takes the new state")
	}
	if err := d.Run(context.Background(), db.Events["opening"].Do); err != nil || !fresh.Flags["opened"] {
		t.Errorf("a restored driver runs on the new state: %v", err)
	}
}

func TestWalkInADungeon(t *testing.T) {
	db := testgame.Load(t, map[string]string{"data/d.yaml": driverData})
	st := testgame.State(db, 1)
	d := headless.New(db, st)
	if err := d.Run(context.Background(), []data.Command{{Move: "crypt"}}); err != nil {
		t.Fatal(err)
	}
	if err := d.Walk('Z'); err == nil || !strings.Contains(err.Error(), `no way to 'Z'`) {
		t.Errorf("a mark that is not there: %v", err)
	}
	if err := d.Walk('G'); err != nil || !st.Flags["goal"] {
		t.Errorf("walking to the goal runs its do: %v", err)
	}
	if err := d.Walk('G'); err != nil {
		t.Errorf("already there: %v", err)
	}
	if err := d.Walk('S'); err != nil {
		t.Errorf("back to the start: %v", err)
	}
	st.Location = "camp"
	if err := d.Walk('G'); err == nil {
		t.Error("there is nothing to walk to outside a dungeon")
	}
}

// TestBattleEventsAreAnsweredQuietly: commands in a troop's battle events run without a screen:
// the first option is taken, a battle inside a battle is won, and effects are ignored.
func TestBattleEventsAreAnsweredQuietly(t *testing.T) {
	db := testgame.Load(t, map[string]string{"data/d.yaml": driverData})
	st := testgame.State(db, 1)
	b := battle.New(db, st, "chatty")
	headless.AutoBattle(db, st, b, nil, 2, nil)
	if st.Vars["chose"] != 1.0 || st.Vars["talked"] != 1.0 {
		t.Errorf("the battle event ran to its end: %v", st.Vars)
	}
	var seen int
	b2 := battle.New(db, st, "chatty")
	headless.AutoBattle(db, st, b2, nil, 2, func(steps []battle.Step) { seen += len(steps) })
	if seen == 0 {
		t.Error("the steps are handed to the observer")
	}
	if b2.Result != battle.Ongoing {
		t.Errorf("a battle cut off by the turn limit is left ongoing for the caller, got %v", b2.Result)
	}
}
