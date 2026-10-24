package command

import (
	"image"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nao1215/gozen/internal/mosaic"
	"github.com/nao1215/gozen/internal/mosaic/detect"
)

// manualEnv is a game with the fixture picture, a fake detector that sees one object, and
// the files the manual commands keep.
type manualEnv struct {
	t         *testing.T
	game, out string
	manual    string
	model     string
	det       *fakeDetector
	models    *fakeModels
}

func newManualEnv(t *testing.T, boxes ...detect.Box) *manualEnv {
	t.Helper()
	game, out := fixture(t)
	m := useModels(t)
	model, det := m.detector(t, "model.onnx", boxes...)
	return &manualEnv{t: t, game: game, out: out, manual: filepath.Join(filepath.Dir(game), "manual.yaml"), model: model, det: det, models: m}
}

// call runs a manual command on pattern.png with the environment's files.
func (e *manualEnv) call(sub string, flags ...string) (int, string, string) {
	e.t.Helper()
	args := []string{sub, "--model", e.model, "--manual", e.manual, "-o", e.out, "--size", "96"}
	args = append(args, flags...)
	args = append(args, e.game, "pattern.png")
	return invoke(e.t, args...)
}

func (e *manualEnv) ok(sub string, flags ...string) string {
	e.t.Helper()
	code, out, diag := e.call(sub, flags...)
	if code != 0 {
		e.t.Fatalf("manual %s %v: exit %d: %s", sub, flags, code, diag)
	}
	return out
}

func (e *manualEnv) fix() mosaic.Fix {
	e.t.Helper()
	fx, err := mosaic.LoadFixes(e.manual)
	if err != nil {
		e.t.Fatal(err)
	}
	return fx["pattern.png"]
}

func (e *manualEnv) auto() {
	e.t.Helper()
	code, _, diag := invoke(e.t, "auto", "--model", e.model, "--manual", e.manual, "-o", e.out, e.game, "pattern.png")
	if code != 0 {
		e.t.Fatal(diag)
	}
}

func TestManualAddGeometry(t *testing.T) {
	e := newManualEnv(t)
	e.ok("skip")
	e.ok("add", "--rect", "2,3,4,5", "--box", "10,10,14,16", "--poly", "0,0,6,0,3,6", "--ellipse", "12,12,3,2", "--note", "first", "--note", "second")
	fx := e.fix()
	if len(fx.Add) != 2 || fx.Add[0] != [4]int{2, 3, 4, 5} || fx.Add[1] != [4]int{10, 10, 4, 6} {
		t.Fatalf("add %v", fx.Add)
	}
	if len(fx.Poly) != 2 || len(fx.Poly[0]) != 6 || len(fx.Poly[1]) != 96 {
		t.Fatalf("polygons %d (an ellipse is a 48-gon)", len(fx.Poly))
	}
	if !fx.Skip || fx.Note != "second" {
		// A repeated flag keeps the last value of a string option.
		t.Fatalf("skip %v note %q", fx.Skip, fx.Note)
	}
	out := e.ok("add", "--rect", "0,0,2,2", "--note", "more")
	_ = out
	if fx = e.fix(); len(fx.Add) != 3 || fx.Note != "second more" {
		t.Fatalf("adds accumulate and notes join: %+v", fx)
	}
	// The output exists and has the adds (the model is skipped).
	if _, err := os.Stat(filepath.Join(e.out, "pattern.png")); err != nil {
		t.Fatal("manual add did not rebuild the image")
	}
	if len(readEntries(t, e.out)[0].Adds) != 3 {
		t.Fatal("report")
	}
	// A view of the result is drawn at once.
	if _, err := os.Stat(filepath.Join(e.out, "manual", "pattern.png.masked.png")); err != nil {
		t.Fatalf("no check image: %v", err)
	}
}

func TestManualNormalizedAndClippedCoordinates(t *testing.T) {
	e := newManualEnv(t)
	e.ok("skip")
	e.ok("add", "--norm", "--rect", "0.5,0.25,0.25,0.5")
	if got := e.fix().Add; len(got) != 1 || got[0] != [4]int{12, 6, 6, 12} {
		t.Fatalf("normalized rect %v", got)
	}
	e.ok("add", "--rect", "-4,20,10,10") // hangs out of the picture: clipped
	if got := e.fix().Add; got[1] != [4]int{0, 20, 6, 4} {
		t.Fatalf("clipped rect %v", got)
	}
	for _, bad := range [][]string{
		{"--norm", "--rect", "0,0,1.5,1"},
		{"--rect", "30,30,5,5"},               // outside
		{"--rect", "1,2,3"},                   // wrong count
		{"--poly", "0,0,4,4"},                 // two points
		{"--poly", "0,0,4,4,8,8,9"},           // odd count
		{"--poly", "0,0,4,4,8,8"},             // a line
		{"--ellipse", "5,5,0,3"},              // no radius
		{"--poly", "100,100,120,100,110,120"}, // outside
	} {
		if code, _, _ := e.call("add", bad...); code == 0 {
			t.Errorf("accepted %v", bad)
		}
	}
	if got := len(e.fix().Add); got != 2 {
		t.Fatalf("a refused command changed the file: %d adds", got)
	}
}

func TestManualCommandArgumentErrors(t *testing.T) {
	e := newManualEnv(t)
	game := e.game
	outside := filepath.Join(t.TempDir(), "x.png")
	if err := encode(outside, image.NewRGBA(image.Rect(0, 0, 8, 8)), 90); err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		name string
		args []string
		code int
		want string
	}{
		{"no command", []string{}, 2, "select a mosaic action"},
		{"unknown command", []string{"frobnicate", game, "pattern.png"}, 2, "unknown command"},
		{"help with operands", []string{"--help", "x"}, 0, ""},
		{"no image", []string{"skip", "--manual", e.manual, game}, 2, "expected a game directory and an image"},
		{"list with image", []string{"list", game, "pattern.png"}, 2, "expected a game directory"},
		{"no manual file", []string{"skip", "-o", e.out, game, "pattern.png"}, 1, "MOSAIC_MANUAL"},
		{"box on grow", []string{"grow", "--manual", e.manual, "--box", "0,0,4,4", game, "pattern.png"}, 1, "require add or drop"},
		{"poly on drop", []string{"drop", "--manual", e.manual, "--poly", "0,0,4,0,2,4", game, "pattern.png"}, 1, "drop requires rectangles"},
		{"geometry on skip", []string{"skip", "--manual", e.manual, "--rect", "0,0,4,4", game, "pattern.png"}, 1, "geometry flags require"},
		{"view size", []string{"view", "--size", "0", game, "pattern.png"}, 1, "view size"},
		{"view size big", []string{"view", "--size", "5000", game, "pattern.png"}, 1, "view size"},
		{"view step", []string{"view", "--step", "-1", game, "pattern.png"}, 1, "view size"},
		{"bad src", []string{"view", "--src", "sideways", "-o", e.out, game, "pattern.png"}, 1, "original か masked"},
		{"region outside", []string{"view", "--region", "100,100,5,5", "-o", e.out, game, "pattern.png"}, 1, "絵の外"},
		{"bad coordinate", []string{"add", "--rect", "a,b,c,d", game, "pattern.png"}, 2, "invalid"},
		{"empty coordinate", []string{"add", "--rect", ",", game, "pattern.png"}, 2, "empty"},
		{"nan", []string{"add", "--rect", "NaN,0,1,1", game, "pattern.png"}, 2, "invalid coordinate"},
		{"huge", []string{"add", "--rect", "2000000,0,1,1", game, "pattern.png"}, 2, "invalid coordinate"},
		{"no geometry", []string{"add", "--manual", e.manual, "-o", e.out, game, "pattern.png"}, 1, "add には"},
		{"drop nothing", []string{"drop", "--manual", e.manual, "-o", e.out, game, "pattern.png"}, 1, "drop には"},
		{"grow nothing", []string{"grow", "--manual", e.manual, "-o", e.out, game, "pattern.png"}, 1, "grow には"},
		{"grow region without amount", []string{"grow", "--manual", e.manual, "--region-id", "A1", "-o", e.out, game, "pattern.png"}, 1, "grow には"},
		{"grow rect without amount", []string{"grow", "--manual", e.manual, "--rect", "0,0,4,4", "-o", e.out, game, "pattern.png"}, 1, "grow には"},
		{"drop region without report", []string{"drop", "--manual", e.manual, "--region-id", "A1", "-o", e.out, game, "pattern.png"}, 1, "report.json に無い"},
		{"outside the assets", []string{"skip", "--manual", e.manual, "-o", e.out, game, outside}, 1, "assets の外"},
		{"escapes", []string{"skip", "--manual", e.manual, "-o", e.out, game, "../x.png"}, 1, ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			code, _, diag := invoke(t, c.args...)
			if code != c.code || !strings.Contains(diag, c.want) {
				t.Fatalf("exit %d (want %d): %s", code, c.code, diag)
			}
		})
	}
	if _, err := os.Stat(e.manual); !os.IsNotExist(err) {
		t.Fatal("a refused command wrote the manual file")
	}
}

func TestManualDropAndGrowAutomaticRegions(t *testing.T) {
	e := newManualEnv(t, penis(4, 4, 8, 8, 0.9), pussy(16, 16, 20, 20, 0.9))
	e.auto()
	before := readEntries(t, e.out)[0]
	if len(before.Regions) != 2 {
		t.Fatalf("regions %v", before.Regions)
	}
	// Grow region A1 (the upper one) by 2 pixels on every side.
	e.ok("grow", "--region-id", "A1", "--by", "2")
	after := readEntries(t, e.out)[0]
	a1 := before.Regions[0]
	if want := [4]int{a1[0] - 2, a1[1] - 2, a1[2] + 4, a1[3] + 4}; after.Regions[0] != want {
		t.Fatalf("A1 %v, want %v", after.Regions[0], want)
	}
	if after.Regions[1] != before.Regions[1] {
		t.Fatalf("A2 changed: %v", after.Regions[1])
	}
	// Individual sides, and all regions at once; a negative amount shrinks.
	e.ok("grow", "--region-id", "all", "--left", "1", "--bottom", "1")
	fx := e.fix()
	if len(fx.Grow) != 3 {
		t.Fatalf("grow entries %v", fx.Grow)
	}
	// A margin for this picture only.
	e.ok("grow", "--margin", "0.5")
	if got := e.fix().Margin; got != 0.5 {
		t.Fatalf("margin %v", got)
	}
	// Drop A2 by its id: the region and its pixelation go; the id may be written a2 or 2.
	e.ok("drop", "--region-id", "a2")
	got := readEntries(t, e.out)[0]
	for _, r := range got.Regions {
		if mosaic.Rect(r).Overlaps(image.Rect(16, 16, 20, 20)) {
			t.Fatalf("the dropped region is still there: %v", got.Regions)
		}
	}
	if len(e.fix().Drop) != 1 {
		t.Fatalf("drop entries %v", e.fix().Drop)
	}
	// Dropping by a rectangle over A1.
	e.ok("drop", "--rect", "0,0,9,9")
	if got := readEntries(t, e.out)[0]; len(got.Regions) != 0 || got.Output != "" {
		t.Fatalf("nothing is left to pixelate: %+v", got)
	}
	if _, err := os.Stat(filepath.Join(e.out, "pattern.png")); !os.IsNotExist(err) {
		t.Fatal("the output of a picture with nothing left was kept")
	}
	// An id past the regions is an error and changes nothing.
	fixBefore := e.fix()
	for _, id := range []string{"A9", "0", "x"} {
		if code, _, _ := e.call("drop", "--region-id", id); code == 0 {
			t.Errorf("region id %q accepted", id)
		}
	}
	if len(e.fix().Drop) != len(fixBefore.Drop) {
		t.Fatal("a refused drop changed the file")
	}
}

func TestManualGrowByRectangleAndClearAndCheck(t *testing.T) {
	e := newManualEnv(t, penis(4, 4, 10, 10, 0.9))
	e.ok("grow", "--rect", "4,4,6,6", "--by", "2")
	fx := e.fix()
	if len(fx.Grow) != 1 || fx.Grow[0] != [8]int{4, 4, 6, 6, 2, 2, 2, 2} {
		t.Fatalf("grow %v", fx.Grow)
	}
	if r := readEntries(t, e.out)[0].Regions; len(r) != 1 || r[0] != [4]int{2, 2, 10, 10} {
		t.Fatalf("regions %v", r)
	}
	e.ok("mark", "--checked")
	if !e.fix().Checked {
		t.Fatal("check")
	}
	// Anything that changes the instructions takes the check back.
	e.ok("add", "--rect", "0,0,2,2")
	if e.fix().Checked {
		t.Fatal("a changed picture stays checked")
	}
	e.ok("mark", "--checked")
	e.ok("clear")
	if _, err := mosaic.LoadFixes(e.manual); err != nil {
		t.Fatal(err)
	}
	if got := e.fix(); !got.Empty() {
		t.Fatalf("clear left %+v", got)
	}
	if b := readFile(t, e.manual); strings.Contains(string(b), "pattern.png") {
		t.Fatalf("an empty entry was kept: %q", b)
	}
}

func TestManualBlockAndHumanFixFile(t *testing.T) {
	e := newManualEnv(t, penis(4, 4, 10, 10, 0.9))
	human := filepath.Join(filepath.Dir(e.game), "human.yaml")
	if err := (mosaic.Fixes{"pattern.png": {Add: [][4]int{{18, 18, 4, 4}}}}).Save(human, ""); err != nil {
		t.Fatal(err)
	}
	e.ok("add", "--fix", human, "--rect", "0,0,2,2", "--block", "6")
	if e.fix().Block != 6 || len(e.fix().Add) != 1 {
		t.Fatalf("manual %+v (the human file's add is not copied into it)", e.fix())
	}
	en := readEntries(t, e.out)[0]
	if en.Block != 6 || len(en.Adds) != 2 {
		t.Fatalf("both files reach the build: %+v", en)
	}
	// The same file for both is refused.
	if code, _, diag := invoke(t, "mark", "--checked", "--fix", e.manual, "--manual", e.manual, "-o", e.out, e.game, "pattern.png"); code != 1 || !strings.Contains(diag, "separate files") {
		t.Fatalf("code %d: %s", code, diag)
	}
}
