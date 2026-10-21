//go:build graphics && !js

package main

import (
	"bytes"
	"context"
	"image/png"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

// TestGraphicsScreens runs the campaign sample in a window (under Xvfb in CI) and takes
// the title, the war map with the band, the region window and the generals, the sortie
// screen and a clash, at 1920x1080.
func TestGraphicsScreens(t *testing.T) {
	for _, tc := range []struct {
		name   string
		args   []string
		script string
	}{
		{name: "title", script: "wait 60\nshot frame\nquit\n"},
		{name: "map", args: []string{"--start", "war"}, script: "wait 90\nshot frame\nquit\n"},
		{name: "sortie", args: []string{"--start", "war"}, script: "wait 90\npress ok\nwait 20\npress ok\nwait 30\nshot frame\nquit\n"},
		{name: "clash", args: []string{"--start", "war"}, script: "wait 90\npress ok\nwait 20\npress ok\nwait 30\npress down\nwait 5\npress ok\nwait 5\npress down\nwait 5\npress ok\nwait 140\nshot frame\nquit\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			script := filepath.Join(root, "script.txt")
			if err := os.WriteFile(script, []byte(tc.script), 0o600); err != nil {
				t.Fatal(err)
			}
			args := append([]string{"run", ".", "--dev", "--mute", "--seed", "3", "--script", script, "--shots", root, "--save-dir", filepath.Join(root, "saves")}, tc.args...)
			ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
			defer cancel()
			cmd := exec.CommandContext(ctx, "go", args...) //nolint:gosec // Fixed Go executable; arguments refer to this test's private directory.
			var out bytes.Buffer
			cmd.Stdout, cmd.Stderr = &out, &out
			if err := cmd.Run(); err != nil {
				t.Fatalf("game loop: %v\n%s", err, &out)
			}
			b, err := fs.ReadFile(os.DirFS(root), "frame.png")
			if err != nil {
				t.Fatal(err)
			}
			img, err := png.Decode(bytes.NewReader(b))
			if err != nil {
				t.Fatal(err)
			}
			if img.Bounds().Dx() != 1920 || img.Bounds().Dy() != 1080 {
				t.Fatalf("frame bounds %v, want the game's 1920x1080", img.Bounds())
			}
			colors := map[[4]uint32]bool{}
			for y := 0; y < 1080; y += 5 {
				for x := 0; x < 1920; x += 5 {
					r, g, b, a := img.At(x, y).RGBA()
					colors[[4]uint32{r, g, b, a}] = true
				}
			}
			if len(colors) < 64 {
				t.Fatalf("the screen shows little: %d colors", len(colors))
			}
		})
	}
}
