package command

import (
	"bytes"
	"image"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// A picture is decoded only when its header is within the limits of a game's
// pictures: a file of a few hundred bytes may claim a picture of gigabytes.
func TestDecodeChecksTheHeaderFirst(t *testing.T) {
	dir := t.TempDir()
	for _, tc := range []struct {
		name  string
		w, h  int
		valid bool
	}{
		{"small.png", 4, 3, true},
		{"widest.png", 8192, 1, true},
		{"wide.png", 8193, 1, false},
		{"tall.png", 1, 8193, false},
	} {
		var b bytes.Buffer
		if err := png.Encode(&b, image.NewGray(image.Rect(0, 0, tc.w, tc.h))); err != nil {
			t.Fatal(err)
		}
		path := filepath.Join(dir, tc.name)
		if err := os.WriteFile(path, b.Bytes(), 0o600); err != nil {
			t.Fatal(err)
		}
		img, err := decode(path)
		if (err == nil) != tc.valid || (err != nil && !strings.Contains(err.Error(), "exceed limits")) {
			t.Fatalf("%s: %v, want valid %v", tc.name, err, tc.valid)
		}
		if err == nil && img.Bounds().Size() != image.Pt(tc.w, tc.h) {
			t.Fatalf("%s decoded as %v", tc.name, img.Bounds())
		}
	}
	if _, err := decode(filepath.Join(dir, "absent.png")); err == nil {
		t.Fatal("a missing picture decoded")
	}
}
