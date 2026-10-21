package gfx

import (
	"errors"
	"io/fs"
	"math"
	"testing"
	"testing/fstest"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/text/v2"
	"golang.org/x/image/font/gofont/goregular"
)

// TestFontDrawCopiesOptions: Draw copies the options (the caller's value is not changed by
// the default line spacing) and accepts nil and invalid sizes without drawing.
func TestFontDrawCopiesOptions(t *testing.T) {
	f, err := Load(fstest.MapFS{"font.ttf": {Data: goregular.TTF}}, "font.ttf")
	if err != nil {
		t.Fatal(err)
	}
	dst := ebiten.NewImage(64, 64)
	op := &text.DrawOptions{}
	f.Draw(dst, "Trail", 16, op)
	if op.LineSpacing != 0 {
		t.Fatalf("Draw changed the caller's options: line spacing %v", op.LineSpacing)
	}
	f.Draw(dst, "Trail", 16, nil)
	spaced := &text.DrawOptions{}
	spaced.LineSpacing = 30
	f.Draw(dst, "a\nb", 16, spaced)
	if spaced.LineSpacing != 30 {
		t.Fatal("a given line spacing was replaced")
	}
	for _, size := range []float64{0, -3, math.NaN(), math.Inf(-1)} {
		f.Draw(dst, "Trail", size, op)
	}
}

func TestFontContracts(t *testing.T) {
	fsys := fstest.MapFS{"font.ttf": {Data: goregular.TTF}, "broken.ttf": {Data: []byte("broken")}}
	f, err := Load(fsys, "font.ttf")
	if err != nil {
		t.Fatal(err)
	}
	a, _ := f.Measure("Trail", 12)
	b, _ := f.Measure("Trail", 24)
	if a <= 0 || math.Abs(b-2*a) > 0.1 {
		t.Fatalf("size scaling %v %v", a, b)
	}
	for _, size := range []float64{0, -1, math.NaN(), math.Inf(1)} {
		w, h := f.Measure("text", size)
		if w != 0 || h != 0 {
			t.Fatal("invalid size")
		}
	}
	if f, err := Load(fsys, "broken.ttf"); err == nil || f != nil {
		t.Fatal("bad font accepted")
	}
	if _, err := Load(fsys, "missing.ttf"); !errors.Is(err, fs.ErrNotExist) {
		t.Fatal(err)
	}
	if _, err := Load(nil, "font.ttf"); !errors.Is(err, fs.ErrInvalid) {
		t.Fatal(err)
	}
}
