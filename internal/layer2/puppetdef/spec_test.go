package puppetdef

import (
	"strings"
	"testing"
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
