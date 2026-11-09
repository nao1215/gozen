package picdecode

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"testing"
)

func TestDecodePNG(t *testing.T) {
	src := image.NewNRGBA(image.Rect(0, 0, 3, 2))
	src.SetNRGBA(1, 1, color.NRGBA{0x12, 0x34, 0x56, 0xff})
	var b bytes.Buffer
	if err := png.Encode(&b, src); err != nil {
		t.Fatal(err)
	}
	img, err := Decode(b.Bytes())
	if err != nil {
		t.Fatal(err)
	}
	if img.Bounds() != src.Bounds() {
		t.Fatalf("bounds %v, want %v", img.Bounds(), src.Bounds())
	}
	if r, g, bl, _ := img.At(1, 1).RGBA(); r>>8 != 0x12 || g>>8 != 0x34 || bl>>8 != 0x56 {
		t.Fatalf("pixel (1,1) is %v", img.At(1, 1))
	}
	if _, err := Decode([]byte("not a picture")); err == nil {
		t.Fatal("no error for bytes that are no picture")
	}
}

func TestBrowserKind(t *testing.T) {
	for _, c := range []struct {
		b    string
		want bool
	}{
		{"RIFF\x00\x00\x00\x00WEBPVP8 ", true},
		{"\xff\xd8\xff\xe0", true},
		{"\x89PNG\r\n\x1a\n", false},
		{"RIFF\x00\x00\x00\x00WAVE", false},
		{"", false},
	} {
		if got := browserKind([]byte(c.b)); got != c.want {
			t.Errorf("browserKind(%q) = %v, want %v", c.b, got, c.want)
		}
	}
}

// A very wide, short picture reproduces the dimension failure without a large allocation.
func TestDecodeRejectsOversize(t *testing.T) {
	var b bytes.Buffer
	if err := png.Encode(&b, image.NewNRGBA(image.Rect(0, 0, 8193, 1))); err != nil {
		t.Fatal(err)
	}
	if _, err := Decode(b.Bytes()); err == nil {
		t.Fatal("oversized image accepted")
	}
}

func TestDecodeHighResolutionPortrait(t *testing.T) {
	src := image.NewNRGBA(image.Rect(0, 0, 4096, 6144))
	src.SetNRGBA(2048, 3072, color.NRGBA{R: 255, A: 255})
	var b bytes.Buffer
	if err := png.Encode(&b, src); err != nil {
		t.Fatal(err)
	}
	img, err := Decode(b.Bytes())
	if err != nil {
		t.Fatal(err)
	}
	if img.Bounds() != src.Bounds() || img.At(2048, 3072) != src.At(2048, 3072) {
		t.Fatal("portrait dimensions or pixels changed")
	}
}

// Size gives the dimensions Decode would, and rejects what Decode rejects.
func TestSize(t *testing.T) {
	var b bytes.Buffer
	if err := png.Encode(&b, image.NewNRGBA(image.Rect(0, 0, 5, 7))); err != nil {
		t.Fatal(err)
	}
	if got, err := Size(b.Bytes()); err != nil || got != image.Pt(5, 7) {
		t.Fatalf("Size = %v, %v", got, err)
	}
	b.Reset()
	if err := png.Encode(&b, image.NewNRGBA(image.Rect(0, 0, 8193, 1))); err != nil {
		t.Fatal(err)
	}
	if _, err := Size(b.Bytes()); err == nil {
		t.Error("oversized image accepted")
	}
	if _, err := Size([]byte("not a picture")); err == nil {
		t.Error("no error for bytes that are no picture")
	}
}
