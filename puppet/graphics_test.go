//go:build graphics

package puppet_test

import (
	"errors"
	"image"
	"image/color"
	"os"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"

	"github.com/nao1215/gozen/puppet"
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
	p, err := puppet.New(rig, puppet.Config{})
	if err != nil {
		t.Fatal(err)
	}
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
		p.Speak()
		for range 90 {
			p.Update()
			img := frame(p, shift(ox, oy))
			if near(at(img, ox+32, oy+23), rgba(blue)) {
				blinked = true
			}
			if near(at(img, ox+32, oy+35), rgba(red)) {
				talked = true
			}
		}
		p.Stop()
		p.Update()
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
		if err := p.SetFace("glad"); err != nil {
			t.Fatal(err)
		}
		mid := false
		for i := range 30 {
			p.Update()
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
		if err := p.React("jump"); err != nil {
			t.Fatal(err)
		}
		for range 12 { // near the top of the hop
			p.Update()
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
