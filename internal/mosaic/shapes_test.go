package mosaic

import (
	"errors"
	"image"
	"image/color"
	"io"
	"io/fs"
	"math/rand/v2"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"testing/fstest"
)

// cellsPicture paints every block x block cell (starting at offset off) one flat color.
func cellsPicture(w, h, block int, off image.Point) *image.RGBA {
	m := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := range h {
		for x := range w {
			cx, cy := (x-off.X+block*10)/block, (y-off.Y+block*10)/block
			v := uint8((cx*37 + cy*91) % 250)                             //nolint:gosec // bounded by the modulo
			m.SetRGBA(x, y, color.RGBA{v, v ^ 0x55, uint8(cx * 11), 255}) //nolint:gosec // a test pattern, wrap-around is fine
		}
	}
	return m
}

func TestFindRecognizesRepeatedFlatCells(t *testing.T) {
	// A picture made of flat 5 pixel cells, with the grid starting two pixels in.
	img := cellsPicture(60, 60, 5, image.Pt(2, 3))
	got := Find(img, img.Bounds())
	if !got.Mosaic || got.Block != 5 || got.Flat != 1 {
		t.Fatalf("found %+v", got)
	}
	// Only the region asked about counts, clipped to the image.
	if got := Find(img, image.Rect(-10, -10, 70, 70)); !got.Mosaic || got.Block != 5 {
		t.Fatalf("region beyond the image: %+v", got)
	}
	// A smooth picture, a flat one, a region with fewer than nine cells, and a region
	// too small for the smallest block are not mosaics.
	if Find(pattern(), pattern().Bounds()).Mosaic {
		t.Fatal("a gradient was taken for a mosaic")
	}
	flat := image.NewRGBA(image.Rect(0, 0, 40, 40))
	if Find(flat, flat.Bounds()).Mosaic {
		t.Fatal("one flat color has no cells to tell apart")
	}
	if Find(img, image.Rect(0, 0, 14, 14)).Mosaic {
		t.Fatal("fewer than nine cells accepted")
	}
	if Find(img, image.Rect(0, 0, 8, 8)).Mosaic || Find(img, image.Rectangle{}).Mosaic {
		t.Fatal("tiny regions accepted")
	}
}

func TestMaskGeometry(t *testing.T) {
	a := image.NewAlpha(image.Rect(2, 3, 12, 13))
	if (Mask{a}).Bounds() != (image.Rectangle{}) || (Mask{a}).Area(image.Rectangle{}) != 0 {
		t.Fatal("an empty mask has no bounds and no area")
	}
	a.SetAlpha(4, 5, color.Alpha{255})
	a.SetAlpha(9, 11, color.Alpha{1})
	m := Mask{a}
	if got := m.Bounds(); got != image.Rect(4, 5, 10, 12) {
		t.Fatalf("bounds %v", got)
	}
	if m.Area(image.Rectangle{}) != 2 || m.Area(image.Rect(0, 0, 6, 8)) != 1 || m.Area(image.Rect(50, 50, 60, 60)) != 0 {
		t.Fatal("area")
	}
	if !m.Hits(image.Rect(3, 4, 5, 6)) || m.Hits(image.Rect(5, 6, 9, 11)) {
		t.Fatal("hits inside the mask")
	}
	// Cells hanging out of the mask's rectangle are clipped before looking.
	if !m.Hits(image.Rect(-5, -5, 5, 6)) || m.Hits(image.Rect(100, 100, 110, 110)) || m.Hits(image.Rect(0, 0, 3, 3)) {
		t.Fatal("hits outside the rectangle")
	}
}

func TestMaskDilateAndClip(t *testing.T) {
	a := image.NewAlpha(image.Rect(0, 0, 20, 20))
	a.SetAlpha(10, 10, color.Alpha{255})
	m := Mask{a}
	if got := m.Dilate(0, a.Bounds()).Area(image.Rectangle{}); got != 1 {
		t.Fatalf("dilation by 0 gives %d", got)
	}
	if got := m.Dilate(-4, a.Bounds()).Area(image.Rectangle{}); got != 1 {
		t.Fatalf("a negative dilation is none, got %d", got)
	}
	d := m.Dilate(2, a.Bounds())
	if d.Area(image.Rectangle{}) != 25 || d.Bounds() != image.Rect(8, 8, 13, 13) {
		t.Fatalf("5x5 expected, got area %d bounds %v", d.Area(image.Rectangle{}), d.Bounds())
	}
	// At the border the dilation is clipped to the given bounds, never beyond.
	b := image.NewAlpha(image.Rect(0, 0, 20, 20))
	b.SetAlpha(0, 0, color.Alpha{255})
	e := (Mask{b}).Dilate(3, image.Rect(0, 0, 20, 20))
	if e.Bounds() != image.Rect(0, 0, 4, 4) || e.Area(image.Rectangle{}) != 16 {
		t.Fatalf("corner dilation %v area %d", e.Bounds(), e.Area(image.Rectangle{}))
	}
	if g := (Mask{b}).Dilate(3, image.Rect(0, 0, 2, 2)); g.Rect != image.Rect(0, 0, 2, 2) {
		t.Fatalf("bounds not respected: %v", g.Rect)
	}
	clip := d.Clip(image.Rect(0, 0, 10, 10))
	if clip.Rect != image.Rect(0, 0, 10, 10) || clip.Area(image.Rectangle{}) != 4 {
		t.Fatalf("clip %v area %d", clip.Rect, clip.Area(image.Rectangle{}))
	}
	if !(Mask{a}).Clip(image.Rect(40, 40, 50, 50)).Rect.Empty() {
		t.Fatal("clipping away everything leaves a non-empty rectangle")
	}
}

func TestPolygonBoundsContainsAndHits(t *testing.T) {
	if (Polygon{}).Bounds() != (image.Rectangle{}) {
		t.Fatal("empty polygon bounds")
	}
	tri := Polygon{{2, 3}, {10, 3}, {6, 9}}
	if got := tri.Bounds(); got != image.Rect(2, 3, 11, 10) {
		t.Fatalf("bounds %v (the far edge is exclusive)", got)
	}
	// A concave "U": the notch is outside.
	u := Polygon{{0, 0}, {9, 0}, {9, 9}, {6, 9}, {6, 3}, {3, 3}, {3, 9}, {0, 9}}
	if !u.Contains(1, 5) || !u.Contains(7.5, 5) || u.Contains(4.5, 5) || !u.Contains(4.5, 1) {
		t.Fatal("even-odd containment of a concave polygon")
	}
	// A cell inside the notch does not hit; one touching a vertex or the body does.
	if u.Hits(image.Rect(4, 5, 5, 6)) {
		t.Fatal("a cell in the notch was hit")
	}
	if !u.Hits(image.Rect(0, 0, 1, 1)) || !u.Hits(image.Rect(6, 8, 8, 10)) {
		t.Fatal("body cells must hit")
	}
	// A vertex inside a cell counts even when no pixel center is inside the polygon.
	sliver := Polygon{{5, 5}, {5, 6}, {6, 5}}
	if !sliver.Hits(image.Rect(5, 5, 6, 6)) {
		t.Fatal("a vertex in the cell must hit")
	}
}

func TestBoxAndApply(t *testing.T) {
	b := Box(image.Rect(1, 2, 5, 6))
	if b.Bounds() != image.Rect(1, 2, 5, 6) || !b.Hits(image.Rect(4, 5, 8, 8)) || b.Hits(image.Rect(5, 2, 8, 6)) {
		t.Fatal("box")
	}
	img := pattern()
	byRects := Apply(img, []image.Rectangle{image.Rect(1, 1, 7, 7)}, 4)
	byShapes := ApplyShapes(img, []Shape{Box(image.Rect(1, 1, 7, 7))}, 4)
	if !reflect.DeepEqual(byRects.Pix, byShapes.Pix) {
		t.Fatal("Apply and ApplyShapes differ")
	}
	// Zero and negative blocks use the default.
	if !reflect.DeepEqual(ApplyShapes(img, []Shape{b}, 0).Pix, ApplyShapes(img, []Shape{b}, DefaultBlock(24, 24)).Pix) {
		t.Fatal("block 0 is not the default")
	}
	if !reflect.DeepEqual(ApplyShapesAt(img, []Shape{b}, -2, image.Point{}).Pix, ApplyShapes(img, []Shape{b}, 0).Pix) {
		t.Fatal("ApplyShapesAt with no offset differs from ApplyShapes")
	}
	// With no shapes the copy equals the input and is a new image.
	none := ApplyShapes(img, nil, 4)
	if !reflect.DeepEqual(none.Pix, img.Pix) || &none.Pix[0] == &img.Pix[0] {
		t.Fatal("copy")
	}
}

func TestApplyShapesAtShiftsTheGrid(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 16, 4))
	for x := range 16 {
		for y := range 4 {
			img.SetRGBA(x, y, color.RGBA{uint8(x * 10), 0, 0, 255})
		}
	}
	px := []Shape{Box(image.Rect(5, 0, 6, 1))}
	// No shift: the cell is columns 4..7, whose red average is 55.
	out := ApplyShapesAt(img, px, 4, image.Point{})
	if out.RGBAAt(4, 0).R != 55 || out.RGBAAt(7, 0).R != 55 || out.RGBAAt(3, 0).R != 30 || out.RGBAAt(8, 0).R != 80 {
		t.Fatalf("unshifted: %v %v %v %v", out.RGBAAt(3, 0), out.RGBAAt(4, 0), out.RGBAAt(7, 0), out.RGBAAt(8, 0))
	}
	// Shifted by 2: the cell is columns 2..5 (average 35)... the pixel at 5 is in 2..5.
	out = ApplyShapesAt(img, px, 4, image.Pt(2, 0))
	if out.RGBAAt(2, 0).R != 35 || out.RGBAAt(5, 0).R != 35 || out.RGBAAt(6, 0).R != 60 {
		t.Fatalf("shifted: %v %v %v", out.RGBAAt(2, 0), out.RGBAAt(5, 0), out.RGBAAt(6, 0))
	}
	// Only the first row of the cell: rows 0..3 are one cell, so all four rows are filled.
	if out.RGBAAt(3, 3).R != 35 {
		t.Fatal("a cell covers its whole height")
	}
	// A shift of a whole block or more is the same grid.
	if !reflect.DeepEqual(ApplyShapesAt(img, px, 4, image.Pt(6, 0)).Pix, out.Pix) {
		t.Fatal("offsets repeat every block")
	}
	if !reflect.DeepEqual(ApplyShapesAt(img, px, 4, image.Pt(-2, 0)).Pix, out.Pix) {
		t.Fatalf("a negative offset is the same grid shifted back")
	}
}

func TestAverageKeepsAlpha(t *testing.T) {
	img := image.NewNRGBA(image.Rect(0, 0, 2, 1))
	img.SetNRGBA(0, 0, color.NRGBA{255, 0, 0, 255})
	img.SetNRGBA(1, 0, color.NRGBA{0, 0, 255, 0})
	got := average(img, img.Bounds())
	// Premultiplied channels are averaged: red 255/2, blue none, alpha 255/2.
	if got.R != 127 || got.B != 0 || got.A != 127 {
		t.Fatalf("average %v", got)
	}
	if average(img, image.Rectangle{}) != (color.RGBA{}) {
		t.Fatal("empty cell")
	}
}

func TestPadGrowMerge(t *testing.T) {
	bounds := image.Rect(0, 0, 100, 80)
	if got := Pad(image.Rect(10, 10, 20, 20), 5, 3, bounds); got != image.Rect(5, 7, 25, 23) {
		t.Fatalf("pad %v", got)
	}
	if got := Pad(image.Rect(2, 2, 20, 78), 5, 5, bounds); got != image.Rect(0, 0, 25, 80) {
		t.Fatalf("padding is clipped to the image: %v", got)
	}
	// Grow lifts a box to a minimum size around its center, kept inside the image.
	if got := Grow(image.Rect(40, 30, 44, 34), 10, 8, bounds); got != image.Rect(37, 28, 47, 36) {
		t.Fatalf("grow %v", got)
	}
	if got := Grow(image.Rect(0, 0, 2, 2), 10, 10, bounds); got != image.Rect(0, 0, 10, 10) {
		t.Fatalf("grow at the corner %v", got)
	}
	if got := Grow(image.Rect(98, 78, 100, 80), 10, 10, bounds); got != image.Rect(90, 70, 100, 80) {
		t.Fatalf("grow at the far corner %v", got)
	}
	if got := Grow(image.Rect(10, 10, 60, 60), 20, 20, bounds); got != image.Rect(10, 10, 60, 60) {
		t.Fatalf("a larger box is not shrunk: %v", got)
	}
	if got := Grow(image.Rect(10, 10, 12, 12), 500, 500, bounds); got != bounds {
		t.Fatalf("grown past the image: %v", got)
	}

	in := []image.Rectangle{image.Rect(0, 0, 4, 4), image.Rect(4, 0, 8, 4), image.Rect(20, 0, 24, 4), image.Rect(9, 0, 12, 4)}
	copyIn := append([]image.Rectangle(nil), in...)
	if got := Merge(in, 0); !reflect.DeepEqual(got, []image.Rectangle{image.Rect(0, 0, 8, 4), image.Rect(20, 0, 24, 4), image.Rect(9, 0, 12, 4)}) {
		// 9..12 is one pixel from 8: not merged with a gap of 0.
		t.Fatalf("merge gap 0: %v", got)
	}
	if !reflect.DeepEqual(in, copyIn) {
		t.Fatal("Merge changed its input")
	}
	// A gap of 1 joins the rectangle one pixel away, and merging chains.
	got := Merge(in, 1)
	if len(got) != 2 {
		t.Fatalf("merge gap 1: %v", got)
	}
	if got := Merge(in, 20); len(got) != 1 || got[0] != image.Rect(0, 0, 24, 4) {
		t.Fatalf("everything within the gap is one: %v", got)
	}
	if len(Merge(nil, 3)) != 0 {
		t.Fatal("nothing to merge")
	}
}

func TestBestOffsetCoversTheLeastArea(t *testing.T) {
	b := image.Rect(0, 0, 40, 40)
	area := func(shapes []Shape, block int, off image.Point) int {
		n := 0
		for _, c := range CellsAt(b, shapes, block, off) {
			n += c.Dx() * c.Dy()
		}
		return n
	}
	// A box exactly the size of one cell, off the zero grid: the best offset aligns the grid to it.
	shapes := []Shape{Box(image.Rect(12, 12, 20, 20))}
	off := BestOffset(b, shapes, 8)
	if got := area(shapes, 8, off); got != 64 {
		t.Fatalf("offset %v covers %d pixels, want one cell (64)", off, got)
	}
	if area(shapes, 8, image.Point{}) <= 64 {
		t.Fatal("test shape does not need an offset")
	}
	// No shapes, or a block of 1 or less, need no offset; so does a shape outside the image.
	if BestOffset(b, nil, 8) != (image.Point{}) || BestOffset(b, shapes, 1) != (image.Point{}) {
		t.Fatal("no offset expected")
	}
	if BestOffset(b, []Shape{Box(image.Rect(100, 100, 110, 110))}, 8) != (image.Point{}) {
		t.Fatal("outside shapes need no offset")
	}
	// Whatever the shapes, the best offset never covers more than the plain grid does.
	rng := rand.New(rand.NewPCG(7, 9)) //nolint:gosec // A fixed seed for a reproducible test.
	for range 40 {
		var ss []Shape
		for range 1 + rng.IntN(3) {
			x, y := rng.IntN(36), rng.IntN(36)
			ss = append(ss, Box(image.Rect(x, y, x+1+rng.IntN(10), y+1+rng.IntN(10))))
		}
		block := 4 + rng.IntN(7)
		off := BestOffset(b, ss, block)
		if off.X < 0 || off.Y < 0 || off.X >= block || off.Y >= block {
			t.Fatalf("offset %v outside the block %d", off, block)
		}
		if area(ss, block, off) > area(ss, block, image.Point{}) {
			t.Fatalf("offset %v covers more than none (block %d)", off, block)
		}
	}
	// A polygon is counted by the pixels it covers, not its box.
	poly := []Shape{Polygon{{4, 4}, {20, 4}, {4, 20}}}
	off = BestOffset(b, poly, 8)
	if area(poly, 8, off) > area(poly, 8, image.Point{}) {
		t.Fatal("polygon offset worse than none")
	}
}

func TestFixHelpers(t *testing.T) {
	f := Fix{Poly: [][]int{{0, 0, 4, 0, 4, 4, 9}}}
	if p := f.Polygons(); len(p) != 1 || len(p[0]) != 3 || p[0][2].X != 4 || p[0][2].Y != 4 {
		t.Fatalf("polygons %v (a trailing odd coordinate is ignored)", p)
	}
	if got := Rect([4]int{1, 2, 3, 4}); got != image.Rect(1, 2, 4, 6) {
		t.Fatalf("Rect %v", got)
	}
	if !(Fix{}).Empty() {
		t.Fatal("zero fix is empty")
	}
	for _, f := range []Fix{{Skip: true}, {Block: 3}, {Margin: 0.1}, {Checked: true}, {Note: "n"}, {Add: [][4]int{{0, 0, 1, 1}}}, {Poly: [][]int{{0, 0, 1, 0, 0, 1}}}, {Drop: [][4]int{{0, 0, 1, 1}}}, {Grow: [][8]int{{}}}} {
		if f.Empty() {
			t.Errorf("%+v is empty", f)
		}
	}
	a := Fix{Add: [][4]int{{0, 0, 1, 1}}, Margin: 0.2, Block: 4, Note: "first"}
	b := Fix{Add: [][4]int{{2, 2, 1, 1}}, Drop: [][4]int{{1, 1, 1, 1}}, Skip: true, Block: 0, Note: "second"}
	c := Combine(a, b)
	if len(c.Add) != 2 || len(c.Drop) != 1 || c.Margin != 0.2 || c.Block != 4 || !c.Skip || c.Checked || c.Note != "first second" {
		t.Fatalf("combined %+v", c)
	}
	if got := Combine(a, Fix{Margin: 0.5, Block: 9, Checked: true}); got.Margin != 0.5 || got.Block != 9 || !got.Checked || got.Note != "first" {
		t.Fatalf("overrides %+v", got)
	}
	// Combining never writes into the first fix's slices.
	c.Add[0] = [4]int{9, 9, 9, 9}
	if a.Add[0] != [4]int{0, 0, 1, 1} {
		t.Fatal("Combine aliased its input")
	}
	if got := Combine(Fix{}, Fix{Note: "only"}); got.Note != "only" {
		t.Fatalf("note %q", got.Note)
	}
}

func TestLoadFixesEdges(t *testing.T) {
	dir := t.TempDir()
	if fx, err := LoadFixes(""); err != nil || len(fx) != 0 {
		t.Fatalf("no path: %v %v", fx, err)
	}
	if fx, err := LoadFixes(filepath.Join(dir, "none.yaml")); err != nil || len(fx) != 0 {
		t.Fatalf("a file that does not exist is an empty set: %v %v", fx, err)
	}
	write := func(s string) string {
		p := filepath.Join(dir, "f.yaml")
		if err := os.WriteFile(p, []byte(s), 0o600); err != nil {
			t.Fatal(err)
		}
		return p
	}
	if fx, err := LoadFixes(write("{}\n")); err != nil || len(fx) != 0 {
		t.Fatalf("empty map: %v %v", fx, err)
	}
	for name, s := range map[string]string{
		"empty file":     "",
		"two documents":  "a.png: {skip: true}\n---\nb.png: {skip: true}\n",
		"broken":         "a.png: {skip: ",
		"list":           "- a.png\n",
		"absolute":       "/a.png: {skip: true}",
		"parent":         "a/../../b.png: {skip: true}",
		"colon":          "'c:a.png': {skip: true}",
		"backslash":      "'a\\b.png': {skip: true}",
		"dot":            "'.': {skip: true}",
		"block too big":  "a.png: {block: 257}",
		"margin over 1":  "a.png: {margin: 1.5}",
		"huge rect":      "a.png: {add: [[0,0,2000000,5]]}",
		"huge poly":      "a.png: {poly: [[0,0,5,0,2000000,9]]}",
		"huge grow":      "a.png: {grow: [[0,0,5,5,0,0,2000000,5]]}",
		"zero grow":      "a.png: {grow: [[0,0,0,5,0,0,5,5]]}",
		"drop no size":   "a.png: {drop: [[0,0,5,0]]}",
		"line polygon":   "a.png: {poly: [[0,0,4,4,8,8]]}",
		"two points":     "a.png: {poly: [[0,0,4,4]]}",
		"unknown key":    "a.png: {mystery: 1}",
		"wrong type":     "a.png: {block: many}",
		"duplicate keys": "a.png: {skip: true}\na.png: {skip: false}\n",
	} {
		if _, err := LoadFixes(write(s)); err == nil {
			t.Errorf("%s: accepted %q", name, s)
		}
	}
	// A directory where the file should be is an error, not an empty set.
	if _, err := LoadFixes(dir); err == nil {
		t.Fatal("a directory was read as a fix file")
	}
}

func TestFixesSave(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "fixes.yaml")
	fx := Fixes{
		"z/last.png": {Skip: true},
		"画像 1.png":   {Add: [][4]int{{1, 2, 3, 4}}, Note: "日本語 note: with colon"},
		"empty.png":  {},
	}
	if err := fx.Save(path, "line one\nline two\n"); err != nil {
		t.Fatal(err)
	}
	text := string(readFile(t, path))
	if !strings.HasPrefix(text, "# line one\n# line two\n") {
		t.Fatalf("header %q", text)
	}
	if strings.Contains(text, "empty.png") {
		t.Fatalf("an empty entry was written: %q", text)
	}
	if strings.Index(text, "z/last.png") > strings.Index(text, "画像") {
		t.Fatalf("keys are not sorted: %q", text)
	}
	back, err := LoadFixes(path)
	if err != nil {
		t.Fatal(err)
	}
	want := Fixes{"z/last.png": fx["z/last.png"], "画像 1.png": fx["画像 1.png"]}
	if !reflect.DeepEqual(back, want) {
		t.Fatalf("round trip %v, want %v", back, want)
	}
	// Nothing to write is an explicit empty map, which loads again.
	if err := (Fixes{"x.png": {}}).Save(path, ""); err != nil {
		t.Fatal(err)
	}
	if b := readFile(t, path); string(b) != "{}\n" {
		t.Fatalf("empty save %q", b)
	}
	// An invalid set is refused and the file stays as it was.
	if err := (Fixes{"../x.png": {Skip: true}}).Save(path, ""); err == nil {
		t.Fatal("saved an invalid path")
	}
	if b := readFile(t, path); string(b) != "{}\n" {
		t.Fatalf("a refused save changed the file: %q", b)
	}
	// No temporary files are left behind.
	for _, e := range readDir(t, dir) {
		if strings.HasPrefix(e.Name(), ".mosaic-write-") {
			t.Fatalf("left %s", e.Name())
		}
	}
	if err := fx.Save(filepath.Join(dir, "no-such-dir", "f.yaml"), ""); err == nil {
		t.Fatal("saved into a missing directory")
	}
}

func TestWriteFile(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "out.txt")
	if err := WriteFile(target, func(w io.Writer) error { _, err := io.WriteString(w, "one"); return err }); err != nil {
		t.Fatal(err)
	}
	// A failing encoder leaves the old file in place and no temporary file.
	boom := errors.New("boom")
	if err := WriteFile(target, func(w io.Writer) error {
		if _, err := io.WriteString(w, "partial"); err != nil {
			return err
		}
		return boom
	}); !errors.Is(err, boom) {
		t.Fatalf("encoder error lost: %v", err)
	}
	if b := readFile(t, target); string(b) != "one" {
		t.Fatalf("a failed write damaged the file: %q", b)
	}
	if ents := readDir(t, dir); len(ents) != 1 {
		t.Fatalf("leftover files: %v", ents)
	}
	// A symbolic link is replaced, not written through: its target keeps its content.
	original := filepath.Join(dir, "real.txt")
	if err := os.WriteFile(original, []byte("precious"), 0o600); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(dir, "link.txt")
	if err := os.Symlink(original, link); err != nil {
		t.Skip("symbolic links are not available")
	}
	if err := WriteFile(link, func(w io.Writer) error { _, err := io.WriteString(w, "new"); return err }); err != nil {
		t.Fatal(err)
	}
	if b := readFile(t, original); string(b) != "precious" {
		t.Fatalf("wrote through the link: %q", b)
	}
	if st, err := os.Lstat(link); err != nil || st.Mode()&fs.ModeSymlink != 0 {
		t.Fatalf("the link was not replaced: %v %v", st, err)
	}
	if err := WriteFile(filepath.Join(dir, "missing", "x"), func(io.Writer) error { return nil }); err == nil {
		t.Fatal("missing directory accepted")
	}
}

func TestReferencesRules(t *testing.T) {
	base := fstest.MapFS{
		"assets/a.png":       {Data: []byte("x")},
		"assets/dir/b.JPG":   {Data: []byte("x")},
		"assets/dir/c.webp":  {Data: []byte("x")},
		"assets/d.gif":       {Data: []byte("x")},
		"data/one.yaml":      {Data: []byte("pic: assets/a.png\nlist:\n  - assets/dir/b.JPG\n  - {nested: assets/missing.png}\n")},
		"data/two.yml":       {Data: []byte("- assets/dir/*.webp\n- a note about assets/a.png with spaces\n- assets/d.gif\n- 42\n")},
		"asset-policy.yaml":  {Data: []byte("allow: assets/policy-only.png\n")},
		"assets/inside.yaml": {Data: []byte("x: assets/inside.png\n")},
		"readme.txt":         {Data: []byte("assets/text.png")},
	}
	pics, missing, err := references(base, "assets", []string{".png", ".jpg", ".webp"})
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(pics, []string{"a.png", "dir/b.JPG", "dir/c.webp"}) {
		t.Fatalf("pictures %v", pics)
	}
	if !reflect.DeepEqual(missing, []string{"missing.png"}) {
		t.Fatalf("missing %v (the policy file, the assets folder's own YAML and non-YAML files are not read)", missing)
	}
	// A broken YAML file is an error, a bad glob pattern is one too.
	if _, _, err := references(fstest.MapFS{"x.yaml": {Data: []byte("a: [")}}, "assets", []string{".png"}); err == nil {
		t.Fatal("broken YAML accepted")
	}
	bad := fstest.MapFS{"assets/a.png": {Data: []byte("x")}, "x.yaml": {Data: []byte("p: assets/[.png\n")}}
	if _, _, err := references(bad, "assets", []string{".png"}); !errors.Is(err, fs.ErrInvalid) && err == nil {
		t.Fatalf("bad pattern: %v", err)
	}
}

// A block of 1 averages each pixel with itself: the picture is unchanged (the documented way to leave it).
func TestBlockOneLeavesThePixelsUnchanged(t *testing.T) {
	img := pattern()
	out := ApplyShapes(img, []Shape{Box(img.Bounds())}, 1)
	if !reflect.DeepEqual(out.Pix, img.Pix) {
		t.Fatal("block 1 changed the picture")
	}
}

// readFile reads a file the test knows about.
func readFile(t *testing.T, path string) []byte {
	t.Helper()
	b, err := os.ReadFile(path) //nolint:gosec // The tests read files they made in t.TempDir.
	if err != nil {
		t.Fatal(err)
	}
	return b
}

// readDir lists a directory the test knows about.
func readDir(t *testing.T, path string) []os.DirEntry {
	t.Helper()
	es, err := os.ReadDir(path)
	if err != nil {
		t.Fatal(err)
	}
	return es
}
