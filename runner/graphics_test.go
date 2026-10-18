//go:build graphics

package runner

import (
	"bytes"
	"context"
	"image/png"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

// Each scenario gets a fresh process because Ebitengine supports one loop per
// process. PNGs are produced by ReadPixels inside Draw, never outside the loop.
func TestGraphicsScreens(t *testing.T) {
	for _, tc := range []struct {
		name string
		args []string
	}{
		{"title", nil},
		{"field", []string{"--start", "camp"}},
		{"dialogue", []string{"--start", "camp", "--event", "greeting"}},
		{"battle", []string{"--start", "camp", "--battle", "lesson"}},
		{"dungeon", []string{"--start", "chamber"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			script := filepath.Join(root, "script.txt")
			if err := os.WriteFile(script, []byte("wait 80\nshot frame\nquit\n"), 0600); err != nil {
				t.Fatal(err)
			}
			args := []string{"run", "../examples/play", "--dev", "--mute", "--seed", "7", "--window", "0.5", "--script", script, "--shots", root, "--save-dir", filepath.Join(root, "saves")}
			args = append(args, tc.args...)
			ctx, cancel := context.WithTimeout(context.Background(), 40*time.Second)
			defer cancel()
			cmd := exec.CommandContext(ctx, "go", args...) //nolint:gosec // Fixed Go executable; arguments refer to the test fixture and isolated output.
			var stdout, stderr bytes.Buffer
			cmd.Stdout = &stdout
			cmd.Stderr = &stderr
			if err := cmd.Run(); err != nil {
				t.Fatalf("game loop: %v\nstdout: %s\nstderr: %s", err, &stdout, &stderr)
			}
			b, err := os.ReadFile(filepath.Join(root, "frame.png")) //nolint:gosec // Fixed basename inside this test's private temporary directory.
			if err != nil {
				t.Fatal(err)
			}
			img, err := png.Decode(bytes.NewReader(b))
			if err != nil {
				t.Fatal(err)
			}
			if img.Bounds().Dx() != 960 || img.Bounds().Dy() != 540 {
				t.Fatalf("frame bounds %v", img.Bounds())
			}
			colors := map[[4]uint32]bool{}
			for y := 0; y < 540; y += 3 {
				for x := 0; x < 960; x += 3 {
					r, g, b, a := img.At(x, y).RGBA()
					colors[[4]uint32{r, g, b, a}] = true
				}
			}
			if len(colors) < 8 {
				t.Fatalf("screen lacks rendered content: %d colors", len(colors))
			}
		})
	}
}
