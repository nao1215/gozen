package puppet_test

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/nao1215/gozen/puppet"
)

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

func TestLoadAndNew(t *testing.T) {
	rig, err := puppet.Load(fixture(), "fig/rig.yaml")
	if err != nil {
		t.Fatal(err)
	}
	if w, h := rig.Size(); w != 64 || h != 96 {
		t.Fatalf("size %dx%d", w, h)
	}
	p, err := puppet.New(rig, puppet.Config{Face: "glad"})
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"jump", "hop", "shrink", "shake", "nod", "tilt"} {
		if err := p.React(name); err != nil {
			t.Fatalf("React(%q): %v", name, err)
		}
	}
	if err := p.React("dance"); err == nil {
		t.Fatal("unknown reaction accepted")
	}
	if err := p.SetFace("calm"); err != nil {
		t.Fatal(err)
	}
	if err := p.SetFace("angry"); err == nil {
		t.Fatal("unknown face accepted")
	}
	p.Speak()
	for range 120 {
		p.Update()
	}
	p.Stop()
	p.Close()
	p.Close() // twice is a no-op
}

func TestNewErrors(t *testing.T) {
	rig, err := puppet.Load(fixture(), "fig/rig.yaml")
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name string
		rig  *puppet.Rig
		cfg  puppet.Config
		want string
	}{
		{"nil rig", nil, puppet.Config{}, "nil rig"},
		{"negative TPS", rig, puppet.Config{TPS: -1}, "TPS"},
		{"unknown face", rig, puppet.Config{Face: "x"}, "no face"},
	} {
		if _, err := puppet.New(tc.rig, tc.cfg); err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Errorf("%s: got %v, want %q", tc.name, err, tc.want)
		}
	}
}

func TestLoadReportsThePlace(t *testing.T) {
	fsys := fixture()
	fsys["fig/rig.yaml"] = &fstest.MapFile{Data: []byte(strings.Replace(rigYAML, "at: [26, 32]", "at: [60, 32]", 1))}
	_, err := puppet.Load(fsys, "fig/rig.yaml")
	if err == nil || !strings.Contains(err.Error(), "fig/rig.yaml") || !strings.Contains(err.Error(), "mouth.frames[0]") {
		t.Fatalf("got %v", err)
	}
	if _, err := puppet.Load(nil, "fig/rig.yaml"); err == nil {
		t.Fatal("nil FS accepted")
	}
}
