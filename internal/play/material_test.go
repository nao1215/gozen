package play

import "testing"

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
