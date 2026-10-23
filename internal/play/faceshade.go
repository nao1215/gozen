package play

import (
	"image"
	"image/draw"
	"log"
	"strings"

	"github.com/nao1215/gozen/internal/layer1/picturelimit"

	"github.com/hajimehoshi/ebiten/v2"

	"github.com/nao1215/gozen/internal/layer5/picdecode"
)

func (p *pictures) shadedFace(path string, r image.Rectangle) *ebiten.Image {
	i := strings.LastIndex(path, "/")
	if i < 0 || r.Empty() {
		return nil
	}
	p.mu.Lock()
	sh, ok := p.shades[path[:i]]
	img := p.shaded[path]
	gen := p.gen
	p.mu.Unlock()
	if !ok {
		return nil
	}
	if img != nil && img.Bounds().Size() == r.Size() {
		return img
	}
	b, err := picturelimit.Read(p.fsys, "assets/"+path)
	if err != nil {
		log.Printf("picture %s: %v", path, err)
		return nil
	}
	src, err := picdecode.Decode(b)
	if err != nil {
		log.Printf("picture %s: %v", path, err)
		return nil
	}
	img = ebiten.NewImageFromImage(shadeFace(src, r, sh[0], sh[1]))
	p.mu.Lock()
	if gen == p.gen {
		p.shaded[path] = img
	}
	p.mu.Unlock()
	return img
}

func shadeFace(src image.Image, r image.Rectangle, top, bottom float64) *image.NRGBA {
	out := image.NewNRGBA(image.Rect(0, 0, r.Dx(), r.Dy()))
	draw.Draw(out, out.Bounds(), src, r.Min, draw.Src)
	h := r.Dy()
	for y := range h {
		t := 0.0
		if h > 1 {
			t = float64(y) / float64(h-1)
		}
		k := 1 - (top + (bottom-top)*t)
		k = min(1, max(0, k))
		row := out.Pix[y*out.Stride : y*out.Stride+r.Dx()*4]
		for x := 0; x < len(row); x += 4 {
			row[x] = uint8(float64(row[x])*k + 0.5)
			row[x+1] = uint8(float64(row[x+1])*k + 0.5)
			row[x+2] = uint8(float64(row[x+2])*k + 0.5)
		}
	}
	return out
}
