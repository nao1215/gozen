package play

import (
	"slices"
	"testing"
	"testing/fstest"

	data "github.com/nao1215/gozen/internal/model/gamedata"
	"github.com/nao1215/gozen/internal/rules/event"
)

func lookGame(t *testing.T) *Game {
	t.Helper()
	fsys := gameFiles(t)
	fsys["game.yaml"] = &fstest.MapFile{Data: append([]byte("schema_version: 12\n"), fsys["game.yaml"].Data...)}
	g, err := New(Options{FS: fsys, NoSound: true, SaveDir: t.TempDir(), ShotDir: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := g.Close(); err != nil {
			t.Error(err)
		}
	})
	g.newGame()
	return g
}

// TestLookChanges: a weather or tone effect eases the screen to the state's look over its
// seconds; a look the state has without an effect (a load, a roll back) is shown at once;
// and the drops of the same play fall the same way (ADR 0066).
func TestLookChanges(t *testing.T) {
	g := lookGame(t)
	g.stepLook()
	if g.look.tone.now() != plainTone() || g.look.weather.power() != 0 {
		t.Fatalf("a new game without weather or tone looks %+v, power %v", g.look.tone.now(), g.look.weather.power())
	}
	g.st.SetWeather(&data.Weather{Kind: data.WeatherRain, Power: 0.8})
	g.st.SetTone(&data.Tone{Gray: 1})
	g.visual(event.VisualEffect{Name: "weather", Cue: data.EffectCue{Name: "weather", Seconds: 1}})
	g.visual(event.VisualEffect{Name: "tone", Cue: data.EffectCue{Name: "tone", Seconds: 0.5}})
	for range 15 {
		g.stepLook()
	}
	if p := g.look.weather.power(); p < 0.15 || p > 0.25 {
		t.Errorf("a quarter of the way the rain is at %v, want about 0.2", p)
	}
	if gray := g.look.tone.now().gray; gray < 0.45 || gray > 0.55 {
		t.Errorf("half way the gray is %v, want about 0.5", gray)
	}
	for range 60 {
		g.stepLook()
	}
	if g.look.weather.power() != 0.8 || g.look.tone.now() != (toneValue{mul: [3]float32{1, 1, 1}, gray: 1}) {
		t.Errorf("after the change: power %v, tone %+v", g.look.weather.power(), g.look.tone.now())
	}
	if len(g.look.weather.drops) == 0 {
		t.Error("no rain falls")
	}

	// The same play draws the same drops.
	var a, b weatherView
	for _, v := range []*weatherView{&a, &b} {
		v.change(&data.Weather{Kind: data.WeatherSnow, Power: 1, Wind: 0.4}, 0)
		for range 90 {
			v.step(1280, 720, 1)
		}
	}
	if len(a.drops) == 0 || !slices.Equal(a.drops, b.drops) {
		t.Errorf("two runs of the same play drop different snow (%d, %d drops)", len(a.drops), len(b.drops))
	}
	// Another kind: the slow snow thins out over the change instead of falling for seconds.
	a.change(&data.Weather{Kind: data.WeatherPetals, Power: 0.5}, 30)
	for range 30 {
		a.step(1280, 720, 1)
	}
	for _, d := range a.drops {
		if d.kind == data.WeatherSnow {
			t.Fatalf("snow is left %d ticks after the weather turned to petals", 30)
		}
	}

	// A loaded look without an effect is shown at once.
	g.st.SetTone(&data.Tone{Color: "#ff0000", Vignette: 1})
	g.st.SetWeather(nil)
	g.stepLook()
	if tone := g.look.tone.now(); tone.mul != [3]float32{1, 0, 0} || tone.vignette != 1 {
		t.Errorf("a loaded tone eased in: %+v", tone)
	}
	if g.look.weather.power() != 0 {
		t.Errorf("a loaded clear sky still rains at %v", g.look.weather.power())
	}
	for range 400 {
		g.stepLook()
	}
	if len(g.look.weather.drops) != 0 {
		t.Errorf("%d drops are left long after the rain stopped", len(g.look.weather.drops))
	}

	// The drops fall on the screen last drawn, in its pixels: sparks rise from its bottom
	// edge, not from below a screen scaled once more by the UI scale.
	g.look.weather.sw, g.look.weather.sh = 640, 360
	ui := g.ui
	g.ui = 1.5 // a 1080-high game
	t.Cleanup(func() { g.ui = ui })
	g.st.SetWeather(&data.Weather{Kind: data.WeatherEmbers, Power: 1})
	for range 10 {
		g.stepLook()
	}
	for _, d := range g.look.weather.drops {
		if d.fade == 0 && (d.y > 360+20*g.ui || d.x > 640+100*g.ui) {
			t.Fatalf("a spark at (%v, %v) is off a 640x360 screen", d.x, d.y)
		}
	}

	// The title (no state) has no look.
	g.st = nil
	g.stepLook()
	if g.look.tone.now() != plainTone() {
		t.Errorf("the title keeps a tone: %+v", g.look.tone.now())
	}
}
