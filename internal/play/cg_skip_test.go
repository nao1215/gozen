package play

import (
	"fmt"
	"strconv"
	"testing"
	"testing/fstest"

	data "github.com/nao1215/gozen/internal/layer2/gamedata"
)

func TestSkippingSettlesPictureTransitions(t *testing.T) {
	for _, mode := range []string{"skip", "control", "replay"} {
		t.Run(mode, func(t *testing.T) {
			g := stagedGame(t, Options{})
			g.Image(data.CG{File: "x.png", Fade: 5})
			g.cgAge = g.cg.FadeTicks()
			switch mode {
			case "skip":
				g.toggleSkip()
			case "control":
				g.in.fast, g.cfg.SkipAll = true, true
			case "replay":
				g.live.quiet = true
			}
			// Each new image arrives before the previous fade could finish.
			for i := range 30 {
				g.Image(data.CG{File: "x.png", Bright: 1 + float64(i)/30, Fade: 5})
				if g.cgOldT != 0 || g.cgUnder.File != "" || g.cgAge < g.cg.FadeTicks() {
					t.Fatalf("%s image %d retained a transition: age=%d old=%d under=%q", mode, i, g.cgAge, g.cgOldT, g.cgUnder.File)
				}
			}
			g.Image(data.CG{})
			if g.cg.File != "" || g.cgOldT != 0 || g.cgUnder.File != "" {
				t.Fatalf("cleared image retained: current=%q old=%d under=%q", g.cg.File, g.cgOldT, g.cgUnder.File)
			}
		})
	}
}

func TestQuietReplayDoesNotLoadIntermediatePictures(t *testing.T) {
	g := stagedGame(t, Options{})
	fsys := gameFiles(t)
	for i := range 100 {
		fsys[fmt.Sprintf("assets/unshown-%d.png", i)] = &fstest.MapFile{Data: fsys["assets/x.png"].Data}
	}
	g.pics.fsys = fsys
	g.live.quiet = true
	for i := range 100 {
		g.Image(data.CG{File: fmt.Sprintf("unshown-%d.png", i)})
	}
	if len(g.pics.imgs) != 0 {
		t.Fatalf("quiet replay loaded %d unshown pictures", len(g.pics.imgs))
	}
	g.live.quiet = false
	if g.pics.get(g.cg.File) == missing || len(g.pics.imgs) != 1 {
		t.Fatal("the landing picture was not loaded on demand")
	}
}

func TestControlDoesNotSkipUnreadPictureTransition(t *testing.T) {
	g := stagedGame(t, Options{})
	g.talk.say.Text = "Unread."
	g.in.fast = true
	g.Image(data.CG{File: "x.png", Fade: 1})
	g.ageCG()
	if g.cgAge != 1 {
		t.Fatalf("unread line skipped transition: age %d", g.cgAge)
	}
}

func TestSkipFinishesAlreadyRunningPictureTransition(t *testing.T) {
	for _, same := range []bool{false, true} {
		t.Run(strconv.FormatBool(same), func(t *testing.T) {
			g := stagedGame(t, Options{})
			g.Image(data.CG{File: "x.png", Fade: 5})
			g.in.fast, g.cfg.SkipAll = true, true
			if same {
				g.Image(g.cg)
			} else {
				g.ageCG()
			}
			if g.cgAge < g.cg.FadeTicks() {
				t.Fatal("already running transition did not finish")
			}
		})
	}
}

func TestNormalPictureTransitionStillFades(t *testing.T) {
	g := stagedGame(t, Options{})
	g.Image(data.CG{File: "x.png", Fade: 1})
	g.cgAge = g.cg.FadeTicks()
	g.Image(data.CG{File: "x.png", Bright: 0.5, Fade: 1})
	if g.cgAge != 0 || g.cgUnder.File == "" {
		t.Fatal("normal transition was skipped")
	}
	for range g.cg.FadeTicks() {
		g.ageCG()
	}
	if g.cgUnder.File != "" || g.cgOldT != 0 {
		t.Fatal("normal transition did not finish")
	}
}
