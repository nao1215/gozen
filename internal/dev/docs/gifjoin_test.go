package docs

import (
	goimage "image"
	"image/color"
	"image/gif"
	"os"
	"path/filepath"
	"testing"
)

func TestJoinGIFKeepsBothPhases(t *testing.T) {
	dir := t.TempDir()
	files := []string{filepath.Join(dir, "full.gif"), filepath.Join(dir, "hurt.gif")}
	for i, file := range files {
		palette := color.Palette{color.RGBA{255, 0, 0, 255}, color.RGBA{0, 0, 0, 255}}
		if i == 1 {
			palette = color.Palette{color.RGBA{0, 0, 255, 255}, color.RGBA{0, 0, 0, 255}}
		}
		frame := goimage.NewPaletted(goimage.Rect(0, 0, 2, 2), palette)
		for y := range 2 {
			for x := range 2 {
				frame.SetColorIndex(x, y, 0)
			}
		}
		f, err := os.Create(file)
		if err != nil {
			t.Fatal(err)
		}
		err = gif.EncodeAll(f, &gif.GIF{Image: []*goimage.Paletted{frame}, Delay: []int{8}, Config: goimage.Config{Width: 2, Height: 2}})
		if closeErr := f.Close(); err == nil {
			err = closeErr
		}
		if err != nil {
			t.Fatal(err)
		}
	}
	combined := filepath.Join(dir, "both.gif")
	if err := joinGIF(files[0], files[1], combined); err != nil {
		t.Fatal(err)
	}
	if err := slimGIF(combined, slimTolerance); err != nil {
		t.Fatal(err)
	}
	frames, err := decodeFrames(t.Context(), combined)
	if err != nil {
		t.Fatal(err)
	}
	if len(frames) != 2 || frames[0].At(0, 0) != (color.RGBA{255, 0, 0, 255}) || frames[1].At(0, 0) != (color.RGBA{0, 0, 255, 255}) {
		t.Fatalf("phases %v", frames)
	}
}
