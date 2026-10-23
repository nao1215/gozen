package dungeon_test

import (
	"bytes"
	"context"
	"testing"

	"github.com/nao1215/gozen/internal/headless"
	data "github.com/nao1215/gozen/internal/layer2/gamedata"
	"github.com/nao1215/gozen/internal/layer3/dungeon"
	"github.com/nao1215/gozen/internal/testgame"
)

const beltData = `
music:
  songs: {theme: {file: x.png}, fight: {file: x.png}}
locations:
  belt:
    name: Belt
    battle_music: {song: fight}
    dungeon:
      map: |
        #########
        #S.>..G.#
        #########
      tiles:
        S: {start: true, facing: east}
        ">": {name: Belt, slide: east}
        G: {name: Goal, solid: "true", do: [{gold: 5}]}
      walked: [{say: All of it is seen.}]
`

// TestSlideNextAndShown: stepping onto a slide tile carries the party on to the goal, the planner finds the way, and the shown cells grow.
func TestSlideNextAndShown(t *testing.T) {
	db := testgame.Load(t, map[string]string{"data/belt.yaml": beltData})
	st := testgame.State(db, 1)
	st.Location = "belt"
	dungeon.Place(db, st, "belt", "", "")
	d := dungeon.Of(db, st)
	if d == nil {
		t.Fatal("no dungeon")
	}
	w, h := d.Size()
	if w != 9 || h != 3 {
		t.Fatalf("size %dx%d", w, h)
	}
	shown := dungeon.Shown(db, st)
	if len(shown) != w*h || !shown[1*w+1] {
		t.Fatalf("the start cell is shown: %v", shown)
	}
	if shown[1*w+6] {
		t.Error("the far goal is not shown yet")
	}
	if move, ok := dungeon.Next(db, st, 'G'); !ok || move != "forward" {
		t.Errorf("Next toward the goal: %q %v", move, ok)
	}
	if move, ok := dungeon.Next(db, st, 'S'); !ok || move != "here" {
		t.Errorf("standing on the mark: %q %v", move, ok)
	}
	if _, ok := dungeon.Next(db, st, 'Q'); ok {
		t.Error("there is no Q to walk to")
	}
	belt := dungeon.CellAt(d, st, 3, 1)
	if !dungeon.Slides(belt) || dungeon.Slides(dungeon.CellAt(d, st, 2, 1)) {
		t.Error("only the belt tile slides")
	}
	x, y, facing, way := dungeon.SlideEnd(d, st, 3, 1, st.Pos.Facing)
	if x != 5 || y != 1 || facing != st.Pos.Facing || len(way) != 2 {
		t.Errorf("the belt carries to the cell before the goal: %d,%d facing %d via %v", x, y, facing, way)
	}
	// walk: step, step onto the belt
	for range 2 {
		moved, cmds := dungeon.Forward(db, st)
		if !moved {
			t.Fatal("blocked")
		}
		if err := headless.New(db, st).Run(context.Background(), cmds); err != nil {
			t.Fatal(err)
		}
	}
	if st.Pos.X != 5 {
		t.Errorf("the party rode the belt to x=5, stands at %d", st.Pos.X)
	}
	if front := dungeon.Front(db, st); front.Char != 'G' {
		t.Errorf("in front: %q", front.Char)
	}
	if got := dungeon.Trigger(d.Tile('G'), true); got != "bump" {
		t.Errorf("a solid tile is bumped: %q", got)
	}
	if got := dungeon.Trigger(d.Tile('>'), false); got != "step" {
		t.Errorf("a floor tile is stepped on: %q", got)
	}
	if cmds := dungeon.Look(db, st); len(cmds) == 0 {
		t.Error("looking at the goal does its do")
	}
}

func TestWalkedGivesItsCommandsOnlyOnTheStartOfAWholeSeenFloor(t *testing.T) {
	db := testgame.Load(t, map[string]string{"data/belt.yaml": beltData})
	st := testgame.State(db, 1)
	st.Location = "belt"
	dungeon.Place(db, st, "belt", "", "")
	if dungeon.Completed(st, "belt") {
		t.Error("nothing is walked yet")
	}
	if cmds := dungeon.Walked(db, st); cmds != nil {
		t.Errorf("the floor is not seen whole: %+v", cmds)
	}
	w, h := dungeon.Of(db, st).Size()
	st.Mapped["belt"] = bytes.Repeat([]byte{1}, w*h)
	cmds := dungeon.Walked(db, st)
	if len(cmds) != 1 || cmds[0].Say.Text != "All of it is seen." {
		t.Fatalf("a whole seen floor, at its start: %+v", cmds)
	}
	if st.Vars["floor_left"] != 0.0 {
		t.Errorf("nothing is left to find: %v", st.Vars["floor_left"])
	}
	st.Pos.X = 4
	if cmds := dungeon.Walked(db, st); cmds != nil {
		t.Errorf("arriving anywhere but the start gives nothing: %+v", cmds)
	}
}

func TestBattleMusicFallsBackToThePlace(t *testing.T) {
	db := testgame.Load(t, map[string]string{"data/belt.yaml": beltData})
	st := testgame.State(db, 1)
	st.Location = "belt"
	if got := dungeon.BattleMusic(db, st, nil); got.Song != "fight" {
		t.Errorf("battle music %q, want the place's", got.Song)
	}
	if got := dungeon.BattleMusic(db, st, &data.Troop{Music: data.MusicCue{Song: "theme"}}); got.Song != "theme" {
		t.Errorf("the troop's music comes first, got %q", got.Song)
	}
	if got := dungeon.Hunters(db, st); got != nil {
		t.Errorf("a floor without hunters: %+v", got)
	}
}
