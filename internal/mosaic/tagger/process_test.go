package tagger

import (
	"image"
	"image/color"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeCSV(t *testing.T, body string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "selected_tags.csv")
	if err := os.WriteFile(p, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestReadNames(t *testing.T) {
	names, err := readNames(writeCSV(t, "tag_id,name,category,count\n1,general,9,100\n2,\"long hair\",0,5\n3,penis,0,1\n"))
	if err != nil || len(names) != 3 || names[0] != "general" || names[1] != "long hair" || names[2] != "penis" {
		t.Fatalf("names %v %v", names, err)
	}
	for name, body := range map[string]string{
		"empty":        "",
		"header only":  "tag_id,name\n",
		"wrong header": "tag_id,label\n1,a\n",
		"one column":   "name\na\n",
		"ragged":       "tag_id,name\n1,a,extra\n",
		"unbalanced":   "tag_id,name\n1,\"a\n",
	} {
		if _, err := readNames(writeCSV(t, body)); err == nil {
			t.Errorf("%s accepted", name)
		}
	}
	if _, err := readNames(filepath.Join(t.TempDir(), "none.csv")); !os.IsNotExist(err) {
		t.Fatalf("missing file: %v", err)
	}
}

func TestPrepareFitsAWhiteSquareInBGR(t *testing.T) {
	// A 4 x 2 red picture in a 4 x 4 square: white above and below, red between. At the same
	// size the scaling keeps the pixels.
	img := image.NewRGBA(image.Rect(0, 0, 4, 2))
	for y := range 2 {
		for x := range 4 {
			img.SetRGBA(x, y, color.RGBA{255, 0, 0, 255})
		}
	}
	data := prepare(img, 4)
	if len(data) != 4*4*3 {
		t.Fatalf("%d values", len(data))
	}
	px := func(x, y int) [3]float32 {
		i := (y*4 + x) * 3
		return [3]float32{data[i], data[i+1], data[i+2]}
	}
	near := func(a, b [3]float32) bool {
		for i := range a {
			if math.Abs(float64(a[i]-b[i])) > 2 {
				return false
			}
		}
		return true
	}
	if got := px(1, 0); !near(got, [3]float32{255, 255, 255}) {
		t.Fatalf("the margin is white, got %v", got)
	}
	// Red in BGR order is (blue 0, green 0, red 255), in the 0 to 255 range.
	if got := px(1, 1); !near(got, [3]float32{0, 0, 255}) {
		t.Fatalf("red as BGR, got %v", got)
	}
	if got := px(2, 3); !near(got, [3]float32{255, 255, 255}) {
		t.Fatalf("the lower margin is white, got %v", got)
	}
	// Transparent pixels show the white behind them.
	blank := image.NewRGBA(image.Rect(0, 0, 4, 4))
	if got := prepare(blank, 4); got[0] != 255 || got[1] != 255 || got[2] != 255 {
		t.Fatalf("transparent over white: %v", got[:3])
	}
	// A picture with an origin away from zero.
	shifted := image.NewRGBA(image.Rect(10, 10, 14, 12))
	for i := 0; i < len(shifted.Pix); i += 4 {
		shifted.Pix[i+2], shifted.Pix[i+3] = 255, 255
	}
	if got := prepare(shifted, 4); !near([3]float32{got[(1*4+1)*3], got[(1*4+1)*3+1], got[(1*4+1)*3+2]}, [3]float32{255, 0, 0}) {
		t.Fatalf("blue as BGR: %v", got[(1*4+1)*3:(1*4+1)*3+3])
	}
}

func TestTagsFrom(t *testing.T) {
	names := []string{"a", "b", "c", "d", "e"}
	got, err := tagsFrom([]float32{0.04, 0.05, 0.12345, 0.9996, 0}, names)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 3 {
		t.Fatalf("kept %v", got)
	}
	// The lowest kept score is Keep itself; scores are rounded to three decimals.
	if got["b"] != 0.05 || got["c"] != 0.123 || got["d"] != 1 {
		t.Fatalf("scores %v", got)
	}
	if _, ok := got["a"]; ok {
		t.Fatal("a score below Keep was reported")
	}
	if _, err := tagsFrom([]float32{1, 2}, names); err == nil || !strings.Contains(err.Error(), "2 values for 5 tags") {
		t.Fatalf("length mismatch: %v", err)
	}
	if got, err := tagsFrom(nil, nil); err != nil || got == nil || len(got) != 0 {
		t.Fatalf("no tags: %v %v", got, err)
	}
}
