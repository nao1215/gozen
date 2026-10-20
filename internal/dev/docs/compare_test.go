package docs

import (
	"fmt"
	goimage "image"
	"image/color"
	"image/color/palette"
	"image/gif"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func solid(w, h int, c color.RGBA) *goimage.RGBA {
	img := goimage.NewRGBA(goimage.Rect(0, 0, w, h))
	for i := 0; i < len(img.Pix); i += 4 {
		img.Pix[i], img.Pix[i+1], img.Pix[i+2], img.Pix[i+3] = c.R, c.G, c.B, c.A
	}
	return img
}

// Renderer and JPEG noise pass; a changed area or a changed frame count is stale.
func TestCompareFrames(t *testing.T) {
	base := solid(100, 100, color.RGBA{100, 120, 140, 255})
	noisy := solid(100, 100, color.RGBA{110, 112, 150, 255}) // every pixel a little off
	changed := solid(100, 100, color.RGBA{100, 120, 140, 255})
	for y := range 20 {
		for x := range 20 {
			changed.SetRGBA(x, y, color.RGBA{250, 10, 10, 255}) // 4% of the picture
		}
	}
	few := solid(100, 100, color.RGBA{100, 120, 140, 255})
	few.SetRGBA(1, 1, color.RGBA{0, 0, 0, 255})
	if err := compareFrames([]goimage.Image{base}, []goimage.Image{noisy}, 0.02); err != nil {
		t.Errorf("noise was stale: %v", err)
	}
	if err := compareFrames([]goimage.Image{base}, []goimage.Image{few}, 0.02); err != nil {
		t.Errorf("one pixel was stale: %v", err)
	}
	if err := compareFrames([]goimage.Image{base}, []goimage.Image{changed}, 0.02); err == nil || !strings.Contains(err.Error(), "4.0%") {
		t.Errorf("a changed area: %v", err)
	}
	if err := compareFrames([]goimage.Image{base, base}, []goimage.Image{base}, 0.02); err == nil || !strings.Contains(err.Error(), "2 frames, now 1") {
		t.Errorf("a frame count: %v", err)
	}
	if err := compareFrames([]goimage.Image{base}, []goimage.Image{solid(50, 100, color.RGBA{})}, 0.02); err == nil || !strings.Contains(err.Error(), "100x100, now 50x100") {
		t.Errorf("a size: %v", err)
	}
	// Only the representative frames of an animation are compared: the last one too.
	anim := []goimage.Image{base, base, base, base, base, base, base, base, base}
	moved := append(append([]goimage.Image{}, anim[:8]...), changed)
	if err := compareFrames(anim, moved, 0.02); err == nil || !strings.Contains(err.Error(), "frame 8") {
		t.Errorf("the last frame: %v", err)
	}
}

func TestRepresentative(t *testing.T) {
	for n, want := range map[int]string{1: "[0]", 2: "[0 1]", 66: "[0 16 33 49 65]"} {
		if got := fmt.Sprint(representative(n)); got != want {
			t.Errorf("representative(%d) = %s, want %s", n, got, want)
		}
	}
}

// A GIF whose later frames are small rectangles reads back as whole frames.
func TestComposeGIF(t *testing.T) {
	full := goimage.NewPaletted(goimage.Rect(0, 0, 4, 4), palette.Plan9)
	for i := range full.Pix {
		full.Pix[i] = 1
	}
	patch := goimage.NewPaletted(goimage.Rect(1, 1, 2, 2), palette.Plan9)
	patch.Pix[0] = 200
	g := &gif.GIF{Image: []*goimage.Paletted{full, patch}, Delay: []int{20, 20}, Config: goimage.Config{Width: 4, Height: 4, ColorModel: color.Palette(palette.Plan9)}}
	file := filepath.Join(t.TempDir(), "a.gif")
	f, err := os.Create(file) //nolint:gosec // A file in the test's own directory.
	if err != nil {
		t.Fatal(err)
	}
	if err := gif.EncodeAll(f, g); err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
	frames, err := decodeFrames(file)
	if err != nil {
		t.Fatal(err)
	}
	if len(frames) != 2 {
		t.Fatalf("%d frames", len(frames))
	}
	if frames[1].At(0, 0) != frames[0].At(0, 0) || frames[1].At(1, 1) == frames[0].At(1, 1) {
		t.Error("the second frame is not the first with the patch")
	}
	if err := compareFiles(file, file, 0); err != nil {
		t.Error(err)
	}
}
