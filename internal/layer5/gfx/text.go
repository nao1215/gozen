package gfx

import (
	"bytes"
	"image/color"
	"math"
	"slices"
	"strings"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/text/v2"
)

// LoadTitleFont sets the typefaces of the title and the subtitle (nil: the regular one,
// and for the subtitle the title's).
func LoadTitleFont(title, subtitle []byte) error {
	active.titleSource, active.subSource = nil, nil
	for _, f := range []struct {
		b   []byte
		dst **text.GoTextFaceSource
	}{{title, &active.titleSource}, {subtitle, &active.subSource}} {
		if f.b == nil {
			continue
		}
		src, err := text.NewGoTextFaceSource(bytes.NewReader(f.b))
		if err != nil {
			return err
		}
		*f.dst = src
	}
	return nil
}

// DrawTitleText draws the title (or with subtitle the subtitle) centered on x.
func DrawTitleText(dst *ebiten.Image, s string, x, y, size, tracking float64, fill, edge color.Color, alpha float32, subtitle bool) {
	src := active.titleSource
	if subtitle && active.subSource != nil {
		src = active.subSource
	}
	if src == nil {
		src = active.regularSource
	}
	x, y, size = x*active.scale, y*active.scale, size*active.scale
	face := &text.GoTextFace{Source: src, Size: size}
	runes := []rune(s)
	widths := make([]float64, len(runes))
	total := 0.0
	for i, r := range runes {
		w, _ := text.Measure(string(r), face, size)
		widths[i] = w
		total += w
	}
	gap := size * tracking
	total += gap * float64(max(0, len(runes)-1))
	draw := func(dx, dy float64, c color.Color, a float32) {
		cx := x - total/2
		for i, r := range runes {
			op := &text.DrawOptions{}
			op.GeoM.Translate(cx+dx, y+dy)
			op.ColorScale.ScaleWithColor(c)
			op.ColorScale.ScaleAlpha(a)
			op.Filter = ebiten.FilterLinear
			drawGlyphText(dst, string(r), face, op)
			cx += widths[i] + gap
		}
	}
	shadow := size * 0.05
	draw(shadow, shadow, color.Black, alpha*0.45)
	e := math.Max(1.5, size*0.025)
	for _, d := range [][2]float64{{-e, 0}, {e, 0}, {0, -e}, {0, e}, {-e * 0.7, -e * 0.7}, {e * 0.7, -e * 0.7}, {-e * 0.7, e * 0.7}, {e * 0.7, e * 0.7}} {
		draw(d[0], d[1], edge, alpha)
	}
	draw(0, 0, fill, alpha)
}

// LoadFonts replaces fonts atomically and invalidates measurements and images.
func LoadFonts(regular, pop []byte) error {
	src, err := text.NewGoTextFaceSource(bytes.NewReader(regular))
	if err != nil {
		return err
	}
	var ps *text.GoTextFaceSource
	if pop != nil {
		ps, err = text.NewGoTextFaceSource(bytes.NewReader(pop))
		if err != nil {
			return err
		}
	}
	Reset()
	active.regularSource, active.popSource = src, ps
	return nil
}

type faceKey struct {
	size float64
	pop  bool
}

// popOn is set inside Pop, and popFaces are its faces by size: the pop font, the regular
// one for a glyph it lacks.

// Pop runs draw with its text drawn and measured in the pop font (fonts.pop) wherever it
// has the glyph, a glyph it lacks in the regular font, for the decorations of a screen (a
// title in a bar, a name on a plate); without a pop font it changes nothing. Outside Pop
// only text all ASCII takes the pop font (Face). Use only on the game thread.
func Pop(draw func()) {
	active.popOn = true
	defer func() { active.popOn = false }()
	draw()
}

// face is the face text s is drawn in at size: Face, or inside Pop the pop font over the
// regular one.
func face(size float64, s string) text.Face {
	if !active.popOn || active.popSource == nil {
		return Face(size, s)
	}
	f, ok := active.popFaces[size]
	if !ok {
		var err error
		if f, err = text.NewMultiFace(&text.GoTextFace{Source: active.popSource, Size: size}, &text.GoTextFace{Source: active.regularSource, Size: size}); err != nil {
			return Face(size, s)
		}
		active.popFaces[size] = f
	}
	return f
}

func Face(size float64, s string) *text.GoTextFace {
	pop := active.popSource != nil && isASCII(s)
	k := faceKey{size, pop}
	f, ok := active.faceCache[k]
	if !ok {
		src := active.regularSource
		if pop {
			src = active.popSource
		}
		f = &text.GoTextFace{Source: src, Size: size}
		active.faceCache[k] = f
	}
	return f
}

func isASCII(s string) bool {
	for _, r := range s {
		if r > 0x7e {
			return false
		}
	}
	return true
}

var baseSizes = []float64{12, 18, 24, 30, 36, 44, 54, 66, 80, 110, 150, 210, 300}

func baseSize(size float64) float64 {
	for _, b := range baseSizes {
		if size <= b {
			return b
		}
	}
	return baseSizes[len(baseSizes)-1]
}

func DrawText(dst *ebiten.Image, s string, x, y, size float64, clr color.Color) {
	drawTextRaw(dst, s, x*active.scale, y*active.scale, size*active.scale, clr)
}

func drawTextRaw(dst *ebiten.Image, s string, x, y, size float64, clr color.Color) {
	if size <= 0 {
		return
	}
	base := baseSize(size)
	k := size / base
	op := &text.DrawOptions{}
	op.GeoM.Scale(k, k)
	op.GeoM.Translate(x, y)
	op.ColorScale.ScaleWithColor(clr)
	op.LineSpacing = base * 1.4
	op.PrimaryAlign = text.AlignCenter
	op.Filter = ebiten.FilterLinear
	drawGlyphText(dst, s, face(base, s), op)
}

// drawGlyphText preserves text/v2 shaping and alignment, but samples a fixed
// atlas. Moving glyphs within Ebitengine's shared atlas changes floating-point
// texture coordinates and can round filtered edge pixels differently.
func drawGlyphText(dst *ebiten.Image, s string, face text.Face, options *text.DrawOptions) {
	active.drawGlyphBuffer = text.AppendLazyGlyphs(active.drawGlyphBuffer[:0], s, face, &options.LayoutOptions)
	bounds := dst.Bounds()
	for _, glyph := range active.drawGlyphBuffer {
		r := glyph.ImageBounds
		if r.Empty() {
			continue
		}
		// These callers only translate and positively scale. Avoid realizing
		// glyphs outside the destination, including long scrolling text.
		x0, y0 := options.GeoM.Apply(float64(r.Min.X), float64(r.Min.Y))
		x1, y1 := options.GeoM.Apply(float64(r.Max.X), float64(r.Max.Y))
		if x1 <= float64(bounds.Min.X) || y1 <= float64(bounds.Min.Y) || x0 >= float64(bounds.Max.X) || y0 >= float64(bounds.Max.Y) {
			continue
		}
		src := glyph.Image()
		if src == nil {
			continue
		}
		img := stableGlyph(src)
		op := options.DrawImageOptions
		op.GeoM.Reset()
		op.GeoM.Translate(float64(r.Min.X), float64(r.Min.Y))
		op.GeoM.Concat(options.GeoM)
		if glyph.Colored() {
			op.ColorScale.Reset()
			op.ColorScale.ScaleAlpha(options.ColorScale.A())
		}
		dst.DrawImage(img, &op)
	}
	clear(active.drawGlyphBuffer)
	active.drawGlyphBuffer = active.drawGlyphBuffer[:0]
}

func textMeasure(s string, size float64) (float64, float64) {
	base := baseSize(size)
	w, h := text.Measure(s, face(base, s), base*1.4)
	return w * size / base, h * size / base
}

func DrawTextOutline(dst *ebiten.Image, str string, x, y, size float64, clr color.Color) {
	DrawTextOutlineColor(dst, str, x, y, size, clr, color.White, 1)
}

func DrawTextOutlineColor(dst *ebiten.Image, str string, x, y, size float64, fill, outline color.Color, alpha float32) {
	if size <= 0 || alpha <= 0 || str == "" {
		return
	}
	x, y, size = x*active.scale, y*active.scale, size*active.scale

	base := baseSize(size)
	t := outlinedText(str, base, toNRGBA(fill), toNRGBA(outline))
	k := size / base
	op := &ebiten.DrawImageOptions{}
	op.GeoM.Translate(-t.cx, -t.cy)
	op.GeoM.Scale(k, k)
	op.GeoM.Translate(x, y)
	op.ColorScale.ScaleAlpha(alpha)
	op.Filter = ebiten.FilterLinear
	dst.DrawImage(t.img, op)
}

func TextWidth(s string, size float64) float64 {
	return textWidthRaw(s, size*active.scale) / active.scale
}

func textWidthRaw(s string, size float64) float64 {
	if size <= 0 {
		return 0
	}
	base := baseSize(size)
	w, _ := text.Measure(s, face(base, s), base*1.4)
	return w * size / base
}

func DrawTextLeft(dst *ebiten.Image, s string, x, y, size float64, clr color.Color) {
	if size <= 0 || s == "" {
		return
	}
	x, y, size = x*active.scale, y*active.scale, size*active.scale
	base := baseSize(size)
	k := size / base
	op := &text.DrawOptions{}
	op.GeoM.Scale(k, k)
	op.GeoM.Translate(x, y)
	op.ColorScale.ScaleWithColor(clr)
	op.LineSpacing = base * 1.4
	op.PrimaryAlign = text.AlignStart
	op.Filter = ebiten.FilterLinear
	drawGlyphText(dst, s, face(base, s), op)
}

func DrawTextRight(dst *ebiten.Image, s string, x, y, size float64, clr color.Color) {
	DrawTextLeft(dst, s, x-TextWidth(s, size), y, size, clr)
}

func Wrap(s string, size, width float64) []string {
	k := fitKey{s, size, width, active.scale, active.popOn}
	if lines, ok := active.wrapCache[k]; ok {
		return slices.Clone(lines)
	}
	lines := wrapWith(s, func(line []rune) bool { return TextWidth(string(line), size) <= width })
	remember(active.wrapCache, &active.wrapBytes, k, lines)
	return slices.Clone(lines)
}

// fitKey identifies a text fitted to a width under the active scale and face.
// Fonts change only through LoadFonts, which calls Reset and drops the results.
type fitKey struct {
	s                  string
	size, width, scale float64
	pop                bool
}

// fitByteLimit bounds the bytes of texts and results one fitted-text cache keeps.
const fitByteLimit = 4 << 20

// remember keeps a fitted result for the draws of the next frames. The cache
// holds at most cacheLimit texts and fitByteLimit bytes, counted in used; a
// full cache starts over, and a result larger than the budget is not kept. The
// key owns its text so a cached label never retains a larger dialogue source.
func remember[V string | []string](cache map[fitKey]V, used *int, k fitKey, v V) {
	size := len(k.s)
	switch v := any(v).(type) {
	case string:
		size += len(v)
	case []string:
		for _, l := range v {
			size += len(l)
		}
	}
	if size > fitByteLimit {
		return
	}
	if len(cache) >= cacheLimit || *used+size > fitByteLimit {
		clear(cache)
		*used = 0
	}
	k.s = strings.Clone(k.s)
	cache[k] = v
	*used += size
}

func wrapWith(s string, fits func([]rune) bool) []string {
	var out []string
	for _, para := range splitLines(s) {
		line := []rune{}
		for _, r := range para {
			next := append(line, r)
			if len(line) > 0 && !fits(next) {

				cut := -1
				for i := len(line) - 1; i > 0; i-- {
					if line[i] == ' ' && !numberUnit(line, i, r) {
						cut = i
						break
					}
				}

				if cut > 0 && cut*3 < len(line)*2 {
					cut = -1
				}
				if cut > 0 && r != ' ' {
					head := r
					if cut+1 < len(line) {
						head = line[cut+1]
					}
					if !noLineStart(head) {
						out = append(out, string(line[:cut]))
						line = append([]rune{}, line[cut+1:]...)
						line = append(line, r)
						continue
					}
				}

				k := len(line)
				if noLineStart(r) {
					for k > 0 && noLineStart(line[k-1]) {
						k--
					}
					k--
				}
				for k > 0 && k <= len(line) && noLineEnd(line[k-1]) {
					k--
				}

				if k > 0 && k <= len(line) {
					after := r
					if k < len(line) {
						after = line[k]
					}
					// A number keeps the unit after it (a count and its counter word): the line breaks before the number.
					if j := numberStart(line, k); j >= 0 && after >= 0x80 && !isClosing(after) && j*2 >= len(line) {
						k = j
					} else if isWordRune(line[k-1]) && isWordRune(after) {
						j := k
						for j > 0 && isWordRune(line[j-1]) {
							j--
						}
						if j*2 >= len(line) {
							k = j
						}
					}
				}
				if k > 0 && k < len(line) {
					out = append(out, string(line[:k]))
					line = append(append([]rune{}, line[k:]...), r)
					continue
				}
				out = append(out, string(line))
				line = []rune{r}
				continue
			}
			line = next
		}
		out = append(out, string(line))
	}
	return out
}

// numberUnit reports whether the space at line[i] joins a number to a unit written
// after it in another script; next is the rune after the line.
func numberUnit(line []rune, i int, next rune) bool {
	after := next
	if i+1 < len(line) {
		after = line[i+1]
	}
	return i > 0 && line[i-1] >= '0' && line[i-1] <= '9' && after >= 0x80
}

// numberStart is where the number ending at line[k-1] (or before a space there)
// begins, -1 when no number ends there.
func numberStart(line []rune, k int) int {
	j := k
	if j > 1 && line[j-1] == ' ' {
		j--
	}
	if j == 0 || line[j-1] < '0' || line[j-1] > '9' {
		return -1
	}
	for j > 0 && isWordRune(line[j-1]) {
		j--
	}
	return j
}

func isWordRune(r rune) bool {
	return r < 0x80 && (r >= '0' && r <= '9' || r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z')
}

func noLineStart(r rune) bool {
	if isClosing(r) {
		return true
	}
	switch r {
	case '—', '―', '〜', '～', '】', '〕', '］', '｝', '〉', '》', ')', ']', '}', '・', '：', '；', '，', '．',
		'ぁ', 'ぃ', 'ぅ', 'ぇ', 'ぉ', 'ゎ', 'ァ', 'ィ', 'ゥ', 'ェ', 'ォ', 'ッ', 'ャ', 'ュ', 'ョ', 'ヮ', '♪', '‥':
		return true
	}
	return false
}

func noLineEnd(r rune) bool {
	switch r {
	case '「', '『', '（', '【', '〔', '［', '｛', '〈', '《', '(', '[', '{':
		return true
	}
	return false
}

func splitLines(s string) []string {
	var out []string
	start := 0
	for i, r := range s {
		if r == '\n' {
			out = append(out, s[start:i])
			start = i + 1
		}
	}
	return append(out, s[start:])
}

func isClosing(r rune) bool {
	switch r {
	case '。', '、', '」', '』', '）', '！', '？', '…', 'ー', 'っ', 'ゃ', 'ゅ', 'ょ', '!', '?', ',', '.':
		return true
	}
	return false
}

func LineHeight(size float64) float64 { return size * 1.4 }

func Ellipsis(s string, size, width float64) string {
	if TextWidth(s, size) <= width {
		return s
	}
	k := fitKey{s, size, width, active.scale, active.popOn}
	if t, ok := active.ellipsisCache[k]; ok {
		return t
	}
	t := "…"
	r := []rune(s)
	for len(r) > 0 {
		r = r[:len(r)-1]
		if c := string(r) + "…"; TextWidth(c, size) <= width {
			t = c
			break
		}
	}
	remember(active.ellipsisCache, &active.ellipsisBytes, k, t)
	return t
}

func FitLines(s string, size, width float64, lines int) ([]string, float64) {
	ls, sz, _ := FitLinesCut(s, size, width, lines)
	return ls, sz
}

func FitLinesCut(s string, size, width float64, lines int) ([]string, float64, bool) {
	if size <= 0 || math.IsNaN(size) || math.IsInf(size, 0) {
		return nil, 0, true
	}
	if lines < 1 {
		lines = 1
	}
	sz := size
	for {
		ls := Wrap(s, sz, width)
		for i := 1; i < len(ls); i++ {
			ls[i] = strings.TrimLeft(ls[i], " ")
		}
		if len(ls) <= lines {
			return ls, sz, false
		}
		if sz*0.94 < size*0.75 {
			ls = ls[:lines]
			ls[lines-1] = Ellipsis(ls[lines-1]+"…", sz, width)
			return ls, sz, true
		}
		sz *= 0.94
	}
}
