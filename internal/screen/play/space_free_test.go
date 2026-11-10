package play

import (
	"bytes"
	"math"
	"os"
	"strings"
	"testing"
	"testing/fstest"

	data "github.com/nao1215/gozen/internal/model/gamedata"
	"github.com/nao1215/gozen/internal/model/rpg"
	"github.com/nao1215/gozen/internal/rules/dungeon"
)

func TestFreeSpaceInputAndMenu(t *testing.T) {
	g := wldOpenWith(t, Options{FS: freePlayFixture(t), Location: "inn"})
	wldFrames(t, g, 25)
	wldPlay(t, g, "hold up 10\nwait 1\n", 30)
	if p := g.st.SpacePose; p == nil || math.Abs(p.Z-(8.5-1.6/6)) > 1e-8 || g.st.Pos.Y != 8 {
		t.Fatalf("input did not stop within cell: %+v %+v", p, g.st.Pos)
	}
	wldPlay(t, g, "hold right 10\nwait 1\n", 30)
	if math.Abs(g.st.SpacePose.Yaw-math.Pi/12) > 1e-8 {
		t.Fatalf("continuous turn: %+v", g.st.SpacePose)
	}
	before := *g.st.SpacePose
	wldPlay(t, g, "press menu\nwait 1\nhold up 10\n", 35)
	if *g.st.SpacePose != before {
		t.Fatal("menu input moved the player")
	}
}

func freePlayFixture(t *testing.T) fstest.MapFS {
	t.Helper()
	fixture := spaceFixtureFiles(t, os.DirFS("../../../examples/space-town/game"))
	fixture["data/world.yaml"].Data = bytes.Replace(fixture["data/world.yaml"].Data, []byte("      space:"), []byte("      space:\n        free_move: {speed: 1.6, turn_speed: 90, acceleration: 0}"), 1)
	return fixture
}

func TestFreeSpaceFocusPausePreservesMemoProgress(t *testing.T) {
	g := wldOpenWith(t, Options{FS: freePlayFixture(t), Location: "inn"})
	g.in.script = nil
	g.opts.Script = ""
	g.StartRendering()
	if !g.pauseForFocus(false) || g.pauseForFocus(true) {
		t.Fatal("ordinary play must pause only while unfocused")
	}
	g.memoToastT = 5
	if g.pauseForFocus(false) {
		t.Fatal("unfocused memo completion toast cannot expire")
	}
	g.memoToastT = 0
	g.memo = &memoBox{}
	if g.pauseForFocus(false) {
		t.Fatal("external memo editor completion cannot be collected")
	}
}

func TestFreeSpaceTraceReplay(t *testing.T) {
	fixture := freePlayFixture(t)
	var trace bytes.Buffer
	g := wldOpenWith(t, Options{FS: fixture, Location: "inn", Seed: 7, Trace: &trace, Script: "wait 30\nhold up 10\nhold right 10\nhold up+shift+right 10\npress menu\nwait 10\npress x\nwait 10\nquit\n"})
	screen := wldScreen(g)
	for i := 0; i < 1000 && wldStep(t, g, screen); i++ {
	}
	want := *g.st.SpacePose
	if err := g.Close(); err != nil {
		t.Fatal(err)
	}
	replayed, err := wldReplayGame(t, trace.Bytes(), func(opts *Options) { opts.FS = fixture })
	if err != nil {
		t.Fatal(err)
	}
	if report := wldReplay(t, replayed); !strings.HasPrefix(report, "replay fully matched:") {
		t.Fatal(report)
	}
	if replayed.st.SpacePose == nil || *replayed.st.SpacePose != want {
		t.Fatalf("replayed pose %+v want %+v", replayed.st.SpacePose, want)
	}
}

func BenchmarkFreeSpaceController(b *testing.B) {
	fixture := spaceScalingFixture(b, 40, 36, 4, true)
	fixture["data/world.yaml"].Data = bytes.Replace(fixture["data/world.yaml"].Data, []byte("space:"), []byte("space:\n        free_move: {speed: 2, turn_speed: 90, acceleration: 0.12, stages: [{after: 0.4, speed: 3.2}]}"), 1)
	db, err := data.Load(fixture)
	if err != nil {
		b.Fatal(err)
	}
	for _, moving := range []bool{false, true} {
		b.Run(map[bool]string{false: "idle", true: "walking"}[moving], func(b *testing.B) {
			st := rpg.New(db, nil)
			dungeon.Ensure(db, st)
			var c dungeon.FreeController
			tick := 0
			dispatch := func([]data.Command) bool { return true }
			b.ReportAllocs()
			for b.Loop() {
				in := dungeon.FreeInput{}
				if moving {
					in.Forward = 1
					if tick%120 >= 60 {
						in.Forward = -1
					}
				}
				c.Update(db, st, in, dispatch)
				tick++
			}
		})
	}
}

func TestFreeSpaceOKNeedsExamine(t *testing.T) {
	for _, expire := range []bool{false, true} {
		t.Run(map[bool]string{false: "retry", true: "timeout"}[expire], func(t *testing.T) {
			g := wldOpenWith(t, Options{FS: freePlayFixture(t), Location: "inn"})
			wldFrames(t, g, 25)
			scene := sysAs[*dungeonScene](t, g.stack[0])
			before := *g.st.SpacePose
			s := sysScript(t, "press ok\nwait 1000")
			s.acknowledge = true
			n := 3
			if expire {
				n = 6002
			}
			for range n {
				sysKey(g)
				s.step(&g.in)
				scene.update(g)
				g.in.finishOffer()
				if s.pc != 0 || *g.st.SpacePose != before || g.talk.active {
					t.Fatal("idle examination acknowledged or acted")
				}
				if s.err != nil {
					break
				}
			}
			if expire {
				if s.err == nil || !strings.Contains(s.err.Error(), "acknowledgement") {
					t.Fatalf("idle examination did not expire: %v", s.err)
				}
				return
			}
			g.st.SpacePose.X, g.st.SpacePose.Z = 5.5, 7.5
			g.st.Pos.X, g.st.Pos.Y = 5, 7
			tile := dungeon.Front(g.db, g.st).Tile
			if tile == nil {
				t.Fatal("missing front tile")
			}
			tile.Trigger = "bump"
			tile.Do = []data.Command{{Say: &data.Say{Text: "A marker."}}}
			sysKey(g)
			s.step(&g.in)
			scene.update(g)
			g.in.finishOffer()
			if s.pc != 1 || !g.events.Running() {
				t.Fatal("pending OK did not start examination")
			}
		})
	}
}

func TestFreeSpaceHumanEmptyExamineResetsAcceleration(t *testing.T) {
	for _, click := range []bool{false, true} {
		t.Run(map[bool]string{false: "OK", true: "click"}[click], func(t *testing.T) {
			fixture := freePlayFixture(t)
			fixture["data/world.yaml"].Data = bytes.Replace(fixture["data/world.yaml"].Data,
				[]byte("acceleration: 0}"), []byte("acceleration: 0.1, stages: [{after: 0.05, speed: 3.2}]}"), 1)
			g := wldOpenWith(t, Options{FS: fixture, Location: "inn"})
			wldFrames(t, g, 25)
			g.in.script = nil
			scene := sysAs[*dungeonScene](t, g.stack[0])
			sysKey(g)
			g.in.held[btnUp] = true
			for range 10 {
				scene.updateFree(g)
			}
			before := g.st.SpacePose.Z
			g.in.pressed[btnOK], g.in.click = !click, click
			scene.updateFree(g)
			if distance := before - g.st.SpacePose.Z; distance <= 1.6/60 {
				t.Fatalf("fixture did not accelerate: distance %g", distance)
			}
			if g.events.Running() || g.talk.active {
				t.Fatal("fixture unexpectedly examined a target")
			}
			g.in.pressed[btnOK], g.in.click = false, false
			before = g.st.SpacePose.Z
			scene.updateFree(g)
			if distance := before - g.st.SpacePose.Z; math.Abs(distance-1.6/60) > 1e-8 {
				t.Fatalf("empty examination did not reset acceleration: distance %g, want %g", distance, 1.6/60)
			}
		})
	}
}

// TestFreeSpacePointerMoves: a free-move space must be walkable without a keyboard. A
// press held on the view (a finger, the left button) is the only pointer input there is.
func TestFreeSpacePointerMoves(t *testing.T) {
	g := wldOpenWith(t, Options{FS: freePlayFixture(t), Location: "inn"})
	wldFrames(t, g, 25)
	ds := mnuAs[*dungeonScene](t, g)
	before := *g.st.SpacePose
	sw, sh := g.screen()
	for _, at := range [][2]float64{{sw / 2, sh / 2}, {sw / 6, sh / 2}, {sw * 5 / 6, sh / 2}, {sw / 2, sh * 0.9}} {
		for i := range 20 {
			g.in = input{pheld: true, pdown: i == 0, click: i == 0, moved: i == 0}
			g.in.mouse.X, g.in.mouse.Y = int(at[0]), int(at[1])
			ds.update(g)
		}
	}
	if *g.st.SpacePose == before {
		t.Fatalf("no pointer input moves or turns the party in a free-move space: %+v", before)
	}
}
