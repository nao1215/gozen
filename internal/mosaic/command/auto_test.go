package command

import (
	"bytes"
	"errors"
	"image"
	"image/color"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nao1215/gozen/internal/mosaic"
	"github.com/nao1215/gozen/internal/mosaic/detect"
)

// averageOf is the mean color of the pixels of r, as the pixelation computes it (16-bit
// channels averaged, then cut to 8 bits).
func averageOf(img image.Image, r image.Rectangle) color.RGBA {
	var sr, sg, sb, sa, n uint64
	for y := r.Min.Y; y < r.Max.Y; y++ {
		for x := r.Min.X; x < r.Max.X; x++ {
			cr, cg, cb, ca := img.At(x, y).RGBA()
			sr, sg, sb, sa, n = sr+uint64(cr), sg+uint64(cg), sb+uint64(cb), sa+uint64(ca), n+1
		}
	}
	return color.RGBA{uint8(sr / n >> 8), uint8(sg / n >> 8), uint8(sb / n >> 8), uint8(sa / n >> 8)} //nolint:gosec // channel averages stay within 0..255
}

func TestAutoPixelatesTheDetectedRegionCellByCell(t *testing.T) {
	game, out := fixture(t)
	m := useModels(t)
	model, det := m.detector(t, "model.onnx", penis(4, 4, 10, 10, 0.9))
	code, stdout, diag := invoke(t, "auto", "--model", model, "-o", out, game, "pattern.png")
	if code != 0 {
		t.Fatalf("exit %d: %s", code, diag)
	}
	if det.calls != 1 || det.closed != 1 {
		t.Fatalf("detector used %d times, closed %d times", det.calls, det.closed)
	}
	if len(det.passes) != len(detect.DefaultPasses) {
		t.Fatalf("passes %d, want the default %d", len(det.passes), len(detect.DefaultPasses))
	}
	if !strings.Contains(stdout, "[1/1] pattern.png 1 箇所") || !strings.Contains(stdout, "1 枚中 1 枚にモザイク") {
		t.Fatalf("progress lines: %q", stdout)
	}
	es := readEntries(t, out)
	if len(es) != 1 {
		t.Fatalf("%d report entries", len(es))
	}
	e := es[0]
	if e.File != "pattern.png" || e.Output != "pattern.png" || e.Block != 4 || e.Size != [2]int{24, 24} {
		t.Fatalf("entry %+v", e)
	}
	if len(e.Regions) != 1 || e.Regions[0] != [4]int{4, 4, 6, 6} || len(e.Found) != 1 || e.Found[0].Label != "penis" {
		t.Fatalf("regions %v found %+v", e.Regions, e.Found)
	}
	if e.Source == "" || e.FixSum == "" {
		t.Fatal("the report must tie itself to the source picture and the fixes")
	}
	// Every cell the region touches is one flat color, the average of the original.
	orig := loadPNG(t, filepath.Join(game, "assets", "pattern.png"))
	masked := loadPNG(t, filepath.Join(out, "pattern.png"))
	b := orig.Bounds()
	cells := mosaic.CellsAt(b, []mosaic.Shape{mosaic.Box(image.Rect(4, 4, 10, 10))}, 4, image.Pt(e.Offset[0], e.Offset[1]))
	covered := image.Rectangle{}
	for _, c := range cells {
		want := averageOf(orig, c)
		for y := c.Min.Y; y < c.Max.Y; y++ {
			for x := c.Min.X; x < c.Max.X; x++ {
				if got := color.RGBAModel.Convert(masked.At(x, y)); got != color.Color(want) {
					t.Fatalf("pixel (%d,%d) of cell %v is %v, want the average %v", x, y, c, got, want)
				}
			}
		}
		covered = covered.Union(c)
	}
	if !image.Rect(4, 4, 10, 10).In(covered) {
		t.Fatalf("the cells %v do not cover the region", covered)
	}
	// Outside the cells the picture is as it was.
	for y := range 24 {
		for x := range 24 {
			p := image.Pt(x, y)
			inside := false
			for _, c := range cells {
				inside = inside || p.In(c)
			}
			if !inside && !samePixels(orig, masked, image.Rect(x, y, x+1, y+1)) {
				t.Fatalf("pixel %v outside the cells changed", p)
			}
		}
	}
	// The mask of the cells is written beside the output.
	if _, err := os.Stat(maskPath(out, "pattern.png")); err != nil {
		t.Fatal("no mask written")
	}
	// The original is never changed.
	after := loadPNG(t, filepath.Join(game, "assets", "pattern.png"))
	if !samePixels(orig, after, b) {
		t.Fatal("the original changed")
	}
}

func TestAutoWithNothingFoundWritesNoPicture(t *testing.T) {
	game, out := fixture(t)
	m := useModels(t)
	model, _ := m.detector(t, "model.onnx")
	code, stdout, diag := invoke(t, "auto", "--model", model, "-o", out, game, "pattern.png")
	if code != 0 {
		t.Fatal(diag)
	}
	if !strings.Contains(stdout, "[1/1] pattern.png -") || !strings.Contains(stdout, "1 枚中 0 枚") {
		t.Fatalf("stdout %q", stdout)
	}
	if _, err := os.Stat(filepath.Join(out, "pattern.png")); !os.IsNotExist(err) {
		t.Fatal("a picture without regions was written")
	}
	es := readEntries(t, out)
	if len(es) != 1 || es[0].Output != "" || len(es[0].Regions) != 0 {
		t.Fatalf("entry %+v", es)
	}
}

func TestAutoThresholdsAndLabels(t *testing.T) {
	game, out := fixture(t)
	m := useModels(t)
	model, _ := m.detector(t, "model.onnx",
		penis(2, 2, 6, 6, 0.8),
		pussy(14, 14, 18, 18, 0.3), // below the threshold
		detect.Box{Rect: image.Rect(10, 2, 14, 6), Label: "nipple_f", Score: 0.9, Pass: "p"}, // not a default target
	)
	code, _, diag := invoke(t, "auto", "--model", model, "-o", out, game, "pattern.png")
	if code != 0 {
		t.Fatal(diag)
	}
	e := readEntries(t, out)[0]
	if len(e.Found) != 1 || e.Found[0].Label != "penis" {
		t.Fatalf("found %+v (only penis and pussy at 0.5 or more are pixelated by default)", e.Found)
	}
	// Lower threshold and more labels find more.
	out2 := filepath.Join(filepath.Dir(out), "out2")
	code, _, diag = invoke(t, "auto", "--model", model, "--threshold", "0.2", "--sure", "0.2", "--labels", "penis,pussy,nipple_f", "-o", out2, game, "pattern.png")
	if code != 0 {
		t.Fatal(diag)
	}
	if got := len(readEntries(t, out2)[0].Found); got != 3 {
		t.Fatalf("found %d, want 3", got)
	}
}

func TestAutoLowConfidenceGoesToReview(t *testing.T) {
	game, out := fixture(t)
	m := useModels(t)
	model, _ := m.detector(t, "model.onnx", penis(4, 4, 10, 10, 0.6))
	code, stdout, diag := invoke(t, "auto", "--model", model, "--sure", "0.9", "-o", out, game, "pattern.png")
	if code != 0 {
		t.Fatal(diag)
	}
	e := readEntries(t, out)[0]
	if e.Output == "" || len(e.Review) != 1 || !strings.Contains(e.Review[0], "low confidence: penis 0.60") {
		t.Fatalf("entry %+v", e)
	}
	if !strings.Contains(stdout, "要確認") || !strings.Contains(stdout, "要確認 1 枚") {
		t.Fatalf("stdout %q", stdout)
	}
	md := readFile(t, filepath.Join(out, "review.md"))
	if !strings.Contains(string(md), "pattern.png") || !strings.Contains(string(md), "sheet_01.jpg") {
		t.Fatalf("review.md: %q", md)
	}
	sheet := loadPNG(t, filepath.Join(out, "review", "sheet_01.jpg"))
	if sheet.Bounds().Dx() != sheetCols*thumbW || sheet.Bounds().Dy() != sheetRows*thumbH {
		t.Fatalf("sheet %v", sheet.Bounds())
	}
	// The list of what to look at is the same set.
	_, listed, _ := invoke(t, "list", "-o", out, game)
	if !strings.Contains(listed, `"file":"pattern.png"`) || !strings.Contains(listed, `"masked":true`) {
		t.Fatalf("manual list: %q", listed)
	}
	// Checking it clears the review.
	manual := filepath.Join(filepath.Dir(out), "manual.yaml")
	code, _, diag = invoke(t, "mark", "--checked", "--model", model, "--sure", "0.9", "--manual", manual, "-o", out, game, "pattern.png")
	if code != 0 {
		t.Fatal(diag)
	}
	if e := readEntries(t, out)[0]; len(e.Review) != 0 {
		t.Fatalf("review after check: %v", e.Review)
	}
}

func TestAutoDropAddAndGrowFixes(t *testing.T) {
	game, out := fixture(t)
	m := useModels(t)
	model, _ := m.detector(t, "model.onnx", penis(2, 2, 6, 6, 0.9), pussy(14, 14, 18, 18, 0.9))
	fixes := filepath.Join(filepath.Dir(out), "fixes.yaml")
	if err := (mosaic.Fixes{"pattern.png": {Drop: [][4]int{{13, 13, 6, 6}}, Add: [][4]int{{20, 0, 4, 4}}}}).Save(fixes, ""); err != nil {
		t.Fatal(err)
	}
	code, stdout, diag := invoke(t, "auto", "--model", model, "--fix", fixes, "-o", out, game, "pattern.png")
	if code != 0 {
		t.Fatal(diag)
	}
	e := readEntries(t, out)[0]
	if len(e.Found) != 1 || e.Found[0].Label != "penis" || len(e.Adds) != 1 || !e.Fixed {
		t.Fatalf("entry %+v", e)
	}
	if !strings.Contains(stdout, "2 箇所（自動 1・手 1）") {
		t.Fatalf("stdout %q", stdout)
	}
	// A grow box widens the region it meets.
	if err := (mosaic.Fixes{"pattern.png": {Grow: [][8]int{{2, 2, 4, 4, 2, 2, 2, 2}}}}).Save(fixes, ""); err != nil {
		t.Fatal(err)
	}
	out2 := filepath.Join(filepath.Dir(out), "grown")
	code, _, diag = invoke(t, "auto", "--model", model, "--fix", fixes, "-o", out2, game, "pattern.png")
	if code != 0 {
		t.Fatal(diag)
	}
	var grown [4]int
	for _, r := range readEntries(t, out2)[0].Regions {
		if r[0] <= 2 && r[1] <= 2 {
			grown = r
		}
	}
	if grown != [4]int{0, 0, 8, 8} {
		t.Fatalf("grown region %v, want the 4x4 box grown by 2 on every side", grown)
	}
}

func TestAutoSkipNeedsNoModelAndAvoidsTheDetector(t *testing.T) {
	game, out := fixture(t)
	m := useModels(t)
	model, det := m.detector(t, "model.onnx", penis(4, 4, 10, 10, 0.9))
	fixes := filepath.Join(filepath.Dir(out), "fixes.yaml")
	if err := (mosaic.Fixes{"pattern.png": {Skip: true, Add: [][4]int{{0, 0, 8, 8}}}}).Save(fixes, ""); err != nil {
		t.Fatal(err)
	}
	code, _, diag := invoke(t, "auto", "--model", model, "--fix", fixes, "-o", out, game, "pattern.png")
	if code != 0 || det.calls != 0 || len(m.opened) != 0 {
		t.Fatalf("a skipped picture opened the model: code %d calls %d opened %v %s", code, det.calls, m.opened, diag)
	}
	e := readEntries(t, out)[0]
	if len(e.Found) != 0 || len(e.Regions) != 0 || len(e.Adds) != 1 || e.Output == "" {
		t.Fatalf("entry %+v", e)
	}
}

func TestAutoDetectorFailureNamesThePicture(t *testing.T) {
	game, out := fixture(t)
	m := useModels(t)
	model, det := m.detector(t, "model.onnx")
	det.err = errors.New("the session broke")
	code, _, diag := invoke(t, "auto", "--model", model, "-o", out, game, "pattern.png")
	if code != 1 || !strings.Contains(diag, "pattern.png: the session broke") {
		t.Fatalf("code %d: %s", code, diag)
	}
	if det.closed != 1 {
		t.Fatal("the model was not closed after the failure")
	}
	if _, err := os.Stat(filepath.Join(out, "report.json")); !os.IsNotExist(err) {
		t.Fatal("a failed run wrote a report")
	}
	// A model that cannot be opened is reported as such.
	delete(m.detectors, "model.onnx")
	code, _, diag = invoke(t, "auto", "--model", model, "-o", out, game, "pattern.png")
	if code != 1 || !strings.Contains(diag, "fake: no such model") {
		t.Fatalf("code %d: %s", code, diag)
	}
}

func TestAutoWarnsOfASmallBlock(t *testing.T) {
	game, out := fixture(t)
	m := useModels(t)
	model, _ := m.detector(t, "model.onnx", penis(4, 4, 10, 10, 0.9))
	// The standard block of this 24 pixel picture is 4; a smaller one warns.
	code, _, diag := invoke(t, "auto", "--model", model, "--block", "2", "-o", out, game, "pattern.png")
	if code != 0 || !strings.Contains(diag, "warning") || !strings.Contains(diag, "2px") {
		t.Fatalf("code %d: %q", code, diag)
	}
	if e := readEntries(t, out)[0]; e.Block != 2 {
		t.Fatalf("block %d", e.Block)
	}
	// A fix file's block wins over --block.
	fixes := filepath.Join(filepath.Dir(out), "fixes.yaml")
	if err := (mosaic.Fixes{"pattern.png": {Block: 6}}).Save(fixes, ""); err != nil {
		t.Fatal(err)
	}
	out2 := filepath.Join(filepath.Dir(out), "out2")
	if code, _, diag := invoke(t, "auto", "--model", model, "--block", "2", "--fix", fixes, "-o", out2, game, "pattern.png"); code != 0 || strings.Contains(diag, "warning") {
		t.Fatalf("code %d: %q", code, diag)
	}
	if e := readEntries(t, out2)[0]; e.Block != 6 {
		t.Fatalf("block %d", e.Block)
	}
}

func TestAutoFaceContainedDetectionGoesToReview(t *testing.T) {
	game, out := fixture(t)
	m := useModels(t)
	model, _ := m.detector(t, "model.onnx", pussy(10, 10, 14, 14, 0.9), penis(15, 15, 18, 18, 0.9))
	faceModel, _ := m.detector(t, "face.onnx", detect.Box{Rect: image.Rect(8, 8, 22, 22), Label: "face", Score: 0.9, Pass: "p"})
	code, _, diag := invoke(t, "auto", "--model", model, "--face", faceModel, "-o", out, game, "pattern.png")
	if code != 0 {
		t.Fatal(diag)
	}
	e := readEntries(t, out)[0]
	// A penis inside a face is still pixelated; anything else inside a face is only listed.
	if len(e.Found) != 1 || e.Found[0].Label != "penis" || len(e.Skipped) != 1 || e.Skipped[0].Label != "pussy" {
		t.Fatalf("found %+v skipped %+v", e.Found, e.Skipped)
	}
	if len(e.Faces) != 1 || len(e.Review) == 0 || !strings.Contains(e.Review[0], "face-contained") {
		t.Fatalf("faces %v review %v", e.Faces, e.Review)
	}
}

func TestAutoSeveralPicturesAndMissingOnes(t *testing.T) {
	game, out := fixture(t)
	assets := filepath.Join(game, "assets")
	if err := os.MkdirAll(filepath.Join(assets, "sub"), 0o750); err != nil {
		t.Fatal(err)
	}
	img := image.NewRGBA(image.Rect(0, 0, 30, 20))
	if err := encode(filepath.Join(assets, "sub", "second.png"), img, 90); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(game, "scene.yaml"), []byte("pics: [assets/pattern.png, assets/sub/second.png, assets/gone.png]\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	// A fix for a picture that is not there only warns.
	fixes := filepath.Join(filepath.Dir(out), "fixes.yaml")
	if err := (mosaic.Fixes{"nowhere.png": {Skip: true}, "pattern.png": {Skip: true}, "sub/second.png": {Skip: true}}).Save(fixes, ""); err != nil {
		t.Fatal(err)
	}
	m := useModels(t)
	model, _ := m.detector(t, "model.onnx")
	code, stdout, diag := invoke(t, "auto", "--model", model, "--fix", fixes, "-o", out, game)
	if code != 0 {
		t.Fatal(diag)
	}
	if !strings.Contains(diag, "assets/gone.png") || !strings.Contains(diag, "assets/nowhere.png") {
		t.Fatalf("missing pictures are not reported: %q", diag)
	}
	if !strings.Contains(stdout, "[1/2]") || !strings.Contains(stdout, "[2/2] sub/second.png") {
		t.Fatalf("the pictures the YAML names were not all processed: %q", stdout)
	}
	// Running again for one picture keeps the other's entry in the report.
	code, _, diag = invoke(t, "auto", "--model", model, "--fix", fixes, "-o", out, game, "pattern.png")
	if code != 0 {
		t.Fatal(diag)
	}
	es := readEntries(t, out)
	if len(es) != 2 || es[0].File != "pattern.png" || es[1].File != "sub/second.png" {
		t.Fatalf("report %+v", es)
	}
}

func TestAutoReportDropsOutputsThatNoLongerApply(t *testing.T) {
	game, out := fixture(t)
	m := useModels(t)
	model, det := m.detector(t, "model.onnx", penis(4, 4, 10, 10, 0.9))
	if code, _, diag := invoke(t, "auto", "--model", model, "-o", out, game, "pattern.png"); code != 0 {
		t.Fatal(diag)
	}
	if _, err := os.Stat(filepath.Join(out, "pattern.png")); err != nil {
		t.Fatal(err)
	}
	// The detector finds nothing now: the earlier output and its mask are removed.
	det.boxes = nil
	if code, _, diag := invoke(t, "auto", "--model", model, "-o", out, game, "pattern.png"); code != 0 {
		t.Fatal(diag)
	}
	if _, err := os.Stat(filepath.Join(out, "pattern.png")); !os.IsNotExist(err) {
		t.Fatal("a stale output stayed")
	}
	if _, err := os.Stat(maskPath(out, "pattern.png")); !os.IsNotExist(err) {
		t.Fatal("a stale mask stayed")
	}
	// A picture that left the game takes its output with it.
	det.boxes = []detect.Box{penis(4, 4, 10, 10, 0.9)}
	if code, _, diag := invoke(t, "auto", "--model", model, "-o", out, game, "pattern.png"); code != 0 {
		t.Fatal(diag)
	}
	if err := os.Rename(filepath.Join(game, "assets", "pattern.png"), filepath.Join(game, "assets", "renamed.png")); err != nil {
		t.Fatal(err)
	}
	if code, _, diag := invoke(t, "auto", "--model", model, "-o", out, game, "renamed.png"); code != 0 {
		t.Fatal(diag)
	}
	es := readEntries(t, out)
	if len(es) != 1 || es[0].File != "renamed.png" {
		t.Fatalf("report %+v", es)
	}
	if _, err := os.Stat(filepath.Join(out, "pattern.png")); !os.IsNotExist(err) {
		t.Fatal("the output of a vanished picture stayed")
	}
}

func TestAutoDryRunWritesNothing(t *testing.T) {
	game, out := fixture(t)
	m := useModels(t)
	model, det := m.detector(t, "model.onnx", penis(4, 4, 10, 10, 0.9))
	code, stdout, diag := invoke(t, "auto", "-n", "--model", model, "-o", out, game, "pattern.png")
	if code != 0 {
		t.Fatal(diag)
	}
	if det.calls != 1 || !strings.Contains(stdout, "1 箇所") || !strings.Contains(diag, "classes: nipple_f,penis,pussy (default)") || !strings.Contains(diag, "pixelated: penis,pussy") {
		t.Fatalf("calls %d stdout %q diag %q", det.calls, stdout, diag)
	}
	if _, err := os.Stat(out); !os.IsNotExist(err) {
		t.Fatal("dry run created the output")
	}
}

func TestAutoNeedsAGameDirectory(t *testing.T) {
	code, _, diag := invoke(t, "auto")
	if code != 2 || !strings.Contains(diag, "expected a game directory") {
		t.Fatalf("code %d: %s", code, diag)
	}
	code, _, diag = invoke(t, "inspect")
	if code != 2 || !strings.Contains(diag, "expected a game directory") {
		t.Fatalf("code %d: %s", code, diag)
	}
}

func TestRunExitCodesAndOutputFailures(t *testing.T) {
	game, out := fixture(t)
	m := useModels(t)
	model, _ := m.detector(t, "model.onnx", penis(4, 4, 10, 10, 0.9))
	var sink bytes.Buffer
	if code := runCommand([]string{"auto", "--model", model, "-o", out, game, "pattern.png"}, failedWriter{}, &sink); code != 1 {
		t.Fatalf("a failing stdout gave %d", code)
	}
	// A usage error stays a usage error (2) even when the message cannot be written.
	if code := runCommand([]string{"nope"}, &sink, failedWriter{}); code != 1 {
		t.Fatalf("a failing diagnostics writer gave %d", code)
	}
}
