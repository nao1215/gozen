package play

import (
	"image/color"
	"math"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"

	data "github.com/nao1215/gozen/internal/layer2/gamedata"
)

// btlCheckBatch reports a problem in the geometry an effect queued: a triangle must name
// vertices that exist, and every vertex must be finite with an alpha in 0..1.
func btlCheckBatch(b *fxBatch) string {
	if len(b.is)%3 != 0 {
		return "indices are not whole triangles"
	}
	for _, i := range b.is {
		if int(i) >= len(b.vs) {
			return "a triangle names a vertex that does not exist"
		}
	}
	for _, v := range b.vs {
		for _, f := range []float32{v.DstX, v.DstY, v.ColorR, v.ColorG, v.ColorB, v.ColorA} {
			if math.IsNaN(float64(f)) || math.IsInf(float64(f), 0) {
				return "a vertex is not finite"
			}
		}
		if v.ColorA < 0 || v.ColorA > 1 {
			return "an alpha is outside 0..1"
		}
	}
	return ""
}

// TestEffectGeometryIsSound: every effect kind, at every power, size and moment of its
// life, builds sound geometry; each kind draws something at some moment, and none draws
// anything for an unknown kind.
func TestEffectGeometryIsSound(t *testing.T) {
	kinds := append(append([]string{}, data.Fxs...), "cast")
	rects := []effectAnim{
		{x: 400, y: 150, w: 240, h: 240},
		{x: 0, y: 0, w: 10, h: 10},
		{x: 1100, y: 500, w: 600, h: 400},
	}
	for _, kind := range kinds {
		drew := false
		for _, r := range rects {
			for power := range 3 {
				for _, seed := range []uint32{1, 2654435761} {
					e := r
					e.kind, e.power, e.seed, e.dur = kind, power, seed, 40
					e.color = fxColor(kind, data.Color{})
					for tick := 0; tick <= e.dur; tick++ {
						e.t = tick
						s := &battleScene{}
						s.drawFx(nil, e)
						if msg := btlCheckBatch(&s.fxb); msg != "" {
							t.Fatalf("%s power %d tick %d: %s", kind, power, tick, msg)
						}
						drew = drew || len(s.fxb.is) > 0
					}
				}
			}
		}
		wantNothing := kind == "none"
		if drew == wantNothing {
			t.Errorf("%s: drew=%v", kind, drew)
		}
	}
	s := &battleScene{}
	s.drawFx(nil, effectAnim{kind: "no-such-effect", dur: 10, w: 100, h: 100})
	if len(s.fxb.is) != 0 {
		t.Fatal("an unknown effect kind drew triangles")
	}
}

// TestEffectBatchPrimitives: a shape that cannot be seen (no size, no alpha, no length)
// adds nothing, and one that can has the triangles of its shape.
func TestEffectBatchPrimitives(t *testing.T) {
	c := color.NRGBA{R: 200, G: 100, B: 50, A: 255}
	for name, draw := range map[string]func(*fxBatch){
		"glow without radius":     func(b *fxBatch) { b.glow(10, 10, 0, 0, c, 1) },
		"glow without alpha":      func(b *fxBatch) { b.glow(10, 10, 5, 0, c, 0) },
		"ring without radius":     func(b *fxBatch) { b.ring(10, 10, 0, 0, 3, c, 1) },
		"ring without alpha":      func(b *fxBatch) { b.ring(10, 10, 5, 0, 3, c, -1) },
		"line of no length":       func(b *fxBatch) { b.line(1, 1, 1, 1, 4, c, 1) },
		"line without alpha":      func(b *fxBatch) { b.line(1, 1, 9, 9, 4, c, 0) },
		"blade of no length":      func(b *fxBatch) { b.blade(1, 1, 1, 1, 4, c, 1) },
		"edge of no length":       func(b *fxBatch) { b.edge(1, 1, 1, 1, 4, c, 1) },
		"poly of two points":      func(b *fxBatch) { b.poly([][2]float64{{0, 0}, {1, 1}}, c, 1) },
		"poly without alpha":      func(b *fxBatch) { b.poly([][2]float64{{0, 0}, {1, 1}, {2, 0}}, c, 0) },
		"crystal without length":  func(b *fxBatch) { b.crystal(0, 0, 0, 0.5, 3, c, c, 1, 1) },
		"crescent without width":  func(b *fxBatch) { b.crescent(0, 0, 10, 0, 1, 0, 0, 1, c, 1, 0.5) },
		"crescent reversed":       func(b *fxBatch) { b.crescent(0, 0, 10, 0, 1, 4, 0.8, 0.2, c, 1, 0.5) },
		"splinter without size":   func(b *fxBatch) { b.splinter(0, 0, 0, 0, c, c, 1) },
		"flame without height":    func(b *fxBatch) { b.flame(0, 0, 0, 5, 0, c, 1) },
		"star without radius":     func(b *fxBatch) { b.star(0, 0, 0, 0.2, 0, 4, c, 1) },
		"star without alpha":      func(b *fxBatch) { b.star(0, 0, 9, 0.2, 0, 4, c, 0) },
		"fan without radius":      func(b *fxBatch) { b.fan(0, 0, -1, 0, c, 1, 0.5) },
		"polyFade without points": func(b *fxBatch) { b.polyFade(nil, c, 1, 1) },
	} {
		var b fxBatch
		draw(&b)
		if len(b.vs) != 0 || len(b.is) != 0 {
			t.Errorf("%s: %d vertices, %d indices", name, len(b.vs), len(b.is))
		}
	}
	var b fxBatch
	b.glow(10, 10, 5, 0, c, 2) // alpha is clamped to 1
	if len(b.vs) != 21 || len(b.is) != 60 {
		t.Fatalf("glow: %d vertices %d indices", len(b.vs), len(b.is))
	}
	if b.vs[0].ColorA != 1 {
		t.Fatalf("alpha %v not clamped", b.vs[0].ColorA)
	}
	b.reset()
	if len(b.vs) != 0 || len(b.is) != 0 {
		t.Fatal("reset left geometry")
	}
	b.ring(0, 0, 10, 0, 4, c, 1)
	if len(b.vs) != 84 || len(b.is) != 28*12 {
		t.Fatalf("ring: %d vertices %d indices", len(b.vs), len(b.is))
	}
	b.reset()
	b.vs = make([]ebiten.Vertex, 60001)
	if !b.full() {
		t.Fatal("a batch of 60001 vertices is not full")
	}
}

// TestEffectHelpers: the small functions the effects share keep to their ranges.
func TestEffectHelpers(t *testing.T) {
	if clamp01(-3) != 0 || clamp01(7) != 1 || clamp01(0.25) != 0.25 {
		t.Fatal("clamp01")
	}
	prev := -1.0
	for i := range 21 {
		v := easeOut(float64(i) / 20)
		if v < prev || v < 0 || v > 1 {
			t.Fatalf("easeOut(%d/20) = %v after %v", i, v, prev)
		}
		prev = v
	}
	if easeOut(-1) != 0 || easeOut(2) != 1 {
		t.Fatal("easeOut outside 0..1")
	}
	a, b := color.NRGBA{0, 0, 0, 255}, color.NRGBA{200, 100, 50, 255}
	if mixColor(a, b, 0) != a || mixColor(a, b, 1) != b || mixColor(a, b, 9) != b || mixColor(a, b, -9) != a {
		t.Fatal("mixColor endpoints")
	}
	if got := mixColor(a, b, 0.5); got.R != 100 || got.G != 50 || got.B != 25 {
		t.Fatalf("mixColor middle %v", got)
	}
	if lighten(b, 1) != colorWhite || lighten(b, 0) != b {
		t.Fatal("lighten")
	}
	for seed := range uint32(50) {
		for i := range 20 {
			if v := rnd(seed*77, i); v < 0 || v >= 1 {
				t.Fatalf("rnd = %v", v)
			}
		}
	}
	if rnd(5, 1) != rnd(5, 1) || rnd(5, 1) == rnd(5, 2) {
		t.Fatal("rnd is not a function of its inputs")
	}
	for _, k := range []string{"slash", "pierce", "strike"} {
		if !physicalFx(k) {
			t.Errorf("%s is not physical", k)
		}
	}
	for _, k := range []string{"fire", "heal", "none", ""} {
		if physicalFx(k) {
			t.Errorf("%s is physical", k)
		}
	}
	for _, k := range []string{"slash", "strike", "fire", "ice", "thunder", "holy", "heal"} {
		if h := fxHit(k); h <= 0 || h >= 1 {
			t.Errorf("fxHit(%s) = %v", k, h)
		}
	}
	if fxHit("buff") != 0 || fxHit("nonsense") != 0 {
		t.Fatal("a kind without an impact moment has a hit time")
	}
	custom := data.Color{Valid: true, NRGBA: color.NRGBA{1, 2, 3, 255}}
	if fxColor("fire", custom) != custom.NRGBA {
		t.Fatal("the skill's own color is not used")
	}
	if fxColor("fire", data.Color{}) != fxDefault["fire"] || fxColor("nonsense", data.Color{}) != colorWhite {
		t.Fatal("default colors")
	}
}

// TestFxKindFollowsTheSkill: the drawn effect is the skill's own fx, or none for anim none,
// or the element of its damage, else from its anim and scope.
func TestFxKindFollowsTheSkill(t *testing.T) {
	for _, tc := range []struct {
		name string
		use  *data.Usage
		want string
	}{
		{"no usage", nil, "slash"},
		{"own fx", &data.Usage{Fx: "holy", Anim: "slash"}, "holy"},
		{"anim none", &data.Usage{Anim: "none"}, "none"},
		{"element", &data.Usage{Effects: []data.Effect{{Element: "ice"}}}, "ice"},
		{"element not drawn", &data.Usage{Effects: []data.Effect{{Element: "poison"}}}, "slash"},
		{"magic", &data.Usage{Anim: "magic"}, "magic"},
		{"heal", &data.Usage{Anim: "heal"}, "heal"},
		{"debuff", &data.Usage{Anim: "debuff"}, "debuff"},
		{"buff on friends", &data.Usage{Anim: "buff", Scope: "one_ally"}, "buff"},
		{"buff on foes is a debuff", &data.Usage{Anim: "buff", Scope: "all_enemies"}, "debuff"},
		{"plain", &data.Usage{Scope: "one_enemy"}, "slash"},
	} {
		if got := fxKind(tc.use); got != tc.want {
			t.Errorf("%s: %s, want %s", tc.name, got, tc.want)
		}
	}
}

// TestAddFxLifecycle: an effect lasts for the time addFx returns, then goes; fast-forward
// shortens it; a weaker effect is shorter; "none" adds nothing; screen flashes last at
// least six ticks.
func TestAddFxLifecycle(t *testing.T) {
	g, _ := btlOpen(t, Options{Battle: "one"}, "", "")
	s := btlBattle(t, g)
	s.fx, s.scr = nil, nil
	if d := s.addFx(g, "none", 0, 0, 10, 10, colorWhite, 1); d != 0 || len(s.fx) != 0 {
		t.Fatalf("none: %d %d", d, len(s.fx))
	}
	if d := s.addFx(g, "", 0, 0, 10, 10, colorWhite, 1); d != 0 {
		t.Fatalf("empty kind: %d", d)
	}
	normal := s.addFx(g, "fire", 10, 10, 100, 100, colorWhite, 1)
	weak := s.addFx(g, "fire", 10, 10, 100, 100, colorWhite, 0)
	strong := s.addFx(g, "fire", 10, 10, 100, 100, colorWhite, 2)
	unknown := s.addFx(g, "mystery", 10, 10, 100, 100, colorWhite, 1)
	if weak >= normal || normal >= strong || unknown <= 0 {
		t.Fatalf("durations weak %d normal %d strong %d unknown %d", weak, normal, strong, unknown)
	}
	if s.fx[0].seed == s.fx[1].seed {
		t.Fatal("two effects share a random seed")
	}
	g.in.fast = true
	quick := s.addFx(g, "fire", 10, 10, 100, 100, colorWhite, 1)
	g.in.fast = false
	if quick >= normal {
		t.Fatalf("fast-forward effect %d not shorter than %d", quick, normal)
	}
	s.addScreen(g, "flash", 0, 0, colorWhite, 0.3, 1)
	if s.scr[0].dur < 6 {
		t.Fatalf("screen effect lasts %d ticks", s.scr[0].dur)
	}
	for range strong + 2 {
		s.stepFx()
	}
	if len(s.fx) != 0 || len(s.scr) != 0 {
		t.Fatalf("effects left after their time: %d %d", len(s.fx), len(s.scr))
	}
}

// TestScreenEffectsDrawn: the flash and the wave across the screen are drawn through their
// whole life without leaving geometry behind.
func TestScreenEffectsDrawn(t *testing.T) {
	g, _ := btlOpen(t, Options{Battle: "one"}, "", "")
	s := btlBattle(t, g)
	s.addScreen(g, "flash", 0, 0, colorWhite, 0.5, 20)
	s.addScreen(g, "wave", 640, 300, fxColor("fire", data.Color{}), 0.7, 34)
	s.addFx(g, "slash", 300, 200, 200, 200, fxColor("slash", data.Color{}), 2)
	for range 200 {
		s.drawEffects(g, btlImage())
		s.stepFx()
		if len(s.fxb.vs) != 0 {
			t.Fatal("geometry left in the batch after a draw")
		}
		if len(s.scr) == 0 && len(s.fx) == 0 {
			break
		}
	}
	if len(s.scr) != 0 || len(s.fx) != 0 {
		t.Fatalf("effects remain: %d %d", len(s.scr), len(s.fx))
	}
}
