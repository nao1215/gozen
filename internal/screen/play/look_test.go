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

	// The title (no state) has no look.
	g.st = nil
	g.stepLook()
	if g.look.tone.now() != plainTone() {
		t.Errorf("the title keeps a tone: %+v", g.look.tone.now())
	}
}
