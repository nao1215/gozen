//go:build graphics

package runner

import (
	"bytes"
	"context"
	"image"
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
		name   string
		args   []string
		script string
	}{
		{name: "title", args: nil},
		{name: "field", args: []string{"--start", "camp"}},
		{name: "dialogue", args: []string{"--start", "camp", "--event", "greeting"}},
		{name: "battle", args: []string{"--start", "camp", "--battle", "lesson"}},
		{name: "dungeon", args: []string{"--start", "chamber"}},
		// F7 のバランス表示。測り終えるまで待ち、右上の面に勝率の棒の色が出ることを見る。
		{name: "balance", args: []string{"--start", "grotto"}, script: "wait 20\npress graphs\nwait 900\nshot frame\nquit\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			script := filepath.Join(root, "script.txt")
			body := tc.script
			if body == "" {
				body = "wait 80\nshot frame\nquit\n"
			}
			if err := os.WriteFile(script, []byte(body), 0600); err != nil {
				t.Fatal(err)
			}
			args := []string{"run", "../examples/play", "--dev", "--mute", "--seed", "7", "--window", "0.5", "--script", script, "--shots", root, "--save-dir", filepath.Join(root, "saves")}
			args = append(args, tc.args...)
			ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
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
			if tc.name == "balance" && !hasBar(img) {
				t.Fatal("the balance panel shows no win-rate bar")
			}
		})
	}
}

// hasBar は右上のバランス表示に、最初の群れの棒の色（#6cc4f0 付近）があるかを見る。
func hasBar(img image.Image) bool {
	for y := 12; y < 280; y++ {
		for x := 500; x < 950; x++ {
			r, g, b, _ := img.At(x, y).RGBA()
			if near(r>>8, 0x6c) && near(g>>8, 0xc4) && near(b>>8, 0xf0) {
				return true
			}
		}
	}
	return false
}

func near(v, want uint32) bool { return v+8 >= want && v <= want+8 }
