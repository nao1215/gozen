package puppet

import (
	"math"
	"testing"
)

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

func TestDamageBoostSwayAndHit(t *testing.T) {
	r := loadYAML(t, "base: base.png\nhead: {center: [60, 50], radius: 30, neck: [60, 100]}\nsoft_sway:\n  - {ellipse: [30, 70, 60, 50], follow: body, axis: y}\nmotion: {breath: {depth: 0.01, period: 4}, sway: {angle: 3, shift: 2, period: 5, damage_boost: 4}}\n")
	m := NewMotion(r, seeded(1, 1), 60)
	m.t = 1
	full := m.Pose()
	m.SetHealth(0.25)
	low := m.Pose()
	if math.Abs(low.HeadAngle) <= math.Abs(full.HeadAngle) && math.Abs(low.HeadShift[0]) <= math.Abs(full.HeadShift[0]) {
		t.Fatalf("low health did not increase sway: full=%+v low=%+v", full, low)
	}
	if len(m.springs) != 1 || m.p.damageBoost != 4 {
		t.Fatal("soft sway and damage boost were not loaded")
	}
	var fullBreath, lowBreath float64
	for i := range 600 {
		m.t = float64(i) / 60
		m.SetHealth(1)
		fullBreath = math.Max(fullBreath, m.Pose().Breath)
		m.SetHealth(0.25)
		lowBreath = math.Max(lowBreath, m.Pose().Breath)
	}
	// Damage must not stretch the body: breathing keeps its depth at any health.
	if math.Abs(lowBreath-fullBreath) > 0.001 {
		t.Fatalf("low health changed the breathing depth: full=%f low=%f", fullBreath, lowBreath)
	}
	m.Hit()
	if m.hitSurge <= 0 || len(m.reactions) == 0 || m.reactions[0].kind != "hop" {
		t.Fatal("low-health hit did not start bounce and sway")
	}
	for range 60 {
		m.Step()
	}
	if m.hitSurge <= 0 {
		t.Fatal("sway ended too soon")
	}
	for range 150 {
		m.Step()
	}
	if m.hitSurge != 0 {
		t.Fatalf("sway did not settle: %f", m.hitSurge)
	}
}
