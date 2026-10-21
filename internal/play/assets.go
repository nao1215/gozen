// Package play is the screen of a running game: input, drawing, and sound on
// top of the rules the headless driver also uses.
package play

import (
	"image"
	"image/color"
	_ "image/jpeg" // decoders for the pictures image.Decode reads
	_ "image/png"
	"io/fs"
	"log"
	"math"
	"strings"
	"sync"

	"github.com/hajimehoshi/ebiten/v2"
	_ "golang.org/x/image/webp" // a browser build may ship its pictures as WebP

	data "github.com/nao1215/gozen/internal/layer2/gamedata"
	"github.com/nao1215/gozen/internal/layer5/doll"
	"github.com/nao1215/gozen/internal/layer5/gfx"
	"github.com/nao1215/gozen/internal/layer5/picdecode"
)

type pictures struct {
	fsys         fs.FS
	mu           sync.Mutex
	workers      sync.WaitGroup
	decoded      map[string]image.Image
	decodedBytes int
	imgs         map[string]*picEntry
	loading      map[string]*picLoad
	bytes        int
	budget       int
	clock        int
	heads        map[string]image.Rectangle
	boxes        map[string][]float64
	blurs        map[string]*ebiten.Image
	shades       map[string][2]float64
	shaded       map[string]*ebiten.Image
	// found are the face boxes gozen art faces wrote (assets/faces.json), read once.
	found     map[string]data.FaceBox
	foundRead bool

	seen func(path string, ok bool)

	gen int

	// after, when set, holds the preloading back until it is closed (a browser's build: the
	// first frame first, on its one thread).
	after <-chan struct{}
}

type picEntry struct {
	img   *ebiten.Image
	bytes int
	used  int
}

type picLoad struct {
	done chan struct{}
	img  *ebiten.Image
}

const picBudget = 256 << 20

func newPictures(fsys fs.FS) *pictures {
	return &pictures{
		fsys: fsys, imgs: map[string]*picEntry{}, loading: map[string]*picLoad{}, decoded: map[string]image.Image{}, budget: picBudget,
		heads: map[string]image.Rectangle{}, blurs: map[string]*ebiten.Image{}, boxes: map[string][]float64{},
		shades: map[string][2]float64{}, shaded: map[string]*ebiten.Image{},
	}
}

var missing = func() *ebiten.Image {
	img := ebiten.NewImage(64, 64)
	img.Fill(color.NRGBA{0xff, 0x00, 0xff, 0xff})
	return img
}()

func (p *pictures) get(path string) *ebiten.Image {
	if path == "" {
		return nil
	}
	img := p.fetch(path)
	if p.seen != nil {
		p.seen(path, img != missing)
	}
	return img
}

func (p *pictures) fetch(path string) *ebiten.Image {
	p.mu.Lock()
	if e, ok := p.imgs[path]; ok {
		p.clock++
		e.used = p.clock
		p.mu.Unlock()
		return e.img
	}
	if l := p.loading[path]; l != nil {
		p.mu.Unlock()
		<-l.done
		return l.img
	}
	l := &picLoad{done: make(chan struct{})}
	p.loading[path] = l
	gen := p.gen
	p.mu.Unlock()

	img, head := p.load(path)
	p.mu.Lock()
	if gen != p.gen {
		if p.loading[path] == l {
			delete(p.loading, path)
		}
		p.mu.Unlock()
		l.img = img
		close(l.done)
		return img
	}
	delete(p.loading, path)
	p.clock++
	e := &picEntry{img: img, used: p.clock}
	if img != missing {
		b := img.Bounds()
		e.bytes = b.Dx() * b.Dy() * 4
	}
	p.imgs[path] = e
	p.heads[path] = head
	p.bytes += e.bytes
	if p.bytes+p.decodedBytes > p.budget {
		clear(p.decoded)
		p.decodedBytes = 0
	}
	p.trim()
	p.mu.Unlock()
	l.img = img
	close(l.done)
	return img
}

func (p *pictures) reset() {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.gen++
	p.imgs = map[string]*picEntry{}
	p.loading = map[string]*picLoad{}
	p.heads = map[string]image.Rectangle{}
	p.blurs = map[string]*ebiten.Image{}
	p.shaded = map[string]*ebiten.Image{}
	p.found, p.foundRead = nil, false
	p.bytes = 0
	clear(p.decoded)
	p.decodedBytes = 0
}

func (p *pictures) trim() {
	for p.bytes > p.budget && len(p.imgs) > 1 {
		oldest, at := "", 0
		for path, e := range p.imgs {
			if e.bytes > 0 && (oldest == "" || e.used < at) {
				oldest, at = path, e.used
			}
		}
		if oldest == "" {
			return
		}
		p.bytes -= p.imgs[oldest].bytes
		delete(p.imgs, oldest)
		delete(p.heads, oldest)
		delete(p.blurs, oldest)
		delete(p.shaded, oldest)
	}
}

func (p *pictures) load(path string) (*ebiten.Image, image.Rectangle) {
	p.mu.Lock()
	src := p.decoded[path]
	if src != nil {
		delete(p.decoded, path)
		p.decodedBytes -= src.Bounds().Dx() * src.Bounds().Dy() * 4
	}
	p.mu.Unlock()
	if src == nil {
		src = p.decode(path)
	}
	if src == nil {
		return missing, image.Rectangle{}
	}
	return ebiten.NewImageFromImage(src), findHead(src)
}

// decode returns only an image in CPU memory. It can finish reading safely even after the screen loop ends.
func (p *pictures) decode(path string) image.Image {
	b, err := fs.ReadFile(p.fsys, "assets/"+path)
	if err != nil {
		log.Printf("picture %s: %v", path, err)
		return nil
	}
	src, err := picdecode.Decode(b)
	if err != nil {
		log.Printf("picture %s: %v", path, err)
		return nil
	}
	return src
}

func (p *pictures) preload(paths ...string) {
	p.workers.Go(func() {
		if p.after != nil {
			<-p.after
		}
		for _, path := range paths {
			if path == "" {
				continue
			}
			p.mu.Lock()
			ready := p.imgs[path] != nil || p.decoded[path] != nil || p.loading[path] != nil
			gen := p.gen
			p.mu.Unlock()
			if ready {
				continue
			}
			src := p.decode(path)
			if src == nil {
				continue
			}
			n := src.Bounds().Dx() * src.Bounds().Dy() * 4
			p.mu.Lock()
			if gen == p.gen && p.imgs[path] == nil && p.decoded[path] == nil && p.loading[path] == nil && n <= p.budget-p.bytes-p.decodedBytes {
				p.decoded[path] = src
				p.decodedBytes += n
			}
			p.mu.Unlock()
		}
	})
}

func (p *pictures) head(path string) image.Rectangle {
	img := p.get(path)
	p.mu.Lock()
	defer p.mu.Unlock()
	if i := strings.LastIndex(path, "/"); i >= 0 {
		if b := p.boxes[path[:i]]; len(b) == 3 {
			w, h := float64(img.Bounds().Dx()), float64(img.Bounds().Dy())
			s := b[2] * w
			return image.Rect(int(b[0]*w-s/2), int(b[1]*h-s/2), int(b[0]*w+s/2), int(b[1]*h+s/2))
		}
	}
	if r, ok := p.foundFace(path, img.Bounds().Size()); ok {
		return r
	}
	if r, ok := p.heads[path]; ok && !r.Empty() {
		return r
	}
	w, h := img.Bounds().Dx(), img.Bounds().Dy()
	s := min(w, h) / 2
	return image.Rect(w/2-s/2, 0, w/2+s/2, s)
}

// foundFace is the square a face icon is cut from around the face gozen art faces found
// in the picture (assets/faces.json), scaled to the picture as loaded; false when it found
// none. The caller holds p.mu.
func (p *pictures) foundFace(path string, size image.Point) (image.Rectangle, bool) {
	if !p.foundRead {
		p.foundRead = true
		if p.fsys != nil {
			f, _, err := data.ReadFaceBoxes(p.fsys)
			if err != nil {
				log.Print(err)
			}
			p.found = f.Pictures
		}
	}
	f, ok := p.found[path]
	if !ok || f.Box == nil || f.Size[0] <= 0 || f.Size[1] <= 0 {
		return image.Rectangle{}, false
	}
	sx, sy := float64(size.X)/float64(f.Size[0]), float64(size.Y)/float64(f.Size[1])
	cx, cy := float64(f.Box[0]+f.Box[2])/2*sx, float64(f.Box[1]+f.Box[3])/2*sy
	half := math.Max(float64(f.Box[2]-f.Box[0])*sx, float64(f.Box[3]-f.Box[1])*sy) * doll.FaceMargin / 2
	return image.Rect(int(cx-half), int(cy-half), int(cx+half), int(cy+half)), true
}

func (p *pictures) foundHead(path string) image.Rectangle {
	img := p.get(path)
	p.mu.Lock()
	defer p.mu.Unlock()
	if r, ok := p.heads[path]; ok && !r.Empty() {
		return r
	}
	w, h := img.Bounds().Dx(), img.Bounds().Dy()
	s := min(w, h) / 2
	return image.Rect(w/2-s/2, 0, w/2+s/2, s)
}

func findHead(img image.Image) image.Rectangle {
	b := img.Bounds()
	w, h := b.Dx(), b.Dy()
	opaque := func(x, y int) bool {
		_, _, _, a := img.At(b.Min.X+x, b.Min.Y+y).RGBA()
		return a > 0x8000
	}
	if opaque(0, 0) && opaque(w-1, 0) {
		return image.Rectangle{}
	}
	side := int(float64(w) * 0.42)
	top := -1
	for y := 0; y < h/2 && top < 0; y += 2 {
		n := 0
		for x := 0; x < w; x += 2 {
			if opaque(x, y) {
				n++
			}
		}
		if n*2 > w/25 {
			top = y
		}
	}
	if top < 0 {
		return image.Rectangle{}
	}

	var sum, n float64
	for y := top + side/5; y < min(h, top+side); y += 3 {
		for x := 0; x < w; x += 3 {
			if opaque(x, y) {
				sum += float64(x)
				n++
			}
		}
	}
	if n == 0 {
		return image.Rectangle{}
	}
	cx := int(sum / n)
	y0 := max(0, top-side/12)
	x0 := max(0, min(w-side, cx-side/2))
	return image.Rect(x0, y0, x0+side, min(h, y0+side))
}

func (p *pictures) blurred(path string) *ebiten.Image {
	p.mu.Lock()
	img, ok := p.blurs[path]
	p.mu.Unlock()
	if ok {
		return img
	}
	src := p.get(path)
	w, h := src.Bounds().Dx(), src.Bounds().Dy()
	small := ebiten.NewImage(max(1, w/24), max(1, h/24))
	op := &ebiten.DrawImageOptions{Filter: ebiten.FilterLinear}
	op.GeoM.Scale(float64(small.Bounds().Dx())/float64(w), float64(small.Bounds().Dy())/float64(h))
	small.DrawImage(src, op)
	p.mu.Lock()
	p.blurs[path] = small
	p.mu.Unlock()
	return small
}

func (g *Game) drawPicture(dst *ebiten.Image, pic data.Picture, x, y, w, h float64, alpha float32) {
	if pic.File == "" {
		return
	}
	img := g.pics.get(pic.File)
	if pic.Blur {
		img = g.pics.blurred(pic.File)
	}
	iw, ih := float64(img.Bounds().Dx()), float64(img.Bounds().Dy())
	s := math.Max(w/iw, h/ih)
	sw, sh := w/s, h/s
	sx, sy := (iw-sw)/2, (ih-sh)/2
	sub, ok := img.SubImage(image.Rect(int(sx), int(sy), int(math.Ceil(sx+sw)), int(math.Ceil(sy+sh)))).(*ebiten.Image)
	if !ok || sub.Bounds().Empty() {
		return
	}
	op := &ebiten.DrawImageOptions{Filter: ebiten.FilterLinear}
	op.GeoM.Scale(w/float64(sub.Bounds().Dx()), h/float64(sub.Bounds().Dy()))
	op.GeoM.Translate(x, y)
	tint(&op.ColorScale, pic)
	op.ColorScale.ScaleAlpha(alpha)
	gfx.DrawImage(dst, sub, op)
}

func tint(cs *ebiten.ColorScale, pic data.Picture) {
	if pic.Tint.Valid {
		c := pic.Tint.NRGBA
		cs.Scale(float32(c.R)/255, float32(c.G)/255, float32(c.B)/255, 1)
	}
	if v := pic.Brightness(); v != 1 {
		b := float32(v)
		cs.Scale(b, b, b, 1)
	}
}

func (g *Game) drawFace(dst *ebiten.Image, path string, x, y, size float64, alpha float32, gray bool) {
	if path == "" {
		return
	}
	img := g.pics.get(path)
	r := g.pics.head(path)
	sub, ok := img.SubImage(r).(*ebiten.Image)
	if !ok || sub.Bounds().Empty() {
		return
	}
	if sh := g.pics.shadedFace(path, r); sh != nil {
		sub = sh
	}
	op := &ebiten.DrawImageOptions{Filter: ebiten.FilterLinear}
	op.GeoM.Scale(size/float64(r.Dx()), size/float64(r.Dy()))
	op.GeoM.Translate(x, y)
	op.ColorScale.ScaleAlpha(alpha)
	if gray {
		op.ColorScale.Scale(0.45, 0.4, 0.5, 1)
	}
	gfx.FillRoundRect(dst, float32(x), float32(y), float32(size), float32(size), 14, color.NRGBA{0xff, 0xff, 0xff, uint8(40 * alpha)})
	gfx.DrawImage(dst, sub, op)
}

const headRoom = 56

func (g *Game) drawStanding(dst *ebiten.Image, path string, cx, bottom, h float64, alpha float32, flip bool, cs *ebiten.ColorScale) {
	img := g.pics.get(path)
	if img == nil {
		return
	}
	iw, ih := float64(img.Bounds().Dx()), float64(img.Bounds().Dy())
	s := h / ih
	op := &ebiten.DrawImageOptions{Filter: ebiten.FilterLinear}
	if flip {
		op.GeoM.Scale(-1, 1)
		op.GeoM.Translate(iw, 0)
	}
	op.GeoM.Scale(s, s)
	op.GeoM.Translate(cx-iw*s/2, bottom-h)
	if cs != nil {
		op.ColorScale = *cs
	}
	op.ColorScale.ScaleAlpha(alpha)
	gfx.DrawImage(dst, img, op)
}

// close waits for borrowed-FS reads before releasing this session's images.
func (p *pictures) close() {
	p.workers.Wait()
	p.mu.Lock()
	defer p.mu.Unlock()
	images := make(map[*ebiten.Image]bool)
	for _, e := range p.imgs {
		images[e.img] = true
	}
	for _, img := range p.blurs {
		images[img] = true
	}
	for _, img := range p.shaded {
		images[img] = true
	}
	for img := range images {
		if img != nil && img != missing {
			img.Deallocate()
		}
	}
	clear(p.imgs)
	clear(p.blurs)
	clear(p.shaded)
	clear(p.decoded)
	p.decodedBytes = 0
}
