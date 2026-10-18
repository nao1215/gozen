package dungeon_test

import (
	"math/rand/v2"
	"strings"
	"testing"

	data "github.com/nao1215/gozen/internal/layer2/gamedata"
	"github.com/nao1215/gozen/internal/layer3/dungeon"
	"github.com/nao1215/gozen/internal/testgame"
	"github.com/nao1215/gozen/rpg"
)

// 自動で進む床（slide）と回る床（spin）のテスト。
const slides = `
locations:
  ice:
    name: Ice
    dungeon:
      map: |
        #########
        #S.ii..k#
        #.####v##
        #.#...<.#
        #.##r.D<#
        #########
      tiles:
        S: {start: true, facing: east}
        i: {name: Ice, slide: facing, mark: "#a0d0ff"}
        v: {name: Rail, slide: south, mark: "#d0a060", do: [{say: carried}]}
        "<": {name: Stream, slide: west, symbol: "←"}
        k: {name: Key, do: [{say: key}]}
        r: {name: Whirl, spin: back}
`

func slideSetup(t *testing.T) (*data.DB, *rpg.State) {
	db := testgame.Load(t, map[string]string{"data/ice.yaml": slides})
	st := testgame.State(db, 1)
	st.Location = "ice"
	dungeon.Ensure(db, st)
	return db, st
}

func TestASlideStopsAtTheWall(t *testing.T) {
	db, st := slideSetup(t)
	dungeon.Forward(db, st) // 2,1
	moved, cmds := dungeon.Forward(db, st)
	// onto the ice at 3,1 going east: over the ice and the floor, stopped on the key (a tile)
	if !moved || st.Pos.X != 7 || st.Pos.Y != 1 || st.Pos.Facing != 1 {
		t.Fatalf("after the ice: %+v, want 7,1 facing east", st.Pos)
	}
	if len(cmds) != 1 || cmds[0].Say == nil || cmds[0].Say.Text != "key" {
		t.Errorf("stopping on the key: %+v", cmds)
	}
	if !dungeon.Seen(db, st, "ice", 5, 1) {
		t.Error("the cells slid over are not on the map")
	}
	// back west onto the ice: carried to the wall beyond the start
	dungeon.Turn(st, 2)
	dungeon.Forward(db, st) // 6,1
	dungeon.Forward(db, st) // 5,1
	dungeon.Forward(db, st) // 4,1 ice → west, stopped on the start tile (another tile) at 1,1
	if st.Pos.X != 1 || st.Pos.Y != 1 {
		t.Errorf("sliding west: %+v, want stopped on the start tile at 1,1", st.Pos)
	}
}

func TestASlideTurnsAndStopsBeforeADoor(t *testing.T) {
	db, st := slideSetup(t)
	// from 6,1 facing south: onto the rail at 6,2, down onto the stream at 6,3 (west), on to 3,3 by the wall
	st.Pos.X, st.Pos.Y, st.Pos.Facing = 6, 1, 2
	moved, cmds := dungeon.Forward(db, st)
	if !moved || st.Pos.X != 3 || st.Pos.Y != 3 || st.Pos.Facing != 3 {
		t.Fatalf("rail then stream: %+v, want 3,3 facing west", st.Pos)
	}
	if len(cmds) != 1 || cmds[0].Say == nil || cmds[0].Say.Text != "carried" {
		t.Errorf("the rail's words: %+v", cmds)
	}
	// a slide stops before a door
	d := dungeon.Of(db, st)
	if x, y, _, _ := dungeon.SlideEnd(d, st, 7, 4, 2); x != 7 || y != 4 {
		t.Errorf("SlideEnd from the stream at 7,4 = %d,%d, want 7,4 (the door at 6,4 stops it)", x, y)
	}
	if x, y, f, _ := dungeon.SlideEnd(d, st, 4, 1, 1); x != 7 || y != 1 || f != 1 {
		t.Errorf("SlideEnd east over the ice from 4,1 = %d,%d facing %d, want 7,1 on the key facing east", x, y, f)
	}
}

func TestASpinTurnsTheParty(t *testing.T) {
	db, st := slideSetup(t)
	st.Pos.X, st.Pos.Y, st.Pos.Facing = 5, 4, 3 // facing west, the whirl at 4,4
	if moved, _ := dungeon.Forward(db, st); !moved || st.Pos.X != 4 || st.Pos.Facing != 1 {
		t.Errorf("on the whirl: %+v, want 4,4 facing east (turned back)", st.Pos)
	}
}

func TestTheWayGoesOverSlides(t *testing.T) {
	db, st := slideSetup(t)
	// the key at 7,1: the shortest way goes over the ice
	for range 20 {
		move, ok := dungeon.Next(db, st, 'k')
		if !ok {
			t.Fatal("no way to the key")
		}
		if move == dungeon.MoveHere {
			return
		}
		switch move {
		case dungeon.MoveForward:
			dungeon.Forward(db, st)
		case dungeon.MoveLeft:
			dungeon.Turn(st, -1)
		case dungeon.MoveRight:
			dungeon.Turn(st, 1)
		}
	}
	t.Fatalf("did not reach the key: %+v", st.Pos)
}

// 生成の迷宮に滑る床を置いても、下り階段と全部の床へ行けて、どこからでもスタートへ戻れる
// （画面と同じ CellAt・SlideEnd で歩いて確かめる）。
func TestSlidesPlacedKeepTheWayOpen(t *testing.T) {
	db := testgame.Load(t, map[string]string{"data/ice.yaml": slides})
	st := testgame.State(db, 1)
	place := []data.Put{{Char: ">", Far: true}}
	for _, ch := range strings.Split("T T T P P 1 1 1 1 2 2 2 2 i i i i i i", " ") {
		place = append(place, data.Put{Char: ch})
	}
	g := &data.Generate{Width: 15, Height: 15, Rooms: 3, Start: "S", Place: place}
	kind := func(r rune) dungeon.Kind {
		switch r {
		case 'T':
			return dungeon.Kind{Solid: true}
		case 'P':
			return dungeon.Kind{Carry: true}
		case '1':
			return dungeon.Kind{Slide: "north"}
		case '2':
			return dungeon.Kind{Slide: "east"}
		case 'i':
			return dungeon.Kind{Slide: "facing"}
		}
		return dungeon.Kind{}
	}
	d := &data.Dungeon{Tiles: map[string]*data.Tile{
		"S": {}, ">": {}, "T": {Solid: data.Expr{}}, "P": {Do: []data.Command{{Move: "elsewhere"}}},
		"1": {Slide: "north"}, "2": {Slide: "east"}, "i": {Slide: "facing"},
	}}
	for _, tl := range d.Tiles {
		tl.Look = "floor"
	}
	d.Tiles["T"].Look = "wall" // solid
	placed := 0
	for seed := range uint64(60) {
		m := dungeon.MazeKinds(g, rand.New(rand.NewPCG(seed, 9)), kind, func(data.Expr) int { return 1 }) //nolint:gosec // Deterministic gameplay randomness, not security-sensitive data.
		placed += strings.Count(m, "1") + strings.Count(m, "2") + strings.Count(m, "i")
		if strings.Count(m, ">") != 1 {
			t.Fatalf("seed %d: no stairs:\n%s", seed, m)
		}
		d.SetMap(m)
		if bad := unwalkable(d, st); bad != "" {
			t.Fatalf("seed %d: %s:\n%s", seed, bad, m)
		}
	}
	if placed < 60*10 {
		t.Errorf("only %d slides placed in 60 floors (want most of the 14 a floor)", placed)
	}
}

// unwalkable walks d from its S with the rules of the screen (a slide carries, P carries
// away) and tells what cannot be walked to, or back from ("" when all is well).
func unwalkable(d *data.Dungeon, st *rpg.State) string {
	sx, sy, _ := d.Find('S')
	w, h := d.Size()
	dx := [4]int{0, 1, 0, -1}
	dy := [4]int{-1, 0, 1, 0}
	type p2 = [2]int
	edges := map[p2][]p2{}
	seen := map[p2]bool{{sx, sy}: true}
	q := []p2{{sx, sy}}
	for len(q) > 0 {
		c := q[0]
		q = q[1:]
		for f := range 4 {
			x, y := c[0]+dx[f], c[1]+dy[f]
			n := dungeon.CellAt(d, st, x, y)
			if !n.Passable() || n.Char == 'P' {
				continue
			}
			if dungeon.Slides(n) {
				x, y, _, _ = dungeon.SlideEnd(d, st, x, y, f)
				if dungeon.CellAt(d, st, x, y).Char == 'P' {
					continue
				}
			}
			e := p2{x, y}
			edges[e] = append(edges[e], c)
			if !seen[e] {
				seen[e] = true
				q = append(q, e)
			}
		}
	}
	for y := range h {
		for x := range w {
			switch r := d.At(x, y); r {
			case '#', 'P', '1', '2', 'i':
			case 'T':
				ok := false
				for f := range 4 {
					ok = ok || seen[p2{x + dx[f], y + dy[f]}]
				}
				if !ok {
					return "a chest cannot be come to"
				}
			default:
				if !seen[p2{x, y}] {
					return "a cell cannot be walked to"
				}
			}
		}
	}
	home := map[p2]bool{{sx, sy}: true}
	q = []p2{{sx, sy}}
	for len(q) > 0 {
		c := q[0]
		q = q[1:]
		for _, b := range edges[c] {
			if !home[b] {
				home[b] = true
				q = append(q, b)
			}
		}
	}
	for c := range seen {
		if !home[c] {
			return "a cell walked to has no way back"
		}
	}
	return ""
}

// 前が塞がって動けない slide は、始まりと止まりが同じマス。そのマスの do は 1 回だけ。
// look: door・trigger: step・once のタイルは、通り抜けの判定で使い切らずに 1 回だけ発火する。
func TestABlockedSlideAndAOnceDoorFireOnce(t *testing.T) {
	db := testgame.Load(t, map[string]string{"data/ice.yaml": `
locations:
  ice:
    name: Ice
    dungeon:
      map: |
        ######
        #S.d.#
        #.w..#
        ######
      tiles:
        S: {start: true, facing: east}
        w: {name: Rail, slide: south, do: [{say: carried}]}
        d: {name: Gate, look: door, trigger: step, once: true, do: [{say: gate}]}
`})
	st := testgame.State(db, 1)
	st.Location = "ice"
	dungeon.Ensure(db, st)
	st.Pos.X, st.Pos.Y, st.Pos.Facing = 2, 1, 2
	moved, cmds := dungeon.Forward(db, st)
	if !moved || st.Pos.X != 2 || st.Pos.Y != 2 {
		t.Fatalf("onto the blocked rail: %+v", st.Pos)
	}
	if len(cmds) != 1 || cmds[0].Say == nil || cmds[0].Say.Text != "carried" {
		t.Errorf("the blocked rail's words: %+v", cmds)
	}
	st.Pos.X, st.Pos.Y, st.Pos.Facing = 2, 1, 1
	_, cmds = dungeon.Forward(db, st)
	if len(cmds) != 1 || cmds[0].Say == nil || cmds[0].Say.Text != "gate" {
		t.Errorf("the once door's words: %+v at %+v", cmds, st.Pos)
	}
}

// 地図や徘徊敵の定義が変わる前のセーブ: 地図の外の位置は入口へ、合わない徘徊敵は消す（範囲外で落ちない）。
func TestEnsureFixesAnOldSave(t *testing.T) {
	db, st := slideSetup(t)
	st.Pos.X, st.Pos.Y = 40, 40
	st.Roamers = []rpg.Roamer{{Kind: 3, Troop: "one", X: 2, Y: 1}, {Kind: 0, Troop: "one", X: 99, Y: 1}}
	dungeon.Ensure(db, st)
	if st.Pos.X != 1 || st.Pos.Y != 1 {
		t.Errorf("position %+v, want the start 1,1", st.Pos)
	}
	if len(st.Roamers) != 0 {
		t.Errorf("roamers kept: %+v", st.Roamers)
	}
	dungeon.Forward(db, st) // no panic
}
