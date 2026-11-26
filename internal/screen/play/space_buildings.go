package play

import (
	"image"
	"image/color"
	"maps"
	"math"
	"slices"

	"github.com/hajimehoshi/ebiten/v2"

	data "github.com/nao1215/gozen/internal/model/gamedata"
)

const spaceBuildingAtlasHeight = 1664

func spaceAtlasHeight(d *data.Dungeon) int {
	if len(d.Space.Roofs) > 0 || len(d.Space.Materials) > 0 || len(d.Space.WallRegions) > 0 {
		return spaceBuildingAtlasHeight
	}
	return data.MaxSpaceSide * data.MaxSpaceLevels
}

// Static appearance and roof indices are cached separately from conditional
// occupancy. Rebuilding a door must not reconstruct every material or roof.
func (v *spaceView) prepareBuildings(d *data.Dungeon) {
	if v.buildingDungeon == d {
		return
	}
	v.buildingDungeon = d
	v.buildingPixels = nil
	v.roofBounds = make([]float32, 64*4)
	v.roofPlanes = make([]float32, 64*4)
	v.roofBottom = make([]float32, 64*2)
	v.roofAppearance = make([]float32, 64*2)
	v.materialNames = slices.Sorted(maps.Keys(d.Space.Materials))
	v.materialSourcesBuilding = nil
	v.buildingTop = float32(len(d.Space.Levels))
	if spaceAtlasHeight(d) == 512 {
		return
	}
	v.buildingPixels = make([]byte, (spaceBuildingAtlasHeight-512)*64*4)
	for i := 3; i < len(v.buildingPixels); i += 4 {
		v.buildingPixels[i] = 255
	}
	id := func(name string) int {
		i, ok := slices.BinarySearch(v.materialNames, name)
		if !ok {
			return 0
		}
		return i + 1
	}
	for i, p := range d.Space.RoofParts() {
		if p.Filled {
			v.roofBottom[i*2] = 1
		}
		v.roofBottom[i*2+1] = float32(p.Base - p.Thickness)
		for j, b := range p.Bounds {
			v.roofBounds[i*4+j] = float32(b)
		}
		for j, b := range []float64{p.A, p.B, p.C, p.Thickness} {
			v.roofPlanes[i*4+j] = float32(b)
		}
		r := d.Space.Roofs[p.Roof]
		v.roofAppearance[i*2] = float32(id(r.Material))
		v.roofAppearance[i*2+1] = float32(id(r.Underside))
		top := float64(r.Level-1)*data.SpaceStoryHeight + r.Base + r.Rise
		v.buildingTop = max(v.buildingTop, float32(math.Ceil(top/data.SpaceStoryHeight)))
	}
	for z := range 64 {
		for x := range 64 {
			for slot, i := range d.Space.RoofCandidates(x, z) {
				offset := ((slot/3*64+z)*64+x)*4 + slot%3
				v.buildingPixels[offset] = byte(i + 1) // #nosec G115 -- At most 64 validated roof parts.
			}
		}
	}
	// Floor materials share the west-face rows: their red channel is the west
	// wall and green is the walking surface of the same cell and story.
	floor := func(level, x, z int, name string) {
		if level < 1 || x < 0 || z < 0 || x >= 64 || z >= 64 {
			return
		}
		v.buildingPixels[((640+(level-1)*64+z)*64+x)*4+1] = byte(id(name)) // #nosec G115 -- At most 32 validated material IDs.
	}
	for _, r := range d.Space.FloorRegions {
		for z := r.Y; z < r.Y+r.Depth; z++ {
			for x := r.X; x < r.X+r.Width; x++ {
				floor(r.Level, x, z, r.Material)
			}
		}
	}
	for _, stair := range d.Space.Stairs {
		if stair.Material == "" {
			continue
		}
		dx, dy := data.StairDelta(stair)
		for n := range stair.Length {
			floor(stair.Level, stair.X+n*dx, stair.Y+n*dy, stair.Material)
		}
	}
	for _, r := range d.Space.WallRegions {
		names := [4]string{r.North, r.East, r.South, r.West}
		for face, name := range names {
			if name == "" {
				name = r.Material
			}
			for z := r.Y; z < r.Y+r.Depth; z++ {
				for x := r.X; x < r.X+r.Width; x++ {
					offset := ((128+(r.Level-1)*64+z)*64+x)*4 + face
					if face == 3 {
						offset = ((640+(r.Level-1)*64+z)*64 + x) * 4
					}
					v.buildingPixels[offset] = byte(id(name)) // #nosec G115 -- At most 32 validated material IDs.
				}
			}
		}
	}
}

func (v *spaceView) roofUniforms(d *data.Dungeon, uniforms map[string]any) {
	if len(d.Space.Roofs) == 0 {
		return
	}
	uniforms["RoofCount"] = float32(len(d.Space.RoofParts()))
	uniforms["RoofBounds"], uniforms["RoofPlanes"] = v.roofBounds, v.roofPlanes
	uniforms["RoofBottom"] = v.roofBottom
	uniforms["GeometryTop"] = v.buildingTop
}

// A single atlas batches all facade and roof textures, 512 pixels a side each
// (a picture hung on a wall stays legible up close). Keep original images as
// cache keys so an asset reload rebuilds it without per-frame uploads; an
// animated material redraws only its own tile, when its frame changes.
func (v *spaceView) buildingMaterials(g *Game, d *data.Dungeon, op *ebiten.DrawTrianglesShaderOptions) {
	if spaceAtlasHeight(d) == 512 {
		return
	}
	if v.materialFramesDungeon != d {
		v.materialFramesDungeon = d
		v.materialFrames = make([][]string, len(v.materialNames))
		for i, name := range v.materialNames {
			v.materialFrames[i] = d.Space.Materials[name].FramePictures(g.db.FS)
		}
	}
	sources := make([]*ebiten.Image, len(v.materialNames)+1)
	sources[0] = g.pics.get(d.Wall.File)
	for i, name := range v.materialNames {
		m := d.Space.Materials[name]
		file := m.Picture.File
		if frames := v.materialFrames[i]; len(frames) > 0 {
			file = frames[int(float64(v.clock)*m.FrameRate()/60)%len(frames)]
		}
		sources[i+1] = g.pics.get(file)
	}
	if v.buildingMaterialAtlas == nil {
		v.buildingMaterialAtlas = ebiten.NewImage(8*512, 5*512)
		v.materialSourcesBuilding = nil
	}
	for i, img := range sources {
		if i < len(v.materialSourcesBuilding) && v.materialSourcesBuilding[i] == img {
			continue
		}
		tile := image.Rect(i%8*512, i/8*512, i%8*512+512, i/8*512+512)
		if slot, ok := v.buildingMaterialAtlas.SubImage(tile).(*ebiten.Image); ok {
			slot.Fill(color.White)
		}
		if img == nil {
			continue
		}
		draw := &ebiten.DrawImageOptions{Filter: ebiten.FilterLinear}
		draw.GeoM.Scale(512/float64(img.Bounds().Dx()), 512/float64(img.Bounds().Dy()))
		draw.GeoM.Translate(float64(tile.Min.X), float64(tile.Min.Y))
		v.buildingMaterialAtlas.DrawImage(img, draw)
	}
	v.materialSourcesBuilding = sources
	// Uniforms are static until DB replacement; the material images above can
	// change independently during hot reload.
	if v.buildingColors == nil || v.buildingColorDungeon != d {
		v.buildingColorDungeon = d
		v.buildingColors = make([]float32, 33*4)
		v.buildingUV = make([]float32, 33*4)
		for i, name := range v.materialNames {
			m := d.Space.Materials[name]
			c := m.Color.Or(color.NRGBA{R: 255, G: 255, B: 255, A: 255})
			tint := m.Picture.Tint.Or(color.NRGBA{R: 255, G: 255, B: 255, A: 255})
			b := float32(m.Picture.Brightness()) / 255 / 255
			n := (i + 1) * 4
			v.buildingColors[n], v.buildingColors[n+1], v.buildingColors[n+2] = float32(c.R)*float32(tint.R)*b, float32(c.G)*float32(tint.G)*b, float32(c.B)*float32(tint.B)*b
			if m.Textured() {
				v.buildingColors[n+3] = 1
			}
			v.buildingUV[n], v.buildingUV[n+1], v.buildingUV[n+2], v.buildingUV[n+3] = float32(m.Width), float32(m.Height), float32(m.OriginU), float32(m.OriginV)
		}
	}
	op.Images[1] = v.buildingMaterialAtlas
	op.Uniforms["BuildingEnabled"] = float32(1)
	op.Uniforms["BuildingColors"], op.Uniforms["BuildingUV"] = v.buildingColors, v.buildingUV
	op.Uniforms["RoofAppearance"] = v.roofAppearance
}
