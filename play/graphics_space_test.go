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
	"testing"
	"time"
)

// The same route supplies the published demonstration and this native E2E.
// Screens must contain geometry, and the trace must prove two climbs and a return.
func TestGraphicsSpace(t *testing.T) {
	out := t.TempDir()
	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Minute)
	defer cancel()
	trace := filepath.Join(out, "trace.jsonl")
	cmd := exec.CommandContext(ctx, hostBin, "--dir", "../examples/space/game", "--dev", "--mute", "--start", "courtyard", "--script", "../scripts/capture-space.txt", "--shots", out, "--save-dir", filepath.Join(out, "saves"), "--trace", trace) //nolint:gosec // TestMain builds the fixed graphics host.
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	if err := cmd.Run(); err != nil {
		t.Fatalf("space: %v\nstdout: %s\nstderr: %s", err, &stdout, &stderr)
	}
	b, err := os.ReadFile(trace) //nolint:gosec // Trace lives in this test's temporary directory.
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]struct{ level, x, y int }{"street": {0, 10, 15}, "stairs": {0, 7, 10}, "stair-side": {0, 8, 8}, "under-stairs": {0, 13, 8}, "second": {1, 7, 6}, "upper-stairs": {1, 13, 6}, "atrium": {2, 13, 10}, "returned": {0, 7, 10}}
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
	// All lower view regions on this route face interior floors or solid stairs.
	// Sky blue here is a geometry hole, not the intentional opening overhead.
	if filepath.Base(file) != "street.png" {
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
