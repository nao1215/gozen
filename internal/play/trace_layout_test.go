package play

import (
	"bytes"
	"encoding/json"
	"hash/maphash"
	"image"
	"image/color"
	"io"
	"math"
	"strconv"
	"testing"
)

func TestContrastRatio(t *testing.T) {
	t.Parallel()
	black, white := color.NRGBA{A: 255}, color.NRGBA{255, 255, 255, 255}
	if r := contrastRatio(white, black); math.Abs(r-21) > 0.01 {
		t.Fatalf("white on black %g, want 21", r)
	}
	if r := contrastRatio(black, white); math.Abs(r-21) > 0.01 {
		t.Fatalf("the order of the colors changed the ratio: %g", r)
	}
	// #767676 is the lightest gray that keeps 4.5:1 on white.
	if r := contrastRatio(color.NRGBA{0x76, 0x76, 0x76, 255}, white); r < 4.5 || r > 4.6 {
		t.Fatalf("#767676 on white %g, want just over 4.5", r)
	}
	// A half transparent white text on black shows as a middle gray.
	if got := blend(color.NRGBA{255, 255, 255, 128}, black); got.R < 127 || got.R > 129 {
		t.Fatalf("blend %v, want a middle gray", got)
	}
}

// pixels makes a w×h screen of back with the text's color on the left fifth of it.
func pixels(w, h int, back, text color.NRGBA, textCols int) []byte {
	pix := make([]byte, 4*w*h)
	for y := range h {
		for x := range w {
			c := back
			if x < textCols {
				c = text
			}
			i := 4 * (y*w + x)
			pix[i], pix[i+1], pix[i+2], pix[i+3] = c.R, c.G, c.B, 255
		}
	}
	return pix
}

func TestBackColor(t *testing.T) {
	t.Parallel()
	b := image.Rect(0, 0, 50, 10)
	gray, white := color.NRGBA{90, 90, 90, 255}, color.NRGBA{250, 250, 250, 255}
	back, ok := backColor(pixels(50, 10, gray, white, 10), b, b, white)
	if !ok || back != gray {
		t.Fatalf("back %v %v, want the gray behind the glyphs", back, ok)
	}
	// Nothing of the text's color shows: something drawn later covers it.
	if _, ok := backColor(pixels(50, 10, gray, white, 0), b, b, white); ok {
		t.Fatal("a covered text was measured")
	}
	// Text that is nearly the color behind it: everything is near it, and the median
	// of all the pixels stands for the back, so the ratio comes out low, not missing.
	near := color.NRGBA{100, 100, 100, 255}
	back, ok = backColor(pixels(50, 10, gray, near, 10), b, b, near)
	if !ok || contrastRatio(near, back) > 1.3 {
		t.Fatalf("back %v %v of a text close to it, want a ratio near 1", back, ok)
	}
	// A white text at alpha 200 on a dark back shows gray, farther than the threshold
	// from its own color: it is still found and measured, not taken for covered.
	dark, half := color.NRGBA{20, 20, 20, 255}, color.NRGBA{255, 255, 255, 200}
	back, ok = backColor(pixels(50, 10, dark, blend(half, dark), 10), b, b, half)
	if !ok || back != dark {
		t.Fatalf("back %v %v of a translucent text, want the dark back", back, ok)
	}
}

func TestScreenChanges(t *testing.T) {
	tr := &tracer{}
	f := &layoutFrame{controls: []screenControl{{Label: "Travel", Rect: [4]float64{10, 20, 100, 30}, Reason: "cost", Focused: true}}, texts: []layoutText{{Text: "Welcome", Size: 20}}, figures: []layoutFigure{{Picture: "guide.png"}}}
	if !tr.screenChanged(f) || tr.screenChanged(f) {
		t.Fatal("first or unchanged screen")
	}
	changes := []func(){
		func() { f.controls[0].Reason = "flag" },
		func() { f.controls[0].Enabled = true; f.controls[0].Reason = "" },
		func() { f.controls[0].Focused = false },
		func() { f.controls[0].Rect[0]++ },
		func() { f.texts[0].Text = "Next" },
		func() { f.figures[0].Picture = "other.png" },
		func() { f.phone = "page" },
		func() { f.phoneCall = true },
		func() { f.backgrounds = []string{"room.png"} },
		func() { f.speaker = "Guide" },
		func() { f.controls = nil; f.texts = nil; f.figures = nil },
	}
	for i, change := range changes {
		change()
		if !tr.screenChanged(f) || tr.screenChanged(f) {
			t.Fatalf("change %d lost or repeated", i)
		}
	}
}

func TestScreenRecordShape(t *testing.T) {
	var out bytes.Buffer
	g := &Game{tr: newTracer(&out)}
	f := &layoutFrame{controls: []screenControl{{Label: "Travel", Rect: [4]float64{10, 20, 100, 30}, Reason: "cost", Focused: true}}, texts: []layoutText{{Text: "Welcome"}}, figures: []layoutFigure{{Picture: "guide.png"}}, phone: "page", speaker: "Guide"}
	g.traceScreenEnd(f)
	g.traceScreenEnd(f)
	if g.tr.err != nil {
		t.Fatal(g.tr.err)
	}
	var record struct {
		Kind     string `json:"t"`
		Phone    string `json:"phone"`
		Speaker  string `json:"speaker"`
		Controls []struct {
			Label   string    `json:"label"`
			Enabled *bool     `json:"enabled"`
			Focused *bool     `json:"focused"`
			Reason  string    `json:"reason"`
			Rect    []float64 `json:"rect"`
		} `json:"controls"`
	}
	if err := json.Unmarshal(bytes.TrimSpace(out.Bytes()), &record); err != nil {
		t.Fatal(err)
	}
	if record.Kind != "screen" || record.Phone != "page" || record.Speaker != "Guide" || len(record.Controls) != 1 {
		t.Fatal(record)
	}
	c := record.Controls[0]
	if c.Enabled == nil || *c.Enabled || c.Focused == nil || !*c.Focused || c.Reason != "cost" || len(c.Rect) != 4 {
		t.Fatal(c)
	}
}

func BenchmarkScreenDigest(b *testing.B) {
	f := &layoutFrame{phone: "page", speaker: "Guide"}
	for range 20 {
		f.controls = append(f.controls, screenControl{Label: "Travel", Reason: "cost"})
		f.texts = append(f.texts, layoutText{Text: "The next train arrives soon.", Size: 20})
	}
	seed := maphash.MakeSeed()
	b.ReportAllocs()
	for b.Loop() {
		_ = f.digest(seed)
	}
}

func BenchmarkScreenChangedRecord(b *testing.B) {
	g := &Game{tr: newTracer(io.Discard)}
	f := &layoutFrame{phone: "page", speaker: "Guide"}
	for range 20 {
		f.controls = append(f.controls, screenControl{Label: "Travel", Reason: "cost"})
		f.texts = append(f.texts, layoutText{Text: "The next train arrives soon.", Size: 20})
	}
	b.ReportAllocs()
	for b.Loop() {
		f.controls[0].Focused = !f.controls[0].Focused
		g.traceScreenEnd(f)
	}
	if g.tr.err != nil {
		b.Fatal(g.tr.err)
	}
}

func TestScreenCanvasCoordinates(t *testing.T) {
	g := &Game{tr: &tracer{layout: &layoutFrame{
		texts:    []layoutText{{Text: "underlying", Rect: [4]float64{1, 2, 3, 4}}, {Text: "clipped", Rect: [4]float64{-10, 10, 30, 20}}, {Text: "outside", Rect: [4]float64{120, 0, 10, 10}}},
		controls: []screenControl{{Label: "Continue", Rect: [4]float64{80, 80, 40, 40}}},
	}}}
	g.clipScreenRecords(1, 0, 0, 100, 100)
	g.transformScreenRecords(1, 0, func(x, y float64) (float64, float64) { return 200 - y, 300 + x })
	f := g.tr.layout
	if len(f.texts) != 2 || f.texts[0].Rect != ([4]float64{1, 2, 3, 4}) || f.texts[1].Rect != ([4]float64{170, 300, 20, 20}) || f.controls[0].Rect != ([4]float64{100, 380, 20, 20}) {
		t.Fatalf("texts=%+v controls=%+v", f.texts, f.controls)
	}
}

func TestScreenTracingOff(t *testing.T) {
	g := &Game{}
	var ink color.Color = color.White
	if allocs := testing.AllocsPerRun(100, func() {
		g.traceControl("Continue", [4]float64{1, 2, 3, 4}, true, false, "")
		g.traceText("Hello", 1, 2, 3, 4, 20, ink)
	}); allocs != 0 {
		t.Fatalf("tracing off allocated %g", allocs)
	}
}

func TestCaptureRetainsScreenOnlyWhenRequested(t *testing.T) {
	for _, enabled := range []bool{false, true} {
		var out bytes.Buffer
		g := &Game{tr: newTracer(&out), opts: Options{capture: &captureStartup{structure: enabled}}}
		f := &layoutFrame{texts: []layoutText{{Text: "Welcome"}}}
		g.traceScreenEnd(f)
		if enabled != (len(g.opts.capture.screen) > 0) {
			t.Fatal("capture structure opt-in not respected")
		}
		if !enabled {
			continue
		}
		first := string(g.opts.capture.screen)
		g.traceScreenEnd(f)
		if string(g.opts.capture.screen) != first {
			t.Fatal("unchanged draw lost structure")
		}
		f.texts[0].Text = "Goodbye"
		g.traceScreenEnd(f)
		if string(g.opts.capture.screen) == first {
			t.Fatal("changed draw retained old structure")
		}
	}
}

func TestCaptureStructureCollectionIsOptional(t *testing.T) {
	for _, enabled := range []bool{false, true} {
		t.Run(strconv.FormatBool(enabled), func(t *testing.T) {
			g := storyGame(t, Options{Location: "camp", Trace: io.Discard}, "schema_version: 4\n")
			g.opts.capture = &captureStartup{structure: enabled}
			g.traceFrameStart()
			if (g.tr.layout != nil) != enabled {
				t.Fatalf("structure=%t still collected layout=%t", enabled, g.tr.layout != nil)
			}
			if g.tr.drawn == nil || g.tr.stand == nil || g.pics.seen == nil {
				t.Fatal("disabling structure disabled capture asset tracking")
			}
		})
	}
}
