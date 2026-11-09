package puppetdef

import (
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/goccy/go-yaml"
)

const good = `
base: base.png
face: smile
faces:
  smile: {}
  surprised:
    image: surprised.png
    eyes: {off: true}
eyes:
  closed: eyes.png
  every: [1, 3]
  double: 0.5
mouth:
  frames: [face, {image: m1.png, at: [10, 20]}, m2.png]
  speed: 8
head: {center: [50, 40], radius: 30, neck: [50, 90]}
hair:
  - rect: [10, 10, 20, 60]
  - poly: [[70, 10], [90, 10], [90, 70]]
springs:
  - {rect: [20, 70, 10, 30]}
  - {poly: [[60, 70], [80, 70], [70, 100]], root: [70, 70], follow: body, axis: y, stiffness: 120, damping: 9, amount: 0.5, limit: 4}
  - {rect: [0, 0, 5, 5], root: left, axis: x}
motion:
  breath: {depth: 0.01, period: 3}
  sway: {angle: 3, shift: 4}
  tilt: {angle: 5, every: [3, 6]}
  hair: {gain: 1, stiffness: 50}
  talk: {nod: 2}
reactions:
  jump: {kind: hop, amount: 1.5}
  nod: {time: 0.8}
`

func TestParseGood(t *testing.T) {
	s, err := Parse([]byte(good))
	if err != nil {
		t.Fatal(err)
	}
	if got := s.FaceNames(); strings.Join(got, ",") != "smile,surprised" {
		t.Fatalf("faces %v", got)
	}
	if f := s.Mouth.Frames; len(f) != 3 || f[0].Image != FaceKeyword || f[1].At[1] != 20 || f[2].Image != "m2.png" {
		t.Fatalf("frames %+v", f)
	}
	if r, ok := s.ReactionOf("jump"); !ok || r.Kind != "hop" || r.Amount != 1.5 {
		t.Fatalf("jump %+v", r)
	}
	if r, ok := s.ReactionOf("nod"); !ok || r.Kind != "nod" || r.Time != 0.8 {
		t.Fatalf("nod %+v", r)
	}
	if r, ok := s.ReactionOf("shake"); !ok || r.Kind != "shake" {
		t.Fatalf("built-in shake %+v", r)
	}
	if _, ok := s.ReactionOf("dance"); ok {
		t.Fatal("unknown reaction found")
	}
	if p := s.Springs; len(p) != 3 || !p[0].Root.IsZero() || p[1].Root.At[0] != 70 || p[1].Follow != "body" || p[1].Limit != 4 || p[2].Root.Side != "left" {
		t.Fatalf("springs %+v", p)
	}
}

// A written spec reads back the same: the root keeps its form and the default root is left out.
func TestSpringsRoundTrip(t *testing.T) {
	s, err := Parse([]byte(good))
	if err != nil {
		t.Fatal(err)
	}
	b, err := yaml.Marshal(s)
	if err != nil {
		t.Fatal(err)
	}
	again, err := Parse(b)
	if err != nil {
		t.Fatalf("%v\n%s", err, b)
	}
	if !reflect.DeepEqual(again.Springs, s.Springs) {
		t.Fatalf("springs %+v, want %+v", again.Springs, s.Springs)
	}
	if strings.Count(string(b), "root:") != 2 {
		t.Fatalf("want the two given roots only:\n%s", b)
	}
}

func TestDefaultFace(t *testing.T) {
	s, err := Parse([]byte("base: a.png\n"))
	if err != nil {
		t.Fatal(err)
	}
	if s.FirstFace() != DefaultFace || len(s.FaceNames()) != 1 {
		t.Fatalf("%v", s.FaceNames())
	}
	s, err = Parse([]byte("base: a.png\nfaces: {only: {}}\n"))
	if err != nil || s.FirstFace() != "only" {
		t.Fatalf("%v %v", s, err)
	}
}

func TestParseErrors(t *testing.T) {
	for _, tc := range []struct{ name, yaml, want string }{
		{"no base", "faces: {a: {}}\n", "base"},
		{"not png", "base: a.jpg\n", "PNG"},
		{"absolute", "base: /a.png\n", "relative"},
		{"unknown key", "base: a.png\ncolour: red\n", "colour"},
		{"unknown face", "base: a.png\nface: x\nfaces: {a: {}}\n", "not in faces"},
		{"no first face", "base: a.png\nfaces: {a: {}, b: {}}\n", "first face"},
		{"at without image", "base: a.png\nfaces: {a: {at: [1, 2]}}\n", "need an image"},
		{"bad at", "base: a.png\neyes: {closed: {image: e.png, at: [1]}}\n", "at needs"},
		{"area and at", "base: a.png\neyes: {closed: {image: e.png, at: [1, 2], area: [0, 0, 1, 1]}}\n", "area"},
		{"bad area", "base: a.png\neyes: {closed: {image: e.png, area: [0, 0, 0, 1]}}\n", "area needs"},
		{"no closed", "base: a.png\neyes: {every: [1, 2]}\n", "closed"},
		{"bad every", "base: a.png\neyes: {closed: e.png, every: [3, 1]}\n", "every"},
		{"bad double", "base: a.png\neyes: {closed: e.png, double: 2}\n", "double"},
		{"off with closed", "base: a.png\neyes: {off: true, closed: e.png}\n", "off eyes"},
		{"one frame", "base: a.png\nmouth: {frames: [m.png]}\n", "at least two"},
		{"negative speed", "base: a.png\nmouth: {frames: [face, m.png], speed: -1}\n", "speed"},
		{"head without radius", "base: a.png\nhead: {center: [1, 1], neck: [1, 9]}\n", "radius"},
		{"neck above", "base: a.png\nhead: {center: [1, 9], radius: 3, neck: [1, 1]}\n", "below"},
		{"hair without head", "base: a.png\nhair: [{rect: [0, 0, 1, 1]}]\n", "needs head"},
		{"rect and poly", "base: a.png\nhead: {center: [1, 1], radius: 3, neck: [1, 9]}\nhair: [{rect: [0, 0, 1, 1], poly: [[0, 0], [1, 0], [1, 1]]}]\n", "not both"},
		{"short poly", "base: a.png\nhead: {center: [1, 1], radius: 3, neck: [1, 9]}\nhair: [{poly: [[0, 0], [1, 0]]}]\n", "three"},
		{"deep breath", "base: a.png\nmotion: {breath: {depth: 0.5}}\n", "fraction"},
		{"wide sway", "base: a.png\nmotion: {sway: {angle: 90}}\n", "degrees"},
		{"negative gain", "base: a.png\nmotion: {hair: {gain: -1}}\n", "hair"},
		{"spring without shape", "base: a.png\nsprings: [{follow: body}]\n", "springs[0]: give rect or poly"},
		{"spring follows missing head", "base: a.png\nsprings: [{rect: [0, 0, 1, 1]}]\n", "needs head"},
		{"spring bad follow", "base: a.png\nsprings: [{rect: [0, 0, 1, 1], follow: wind}]\n", "follow"},
		{"spring bad root", "base: a.png\nsprings: [{rect: [0, 0, 1, 1], follow: body, root: middle}]\n", "root"},
		{"spring short root", "base: a.png\nsprings: [{rect: [0, 0, 1, 1], follow: body, root: [1]}]\n", "root point"},
		{"spring map root", "base: a.png\nsprings: [{rect: [0, 0, 1, 1], follow: body, root: {x: 1}}]\n", "root"},
		{"spring bad axis", "base: a.png\nsprings: [{rect: [0, 0, 1, 1], follow: body, axis: z}]\n", "axis"},
		{"spring negative", "base: a.png\nsprings: [{rect: [0, 0, 1, 1], follow: body, limit: -1}]\n", "not negative"},
		{"spring stiff", "base: a.png\nsprings: [{rect: [0, 0, 1, 1], follow: body, stiffness: 20000}]\n", "at most"},
		{"spring unknown key", "base: a.png\nsprings: [{rect: [0, 0, 1, 1], follow: body, mass: 2}]\n", "mass"},
		{"hair stiff", "base: a.png\nmotion: {hair: {damping: 5000}}\n", "at most"},
		{"too many springs", "base: a.png\nhead: {center: [1, 1], radius: 3, neck: [1, 9]}\nhair: [{rect: [0, 0, 1, 1]}]\nsprings: [" + strings.Repeat("{rect: [0, 0, 1, 1]}, ", MaxSprings-1) + "{rect: [0, 0, 1, 1]}]\n", fmt.Sprintf("at most %d", MaxSprings)},
		{"unknown kind", "base: a.png\nreactions: {dance: {}}\n", "kind"},
		{"face mouth", "base: a.png\nfaces: {a: {mouth: {frames: [face]}}}\n", "faces.a.mouth"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := Parse([]byte(tc.yaml))
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("got %v, want %q", err, tc.want)
			}
		})
	}
}

func FuzzParse(f *testing.F) {
	f.Add([]byte(good))
	f.Add([]byte("base: a.png\n"))
	f.Fuzz(func(t *testing.T, b []byte) {
		s, err := Parse(b)
		if err != nil {
			return
		}
		// A valid spec always names a first face that FaceNames lists first.
		if names := s.FaceNames(); len(names) == 0 || names[0] != s.FirstFace() {
			t.Fatalf("faces %v first %q", names, s.FirstFace())
		}
	})
}

func TestSoftSwayShapes(t *testing.T) {
	for _, shape := range []string{"rect: [10, 20, 30, 40]", "ellipse: [10, 20, 30, 40]"} {
		y := "base: base.png\nsoft_sway:\n  - {" + shape + ", follow: body}\n"
		s, err := Parse([]byte(y))
		if err != nil || len(s.SoftSway) != 1 {
			t.Fatalf("%s: spec=%+v, error=%v", shape, s, err)
		}
	}
	for _, shape := range []string{"ellipse: [10, 20, 0, 40]", "ellipse: [10, 20, 30]", "ellipse: [10, 20, 30, 40], rect: [1, 2, 3, 4]"} {
		y := "base: base.png\nsoft_sway:\n  - {" + shape + ", follow: body}\n"
		if _, err := Parse([]byte(y)); err == nil || !strings.Contains(err.Error(), "soft_sway[0]") {
			t.Fatalf("%s: wanted soft_sway error, got %v", shape, err)
		}
	}
}

func TestSwayDamageBoost(t *testing.T) {
	s, err := Parse([]byte("base: base.png\nmotion: {sway: {angle: 2, damage_boost: 4}}\n"))
	if err != nil || s.Motion.Sway == nil || s.Motion.Sway.DamageBoost != 4 {
		t.Fatalf("damage boost: spec=%+v error=%v", s, err)
	}
	if _, err := Parse([]byte("base: base.png\nmotion: {sway: {damage_boost: -1}}\n")); err == nil {
		t.Fatal("negative damage boost accepted")
	}
}

func TestSoftOnlyHitValidation(t *testing.T) {
	valid := "base: base.png\nsoft_sway:\n  - {ellipse: [10, 20, 30, 40], follow: body}\nmotion: {hit: soft_only}\n"
	if s, err := Parse([]byte(valid)); err != nil || s.Motion.Hit != "soft_only" {
		t.Fatalf("soft_only: spec=%+v error=%v", s, err)
	}
	for _, y := range []string{
		"base: base.png\nmotion: {hit: soft_only}\n",
		"base: base.png\nmotion: {hit: unknown}\n",
	} {
		if _, err := Parse([]byte(y)); err == nil {
			t.Fatalf("accepted invalid hit motion: %q", y)
		}
	}
}

func TestPartIdleValidation(t *testing.T) {
	for _, block := range []string{"{}", "{amplitude: 4, period: 2, phase: -90}", "{amplitude: 0, period: 0}"} {
		_, err := Parse([]byte("base: base.png\nsoft_sway: [{ellipse: [10, 20, 30, 40], follow: body, idle: " + block + "}]\n"))
		if err != nil {
			t.Fatalf("%s: %v", block, err)
		}
	}
	for _, block := range []string{"{amplitude: -1}", "{amplitude: .nan}", "{period: .inf}", "{period: -1}", "{period: 0.001}", "{phase: .nan}"} {
		_, err := Parse([]byte("base: base.png\nsoft_sway: [{ellipse: [10, 20, 30, 40], follow: body, idle: " + block + "}]\n"))
		if err == nil {
			t.Fatalf("accepted %s", block)
		}
	}
}

func TestParseAdditionalInvalidFields(t *testing.T) {
	head := "base: a.png\nhead: {center: [1, 1], radius: 3, neck: [1, 9]}\n"
	for _, tc := range []struct{ name, yaml, want string }{
		{"face given but no faces", "base: a.png\nface: x\n", "not in faces"},
		{"negative fade", "base: a.png\nfade: -1\n", "fade"},
		{"empty face name", "base: a.png\nfaces: {'': {}}\n", "empty name"},
		{"face picture without position", "base: a.png\nfaces: {a: {image: f.png, at: [1]}}\n", "faces.a"},
		{"face eyes that are broken", "base: a.png\nfaces: {a: {eyes: {every: [1, 2]}}}\n", "faces.a.eyes"},
		{"head center of one number", "base: a.png\nhead: {center: [1], radius: 3, neck: [1, 9]}\n", "center and neck"},
		{"head neck of one number", "base: a.png\nhead: {center: [1, 1], radius: 3, neck: [9]}\n", "center and neck"},
		{"negative reaction amount", "base: a.png\nreactions: {hop: {amount: -1}}\n", "amount"},
		{"negative reaction time", "base: a.png\nreactions: {hop: {time: -1}}\n", "time"},
		{"negative breath", "base: a.png\nmotion: {breath: {period: -3}}\n", "breath"},
		{"negative sway", "base: a.png\nmotion: {sway: {shift: -3}}\n", "sway"},
		{"negative tilt", "base: a.png\nmotion: {tilt: {angle: -3}}\n", "tilt"},
		{"too wide tilt", "base: a.png\nmotion: {tilt: {angle: 60}}\n", "at most 45"},
		{"bad tilt range", "base: a.png\nmotion: {tilt: {angle: 3, every: [5, 1]}}\n", "every"},
		{"negative nod", "base: a.png\nmotion: {talk: {nod: -1}}\n", "talk"},
		{"off mouth with frames", "base: a.png\nmouth: {off: true, frames: [face, m.png]}\n", "off mouth"},
		{"mouth frame that is broken", "base: a.png\nmouth: {frames: [face, {image: m.png, at: [1]}]}\n", "frames[1]"},
		{"rect of three", head + "hair: [{rect: [1, 2, 3]}]\n", "rect needs"},
		{"rect of no size", head + "hair: [{rect: [1, 2, 0, 3]}]\n", "positive size"},
		{"poly point of three", head + "hair: [{poly: [[0, 0], [1, 0], [1, 1, 1]]}]\n", "poly points"},
		{"poly point not a number", head + "hair: [{poly: [[0, 0], [1, 0], [1, .nan]]}]\n", "poly points"},
		{"coordinate that is infinite", "base: a.png\nfaces: {a: {image: f.png, at: [.inf, 1]}}\n", "at"},
		{"yaml that is not a mapping", "- a\n- b\n", ""},
		{"yaml that is broken", "base: [\n", ""},
		{"empty", "", "base"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := Parse([]byte(tc.yaml))
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("got %v, want an error with %q", err, tc.want)
			}
		})
	}
}

func TestPicturesAreListedOnceInOrder(t *testing.T) {
	s, err := Parse([]byte(good))
	if err != nil {
		t.Fatal(err)
	}
	got := s.Pictures()
	want := []string{"base.png", "eyes.png", "m1.png", "m2.png", "surprised.png"}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Errorf("pictures %v, want %v", got, want)
	}
	seen := map[string]bool{}
	for _, p := range got {
		if seen[p] || p == FaceKeyword || p == "" {
			t.Errorf("bad entry %q in %v", p, got)
		}
		seen[p] = true
	}
}

func TestFaceNamesPutTheFirstFaceFirst(t *testing.T) {
	s, err := Parse([]byte("base: a.png\nface: zeta\nfaces: {alpha: {}, zeta: {}, mid: {}}\n"))
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(s.FaceNames(), ","); got != "zeta,alpha,mid" {
		t.Errorf("%s", got)
	}
}

func TestPicAcceptsBothForms(t *testing.T) {
	s, err := Parse([]byte("base: a.png\neyes: {closed: e.png}\nmouth: {frames: [face, {image: m.png, at: [1, 2]}]}\n"))
	if err != nil {
		t.Fatal(err)
	}
	if s.Eyes.Closed.Image != "e.png" || s.Mouth.Frames[1].At[0] != 1 {
		t.Errorf("%+v %+v", s.Eyes, s.Mouth)
	}
	if _, err := Parse([]byte("base: a.png\neyes: {closed: {image: e.png, tint: red}}\n")); err == nil {
		t.Error("an unknown key of a picture")
	}
}
