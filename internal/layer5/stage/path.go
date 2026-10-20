package stage

import (
	"math"

	"github.com/nao1215/gozen/internal/layer1/geom"
)

// The way to a clicked or tapped place (ADR 0022, stage 4): A* over a grid of cells of the
// background where feet may stand, then the corners that can be cut are cut. Everything
// after the grid is made works on the grid's cells (no polygon tests), and its arrays are
// kept from click to click, so a click on a 1920-wide scene costs well under a millisecond.

// grid is where feet may stand, in square cells of the background: a 1920-wide one in
// cells of 16 pixels, a smaller one finer (about 100 cells across, 4 pixels at least).
type grid struct {
	x0, y0, w, h int
	cell         float64
	free         []bool
	// the search's arrays, kept: cost and prev are valid where seen == run
	cost []float64
	prev []int32
	seen []uint32
	run  uint32
	open nodes
	way  []int
}

func (s *Scene) grid() *grid {
	if s.g != nil {
		return s.g
	}
	var lo, hi geom.Pt
	for i, w := range s.Walk {
		a, b := w.Bounds()
		if i == 0 {
			lo, hi = a, b
			continue
		}
		lo = geom.Pt{X: math.Min(lo.X, a.X), Y: math.Min(lo.Y, a.Y)}
		hi = geom.Pt{X: math.Max(hi.X, b.X), Y: math.Max(hi.Y, b.Y)}
	}
	c := math.Max(4, math.Min(16, math.Max(hi.X-lo.X, hi.Y-lo.Y)/100))
	g := &grid{x0: int(lo.X), y0: int(lo.Y), cell: c, w: int((hi.X-lo.X)/c) + 1, h: int((hi.Y-lo.Y)/c) + 1}
	n := g.w * g.h
	g.free, g.cost, g.prev, g.seen = make([]bool, n), make([]float64, n), make([]int32, n), make([]uint32, n)
	for y := range g.h {
		for x := range g.w {
			g.free[y*g.w+x] = s.Free(g.center(x, y))
		}
	}
	s.g = g
	return g
}

func (g *grid) center(x, y int) geom.Pt {
	return geom.Pt{X: float64(g.x0) + (float64(x)+0.5)*g.cell, Y: float64(g.y0) + (float64(y)+0.5)*g.cell}
}

func (g *grid) at(p geom.Pt) (int, int) {
	return int(math.Floor((p.X - float64(g.x0)) / g.cell)), int(math.Floor((p.Y - float64(g.y0)) / g.cell))
}

func (g *grid) ok(x, y int) bool { return x >= 0 && y >= 0 && x < g.w && y < g.h && g.free[y*g.w+x] }

// nearestFree is the free cell nearest (x, y) (false when the grid has none): a cell
// outside the grid is first brought onto its edge, then rings around it are searched (only
// the ring's cells, not the square inside).
func (g *grid) nearestFree(x, y int) (int, int, bool) {
	x, y = max(0, min(g.w-1, x)), max(0, min(g.h-1, y))
	if g.ok(x, y) {
		return x, y, true
	}
	for r := 1; r < max(g.w, g.h); r++ {
		bx, by, bd := 0, 0, math.Inf(1)
		try := func(cx, cy int) {
			if g.ok(cx, cy) {
				if d := math.Hypot(float64(cx-x), float64(cy-y)); d < bd {
					bx, by, bd = cx, cy, d
				}
			}
		}
		for d := -r; d <= r; d++ {
			try(x+d, y-r)
			try(x+d, y+r)
			try(x-r, y+d)
			try(x+r, y+d)
		}
		if bd < math.Inf(1) {
			return bx, by, true
		}
	}
	return 0, 0, false
}

// line reports whether the cells on the straight line between two cells are all free
// (every cell the line passes, its corners included).
func (g *grid) line(ax, ay, bx, by int) bool {
	dx, dy := abs(bx-ax), abs(by-ay)
	sx, sy := sign(bx-ax), sign(by-ay)
	x, y := ax, ay
	e := dx - dy
	for {
		if !g.ok(x, y) {
			return false
		}
		if x == bx && y == by {
			return true
		}
		e2 := 2 * e
		switch {
		case e2 > -dy && e2 < dx: // a diagonal step: both cells beside it must be free too
			if !g.ok(x+sx, y) || !g.ok(x, y+sy) {
				return false
			}
			e -= dy
			x += sx
			e += dx
			y += sy
		case e2 > -dy:
			e -= dy
			x += sx
		default:
			e += dx
			y += sy
		}
	}
}

func abs(v int) int {
	if v < 0 {
		return -v
	}
	return v
}

func sign(v int) int {
	switch {
	case v > 0:
		return 1
	case v < 0:
		return -1
	}
	return 0
}

type node struct {
	i int
	f float64
}

type nodes []node

// push and pop keep the nodes a binary heap by f (typed: no boxing, no allocation once
// the slice has grown).
func (q *nodes) push(n node) {
	*q = append(*q, n)
	h := *q
	for i := len(h) - 1; i > 0; {
		up := (i - 1) / 2
		if h[up].f <= h[i].f {
			break
		}
		h[up], h[i] = h[i], h[up]
		i = up
	}
}

func (q *nodes) pop() node {
	h := *q
	top := h[0]
	last := len(h) - 1
	h[0] = h[last]
	h = h[:last]
	for i := 0; ; {
		l, r, m := 2*i+1, 2*i+2, i
		if l < len(h) && h[l].f < h[m].f {
			m = l
		}
		if r < len(h) && h[r].f < h[m].f {
			m = r
		}
		if m == i {
			break
		}
		h[m], h[i] = h[i], h[m]
		i = m
	}
	*q = h
	return top
}

var steps = [8][3]float64{{1, 0, 1}, {-1, 0, 1}, {0, 1, 1}, {0, -1, 1}, {1, 1, math.Sqrt2}, {1, -1, math.Sqrt2}, {-1, 1, math.Sqrt2}, {-1, -1, math.Sqrt2}}

// Path is the way from p to q where feet may go: the points to walk to in order, the last
// q itself when feet may stand there, else the nearest place to it they may. Empty when
// no way leads there.
func (s *Scene) Path(p, q geom.Pt) []geom.Pt {
	if s.Free(q) && s.Clear(p, q) {
		return []geom.Pt{q}
	}
	g := s.grid()
	sx, sy, ok1 := g.nearestFree(g.at(p))
	tx, ty, ok2 := g.nearestFree(g.at(q))
	if !ok1 || !ok2 {
		return nil
	}
	start, goal := sy*g.w+sx, ty*g.w+tx
	g.run++
	if g.run == 0 { // the stamps wrapped: forget them all
		clear(g.seen)
		g.run = 1
	}
	h := func(i int) float64 { return math.Hypot(float64(i%g.w-tx), float64(i/g.w-ty)) }
	g.seen[start], g.cost[start], g.prev[start] = g.run, 0, int32(start) //nolint:gosec // a cell index of the grid
	g.open = append(g.open[:0], node{start, h(start)})
	for len(g.open) > 0 {
		n := g.open.pop()
		if n.i == goal {
			break
		}
		if n.f > g.cost[n.i]+h(n.i)+1e-9 {
			continue // an old entry of a cell reached cheaper since
		}
		x, y := n.i%g.w, n.i/g.w
		for _, d := range steps {
			nx, ny := x+int(d[0]), y+int(d[1])
			if !g.ok(nx, ny) || d[2] > 1 && (!g.ok(nx, y) || !g.ok(x, ny)) {
				continue // no cutting a blocked corner
			}
			j := ny*g.w + nx
			c := g.cost[n.i] + d[2]
			if g.seen[j] != g.run || c < g.cost[j] {
				g.seen[j], g.cost[j], g.prev[j] = g.run, c, int32(n.i) //nolint:gosec // a cell index of the grid
				g.open.push(node{j, c + h(j)})
			}
		}
	}
	if g.seen[goal] != g.run {
		return nil
	}
	g.way = g.way[:0]
	for i := goal; ; i = int(g.prev[i]) {
		g.way = append(g.way, i)
		if i == start {
			break
		}
	}
	// cut the corners on the grid: from each cell, the farthest cell of the way in a straight
	// free line (walking forward and stopping at the first that is not)
	out := []geom.Pt{}
	at := len(g.way) - 1 // the way runs from the goal back to the start
	for at > 0 {
		next := at - 1
		for j := at - 2; j >= 0; j-- {
			if !g.line(g.way[at]%g.w, g.way[at]/g.w, g.way[j]%g.w, g.way[j]/g.w) {
				break
			}
			next = j
		}
		out = append(out, g.center(g.way[next]%g.w, g.way[next]/g.w))
		at = next
	}
	// the feet may not go straight to the first cell's middle: go to the start cell's first
	if len(out) > 0 && !s.Clear(p, out[0]) {
		out = append([]geom.Pt{g.center(sx, sy)}, out...)
	}
	if s.Free(q) {
		last := g.center(tx, ty)
		if len(out) > 0 {
			last = out[len(out)-1]
		}
		if s.Clear(last, q) {
			out = append(out, q)
		}
	}
	if len(out) == 0 {
		out = append(out, g.center(tx, ty))
	}
	return out
}

// Clear reports whether feet may go straight from p to q (checked every few pixels).
func (s *Scene) Clear(p, q geom.Pt) bool {
	d := geom.Dist(p, q)
	n := int(d/4) + 1
	for i := 1; i <= n; i++ {
		if !s.Free(p.Add(q.Sub(p).Scale(float64(i) / float64(n)))) {
			return false
		}
	}
	return true
}
