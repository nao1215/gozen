//go:build graphics

package play

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"image"
	"image/png"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
	"time"
)

// hostBin is testdata/host, built once by TestMain for every scenario.
var hostBin string

// TestMain builds testdata/host once: every scenario runs it as a process of its own,
// so the scenarios run side by side (t.Parallel) instead of each waiting for go run.
func TestMain(m *testing.M) {
	code, err := buildHostAndRun(m)
	if err != nil {
		if _, werr := fmt.Fprintln(os.Stderr, err); werr != nil {
			os.Exit(2)
		}
		if code == 0 {
			code = 1
		}
	}
	os.Exit(code)
}

func buildHostAndRun(m *testing.M) (code int, err error) {
	dir, err := os.MkdirTemp("", "gozen-graphics-")
	if err != nil {
		return 1, err
	}
	defer func() { err = errors.Join(err, os.RemoveAll(dir)) }()
	hostBin = filepath.Join(dir, "host")
	if runtime.GOOS == "windows" {
		hostBin += ".exe"
	}
	build := exec.CommandContext(context.Background(), "go", "build", "-o", hostBin, "./testdata/host") //nolint:gosec // Fixed Go executable building the test host.
	build.Stdout, build.Stderr = os.Stdout, os.Stderr
	if err := build.Run(); err != nil {
		return 1, fmt.Errorf("build testdata/host: %w", err)
	}
	return m.Run(), nil
}

// Each scenario gets a fresh process because Ebitengine supports one loop per
// process. PNGs are produced by ReadPixels inside Draw, never outside the loop.
func TestGraphicsScreens(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name   string
		args   []string
		script string
	}{
		{name: "title", args: nil},
		{name: "field", args: []string{"--start", "inn"}},
		{name: "dialogue", args: []string{"--start", "plaza", "--event", "kes_roof"}},
		{name: "battle", args: []string{"--start", "b1", "--party", "maren,ox,tilly", "--battle", "drips"}},
		{name: "dungeon", args: []string{"--start", "b1", "--party", "maren,ox,tilly"}},
		{name: "recruit", args: []string{"--start", "b1", "--party", "maren,ox,tilly,kes", "--level", "6"}, script: "guarantee-recruit drip_slime\ndo battle: drips\nplay join 5\nwait 30\nshot frame\nquit\n"},
		// F7 balance display. Wait until measuring finishes, and check that the win-rate bar color appears in the upper-right panel.
		{name: "balance", args: []string{"--start", "b2", "--party", "maren,ox,tilly", "--level", "1"}, script: "wait 20\npress graphs\nwait-graphs\nshot frame\nquit\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			root := t.TempDir()
			script := filepath.Join(root, "script.txt")
			body := tc.script
			if body == "" {
				body = "wait 80\nshot frame\nquit\n"
			}
			if err := os.WriteFile(script, []byte(body), 0o600); err != nil {
				t.Fatal(err)
			}
			// the Go Zen sample (1920x1080) at half size in the window
			args := []string{"--dir", "../../examples/go-zen/game", "--dev", "--mute", "--seed", "7", "--window", "0.5", "--script", script, "--shots", root, "--save-dir", filepath.Join(root, "saves")}
			args = append(args, tc.args...)
			ctx, cancel := context.WithTimeout(context.Background(), 4*time.Minute)
			defer cancel()
			cmd := exec.CommandContext(ctx, hostBin, args...) //nolint:gosec // The host built by TestMain; arguments refer to the test fixture and isolated output.
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
			if img.Bounds().Dx() != 1920 || img.Bounds().Dy() != 1080 { // the screen of Go Zen, whatever the window
				t.Fatalf("frame bounds %v", img.Bounds())
			}
			colors := map[[4]uint32]bool{}
			for y := 0; y < 1080; y += 6 {
				for x := 0; x < 1920; x += 6 {
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
			if tc.name == "recruit" && !hasRecruitMonster(img) {
				t.Fatal("recruit question has no large blue monster in the center")
			}
		})
	}
}

// hasBar checks whether the upper-right balance display has the first troop's bar color (around #6cc4f0).
func hasBar(img image.Image) bool {
	for y := 24; y < 560; y++ {
		for x := 1000; x < 1900; x++ {
			r, g, b, _ := img.At(x, y).RGBA()
			if nearColor(r>>8, 0x6c) && nearColor(g>>8, 0xc4) && nearColor(b>>8, 0xf0) {
				return true
			}
		}
	}
	return false
}

func nearColor(v, want uint32) bool { return v+8 >= want && v <= want+8 }

func hasRecruitMonster(img image.Image) bool {
	blue := 0
	for y := 280; y < 680; y += 4 {
		for x := 700; x < 1220; x += 4 {
			r, g, b, _ := img.At(x, y).RGBA()
			if b > r*3/2 && g > r*5/4 && b > 0x5000 {
				blue++
			}
		}
	}
	return blue > 250
}
