package puppet

import (
	"math"
	"testing"
)

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

func TestSoftOnlyHitMovesRegionWithoutBodyReaction(t *testing.T) {
	r := loadYAML(t, "base: base.png\nhead: {center: [60, 50], radius: 30, neck: [60, 100]}\nsoft_sway:\n  - {ellipse: [30, 70, 60, 50], follow: body, axis: y, limit: 20}\nmotion: {hit: soft_only, sway: {angle: 3, shift: 2, damage_boost: 4}}\n")
	m := NewMotion(r, seeded(1, 1), 60)
	m.SetHealth(0.25)
	m.t = 0.25
	before := m.Pose()
	m.Hit()
	after := m.Pose()
	if len(m.reactions) != 0 || after.Body != before.Body || after.BodyScale != before.BodyScale || after.HeadAngle != before.HeadAngle || after.HeadShift != before.HeadShift {
		t.Fatalf("hit moved body or head: before=%+v after=%+v reactions=%+v", before, after, m.reactions)
	}
	if after.Lag[0] == before.Lag[0] {
		t.Fatalf("hit did not bounce soft region: before=%+v after=%+v", before.Lag[0], after.Lag[0])
	}
	for range 60 {
		m.Step()
	}
	if m.hitSurge <= 0 || m.Pose().Lag[0] == [2]float64{} {
		t.Fatal("soft sway did not continue after hit")
	}
}

func TestSoftIdleIndependentOfBodyAndDamage(t *testing.T) {
	for _, idle := range []string{"", ", idle: {amplitude: 4, period: 2, phase: 0}"} {
		r := loadYAML(t, "base: base.png\nhead: {center: [60, 50], radius: 30, neck: [60, 100]}\nsoft_sway: [{ellipse: [30, 70, 60, 50], follow: body, axis: y, limit: 5"+idle+"}]\nmotion: {hit: soft_only, breath: {depth: 0}, sway: {angle: 0, shift: 0, damage_boost: 4}}\n")
		m := NewMotion(r, seeded(1, 1), 60)
		first := m.Pose()
		for range 30 {
			m.Step()
		}
		p := m.Pose()
		if idle == "" && p.Lag[0] != [2]float64{} {
			t.Fatal("omitted idle moves stationary region")
		}
		if idle != "" && math.Abs(p.Lag[0][1]-4) > 1e-8 {
			t.Fatalf("quarter-period lag = %v", p.Lag[0])
		}
		if p.Body != first.Body || p.BodyScale != first.BodyScale || p.Breath != first.Breath || p.HeadAngle != first.HeadAngle || p.HeadShift != first.HeadShift {
			t.Fatal("idle moves body/head")
		}
		m.SetHealth(.25)
		m.Hit()
		for range 240 {
			m.Step()
			p = m.Pose()
			if math.Abs(p.Lag[0][1]) > 5 || p.Lag[0][0] != 0 {
				t.Fatalf("axis/limit violated: %v", p.Lag[0])
			}
			if p.Body != first.Body || p.BodyScale != first.BodyScale || p.HeadShift != first.HeadShift || p.HeadAngle != first.HeadAngle {
				t.Fatal("composed hit moves body")
			}
		}
	}
}

func TestPartIdleDefaultsPhaseAndDisable(t *testing.T) {
	for _, tc := range []struct {
		idle string
		want float64
	}{
		{"amplitude: 4", 4},
		{"amplitude: 4, phase: 180", -4},
		{"amplitude: 4, phase: -180", -4},
		{"amplitude: 0, phase: 90", 0},
	} {
		r := loadYAML(t, "base: base.png\nsprings: [{rect: [30, 70, 60, 50], follow: body, axis: x, limit: 5, idle: {"+tc.idle+"}}]\nmotion: {breath: {depth: 0}, sway: {angle: 0, shift: 0}}\n")
		m := NewMotion(r, seeded(1, 1), 60)
		for range 30 {
			m.Step()
		}
		p := m.Pose()
		if math.Abs(p.Lag[0][0]-tc.want) > 1e-8 || p.Lag[0][1] != 0 {
			t.Fatalf("%s: lag %v", tc.idle, p.Lag[0])
		}
	}
}

// TestTalkBounceJigglesOnlySoftRegions: with talk.bounce, a line's start sets the soft regions
// bouncing and the bounce dies away; the body and head stay still, and a still puppet stays still.
func TestTalkBounceJigglesOnlySoftRegions(t *testing.T) {
	r := loadYAML(t, "base: base.png\nhead: {center: [60, 50], radius: 30, neck: [60, 100]}\nsoft_sway:\n  - {ellipse: [30, 70, 60, 50], follow: body, axis: y, limit: 20}\nmotion: {breath: {depth: 0}, sway: {angle: 0, shift: 0}, tilt: {angle: 0}, talk: {nod: 0, bounce: 1}}\n")
	m := NewMotion(r, seeded(1, 1), 60)
	first := m.Pose()
	for range 120 {
		m.Step()
	}
	if m.Pose().Lag[0] != [2]float64{} {
		t.Fatalf("a silent puppet moved its soft region: %v", m.Pose().Lag[0])
	}
	m.Speak(true)
	moved := 0.0
	for range 30 {
		m.Step()
		p := m.Pose()
		moved = math.Max(moved, math.Abs(p.Lag[0][1]))
		if p.Lag[0][0] != 0 || math.Abs(p.Lag[0][1]) > 20 {
			t.Fatalf("axis or limit broken: %v", p.Lag[0])
		}
		if p.Body != first.Body || p.BodyScale != first.BodyScale || p.HeadAngle != first.HeadAngle || p.HeadShift != first.HeadShift {
			t.Fatal("the bounce moved the body or head")
		}
	}
	if moved < 5 {
		t.Fatalf("the line's start barely moved the soft region: %v", moved)
	}
	for range 120 { // still speaking: one bounce per line, not a constant one
		m.Step()
	}
	if m.Pose().Lag[0] != [2]float64{} {
		t.Fatalf("the bounce did not die away: %v", m.Pose().Lag[0])
	}
}
