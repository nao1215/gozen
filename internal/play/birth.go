package play

import (
	"image/color"
	"math"

	"github.com/hajimehoshi/ebiten/v2"

	"github.com/nao1215/gozen/internal/event"
	"github.com/nao1215/gozen/internal/layer5/gfx"
)

const birthDuration = 270 // 4.5 seconds at 60 updates per second.

type birthAnimation struct {
	effect event.BreedEffect
	age    int
}

func (b *birthAnimation) duration() int { return birthDuration + birthCutInFrames + 30*b.effect.Tier }

func (b *birthAnimation) flashStart() int { return 168 + 10*b.effect.Tier }
func (b *birthAnimation) childStart() int { return 200 + birthCutInFrames + 20*b.effect.Tier }

// birthCutInFrames is how long the child's silhouette sweeps across before it appears.
const birthCutInFrames = 50

// Figures at the altar are shown from the waist up: tall pictures whose feet
// fall below the screen.
const (
	birthHeight = 1.25
	birthBottom = 1.32
)

type birthPhase uint8

const (
	birthParents birthPhase = iota
	birthSpiral
	birthFlash
	birthChild
)

func (b *birthAnimation) phase() birthPhase {
	switch {
	case b.age < 65:
		return birthParents
	case b.age < b.flashStart():
		return birthSpiral
	case b.age < b.childStart():
		return birthFlash
	default:
		return birthChild
	}
}

type birthPoint struct{ x, y float64 }

func birthPositions(sw, sh float64) (parents [2]birthPoint, crystal birthPoint) {
	parents = [2]birthPoint{{sw * 0.2, sh * 0.76}, {sw * 0.8, sh * 0.76}}
	crystal = birthPoint{sw * 0.5, sh * 0.53}
	return
}

func birthOrbPosition(sw, sh float64, parent, age, end int) birthPoint {
	_, crystal := birthPositions(sw, sh)
	progress := min(1, float64(age-65)/float64(end-65))
	parents, _ := birthPositions(sw, sh)
	start := parents[parent]
	start.y -= sh * 0.1
	return birthPoint{
		x: start.x + (crystal.x-start.x)*progress,
		y: start.y + (crystal.y-start.y)*progress - math.Sin(progress*math.Pi)*sh*0.08,
	}
}

func (g *Game) drawBirth(dst *ebiten.Image) {
	b := g.birth
	if b == nil {
		return
	}
	sw, sh := g.screen()
	if loc := g.db.Locations[g.st.Location]; loc != nil {
		g.drawPicture(dst, loc.Background, 0, 0, sw, sh, 1)
	}
	parents, crystal := birthPositions(sw, sh)
	white := color.NRGBA{255, 255, 255, 255}
	gold := color.NRGBA{255, 216, 126, 255}
	age := b.age
	phase := b.phase()
	light := white
	if b.effect.Tier == 1 {
		light = gold
	}
	if b.effect.Tier == 2 {
		light = rainbowColor(age)
	}
	// Yorn stands at the center and steps into the glow as the child appears.
	if phase != birthChild {
		if loc := g.db.Locations[g.st.Location]; loc != nil {
			for _, person := range loc.People {
				if person.Image != "" {
					g.drawStanding(dst, person.Image, sw*0.5, sh*birthBottom, sh*birthHeight, 1, false, nil)
					break
				}
			}
		}
		if phase >= birthSpiral {
			// A pair of rising hands marks the start of the transformation.
			for _, side := range []float64{-1, 1} {
				gfx.StrokeLine(dst, float32(sw*0.5+side*sw*0.045), float32(sh*0.67), float32(sw*0.5+side*sw*0.095), float32(sh*0.48), 5, withAlpha(light, 0xb0), true)
			}
		}
	}
	// The center grows brighter before the parents become light.
	if phase != birthChild {
		glow := 12 + 12*math.Sin(float64(age)*0.15)
		if age >= 60 {
			glow += 15
		}
		gfx.FillCircle(dst, float32(crystal.x), float32(crystal.y), float32(glow+12), withAlpha(light, 0x48), true)
		gfx.FillCircle(dst, float32(crystal.x), float32(crystal.y), float32(max(5, glow*0.48)), withAlpha(white, 0xd0), true)
	}
	for i, p := range parents {
		if phase >= birthFlash {
			break
		}
		if phase == birthParents {
			glow := uint8(45 + age*2)
			gfx.FillCircle(dst, float32(p.x), float32(p.y-sh*0.24), float32(sh*0.16), withAlpha(light, glow), true)
			g.drawStanding(dst, b.effect.ParentFaces[i], p.x, sh*birthBottom, sh*birthHeight*0.85, 1, false, nil)
			g.window(dst, p.x-sw*0.115, p.y+8, sw*0.23, 44)
			g.text(dst, b.effect.ParentNames[i], p.x-sw*0.105, p.y+13, sizeS, g.th.text)
			continue
		}
		// Each picture contracts into its light before the two lights orbit.
		if age < 110 {
			shrink := float64(age-65) / 45
			h := sh * birthHeight * 0.85 * (1 - shrink)
			g.drawStanding(dst, b.effect.ParentFaces[i], p.x, sh*birthBottom-(sh*birthBottom-p.y)*shrink, h, float32(1-shrink), false, nil)
		}
		progress := min(1, float64(age-65)/float64(b.flashStart()-65))
		orb := birthOrbPosition(sw, sh, i, age, b.flashStart())
		r := float32(4 + (1-progress)*14)
		gfx.FillCircle(dst, float32(orb.x), float32(orb.y), r*2.3, withAlpha(light, 0x68), true)
		gfx.FillCircle(dst, float32(orb.x), float32(orb.y), r, white, true)
	}
	if phase == birthFlash {
		strength := 1 - math.Abs(float64(age-(b.flashStart()+b.childStart())/2))/float64(b.childStart()-b.flashStart())*2
		if strength > 0 {
			rays := 12 + b.effect.Tier*12
			for i := 0; i < rays; i++ {
				a := float64(i) * math.Pi * 2 / float64(rays)
				length := strength * sh * 0.54
				gfx.StrokeLine(dst, float32(crystal.x), float32(crystal.y), float32(crystal.x+math.Cos(a)*length), float32(crystal.y+math.Sin(a)*length), 3, withAlpha(light, 0xc0), true)
			}
			gfx.FillCircle(dst, float32(crystal.x), float32(crystal.y), float32(strength*sh*0.38), withAlpha(light, uint8(strength*245)), true)
		}
	}
	if phase != birthChild {
		if age >= b.childStart()-birthCutInFrames {
			g.drawBirthCutIn(dst)
		}
		return
	}
	g.drawStanding(dst, b.effect.ChildFace, sw*0.5, sh*birthBottom, sh*birthHeight, 1, false, nil)
	for i := 0; i < 24+b.effect.Tier*12; i++ {
		a := float64(i)*math.Pi/12 + float64(age-200)*0.02
		r := sh * (0.15 + 0.07*math.Sin(float64(i)*3+float64(age)*0.08))
		x, y := sw*0.5+math.Cos(a)*r, sh*0.47+math.Sin(a)*r
		gfx.FillCircle(dst, float32(x), float32(y), 2+float32(i%3), light, true)
	}
	if age < b.childStart()+25 {
		strength := 1 - float64(age-b.childStart())/25
		for i := 0; i < 12+b.effect.Tier*12; i++ {
			a := float64(i) * math.Pi * 2 / float64(12+b.effect.Tier*12)
			gfx.StrokeLine(dst, float32(crystal.x), float32(crystal.y), float32(crystal.x+math.Cos(a)*sh*0.45*strength), float32(crystal.y+math.Sin(a)*sh*0.45*strength), 3, withAlpha(light, 0xe0), true)
		}
	}
	bannerW := min(sw*0.6, 600.0)
	g.window(dst, (sw-bannerW)/2, sh*0.07, bannerW, 56)
	label := b.effect.ChildName + " was born!"
	g.text(dst, label, (sw-gfx.TextWidth(label, sizeL))/2, sh*0.07+12, sizeL, g.th.accent)
}

func rainbowColor(age int) color.NRGBA {
	colors := [...]color.NRGBA{{255, 92, 112, 255}, {255, 209, 86, 255}, {107, 236, 151, 255}, {103, 191, 255, 255}, {212, 130, 255, 255}}
	return colors[(age/8)%len(colors)]
}

// drawBirthCutIn uses the battle skill cut-in's darkened band, speed lines and
// picture sweep for a legendary child.
func (g *Game) drawBirthCutIn(dst *ebiten.Image) {
	b := g.birth
	sw, sh := g.screen()
	t := b.age - (b.childStart() - birthCutInFrames)
	open := min(1, float64(t)/8)
	if t > 40 {
		open = max(0, float64(50-t)/10)
	}
	if open <= 0 {
		return
	}
	accent := rainbowColor(b.age)
	gfx.FillRect(dst, 0, 0, float32(sw), float32(sh), withAlpha(color.NRGBA{0, 0, 0, 255}, uint8(100*open)), false)
	for i := range 14 {
		y := math.Mod(float64(i)*83.7, sh)
		x := math.Mod(float64(i)*311.3-float64(t)*38, sw+400) - 200
		gfx.StrokeLine(dst, float32(x), float32(y), float32(x+180), float32(y), 2, withAlpha(accent, 0x80), true)
	}
	if b.effect.ChildFace != "" {
		// A silhouette at the very place and size the child then appears in.
		var black ebiten.ColorScale
		black.Scale(0, 0, 0, 1)
		g.drawStanding(dst, b.effect.ChildFace, sw*0.5, sh*birthBottom, sh*birthHeight, float32(open), false, &black)
	}
}
