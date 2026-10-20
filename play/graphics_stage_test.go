//go:build graphics

package play

import (
	"bytes"
	"context"
	"image"
	"image/png"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"golang.org/x/image/font/gofont/goregular"
)

// TestGraphicsStage walks the painted scene of testdata/stage (ADR 0022): the leader's doll
// (its top is magenta, from the face picture) shows at the start, is hidden behind the
// pillar's front, talks to the guide standing there (OK by them), and walking into the
// exit's area leaves for the next location. The click is in the game's logical units
// (720 high): the screen here is 360 high.
func TestGraphicsStage(t *testing.T) {
	root := t.TempDir()
	game := filepath.Join(root, "game")
	if err := os.CopyFS(game, os.DirFS("testdata/stage")); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(game, "assets", "regular.ttf"), goregular.TTF, 0o600); err != nil {
		t.Fatal(err)
	}
	script := filepath.Join(root, "script.txt")
	body := "wait 40\nshot open\npress reload\nwait 20\nshot reloaded\nwalk behind\nwait 10\nshot hidden\nwalk by_guide\npress z\nwait 30\nshot talk\npress z\nwait 10\nwalk open\nclick 960 492\nwait 150\npress z\nwait 20\nwalk open\nclick 1220 500\nwait 400\nshot left\nquit\n"
	if err := os.WriteFile(script, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	trace := filepath.Join(root, "run.jsonl")
	args := []string{"run", "./testdata/host", "--dir", game, "--dev", "--mute", "--seed", "7", "--start", "square",
		"--script", script, "--shots", root, "--save-dir", filepath.Join(root, "saves"), "--trace", trace}
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "go", args...) //nolint:gosec // Fixed Go executable; arguments refer to the test fixture and isolated output.
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	if err := cmd.Run(); err != nil {
		t.Fatalf("game loop: %v\nstdout: %s\nstderr: %s", err, &stdout, &stderr)
	}
	magenta := func(name string) int {
		b, err := os.ReadFile(filepath.Join(root, name+".png")) //nolint:gosec // A fixed name in this test's temporary directory.
		if err != nil {
			t.Fatal(err)
		}
		img, err := png.Decode(bytes.NewReader(b))
		if err != nil {
			t.Fatal(err)
		}
		return countMagenta(img)
	}
	if n := magenta("open"); n < 100 {
		t.Errorf("the doll does not show at the start (%d magenta pixels)", n)
	}
	// F5 with only the pictures to read again (the data unchanged): the scene makes its
	// textures again, the background and the doll show as before
	if n := magenta("reloaded"); n < 100 {
		t.Errorf("the doll is gone after F5 (%d magenta pixels)", n)
	}
	if a, b := shotPixel(t, root, "open", 600, 30), shotPixel(t, root, "reloaded", 600, 30); a != b {
		t.Errorf("the background after F5 is %v, before %v", b, a)
	}
	if n := magenta("hidden"); n != 0 {
		t.Errorf("the doll shows through the pillar (%d magenta pixels)", n)
	}
	tr, err := fs.ReadFile(os.DirFS(root), "run.jsonl")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{`"t":"spot","tick"`, `"spot":"behind"`, `"location":"room"`, `Hello there.`} {
		if !strings.Contains(string(tr), want) {
			t.Errorf("the trace has no %s", want)
		}
	}
	// a click on the guide's doll walks there and talks (the second time the line is said)
	if n := strings.Count(string(tr), `Hello there.`); n < 2 {
		t.Errorf("a click on the guide did not talk to them (%d lines)", n)
	}
}

// shotPixel is a pixel of a screenshot (the background's sky at the top right).
func shotPixel(t *testing.T, root, name string, x, y int) [4]uint32 {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(root, name+".png")) //nolint:gosec // A fixed name in this test's temporary directory.
	if err != nil {
		t.Fatal(err)
	}
	img, err := png.Decode(bytes.NewReader(b))
	if err != nil {
		t.Fatal(err)
	}
	r, g, bl, a := img.At(x, y).RGBA()
	return [4]uint32{r, g, bl, a}
}

func countMagenta(img image.Image) int {
	n := 0
	b := img.Bounds()
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			r, g, bl, _ := img.At(x, y).RGBA()
			if r>>8 > 120 && g>>8 < 50 && bl>>8 > 120 {
				n++
			}
		}
	}
	return n
}

// TestGraphicsMap walks the map of testdata/stage (ADR 0022): the token sets off along the
// roads to the door (a travel in the trace), the closed point only says why, and the door
// leads into the room.
func TestGraphicsMap(t *testing.T) {
	root := t.TempDir()
	game := filepath.Join(root, "game")
	if err := os.CopyFS(game, os.DirFS("testdata/stage")); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(game, "assets", "regular.ttf"), goregular.TTF, 0o600); err != nil {
		t.Fatal(err)
	}
	script := filepath.Join(root, "script.txt")
	body := "wait 30\nshot map\nwalk shut\nwait 30\npress z\nwait 20\nwalk door\nwait 30\nshot walking\nwait 700\nshot room\nquit\n"
	if err := os.WriteFile(script, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	trace := filepath.Join(root, "run.jsonl")
	args := []string{"run", "./testdata/host", "--dir", game, "--dev", "--mute", "--seed", "7", "--start", "town",
		"--script", script, "--shots", root, "--save-dir", filepath.Join(root, "saves"), "--trace", trace}
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "go", args...) //nolint:gosec // Fixed Go executable; arguments refer to the test fixture and isolated output.
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	if err := cmd.Run(); err != nil {
		t.Fatalf("game loop: %v\nstdout: %s\nstderr: %s", err, &stdout, &stderr)
	}
	tr, err := fs.ReadFile(os.DirFS(root), "run.jsonl")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{`"stack":["atlas"]`, `"t":"travel"`, `"to":"door"`, `"location":"room"`, `Shut.`} {
		if !strings.Contains(string(tr), want) {
			t.Errorf("the trace has no %s", want)
		}
	}
	// the token moved between the two shots (the screens differ)
	if shotPixelSum(t, root, "map") == shotPixelSum(t, root, "walking") {
		t.Error("the token did not move")
	}
}

func shotPixelSum(t *testing.T, root, name string) uint64 {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(root, name+".png")) //nolint:gosec // A fixed name in this test's temporary directory.
	if err != nil {
		t.Fatal(err)
	}
	img, err := png.Decode(bytes.NewReader(b))
	if err != nil {
		t.Fatal(err)
	}
	var sum uint64
	bd := img.Bounds()
	for y := bd.Min.Y; y < bd.Max.Y; y += 2 {
		for x := bd.Min.X; x < bd.Max.X; x += 2 {
			r, g, b, _ := img.At(x, y).RGBA()
			sum += uint64(r)*uint64(x+1) + uint64(g)*uint64(y+1) + uint64(b)
		}
	}
	return sum
}

// TestGraphicsWild walks into a map in the map (at_point), whose roads are drawn and whose
// unvisited points are hidden, with a token picture: the token shows, an open road is drawn
// light, the hidden point is not in the list, and the road's battle comes before the token
// reaches the ruin.
func TestGraphicsWild(t *testing.T) {
	root := t.TempDir()
	game := filepath.Join(root, "game")
	if err := os.CopyFS(game, os.DirFS("testdata/stage")); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(game, "assets", "regular.ttf"), goregular.TTF, 0o600); err != nil {
		t.Fatal(err)
	}
	script := filepath.Join(root, "script.txt")
	body := "wait 30\nwalk pass\nwait 200\nshot wild\npress menu\nwait 20\nshot list\npress cancel\nwait 20\nwalk ruin\nwait 30\nuntil\nwait 200\nshot ruin\nquit\n"
	if err := os.WriteFile(script, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	args := []string{"run", "./testdata/host", "--dir", game, "--dev", "--mute", "--seed", "7", "--start", "town",
		"--script", script, "--shots", root, "--save-dir", filepath.Join(root, "saves"), "--trace", filepath.Join(root, "run.jsonl")}
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "go", args...) //nolint:gosec // Fixed Go executable; arguments refer to the test fixture and isolated output.
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	if err := cmd.Run(); err != nil {
		t.Fatalf("game loop: %v\nstdout: %s\nstderr: %s", err, &stdout, &stderr)
	}
	b, err := os.ReadFile(filepath.Join(root, "run.jsonl")) //nolint:gosec // A fixed name in this test's temporary directory.
	if err != nil {
		t.Fatal(err)
	}
	tr := string(b)
	for _, want := range []string{`"location":"wild","spot":"gate"`, `→ Ruin`} {
		if !strings.Contains(tr, want) {
			t.Errorf("the trace has no %s", want)
		}
	}
	if strings.Contains(tr, "→ Cave") {
		t.Error("a hidden point is in the list")
	}
	battle, room := strings.Index(tr, `"t":"battle"`), strings.Index(tr, `"location":"room"`)
	if battle < 0 || room < 0 || battle > room {
		t.Errorf("the road's battle (%d) does not come before the ruin (%d)", battle, room)
	}
	// the token picture (pure red) shows; the open road gate-ruin is drawn light at its middle
	img := shotImage(t, root, "wild")
	red := 0
	bd := img.Bounds()
	for y := bd.Min.Y; y < bd.Max.Y; y++ {
		for x := bd.Min.X; x < bd.Max.X; x++ {
			if r, g, b, _ := img.At(x, y).RGBA(); r>>8 > 230 && g>>8 < 30 && b>>8 < 30 {
				red++
			}
		}
	}
	if red < 50 {
		t.Errorf("the token picture does not show (%d red pixels)", red)
	}
	if r, g, b, _ := img.At(320, 320).RGBA(); r>>8 < 200 || g>>8 < 200 || b>>8 < 190 { // (160, 160) in the picture, x2
		t.Errorf("the open road is not drawn at its middle: %v %v %v", r>>8, g>>8, b>>8)
	}
}

func shotImage(t *testing.T, root, name string) image.Image {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(root, name+".png")) //nolint:gosec // A fixed name in this test's temporary directory.
	if err != nil {
		t.Fatal(err)
	}
	img, err := png.Decode(bytes.NewReader(b))
	if err != nil {
		t.Fatal(err)
	}
	return img
}
