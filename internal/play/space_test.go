package play

import (
	"bytes"
	"fmt"
	"io/fs"
	"os"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/goccy/go-yaml"

	"github.com/nao1215/gozen/internal/expr"
	data "github.com/nao1215/gozen/internal/layer2/gamedata"

	"github.com/hajimehoshi/ebiten/v2"

	"github.com/nao1215/gozen/internal/rpg"
)

func TestSpaceArrivalClearsMotion(t *testing.T) {
	s := &dungeonScene{}
	s.spaceMotion(rpg.Pos{X: 10, Y: 10, Level: 1}, 0, 0)
	s.arrive()
	if s.space.remaining != 0 {
		t.Fatal("arrival keeps motion from the previous location")
	}
}

func TestSpaceConfiguredMotion(t *testing.T) {
	g := wldOpenWith(t, Options{FS: os.DirFS("../../examples/space/game"), Location: "courtyard"})
	wldFrames(t, g, 25)
	s, ok := g.stack[0].(*dungeonScene)
	if !ok {
		t.Fatalf("scene %T", g.stack[0])
	}
	space := g.db.Locations["courtyard"].Dungeon.Space
	space.StepTicks, space.TurnTicks = 9, 6
	g.st.Pos = rpg.Pos{In: "courtyard", X: 6, Y: 10}
	for n := 1; n <= 4; n++ {
		s.forward(g)
		if s.space.duration != 9 || s.space.remaining != 9 || g.st.Pos.Y != 10-n {
			t.Fatalf("step %d: duration %d, remaining %d, pos %+v", n, s.space.duration, s.space.remaining, g.st.Pos)
		}
		for range 9 {
			s.update(g)
		}
		if s.space.remaining != 0 {
			t.Fatal("step did not finish on its configured tick")
		}
	}
	if g.st.Pos.Level != 1 {
		t.Fatal("fast steps did not reach the next floor")
	}
	s.turn(g, 2, "back")
	if s.space.duration != 6 || s.space.remaining != 6 {
		t.Fatal("turn reused the step duration")
	}
	for range 6 {
		s.update(g)
	}
	if s.space.remaining != 0 {
		t.Fatal("turn did not finish on its configured tick")
	}
}

func TestSpacePresentation(t *testing.T) {
	g := wldOpenWith(t, Options{FS: os.DirFS("../../examples/space/game"), Location: "courtyard"})
	g.cfg.DungeonSnap = true // Ordinary-dungeon presentation must not disable space interpolation.
	wldFrames(t, g, 25)
	s, ok := g.stack[len(g.stack)-1].(*dungeonScene)
	if !ok {
		t.Fatalf("scene = %T", g.stack[len(g.stack)-1])
	}
	g.st.Pos = rpg.Pos{In: "courtyard", X: 6, Y: 10}
	screen := wldScreen(g)
	for n := 1; n <= 4; n++ {
		s.forward(g)
		if s.space == nil || s.space.remaining != 18 {
			t.Fatal("instant dungeon transitions disabled stacked-space motion")
		}
		if g.st.Pos.Y != 10-n {
			t.Fatalf("step %d: %+v", n, g.st.Pos)
		}
		for range 18 {
			s.update(g)
			s.draw(g, screen)
		}
		if s.space.remaining != 0 {
			t.Fatal("motion did not settle")
		}
	}
	if g.st.Pos.Level != 1 {
		t.Fatal("did not reach upper level")
	}
	s.turn(g, 2, "back")
	for range 18 {
		s.update(g)
		s.draw(g, screen)
	}
	for range 4 {
		s.forward(g)
		for range 18 {
			s.update(g)
			s.draw(g, screen)
		}
	}
	if g.st.Pos.Level != 0 || g.st.Pos.Y != 10 {
		t.Fatalf("return: %+v", g.st.Pos)
	}
	// A removed floor must become open air rather than an opaque wall.
	g.st.Put = map[string]string{"courtyard/level/0@10,15": " "}
	s.draw(g, screen)
	if s.space.pixels[(15*64+10)*4] != 0 {
		t.Fatal("removed floor rendered as a wall")
	}
	// A resize retains bounded rendering, and a second draw reuses material storage.
	s.drawSpace(g, ebiten.NewImage(320, 180), g.db.Locations["courtyard"].Dungeon)
	if s.space.frame.Bounds().Dx() != 320 {
		t.Fatal("render size did not follow window")
	}
	materials := s.space.ceilingSky
	s.draw(g, screen)
	if s.space.ceilingSky != materials {
		t.Fatal("unchanged materials reallocated")
	}
}

func TestSpaceOccupancyReuse(t *testing.T) {
	g := wldOpenWith(t, Options{FS: os.DirFS("../../examples/space/game"), Location: "courtyard"})
	d := g.db.Locations["courtyard"].Dungeon
	wldFrames(t, g, 25)
	scene, ok := g.stack[0].(*dungeonScene)
	if !ok {
		t.Fatalf("unexpected scene %T", g.stack[0])
	}
	scene.draw(g, wldScreen(g))
	if scene.space.refreshOccupancy(g.st, d) {
		t.Fatal("Draw cached a temporary level view instead of the immutable definition")
	}

	v := &spaceView{}
	if !v.refreshOccupancy(g.st, d) {
		t.Fatal("first occupancy was not built")
	}
	g.st.Frames++
	if v.refreshOccupancy(g.st, d) {
		t.Fatal("unchanged geometry rebuilt for frame advancement")
	}
	g.st.Put = map[string]string{"courtyard/level/0@10,15": " "}
	if !v.refreshOccupancy(g.st, d) || v.pixels[(15*64+10)*4] != 0 {
		t.Fatal("tile mutation did not invalidate occupancy")
	}
	if v.refreshOccupancy(g.st, d) {
		t.Fatal("unchanged mutated geometry rebuilt")
	}
}

func TestSpaceOccupancyStateChanges(t *testing.T) {
	for _, tc := range []struct {
		name, condition string
		change          func(*rpg.State)
	}{
		{"flag", "flag.closed", func(s *rpg.State) { s.Flags["closed"] = true }},
		{"variable", "var.wall > 0", func(s *rpg.State) { s.Vars["wall"] = float64(1) }},
		{"gold", "gold > 0", func(s *rpg.State) { s.Gold = 1 }},
		{"position", "pos.x == 11", func(s *rpg.State) { s.Pos.X = 11 }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			g := wldOpenWith(t, Options{FS: os.DirFS("../../examples/space/game"), Location: "courtyard"})
			d := g.db.Locations["courtyard"].Dungeon
			e, err := expr.Parse(tc.condition)
			if err != nil {
				t.Fatal(err)
			}
			d.Tiles["S"].Solid = data.Expr{Expr: e}
			v := &spaceView{}
			v.refreshOccupancy(g.st, d)
			idx := (15*64 + 10) * 4
			if v.pixels[idx] != 1 {
				t.Fatalf("initial occupancy %d", v.pixels[idx])
			}
			tc.change(g.st)
			if !v.refreshOccupancy(g.st, d) || v.pixels[idx] != 2 {
				t.Fatalf("state change not reflected: %d", v.pixels[idx])
			}
			expected := &spaceView{}
			expected.buildOccupancy(g.st, d)
			if !bytes.Equal(v.pixels, expected.pixels) {
				t.Fatal("cached geometry disagrees with movement's cell rules")
			}
			if v.refreshOccupancy(g.st, d) {
				t.Fatal("unchanged conditional scene rebuilt")
			}
		})
	}
}

func TestSpaceOccupancyOnceRestoreAndReload(t *testing.T) {
	g := wldOpenWith(t, Options{FS: os.DirFS("../../examples/space/game"), Location: "courtyard"})
	d := g.db.Locations["courtyard"].Dungeon
	d.Tiles["S"].Once = true
	d.Tiles["S"].After = "#"
	v := &spaceView{}
	v.refreshOccupancy(g.st, d)
	saved, err := g.st.Save()
	if err != nil {
		t.Fatal(err)
	}
	g.st.Done["courtyard/level/0@10,15"] = true
	if !v.refreshOccupancy(g.st, d) || v.pixels[(15*64+10)*4] != 2 {
		t.Fatal("once/after not rebuilt")
	}
	restored, err := rpg.Parse(g.db, saved, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !v.refreshOccupancy(restored, d) || v.pixels[(15*64+10)*4] != 1 {
		t.Fatal("save restore retained stale geometry")
	}
	db, err := data.Load(os.DirFS("../../examples/space/game"))
	if err != nil {
		t.Fatal(err)
	}
	reloaded := db.Locations["courtyard"].Dungeon
	if !v.refreshOccupancy(restored, reloaded) {
		t.Fatal("reloaded definition not rebuilt")
	}
	// A changed stair definition must be reflected after reload as well.
	reloadedCopy := *reloaded
	spaceCopy := *reloaded.Space
	spaceCopy.Stairs = nil
	reloadedCopy.Space = &spaceCopy
	v.refreshOccupancy(restored, &reloadedCopy)
	expected := &spaceView{}
	expected.buildOccupancy(restored, &reloadedCopy)
	if !bytes.Equal(v.pixels, expected.pixels) {
		t.Fatal("reload retained old stairs")
	}
}

func TestSpaceOccupancyRandomConditionsAreNotCached(t *testing.T) {
	g := wldOpenWith(t, Options{FS: os.DirFS("../../examples/space/game"), Location: "courtyard"})
	d := g.db.Locations["courtyard"].Dungeon
	e, err := expr.Parse("chance(0.5)")
	if err != nil {
		t.Fatal(err)
	}
	d.Tiles["S"].Solid = data.Expr{Expr: e}
	v := &spaceView{}
	for range 3 {
		if !v.refreshOccupancy(g.st, d) {
			t.Fatal("random condition cached")
		}
	}
}

// spaceScalingFixture preserves the public sample in the northwest corner and
// pads aligned levels with open air. Dense variants fill only the added area;
// the one-level fixture removes the stairs that no longer have a destination.
func spaceScalingFixture(tb testing.TB, w, h, levels int, dense bool) fstest.MapFS {
	tb.Helper()
	source := os.DirFS("../../examples/space/game")
	out := fstest.MapFS{}
	if err := fs.WalkDir(source, ".", func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			return nil
		}
		b, err := fs.ReadFile(source, path)
		if err != nil {
			return err
		}
		out[path] = &fstest.MapFile{Data: b}
		return nil
	}); err != nil {
		tb.Fatal(err)
	}
	var root map[string]any
	if err := yaml.Unmarshal(out["data/world.yaml"].Data, &root); err != nil {
		tb.Fatal(err)
	}
	d := spaceFixtureValue[map[string]any](tb, spaceFixtureValue[map[string]any](tb, spaceFixtureValue[map[string]any](tb, root["locations"])["courtyard"])["dungeon"])
	space := spaceFixtureValue[map[string]any](tb, d["space"])
	original := spaceFixtureValue[[]any](tb, space["levels"])
	padded := make([]string, levels)
	for level := range levels {
		var rows []string
		if level < len(original) {
			rows = strings.Split(strings.TrimRight(spaceFixtureValue[string](tb, original[level]), "\n"), "\n")
		}
		var grid strings.Builder
		for y := range h {
			row := ""
			if y < len(rows) {
				row = rows[y]
			}
			for x := len(row); x < w; x++ {
				ch := byte('_')
				if dense {
					ch = '.'
					if x%4 == 0 && y%4 == 0 {
						ch = '#'
					}
				}
				row += string(ch)
			}
			grid.WriteString(row)
			grid.WriteByte('\n')
		}
		padded[level] = grid.String()
	}
	space["levels"] = padded
	if levels == 1 {
		delete(space, "stairs")
	}
	encoded, err := yaml.Marshal(root)
	if err != nil {
		tb.Fatal(err)
	}
	out["data/world.yaml"] = &fstest.MapFile{Data: encoded}
	return out
}

func BenchmarkSpaceOccupancy(b *testing.B) {
	for _, size := range [][2]int{{48, 40}, {64, 64}} {
		for _, levels := range []int{1, 3, 8} {
			for _, dense := range []bool{false, true} {
				label := fmt.Sprintf("%dx%dx%d/dense=%t", size[0], size[1], levels, dense)
				db, err := data.Load(spaceScalingFixture(b, size[0], size[1], levels, dense))
				if err != nil {
					b.Fatal(err)
				}
				state := rpg.New(db, nil)
				d := db.Locations["courtyard"].Dungeon
				for _, cached := range []bool{false, true} {
					b.Run(fmt.Sprintf("%s/cached=%t", label, cached), func(b *testing.B) {
						v := &spaceView{}
						v.refreshOccupancy(state, d)
						b.ReportAllocs()
						b.ResetTimer()
						for b.Loop() {
							if cached {
								v.refreshOccupancy(state, d)
							} else {
								v.buildOccupancy(state, d)
							}
						}
					})
				}
			}
		}
	}
}

func TestSpaceOccupancyWhenAndFailureFallback(t *testing.T) {
	g := wldOpenWith(t, Options{FS: os.DirFS("../../examples/space/game"), Location: "courtyard"})
	d := g.db.Locations["courtyard"].Dungeon
	e, err := expr.Parse("flag.visible")
	if err != nil {
		t.Fatal(err)
	}
	d.Tiles["S"].When = data.Expr{Expr: e}
	d.Tiles["S"].Else = "#"
	v := &spaceView{}
	v.refreshOccupancy(g.st, d)
	if v.pixels[(15*64+10)*4] != 2 {
		t.Fatal("conditional fallback missing")
	}
	g.st.Flags["visible"] = true
	if !v.refreshOccupancy(g.st, d) || v.pixels[(15*64+10)*4] != 1 {
		t.Fatal("condition visibility remained cached")
	}
	// Unserializable transient state must never freeze the previous geometry.
	g.st.Vars["unsupported"] = func() {}
	for range 2 {
		if !v.refreshOccupancy(g.st, d) {
			t.Fatal("failed state snapshot reused")
		}
	}
}

func BenchmarkSpaceOccupancyConditional(b *testing.B) {
	db, err := data.Load(spaceScalingFixture(b, 64, 64, 8, false))
	if err != nil {
		b.Fatal(err)
	}
	state := rpg.New(db, nil)
	d := db.Locations["courtyard"].Dungeon
	e, err := expr.Parse("gold > 0")
	if err != nil {
		b.Fatal(err)
	}
	d.Tiles["S"].Solid = data.Expr{Expr: e}
	for _, mode := range []string{"rebuild", "unchanged", "changed"} {
		b.Run(mode, func(b *testing.B) {
			v := &spaceView{}
			v.refreshOccupancy(state, d)
			b.ReportAllocs()
			b.ResetTimer()
			for b.Loop() {
				switch mode {
				case "rebuild":
					v.buildOccupancy(state, d)
				case "changed":
					state.Gold++
					v.refreshOccupancy(state, d)
				default:
					v.refreshOccupancy(state, d)
				}
			}
		})
	}
}

func spaceFixtureValue[T any](tb testing.TB, value any) T {
	tb.Helper()
	out, ok := value.(T)
	if !ok {
		tb.Fatalf("fixture value has unexpected type %T", value)
	}
	return out
}
