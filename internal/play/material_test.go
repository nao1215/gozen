package play

import (
	"image/color"
	"testing"

	data "github.com/nao1215/gozen/internal/layer2/gamedata"
)

func TestMaterialOpacityPreservesContentContrast(t *testing.T) {
	for _, c := range []struct {
		kind          string
		opacity, want float64
	}{
		{"mica", .2, .9}, {"acrylic", .2, .72}, {"glass", .2, .2}, {"opaque", .2, 1}, {"acrylic", .95, .95},
	} {
		if got := materialOpacity(c.kind, c.opacity); got != c.want {
			t.Errorf("%s: %g, want %g", c.kind, got, c.want)
		}
	}
}

// TestToastOpacity raises a glass card under a notice to nearly opaque and keeps a
// card already more opaque; on the darkest backdrop the light card under the light
// palette's text and accent then keeps 4.5:1.
func TestToastOpacity(t *testing.T) {
	for _, c := range []struct {
		m    data.MaterialStyle
		want float64
	}{
		{data.MaterialStyle{Kind: "glass", Opacity: .4}, toastOpacityFloor},
		{data.MaterialStyle{Kind: "glass", Opacity: .4, Opacities: map[string]float64{"card": .98}}, .98},
		{data.MaterialStyle{Kind: "glass", Opacity: 1, Opacities: map[string]float64{"card": .2}}, toastOpacityFloor},
	} {
		if got := toastOpacity(&c.m); got != c.want {
			t.Errorf("%+v: %g, want %g", c.m, got, c.want)
		}
	}
	card := color.NRGBA{250, 252, 254, 255}
	mix := func(v uint8) uint8 { return uint8(float64(v) * toastOpacityFloor) }
	// An approximation without the shader's edge and sheen; the finished screen is
	// measured by TestGraphicsToastsStayReadableOnGlass.
	back := color.NRGBA{mix(card.R), mix(card.G), mix(card.B), 255} // over black
	light := (&Game{db: &data.DB{}}).lightPalette()
	for _, ink := range []color.NRGBA{light.text, light.accent} {
		if r := contrastRatio(ink, back); r < 4.5 {
			t.Errorf("%v on a notice over black: %.2f, want 4.5 at least", ink, r)
		}
	}
}
