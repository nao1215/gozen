package docs

import (
	"fmt"
	goimage "image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"slices"
	"testing"
)

// TestSceneCuts: a GIF's scenes are cut where the picture changes much (a dim
// phone, then a lit figure), never closer than scenePhase frames, so each
// scene gets its own palette.
func TestSceneCuts(t *testing.T) {
	dir := t.TempDir()
	var frames []string
	shade := func(i int) uint8 {
		switch {
		case i < 15:
			return 40
		case i < 17: // a flicker too close to the cut before it
			return 200
		case i < 30:
			return 200 + uint8(i%3)
		}
		return 90
	}
	for i := range 45 {
		img := goimage.NewRGBA(goimage.Rect(0, 0, 64, 36))
		v := shade(i)
		for p := 0; p < len(img.Pix); p += 4 {
			img.Pix[p], img.Pix[p+1], img.Pix[p+2], img.Pix[p+3] = v, v, v, 255
		}
		img.Set(0, 0, color.RGBA{v, 0, 0, 255})
		path := filepath.Join(dir, fmt.Sprintf("f%03d.png", i))
		f, err := os.Create(path) //nolint:gosec // A file in the test's own directory.
		if err != nil {
			t.Fatal(err)
		}
		if err := png.Encode(f, img); err != nil {
			t.Fatal(err)
		}
		if err := f.Close(); err != nil {
			t.Fatal(err)
		}
		frames = append(frames, path)
	}
	cuts, err := sceneCuts(frames)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(cuts, []int{15, 30}) {
		t.Fatalf("cuts %v, want [15 30]", cuts)
	}
}
