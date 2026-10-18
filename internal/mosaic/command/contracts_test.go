package command

import (
	"bytes"
	"encoding/json"
	"image"
	"image/color"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nao1215/gozen/internal/mosaic"
	"github.com/nao1215/gozen/internal/mosaic/detect"
)

func fixture(t *testing.T) (string, string) {
	t.Helper()
	for _, key := range []string{"MOSAIC_OUT", "MOSAIC_FIX", "MOSAIC_MANUAL", "MOSAIC_TAGS", "MOSAIC_TAGGER", "MOSAIC_SHAPE", "MOSAIC_PASSES", "ONNXRUNTIME_LIB"} {
		t.Setenv(key, "")
	}
	root := t.TempDir()
	game := filepath.Join(root, "game")
	if err := os.MkdirAll(filepath.Join(game, "assets"), 0o750); err != nil {
		t.Fatal(err)
	}
	img := image.NewRGBA(image.Rect(0, 0, 24, 24))
	for y := 0; y < 24; y++ {
		for x := 0; x < 24; x++ {
			img.SetRGBA(x, y, color.RGBA{uint8(x * 8), uint8(y * 8), 0, 255})
		}
	}
	if err := encode(filepath.Join(game, "assets", "pattern.png"), img, 90); err != nil {
		t.Fatal(err)
	}
	return game, filepath.Join(root, "output")
}
func invoke(t *testing.T, args ...string) (int, string, string) {
	t.Helper()
	var out, diag bytes.Buffer
	code := Run(args, &out, &diag)
	return code, out.String(), diag.String()
}
func TestHelpAndPrerequisites(t *testing.T) {
	for _, args := range [][]string{{"--help"}, {"auto", "--help"}, {"inspect", "-h"}, {"manual", "help"}, {"manual", "add", "--help"}} {
		code, out, diag := invoke(t, args...)
		if code != 0 || !strings.Contains(out, "Requirements:") || diag != "" {
			t.Fatalf("%v => %d %q %q", args, code, out, diag)
		}
	}
	game, out := fixture(t)
	code, _, diag := invoke(t, "auto", "-out", out, game, "pattern.png")
	if code != 1 || !strings.Contains(diag, "Requirements:") {
		t.Fatalf("missing prerequisites: %d %s", code, diag)
	}
	if _, err := os.Stat(out); !os.IsNotExist(err) {
		t.Fatal("missing model wrote output")
	}
	for _, args := range [][]string{{}, {"unknown"}, {"manual", "unknown", game, "pattern.png"}, {"auto", "-unknown"}} {
		code, _, _ := invoke(t, args...)
		if code != 2 {
			t.Fatalf("argument exit %v: %d", args, code)
		}
	}
	if code := Run([]string{"--help"}, failedWriter{}, io.Discard); code != 1 {
		t.Fatal("lost help output failure")
	}
}
func TestManualStateAndRollback(t *testing.T) {
	game, out := fixture(t)
	manual := filepath.Join(filepath.Dir(game), "manual.yaml")
	orig, err := os.ReadFile(filepath.Join(game, "assets", "pattern.png")) //nolint:gosec // Generated fixture paths are isolated by t.TempDir.
	if err != nil {
		t.Fatal(err)
	}
	base := []string{"-out", out, "-manual", manual, "-size", "48"}
	call := func(sub string, flags ...string) int {
		args := append([]string{"manual", sub}, base...)
		args = append(args, flags...)
		args = append(args, game, "pattern.png")
		code, _, diag := invoke(t, args...)
		if code != 0 {
			t.Log(diag)
		}
		return code
	}
	if call("skip") != 0 {
		t.Fatal("manual-only skip failed")
	}
	if call("add", "-rect", "-2,0,8,8", "-rect", "12,12,8,8") != 0 {
		t.Fatal("multiple clipped rectangles failed")
	}
	fx, err := mosaic.LoadFixes(manual)
	if err != nil || len(fx["pattern.png"].Add) != 2 || fx["pattern.png"].Add[0] != [4]int{0, 0, 6, 8} {
		t.Fatalf("saved geometry: %+v %v", fx, err)
	}
	if call("grow", "-rect", "0,0,4,4", "-by", "2") != 0 {
		t.Fatal("grow -rect unreachable")
	}
	if call("check") != 0 {
		t.Fatal("check failed")
	}
	fx, err = mosaic.LoadFixes(manual)
	if err != nil || !fx["pattern.png"].Checked {
		t.Fatal("check not persisted")
	}
	before, err := os.ReadFile(manual) //nolint:gosec // Generated fixture paths are isolated by t.TempDir.
	if err != nil {
		t.Fatal(err)
	}
	// clear would resume detection; failure must preserve the manual document and output.
	masked, err := os.ReadFile(filepath.Join(out, "pattern.png")) //nolint:gosec // Generated fixture paths are isolated by t.TempDir.
	if err != nil {
		t.Fatal(err)
	}
	if call("clear") == 0 {
		t.Fatal("clear unexpectedly succeeded without a model")
	}
	after, err := os.ReadFile(manual) //nolint:gosec // Generated fixture paths are isolated by t.TempDir.
	if err != nil || !bytes.Equal(before, after) {
		t.Fatal("failed rebuild changed YAML")
	}
	after, err = os.ReadFile(filepath.Join(out, "pattern.png")) //nolint:gosec // Generated fixture paths are isolated by t.TempDir.
	if err != nil || !bytes.Equal(masked, after) {
		t.Fatal("failed rebuild changed output")
	}
	after, err = os.ReadFile(filepath.Join(game, "assets", "pattern.png")) //nolint:gosec // Generated fixture paths are isolated by t.TempDir.
	if err != nil || !bytes.Equal(orig, after) {
		t.Fatal("source modified")
	}
}
func TestInvalidGeometryAndOptions(t *testing.T) {
	game, out := fixture(t)
	manual := filepath.Join(filepath.Dir(game), "manual.yaml")
	for _, flags := range [][]string{
		{"-rect", "0,0,0,4"}, {"-rect", "0,0,-4,4"}, {"-box", "8,8,1,1"}, {"-rect", "50,50,4,4"},
		{"-rect", "x1,2,3,4"}, {"-rect", "NaN,0,4,4"}, {"-rect", "1e100,0,4,4"}, {"-ellipse", "2,2,-1,3"},
		{"-threshold", "NaN", "-rect", "0,0,4,4"}, {"-quality", "101", "-rect", "0,0,4,4"}, {"-block", "-1", "-rect", "0,0,4,4"},
	} {
		args := append([]string{"manual", "add", "-manual", manual, "-out", out}, flags...)
		args = append(args, game, "pattern.png")
		code, _, _ := invoke(t, args...)
		if code == 0 {
			t.Fatalf("accepted invalid geometry %v", flags)
		}
		if _, err := os.Stat(manual); !os.IsNotExist(err) {
			t.Fatal("invalid input wrote manual file")
		}
	}
}
func TestReadOnlyInspectionAndDryRun(t *testing.T) {
	game, out := fixture(t)
	manual := filepath.Join(filepath.Dir(game), "manual.yaml")
	tags := filepath.Join(filepath.Dir(game), "tags.json")
	if err := (mosaic.Fixes{"pattern.png": {Skip: true, Add: [][4]int{{0, 0, 8, 8}}, Block: 4}}).Save(manual, ""); err != nil {
		t.Fatal(err)
	}
	code, _, diag := invoke(t, "auto", "-manual", manual, "-out", out, game, "pattern.png")
	if code != 0 {
		t.Fatal(diag)
	}
	if err := os.WriteFile(tags, []byte("{}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(tags) //nolint:gosec // Generated fixture paths are isolated by t.TempDir.
	if err != nil {
		t.Fatal(err)
	}
	code, text, diag := invoke(t, "inspect", "-manual", manual, "-out", out, "-tags", tags, game, "pattern.png")
	if code != 0 {
		t.Fatal(diag)
	}
	var in inspection
	if err := json.Unmarshal([]byte(text), &in); err != nil || in.Status != "done" {
		t.Fatalf("inspection result %s %v", text, err)
	}
	dryOut := filepath.Join(filepath.Dir(game), "dry")
	code, _, diag = invoke(t, "auto", "-dry", "-manual", manual, "-out", dryOut, "-tags", tags, game, "pattern.png")
	if code != 0 {
		t.Fatal(diag)
	}
	if _, err := os.Stat(dryOut); !os.IsNotExist(err) {
		t.Fatal("dry-run wrote images/report")
	}
	after, err := os.ReadFile(tags) //nolint:gosec // Generated fixture paths are isolated by t.TempDir.
	if err != nil || !bytes.Equal(before, after) {
		t.Fatal("read-only command changed tag cache")
	}
}
func TestSourceProtection(t *testing.T) {
	game, out := fixture(t)
	manual := filepath.Join(filepath.Dir(game), "manual.yaml")
	for _, destination := range []string{filepath.Join(game, "assets"), filepath.Join(game, "assets", "nested"), filepath.Dir(game)} {
		code, _, _ := invoke(t, "manual", "skip", "-manual", manual, "-out", destination, game, "pattern.png")
		if code == 0 {
			t.Fatalf("accepted unsafe output %s", destination)
		}
	}
	link := filepath.Join(filepath.Dir(game), "alias")
	if err := os.Symlink(filepath.Join(game, "assets"), link); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}
	code, _, _ := invoke(t, "manual", "skip", "-manual", manual, "-out", filepath.Join(link, "nested"), game, "pattern.png")
	if code == 0 {
		t.Fatal("symlink source alias accepted")
	}
	code, _, _ = invoke(t, "auto", "-out", out, game, "../pattern.png")
	if code == 0 {
		t.Fatal("traversal accepted")
	}
}
func TestWebPOutputIsHonest(t *testing.T) {
	// An original lossless WebP fixture verifies the actual decoder and output naming.
	game, out := fixture(t)
	source := filepath.Join(game, "assets", "pattern.webp")
	b, err := os.ReadFile("../../../docs/examples/production/pattern.webp")
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(source, b, 0o600); err != nil { //nolint:gosec // Generated fixture paths are isolated by t.TempDir.
		t.Fatal(err)
	}
	manual := filepath.Join(filepath.Dir(game), "manual.yaml")
	if err = (mosaic.Fixes{"pattern.webp": {Skip: true, Add: [][4]int{{0, 0, 8, 8}}}}).Save(manual, ""); err != nil {
		t.Fatal(err)
	}
	code, _, diag := invoke(t, "auto", "-manual", manual, "-out", out, game, "pattern.webp")
	if code != 0 {
		t.Fatal(diag)
	}
	b, err = os.ReadFile(filepath.Join(out, "pattern.webp.png")) //nolint:gosec // Generated fixture paths are isolated by t.TempDir.
	if err != nil || !bytes.HasPrefix(b, []byte("\x89PNG\r\n\x1a\n")) {
		t.Fatal("WebP output name/encoding mismatch")
	}
}

type failedWriter struct{}

func (failedWriter) Write([]byte) (int, error) { return 0, io.ErrClosedPipe }

func TestPlanGeometryAndFaceReview(t *testing.T) {
	b := image.Rect(0, 0, 24, 24)
	p := defaultParams
	p.minScore = 0.5
	p.sureScore = 0.9
	boxes := []detect.Box{
		{Rect: image.Rect(1, 1, 5, 5), Label: "pussy", Score: 0.8, Pass: "first"},
		{Rect: image.Rect(12, 12, 16, 16), Label: "penis", Score: 0.8, Pass: "first"},
		{Rect: image.Rect(20, 20, 24, 24), Label: "penis", Score: 0.1, Pass: "first"},
	}
	fx := mosaic.Fix{Grow: [][8]int{{12, 12, 4, 4, 2, 2, 2, 2}}}
	groups, skipped, regions, reasons, _ := plan(b, boxes, []image.Rectangle{image.Rect(0, 0, 8, 8)}, fx, nil, 4, p)
	if len(groups) != 1 || len(skipped) != 1 || len(regions) != 1 || regions[0] != image.Rect(10, 10, 18, 18) || len(reasons) == 0 {
		t.Fatalf("plan: %v %v %v %v", groups, skipped, regions, reasons)
	}
	fx.Skip = true
	groups, _, regions, _, _ = plan(b, boxes, nil, fx, nil, 4, p)
	if len(groups) != 0 || len(regions) != 0 {
		t.Fatal("skip still applied detection")
	}
}

func TestUnknownReportAndOutputFailures(t *testing.T) {
	game, out := fixture(t)
	if err := os.MkdirAll(out, 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(out, "report.json"), []byte("broken"), 0o600); err != nil {
		t.Fatal(err)
	}
	code, _, diag := invoke(t, "manual", "list", "-out", out, game)
	if code != 1 || !strings.Contains(diag, "invalid report") {
		t.Fatalf("corrupt report hidden: %d %s", code, diag)
	}
	if err := os.WriteFile(filepath.Join(out, "report.json"), []byte(`[{"file":"pattern.png","output":"../escape.png"}]`), 0o600); err != nil {
		t.Fatal(err)
	}
	code, _, _ = invoke(t, "inspect", "-out", out, game, "pattern.png")
	if code != 1 {
		t.Fatal("unsafe report accepted")
	}
	if err := encode(filepath.Join(out, "bad.webp"), image.NewRGBA(image.Rect(0, 0, 1, 1)), 90); err == nil {
		t.Fatal("unsupported encoding accepted")
	}
	if err := encode(out, image.NewRGBA(image.Rect(0, 0, 1, 1)), 90); err == nil {
		t.Fatal("I/O failure hidden")
	}
}
