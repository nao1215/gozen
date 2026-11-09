package puppet

import (
	"math"
	"strings"
	"testing"
	"testing/fstest"
)

// loadYAML loads the fixture pictures with another YAML.
func loadYAML(t *testing.T, y string) *Rig {
	t.Helper()
	fsys := fixture()
	fsys["p/rig.yaml"] = &fstest.MapFile{Data: []byte(y)}
	r, err := Load(fsys, "p/rig.yaml")
	if err != nil {
		t.Fatal(err)
	}
	return r
}

// script plays the same calls on a motion: a hop, talking with a shake, a tilt.
func script(r *Rig, m *Motion, i int) {
	name := map[int]string{60: "hop", 150: "shake", 300: "tilt"}[i]
	if name == "" {
		return
	}
	if i == 150 {
		m.Speak(true)
	}
	react, _ := r.Spec.ReactionOf(name)
	m.React(react)
}

// The hair keeps its look: these sums of the deformed mesh were taken from the
// hair-only implementation before swaying parts were added (bit-identical on amd64;
// the tolerance allows fused multiply-add on other machines).
func TestHairUnchanged(t *testing.T) {
	want := [][5]float64{
		{59, 20153.7514, 26887.5923, 118.6353, 24.9161},
		{119, 20060.6482, 26785.1107, 212.7392, 44.1849},
		{179, 20053.1400, 26766.4354, 188.2902, 30.3991},
		{239, 20104.9541, 26867.8252, 89.1811, 13.9005},
		{299, 20109.6469, 26876.2178, 91.5063, 13.2322},
		{359, 20862.9909, 26866.9703, 876.1730, 242.1077},
		{419, 20262.4785, 26802.1757, 175.0186, 34.1003},
		{479, 20263.6360, 26880.0902, 104.7376, 17.4429},
	}
	r := load(t)
	mesh := NewMesh(r)
	m := NewMotion(r, seeded(4, 2), 60)
	dst := make([][2]float32, len(mesh.Src))
	n := 0
	for i := range 480 {
		script(r, m, i)
		m.Step()
		if i%60 != 59 {
			continue
		}
		mesh.Deform(m.Pose(), dst)
		var sx, sy, sd, sh float64
		for k, d := range dst {
			s := mesh.Src[k]
			moved := math.Hypot(float64(d[0]-s[0]), float64(d[1]-s[1]))
			sx, sy, sd = sx+float64(d[0]), sy+float64(d[1]), sd+moved
			if s[0] >= 14 && s[0] <= 46 && s[1] >= 24 && s[1] <= 96 { // around the hair
				sh += moved
			}
		}
		got := [5]float64{float64(i), sx, sy, sd, sh}
		for j := range got {
			if math.Abs(got[j]-want[n][j]) > 0.01 {
				t.Fatalf("step %d: %v, want %v", i, got, want[n])
			}
		}
		n++
	}
}

// The hair written as a swaying part with the defaults moves the same.
func TestHairIsASpring(t *testing.T) {
	hair := load(t)
	part := loadYAML(t, strings.Replace(rigYAML, "hair:\n  - rect: [20, 30, 20, 60]", "springs:\n  - {rect: [20, 30, 20, 60], root: top, follow: head}", 1))
	if len(part.Spec.Hair) != 0 || len(part.Spec.Springs) != 1 {
		t.Fatalf("replace failed: %+v", part.Spec)
	}
	ma, mb := NewMesh(hair), NewMesh(part)
	a, b := NewMotion(hair, seeded(7, 7), 60), NewMotion(part, seeded(7, 7), 60)
	da, db := make([][2]float32, len(ma.Src)), make([][2]float32, len(mb.Src))
	for i := range 400 {
		script(hair, a, i)
		script(part, b, i)
		a.Step()
		b.Step()
		ma.Deform(a.Pose(), da)
		mb.Deform(b.Pose(), db)
		for k := range da {
			if da[k] != db[k] {
				t.Fatalf("step %d vertex %v: hair %v, part %v", i, ma.Src[k], da[k], db[k])
			}
		}
	}
}

// A cord hanging below the body: it follows the body (breath and hop) and sways
// only up and down.
const cordYAML = rigYAML + `springs:
  - {rect: [70, 110, 10, 40], follow: body, axis: y, limit: 6}
  - {rect: [74, 130, 30, 20], root: left, follow: body, axis: x}
`

func TestSpringsAreDeterministic(t *testing.T) {
	r := loadYAML(t, cordYAML)
	a, b, c := NewMotion(r, seeded(3, 4), 60), NewMotion(r, seeded(3, 4), 60), NewMotion(r, seeded(5, 6), 60)
	differs := false
	for i := range 600 {
		for _, m := range []*Motion{a, b, c} {
			script(r, m, i)
			m.Step()
		}
		if a.Pose() != b.Pose() {
			t.Fatalf("step %d: same seed, different pose", i)
		}
		if a.Pose().Lag != c.Pose().Lag {
			differs = true
		}
	}
	if !differs {
		t.Fatal("different seeds gave the same lags")
	}
}

func TestBodySpringLagsAndSettles(t *testing.T) {
	r := loadYAML(t, cordYAML)
	m := NewMotion(r, seeded(1, 1), 60)
	hop, _ := r.Spec.ReactionOf("hop")
	m.React(hop)
	most := [2]float64{}
	for range 40 {
		m.Step()
		p := m.Pose()
		for i := range 2 {
			most[i] = math.Max(most[i], math.Abs(p.Lag[1+i][1-i])) // hair is Lag[0]
		}
		if p.Lag[1][0] != 0 || p.Lag[2][1] != 0 {
			t.Fatalf("a part swayed across its axis: %v", p.Lag)
		}
		if math.Abs(p.Lag[1][1]) > 6 {
			t.Fatalf("lag %v beyond the limit 6", p.Lag[1])
		}
	}
	if most[0] < 1 {
		t.Fatalf("the cord hardly trailed the hop: %v", most)
	}
	for range 600 {
		m.Step()
	}
	// Back at rest, the lag is only what the slow breath leaves.
	if l := m.Pose().Lag[1][1]; math.Abs(l) > 0.2 {
		t.Fatalf("the cord did not settle: %v", l)
	}
}

// Parts move only their own area (with the soft edge) and the later part decides
// where two parts overlap.
func TestSpringsStayInTheirArea(t *testing.T) {
	r := loadYAML(t, cordYAML)
	mesh := NewMesh(r)
	base := NewMesh(load(t))
	pose := Pose{BodyScale: [2]float64{1, 1}}
	pose.Lag[1] = [2]float64{0, 5}
	pose.Lag[2] = [2]float64{-4, 0}
	got, want := make([][2]float32, len(mesh.Src)), make([][2]float32, len(mesh.Src))
	mesh.Deform(pose, got)
	base.Deform(Pose{BodyScale: [2]float64{1, 1}}, want)
	feather := 0.08 * r.Spec.Head.Radius
	for k, s := range mesh.Src {
		x, y := float64(s[0]), float64(s[1])
		inCord := x > 70-feather && x < 80+feather && y > 110-feather
		inTail := x > 74-feather && y > 130-feather
		if !inCord && !inTail && got[k] != want[k] {
			t.Fatalf("vertex %v outside the parts moved to %v", s, got[k])
		}
		dx, dy := float64(got[k][0]-want[k][0]), float64(got[k][1]-want[k][1])
		if dx < -4-1e-6 || dx > 0 || dy < 0 || dy > 5+1e-6 {
			t.Fatalf("vertex %v moved by %v, %v: beyond its part's lag", s, dx, dy)
		}
		// Deep inside the overlap only the later part (x) moves the vertex.
		if x >= 74+feather && x <= 80-feather && y >= 130+feather && y < 150 && dy != 0 {
			t.Fatalf("vertex %v in the overlap moved down by %v under the later part", s, dy)
		}
	}
}

// A stiff spring at a low update rate stays finite and within its limit.
func TestStiffSpringStaysStable(t *testing.T) {
	y := rigYAML + "springs:\n  - {rect: [70, 110, 10, 40], follow: body, stiffness: 10000, damping: 1000}\n"
	r := loadYAML(t, y)
	m := NewMotion(r, seeded(2, 2), 5)
	limit := 0.15 * r.Spec.Head.Radius
	for i := range 300 {
		if i%3 == 0 {
			hop, _ := r.Spec.ReactionOf("hop")
			m.React(hop)
		}
		m.Step()
		for _, st := range m.state {
			for _, v := range append(st.pos[:], st.vel[:]...) {
				if math.IsNaN(v) || math.IsInf(v, 0) || math.Abs(v) > 1e6 {
					t.Fatalf("step %d: spring state %+v", i, st)
				}
			}
		}
		if l := m.Pose().Lag[1]; math.Abs(l[0]) > limit || math.Abs(l[1]) > limit {
			t.Fatalf("lag %v beyond %v", l, limit)
		}
	}
}

// A point root: the part moves least at the root and more toward the far end.
func TestPointRootWeights(t *testing.T) {
	r := loadYAML(t, rigYAML+"springs:\n  - {rect: [60, 100, 40, 40], root: [60, 100], follow: body}\n")
	sp := resolveSprings(r.Spec, fh)[1]
	near, _ := sp.weight(61, 101)
	far, _ := sp.weight(90, 130)
	if !(near < 0.2 && far > 0.8) {
		t.Fatalf("weights near the root %v, at the far corner %v", near, far)
	}
}

func TestEllipseSoftSwayWeight(t *testing.T) {
	r := loadYAML(t, "base: base.png\nsoft_sway:\n  - {ellipse: [20, 30, 40, 60], follow: body, root: top}\n")
	sp := resolveSprings(r.Spec, 200)
	if len(sp) != 1 {
		t.Fatalf("got %d springs", len(sp))
	}
	center, _ := sp[0].weight(40, 60)
	corner, _ := sp[0].weight(20, 30)
	outside, _ := sp[0].weight(0, 0)
	if !(center > corner && corner >= outside && math.Abs(outside) < 1e-9) {
		t.Fatalf("weights center=%f corner=%f outside=%f", center, corner, outside)
	}
}
