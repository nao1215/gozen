package puppetdef

import (
	"strings"
	"testing"
)

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
