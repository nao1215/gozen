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
	case b.age < 168:
		return birthSpiral
	case b.age < 200:
		return birthFlash
	default:
		return birthChild
	}
}

type birthPoint struct{ x, y float64 }

func birthPositions(sw, sh float64) (parents [2]birthPoint, crystal birthPoint) {
	parents = [2]birthPoint{{sw * 0.29, sh * 0.76}, {sw * 0.71, sh * 0.76}}
	crystal = birthPoint{sw * 0.5, sh * 0.53}
	return
}

func birthOrbPosition(sw, sh float64, parent, age int) birthPoint {
	_, crystal := birthPositions(sw, sh)
	progress := float64(age-65) / 103
	angle := progress*math.Pi*4 + float64(1-parent)*math.Pi
	radius := (1 - progress) * sw * 0.21
	return birthPoint{
		x: crystal.x + radius*math.Cos(angle),
		y: crystal.y + (1-progress)*sh*(0.13+0.13*math.Sin(angle)),
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
	blue := color.NRGBA{95, 233, 255, 255}
	gold := color.NRGBA{255, 216, 126, 255}
	age := b.age
	phase := b.phase()
	// The crystal grows brighter before the parents become light.
	if phase != birthChild {
		glow := 12 + 12*math.Sin(float64(age)*0.15)
		if age >= 60 {
			glow += 15
		}
		gfx.FillCircle(dst, float32(crystal.x), float32(crystal.y), float32(glow+12), withAlpha(blue, 0x48), true)
		gfx.FillCircle(dst, float32(crystal.x), float32(crystal.y), float32(max(5, glow*0.48)), withAlpha(white, 0xd0), true)
	}
	for i, p := range parents {
		if phase >= birthFlash {
			break
		}
		if phase == birthParents {
			glow := uint8(45 + age*2)
			gfx.FillCircle(dst, float32(p.x), float32(p.y-sh*0.24), float32(sh*0.16), withAlpha(blue, glow), true)
			g.drawStanding(dst, b.effect.ParentFaces[i], p.x, p.y, sh*0.45, 1, false, nil)
			g.window(dst, p.x-sw*0.115, p.y+8, sw*0.23, 44)
			g.text(dst, b.effect.ParentNames[i], p.x-sw*0.105, p.y+13, sizeS, g.th.text)
			continue
		}
		// Each picture contracts into its light before the two lights orbit.
		if age < 110 {
			shrink := float64(age-65) / 45
			h := sh * 0.45 * (1 - shrink)
			g.drawStanding(dst, b.effect.ParentFaces[i], p.x, p.y-sh*0.1*shrink, h, float32(1-shrink), false, nil)
		}
		progress := float64(age-65) / 103
		orb := birthOrbPosition(sw, sh, i, age)
		r := float32(4 + (1-progress)*14)
		gfx.FillCircle(dst, float32(orb.x), float32(orb.y), r*2.3, withAlpha(blue, 0x68), true)
		gfx.FillCircle(dst, float32(orb.x), float32(orb.y), r, white, true)
	}
	if phase == birthFlash {
		strength := 1 - math.Abs(float64(age-183))/22
		if strength > 0 {
			for i := 0; i < 12; i++ {
				a := float64(i) * math.Pi / 6
				length := strength * sh * 0.54
				gfx.StrokeLine(dst, float32(crystal.x), float32(crystal.y), float32(crystal.x+math.Cos(a)*length), float32(crystal.y+math.Sin(a)*length), 3, withAlpha(white, 0xc0), true)
			}
			gfx.FillCircle(dst, float32(crystal.x), float32(crystal.y), float32(strength*sh*0.38), withAlpha(white, uint8(strength*245)), true)
		}
	}
	if phase != birthChild {
		return
	}
	// Keep the child at full brightness through the following message lines.
	g.drawStanding(dst, b.effect.ChildFace, sw*0.5, sh*0.79, sh*0.59, 1, false, nil)
	for i := 0; i < 24; i++ {
		a := float64(i)*math.Pi/12 + float64(age-200)*0.02
		r := sh * (0.15 + 0.07*math.Sin(float64(i)*3+float64(age)*0.08))
		x, y := sw*0.5+math.Cos(a)*r, sh*0.47+math.Sin(a)*r
		gfx.FillCircle(dst, float32(x), float32(y), 2+float32(i%3), gold, true)
	}
	bannerW := min(sw*0.6, 600.0)
	g.window(dst, (sw-bannerW)/2, sh*0.07, bannerW, 56)
	label := b.effect.ChildName + " was born!"
	g.text(dst, label, (sw-gfx.TextWidth(label, sizeL))/2, sh*0.07+12, sizeL, g.th.accent)
}
