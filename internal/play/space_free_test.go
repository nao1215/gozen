package play

import (
	"bytes"
	"math"
	"os"
	"strings"
	"testing"
	"testing/fstest"

	data "github.com/nao1215/gozen/internal/layer2/gamedata"
	"github.com/nao1215/gozen/internal/layer3/dungeon"
	"github.com/nao1215/gozen/internal/rpg"
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
	fixture := spaceFixtureFiles(t, os.DirFS("../../examples/space-town/game"))
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
