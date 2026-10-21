package play

import (
	"errors"
	"strings"
	"testing"

	"github.com/nao1215/gozen/internal/event"
	data "github.com/nao1215/gozen/internal/layer2/gamedata"
	"github.com/nao1215/gozen/internal/layer3/dungeon"
	"github.com/nao1215/gozen/internal/rpg"
	"github.com/nao1215/gozen/internal/testgame"
)

// btlCells are small fixed dungeons: a closed cell with nothing to do, and a rail with a slide, a
// chest and an event tile.
const btlCells = `
items:
  beacon: {name: Beacon, event: noop, command: true, event_when: "true", price: 1}
  blocked: {name: Seal, event: noop, event_when: "false"}
  rope: {name: Rope, event: noop, price: 5}
events:
  noop: {do: [{gold: "1"}]}
locations:
  cell:
    name: Cell
    dungeon:
      map: |
        ###
        #S#
        ###
      tiles: {S: {start: true, facing: east}}
  rail:
    name: Rail
    dungeon:
      map: |
        #########
        #S~~T.E.#
        #########
      tiles:
        S: {start: true, facing: east}
        "~": {name: Ice, slide: east}
        T: {name: Chest, once: true, after: ".", do: [{gold: "10"}]}
        E: {name: Switch, do: [{set: {flag.pulled: true}}]}
`

func btlCellGame(t *testing.T, loc string, opts Options) *Game {
	t.Helper()
	opts.Location = "camp"
	opts.Autopilot = &Autopilot{Seconds: 100}
	g, _ := btlOpen(t, opts, "", "", map[string]string{"data/cells.yaml": btlCells})
	g.Handle(event.MoveEffect{Location: loc})
	dungeon.Ensure(g.db, g.st)
	if _, ok := g.stack[0].(*dungeonScene); !ok {
		t.Fatalf("not in the dungeon: %T", g.stack[0])
	}
	return g
}

// btlPresses records the buttons the autopilot presses.
type btlPresses struct{ got []button }

func (p *btlPresses) press(b button, _ int) { p.got = append(p.got, b) }

// TestAutopilotWalksToWhatIsToDo: on a rail with a slide it plans a way to the chest first,
// then to the switch, and steps toward the next cell of its path by turning first.
func TestAutopilotWalksToWhatIsToDo(t *testing.T) {
	g := btlCellGame(t, "rail", Options{})
	a := g.auto
	d := dungeon.Of(g.db, g.st)
	a.plan(g, d)
	if len(a.path) == 0 || a.target != "T" || a.chests != 1 {
		t.Fatalf("path %v target %q chests %d", a.path, a.target, a.chests)
	}
	if a.goal != [2]int{4, 1} {
		t.Fatalf("goal %v, the chest is at 4,1", a.goal)
	}
	// Facing north, the way east is a right turn; south and west turn the other ways round.
	g.st.Pos.X, g.st.Pos.Y = 1, 1
	a.path = nil
	a.plan(g, d)
	next := a.path[0]
	if next.X != 2 && next.X != 5 {
		t.Fatalf("first cell %v", next)
	}
	for _, tc := range []struct {
		facing int
		want   button
	}{{0, btnRight}, {2, btnLeft}, {3, btnRight}, {1, btnUp}} {
		g.st.Pos.Facing = tc.facing
		a.path = []rpg.Pos{{In: "rail", X: 2, Y: 1}}
		a.wait = 0
		var p btlPresses
		a.inDungeon(g, p.press)
		if len(p.got) != 1 || p.got[0] != tc.want {
			t.Errorf("facing %d: pressed %v, want %v", tc.facing, p.got, tc.want)
		}
	}
	// A next cell too far away drops the path.
	a.path = []rpg.Pos{{In: "rail", X: 6, Y: 1}}
	var p btlPresses
	a.inDungeon(g, p.press)
	if a.path != nil || len(p.got) != 0 {
		t.Fatalf("path %v pressed %v", a.path, p.got)
	}
	// Standing on the next cell consumes it; at the end of the path the goal is marked used.
	a.path = []rpg.Pos{{In: "rail", X: 1, Y: 1}}
	a.goal = [2]int{1, 1}
	a.inDungeon(g, p.press)
	if len(a.path) != 0 || !a.used[usedKey(g, 1, 1)] || a.wait != 20 {
		t.Fatalf("path %v used %v wait %d", a.path, a.used, a.wait)
	}
}

// TestAutopilotFrontDoorIsMarkedUsed: a way blocked by something solid is dropped and its goal is
// given up, not tried for ever.
func TestAutopilotFrontDoorIsMarkedUsed(t *testing.T) {
	g := btlCellGame(t, "rail", Options{})
	a := g.auto
	g.st.Pos.X, g.st.Pos.Y, g.st.Pos.Facing = 1, 1, 0 // facing the wall to the north
	a.path = []rpg.Pos{{In: "rail", X: 1, Y: 0}}
	a.goal = [2]int{1, 0}
	var p btlPresses
	a.inDungeon(g, p.press)
	if a.path != nil || !a.used[usedKey(g, 1, 0)] {
		t.Fatalf("path %v used %v", a.path, a.used)
	}
}

// TestAutopilotNotesWhenStuck: a party that has not moved for twenty seconds, and one with
// nowhere to go, are reported as problems.
func TestAutopilotNotesWhenStuck(t *testing.T) {
	g := btlCellGame(t, "cell", Options{})
	a := g.auto
	a.lastPos = g.st.Pos
	a.still = 60*20 - 1
	var p btlPresses
	a.inDungeon(g, p.press)
	if len(a.problems) != 1 || !strings.Contains(a.problems[0], "has not moved for 20 seconds") {
		t.Fatalf("problems %v", a.problems)
	}
	a.problems = nil
	a.still = 60*30 - 1
	a.inDungeon(g, p.press)
	if len(a.problems) != 1 || !strings.Contains(a.problems[0], "nowhere to go") {
		t.Fatalf("problems %v", a.problems)
	}
	if len(p.got) == 0 || p.got[len(p.got)-1] != btnRight {
		t.Fatalf("pressed %v: with no way, it turns", p.got)
	}
	// A move resets the count.
	a.lastPos = rpg.Pos{X: 9, Y: 9}
	a.inDungeon(g, p.press)
	if a.still != 0 {
		t.Fatalf("still %d", a.still)
	}
}

// TestAutopilotReturnsWhenWorn: a worn-out party uses a return item that is allowed now, and
// leaves alone one whose condition fails.
func TestAutopilotReturnsWhenWorn(t *testing.T) {
	g := btlCellGame(t, "cell", Options{Items: []string{"blocked"}})
	a := g.auto
	if a.useReturn(g) {
		t.Fatal("used an item whose condition fails")
	}
	g.st.AddItem("rope", 1)
	g.st.Members()[0].HP = 1
	a.cfg.ReturnHP = 0.9
	var p btlPresses
	a.inDungeon(g, p.press)
	if a.returns != 1 {
		t.Fatalf("returns %d", a.returns)
	}
	// Deep enough on a dive: go back although healthy.
	a.returns = 0
	g.st.Members()[0].HP = g.st.Members()[0].Stat("mhp")
	g.st.AddItem("rope", 1)
	g.st.Vars[dungeon.VarFloor] = 3.0
	a.diveAt, a.cfg.Floors = 1, 2
	a.inDungeon(g, p.press)
	if a.returns != 1 || a.diveAt != 0 {
		t.Fatalf("returns %d diveAt %d", a.returns, a.diveAt)
	}
}

// TestAutopilotUsesActionItems: an action item that is allowed now is used, and not again until
// 30 more steps have been walked.
func TestAutopilotUsesActionItems(t *testing.T) {
	g := btlCellGame(t, "cell", Options{Items: []string{"beacon"}})
	a := g.auto
	g.st.Steps = commandEvery
	if !a.useCommand(g) || a.commands != 1 || a.cmdAt != commandEvery {
		t.Fatalf("commands %d at %d", a.commands, a.cmdAt)
	}
	if a.useCommand(g) {
		t.Fatal("used again at once")
	}
	g.st.Steps += commandEvery
	g.db.Items["beacon"].EventOK = data.Expr{Expr: testgame.Expr("false")}
	if a.useCommand(g) {
		t.Fatal("used an item whose condition fails")
	}
	g.db.Items["beacon"].EventOK = data.Expr{}
	if a.useCommand(g) {
		t.Fatal("used an item with no condition")
	}
}

// TestAutopilotHandlesScreens: the autopilot presses OK on the title, the game over and the ending,
// counts a wipe, answers choices, advances finished lines, and backs out of a menu opened by accident.
func TestAutopilotHandlesScreens(t *testing.T) {
	g, _ := btlOpen(t, Options{Location: "-", Autopilot: &Autopilot{Seconds: 100}}, "", "", btlDiveFiles())
	a := g.auto
	a.step(g)
	if !g.in.pressed[btnOK] || a.wait != 20 {
		t.Fatalf("title: pressed %v wait %d", g.in.pressed[btnOK], a.wait)
	}
	g.in.pressed = [numButtons]bool{}
	a.wait = 0
	g.st = nil
	g.push(newGameOver(g))
	a.step(g)
	if a.wipes != 1 || !g.in.pressed[btnOK] {
		t.Fatalf("game over: wipes %d", a.wipes)
	}
	g.in.pressed = [numButtons]bool{}
	a.wait = 0
	g.stack = []scene{newEnding(g)}
	a.step(g)
	if !g.in.pressed[btnOK] {
		t.Fatal("ending: not pressed")
	}
	// A countdown holds it off.
	g.in.pressed = [numButtons]bool{}
	before := a.wait
	a.step(g)
	if g.in.pressed[btnOK] || a.wait != before-1 {
		t.Fatalf("waiting: pressed %v wait %d -> %d", g.in.pressed[btnOK], before, a.wait)
	}
}

// TestAutopilotSpeaksAndAnswers: a finished line is passed on, a choice is answered, and a screen
// opened over the field is closed.
func TestAutopilotSpeaksAndAnswers(t *testing.T) {
	g, _ := btlOpen(t, Options{Location: "town", Autopilot: &Autopilot{Seconds: 100}}, "", "", btlDiveFiles())
	a := g.auto
	g.talk.open(g, event.Say{Text: "Hello there"})
	g.talk.age, g.talk.full = 100, 100
	g.talk.shown = g.talk.total
	a.wait = 0
	a.step(g)
	if !g.in.pressed[btnOK] {
		t.Fatal("a finished line was not passed on")
	}
	g.in.pressed = [numButtons]bool{}
	g.talk = talk{}
	g.talk.ask(g, event.Choice{Prompt: "Which?", Options: []string{"A", "B"}})
	a.wait = 0
	a.step(g)
	if !g.in.pressed[btnOK] {
		t.Fatal("a choice was not answered")
	}
	g.in.pressed = [numButtons]bool{}
	g.talk = talk{}
	g.push(newConfirm("Sure?", func() {}))
	a.wait = 0
	a.step(g)
	if !g.in.pressed[btnOK] {
		t.Fatal("a question was not answered")
	}
	g.in.pressed = [numButtons]bool{}
	g.stack = g.stack[:1]
	g.push(newConfigScreen(g))
	a.wait = 0
	a.step(g)
	if !g.in.pressed[btnCancel] {
		t.Fatal("a menu opened over the field was not closed")
	}
}

// TestAutopilotInBattle: in a battle it turns on auto at the first command, counts the battle
// once, and passes result lines.
func TestAutopilotInBattle(t *testing.T) {
	g, _ := btlOpen(t, Options{Battle: "one", Autopilot: &Autopilot{Seconds: 100}}, "", "")
	a := g.auto
	s := btlAsk(t, g)
	a.step(g)
	if a.battles != 1 || !s.auto {
		t.Fatalf("battles %d auto %v", a.battles, s.auto)
	}
	a.step(g)
	if a.battles != 1 {
		t.Fatal("one battle counted twice")
	}
	s.phase, s.results, s.endT = bpResult, []string{"Won"}, 100
	a.wait = 0
	a.step(g)
	if !g.in.pressed[btnOK] {
		t.Fatal("a result line was not passed on")
	}
}

// TestAutopilotSpendsSkillPointsAndChoosesTheDive: in town it raises the tree of a member with
// points, equips the best, and picks the dive whose label holds the requested words.
func TestAutopilotSpendsSkillPointsAndChoosesTheDive(t *testing.T) {
	g, _ := btlOpen(t, Options{Location: "town", Autopilot: &Autopilot{Seconds: 100, Dive: "other"}}, "", "", btlDiveFiles())
	g.db.Classes["ranger"].Tree = []data.TreeNode{{ID: "care", Skill: "group_heal", Max: 1}}
	g.db.Locations["town"].Actions = append(g.db.Locations["town"].Actions, data.Action{Label: "Dive into the other", Do: []data.Command{{Dive: "deep"}}})
	g.st.Actors["scout"].SP = 2
	a := g.auto
	a.inTown(g)
	if a.dives != 1 {
		t.Fatalf("dives %d", a.dives)
	}
	if g.st.Actors["scout"].Tree["care"] != 1 {
		t.Fatalf("tree %v", g.st.Actors["scout"].Tree)
	}
	// A dive that is not open yet is left out.
	g.db.Locations["town"].Actions[0].When = data.Expr{Expr: testgame.Expr("flag.never")}
	g.db.Locations["town"].Actions[2].When = data.Expr{Expr: testgame.Expr("flag.never")}
	a.cfg.Dive = ""
	a.inTown(g)
	if !g.quit || len(a.problems) == 0 {
		t.Fatal("no dive was open, but the autopilot did not stop")
	}
}

// TestAutopilotShotsAndReportErrors: a screenshot is asked for at the set interval, the time limit ends
// the run, and a report writer that fails is an error.
func TestAutopilotShotsAndReportErrors(t *testing.T) {
	g, _ := btlOpen(t, Options{Location: "town", Report: btlErrWriter{errors.New("disk full")}, Autopilot: &Autopilot{Seconds: 2, ShotEvery: 1}}, "", "", btlDiveFiles())
	a := g.auto
	a.ticks = 59
	a.step(g)
	if g.autoShot != "auto_00001" {
		t.Fatalf("shot %q", g.autoShot)
	}
	g.autoShot = "" // not drawn: a shot reads pixels
	a.ticks = 2*60 + 1
	a.step(g)
	if !g.quit {
		t.Fatal("the time limit did not end the run")
	}
	if g.failure == nil || !strings.Contains(g.failure.Error(), "disk full") {
		t.Fatalf("failure %v", g.failure)
	}
	g.failure = nil
}

type btlErrWriter struct{ err error }

func (w btlErrWriter) Write([]byte) (int, error) { return 0, w.err }
