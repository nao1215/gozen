package gfx

import (
	"errors"
	"io/fs"
	"math"
	"testing"
	"testing/fstest"

	"golang.org/x/image/font/gofont/goregular"
)

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
