package stage

import (
	"cmp"
	_ "embed"
	"fmt"
	"image"
	"image/color"
	"math"
	"slices"

	"github.com/hajimehoshi/ebiten/v2"

	"github.com/nao1215/gozen/internal/layer1/geom"
	"github.com/nao1215/gozen/internal/layer5/doll"
)

//go:embed doll.kage
var dollShader []byte

// SheetW and SheetH are the size of one picture of a baked doll.
const (
	SheetW, SheetH = 96, 168
	unitsH         = 1.12 // a picture's height in the doll's heights
)

// Figure is someone drawn in the scene.
type Figure struct {
	At     geom.Pt // the feet, in the background's pixels
	Facing geom.Pt // the screen direction they look (y down: toward the camera)
	Moving bool
	Phase  float64 // the walk's phase in radians
	Sheet  *ebiten.Image
	Fade   float64 // 0 fully shown .. 1 gone
}

// Renderer draws a scene: the background, then the figures where the picture does not
// hide them.
type Renderer struct {
	s      *Scene
	bg     *ebiten.Image
	depth  *ebiten.Image // the depth as R (high byte) and G (low byte)
	fronts *ebiten.Image // the fronts' ground lines, in sixteenths of a pixel (R high, G low)
	gray   *image.Gray16
	shader *ebiten.Shader
	shadow *ebiten.Image
	// Look scales the background's colors (its tint and brightness).
	Look ebiten.ColorScale

	vs                        [4]ebiten.Vertex
	sop                       ebiten.DrawTrianglesShaderOptions
	depthScale, offset, frame []float32
}

// NewRenderer makes the textures of a scene: bg is the background, depth its depth picture
// (nil: nothing hides anyone but the fronts).
func NewRenderer(s *Scene, bg, depth image.Image) (*Renderer, error) {
	sh, err := ebiten.NewShader(dollShader)
	if err != nil {
		return nil, fmt.Errorf("doll shader: %w", err)
	}
	b := bg.Bounds().Sub(bg.Bounds().Min)
	r := &Renderer{s: s, shader: sh, bg: ebiten.NewImageFromImage(bg)}
	r.gray = Gray16(depth, b)
	d := image.NewRGBA(b)
	for y := range b.Dy() {
		for x := range b.Dx() {
			v := r.gray.Gray16At(x, y).Y
			d.SetRGBA(x, y, color.RGBA{uint8(v >> 8), uint8(v), 0, 255}) //nolint:gosec // the two bytes
		}
	}
	fi := image.NewRGBA(b)
	for i := 3; i < len(fi.Pix); i += 4 {
		fi.Pix[i] = 255
	}
	for _, fr := range s.Fronts {
		lo, hi := fr.Area.Bounds()
		v := uint16(math.Min(65535, fr.Base*16))
		for y := int(lo.Y); y <= int(hi.Y); y++ {
			for x := int(lo.X); x <= int(hi.X); x++ {
				if !(image.Point{x, y}).In(b) || !fr.Area.Inside(geom.Pt{X: float64(x) + 0.5, Y: float64(y) + 0.5}) {
					continue
				}
				o := fi.PixOffset(x, y)
				if old := uint16(fi.Pix[o])<<8 | uint16(fi.Pix[o+1]); old == 0 || v > old {
					fi.Pix[o], fi.Pix[o+1] = uint8(v>>8), uint8(v) //nolint:gosec // the two bytes
				}
			}
		}
	}
	// managed images (not Unmanaged): Ebitengine keeps their pixels and puts them back when
	// a browser loses the WebGL context; the shader reads them by the doll's own position
	// (imageSrcNUnsafeAtFromSrc0Pos), wherever they lie in the texture atlas
	r.depth = ebiten.NewImageFromImage(d)
	r.fronts = ebiten.NewImageFromImage(fi)
	sd := image.NewRGBA(image.Rect(0, 0, 64, 32))
	for y := range 32 {
		for x := range 64 {
			dx, dy := (float64(x)+0.5-32)/32, (float64(y)+0.5-16)/16
			a := math.Max(0, 1-math.Sqrt(dx*dx+dy*dy))
			sd.SetRGBA(x, y, color.RGBA{0, 0, 0, uint8(110 * a * a)})
		}
	}
	r.shadow = ebiten.NewImageFromImage(sd)
	return r, nil
}

// Close frees the textures.
func (r *Renderer) Close() {
	for _, img := range []*ebiten.Image{r.bg, r.depth, r.fronts, r.shadow} {
		img.Deallocate()
	}
	r.shader.Deallocate()
}

// Size is the background's size in pixels.
func (r *Renderer) Size() (int, int) { return r.bg.Bounds().Dx(), r.bg.Bounds().Dy() }

// Fit is how the background lies on a screen of w x h: the scale and the offset (centered,
// the whole picture shown).
func (r *Renderer) Fit(w, h float64) (k, ox, oy float64) {
	bw, bh := r.Size()
	k = math.Min(w/float64(bw), h/float64(bh))
	return k, (w - float64(bw)*k) / 2, (h - float64(bh)*k) / 2
}

// ScreenOf is where a point of the background shows on a screen of w x h, and the scale.
func (r *Renderer) ScreenOf(p geom.Pt, w, h float64) (float64, float64, float64) {
	k, ox, oy := r.Fit(w, h)
	return ox + p.X*k, oy + p.Y*k, k
}

// HeadOf is where the top of the head of a doll standing at p shows on a screen of w x h.
func (r *Renderer) HeadOf(p geom.Pt, w, h float64) (float64, float64) {
	x, y, k := r.ScreenOf(p, w, h)
	ph := r.s.Height(p.Y) * k * unitsH / math.Cos(r.s.Camera*math.Pi/180)
	return x, y - ph*headTop
}

// headTop is the top of a doll's head over its feet, in its picture's heights (the head
// reaches about 1.02 of the doll's height, the picture is 1.12 and stands 0.02 low).
const headTop = 1.02/unitsH - 0.02

// PointOf is the point of the background under a point of a screen of w x h.
func (r *Renderer) PointOf(x, y, w, h float64) geom.Pt {
	k, ox, oy := r.Fit(w, h)
	return geom.Pt{X: (x - ox) / k, Y: (y - oy) / k}
}

// DepthAt is the background's nearness at p (0 far .. 1 near).
func (r *Renderer) DepthAt(p geom.Pt) float64 {
	b := r.gray.Bounds()
	x := int(math.Max(float64(b.Min.X), math.Min(float64(b.Max.X-1), p.X)))
	y := int(math.Max(float64(b.Min.Y), math.Min(float64(b.Max.Y-1), p.Y)))
	return float64(r.gray.Gray16At(x, y).Y) / 65535
}

// behindFront reports whether feet at p stand behind a front that covers them.
func (r *Renderer) behindFront(p geom.Pt) bool {
	for _, f := range r.s.Fronts {
		if f.Base > p.Y && f.Area.Inside(p) {
			return true
		}
	}
	return false
}

// Draw lays the background and the figures on dst (fitted to it), the far ones first.
func (r *Renderer) Draw(dst *ebiten.Image, figures []Figure) {
	db := dst.Bounds()
	w, h := float64(db.Dx()), float64(db.Dy())
	k, ox, oy := r.Fit(w, h)
	op := &ebiten.DrawImageOptions{Filter: ebiten.FilterLinear}
	op.GeoM.Scale(k, k)
	op.GeoM.Translate(ox, oy)
	op.ColorScale = r.Look
	dst.DrawImage(r.bg, op)
	slices.SortStableFunc(figures, func(a, b Figure) int { return cmp.Compare(a.At.Y, b.At.Y) })
	for _, f := range figures {
		if f.Sheet != nil {
			r.drawFigure(dst, f, k, ox, oy)
		}
	}
}

func (r *Renderer) drawFigure(dst *ebiten.Image, f Figure, k, ox, oy float64) {
	s := r.s
	alpha := float32(1 - f.Fade)
	hgt := s.Height(f.At.Y) * k // the person's height on dst
	ph := hgt * unitsH / math.Cos(s.Camera*math.Pi/180)
	pw := ph * SheetW / SheetH
	fx, fy := ox+f.At.X*k, oy+f.At.Y*k
	if !r.behindFront(f.At) { // the shadow on the ground, unless a front stands over it
		so := &ebiten.DrawImageOptions{Filter: ebiten.FilterLinear}
		so.GeoM.Scale(hgt*0.42/64, hgt*0.42*0.55/32)
		so.GeoM.Translate(fx-hgt*0.21, fy-hgt*0.42*0.55/2)
		so.ColorScale.ScaleAlpha(alpha)
		dst.DrawImage(r.shadow, so)
	}
	side := doll.Side(f.Facing.X, f.Facing.Y)
	pose := doll.Pose(f.Moving, f.Phase)
	sx, sy := float32(side*SheetW), float32(pose*SheetH)
	x0, y0 := float32(fx-pw/2), float32(fy-ph+ph*0.02)
	x1, y1 := float32(fx+pw/2), float32(fy+ph*0.02)
	// the vertices, the options and the uniforms are kept and filled again (Ebitengine copies
	// them in the call), so drawing makes no garbage
	for i, v := range [4][4]float32{{x0, y0, sx, sy}, {x1, y0, sx + SheetW, sy}, {x0, y1, sx, sy + SheetH}, {x1, y1, sx + SheetW, sy + SheetH}} {
		r.vs[i] = ebiten.Vertex{DstX: v[0], DstY: v[1], SrcX: v[2], SrcY: v[3], ColorR: 1, ColorG: 1, ColorB: 1, ColorA: 1}
	}
	if r.sop.Uniforms == nil {
		r.sop.Uniforms = map[string]any{}
		r.depthScale, r.offset, r.frame = make([]float32, 2), make([]float32, 2), make([]float32, 4)
	}
	r.sop.Images = [4]*ebiten.Image{f.Sheet, r.depth, r.fronts}
	u := r.sop.Uniforms
	r.depthScale[0], r.depthScale[1] = float32(1/k), float32(1/k)
	r.offset[0], r.offset[1] = float32(ox), float32(oy)
	r.frame[0], r.frame[1], r.frame[2], r.frame[3] = sx, sy, SheetW, SheetH
	u["FootDepth"], u["FootY"], u["Bias"], u["Alpha"] = float32(r.DepthAt(f.At)), float32(f.At.Y), float32(s.DepthBias), alpha
	u["DepthScale"], u["Offset"], u["Frame"] = r.depthScale, r.offset, r.frame
	dst.DrawTrianglesShader(r.vs[:], quad, r.shader, &r.sop)
}

var quad = []uint16{0, 1, 2, 1, 3, 2}
