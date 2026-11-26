package play

import (
	"bytes"
	_ "embed"
	"encoding/json"
	"fmt"
	"image/color"
	"math"
	"slices"

	"github.com/hajimehoshi/ebiten/v2"

	data "github.com/nao1215/gozen/internal/model/gamedata"
	"github.com/nao1215/gozen/internal/model/rpg"
	"github.com/nao1215/gozen/internal/rules/dungeon"
)

//go:embed space_geometry.kage
var spaceGeometrySource []byte

//go:embed space.kage
var spaceShaderSource []byte

type spaceView struct {
	buildingDungeon, buildingColorDungeon   *data.Dungeon
	buildingPixels                          []byte
	roofBounds, roofPlanes, roofAppearance  []float32
	roofBottom                              []float32
	buildingTop                             float32
	materialNames                           []string
	materialSourcesBuilding                 []*ebiten.Image
	materialNormalSources                   []*ebiten.Image
	lights, lightColors                     []float32
	openingPlane, openingRect, openingLook  []float32
	buildingGlow                            []float32
	materialFrames                          [][]string
	materialFramesDungeon                   *data.Dungeon
	clock                                   int
	buildingMaterialAtlas                   *ebiten.Image
	buildingColors, buildingUV              []float32
	spriteShader                            *ebiten.Shader
	sprites                                 []spaceSprite
	visibleSprites                          []visibleSpaceSprite
	shader                                  *ebiten.Shader
	atlas                                   *ebiten.Image
	frame                                   *ebiten.Image
	ceilingSky                              *ebiten.Image
	materialSources                         [2]*ebiten.Image
	pixels                                  []byte
	from                                    rpg.Pos
	remaining, duration                     int
	turn                                    float64
	occupancyDungeon                        *data.Dungeon
	occupancyDB                             *data.DB
	occupancyState                          *rpg.State
	occupancyKey                            []byte
	occupancyConditional, occupancyVolatile bool
}

func (s *dungeonScene) spaceMotion(from rpg.Pos, turn float64, ticks int) {
	if s.space == nil {
		s.space = &spaceView{}
	}
	v := s.space
	v.from, v.turn = from, turn
	v.duration = ticks
	if v.duration == 0 {
		v.duration = 18
	}
	v.remaining = v.duration
}

func (s *dungeonScene) drawSpace(g *Game, dst *ebiten.Image, d *data.Dungeon) {
	if s.space == nil {
		s.space = &spaceView{}
	}
	v := s.space
	v.clock = s.age
	newAtlas := v.atlas == nil || v.atlas.Bounds().Dy() != spaceAtlasHeight(d)
	if v.shader == nil {
		var err error
		v.shader, err = newSpaceShader(spaceShaderSource)
		if err != nil {
			g.failure = fmt.Errorf("space shader: %w", err)
			return
		}
	}
	if newAtlas {
		if v.atlas != nil {
			v.atlas.Deallocate()
		}
		v.atlas = ebiten.NewImage(data.MaxSpaceSide, spaceAtlasHeight(d))
	}
	if v.refreshOccupancy(g.st, d) || newAtlas {
		v.atlas.WritePixels(v.pixels)
	}
	// Keep software-rendered CI and browser costs bounded independently of window
	// size; a game raises the cap with render_width.
	w := min(dst.Bounds().Dx(), d.Space.ViewWidth())
	h := max(1, w*dst.Bounds().Dy()/dst.Bounds().Dx())
	if v.frame == nil || v.frame.Bounds().Dx() != w || v.frame.Bounds().Dy() != h {
		if v.frame != nil {
			v.frame.Deallocate()
		}
		v.frame = ebiten.NewImage(w, h)
	}
	now := g.st.Pos
	x, z := float64(now.X)+.5, float64(now.Y)+.5
	height := data.SpaceHeight(d.Space, now.Level, now.X, now.Y) + .65
	yaw := float64(now.Facing) * math.Pi / 2
	if p := g.st.SpacePose; d.Space.FreeMove != nil && p != nil && p.In == g.st.Location {
		x, z, yaw = p.X, p.Z, p.Yaw
		height = data.SpaceHeightAt(d.Space, p.Level, p.X, p.Z) + .65
	} else if v.remaining > 0 {
		t := 1 - float64(v.remaining)/float64(v.duration)
		t = t * t * (3 - 2*t)
		x = float64(v.from.X) + .5 + float64(now.X-v.from.X)*t
		z = float64(v.from.Y) + .5 + float64(now.Y-v.from.Y)*t
		old := data.SpaceHeight(d.Space, v.from.Level, v.from.X, v.from.Y) + .65
		height = old + (height-old)*t
		yaw = float64(v.from.Facing)*math.Pi/2 + v.turn*t
	}
	mw, mh := d.Size()
	floor := d.Floor.Or(color.NRGBA{R: 122, G: 138, B: 148, A: 255})
	ceiling := d.Ceiling.Or(color.NRGBA{R: 122, G: 138, B: 148, A: 255})
	rgb := func(c color.NRGBA) []float32 {
		return []float32{float32(c.R) / 255, float32(c.G) / 255, float32(c.B) / 255}
	}
	op := &ebiten.DrawTrianglesShaderOptions{Uniforms: map[string]any{"FloorColor": rgb(floor), "CeilingColor": rgb(ceiling), "Darkness": float32(d.Dark), "Eye": []float32{float32(x), float32(height), float32(z)}, "Facing": float32(yaw), "ViewSize": []float32{float32(w), float32(h)}, "MapSize": []float32{float32(mw), float32(mh)}, "Levels": float32(len(d.Space.Levels)), "Far": float32(d.Depth)}}
	op.Images[0] = v.atlas
	materials := [3]data.Picture{d.Wall, d.FloorTex, d.CeilingTex}
	var enabled [3]float32
	for i, material := range materials {
		if material.File == "" {
			continue
		}
		if i < 2 {
			op.Images[i+1] = g.pics.get(material.File)
		}
		enabled[i] = 1
	}
	op.Uniforms["Textured"] = enabled[:]
	op.Images[3] = v.ceilingAndSky(g, d.CeilingTex, d.Space.Sky)
	skyTint := rgb(d.Space.Sky.Tint.Or(color.NRGBA{R: 255, G: 255, B: 255, A: 255}))
	for i := range skyTint {
		skyTint[i] *= float32(d.Space.Sky.Brightness())
	}
	op.Uniforms["SkyTint"] = skyTint
	op.Uniforms["HasSky"] = float32(0)
	if d.Space.Sky.File != "" {
		op.Uniforms["HasSky"] = float32(1)
	}
	for i, name := range []string{"WallTint", "FloorTint", "CeilingTint"} {
		c := materials[i].Tint.Or(color.NRGBA{R: 255, G: 255, B: 255, A: 255})
		tint := rgb(c)
		base := color.NRGBA{R: 255, G: 255, B: 255, A: 255}
		if i == 1 {
			base = d.Floor.Or(base)
		}
		if i == 2 {
			base = d.Ceiling.Or(base)
		}
		baseRGB := rgb(base)
		for j := range tint {
			tint[j] *= baseRGB[j] * float32(materials[i].Brightness())
		}
		op.Uniforms[name] = tint
	}
	v.roofUniforms(d, op.Uniforms)
	v.lightUniforms(d, op.Uniforms)
	v.buildingMaterials(g, d, op)
	v.frame.DrawTrianglesShader([]ebiten.Vertex{{DstX: 0, DstY: 0}, {DstX: float32(w), DstY: 0}, {DstX: 0, DstY: float32(h)}, {DstX: float32(w), DstY: float32(h)}}, []uint16{0, 1, 2, 1, 3, 2}, v.shader, op)
	v.drawSprites(g, d, [3]float64{x, height, z}, yaw, w, h)
	imageOp := &ebiten.DrawImageOptions{Filter: ebiten.FilterLinear}
	imageOp.GeoM.Scale(float64(dst.Bounds().Dx())/float64(w), float64(dst.Bounds().Dy())/float64(h))
	dst.DrawImage(v.frame, imageOp)
}

// lightUniforms passes the space's daylight, clock and point lights.
func (v *spaceView) lightUniforms(d *data.Dungeon, uniforms map[string]any) {
	if len(v.lights) != data.MaxSpaceLights*4 {
		v.lights, v.lightColors = make([]float32, data.MaxSpaceLights*4), make([]float32, data.MaxSpaceLights*4)
	}
	n := min(len(d.Space.Lights), data.MaxSpaceLights)
	for i, l := range d.Space.Lights[:n] {
		x, y, z, r := l.Position()
		c := l.Color.Or(color.NRGBA{R: 255, G: 214, B: 160, A: 255})
		p := l.Strength()
		v.lights[i*4], v.lights[i*4+1], v.lights[i*4+2], v.lights[i*4+3] = float32(x), float32(y), float32(z), float32(r)
		v.lightColors[i*4], v.lightColors[i*4+1], v.lightColors[i*4+2], v.lightColors[i*4+3] = float32(float64(c.R)/255*p), float32(float64(c.G)/255*p), float32(float64(c.B)/255*p), float32(l.Flicker)
	}
	if len(v.openingPlane) != data.MaxSpaceOpenings*4 {
		v.openingPlane, v.openingRect, v.openingLook = make([]float32, data.MaxSpaceOpenings*4), make([]float32, data.MaxSpaceOpenings*4), make([]float32, data.MaxSpaceOpenings)
	}
	openings := d.Space.Openings[:min(len(d.Space.Openings), data.MaxSpaceOpenings)]
	for i, o := range openings {
		axis, at, out := o.Plane()
		base := float64(o.Level-1) * data.SpaceStoryHeight
		v.openingPlane[i*4], v.openingPlane[i*4+1], v.openingPlane[i*4+2], v.openingPlane[i*4+3] = float32(axis), float32(at), float32(out), float32(o.Depth)
		v.openingRect[i*4], v.openingRect[i*4+1], v.openingRect[i*4+2], v.openingRect[i*4+3] = float32(o.Along), float32(o.Along+o.Width), float32(base+o.Bottom), float32(base+o.Bottom+o.Height)
		v.openingLook[i] = 0
		if j, ok := slices.BinarySearch(v.materialNames, o.Material); ok {
			v.openingLook[i] = float32(j + 1)
		}
	}
	uniforms["OpeningCount"] = float32(len(openings))
	uniforms["OpeningPlane"], uniforms["OpeningRect"], uniforms["OpeningLook"] = v.openingPlane, v.openingRect, v.openingLook
	if len(v.buildingGlow) != 33 {
		v.buildingGlow = make([]float32, 33)
	}
	for i, name := range v.materialNames[:min(len(v.materialNames), 32)] {
		v.buildingGlow[i+1] = float32(d.Space.Materials[name].Glow)
	}
	uniforms["BuildingGlow"] = v.buildingGlow
	uniforms["Daylight"] = float32(d.Space.DaylightLevel())
	uniforms["Time"] = float32(float64(v.clock) / 60)
	uniforms["LightCount"] = float32(n)
	uniforms["Lights"], uniforms["LightColors"] = v.lights, v.lightColors
}

// spriteLight is how lit an upright picture at a point is: the daylight fill
// and the reach of the point lights, without shadows (a picture stands in one
// cell and is lit as a whole).
func spriteLight(s *data.Space, at [3]float64) float64 {
	day := s.DaylightLevel()
	light := day + (1-day)*.12
	for _, l := range s.Lights[:min(len(s.Lights), data.MaxSpaceLights)] {
		x, y, z, r := l.Position()
		d := math.Sqrt((x-at[0])*(x-at[0]) + (y-at[1]-.45)*(y-at[1]-.45) + (z-at[2])*(z-at[2]))
		if d < r {
			light += (1 - d/r) * (1 - d/r) * l.Strength()
		}
	}
	return min(light, 1.3)
}

// Share one sampler between the ceiling tile and the sky panorama. Rebuild only
// when their cached images change, including after an asset hot reload.
func (v *spaceView) ceilingAndSky(g *Game, ceiling, sky data.Picture) *ebiten.Image {
	sources := [2]*ebiten.Image{g.pics.get(ceiling.File), g.pics.get(sky.File)}
	if v.ceilingSky != nil && v.materialSources == sources {
		return v.ceilingSky
	}
	if v.ceilingSky == nil {
		v.ceilingSky = ebiten.NewImage(1024, 1024)
	}
	v.ceilingSky.Clear()
	for i, img := range sources {
		if img == nil {
			continue
		}
		width := 512
		if i == 1 {
			width = 1024
		}
		op := &ebiten.DrawImageOptions{Filter: ebiten.FilterLinear}
		op.GeoM.Scale(float64(width)/float64(img.Bounds().Dx()), 512/float64(img.Bounds().Dy()))
		op.GeoM.Translate(0, float64(i*512))
		v.ceilingSky.DrawImage(img, op)
	}
	v.materialSources = sources
	return v.ceilingSky
}

// refreshOccupancy reports whether the atlas needs uploading.
func (v *spaceView) refreshOccupancy(state *rpg.State, d *data.Dungeon) bool {
	changed := v.occupancyDungeon != d || v.occupancyDB != state.DB || v.occupancyState != state
	if changed {
		v.occupancyDungeon, v.occupancyDB, v.occupancyState = d, state.DB, state
		v.occupancyConditional, v.occupancyVolatile = false, false
		for _, tile := range d.Tiles {
			for _, condition := range []data.Expr{tile.When, tile.Solid} {
				v.occupancyConditional = v.occupancyConditional || condition.Set()
				names := condition.Names()
				v.occupancyVolatile = v.occupancyVolatile || slices.Contains(names, "rand") || slices.Contains(names, "chance")
			}
		}
	}
	// Definitions are immutable between reloads (which replace DB/dungeon pointers).
	// Static tiles need only mutations and once-state. Expressions can read actors,
	// inventory, campaign, choices and position: conservatively snapshot the state.
	var input any = struct {
		Location string
		Done     map[string]bool
		Put      map[string]string
	}{state.Location, state.Done, state.Put}
	if v.occupancyConditional {
		snapshot := *state
		// These do not participate in field expressions; camera-only updates must
		// not invalidate geometry. Everything else saved stays in the snapshot.
		snapshot.Frames, snapshot.Mapped, snapshot.Reactions = 0, nil, nil
		input = struct {
			State   *rpg.State
			Repairs rpg.LoadReport
		}{&snapshot, state.LoadReport}
	}
	key, err := json.Marshal(input)
	if !changed && !v.occupancyVolatile && err == nil && bytes.Equal(key, v.occupancyKey) {
		return false
	}
	v.buildOccupancy(state, d)
	v.occupancyKey = key
	// Failed serialization deliberately forces another rebuild next Draw.
	if err != nil {
		v.occupancyDungeon = nil
	}
	return true
}

func (v *spaceView) buildOccupancy(state *rpg.State, d *data.Dungeon) {
	v.prepareBuildings(d)
	if len(v.pixels) != data.MaxSpaceSide*spaceAtlasHeight(d)*4 {
		v.pixels = make([]byte, data.MaxSpaceSide*spaceAtlasHeight(d)*4)
	}
	// One compact bounded occupancy atlas drives depth-correct intersections. Build
	// from the same cell rules as movement, so once-only doors and conditions agree.
	clear(v.pixels)
	if len(v.buildingPixels) > 0 {
		copy(v.pixels[512*64*4:], v.buildingPixels)
	}
	clear(v.sprites)
	v.sprites = v.sprites[:0]
	st := *state
	for level := range d.Space.Levels {
		st.Pos.Level = level
		view := data.DungeonLevel(d, level)
		for y, row := range view.Grid {
			for x := range row {
				i := ((level*data.MaxSpaceSide+y)*data.MaxSpaceSide + x) * 4
				v.pixels[i+3] = 255
				c := dungeon.CellAt(view, &st, x, y)
				if c.Char == data.CellNone {
					continue
				}
				if c.Tile != nil && c.Tile.Picture.File != "" {
					v.sprites = append(v.sprites, spaceSprite{[3]float64{float64(x) + .5, data.SpaceHeight(d.Space, level, x, y), float64(y) + .5}, c.Tile})
				}
				v.pixels[i] = 1
				if c.Look == "wall" || (c.Solid && (c.Tile == nil || c.Tile.Picture.File == "")) {
					v.pixels[i] = 2
				}
			}
		}
	}
	for _, stair := range d.Space.Stairs {
		dx, dy := data.StairDelta(stair)
		for n := range stair.Length {
			x, y := stair.X+n*dx, stair.Y+n*dy
			i := (((stair.Level-1)*data.MaxSpaceSide+y)*data.MaxSpaceSide + x) * 4
			if i < 0 || i+3 >= len(v.pixels) {
				continue
			}
			v.pixels[i], v.pixels[i+1], v.pixels[i+2] = 3, byte(data.FacingIndex(stair.Facing)*16+stair.Length), byte(n) // #nosec G115 -- Validated direction <= 3, length <= 8 and offset < length.
		}
	}
}

// Both passes use identical bounded terrain intersections.
func newSpaceShader(body []byte) (*ebiten.Shader, error) {
	source := append([]byte(nil), spaceGeometrySource...)
	return ebiten.NewShader(append(source, body...))
}
