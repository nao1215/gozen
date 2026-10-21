package tagger

import (
	"encoding/csv"
	"errors"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"os"

	xdraw "golang.org/x/image/draw"
)

// The tag list, the preprocessing and the reading of the output of a WD tagger, apart from
// the ONNX session. They need no ONNX Runtime, so they are checked without a model.

// Keep is the lowest score a tag is reported with.
const Keep = 0.05

// readNames reads the tag names, in the model's output order, from selected_tags.csv:
// a header row whose second column is "name", then one row per tag.
func readNames(path string) (result []string, err error) {
	f, err := os.Open(path) //nolint:gosec // The caller explicitly selects this input file.
	if err != nil {
		return nil, err
	}
	defer func() { err = errors.Join(err, f.Close()) }()
	rows, err := csv.NewReader(f).ReadAll()
	if err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	if len(rows) < 2 || len(rows[0]) < 2 || rows[0][1] != "name" {
		return nil, fmt.Errorf("%s: the second column must be name", path)
	}
	names := make([]string, 0, len(rows)-1)
	for _, r := range rows[1:] {
		names = append(names, r[1])
	}
	return names, nil
}

// prepare puts img in the middle of a white square, scales that to size x size and returns
// the pixels as the NHWC float input of a WD tagger: blue, green, red, each 0 to 255.
func prepare(img image.Image, size int) []float32 {
	b := img.Bounds()
	s := max(b.Dx(), b.Dy())
	sq := image.NewRGBA(image.Rect(0, 0, s, s))
	draw.Draw(sq, sq.Bounds(), image.NewUniform(color.White), image.Point{}, draw.Src)
	at := image.Pt((s-b.Dx())/2, (s-b.Dy())/2)
	draw.Draw(sq, image.Rectangle{Min: at, Max: at.Add(b.Size())}, img, b.Min, draw.Over)
	small := image.NewRGBA(image.Rect(0, 0, size, size))
	xdraw.CatmullRom.Scale(small, small.Bounds(), sq, sq.Bounds(), draw.Src, nil)

	data := make([]float32, size*size*3)
	for i := range size * size {
		p := small.Pix[i*4 : i*4+3]
		data[i*3], data[i*3+1], data[i*3+2] = float32(p[2]), float32(p[1]), float32(p[0])
	}
	return data
}

// tagsFrom names the scores v (one per tag, in the order of names) that are at least Keep,
// rounded to three decimals.
func tagsFrom(v []float32, names []string) (map[string]float32, error) {
	if len(v) != len(names) {
		return nil, fmt.Errorf("the tagger gives %d values for %d tags", len(v), len(names))
	}
	tags := map[string]float32{}
	for i, p := range v {
		if p >= Keep {
			tags[names[i]] = float32(int(p*1000+0.5)) / 1000
		}
	}
	return tags, nil
}
