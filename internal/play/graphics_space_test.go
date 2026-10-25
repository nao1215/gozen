//go:build graphics

package play

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"image/png"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/goccy/go-yaml"

	"github.com/nao1215/gozen/internal/layer2/gamedata"
)

// The same route supplies the published demonstration and this native E2E.
// Screens must contain geometry, and the trace must prove two climbs and a return.
func TestGraphicsSpace(t *testing.T) {
	for _, fast := range []bool{false, true} {
		t.Run(map[bool]string{false: "default", true: "double"}[fast], func(t *testing.T) {
			out := t.TempDir()
			game := filepath.Join(out, "game")
			if err := os.CopyFS(game, os.DirFS("../../examples/space/game")); err != nil {
				t.Fatal(err)
			}
			if fast {
				path := filepath.Join(game, "data", "world.yaml")
				b, err := os.ReadFile(path) //nolint:gosec // Path is a fixture inside this test's private temporary directory.
				if err != nil {
					t.Fatal(err)
				}
				b = bytes.Replace(b, []byte("      space:"), []byte("      space:\n        step_ticks: 9\n        turn_ticks: 9"), 1)
				if err := os.WriteFile(path, b, 0o600); err != nil { //nolint:gosec // Path is a fixture inside this test's private temporary directory.
					t.Fatal(err)
				}
			}
			ctx, cancel := context.WithTimeout(context.Background(), 4*time.Minute)
			defer cancel()
			trace := filepath.Join(out, "trace.jsonl")
			cmd := exec.CommandContext(ctx, hostBin, "--build-info", "--dir", game, "--dev", "--mute", "--start", "courtyard", "--script", "../../scripts/capture-space.txt", "--shots", out, "--save-dir", filepath.Join(out, "saves"), "--trace", trace) //nolint:gosec // TestMain builds the fixed graphics host.
			cmd.Env = append(os.Environ(), "GOZEN_DEBUG=1")
			started := time.Now()
			var stdout, stderr bytes.Buffer
			cmd.Stdout, cmd.Stderr = &stdout, &stderr
			if err := cmd.Run(); err != nil {
				t.Fatalf("space: %v\nstdout: %s\nstderr: %s", err, &stdout, &stderr)
			}
			logGraphicsTiming(t, started, stderr.String())
			b, err := os.ReadFile(trace) //nolint:gosec // Trace lives in this test's temporary directory.
			if err != nil {
				t.Fatal(err)
			}
			want := map[string]struct{ level, x, y int }{"street": {0, 10, 15}, "stairs": {0, 6, 10}, "stair-side": {0, 7, 8}, "under-stairs": {0, 14, 8}, "lower-join": {0, 6, 7}, "second": {1, 6, 6}, "upper-stairs": {1, 14, 6}, "upper-join": {1, 14, 9}, "atrium": {2, 14, 10}, "returned": {0, 6, 10}}
			shot := ""
			for line := range bytes.SplitSeq(b, []byte("\n")) {
				if len(line) == 0 {
					continue
				}
				var row struct {
					T     string `json:"t"`
					File  string `json:"file"`
					Level int    `json:"level"`
					Pos   []int  `json:"pos"`
				}
				if err := json.Unmarshal(line, &row); err != nil {
					t.Fatal(err)
				}
				if row.T == "shot" {
					shot = filepath.Base(row.File)
					shot = shot[:len(shot)-len(filepath.Ext(shot))]
				}
				if row.T == "state" && shot != "" {
					expected, ok := want[shot]
					if !ok {
						t.Fatalf("unexpected shot %s", shot)
					}
					if row.Level != expected.level || len(row.Pos) != 2 || row.Pos[0] != expected.x || row.Pos[1] != expected.y {
						t.Fatalf("%s: position %+v level %d", shot, row.Pos, row.Level)
					}
					checkSpaceFrame(t, filepath.Join(out, shot+".png"))
					delete(want, shot)
					shot = ""
				}
			}
			if len(want) > 0 {
				t.Fatalf("missing checkpoints: %v", want)
			}
		})
	}
	t.Run("slab surfaces", checkSpaceSlab)
}

func checkSpaceFrame(t *testing.T, file string) {
	t.Helper()
	f, err := os.Open(file) //nolint:gosec // Screenshot path is supplied by this test.
	if err != nil {
		t.Fatal(err)
	}
	img, err := png.Decode(f)
	closeErr := f.Close()
	if err != nil {
		t.Fatal(err)
	}
	if closeErr != nil {
		t.Fatal(closeErr)
	}
	// Exclude HUD panels: a broken occupancy atlas once produced only uniform sky.
	colors := map[uint32]bool{}
	bounds := img.Bounds()
	for y := bounds.Dy() / 3; y < bounds.Dy()*3/4; y += 4 {
		for x := bounds.Dx() / 3; x < bounds.Dx()*2/3; x += 4 {
			r, g, b, _ := img.At(x, y).RGBA()
			colors[(r>>11)<<10|(g>>11)<<5|b>>11] = true
		}
	}
	// These ground-level views face floors or solid stairs.
	// Sky blue here is a geometry hole, not the intentional opening overhead.
	if name := filepath.Base(file); name == "stairs.png" || name == "stair-side.png" || name == "under-stairs.png" || name == "returned.png" {
		for y := bounds.Dy() * 3 / 4; y < bounds.Dy(); y += 2 {
			for x := bounds.Dx() / 3; x < bounds.Dx()*2/3; x += 2 {
				r, g, b, _ := img.At(x, y).RGBA()
				if b > r*5/4 && g > r*11/10 && b > 30000 {
					t.Fatalf("%s: sky leaks through floor or stairs at (%d,%d)", file, x, y)
				}
			}
		}
	}
	if len(colors) < 8 {
		t.Fatalf("%s: only %d scene colors, no rendered geometry", file, len(colors))
	}
}

// A colored platform isolates the edge and underside from textures and lighting.
// A zero-thickness floor shows blue sky at the red edge probe.
func checkSpaceSlab(t *testing.T) {
	t.Helper()
	out := t.TempDir()
	game, err := os.ReadFile("../../examples/space/game/game.yaml")
	if err != nil {
		t.Fatal(err)
	}
	world, err := os.ReadFile("../../examples/space/game/data/world.yaml")
	if err != nil {
		t.Fatal(err)
	}
	prefix, _, _ := strings.Cut(string(world), "      space:")
	for _, line := range []string{"      wall: dungeon/limestone.png\n", "      floor_texture: dungeon/paving.png\n", "      ceiling_texture: dungeon/limestone.png\n"} {
		prefix = strings.ReplaceAll(prefix, line, "")
	}
	world = []byte(prefix + `      floor: "#d03020"
      ceiling: "#20c030"
      space:
        levels:
          - |
              .....
              .....
              .....
              .....
              .....
              .....
              .....
              ..S..
          - |
              _____
              _____
              __.__
              _____
              _____
              _____
              _____
              _____
`)
	if err := os.Mkdir(filepath.Join(out, "data"), 0o750); err != nil {
		t.Fatal(err)
	}
	for name, b := range map[string][]byte{"game.yaml": game, "data/world.yaml": world, "capture.txt": []byte("wait 30\nshot slab\nquit\n")} {
		if err := os.WriteFile(filepath.Join(out, name), b, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	cmd := exec.CommandContext(ctx, hostBin, "--dir", out, "--dev", "--mute", "--start", "courtyard", "--script", filepath.Join(out, "capture.txt"), "--shots", out, "--save-dir", filepath.Join(out, "saves")) //nolint:gosec // Fixed test host and isolated generated fixture.
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	if err := cmd.Run(); err != nil {
		t.Fatalf("slab: %v\nstdout: %s\nstderr: %s", err, &stdout, &stderr)
	}
	f, err := os.Open(filepath.Join(out, "slab.png")) //nolint:gosec // Screenshot generated in the test directory.
	if err != nil {
		t.Fatal(err)
	}
	img, err := png.Decode(f)
	closeErr := f.Close()
	if err != nil {
		t.Fatal(err)
	}
	if closeErr != nil {
		t.Fatal(closeErr)
	}
	for _, probe := range []struct {
		name       string
		y, channel int
	}{{"solid edge", 144, 0}, {"underside", 160, 1}, {"open sky", 180, 2}} {
		r, g, b, _ := img.At(480, probe.y).RGBA()
		channels := [3]uint32{r, g, b}
		dominant := channels[probe.channel]
		for i, v := range channels {
			if i != probe.channel && dominant <= v {
				t.Errorf("%s: RGB %v does not show expected material", probe.name, channels)
			}
		}
	}
}

// Run explicitly with -bench BenchmarkGraphicsSpaceScaling -benchtime=1x.
// Normal CI does not run benchmarks. An optional external host permits the same
// harness to compare revisions without changing gameplay configuration.
func BenchmarkGraphicsSpaceScaling(b *testing.B) {
	buildings := os.Getenv("GOZEN_SPACE_BENCH_BUILDINGS")
	free := os.Getenv("GOZEN_SPACE_BENCH_FREE") == "1" || buildings != ""
	sizes, counts, depths := [][2]int{{48, 40}, {64, 64}}, []int{1, 3, 8}, []int{8, 16}
	if free {
		sizes, counts, depths = [][2]int{{40, 36}}, []int{4}, []int{16}
	}
	for _, size := range sizes {
		for _, levels := range counts {
			for _, depth := range depths {
				for _, moving := range []bool{false, true} {
					b.Run(fmt.Sprintf("%dx%dx%d/depth=%d/moving=%t", size[0], size[1], levels, depth, moving), func(b *testing.B) {
						dense := os.Getenv("GOZEN_SPACE_BENCH_DENSE") == "1"
						fixture := spaceScalingFixture(b, size[0], size[1], levels, dense)
						if buildings != "" {
							if buildings != "boxes" && buildings != "materials" && buildings != "roofs" && buildings != "combined" {
								b.Fatal("unknown building benchmark mode")
							}
							fixture = buildingBenchmarkFixture(b, buildings)
						} else if free {
							fixture["data/world.yaml"].Data = bytes.Replace(fixture["data/world.yaml"].Data, []byte("space:"), []byte("space:\n        free_move: {speed: 2, turn_speed: 90, acceleration: 0}"), 1)
						}
						spriteCount := 0
						if raw := os.Getenv("GOZEN_SPACE_BENCH_SPRITES"); raw != "" {
							var err error
							spriteCount, err = strconv.Atoi(raw)
							if err != nil || spriteCount < 0 || spriteCount > 256 {
								b.Fatal("GOZEN_SPACE_BENCH_SPRITES must be in [0, 256]")
							}
							addSpaceBenchmarkSprites(b, fixture, spriteCount)
						}
						fixture["data/world.yaml"].Data = bytes.Replace(fixture["data/world.yaml"].Data, []byte("depth: 16"), fmt.Appendf(nil, "depth: %d", depth), 1)
						root := b.TempDir()
						game := filepath.Join(root, "game")
						if err := os.CopyFS(game, fixture); err != nil {
							b.Fatal(err)
						}
						script := "gap 24\nwait 120\n"
						if moving {
							if free {
								script += "hold up 120\nhold right 120\nhold up 120\n"
							} else {
								script += "press up 4\npress right 2\npress up 4\n"
							}
						} else {
							script += "wait 240\n"
						}
						script += "quit\n"
						scriptPath := filepath.Join(root, "script.txt")
						if err := os.WriteFile(scriptPath, []byte(script), 0o600); err != nil {
							b.Fatal(err)
						}
						executable := hostBin
						if override := os.Getenv("GOZEN_SPACE_BENCH_HOST"); override != "" {
							executable = override
						}
						for range b.N {
							ctx, cancel := context.WithTimeout(b.Context(), 2*time.Minute)
							cmd := exec.CommandContext(ctx, executable, "--build-info", "--dir", game, "--start", "courtyard", "--dev", "--mute", "--script", scriptPath, "--shots", root, "--save-dir", filepath.Join(root, "saves")) //nolint:gosec // Explicit benchmark host, supplied only by the developer running this test.
							cmd.Env = append(os.Environ(), "GOZEN_DEBUG=1")
							var stdout, diagnostic bytes.Buffer
							cmd.Stdout = &stdout
							pipe, err := cmd.StderrPipe()
							if err != nil {
								cancel()
								b.Fatal(err)
							}
							start := time.Now()
							if err := cmd.Start(); err != nil {
								cancel()
								b.Fatal(err)
							}
							var draws, updates, fps, tps, frames []float64
							startup := time.Duration(0)
							tick := 0
							scanner := bufio.NewScanner(pipe)
							for scanner.Scan() {
								line := scanner.Text()
								diagnostic.WriteString(line + "\n")
								fields := strings.Fields(line)
								for i, field := range fields {
									if field == "tick" && i+1 < len(fields) {
										value, parseErr := strconv.Atoi(fields[i+1])
										if parseErr == nil {
											tick = value
										}
									}
									if (field == "draw" || field == "update" || field == "frame") && i+1 < len(fields) {
										elapsed, err := time.ParseDuration(fields[i+1])
										if err != nil {
											continue
										}
										if field == "draw" && startup == 0 {
											startup = time.Since(start)
										}
										if tick < 120 {
											continue
										}
										switch field {
										case "draw":
											draws = append(draws, float64(elapsed)/float64(time.Millisecond))
										case "frame":
											frames = append(frames, float64(elapsed)/float64(time.Millisecond))
										default:
											updates = append(updates, float64(elapsed)/float64(time.Millisecond))
										}
									}
									if tick >= 120 && (field == "fps" || field == "tps") && i+1 < len(fields) {
										value, err := strconv.ParseFloat(fields[i+1], 64)
										if err != nil {
											continue
										}
										if field == "fps" {
											fps = append(fps, value)
										} else {
											tps = append(tps, value)
										}
									}
								}
							}
							scanErr := scanner.Err()
							err = cmd.Wait()
							cancel()
							if err != nil || scanErr != nil {
								b.Fatalf("host: %v / %v\n%s\n%s", err, scanErr, &stdout, &diagnostic)
							}
							if len(draws) == 0 || len(updates) == 0 {
								b.Fatalf("missing samples: %s", &diagnostic)
							}
							b.ReportMetric(float64(startup)/float64(time.Millisecond), "startup-ms")
							if dense {
								b.ReportMetric(1, "dense")
							}
							if free {
								b.ReportMetric(1, "free")
							}
							if spriteCount > 0 {
								b.ReportMetric(float64(spriteCount), "sprites")
							}
							for name, values := range map[string][]float64{"draw": draws, "update": updates, "fps": fps, "tps": tps, "frame": frames} {
								if len(values) == 0 {
									continue
								} // Older comparison hosts lack frame intervals.
								slices.Sort(values)
								b.ReportMetric(values[len(values)/2], name+"-p50")
								b.ReportMetric(values[(len(values)-1)*95/100], name+"-p95")
							}
						}
					})
				}
			}
		}
	}
}

func TestGraphicsSpaceConditionalOccupancy(t *testing.T) {
	fixture := spaceScalingFixture(t, 48, 40, 3, false)
	var root map[string]any
	if err := yaml.Unmarshal(fixture["data/world.yaml"].Data, &root); err != nil {
		t.Fatal(err)
	}
	d := spaceFixtureValue[map[string]any](t, spaceFixtureValue[map[string]any](t, spaceFixtureValue[map[string]any](t, root["locations"])["courtyard"])["dungeon"])
	spaceFixtureValue[map[string]any](t, d["tiles"])["X"] = map[string]any{"look": "wall", "when": "flag.closed", "else": "."}
	levels := spaceFixtureValue[[]any](t, spaceFixtureValue[map[string]any](t, d["space"])["levels"])
	rows := strings.Split(spaceFixtureValue[string](t, levels[0]), "\n")
	row := []byte(rows[14])
	row[10] = 'X'
	rows[14] = string(row)
	levels[0] = strings.Join(rows, "\n")
	var err error
	fixture["data/world.yaml"].Data, err = yaml.Marshal(root)
	if err != nil {
		t.Fatal(err)
	}
	out := t.TempDir()
	game := filepath.Join(out, "game")
	if err := os.CopyFS(game, fixture); err != nil {
		t.Fatal(err)
	}
	script := filepath.Join(out, "script.txt")
	if err := os.WriteFile(script, []byte("wait 40\nshot open\ndo {set: {flag.closed: true}}\nwait 20\nshot closed\ndo {set: {flag.closed: false}}\nwait 20\nshot reopened\npress reload\nwait 30\nshot reloaded\nquit\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(t.Context(), time.Minute)
	defer cancel()
	cmd := exec.CommandContext(ctx, hostBin, "--dir", game, "--dev", "--mute", "--start", "courtyard", "--script", script, "--shots", out, "--save-dir", filepath.Join(out, "saves")) //nolint:gosec // Fixed test host with isolated generated fixture paths.
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	if err := cmd.Run(); err != nil {
		t.Fatalf("%v\n%s\n%s", err, &stdout, &stderr)
	}
	initial := shotImage(t, out, "open")
	for _, name := range []string{"closed", "reopened", "reloaded"} {
		other := shotImage(t, out, name)
		changed := 0
		bounds := initial.Bounds()
		for y := bounds.Dy() / 3; y < bounds.Dy()*2/3; y++ {
			for x := bounds.Dx() / 3; x < bounds.Dx()*2/3; x++ {
				ar, ag, ab, _ := initial.At(x, y).RGBA()
				br, bg, bb, _ := other.At(x, y).RGBA()
				if abs(int(ar)-int(br)) > 257 || abs(int(ag)-int(bg)) > 257 || abs(int(ab)-int(bb)) > 257 {
					changed++
				}
			}
		}
		if name == "closed" && changed < 1000 {
			t.Errorf("conditional wall did not appear: %d changed pixels", changed)
		}
		if name != "closed" && changed != 0 {
			t.Errorf("%s retained stale geometry: %d changed pixels", name, changed)
		}
	}
}

func TestGraphicsSpaceSprites(t *testing.T) {
	t.Parallel()
	out := t.TempDir()
	ctx, cancel := context.WithTimeout(t.Context(), 2*time.Minute)
	defer cancel()
	trace := filepath.Join(out, "trace.jsonl")
	cmd := exec.CommandContext(ctx, hostBin, "--build-info", "--dir", "../../examples/space-town/game", "--dev", "--mute", "--start", "inn", "--script", "../../scripts/capture-space-town.txt", "--shots", out, "--trace", trace, "--save-dir", filepath.Join(out, "saves")) //nolint:gosec // Fixed host, public fixture and isolated outputs.
	cmd.Env = append(os.Environ(), "GOZEN_DEBUG=1")
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	start := time.Now()
	if err := cmd.Run(); err != nil {
		t.Fatalf("town: %v\n%s\n%s", err, &stdout, &stderr)
	}
	logGraphicsTiming(t, start, stderr.String())
	content, err := os.ReadFile(trace) //nolint:gosec // Trace is in this test's temporary directory.
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]struct{ level, x, y int }{"street": {0, 3, 8}, "ground-npc": {0, 4, 6}, "mid-stair": {0, 3, 6}, "turn-back": {0, 3, 7}, "landing": {1, 3, 4}, "bed": {1, 4, 3}, "upper-npc": {1, 4, 6}, "returned": {0, 3, 8}, "reloaded": {0, 3, 8}}
	shot := ""
	for line := range bytes.SplitSeq(content, []byte("\n")) {
		if len(line) == 0 {
			continue
		}
		var row struct {
			T, File, Location string
			Level             int
			Pos               []int
		}
		if err := json.Unmarshal(line, &row); err != nil {
			t.Fatal(err)
		}
		if row.T == "shot" {
			shot = strings.TrimSuffix(filepath.Base(row.File), ".png")
		}
		if row.T == "state" {
			if row.Location != "inn" {
				t.Fatalf("left continuous town: %s", row.Location)
			}
			if expected, ok := want[shot]; ok {
				if row.Level != expected.level || len(row.Pos) != 2 || row.Pos[0] != expected.x || row.Pos[1] != expected.y {
					t.Fatalf("%s: %+v", shot, row)
				}
				delete(want, shot)
			}
			shot = ""
		}
	}
	if len(want) != 0 {
		t.Fatalf("missing checkpoints %v", want)
	}
	colored := func(name string, red bool) int {
		img := shotImage(t, out, name)
		n := 0
		b := img.Bounds()
		for y := b.Dy() / 4; y < b.Dy()*3/4; y++ {
			for x := b.Dx() / 5; x < b.Dx()*4/5; x++ {
				r, g, blue, _ := img.At(x, y).RGBA()
				if (!red && g > r*2 && g > blue*3/2) || (red && r > g*2 && r > blue*2) {
					n++
				}
			}
		}
		return n
	}
	for _, name := range []string{"ground-npc", "upper-npc"} {
		if colored(name, false) < 100 {
			t.Errorf("%s has no visible innkeeper", name)
		}
	}
	for _, name := range []string{"ground-used", "upper-used"} {
		if n := colored(name, false); n != 0 {
			t.Errorf("%s retains %d green pixels after once", name, n)
		}
	}
	if colored("bed", true) < 100 {
		t.Error("upstairs bed is not visible")
	}
}

func TestGraphicsSpaceSpriteOcclusion(t *testing.T) {
	for _, kind := range []string{"open", "wall", "slab", "stairs", "roof", "roof-edge"} {
		t.Run(kind, func(t *testing.T) {
			out := t.TempDir()
			game := filepath.Join(out, "game")
			if err := os.CopyFS(game, os.DirFS("../../examples/space-town/game")); err != nil {
				t.Fatal(err)
			}
			lower, upper := make([]string, 7), make([]string, 7)
			for i := range lower {
				lower[i] = "......."
				upper[i] = "_______"
			}
			put := func(rows []string, x, y int, ch byte) { row := []byte(rows[y]); row[x] = ch; rows[y] = string(row) }
			put(lower, 3, 5, 'S')
			put(lower, 3, 2, 'N')
			stairs := ""
			spriteSize := 0.6
			switch kind {
			case "wall":
				put(lower, 3, 3, '#')
			case "slab":
				put(lower, 3, 2, '.')
				for i := range upper {
					upper[i] = "......."
				}
				put(upper, 3, 2, 'N')
			case "stairs":
				put(lower, 3, 2, '.')
				put(lower, 3, 0, 'N')
				put(upper, 3, 1, '.')
				stairs = "\n        stairs: [{level: 1, x: 3, y: 4, facing: north, length: 3}]"
			case "roof", "roof-edge":
				lower[3] = "_______"
				rise := 1.4
				if kind == "roof-edge" {
					rise = 0.65
					spriteSize = 2
				}
				stairs = fmt.Sprintf("\n        roofs: [{shape: gable, filled: true, level: 1, base: 0.1, x: 2.5, y: 3.2, width: 2, depth: 0.6, rise: %g, thickness: 0.1, facing: east}]", rise)
			}
			source, err := os.ReadFile("../../examples/space-town/game/data/world.yaml")
			if err != nil {
				t.Fatal(err)
			}
			prefix, _, _ := strings.Cut(string(source), "locations:")
			world := prefix + fmt.Sprintf("locations:\n  inn:\n    name: Occlusion\n    dungeon:\n      depth: 16\n      darkness: 0\n      tiles:\n        S: {start: true, facing: north}\n        N: {picture: innkeeper.png, size: 0.6, when: 'not flag.hide', else: '.'}\n      space:\n        levels: [%q, %q]%s\n", strings.Join(lower, "\n"), strings.Join(upper, "\n"), stairs)
			world = strings.Replace(world, "size: 0.6", fmt.Sprintf("size: %g", spriteSize), 1)
			if err := os.WriteFile(filepath.Join(game, "data", "world.yaml"), []byte(world), 0o600); err != nil { //nolint:gosec // Generated fixture under this test's private temporary directory.
				t.Fatal(err)
			}
			if _, err := gamedata.Load(os.DirFS(game)); err != nil {
				t.Fatal(err)
			}
			script := filepath.Join(out, "script.txt")
			if err := os.WriteFile(script, []byte("wait 40\nshot visible\ndo {set: {flag.hide: true}}\nwait 20\nshot control\nquit\n"), 0o600); err != nil {
				t.Fatal(err)
			}
			ctx, cancel := context.WithTimeout(t.Context(), time.Minute)
			defer cancel()
			cmd := exec.CommandContext(ctx, hostBin, "--dir", game, "--dev", "--mute", "--start", "inn", "--script", script, "--shots", out, "--save-dir", filepath.Join(out, "saves")) //nolint:gosec // Generated fixture and fixed graphics host.
			var stdout, stderr bytes.Buffer
			cmd.Stdout, cmd.Stderr = &stdout, &stderr
			if err := cmd.Run(); err != nil {
				t.Fatalf("%v\n%s\n%s", err, &stdout, &stderr)
			}
			a, b := shotImage(t, out, "visible"), shotImage(t, out, "control")
			bounds := a.Bounds()
			changed := 0
			for y := bounds.Dy() / 4; y < bounds.Dy()*3/4; y++ {
				for x := bounds.Dx() / 4; x < bounds.Dx()*3/4; x++ {
					ar, ag, ab, _ := a.At(x, y).RGBA()
					br, bg, bb, _ := b.At(x, y).RGBA()
					if abs(int(ar)-int(br)) > 257 || abs(int(ag)-int(bg)) > 257 || abs(int(ab)-int(bb)) > 257 {
						changed++
					}
				}
			}
			if kind == "roof-edge" {
				if changed < 100 {
					t.Fatal("large sprite was hidden entirely by its anchor")
				}
				for y := bounds.Dy() / 2; y < bounds.Dy()*3/4; y++ {
					for x := bounds.Dx() * 2 / 5; x < bounds.Dx()*3/5; x++ {
						if a.At(x, y) != b.At(x, y) {
							t.Fatal("large sprite leaked through lower roof volume")
						}
					}
				}
			} else if kind == "open" {
				if changed < 100 {
					t.Fatal("positive control has no visible sprite")
				}
				// This point lies in the image's transparent upper-left margin.
				x, y := bounds.Dx()/2-65, bounds.Dy()/2-70
				if a.At(x, y) != b.At(x, y) {
					t.Fatal("transparent margin changed the background")
				}
			} else if changed != 0 {
				t.Fatalf("sprite leaked through %s: %d pixels", kind, changed)
			}
		})
	}
}

func TestGraphicsSpaceFreeMovement(t *testing.T) {
	for _, town := range []bool{true, false} {
		t.Run(map[bool]string{true: "inn", false: "plaza"}[town], func(t *testing.T) {
			out := t.TempDir()
			game := filepath.Join(out, "game")
			root, loc, route := "../../examples/space/game", "courtyard", "../../scripts/capture-space.txt"
			if town {
				root, loc, route = "../../examples/space-town/game", "inn", "../../scripts/capture-space-free.txt"
			}
			if err := os.CopyFS(game, os.DirFS(root)); err != nil {
				t.Fatal(err)
			}
			worldPath := filepath.Join(game, "data", "world.yaml")
			world, err := os.ReadFile(worldPath) //nolint:gosec // Isolated copied fixture.
			if err != nil {
				t.Fatal(err)
			}
			world = bytes.Replace(world, []byte("      space:"), []byte("      space:\n        free_move: {speed: 2, turn_speed: 90, acceleration: 0}"), 1)
			if err := os.WriteFile(worldPath, world, 0o600); err != nil { //nolint:gosec // Isolated copied fixture.
				t.Fatal(err)
			}
			if _, err := gamedata.Load(os.DirFS(game)); err != nil {
				t.Fatal(err)
			}
			script, err := os.ReadFile(route)
			if err != nil {
				t.Fatal(err)
			}
			if !town {
				var converted strings.Builder
				for line := range strings.SplitSeq(string(script), "\n") {
					fields := strings.Fields(line)
					if len(fields) >= 2 && fields[0] == "press" {
						n := 1
						if len(fields) == 3 {
							n, err = strconv.Atoi(fields[2])
							if err != nil {
								t.Fatal(err)
							}
						}
						ticks := 60
						if fields[1] == "up" {
							ticks = 30
						}
						fmt.Fprintf(&converted, "hold %s %d\n", fields[1], n*ticks)
					} else {
						converted.WriteString(line + "\n")
					}
				}
				script = []byte(converted.String())
			}
			scriptPath := filepath.Join(out, "route.txt")
			if err := os.WriteFile(scriptPath, script, 0o600); err != nil { //nolint:gosec // Isolated test script.
				t.Fatal(err)
			}
			trace := filepath.Join(out, "trace.jsonl")
			ctx, cancel := context.WithTimeout(t.Context(), 3*time.Minute)
			defer cancel()
			cmd := exec.CommandContext(ctx, hostBin, "--build-info", "--dir", game, "--dev", "--mute", "--start", loc, "--script", scriptPath, "--shots", out, "--trace", trace, "--save-dir", filepath.Join(out, "saves")) //nolint:gosec // Fixed native test host, isolated paths.
			cmd.Env = append(os.Environ(), "GOZEN_DEBUG=1")
			var stdout, stderr bytes.Buffer
			cmd.Stdout, cmd.Stderr = &stdout, &stderr
			start := time.Now()
			if err := cmd.Run(); err != nil {
				t.Fatalf("%v\n%s\n%s", err, &stdout, &stderr)
			}
			logGraphicsTiming(t, start, stderr.String())
			content, err := os.ReadFile(trace) //nolint:gosec // Isolated test output.
			if err != nil {
				t.Fatal(err)
			}
			want := map[string]int{"street": 0, "second": 1, "atrium": 2, "returned": 0}
			if town {
				want = map[string]int{"street": 0, "mid-stair": 0, "turn-back": 0, "landing": 1, "bed": 1, "upper-npc": 1, "returned": 0, "off-center": 0, "reloaded": 0}
			}
			positions := map[string][2]float64{"street": {10.5, 15.5}, "second": {6.5, 6.5}, "atrium": {14.5, 10.5}, "returned": {6.5, 10.5}}
			if town {
				positions = map[string][2]float64{"street": {3.5, 8.5}, "mid-stair": {3.5, 6.5}, "turn-back": {3.5, 7.5}, "landing": {3.5, 4.5}, "bed": {4.5, 3.5}, "upper-npc": {4.5, 6.5}, "returned": {3.5, 8.5}}
			}
			var before map[string]float64
			shot := ""
			for line := range bytes.SplitSeq(content, []byte("\n")) {
				if len(line) == 0 {
					continue
				}
				var row struct {
					T, File, Location string
					Level             int
					SpacePose         map[string]float64 `json:"space_pose"`
					Summary           struct {
						Flags map[string]bool
						Vars  map[string]any
					}
				}
				if err := json.Unmarshal(line, &row); err != nil {
					t.Fatal(err)
				}
				if row.T == "shot" {
					shot = strings.TrimSuffix(filepath.Base(row.File), ".png")
				}
				if row.T != "state" || shot == "" {
					continue
				}
				if row.Location != loc || row.SpacePose == nil {
					t.Fatalf("invalid continuous state: %+v", row)
				}
				if level, ok := want[shot]; ok {
					if row.Level != level {
						t.Fatalf("%s level=%d want=%d pose=%v", shot, row.Level, level, row.SpacePose)
					}
					delete(want, shot)
				}
				if pos, ok := positions[shot]; ok {
					if math.Abs(row.SpacePose["x"]-pos[0]) > .01 || math.Abs(row.SpacePose["z"]-pos[1]) > .01 {
						t.Fatalf("%s pose=%v want x/z=%v", shot, row.SpacePose, pos)
					}
				}
				if town && shot == "returned" && (!row.Summary.Flags["bed_seen"] || row.Summary.Vars["greetings"] != float64(1)) {
					t.Fatalf("free interaction did not complete: %+v", row.Summary)
				}
				if shot == "off-center" {
					before = row.SpacePose
					if math.Abs(before["x"]-math.Floor(before["x"])-.5) < .01 {
						t.Fatal("position snapped to cell center")
					}
					if math.Abs(math.Remainder(before["yaw"], math.Pi/2)) < .01 {
						t.Fatal("yaw snapped to cardinal")
					}
				}
				if shot == "reloaded" {
					for _, key := range []string{"x", "z", "yaw", "level"} {
						if math.Abs(row.SpacePose[key]-before[key]) > 1e-8 {
							t.Fatalf("reload changed %s", key)
						}
					}
				}
				shot = ""
			}
			if len(want) != 0 {
				t.Fatalf("missing checkpoints %v", want)
			}
			checkSpaceFrame(t, filepath.Join(out, "returned.png"))
		})
	}
}

func TestGraphicsSpaceBuildings(t *testing.T) {
	out := t.TempDir()
	trace := filepath.Join(out, "trace.jsonl")
	ctx, cancel := context.WithTimeout(t.Context(), 3*time.Minute)
	defer cancel()
	cmd := exec.CommandContext(ctx, hostBin, "--dir", "../../examples/space-buildings/game", "--start", "inn", "--dev", "--mute", "--script", "../../scripts/capture-space-buildings.txt", "--shots", out, "--trace", trace, "--save-dir", filepath.Join(out, "saves")) //nolint:gosec // Fixed public fixture and isolated output.
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	if err := cmd.Run(); err != nil {
		t.Fatalf("%v\n%s\n%s", err, &stdout, &stderr)
	}
	content, err := os.ReadFile(trace) //nolint:gosec // Isolated trace.
	if err != nil {
		t.Fatal(err)
	}
	want := map[string][3]int{"town": {0, 3, 12}, "shop": {0, 14, 8}, "shop-room": {0, 14, 5}, "stone-landing": {1, 11, 11}, "elevated": {1, 12, 11}, "street-return": {0, 3, 8}, "inn-landing": {1, 3, 4}, "inn-bed": {1, 4, 3}, "inn-npc": {1, 4, 6}, "returned": {0, 3, 8}}
	shot := ""
	var before map[string]float64
	for line := range bytes.SplitSeq(content, []byte("\n")) {
		if len(line) == 0 {
			continue
		}
		var row struct {
			T, File, Location string
			Level             int
			Pos               []int
			SpacePose         map[string]float64 `json:"space_pose"`
			Summary           struct {
				Flags map[string]bool
				Vars  map[string]any
			}
		}
		if err := json.Unmarshal(line, &row); err != nil {
			t.Fatal(err)
		}
		if row.T == "shot" {
			shot = strings.TrimSuffix(filepath.Base(row.File), ".png")
		}
		if row.T != "state" || shot == "" {
			continue
		}
		if row.Location != "inn" || row.SpacePose == nil {
			t.Fatalf("invalid continuous town state: %+v", row)
		}
		if pos, ok := want[shot]; ok {
			if row.Level != pos[0] || len(row.Pos) != 2 || row.Pos[0] != pos[1] || row.Pos[1] != pos[2] {
				t.Fatalf("%s: %+v want %v", shot, row, pos)
			}
			delete(want, shot)
		}
		if shot == "returned" && (!row.Summary.Flags["bed_seen"] || row.Summary.Vars["greetings"] != float64(1)) {
			t.Fatalf("missing interactions: %+v", row.Summary)
		}
		if shot == "angled" {
			before = row.SpacePose
		}
		if shot == "reloaded" {
			for _, key := range []string{"x", "z", "yaw", "level"} {
				if math.Abs(row.SpacePose[key]-before[key]) > 1e-8 {
					t.Fatalf("reload changed %s", key)
				}
			}
		}
		shot = ""
	}
	if len(want) > 0 {
		t.Fatalf("missing checkpoints: %v", want)
	}
	for _, name := range []string{"town", "shop", "shop-room", "elevated", "inn-landing"} {
		checkSpaceFrame(t, filepath.Join(out, name+".png"))
	}
	// Above the shop, the roof is red; immediately below its edge the canopy
	// alternates cream/red. The view must retain open blue sky above both.
	img := shotImage(t, out, "elevated")
	red, sky := 0, 0
	for y := 200; y < 350; y++ {
		for x := 350; x < 700; x++ {
			r, g, b, _ := img.At(x, y).RGBA()
			if r > g*3/2 && r > b*3/2 {
				red++
			}
			if b > r && b > g {
				sky++
			}
		}
	}
	if red < 1000 || sky < 1000 {
		t.Fatalf("roof/sky junction missing: red=%d sky=%d", red, sky)
	}
}
