package play

import (
	"image/color"
	"math"
	"strings"

	"github.com/hajimehoshi/ebiten/v2"

	"github.com/nao1215/gozen/internal/event"
	"github.com/nao1215/gozen/internal/layer5/gfx"
)

// A birth uses update frames. The result remains until a fresh confirm press.
const birthDuration = 288

// Portrait framing places Yorn's lower half outside the screen.
const (
	birthHeight = 1.25
	birthBottom = 1.32
)

type birthAnimation struct {
	effect  event.BreedEffect
	age     int
	skipT   int
	ready   bool
	waiting bool
}

func (b *birthAnimation) silhouetteEnd() int { return 144 + 24*b.effect.Tier }
func (b *birthAnimation) revealEnd() int     { return b.silhouetteEnd() + 48 + 12*b.effect.Tier }
func (b *birthAnimation) viewEnd() int       { return b.revealEnd() + 36 }
func (b *birthAnimation) duration() int      { return b.viewEnd() + 60 }

type birthPhase uint8

const (
	birthParents birthPhase = iota
	birthYornExit
	birthFade
	birthMerge
	birthSilhouette
	birthReveal
	birthView
	birthResult
)

func (b *birthAnimation) phase() birthPhase {
	switch {
	case b.age < 30:
		return birthParents
	case b.age < 54:
		return birthYornExit
	case b.age < 90:
		return birthFade
	case b.age < 120:
		return birthMerge
	case b.age < b.silhouetteEnd():
		return birthSilhouette
	case b.age < b.revealEnd():
		return birthReveal
	case b.age < b.viewEnd():
		return birthView
	default:
		return birthResult
	}
}

type birthPoint struct{ x, y float64 }

func birthPositions(sw, sh float64) (parents [2]birthPoint, crystal birthPoint) {
	return [2]birthPoint{{sw * 0.22, sh * 0.78}, {sw * 0.78, sh * 0.78}}, birthPoint{sw * 0.5, sh * 0.52}
}

func birthOrbPosition(sw, sh float64, parent, age, end int) birthPoint {
	parents, crystal := birthPositions(sw, sh)
	p := min(1, max(0, float64(age-90)/float64(end-90)))
	start := birthPoint{parents[parent].x, parents[parent].y - sh*0.23}
	return birthPoint{start.x + (crystal.x-start.x)*p, start.y + (crystal.y-start.y)*p}
}

func birthChildFrame(sw, sh float64) (x, feet, maxH, maxW float64) {
	return sw * 0.5, sh * 0.82, sh * 0.62, sw * 0.58
}

// drawBirthMonster fits the complete image inside both limits without changing its aspect ratio.
func (g *Game) drawBirthMonster(dst *ebiten.Image, path string, cx, feet, maxH, maxW float64, alpha float32, black bool) {
	img := g.pics.get(path)
	if img == nil {
		return
	}
	iw, ih := float64(img.Bounds().Dx()), float64(img.Bounds().Dy())
	if iw == 0 || ih == 0 {
		return
	}
	scale := min(maxH/ih, maxW/iw)
	op := &ebiten.DrawImageOptions{Filter: ebiten.FilterLinear}
	op.GeoM.Scale(scale, scale)
	op.GeoM.Translate(cx-iw*scale/2, feet-ih*scale)
	if black {
		op.ColorScale.Scale(0, 0, 0, 1)
	}
	op.ColorScale.ScaleAlpha(alpha)
	gfx.DrawImage(dst, img, op)
}

func birthLight(tier int) color.NRGBA {
	switch tier {
	case 1:
		return color.NRGBA{255, 216, 126, 255}
	case 2:
		return color.NRGBA{157, 183, 203, 255}
	default:
		return color.NRGBA{255, 244, 218, 255}
	}
}

func birthRank(tier int) (string, int) {
	switch tier {
	case 1:
		return "Rare", 2
	case 2:
		return "Legendary", 3
	default:
		return "Normal", 1
	}
}

func (g *Game) drawBirth(dst *ebiten.Image) {
	b := g.birth
	if b == nil {
		return
	}
	sw, sh := g.screen()
	if loc := g.db.Locations[g.st.Location]; loc != nil {
		zoom := 1.0
		if b.age >= 90 {
			zoom += 0.025 * min(1, float64(b.age-90)/30)
		}
		g.drawPicture(dst, loc.Background, (1-zoom)*sw/2, (1-zoom)*sh/2, sw*zoom, sh*zoom, 1)
	}
	parents, crystal := birthPositions(sw, sh)
	phase, age := b.phase(), b.age
	light := birthLight(b.effect.Tier)
	if phase >= birthSilhouette {
		dim := uint8(61 * min(1, float64(age-120)/12))
		fill(dst, withAlpha(colorBlack, dim))
		// The rings stay behind the child. Their count and fixed color convey rarity.
		rings := 1 + min(1, b.effect.Tier)
		for i := range rings {
			c := light
			if b.effect.Tier == 2 && i == 0 {
				c = color.NRGBA{255, 216, 126, 255}
			}
			r := float32(sh * (0.27 + float64(i)*0.045))
			if b.effect.Tier == 2 && i == 1 {
				r *= float32(0.85 + 0.15*min(1, float64(age-120)/36))
			}
			for j := range 64 {
				a, z := float64(j)*math.Pi/32, float64(j+1)*math.Pi/32
				gfx.StrokeLine(dst, float32(crystal.x)+r*float32(math.Cos(a)), float32(sh*0.49)+r*float32(math.Sin(a)), float32(crystal.x)+r*float32(math.Cos(z)), float32(sh*0.49)+r*float32(math.Sin(z)), 2, withAlpha(c, 0x78), true)
			}
		}
	}
	// Yorn has the opening moment, then leaves completely before the parents fade.
	if age < 54 {
		alpha, offset := float32(1), 0.0
		if age >= 30 {
			p := float64(age-30) / 24
			alpha, offset = float32(1-p), p*sw*0.08
		}
		if loc := g.db.Locations[g.st.Location]; loc != nil {
			for _, person := range loc.People {
				if person.Image != "" {
					g.drawStanding(dst, person.Image, sw*0.5+offset, sh*1.32, sh*1.25, alpha, false, nil)
					break
				}
			}
		}
	}
	if age < 120 {
		for i, pt := range parents {
			alpha := float32(1)
			if age >= 54 {
				alpha = float32(max(0, 1-float64(age-54)/36))
			}
			if alpha > 0 {
				g.drawBirthMonster(dst, b.effect.ParentFaces[i], pt.x, pt.y, sh*0.46, sw*0.27, alpha, false)
			}
			if age < 66 {
				nameAlpha := float32(1)
				if age >= 54 {
					nameAlpha = float32(1 - float64(age-54)/12)
				}
				if nameAlpha > 0 {
					g.text(dst, b.effect.ParentNames[i], pt.x-sw*0.11, pt.y+10, sizeS, withAlpha(g.th.text, uint8(255*nameAlpha)))
				}
			}
			if age >= 54 {
				orb := birthPoint{pt.x, pt.y - sh*0.23}
				if age >= 90 {
					orb = birthOrbPosition(sw, sh, i, age, 120)
					for tail := 1; tail <= 4; tail++ {
						past := birthOrbPosition(sw, sh, i, max(90, age-tail*3), 120)
						gfx.FillCircle(dst, float32(past.x), float32(past.y), float32(4-tail/2), withAlpha(light, uint8(70-tail*12)), true)
					}
				}
				gfx.FillCircle(dst, float32(orb.x), float32(orb.y), 14, withAlpha(light, 0x88), true)
				gfx.FillCircle(dst, float32(orb.x), float32(orb.y), 5, light, true)
			}
		}
		return
	}
	x, feet, h, w := birthChildFrame(sw, sh)
	reveal := float32(0)
	if phase >= birthReveal {
		reveal = float32(min(1, float64(age-b.silhouetteEnd())/float64(b.revealEnd()-b.silhouetteEnd())))
	}
	if b.skipT > 0 {
		reveal = float32(b.skipT) / 12
	}
	g.drawBirthMonster(dst, b.effect.ChildFace, x, feet, h, w, 1, true)
	if reveal > 0 {
		g.drawBirthMonster(dst, b.effect.ChildFace, x, feet, h, w, reveal, false)
	}
	if phase >= birthView && phase < birthResult && b.skipT == 0 {
		count := 6 + 4*b.effect.Tier
		for i := range count {
			p := float64(age-b.revealEnd()) / 36
			px := x + (float64(i%5)-2)*sw*0.055
			py := feet - p*sh*0.17 - float64(i/5)*sh*0.055
			gfx.FillCircle(dst, float32(px), float32(py), 2, withAlpha(light, uint8(120*(1-p))), true)
		}
	}
	if phase == birthResult || b.skipT > 0 {
		rank, marks := birthRank(b.effect.Tier)
		for i := range marks {
			gfx.FillRect(dst, float32(sw*0.055+float64(i)*10), float32(sh*0.07), 5, 19, light, false)
		}
		g.text(dst, rank, sw*0.055, sh*0.11, sizeS*0.8, g.th.text)
		bannerW := min(sw*0.6, 600.0)
		g.window(dst, (sw-bannerW)/2, sh*0.045, bannerW, 62)
		label := b.effect.ChildName + " was born!"
		g.text(dst, label, (sw-gfx.TextWidth(label, sizeL))/2, sh*0.06, sizeL, g.th.accent)
		if b.effect.FirstDiscovery {
			g.text(dst, "New species discovered", sw*0.5-gfx.TextWidth("New species discovered", sizeS)/2, sh*0.15, sizeS, g.th.accent)
		}
		g.window(dst, sw*0.18, sh*0.83, sw*0.64, sh*0.16)
		for i, line := range strings.Split(b.effect.Result, "\n") {
			g.text(dst, line, sw*0.20, sh*0.845+float64(i)*gfx.LineHeight(sizeS*0.8), sizeS*0.8, g.th.text)
		}
		g.text(dst, "Continue", sw*0.70, sh*0.945, sizeS*0.8, g.th.dim)
	}
}
