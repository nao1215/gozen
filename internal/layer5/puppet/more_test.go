package puppet

import (
	"bytes"
	"image/png"
	"math"
	"testing"
	"testing/fstest"

	"github.com/goccy/go-yaml"
	"github.com/hajimehoshi/ebiten/v2"

	"github.com/nao1215/gozen/internal/layer2/puppetdef"
)

func TestRigFaceLookup(t *testing.T) {
	r := load(t)
	if i, ok := r.Face("happy"); !ok || i != 1 {
		t.Fatalf("happy: %d %v", i, ok)
	}
	if i, ok := r.Face("calm"); !ok || i != 0 {
		t.Fatalf("calm: %d %v", i, ok)
	}
	if _, ok := r.Face("angry"); ok {
		t.Fatal("unknown face found")
	}
}

func TestResolveParamsDefaultsAndOverrides(t *testing.T) {
	plain := &puppetdef.Spec{Head: &puppetdef.Head{Center: []float64{60, 50}, Radius: 30, Neck: []float64{60, 100}}}
	p := resolveParams(plain, 120, 160)
	if !p.hasHead || p.unit != 30 || p.fade != 0.25 || p.breathPeriod != 3.8 || p.swayPeriod != 6.5 {
		t.Fatalf("defaults %+v", p)
	}
	if math.Abs(p.swayShift-0.75) > 1e-12 || math.Abs(p.talkNod-0.6) > 1e-12 || p.tiltEvery != [2]float64{5, 11} {
		t.Fatalf("defaults scale with the head radius: %+v", p)
	}

	custom := &puppetdef.Spec{
		Head: plain.Head, Fade: 0.5,
		Motion: puppetdef.Motion{
			Breath: &puppetdef.Breath{Depth: 0.01, Period: 2},
			Sway:   &puppetdef.Sway{Angle: 4, Shift: 3, Period: 5},
			Tilt:   &puppetdef.Tilt{Angle: 10, Every: []float64{1, 2}},
			Talk:   &puppetdef.Talk{Nod: 5},
		},
	}
	p = resolveParams(custom, 120, 160)
	if p.breathDepth != 0.01 || p.breathPeriod != 2 || p.swayShift != 3 || p.swayPeriod != 5 || p.talkNod != 5 || p.fade != 0.5 {
		t.Fatalf("overrides %+v", p)
	}
	if math.Abs(p.swayAngle-4*math.Pi/180) > 1e-12 || math.Abs(p.tiltAngle-10*math.Pi/180) > 1e-12 || p.tiltEvery != [2]float64{1, 2} {
		t.Fatalf("angles %+v", p)
	}
	// A zero period inside a block means the default period.
	zero := &puppetdef.Spec{Head: plain.Head, Motion: puppetdef.Motion{Breath: &puppetdef.Breath{Depth: 0}, Sway: &puppetdef.Sway{}}}
	p = resolveParams(zero, 120, 160)
	if p.breathDepth != 0 || p.breathPeriod != 3.8 || p.swayPeriod != 6.5 || p.swayAngle != 0 {
		t.Fatalf("zero strengths turn motion off, zero periods use defaults: %+v", p)
	}

	// Without a head nothing turns or nods, and the unit is a fifth of the height.
	p = resolveParams(&puppetdef.Spec{}, 120, 160)
	if p.hasHead || p.unit != 32 || p.swayAngle != 0 || p.swayShift != 0 || p.tiltAngle != 0 || p.talkNod != 0 {
		t.Fatalf("no head: %+v", p)
	}
	if p.breathDepth == 0 {
		t.Fatal("a headless figure still breathes")
	}
}

func TestSetFaceAndSkipFade(t *testing.T) {
	m := NewMotion(load(t), seeded(1, 1), 60)
	m.SetFace(0)
	if p := m.Pose(); p.Fade != 1 || p.Face != 0 {
		t.Fatalf("setting the current face must do nothing: %+v", p)
	}
	m.SetFace(1)
	m.SkipFade()
	if p := m.Pose(); p.Fade != 1 || p.Face != 1 || p.PrevFace != 1 {
		t.Fatalf("SkipFade: %+v", p)
	}
}

func TestReactionDefaultsAndLimits(t *testing.T) {
	m := NewMotion(load(t), seeded(1, 1), 60)
	m.React(puppetdef.Reaction{Kind: "wiggle"})
	if len(m.reactions) != 0 {
		t.Fatal("an unknown kind without a time must not play")
	}
	m.React(puppetdef.Reaction{Kind: "wiggle", Time: 1})
	if len(m.reactions) != 1 || m.reactions[0].amount != 1 {
		t.Fatalf("explicit time plays with amount 1: %+v", m.reactions)
	}
	times := map[string]float64{"hop": 0.45, "shrink": 0.5, "shake": 0.6, "nod": 0.5, "tilt": 1.4, "freeze": 1.5}
	for kind, want := range times {
		m = NewMotion(load(t), seeded(1, 1), 60)
		m.React(puppetdef.Reaction{Kind: kind})
		if len(m.reactions) != 1 || m.reactions[0].time != want {
			t.Errorf("%s default time %+v, want %v", kind, m.reactions, want)
		}
	}
	// A burst cannot pile up: only the newest eight play.
	m = NewMotion(load(t), seeded(1, 1), 60)
	for i := range 20 {
		m.React(puppetdef.Reaction{Kind: "hop", Amount: float64(i + 1)})
	}
	if len(m.reactions) != 8 || m.reactions[0].amount != 13 || m.reactions[7].amount != 20 {
		t.Fatalf("kept %d reactions, first amount %v", len(m.reactions), m.reactions[0].amount)
	}
	// They end when their time is up.
	for range 60 {
		m.Step()
	}
	if len(m.reactions) != 0 {
		t.Fatalf("%d reactions still playing after a second", len(m.reactions))
	}
}

func TestReactionsOfAFigureWithoutAHead(t *testing.T) {
	r := loadYAML(t, "base: base.png\n")
	quiet, m := NewMotion(r, seeded(2, 2), 60), NewMotion(r, seeded(2, 2), 60)
	m.React(puppetdef.Reaction{Kind: "nod"})
	for range 10 {
		quiet.Step()
		m.Step()
	}
	a, b := quiet.Pose(), m.Pose()
	if b.Body[1] <= a.Body[1] || b.HeadShift != a.HeadShift || b.HeadSquash != 0 {
		t.Fatalf("a headless nod moves the body: %+v vs %+v", b, a)
	}
	m.React(puppetdef.Reaction{Kind: "tilt"})
	m.React(puppetdef.Reaction{Kind: "shake"})
	for range 10 {
		m.Step()
	}
	if m.Pose().HeadAngle != 0 {
		t.Fatalf("a headless figure cannot turn its head: %v", m.Pose().HeadAngle)
	}
}

func TestHeadTiltReaction(t *testing.T) {
	r := load(t)
	quiet, m := NewMotion(r, seeded(2, 2), 60), NewMotion(r, seeded(2, 2), 60)
	m.React(puppetdef.Reaction{Kind: "tilt", Amount: 1, Time: 1})
	for range 30 {
		quiet.Step()
		m.Step()
	}
	if got := m.Pose().HeadAngle - quiet.Pose().HeadAngle; math.Abs(got-12*math.Pi/180) > 0.01 {
		t.Fatalf("holds a tilt of about 12 degrees, got %v", got*180/math.Pi)
	}
}

func TestPolyDist(t *testing.T) {
	sq := [][]float64{{0, 0}, {10, 0}, {10, 10}, {0, 10}}
	cases := []struct{ x, y, want float64 }{
		{5, 5, -5}, {2, 5, -2}, {20, 5, 10}, {5, -3, 3}, {13, 14, 5}, {10, 5, 0},
	}
	for _, c := range cases {
		if got := polyDist(c.x, c.y, sq); math.Abs(math.Abs(got)-math.Abs(c.want)) > 1e-9 || (c.want < 0) != (got < 0) && c.want != 0 {
			t.Errorf("polyDist(%v,%v) = %v, want %v", c.x, c.y, got, c.want)
		}
	}
}

func TestResolveSpringsOptions(t *testing.T) {
	head := &puppetdef.Head{Center: []float64{60, 50}, Radius: 30, Neck: []float64{60, 100}}
	rect := func(x, y, w, h float64) []float64 { return []float64{x, y, w, h} }
	spec := &puppetdef.Spec{
		Head: head,
		Hair: []puppetdef.Shape{{Rect: rect(20, 30, 20, 60)}},
		Motion: puppetdef.Motion{
			Hair: &puppetdef.Spring{Gain: 2, Stiffness: 100, Damping: 9},
		},
		Springs: []puppetdef.Part{
			{Rect: rect(0, 100, 20, 40), Root: puppetdef.Root{Side: "left"}, Axis: "x", Follow: "body", Stiffness: 30, Damping: 4, Amount: 0.5, Limit: 3},
			{Poly: [][]float64{{50, 100}, {70, 100}, {60, 140}}, Root: puppetdef.Root{At: []float64{60, 100}}, Axis: "y"},
			{Rect: rect(80, 100, 20, 40), Root: puppetdef.Root{Side: "bottom"}},
			{Rect: rect(0, 0, 10, 10)},
		},
	}
	sp := resolveSprings(spec, 160)
	if len(sp) != 5 {
		t.Fatalf("%d springs", len(sp))
	}
	hair := sp[0]
	if hair.k != 100 || hair.c != 9 || hair.gain != 2 || !hair.head || hair.side != "top" || hair.limit != 0.15*30 {
		t.Fatalf("hair %+v", hair)
	}
	if hair.root != [2]float64{30, 30} || math.Abs(hair.feather-0.08*30) > 1e-12 {
		t.Fatalf("hair root %v feather %v", hair.root, hair.feather)
	}
	left := sp[1]
	if left.mask != [2]float64{1, 0} || left.head || left.k != 30 || left.c != 4 || left.gain != 0.5 || left.limit != 3 || left.root != [2]float64{0, 120} {
		t.Fatalf("left part %+v", left)
	}
	pt := sp[2]
	if pt.mask != [2]float64{0, 1} || pt.side != "" || pt.root != [2]float64{60, 100} || pt.reachSide < 39 {
		t.Fatalf("point part %+v", pt)
	}
	bottom := sp[3]
	if bottom.root != [2]float64{90, 140} || !bottom.head || bottom.mask != [2]float64{1, 1} {
		t.Fatalf("bottom part %+v", bottom)
	}
	if def := sp[4]; def.side != "top" || def.k != 60 || def.c != 7 || def.gain != 1 {
		t.Fatalf("default part %+v", def)
	}
	right := resolveSprings(&puppetdef.Spec{Springs: []puppetdef.Part{{Rect: rect(0, 0, 10, 20), Root: puppetdef.Root{Side: "right"}}}}, 100)
	if len(right) != 1 || right[0].root != [2]float64{10, 10} {
		t.Fatalf("right part %+v", right)
	}

	// A hair gain of zero turns the hair off; hair without a head is ignored.
	off := *spec
	off.Motion = puppetdef.Motion{Hair: &puppetdef.Spring{}}
	off.Springs = nil
	if got := resolveSprings(&off, 160); len(got) != 0 {
		t.Fatalf("hair with zero gain still sways: %d", len(got))
	}
	headless := *spec
	headless.Head = nil
	headless.Springs = nil
	if got := resolveSprings(&headless, 160); len(got) != 0 {
		t.Fatalf("hair without a head still sways: %d", len(got))
	}
	if unitOf(spec, 160) != 30 || unitOf(&puppetdef.Spec{}, 160) != 32 {
		t.Fatal("unitOf")
	}
}

func TestSpringWeightsGrowAwayFromTheRoot(t *testing.T) {
	for _, tc := range []struct {
		side       string
		near, far  [2]float64
		rootAtEdge string
	}{
		{"top", [2]float64{50, 105}, [2]float64{50, 135}, "top edge"},
		{"bottom", [2]float64{50, 135}, [2]float64{50, 105}, "bottom edge"},
		{"left", [2]float64{22, 120}, [2]float64{78, 120}, "left edge"},
		{"right", [2]float64{78, 120}, [2]float64{22, 120}, "right edge"},
	} {
		spec := &puppetdef.Spec{
			Head:    &puppetdef.Head{Center: []float64{50, 50}, Radius: 30, Neck: []float64{50, 100}},
			Springs: []puppetdef.Part{{Rect: []float64{20, 100, 60, 40}, Root: puppetdef.Root{Side: tc.side}}},
		}
		sp := resolveSprings(spec, 160)[0]
		wn, cn := sp.weight(tc.near[0], tc.near[1])
		wf, cf := sp.weight(tc.far[0], tc.far[1])
		if !(wf > wn) || cn < 0.9 || cf < 0.9 {
			t.Errorf("%s: weights near the %s %v, far %v (cover %v %v)", tc.side, tc.rootAtEdge, wn, wf, cn, cf)
		}
		if w, c := sp.weight(500, 500); w != 0 || c != 0 {
			t.Errorf("%s: a far point follows the part: %v %v", tc.side, w, c)
		}
	}
	// A polygon part has the same soft edge: inside it follows, far outside it does not.
	spec := &puppetdef.Spec{Springs: []puppetdef.Part{{Poly: [][]float64{{0, 0}, {100, 0}, {100, 100}, {0, 100}}, Root: puppetdef.Root{Side: "top"}}}}
	sp := resolveSprings(spec, 100)[0]
	if w, c := sp.weight(50, 90); w < 0.5 || c < 0.99 {
		t.Fatalf("inside the polygon: %v %v", w, c)
	}
	if w, c := sp.weight(300, 50); w != 0 || c != 0 {
		t.Fatalf("outside the polygon: %v %v", w, c)
	}
}

func TestPackWithOverlaysThatDifferPerFace(t *testing.T) {
	y := `
base: base.png
face: calm
faces:
  calm: {}
  happy:
    image: happy.png
    eyes: {off: true}
    mouth: {speed: 4, frames: [face, {image: m.png, at: [50, 64]}]}
eyes:
  closed: eyes.png
mouth:
  frames: [face, {image: m.png, at: [50, 64]}]
`
	r := loadYAML(t, y)
	spec, files := Pack(r)
	if spec.Eyes != nil || spec.Mouth != nil {
		t.Fatal("overlays that differ between faces must be written per face")
	}
	if spec.Faces["happy"].Eyes == nil || !spec.Faces["happy"].Eyes.Off || spec.Faces["calm"].Eyes == nil || spec.Faces["calm"].Eyes.Closed.Image == "" {
		t.Fatalf("faces %+v", spec.Faces)
	}
	if spec.Faces["happy"].Mouth.Speed != 4 || spec.Faces["calm"].Mouth.Speed != 10 {
		t.Fatalf("mouth speeds %v %v", spec.Faces["happy"].Mouth.Speed, spec.Faces["calm"].Mouth.Speed)
	}
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
		t.Fatalf("round trip differs:\n%s\nwant:\n%s", Describe(again), Describe(r))
	}
	if again.Faces[1].Eyes != nil || again.Faces[1].Mouth.Speed != 4 {
		t.Fatal("per-face overlays lost")
	}
}

func TestPackPlainRigWritesOnlyTheBase(t *testing.T) {
	r := loadYAML(t, "base: base.png\n")
	spec, files := Pack(r)
	if spec.Faces != nil || spec.Eyes != nil || spec.Mouth != nil || len(files) != 1 || files["base.png"] == nil {
		t.Fatalf("spec %+v files %d", spec, len(files))
	}
}

func TestSameOverlayAndSpeed(t *testing.T) {
	r := load(t)
	calm, happy := r.Faces[0], r.Faces[1]
	if !sameOverlay(eyesPatches(calm), eyesPatches(calm)) {
		t.Fatal("an overlay equals itself")
	}
	if sameOverlay(eyesPatches(calm), nil) || sameOverlay(mouthPatches(calm), mouthPatches(Face{})) {
		t.Fatal("overlay lists of different length are different")
	}
	if !sameOverlay([]*Patch{nil}, []*Patch{nil}) || sameOverlay([]*Patch{nil}, mouthPatches(calm)[1:]) {
		t.Fatal("nil frames")
	}
	if !sameSpeed(calm, happy) || !sameSpeed(Face{}, Face{}) || sameSpeed(calm, Face{}) {
		t.Fatal("sameSpeed")
	}
	moved := *calm.Eyes
	moved.Closed.At.X++
	if sameOverlay(eyesPatches(calm), []*Patch{&moved.Closed}) {
		t.Fatal("a moved patch is different")
	}
}

func TestPuppetDrawsAndCaches(t *testing.T) {
	r := load(t)
	p := New(r, seeded(1, 1), 60)
	if p.Rig() != r || p.Motion == nil {
		t.Fatal("New")
	}
	dst := ebiten.NewImage(300, 300)
	p.Draw(dst, nil)
	if !p.key.valid || p.canvas == nil || p.base == nil {
		t.Fatal("first Draw must compose the picture")
	}
	if len(p.images) != 0 {
		t.Fatal("the first face has no patch and the eyes are open: no overlay is needed")
	}
	canvas := p.canvas
	p.Draw(dst, nil)
	if p.canvas != canvas {
		t.Fatal("an unchanged pose recomposed the canvas")
	}
	p.Motion.SetFace(1)
	p.Draw(dst, nil)
	if len(p.images) != 1 {
		t.Fatalf("face change loads its patch, have %d images", len(p.images))
	}
	if p.key.face != 1 || p.key.prev != 0 {
		t.Fatalf("key %+v", p.key)
	}
	// Blinking brings in the closed eyes; mouths come in while speaking.
	p.Motion.Speak(true)
	sawEyes, sawMouth := false, false
	for range 600 {
		p.Motion.Step()
		p.Draw(dst, nil)
		pose := p.Motion.Pose()
		sawEyes = sawEyes || pose.Blink > 0
		sawMouth = sawMouth || pose.Mouth >= 0
	}
	if !sawEyes || !sawMouth {
		t.Fatal("test never reached a blink or a mouth overlay")
	}
	if len(p.images) < 3 {
		t.Fatalf("patches for face, eyes and mouth expected, have %d", len(p.images))
	}
}

func TestPuppetDrawTransformAndColor(t *testing.T) {
	r := load(t)
	p := New(r, seeded(1, 1), 60)
	dst := ebiten.NewImage(400, 400)
	p.Draw(dst, nil)
	x0, y0 := p.verts[0].DstX, p.verts[0].DstY
	if p.verts[0].ColorR != 1 || p.verts[0].ColorA != 1 {
		t.Fatalf("default color %+v", p.verts[0])
	}
	op := &ebiten.DrawImageOptions{Blend: ebiten.BlendLighter}
	op.GeoM.Translate(30, 40)
	op.ColorScale.ScaleAlpha(0.5)
	p.Draw(dst, op)
	v := p.verts[0]
	if v.DstX != x0+30 || v.DstY != y0+40 {
		t.Fatalf("translate: %v,%v from %v,%v", v.DstX, v.DstY, x0, y0)
	}
	if math.Abs(float64(v.ColorA)-0.5) > 1e-6 || p.drawOpt.Blend != ebiten.BlendLighter || p.drawOpt.Filter != ebiten.FilterLinear {
		t.Fatalf("color %v blend %v", v.ColorA, p.drawOpt.Blend)
	}
	p.Close()
	p.Close()
	if !p.closed || p.images != nil {
		t.Fatal("Close")
	}
	op2 := &ebiten.DrawImageOptions{}
	op2.GeoM.Translate(500, 500)
	p.Draw(dst, op2)
	if p.verts[0].DstX != v.DstX {
		t.Fatal("Draw after Close drew")
	}
}
