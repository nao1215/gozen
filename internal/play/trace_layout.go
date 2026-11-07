package play

import (
	"encoding/json"
	"fmt"
	"hash/maphash"
	"image"
	"image/color"
	"math"
	"slices"
	"strings"

	"github.com/hajimehoshi/ebiten/v2"

	"github.com/nao1215/gozen/internal/layer5/gfx"
)

// The layout record of a trace says where the standing pictures and the texts of a
// stable frame were drawn, so a game's own checks (its linter, its screenshot review)
// can find a head cut off at the top of a frame, a figure standing above where it
// should, or a text hard to read on what is behind it. gozen only records; it does not
// judge, because what is intended (a close-up, a figure leaving the screen, a cut-in)
// is the game's to say. Everything here runs only with --trace.

// layoutFrame gathers what one frame drew; the tracer turns it into a layout line once
// it has stayed the same for layoutStable ticks.
type layoutFrame struct {
	figures     []layoutFigure
	texts       []layoutText
	controls    []screenControl
	backgrounds []string
	phone       string
	phoneCall   bool
	speaker     string
	cast        []standFig
}

// screenControl describes a drawn hit target; reason is factual and empty when enabled.
type screenControl struct {
	Label   string     `json:"label"`
	Rect    [4]float64 `json:"rect"`
	Enabled bool       `json:"enabled"`
	Reason  string     `json:"reason,omitempty"`
	Focused bool       `json:"focused"`
}

func (g *Game) traceControl(label string, r [4]float64, enabled, focused bool, reason string) {
	if g.tr == nil || g.tr.layout == nil {
		return
	}
	if enabled {
		reason = ""
	} else if reason == "" {
		reason = "disabled"
	}
	g.tr.layout.controls = append(g.tr.layout.controls, screenControl{label, r, enabled, reason, focused})
}

// digest hashes comparable drawing facts without formatting, allocation or pixel reads.
// The seed is local to the recorder; only equality within a run matters.
func (f *layoutFrame) digest(seed maphash.Seed) uint64 {
	var h maphash.Hash
	h.SetSeed(seed)
	add := func(v uint64) {
		for range 8 {
			_ = h.WriteByte(byte(v)) //nolint:errcheck // maphash.WriteByte always returns nil.
			v >>= 8
		}
	}
	add(uint64(len(f.figures)))
	for _, v := range f.figures {
		add(maphash.String(seed, v.Picture))
		add(maphash.Comparable(seed, v.Rect))
		add(maphash.Comparable(seed, v.Content))
		add(maphash.Comparable(seed, v.Clip))
		add(math.Float64bits(v.Alpha))
		add(uint64(len(v.SourceCut)))
		for _, edge := range v.SourceCut {
			add(maphash.String(seed, edge))
		}
	}
	add(uint64(len(f.texts)))
	for _, v := range f.texts {
		add(maphash.Comparable(seed, v))
	}
	add(uint64(len(f.controls)))
	for _, v := range f.controls {
		add(maphash.Comparable(seed, v))
	}
	add(uint64(len(f.backgrounds)))
	for _, v := range f.backgrounds {
		add(maphash.String(seed, v))
	}
	add(uint64(len(f.cast)))
	for _, v := range f.cast {
		add(maphash.Comparable(seed, v))
	}
	add(maphash.String(seed, f.phone))
	add(maphash.Comparable(seed, f.phoneCall))
	add(maphash.String(seed, f.speaker))
	return h.Sum64()
}

func (tr *tracer) screenChanged(f *layoutFrame) bool {
	if tr.screenSeed == (maphash.Seed{}) {
		tr.screenSeed = maphash.MakeSeed()
	}
	digest := f.digest(tr.screenSeed)
	if tr.screenSeen && digest == tr.screenDigest {
		return false
	}
	tr.screenSeen, tr.screenDigest = true, digest
	return true
}

func (g *Game) traceScreenEnd(f *layoutFrame) {
	if !g.tr.screenChanged(f) {
		return
	}
	record := traceLine{
		"controls": f.controls, "texts": f.texts,
		"figures": f.figures, "cast": f.cast, "backgrounds": f.backgrounds,
		"phone": f.phone, "phone_call": f.phoneCall, "speaker": f.speaker,
	}
	if c := g.opts.capture; c != nil && c.structure {
		var err error
		c.screen, err = json.Marshal(record)
		if err != nil {
			g.tr.err = err
		}
	}
	g.emit("screen", record)
}

// layoutFigure is one standing picture as drawn: rectangles in UI units of the screen.
type layoutFigure struct {
	Picture string `json:"picture"`
	// Rect is where the whole picture (its canvas) is drawn.
	Rect [4]float64 `json:"rect"`
	// Content is where its opaque part is drawn: the part a player sees.
	Content [4]float64 `json:"content"`
	// Clip is what it is drawn into: the screen, or the frame (a call's view) that cuts it.
	Clip [4]float64 `json:"clip"`
	// SourceCut names the edges (top, bottom) the opaque part already touches in the
	// picture itself: drawn cut, not cut by where it stands.
	SourceCut []string `json:"source_cut,omitempty"`
	Alpha     float64  `json:"alpha"`
}

// layoutText is one text as drawn and what is behind it.
type layoutText struct {
	Text  string     `json:"text"`
	Rect  [4]float64 `json:"rect"`
	Size  float64    `json:"size"`
	Color string     `json:"color"`
	// Back is the color behind the text, read from the finished frame; Ratio is the WCAG
	// contrast ratio of the text (blended by its alpha) on it.
	// Both are left out when the screen cannot be read back: a game drawn outside the
	// Ebitengine loop (offscreen tests) has no pixels to read.
	Back  string  `json:"back,omitempty"`
	Ratio float64 `json:"ratio,omitempty"`

	fill color.NRGBA
}

// layoutStable is how many ticks the layout must stay the same (over two frames at
// least) before it is written: figures rising or fading and texts typing out are not
// recorded halfway.
const layoutStable = 10

// traceFigure records a standing picture of path drawn into clip (physical pixels)
// with its canvas at left, top and scale s (UI units), mirrored when flip.
func (g *Game) traceFigure(path string, left, top, s float64, flip bool, alpha float32, clip image.Rectangle) {
	if g.tr == nil || g.tr.layout == nil || alpha < 0.05 || path == "" {
		return
	}
	img := g.pics.get(path)
	if img == nil || img == missing {
		return
	}
	b := img.Bounds()
	c := g.pics.content(path)
	iw, ih := float64(b.Dx()), float64(b.Dy())
	cx0, cx1 := float64(c.Min.X), float64(c.Max.X)
	if flip {
		cx0, cx1 = iw-cx1, iw-cx0
	}
	k := gfx.Scale()
	f := layoutFigure{
		Picture: path,
		Rect:    roundRect(left, top, iw*s, ih*s),
		Content: roundRect(left+cx0*s, top+float64(c.Min.Y)*s, (cx1-cx0)*s, float64(c.Dy())*s),
		Clip:    roundRect(float64(clip.Min.X)/k, float64(clip.Min.Y)/k, float64(clip.Dx())/k, float64(clip.Dy())/k),
		Alpha:   math.Round(float64(alpha)*100) / 100,
	}
	if c.Min.Y <= 0 {
		f.SourceCut = append(f.SourceCut, "top")
	}
	if c.Max.Y >= b.Dy() {
		f.SourceCut = append(f.SourceCut, "bottom")
	}
	g.tr.layout.figures = append(g.tr.layout.figures, f)
}

// traceText records a text whose box is x, y (its top left), w, h in UI units.
func (g *Game) traceText(s string, x, y, w, h, size float64, c color.Color) {
	if g.tr == nil || g.tr.layout == nil || strings.TrimSpace(s) == "" || size <= 0 {
		return
	}
	n, ok := color.NRGBAModel.Convert(c).(color.NRGBA)
	if !ok || n.A < 13 {
		return
	}
	g.tr.layout.texts = append(g.tr.layout.texts, layoutText{
		Text: s, Rect: roundRect(x, y, w, h), Size: size, Color: hexColor(n), fill: n,
	})
}

// traceLeftText records a text drawn by gfx.DrawTextLeft at x, y.
func (g *Game) traceLeftText(s string, x, y, size float64, c color.Color) {
	if g.tr == nil || g.tr.layout == nil {
		return
	}
	lines := float64(strings.Count(s, "\n") + 1)
	g.traceText(s, x, y, gfx.TextWidth(s, size), gfx.LineHeight(size)*lines, size, c)
}

func roundRect(x, y, w, h float64) [4]float64 {
	r := func(v float64) float64 { return math.Round(v*10) / 10 }
	return [4]float64{r(x), r(y), r(w), r(h)}
}

func hexColor(c color.NRGBA) string { return fmt.Sprintf("#%02x%02x%02x", c.R, c.G, c.B) }

// key is what must stay the same for the layout to count as stable. Positions are
// taken to eight units and alphas to eighths, so a figure that bobs a little still
// settles, while one sliding in or a text fading in does not count until it stops.
func (f *layoutFrame) key() string {
	q := func(r [4]float64) [4]int {
		return [4]int{int(math.Round(r[0] / 8)), int(math.Round(r[1] / 8)), int(math.Round(r[2] / 8)), int(math.Round(r[3] / 8))}
	}
	var b strings.Builder
	for _, fg := range f.figures {
		fmt.Fprintf(&b, "%s%v%v%d;", fg.Picture, q(fg.Rect), fg.Clip, int(fg.Alpha*8))
	}
	b.WriteByte('|')
	for _, t := range f.texts {
		fmt.Fprintf(&b, "%s%v%s%d;", t.Text, q(t.Rect), t.Color, t.fill.A/32)
	}
	return b.String()
}

// traceLayoutEnd writes the frame's layout once it is stable. Only then is the screen
// read back (one read for the whole line), to measure what is behind each text.
func (g *Game) traceLayoutEnd(screen *ebiten.Image) {
	tr := g.tr
	f := tr.layout
	tr.layout = nil
	if f == nil {
		return
	}
	g.traceScreenEnd(f)
	// A shaking, flashing or fading screen is not read: what is behind the texts
	// would be the flash, not the screen.
	if g.shake > 0 || g.flash > 0 || g.fadeT > 0 || g.freezeT > 0 {
		tr.layoutN, tr.layoutPending = 0, ""
		return
	}
	key := f.key()
	if key != tr.layoutPending {
		tr.layoutPending, tr.layoutN, tr.layoutAt = key, 0, g.tick
	}
	tr.layoutN++
	// Ticks, not frames: a heavy screen is drawn every few ticks under a slow display.
	if tr.layoutN < 2 || g.tick-tr.layoutAt < layoutStable || key == tr.layoutLast {
		return
	}
	tr.layoutLast = key
	if len(f.figures) == 0 && len(f.texts) == 0 {
		return
	}
	l := traceLine{"since": tr.layoutAt}
	if len(f.figures) > 0 {
		l["figures"] = f.figures
	}
	if len(f.texts) > 0 {
		texts := f.texts
		if g.loopStarted {
			texts = measureTexts(screen, f.texts, gfx.Scale())
		}
		if len(texts) > 0 {
			l["texts"] = texts
		}
	}
	g.emit("layout", l)
}

// measureTexts reads the screen once and gives each text the color behind it and its
// contrast ratio on it. A text covered by what was drawn later (fewer than one pixel in
// a hundred of its color shows) is left out: the player does not see it.
func measureTexts(screen *ebiten.Image, texts []layoutText, k float64) []layoutText {
	b := screen.Bounds()
	pix := make([]byte, 4*b.Dx()*b.Dy())
	screen.ReadPixels(pix)
	out := texts[:0]
	for _, t := range texts {
		r := image.Rect(int(t.Rect[0]*k), int(t.Rect[1]*k), int(math.Ceil((t.Rect[0]+t.Rect[2])*k)), int(math.Ceil((t.Rect[1]+t.Rect[3])*k))).Intersect(b)
		if r.Empty() {
			continue
		}
		back, ok := backColor(pix, b, r, t.fill)
		if !ok {
			continue
		}
		t.Back = hexColor(back)
		t.Ratio = math.Round(contrastRatio(blend(t.fill, back), back)*100) / 100
		out = append(out, t)
	}
	return out
}

// backColor is the median (by luminance) of the pixels of r that are not near the
// text as it shows: the glyphs are left out, what is behind them stays. A text with
// alpha shows blended with what is behind it, so the glyphs are looked for in that
// blend, over the median of the whole box first.
func backColor(pix []byte, b, r image.Rectangle, text color.NRGBA) (color.NRGBA, bool) {
	all := make([]color.NRGBA, 0, r.Dx()*r.Dy())
	for y := r.Min.Y; y < r.Max.Y; y++ {
		for x := r.Min.X; x < r.Max.X; x++ {
			i := 4 * ((y-b.Min.Y)*b.Dx() + x - b.Min.X)
			all = append(all, color.NRGBA{pix[i], pix[i+1], pix[i+2], 0xff})
		}
	}
	shown := blend(text, median(slices.Clone(all)))
	var near int
	var rest []color.NRGBA
	for _, c := range all {
		if colorDistance(c, shown) < 48 {
			near++
			continue
		}
		rest = append(rest, c)
	}
	if near*100 < len(all) {
		return color.NRGBA{}, false
	}
	if len(rest)*10 < len(all) {
		rest = all
	}
	return median(rest), true
}

// median is the middle color by luminance. The weights of luminance on the stored
// values order the pixels well enough, without the power curve per pixel.
func median(cs []color.NRGBA) color.NRGBA {
	key := func(c color.NRGBA) int { return 2126*int(c.R) + 7152*int(c.G) + 722*int(c.B) }
	slices.SortFunc(cs, func(a, b color.NRGBA) int { return key(a) - key(b) })
	return cs[len(cs)/2]
}

func colorDistance(a, b color.NRGBA) int {
	d := func(x, y uint8) int { return max(int(x)-int(y), int(y)-int(x)) }
	return max(d(a.R, b.R), d(a.G, b.G), d(a.B, b.B))
}

// blend is the text's color as it shows over back: its alpha mixes them.
func blend(c, back color.NRGBA) color.NRGBA {
	a := float64(c.A) / 255
	m := func(x, y uint8) uint8 { return uint8(math.Round(float64(x)*a + float64(y)*(1-a))) }
	return color.NRGBA{m(c.R, back.R), m(c.G, back.G), m(c.B, back.B), 0xff}
}

// luminance is the WCAG 2 relative luminance of an opaque color.
func luminance(c color.NRGBA) float64 {
	ch := func(v uint8) float64 {
		s := float64(v) / 255
		if s <= 0.04045 {
			return s / 12.92
		}
		return math.Pow((s+0.055)/1.055, 2.4)
	}
	return 0.2126*ch(c.R) + 0.7152*ch(c.G) + 0.0722*ch(c.B)
}

// contrastRatio is the WCAG 2 contrast ratio of two opaque colors, from 1 to 21.
func contrastRatio(a, b color.NRGBA) float64 {
	la, lb := luminance(a), luminance(b)
	if la < lb {
		la, lb = lb, la
	}
	return (la + 0.05) / (lb + 0.05)
}

// These text helpers also feed the existing layout collector for direct gfx callers.
func (g *Game) screenText(dst *ebiten.Image, s string, x, y, size float64, c color.Color) {
	gfx.DrawText(dst, s, x, y, size, c)
	if g.tr != nil {
		g.traceLeftText(s, x-gfx.TextWidth(s, size)/2, y, size, c)
	}
}

func (g *Game) screenTextRight(dst *ebiten.Image, s string, x, y, size float64, c color.Color) {
	gfx.DrawTextRight(dst, s, x, y, size, c)
	if g.tr != nil {
		g.traceLeftText(s, x-gfx.TextWidth(s, size), y, size, c)
	}
}

func (g *Game) screenOffsets() (int, int) {
	if g.tr == nil || g.tr.layout == nil {
		return 0, 0
	}
	return len(g.tr.layout.texts), len(g.tr.layout.controls)
}

// The phone draws on intermediate canvases. Convert their boxes into screen UI units.
func (g *Game) transformScreenRecords(textStart, controlStart int, transform func(float64, float64) (float64, float64)) {
	if g.tr.layout == nil {
		return
	}
	box := func(r [4]float64) [4]float64 {
		x0, y0 := transform(r[0], r[1])
		x1, y1 := x0, y0
		for _, p := range [][2]float64{{r[0] + r[2], r[1]}, {r[0], r[1] + r[3]}, {r[0] + r[2], r[1] + r[3]}} {
			x, y := transform(p[0], p[1])
			x0, y0 = min(x0, x), min(y0, y)
			x1, y1 = max(x1, x), max(y1, y)
		}
		return roundRect(x0, y0, x1-x0, y1-y0)
	}
	f := g.tr.layout
	for i := textStart; i < len(f.texts); i++ {
		f.texts[i].Rect = box(f.texts[i].Rect)
	}
	for i := controlStart; i < len(f.controls); i++ {
		f.controls[i].Rect = box(f.controls[i].Rect)
	}
}

func (g *Game) clipScreenRecords(textStart, controlStart int, w, h float64) {
	if g.tr.layout == nil {
		return
	}
	clip := func(r [4]float64) [4]float64 {
		x, y := max(0, r[0]), max(0, r[1])
		return roundRect(x, y, max(0, min(w, r[0]+r[2])-x), max(0, min(h, r[1]+r[3])-y))
	}
	f := g.tr.layout
	texts := f.texts[:textStart]
	for _, t := range f.texts[textStart:] {
		t.Rect = clip(t.Rect)
		if t.Rect[2] > 0 && t.Rect[3] > 0 {
			texts = append(texts, t)
		}
	}
	f.texts = texts
	controls := f.controls[:controlStart]
	for _, c := range f.controls[controlStart:] {
		c.Rect = clip(c.Rect)
		if c.Rect[2] > 0 && c.Rect[3] > 0 {
			controls = append(controls, c)
		}
	}
	f.controls = controls
}
