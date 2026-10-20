package mosaic

import (
	"bytes"
	"image"
	"image/color"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func pattern() *image.RGBA {
	m := image.NewRGBA(image.Rect(0, 0, 24, 24))
	for y := 0; y < 24; y++ {
		for x := 0; x < 24; x++ {
			m.SetRGBA(x, y, color.RGBA{uint8(x * 8), uint8(y * 8), uint8((x + y) * 4), 255})
		}
	}
	return m
}
func TestPixelationContracts(t *testing.T) {
	img := pattern()
	before := bytes.Clone(img.Pix)
	shape := Box(image.Rect(1, 1, 7, 7))
	single := ApplyShapes(img, []Shape{shape}, 4)
	double := ApplyShapes(img, []Shape{shape, shape}, 4)
	if !bytes.Equal(single.Pix, double.Pix) {
		t.Fatal("overlap applied more than once")
	}
	if !bytes.Equal(before, img.Pix) {
		t.Fatal("input mutated")
	}
	if got := single.RGBAAt(0, 0); got != (color.RGBA{12, 12, 12, 255}) {
		t.Fatalf("wrong source pixel average: %v", got)
	}
	if single.RGBAAt(9, 9) != img.RGBAAt(9, 9) {
		t.Fatal("changed unselected cell")
	}
	for _, block := range []int{-1, 0, 1, 4, 100000} {
		got := CellsAt(img.Bounds(), []Shape{shape, Box(image.Rect(-5, -5, 1, 1)), Box(image.Rect(50, 50, 60, 60))}, block, image.Pt(-3, 29))
		seen := map[image.Rectangle]bool{}
		for _, r := range got {
			if r.Empty() || !r.In(img.Bounds()) || seen[r] {
				t.Fatalf("invalid/double cell %v", r)
			}
			seen[r] = true
		}
	}
	if !reflect.DeepEqual(Cells(img.Bounds(), []Shape{shape}, 0), Cells(img.Bounds(), []Shape{shape}, DefaultBlock(24, 24))) {
		t.Fatal("low-level default block differs")
	}
	for _, tc := range []struct{ w, h, want int }{{0, 0, 4}, {400, 20, 4}, {401, 20, 5}, {1, 1200, 12}} {
		if got := DefaultBlock(tc.w, tc.h); got != tc.want {
			t.Errorf("default(%d,%d)=%d", tc.w, tc.h, got)
		}
	}
}
func TestShapesAndFixRoundTrip(t *testing.T) {
	m := image.NewAlpha(image.Rect(0, 0, 10, 10))
	m.SetAlpha(4, 4, color.Alpha{255})
	mask := Mask{m}
	if mask.Area(image.Rectangle{}) != 1 || mask.Dilate(1, m.Bounds()).Area(image.Rectangle{}) != 9 {
		t.Fatal("mask dilation")
	}
	pg := Polygon{{1, 1}, {8, 1}, {4, 8}}
	if !pg.Hits(image.Rect(3, 3, 4, 4)) || pg.Hits(image.Rect(20, 20, 21, 21)) {
		t.Fatal("polygon coverage")
	}
	path := filepath.Join(t.TempDir(), "fix.yaml")
	want := Fixes{"pattern.png": {Add: [][4]int{{1, 1, 3, 4}, {8, 8, 2, 2}}, Poly: [][]int{{1, 1, 7, 1, 4, 7}}, Grow: [][8]int{{1, 1, 2, 2, -1, 2, 3, 4}}, Skip: true, Checked: true, Block: 4, Note: "synthetic pattern"}}
	if err := want.Save(path, "original neutral fixture"); err != nil {
		t.Fatal(err)
	}
	got, err := LoadFixes(path)
	if err != nil || !reflect.DeepEqual(got, want) {
		t.Fatalf("round trip %v %v", got, err)
	}
	for _, s := range []string{"pattern.png: {unknown: true}", "pattern.png: {add: [[1,1,0,4]]}", "pattern.png: {add: [[1,1,-3,4]]}", "pattern.png: {margin: .nan}", "pattern.png: {block: -1}", "pattern.png: {poly: [[1,2,3]]}", "../pattern.png: {skip: true}"} {
		if err := os.WriteFile(path, []byte(s), 0o600); err != nil {
			t.Fatal(err)
		}
		if _, err := LoadFixes(path); err == nil {
			t.Errorf("accepted %q", s)
		}
	}
}

func TestPictureReferencesUseNeutralYAML(t *testing.T) {
	root := t.TempDir()
	for _, name := range []string{"assets/pattern.png", "assets/group/marker.png"} {
		p := filepath.Join(root, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(p), 0o750); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte("reference-only"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	source := []byte("images: [assets/pattern.png, assets/group/*.png, assets/missing.webp]\n")
	if err := os.WriteFile(filepath.Join(root, "scene.yaml"), source, 0o600); err != nil {
		t.Fatal(err)
	}
	pics, missing, err := Pictures(root)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(pics, []string{"group/marker.png", "pattern.png"}) || !reflect.DeepEqual(missing, []string{"missing.webp"}) {
		t.Fatalf("references: %v %v", pics, missing)
	}
}
