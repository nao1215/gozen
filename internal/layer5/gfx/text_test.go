package gfx

import (
	"image/color"
	"math"
	"os"
	"slices"
	"strings"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
	"golang.org/x/image/font/gofont/gobold"
	"golang.org/x/image/font/gofont/goregular"
)

func useFont(t *testing.T) {
	t.Helper()
	t.Cleanup(Reset)
	if err := LoadFonts(goregular.TTF, nil); err != nil {
		t.Fatal(err)
	}
}

// runesFit is a width model where each rune is one unit wide.
func runesFit(n int) func([]rune) bool {
	return func(line []rune) bool { return len(line) <= n }
}

func TestWrapRespectsLineBreakRules(t *testing.T) {
	cases := []struct {
		name string
		in   string
		fit  int
		want []string
	}{
		{"plain", "あいうえおかきく", 5, []string{"あいうえお", "かきく"}},
		{"closing mark never starts a line", "あいうえお。", 5, []string{"あいうえ", "お。"}},
		{"small kana never starts a line", "あいうえおっか", 5, []string{"あいうえ", "おっか"}},
		{"opening bracket never ends a line", "あいうえ「おか", 5, []string{"あいうえ", "「おか"}},
		{"hard newline", "ab\ncd", 5, []string{"ab", "cd"}},
		{"trailing newline keeps an empty line", "a\n", 5, []string{"a", ""}},
		{"empty", "", 5, []string{""}},
		{"break at a late space", "abcdefghi jklmn", 12, []string{"abcdefghi", "jklmn"}},
		{"keep a late word whole", "abc defghijk", 8, []string{"abc ", "defghijk"}},
		{"split a long word", "ab cdefghijkl", 8, []string{"ab cdefg", "hijkl"}},
		{"exact fit", "abcde", 5, []string{"abcde"}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := wrapWith(c.in, runesFit(c.fit)); !slices.Equal(got, c.want) {
				t.Fatalf("wrap %q = %q, want %q", c.in, got, c.want)
			}
		})
	}
}

func strip(s string) string { return strings.NewReplacer(" ", "", "\n", "").Replace(s) }

func TestWrapNeverLosesOrReordersText(t *testing.T) {
	inputs := []string{
		"吾輩は猫である。名前はまだ無い。どこで生れたかとんと見当がつかぬ。",
		"「あのね」と彼女は言った。「それは違う！」",
		"The quick brown fox jumps over the lazy dog, again and again.",
		"混在 mixed テキスト with spaces、句読点。そして…続く",
		"ーっっ。。。」」」」」」",
		"a\n\nb\n",
	}
	for _, in := range inputs {
		for width := 1; width <= 12; width++ {
			got := wrapWith(in, runesFit(width))
			// A break at a space consumes the space.
			if strip(strings.Join(got, "")) != strip(in) {
				t.Fatalf("width %d lost text: %q -> %q", width, in, got)
			}
			// A line only exceeds the width when a single rune had nowhere else to go.
			for _, l := range got {
				if len([]rune(l)) > width+8 {
					t.Fatalf("width %d: absurdly long line %q", width, l)
				}
			}
		}
	}
}

func TestSentencesSplitAfterTerminators(t *testing.T) {
	got := sentences("はい。いいえ！\n\nそれは？「うん」ね")
	want := []string{"はい。", "いいえ！\n\n", "それは？", "「うん」", "ね"}
	if !slices.Equal(got, want) {
		t.Fatalf("sentences %q, want %q", got, want)
	}
	// A terminator followed by a closing mark stays with it.
	if got := sentences("ええ！？そう。」"); !slices.Equal(got, []string{"ええ！？", "そう。」"}) {
		t.Fatalf("closing marks split off: %q", got)
	}
	if got := sentences(""); len(got) != 0 {
		t.Fatalf("empty gives %q", got)
	}
}

// widthWrap wraps with rune width = size, so a smaller size fits more runes in a line.
func widthWrap(width float64) func(string, float64) []string {
	return func(s string, sz float64) []string {
		return wrapWith(s, func(line []rune) bool { return float64(len(line))*sz <= width })
	}
}

func TestPaginateShrinksBeforeSplitting(t *testing.T) {
	text := strings.Repeat("あ", 22)
	pages, sz := paginateWith(text, 10, 2, widthWrap(100))
	if len(pages) != 1 || len(pages[0]) != 2 {
		t.Fatalf("pages %q", pages)
	}
	if want := 10 * 0.94 * 0.94; math.Abs(sz-want) > 1e-9 {
		t.Fatalf("size %v, want %v (two shrink steps)", sz, want)
	}
	if pages, sz := paginateWith("short", 10, 2, widthWrap(100)); len(pages) != 1 || sz != 10 {
		t.Fatalf("short text shrunk: %v %v", pages, sz)
	}
}

func TestPaginateSplitsAtSentencesAndKeepsEverything(t *testing.T) {
	sentence := "これは一つの文です。"
	text := strings.Repeat(sentence, 9)
	for lines := 1; lines <= 4; lines++ {
		pages, sz := paginateWith(text, 12, lines, widthWrap(120))
		if len(pages) < 2 {
			t.Fatalf("lines %d: %d page", lines, len(pages))
		}
		if sz > 12 || sz < 12*PageShrink-1e-9 {
			t.Fatalf("lines %d: size %v out of [%v, 12]", lines, sz, 12*PageShrink)
		}
		var all strings.Builder
		for _, p := range pages {
			if len(p) > lines {
				t.Fatalf("lines %d: page of %d lines", lines, len(p))
			}
			for _, l := range p {
				all.WriteString(l)
			}
		}
		if all.String() != text {
			t.Fatalf("lines %d: text changed:\n%s", lines, all.String())
		}
	}
}

func TestPaginateFallsBackToPlainChunksOfLines(t *testing.T) {
	// One unbreakable run longer than the page can hold cannot be fitted by splitting at
	// sentences or shrinking, so the lines are simply cut into pages of the given height.
	wrap := func(s string, _ float64) []string {
		out := []string{}
		for s != "" {
			n := min(3, len(s))
			out = append(out, s[:n])
			s = s[n:]
		}
		return out
	}
	pages, sz := paginateWith("abcdefghij", 10, 2, wrap)
	if sz != 10 {
		t.Fatalf("size %v", sz)
	}
	if len(pages) < 1 {
		t.Fatal("no pages")
	}
	var joined string
	for _, p := range pages {
		if len(p) > 2 {
			t.Fatalf("page %q taller than 2 lines", p)
		}
		joined += strings.Join(p, "")
	}
	if joined != "abcdefghij" {
		t.Fatalf("joined %q", joined)
	}
}

func TestBaseSizeSnapsUpToTheTable(t *testing.T) {
	cases := map[float64]float64{-1: 12, 0: 12, 12: 12, 12.01: 18, 30: 30, 31: 36, 300: 300, 301: 300, 1e9: 300}
	for in, want := range cases {
		if got := baseSize(in); got != want {
			t.Errorf("baseSize(%v) = %v, want %v", in, got, want)
		}
	}
}

func TestFaceChoosesPopFontOnlyForASCII(t *testing.T) {
	t.Cleanup(Reset)
	if err := LoadFonts(goregular.TTF, gobold.TTF); err != nil {
		t.Fatal(err)
	}
	a := Face(24, "Hello")
	if a.Source != popSource {
		t.Fatal("ASCII text must use the pop font")
	}
	if b := Face(24, "Hello again"); b != a {
		t.Fatal("faces of one size and kind are cached")
	}
	if j := Face(24, "こんにちは"); j.Source != regularSource || j == a {
		t.Fatal("non-ASCII text must use the regular font")
	}
	if Face(24, "mixed あ").Source != regularSource {
		t.Fatal("one non-ASCII rune switches to the regular font")
	}
	if Face(36, "x").Size != 36 {
		t.Fatal("size")
	}
	if !isASCII("~ plain 123") || isASCII("é") || isASCII("\x7f") {
		t.Fatal("isASCII boundary is 0x7e")
	}
}

// TestPopDrawsDecorationsInThePopFont: inside Pop, text is measured in the pop font where it
// has the glyphs (Japanese falls back to the regular font glyph by glyph), and as before
// outside it or without a pop font.
func TestPopDrawsDecorationsInThePopFont(t *testing.T) {
	t.Cleanup(Reset)
	if err := LoadFonts(goregular.TTF, nil); err != nil {
		t.Fatal(err)
	}
	plain := TextWidth("Ann あ", 24)
	var noPop float64
	Pop(func() { noPop = TextWidth("Ann あ", 24) })
	if noPop != plain {
		t.Fatalf("without a pop font Pop measures %v, not %v", noPop, plain)
	}
	if err := LoadFonts(goregular.TTF, gobold.TTF); err != nil {
		t.Fatal(err)
	}
	regular := TextWidth("Ann あ", 24)
	var popped float64
	Pop(func() { popped = TextWidth("Ann あ", 24) })
	if popped == regular || TextWidth("Ann あ", 24) != regular {
		t.Fatalf("Pop measures %v, the regular font %v, and after Pop %v", popped, regular, TextWidth("Ann あ", 24))
	}
	// a glyph the pop font lacks is the regular font's: a Japanese one of M+ under Go Bold
	mplus, err := os.ReadFile("../../../examples/novel/game/assets/fonts/mplus-1p-regular.ttf")
	if err != nil {
		t.Fatal(err)
	}
	if err := LoadFonts(mplus, gobold.TTF); err != nil {
		t.Fatal(err)
	}
	kana := TextWidth("あい", 24)
	var poppedKana float64
	Pop(func() { poppedKana = TextWidth("あい", 24) })
	if math.Abs(poppedKana-kana) > 0.01 {
		t.Fatalf("Pop measures a glyph the pop font lacks %v wide, not the regular font's %v", poppedKana, kana)
	}
}

func TestLoadFontsFailedPopKeepsTheOldFonts(t *testing.T) {
	useFont(t)
	old := regularSource
	if err := LoadFonts(gobold.TTF, []byte("broken pop")); err == nil {
		t.Fatal("bad pop font accepted")
	}
	if regularSource != old {
		t.Fatal("a failed load replaced the regular font")
	}
}

func TestLoadTitleFont(t *testing.T) {
	useFont(t)
	if err := LoadTitleFont(gobold.TTF, goregular.TTF); err != nil {
		t.Fatal(err)
	}
	if titleSource == nil || subSource == nil {
		t.Fatal("fonts not set")
	}
	if err := LoadTitleFont(nil, nil); err != nil || titleSource != nil || subSource != nil {
		t.Fatal("nil fonts must clear the title typefaces")
	}
	if err := LoadTitleFont([]byte("bad"), nil); err == nil {
		t.Fatal("bad title font accepted")
	}
	if err := LoadTitleFont(gobold.TTF, []byte("bad")); err == nil {
		t.Fatal("bad subtitle font accepted")
	}
}

func TestDrawTitleTextWithoutAndWithTitleFonts(t *testing.T) {
	useFont(t)
	dst := ebiten.NewImage(200, 80)
	DrawTitleText(dst, "Title", 100, 10, 30, 0.1, color.White, color.Black, 1, false)
	DrawTitleText(dst, "", 100, 10, 30, 0, color.White, color.Black, 1, true)
	if err := LoadTitleFont(gobold.TTF, nil); err != nil {
		t.Fatal(err)
	}
	DrawTitleText(dst, "Sub", 100, 40, 20, 0, color.White, color.Black, 0.5, true)
	if dst.Bounds().Dx() != 200 {
		t.Fatal("destination changed")
	}
}

func TestTextWidthMeasuresWithTheLoadedFont(t *testing.T) {
	useFont(t)
	if TextWidth("hello", 0) != 0 || TextWidth("hello", -5) != 0 {
		t.Fatal("non-positive size has no width")
	}
	w20 := TextWidth("hello", 20)
	if w20 <= 0 || math.Abs(TextWidth("hello", 40)-2*w20) > 0.01*w20 {
		t.Fatalf("width not linear in size: %v %v", w20, TextWidth("hello", 40))
	}
	if TextWidth("hello hello", 20) <= w20 {
		t.Fatal("longer text is not wider")
	}
	SetScale(2)
	if got := TextWidth("hello", 20); math.Abs(got-w20) > 0.01*w20 {
		t.Fatalf("the layout width changed with the render scale: %v vs %v", got, w20)
	}
	if LineHeight(20) != 28 {
		t.Fatalf("line height %v", LineHeight(20))
	}
}

func TestEllipsis(t *testing.T) {
	useFont(t)
	s := "hello world, hello world"
	full := TextWidth(s, 20)
	if got := Ellipsis(s, 20, full); got != s {
		t.Fatalf("text that fits changed to %q", got)
	}
	limit := full / 2
	got := Ellipsis(s, 20, limit)
	if !strings.HasSuffix(got, "…") || TextWidth(got, 20) > limit {
		t.Fatalf("%q does not fit %v (width %v)", got, limit, TextWidth(got, 20))
	}
	body := strings.TrimSuffix(got, "…")
	longer := s[:len(body)+1] + "…"
	if TextWidth(longer, 20) <= limit {
		t.Fatalf("%q was cut shorter than needed", got)
	}
	if got := Ellipsis(s, 20, 0); got != "…" {
		t.Fatalf("no room gives %q", got)
	}
}

func TestFitLinesShrinksThenCuts(t *testing.T) {
	useFont(t)
	text := "hello"
	w := TextWidth(text, 20)
	// Slightly too narrow for one line: one shrink step (x0.94) makes it fit.
	ls, sz, cut := FitLinesCut(text, 20, w*0.95, 1)
	if cut || len(ls) != 1 || ls[0] != text || math.Abs(sz-18.8) > 1e-9 {
		t.Fatalf("shrink: %q %v %v", ls, sz, cut)
	}
	ls, sz = FitLines(text, 20, w*10, 3)
	if len(ls) != 1 || sz != 20 {
		t.Fatalf("roomy: %q %v", ls, sz)
	}
	// Far too narrow: shrinking stops at 75 percent and the last line is cut with an ellipsis.
	long := strings.Repeat("hello ", 30)
	ls, sz, cut = FitLinesCut(long, 20, w*2, 2)
	if !cut || len(ls) != 2 || !strings.HasSuffix(ls[1], "…") {
		t.Fatalf("cut: %q %v %v", ls, sz, cut)
	}
	if sz < 20*0.75*0.94-1e-9 || TextWidth(ls[1], sz) > w*2 {
		t.Fatalf("cut size %v / last line too wide", sz)
	}
	// Continuation lines never start with a space.
	ls, _ = FitLines("alpha beta gamma delta epsilon zeta", 20, TextWidth("alpha beta gamma", 20), 6)
	for i, l := range ls[1:] {
		if strings.HasPrefix(l, " ") {
			t.Fatalf("line %d starts with a space: %q", i+1, l)
		}
	}
	// A lines count below one is treated as one.
	if ls, _, _ := FitLinesCut("alpha beta gamma", 20, 1, 0); len(ls) != 1 {
		t.Fatalf("lines 0 gave %d lines", len(ls))
	}
}

func TestPaginateWithRealFont(t *testing.T) {
	useFont(t)
	text := strings.Repeat("The road goes on and on. ", 6)
	width := TextWidth("The road goes on and on.", 20)
	pages, sz := Paginate(text, 20, width, 2)
	if len(pages) < 2 || sz > 20 || sz < 20*PageShrink-1e-9 {
		t.Fatalf("pages %d size %v", len(pages), sz)
	}
	for _, p := range pages {
		if len(p) > 2 {
			t.Fatalf("page of %d lines", len(p))
		}
	}
}

func TestDrawTextCachesFacesPerBaseSize(t *testing.T) {
	useFont(t)
	dst := ebiten.NewImage(200, 100)
	DrawText(dst, "hello", 10, 10, 20, color.White)
	DrawTextLeft(dst, "hello", 10, 10, 20, color.White)
	DrawTextRight(dst, "hello", 190, 40, 20, color.White)
	if len(faceCache) != 1 {
		t.Fatalf("%d faces for one base size", len(faceCache))
	}
	DrawText(dst, "hello", 10, 10, 40, color.White)
	if len(faceCache) != 2 {
		t.Fatalf("a new base size must add a face, have %d", len(faceCache))
	}
	DrawText(dst, "hello", 10, 10, 0, color.White)
	DrawTextLeft(dst, "", 10, 10, 20, color.White)
	DrawTextLeft(dst, "x", 10, 10, -3, color.White)
	if len(faceCache) != 2 {
		t.Fatal("invalid draws touched the face cache")
	}
}

func TestOutlinedTextIsCached(t *testing.T) {
	useFont(t)
	dst := ebiten.NewImage(200, 100)
	DrawTextOutline(dst, "hello", 100, 10, 20, color.Black)
	if len(textCache) != 1 {
		t.Fatalf("%d cached texts", len(textCache))
	}
	DrawTextOutline(dst, "hello", 50, 30, 20, color.Black)
	DrawTextOutlineColor(dst, "hello", 50, 30, 20, color.Black, color.White, 0.5)
	if len(textCache) != 1 {
		t.Fatal("same text and colors must share one image")
	}
	DrawTextOutlineColor(dst, "hello", 50, 30, 20, color.Black, color.NRGBA{255, 0, 0, 255}, 1)
	if len(textCache) != 2 {
		t.Fatal("another outline color is another image")
	}
	DrawTextOutlineColor(dst, "", 1, 1, 20, color.Black, color.White, 1)
	DrawTextOutlineColor(dst, "x", 1, 1, 0, color.Black, color.White, 1)
	DrawTextOutlineColor(dst, "x", 1, 1, 20, color.Black, color.White, 0)
	if len(textCache) != 2 {
		t.Fatal("nothing-to-draw calls must not build images")
	}
	for i := range cacheLimit + 5 {
		outlinedText(strings.Repeat("a", 1+i%7)+string(rune('A'+i%26))+strings.Repeat("b", i/26), 12, color.NRGBA{A: 255}, color.NRGBA{})
	}
	if len(textCache) > cacheLimit+1 {
		t.Fatalf("text cache grew to %d", len(textCache))
	}
}

func TestShapeCacheAndLimits(t *testing.T) {
	t.Cleanup(Reset)
	dst := ebiten.NewImage(200, 100)
	FillRoundRect(dst, 5, 5, 50, 20, 4, color.White)
	FillRoundRect(dst, 60, 40, 50, 20, 4, color.White)
	if len(shapeCache) != 1 {
		t.Fatalf("same shape drawn twice made %d images", len(shapeCache))
	}
	StrokeRoundRect(dst, 5, 5, 50, 20, 4, 2, color.White)
	if len(shapeCache) != 2 {
		t.Fatalf("a stroke is another image, have %d", len(shapeCache))
	}
	FillRoundRect(dst, 0, 0, 0, 10, 2, color.White)
	StrokeRoundRect(dst, 0, 0, 10, -1, 2, 1, color.White)
	if len(shapeCache) != 2 {
		t.Fatal("empty shapes must draw nothing")
	}
	SetScale(2)
	FillRoundRect(dst, 5, 5, 50, 20, 4, color.White)
	if len(shapeCache) != 3 {
		t.Fatal("scale changes the rendered shape size, so the key")
	}
	for i := range cacheLimit + 5 {
		FillRoundRect(dst, 0, 0, float32(10+i), 10, 2, color.White)
	}
	if len(shapeCache) > cacheLimit+1 {
		t.Fatalf("shape cache grew to %d", len(shapeCache))
	}
}

func TestRoundedMaskIsCachedPerSizeAndRadius(t *testing.T) {
	t.Cleanup(Reset)
	img := ebiten.NewImage(40, 30)
	if RoundedMask(img, 6) != img {
		t.Fatal("RoundedMask must return the image it masked")
	}
	RoundedMask(ebiten.NewImage(40, 30), 6)
	if len(maskCache) != 1 {
		t.Fatalf("same size and radius made %d masks", len(maskCache))
	}
	RoundedMask(ebiten.NewImage(40, 30), 8)
	RoundedMask(ebiten.NewImage(41, 30), 6)
	if len(maskCache) != 3 {
		t.Fatalf("masks %d", len(maskCache))
	}
}

func TestResetClearsEverything(t *testing.T) {
	if err := LoadFonts(goregular.TTF, gobold.TTF); err != nil {
		t.Fatal(err)
	}
	dst := ebiten.NewImage(100, 100)
	SetScale(3)
	FillRoundRect(dst, 0, 0, 10, 10, 2, color.White)
	DrawTextOutline(dst, "x", 0, 0, 12, color.White)
	RoundedMask(ebiten.NewImage(8, 8), 2)
	Reset()
	if len(shapeCache)+len(textCache)+len(maskCache)+len(faceCache) != 0 {
		t.Fatal("caches survive Reset")
	}
	if regularSource != nil || popSource != nil || Scale() != 1 {
		t.Fatal("fonts or scale survive Reset")
	}
}

func TestSetScaleRejectsNonPositive(t *testing.T) {
	t.Cleanup(Reset)
	SetScale(2.5)
	if Scale() != 2.5 {
		t.Fatal("scale not set")
	}
	for _, k := range []float64{0, -1} {
		SetScale(k)
		if Scale() != 1 {
			t.Fatalf("SetScale(%v) gives %v, want 1", k, Scale())
		}
	}
}

func TestScaleVertices(t *testing.T) {
	t.Cleanup(Reset)
	vs := []ebiten.Vertex{{DstX: 2, DstY: 3, SrcX: 7}, {DstX: -1, DstY: 0.5}}
	ScaleVertices(vs)
	if vs[0].DstX != 2 || vs[1].DstY != 0.5 {
		t.Fatal("scale 1 must not move vertices")
	}
	SetScale(2)
	ScaleVertices(vs)
	if vs[0].DstX != 4 || vs[0].DstY != 6 || vs[1].DstX != -2 || vs[1].DstY != 1 || vs[0].SrcX != 7 {
		t.Fatalf("scaled vertices %+v", vs)
	}
}

func TestDrawImageDoesNotChangeTheCallersOptions(t *testing.T) {
	t.Cleanup(Reset)
	dst, img := ebiten.NewImage(20, 20), ebiten.NewImage(4, 4)
	op := &ebiten.DrawImageOptions{}
	op.GeoM.Translate(3, 4)
	SetScale(2)
	DrawImage(dst, img, op)
	if got := op.GeoM.Element(0, 2); got != 3 {
		t.Fatalf("caller's geometry was scaled in place: %v", got)
	}
	DrawImage(dst, img, nil)
	SetScale(1)
	DrawImage(dst, img, nil)
	DrawImage(dst, img, op)
}

func TestShapesAtAScale(t *testing.T) {
	t.Cleanup(Reset)
	dst := ebiten.NewImage(100, 100)
	SetScale(2)
	FillRect(dst, 1, 1, 10, 10, color.White, true)
	StrokeLine(dst, 0, 0, 5, 5, 1, color.White, false)
	FillCircle(dst, 10, 10, 4, color.White, true)
	StrokeCircle(dst, 10, 10, 4, 1, color.White, true)
	if Scale() != 2 || BaseHeight != 720 {
		t.Fatal("drawing changed the scale")
	}
}
