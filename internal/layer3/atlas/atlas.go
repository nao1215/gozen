// Package atlas is the rules of maps and stages (ADR 0022): where the party stands (a
// point of a map, a spot of a stage), which points the open roads reach, what going along
// a road does (steps, battles met on the way, its do), and where one stands on arriving.
// The screen and the headless player share it; the screen only draws the walk between.
package atlas

import (
	"container/heap"
	"fmt"
	"math"
	"slices"
	"strconv"
	"strings"

	data "github.com/nao1215/gozen/internal/layer2/gamedata"
	"github.com/nao1215/gozen/internal/rpg"
)

func holds(st *rpg.State, e data.Expr) bool {
	ok, err := e.Bool(st.Env())
	return err == nil && ok
}

// Here is where the party stands at its location: its spot when that is a point of the
// map or a spot of the stage, else the location's start ("" for other locations).
func Here(db *data.DB, st *rpg.State) string {
	l := db.Locations[st.Location]
	switch {
	case l == nil:
		return ""
	case l.Spotted(st.Spot):
		return st.Spot
	case l.Stage != nil:
		return l.Stage.StartSpot()
	case l.Map != nil:
		return l.Map.StartPoint()
	}
	return ""
}

// Open reports whether a point of a map is open now (its when holds).
func Open(st *rpg.State, p *data.MapPoint) bool { return p != nil && holds(st, p.When) }

// Usable reports whether a road is open now: its when holds and both ends are open.
func Usable(st *rpg.State, a *data.Atlas, r *data.Road) bool {
	if len(r.Between) != 2 || !holds(st, r.When) {
		return false
	}
	return Open(st, a.Points[r.Between[0]]) && Open(st, a.Points[r.Between[1]])
}

// Other is the point at the other end of the road from p.
func Other(r *data.Road, p string) string {
	if len(r.Between) != 2 {
		return ""
	}
	if r.Between[0] == p {
		return r.Between[1]
	}
	return r.Between[0]
}

type item struct {
	id    string
	steps int
}

type queue []item

func (q queue) Len() int           { return len(q) }
func (q queue) Less(i, j int) bool { return q[i].steps < q[j].steps }
func (q queue) Swap(i, j int)      { q[i], q[j] = q[j], q[i] }
func (q *queue) Push(x any) {
	if it, ok := x.(item); ok {
		*q = append(*q, it)
	}
}

func (q *queue) Pop() any {
	old := *q
	it := old[len(old)-1]
	*q = old[:len(old)-1]
	return it
}

// routes keeps a shortest-path tree; complete paths are built only for offered choices.
type routes struct {
	from  string
	atlas *data.Atlas
	dist  map[string]int
	prev  map[string]int
}

func findRoutes(st *rpg.State, a *data.Atlas, from string) *routes {
	out := &routes{from: from, atlas: a, dist: map[string]int{}, prev: map[string]int{}}
	if a.Points[from] == nil {
		return out
	}
	adjacent := make(map[string][]int, len(a.Points))
	for i := range a.Roads {
		r := &a.Roads[i]
		if !Usable(st, a, r) {
			continue
		}
		for _, id := range r.Between {
			adjacent[id] = append(adjacent[id], i)
		}
	}
	out.dist[from] = 0
	q := &queue{{from, 0}}
	for q.Len() > 0 {
		it, ok := heap.Pop(q).(item)
		if !ok || it.steps > out.dist[it.id] {
			continue
		}
		for _, i := range adjacent[it.id] {
			r := &a.Roads[i]
			steps := r.StepsOrOne()
			if steps > math.MaxInt-it.steps {
				continue
			}
			to, n := Other(r, it.id), it.steps+steps
			if d, seen := out.dist[to]; !seen || n < d {
				out.dist[to], out.prev[to] = n, i
				heap.Push(q, item{to, n})
			}
		}
	}
	return out
}

func (r *routes) path(id string) ([]int, bool) {
	if _, ok := r.dist[id]; !ok {
		return nil, false
	}
	var way []int
	for at := id; at != r.from; {
		i := r.prev[at]
		way = append(way, i)
		at = Other(&r.atlas.Roads[i], at)
	}
	slices.Reverse(way)
	return way, true
}

// Choice is a point of the map the party can go to from where it stands.
type Choice struct {
	Point  string
	Roads  []int // the roads to go along (none: the point the party stands on)
	Locked bool  // the point is closed; choosing it says its locked line
}

// Choices are the points of the party's map to choose now, in the order written: those the
// open roads reach that lead somewhere (to) or have something to do (do, not yet done when
// once), and the closed points next to them that say why (locked). Instant maps
// offer actions independently of roads, including the current point.
func Choices(db *data.DB, st *rpg.State) []Choice {
	l := db.Locations[st.Location]
	if l == nil || l.Map == nil {
		return nil
	}
	a := l.Map
	here := Here(db, st)
	var routes *routes
	if a.Activation != "instant" {
		routes = findRoutes(st, a, here)
	}
	var out []Choice
	for _, id := range a.PointIDs() {
		p := a.Points[id]
		if p == nil || p.Hide == "closed" && !Open(st, p) {
			continue
		}
		if a.Activation == "instant" {
			open := Open(st, p)
			if open && (p.To != "" || pending(st, st.Location, id, p)) {
				out = append(out, Choice{Point: id})
			} else if !open && p.Locked != "" {
				out = append(out, Choice{Point: id, Locked: true})
			}
			continue
		}
		_, ok := routes.dist[id]
		switch {
		case ok && (p.To != "" || pending(st, st.Location, id, p)):
			if id == here && p.To == "" {
				continue // standing on it: its do has run
			}
			way, _ := routes.path(id)
			out = append(out, Choice{Point: id, Roads: way})
		case !ok && p.Locked != "" && !Open(st, p):
			// closed: offered when an open road from a reached point leads to it
			for i := range a.Roads {
				r := &a.Roads[i]
				if !slices.Contains(r.Between, id) || !holds(st, r.When) {
					continue
				}
				if w, reached := routes.path(Other(r, id)); reached {
					out = append(out, Choice{Point: id, Roads: append(slices.Clone(w), i), Locked: true})
					break
				}
			}
		}
	}
	return out
}

// pending reports whether a point has a do to run (not one with once already run).
func pending(st *rpg.State, loc, id string, p *data.MapPoint) bool {
	return len(p.Do) > 0 && (!p.Once || !st.Done[DoneKey(loc, id)])
}

// DoneKey is the key in State.Done of a point, spot or thing of a location that has once.
func DoneKey(loc, id string) string { return loc + "/" + id }

// Go is what going to a choice does, as commands run one road at a time: along each
// road its steps, battles and do, then the party stands on the road's far point; at the
// end the point's do (once marked done) and, when the point leads somewhere, the move. A
// battle lost on the way leaves the party on the last point it reached (ADR 0022: the
// rules settle road by road). A locked choice only says why. Instant maps run
// only the point action, without crossing roads or changing the current point.
func Go(db *data.DB, st *rpg.State, c Choice) []data.Command {
	l := db.Locations[st.Location]
	if l == nil || l.Map == nil {
		return nil
	}
	a := l.Map
	p := a.Points[c.Point]
	if p == nil {
		return nil
	}
	if c.Locked {
		return []data.Command{{Say: &data.Say{Text: p.Locked}}}
	}
	if a.Activation == "instant" {
		return []data.Command{{Reach: c.Point}}
	}
	var cmds []data.Command
	at := Here(db, st)
	for _, i := range c.Roads {
		at = Other(&a.Roads[i], at)
		cmds = append(cmds, data.Command{Cross: fmt.Sprintf("%d>%s", i, at)})
	}
	return append(cmds, data.Command{Reach: c.Point})
}

// CrossRoad runs the internal command cross ("<road>><point>", put by Go): the road's
// steps are added to the party's steps, each step may meet a battle of its encounters
// (not when calm), its do follows, and then the party passes the point at its far end.
func CrossRoad(db *data.DB, st *rpg.State, arg string) []data.Command {
	l := db.Locations[st.Location]
	is, point, ok := strings.Cut(arg, ">")
	i, err := strconv.Atoi(is)
	if l == nil || l.Map == nil || !ok || err != nil || i < 0 || i >= len(l.Map.Roads) {
		return nil
	}
	return append(Cross(db, st, &l.Map.Roads[i]), data.Command{Reach: passing + point})
}

// passing marks a point the party only passes on its way (Reach puts it there, no more).
const passing = "~"

// Reach runs the internal command reach (put by Go): the party stands on the point of its
// map and has visited it; at the end of the way, the point's do (once marked done) and,
// when the point leads somewhere, the move follow. In instant mode only the
// action and explicit move run; selecting a point does not count as visiting it.
func Reach(db *data.DB, st *rpg.State, arg string) []data.Command {
	l := db.Locations[st.Location]
	point, pass := strings.CutPrefix(arg, passing)
	if l == nil || l.Map == nil || l.Map.Points[point] == nil {
		return nil
	}
	if l.Map.Activation != "instant" {
		st.Spot = point
		Visit(db, st, st.Location, point)
	}
	if pass {
		return nil
	}
	p := l.Map.Points[point]
	if l.Map.Activation == "instant" && !Open(st, p) {
		if p.Locked != "" {
			return []data.Command{{Say: &data.Say{Text: p.Locked}}}
		}
		return nil
	}
	var cmds []data.Command
	if pending(st, st.Location, point, p) {
		if p.Once {
			st.Done[DoneKey(st.Location, point)] = true
		}
		cmds = append(cmds, p.Do...)
	}
	if p.To != "" {
		cmds = append(cmds, data.Command{Move: p.To, At: p.AtPoint})
	}
	return cmds
}

// Cross is going along a road: its steps are added to the party's steps, each step may
// meet a battle of its encounters (not when calm), and its do follows.
func Cross(_ *data.DB, st *rpg.State, r *data.Road) []data.Command {
	n := min(r.StepsOrOne(), data.MaxRoadSteps)
	if st.Steps <= math.MaxInt-n {
		st.Steps += n
	} else {
		st.Steps = math.MaxInt
	}
	var cmds []data.Command
	if e := r.Encounters; e.Rate.Set() && !st.Calm && len(e.Troops) > 0 {
		for range n {
			rate, err := e.Rate.Number(st.Env())
			if err != nil || st.Rand.Float64() >= rate {
				continue
			}
			if t := PickTroop(st, e.Troops); t != "" {
				cmds = append(cmds, data.Command{Battle: t})
			}
		}
	}
	return append(cmds, r.Do...)
}

// PickTroop draws one of the troops by weight ("" when none holds).
func PickTroop(st *rpg.State, troops []data.TroopPick) string {
	var picks []data.TroopPick
	total := 0.0
	for _, t := range troops {
		if !holds(st, t.When) {
			continue
		}
		if t.Weight <= 0 {
			t.Weight = 1
		}
		picks = append(picks, t)
		total += t.Weight
	}
	x := st.Rand.Float64() * total
	for _, t := range picks {
		if x -= t.Weight; x < 0 {
			return t.Troop
		}
	}
	return ""
}

// Visit marks a point of a map as visited (State.Mapped[loc], a byte a point in the order
// the points are written).
func Visit(db *data.DB, st *rpg.State, loc, point string) {
	l := db.Locations[loc]
	if l == nil || l.Map == nil {
		return
	}
	ids := l.Map.PointIDs()
	i := slices.Index(ids, point)
	if i < 0 {
		return
	}
	m := st.Mapped[loc]
	if len(m) != len(ids) { // the points changed: keep what fits
		grown := make([]byte, len(ids))
		copy(grown, m)
		m = grown
	}
	m[i] = 1
	if st.Mapped == nil {
		st.Mapped = map[string][]byte{}
	}
	st.Mapped[loc] = m
}

// Visited reports whether the party has been at a point of a map.
func Visited(db *data.DB, st *rpg.State, loc, point string) bool {
	l := db.Locations[loc]
	if l == nil || l.Map == nil {
		return false
	}
	i := slices.Index(l.Map.PointIDs(), point)
	m := st.Mapped[loc]
	return i >= 0 && i < len(m) && m[i] != 0
}

// Place puts the party where a move into loc arrives, from the location from: in a stage
// on the spot at, else the start spot; on a map on the point at, else the point leading
// back to from, else the start point. Other locations (a dungeon places the party with
// dungeon.Place) have no spot.
func Place(db *data.DB, st *rpg.State, from, loc, at string) {
	l := db.Locations[loc]
	switch {
	case l == nil:
		return
	case l.Stage != nil:
		st.Spot = at
		if l.Stage.Spots[at] == nil {
			st.Spot = l.Stage.StartSpot()
		}
	case l.Map != nil:
		switch {
		case l.Map.Points[at] != nil:
			st.Spot = at
		case l.Map.PointTo(from) != "":
			st.Spot = l.Map.PointTo(from)
		default:
			st.Spot = l.Map.StartPoint()
		}
		Visit(db, st, loc, st.Spot)
	default:
		st.Spot = ""
	}
}
