//go:build graphics

package play

import (
	"bytes"
	"context"
	"image"
	"image/png"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"golang.org/x/image/font/gofont/goregular"
)

// TestGraphicsCampaign plays the campaign of testdata/campaign (ADR 0030): its map shows a
// flag in the owner's colour on each region (blue on home, none of the player's on the free
// field), OK on the field lists its orders, Attack asks for the general and marches, and the
// field's flag turns blue; the script's end_turn ends the turn. The trace has the order and
// the end of the turn.
func TestGraphicsCampaign(t *testing.T) {
	root := t.TempDir()
	game := filepath.Join(root, "game")
	if err := os.CopyFS(game, os.DirFS("testdata/campaign")); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(game, "assets", "regular.ttf"), goregular.TTF, 0o600); err != nil {
		t.Fatal(err)
	}
	script := filepath.Join(root, "script.txt")
	body := "wait 40\nshot map\npress ok\nwait 20\nshot orders\npress ok\nwait 20\npress ok\nwait 30\npress ok\nwait 30\nshot taken\nend_turn\nwait 90\nshot turn\norder storm - keep\nwait 30\nshot sortie\npress cancel\nwait 20\nshot back\norder storm hero keep\nwait 80\nshot clash\nwait 900\nquit\n"
	if err := os.WriteFile(script, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	trace := filepath.Join(root, "run.jsonl")
	args := []string{
		"run", "./testdata/host", "--dir", game, "--dev", "--mute", "--seed", "7", "--start", "war",
		"--script", script, "--shots", root, "--save-dir", filepath.Join(root, "saves"), "--trace", trace,
	}
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "go", args...) //nolint:gosec // Fixed Go executable; arguments refer to the test fixture and isolated output.
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	if err := cmd.Run(); err != nil {
		t.Fatalf("game loop: %v\nstdout: %s\nstderr: %s", err, &stdout, &stderr)
	}
	tr, err := os.ReadFile(trace) //nolint:gosec // This test's own trace.
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{`"t":"region"`, `"t":"order"`, `"target":"field"`, `"result":"won"`, `"t":"end_turn"`, `"turn":2`, `"t":"clash"`, `"t":"clash_hit"`} {
		if !strings.Contains(string(tr), want) {
			t.Errorf("the trace has no %s", want)
		}
	}
	blue := func(name string, x, y float64) bool {
		return flagNear(t, root, name, x, y, func(r, g, b uint32) bool { return b > 0xd000 && b > r+0x6000 && b > g+0x5000 })
	}
	red := func(name string, x, y float64) bool {
		return flagNear(t, root, name, x, y, func(r, g, b uint32) bool { return r > 0xd000 && r > g+0x8000 && r > b+0x8000 })
	}
	switch {
	case !blue("map", 150, 260):
		t.Error("home has no blue flag beside its landmark")
	case !flagNear(t, root, "map", 100, 255, func(r, g, b uint32) bool { return r>>8 == 150 && g>>8 == 150 && b>>8 == 160 }):
		t.Error("home's landmark is not on its point")
	case !red("map", 520, 100):
		t.Error("keep has no red flag")
	case blue("map", 320, 180):
		t.Error("the free field has a blue flag before the march")
	case !blue("taken", 320, 180):
		t.Error("the field has no blue flag after the march")
	}
	if shotPixelSum(t, root, "turn") == shotPixelSum(t, root, "sortie") || shotPixelSum(t, root, "sortie") == shotPixelSum(t, root, "back") {
		t.Error("the sortie screen did not show, or did not close on cancel")
	}
	if shotPixelSum(t, root, "turn") == shotPixelSum(t, root, "clash") {
		t.Error("the clash screen did not show")
	}
	if shotPixelSum(t, root, "map") == shotPixelSum(t, root, "orders") {
		t.Error("OK on a region showed no orders")
	}
}

// flagNear reports whether a pixel within a flag's reach over the map point (x, y) of the
// 640x360 picture matches.
func flagNear(t *testing.T, root, name string, x, y float64, match func(r, g, b uint32) bool) bool {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(root, name+".png")) //nolint:gosec // A fixed name in this test's temporary directory.
	if err != nil {
		t.Fatal(err)
	}
	img, err := png.Decode(bytes.NewReader(b))
	if err != nil {
		t.Fatal(err)
	}
	bd := img.Bounds()
	k := float64(bd.Dx()) / 640
	cx, cy := int(x*k), int(y*k)
	h := int(0.06 * float64(bd.Dy()) * 1.4)
	for py := cy - h; py <= cy; py++ {
		for px := cx - h/3; px <= cx+h; px++ {
			if !(image.Point{px, py}.In(bd)) {
				continue
			}
			r, g, bl, _ := img.At(px, py).RGBA()
			if match(r, g, bl) {
				return true
			}
		}
	}
	return false
}
