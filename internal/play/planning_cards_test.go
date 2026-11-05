package play

import (
	"image"
	"testing"

	data "github.com/nao1215/gozen/internal/layer2/gamedata"
)

func TestResidentGaugeLimitsAndWarnings(t *testing.T) {
	field := data.PlanningField{Key: "used", MaximumKey: "limit", WarningKey: "warning", CriticalRatio: .9}
	for _, tc := range []struct {
		used, limit float64
		level       int
	}{{0, 80, 0}, {45, 80, 1}, {72, 80, 2}, {41, 45, 2}, {0, 0, 0}, {100, 80, 2}} {
		value, limit, level := residentGauge(map[string]any{"used": tc.used, "limit": tc.limit, "warning": 45.0}, field)
		if value != tc.used || limit != tc.limit || level != tc.level {
			t.Fatalf("%+v: %v %v %v", tc, value, limit, level)
		}
	}
	for _, count := range []int{8, 9, 13, 16} {
		for _, r := range islandCells(12, 202, 1280*.72-24, 400, count) {
			x, y, radius := residentFaceRect(r, 2)
			if x != float64(r.Min.X+r.Max.X)/2 || !image.Pt(int(x), int(y)).In(r) || y+radius > float64(r.Max.Y)-78 || y-radius < float64(r.Min.Y) {
				t.Fatalf("face overlaps text or leaves card: %v %.1f %.1f %.1f", r, x, y, radius)
			}
		}
	}
}
