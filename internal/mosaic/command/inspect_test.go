package command

import (
	"encoding/json"
	"image"
	"image/color"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nao1215/gozen/internal/mosaic"
)

func inspectOne(t *testing.T, args ...string) inspection {
	t.Helper()
	code, out, diag := invoke(t, append([]string{"inspect"}, args...)...)
	if code != 0 {
		t.Fatalf("inspect exit %d: %s", code, diag)
	}
	var in inspection
	if err := json.Unmarshal([]byte(out), &in); err != nil {
		t.Fatalf("%v\n%s", err, out)
	}
	return in
}

func TestInspectStatusFollowsTheWork(t *testing.T) {
	game, out := fixture(t)
	m := useModels(t)
	model, det := m.detector(t, "model.onnx", penis(4, 4, 10, 10, 0.9))
	base := []string{"--model", model, "-o", out, game, "pattern.png"}

	// Found but not yet pixelated: to do. Nothing is written.
	in := inspectOne(t, base...)
	if in.Status != "todo" || !in.NeedsMosaic || len(in.Objects) != 1 || in.Objects[0].Label != "penis" || in.Mosaicked.Output != "" {
		t.Fatalf("before auto: %+v", in)
	}
	if !strings.Contains(in.Mosaicked.Note, "auto") {
		t.Fatalf("note %q", in.Mosaicked.Note)
	}
	if _, err := os.Stat(out); !os.IsNotExist(err) {
		t.Fatal("inspect wrote the output directory")
	}
	if in.Size != [2]int{24, 24} || in.Mosaicked.StandardBlock != 4 || in.Source == "" || in.File != "pattern.png" {
		t.Fatalf("basic facts %+v", in)
	}
	if len(in.Categories) != 1 || in.Categories[0] != "男性器" {
		t.Fatalf("categories %v", in.Categories)
	}

	// After auto the report answers without running the model again.
	if code, _, diag := invoke(t, append([]string{"auto"}, base...)...); code != 0 {
		t.Fatal(diag)
	}
	before := det.calls
	in = inspectOne(t, base...)
	if in.Status != "done" || !in.Mosaicked.OutputCovers || !in.Mosaicked.BlockOK || in.Mosaicked.OutputBlock != 4 {
		t.Fatalf("after auto: %+v", in)
	}
	if det.calls != before {
		t.Fatal("inspect detected again although the report matches")
	}
	if in.Mosaicked.Output != filepath.Join(out, "pattern.png") {
		t.Fatalf("output %q", in.Mosaicked.Output)
	}

	// A changed picture no longer matches the report: it is looked at again.
	img := image.NewRGBA(image.Rect(0, 0, 24, 24))
	for y := range 24 {
		for x := range 24 {
			img.SetRGBA(x, y, color.RGBA{uint8(255 - x*8), 0, uint8(y * 8), 255})
		}
	}
	if err := encode(filepath.Join(game, "assets", "pattern.png"), img, 90); err != nil {
		t.Fatal(err)
	}
	before = det.calls
	in = inspectOne(t, base...)
	if det.calls != before+1 || in.Status != "todo" {
		t.Fatalf("changed picture: calls %d status %s note %q", det.calls, in.Status, in.Mosaicked.Note)
	}
}

func TestInspectNothingFoundIsNone(t *testing.T) {
	game, out := fixture(t)
	m := useModels(t)
	model, _ := m.detector(t, "model.onnx")
	in := inspectOne(t, "--model", model, "-o", out, game, "pattern.png")
	if in.Status != "none" || in.NeedsMosaic || len(in.Objects) != 0 || len(in.Categories) != 0 {
		t.Fatalf("%+v", in)
	}
	// The slices are present (empty), not null, for a script that reads them.
	code, raw, _ := invoke(t, "inspect", "--model", model, "-o", out, game, "pattern.png")
	if code != 0 || !strings.Contains(raw, `"objects":[]`) || !strings.Contains(raw, `"categories":[]`) {
		t.Fatalf("raw %q", raw)
	}
}

func TestInspectLowConfidenceIsReviewUntilChecked(t *testing.T) {
	game, out := fixture(t)
	m := useModels(t)
	model, _ := m.detector(t, "model.onnx", penis(4, 4, 10, 10, 0.6))
	args := []string{"--model", model, "--sure", "0.9", "-o", out, game, "pattern.png"}
	in := inspectOne(t, args...)
	if in.Status != "review" || len(in.Review) != 1 {
		t.Fatalf("%+v", in)
	}
	fixes := filepath.Join(filepath.Dir(out), "fixes.yaml")
	if err := (mosaic.Fixes{"pattern.png": {Checked: true}}).Save(fixes, ""); err != nil {
		t.Fatal(err)
	}
	in = inspectOne(t, append([]string{"--fix", fixes}, args...)...)
	if in.Status == "review" || len(in.Review) != 0 {
		t.Fatalf("a checked picture is not under review: %+v", in)
	}
}

func TestInspectSeesAMosaicAlreadyInThePicture(t *testing.T) {
	game, out := fixture(t)
	assets := filepath.Join(game, "assets")
	img := image.NewRGBA(image.Rect(0, 0, 60, 60))
	for y := range 60 {
		for x := range 60 {
			v := uint8(((x/5)*29 + (y/5)*53) % 250)
			img.SetRGBA(x, y, color.RGBA{v, 255 - v, v / 2, 255})
		}
	}
	if err := encode(filepath.Join(assets, "done.png"), img, 90); err != nil {
		t.Fatal(err)
	}
	m := useModels(t)
	model, _ := m.detector(t, "model.onnx", penis(5, 5, 55, 55, 0.9))
	in := inspectOne(t, "--model", model, "-o", out, game, "done.png")
	if !in.Mosaicked.InImage || len(in.Objects) != 1 || !in.Objects[0].Mosaic || in.Objects[0].Block != 5 {
		t.Fatalf("%+v", in)
	}
	// 5 is above the standard block of a 60 pixel picture (4), so it counts as done.
	if !in.Mosaicked.BlockOK || in.Status != "done" {
		t.Fatalf("status %s: %+v", in.Status, in.Mosaicked)
	}
	// The plain pattern is not a mosaic.
	in = inspectOne(t, "--model", model, "-o", out, game, "pattern.png")
	if in.Mosaicked.InImage || in.Status == "done" {
		t.Fatalf("%+v", in)
	}
}

func TestInspectManualInstructionsAndSkips(t *testing.T) {
	game, out := fixture(t)
	m := useModels(t)
	model, det := m.detector(t, "model.onnx", penis(4, 4, 10, 10, 0.9))
	manual := filepath.Join(filepath.Dir(out), "manual.yaml")
	if err := (mosaic.Fixes{"pattern.png": {Skip: true, Add: [][4]int{{0, 0, 8, 8}}, Note: "by hand"}}).Save(manual, ""); err != nil {
		t.Fatal(err)
	}
	in := inspectOne(t, "--model", model, "--manual", manual, "-o", out, game, "pattern.png")
	if det.calls != 0 {
		t.Fatal("a skipped picture was detected")
	}
	if !in.NeedsMosaic || in.Manual == nil || len(in.Manual.Add) != 1 || !in.Manual.Skip || in.Manual.Note != "by hand" || len(in.Objects) != 0 {
		t.Fatalf("%+v", in)
	}
	found := false
	for _, e := range in.Evidence {
		found = found || strings.Contains(e, "手の指示 1 箇所")
	}
	if !found {
		t.Fatalf("evidence %v", in.Evidence)
	}
}

func TestInspectPicturesByPathAndPretty(t *testing.T) {
	game, out := fixture(t)
	m := useModels(t)
	model, _ := m.detector(t, "model.onnx", penis(4, 4, 10, 10, 0.9))
	if code, _, diag := invoke(t, "auto", "--model", model, "-o", out, game, "pattern.png"); code != 0 {
		t.Fatal(diag)
	}
	assetPath := filepath.Join(game, "assets", "pattern.png")
	for _, arg := range []string{"pattern.png", "assets/pattern.png", assetPath, filepath.Join(out, "pattern.png")} {
		in := inspectOne(t, "--model", model, "-o", out, game, arg)
		if in.File != "pattern.png" {
			t.Errorf("%q resolved to %q", arg, in.File)
		}
	}
	// The masked file is read from the output, the original from the assets.
	in := inspectOne(t, "--model", model, "-o", out, game, filepath.Join(out, "pattern.png"))
	if in.Path != filepath.Join(out, "pattern.png") {
		t.Fatalf("path %q", in.Path)
	}
	// A file elsewhere has no asset name.
	other := filepath.Join(t.TempDir(), "elsewhere.png")
	if err := encode(other, image.NewRGBA(image.Rect(0, 0, 8, 8)), 90); err != nil {
		t.Fatal(err)
	}
	in = inspectOne(t, "--model", model, "-o", out, game, other)
	if in.File != "" || in.Path != other {
		t.Fatalf("%+v", in)
	}
	// --pretty writes indented JSON.
	_, raw, _ := invoke(t, "inspect", "--pretty", "--model", model, "-o", out, game, "pattern.png")
	if !strings.Contains(raw, "\n  \"source\"") {
		t.Fatalf("not indented: %q", raw)
	}
	// Several pictures are one line each; with none named, every picture of the game is.
	if err := os.WriteFile(filepath.Join(game, "scene.yaml"), []byte("pics: [assets/pattern.png]\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	_, raw, _ = invoke(t, "inspect", "--model", model, "-o", out, game)
	if lines := strings.Split(strings.TrimSpace(raw), "\n"); len(lines) != 1 {
		t.Fatalf("%d lines: %q", len(lines), raw)
	}
	// A picture that is not there is an error.
	code, _, diag := invoke(t, "inspect", "--model", model, "-o", out, game, "nowhere.png")
	if code != 1 || !strings.Contains(diag, "nowhere.png") {
		t.Fatalf("code %d: %s", code, diag)
	}
}

func TestInspectReportAndOutputMismatches(t *testing.T) {
	game, out := fixture(t)
	m := useModels(t)
	model, _ := m.detector(t, "model.onnx", penis(4, 4, 10, 10, 0.9))
	args := []string{"--model", model, "-o", out, game, "pattern.png"}
	if code, _, diag := invoke(t, append([]string{"auto"}, args...)...); code != 0 {
		t.Fatal(diag)
	}
	// The output went missing although the report names it.
	if err := os.Remove(filepath.Join(out, "pattern.png")); err != nil {
		t.Fatal(err)
	}
	in := inspectOne(t, args...)
	if in.Status != "todo" || !strings.Contains(in.Mosaicked.Note, "ファイルが無い") {
		t.Fatalf("%+v", in)
	}
	// An output that exists, but no report to tie it to.
	if code, _, diag := invoke(t, append([]string{"auto"}, args...)...); code != 0 {
		t.Fatal(diag)
	}
	if err := os.Remove(filepath.Join(out, "report.json")); err != nil {
		t.Fatal(err)
	}
	in = inspectOne(t, args...)
	if in.Mosaicked.OutputCovers && in.Status == "done" && !strings.Contains(in.Mosaicked.Note, "report.json") {
		t.Fatalf("an output without a report was trusted: %+v", in)
	}
}

func TestInspectWithTags(t *testing.T) {
	game, out := fixture(t)
	m := useModels(t)
	model, _ := m.detector(t, "model.onnx")
	cache := filepath.Join(filepath.Dir(out), "tags.json")
	pic := filepath.Join(game, "assets", "pattern.png")
	st, err := stamp(pic)
	if err != nil {
		t.Fatal(err)
	}
	b, err := json.Marshal(map[string]tagEntry{"pattern.png": {Stamp: st, Tags: map[string]float32{"penis": 0.9, "pussy": 0.7, "sex": 0.6, "explicit": 0.9, "general": 0.1, "unrelated": 0.99}}})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(cache, b, 0o600); err != nil {
		t.Fatal(err)
	}
	in := inspectOne(t, "--model", model, "--tags", cache, "-o", out, game, "pattern.png")
	if in.Rating != "explicit" {
		t.Fatalf("rating %q", in.Rating)
	}
	if in.Tags["penis"] != 0.9 || in.Tags["pussy"] != 0.7 || in.Tags["sex"] != 0.6 || len(in.Tags) != 3 {
		t.Fatalf("tag summary %v (only the tags that matter are reported)", in.Tags)
	}
	if !in.NeedsMosaic || len(in.Categories) != 2 || in.Status != "review" {
		t.Fatalf("tags alone: %+v", in)
	}
	// After auto, the report is read: the tags still show.
	if code, _, diag := invoke(t, "auto", "--model", model, "--tags", cache, "-o", out, game, "pattern.png"); code != 0 {
		t.Fatal(diag)
	}
	in = inspectOne(t, "--model", model, "--tags", cache, "-o", out, game, "pattern.png")
	if in.Rating != "explicit" || len(in.Categories) != 2 {
		t.Fatalf("from the report: %+v", in)
	}
}

func TestRatingAndTagMax(t *testing.T) {
	cases := []struct {
		tags map[string]float32
		want string
	}{
		{nil, ""},
		{map[string]float32{"unrelated": 1}, ""},
		{map[string]float32{"general": 0.3, "sensitive": 0.5}, "sensitive"},
		{map[string]float32{"general": 0.9, "explicit": 0.1}, "general"},
		{map[string]float32{"questionable": 0.4, "explicit": 0.4}, "questionable"}, // a tie keeps the first of the order
	}
	for _, c := range cases {
		if got := rating(c.tags); got != c.want {
			t.Errorf("rating(%v) = %q, want %q", c.tags, got, c.want)
		}
	}
	if tagMax(map[string]float32{"a": 0.2, "b": 0.7}, []string{"a", "b", "c"}) != 0.7 || tagMax(nil, []string{"a"}) != 0 {
		t.Fatal("tagMax")
	}
}

func TestCoveredBy(t *testing.T) {
	boxes := []mosaic.Shape{mosaic.Box(image.Rect(0, 0, 6, 6)), mosaic.Box(image.Rect(6, 0, 12, 6))}
	if !coveredBy(image.Rect(1, 1, 11, 5), boxes) {
		t.Fatal("two boxes side by side cover the span")
	}
	if coveredBy(image.Rect(1, 1, 11, 8), boxes) {
		t.Fatal("a span below the boxes is not covered")
	}
	if !coveredBy(image.Rectangle{}, nil) {
		t.Fatal("nothing to cover is covered")
	}
}
