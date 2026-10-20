package play

import (
	"testing"
	"testing/fstest"

	"github.com/nao1215/gozen/internal/event"
	data "github.com/nao1215/gozen/internal/layer2/gamedata"
)

const stagedData = `events:
  staged:
    do:
      - stand: {who: Courier, face: x.png, at: left}
      - say: {who: Courier, face: x.png, text: "Hello."}
      - stand: {who: Guard, face: x.png, at: right, enter: slide}
      - say: {who: Guard, face: x.png, text: "Halt."}
      - stand: none
      - say: "Quiet."
  held:
    do:
      - say: "Before."
      - wait: {seconds: 2, skip: false}
      - say: "After."
`

func stagedGame(t *testing.T, opts Options) *Game {
	t.Helper()
	fsys := gameFiles(t)
	fsys["game.yaml"] = &fstest.MapFile{Data: append([]byte("schema_version: 4\nui: {message: novel}\n"), fsys["game.yaml"].Data...)}
	fsys["data/staged.yaml"] = &fstest.MapFile{Data: []byte(stagedData)}
	opts.FS, opts.NoSound, opts.SaveDir, opts.ShotDir = fsys, true, t.TempDir(), t.TempDir()
	g, err := New(opts)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := g.Close(); err != nil {
			t.Error(err)
		}
	})
	return g
}

// TestStandPlacesPortraits: an event with stand places its portraits as written: the speaker
// lit, the others dimmed, none taking everyone off; the end of the event clears them.
func TestStandPlacesPortraits(t *testing.T) {
	g := stagedGame(t, Options{Location: "camp", Event: "staged", Script: "wait 30\npress ok\nwait 30\nquit\n"})
	runToEnd(t, g)
	if at(g) != "Halt." || !g.castOn {
		t.Fatalf("at %q, cast on %v", at(g), g.castOn)
	}
	figs := g.castFigures("name:Guard")
	if len(figs) != 2 || figs[0].id != "name:Courier" || figs[0].dim == 0 || figs[1].dim != 0 {
		t.Fatalf("figures %+v", figs)
	}
	sw, _ := g.screen()
	if figs[0].x != sw*0.25 || figs[1].x != sw*0.75 {
		t.Fatalf("places %v and %v", figs[0].x/sw, figs[1].x/sw)
	}
	g.stand(event.StandEffect{None: true})
	g.tick += castOut
	if figs := g.castFigures(""); len(figs) != 0 {
		t.Fatalf("after none: %+v", figs)
	}
	g.clearCast()
	if g.castOn {
		t.Fatal("the cast stayed on")
	}
}

// TestEffectMaps: a map effect shakes one portrait, fades through a color, and a reaction
// word does nothing to a portrait that does not move.
func TestEffectMaps(t *testing.T) {
	g := stagedGame(t, Options{})
	g.visual(event.VisualEffect{Name: "shake", Cue: data.EffectCue{Name: "shake", Who: "Courier", Seconds: 0.5}})
	if g.figShake["name:Courier"] != 30 || g.shake != 0 {
		t.Fatalf("shake %v, screen %d", g.figShake, g.shake)
	}
	red := data.Color{Valid: true}
	red.R = 0xff
	g.visual(event.VisualEffect{Name: "fade", Cue: data.EffectCue{Name: "fade", Color: red, Seconds: 0.5, Hold: 0.25}})
	if f := g.fadeColor; f == nil || f.out != 30 || f.hold != 15 || f.c.R != 0xff || f.c.A != 0xff {
		t.Fatalf("fade %+v", g.fadeColor)
	}
	for range 2*30 + 15 {
		g.stepStaging()
	}
	if g.fadeColor != nil || len(g.figShake) != 0 {
		t.Fatalf("not over: %+v %v", g.fadeColor, g.figShake)
	}
	g.visual(event.VisualEffect{Name: "nod", Cue: data.EffectCue{Name: "nod"}})
}

// TestWaitThatSkippingHolds: skip mode passes the lines read but not a wait with skip: false.
func TestWaitThatSkippingHolds(t *testing.T) {
	g := stagedGame(t, Options{Location: "camp", Event: "held", Script: "wait 10\npress skip\nwait 40\nquit\n"})
	g.read = map[uint64]bool{source("Before."): true, source("After."): true}
	runToEnd(t, g)
	if _, ok := g.req.(event.Wait); !ok || !g.skipping || g.waitT <= 0 {
		t.Fatalf("request %T, skipping %v, wait %d", g.req, g.skipping, g.waitT)
	}
}
