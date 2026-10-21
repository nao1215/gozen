package play

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"

	data "github.com/nao1215/gozen/internal/layer2/gamedata"
	"github.com/nao1215/gozen/internal/layer3/dungeon"
)

// btlDiveGame is a town with a rest, a shop and a way down into a made dungeon whose
// floors have stairs, a chest and fights; a rope takes the party back to town.
const btlDiveGame = `title: Dive course
id: screen-test
fonts: {regular: test.ttf}
start: {location: town, party: [scout, guide], gold: 100, items: {potion: 2}}
battle:
  escape: "1"
  critical: "0"
  variance: 0.0001
  speed: "a.agi"
`

const btlDiveData = `
items:
  rope: {name: Rope, price: 5, event: go_town}
shops:
  store: {name: Store, goods: [rope, potion]}
events:
  go_town: {do: [{move: town}]}
locations:
  town:
    name: Town
    actions:
      - {label: "Dive into the deep", do: [{dive: deep}]}
      - {label: "Rest", do: [{heal: all}]}
  deep:
    name: Deep
    encounters: {rate: "0.3", troops: [{troop: one}]}
    dungeon:
      generate:
        width: 11
        height: 11
        rooms: 3
        start: S
        place:
          - {char: ">", far: true}
          - {char: T, count: "1"}
      tiles:
        S: {name: Up}
        T: {name: Chest, solid: "true", once: true, after: ".", do: [{gold: "10"}]}
        ">": {name: Down, trigger: step, do: [{dive: deep, floor: "var.floor + 1"}]}
`

func btlDiveFiles() map[string]string {
	return map[string]string{"game.yaml": btlDiveGame, "data/dive.yaml": btlDiveData}
}

// btlRunToEnd steps the game until it quits and returns how many ticks it took.
func btlRunToEnd(t *testing.T, g *Game, limit int) int {
	t.Helper()
	for i := range limit {
		err := g.Update()
		if errors.Is(err, ebiten.Termination) {
			return i
		}
		if err != nil {
			t.Fatal(err)
		}
		g.Draw(btlImage())
	}
	t.Fatalf("the game did not quit in %d ticks", limit)
	return 0
}

// TestAutopilotPlaysAndReports: with --autoplay the game starts itself from the title, dives,
// fights, takes the chests and writes a report with what it did when the time is up.
func TestAutopilotPlaysAndReports(t *testing.T) {
	var report bytes.Buffer
	file := filepath.Join(t.TempDir(), "report.txt")
	g, out := btlOpen(t, Options{Seed: 3, Location: "-", Autopilot: &Autopilot{Seconds: 25, Report: file, Floors: 2}, Report: &report}, "", "", btlDiveFiles())
	_ = out
	n := btlRunToEnd(t, g, 25*60+200)
	if n < 25*60 {
		t.Fatalf("quit after %d ticks, before the time was up", n)
	}
	text := report.String()
	for _, want := range []string{"autopilot report", "dives ", "battles ", "gold ", "problems 0"} {
		if !strings.Contains(text, want) {
			t.Errorf("the report lacks %q:\n%s", want, text)
		}
	}
	if strings.Contains(text, "dives 0,") {
		t.Errorf("never dived:\n%s", text)
	}
	b, err := os.ReadFile(file)
	if err != nil || string(b) != text {
		t.Fatalf("the report file differs from the report (%v)", err)
	}
	if g.auto.battles == 0 || g.auto.floorsWalked == 0 {
		t.Errorf("battles %d floors %d", g.auto.battles, g.auto.floorsWalked)
	}
}

// TestAutopilotFacingAndPlan: the path planner walks to the stairs on a floor, the facing
// helper maps steps to the four directions.
func TestAutopilotFacingAndPlan(t *testing.T) {
	for _, tc := range []struct{ x, y, nx, ny, want int }{
		{5, 5, 5, 4, 0}, {5, 5, 6, 5, 1}, {5, 5, 5, 6, 2}, {5, 5, 4, 5, 3},
	} {
		if got := facingTo(tc.x, tc.y, tc.nx, tc.ny); got != tc.want {
			t.Errorf("facingTo(%d,%d,%d,%d) = %d, want %d", tc.x, tc.y, tc.nx, tc.ny, got, tc.want)
		}
	}
	g, _ := btlOpen(t, Options{Location: "town", Autopilot: &Autopilot{Seconds: 5}}, "", "", btlDiveFiles())
	g.events.Start(g.db.Locations["town"].Actions[0].Do)
	for range 600 {
		if _, ok := g.stack[len(g.stack)-1].(*dungeonScene); ok {
			break
		}
		btlFrame(t, g)
	}
	d := dungeon.Of(g.db, g.st)
	if d == nil {
		t.Fatalf("not in a dungeon: %s", g.st.Location)
	}
	a := g.auto
	a.plan(g, d)
	if len(a.path) == 0 {
		t.Fatal("no way planned on a fresh floor")
	}
	first := a.path[0]
	if abs(first.X-g.st.Pos.X)+abs(first.Y-g.st.Pos.Y) != 1 {
		t.Fatalf("the first step %v is not next to %v", first, g.st.Pos)
	}
	last := a.path[len(a.path)-1]
	if a.goal != [2]int{last.X, last.Y} {
		t.Fatalf("goal %v, path ends at %v", a.goal, last)
	}
}

// TestAutopilotHelpers: the checks the autopilot makes on the data.
func TestAutopilotHelpers(t *testing.T) {
	g, _ := btlOpen(t, Options{Location: "town"}, "", "", btlDiveFiles())
	db := g.db
	heal := db.Locations["town"].Actions[1].Do
	dive := db.Locations["town"].Actions[0].Do
	if !hasHeal(db, heal) || hasHeal(db, dive) || !hasDive(dive) || hasDive(heal) {
		t.Fatal("hasHeal/hasDive")
	}
	nested := []data.Command{{Then: heal}}
	if !hasHeal(db, nested) {
		t.Fatal("hasHeal does not look into branches")
	}
	db.Events["rest"] = &data.Event{Do: heal}
	if !hasHeal(db, []data.Command{{Call: "rest"}}) {
		t.Fatal("hasHeal does not follow a call")
	}
	if shopOf(g, "rope") != "store" || shopOf(g, "key") != "" {
		t.Fatal("shopOf")
	}
	if !whenOK(g, data.Expr{}) {
		t.Fatal("an empty condition is not true")
	}
	a := newAutopilot(Autopilot{})
	if a.cfg.ReturnHP != 0.35 || a.cfg.Floors != 3 {
		t.Fatalf("defaults %+v", a.cfg)
	}
	if a.worn(g) || a.anyHurt(g) {
		t.Fatal("a healthy party is worn or hurt")
	}
	g.st.Members()[0].HP = 1
	if !a.anyHurt(g) {
		t.Fatal("a hurt member is not noticed")
	}
	a.cfg.ReturnHP = 0.9
	if !a.worn(g) {
		t.Fatal("a party below the return level is not worn")
	}
	g.st.Members()[0].HP = 0
	a.cfg.ReturnHP = 0.01
	if !a.worn(g) {
		t.Fatal("a fallen member is not worn")
	}
	if got := itemSet(db); !got["rope"] || !got["potion"] {
		t.Fatal("itemSet")
	}
}

// TestAutopilotBuysAReturnItem: in town, with gold, the autopilot buys the item that takes
// it home and potions, and a party that is hurt rests.
func TestAutopilotBuysAReturnItem(t *testing.T) {
	g, _ := btlOpen(t, Options{Location: "town", Gold: 200}, "", "", btlDiveFiles())
	a := newAutopilot(Autopilot{})
	g.auto = a
	g.st.Items = map[string]int{}
	a.buyReturn(g)
	if g.st.Items["rope"] != 1 {
		t.Fatalf("items %v", g.st.Items)
	}
	a.buyReturn(g)
	if g.st.Items["potion"] != 3 {
		t.Fatalf("potions %d", g.st.Items["potion"])
	}
	g.st.Members()[0].HP = 1
	before := g.st.Gold
	a.inTown(g)
	for range 200 {
		btlFrame(t, g)
		if g.st.Members()[0].HP == g.st.Members()[0].Stat("mhp") {
			break
		}
	}
	if g.st.Members()[0].HP != g.st.Members()[0].Stat("mhp") {
		t.Fatal("did not rest while hurt")
	}
	if before != g.st.Gold {
		t.Fatalf("gold %d -> %d for a free rest", before, g.st.Gold)
	}
}

// TestAutopilotReturnsWithItems: when worn out in a dungeon, it uses the rope; the action
// item of a floor is used once the steps allow it.
func TestAutopilotReturnsWithItems(t *testing.T) {
	g, _ := btlOpen(t, Options{Location: "town", Gold: 200, Items: []string{"rope"}}, "", "", btlDiveFiles())
	a := newAutopilot(Autopilot{})
	g.auto = a
	if !a.useReturn(g) {
		t.Fatal("the rope was not used")
	}
	if a.returns != 1 {
		t.Fatalf("returns %d", a.returns)
	}
	g.st.Items["rope"] = 0
	delete(g.st.Items, "rope")
	if a.useReturn(g) {
		t.Fatal("used a return item it does not have")
	}
	if a.useCommand(g) {
		t.Fatal("used a command item it does not have")
	}
}

// TestAutopilotProblemsAreReported: a problem is traced, counted and printed in the report.
func TestAutopilotProblemsAreReported(t *testing.T) {
	var report bytes.Buffer
	g, out := btlOpen(t, Options{Location: "town", Report: &report}, "", "", btlDiveFiles())
	a := newAutopilot(Autopilot{})
	g.auto = a
	a.problem(g, "stuck at %d", 7)
	a.errs = append(a.errs, "boom")
	a.finish(g)
	if len(a.problems) != 1 || !strings.Contains(a.problems[0], "stuck at 7") {
		t.Fatalf("problems %v", a.problems)
	}
	if p := btlLines(t, out, "problem"); len(p) != 1 {
		t.Fatalf("problem lines %v", p)
	}
	for _, want := range []string{"problems 1", "stuck at 7", "event error: boom", "Scout Lv", "Guide Lv"} {
		if !strings.Contains(report.String(), want) {
			t.Errorf("report lacks %q:\n%s", want, report.String())
		}
	}
}

// TestAutopilotReportFileFailure: an unwritable report file stops the game with the error.
func TestAutopilotReportFileFailure(t *testing.T) {
	g, _ := btlOpen(t, Options{Location: "town"}, "", "", btlDiveFiles())
	a := newAutopilot(Autopilot{Report: filepath.Join(t.TempDir(), "no-such-dir", "r.txt")})
	g.auto = a
	a.finish(g)
	if g.failure == nil || !strings.Contains(g.failure.Error(), "no-such-dir") {
		t.Fatalf("a failed report write was not reported: %v", g.failure)
	}
	g.failure = nil // so that closing the game does not report it again
}

// TestAutopilotNoWayDown: a town without a dive makes it say so and stop.
func TestAutopilotNoWayDown(t *testing.T) {
	g, _ := btlOpen(t, Options{Location: "camp", Autopilot: &Autopilot{Seconds: 5}}, "", "")
	g.st.Location = "camp"
	g.auto.inTown(g)
	if !g.quit || len(g.auto.problems) != 1 || !strings.Contains(g.auto.problems[0], "no way into the dungeon") {
		t.Fatalf("quit %v problems %v", g.quit, g.auto.problems)
	}
}
