package play

import (
	"bytes"
	"log"
	"strings"
	"testing"

	"github.com/nao1215/gozen/internal/layer3/dungeon"
)

const sysMadeDungeon = `locations:
  deep:
    name: Deep
    dungeon:
      generate:
        width: 11
        height: 11
        rooms: 2
        start: S
        place: [{char: ">", far: true}]
      tiles:
        S: {start: true}
        ">": {name: Down, solid: "true", do: [{dive: deep, floor: "var.floor + 1"}]}
`

// TestFloorOptionDivesIntoAGeneratedDungeon: --floor starts on that floor of a generated dungeon;
// saves and the auto slot name the floor.
func TestFloorOptionDivesIntoAGeneratedDungeon(t *testing.T) {
	g := sysOpen(t, Options{Seed: 4, Location: "deep", Floor: 3}, map[string]string{"data/deep.yaml": sysMadeDungeon})
	if f, ok := g.st.Vars[dungeon.VarFloor].(float64); !ok || f != 3 {
		t.Fatalf("floor %v", g.st.Vars[dungeon.VarFloor])
	}
	if err := g.save(0); err != nil {
		t.Fatal(err)
	}
	label, ok := g.slotLabel(0)
	if !ok || !strings.Contains(label, "B3F") {
		t.Fatalf("label %q", label)
	}
	sysFrames(t, g, 12)
	if g.autoKey != "deep/3" {
		t.Fatalf("auto slot key %q", g.autoKey)
	}
	// the floor is a generated square of the size asked for.
	grid := dungeon.Of(g.db, g.st).Grid
	if len(grid) != 11 {
		t.Fatalf("map has %d rows", len(grid))
	}
}

// TestDebugLogsWhenAsked: with build info and GOZEN_DEBUG the loop logs its position every half
// second and the drawing its cost.
func TestDebugLogsWhenAsked(t *testing.T) {
	t.Setenv("GOZEN_DEBUG", "1")
	var out bytes.Buffer
	log.SetOutput(&out)
	t.Cleanup(func() { log.SetOutput(sysLogOriginal) })
	g := sysOpen(t, Options{Location: "camp", CollectBuildInfo: true, Script: "wait 80\nquit"}, nil)
	for range 70 { // every frame drawn: the drawing logs its cost on some frames only
		sysFrames(t, g, 1)
	}
	if !strings.Contains(out.String(), "tick 30 pc ") || !strings.Contains(out.String(), "draw ") {
		t.Fatalf("log: %q", out.String())
	}
}

var sysLogOriginal = log.Writer()

// TestBacklogViewOpensAndCloses: the log key shows the lines said so far in a view of its own and
// cancel closes it; with nothing said it does not open.
func TestBacklogViewOpensAndCloses(t *testing.T) {
	g := sysOpen4(t, Options{Seed: 1, Location: "camp", Event: "story", Line: 3, Script: "wait 5\npress log\nwait 3\nquit"},
		map[string]string{"data/novel.yaml": novelData, "game.yaml+": "ui: {message: novel}\n"})
	sysRunFast(t, g, 3000)
	if len(g.backlog) == 0 {
		t.Fatal("nothing in the backlog")
	}
	for range 40 {
		sysFrames(t, g, 1)
		if g.logView != nil {
			break
		}
	}
	if g.logView == nil {
		t.Fatal("the log key did not open the log")
	}
	sysFrames(t, g, 2)
	if g.logView == nil {
		t.Fatal("the log closed by itself")
	}
	sysKey(g, btnCancel)
	g.logView.update(g)
	if g.logView != nil {
		t.Fatal("cancel did not close the log")
	}
}
