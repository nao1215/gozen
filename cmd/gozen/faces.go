package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"io/fs"
	"math"
	"os"
	"path"
	"path/filepath"
	"strings"

	xdraw "golang.org/x/image/draw"

	"github.com/nao1215/gozen/cmd/internal/cli"
	"github.com/nao1215/gozen/internal/clihelp"
	"github.com/nao1215/gozen/internal/diag"
	"github.com/nao1215/gozen/internal/layer2/gamedata"
	"github.com/nao1215/gozen/internal/layer5/doll"
	"github.com/nao1215/gozen/internal/mosaic/detect"
	data "github.com/nao1215/gozen/schema"
)

// faces finds the face of each standing picture a painted scene cuts a balloon face from,
// with the anime face detector of gozen art mosaic, and writes the boxes to assets/faces.json
// (ADR 0022). The game reads that file; it never runs the model.
func (c *command) faces(args []string) (err error) {
	f := clihelp.NewFlagSet("gozen art faces")
	model := f.String("model", "", "the face detection `MODEL` (default: the one gozen art fetch face keeps in the cache)")
	lib := f.String("ort", detect.LibraryPath(), "the ONNX Runtime shared `LIBRARY` (default: $ONNXRUNTIME_LIB)")
	minScore := f.Float32("min", 0.3, "the lowest `SCORE` of a face")
	preview := f.String("preview", "", "also write each picture with its face box and its balloon face to this new `DIR`")
	if err := c.parse(f, args); err != nil {
		return err
	}
	if f.NArg() != 1 {
		return cli.Invalid(diag.Operands, "art faces expects one game directory")
	}
	dir := f.Arg(0)
	fsys := os.DirFS(dir)
	db, err := data.Load(fsys, ".")
	if err != nil {
		return err
	}
	pics := db.ScenePictures()
	if len(pics) == 0 {
		c.out.Println("no standing pictures in a painted scene (stage.people, the actors' faces): nothing to do")
		return nil
	}
	if *model == "" {
		cache, err := os.UserCacheDir()
		if err != nil {
			return err
		}
		*model = filepath.Join(cache, "gozen", "mosaic", "face", "model.onnx")
		if _, err := os.Stat(*model); err != nil {
			return fmt.Errorf("no face model at %s: download it with gozen art fetch face, or give --model", *model)
		}
	}
	d, err := detect.NewWithLabels(*model, *lib, 0, detect.FaceLabels)
	if err != nil {
		return err
	}
	defer func() { err = errors.Join(err, d.Close()) }()
	passes, err := detect.PassesByName("full640,flip640")
	if err != nil {
		return err
	}
	out := gamedata.FaceBoxes{Version: 1, Model: filepath.Base(filepath.Dir(*model)) + "/" + filepath.Base(*model), Pictures: map[string]gamedata.FaceBox{}}
	previews := map[string][]byte{}
	found := 0
	for _, p := range pics {
		b, err := fs.ReadFile(fsys, "assets/"+p)
		if err != nil {
			return err
		}
		img, _, err := image.Decode(bytes.NewReader(b))
		if err != nil {
			return fmt.Errorf("assets/%s: %w", p, err)
		}
		sum, err := gamedata.FileSHA256(fsys, "assets/"+p)
		if err != nil {
			return err
		}
		boxes, err := d.Detect(img, passes, *minScore)
		if err != nil {
			return fmt.Errorf("assets/%s: %w", p, err)
		}
		fb := gamedata.FaceBox{Size: [2]int{img.Bounds().Dx(), img.Bounds().Dy()}, SHA256: sum}
		if best, ok := bestFace(boxes, img.Bounds()); ok {
			r := best.Rect.Sub(img.Bounds().Min)
			fb.Box, fb.Score = &[4]int{r.Min.X, r.Min.Y, r.Max.X, r.Max.Y}, best.Score
			found++
			c.out.Printf("assets/%s: face %v (%.2f)\n", p, r, best.Score)
		} else {
			c.out.Printf("assets/%s: no face found\n", p)
		}
		out.Pictures[p] = fb
		if *preview != "" {
			name := strings.ReplaceAll(strings.TrimSuffix(p, path.Ext(p)), "/", "_")
			if previews[name+".check.png"], err = encodePNG(faceCheck(img, fb.Box)); err != nil {
				return err
			}
			if fb.Box != nil {
				r := image.Rect(fb.Box[0], fb.Box[1], fb.Box[2], fb.Box[3]).Add(img.Bounds().Min)
				if previews[name+".face.png"], err = encodePNG(doll.Round(doll.FaceIn(img, r))); err != nil {
					return err
				}
			}
		}
	}
	js, err := json.MarshalIndent(out, "", "  ")
	if err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(dir, filepath.FromSlash(gamedata.FacesFile)), append(js, '\n'), 0o600); err != nil {
		return err
	}
	c.out.Printf("wrote %s: %d of %d pictures with a face\n", filepath.Join(dir, filepath.FromSlash(gamedata.FacesFile)), found, len(pics))
	if *preview == "" {
		return nil
	}
	return cli.Publish(*preview, nil, func(stage string) error {
		for n, b := range previews {
			if err := cli.WriteNew(filepath.Join(stage, n), b); err != nil {
				return err
			}
		}
		return nil
	})
}

// bestFace is the most certain face, preferring the upper part of the picture (a standing
// picture's face is near its top; a face on a shield or a badge is not the person's).
func bestFace(boxes []detect.Box, b image.Rectangle) (detect.Box, bool) {
	best, ok := detect.Box{}, false
	score := func(x detect.Box) float32 {
		if x.Rect.Min.Y > b.Min.Y+b.Dy()/3 {
			return x.Score * 0.5
		}
		return x.Score
	}
	for _, x := range boxes {
		if !ok || score(x) > score(best) {
			best, ok = x, true
		}
	}
	return best, ok
}

// faceCheck is the picture's top third, 480 high, with the face box (red) and the circle
// the balloon face is cut from (green).
func faceCheck(img image.Image, box *[4]int) image.Image {
	b := img.Bounds()
	top := image.Rect(b.Min.X, b.Min.Y, b.Max.X, b.Min.Y+b.Dy()/3)
	k := 480 / float64(top.Dy())
	out := image.NewRGBA(image.Rect(0, 0, int(float64(top.Dx())*k), 480))
	for i := range out.Pix {
		out.Pix[i] = 200
	}
	xdraw.CatmullRom.Scale(out, out.Bounds(), img, top, xdraw.Over, nil)
	if box == nil {
		return out
	}
	red, green := color.RGBA{230, 30, 30, 255}, color.RGBA{30, 200, 60, 255}
	x0, y0, x1, y1 := float64(box[0])*k, float64(box[1])*k, float64(box[2])*k, float64(box[3])*k
	for t := 0.0; t < 3; t++ {
		for x := x0; x <= x1; x++ {
			out.Set(int(x), int(y0+t), red)
			out.Set(int(x), int(y1-t), red)
		}
		for y := y0; y <= y1; y++ {
			out.Set(int(x0+t), int(y), red)
			out.Set(int(x1-t), int(y), red)
		}
	}
	cx, cy := (x0+x1)/2, (y0+y1)/2
	r := max(x1-x0, y1-y0) * doll.FaceMargin / 2
	for a := range 2000 {
		th := float64(a) / 2000 * 2 * math.Pi
		for t := 0.0; t < 3; t++ {
			out.Set(int(cx+(r-t)*math.Cos(th)), int(cy+(r-t)*math.Sin(th)), green)
		}
	}
	return out
}

func encodePNG(img image.Image) ([]byte, error) {
	var b bytes.Buffer
	err := png.Encode(&b, img)
	return b.Bytes(), err
}
