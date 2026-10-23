package picturelimit

import (
	"bytes"
	"image"
	"image/png"
	"io"
	"io/fs"
	"math"
	"testing"
	"testing/fstest"
)

func TestDimensions(t *testing.T) {
	for _, c := range []struct {
		w, h  int
		valid bool
	}{
		{8192, 2048, true},
		{4096, 4096, true},
		{1, 8192, true},
		{8193, 1, false},
		{4096, 4097, false},
		{0, 1, false},
		{-1, 1, false},
		{math.MaxInt, math.MaxInt, false},
	} {
		if got := Dimensions(c.w, c.h) == nil; got != c.valid {
			t.Errorf("%dx%d: valid=%v", c.w, c.h, got)
		}
	}
}

func TestHeaderAndRead(t *testing.T) {
	for _, w := range []int{8192, 8193} {
		var b bytes.Buffer
		if err := png.Encode(&b, image.NewNRGBA(image.Rect(0, 0, w, 1))); err != nil {
			t.Fatal(err)
		}
		if got := Header(b.Bytes()) == nil; got != (w == 8192) {
			t.Fatalf("width %d: %v", w, got)
		}
	}
	if Header([]byte("bad")) == nil {
		t.Fatal("invalid header accepted")
	}
	fsys := fstest.MapFS{"small": {Data: []byte("bytes")}}
	if b, err := Read(fsys, "small"); err != nil || string(b) != "bytes" {
		t.Fatalf("%q %v", b, err)
	}
	if _, err := Read(fsys, "missing"); err == nil {
		t.Fatal("missing file accepted")
	}
}

// An oversized encoded file is refused from metadata before reading its payload.
func TestReadRejectsEncodedSizeBeforeAllocation(t *testing.T) {
	f := &boundedFile{}
	if _, err := Read(f, "x"); err == nil {
		t.Fatal("oversized encoded file accepted")
	}
	if f.read || !f.closed {
		t.Fatalf("read=%v closed=%v", f.read, f.closed)
	}
}

type boundedFile struct {
	read, closed bool
}

func (f *boundedFile) Open(string) (fs.File, error) { return f, nil }
func (f *boundedFile) Stat() (fs.FileInfo, error)   { return largeInfo{}, nil }
func (f *boundedFile) Read([]byte) (int, error)     { f.read = true; return 0, io.EOF }
func (f *boundedFile) Close() error                 { f.closed = true; return nil }

type largeInfo struct{ fs.FileInfo }

func (largeInfo) Size() int64 { return MaxBytes + 1 }
