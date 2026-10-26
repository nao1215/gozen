package puppet

import (
	"image"
	"math"
	"math/rand/v2"
	"runtime"
	"sync"

	"github.com/hajimehoshi/ebiten/v2"
)

// Puppet draws a rig with its motion. Use it only from the Ebitengine game thread.
type Puppet struct {
	Motion *Motion
	// Reduce composes the picture at a smaller size when it is drawn much smaller than
	// its base: the base and the overlays are box-filtered by a power of two, at most
	// down to the drawn size, so a large picture costs less GPU memory and fill and is
	// not aliased by the shrink. Off, the picture is composed at the base size.
	Reduce bool

	rig  *Rig
	mesh *Mesh

	layers  map[int]*layer // by reduction level (0 is the base size)
	pos     [][2]float32
	verts   []ebiten.Vertex
	closed  bool
	drawOpt ebiten.DrawTrianglesOptions
}

// layer is the composed picture at one reduction level: 1/2^level of the base size.
type layer struct {
	level  int
	base   *ebiten.Image
	canvas *ebiten.Image
	images map[*Patch]*ebiten.Image
	at     map[*Patch]image.Point // where each reduced overlay goes on the canvas
	key    canvasKey
}

// canvasKey is what the composed flat picture depends on.
type canvasKey struct {
	face, prev, mouth int
	fade, blink       int // quantized to 1/32
	valid             bool
}

// maxLevel bounds the reduction to 1/32 of the base size.
const maxLevel = 5

// New prepares a puppet. GPU images are made on the first Draw.
func New(rig *Rig, rng *rand.Rand, tps int) *Puppet {
	mesh := NewMesh(rig)
	return &Puppet{
		Motion: NewMotion(rig, rng, tps),
		rig:    rig,
		mesh:   mesh,
		layers: map[int]*layer{},
		pos:    make([][2]float32, len(mesh.Src)),
		verts:  make([]ebiten.Vertex, len(mesh.Src)),
	}
}

// Rig is the rig the puppet draws.
func (p *Puppet) Rig() *Rig { return p.rig }

// Level is the reduction level Draw uses for a transform: 0 unless Reduce is set, else
// the largest level whose picture (1/2^level of the base) is still no smaller than the
// drawn size.
func (p *Puppet) Level(geo ebiten.GeoM) int {
	if !p.Reduce {
		return 0
	}
	a, b, c, d := geo.Element(0, 0), geo.Element(0, 1), geo.Element(1, 0), geo.Element(1, 1)
	s := math.Sqrt(math.Abs(a*d - b*c))
	w, h := p.rig.Size()
	level := 0
	for level < maxLevel && s*float64(int(1)<<(level+1)) <= 1 && min(w, h)>>(level+1) >= 1 {
		level++
	}
	return level
}

func (p *Puppet) layer(level int) *layer {
	l := p.layers[level]
	if l == nil {
		l = &layer{level: level, images: map[*Patch]*ebiten.Image{}, at: map[*Patch]image.Point{}}
		p.layers[level] = l
	}
	return l
}

func (l *layer) image(pt *Patch) (*ebiten.Image, image.Point) {
	img, ok := l.images[pt]
	if !ok {
		if l.level == 0 {
			img = ebiten.NewImageFromImage(pt.Img)
			l.at[pt] = pt.At
		} else {
			small, at := shrink(pt.Img, pt.At, l.level)
			img = ebiten.NewImageFromImage(small)
			l.at[pt] = at
		}
		l.images[pt] = img
	}
	return img, l.at[pt]
}

func (p *Puppet) compose(l *layer, pose Pose) {
	key := canvasKey{
		face: pose.Face, prev: pose.PrevFace, mouth: pose.Mouth,
		fade: int(math.Round(pose.Fade * 32)), blink: int(math.Round(pose.Blink * 32)), valid: true,
	}
	if key == l.key {
		return
	}
	l.key = key
	if l.base == nil {
		if l.level == 0 {
			l.base = ebiten.NewImageFromImage(p.rig.Base)
		} else {
			small, _ := shrink(p.rig.Base, image.Point{}, l.level)
			l.base = ebiten.NewImageFromImage(small)
		}
		b := l.base.Bounds()
		l.canvas = ebiten.NewImage(b.Dx(), b.Dy())
	}
	c := l.canvas
	c.Clear()
	c.DrawImage(l.base, nil)
	face := p.rig.Faces[pose.Face]
	fade := float32(key.fade) / 32
	if fade < 1 {
		if prev := p.rig.Faces[pose.PrevFace].Patch; prev != nil {
			l.put(c, prev, 1-fade)
		}
	}
	if face.Patch != nil {
		l.put(c, face.Patch, fade)
	}
	if e := face.Eyes; e != nil && key.blink > 0 {
		l.put(c, &e.Closed, float32(key.blink)/32)
	}
	if mo := face.Mouth; mo != nil && pose.Mouth >= 0 {
		l.put(c, mo.Frames[pose.Mouth], 1)
	}
}

func (l *layer) put(dst *ebiten.Image, pt *Patch, alpha float32) {
	img, at := l.image(pt)
	op := &ebiten.DrawImageOptions{}
	op.GeoM.Translate(float64(at.X), float64(at.Y))
	op.ColorScale.ScaleAlpha(alpha)
	dst.DrawImage(img, op)
}

// Draw draws the current frame. op supplies the transform from base pixels to
// dst, the color scale, and the blend; nil draws at the origin. The picture is
// always filtered linearly. Draw does nothing after Close.
func (p *Puppet) Draw(dst *ebiten.Image, op *ebiten.DrawImageOptions) {
	if p.closed {
		return
	}
	pose := p.Motion.Pose()
	var geo ebiten.GeoM
	cs := ebiten.ColorScale{}
	p.drawOpt = ebiten.DrawTrianglesOptions{Filter: ebiten.FilterLinear, ColorScaleMode: ebiten.ColorScaleModePremultipliedAlpha}
	if op != nil {
		geo = op.GeoM
		cs = op.ColorScale
		p.drawOpt.Blend = op.Blend
	}
	l := p.layer(p.Level(geo))
	p.compose(l, pose)
	p.mesh.Deform(pose, p.pos)
	k := 1 / float32(int(1)<<l.level)
	r, g, b, a := cs.R(), cs.G(), cs.B(), cs.A()
	for i, s := range p.mesh.Src {
		x, y := geo.Apply(float64(p.pos[i][0]), float64(p.pos[i][1]))
		p.verts[i] = ebiten.Vertex{DstX: float32(x), DstY: float32(y), SrcX: s[0] * k, SrcY: s[1] * k, ColorR: r, ColorG: g, ColorB: b, ColorA: a}
	}
	dst.DrawTriangles(p.verts, p.mesh.Index, l.canvas, &p.drawOpt)
}

// Close releases the GPU images. Later Draw calls draw nothing.
func (p *Puppet) Close() {
	if p.closed {
		return
	}
	p.closed = true
	for _, l := range p.layers {
		for _, img := range l.images {
			img.Deallocate()
		}
		if l.base != nil {
			l.base.Deallocate()
			l.canvas.Deallocate()
		}
	}
	p.layers = nil
}

// shrink box-filters img, placed at at on the base, to 1/2^level of its size. The
// blocks are aligned to the base, so a reduced overlay lies exactly over the reduced
// base; it returns the reduced picture and where it goes. Colors are averaged by
// their alpha, so transparent pixels do not darken the edges. Large pictures are
// shrunk in bands on several goroutines.
func shrink(img *image.NRGBA, at image.Point, level int) (*image.NRGBA, image.Point) {
	n := 1 << level
	b := img.Bounds()
	x0, y0 := floorDiv(at.X, n), floorDiv(at.Y, n)
	x1, y1 := floorDiv(at.X+b.Dx()+n-1, n), floorDiv(at.Y+b.Dy()+n-1, n)
	out := image.NewNRGBA(image.Rect(0, 0, x1-x0, y1-y0))
	rows := func(from, to int) {
		area := uint64(n) * uint64(n) //nolint:gosec // n is a power of two up to 1<<maxLevel.
		for oy := from; oy < to; oy++ {
			for ox := range x1 - x0 {
				var sr, sg, sb, sa uint64
				for j := range n {
					y := (y0+oy)*n + j - at.Y
					if y < 0 || y >= b.Dy() {
						continue
					}
					row := img.Pix[y*img.Stride:]
					for i := range n {
						x := (x0+ox)*n + i - at.X
						if x < 0 || x >= b.Dx() {
							continue
						}
						px := row[x*4 : x*4+4]
						a := uint64(px[3])
						sr += uint64(px[0]) * a
						sg += uint64(px[1]) * a
						sb += uint64(px[2]) * a
						sa += a
					}
				}
				if sa == 0 {
					continue
				}
				o := out.Pix[oy*out.Stride+ox*4:]
				o[0] = uint8((sr + sa/2) / sa)     //nolint:gosec // An average of bytes is a byte.
				o[1] = uint8((sg + sa/2) / sa)     //nolint:gosec // An average of bytes is a byte.
				o[2] = uint8((sb + sa/2) / sa)     //nolint:gosec // An average of bytes is a byte.
				o[3] = uint8((sa + area/2) / area) //nolint:gosec // An average of bytes is a byte.
			}
		}
	}
	h := y1 - y0
	bands := min(runtime.GOMAXPROCS(0), max(1, b.Dx()*b.Dy()/(1<<20)), h)
	if bands <= 1 {
		rows(0, h)
		return out, image.Pt(x0, y0)
	}
	var wg sync.WaitGroup
	for k := range bands {
		wg.Go(func() { rows(h*k/bands, h*(k+1)/bands) })
	}
	wg.Wait()
	return out, image.Pt(x0, y0)
}

func floorDiv(a, n int) int {
	if a >= 0 {
		return a / n
	}
	return -((-a + n - 1) / n)
}
