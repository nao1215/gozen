package play

import (
	"bytes"
	"encoding/json"
	"math"
	"testing"

	"github.com/nao1215/gozen/internal/rules/event"
)

func TestBirthTimelineSkipAndPositions(t *testing.T) {
	g := ranchGame(t)
	before, err := json.Marshal(g.st)
	if err != nil {
		t.Fatal(err)
	}
	random, err := g.pcg.MarshalBinary()
	if err != nil {
		t.Fatal(err)
	}
	dst := covScreen()
	for tier, duration := range []int{348, 384, 420} {
		g.Handle(event.BreedEffect{ParentNames: [2]string{"Mote ♂", "Mote ♀"}, ChildName: "Sentinel", Tier: tier, ParentFaces: [2]string{"x.png", "x.png"}, ChildFace: "x.png", Summary: "A new companion.", Result: "Lv 1\nSkills: Spark", FirstDiscovery: true})
		b := g.birth
		if b.duration() != duration {
			t.Fatalf("tier %d duration %d", tier, b.duration())
		}
		if b.silhouetteEnd() != 204+24*tier || b.viewEnd()-b.revealEnd() != 36+12*min(1, tier) {
			t.Fatalf("tier %d pause and afterglow: silhouette=%d afterglow=%d", tier, b.silhouetteEnd(), b.viewEnd()-b.revealEnd())
		}
		for _, tc := range []struct {
			age   int
			phase birthPhase
		}{
			{0, birthParents},
			{44, birthParents},
			{45, birthYornFade},
			{80, birthYornFade},
			{81, birthFade},
			{134, birthFade},
			{135, birthMerge},
			{179, birthMerge},
			{180, birthSilhouette},
			{b.silhouetteEnd() - 1, birthSilhouette},
			{b.silhouetteEnd(), birthReveal},
			{b.revealEnd() - 1, birthReveal},
			{b.revealEnd(), birthView},
			{b.viewEnd() - 1, birthView},
			{b.viewEnd(), birthResult},
			{b.duration(), birthResult},
		} {
			b.age = tc.age
			g.drawBirth(dst)
			if b.age != tc.age {
				t.Fatal("drawing advanced the birth animation")
			}
			if tc.phase == birthResult {
				b.details = true
				g.drawBirth(dst)
				b.details = false
			}
			if got := b.phase(); got != tc.phase {
				t.Fatalf("tier %d age %d: %d != %d", tier, tc.age, got, tc.phase)
			}
		}
	}
	after, err := json.Marshal(g.st)
	if err != nil || !bytes.Equal(before, after) {
		t.Fatal("drawing changed saved state", err)
	}
	randomAfter, err := g.pcg.MarshalBinary()
	if err != nil || !bytes.Equal(random, randomAfter) {
		t.Fatal("drawing consumed simulation randomness", err)
	}
	for _, size := range [][2]float64{{1280, 720}, {1920, 1080}} {
		sw, sh := size[0], size[1]
		parents, crystal := birthPositions(sw, sh)
		if parents[0] != (birthPoint{sw * 0.22, sh * 0.78}) || parents[1] != (birthPoint{sw * 0.78, sh * 0.78}) || crystal != (birthPoint{sw * 0.5, sh * 0.52}) {
			t.Fatalf("positions at %v: %v %v", size, parents, crystal)
		}
		x, feet, h, w := birthChildFrame(sw, sh)
		if x != sw*0.5 || feet != sh*0.82 || h != sh*0.62 || w != sw*0.58 {
			t.Fatalf("child frame at %v: %v %v %v %v", size, x, feet, h, w)
		}
		for i, p := range parents {
			start := birthOrbPosition(sw, sh, i, 135, 180)
			end := birthOrbPosition(sw, sh, i, 180, 180)
			if math.Abs(start.x-p.x) > 0.01 || math.Abs(start.y-(p.y-sh*0.23)) > 0.01 || math.Abs(end.x-crystal.x) > 0.01 || math.Abs(end.y-crystal.y) > 0.01 {
				t.Fatalf("orb %d at %v: %v %v", i, size, start, end)
			}
		}
	}
	g.Handle(event.BreedEffect{})
	g.req = event.Wait{Seconds: float64(birthDuration) / 60}
	g.waitT = birthDuration
	g.in.held[btnOK] = true
	g.in.pressed[btnOK] = true
	g.pump()
	if g.birth.skipT != 0 || g.req == nil {
		t.Fatal("confirming breed must not skip or dismiss")
	}
	g.in.held[btnOK] = false
	g.in.pressed[btnOK] = false
	g.pump()
	g.in.held[btnOK] = true
	g.in.pressed[btnOK] = true
	g.pump()
	if g.birth.skipT != 1 || g.req == nil {
		t.Fatal("fresh confirm did not start skip")
	}
	g.in.pressed[btnOK] = false
	for range 12 {
		g.birth.skipT++
		if g.birth.skipT >= 12 {
			g.birth.skipT = 0
			g.birth.age = g.birth.duration()
		}
		g.pump()
	}
	if g.req == nil || g.waitT != 1 {
		t.Fatalf("result was not held: wait=%d req=%T", g.waitT, g.req)
	}
	g.in.held[btnOK] = false
	g.pump()
	g.in.held[btnOK] = true
	g.in.pressed[btnOK] = true
	g.pump()
	if g.req != nil {
		t.Fatal("fresh result confirm did not continue")
	}
}

// A click (a tap on a touch screen) passes the result of a birth as OK does: nothing else
// on the screen can be pressed while it waits.
func TestBirthResultPassesOnClick(t *testing.T) {
	g := ranchGame(t)
	g.Handle(event.BreedEffect{ParentNames: [2]string{"Mote ♂", "Mote ♀"}, ChildName: "Sentinel", ParentFaces: [2]string{"x.png", "x.png"}, ChildFace: "x.png", Summary: "A new companion.", Result: "Lv 1"})
	g.req = event.Wait{Seconds: 1}
	g.birth.age = g.birth.duration()
	g.in = input{}
	g.pump() // nothing held: the result is ready for a press
	g.in = input{click: true, moved: true}
	g.pump()
	g.in = input{}
	if g.req != nil {
		t.Fatalf("a click left the birth's result waiting (%T): only the OK key passes it", g.req)
	}
}
