package docs

import (
	"bytes"
	goimage "image"
	"image/color"
	"image/draw"
	"image/gif"
	"os"
	"path/filepath"
	"testing"
)

// slimTolerance is how far (in RGB distance) a pixel may be from the one a
// viewer already shows and still be left alone. Palette steps and ordered-dither
// flicker are far below it; text and sprites are far above it.
const slimTolerance = 16

// slimGIF rewrites a GIF so that each frame holds only the pixels that changed
// since the frame before, as a transparent-keyed patch. A pixel that differs from
// what is already shown by no more than tolerance keeps the shown color, so the
// compressor sees long runs. The result plays the same frames with the same
// delays; it is the pure Go counterpart of gifsicle -O3 --lossy.
func slimGIF(file string, tolerance int) error {
	b, err := os.ReadFile(file) //nolint:gosec // A GIF the test just wrote.
	if err != nil {
		return err
	}
	g, err := gif.DecodeAll(bytes.NewReader(b))
	if err != nil {
		return err
	}
	if len(g.Image) < 2 {
		return nil
	}
	pal := g.Image[0].Palette
	if len(pal) > 256 {
		return nil
	}
	// The encoder writes the first fully transparent entry of the palette as the
	// transparent index, so that entry is the one to use (ffmpeg reserves one).
	newPal := append(color.Palette{}, pal...)
	trans := -1
	index := map[color.RGBA]uint8{}
	for i, c := range pal {
		r, g, b, a := c.RGBA()
		if a == 0 {
			if trans < 0 {
				trans = i
			}
			continue
		}
		k := color.RGBA{uint8(r >> 8), uint8(g >> 8), uint8(b >> 8), uint8(a >> 8)}
		if _, ok := index[k]; !ok {
			index[k] = uint8(i)
		}
	}
	if trans < 0 {
		if len(pal) > 255 {
			return nil // no room for a transparent entry
		}
		trans = len(newPal)
		newPal = append(newPal, color.RGBA{})
	}

	frames := composeGIF(g)
	bounds := frames[0].Bounds()
	shown := goimage.NewRGBA(bounds)
	out := &gif.GIF{LoopCount: g.LoopCount, Config: g.Config}
	limit := tolerance * tolerance
	for n, f := range frames {
		full := f.(*goimage.RGBA)
		if n == 0 {
			p := goimage.NewPaletted(bounds, newPal)
			for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
				for x := bounds.Min.X; x < bounds.Max.X; x++ {
					i, ok := index[full.RGBAAt(x, y)]
					if !ok {
						return nil // a color of a later palette: the GIF stays as it is
					}
					p.SetColorIndex(x, y, i)
				}
			}
			draw.Draw(shown, bounds, full, bounds.Min, draw.Src)
			out.Image = append(out.Image, p)
			out.Delay = append(out.Delay, g.Delay[n])
			out.Disposal = append(out.Disposal, gif.DisposalNone)
			continue
		}
		box := goimage.Rectangle{}
		for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
			for x := bounds.Min.X; x < bounds.Max.X; x++ {
				if rgbDist(full.RGBAAt(x, y), shown.RGBAAt(x, y)) > limit {
					box = box.Union(goimage.Rect(x, y, x+1, y+1))
				}
			}
		}
		if box.Empty() {
			box = goimage.Rect(0, 0, 1, 1)
		}
		p := goimage.NewPaletted(box, newPal)
		for y := box.Min.Y; y < box.Max.Y; y++ {
			for x := box.Min.X; x < box.Max.X; x++ {
				c := full.RGBAAt(x, y)
				if rgbDist(c, shown.RGBAAt(x, y)) > limit {
					i, ok := index[c]
					if !ok {
						return nil
					}
					p.SetColorIndex(x, y, i)
					shown.SetRGBA(x, y, c)
				} else {
					p.SetColorIndex(x, y, uint8(trans))
				}
			}
		}
		out.Image = append(out.Image, p)
		out.Delay = append(out.Delay, g.Delay[n])
		out.Disposal = append(out.Disposal, gif.DisposalNone)
	}
	for i, p := range out.Image {
		if i > 0 {
			p.Palette = newPal
		}
	}
	// The transparent entry is the same for every frame; the encoder writes it
	// through the GIF extension when the first frame carries it.
	var buf bytes.Buffer
	if err := gif.EncodeAll(&buf, out); err != nil {
		return err
	}
	if buf.Len() >= len(b) {
		return nil
	}
	return os.WriteFile(file, buf.Bytes(), 0o600) //nolint:gosec // A GIF in the test's work directory.
}

func rgbDist(a, b color.RGBA) int {
	dr, dg, db := int(a.R)-int(b.R), int(a.G)-int(b.G), int(a.B)-int(b.B)
	return dr*dr + dg*dg + db*db
}

// Slimming keeps the frames, the delays and the look, and makes a moving
// picture smaller.
func TestSlimGIF(t *testing.T) {
	pal := color.Palette{color.RGBA{}, color.RGBA{20, 30, 40, 255}, color.RGBA{22, 31, 41, 255}, color.RGBA{200, 10, 10, 255}}
	g := &gif.GIF{Config: goimage.Config{Width: 64, Height: 64, ColorModel: pal}}
	for n := range 6 {
		p := goimage.NewPaletted(goimage.Rect(0, 0, 64, 64), pal)
		for i := range p.Pix {
			p.Pix[i] = uint8(1 + (i+n)%2) // a flicker of two near colors
		}
		for y := 10; y < 20; y++ {
			for x := 10 * n; x < 10*n+10; x++ {
				p.SetColorIndex(x, y, 3) // a block that moves
			}
		}
		g.Image = append(g.Image, p)
		g.Delay = append(g.Delay, 8)
		g.Disposal = append(g.Disposal, gif.DisposalNone)
	}
	file := filepath.Join(t.TempDir(), "a.gif")
	var buf bytes.Buffer
	if err := gif.EncodeAll(&buf, g); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(file, buf.Bytes(), 0o600); err != nil {
		t.Fatal(err)
	}
	before, err := decodeFrames(t.Context(), file)
	if err != nil {
		t.Fatal(err)
	}
	if err := slimGIF(file, slimTolerance); err != nil {
		t.Fatal(err)
	}
	st, err := os.Stat(file)
	if err != nil {
		t.Fatal(err)
	}
	if st.Size() >= int64(buf.Len()) {
		t.Errorf("not smaller: %d bytes, was %d", st.Size(), buf.Len())
	}
	after, err := decodeFrames(t.Context(), file)
	if err != nil {
		t.Fatal(err)
	}
	if len(after) != len(before) {
		t.Fatalf("%d frames, want %d", len(after), len(before))
	}
	for n := range before {
		a, b := before[n].(*goimage.RGBA), after[n].(*goimage.RGBA)
		for y := range 64 {
			for x := range 64 {
				if rgbDist(a.RGBAAt(x, y), b.RGBAAt(x, y)) > slimTolerance*slimTolerance {
					t.Fatalf("frame %d at %d,%d: %v became %v", n, x, y, a.RGBAAt(x, y), b.RGBAAt(x, y))
				}
			}
		}
	}
}
