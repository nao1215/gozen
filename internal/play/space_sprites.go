package play

import (
	_ "embed"
	"fmt"
	"image"
	"image/color"
	"math"
	"slices"

	"github.com/hajimehoshi/ebiten/v2"

	data "github.com/nao1215/gozen/internal/layer2/gamedata"
)

//go:embed space_sprites.kage
var spaceSpriteSource []byte

// A tile's art is anchored to its actual support surface. Its collider remains
// the tile, while the camera-facing image can extend beyond that cell.
type spaceSprite struct {
	at   [3]float64
	tile *data.Tile
}

type visibleSpaceSprite struct {
	sprite               spaceSprite
	image                *ebiten.Image
	depth, width, height float64
	bounds               image.Rectangle
}

// spriteBounds projects the entire image, including art outside its anchor cell.
// Intersect before converting to integer coordinates to bound GPU work.
func spriteBounds(at, eye [3]float64, yaw, width, height float64, w, h int) (image.Rectangle, float64) {
	dx, dz := at[0]-eye[0], at[2]-eye[2]
	depth := dx*math.Sin(yaw) - dz*math.Cos(yaw)
	if depth <= .01 {
		return image.Rectangle{}, depth
	}
	side := dx*math.Cos(yaw) + dz*math.Sin(yaw)
	focal := float64(h) * .9
	cx := float64(w)*.5 + side*focal/depth
	bottom := float64(h)*.42 - (at[1]-eye[1])*focal/depth
	half := width * focal / depth / 2
	left, right := math.Max(0, cx-half), math.Min(float64(w), cx+half)
	top, bottom := math.Max(0, bottom-height*focal/depth), math.Min(float64(h), bottom)
	if right <= left || bottom <= top {
		return image.Rectangle{}, depth
	}
	return image.Rect(int(math.Floor(left)), int(math.Floor(top)), int(math.Ceil(right)), int(math.Ceil(bottom))), depth
}

func (v *spaceView) drawSprites(g *Game, d *data.Dungeon, eye [3]float64, yaw float64, w, h int) {
	clear(v.visibleSprites)
	v.visibleSprites = v.visibleSprites[:0]
	if len(v.sprites) == 0 {
		return
	}
	if v.spriteShader == nil {
		var err error
		v.spriteShader, err = newSpaceShader(spaceSpriteSource)
		if err != nil {
			g.failure = fmt.Errorf("space sprite shader: %w", err)
			return
		}
	}
	for _, sprite := range v.sprites {
		img := g.pics.get(sprite.tile.Picture.File)
		if img == nil {
			continue
		}
		height := sprite.tile.Size * data.SpaceStoryHeight
		width := height * float64(img.Bounds().Dx()) / float64(img.Bounds().Dy())
		// A large tree or furnishing may overlap the view with its anchor outside Far.
		if math.Hypot(sprite.at[0]-eye[0], sprite.at[2]-eye[2]) > float64(d.Depth)+width/2 {
			continue
		}
		bounds, depth := spriteBounds(sprite.at, eye, yaw, width, height, w, h)
		if bounds.Empty() {
			continue
		}
		v.visibleSprites = append(v.visibleSprites, visibleSpaceSprite{sprite, img, depth, width, height, bounds})
	}
	// All billboards are parallel to the camera plane. Back-to-front order also
	// preserves the alpha holes of nearer sprites; geometry is checked per pixel.
	slices.SortStableFunc(v.visibleSprites, func(a, b visibleSpaceSprite) int {
		if a.depth > b.depth {
			return -1
		}
		if a.depth < b.depth {
			return 1
		}
		return 0
	})
	mw, mh := d.Size()
	for _, item := range v.visibleSprites {
		at := item.sprite.at
		pic := item.sprite.tile.Picture
		color := pic.Tint.Or(color.NRGBA{R: 255, G: 255, B: 255, A: 255})
		brightness := float32(pic.Brightness())
		op := &ebiten.DrawTrianglesShaderOptions{Uniforms: map[string]any{
			"Eye":    []float32{float32(eye[0]), float32(eye[1]), float32(eye[2])},
			"Facing": float32(yaw), "ViewSize": []float32{float32(w), float32(h)},
			"MapSize": []float32{float32(mw), float32(mh)}, "Levels": float32(len(d.Space.Levels)), "Far": float32(d.Depth),
			"SpriteOrigin":   []float32{float32(at[0]), float32(at[1]), float32(at[2])},
			"SpriteSize":     []float32{float32(item.width), float32(item.height)},
			"SpriteTint":     []float32{float32(color.R) / 255 * brightness, float32(color.G) / 255 * brightness, float32(color.B) / 255 * brightness},
			"SpriteDarkness": float32(d.Dark),
		}}
		op.Images[0], op.Images[1] = v.atlas, item.image
		v.roofUniforms(d, op.Uniforms)
		r := item.bounds
		v.frame.DrawTrianglesShader([]ebiten.Vertex{{DstX: float32(r.Min.X), DstY: float32(r.Min.Y)}, {DstX: float32(r.Max.X), DstY: float32(r.Min.Y)}, {DstX: float32(r.Min.X), DstY: float32(r.Max.Y)}, {DstX: float32(r.Max.X), DstY: float32(r.Max.Y)}}, []uint16{0, 1, 2, 1, 3, 2}, v.spriteShader, op)
	}
}
