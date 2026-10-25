package gamedata

import (
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/nao1215/gozen/internal/diag"
)

// Space is an opt-in bounded stack of maps. Blank cells are open air; ordinary
// floors and tiles provide surfaces. Levels are ordered from lowest to highest.
type Space struct {
	// StepTicks and TurnTicks are the presentation's independent camera durations
	// at 60 updates per second. Zero selects 18; explicit durations are [1, 120].
	StepTicks int `yaml:"step_ticks"`
	TurnTicks int `yaml:"turn_ticks"`
	// Sky is an optional horizontally wrapping upper-hemisphere panorama.
	// Its top is the zenith and its bottom is the horizon; tint and brightness apply.
	Sky    Picture      `yaml:"sky"`
	Levels []string     `yaml:"levels"`
	Stairs []SpaceStair `yaml:"stairs"`
	grids  [][][]rune   `yaml:"-"`
}

// SpaceStair joins a lower-level cell to a cell Length steps along Facing on
// the next level. Level is one-based in YAML. Both directions are traversable.
type SpaceStair struct {
	Level  int    `yaml:"level"`
	X      int    `yaml:"x"`
	Y      int    `yaml:"y"`
	Facing string `yaml:"facing"`
	Length int    `yaml:"length"`
}

const (
	MaxSpaceLevels   = 8
	MaxSpaceSide     = 64
	SpaceStoryHeight = 1.5
)

func (s *Space) parse() {
	s.grids = nil
	for _, m := range s.Levels[:min(len(s.Levels), MaxSpaceLevels)] {
		// Keep allocation bounded even before validation reports oversized input.
		rows := strings.SplitN(strings.TrimRight(m, "\n"), "\n", MaxSpaceSide+1)
		grid := make([][]rune, 0, min(len(rows), MaxSpaceSide))
		for _, row := range rows[:min(len(rows), MaxSpaceSide)] {
			r := make([]rune, 0, min(len(row), MaxSpaceSide))
			for _, ch := range row {
				if len(r) == MaxSpaceSide {
					break
				}
				if ch == '_' {
					ch = CellNone
				}
				r = append(r, ch)
			}
			grid = append(grid, r)
		}
		s.grids = append(s.grids, grid)
	}
}

// SpaceCell reads one authored cell, returning open air outside the volume.
func SpaceCell(s *Space, level, x, y int) rune {
	if level < 0 || level >= len(s.grids) || y < 0 || y >= len(s.grids[level]) || x < 0 || x >= len(s.grids[level][y]) {
		return CellNone
	}
	return s.grids[level][y][x]
}

// StairDelta is the horizontal unit vector of the ascending stair run.
func StairDelta(s SpaceStair) (int, int) {
	switch FacingIndex(s.Facing) {
	case 0:
		return 0, -1
	case 1:
		return 1, 0
	case 2:
		return 0, 1
	case 3:
		return -1, 0
	}
	return 0, 0
}

// StairOffset is a cell's distance along this stair; false means it is off the run.
func StairOffset(s SpaceStair, x, y int) (int, bool) {
	dx, dy := StairDelta(s)
	n := (x-s.X)*dx + (y-s.Y)*dy
	return n, n >= 0 && n <= s.Length && x == s.X+n*dx && y == s.Y+n*dy
}

// SpaceHeight is the smoothed standing height at a walkable cell center, in horizontal cell units.
func SpaceHeight(s *Space, level, x, y int) float64 {
	for _, stair := range s.Stairs {
		if stair.Level-1 != level {
			continue
		}
		if n, ok := StairOffset(stair, x, y); ok && stair.Length > 0 && n < stair.Length {
			return (float64(level) + (float64(n)+0.5)/float64(stair.Length)) * SpaceStoryHeight
		}
	}
	return float64(level) * SpaceStoryHeight
}

// DungeonLevel shares immutable definitions while selecting a level's grid.
func DungeonLevel(d *Dungeon, level int) *Dungeon {
	if d == nil || d.Space == nil {
		return d
	}
	c := *d
	c.Grid = nil
	if level >= 0 && level < len(d.Space.grids) {
		c.Grid = d.Space.grids[level]
	}
	return &c
}

func (c *checker) space(w string, d *Dungeon, used map[rune]bool) {
	s := d.Space
	w += ".space"
	for name, ticks := range map[string]int{"step_ticks": s.StepTicks, "turn_ticks": s.TurnTicks} {
		if ticks < 0 || ticks > 120 {
			c.errorf(diag.OutOfRange, w+"."+name, "want 0 (default 18) or ticks in [1, 120]")
		}
	}
	c.needsVersion(w, "stacked spaces", SchemaVersion8)
	c.picture(w+".sky", s.Sky)
	if d.Map != "" || d.Generate != nil || len(d.Roamers) > 0 {
		c.errorf(diag.Shape, w, "space cannot combine with map, generate or roamers")
	}
	if len(s.Levels) < 1 || len(s.Levels) > MaxSpaceLevels {
		c.errorf(diag.OutOfRange, w+".levels", "want 1 to %d levels", MaxSpaceLevels)
		return
	}
	width, height := 0, 0
	for level, m := range s.Levels {
		rows := strings.SplitN(strings.TrimRight(m, "\n"), "\n", MaxSpaceSide+1)
		lw, lh := 0, len(rows)
		for _, row := range rows {
			lw = max(lw, utf8.RuneCountInString(row))
		}
		if lw < 3 || lh < 3 || lw > MaxSpaceSide || lh > MaxSpaceSide {
			c.errorf(diag.OutOfRange, w+".levels", "each level must be 3 to %d cells wide and high", MaxSpaceSide)
			return
		}
		if level == 0 {
			width, height = lw, lh
		}
		if lw != width || lh != height {
			c.errorf(diag.Shape, w+".levels", "levels must have equal dimensions")
		}
		for _, row := range rows {
			if utf8.RuneCountInString(row) != width {
				c.errorf(diag.Shape, w+".levels", "rows must have equal width; use spaces for open air")
			}
			for _, r := range row {
				used[r] = true
				if r != '_' && !builtin(r) && d.Tile(r) == nil {
					c.errorf(diag.NotAWord, w+".levels", "unknown cell %q", string(r))
				}
			}
		}
	}
	if _, ok := DungeonLevel(d, 0).StartChar(); !ok {
		c.errorf(diag.Shape, w, "the lowest level needs a start tile")
	}
	if d.Depth > 16 {
		c.errorf(diag.OutOfRange, w, "space view depth must be at most 16")
	}
	// Unsupported terrain effects must be rejected, never half-applied by the new movement.
	if d.Theme != "" || d.Door.File != "" || len(d.Looks) > 0 {
		c.errorf(diag.Shape, w, "space does not support themes, door pictures or conditional looks")
	}
	for name, t := range d.Tiles {
		if name == "_" || t.Picture.File != "" {
			c.errorf(diag.Shape, w+".tiles."+name, "space reserves underscore for open air and does not support tile pictures")
		}
		if t.Slide != "" || t.Spin != "" {
			c.errorf(diag.Shape, w+".tiles."+name, "space tiles cannot slide or spin")
		}
	}
	if len(s.Stairs) > 128 {
		c.errorf(diag.OutOfRange, w+".stairs", "want at most 128 stair runs")
		return
	}
	occupied := map[[3]int]bool{}
	walkable := func(level, x, y int) bool {
		r := SpaceCell(s, level, x, y)
		if r == CellFloor {
			return true
		}
		t := d.Tile(r)
		return t != nil && t.Look != "wall" && t.Look != "door" && !t.Solid.Set() && len(t.Do) == 0 && !t.When.Set()
	}
	for i, stair := range s.Stairs {
		sw := fmt.Sprintf("%s.stairs[%d]", w, i)
		if stair.Level < 1 || stair.Level >= len(s.Levels) || stair.Length < 3 || stair.Length > 8 || FacingIndex(stair.Facing) < 0 || stair.Facing == "" {
			c.errorf(diag.OutOfRange, sw, "want adjacent levels, length 3 to 8 and a facing")
			continue
		}
		dx, dy := StairDelta(stair)
		for n := 0; n <= stair.Length; n++ {
			x, y := stair.X+n*dx, stair.Y+n*dy
			level := stair.Level - 1
			if n == stair.Length {
				level++
			}
			if !walkable(level, x, y) {
				c.errorf(diag.Shape, sw, "stair cell (%d,%d) on level %d must be an unconditional floor without commands", x, y, level+1)
			}
			if n < stair.Length && SpaceCell(s, level+1, x, y) != CellNone {
				c.errorf(diag.Shape, sw, "the space above the run must be open for headroom")
			}
			key := [3]int{level, x, y}
			if occupied[key] {
				c.errorf(diag.Shape, sw, "stair runs cannot overlap")
			}
			occupied[key] = true
		}
	}
}
