package gamedata_test

import (
	"fmt"
	"math"
	"strings"
	"testing"

	"github.com/nao1215/gozen/internal/testgame"
)

func buildingData(extra string) map[string]string {
	prefix, _, _ := strings.Cut(testgame.Data, "    name: Camp")
	_, suffix, _ := strings.Cut(testgame.Data, "  field:\n")
	spec := "\n    dungeon:\n      tiles: {S: {start: true}}\n      space:\n        levels: [\"......\\n......\\n......\\n......\\n......\\n..S...\"]\n" + extra
	return map[string]string{"game.yaml": "schema_version: 8\nid: buildings-test\n" + testgame.Game, "data/main.yaml": prefix + "    name: Camp" + spec + "  field:\n" + suffix}
}

func TestSpaceRoofGeometry(t *testing.T) {
	for _, shape := range []string{"gable", "shed", "flat"} {
		t.Run(shape, func(t *testing.T) {
			rise := .6
			if shape == "flat" {
				rise = 0
			}
			db := testgame.Load(t, buildingData(fmt.Sprintf("        roofs: [{shape: %s, level: 2, x: 1, y: 1, width: 4, depth: 3, rise: %g, thickness: 0.12, facing: east}]\n", shape, rise)))
			s := db.Locations["camp"].Dungeon.Space
			parts := s.RoofParts()
			want := 1
			if shape == "gable" {
				want = 2
			}
			if len(parts) != want {
				t.Fatalf("parts=%d want=%d", len(parts), want)
			}
			// Vertical rays hit the actual slope, its underside, and empty sky outside.
			for _, x := range []float64{1.1, 2.5, 3.5, 4.9} {
				height := 1.5
				if shape == "gable" {
					height += .6 * (1 - math.Abs(x-3)/2)
				}
				if shape == "shed" {
					height += .6 * (x - 1) / 4
				}
				dist, hit := s.RoofRay([3]float64{x, 4, 2}, [3]float64{0, -1, 0}, 5)
				if !hit || math.Abs(dist-(4-height)) > 1e-8 {
					t.Fatalf("top x=%v hit=%v distance=%v height=%v", x, hit, dist, height)
				}
				dist, hit = s.RoofRay([3]float64{x, 0, 2}, [3]float64{0, 1, 0}, 5)
				if !hit || math.Abs(dist-(height-.12)) > 1e-8 {
					t.Fatalf("underside x=%v hit=%v distance=%v", x, hit, dist)
				}
			}
			if _, hit := s.RoofRay([3]float64{.5, 0, 2}, [3]float64{0, 1, 0}, 5); hit {
				t.Fatal("roof caps street outside footprint")
			}
			if !s.RoofBlocks(1.05, 2, .18, 1.45, 2.35) || s.RoofBlocks(.5, 2, .18, 1.45, 2.35) || s.RoofBlocks(2, 2, .18, 0, .9) {
				t.Fatal("roof body collision disagrees with footprint/headroom")
			}
		})
	}
}

func TestSpaceBuildingValidation(t *testing.T) {
	base := "        materials: {wood: {color: '#a08050'}}\n        roofs: [{shape: gable, level: 2, x: 1, y: 1, width: 4, depth: 3, rise: 0.6, thickness: 0.12, facing: east, material: wood}]\n"
	for _, tc := range []struct{ old, new, diagnostic string }{
		{"gable", "dome", "shape"},
		{"width: 4", "width: 0", "dimensions"},
		{"rise: 0.6", "rise: .nan", "finite"},
		{"thickness: 0.12", "thickness: 0", "thickness"},
		{"x: 1", "x: -1", "bounds"},
		{"material: wood", "material: missing", "material"},
		{"level: 2", "level: 0", "level"},
		{"facing: east", "facing: diagonal", "facing"},
		{"level: 2", "level: 1", "headroom"},
		{"width: 4", "width: 1e-300", "dimensions"},
		{"thickness: 0.12", "thickness: 1e-300", "thickness"},
	} {
		t.Run(tc.new, func(t *testing.T) {
			_, err := testgame.TryLoad(buildingData(strings.Replace(base, tc.old, tc.new, 1)))
			if err == nil || !strings.Contains(err.Error(), tc.diagnostic) {
				t.Fatalf("want %s diagnostic, got %v", tc.diagnostic, err)
			}
		})
	}
}

func TestFilledRoofClosesGableEnds(t *testing.T) {
	db := testgame.Load(t, buildingData("        roofs: [{shape: gable, filled: true, level: 2, x: 1, y: 1, width: 4, depth: 3, rise: 0.6, thickness: 0.12, facing: east}]\n"))
	s := db.Locations["camp"].Dungeon.Space
	distance, hit := s.RoofRay([3]float64{3, 0, 2}, [3]float64{0, 1, 0}, 5)
	if !hit || math.Abs(distance-1.38) > 1e-8 {
		t.Fatalf("filled roof underside: %v %v", distance, hit)
	}
	if _, hit := s.RoofRay([3]float64{3, 1.8, 0}, [3]float64{0, 0, 1}, 5); !hit {
		t.Fatal("open gable end exposed sky through the building")
	}
}

func TestSpaceRoofIntersections(t *testing.T) {
	for _, other := range []string{
		"{shape: flat, level: 2, base: 0.3, x: 2, y: 2, width: 2, depth: 1, thickness: 0.2, facing: north}",
		"{shape: shed, filled: true, level: 2, x: 2, y: 2, width: 2, depth: 1, rise: 0.8, thickness: 0.1, facing: east}",
	} {
		_, err := testgame.TryLoad(buildingData("        roofs: [{shape: gable, filled: true, level: 2, x: 1, y: 1, width: 4, depth: 3, rise: 0.6, thickness: 0.12, facing: east}, " + other + "]\n"))
		if err == nil || !strings.Contains(err.Error(), "overlap") {
			t.Fatalf("intersecting roof volumes accepted: %v", err)
		}
	}
}

func TestSpaceMaterialDiagnostics(t *testing.T) {
	for _, tc := range []struct{ extra, path string }{
		{"materials: {'': {color: '#ffffff'}}", "material name"},
		{"materials: {wood: {width: -1}}", "materials.wood"},
		{"materials: {wood: {width: 1e-300}}", "materials.wood"},
		{"materials: {wood: {height: .inf}}", "materials.wood"},
		{"materials: {wood: {origin_u: .nan}}", "materials.wood"},
		{"materials: {wood: {color: '#ffffff80'}}", "materials.wood"},
		{"materials: {wood: {picture: absent.png}}", "materials.wood.picture"},
		{"wall_regions: [{level: 1, x: 0, y: 0, width: 2, depth: 2, material: absent}]", "wall_regions[0]"},
		{"wall_regions: [{level: 2, x: 0, y: 0, width: 2, depth: 2}]", "wall_regions[0]"},
		{"wall_regions: [{level: 1, x: 0, y: 0, width: 7, depth: 2}]", "wall_regions[0]"},
		{"wall_regions: [{level: 1, x: 0, y: 0, width: 2, depth: 2}, {level: 1, x: 1, y: 1, width: 2, depth: 2}]", "overlap"},
	} {
		t.Run(tc.extra, func(t *testing.T) {
			_, err := testgame.TryLoad(buildingData("        " + tc.extra + "\n"))
			if err == nil || !strings.Contains(err.Error(), tc.path) {
				t.Fatalf("want %s, got %v", tc.path, err)
			}
		})
	}
}
