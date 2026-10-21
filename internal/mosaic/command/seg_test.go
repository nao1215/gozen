package command

import (
	"image"
	"image/color"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nao1215/gozen/internal/mosaic"
	"github.com/nao1215/gozen/internal/mosaic/segment"
)

// maskOf is a mask the size of r with the pixels of fill set.
func maskOf(r image.Rectangle, fill image.Rectangle) *image.Alpha {
	a := image.NewAlpha(r)
	for y := fill.Min.Y; y < fill.Max.Y; y++ {
		for x := fill.Min.X; x < fill.Max.X; x++ {
			if (image.Point{X: x, Y: y}).In(r) {
				a.SetAlpha(x, y, color.Alpha{A: 255})
			}
		}
	}
	return a
}

func segObject(label string, score float32, r image.Rectangle, fill image.Rectangle) segment.Object {
	return segment.Object{Label: label, Score: score, Rect: r, Mask: maskOf(r, fill)}
}

// maskRun runs auto in mask mode with the fakes and returns the entry and the outputs.
func maskRun(t *testing.T, model string, extra ...string) (entry, image.Image, image.Image) {
	t.Helper()
	game, out := fixture(t)
	args := append([]string{"auto", "--model", model, "--shape", "mask", "--seg", "seg.onnx", "-o", out}, extra...)
	args = append(args, game, "pattern.png")
	code, _, diag := invoke(t, args...)
	if code != 0 {
		t.Fatalf("exit %d: %s", code, diag)
	}
	e := readEntries(t, out)[0]
	orig := loadPNG(t, filepath.Join(game, "assets", "pattern.png"))
	var masked image.Image
	if e.Output != "" {
		masked = loadPNG(t, filepath.Join(out, e.Output))
	}
	return e, orig, masked
}

func TestMaskShapeFollowsTheSegmentationMask(t *testing.T) {
	box := image.Rect(4, 4, 10, 10)
	m := useModels(t)
	model, _ := m.detector(t, "model.onnx", penis(4, 4, 10, 10, 0.9))
	m.seg = &fakeSegmenter{objects: []segment.Object{segObject("penis", 0.8, box, box)}}
	e, orig, masked := maskRun(t, model, "--dilate", "0", "--block", "2")
	if len(e.Shapes) != 1 || !strings.HasPrefix(e.Shapes[0], "mask（分割モデルの性器が枠の 100%") {
		t.Fatalf("shapes %v", e.Shapes)
	}
	if e.Found[0].Shape != e.Shapes[0] {
		t.Fatalf("the found object should say how it was painted: %+v", e.Found)
	}
	// With no dilation only the cells of the mask itself are painted: (6,6) is, (12,12) is not.
	if samePixels(orig, masked, image.Rect(6, 6, 7, 7)) {
		t.Fatal("the mask area was not pixelated")
	}
	if !samePixels(orig, masked, image.Rect(12, 12, 14, 14)) {
		t.Fatal("outside the mask was pixelated")
	}
	if m.seg.closed != 1 {
		t.Fatal("the segmentation model was not closed")
	}
}

func TestMaskDilationGrowsTheMask(t *testing.T) {
	box := image.Rect(4, 4, 10, 10)
	m := useModels(t)
	model, _ := m.detector(t, "model.onnx", penis(4, 4, 10, 10, 0.9))
	m.seg = &fakeSegmenter{objects: []segment.Object{segObject("penis", 0.8, box, box)}}
	// The default dilation is one block: the mask grows 4 pixels on every side.
	e, orig, masked := maskRun(t, model)
	if len(e.Shapes) != 1 || !strings.HasPrefix(e.Shapes[0], "mask") {
		t.Fatalf("shapes %v", e.Shapes)
	}
	if samePixels(orig, masked, image.Rect(11, 6, 12, 7)) {
		t.Fatal("the grown mask did not reach 2 pixels beyond the box")
	}
	if !samePixels(orig, masked, image.Rect(18, 18, 24, 24)) {
		t.Fatal("far pixels changed")
	}
}

func TestMaskFallsBackToTheBoxWhenTheMaskIsThin(t *testing.T) {
	box := image.Rect(4, 4, 10, 10)
	thin := image.Rect(4, 4, 5, 10) // one column of six: 1/6 of the box
	m := useModels(t)
	model, _ := m.detector(t, "model.onnx", penis(4, 4, 10, 10, 0.9))
	m.seg = &fakeSegmenter{objects: []segment.Object{segObject("penis", 0.8, box, thin)}}
	e, orig, masked := maskRun(t, model, "--dilate", "0", "--block", "2")
	if len(e.Shapes) != 1 || !strings.HasPrefix(e.Shapes[0], "box（分割モデルの性器が枠の 17%") {
		t.Fatalf("shapes %v", e.Shapes)
	}
	if samePixels(orig, masked, image.Rect(8, 8, 9, 9)) {
		t.Fatal("the whole box should be pixelated")
	}
	if m.seg.calls != 4 {
		t.Fatalf("two passes for the picture, two more zoomed: %d calls", m.seg.calls)
	}
}

func TestMaskZoomRescuesAThinMask(t *testing.T) {
	box := image.Rect(4, 4, 10, 10)
	thin := image.Rect(4, 4, 5, 10)
	m := useModels(t)
	model, _ := m.detector(t, "model.onnx", penis(4, 4, 10, 10, 0.9))
	// The first two calls (the picture, flipped and not) see little; the zoomed ones see the whole.
	m.seg = &fakeSegmenter{
		sequence: [][]segment.Object{{segObject("penis", 0.8, box, thin)}, {segObject("penis", 0.8, box, thin)}},
		objects:  []segment.Object{segObject("penis", 0.7, box, box)},
	}
	e, _, _ := maskRun(t, model, "--dilate", "0", "--block", "2")
	if len(e.Shapes) != 1 || !strings.HasPrefix(e.Shapes[0], "mask（拡大して分割モデルの性器 0.70 が枠の 100%") {
		t.Fatalf("shapes %v", e.Shapes)
	}
}

func TestMaskRequiresASegmentationModel(t *testing.T) {
	game, out := fixture(t)
	m := useModels(t)
	model, _ := m.detector(t, "model.onnx", penis(4, 4, 10, 10, 0.9))
	code, _, diag := invoke(t, "auto", "--model", model, "--shape", "mask", "-o", out, game, "pattern.png")
	if code != 1 || !strings.Contains(diag, "--shape mask requires a user-supplied --seg model") {
		t.Fatalf("code %d: %s", code, diag)
	}
	code, _, diag = invoke(t, "auto", "--model", model, "--shape", "circle", "-o", out, game, "pattern.png")
	if code == 0 {
		t.Fatalf("unknown shape accepted: %s", diag)
	}
	// A segmentation failure stops the run and names the picture.
	m.seg = &fakeSegmenter{err: os.ErrInvalid}
	code, _, diag = invoke(t, "auto", "--model", model, "--shape", "mask", "--seg", "seg.onnx", "-o", out, game, "pattern.png")
	if code != 1 || !strings.Contains(diag, "pattern.png") {
		t.Fatalf("code %d: %s", code, diag)
	}
}

func TestSegmentationAloneIsOnlyAHintWithoutTags(t *testing.T) {
	m := useModels(t)
	model, _ := m.detector(t, "model.onnx", penis(2, 2, 6, 6, 0.9))
	elsewhere := image.Rect(14, 14, 20, 20)
	m.seg = &fakeSegmenter{objects: []segment.Object{
		segObject("penis", 0.8, image.Rect(2, 2, 6, 6), image.Rect(2, 2, 6, 6)),
		segObject("penis", 0.8, elsewhere, elsewhere),
	}}
	e, orig, masked := maskRun(t, model, "--dilate", "0", "--block", "2")
	found := false
	for _, r := range e.Review {
		found = found || strings.Contains(r, "分割モデルだけが penis を見つけた") && strings.Contains(r, "タグが無い")
	}
	if !found {
		t.Fatalf("review %v", e.Review)
	}
	if !samePixels(orig, masked, elsewhere) {
		t.Fatal("a region only the segmentation model saw was pixelated without the tags to back it")
	}
}

func TestSegmentationAloneIsPixelatedWhenTheTagsAgree(t *testing.T) {
	game, out := fixture(t)
	m := useModels(t)
	model, _ := m.detector(t, "model.onnx", penis(2, 2, 6, 6, 0.9))
	elsewhere := image.Rect(14, 14, 20, 20)
	m.seg = &fakeSegmenter{objects: []segment.Object{
		segObject("penis", 0.8, image.Rect(2, 2, 6, 6), image.Rect(2, 2, 6, 6)),
		segObject("penis", 0.8, elsewhere, elsewhere),
	}}
	m.tagger = &fakeTagger{tags: map[string]float32{"penis": 0.9, "sensitive": 0.8}}
	tagDir := filepath.Join(t.TempDir(), "tagger")
	if err := os.MkdirAll(tagDir, 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(tagDir, "model.onnx"), []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	tags := filepath.Join(filepath.Dir(out), "tags.json")
	code, _, diag := invoke(t, "auto", "--model", model, "--shape", "mask", "--seg", "seg.onnx", "--dilate", "0", "--block", "2",
		"--tagger", tagDir, "--tags", tags, "-o", out, game, "pattern.png")
	if code != 0 {
		t.Fatalf("exit %d: %s", code, diag)
	}
	e := readEntries(t, out)[0]
	painted := false
	for _, s := range e.Shapes {
		painted = painted || strings.HasPrefix(s, "mask（分割モデルだけ、penis）")
	}
	if !painted {
		t.Fatalf("shapes %v", e.Shapes)
	}
	orig := loadPNG(t, filepath.Join(game, "assets", "pattern.png"))
	masked := loadPNG(t, filepath.Join(out, "pattern.png"))
	if samePixels(orig, masked, elsewhere) {
		t.Fatal("the region both the segmentation model and the tags saw was left alone")
	}
	review := strings.Join(e.Review, "\n")
	if !strings.Contains(review, "検出モデルは見落としたが分割モデルが penis を見つけた") {
		t.Fatalf("review %q", review)
	}
}

func TestSegmentationAloneHonorsDropAndGrow(t *testing.T) {
	elsewhere := image.Rect(14, 14, 20, 20)
	seg := func() *fakeSegmenter {
		return &fakeSegmenter{objects: []segment.Object{
			segObject("penis", 0.8, image.Rect(2, 2, 6, 6), image.Rect(2, 2, 6, 6)),
			segObject("penis", 0.8, elsewhere, elsewhere),
		}}
	}
	for name, c := range map[string]struct {
		fix     mosaic.Fix
		wantBox bool
		wantAny bool
	}{
		"drop": {mosaic.Fix{Drop: [][4]int{{13, 13, 8, 8}}}, false, false},
		"grow": {mosaic.Fix{Grow: [][8]int{{14, 14, 6, 6, 2, 2, 2, 2}}}, true, true},
	} {
		t.Run(name, func(t *testing.T) {
			game, out := fixture(t)
			m := useModels(t)
			model, _ := m.detector(t, "model.onnx", penis(2, 2, 6, 6, 0.9))
			m.seg = seg()
			m.tagger = &fakeTagger{tags: map[string]float32{"penis": 0.9, "sensitive": 0.8}}
			tagDir := t.TempDir()
			if err := os.WriteFile(filepath.Join(tagDir, "model.onnx"), []byte("x"), 0o600); err != nil {
				t.Fatal(err)
			}
			fixes := filepath.Join(filepath.Dir(out), "fixes.yaml")
			if err := (mosaic.Fixes{"pattern.png": c.fix}).Save(fixes, ""); err != nil {
				t.Fatal(err)
			}
			code, _, diag := invoke(t, "auto", "--model", model, "--shape", "mask", "--seg", "seg.onnx", "--dilate", "0", "--block", "2",
				"--tagger", tagDir, "--tags", filepath.Join(filepath.Dir(out), "tags.json"), "--fix", fixes, "-o", out, game, "pattern.png")
			if code != 0 {
				t.Fatalf("exit %d: %s", code, diag)
			}
			e := readEntries(t, out)[0]
			sawBox, sawAny := false, false
			for _, s := range e.Shapes {
				sawAny = sawAny || strings.Contains(s, "分割モデルだけ")
				sawBox = sawBox || strings.HasPrefix(s, "box（分割モデルだけ、penis、grow）")
			}
			if sawBox != c.wantBox || sawAny != c.wantAny {
				t.Fatalf("shapes %v", e.Shapes)
			}
		})
	}
}

func TestSegmentationFindsTheAnusOnlyWithTheAnalTag(t *testing.T) {
	anus := image.Rect(14, 14, 20, 20)
	for _, tagged := range []bool{false, true} {
		game, out := fixture(t)
		m := useModels(t)
		model, _ := m.detector(t, "model.onnx", penis(2, 2, 6, 6, 0.9))
		m.seg = &fakeSegmenter{objects: []segment.Object{
			segObject("penis", 0.8, image.Rect(2, 2, 6, 6), image.Rect(2, 2, 6, 6)),
			segObject("anus", 0.8, anus, anus),
		}}
		args := []string{"auto", "--model", model, "--shape", "mask", "--seg", "seg.onnx", "--dilate", "0", "--block", "2", "-o", out}
		if tagged {
			m.tagger = &fakeTagger{tags: map[string]float32{"anal": 0.9, "general": 0.1}}
			tagDir := t.TempDir()
			if err := os.WriteFile(filepath.Join(tagDir, "model.onnx"), []byte("x"), 0o600); err != nil {
				t.Fatal(err)
			}
			args = append(args, "--tagger", tagDir, "--tags", filepath.Join(filepath.Dir(out), "tags.json"))
		}
		code, _, diag := invoke(t, append(args, game, "pattern.png")...)
		if code != 0 {
			t.Fatalf("exit %d: %s", code, diag)
		}
		orig := loadPNG(t, filepath.Join(game, "assets", "pattern.png"))
		masked := loadPNG(t, filepath.Join(out, "pattern.png"))
		changed := !samePixels(orig, masked, anus)
		if changed != tagged {
			t.Fatalf("tagged=%v: the anus region changed=%v (it is pixelated only for the insertion or excretion tag)", tagged, changed)
		}
	}
}
