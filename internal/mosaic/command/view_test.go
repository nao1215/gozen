package command

import (
	"encoding/json"
	"fmt"
	"image"
	"image/color"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nao1215/gozen/internal/mosaic"
)

func TestManualShowAndListReportWhatTheReportSays(t *testing.T) {
	e := newManualEnv(t, penis(4, 4, 10, 10, 0.6))
	// Before any run there is no report: show says so, list is empty.
	out := e.ok("show")
	var shown map[string]any
	if err := json.Unmarshal([]byte(out), &shown); err != nil || shown["file"] != "pattern.png" || !strings.Contains(fmt.Sprint(shown["report"]), "report.json に無い") {
		t.Fatalf("show before a run: %v %v", shown, err)
	}
	code, listed, _ := invoke(t, "manual", "list", "-o", e.out, e.game)
	if code != 0 || listed != "" {
		t.Fatalf("list before a run: %d %q", code, listed)
	}
	code, _, diag := invoke(t, "auto", "--model", e.model, "--sure", "0.9", "--manual", e.manual, "-o", e.out, e.game, "pattern.png")
	if code != 0 {
		t.Fatal(diag)
	}
	e.ok("add", "--sure", "0.9", "--rect", "18,18,4,4")
	out = e.ok("show")
	var s struct {
		File   string
		Manual mosaic.Fix
		Fix    mosaic.Fix
		Report entry
	}
	if err := json.Unmarshal([]byte(out), &s); err != nil {
		t.Fatalf("%v\n%s", err, out)
	}
	if s.File != "pattern.png" || len(s.Manual.Add) != 1 || len(s.Fix.Add) != 1 || len(s.Report.Regions) != 1 {
		t.Fatalf("show %+v", s)
	}
	if !strings.Contains(out, "\n  \"") {
		t.Fatalf("show is indented JSON: %q", out)
	}
	// list names the pictures under review with the reasons, one JSON line each.
	code, listed, diag = invoke(t, "manual", "list", "--manual", e.manual, "-o", e.out, e.game)
	if code != 0 {
		t.Fatal(diag)
	}
	var line struct {
		File   string
		Review []string
		Masked bool
	}
	if err := json.Unmarshal([]byte(strings.TrimSpace(listed)), &line); err != nil || line.File != "pattern.png" || len(line.Review) == 0 || !line.Masked {
		t.Fatalf("list %q %v", listed, err)
	}
}

func TestManualViewDrawsTheImageWithItsScale(t *testing.T) {
	e := newManualEnv(t, penis(4, 4, 10, 10, 0.9))
	e.auto()
	out := e.ok("view", "--size", "240")
	if !strings.Contains(out, "view:") || !strings.Contains(out, filepath.Join(e.out, "manual", "pattern.png.original.png")) {
		t.Fatalf("stdout %q", out)
	}
	view := loadPNG(t, filepath.Join(e.out, "manual", "pattern.png.original.png"))
	// 24 pixels at 240: scale 10, with a ruler above and to the left.
	if view.Bounds().Dx() != 240+rulerLeft || view.Bounds().Dy() != 240+rulerTop {
		t.Fatalf("view %v", view.Bounds())
	}
	// The automatic region A1 is outlined in yellow where (4,4) of the original lies.
	if got := color.RGBAModel.Convert(view.At(rulerLeft+4*10, rulerTop+4*10+30)); got != color.Color(color.RGBA{255, 220, 0, 255}) {
		t.Fatalf("outline of A1: %v", got)
	}
	// masked draws the output and tints the pixelated cells; a zoom shows only a part.
	e.ok("view", "--src", "masked", "--size", "100", "--region", "0,0,12,12")
	masked := loadPNG(t, filepath.Join(e.out, "manual", "pattern.png.masked.png"))
	if masked.Bounds().Dx() != 100+rulerLeft || masked.Bounds().Dy() != 100+rulerTop {
		t.Fatalf("zoomed view %v", masked.Bounds())
	}
	// A normalized region.
	e.ok("view", "--norm", "--region", "0,0,0.5,0.5", "--size", "48")
	// An explicit output file and step.
	custom := filepath.Join(t.TempDir(), "mine.png")
	if out := e.ok("view", "--view-out", custom, "--step", "6", "--size", "120"); !strings.Contains(out, custom) {
		t.Fatalf("stdout %q", out)
	}
	if _, err := os.Stat(custom); err != nil {
		t.Fatal("view-out not written")
	}
}

func TestManualViewMayNotReplaceSourcesOrSettings(t *testing.T) {
	e := newManualEnv(t, penis(4, 4, 10, 10, 0.9))
	source := filepath.Join(e.game, "assets", "pattern.png")
	for name, dst := range map[string]string{
		"the source":        source,
		"inside the assets": filepath.Join(e.game, "assets", "view.png"),
		"the manual file":   e.manual,
	} {
		before := readFile(t, source)
		code, _, diag := e.call("view", "--view-out", dst)
		if code != 1 || !strings.Contains(diag, "view output cannot replace") {
			t.Errorf("%s: code %d: %s", name, code, diag)
		}
		after := readFile(t, source)
		if string(before) != string(after) {
			t.Fatalf("%s: the source changed", name)
		}
	}
	if _, err := os.Stat(e.manual); !os.IsNotExist(err) {
		t.Fatal("the manual file was created by a refused view")
	}
}

func TestManualViewOfAFileOutsideTheAssets(t *testing.T) {
	e := newManualEnv(t)
	other := filepath.Join(t.TempDir(), "elsewhere.png")
	if err := encode(other, image.NewRGBA(image.Rect(0, 0, 20, 10)), 90); err != nil {
		t.Fatal(err)
	}
	code, out, diag := invoke(t, "manual", "view", "-o", e.out, "--size", "40", e.game, other)
	if code != 0 {
		t.Fatal(diag)
	}
	if !strings.Contains(out, filepath.Join(e.out, "manual", "elsewhere.original.png")) {
		t.Fatalf("stdout %q", out)
	}
}

func TestRenderViewDrawsTheOverlays(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 40, 40))
	for i := range img.Pix {
		img.Pix[i] = 200
	}
	painted := image.NewAlpha(img.Bounds())
	painted.SetAlpha(30, 30, color.Alpha{A: 255})
	ov := overlay{
		auto:    [][4]int{{4, 4, 10, 10}},
		add:     [][4]int{{20, 4, 10, 10}},
		poly:    [][]int{{4, 20, 14, 20, 9, 34}},
		painted: painted,
	}
	v := renderView(img, img.Bounds(), 400, 0, ov)
	if v.Bounds().Dx() != 400+rulerLeft || v.Bounds().Dy() != 400+rulerTop {
		t.Fatalf("size %v", v.Bounds())
	}
	at := func(x, y int) color.RGBA { return v.RGBAAt(rulerLeft+x*10, rulerTop+y*10) }
	if got := at(4, 9); got != (color.RGBA{255, 220, 0, 255}) {
		t.Errorf("automatic region outline %v", got)
	}
	if got := at(20, 9); got != (color.RGBA{0, 220, 255, 255}) {
		t.Errorf("added region outline %v", got)
	}
	// The pixelated cell is tinted toward yellow-green: its red and green rise, blue falls.
	if p, q := v.RGBAAt(rulerLeft+30*10+5, rulerTop+30*10+5), v.RGBAAt(rulerLeft+37*10+5, rulerTop+37*10+5); p.R <= q.R || p.B >= q.B {
		t.Errorf("painted cell %v against %v", p, q)
	}
	// A scale of 2 or more samples nearest; below it, smoothly (no panic and the same shape).
	small := renderView(img, img.Bounds(), 40, 0, overlay{})
	if small.Bounds().Dx() != 40+rulerLeft {
		t.Fatalf("small %v", small.Bounds())
	}
	// A zoom reads the region only.
	zoom := renderView(img, image.Rect(10, 10, 20, 20), 100, 5, overlay{auto: [][4]int{{0, 0, 5, 5}}})
	if zoom.Bounds().Dx() != 100+rulerLeft {
		t.Fatalf("zoom %v", zoom.Bounds())
	}
}

func TestViewHelpers(t *testing.T) {
	for in, want := range map[int]int{-5: 1, 0: 1, 1: 1, 2: 2, 3: 5, 5: 5, 6: 10, 11: 20, 21: 50, 51: 100, 999: 1000} {
		if got := niceStep(in); got != want {
			t.Errorf("niceStep(%d) = %d, want %d", in, got, want)
		}
	}
	if got := clipRect([]int{-2, 5, 10, 10}, image.Rect(0, 0, 24, 24)); got != [4]int{0, 5, 8, 10} {
		t.Fatalf("clipRect %v", got)
	}
	if got := clipRect([]int{20, 20, 10, 10}, image.Rect(0, 0, 24, 24)); got != [4]int{20, 20, 4, 4} {
		t.Fatalf("clipRect %v", got)
	}
	if got := clipRect([]int{5, 5, -3, -3}, image.Rect(0, 0, 24, 24)); got != [4]int{2, 2, 3, 3} {
		t.Fatalf("a negative size is the same box turned around: %v", got)
	}
	if abs(-3) != 3 || abs(3) != 3 || abs(0) != 0 {
		t.Fatal("abs")
	}
}

func TestEllipseCoversTheEllipse(t *testing.T) {
	pts := ellipse(50, 40, 20, 10, 48)
	if len(pts) != 96 {
		t.Fatalf("%d numbers", len(pts))
	}
	var pg mosaic.Polygon
	for i := 0; i+1 < len(pts); i += 2 {
		pg = append(pg, image.Pt(pts[i], pts[i+1]))
	}
	// Every point of the ellipse (a hair inside) is inside the polygon: the polygon is drawn
	// around the ellipse, so that the blocks it touches cover the outline.
	for k := range 360 {
		a := float64(k) * math.Pi / 180
		x, y := 50+19.5*math.Cos(a), 40+9.75*math.Sin(a)
		if !pg.Contains(x, y) {
			t.Fatalf("(%.1f, %.1f) at %d degrees is outside the polygon", x, y, k)
		}
	}
	// And it is not much larger than the ellipse.
	if b := pg.Bounds(); b.Dx() > 20*2+4 || b.Dy() > 10*2+4 {
		t.Fatalf("polygon bounds %v", b)
	}
}

func TestCoordsFlagParsing(t *testing.T) {
	var c coords
	if c.Type() != "coords" {
		t.Fatal("Type")
	}
	for _, ok := range []string{"1,2,3,4", "1 2 3 4", "1,\t2 , 3,4", "-1.5,0,1e3,4"} {
		if err := c.Set(ok); err != nil {
			t.Errorf("%q: %v", ok, err)
		}
	}
	if len(c) != 4 || len(c[0]) != 4 || c[3][2] != 1000 {
		t.Fatalf("parsed %v", c)
	}
	if !strings.Contains(c.String(), "1.5") {
		t.Fatalf("String %q", c.String())
	}
	for _, bad := range []string{"", " , ", "a", "1,,x", "NaN", "Inf", "-Inf", "1000001", "-1000001"} {
		if err := (&coords{}).Set(bad); err == nil {
			t.Errorf("%q accepted", bad)
		}
	}
}
