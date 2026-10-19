package puppet

import (
	"math"
	"math/rand/v2"

	"github.com/hajimehoshi/ebiten/v2"
)

// Puppet draws a rig with its motion. Use it only from the Ebitengine game thread.
type Puppet struct {
	Motion *Motion
	rig    *Rig
	mesh   *Mesh

	base    *ebiten.Image
	images  map[*Patch]*ebiten.Image
	canvas  *ebiten.Image
	key     canvasKey
	pos     [][2]float32
	verts   []ebiten.Vertex
	closed  bool
	drawOpt ebiten.DrawTrianglesOptions
}

// canvasKey is what the composed flat picture depends on.
type canvasKey struct {
	face, prev, mouth int
	fade, blink       int // quantized to 1/32
	valid             bool
}

// New prepares a puppet. GPU images are made on the first Draw.
func New(rig *Rig, rng *rand.Rand, tps int) *Puppet {
	mesh := NewMesh(rig)
	return &Puppet{
		Motion: NewMotion(rig, rng, tps),
		rig:    rig,
		mesh:   mesh,
		images: map[*Patch]*ebiten.Image{},
		pos:    make([][2]float32, len(mesh.Src)),
		verts:  make([]ebiten.Vertex, len(mesh.Src)),
	}
}

func (p *Puppet) image(pt *Patch) *ebiten.Image {
	img, ok := p.images[pt]
	if !ok {
		img = ebiten.NewImageFromImage(pt.Img)
		p.images[pt] = img
	}
	return img
}

func (p *Puppet) compose(pose Pose) {
	key := canvasKey{face: pose.Face, prev: pose.PrevFace, mouth: pose.Mouth,
		fade: int(math.Round(pose.Fade * 32)), blink: int(math.Round(pose.Blink * 32)), valid: true}
	if key == p.key {
		return
	}
	p.key = key
	if p.base == nil {
		p.base = ebiten.NewImageFromImage(p.rig.Base)
		w, h := p.rig.Size()
		p.canvas = ebiten.NewImage(w, h)
	}
	c := p.canvas
	c.Clear()
	c.DrawImage(p.base, nil)
	face := p.rig.Faces[pose.Face]
	fade := float32(key.fade) / 32
	if fade < 1 {
		if prev := p.rig.Faces[pose.PrevFace].Patch; prev != nil {
			p.put(c, prev, 1-fade)
		}
	}
	if face.Patch != nil {
		p.put(c, face.Patch, fade)
	}
	if e := face.Eyes; e != nil && key.blink > 0 {
		p.put(c, &e.Closed, float32(key.blink)/32)
	}
	if mo := face.Mouth; mo != nil && pose.Mouth >= 0 {
		p.put(c, mo.Frames[pose.Mouth], 1)
	}
}

func (p *Puppet) put(dst *ebiten.Image, pt *Patch, alpha float32) {
	op := &ebiten.DrawImageOptions{}
	op.GeoM.Translate(float64(pt.At.X), float64(pt.At.Y))
	op.ColorScale.ScaleAlpha(alpha)
	dst.DrawImage(p.image(pt), op)
}

// Draw draws the current frame. op supplies the transform from base pixels to
// dst, the color scale, and the blend; nil draws at the origin. The picture is
// always filtered linearly. Draw does nothing after Close.
func (p *Puppet) Draw(dst *ebiten.Image, op *ebiten.DrawImageOptions) {
	if p.closed {
		return
	}
	pose := p.Motion.Pose()
	p.compose(pose)
	p.mesh.Deform(pose, p.pos)
	var geo ebiten.GeoM
	cs := ebiten.ColorScale{}
	p.drawOpt = ebiten.DrawTrianglesOptions{Filter: ebiten.FilterLinear, ColorScaleMode: ebiten.ColorScaleModePremultipliedAlpha}
	if op != nil {
		geo = op.GeoM
		cs = op.ColorScale
		p.drawOpt.Blend = op.Blend
	}
	r, g, b, a := cs.R(), cs.G(), cs.B(), cs.A()
	for k, s := range p.mesh.Src {
		x, y := geo.Apply(float64(p.pos[k][0]), float64(p.pos[k][1]))
		p.verts[k] = ebiten.Vertex{DstX: float32(x), DstY: float32(y), SrcX: s[0], SrcY: s[1], ColorR: r, ColorG: g, ColorB: b, ColorA: a}
	}
	dst.DrawTriangles(p.verts, p.mesh.Index, p.canvas, &p.drawOpt)
}

// Close releases the GPU images. Later Draw calls draw nothing.
func (p *Puppet) Close() {
	if p.closed {
		return
	}
	p.closed = true
	for _, img := range p.images {
		img.Deallocate()
	}
	if p.base != nil {
		p.base.Deallocate()
		p.canvas.Deallocate()
	}
	p.images = nil
}
