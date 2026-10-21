package gamedata

import (
	"fmt"
	"slices"

	"github.com/nao1215/gozen/internal/diag"
)

// Atlas is a location shown as a map: one picture with points joined by roads (map:). A
// point with to is the way into another location; the party walks from point to point
// along the open roads.
type Atlas struct {
	Picture string `yaml:"picture"`
	Token   *Token `yaml:"token"`
	// Marker is the picture standing on each point (a pin or a flag; a small pin drawn by
	// the screen when not given) and its height. A point's own marker replaces it.
	Marker *Token `yaml:"marker"`
	// DrawRoads draws the roads over the picture. The roads are meant to be painted in the
	// picture; their lines are what the token walks along, not drawn unless this is true.
	DrawRoads bool `yaml:"draw_roads"`
	// RegionRoads and RegionBorders are for a campaign's map (a map whose points are
	// regions). RegionRoads is when the arrows to the regions the one chosen can march on
	// are drawn, and RegionBorders when the outlines of the regions are: selected (the
	// default) only around the chosen region, always for every region (the old look; for
	// roads, the lines of all the roads). draw_roads is not read on a campaign's map.
	RegionRoads   string `yaml:"region_roads"`
	RegionBorders string `yaml:"region_borders"`
	// LabelSize is the text height of the points' name plates, a fraction of the screen's
	// height (0.03 when 0).
	LabelSize float64 `yaml:"label_size"`
	// Unknown is how a point not yet visited shows: hidden (until the roads reach it), dim,
	// mark (a "?" plate, the default) or named (as a visited one: a town whose places are
	// known).
	Unknown string               `yaml:"unknown"`
	Points  map[string]*MapPoint `yaml:"points"`
	Roads   []Road               `yaml:"roads"`

	pointOrder []string // the points as written
}

// Token is the piece that stands for the party on a map: its picture, else the leader's
// round face (as in the balloons), else with doll the leader's doll walking. Written as a
// map, or as the word doll.
type Token struct {
	Picture string  `yaml:"picture"` // the leader's face (or doll) when empty
	Height  float64 `yaml:"height"`  // of the screen's height
	Doll    bool    `yaml:"doll"`    // the leader's doll instead of the face (a token only)
}

// UnmarshalYAML reads a map, or the word doll.
func (t *Token) UnmarshalYAML(unmarshal func(any) error) error {
	var s string
	if err := unmarshal(&s); err == nil {
		if s != "doll" {
			return fmt.Errorf("want a map or doll, not %q", s)
		}
		t.Doll = true
		return nil
	}
	type plain Token
	return unmarshal((*plain)(t))
}

// MapPoint is a point of a map.
type MapPoint struct {
	At    Point  `yaml:"at"`
	To    string `yaml:"to"`    // the location it leads into
	Label string `yaml:"label"` // the name of to when empty
	// When must hold for the point to be open (the roads through a closed point are closed).
	When Expr `yaml:"when"`
	// Locked is what is said on choosing the point while it is closed.
	Locked string `yaml:"locked"`
	Start  bool   `yaml:"start"` // where one stands on a first coming
	// Do runs on arriving at the point (once: the first time only).
	Do   []Command `yaml:"do"`
	Once bool      `yaml:"once"`
	// AtPoint is, when to is a map, the point of that map to stand on.
	AtPoint string `yaml:"at_point"`
	// Marker is the point's own marker picture (instead of the map's).
	Marker string `yaml:"marker"`

	hasAt bool // at was written (a point without it would stand at the corner)
}

// UnmarshalYAML reads the point and notes whether it has at, which check requires.
func (p *MapPoint) UnmarshalYAML(unmarshal func(any) error) error {
	type plain MapPoint
	if err := unmarshal((*plain)(p)); err != nil {
		return err
	}
	p.hasAt = hasKey(unmarshal, "at")
	return nil
}

// Road joins two points of a map (both ways).
type Road struct {
	Between    []string  `yaml:"between"`
	Via        []Point   `yaml:"via"` // the points the road bends at
	When       Expr      `yaml:"when"`
	Steps      int       `yaml:"steps"` // the steps it takes (1 when 0)
	Encounters Encounter `yaml:"encounters"`
	Do         []Command `yaml:"do"` // run on going along it
}

// StepsOrOne is the steps of the road (1 when not given).
func (r *Road) StepsOrOne() int { return max(1, r.Steps) }

// PointIDs are the map's points in the order they were written (by name when the map
// was not read from YAML).
func (a *Atlas) PointIDs() []string { return orderedKeys(a.Points, a.pointOrder) }

// StartPoint is the point with start, else the first.
func (a *Atlas) StartPoint() string {
	ids := a.PointIDs()
	for _, id := range ids {
		if a.Points[id].Start {
			return id
		}
	}
	if len(ids) > 0 {
		return ids[0]
	}
	return ""
}

// PointTo is the first point leading into loc ("" when none).
func (a *Atlas) PointTo(loc string) string {
	for _, id := range a.PointIDs() {
		if p := a.Points[id]; p.To == loc && loc != "" {
			return id
		}
	}
	return ""
}

// atlas checks a location's map.
func (c *checker) atlas(w, id string, l *Location) {
	a := l.Map
	w += ".map"
	c.needsVersion(w, "map", SchemaVersion3)
	if a.Picture == "" {
		c.errorf(diag.Shape, w+".picture", "a map needs a picture")
	}
	c.file(w+".picture", a.Picture)
	size, known := c.pictureSize(a.Picture)
	for name, t := range map[string]*Token{"token": a.Token, "marker": a.Marker} {
		if t == nil {
			continue
		}
		c.file(w+"."+name+".picture", t.Picture)
		if name == "marker" && t.Doll {
			c.errorf(diag.Shape, w+".marker.doll", "a marker is not a doll: give it a picture")
		}
		if t.Height < 0 || t.Height > 1 {
			c.errorf(diag.OutOfRange, w+"."+name+".height", "want a fraction of the screen's height in [0, 1], got %g", t.Height)
		}
	}
	if a.LabelSize < 0 || a.LabelSize > 0.2 {
		c.errorf(diag.OutOfRange, w+".label_size", "want a fraction of the screen's height in [0, 0.2], got %g", a.LabelSize)
	}
	switch a.Unknown {
	case "", "hidden", "dim", "mark", "named":
	default:
		c.errorf(diag.NotAWord, w+".unknown", "want hidden, dim, mark or named, not %q", a.Unknown)
	}
	for name, v := range map[string]string{"region_roads": a.RegionRoads, "region_borders": a.RegionBorders} {
		switch v {
		case "", "selected", "always":
		default:
			c.errorf(diag.NotAWord, w+"."+name, "want selected or always, not %q", v)
		}
	}
	if len(a.Points) == 0 {
		c.errorf(diag.Shape, w+".points", "a map needs points")
	}
	starts := 0
	for _, pid := range a.PointIDs() {
		p := a.Points[pid]
		ww := w + ".points." + pid
		if p == nil || !p.hasAt {
			c.errorf(diag.Shape, ww, "a point needs at")
			continue
		}
		c.inPicture(ww+".at", p.At, size, known)
		if p.To != "" {
			to, ok := c.db.Locations[p.To]
			c.ref(ww+".to", "location", p.To, ok)
			if p.To == id {
				c.errorf(diag.Shape, ww+".to", "a point of a map leads into another location, not the map itself")
			}
			if ok && p.AtPoint != "" {
				if to.Map == nil {
					c.errorf(diag.UnknownReference, ww+".at_point", "%s is not a map (at_point is a point of a map)", p.To)
				} else if to.Map.Points[p.AtPoint] == nil {
					c.errorf(diag.UnknownReference, ww+".at_point", "%s has no point %q", p.To, p.AtPoint)
				}
			}
		} else if p.AtPoint != "" {
			c.errorf(diag.Shape, ww+".at_point", "at_point is for a point with to")
		}
		if p.Start {
			starts++
		}
		if p.Locked != "" && !p.When.Set() {
			c.errorf(diag.Shape, ww+".locked", "locked is said when the point is closed: it needs when")
		}
		if p.Once && len(p.Do) == 0 {
			c.errorf(diag.Shape, ww+".once", "once is for a point with do")
		}
		c.file(ww+".marker", p.Marker)
		c.text(ww+".label", p.Label, FieldNames)
		c.text(ww+".locked", p.Locked, FieldNames)
		c.expr(ww+".when", p.When, FieldNames)
		c.commands(ww+".do", p.Do, FieldNames)
	}
	if starts > 1 {
		c.errorf(diag.OutOfRange, w+".points", "%d points have start (one at most)", starts)
	}
	pairs := map[[2]string]bool{}
	roadsOf := map[string]int{}
	for i := range a.Roads {
		r := &a.Roads[i]
		ww := fmt.Sprintf("%s.roads[%d]", w, i)
		if len(r.Between) != 2 {
			c.errorf(diag.Shape, ww+".between", "a road is between two points: [a, b]")
			continue
		}
		for j, pid := range r.Between {
			if a.Points[pid] == nil {
				c.errorf(diag.UnknownReference, fmt.Sprintf("%s.between[%d]", ww, j), "unknown point %q", pid)
			}
			roadsOf[pid]++
		}
		if r.Between[0] == r.Between[1] {
			c.errorf(diag.Shape, ww+".between", "a road joins two different points")
		}
		key := [2]string{r.Between[0], r.Between[1]}
		if key[0] > key[1] {
			key[0], key[1] = key[1], key[0]
		}
		if pairs[key] {
			c.errorf(diag.Shape, ww+".between", "a second road between %s and %s", key[0], key[1])
		}
		pairs[key] = true
		for j, v := range r.Via {
			c.inPicture(fmt.Sprintf("%s.via[%d]", ww, j), v, size, known)
		}
		if r.Steps < 0 {
			c.errorf(diag.OutOfRange, ww+".steps", "want steps of 1 or more, got %d", r.Steps)
		}
		c.expr(ww+".when", r.When, FieldNames)
		c.commands(ww+".do", r.Do, FieldNames)
		c.expr(ww+".encounters.rate", r.Encounters.Rate, FieldNames)
		for _, tp := range r.Encounters.Troops {
			_, ok := c.db.Troops[tp.Troop]
			c.ref(ww+".encounters", "troop", tp.Troop, ok)
			c.expr(ww+".encounters.when", tp.When, FieldNames)
		}
	}
	if len(a.Points) > 1 {
		for _, pid := range a.PointIDs() {
			if roadsOf[pid] == 0 && a.Points[pid] != nil {
				c.errorf(diag.Shape, w+".points."+pid, "no road leads to the point")
			}
		}
	}
	// a warning: points that cannot be reached from the start even with every when holding
	if start := a.StartPoint(); start != "" {
		seen := map[string]bool{start: true}
		queue := []string{start}
		for len(queue) > 0 {
			at := queue[0]
			queue = queue[1:]
			for _, r := range a.Roads {
				if len(r.Between) != 2 {
					continue
				}
				for k, end := range r.Between {
					if end == at && !seen[r.Between[1-k]] && a.Points[r.Between[1-k]] != nil {
						seen[r.Between[1-k]] = true
						queue = append(queue, r.Between[1-k])
					}
				}
			}
		}
		for _, pid := range a.PointIDs() {
			if !seen[pid] && roadsOf[pid] > 0 {
				c.warnf(diag.Unreachable, w+".points."+pid, "the point cannot be reached from %s (even with every when holding)", start)
			}
		}
	}
	// a warning: a location the map leads into with no way back
	for _, pid := range a.PointIDs() {
		p := a.Points[pid]
		if p == nil || p.To == "" {
			continue
		}
		to := c.db.Locations[p.To]
		if to == nil || to.Map != nil || to.ResumeTo != "" {
			continue
		}
		back := slices.ContainsFunc(to.Exits, func(e Exit) bool { return e.To == id })
		if !back && to.Dungeon == nil {
			c.warnf(diag.Unreachable, w+".points."+pid+".to", "%s has no exit back to %s", p.To, id)
		}
	}
}
