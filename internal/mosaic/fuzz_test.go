package mosaic

import (
	"encoding/json"
	"image"
	"os"
	"path/filepath"
	"testing"
)

// FuzzLoadFixes: a fix file that loads can be saved and loaded again to the same fixes, and
// whatever the bytes are, loading does not panic.
func FuzzLoadFixes(f *testing.F) {
	for _, s := range []string{
		"pattern.png: {skip: true}",
		"a.png: {add: [[1,2,3,4]], poly: [[0,0,5,0,0,5]], block: 4, margin: 0.5, note: x}",
		"{}",
		"",
		"- a",
		"a.png: {grow: [[0,0,1,1,2,2,2,2]]}",
		"a.png: {add: [[0,0,0,0]]}",
	} {
		f.Add([]byte(s))
	}
	f.Fuzz(func(t *testing.T, b []byte) {
		dir := t.TempDir()
		path := filepath.Join(dir, "f.yaml")
		if err := os.WriteFile(path, b, 0o600); err != nil {
			t.Fatal(err)
		}
		fx, err := LoadFixes(path)
		if err != nil {
			return
		}
		out := filepath.Join(dir, "g.yaml")
		if err := fx.Save(out, "fuzz"); err != nil {
			t.Fatalf("a loaded set cannot be saved: %v", err)
		}
		again, err := LoadFixes(out)
		if err != nil {
			t.Fatalf("a saved set cannot be loaded: %v", err)
		}
		// Entries with nothing in them are not written.
		for k, v := range fx {
			if v.Empty() {
				delete(fx, k)
			}
		}
		// Compared as JSON, where an empty list and none are the same.
		want, err1 := json.Marshal(fx)
		got, err2 := json.Marshal(again)
		if err1 != nil || err2 != nil || string(want) != string(got) {
			t.Fatalf("round trip changed the fixes:\n%s\n%s", want, got)
		}
	})
}

// FuzzCellsStayInsideAndDoNotRepeat: for any block, offset and boxes, the cells are
// non-empty, inside the picture, and each appears once.
func FuzzCellsStayInsideAndDoNotRepeat(f *testing.F) {
	f.Add(24, 24, 4, 0, 0, 1, 1, 7, 7)
	f.Add(10, 30, -3, -7, 29, -5, -5, 50, 50)
	f.Add(1, 1, 100000, 3, 3, 0, 0, 1, 1)
	f.Fuzz(func(t *testing.T, w, h, block, ox, oy, x0, y0, x1, y1 int) {
		if w < 1 || h < 1 || w > 200 || h > 200 || ox < -1000 || ox > 1000 || oy < -1000 || oy > 1000 {
			return
		}
		b := image.Rect(0, 0, w, h)
		cells := CellsAt(b, []Shape{Box(image.Rect(x0, y0, x1, y1))}, block, image.Pt(ox, oy))
		seen := map[image.Rectangle]bool{}
		for _, c := range cells {
			if c.Empty() || !c.In(b) || seen[c] {
				t.Fatalf("cell %v (picture %v)", c, b)
			}
			seen[c] = true
		}
	})
}
