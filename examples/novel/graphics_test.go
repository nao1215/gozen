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

// TestGraphicsScreens runs the novel sample in a window (under Xvfb in CI) and takes the
// title, a line of the opening with two portraits on the stage, and the character book
// opened with P (ADR 0031), at 1920x1080.
func TestGraphicsScreens(t *testing.T) {
	for _, tc := range []struct {
		name   string
		args   []string
		script string
		book   bool
	}{
		{name: "title", script: "wait 60\nshot frame\nquit\n"},
		{name: "stage", args: []string{"--event", "opening", "--line", "12"}, script: "wait 60\nshot frame\nquit\n"},
		{name: "characters", args: []string{"--event", "toast"}, script: "wait 60\npress characters\nwait 40\nshot frame\nquit\n", book: true},
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
			if tc.book {
				// The book's band across the top is the window color (#2a1a12), over the
				// line's text box that the scene would show at the bottom otherwise.
				for _, x := range []int{900, 960, 1000} {
					r, g, b, _ := img.At(x, 40).RGBA()
					if r>>8 > 0x50 || g>>8 > 0x40 || b>>8 > 0x38 {
						t.Fatalf("the band at (%d, 40) is %02x%02x%02x, want near 2a1a12", x, r>>8, g>>8, b>>8)
					}
				}
				// Brenna's face shows in the first row of the list.
				face := map[[4]uint32]bool{}
				for y := 180; y < 280; y += 4 {
					for x := 70; x < 160; x += 4 {
						r, g, b, a := img.At(x, y).RGBA()
						face[[4]uint32{r, g, b, a}] = true
					}
				}
				if len(face) < 40 {
					t.Fatalf("no face in the list: %d colors", len(face))
				}
			}
		})
	}
}
