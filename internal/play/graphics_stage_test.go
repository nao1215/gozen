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
	t.Parallel()
	for _, fast := range []bool{false, true} {
		t.Run(map[bool]string{false: "default", true: "double"}[fast], func(t *testing.T) {
			t.Parallel()
			root := t.TempDir()
			game := filepath.Join(root, "game")
			if err := os.CopyFS(game, os.DirFS("testdata/stage")); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(game, "assets", "regular.ttf"), goregular.TTF, 0o600); err != nil {
				t.Fatal(err)
			}
			if fast {
				path := filepath.Join(game, "data", "main.yaml")
				b, err := os.ReadFile(path) //nolint:gosec // Path is a fixture inside this test's private temporary directory.
				if err != nil {
					t.Fatal(err)
				}
				b = bytes.ReplaceAll(b, []byte("    stage:"), []byte("    stage:\n      walk_speed: 3.2"))
				if err := os.WriteFile(path, b, 0o600); err != nil { //nolint:gosec // Path is a fixture inside this test's private temporary directory.
					t.Fatal(err)
				}
			}
			script := filepath.Join(root, "script.txt")
			body := "wait 40\nshot open\npress reload\nwait 20\nshot reloaded\nwalk behind\nwait 10\nshot hidden\nwalk by_guide\npress z\nwait 30\nshot talk\npress z\nwait 10\nwalk open\nclick 960 492\nwait 150\npress z\nwait 20\nwalk open\nclick 1220 500\nwait 400\nshot left\nquit\n"
			if err := os.WriteFile(script, []byte(body), 0o600); err != nil {
				t.Fatal(err)
			}
			trace := filepath.Join(root, "run.jsonl")
			args := []string{
				"--dir", game, "--dev", "--mute", "--seed", "7", "--start", "square",
				"--script", script, "--shots", root, "--save-dir", filepath.Join(root, "saves"), "--trace", trace,
			}
			ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
			defer cancel()
			args = append(args, "--build-info")
			cmd := exec.CommandContext(ctx, hostBin, args...) //nolint:gosec // The host built by TestMain; arguments refer to the test fixture and isolated output.
			cmd.Env = append(os.Environ(), "GOZEN_DEBUG=1")
			started := time.Now()
			var stdout, stderr bytes.Buffer
			cmd.Stdout, cmd.Stderr = &stdout, &stderr
			if err := cmd.Run(); err != nil {
				t.Fatalf("game loop: %v\nstdout: %s\nstderr: %s", err, &stdout, &stderr)
			}
			logGraphicsTiming(t, started, stderr.String())
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
		})
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
	t.Parallel()
	for _, fast := range []bool{false, true} {
		t.Run(map[bool]string{false: "default", true: "double"}[fast], func(t *testing.T) {
			t.Parallel()
			root := t.TempDir()
			game := filepath.Join(root, "game")
			if err := os.CopyFS(game, os.DirFS("testdata/stage")); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(game, "assets", "regular.ttf"), goregular.TTF, 0o600); err != nil {
				t.Fatal(err)
			}
			if fast {
				path := filepath.Join(game, "data", "main.yaml")
				b, err := os.ReadFile(path) //nolint:gosec // Path is a fixture inside this test's private temporary directory.
				if err != nil {
					t.Fatal(err)
				}
				b = bytes.ReplaceAll(b, []byte("    map:"), []byte("    map:\n      travel_speed: 0.24"))
				if err := os.WriteFile(path, b, 0o600); err != nil { //nolint:gosec // Path is a fixture inside this test's private temporary directory.
					t.Fatal(err)
				}
			}
			script := filepath.Join(root, "script.txt")
			body := "wait 30\nshot map\nwalk shut\nwait 30\npress z\nwait 20\nwalk door\nwait 30\nshot walking\nwait 700\nshot room\nquit\n"
			if err := os.WriteFile(script, []byte(body), 0o600); err != nil {
				t.Fatal(err)
			}
			trace := filepath.Join(root, "run.jsonl")
			args := []string{
				"--dir", game, "--dev", "--mute", "--seed", "7", "--start", "town",
				"--script", script, "--shots", root, "--save-dir", filepath.Join(root, "saves"), "--trace", trace,
			}
			ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
			defer cancel()
			args = append(args, "--build-info")
			cmd := exec.CommandContext(ctx, hostBin, args...) //nolint:gosec // The host built by TestMain; arguments refer to the test fixture and isolated output.
			cmd.Env = append(os.Environ(), "GOZEN_DEBUG=1")
			started := time.Now()
			var stdout, stderr bytes.Buffer
			cmd.Stdout, cmd.Stderr = &stdout, &stderr
			if err := cmd.Run(); err != nil {
				t.Fatalf("game loop: %v\nstdout: %s\nstderr: %s", err, &stdout, &stderr)
			}
			logGraphicsTiming(t, started, stderr.String())
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
		})
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
	// weigh each sample by its column and row, counted from 1, so a moved token changes the sum
	var sum, wy uint64
	bd := img.Bounds()
	for y := bd.Min.Y; y < bd.Max.Y; y += 2 {
		wy++
		var wx uint64
		for x := bd.Min.X; x < bd.Max.X; x += 2 {
			wx++
			r, g, b, _ := img.At(x, y).RGBA()
			sum += uint64(r)*wx + uint64(g)*wy + uint64(b)
		}
	}
	return sum
}

// TestGraphicsWild walks into a map in the map (at_point), whose roads are drawn and whose
// unvisited points are hidden, with a token picture: the token shows, an open road is drawn
// light, the hidden point is not in the list, and the road's battle comes before the token
// reaches the ruin.
func TestGraphicsWild(t *testing.T) {
	t.Parallel()
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
	args := []string{
		"--dir", game, "--dev", "--mute", "--seed", "7", "--start", "town",
		"--script", script, "--shots", root, "--save-dir", filepath.Join(root, "saves"), "--trace", filepath.Join(root, "run.jsonl"),
	}
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, hostBin, args...) //nolint:gosec // The host built by TestMain; arguments refer to the test fixture and isolated output.
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

// The camera is exercised through native input and pixel readback; the grid is a
// neutral test asset, not a README illustration.
func TestGraphicsMapCamera(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	game := filepath.Join(root, "game")
	if err := os.CopyFS(game, atlasCameraFiles(t)); err != nil {
		t.Fatal(err)
	}
	path, trace := filepath.Join(root, "script.txt"), filepath.Join(root, "trace.jsonl")
	body := "gap 1\nwait 20\nshot focused\npress map_overview\nwait 5\nshot overview\npress map_focus\npress map_pan\nhold right 200\nhold down 200\nshot corner\npress map_focus\nwait 5\nshot returned\npress tab 58\nwait 5\nshot selected\nwalk p59\nwait 900\nshot arrived\nquit\n"
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(t.Context(), 2*time.Minute)
	defer cancel()
	cmd := exec.CommandContext(ctx, hostBin, "--dir", game, "--dev", "--mute", "--start", "large", "--seed", "1", "--script", path, "--trace", trace, "--shots", root, "--save-dir", filepath.Join(root, "saves")) //nolint:gosec // Fixed test host and isolated fixture paths.
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	if err := cmd.Run(); err != nil {
		t.Fatalf("host: %v\nstdout: %s\nstderr: %s", err, &stdout, &stderr)
	}
	// The background's gradient at an unobstructed central pixel identifies the
	// world coordinates being rendered, independently of marker layout.
	for _, tc := range []struct {
		name string
		r, g uint32
	}{
		{"focused", 38, 70}, {"returned", 38, 70}, {"overview", 55, 90}, {"corner", 71, 110},
	} {
		img := shotImage(t, root, tc.name)
		bounds := img.Bounds()
		r, g, b, _ := img.At(bounds.Dx()/2, bounds.Dy()/2).RGBA()
		if abs(int(r>>8)-int(tc.r)) > 2 || abs(int(g>>8)-int(tc.g)) > 2 || abs(int(b>>8)-60) > 2 {
			t.Errorf("%s center RGB %d,%d,%d", tc.name, r>>8, g>>8, b>>8)
		}
	}
	if shotPixelSum(t, root, "returned") == shotPixelSum(t, root, "selected") {
		t.Fatal("offscreen selection did not move camera")
	}
	raw, err := fs.ReadFile(os.DirFS(root), "trace.jsonl")
	if err != nil {
		t.Fatal(err)
	}
	trips := traceLines(t, raw, "travel")
	if len(trips) != 1 || trips[0]["to"] != "p59" {
		t.Fatalf("browsing triggered travel: %v", trips)
	}
	spots := traceLines(t, raw, "spot")
	if len(spots) == 0 || spots[len(spots)-1]["spot"] != "p59" {
		t.Fatal("journey did not finish")
	}
}

func TestGraphicsMapSelectionPanel(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	game := filepath.Join(root, "game")
	if err := os.CopyFS(game, atlasPanelFiles(t)); err != nil {
		t.Fatal(err)
	}
	script := filepath.Join(root, "script.txt")
	trace := filepath.Join(root, "trace.jsonl")
	body := "gap 1\nwait 20\nshot advice\npress map_zoom_in\npress map_pan\nhold right 20\nshot panned\npress cancel\nshot hidden\npress ok\nshot reopened\npress ok\nwait 60\nshot activated\nquit\n"
	if err := os.WriteFile(script, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(t.Context(), 2*time.Minute)
	defer cancel()
	cmd := exec.CommandContext(ctx, hostBin, "--dir", game, "--dev", "--mute", "--start", "selection", "--seed", "1", "--script", script, "--trace", trace, "--shots", root, "--save-dir", filepath.Join(root, "saves")) //nolint:gosec // Fixed test host and isolated fixture paths.
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	if err := cmd.Run(); err != nil {
		t.Fatalf("host: %v\nstdout: %s\nstderr: %s", err, &stdout, &stderr)
	}
	// The portrait and heading remain at identical screen coordinates while the
	// map pans. The panel is opaque except for its rounded frame.
	first := shotImage(t, root, "advice")
	for _, name := range []string{"panned", "reopened", "activated"} {
		img := shotImage(t, root, name)
		for y := 90; y < 170; y++ {
			for x := 850; x < 1200; x++ {
				if first.At(x, y) != img.At(x, y) {
					t.Fatalf("%s moved or changed fixed portrait/header at %d,%d", name, x, y)
				}
			}
		}
	}
	if shotPixelSum(t, root, "hidden") == shotPixelSum(t, root, "reopened") {
		t.Fatal("cancel did not dismiss the panel")
	}
	raw, err := fs.ReadFile(os.DirFS(root), "trace.jsonl")
	if err != nil {
		t.Fatal(err)
	}
	actions := traceLines(t, raw, "activate")
	if len(actions) != 1 || actions[0]["to"] != "quarry" || len(traceLines(t, raw, "travel")) != 0 {
		t.Fatalf("browsing activated a destination: %v", actions)
	}
}
