package gfx

import (
	"math"
	"testing"

	"golang.org/x/image/font/gofont/gomono"
	"golang.org/x/image/font/gofont/goregular"
)

func TestFontReloadInvalidatesMeasurements(t *testing.T) {
	t.Cleanup(Reset)
	if err := LoadFonts(goregular.TTF, nil); err != nil {
		t.Fatal(err)
	}
	before := TextWidth("iiiWWW", 20)
	if err := LoadFonts([]byte("bad font"), nil); err == nil {
		t.Fatal("accepted invalid font")
	}
	if got := TextWidth("iiiWWW", 20); got != before {
		t.Fatal("failed load replaced active font")
	}
	if err := LoadFonts(gomono.TTF, nil); err != nil {
		t.Fatal(err)
	}
	if got := TextWidth("iiiWWW", 20); got == before {
		t.Fatal("measurement uses stale font")
	}
}

func TestPaginationInvalidSizeTerminates(t *testing.T) {
	for _, size := range []float64{0, -1, math.Inf(1), math.NaN()} {
		if pages, sz := Paginate("A short note.", size, 120, 2); len(pages) != 0 || sz != 0 {
			t.Fatal("invalid pagination result")
		}
		if lines, sz, cut := FitLinesCut("A short note.", size, 120, 2); len(lines) != 0 || sz != 0 || !cut {
			t.Fatal("invalid fitting result")
		}
	}
}
