package play

import (
	"image"
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
	details bool
}

func (b *birthAnimation) silhouetteEnd() int { return 144 + 24*b.effect.Tier }
func (b *birthAnimation) revealEnd() int     { return b.silhouetteEnd() + 48 + 12*b.effect.Tier }
func (b *birthAnimation) cutInStart() int    { return b.revealEnd() + 8 }
func (b *birthAnimation) cutInEnd() int      { return b.cutInStart() + 48 + 6*(b.effect.Tier-1) }
func (b *birthAnimation) viewEnd() int {
	if b.effect.Tier > 0 { return b.cutInEnd() + 12 }
	return b.revealEnd() + 36
}
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

// birthFaceCrop uses an authored point first, then the generated face box, then
// a centered crop of the upper picture. The rectangle is clamped to image bounds.
func birthFaceCrop(size image.Point, authored []float64, found image.Rectangle, hasFound bool) image.Rectangle {
	w, h := size.X, size.Y
	if w <= 0 || h <= 0 { return image.Rectangle{} }
	side := min(w, h)
	cx, cy := w/2, h/4
	if hasFound && !found.Empty() {
		cx, cy = (found.Min.X+found.Max.X)/2, (found.Min.Y+found.Max.Y)/2
		side = min(w, h, max(found.Dx(), found.Dy()))
	}
	if len(authored) == 3 && authored[2] > 0 {
		cx, cy = int(authored[0]*float64(w)), int(authored[1]*float64(h))
		side = max(1, min(w, h, int(authored[2]*float64(w))))
	}
	x := max(0, min(w-side, cx-side/2))
	y := max(0, min(h-side, cy-side/2))
	return image.Rect(x, y, x+side, y+side)
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
	if age >= 54 && age < 120 {
		for i, pt := range parents {
			alpha := float32(min(1, float64(age-54)/18))
			if age >= 72 { alpha = float32(max(0, 1-float64(age-72)/18)) }
			from := -sw*0.08
			if i == 1 { from = sw*0.08 }
			pt.x += from*(1-min(1, float64(age-54)/18))
			if alpha > 0 {
				g.drawBirthMonster(dst, b.effect.ParentFaces[i], pt.x, pt.y, sh*0.46, sw*0.27, alpha, false)
			}
			if age < 72 && alpha > 0 {
				g.text(dst, b.effect.ParentNames[i], pt.x-sw*0.11, pt.y+10, sizeS, withAlpha(g.th.text, uint8(255*alpha)))
			}
			if age >= 72 {
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
	if age < 54 { return }
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
	if b.effect.Tier > 0 && age >= b.cutInStart() && age < b.cutInEnd() && b.skipT == 0 {
		g.drawBirthCutIn(dst, b, sw, sh)
	}
	if phase == birthResult || b.skipT > 0 {
		bannerW := min(sw*0.6, 600.0)
		g.window(dst, (sw-bannerW)/2, sh*0.045, bannerW, 62)
		label := b.effect.ChildName + " was born!"
		g.text(dst, label, (sw-gfx.TextWidth(label, sizeL))/2, sh*0.06, sizeL, g.th.accent)
		if b.details {
			g.window(dst, sw*0.18, sh*0.66, sw*0.64, sh*0.31)
			lines := []string{b.effect.Result}
			if b.effect.FirstDiscovery { lines = append(lines, "New species discovered") }
			for i, line := range strings.Split(strings.Join(lines, "\n"), "\n") { g.text(dst, line, sw*0.21, sh*0.69+float64(i)*gfx.LineHeight(sizeM), sizeM, g.th.text) }
			g.text(dst, "Back", sw*0.71, sh*0.91, sizeS, g.th.dim)
		} else if age >= b.viewEnd()+12 || b.skipT > 0 {
			x, y, w, h := g.talkBox()
			g.window(dst, x, y, w, h)
			fit, _ := gfx.FitLines(b.effect.Summary, sizeM, w-100, 1)
			for _, line := range fit { g.text(dst, line, x+50, y+40, sizeM, g.th.text) }
			g.text(dst, "Continue  ·  X Details", x+w-280, y+h-45, sizeS, g.th.dim)
		}
	}
}

func birthCutInOffset(age, start, end int, sw float64) float64 {
	step := age-start
	if step < 8 { return (1-float64(step)/8)*sw }
	if age >= end-8 { return float64(age-(end-8))/8*sw }
	return 0
}

func (g *Game) drawBirthCutIn(dst *ebiten.Image, b *birthAnimation, sw, sh float64) {
	img := g.pics.get(b.effect.ChildFace)
	if img == nil { return }
	size := img.Bounds().Size()
	var found image.Rectangle
	var ok bool
	if len(b.effect.ChildFaceAt) == 0 {
		g.pics.mu.Lock()
		found, ok = g.pics.foundFace(b.effect.ChildFace, size)
		g.pics.mu.Unlock()
	}
	crop := birthFaceCrop(size, b.effect.ChildFaceAt, found, ok)
	if crop.Empty() { return }
	offset := birthCutInOffset(b.age, b.cutInStart(), b.cutInEnd(), sw)
	x, y, w, h := sw*0.10+offset, sh*0.30, sw*0.80, sh*0.30
	gfx.FillRect(dst, float32(x), float32(y), float32(w), float32(h), withAlpha(colorBlack, 220), false)
	for i := range 13 {
		ly := y+float64(i)*h/12
		gfx.StrokeLine(dst, float32(x-18), float32(ly), float32(x+w+18), float32(ly-6), 1.5, withAlpha(birthLight(b.effect.Tier), uint8(28+i%3*16)), true)
	}
	face := img.SubImage(crop).(*ebiten.Image)
	scale := h*1.15/float64(crop.Dy())
	op := &ebiten.DrawImageOptions{Filter: ebiten.FilterLinear}
	op.GeoM.Scale(scale, scale)
	op.GeoM.Translate(x+w*0.54-float64(crop.Dx())*scale/2, y-(float64(crop.Dy())*scale-h)/2)
	gfx.DrawImage(dst, face, op)
	gfx.StrokeLine(dst, float32(x-18), float32(y), float32(x+w+18), float32(y), 3, birthLight(b.effect.Tier), true)
	gfx.StrokeLine(dst, float32(x-18), float32(y+h), float32(x+w+18), float32(y+h), 3, birthLight(b.effect.Tier), true)
}
