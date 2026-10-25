//go:build graphics

package play

import (
	"bytes"
	"context"
	"encoding/json"
	"image/png"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
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
