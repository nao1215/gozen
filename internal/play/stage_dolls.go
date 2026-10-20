package play

import (
	"fmt"
	"image"
	"image/color"
	"log"
	"strconv"
	"strings"
	"sync"

	"github.com/hajimehoshi/ebiten/v2"

	data "github.com/nao1215/gozen/internal/layer2/gamedata"
	"github.com/nao1215/gozen/internal/layer5/doll"
	"github.com/nao1215/gozen/internal/layer5/stage"
)

// sceneCache keeps what the painted scenes make once: the dolls baked from the standing
// pictures (in the background; a doll shows when it is ready) and each scene's textures.
// A reload of the pictures (F5) drops it.
type sceneCache struct {
	mu      sync.Mutex
	baked   map[string]baked // baked, not yet on the GPU
	pending map[string]bool
	wg      sync.WaitGroup

	sheets    map[string]*ebiten.Image
	faces     map[string]*ebiten.Image
	renderers map[string]*stage.Renderer

	facesMu   sync.Mutex
	facesRead bool
	found     map[string]data.FaceBox // assets/faces.json
}

// baked is a doll's sheet and the round face for its balloons (nil without a picture).
type baked struct {
	sheet, face *image.RGBA
}

func (g *Game) scenes() *sceneCache {
	if g.sceneCache == nil {
		g.sceneCache = &sceneCache{baked: map[string]baked{}, pending: map[string]bool{},
			sheets: map[string]*ebiten.Image{}, faces: map[string]*ebiten.Image{}, renderers: map[string]*stage.Renderer{}}
	}
	return g.sceneCache
}

// closeScenes frees the scenes' textures and forgets the dolls (the pictures may have changed).
func (g *Game) closeScenes() {
	c := g.sceneCache
	if c == nil {
		return
	}
	// the bakes still running finish into this dropped cache (not waited for)
	for _, img := range c.sheets {
		img.Deallocate()
	}
	for _, img := range c.faces {
		img.Deallocate()
	}
	for _, r := range c.renderers {
		r.Close()
	}
	g.sceneCache = nil
	for _, s := range g.stack { // the scene shown makes its textures again
		if ss, ok := s.(*stageScene); ok {
			ss.r = nil
		}
	}
}

// newDollSpec is a doll to bake, with its key made once.
func newDollSpec(picture string, d *data.Doll, camera float64) dollSpec {
	sp := dollSpec{picture: picture, doll: d, camera: camera}
	sp.k = sp.key()
	return sp
}

func (sp dollSpec) key() string {
	var b strings.Builder
	b.WriteString(sp.picture)
	b.WriteString("|")
	b.WriteString(strconv.FormatFloat(sp.camera, 'g', -1, 64))
	if d := sp.doll; d != nil {
		fmt.Fprintf(&b, "|%s|%v|%v|%s", d.Hair, d.Parts, d.FaceAt, d.Icon)
	}
	return b.String()
}

// waitsForDolls is whether the screen waits for the dolls instead of showing them as they
// come (scripted runs and recordings: the same pictures every time).
func (g *Game) waitsForDolls() bool { return g.opts.Script != "" || g.opts.Video != nil }

// dollSheet is the baked sheet of a doll, or nil while it is being baked.
func (g *Game) dollSheet(sp dollSpec) *ebiten.Image {
	c := g.scenes()
	key := sp.k
	if key == "" {
		key = sp.key()
	}
	if img := c.sheets[key]; img != nil {
		return img
	}
	c.mu.Lock()
	b, done := c.baked[key]
	if !done && !c.pending[key] {
		c.pending[key] = true
		c.wg.Add(1)
		go func() {
			defer c.wg.Done()
			pic := g.pics.decode(sp.picture)
			var found *[4]int
			if pic != nil {
				found = g.foundFace(sp.picture, pic.Bounds().Size())
			}
			out := baked{sheet: bakeDoll(pic, sp), face: faceIcon(pic, sp, found)}
			if sp.doll != nil && sp.doll.Icon != "" { // a face drawn for the balloons
				if icon := g.pics.decode(sp.doll.Icon); icon != nil {
					out.face = doll.Round(doll.Square(icon))
				}
			}
			c.mu.Lock()
			c.baked[key] = out
			c.mu.Unlock()
		}()
	}
	c.mu.Unlock()
	if !done && g.waitsForDolls() {
		c.wg.Wait()
		c.mu.Lock()
		b, done = c.baked[key]
		c.mu.Unlock()
	}
	if !done {
		return nil
	}
	c.mu.Lock()
	delete(c.baked, key)
	c.mu.Unlock()
	img := ebiten.NewImageFromImage(b.sheet)
	c.sheets[key] = img
	if b.face != nil {
		c.faces[key] = ebiten.NewImageFromImage(b.face)
	}
	return img
}

// dollFace is the round face of a doll's balloons, or nil (not baked yet, or no picture).
func (g *Game) dollFace(sp dollSpec) *ebiten.Image {
	if g.dollSheet(sp) == nil {
		return nil
	}
	key := sp.k
	if key == "" {
		key = sp.key()
	}
	return g.scenes().faces[key]
}

// faceIcon cuts the round face of a balloon from the standing picture (a doll's icon, its
// own face picture, comes before it): around the doll's
// face_at when written, else around the face gozen art faces found (assets/faces.json, when it
// is of this picture's size), else where a face usually is.
func faceIcon(pic image.Image, sp dollSpec, found *[4]int) *image.RGBA {
	if pic == nil {
		return nil
	}
	var face image.Image
	switch {
	case sp.doll != nil && len(sp.doll.FaceAt) == 3:
		face = doll.FaceAt(pic, sp.doll.FaceAt[0], sp.doll.FaceAt[1], sp.doll.FaceAt[2])
	case found != nil:
		face = doll.FaceIn(pic, image.Rect(found[0], found[1], found[2], found[3]).Add(pic.Bounds().Min))
	default:
		face = doll.FaceFrom(pic)
	}
	return doll.Round(face)
}

// foundFace is the face box gozen art faces wrote for a picture of the size, or nil.
func (g *Game) foundFace(picture string, size image.Point) *[4]int {
	c := g.scenes()
	c.facesMu.Lock()
	defer c.facesMu.Unlock()
	if !c.facesRead {
		c.facesRead = true
		f, _, err := data.ReadFaceBoxes(g.opts.FS)
		if err != nil {
			log.Print(err)
		}
		c.found = f.Pictures
	}
	f, ok := c.found[picture]
	if !ok || f.Box == nil || f.Size != [2]int{size.X, size.Y} {
		return nil
	}
	return f.Box
}

// bakeDoll bakes a doll with the colors of its picture (plain colors without one).
func bakeDoll(pic image.Image, sp dollSpec) *image.RGBA {
	pal := doll.Palette{Hair: color.RGBA{90, 60, 40, 255}, Skin: color.RGBA{250, 214, 186, 255}, Top: color.RGBA{90, 120, 180, 255},
		Sleeve: color.RGBA{90, 120, 180, 255}, Bottom: color.RGBA{80, 70, 60, 255}, Shoes: color.RGBA{70, 50, 40, 255}}
	hair := ""
	var parts []doll.Part
	if d := sp.doll; d != nil {
		hair = d.Hair
		for _, p := range d.Parts {
			parts = append(parts, doll.Part{Kind: p.Kind, Color: hexRGBA(p.Color)})
		}
	}
	if pic != nil {
		pal = doll.PaletteFrom(pic)
		if hair == "" {
			hair = doll.HairStyleFrom(pic, pal)
		}
	}
	if hair == "" {
		hair = doll.HairShort
	}
	return doll.Bake(pal, hair, parts, stage.SheetW, stage.SheetH, sp.camera)
}

// hexRGBA reads #rrggbb (checked by gozen check; black when not).
func hexRGBA(s string) color.RGBA {
	v, err := strconv.ParseUint(strings.TrimPrefix(s, "#"), 16, 32)
	if err != nil {
		return color.RGBA{A: 255}
	}
	return color.RGBA{uint8(v >> 16), uint8(v >> 8), uint8(v), 255} //nolint:gosec // 24-bit value, split into bytes
}

// stageRenderer is the textures of a location's scene (made on its first showing).
func (g *Game) stageRenderer(id string, l *data.Location, sc *stage.Scene) *stage.Renderer {
	c := g.scenes()
	if r := c.renderers[id]; r != nil {
		return r
	}
	bg := g.pics.decode(l.Background.File)
	if bg == nil {
		bg = image.NewRGBA(image.Rect(0, 0, 1280, 720))
	}
	var depth image.Image
	if l.Stage.Depth != "" {
		depth = g.pics.decode(l.Stage.Depth)
	}
	r, err := stage.NewRenderer(sc, bg, depth)
	if err != nil {
		log.Printf("stage %s: %v", id, err)
		g.failure = err
		return nil
	}
	tint(&r.Look, l.Background)
	c.renderers[id] = r
	return r
}
