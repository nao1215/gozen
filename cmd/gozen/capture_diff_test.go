package main

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"image"
	"image/color"
	"image/gif"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCapturePixelComparison(t *testing.T) {
	for _, tc := range []struct {
		name  string
		size  int
		pixel bool
		count int
		ratio float64
		box   *[4]int
	}{
		{"identical", 2, false, 0, 0, nil},
		{"one pixel", 2, true, 1, .25, &[4]int{1, 1, 1, 1}},
		{"size change", 3, false, 5, 5.0 / 9, &[4]int{0, 0, 3, 3}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			a, b := image.NewRGBA(image.Rect(0, 0, 2, 2)), image.NewRGBA(image.Rect(0, 0, tc.size, tc.size))
			if tc.pixel {
				b.SetRGBA(1, 1, color.RGBA{R: 255, A: 255})
			}
			d, _ := compareRGBA(a, b)
			if d.DiffPixels != tc.count || d.DiffRatio != tc.ratio || (d.DiffBBox == nil) != (tc.box == nil) {
				t.Fatalf("%+v", d)
			}
			if tc.box != nil && *d.DiffBBox != *tc.box {
				t.Fatalf("box %v", d.DiffBBox)
			}
		})
	}
	a := image.NewRGBA(image.Rect(0, 0, 2, 2))
	b := image.NewRGBA(a.Bounds())
	b.SetRGBA(1, 1, color.RGBA{A: 255})
	d, _ := compareShotImages(captureImage{frames: []*image.RGBA{a, a}, delay: []int{10, 10}}, captureImage{frames: []*image.RGBA{a, b}, delay: []int{10, 10}})
	if !d.Changed || d.DiffPixels != 1 || d.DiffRatio != .125 {
		t.Fatalf("later GIF frame: %+v", d)
	}
	d, _ = compareShotImages(captureImage{frames: []*image.RGBA{a}, delay: []int{10}}, captureImage{frames: []*image.RGBA{a}, delay: []int{20}})
	if !d.Changed || !d.AnimationChanged || d.DiffPixels != 0 {
		t.Fatalf("GIF timing: %+v", d)
	}
}

func TestCaptureDiffStatesFactsNeverFails(t *testing.T) {
	old, out := t.TempDir(), t.TempDir()
	save := func(dir, id string, red uint8) {
		t.Helper()
		im := image.NewRGBA(image.Rect(0, 0, 2, 2))
		im.SetRGBA(1, 1, color.RGBA{R: red, A: 255})
		var b bytes.Buffer
		if err := png.Encode(&b, im); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, id+".png"), b.Bytes(), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	row := func(id string) shotResult {
		return shotResult{ID: id, Status: "ok", Artifact: id + ".png", item: shotItem{ID: id}}
	}
	for _, id := range []string{"same", "edited", "removed"} {
		save(old, id, 0)
	}
	for _, id := range []string{"same", "edited", "new"} {
		save(out, id, 0)
	}
	save(out, "edited", 255)
	rows, summary, err := compareShots(out, old, []shotResult{row("same"), row("edited"), row("new")}, []shotResult{row("same"), row("edited"), row("removed")})
	if err != nil {
		t.Fatal(err)
	}
	if *summary != (captureDiffSummary{Changed: 3, Unchanged: 1, New: 1, Removed: 1}) {
		t.Fatal(summary)
	}
	for _, r := range rows {
		if r.Status == "failed" {
			t.Fatal("difference became a failure", r)
		}
	}
	if rows[0].Changed || rows[1].DiffPixels != 1 || rows[2].Change != "new" || rows[3].Change != "removed" {
		t.Fatal(rows)
	}
	if err := writeShotIndex(out, &shotList{}, rows, false); err != nil {
		t.Fatal(err)
	}
	if _, err := readPreviousShots(out); err != nil {
		t.Fatal(err)
	}
	var decoded []map[string]any
	if err := json.Unmarshal(read(t, filepath.Join(out, "shots.json")), &decoded); err != nil {
		t.Fatal(err)
	}
	if decoded[0]["changed"] != false || decoded[1]["diff_pixels"] != float64(1) || decoded[3]["change"] != "removed" {
		t.Fatal(decoded)
	}
	html := string(read(t, filepath.Join(out, "index.html")))
	if !strings.Contains(html, "edited.previous.png") || !strings.Contains(html, "edited.diff.png") || strings.Index(html, "id edited") > strings.Index(html, "<details>") || strings.Index(html, "id same") < strings.Index(html, "<details>") {
		t.Fatal(html)
	}
	// A saved comparison can itself be the baseline, including its removed rows.
	previous, err := readPreviousShots(out)
	if err != nil {
		t.Fatal(err)
	}
	again := t.TempDir()
	for _, id := range []string{"same", "edited", "new"} {
		red := uint8(0)
		if id == "edited" {
			red = 255
		}
		save(again, id, red)
	}
	repeated, repeatedSummary, err := compareShots(again, out, []shotResult{row("same"), row("edited"), row("new")}, previous)
	if err != nil {
		t.Fatal(err)
	}
	if *repeatedSummary != (captureDiffSummary{Unchanged: 3}) {
		t.Fatalf("repeated comparison: %+v, rows: %+v", repeatedSummary, repeated)
	}
	// Missing artifacts are I/O failures, not fabricated zero differences.
	if err := os.Remove(filepath.Join(old, "same.png")); err != nil {
		t.Fatal(err)
	}
	if _, _, err := compareShots(out, old, []shotResult{row("same")}, []shotResult{row("same")}); err == nil {
		t.Fatal("missing image accepted")
	}
}

func TestCaptureStructureComparison(t *testing.T) {
	a := json.RawMessage(`{"texts":[{"text":"old","rect":[0,0,1,1]},{"text":"gone","rect":[1,0,1,1]}],"controls":[]}`)
	b := json.RawMessage(`{"texts":[{"text":"new","rect":[0,0,1,1]}],"controls":[{"label":"Next","rect":[2,0,1,1]}]}`)
	d := diffStructure(a, b)
	if len(d["texts"].Changed) != 1 || len(d["texts"].Removed) != 1 || len(d["controls"].Added) != 1 {
		t.Fatal(d)
	}
	if diffStructure(nil, b) != nil {
		t.Fatal("old outputs have no structure")
	}
}

func TestPreviousCaptureValidation(t *testing.T) {
	if _, _, err := compareShots(t.TempDir(), t.TempDir(), []shotResult{{ID: "view", Artifact: "view.png"}, {ID: "view.diff", Artifact: "view.diff.png"}}, nil); err == nil {
		t.Fatal("comparison overwrites another job")
	}
	for _, body := range []string{`[{"id":"../escape"}]`, `[{"id":"a","artifact":"../a.png"}]`, `[{"id":"a"},{"id":"a"}]`, `broken`} {
		dir := t.TempDir()
		write(t, filepath.Join(dir, "shots.json"), body)
		if _, err := readPreviousShots(dir); err == nil {
			t.Fatal(body)
		}
	}
}

func TestCaptureGIFComparisonDecodesFrames(t *testing.T) {
	frame := image.NewPaletted(image.Rect(0, 0, 2, 2), color.Palette{color.Black, color.White})
	last := image.NewPaletted(frame.Bounds(), frame.Palette)
	last.SetColorIndex(1, 1, 1)
	encode := func(frames []*image.Paletted) captureImage {
		t.Helper()
		var buf bytes.Buffer
		if err := gif.EncodeAll(&buf, &gif.GIF{Image: frames, Delay: []int{10, 10}}); err != nil {
			t.Fatal(err)
		}
		decoded, err := decodeShot(buf.Bytes())
		if err != nil {
			t.Fatal(err)
		}
		return decoded
	}
	old := encode([]*image.Paletted{frame, frame})
	next := encode([]*image.Paletted{frame, last})
	d, mask := compareShotImages(old, next)
	if !d.Changed || d.DiffPixels != 1 || *d.DiffBBox != [4]int{1, 1, 1, 1} || mask.RGBAAt(1, 1).A != 255 {
		t.Fatal(d)
	}
	d, _ = compareShotImages(next, next)
	if d.Changed {
		t.Fatal("identical GIF differs")
	}
	if _, err := decodeShot([]byte("not an image")); err == nil {
		t.Fatal("corrupt image accepted")
	}
}

func BenchmarkCapturePixelComparison(b *testing.B) {
	old, next := image.NewRGBA(image.Rect(0, 0, 960, 540)), image.NewRGBA(image.Rect(0, 0, 960, 540))
	next.SetRGBA(500, 300, color.RGBA{A: 255})
	b.ReportAllocs()
	for b.Loop() {
		compareRGBA(old, next)
	}
}

func TestCaptureGIFFrameBudget(t *testing.T) {
	frame := image.NewPaletted(image.Rect(0, 0, 1, 1), color.Palette{color.Black, color.White})
	for _, count := range []int{300, 301} {
		g := &gif.GIF{Delay: make([]int, count)}
		for range count {
			g.Image = append(g.Image, frame)
		}
		var buf bytes.Buffer
		if err := gif.EncodeAll(&buf, g); err != nil {
			t.Fatal(err)
		}
		_, err := decodeShot(buf.Bytes())
		if count == 300 && err != nil {
			t.Fatal(err)
		}
		if count == 301 && (err == nil || !strings.Contains(err.Error(), "budget")) {
			t.Fatalf("301 frames: %v", err)
		}
	}
}

func TestCaptureGIFBudgetBeforeExpansion(t *testing.T) {
	// Container-only frames deliberately lack LZW pixels. The budget must be
	// diagnosed before the pixel decoder reaches the invalid compressed content.
	header := append([]byte("GIF89a"), make([]byte, 7)...)
	binary.LittleEndian.PutUint16(header[6:8], 1280)
	binary.LittleEndian.PutUint16(header[8:10], 720)
	frame := make([]byte, 12)
	frame[0], frame[10] = 0x2c, 2
	binary.LittleEndian.PutUint16(frame[5:7], 1280)
	binary.LittleEndian.PutUint16(frame[7:9], 720)
	for _, count := range []int{72, 73} {
		data := append(bytes.Clone(header), bytes.Repeat(frame, count)...)
		data = append(data, 0x3b)
		err := captureGIFBudget(data)
		if count == 72 && err != nil {
			t.Fatal(err)
		}
		if count == 73 {
			if err == nil || !strings.Contains(err.Error(), "budget") {
				t.Fatalf("pixel budget: %v", err)
			}
			if _, err := decodeShot(data); err == nil || !strings.Contains(err.Error(), "budget") {
				t.Fatalf("decode before budget: %v", err)
			}
		}
	}
}

func TestCaptureGIFBudgetTruncation(t *testing.T) {
	frame := image.NewPaletted(image.Rect(0, 0, 2, 2), color.Palette{color.Black, color.White})
	var buf bytes.Buffer
	if err := gif.EncodeAll(&buf, &gif.GIF{Image: []*image.Paletted{frame, frame}, Delay: []int{10, 10}}); err != nil {
		t.Fatal(err)
	}
	data := buf.Bytes()
	if err := captureGIFBudget(data); err != nil {
		t.Fatal(err)
	}
	for end := range data {
		if err := captureGIFBudget(data[:end]); err == nil {
			t.Fatalf("accepted truncated container at %d", end)
		}
	}
}
