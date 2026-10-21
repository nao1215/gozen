package gamedata

import (
	"bytes"
	"fmt"
	"image"
	_ "image/jpeg" // backgrounds and maps may be JPEG
	_ "image/png"  // depth maps are 16-bit PNG
	"io/fs"
	"math"
	"regexp"
	"slices"

	"github.com/nao1215/gozen/internal/diag"

	"github.com/goccy/go-yaml"

	"github.com/nao1215/gozen/internal/layer1/geom"
)

// Painted scenes and maps (ADR 0022): two more ways to show a location, beside the menu
// and the dungeon. The rules know only a point (State.Spot); where someone stands between
// points belongs to the screen and is never saved.

// SchemaVersion3 is the first schema version with map, stage, doll and bubbles.
const SchemaVersion3 = 3

// Point is a point of a picture, in its pixels: [x, y].
type Point [2]float64

// Pt is the point for the geometry.
func (p Point) Pt() geom.Pt { return geom.Pt{X: p[0], Y: p[1]} }

// Polygon is a closed polygon of a picture: [[x, y], ...].
type Polygon []Point

// Poly is the polygon for the geometry.
func (q Polygon) Poly() geom.Poly {
	out := make(geom.Poly, len(q))
	for i, p := range q {
		out[i] = p.Pt()
	}
	return out
}

// Stage is a location shown as a painted background seen from a fixed camera, with
// people as small dolls walking in it (stage:). The location's background is the
// painting; its people, actions and exits are the usual ones, given a place here.
type Stage struct {
	// Depth is a 16-bit gray picture of the background's nearness (0 far .. 65535 near),
	// as large as the background; without it nothing but fronts hides anyone.
	Depth string `yaml:"depth"`
	// Camera is how many degrees the camera looks down (the dolls are drawn so; 30 when 0).
	Camera float64 `yaml:"camera"`
	// DepthBias is how much nearer (0..1) the picture must be than the feet to hide them.
	DepthBias float64 `yaml:"depth_bias"`
	// Scale gives a person's height in pixels at two or more feet heights (a line through them).
	Scale  []ScaleMark `yaml:"scale"`
	Walk   []Polygon   `yaml:"walk"`   // where feet may go
	Blocks []Polygon   `yaml:"blocks"` // and may not
	Fronts []Front     `yaml:"fronts"`
	// Spots are the named places to stand: the rules' only positions (State.Spot).
	Spots  map[string]*Spot `yaml:"spots"`
	Exits  []StageExit      `yaml:"exits"`
	People []StagePerson    `yaml:"people"`
	Things []Thing          `yaml:"things"`

	spotOrder []string // the spots as written
}

// ScaleMark is a person's height in pixels with the feet at At.
type ScaleMark struct {
	At     Point   `yaml:"at"`
	Height float64 `yaml:"height"`
}

// Front is a part of the painting standing up from the ground at the line y = Base (a
// pillar, a stall): people whose feet are behind that line are hidden by it.
type Front struct {
	Area Polygon `yaml:"area"`
	Base float64 `yaml:"base"`
}

// Spot is a named place to stand in a stage.
type Spot struct {
	At     Point  `yaml:"at"`
	Facing string `yaml:"facing"`
	Start  bool   `yaml:"start"` // where one stands on coming in without a spot named
	Name   string `yaml:"name"`  // the label of its entry (when it has do)
	// Do runs on stepping on the spot (once: the first time only); a spot with do is an
	// entry of the location's menu too.
	Do   []Command `yaml:"do"`
	Once bool      `yaml:"once"`
}

// StageExit gives an exit of the location (exits[].to) an area: walking into it leaves.
type StageExit struct {
	To   string  `yaml:"to"`
	Area Polygon `yaml:"area"`
}

// StagePerson places someone of the location's people (people[].id) in the stage.
type StagePerson struct {
	ID     string  `yaml:"id"`
	At     *Point  `yaml:"at"`
	Facing string  `yaml:"facing"`
	Path   []Point `yaml:"path"`  // a loop to walk (instead of at)
	Speed  float64 `yaml:"speed"` // meters a second along the path (1.1 when 0)
	Doll   *Doll   `yaml:"doll"`
}

// Feet is where the person stands (the path's first point when walking).
func (p *StagePerson) Feet() Point {
	if p.At != nil {
		return *p.At
	}
	if len(p.Path) > 0 {
		return p.Path[0]
	}
	return Point{}
}

// Thing is something in a stage to look at, or an action of the location given a place.
type Thing struct {
	ID      string    `yaml:"id"`
	Name    string    `yaml:"name"`
	Area    Polygon   `yaml:"area"`
	Ask     string    `yaml:"ask"` // the word of the button (Examine when empty)
	When    Expr      `yaml:"when"`
	Do      []Command `yaml:"do"`
	Once    bool      `yaml:"once"`
	Trigger string    `yaml:"trigger"` // examine (the default) or step
	Action  string    `yaml:"action"`  // instead of do: the id of one of the location's actions
}

// Doll is how someone looks as a small box doll in a stage. The colors come from the
// standing picture; these choose the hair and add parts.
type Doll struct {
	Hair  string     `yaml:"hair"` // short, long, bald or bun; guessed from the picture when empty
	Parts []DollPart `yaml:"parts"`
	// FaceAt cuts the face for the balloons: [middle, top, width] in fractions of the
	// picture's visible part (found from the picture when empty).
	FaceAt []float64 `yaml:"face_at"`
	// Icon is a square picture of the face drawn for the balloons (cut round); it comes
	// before face_at and the face gozen art faces finds.
	Icon string `yaml:"icon"`
}

// DollPart is a piece added to a doll.
type DollPart struct {
	Kind  string `yaml:"kind"`
	Color string `yaml:"color"` // #rrggbb
}

// DollPartKinds are the parts a doll can wear or carry.
var DollPartKinds = []string{
	"apron", "cape", "robe", "skirt", "dress", "coat", "shield", "staff", "glasses",
	"hat", "hood", "bag", "goggles", "muffler", "shawl", "ponytail", "lashes",
}

// DollHairs are the hair styles of a doll.
var DollHairs = []string{"short", "long", "bald", "bun"}

// Bubble is a line someone says in a balloon over their doll now and then. Written as the
// text, or as {text, when}.
type Bubble struct {
	Text string `yaml:"text"`
	When Expr   `yaml:"when"`
}

// UnmarshalYAML reads a line or a map.
func (b *Bubble) UnmarshalYAML(unmarshal func(any) error) error {
	var s string
	if err := unmarshal(&s); err == nil {
		b.Text = s
		return nil
	}
	type plain Bubble
	return unmarshal((*plain)(b))
}

// SpotIDs are the stage's spots in the order they were written (by name when the
// stage was not read from YAML).
func (s *Stage) SpotIDs() []string { return orderedKeys(s.Spots, s.spotOrder) }

// StartSpot is the spot one stands on coming in without one named: the one with start,
// else the first.
func (s *Stage) StartSpot() string {
	ids := s.SpotIDs()
	for _, id := range ids {
		if s.Spots[id].Start {
			return id
		}
	}
	if len(ids) > 0 {
		return ids[0]
	}
	return ""
}

// NearestSpot is the spot nearest p ("" when there is none).
func (s *Stage) NearestSpot(p Point) string {
	best, bd := "", math.Inf(1)
	for _, id := range s.SpotIDs() {
		if d := geom.Dist(s.Spots[id].At.Pt(), p.Pt()); d < bd {
			best, bd = id, d
		}
	}
	return best
}

// Person is the stage's place of the person with the id, or nil.
func (s *Stage) Person(id string) *StagePerson {
	for i := range s.People {
		if s.People[i].ID == id {
			return &s.People[i]
		}
	}
	return nil
}

// ThingOf is the thing that gives the action with the id a place, or nil.
func (s *Stage) ThingOf(action string) *Thing {
	for i := range s.Things {
		if s.Things[i].Action == action && action != "" {
			return &s.Things[i]
		}
	}
	return nil
}

// Free reports whether feet may stand at p: in a walk polygon and in no block.
func (s *Stage) Free(p geom.Pt) bool {
	in := false
	for _, w := range s.Walk {
		if w.Poly().Inside(p) {
			in = true
			break
		}
	}
	if !in {
		return false
	}
	for _, b := range s.Blocks {
		if b.Poly().Inside(p) {
			return false
		}
	}
	return true
}

// Spotted reports whether id is a spot of the location's stage or a point of its map.
func (l *Location) Spotted(id string) bool {
	switch {
	case l == nil || id == "":
		return false
	case l.Stage != nil:
		return l.Stage.Spots[id] != nil
	case l.Map != nil:
		return l.Map.Points[id] != nil
	}
	return false
}

// keepOrder keeps the written order of a location's stage spots and map points (raw is
// the location as an ordered map).
func (l *Location) keepOrder(raw any) {
	keys := func(path ...string) []string {
		cur := raw
		for _, name := range path {
			m, ok := cur.(yaml.MapSlice)
			if !ok {
				return nil
			}
			cur = nil
			for _, it := range m {
				if fmt.Sprint(it.Key) == name {
					cur = it.Value
				}
			}
		}
		m, ok := cur.(yaml.MapSlice)
		if !ok {
			return nil
		}
		out := make([]string, 0, len(m))
		for _, it := range m {
			out = append(out, fmt.Sprint(it.Key))
		}
		return out
	}
	if l.Stage != nil {
		l.Stage.spotOrder = keys("stage", "spots")
	}
	if l.Map != nil {
		l.Map.pointOrder = keys("map", "points")
	}
}

// SceneWarnings are the problems of the maps and stages that do not stop the game loading
// (a point no road reaches, an exit no one can walk into, a polygon crossing itself).
func (db *DB) SceneWarnings() []error {
	if db.sceneWarned {
		return slices.Clone(db.sceneWarns)
	}
	c := &checker{db: db}
	for _, id := range db.Ordered("locations") {
		l := db.Locations[id]
		if l == nil {
			continue
		}
		if l.Map != nil {
			c.atlas("locations."+id, id, l)
		}
		if l.Stage != nil {
			c.stage("locations."+id, id, l)
		}
	}
	c.faceWarnings()
	c.iconWarnings()
	c.campaignWarnings()
	return sortedDiagnostics(c.warns)
}

// orderedKeys lists m's keys in order (written order first, then the rest by name).
func orderedKeys[V any](m map[string]V, order []string) []string {
	out := make([]string, 0, len(m))
	seen := map[string]bool{}
	for _, id := range order {
		if _, ok := m[id]; ok && !seen[id] {
			out = append(out, id)
			seen[id] = true
		}
	}
	var rest []string
	for id := range m {
		if !seen[id] {
			rest = append(rest, id)
		}
	}
	slices.Sort(rest)
	return append(out, rest...)
}

var hexColor = regexp.MustCompile(`^#[0-9a-fA-F]{6}$`)

// pictureSize is the size of a picture under assets/ (false when it cannot be read).
func (c *checker) pictureSize(p string) (image.Point, bool) {
	if p == "" || c.db.FS == nil {
		return image.Point{}, false
	}
	b, err := fs.ReadFile(c.db.FS, "assets/"+p)
	if err != nil {
		return image.Point{}, false
	}
	cfg, _, err := image.DecodeConfig(bytes.NewReader(b))
	if err != nil {
		return image.Point{}, false
	}
	return image.Pt(cfg.Width, cfg.Height), true
}

// inPicture checks that a point is in a picture of the size (when the size is known).
func (c *checker) inPicture(w string, p Point, size image.Point, known bool) {
	if known && (p[0] < 0 || p[1] < 0 || p[0] > float64(size.X) || p[1] > float64(size.Y)) {
		c.errorf(diag.OutOfRange, w, "[%g, %g] is outside the picture (%dx%d)", p[0], p[1], size.X, size.Y)
	}
}

// polygon checks a polygon: three points or more, in the picture, and (a warning) not
// crossing itself.
func (c *checker) polygon(w string, q Polygon, size image.Point, known bool) bool {
	if len(q) < 3 {
		c.errorf(diag.OutOfRange, w, "a polygon needs three points or more (it has %d)", len(q))
		return false
	}
	for i, p := range q {
		c.inPicture(fmt.Sprintf("%s[%d]", w, i), p, size, known)
	}
	if q.Poly().SelfCrossing() {
		c.warnf(diag.DrawnDifferently, w, "the polygon crosses itself")
	}
	return true
}

// facingWord checks a way to face.
func (c *checker) facingWord(w, f string) {
	if f != "" && FacingIndex(f) < 0 {
		c.errorf(diag.NotAWord, w, "want north, east, south or west, not %q", f)
	}
}

// doll checks a doll.
func (c *checker) doll(w string, d *Doll) {
	if d == nil {
		return
	}
	c.needsVersion(w, "doll", SchemaVersion3)
	if d.Hair != "" && !slices.Contains(DollHairs, d.Hair) {
		c.errorf(diag.NotAWord, w+".hair", "want one of %v, not %q", DollHairs, d.Hair)
	}
	for i, p := range d.Parts {
		ww := fmt.Sprintf("%s.parts[%d]", w, i)
		if !slices.Contains(DollPartKinds, p.Kind) {
			c.errorf(diag.NotAWord, ww+".kind", "%q is not a part (want one of %v)", p.Kind, DollPartKinds)
		}
		if !hexColor.MatchString(p.Color) {
			c.errorf(diag.NotAWord, ww+".color", "want #rrggbb, not %q", p.Color)
		}
	}
	c.file(w+".icon", d.Icon)
	if len(d.FaceAt) != 0 && len(d.FaceAt) != 3 {
		c.errorf(diag.Shape, w+".face_at", "face_at is [middle, top, width]")
	}
	for i, v := range d.FaceAt {
		if v < 0 || v > 1 {
			c.errorf(diag.OutOfRange, fmt.Sprintf("%s.face_at[%d]", w, i), "want a fraction in [0, 1], got %g", v)
		}
	}
}

// bubbles checks a person's balloon lines.
func (c *checker) bubbles(w string, bs []Bubble) {
	if len(bs) == 0 {
		return
	}
	c.needsVersion(w, "bubbles", SchemaVersion3)
	for i, b := range bs {
		ww := fmt.Sprintf("%s[%d]", w, i)
		if b.Text == "" {
			c.errorf(diag.Shape, ww, "a balloon needs text")
		}
		c.text(ww+".text", b.Text, FieldNames)
		c.expr(ww+".when", b.When, FieldNames)
	}
}

// stage checks a location's stage.
func (c *checker) stage(w, _ string, l *Location) {
	s := l.Stage
	w += ".stage"
	c.needsVersion(w, "stage", SchemaVersion3)
	size, known := c.pictureSize(l.Background.File)
	if l.Background.File == "" {
		c.errorf(diag.Shape, w, "a stage needs the location's background (the painting)")
	}
	if s.Depth != "" {
		c.file(w+".depth", s.Depth)
		if ds, ok := c.pictureSize(s.Depth); ok && known && ds != size {
			c.errorf(diag.Shape, w+".depth", "the depth is %dx%d, the background %dx%d (they must be the same size)", ds.X, ds.Y, size.X, size.Y)
		}
	}
	if s.Camera < 0 || s.Camera >= 90 {
		c.errorf(diag.OutOfRange, w+".camera", "want degrees in [0, 90), got %g", s.Camera)
	}
	if s.DepthBias < 0 || s.DepthBias >= 1 {
		c.errorf(diag.OutOfRange, w+".depth_bias", "want a fraction in [0, 1), got %g", s.DepthBias)
	}
	switch {
	case len(s.Scale) < 2:
		c.errorf(diag.OutOfRange, w+".scale", "scale needs two marks or more ({at: [x, y], height: pixels})")
	default:
		ys := map[float64]bool{}
		for i, m := range s.Scale {
			ww := fmt.Sprintf("%s.scale[%d]", w, i)
			if m.Height <= 0 {
				c.errorf(diag.OutOfRange, ww+".height", "want a height above 0, got %g", m.Height)
			}
			c.inPicture(ww+".at", m.At, size, known)
			ys[m.At[1]] = true
		}
		if len(ys) < 2 {
			c.errorf(diag.Shape, w+".scale", "the marks need different heights on the picture (y)")
		}
	}
	if len(s.Walk) == 0 {
		c.errorf(diag.Shape, w+".walk", "walk is empty (where may one walk?)")
	}
	for i, q := range s.Walk {
		c.polygon(fmt.Sprintf("%s.walk[%d]", w, i), q, size, known)
	}
	for i, q := range s.Blocks {
		c.polygon(fmt.Sprintf("%s.blocks[%d]", w, i), q, size, known)
	}
	for i, f := range s.Fronts {
		ww := fmt.Sprintf("%s.fronts[%d]", w, i)
		c.polygon(ww+".area", f.Area, size, known)
		if f.Base <= 0 || known && f.Base > float64(size.Y) {
			c.errorf(diag.OutOfRange, ww+".base", "base is the ground's y under the front, in the picture (got %g)", f.Base)
		}
	}
	free := func(ww string, p Point) {
		if len(s.Walk) == 0 {
			return
		}
		c.inPicture(ww, p, size, known)
		if !s.Free(p.Pt()) {
			c.errorf(diag.Shape, ww, "[%g, %g] is not where one can walk (outside walk or in blocks)", p[0], p[1])
		}
	}
	walkable := func(q Polygon) bool {
		for _, wq := range s.Walk {
			if len(q) >= 3 && len(wq) >= 3 && wq.Poly().Touches(q.Poly()) {
				return true
			}
		}
		return false
	}
	starts := 0
	for _, sid := range s.SpotIDs() {
		sp := s.Spots[sid]
		ww := w + ".spots." + sid
		if sp == nil {
			c.errorf(diag.Shape, ww, "a spot needs at")
			continue
		}
		free(ww+".at", sp.At)
		c.facingWord(ww+".facing", sp.Facing)
		if sp.Start {
			starts++
		}
		c.text(ww+".name", sp.Name, FieldNames)
		if sp.Name != "" && len(sp.Do) == 0 {
			c.warnf(diag.Unreachable, ww+".name", "a spot without do is no entry: its name is not shown")
		}
		c.commands(ww+".do", sp.Do, FieldNames)
		if sp.Once && len(sp.Do) == 0 {
			c.errorf(diag.Shape, ww+".once", "once is for a spot with do")
		}
	}
	if len(s.Spots) == 0 {
		c.errorf(diag.OutOfRange, w+".spots", "a stage needs a spot or more (where one stands)")
	}
	if starts > 1 {
		c.errorf(diag.OutOfRange, w+".spots", "%d spots have start (one at most)", starts)
	}
	exits := map[string]bool{}
	for _, e := range l.Exits {
		exits[e.To] = true
	}
	areas := map[string]bool{}
	for i, e := range s.Exits {
		ww := fmt.Sprintf("%s.exits[%d]", w, i)
		if !exits[e.To] {
			c.errorf(diag.UnknownReference, ww+".to", "%q is not the to of one of the location's exits", e.To)
		}
		if areas[e.To] {
			c.errorf(diag.Shape, ww+".to", "a second area for the exit to %s", e.To)
		}
		areas[e.To] = true
		if c.polygon(ww+".area", e.Area, size, known) && len(s.Walk) > 0 && !walkable(e.Area) {
			c.warnf(diag.Unreachable, ww+".area", "the exit does not touch walk (no one can walk into it)")
		}
	}
	people := map[string]bool{}
	pictured := map[string]bool{}
	for _, p := range l.People {
		people[p.ID] = true
		pictured[p.ID] = p.Image != ""
	}
	placed := map[string]bool{}
	for i := range s.People {
		p := &s.People[i]
		ww := fmt.Sprintf("%s.people[%d]", w, i)
		switch {
		case p.ID == "":
			c.errorf(diag.Shape, ww+".id", "id is missing (the id of one of the location's people)")
		case !people[p.ID]:
			c.errorf(diag.UnknownReference, ww+".id", "%q is not the id of one of the location's people", p.ID)
		case placed[p.ID]:
			c.errorf(diag.Duplicate, ww+".id", "%q is placed twice", p.ID)
		}
		placed[p.ID] = true
		if people[p.ID] && !pictured[p.ID] {
			c.warnf(diag.DrawnDifferently, ww+".id", "%s has no image: the doll takes plain colors (give the person an image)", p.ID)
		}
		if (p.At == nil) == (len(p.Path) == 0) {
			c.errorf(diag.Shape, ww, "a person in a stage has at (stands) or path (walks), one of them")
		}
		if p.At != nil {
			free(ww+".at", *p.At)
		}
		if len(p.Path) == 1 {
			c.errorf(diag.OutOfRange, ww+".path", "a path needs two points or more")
		}
		for j, q := range p.Path {
			free(fmt.Sprintf("%s.path[%d]", ww, j), q)
		}
		if p.Speed < 0 {
			c.errorf(diag.OutOfRange, ww+".speed", "want a speed of 0 or more, got %g", p.Speed)
		}
		c.facingWord(ww+".facing", p.Facing)
		c.doll(ww+".doll", p.Doll)
	}
	actions := map[string]bool{}
	for _, a := range l.Actions {
		actions[a.ID] = true
	}
	thingIDs := map[string]bool{}
	for i := range s.Things {
		t := &s.Things[i]
		ww := fmt.Sprintf("%s.things[%d]", w, i)
		if c.polygon(ww+".area", t.Area, size, known) && len(s.Walk) > 0 && !walkable(t.Area) {
			c.warnf(diag.Unreachable, ww+".area", "the thing does not touch walk (no one can reach it)")
		}
		switch t.Trigger {
		case "", "examine", "step":
		default:
			c.errorf(diag.NotAWord, ww+".trigger", "want examine or step, not %q", t.Trigger)
		}
		if t.Action != "" {
			if !actions[t.Action] {
				c.errorf(diag.UnknownReference, ww+".action", "%q is not the id of one of the location's actions", t.Action)
			}
			if len(t.Do) > 0 || t.ID != "" || t.Name != "" || t.Once || t.When.Set() || t.Trigger != "" || t.Ask != "" {
				c.errorf(diag.Shape, ww, "a thing with action takes the action's label, when and do (no id, name, do, once, when, trigger or ask)")
			}
			continue
		}
		switch {
		case t.ID == "":
			c.errorf(diag.Shape, ww+".id", "a thing needs an id (or action)")
		case thingIDs[t.ID]:
			c.errorf(diag.Duplicate, ww+".id", "duplicate thing id %q", t.ID)
		}
		thingIDs[t.ID] = true
		if t.Name == "" {
			c.errorf(diag.Shape, ww+".name", "a thing needs a name (its label)")
		}
		if len(t.Do) == 0 {
			c.errorf(diag.Shape, ww+".do", "a thing needs do (or action)")
		}
		c.text(ww+".name", t.Name, FieldNames)
		c.text(ww+".ask", t.Ask, FieldNames)
		c.expr(ww+".when", t.When, FieldNames)
		c.commands(ww+".do", t.Do, FieldNames)
	}
}
