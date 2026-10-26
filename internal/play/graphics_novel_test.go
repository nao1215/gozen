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
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"golang.org/x/image/font/gofont/goregular"

	"github.com/nao1215/gozen/internal/testgame"
)

func TestGraphicsNovelHeadFraming(t *testing.T) {
	t.Parallel()
	for _, width := range []int{1280, 960} {
		t.Run(strconv.Itoa(width), func(t *testing.T) {
			t.Parallel()
			root := t.TempDir()
			game, saves := filepath.Join(root, "game"), filepath.Join(root, "saves")
			if err := os.CopyFS(game, gameFiles(t)); err != nil {
				t.Fatal(err)
			}
			write := func(path string, b []byte) {
				t.Helper()
				if err := os.WriteFile(filepath.Join(game, path), b, 0o600); err != nil {
					t.Fatal(err)
				}
			}
			var framing strings.Builder
			for _, spec := range []struct {
				name   string
				w, h   int
				dx, dy int
			}{{"tight", 200, 350, 0, 0}, {"padded", 1600, 900, 600, 180}, {"changed", 800, 800, 200, 60}} {
				img := image.NewNRGBA(image.Rect(0, 0, spec.w, spec.h))
				draw.Draw(img, image.Rect(50+spec.dx, 90+spec.dy, 110+spec.dx, 340+spec.dy), &image.Uniform{color.NRGBA{R: 50, G: 80, B: 90, A: 255}}, image.Point{}, draw.Src)
				for y := 30; y < 90; y++ {
					for x := 50; x < 110; x++ {
						if (x-80)*(x-80)+(y-60)*(y-60) < 30*30 {
							head := color.NRGBA{R: 255, B: 255, A: 255}
							if spec.name == "changed" {
								head.G, head.B = 255, 0
							}
							img.SetNRGBA(x+spec.dx, y+spec.dy, head)
						}
					}
				}
				var b bytes.Buffer
				if err := png.Encode(&b, img); err != nil {
					t.Fatal(err)
				}
				write("assets/"+spec.name+".png", b.Bytes())
				fmt.Fprintf(&framing, "    %s.png: {head: [%g, %g, %g], height: 0.18, top: 0.1}\n", spec.name, float64(80+spec.dx)/float64(spec.w), float64(30+spec.dy)/float64(spec.h), 60/float64(spec.h))
			}
			write("game.yaml", []byte(fmt.Sprintf("schema_version: 8\nid: framing\n%s\nfonts: {regular: test.ttf}\nscreen: {width: %d, height: 720}\nui:\n  message: novel\n  stage_scale: {tight.png: 1.25, padded.png: 1.25, changed.png: 1.25}\n  stage_drop: {tight.png: 0.02, padded.png: 0.02, changed.png: 0.02}\n  stage_framing:\n%s", testgame.Game, width, &framing)))
			write("data/framing.yaml", []byte(`events:
  framing:
    do:
      - stand: {who: Left, face: tight.png, at: 0.25}
      - stand: {who: Right, face: padded.png, at: 0.75}
      - say: {who: Left, face: tight.png, text: "Compare the heads."}
      - stand: {who: Left, face: changed.png, at: 0.25}
      - say: {who: Left, face: changed.png, text: "A different expression canvas."}
  mirrored:
    do:
      - say: {who: Left, face: tight.png, side: left, text: "Automatic placement."}
      - say: {who: Right, face: padded.png, side: right, text: "A mirrored head anchor."}
`))
			run := func(name, script string, extra ...string) {
				t.Helper()
				path := filepath.Join(root, name+".txt")
				if err := os.WriteFile(path, []byte(script), 0o600); err != nil {
					t.Fatal(err)
				}
				ctx, cancel := context.WithTimeout(t.Context(), 2*time.Minute)
				defer cancel()
				args := []string{"--dir", game, "--dev", "--mute", "--seed", "7", "--script", path, "--shots", root, "--save-dir", saves}
				args = append(args, extra...)
				cmd := exec.CommandContext(ctx, hostBin, args...) //nolint:gosec // Fixed test host and isolated fixture paths.
				var stdout, stderr bytes.Buffer
				cmd.Stdout, cmd.Stderr = &stdout, &stderr
				if err := cmd.Run(); err != nil {
					t.Fatalf("%s: %v\nstdout: %s\nstderr: %s", name, err, &stdout, &stderr)
				}
			}
			run("initial", "wait 40\nshot initial\npress save\nwait 20\npress ok\nwait 100\npress ok\nwait 40\nshot changed\npress reload\nwait 50\nshot reload\nquit\n", "--start", "camp", "--event", "framing")
			run("load", "wait 20\npress ok\nwait 20\npress ok\nwait 50\nshot loaded\nquit\n")
			run("mirrored", "wait 40\npress ok\nwait 40\nshot mirrored\nquit\n", "--start", "camp", "--event", "mirrored")
			for _, shot := range []string{"initial", "changed", "reload", "loaded", "mirrored"} {
				img := shotImage(t, root, shot)
				for side := range 2 {
					box := image.Rectangle{}
					for y := range img.Bounds().Dy() / 2 {
						for x := side * width / 2; x < (side+1)*width/2; x++ {
							r, g, b, _ := img.At(x, y).RGBA()
							match := r > 20000 && b > 20000 && g < 10000
							if side == 0 && (shot == "changed" || shot == "reload") {
								match = r > 20000 && g > 20000 && b < 10000
							}
							if match {
								box = box.Union(image.Rect(x, y, x+1, y+1))
							}
						}
					}
					cx := float64(box.Min.X+box.Max.X) / 2
					wantX := float64(width) * (.25 + .5*float64(side))
					if shot == "mirrored" {
						wantX = float64(width) * (.3 + .4*float64(side))
					}
					if box.Empty() || math.Abs(cx-wantX) > 3 || abs(box.Min.Y-86) > 4 || abs(box.Dy()-162) > 4 || abs(box.Dx()-162) > 4 {
						t.Fatalf("%s side %d: head %v, want center %g, top 86, size 162", shot, side, box, wantX)
					}
				}
			}
		})
	}
}

// TestGraphicsNovelStagePuppets stands two actors who share a still puppet (no breathing or
// sway) and compares the stage with ui.stage_puppets to the stage of the face pictures: the
// heads stand in the same frames, only the speaker's figure shows its green mouth frame, and
// a line's face turns the head yellow.
func TestGraphicsNovelStagePuppets(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	game := filepath.Join(root, "game")
	if err := os.CopyFS(game, gameFiles(t)); err != nil {
		t.Fatal(err)
	}
	write := func(path string, b []byte) {
		t.Helper()
		if err := os.MkdirAll(filepath.Dir(filepath.Join(game, path)), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(game, path), b, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	magenta, yellow := color.NRGBA{R: 255, B: 255, A: 255}, color.NRGBA{R: 255, G: 255, A: 255}
	green := color.NRGBA{G: 220, A: 255}
	type part struct {
		r image.Rectangle
		c color.NRGBA
	}
	picture := func(w, h int, parts ...part) []byte {
		img := image.NewNRGBA(image.Rect(0, 0, w, h))
		for _, pt := range parts {
			r, c := pt.r, pt.c
			for y := r.Min.Y; y < r.Max.Y; y++ {
				for x := r.Min.X; x < r.Max.X; x++ {
					if r.Dx() == r.Dy() && (2*x-r.Min.X-r.Max.X+1)*(2*x-r.Min.X-r.Max.X+1)+(2*y-r.Min.Y-r.Max.Y+1)*(2*y-r.Min.Y-r.Max.Y+1) > r.Dx()*r.Dx() {
						continue // a square is drawn as its circle
					}
					img.SetNRGBA(x, y, c)
				}
			}
		}
		var b bytes.Buffer
		if err := png.Encode(&b, img); err != nil {
			t.Fatal(err)
		}
		return b.Bytes()
	}
	body, head := image.Rect(90, 130, 210, 600), image.Rect(100, 30, 200, 130)
	gray := color.NRGBA{R: 90, G: 100, B: 110, A: 255}
	write("assets/fig/normal.png", picture(300, 600, part{body, gray}, part{head, magenta}))
	write("assets/fig/glad.png", picture(300, 600, part{body, gray}, part{head, yellow}))
	write("assets/fig/mouth.png", picture(30, 10, part{image.Rect(0, 0, 30, 10), green}))
	write("assets/fig/puppet.yaml", []byte(`base: normal.png
face: normal
fade: 0.2
faces:
  normal: {}
  glad: {image: glad.png}
mouth:
  frames: [{image: mouth.png, at: [135, 100]}, face]
  speed: 12
motion: {breath: {depth: 0}}
`))
	write("data/stage_puppets.yaml", []byte(`actors:
  lefty: {name: Lefty, faces: fig, face: normal, puppet: fig/puppet.yaml}
  righty: {name: Righty, faces: fig, face: normal, puppet: fig/puppet.yaml}
events:
  scene:
    do:
      - stand: {who: lefty, face: normal, at: 0.25}
      - stand: {who: righty, face: normal, at: 0.75}
      - say: {who: lefty, text: "Hello."}
      - say: {who: lefty, text: "A long line that keeps the text typing for a while, so that the mouth of the speaker has time to open and close many times over."}
      - say: {who: lefty, face: glad, text: "Glad."}
`))
	frame := "{head: [0.5, 0.05, 0.16667], height: 0.2, top: 0.1}"
	var shots []string
	script := "wait 20\npress ok\nwait 2\n" // --event shows its first line whole; the second types
	for i := range 8 {
		shots = append(shots, fmt.Sprintf("talk%d", i))
		script += fmt.Sprintf("shot talk%d\nwait 1\n", i)
	}
	script += "wait 200\npress ok\nwait 60\nshot glad\nquit\n"
	run := func(name, ui string) {
		t.Helper()
		write("game.yaml", []byte(fmt.Sprintf("schema_version: 8\nid: stage-puppets\n%s\nfonts: {regular: test.ttf}\nscreen: {width: 1280, height: 720}\nui:\n  message: novel\n%s  stage_framing: {fig/normal.png: %s, fig/glad.png: %s}\n", testgame.Game, ui, frame, frame)))
		path := filepath.Join(root, name+".txt")
		if err := os.WriteFile(path, []byte(strings.ReplaceAll(script, "shot ", "shot "+name+"-")), 0o600); err != nil {
			t.Fatal(err)
		}
		ctx, cancel := context.WithTimeout(t.Context(), 2*time.Minute)
		defer cancel()
		cmd := exec.CommandContext(ctx, hostBin, "--dir", game, "--dev", "--mute", "--seed", "7", "--start", "camp", "--event", "scene", "--script", path, "--shots", root, "--save-dir", filepath.Join(root, name+"-saves")) //nolint:gosec // Fixed test host and isolated fixture paths.
		var stdout, stderr bytes.Buffer
		cmd.Stdout, cmd.Stderr = &stdout, &stderr
		if err := cmd.Run(); err != nil {
			t.Fatalf("%s: %v\nstdout: %s\nstderr: %s", name, err, &stdout, &stderr)
		}
	}
	run("puppet", "  stage_puppets: true\n")
	run("picture", "")
	// box is the bounding box of the pixels like c in one half of the screen, above the window.
	box := func(img image.Image, side int, like func(r, g, b uint32) bool) image.Rectangle {
		out := image.Rectangle{}
		for y := range 470 {
			for x := side * 640; x < (side+1)*640; x++ {
				r, g, b, _ := img.At(x, y).RGBA()
				if like(r>>8, g>>8, b>>8) {
					out = out.Union(image.Rect(x, y, x+1, y+1))
				}
			}
		}
		return out
	}
	isMagenta := func(r, g, b uint32) bool { return r > 120 && b > 120 && g < 40 }
	isGreen := func(r, g, b uint32) bool { return g > 120 && r < 60 && b < 60 }
	isYellow := func(r, g, b uint32) bool { return r > 180 && g > 180 && b < 60 }
	talked := false
	for _, shot := range shots {
		p, still := shotImage(t, root, "puppet-"+shot), shotImage(t, root, "picture-"+shot)
		for side := range 2 {
			a, b := box(p, side, isMagenta), box(still, side, isMagenta)
			if a.Empty() || b.Empty() || abs(a.Min.X-b.Min.X) > 2 || abs(a.Min.Y-b.Min.Y) > 2 || abs(a.Dx()-b.Dx()) > 2 || abs(a.Dy()-b.Dy()) > 2 {
				t.Fatalf("%s side %d: the puppet's head %v, the picture's %v", shot, side, a, b)
			}
		}
		talked = talked || !box(p, 0, isGreen).Empty()
		if m := box(p, 1, isGreen); !m.Empty() {
			t.Fatalf("%s: the figure who does not speak opened its mouth at %v", shot, m)
		}
		if m := box(still, 0, isGreen); !m.Empty() {
			t.Fatalf("%s: the still picture has a mouth frame at %v", shot, m)
		}
	}
	if !talked {
		t.Fatal("the speaker's mouth never opened while the text was typing")
	}
	glad := shotImage(t, root, "puppet-glad")
	if box(glad, 0, isYellow).Empty() || !box(glad, 1, isYellow).Empty() || box(glad, 1, isMagenta).Empty() {
		t.Fatal("the line's face did not turn only the speaker's head yellow")
	}
}

// TestGraphicsNovel plays testdata/novel six times with one save directory (ADR 0026).
// The first run reads two lines. In the second, skip mode passes them and stops before the
// third line, which was not read; the player reads it. In the third, skip mode passes the
// three lines and stops before the choice. In the fourth, rolling back from the third line
// shows the second, and going on shows the third again. The fifth stands a figure with stand
// and lets it leave (ADR 0028). The sixth opens the F8 panel and jumps from it (ADR 0029).
// read.json carries the read lines between the runs.
func TestGraphicsNovel(t *testing.T) {
	t.Parallel()
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
			"--dir", game, "--dev", "--mute", "--seed", "7", "--start", "room", "--event", append(event, "story")[0],
			"--script", script, "--shots", root, "--save-dir", saves, "--trace", trace,
		}
		ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
		defer cancel()
		cmd := exec.CommandContext(ctx, hostBin, args...) //nolint:gosec // The host built by TestMain; arguments refer to the test fixture and isolated output.
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
			if l["t"] == "say" || l["t"] == "skip" || l["t"] == "rollback" || l["t"] == "jump" {
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
			case l["t"] == "jump":
				fmt.Fprintf(&b, "jump %v\n", l["event"])
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
	// The development panel (ADR 0029): F8 opens it on the state, the flags tab draws its
	// table, and the scenes tab jumps to the staged event, whose figure then stands.
	sixth := run("sixth", "wait 30\npress panel\nwait 10\nshot panel\npress right\nwait 10\nshot flagtable\npress right\nwait 5\npress down\nwait 5\npress ok\nwait 40\nshot jumped\nquit\n")
	if got := story(sixth); !strings.Contains(got, "jump staged\n") {
		t.Fatalf("sixth run:\n%s", got)
	}
	if l, _ := magenta("jumped"); l < 200 {
		t.Fatalf("the jump did not reach the staged figure: %d pixels", l)
	}
	for _, name := range []string{"unread", "choice", "back", "panel", "flagtable"} {
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
	// A long fade must not leave the first picture visible while read lines skip.
	for name, c := range map[string]color.NRGBA{
		"red": {R: 255, A: 255}, "blue": {B: 255, A: 255},
	} {
		img := image.NewNRGBA(image.Rect(0, 0, 64, 64))
		draw.Draw(img, img.Bounds(), &image.Uniform{c}, image.Point{}, draw.Src)
		var buf bytes.Buffer
		if err := png.Encode(&buf, img); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(game, "assets", name+".png"), buf.Bytes(), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	const pictures = `events:
  picture_skip:
    do:
      - image: {file: red.png, fit: cover, fade: 0}
      - say: "Red."
      - image: {file: blue.png, fit: cover, fade: 5}
      - say: "Blue."
`
	if err := os.WriteFile(filepath.Join(game, "data", "picture_skip.yaml"), []byte(pictures), 0o600); err != nil {
		t.Fatal(err)
	}
	run("pictures-read", "wait 30\npress ok\nwait 30\npress ok\nwait 5\nquit\n", "picture_skip")
	run("pictures-skip", "wait 30\npress skip\nwait 8\nshot skipped-picture\nquit\n", "picture_skip")
	raw, err := os.ReadFile(filepath.Join(root, "skipped-picture.png")) //nolint:gosec // A fixed screenshot name in the isolated test directory.
	if err != nil {
		t.Fatal(err)
	}
	img, err := png.Decode(bytes.NewReader(raw))
	if err != nil {
		t.Fatal(err)
	}
	r, _, blue, _ := img.At(img.Bounds().Dx()/2, 30).RGBA()
	if blue>>8 < 240 || r>>8 > 15 {
		t.Fatalf("skipping retained the red picture: red=%d blue=%d", r>>8, blue>>8)
	}
}
