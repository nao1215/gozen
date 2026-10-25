package play

import (
	"bytes"
	"os"
	"slices"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/goccy/go-yaml"
	"github.com/hajimehoshi/ebiten/v2"

	data "github.com/nao1215/gozen/internal/layer2/gamedata"
	"github.com/nao1215/gozen/internal/layer3/dungeon"
	"github.com/nao1215/gozen/internal/rpg"
)

func TestBuildingGeometryCacheAndFaces(t *testing.T) {
	fixture := spaceFixtureFiles(t, os.DirFS("../../examples/space-buildings/game"))
	db, err := data.Load(fixture)
	if err != nil {
		t.Fatal(err)
	}
	st := rpg.New(db, nil)
	d := db.Locations["inn"].Dungeon
	v := &spaceView{}
	if !v.refreshOccupancy(st, d) {
		t.Fatal("first geometry was not built")
	}
	if len(v.pixels) != 64*spaceBuildingAtlasHeight*4 {
		t.Fatal("missing building planes")
	}
	material := func(level, x, z int) byte { return v.pixels[((640+level*64+z)*64+x)*4] }
	if material(0, 1, 1) == 0 || material(0, 1, 1) == material(0, 12, 2) || material(0, 12, 2) == material(0, 12, 11) {
		t.Fatal("building wall regions did not select distinct materials")
	}
	// An overhang is indexed outside the wall anchor and above its logical level.
	if v.pixels[(512*64)*4] == 0 {
		t.Fatal("gable eave was culled to anchor cells")
	}
	before := append([]byte(nil), v.buildingPixels...)
	parts := &v.roofPlanes[0]
	if v.refreshOccupancy(st, d) {
		t.Fatal("unchanged geometry uploaded again")
	}
	st.Put = map[string]string{"inn:0:5,6": "."}
	v.refreshOccupancy(st, d)
	if parts != &v.roofPlanes[0] || !bytes.Equal(before, v.buildingPixels) {
		t.Fatal("dynamic occupancy rebuilt static roof/material data")
	}
	// Replacing the DB on reload must invalidate geometry even at the same location.
	fixture["data/world.yaml"].Data = bytes.Replace(fixture["data/world.yaml"].Data, []byte("rise: 1,"), []byte("rise: 0.7,"), 1)
	newer, err := data.Load(fixture)
	if err != nil {
		t.Fatal(err)
	}
	if !v.refreshOccupancy(rpg.New(newer, nil), newer.Locations["inn"].Dungeon) || parts == &v.roofPlanes[0] {
		t.Fatal("reload reused obsolete roof geometry")
	}
}

func buildingBenchmarkFixture(tb testing.TB, mode string) fstest.MapFS {
	tb.Helper()
	fixture := spaceScalingFixture(tb, 40, 36, 4, true)
	var root map[string]any
	if err := yaml.Unmarshal(fixture["data/world.yaml"].Data, &root); err != nil {
		tb.Fatal(err)
	}
	d := spaceFixtureValue[map[string]any](tb, spaceFixtureValue[map[string]any](tb, spaceFixtureValue[map[string]any](tb, root["locations"])["courtyard"])["dungeon"])
	space := spaceFixtureValue[map[string]any](tb, d["space"])
	levels := spaceFixtureValue[[]any](tb, space["levels"])
	rows := strings.Split(strings.TrimRight(spaceFixtureValue[string](tb, levels[3]), "\n"), "\n")
	for z := range 21 {
		rows[z] = strings.Repeat("_", 28) + rows[z][28:]
	}
	levels[3] = strings.Join(rows, "\n")
	space["free_move"] = map[string]any{"speed": 2, "turn_speed": 90, "acceleration": 0}
	if mode != "boxes" {
		space["materials"] = map[string]any{"facade": map[string]any{"picture": "dungeon/limestone.png", "color": "#e0c6a0", "width": 4, "height": 3}, "roof": map[string]any{"picture": "dungeon/paving.png", "color": "#d07755", "width": 1, "height": 1}}
		var regions []any
		for level := 1; level <= 3; level++ {
			regions = append(regions, map[string]any{"level": level, "x": 0, "y": 0, "width": 28, "depth": 21, "material": "facade"})
		}
		space["wall_regions"] = regions
	}
	if mode == "roofs" || mode == "combined" {
		space["roofs"] = []any{
			map[string]any{"shape": "gable", "filled": true, "level": 4, "base": .1, "x": 1.8, "y": 2.8, "width": 4.4, "depth": 10.4, "rise": .7, "thickness": .1, "facing": "east", "material": "roof"},
			map[string]any{"shape": "shed", "filled": true, "level": 4, "base": .1, "x": 14.8, "y": 2.8, "width": 5.4, "depth": 10.4, "rise": .7, "thickness": .1, "facing": "east", "material": "roof"},
			map[string]any{"shape": "flat", "level": 2, "base": .1, "x": 8, "y": 8, "width": 2, "depth": 2, "thickness": .1, "facing": "east", "material": "facade"},
		}
	}
	encoded, err := yaml.Marshal(root)
	if err != nil {
		tb.Fatal(err)
	}
	fixture["data/world.yaml"].Data = encoded
	if mode == "combined" {
		addSpaceBenchmarkSprites(tb, fixture, 64)
	}
	return fixture
}

func BenchmarkBuildingSpaceUpdates(b *testing.B) {
	for _, mode := range []string{"boxes", "materials", "roofs", "combined"} {
		b.Run(mode, func(b *testing.B) {
			db, err := data.Load(buildingBenchmarkFixture(b, mode))
			if err != nil {
				b.Fatal(err)
			}
			st := rpg.New(db, nil)
			dungeon.Ensure(db, st)
			d := db.Locations["courtyard"].Dungeon
			var view spaceView
			view.refreshOccupancy(st, d)
			var controller dungeon.FreeController
			tick := 0
			b.ReportAllocs()
			for b.Loop() {
				forward := 1.0
				if tick%120 >= 60 {
					forward = -1
				}
				controller.Update(db, st, dungeon.FreeInput{Forward: forward}, func([]data.Command) bool { return true })
				if view.refreshOccupancy(st, d) {
					b.Fatal("walking caused static geometry upload")
				}
				tick++
			}
		})
	}
}

func TestBuildingMaterialsFollowAssetReload(t *testing.T) {
	g := wldOpenWith(t, Options{FS: os.DirFS("../../examples/space-buildings/game"), Location: "inn"})
	wldFrames(t, g, 25)
	screen := wldScreen(g)
	scene := sysAs[*dungeonScene](t, g.stack[0])
	scene.draw(g, screen)
	if g.failure != nil {
		t.Fatal(g.failure)
	}
	atlas := scene.space.buildingMaterialAtlas
	sources := append([]*ebiten.Image(nil), scene.space.materialSourcesBuilding...)
	if atlas == nil || len(sources) < 2 {
		t.Fatal("material atlas not populated")
	}
	scene.draw(g, screen)
	if scene.space.buildingMaterialAtlas != atlas || !slices.Equal(sources, scene.space.materialSourcesBuilding) {
		t.Fatal("unchanged materials were rebuilt")
	}
	g.pics.reset()
	scene.draw(g, screen)
	if slices.Equal(sources, scene.space.materialSourcesBuilding) {
		t.Fatal("asset reload kept stale material images")
	}
}
