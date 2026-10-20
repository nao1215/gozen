package gfx

import (
	"bytes"
	"image/color"
	"math"
	"strings"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/text/v2"
)

var (
	regularSource *text.GoTextFaceSource
	popSource     *text.GoTextFaceSource
	titleSource   *text.GoTextFaceSource
	subSource     *text.GoTextFaceSource
)

// LoadTitleFont sets the typefaces of the title and the subtitle (nil: the regular one,
// and for the subtitle the title's).
func LoadTitleFont(title, subtitle []byte) error {
	titleSource, subSource = nil, nil
	for _, f := range []struct {
		b   []byte
		dst **text.GoTextFaceSource
	}{{title, &titleSource}, {subtitle, &subSource}} {
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
	src := titleSource
	if subtitle && subSource != nil {
		src = subSource
	}
	if src == nil {
		src = regularSource
	}
	x, y, size = x*scale, y*scale, size*scale
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
			text.Draw(dst, string(r), face, op)
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
	regularSource, popSource = src, ps
	return nil
}

type faceKey struct {
	size float64
	pop  bool
}

var faceCache = map[faceKey]*text.GoTextFace{}

func Face(size float64, s string) *text.GoTextFace {
	pop := popSource != nil && isASCII(s)
	k := faceKey{size, pop}
	f, ok := faceCache[k]
	if !ok {
		src := regularSource
		if pop {
			src = popSource
		}
		f = &text.GoTextFace{Source: src, Size: size}
		faceCache[k] = f
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
	drawTextRaw(dst, s, x*scale, y*scale, size*scale, clr)
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
	text.Draw(dst, s, Face(base, s), op)
}

func textMeasure(s string, size float64) (float64, float64) {
	base := baseSize(size)
	w, h := text.Measure(s, Face(base, s), base*1.4)
	return w * size / base, h * size / base
}

func DrawTextOutline(dst *ebiten.Image, str string, x, y, size float64, clr color.Color) {
	DrawTextOutlineColor(dst, str, x, y, size, clr, color.White, 1)
}

func DrawTextOutlineColor(dst *ebiten.Image, str string, x, y, size float64, fill, outline color.Color, alpha float32) {
	if size <= 0 || alpha <= 0 || str == "" {
		return
	}
	x, y, size = x*scale, y*scale, size*scale

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
	return textWidthRaw(s, size*scale) / scale
}

func textWidthRaw(s string, size float64) float64 {
	if size <= 0 {
		return 0
	}
	base := baseSize(size)
	w, _ := text.Measure(s, Face(base, s), base*1.4)
	return w * size / base
}

func DrawTextLeft(dst *ebiten.Image, s string, x, y, size float64, clr color.Color) {
	if size <= 0 || s == "" {
		return
	}
	x, y, size = x*scale, y*scale, size*scale
	base := baseSize(size)
	k := size / base
	op := &text.DrawOptions{}
	op.GeoM.Scale(k, k)
	op.GeoM.Translate(x, y)
	op.ColorScale.ScaleWithColor(clr)
	op.LineSpacing = base * 1.4
	op.PrimaryAlign = text.AlignStart
	op.Filter = ebiten.FilterLinear
	text.Draw(dst, s, Face(base, s), op)
}

func DrawTextRight(dst *ebiten.Image, s string, x, y, size float64, clr color.Color) {
	DrawTextLeft(dst, s, x-TextWidth(s, size), y, size, clr)
}

func DrawTextShadow(dst *ebiten.Image, s string, x, y, size float64, clr color.Color) {
	DrawTextLeft(dst, s, x+2, y+2, size, color.NRGBA{0, 0, 0, 0xa0})
	DrawTextLeft(dst, s, x, y, size, clr)
}

func Wrap(s string, size, width float64) []string {
	return wrapWith(s, func(line []rune) bool { return TextWidth(string(line), size) <= width })
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
					if line[i] == ' ' {
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
					if isWordRune(line[k-1]) && isWordRune(after) {
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
	r := []rune(s)
	for len(r) > 0 {
		r = r[:len(r)-1]
		if t := string(r) + "…"; TextWidth(t, size) <= width {
			return t
		}
	}
	return "…"
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
