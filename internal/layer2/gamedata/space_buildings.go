package gamedata

import (
	"fmt"
	"math"

	"github.com/nao1215/gozen/internal/diag"
)

// SpaceMaterial supplies play with a building surface. Width/Height are world
// units per image repetition (default 1); origins align neighboring faces.
// Pictures and colors decorate opaque geometry and never create openings.
type SpaceMaterial struct {
	Picture Picture `yaml:"picture"`
	Color   Color   `yaml:"color"`
	Width   float64 `yaml:"width"`
	Height  float64 `yaml:"height"`
	OriginU float64 `yaml:"origin_u"`
	OriginV float64 `yaml:"origin_v"`
}

// SpaceWallRegion assigns play's wall faces in a grid rectangle on one level.
// Level is one-based; directional overrides take precedence over Material.
// Regions cannot overlap on the same level. Floors and collision are unchanged.
type SpaceWallRegion struct {
	Level    int    `yaml:"level"`
	X        int    `yaml:"x"`
	Y        int    `yaml:"y"`
	Width    int    `yaml:"width"`
	Depth    int    `yaml:"depth"`
	Material string `yaml:"material"`
	North    string `yaml:"north"`
	East     string `yaml:"east"`
	South    string `yaml:"south"`
	West     string `yaml:"west"`
}

// SpaceRoof is a closed, nonwalkable building roof for dungeon collision and
// play rendering. Base offsets the one-based level's floor in world units.
// Facing is the uphill direction; a gable also rises from the opposite edge.
// X/Y/Width/Depth include eaves. Thickness is vertical, not slope-normal.
type SpaceRoof struct {
	// Filled seals the volume down to a horizontal base underside, closing gable
	// ends. False keeps a constant vertical thickness beneath the inclined top.
	Filled    bool    `yaml:"filled"`
	Shape     string  `yaml:"shape"`
	Level     int     `yaml:"level"`
	X         float64 `yaml:"x"`
	Y         float64 `yaml:"y"`
	Width     float64 `yaml:"width"`
	Depth     float64 `yaml:"depth"`
	Base      float64 `yaml:"base"`
	Rise      float64 `yaml:"rise"`
	Thickness float64 `yaml:"thickness"`
	Facing    string  `yaml:"facing"`
	Material  string  `yaml:"material"`
	Underside string  `yaml:"underside"`
}

// SpaceRoofPart is a compiled closed inclined slab shared by play and dungeon.
// Its top is A*x+B*z+C inside Bounds [minX,minZ,maxX,maxZ]; its bottom is
// Thickness below, or Base-Thickness when Filled. Roof identifies the authored material. Returned parts are
// immutable for the lifetime of the loaded DB.
type SpaceRoofPart struct {
	Filled             bool
	Base               float64
	Bounds             [4]float64
	A, B, C, Thickness float64
	Roof               int
}

// RoofParts exposes immutable bounded geometry to play's cached GPU data.
func (s *Space) RoofParts() []SpaceRoofPart { return s.roofParts }

// RoofCandidates returns immutable part indices intersecting a horizontal cell.
// Dungeon and play use this index instead of scanning all authored roofs.
func (s *Space) RoofCandidates(x, z int) []int { return s.roofCells[[2]int{x, z}] }

func finiteBuilding(v float64) bool { return !math.IsNaN(v) && !math.IsInf(v, 0) }

func (s *Space) parseBuildings() {
	for name, m := range s.Materials {
		if m.Width == 0 {
			m.Width = 1
		}
		if m.Height == 0 {
			m.Height = 1
		}
		s.Materials[name] = m
	}
	s.roofParts = nil
	s.roofCells = nil
	for i, r := range s.Roofs[:min(len(s.Roofs), 32)] {
		if !finiteBuilding(r.X+r.Y+r.Width+r.Depth+r.Base+r.Rise+r.Thickness) || r.Width <= 0 || r.Depth <= 0 || r.X < 0 || r.Y < 0 || r.X+r.Width > 64 || r.Y+r.Depth > 64 {
			continue
		}
		p := SpaceRoofPart{Bounds: [4]float64{r.X, r.Y, r.X + r.Width, r.Y + r.Depth}, C: float64(r.Level-1)*SpaceStoryHeight + r.Base, Thickness: r.Thickness, Roof: i}
		p.Filled, p.Base = r.Filled, p.C
		axis := 0
		if r.Facing == "north" || r.Facing == "south" {
			axis = 1
		}
		low, high := p.Bounds[axis], p.Bounds[axis+2]
		parts := 1
		if r.Shape == "gable" {
			parts = 2
		}
		for half := range parts {
			q := p
			slope := 0.0
			if r.Shape != "flat" {
				slope = r.Rise / (high - low)
			}
			if r.Shape == "gable" {
				slope *= 2
				if half == 0 {
					q.Bounds[axis+2] = (low + high) / 2
				} else {
					q.Bounds[axis] = (low + high) / 2
					slope = -slope
				}
			} else if r.Facing == "north" || r.Facing == "west" {
				slope = -slope
			}
			if axis == 0 {
				q.A = slope
			} else {
				q.B = slope
			}
			if slope >= 0 {
				q.C -= slope * low
			} else {
				q.C -= slope * high
			}
			id := len(s.roofParts)
			s.roofParts = append(s.roofParts, q)
			if s.roofCells == nil {
				s.roofCells = map[[2]int][]int{}
			}
			for z := int(math.Floor(q.Bounds[1])); z < int(math.Ceil(q.Bounds[3])); z++ {
				for x := int(math.Floor(q.Bounds[0])); x < int(math.Ceil(q.Bounds[2])); x++ {
					key := [2]int{x, z}
					s.roofCells[key] = append(s.roofCells[key], id)
				}
			}
		}
	}
}

// RoofBlocks tests a grounded circular footprint and body height against nearby
// roofs. It is consumed by both grid steps and the continuous dungeon controller.
func (s *Space) RoofBlocks(x, z, radius, feet, head float64) bool {
	if len(s.roofParts) == 0 {
		return false
	}
	for iz := int(math.Floor(z - radius)); iz <= int(math.Floor(z+radius)); iz++ {
		for ix := int(math.Floor(x - radius)); ix <= int(math.Floor(x+radius)); ix++ {
			for _, id := range s.RoofCandidates(ix, iz) {
				p := s.roofParts[id]
				nearX := max(p.Bounds[0], min(p.Bounds[2], x))
				nearZ := max(p.Bounds[1], min(p.Bounds[3], z))
				if math.Hypot(nearX-x, nearZ-z) >= radius-1e-9 {
					continue
				}
				lo, hi := p.C, p.C
				if p.A != 0 {
					extent := math.Sqrt(max(0, radius*radius-(nearZ-z)*(nearZ-z)))
					lo += p.A * max(p.Bounds[0], x-extent)
					hi += p.A * min(p.Bounds[2], x+extent)
				} else if p.B != 0 {
					extent := math.Sqrt(max(0, radius*radius-(nearX-x)*(nearX-x)))
					lo += p.B * max(p.Bounds[1], z-extent)
					hi += p.B * min(p.Bounds[3], z+extent)
				}
				if lo > hi {
					lo, hi = hi, lo
				}
				if p.Filled {
					lo = p.Base
				}
				if feet < hi-1e-8 && head > lo-p.Thickness+1e-8 {
					return true
				}
			}
		}
	}
	return false
}

// RoofRay returns the nearest closed-roof intersection within limit. Dungeon
// interaction uses the same compiled planes as play's per-pixel occlusion.
// Direction is a unit vector; distances and coordinates use world units.
func (s *Space) RoofRay(origin, direction [3]float64, limit float64) (float64, bool) {
	if len(s.roofParts) == 0 {
		return 0, false
	}
	best := limit + 1
	// A short interaction ray crosses at most four horizontal cells. Enumerating
	// its bounded rectangle also supports vertical rays without DDA singularities.
	minX, maxX := origin[0], origin[0]+direction[0]*limit
	minZ, maxZ := origin[2], origin[2]+direction[2]*limit
	if minX > maxX {
		minX, maxX = maxX, minX
	}
	if minZ > maxZ {
		minZ, maxZ = maxZ, minZ
	}
	for z := max(0, int(math.Floor(minZ))); z <= min(63, int(math.Floor(maxZ))); z++ {
		for x := max(0, int(math.Floor(minX))); x <= min(63, int(math.Floor(maxX))); x++ {
			for _, id := range s.RoofCandidates(x, z) {
				p := s.roofParts[id]
				lo, hi := 0.0, limit
				clip := func(value, velocity, lower, upper float64) bool {
					if math.Abs(velocity) < 1e-12 {
						return value >= lower && value <= upper
					}
					a, b := (lower-value)/velocity, (upper-value)/velocity
					if a > b {
						a, b = b, a
					}
					lo = max(lo, a)
					hi = min(hi, b)
					return lo <= hi+1e-10
				}
				lower := -p.Thickness
				if p.Filled {
					lower = -10000
				}
				if clip(origin[0], direction[0], p.Bounds[0], p.Bounds[2]) && clip(origin[2], direction[2], p.Bounds[1], p.Bounds[3]) && clip(origin[1]-p.A*origin[0]-p.B*origin[2]-p.C, direction[1]-p.A*direction[0]-p.B*direction[2], lower, 0) && (!p.Filled || clip(origin[1], direction[1], p.Base-p.Thickness, 10000)) {
					best = min(best, lo)
				}
			}
		}
	}
	return best, best <= limit
}

func (c *checker) spaceBuildings(w string, d *Dungeon, width, height int) {
	s := d.Space
	if len(s.Materials) > 32 || len(s.WallRegions) > 128 || len(s.Roofs) > 32 {
		c.errorf(diag.OutOfRange, w, "want at most 32 materials, 128 wall regions and 32 roofs")
	}
	material := func(where, name string) {
		if _, ok := s.Materials[name]; name != "" && !ok {
			c.errorf(diag.UnknownReference, where, "unknown building material %q", name)
		}
	}
	for name, m := range s.Materials {
		where := w + ".materials." + name
		if !declaredName.MatchString(name) {
			c.errorf(diag.NotAWord, where, "material name must use lowercase letters, digits and underscore, starting with a letter")
		}
		c.picture(where+".picture", m.Picture)
		if !finiteBuilding(m.Width+m.Height+m.OriginU+m.OriginV) || m.Width < .125 || m.Height < .125 || m.Width > 128 || m.Height > 128 || math.Abs(m.OriginU) > 128 || math.Abs(m.OriginV) > 128 {
			c.errorf(diag.OutOfRange, where, "want finite material dimensions in [0.125,128] and origins in [-128,128]")
		}
		if m.Color.Valid && m.Color.A != 255 {
			c.errorf(diag.Shape, where, "building material colors must be opaque")
		}
	}
	occupied := map[[3]int]bool{}
	for i, r := range s.WallRegions {
		where := fmt.Sprintf("%s.wall_regions[%d]", w, i)
		for _, name := range []string{r.Material, r.North, r.East, r.South, r.West} {
			material(where, name)
		}
		if r.Level < 1 || r.Level > len(s.Levels) || r.X < 0 || r.Y < 0 || r.Width < 1 || r.Depth < 1 || r.Width > width || r.Depth > height || r.X > width-r.Width || r.Y > height-r.Depth {
			c.errorf(diag.OutOfRange, where, "wall region level and bounds must fit the map")
			continue
		}
		overlap := false
		for z := r.Y; z < r.Y+r.Depth; z++ {
			for x := r.X; x < r.X+r.Width; x++ {
				key := [3]int{r.Level, x, z}
				overlap = overlap || occupied[key]
				occupied[key] = true
			}
		}
		if overlap {
			c.errorf(diag.Shape, where, "wall regions on the same level cannot overlap")
		}
	}
	for i, r := range s.Roofs {
		where := fmt.Sprintf("%s.roofs[%d]", w, i)
		material(where+".material", r.Material)
		material(where+".underside", r.Underside)
		if r.Shape != "gable" && r.Shape != "shed" && r.Shape != "flat" {
			c.errorf(diag.Shape, where+".shape", "roof shape must be gable, shed or flat")
		}
		if !finiteBuilding(r.X + r.Y + r.Width + r.Depth + r.Base + r.Rise + r.Thickness) {
			c.errorf(diag.OutOfRange, where, "roof dimensions must be finite")
			continue
		}
		if r.Level < 1 || r.Level > len(s.Levels)+1 {
			c.errorf(diag.OutOfRange, where+".level", "roof level must anchor to an existing level or the top of its last story")
		}
		if r.Width < .125 || r.Depth < .125 || r.Width > 64 || r.Depth > 64 {
			c.errorf(diag.OutOfRange, where, "roof dimensions must be in [0.125,64]")
		}
		if r.X < 0 || r.Y < 0 || r.X+r.Width > float64(width) || r.Y+r.Depth > float64(height) {
			c.errorf(diag.OutOfRange, where, "roof bounds including eaves must fit the map")
		}
		if r.Thickness < .01 || r.Thickness > .5 {
			c.errorf(diag.OutOfRange, where+".thickness", "roof thickness must be in [0.01,0.5]")
		}
		if r.Base < 0 || r.Base > 1.5 || r.Rise < 0 || r.Rise > 2 || (r.Shape == "flat" && r.Rise != 0) || (r.Shape != "flat" && r.Rise == 0) {
			c.errorf(diag.OutOfRange, where, "roof base must be in [0,1.5], rise in (0,2], or zero for flat")
		}
		if r.Facing == "" || FacingIndex(r.Facing) < 0 {
			c.errorf(diag.Shape, where+".facing", "roof facing must be north, east, south or west")
		}
	}
	for _, ids := range s.roofCells {
		if len(ids) > 4 {
			c.errorf(diag.OutOfRange, w+".roofs", "at most four roof parts may overlap a horizontal cell")
			break
		}
	}
	for i, p := range s.roofParts {
		for _, q := range s.roofParts[:i] {
			if p.Roof != q.Roof && roofPartsOverlap(p, q) {
				c.errorf(diag.Shape, w+".roofs", "roof volumes %d and %d overlap", q.Roof, p.Roof)
			}
		}
	}
	if len(s.roofParts) == 0 {
		return
	}
	// Roof/wall junctions are allowed, but a roof may not occupy the body or
	// stair headroom of an authored walking cell. Adjacent elevated areas use
	// runtime footprint collision; roofs never become walkable support.
	for level := range s.Levels {
		for z := range height {
			for x := range width {
				ch := SpaceCell(s, level, x, z)
				walk := ch == CellFloor
				if t := d.Tile(ch); t != nil {
					walk = t.Look != "wall" && t.Look != "door" && !t.Solid.Set()
				}
				if !walk {
					continue
				}
				feet := SpaceHeight(s, level, x, z)
				if s.RoofBlocks(float64(x)+.5, float64(z)+.5, .18, feet, feet+.9) {
					c.errorf(diag.Shape, w+".roofs", "roof intersects walking/stair headroom at (%d,%d) on level %d", x, z, level+1)
					return
				}
			}
		}
	}
}

// Project two roof volumes onto their common footprint, then clip by both
// top-above-bottom inequalities. A nonempty area is an actual solid overlap,
// not merely overlapping bounding boxes or a shared eave edge.
func roofPartsOverlap(p, q SpaceRoofPart) bool {
	x0, z0 := max(p.Bounds[0], q.Bounds[0]), max(p.Bounds[1], q.Bounds[1])
	x1, z1 := min(p.Bounds[2], q.Bounds[2]), min(p.Bounds[3], q.Bounds[3])
	if x0 >= x1 || z0 >= z1 {
		return false
	}
	polygon := [][2]float64{{x0, z0}, {x1, z0}, {x1, z1}, {x0, z1}}
	bottom := func(r SpaceRoofPart) [3]float64 {
		if r.Filled {
			return [3]float64{0, 0, r.Base - r.Thickness}
		}
		return [3]float64{r.A, r.B, r.C - r.Thickness}
	}
	pb, qb := bottom(p), bottom(q)
	for _, plane := range [][3]float64{{p.A - qb[0], p.B - qb[1], p.C - qb[2] - 1e-8}, {q.A - pb[0], q.B - pb[1], q.C - pb[2] - 1e-8}} {
		out := make([][2]float64, 0, len(polygon)+2)
		value := func(v [2]float64) float64 { return plane[0]*v[0] + plane[1]*v[1] + plane[2] }
		for i, a := range polygon {
			b := polygon[(i+1)%len(polygon)]
			va, vb := value(a), value(b)
			if va >= 0 {
				out = append(out, a)
			}
			if (va >= 0) != (vb >= 0) {
				f := va / (va - vb)
				out = append(out, [2]float64{a[0] + (b[0]-a[0])*f, a[1] + (b[1]-a[1])*f})
			}
		}
		polygon = out
		if len(polygon) < 3 {
			return false
		}
	}
	area := 0.0
	for i, a := range polygon {
		b := polygon[(i+1)%len(polygon)]
		area += a[0]*b[1] - a[1]*b[0]
	}
	return math.Abs(area) > 1e-10
}
