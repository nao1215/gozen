//go:build graphics

package puppet_test

import (
	"bytes"
	"errors"
	"image"
	"image/color"
	"image/png"
	"math/rand/v2"
	"os"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/hajimehoshi/ebiten/v2"

	"github.com/nao1215/gozen/internal/layer2/puppetdef"
	"github.com/nao1215/gozen/internal/layer5/puppet"
)

// The pixel tests need a display (Xvfb on Linux) and one Ebitengine loop for the
// whole process. TestMain runs the loop; each test sends its work into Update,
// where drawing to offscreen images and ReadPixels are allowed.

var work = make(chan func())

type loop struct{ done chan struct{} }

func (l *loop) Update() error {
	select {
	case f := <-work:
		f()
	case <-l.done:
		return ebiten.Termination
	default:
	}
	return nil
}

func (l *loop) Draw(*ebiten.Image)            { /* nothing on screen */ }
func (l *loop) Layout(int, int) (int, int)    { return 64, 64 }
func onLoop(f func())                         { ok := make(chan struct{}); work <- func() { f(); close(ok) }; <-ok }
func at(img *image.RGBA, x, y int) color.RGBA { return img.RGBAAt(x, y) }
func near(a, b color.RGBA) bool {
	return d(a.R, b.R) < 24 && d(a.G, b.G) < 24 && d(a.B, b.B) < 24 && d(a.A, b.A) < 24
}
func d(a, b uint8) int { return max(int(a)-int(b), int(b)-int(a)) }
func rgba(c color.NRGBA) color.RGBA {
	r, ok := color.RGBAModel.Convert(c).(color.RGBA)
	if !ok {
		panic("RGBAModel returned another type")
	}
	return r
}

func px(dst *ebiten.Image) *image.RGBA {
	img := image.NewRGBA(dst.Bounds())
	dst.ReadPixels(img.Pix)
	return img
}

func shift(x, y int) *ebiten.DrawImageOptions {
	op := &ebiten.DrawImageOptions{}
	op.GeoM.Translate(float64(x), float64(y))
	return op
}

func TestMain(m *testing.M) {
	l := &loop{done: make(chan struct{})}
	code := 0
	go func() {
		code = m.Run()
		close(l.done)
	}()
	ebiten.SetWindowSize(64, 64)
	if err := ebiten.RunGame(l); err != nil && !errors.Is(err, ebiten.Termination) {
		panic(err)
	}
	os.Exit(code)
}

// The figure is drawn at (32, 32) on a 128x160 canvas.
const ox, oy = 32, 32

func newPuppet(t *testing.T) *puppet.Puppet {
	t.Helper()
	rig, err := puppet.Load(fixture(), "fig/rig.yaml")
	if err != nil {
		t.Fatal(err)
	}
	p := puppet.New(rig, rand.New(rand.NewPCG(1, 2)), 60) //nolint:gosec // Deterministic animation timing, not security.
	t.Cleanup(func() { onLoop(p.Close) })
	return p
}

func frame(p *puppet.Puppet, op *ebiten.DrawImageOptions) *image.RGBA {
	dst := ebiten.NewImage(128, 160)
	defer dst.Deallocate()
	p.Draw(dst, op)
	return px(dst)
}

func TestGraphicsDrawsTheBase(t *testing.T) {
	p := newPuppet(t)
	onLoop(func() {
		img := frame(p, shift(ox, oy))
		if c := at(img, ox+32, oy+70); !near(c, rgba(gray)) {
			t.Errorf("body %v, want gray", c)
		}
		if c := at(img, ox+32, oy+23); !near(c, rgba(white)) {
			t.Errorf("open eyes %v, want white", c)
		}
		// Nothing is drawn far outside the figure: the mesh stays near the picture.
		for _, q := range [][2]int{{2, 2}, {125, 2}, {2, 157}, {125, 157}, {ox - 12, oy + 60}, {ox + 64 + 12, oy + 60}} {
			if c := at(img, q[0], q[1]); c.A != 0 {
				t.Errorf("pixel %v drawn outside the figure: %v", q, c)
			}
		}
		op := shift(ox, oy)
		op.ColorScale.ScaleAlpha(0.5)
		if c := at(frame(p, op), ox+32, oy+70); d(c.A, 128) > 6 {
			t.Errorf("half alpha gives %v", c)
		}
	})
}

func TestGraphicsBlinkTalkAndFace(t *testing.T) {
	p := newPuppet(t)
	var blinked, talked, silentRed bool
	onLoop(func() {
		p.Motion.Speak(true)
		for range 90 {
			p.Motion.Step()
			img := frame(p, shift(ox, oy))
			if near(at(img, ox+32, oy+23), rgba(blue)) {
				blinked = true
			}
			if near(at(img, ox+32, oy+35), rgba(red)) {
				talked = true
			}
		}
		p.Motion.Speak(false)
		p.Motion.Step()
		silentRed = near(at(frame(p, shift(ox, oy)), ox+32, oy+35), rgba(red))
	})
	if !blinked {
		t.Error("the eyes never closed in 1.5 seconds (every 0.3 to 0.6)")
	}
	if !talked {
		t.Error("the mouth never showed the red frame while speaking")
	}
	if silentRed {
		t.Error("the mouth frame stays after Stop")
	}
	onLoop(func() {
		face, ok := p.Rig().Face("glad")
		if !ok {
			t.Fatal("missing glad face")
		}
		p.Motion.SetFace(face)
		mid := false
		for i := range 30 {
			p.Motion.Step()
			c := at(frame(p, shift(ox, oy)), ox+32, oy+15)
			if i == 6 && !near(c, rgba(skin)) && !near(c, rgba(green)) {
				mid = true // between the two faces while fading
			}
		}
		if c := at(frame(p, shift(ox, oy)), ox+32, oy+15); !near(c, rgba(green)) {
			t.Errorf("glad face %v, want green", c)
		}
		if !mid {
			t.Error("the face changed without a fade")
		}
	})
}

func TestGraphicsHopMovesTheFigure(t *testing.T) {
	p := newPuppet(t)
	onLoop(func() {
		before := frame(p, shift(ox, oy))
		reaction, ok := p.Rig().Spec.ReactionOf("jump")
		if !ok {
			t.Fatal("missing jump reaction")
		}
		p.Motion.React(reaction)
		for range 12 { // near the top of the hop
			p.Motion.Step()
		}
		after := frame(p, shift(ox, oy))
		// The bottom row of the body lifts off: the pixel just above the bottom edge empties.
		if before.RGBAAt(ox+32, oy+94).A == 0 || after.RGBAAt(ox+32, oy+94).A != 0 {
			t.Errorf("hop did not lift the figure: before %v after %v", before.RGBAAt(ox+32, oy+94), after.RGBAAt(ox+32, oy+94))
		}
	})
}

func TestGraphicsCloseStopsDrawing(t *testing.T) {
	p := newPuppet(t)
	onLoop(func() {
		p.Close()
		if c := at(frame(p, shift(ox, oy)), ox+32, oy+70); c.A != 0 {
			t.Errorf("drawn after Close: %v", c)
		}
	})
}

// A swaying part trails the body during a hop and changes no pixel beyond its
// area and the mesh cell around it (8 pixels for this small figure).
func TestGraphicsSpringStaysInItsArea(t *testing.T) {
	tassel := image.Rect(40, 60, 46, 90)
	area := image.Rect(34, 56, 52, 96).Inset(-8) // the part's rect below and one mesh cell
	load := func(extra string) *puppet.Puppet {
		fsys := fixture()
		fsys["fig/tassel.png"] = &fstest.MapFile{Data: figure(64, 96, map[image.Rectangle]color.NRGBA{tassel: red})}
		fsys["fig/rig.yaml"] = &fstest.MapFile{Data: []byte(strings.Replace(rigYAML, "base: base.png", "base: tassel.png", 1) + extra)}
		rig, err := puppet.Load(fsys, "fig/rig.yaml")
		if err != nil {
			t.Fatal(err)
		}
		p := puppet.New(rig, rand.New(rand.NewPCG(1, 2)), 60) //nolint:gosec // Deterministic animation timing, not security.
		t.Cleanup(func() { onLoop(p.Close) })
		return p
	}
	plain := load("")
	part := load("springs:\n  - {rect: [34, 56, 18, 40], follow: body, axis: y}\n")
	onLoop(func() {
		for _, p := range []*puppet.Puppet{plain, part} {
			reaction, ok := p.Rig().Spec.ReactionOf("jump")
			if !ok {
				t.Fatal("missing jump reaction")
			}
			p.Motion.React(reaction)
		}
		inside := 0
		for range 30 {
			plain.Motion.Step()
			part.Motion.Step()
			a, b := frame(plain, shift(ox, oy)), frame(part, shift(ox, oy))
			for y := range 160 {
				for x := range 128 {
					if near(a.RGBAAt(x, y), b.RGBAAt(x, y)) {
						continue
					}
					// The body moves with the hop, so allow the area's own shift: up to
					// the hop height above it.
					if !(image.Rectangle{Min: area.Min.Add(image.Pt(ox, oy-20)), Max: area.Max.Add(image.Pt(ox, oy))}).Overlaps(image.Rect(x, y, x+1, y+1)) {
						t.Fatalf("pixel (%d, %d) outside the part differs: %v vs %v", x, y, a.RGBAAt(x, y), b.RGBAAt(x, y))
					}
					inside++
				}
			}
		}
		if inside == 0 {
			t.Error("the part never trailed the hop")
		}
	})
}

// A neutral 64x96 figure: a gray body, a lighter head, and white eyes. The
// variants change the eyes (blue when closed), the mouth (red), and the face (green).
var (
	gray  = color.NRGBA{120, 120, 130, 255}
	skin  = color.NRGBA{230, 210, 190, 255}
	white = color.NRGBA{250, 250, 250, 255}
	blue  = color.NRGBA{20, 40, 220, 255}
	red   = color.NRGBA{220, 30, 30, 255}
	green = color.NRGBA{20, 200, 40, 255}
	head  = image.Rect(16, 8, 48, 44)
	eyes  = image.Rect(22, 20, 42, 26)
	mouth = image.Rect(26, 32, 38, 38)
)

func figure(w, h int, extra map[image.Rectangle]color.NRGBA) []byte {
	img := image.NewNRGBA(image.Rect(0, 0, w, h))
	fill := func(r image.Rectangle, c color.NRGBA) {
		for y := r.Min.Y; y < r.Max.Y; y++ {
			for x := r.Min.X; x < r.Max.X; x++ {
				img.SetNRGBA(x, y, c)
			}
		}
	}
	fill(image.Rect(8, 40, 56, 96), gray)
	fill(head, skin)
	fill(eyes, white)
	for r, c := range extra {
		fill(r, c)
	}
	var b bytes.Buffer
	if err := png.Encode(&b, img); err != nil {
		panic(err)
	}
	return b.Bytes()
}

func solid(r image.Rectangle, c color.NRGBA) []byte {
	img := image.NewNRGBA(image.Rect(0, 0, r.Dx(), r.Dy()))
	for i := 0; i < len(img.Pix); i += 4 {
		img.Pix[i], img.Pix[i+1], img.Pix[i+2], img.Pix[i+3] = c.R, c.G, c.B, c.A
	}
	var b bytes.Buffer
	if err := png.Encode(&b, img); err != nil {
		panic(err)
	}
	return b.Bytes()
}

const rigYAML = `
base: base.png
face: calm
faces:
  calm: {}
  glad: {image: glad.png}
eyes:
  closed: closed.png
  every: [0.3, 0.6]
mouth:
  frames: [{image: mouth.png, at: [26, 32]}, face]
  speed: 12
head: {center: [32, 26], radius: 20, neck: [32, 52]}
hair:
  - rect: [12, 10, 8, 30]
reactions:
  jump: {kind: hop, amount: 2}
`

func fixture() fstest.MapFS {
	return fstest.MapFS{
		"fig/rig.yaml":   {Data: []byte(rigYAML)},
		"fig/base.png":   {Data: figure(64, 96, nil)},
		"fig/closed.png": {Data: figure(64, 96, map[image.Rectangle]color.NRGBA{eyes: blue})},
		"fig/glad.png":   {Data: figure(64, 96, map[image.Rectangle]color.NRGBA{image.Rect(20, 12, 44, 18): green})},
		"fig/mouth.png":  {Data: solid(mouth, red)},
	}
}

func TestGraphicsSoftIdleStaysLocal(t *testing.T) {
	load := func(idle string) *puppet.Puppet {
		fsys := fixture()
		fsys["fig/base.png"] = &fstest.MapFile{Data: figure(64, 96, map[image.Rectangle]color.NRGBA{image.Rect(40, 60, 46, 82): red})}
		fsys["fig/rig.yaml"] = &fstest.MapFile{Data: []byte("base: base.png\nsoft_sway: [{ellipse: [34, 52, 20, 36], follow: body, axis: y, limit: 4" + idle + "}]\nmotion: {hit: soft_only, breath: {depth: 0}, sway: {angle: 0, shift: 0}}\n")}
		rig, err := puppet.Load(fsys, "fig/rig.yaml")
		if err != nil {
			t.Fatal(err)
		}
		p := puppet.New(rig, rand.New(rand.NewPCG(1, 2)), 60) //nolint:gosec // Reproducible animation, not security.
		t.Cleanup(func() { onLoop(p.Close) })
		return p
	}
	control, moving := load(""), load(", idle: {amplitude: 4, period: 2}")
	onLoop(func() {
		baseline := frame(control, shift(ox, oy))
		// A 64x96 image uses an eight-pixel mesh cell. Allow that cell and the
		// four-pixel maximum displacement around the authored ellipse bounds.
		area := image.Rect(34, 52, 54, 88).Inset(-12).Add(image.Pt(ox, oy))
		changed := 0
		for i := range 120 {
			control.Motion.Step()
			moving.Motion.Step()
			if i%15 != 14 {
				continue
			}
			a, b := frame(control, shift(ox, oy)), frame(moving, shift(ox, oy))
			if !bytes.Equal(a.Pix, baseline.Pix) {
				t.Error("static control changed")
			}
			for y := range b.Bounds().Dy() {
				for x := range b.Bounds().Dx() {
					if a.RGBAAt(x, y) == b.RGBAAt(x, y) {
						continue
					}
					changed++
					if !image.Pt(x, y).In(area) {
						t.Errorf("idle changed pixel outside local region: %d,%d", x, y)
						return
					}
				}
			}
		}
		if changed == 0 {
			t.Error("full-health idle never changed a pixel")
		}
	})
}

// A puppet drawn at half its size with Reduce composes at half the size and looks like
// the one composed at full size: the same colors inside the parts, the blink and the
// mouth still shown.
func TestGraphicsReduceKeepsTheFigure(t *testing.T) {
	full, small := newPuppet(t), newPuppet(t)
	small.Reduce = true
	half := func() *ebiten.DrawImageOptions {
		op := &ebiten.DrawImageOptions{}
		op.GeoM.Scale(0.5, 0.5)
		op.GeoM.Translate(ox, oy)
		return op
	}
	onLoop(func() {
		a, b := frame(full, half()), frame(small, half())
		differ, drawn := 0, 0
		for y := range 160 {
			for x := range 128 {
				if a.RGBAAt(x, y).A != 0 || b.RGBAAt(x, y).A != 0 {
					drawn++
				}
				if !near(a.RGBAAt(x, y), b.RGBAAt(x, y)) {
					differ++
				}
			}
		}
		// Only the one-pixel edges between parts may differ (a box filter, not a sample).
		if drawn == 0 || differ*10 > drawn {
			t.Errorf("%d of %d drawn pixels differ", differ, drawn)
		}
		for _, q := range []struct {
			x, y int
			c    color.NRGBA
		}{{16, 35, gray}, {16, 11, white}, {16, 6, skin}} {
			if c := at(b, ox+q.x, oy+q.y); !near(c, rgba(q.c)) {
				t.Errorf("reduced (%d, %d) %v, want %v", q.x, q.y, c, q.c)
			}
		}
		small.Motion.Speak(true)
		blinked, talked := false, false
		for range 90 {
			small.Motion.Step()
			img := frame(small, half())
			blinked = blinked || near(at(img, ox+16, oy+11), rgba(blue))
			talked = talked || near(at(img, ox+16, oy+17), rgba(red))
		}
		if !blinked || !talked {
			t.Errorf("reduced: blinked %v, talked %v", blinked, talked)
		}
	})
}

// largeRig is a 4096x6144 figure: an opaque body, a 1200 pixel face patch, closed eyes
// and two mouth frames, the size of a high-resolution standing portrait.
func largeRig(b *testing.B) *puppet.Rig {
	b.Helper()
	spec, err := puppetdef.Parse([]byte("base: base.png\nface: calm\nfaces: {calm: {}, glad: {image: glad.png}}\n" +
		"head: {center: [2048, 1200], radius: 600, neck: [2048, 2000]}\n" +
		"hair: [{rect: [1200, 700, 400, 2200]}, {rect: [2500, 700, 400, 2200]}]\n" +
		"soft_sway: [{ellipse: [1500, 2600, 1100, 700], follow: body, axis: y, limit: 40}]\n"))
	if err != nil {
		b.Fatal(err)
	}
	const w, h = 4096, 6144
	paint := func(w, h int, c color.NRGBA) *image.NRGBA {
		img := image.NewNRGBA(image.Rect(0, 0, w, h))
		for i := 0; i < len(img.Pix); i += 4 {
			img.Pix[i], img.Pix[i+1], img.Pix[i+2], img.Pix[i+3] = c.R, c.G, c.B, c.A
		}
		return img
	}
	base := image.NewNRGBA(image.Rect(0, 0, w, h))
	body := paint(2400, 5600, gray)
	for y := range 5600 {
		copy(base.Pix[(y+544)*base.Stride+848*4:], body.Pix[y*body.Stride:(y+1)*body.Stride])
	}
	patch := func(x, y, w, h int, c color.NRGBA) *puppet.Patch {
		return &puppet.Patch{Img: paint(w, h, c), At: image.Pt(x, y)}
	}
	eyes := &puppet.Eyes{Closed: *patch(1648, 1000, 800, 200, blue), Every: [2]float64{0.5, 1}}
	mouth := &puppet.Mouth{Frames: []*puppet.Patch{patch(1898, 1500, 300, 150, red), nil}, Speed: 12}
	return &puppet.Rig{Spec: spec, Base: base, Faces: []puppet.Face{
		{Name: "calm", Eyes: eyes, Mouth: mouth},
		{Name: "glad", Patch: patch(1448, 600, 1200, 1200, green), Eyes: eyes, Mouth: mouth},
	}}
}

// BenchmarkGraphicsLargePuppet is the cost of a frame of a 4096x6144 puppet drawn 1.2
// screens tall on a 1280x720 screen, waiting for the GPU: idle (breathing and sway,
// nothing recomposed), talking with a face change every frame (recomposed every frame),
// and the first frame (the pictures made). static is the picture drawn without a
// puppet, as the novel stage draws it. Run it with xvfb-run (or on a display):
// go test -tags=graphics -run '^$' -bench LargePuppet ./internal/layer5/puppet.
func BenchmarkGraphicsLargePuppet(b *testing.B) {
	rig := largeRig(b)
	w, h := rig.Size()
	scale := 720 * 1.2 / float64(h)
	op := func() *ebiten.DrawImageOptions {
		op := &ebiten.DrawImageOptions{}
		op.GeoM.Scale(scale, scale)
		op.GeoM.Translate(640-float64(w)*scale/2, -100)
		return op
	}
	var dst *ebiten.Image
	sync := func() {
		var px [4]byte
		if one, ok := dst.SubImage(image.Rect(640, 400, 641, 401)).(*ebiten.Image); ok {
			one.ReadPixels(px[:])
		}
	}
	onLoop(func() { dst = ebiten.NewImage(1280, 720) })
	defer onLoop(dst.Deallocate)
	b.Run("static", func(b *testing.B) {
		onLoop(func() {
			img := ebiten.NewImageFromImage(rig.Base)
			defer img.Deallocate()
			for b.Loop() {
				dst.Clear()
				o := op()
				o.Filter = ebiten.FilterLinear
				dst.DrawImage(img, o)
				sync()
			}
		})
	})
	for _, reduce := range []bool{false, true} {
		name := "full"
		if reduce {
			name = "reduced"
		}
		p := puppet.New(rig, rand.New(rand.NewPCG(1, 2)), 60) //nolint:gosec // Deterministic animation timing, not security.
		p.Reduce = reduce
		b.Run(name+"/idle", func(b *testing.B) {
			onLoop(func() {
				p.Draw(dst, op())
				sync()
				for b.Loop() {
					dst.Clear()
					p.Motion.Step()
					p.Draw(dst, op())
					sync()
				}
			})
		})
		b.Run(name+"/recompose", func(b *testing.B) {
			onLoop(func() {
				p.Motion.Speak(true)
				face := 0
				for b.Loop() {
					face = 1 - face
					p.Motion.SetFace(face)
					dst.Clear()
					p.Motion.Step()
					p.Draw(dst, op())
					sync()
				}
			})
		})
		onLoop(p.Close)
		b.Run(name+"/first", func(b *testing.B) {
			onLoop(func() {
				for b.Loop() {
					q := puppet.New(rig, rand.New(rand.NewPCG(1, 2)), 60) //nolint:gosec // Deterministic animation timing, not security.
					q.Reduce = reduce
					q.Draw(dst, op())
					sync()
					q.Close()
				}
			})
		})
	}
}
