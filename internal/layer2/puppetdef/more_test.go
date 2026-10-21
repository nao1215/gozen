package puppetdef

import (
	"strings"
	"testing"
)

func TestMoreParseErrors(t *testing.T) {
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
