package main

import (
	"math/rand/v2"
	"slices"
	"strconv"
	"strings"

	"github.com/nao1215/gozen/internal/cliio"

	"github.com/nao1215/gozen/internal/clihelp"
	"github.com/nao1215/gozen/internal/diag"

	data "github.com/nao1215/gozen/internal/layer2/gamedata"
	"github.com/nao1215/gozen/internal/layer3/dungeon"
	"github.com/nao1215/gozen/internal/rpg"
)

func (c *command) floorsCmd(args []string) error {
	fs := clihelp.NewFlagSet("gozen sim floors")
	var loc string
	fs.StringVar(&loc, "location", "", "the location `ID`; required")
	from := fs.Int("from", 1, "the first floor to show")
	to := fs.Int("to", 20, "the last floor to show")
	floor := fs.Int("floor", 0, "show only this floor; with --try, the floor whose seeds are compared")
	try := fs.Int("try", 0, "compare `N` seeds (0 to N-1) on --floor, one line each")
	noMap := fs.Bool("no-map", false, "print the numbers only, without the maps")
	if err := c.parse(fs, args); err != nil {
		return err
	}
	if fs.NArg() != 1 {
		return cliio.Invalid(diag.Operands, "expected one game directory")
	}
	if loc == "" {
		return cliio.Invalid(diag.OptionRequired, "--location ID is required")
	}
	dir := fs.Arg(0)
	if *from <= 0 || *to < *from || *floor < 0 || *try < 0 || (*try > 0 && *floor == 0) {
		return cliio.Invalid(diag.BadValue, "invalid floor range or seed count; --try requires --floor")
	}
	fsys, err := gameFS(dir)
	if err != nil {
		return err
	}
	if err := data.UseWords(fsys); err != nil {
		return err
	}
	db, err := data.Load(fsys)
	if err != nil {
		return err
	}
	l := db.Locations[loc]
	if l == nil || l.Dungeon == nil || l.Dungeon.Generate == nil {
		return cliio.Invalid(diag.UnknownName, "%s is not a generated dungeon of the game", loc)
	}
	g := l.Dungeon.Generate
	if *floor > 0 {
		*from, *to = *floor, *floor
	}
	if *try > 0 {
		if *floor == 0 {
			return cliio.Invalid(diag.OptionNeedsAnother, "--try needs --floor")
		}
		keep, had := g.Floors[*floor]
		if g.Floors == nil {
			g.Floors = map[int]data.FloorPlan{}
		}
		for s := range uint64(*try) {
			g.Floors[*floor] = data.FloorPlan{Seed: &s}
			st := floorState(db, loc, *floor)
			c.out.Printf("seed %3d  %s\n", s, floorStats(db, st).line())
		}
		if had {
			g.Floors[*floor] = keep
		} else {
			delete(g.Floors, *floor)
		}
		return nil
	}
	for f := *from; f <= *to; f++ {
		st := floorState(db, loc, f)
		how := "made"
		if s, ok := g.SeedFor(f); ok {
			how = "seed " + strconv.FormatUint(s, 10)
		}
		if g.Drawn(f) != "" {
			how = "drawn"
		}
		c.out.Printf("B%dF (%s)  %s\n", f, how, floorStats(db, st).line())
		if n := g.Floors[f].Note; n != "" {
			c.out.Printf("  %s\n", n)
		}
		if !*noMap {
			c.out.Println(dungeon.Of(db, st).Map)
		}
	}
	return nil
}

func floorState(db *data.DB, loc string, floor int) *rpg.State {
	st := rpg.New(db, rand.New(rand.NewPCG(1, 2))) //nolint:gosec // Reproducible gameplay simulation, not security randomness.
	st.Calm = true
	st.Location = loc
	dungeon.Dive(db, st, loc, floor)
	return st
}

type floorNumbers struct {
	w, h, open, deadEnds int
	down, downWarp       int
	at                   map[rune][]int
	battles, battlesAll  float64
}

func (n floorNumbers) line() string {
	var b strings.Builder
	appendf(&b, "%dx%d open %d deadends %d  down %d", n.w, n.h, n.open, n.deadEnds, n.down)

	if n.downWarp < n.down {
		appendf(&b, " (warp %d)", n.downWarp)
	}
	appendf(&b, "  battles %.1f (all %.1f) ", n.battles, n.battlesAll)

	for _, r := range slices.Sorted(func(yield func(rune) bool) {
		for r := range n.at {
			if !yield(r) {
				return
			}
		}
	}) {
		ds := slices.Clone(n.at[r])
		slices.Sort(ds)
		var ss []string
		for _, d := range ds {
			ss = append(ss, strconv.Itoa(d))
		}
		appendf(&b, " %c:%s", r, strings.Join(ss, ","))

	}
	return b.String()
}

func floorStats(db *data.DB, st *rpg.State) floorNumbers {
	d := dungeon.Of(db, st)
	w, h := d.Size()
	steps := [4][2]int{{0, -1}, {1, 0}, {0, 1}, {-1, 0}}
	pass := func(x, y int) bool {
		return x >= 0 && y >= 0 && x < w && y < h && dungeon.CellAt(d, st, x, y).Passable()
	}
	dests := map[rune][][2]int{}
	for y := range h {
		for x := range w {
			dests[d.At(x, y)] = append(dests[d.At(x, y)], [2]int{x, y})
		}
	}
	bfs := func(warp bool) map[[2]int]int {
		start := [2]int{st.Pos.X, st.Pos.Y}
		dist := map[[2]int]int{start: 0}
		q := [][2]int{start}
		for len(q) > 0 {
			c := q[0]
			q = q[1:]
			var next [][2]int
			if to, ok := carryTo(db, st.Location, d, d.At(c[0], c[1])); ok && c != start {
				if !warp {
					continue
				}
				next = dests[to]
			} else {
				for _, s := range steps {
					if n := [2]int{c[0] + s[0], c[1] + s[1]}; pass(n[0], n[1]) {
						next = append(next, n)
					}
				}
			}
			for _, n := range next {
				if _, ok := dist[n]; !ok {
					dist[n] = dist[c] + 1
					q = append(q, n)
				}
			}
		}
		return dist
	}
	plain, warped := bfs(false), bfs(true)
	reach := func(dist map[[2]int]int, x, y int) int {
		if v, ok := dist[[2]int{x, y}]; ok {
			return v
		}
		best := -1
		for _, s := range steps {
			if v, ok := dist[[2]int{x + s[0], y + s[1]}]; ok && (best < 0 || v+1 < best) {
				best = v + 1
			}
		}
		return best
	}
	n := floorNumbers{w: w, h: h, at: map[rune][]int{}, down: -1, downWarp: -1}
	for y := range h {
		for x := range w {
			r := d.At(x, y)
			if r == data.CellWall || r == data.CellNone {
				continue
			}
			n.open++
			if r != data.CellFloor && r != data.CellDoor {
				n.at[r] = append(n.at[r], reach(plain, x, y))
			}
			if pass(x, y) {
				open := 0
				for _, s := range steps {
					if pass(x+s[0], y+s[1]) {
						open++
					}
				}
				if open <= 1 {
					n.deadEnds++
				}
			}
		}
	}
	if xs := dests['>']; len(xs) > 0 {
		n.down, n.downWarp = reach(plain, xs[0][0], xs[0][1]), reach(warped, xs[0][0], xs[0][1])
	}
	n.battles = expectedBattles(db, st, max(0, n.downWarp))
	n.battlesAll = expectedBattles(db, st, n.open)
	return n
}

func carryTo(db *data.DB, loc string, d *data.Dungeon, r rune) (rune, bool) {
	t := d.Tile(r)
	if t == nil || dungeon.Trigger(t, t.Solid.Set() || t.Look == "wall") != "step" {
		return 0, false
	}
	var find func(cmds []data.Command, seen map[string]bool) (rune, bool)
	find = func(cmds []data.Command, seen map[string]bool) (rune, bool) {
		for _, c := range cmds {
			if c.Move == loc && c.At != "" {
				return []rune(c.At)[0], true
			}
			if c.Call != "" && !seen[c.Call] && db.Events[c.Call] != nil {
				seen[c.Call] = true
				if to, ok := find(db.Events[c.Call].Do, seen); ok {
					return to, true
				}
			}
		}
		return 0, false
	}
	return find(t.Do, map[string]bool{})
}

func expectedBattles(db *data.DB, st *rpg.State, walk int) float64 {
	e := db.Locations[st.Location].Encounters
	if !e.Rate.Set() || walk <= 0 {
		return 0
	}
	p, err := e.Rate.Number(st.Env())
	if err != nil {
		return 0
	}
	rng := rand.New(rand.NewPCG(7, 7)) //nolint:gosec // Reproducible gameplay simulation, not security randomness.
	total := 0
	const runs = 2000
	for range runs {
		gauge := 0.0
		for range walk {
			chance := p
			if e.Gauge > 0 {
				gauge = min(1, gauge+p)
				chance = gauge * e.Gauge
			}
			if rng.Float64() < chance {
				total++
				gauge = 0
			}
		}
	}
	return float64(total) / runs
}
