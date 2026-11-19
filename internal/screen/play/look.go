package play

import (
	"image/color"
	"math"
	"math/rand/v2"

	"github.com/hajimehoshi/ebiten/v2"

	data "github.com/nao1215/gozen/internal/model/gamedata"
	"github.com/nao1215/gozen/internal/model/rpg"
)

// sceneLook draws the weather and the tone the state's look holds (ADR 0066) over the place
// and its pictures, under the windows. The state keeps only what the commands left; how far
// a change has come, and the drops falling, are the screen's own.
type sceneLook struct {
	weather weatherView
	tone    toneView
	// seen is the look last taken from the state: a different one that came without a
	// weather or tone effect (a load, a roll back, a new game) is shown at once.
	seen rpg.Look
}

// toneValue is a tone as the shader takes it: white Mul and zero parts draw the scene as it is.
type toneValue struct {
	mul                    [3]float32
	gray, bright, vignette float32
}

// plainTone draws the scene as it is.
func plainTone() toneValue { return toneValue{mul: [3]float32{1, 1, 1}} }

func toneOf(t *data.Tone) toneValue {
	if t == nil {
		return plainTone()
	}
	v := toneValue{mul: [3]float32{1, 1, 1}, gray: float32(t.Gray), bright: float32(t.Bright), vignette: float32(t.Vignette)}
	if c, ok := lookColor(t.Color); ok {
		v.mul = [3]float32{float32(c.R) / 255, float32(c.G) / 255, float32(c.B) / 255}
	}
	return v
}

func (v toneValue) lerp(to toneValue, f float32) toneValue {
	mix := func(a, b float32) float32 { return a + (b-a)*f }
	return toneValue{
		mul:  [3]float32{mix(v.mul[0], to.mul[0]), mix(v.mul[1], to.mul[1]), mix(v.mul[2], to.mul[2])},
		gray: mix(v.gray, to.gray), bright: mix(v.bright, to.bright), vignette: mix(v.vignette, to.vignette),
	}
}

// toneView eases from one tone to the next over length ticks. Before its first change it
// is the scene as drawn (the zero toneValue would multiply the scene by black).
type toneView struct {
	from, to    toneValue
	set         bool
	age, length int
	shader      *ebiten.Shader
	copy        *ebiten.Image
}

func (t *toneView) now() toneValue {
	if !t.set {
		return plainTone()
	}
	if t.length <= 0 || t.age >= t.length {
		return t.to
	}
	return t.from.lerp(t.to, float32(t.age)/float32(t.length))
}

func (t *toneView) change(to toneValue, ticks int) {
	t.from, t.to, t.age, t.length, t.set = t.now(), to, 0, ticks, true
}

// weatherDrop is one drop, flake, petal or spark, in pixels of the screen.
type weatherDrop struct {
	kind         string
	x, y, vx, vy float64
	spin, turn   float64
	size, life   float64
	age          int
	tint         color.NRGBA
}

// weatherView keeps the drops falling. power eases toward the weather's; a change of kind
// lets the drops of the old one fall out while the new one comes in.
type weatherView struct {
	w                          data.Weather
	tint                       color.NRGBA
	tinted                     bool
	from, to                   float64
	age, length                int
	drops                      []weatherDrop
	rng                        *rand.Rand
	carry                      float64
	streak, flake, petal, glow *ebiten.Image
}

// maxDrops bounds the drops on the screen, whatever the power.
const maxDrops = 900

func (v *weatherView) power() float64 {
	if v.length <= 0 || v.age >= v.length {
		return v.to
	}
	return v.from + (v.to-v.from)*float64(v.age)/float64(v.length)
}

func (v *weatherView) change(w *data.Weather, ticks int) {
	from := v.power()
	if w == nil {
		v.from, v.to, v.age, v.length = from, 0, 0, ticks
		return
	}
	if w.Kind != v.w.Kind {
		from = 0 // the drops of the old kind fall out by themselves
	}
	v.w, v.from, v.to, v.age, v.length = *w, from, w.Power, 0, ticks
	v.tint, v.tinted = lookColor(w.Color)
}

// step moves the drops one tick across a screen of w by h pixels at scale k, and lets new
// ones come in at the weather's power. The drops come from a generator of their own with a
// fixed seed, so a capture of the same play draws the same drops.
func (v *weatherView) step(w, h, k float64) {
	if v.rng == nil {
		v.rng = rand.New(rand.NewPCG(0x77656174, 0x68657221)) // #nosec G404 -- Where drops fall, reproducible on purpose.
		v.drops = make([]weatherDrop, 0, maxDrops)
	}
	if v.age < v.length {
		v.age++
	}
	kept := v.drops[:0]
	for _, d := range v.drops {
		d.age++
		d.x += d.vx
		d.y += d.vy
		d.spin += d.turn
		if d.kind == data.WeatherPetals || d.kind == data.WeatherSnow {
			d.x += math.Sin(float64(d.age)*0.04+d.spin) * 0.6 * k
		}
		gone := d.y > h+20*k || d.y < -40*k || d.x < -60*k || d.x > w+60*k || d.life > 0 && float64(d.age) > d.life
		if !gone {
			kept = append(kept, d)
		}
	}
	v.drops = kept
	p := v.power()
	if p <= 0 || v.w.Kind == "" {
		return
	}
	rate := map[string]float64{data.WeatherRain: 9, data.WeatherSnow: 2.2, data.WeatherPetals: 0.9, data.WeatherEmbers: 1.4}[v.w.Kind]
	v.carry += rate * p * w / 1280
	for ; v.carry >= 1 && len(v.drops) < maxDrops; v.carry-- {
		v.drops = append(v.drops, v.newDrop(w, h, k))
	}
	v.carry = math.Min(v.carry, 1)
}

func (v *weatherView) newDrop(w, h, k float64) weatherDrop {
	r := v.rng.Float64
	wind := v.w.Wind
	d := weatherDrop{kind: v.w.Kind, x: r()*(w+200*k) - 100*k, y: -20 * k, spin: r() * math.Pi * 2}
	switch v.w.Kind {
	case data.WeatherRain:
		d.vy = (14 + 6*r()) * k
		d.vx = wind * 6 * k
		d.size = 0.7 + 0.6*r()
		d.tint = color.NRGBA{200, 215, 235, 150}
	case data.WeatherSnow:
		d.vy = (0.8 + 1.2*r()) * k
		d.vx = wind * 1.5 * k
		d.size = 0.4 + 0.8*r()
		d.tint = color.NRGBA{255, 255, 255, 220}
	case data.WeatherPetals:
		d.vy = (0.9 + 0.9*r()) * k
		d.vx = (wind*1.8 + 0.4) * k
		d.turn = (r() - 0.5) * 0.08
		d.size = 0.6 + 0.6*r()
		d.tint = color.NRGBA{255, 190, 205, 235}
	case data.WeatherEmbers:
		d.y = h + 10*k
		d.vy = -(0.8 + 1.4*r()) * k
		d.vx = (wind*1.5 + (r()-0.5)*0.6) * k
		d.size = 0.4 + 0.7*r()
		d.life = 120 + 160*r()
		d.tint = color.NRGBA{255, 170, 70, 255}
	}
	if v.tinted {
		d.tint.R, d.tint.G, d.tint.B = v.tint.R, v.tint.G, v.tint.B
	}
	return d
}

// pictures makes the small pictures the drops are drawn from, once.
func (v *weatherView) pictures() {
	if v.streak != nil {
		return
	}
	v.streak = ebiten.NewImage(2, 24)
	v.streak.Fill(color.White)
	round := func(size int, edge float64) *ebiten.Image {
		img := ebiten.NewImage(size, size)
		px := make([]byte, 4*size*size)
		c := float64(size-1) / 2
		for y := range size {
			for x := range size {
				d := math.Hypot(float64(x)-c, float64(y)-c) / c
				a := byte(255 * math.Max(0, math.Min(1, (1-d)/edge)))
				i := 4 * (y*size + x)
				px[i], px[i+1], px[i+2], px[i+3] = a, a, a, a
			}
		}
		img.WritePixels(px)
		return img
	}
	v.flake = round(12, 0.5)
	v.glow = round(16, 1)
	v.petal = round(14, 0.25)
}

func (v *weatherView) draw(dst *ebiten.Image, k float64) {
	if len(v.drops) == 0 {
		return
	}
	v.pictures()
	op := &ebiten.DrawImageOptions{}
	for _, d := range v.drops {
		op.GeoM.Reset()
		op.ColorScale.Reset()
		op.Blend = ebiten.BlendSourceOver
		img := v.flake
		switch d.kind {
		case data.WeatherRain:
			img = v.streak
			op.GeoM.Translate(-1, -12)
			op.GeoM.Scale(d.size*k, d.size*k)
			op.GeoM.Rotate(-math.Atan2(d.vx, d.vy))
		case data.WeatherSnow:
			op.GeoM.Translate(-6, -6)
			op.GeoM.Scale(d.size*k, d.size*k)
		case data.WeatherPetals:
			img = v.petal
			op.GeoM.Translate(-7, -7)
			op.GeoM.Scale(d.size*k, d.size*k*(0.35+0.3*math.Abs(math.Sin(d.spin))))
			op.GeoM.Rotate(d.spin)
		case data.WeatherEmbers:
			img = v.glow
			op.GeoM.Translate(-8, -8)
			op.GeoM.Scale(d.size*k, d.size*k)
			op.Blend = ebiten.BlendLighter
			if d.life > 0 {
				op.ColorScale.ScaleAlpha(float32(math.Max(0, 1-float64(d.age)/d.life)))
			}
		}
		op.GeoM.Translate(d.x, d.y)
		op.ColorScale.ScaleWithColor(d.tint)
		dst.DrawImage(img, op)
	}
}

// changeLook starts the change to the state's look that a weather or tone effect told,
// over ticks.
func (g *Game) changeLook(name string, ticks int) {
	if g.st == nil {
		return
	}
	now := lookOf(g.st)
	switch name {
	case "weather":
		g.look.weather.change(now.Weather, ticks)
		g.look.seen.Weather = now.Weather
	case "tone":
		g.look.tone.change(toneOf(now.Tone), ticks)
		g.look.seen.Tone = now.Tone
	}
}

func lookOf(st *rpg.State) rpg.Look {
	if st == nil || st.Look == nil {
		return rpg.Look{}
	}
	return *st.Look
}

// stepLook moves the look one tick: a look the state has without an effect telling the
// change (a load, a roll back, a new game) is shown at once.
func (g *Game) stepLook() {
	now := lookOf(g.st)
	if now.Weather != g.look.seen.Weather {
		g.look.weather.change(now.Weather, 0)
		g.look.seen.Weather = now.Weather
	}
	if now.Tone != g.look.seen.Tone {
		g.look.tone.change(toneOf(now.Tone), 0)
		g.look.seen.Tone = now.Tone
	}
	if g.look.tone.age < g.look.tone.length {
		g.look.tone.age++
	}
	w, h := g.screenPixels()
	g.look.weather.step(w, h, g.ui)
}

// screenPixels is the size of the screen drawn, in pixels.
func (g *Game) screenPixels() (float64, float64) {
	return float64(g.db.Game.Screen.Width) * g.ui, float64(g.db.Game.Screen.Height) * g.ui
}

// drawLook colors the scene drawn so far with the tone, then draws the weather over it. A
// battle has neither: its own screen is not the place's.
func (g *Game) drawLook(dst *ebiten.Image) {
	for _, s := range g.stack {
		if _, ok := s.(*battleScene); ok {
			return
		}
	}
	if t := g.look.tone.now(); t != plainTone() {
		g.drawTone(dst, t)
	}
	g.look.weather.draw(dst, g.ui)
}

func (g *Game) drawTone(dst *ebiten.Image, t toneValue) {
	v := &g.look.tone
	if v.shader == nil {
		s, err := ebiten.NewShader([]byte(toneShader))
		if err != nil {
			panic(err)
		}
		v.shader = s
	}
	b := dst.Bounds()
	v.copy = sized(v.copy, b.Dx(), b.Dy())
	v.copy.Clear()
	v.copy.DrawImage(dst, nil)
	op := &ebiten.DrawRectShaderOptions{Blend: ebiten.BlendCopy}
	op.Images[0] = v.copy
	op.Uniforms = map[string]any{
		"Mul":      t.mul[:],
		"Gray":     t.gray,
		"Bright":   t.bright,
		"Vignette": t.vignette,
		"Size":     []float32{float32(b.Dx()), float32(b.Dy())},
	}
	dst.DrawRectShader(b.Dx(), b.Dy(), v.shader, op)
}

// toneShader takes the color away (Gray), multiplies by Mul, darkens toward black or
// lightens toward white (Bright) and darkens the edges (Vignette), on premultiplied colors.
const toneShader = `//kage:unit pixels
package main

var Mul vec3
var Gray float
var Bright float
var Vignette float
var Size vec2

func Fragment(dst vec4, src vec2, color vec4) vec4 {
	c := imageSrc0At(src)
	rgb := c.rgb
	l := dot(rgb, vec3(0.299, 0.587, 0.114))
	rgb = mix(rgb, vec3(l), Gray)
	rgb *= Mul
	if Bright < 0 {
		rgb *= 1 + Bright
	} else {
		rgb = mix(rgb, vec3(c.a), Bright)
	}
	p := (dst.xy - imageDstOrigin()) / Size
	d := distance(p, vec2(0.5)) * 1.4142
	rgb *= 1 - Vignette*smoothstep(0.35, 1.0, d)
	return vec4(rgb, c.a)
}
`

// lookColor reads a #rrggbb the state keeps (false for none).
func lookColor(s string) (color.NRGBA, bool) {
	var c data.Color
	if s == "" || c.UnmarshalText([]byte(s)) != nil {
		return color.NRGBA{}, false
	}
	return c.NRGBA, true
}
