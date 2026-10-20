package puppet

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"math"
	"math/rand/v2"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/goccy/go-yaml"

	"github.com/nao1215/gozen/internal/layer2/puppetdef"
)

// Fixture pictures: a 120x160 figure. The base is a gray body with a face; the
// variants change only small boxes, so the cut must find exactly those boxes.
const fw, fh = 120, 160

func pic(w, h int, fill color.NRGBA, boxes map[image.Rectangle]color.NRGBA) []byte {
	img := image.NewNRGBA(image.Rect(0, 0, w, h))
	for y := range h {
		for x := range w {
			img.SetNRGBA(x, y, fill)
		}
	}
	for r, c := range boxes {
		for y := r.Min.Y; y < r.Max.Y; y++ {
			for x := r.Min.X; x < r.Max.X; x++ {
				img.SetNRGBA(x, y, c)
			}
		}
	}
	var b bytes.Buffer
	if err := png.Encode(&b, img); err != nil {
		panic(err)
	}
	return b.Bytes()
}

var (
	gray  = color.NRGBA{128, 128, 128, 255}
	white = color.NRGBA{250, 250, 250, 255}
	blue  = color.NRGBA{20, 40, 220, 255}
	red   = color.NRGBA{220, 30, 30, 255}
	green = color.NRGBA{20, 200, 40, 255}
	eyes  = image.Rect(40, 40, 80, 50)
	mouth = image.Rect(50, 64, 70, 72)
)

const rigYAML = `
base: base.png
face: calm
faces:
  calm: {}
  happy: {image: happy.png}
eyes:
  closed: eyes.png
  every: [0.5, 1]
  double: 0.5
mouth:
  frames: [face, {image: m.png, at: [50, 64]}]
head: {center: [60, 50], radius: 30, neck: [60, 100]}
hair:
  - rect: [20, 30, 20, 60]
`

func fixture() fstest.MapFS {
	return fstest.MapFS{
		"p/rig.yaml":  {Data: []byte(rigYAML)},
		"p/base.png":  {Data: pic(fw, fh, gray, map[image.Rectangle]color.NRGBA{eyes: white})},
		"p/eyes.png":  {Data: pic(fw, fh, gray, map[image.Rectangle]color.NRGBA{eyes: blue})},
		"p/happy.png": {Data: pic(fw, fh, gray, map[image.Rectangle]color.NRGBA{eyes: white, mouth: green})},
		"p/m.png":     {Data: pic(mouth.Dx(), mouth.Dy(), red, nil)},
	}
}

func load(t *testing.T) *Rig {
	t.Helper()
	r, err := Load(fixture(), "p/rig.yaml")
	if err != nil {
		t.Fatal(err)
	}
	return r
}

func TestLoadCutsDifferences(t *testing.T) {
	r := load(t)
	if w, h := r.Size(); w != fw || h != fh {
		t.Fatalf("size %dx%d", w, h)
	}
	if len(r.Faces) != 2 || r.Faces[0].Name != "calm" || r.Faces[1].Name != "happy" {
		t.Fatalf("faces %+v", r.Faces)
	}
	closed := r.Faces[0].Eyes.Closed.Rect()
	if !eyes.In(closed) || closed.Dx() > eyes.Dx()+4*cutBlock+cutBlock || closed.Dy() > eyes.Dy()+4*cutBlock+cutBlock {
		t.Fatalf("closed eyes cut to %v, want a padded box around %v", closed, eyes)
	}
	// The patch is opaque inside the changed box and fades at the padded edge.
	c := r.Faces[0].Eyes.Closed
	if a := c.Img.NRGBAAt(eyes.Min.X-c.At.X+5, eyes.Min.Y-c.At.Y+5).A; a != 255 {
		t.Fatalf("inside alpha %d", a)
	}
	if a := c.Img.NRGBAAt(0, 0).A; a > 40 {
		t.Fatalf("corner alpha %d", a)
	}
	happy := r.Faces[1].Patch.Rect()
	if !mouth.In(happy) {
		t.Fatalf("happy face cut to %v, want around %v", happy, mouth)
	}
	m := r.Faces[0].Mouth
	if len(m.Frames) != 2 || m.Frames[0] != nil || m.Frames[1].At != mouth.Min || m.Speed != 10 {
		t.Fatalf("mouth %+v", m)
	}
	// The happy face inherits the eyes, cut against the happy picture.
	if r.Faces[1].Eyes == nil || !eyes.In(r.Faces[1].Eyes.Closed.Rect()) {
		t.Fatal("happy face lacks the closed eyes")
	}
}

func TestLoadErrors(t *testing.T) {
	for _, tc := range []struct {
		name, file string
		data       []byte
		want       string
	}{
		{"missing picture", "p/m.png", nil, "m.png"},
		{"outside", "p/rig.yaml", []byte(strings.Replace(rigYAML, "at: [50, 64]", "at: [110, 64]", 1)), "outside the base"},
		{"wrong size", "p/eyes.png", pic(fw, fh+1, gray, nil), "as large as the base"},
		{"no difference", "p/eyes.png", pic(fw, fh, gray, map[image.Rectangle]color.NRGBA{eyes: white}), "does not differ"},
		{"not png", "p/eyes.png", []byte("text"), "eyes.png"},
		{"bad yaml", "p/rig.yaml", []byte("base: base.png\nsize: 3\n"), "size"},
		{"escape", "p/rig.yaml", []byte("base: ../../base.png\n"), "leaves"},
		{"area outside", "p/rig.yaml", []byte(strings.Replace(rigYAML, "closed: eyes.png", "closed: {image: eyes.png, area: [100, 0, 50, 10]}", 1)), "area"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			fsys := fixture()
			if tc.data == nil {
				delete(fsys, tc.file)
			} else {
				fsys[tc.file] = &fstest.MapFile{Data: tc.data}
			}
			_, err := Load(fsys, "p/rig.yaml")
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("got %v, want %q", err, tc.want)
			}
		})
	}
	if _, err := Load(fixture(), "../rig.yaml"); err == nil {
		t.Fatal("invalid path accepted")
	}
}

func TestCutIgnoresSpecks(t *testing.T) {
	base := image.NewNRGBA(image.Rect(0, 0, 200, 200))
	v := image.NewNRGBA(base.Bounds())
	for i := range base.Pix {
		base.Pix[i] = 100
		v.Pix[i] = 100 + uint8(i%7) // a little noise everywhere, as after a redraw
	}
	for y := 120; y < 150; y++ {
		for x := 60; x < 140; x++ {
			v.SetNRGBA(x, y, color.NRGBA{255, 0, 0, 255})
		}
	}
	v.SetNRGBA(10, 10, color.NRGBA{0, 0, 0, 255}) // a speck far away
	p, ok := Cut(base, v, image.Rectangle{})
	if !ok {
		t.Fatal("nothing found")
	}
	if r := p.Rect(); !image.Rect(60, 120, 140, 150).In(r) || r.Min.X < 40 || r.Min.Y < 100 {
		t.Fatalf("cut %v", r)
	}
	if _, ok := Cut(base, base, image.Rectangle{}); ok {
		t.Fatal("identical pictures differ")
	}
}

func seeded(a, b uint64) *rand.Rand {
	return rand.New(rand.NewPCG(a, b)) //nolint:gosec // Deterministic animation test, not security-sensitive data.
}

func run(m *Motion, n int, f func(int, Pose)) {
	for i := range n {
		m.Step()
		f(i, m.Pose())
	}
}

func TestMotionIsDeterministic(t *testing.T) {
	r := load(t)
	a := NewMotion(r, seeded(3, 4), 60)
	b := NewMotion(r, seeded(3, 4), 60)
	c := NewMotion(r, seeded(5, 6), 60)
	differs := false
	for i := range 900 {
		if i == 100 {
			for _, m := range []*Motion{a, b, c} {
				m.Speak(true)
				hop, _ := r.Spec.ReactionOf("hop")
				m.React(hop)
			}
		}
		a.Step()
		b.Step()
		c.Step()
		if a.Pose() != b.Pose() {
			t.Fatalf("step %d: same seed, different pose\n%+v\n%+v", i, a.Pose(), b.Pose())
		}
		if a.Pose() != c.Pose() {
			differs = true
		}
	}
	if !differs {
		t.Fatal("different seeds gave the same motion")
	}
}

func TestBlinkAndMouth(t *testing.T) {
	r := load(t)
	m := NewMotion(r, seeded(1, 1), 60)
	blinks, closedFrames := 0, 0
	open := true
	run(m, 600, func(_ int, p Pose) {
		if p.Blink > 0.99 {
			closedFrames++
		}
		if p.Blink > 0 && open {
			blinks++
		}
		open = p.Blink == 0
		if p.Mouth != -1 {
			t.Fatal("mouth moves while silent")
		}
	})
	// every [0.5, 1] seconds over 10 seconds, sometimes twice in a row.
	if blinks < 9 || blinks > 30 || closedFrames == 0 {
		t.Fatalf("%d blinks, %d closed frames", blinks, closedFrames)
	}
	m.Speak(true)
	seen := map[int]bool{}
	run(m, 120, func(_ int, p Pose) { seen[p.Mouth] = true })
	if !seen[-1] || !seen[1] {
		t.Fatalf("mouth frames while speaking: %v", seen)
	}
	m.Speak(false)
	m.Step()
	if m.Pose().Mouth != -1 {
		t.Fatal("mouth still moving after Stop")
	}
}

func TestFaceFade(t *testing.T) {
	r := load(t)
	m := NewMotion(r, seeded(1, 1), 60)
	m.SetFace(1)
	if p := m.Pose(); p.Face != 1 || p.PrevFace != 0 || p.Fade != 0 {
		t.Fatalf("%+v", p)
	}
	run(m, 8, func(int, Pose) {})
	if f := m.Pose().Fade; f <= 0 || f >= 1 {
		t.Fatalf("fade %v halfway", f)
	}
	run(m, 20, func(int, Pose) {})
	if f := m.Pose().Fade; f != 1 {
		t.Fatalf("fade %v after 0.47 s", f)
	}
}

func TestReactionsSettle(t *testing.T) {
	r := load(t)
	for _, kind := range []string{"hop", "shrink", "shake", "nod", "tilt"} {
		quiet := NewMotion(r, seeded(9, 9), 60)
		m := NewMotion(r, seeded(9, 9), 60)
		react, _ := r.Spec.ReactionOf(kind)
		m.React(react)
		moved := false
		for range 180 {
			quiet.Step()
			m.Step()
			a, b := quiet.Pose(), m.Pose()
			if a.Body != b.Body || a.BodyScale != b.BodyScale || a.HeadAngle != b.HeadAngle || a.HeadShift != b.HeadShift {
				moved = true
			}
		}
		if !moved {
			t.Fatalf("%s did nothing", kind)
		}
		a, b := quiet.Pose(), m.Pose()
		if math.Abs(a.Body[1]-b.Body[1]) > 1e-9 || math.Abs(a.HeadAngle-b.HeadAngle) > 0.002 {
			t.Fatalf("%s did not settle: %+v vs %+v", kind, a, b)
		}
	}
}

func TestMeshStaysInBounds(t *testing.T) {
	r := load(t)
	mesh := NewMesh(r)
	w, h := r.Size()
	if len(mesh.Src) != mesh.Cols*mesh.Rows || len(mesh.Src) > math.MaxUint16 || len(mesh.Index) != (mesh.Cols-1)*(mesh.Rows-1)*6 {
		t.Fatalf("mesh %dx%d, %d vertices, %d indices", mesh.Cols, mesh.Rows, len(mesh.Src), len(mesh.Index))
	}
	for _, s := range mesh.Src {
		if s[0] < 0 || s[1] < 0 || s[0] > float32(w) || s[1] > float32(h) {
			t.Fatalf("source %v outside %dx%d", s, w, h)
		}
	}
	// Run long with every reaction firing often: the deformed vertices must stay
	// near the picture. The hair spring is clamped, reactions are bounded.
	m := NewMotion(r, seeded(2, 2), 60)
	m.Speak(true)
	dst := make([][2]float32, len(mesh.Src))
	unit := r.Spec.Head.Radius
	limit := unit
	rng := seeded(8, 8)
	for i := range 3000 {
		if i%7 == 0 {
			react, _ := r.Spec.ReactionOf([]string{"hop", "shrink", "shake", "nod", "tilt"}[rng.IntN(5)])
			react.Amount = 1 + rng.Float64()/2
			m.React(react)
		}
		m.Step()
		mesh.Deform(m.Pose(), dst)
		for k, d := range dst {
			if math.IsNaN(float64(d[0])) || d[0] < -float32(limit) || d[1] < -float32(limit) || d[0] > float32(float64(w)+limit) || d[1] > float32(float64(h)+limit) {
				t.Fatalf("step %d vertex %d at %v leaves %dx%d by more than %v", i, k, d, w, h, limit)
			}
		}
	}
}

func TestMeshWeights(t *testing.T) {
	r := load(t)
	mesh := NewMesh(r)
	pose := Pose{HeadAngle: 0.1, HeadShift: [2]float64{3, 2}, BodyScale: [2]float64{1, 1}}
	dst := make([][2]float32, len(mesh.Src))
	mesh.Deform(pose, dst)
	for k, s := range mesh.Src {
		moved := math.Hypot(float64(dst[k][0]-s[0]), float64(dst[k][1]-s[1]))
		switch {
		case s[1] >= 100 && moved > 1e-3: // below the neck root nothing follows the head
			t.Fatalf("vertex %v below the neck moved %v", s, moved)
		case math.Hypot(float64(s[0])-60, float64(s[1])-50) < 20 && moved < 1:
			t.Fatalf("vertex %v in the head did not move", s)
		}
	}
	// The identity pose leaves every vertex in place.
	mesh.Deform(Pose{BodyScale: [2]float64{1, 1}}, dst)
	for k, s := range mesh.Src {
		if dst[k] != s {
			t.Fatalf("identity moved %v to %v", s, dst[k])
		}
	}
}

func TestPackRoundTrip(t *testing.T) {
	r := load(t)
	spec, files := Pack(r)
	b, err := yaml.Marshal(spec)
	if err != nil {
		t.Fatal(err)
	}
	fsys := fstest.MapFS{"out/rig.yaml": {Data: b}}
	for name, img := range files {
		var buf bytes.Buffer
		if err := png.Encode(&buf, img); err != nil {
			t.Fatal(err)
		}
		fsys["out/"+name] = &fstest.MapFile{Data: buf.Bytes()}
	}
	again, err := Load(fsys, "out/rig.yaml")
	if err != nil {
		t.Fatalf("%v\n%s", err, b)
	}
	if Describe(again) != Describe(r) {
		t.Fatalf("packed rig differs:\n%s\nwant:\n%s", Describe(again), Describe(r))
	}
	for i, f := range r.Faces {
		g := again.Faces[i]
		if !sameOverlay(eyesPatches(f), eyesPatches(g)) || !sameOverlay(mouthPatches(f), mouthPatches(g)) {
			t.Fatalf("face %s overlays differ", f.Name)
		}
		if (f.Patch == nil) != (g.Patch == nil) || f.Patch != nil && !sameOverlay([]*Patch{f.Patch}, []*Patch{g.Patch}) {
			t.Fatalf("face %s picture differs", f.Name)
		}
	}
	// Only the cut pictures are written: they are much smaller than the base.
	for name, img := range files {
		if name != "base.png" && img.Bounds().Dx()*img.Bounds().Dy()*4 > fw*fh {
			t.Errorf("%s is %v, not cut down", name, img.Bounds())
		}
	}
}

// A freeze stops the idle motion while it plays and lets it go on afterwards.
func TestFreeze(t *testing.T) {
	m := NewMotion(load(t), seeded(1, 2), 60)
	for range 30 {
		m.Step()
	}
	m.React(puppetdef.Reaction{Kind: "freeze", Time: 0.5})
	m.Step()
	before := m.t
	for range 20 {
		m.Step()
	}
	if m.t != before || !m.frozen() {
		t.Fatalf("time moved while frozen: %v -> %v", before, m.t)
	}
	for range 30 {
		m.Step()
	}
	if m.frozen() || m.t == before {
		t.Fatal("still frozen after its time")
	}
}
