package gfx

import (
	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/text/v2"
	"github.com/hajimehoshi/ebiten/v2/vector"
)

// Reset drops cached GPU resources between runs; use only on the game thread.
func Reset() {
	resetGlyphs()
	for _, img := range active.shapeCache {
		img.Deallocate()
	}
	clear(active.shapeCache)
	for _, t := range active.textCache {
		t.img.Deallocate()
	}
	clear(active.textCache)
	for _, img := range active.maskCache {
		img.Deallocate()
	}
	clear(active.maskCache)
	clear(active.faceCache)
	clear(active.popFaces)
	clear(active.wrapCache)
	clear(active.ellipsisCache)
	active.wrapBytes, active.ellipsisBytes = 0, 0
	active.regularSource, active.popSource, active.titleSource, active.subSource = nil, nil, nil, nil
	active.scale = 1
}

// Context owns one game's fonts, scale and cached GPU resources. Presentation
// activates it only on the game thread; different contexts must not draw concurrently.
type Context struct {
	regularSource, popSource, titleSource, subSource *text.GoTextFaceSource
	faceCache                                        map[faceKey]*text.GoTextFace
	popOn                                            bool
	popFaces                                         map[float64]*text.MultiFace
	shapeCache                                       map[shapeKey]*ebiten.Image
	textCache                                        map[textKey]textImage
	wrapCache                                        map[fitKey][]string
	ellipsisCache                                    map[fitKey]string
	wrapBytes, ellipsisBytes                         int
	glyphCache                                       map[*ebiten.Image]*ebiten.Image
	glyphPage                                        *ebiten.Image
	glyphX, glyphY, glyphRow                         int
	maskCache                                        map[[3]int]*ebiten.Image
	scale                                            float64
	drawGlyphBuffer                                  []text.LazyGlyph
	roundPath                                        vector.Path
}

// NewContext allocates empty presentation resources for a game.
func NewContext() *Context {
	return &Context{scale: 1, faceCache: map[faceKey]*text.GoTextFace{}, popFaces: map[float64]*text.MultiFace{}, shapeCache: map[shapeKey]*ebiten.Image{}, textCache: map[textKey]textImage{}, wrapCache: map[fitKey][]string{}, ellipsisCache: map[fitKey]string{}, glyphCache: map[*ebiten.Image]*ebiten.Image{}, maskCache: map[[3]int]*ebiten.Image{}}
}

var active = NewContext()

// Activate selects this context until the returned restore function is called.
// Both calls must occur on the game thread, with nested activations restored in order.
func (c *Context) Activate() func() {
	previous := active
	active = c
	return func() { active = previous }
}

// Close releases only this game's GPU resources, preserving the active caller.
func (c *Context) Close() {
	defer c.Activate()()
	Reset()
}
