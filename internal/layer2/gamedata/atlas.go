package gamedata

import (
	"fmt"
	"math"
	"slices"

	"github.com/nao1215/gozen/internal/diag"
)

// Atlas is a location shown as a map: one picture with points joined by roads (map:). A
// point with to is the way into another location; the party walks from point to point
// along the open roads.
type Atlas struct {
	// Activation is travel (default), or instant for an illustrated action menu.
	Activation string `yaml:"activation"`
	// SelectionPanel is the play renderer's fixed selected-point advice panel.
	SelectionPanel *AtlasPanel `yaml:"selection_panel"`
	// Camera enables presentation-only pan and zoom; nil keeps the fitted view.
	Camera *AtlasCamera `yaml:"camera"`
	// SafeArea keeps the picture inside the screen less its edges [top, right, bottom,
	// left], fractions of the screen, where the windows over the map stand: the picture is
	// fitted to the rest, and the camera brings any part of it there. Empty: the whole screen.
	SafeArea []float64 `yaml:"safe_area"`
	// TravelSpeed is the map renderer's token speed in picture widths per second.
	// Zero selects 0.12; explicit speeds must be finite and in [0.01, 2].
	TravelSpeed float64 `yaml:"travel_speed"`
	Picture     string  `yaml:"picture"`
	Token       *Token  `yaml:"token"`
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
	// Talk is how the map shows under a conversation that is not its own question (a
	// region's orders): as it is (the default), dim (darkened) or hide (darkened, without
	// the marks, the flags and the name plates).
	Talk string `yaml:"talk"`
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

// AtlasPanel places selected-point advice in screen space for the play renderer.
// Rect is [x,y,width,height], as fractions of the screen; omission uses the right side.
type AtlasPanel struct {
	Rect     []float64 `yaml:"rect"`
	Portrait string    `yaml:"portrait"` // Default face picture, relative to assets/.
}

// MapAdvice is a first-matching advice variant for the selected point. The play
// renderer evaluates When and renders Heading/Text without advancing gameplay RNG.
type MapAdvice struct {
	When     Expr   `yaml:"when"`
	Heading  string `yaml:"heading"`
	Text     string `yaml:"text"`
	Portrait string `yaml:"portrait"` // Replaces the panel's default face picture.
}

// AtlasCamera configures the play renderer's navigable map. Zoom is relative to
// the fitted overview; camera position is transient and never part of game saves.
type AtlasCamera struct {
	Zoom    float64 `yaml:"zoom"`     // Initial/current-position zoom; zero selects min(2, max_zoom).
	MaxZoom float64 `yaml:"max_zoom"` // Zoom limit in [1,64]; zero selects 8.
	// Buttons is how the camera's on-screen buttons show: shown (the default), faint
	// (see-through until the pointer is over them) or hidden (keys, wheel and pad only).
	Buttons string `yaml:"buttons"`
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
	// Hide lets the checker and atlas consumers omit a closed point entirely.
	// Empty preserves its usual appearance; closed hides it while When is false.
	Hide   string      `yaml:"hide"`
	Advice []MapAdvice `yaml:"advice"`
	At     Point       `yaml:"at"`
	To     string      `yaml:"to"`    // the location it leads into
	Label  string      `yaml:"label"` // the name of to when empty
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

// MaxRoadSteps bounds encounter work performed by crossing one road.
const MaxRoadSteps = 10000

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
	if math.IsNaN(a.TravelSpeed) || math.IsInf(a.TravelSpeed, 0) || a.TravelSpeed != 0 && (a.TravelSpeed < 0.01 || a.TravelSpeed > 2) {
		c.errorf(diag.OutOfRange, w+".travel_speed", "want 0 (default 0.12) or a finite speed in [0.01, 2] picture widths per second")
	}
	switch a.Activation {
	case "", "travel", "instant":
	default:
		c.errorf(diag.NotAWord, w+".activation", "want travel or instant")
	}
	if panel := a.SelectionPanel; panel != nil {
		c.file(w+".selection_panel.portrait", panel.Portrait)
		if len(panel.Rect) > 0 {
			valid := len(panel.Rect) == 4
			for _, v := range panel.Rect {
				valid = valid && !math.IsNaN(v) && !math.IsInf(v, 0) && v >= 0 && v <= 1
			}
			if valid {
				valid = panel.Rect[2] >= .2 && panel.Rect[3] >= .25 && panel.Rect[0]+panel.Rect[2] <= 1 && panel.Rect[1]+panel.Rect[3] <= 1
			}
			if !valid {
				c.errorf(diag.OutOfRange, w+".selection_panel.rect", "want finite [x,y,width,height] within the screen, width >= 0.2 and height >= 0.25")
			}
		}
	}
	if len(a.SafeArea) > 0 {
		valid := len(a.SafeArea) == 4
		for _, v := range a.SafeArea {
			valid = valid && !math.IsNaN(v) && v >= 0 && v <= 0.45
		}
		if valid {
			valid = a.SafeArea[0]+a.SafeArea[2] <= 0.8 && a.SafeArea[1]+a.SafeArea[3] <= 0.8
		}
		if !valid {
			c.errorf(diag.OutOfRange, w+".safe_area", "want [top, right, bottom, left], each a fraction of the screen in [0, 0.45], leaving at least 0.2 of it each way")
		}
	}
	if camera := a.Camera; camera != nil {
		limit := camera.MaxZoom
		if limit == 0 {
			limit = 8
		}
		if math.IsNaN(limit) || math.IsInf(limit, 0) || limit < 1 || limit > 64 {
			c.errorf(diag.OutOfRange, w+".camera.max_zoom", "want 0 (default 8) or a finite zoom in [1, 64]")
		}
		switch camera.Buttons {
		case "", "shown", "faint", "hidden":
		default:
			c.errorf(diag.NotAWord, w+".camera.buttons", "want shown, faint or hidden, not %q", camera.Buttons)
		}
		if z := camera.Zoom; math.IsNaN(z) || math.IsInf(z, 0) || z != 0 && (z < 1 || z > limit) {
			c.errorf(diag.OutOfRange, w+".camera.zoom", "want 0 (default 2, capped at max_zoom) or a finite zoom in [1, max_zoom]")
		}
	}
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
	switch a.Talk {
	case "", "dim", "hide":
	default:
		c.errorf(diag.NotAWord, w+".talk", "want dim or hide, not %q", a.Talk)
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
		if p.Hide != "" && p.Hide != "closed" {
			c.errorf(diag.NotAWord, ww+".hide", "want closed or an empty value")
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
		for i, advice := range p.Advice {
			where := fmt.Sprintf("%s.advice[%d]", ww, i)
			c.expr(where+".when", advice.When, FieldNames)
			c.text(where+".heading", advice.Heading, FieldNames)
			c.text(where+".text", advice.Text, FieldNames)
			c.file(where+".portrait", advice.Portrait)
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
		if r.Steps < 0 || r.Steps > MaxRoadSteps {
			c.errorf(diag.OutOfRange, ww+".steps", "want steps in [0, %d] (0 means 1), got %d", MaxRoadSteps, r.Steps)
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
	if len(a.Points) > 1 && a.Activation != "instant" {
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
