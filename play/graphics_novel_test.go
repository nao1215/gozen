//go:build graphics

package play

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"golang.org/x/image/font/gofont/goregular"
)

// TestGraphicsNovel plays testdata/novel five times with one save directory (ADR 0026).
// The first run reads two lines. In the second, skip mode passes them and stops before the
// third line, which was not read; the player reads it. In the third, skip mode passes the
// three lines and stops before the choice. In the fourth, rolling back from the third line
// shows the second, and going on shows the third again. The fifth stands a figure with stand
// and lets it leave (ADR 0028). read.json carries the read lines between the runs.
func TestGraphicsNovel(t *testing.T) {
	root := t.TempDir()
	game := filepath.Join(root, "game")
	if err := os.CopyFS(game, os.DirFS("testdata/novel")); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(game, "assets"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(game, "assets", "regular.ttf"), goregular.TTF, 0o600); err != nil {
		t.Fatal(err)
	}
	// A tall magenta figure for stand (a portrait 600 pixels or taller stands on the stage).
	fig := image.NewNRGBA(image.Rect(0, 0, 240, 720))
	draw.Draw(fig, fig.Bounds(), &image.Uniform{color.NRGBA{0xff, 0, 0xff, 0xff}}, image.Point{}, draw.Src)
	var pb bytes.Buffer
	if err := png.Encode(&pb, fig); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(game, "assets", "fig.png"), pb.Bytes(), 0o600); err != nil {
		t.Fatal(err)
	}
	saves := filepath.Join(root, "saves")
	run := func(name, body string, event ...string) []map[string]any {
		t.Helper()
		script := filepath.Join(root, name+".txt")
		if err := os.WriteFile(script, []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
		trace := filepath.Join(root, name+".jsonl")
		args := []string{
			"run", "./testdata/host", "--dir", game, "--dev", "--mute", "--seed", "7", "--start", "room", "--event", append(event, "story")[0],
			"--script", script, "--shots", root, "--save-dir", saves, "--trace", trace,
		}
		ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
		defer cancel()
		cmd := exec.CommandContext(ctx, "go", args...) //nolint:gosec // Fixed Go executable; arguments refer to the test fixture and isolated output.
		var stdout, stderr bytes.Buffer
		cmd.Stdout, cmd.Stderr = &stdout, &stderr
		if err := cmd.Run(); err != nil {
			t.Fatalf("%s: game loop: %v\nstdout: %s\nstderr: %s", name, err, &stdout, &stderr)
		}
		f, err := os.Open(trace) //nolint:gosec // A fixed name in this test's temporary directory.
		if err != nil {
			t.Fatal(err)
		}
		defer func() {
			if err := f.Close(); err != nil {
				t.Error(err)
			}
		}()
		var out []map[string]any
		sc := bufio.NewScanner(f)
		sc.Buffer(make([]byte, 1<<20), 1<<24)
		for sc.Scan() {
			var l map[string]any
			if err := json.Unmarshal(sc.Bytes(), &l); err != nil {
				t.Fatal(err)
			}
			if l["t"] == "say" || l["t"] == "skip" || l["t"] == "rollback" {
				out = append(out, l)
			}
		}
		return out
	}
	// story is the say and skip lines of a run, one string each, in order.
	story := func(lines []map[string]any) string {
		var b strings.Builder
		for _, l := range lines {
			switch {
			case l["t"] == "rollback":
				fmt.Fprintf(&b, "back %v %v\n", l["from"], l["to"])
			case l["t"] == "skip":
				fmt.Fprintf(&b, "skip %v %v\n", l["on"], l["why"])
			case l["choices"] != nil:
				fmt.Fprintf(&b, "choice\n")
			default:
				fmt.Fprintf(&b, "say %q %v\n", l["text"], l["read"])
			}
		}
		return b.String()
	}
	first := run("first", "wait 30\npress ok\nwait 30\npress ok\nwait 30\nquit\n")
	if got, want := story(first), "say \"The lamps are lit.\" false\nsay \"The road is long.\" false\nsay \"A bell rings far off.\" false\n"; got != want {
		t.Fatalf("first run:\n%s\nwant:\n%s", got, want)
	}
	second := run("second", "wait 30\npress skip\nwait 60\nshot unread\npress ok\nwait 30\nquit\n")
	want := "say \"The lamps are lit.\" true\nskip true <nil>\nsay \"The road is long.\" true\nskip false unread\nsay \"A bell rings far off.\" false\nchoice\n"
	if got := story(second); got != want {
		t.Fatalf("second run:\n%s\nwant:\n%s", got, want)
	}
	third := run("third", "wait 30\npress skip\nwait 60\nshot choice\nquit\n")
	want = "say \"The lamps are lit.\" true\nskip true <nil>\nsay \"The road is long.\" true\nsay \"A bell rings far off.\" true\nskip false choice\nchoice\n"
	if got := story(third); got != want {
		t.Fatalf("third run:\n%s\nwant:\n%s", got, want)
	}
	// Rolling back (ADR 0025): back from the third line to the second, which is replayed from
	// the start of the event (the trace has the replayed lines, then the rollback line), then
	// on to the third again.
	fourth := run("fourth", "wait 30\npress ok\nwait 30\npress ok\nwait 30\npress back\nwait 30\nshot back\npress ok\nwait 30\nquit\n")
	want = "say \"The lamps are lit.\" true\nsay \"The road is long.\" true\nsay \"A bell rings far off.\" true\nsay \"The lamps are lit.\" true\nsay \"The road is long.\" true\nback 3 2\nsay \"A bell rings far off.\" true\n"
	if got := story(fourth); got != want {
		t.Fatalf("fourth run:\n%s\nwant:\n%s", got, want)
	}
	// Staging (ADR 0028): stand brings the figure in on the left, and it is gone after it leaves.
	run("fifth", "wait 40\nshot standing\npress ok\nwait 40\nshot left\nquit\n", "staged")
	magenta := func(name string) (left, right int) {
		b, err := os.ReadFile(filepath.Join(root, name+".png")) //nolint:gosec // A fixed name in this test's temporary directory.
		if err != nil {
			t.Fatal(err)
		}
		img, err := png.Decode(bytes.NewReader(b))
		if err != nil {
			t.Fatal(err)
		}
		w := img.Bounds().Dx()
		for y := 0; y < img.Bounds().Dy(); y += 2 {
			for x := 0; x < w; x += 2 {
				r, g, b, _ := img.At(x, y).RGBA()
				if r>>8 > 0xc0 && g>>8 < 0x40 && b>>8 > 0xc0 {
					if x < w/2 {
						left++
					} else {
						right++
					}
				}
			}
		}
		return left, right
	}
	if l, r := magenta("standing"); l < 200 || r > l/10 {
		t.Fatalf("the figure stood at the left: %d pixels on the left, %d on the right", l, r)
	}
	if l, r := magenta("left"); l+r > 0 {
		t.Fatalf("the figure that left is still drawn: %d pixels", l+r)
	}
	for _, name := range []string{"unread", "choice", "back"} {
		b, err := os.ReadFile(filepath.Join(root, name+".png")) //nolint:gosec // A fixed name in this test's temporary directory.
		if err != nil {
			t.Fatal(err)
		}
		img, err := png.Decode(bytes.NewReader(b))
		if err != nil {
			t.Fatal(err)
		}
		colors := map[uint32]bool{}
		for y := img.Bounds().Dy() * 2 / 3; y < img.Bounds().Dy(); y += 2 {
			for x := 0; x < img.Bounds().Dx(); x += 2 {
				r, g, b, _ := img.At(x, y).RGBA()
				colors[r>>8<<16|g>>8<<8|b>>8] = true
			}
		}
		if len(colors) < 4 {
			t.Fatalf("%s: the novel window shows nothing (%d colors)", name, len(colors))
		}
	}
}
