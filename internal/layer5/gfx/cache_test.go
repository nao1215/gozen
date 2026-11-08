package gfx

import (
	"image/color"
	"strconv"
	"strings"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
	"golang.org/x/image/font/gofont/gobold"
	"golang.org/x/image/font/gofont/goregular"
)

func TestOutlinedTextIsCached(t *testing.T) {
	useFont(t)
	dst := ebiten.NewImage(200, 100)
	DrawTextOutline(dst, "hello", 100, 10, 20, color.Black)
	if len(active.textCache) != 1 {
		t.Fatalf("%d cached texts", len(active.textCache))
	}
	DrawTextOutline(dst, "hello", 50, 30, 20, color.Black)
	DrawTextOutlineColor(dst, "hello", 50, 30, 20, color.Black, color.White, 0.5)
	if len(active.textCache) != 1 {
		t.Fatal("same text and colors must share one image")
	}
	DrawTextOutlineColor(dst, "hello", 50, 30, 20, color.Black, color.NRGBA{255, 0, 0, 255}, 1)
	if len(active.textCache) != 2 {
		t.Fatal("another outline color is another image")
	}
	DrawTextOutlineColor(dst, "", 1, 1, 20, color.Black, color.White, 1)
	DrawTextOutlineColor(dst, "x", 1, 1, 0, color.Black, color.White, 1)
	DrawTextOutlineColor(dst, "x", 1, 1, 20, color.Black, color.White, 0)
	if len(active.textCache) != 2 {
		t.Fatal("nothing-to-draw calls must not build images")
	}
	for i := range cacheLimit + 5 {
		outlinedText(strings.Repeat("a", 1+i%7)+string(rune('A'+i%26))+strings.Repeat("b", i/26), 12, color.NRGBA{A: 255}, color.NRGBA{})
	}
	if len(active.textCache) > cacheLimit+1 {
		t.Fatalf("text cache grew to %d", len(active.textCache))
	}
}

func TestShapeCacheAndLimits(t *testing.T) {
	t.Cleanup(Reset)
	dst := ebiten.NewImage(200, 100)
	FillRoundRect(dst, 5, 5, 50, 20, 4, color.White)
	FillRoundRect(dst, 60, 40, 50, 20, 4, color.White)
	if len(active.shapeCache) != 1 {
		t.Fatalf("same shape drawn twice made %d images", len(active.shapeCache))
	}
	StrokeRoundRect(dst, 5, 5, 50, 20, 4, 2, color.White)
	if len(active.shapeCache) != 2 {
		t.Fatalf("a stroke is another image, have %d", len(active.shapeCache))
	}
	FillRoundRect(dst, 0, 0, 0, 10, 2, color.White)
	StrokeRoundRect(dst, 0, 0, 10, -1, 2, 1, color.White)
	if len(active.shapeCache) != 2 {
		t.Fatal("empty shapes must draw nothing")
	}
	SetScale(2)
	FillRoundRect(dst, 5, 5, 50, 20, 4, color.White)
	if len(active.shapeCache) != 3 {
		t.Fatal("scale changes the rendered shape size, so the key")
	}
	for i := range cacheLimit + 5 {
		FillRoundRect(dst, 0, 0, float32(10+i), 10, 2, color.White)
	}
	if len(active.shapeCache) > cacheLimit+1 {
		t.Fatalf("shape cache grew to %d", len(active.shapeCache))
	}
}

// Cache geometry must use the same font selection as measuring and drawing.
// This checks CPU-side cache identity and metrics, not rendered pixels.
func TestOutlinedTextCacheSeparatesPopFont(t *testing.T) {
	t.Cleanup(Reset)
	if err := LoadFonts(goregular.TTF, gobold.TTF); err != nil {
		t.Fatal(err)
	}
	const text = "iii界"
	fill, edge := color.NRGBA{A: 255}, color.NRGBA{R: 255, G: 255, B: 255, A: 255}
	normal := outlinedText(text, 20, fill, edge)
	var decorated textImage
	var normalWidth, popWidth float64
	normalWidth = textWidthRaw(text, 20)
	Pop(func() { popWidth = textWidthRaw(text, 20); decorated = outlinedText(text, 20, fill, edge) })
	if normalWidth == popWidth {
		t.Fatal("fixture does not distinguish the typefaces")
	}
	if decorated.img == normal.img || decorated.cx == normal.cx {
		t.Fatal("decoration reused an image measured with the regular font")
	}
	if again := outlinedText(text, 20, fill, edge); again.img != normal.img {
		t.Fatal("normal text lost its cached image")
	}
	Pop(func() {
		if again := outlinedText(text, 20, fill, edge); again.img != decorated.img {
			t.Fatal("decorative text lost its cached image")
		}
	})
	ascii := outlinedText("plain", 20, fill, edge)
	Pop(func() {
		if again := outlinedText("plain", 20, fill, edge); again.img != ascii.img {
			t.Fatal("ASCII should share the pop face in both contexts")
		}
	})
}

// Keep the cache alive for inuse_space profiling of short labels sliced from
// larger dialogue sources. Run with -benchtime=512x to stay below cacheLimit.
func BenchmarkOutlinedTextRetainedSource(b *testing.B) {
	if err := LoadFonts(goregular.TTF, nil); err != nil {
		b.Fatal(err)
	}
	for i := range b.N {
		source := strings.Repeat("x", 96<<10) + strconv.Itoa(i)
		outlinedText(source[96<<10:], 12, color.NRGBA{A: 255}, color.NRGBA{})
	}
}

func TestGlyphPageReuseAndRollover(t *testing.T) {
	t.Cleanup(Reset)
	Reset()
	a, b := ebiten.NewImage(20, 30), ebiten.NewImage(40, 50)
	first := stableGlyph(a)
	page := active.glyphPage
	if stableGlyph(a) != first {
		t.Fatal("repeated glyph was copied again")
	}
	second := stableGlyph(b)
	if active.glyphPage != page || first.Bounds().Overlaps(second.Bounds()) {
		t.Fatal("glyphs must share a page without overlapping")
	}
	if first.Bounds().Dx() != 20 || first.Bounds().Dy() != 30 {
		t.Fatal("glyph dimensions changed")
	}
	// A full page rolls over without retaining old source images.
	large := ebiten.NewImage(1024, 1024)
	stableGlyph(large)
	if active.glyphPage == page || len(active.glyphCache) != 1 || active.glyphCache[a] != nil {
		t.Fatal("page rollover retained old glyphs")
	}
	Reset()
	if active.glyphPage != nil || len(active.glyphCache) != 0 {
		t.Fatal("reset retained glyph resources")
	}
}

func TestContextIsolation(t *testing.T) {
	parent, child := NewContext(), NewContext()
	restore := parent.Activate()
	defer restore()
	defer parent.Close()
	SetScale(2)
	parent.glyphX = 17
	childRestore := child.Activate()
	SetScale(3)
	child.glyphX = 4
	childRestore()
	if Scale() != 2 || active.glyphX != 17 {
		t.Fatal("child changed parent's scale or glyph position")
	}
	child.Close()
	if Scale() != 2 || active.glyphX != 17 {
		t.Fatal("closing child reset parent")
	}
}
