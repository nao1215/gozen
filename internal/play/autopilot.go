package play

import (
	"fmt"
	"io"
	"log"
	"os"
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/nao1215/gozen/internal/layer1/memo"
	data "github.com/nao1215/gozen/internal/layer2/gamedata"
	"github.com/nao1215/gozen/internal/layer3/dungeon"
	"github.com/nao1215/gozen/internal/rpg"
)

type Autopilot struct {
	Seconds   int
	Dive      string
	ReturnHP  float64
	Floors    int
	Report    string
	ShotEvery int
}

type autopilot struct {
	cfg     Autopilot
	start   time.Time
	ticks   int
	wait    int
	path    []rpg.Pos
	target  string
	goal    [2]int
	used    map[string]bool
	lastPos rpg.Pos
	still   int
	diveAt  int
	cmdAt   int

	battles, wipes, dives, deepest, floorsWalked int
	lastBattle                                   *battleScene
	chests, returns, commands                    int
	problems                                     []string
	errs                                         []string
	visited                                      map[string]bool
}

func newAutopilot(c Autopilot) *autopilot {
	if c.ReturnHP == 0 {
		c.ReturnHP = 0.35
	}
	if c.Floors == 0 {
		c.Floors = 3
	}
	return &autopilot{cfg: c, start: time.Now(), visited: map[string]bool{}, used: map[string]bool{}}
}

func (a *autopilot) problem(g *Game, format string, args ...any) {
	msg := fmt.Sprintf(format, args...)
	where := ""
	if g.st != nil {
		where = fmt.Sprintf(" [%s %d,%d floor %v]", g.st.Location, g.st.Pos.X, g.st.Pos.Y, g.st.Vars[dungeon.VarFloor])
	}
	a.problems = append(a.problems, msg+where)
	g.emit("problem", traceLine{"text": msg + where})
	g.quickMemo(memo.Bug, msg, memo.Autoplay)
	log.Printf("autopilot: %s%s", msg, where)
}

func (a *autopilot) step(g *Game) {
	a.ticks++
	if a.cfg.ShotEvery > 0 && a.ticks%(a.cfg.ShotEvery*60) == 0 {
		g.autoShot = fmt.Sprintf("auto_%05d", a.ticks/60)
	}
	if a.cfg.Seconds > 0 && a.ticks > a.cfg.Seconds*60 {
		a.finish(g)
		g.quit = true
		return
	}
	if a.wait > 0 {
		a.wait--
		return
	}
	press := func(b button, wait int) {
		g.in.pressed[b] = true
		a.wait = wait
	}
	top := g.stack[len(g.stack)-1]
	switch s := top.(type) {
	case *title:
		if s.menu != nil {
			s.menu.sel = 0
		}
		press(btnOK, 20)
		return
	case *gameOver:
		a.wipes++
		log.Printf("autopilot: the party fell (%d)", a.wipes)
		press(btnOK, 30)
		return
	case *ending:
		press(btnOK, 30)
		return
	case *battleScene:
		if a.lastBattle != s {
			a.lastBattle = s
			a.battles++
		}
		if s.phase == bpInput && !s.auto {
			g.act("battle_auto", nil)
		}
		if what, t := g.waiting(); what == "text" && t > 30 {
			press(btnOK, 4)
		}
		return
	}

	if what, t := g.waiting(); what != "" {
		switch {
		case what == "choice":
			press(btnOK, 12)
		case what == "text" && t > 25:
			press(btnOK, 6)
		}
		return
	}
	if g.busy() && len(g.stack) > 1 {

		press(btnCancel, 20)
		return
	}
	if g.busy() {
		return
	}
	switch top.(type) {
	case *dungeonScene:
		a.inDungeon(g, press)
	case *field, *stageScene, *atlasScene:
		a.inTown(g)
		a.wait = 40
	}
}

func (a *autopilot) inDungeon(g *Game, press func(button, int)) {
	d := dungeon.Of(g.db, g.st)
	if d == nil {
		return
	}
	st := g.st
	a.arrived(g)
	floor := 0
	if f, ok := st.Vars[dungeon.VarFloor].(float64); ok {
		floor = int(f)
		key := fmt.Sprintf("%v/%d", st.Vars[dungeon.VarSeed], floor)
		if !a.visited[key] {
			a.visited[key] = true
			a.floorsWalked++
			a.path = nil
		}
		a.deepest = max(a.deepest, floor)
	}

	if a.worn(g) || (a.diveAt > 0 && floor >= a.diveAt+a.cfg.Floors-1 && floor%5 != 0) {
		if a.useReturn(g) {
			return
		}
	}

	if a.useCommand(g) {
		return
	}

	if st.Pos == a.lastPos {
		a.still++
		if a.still == 60*20 {
			a.problem(g, "the party has not moved for 20 seconds")
			a.path = nil
		}
	} else {
		a.still, a.lastPos = 0, st.Pos
	}
	if len(a.path) == 0 {
		a.plan(g, d)
		if len(a.path) == 0 {
			if a.still > 60*5 && a.still%(60*30) == 0 {
				a.problem(g, "nowhere to go on this floor (no stairs reached)")
				a.useReturn(g)
			}
			press(btnRight, 20)
			return
		}
	}

	for i, p := range a.path {
		if p.X == st.Pos.X && p.Y == st.Pos.Y {
			a.path = a.path[i:]
			break
		}
	}
	next := a.path[0]
	if abs(next.X-st.Pos.X)+abs(next.Y-st.Pos.Y) > 1 {
		a.path = nil
		return
	}
	if next.X == st.Pos.X && next.Y == st.Pos.Y {
		a.path = a.path[1:]
		if len(a.path) == 0 {
			a.use(g)
			a.wait = 20
		}
		return
	}
	want := facingTo(st.Pos.X, st.Pos.Y, next.X, next.Y)
	switch (want - st.Pos.Facing + 4) % 4 {
	case 0:
		front := dungeon.Front(g.db, st)
		if front.Solid {
			a.path = nil
			a.use(g)
			press(btnUp, 20)
			return
		}
		press(btnUp, 10)
	case 1:
		press(btnRight, 10)
	case 3:
		press(btnLeft, 10)
	default:
		press(btnRight, 10)
	}
}

func usedKey(g *Game, x, y int) string {
	return fmt.Sprintf("%s/%v/%v/%d,%d", g.st.Location, g.st.Vars[dungeon.VarSeed], g.st.Vars[dungeon.VarFloor], x, y)
}

func (a *autopilot) use(g *Game) {
	a.used[usedKey(g, a.goal[0], a.goal[1])] = true
}

func facingTo(x, y, nx, ny int) int {
	switch {
	case ny < y:
		return 0
	case nx > x:
		return 1
	case ny > y:
		return 2
	}
	return 3
}

func (a *autopilot) plan(g *Game, d *data.Dungeon) {
	st := g.st
	w, h := d.Size()
	type node struct{ x, y int }
	start := node{st.Pos.X, st.Pos.Y}
	prev := map[node]node{start: start}
	q := []node{start}
	var goals []node
	var unseen []node
	for len(q) > 0 {
		c := q[0]
		q = q[1:]
		if c != start && !dungeon.Seen(g.db, st, st.Location, c.x, c.y) {
			unseen = append(unseen, c)
		}
		for f, dd := range [][2]int{{0, -1}, {1, 0}, {0, 1}, {-1, 0}} {
			n := node{c.x + dd[0], c.y + dd[1]}
			if n.x < 0 || n.y < 0 || n.x >= w || n.y >= h {
				continue
			}
			if _, seen := prev[n]; seen {
				continue
			}
			cell := dungeon.CellAt(d, st, n.x, n.y)
			if dungeon.Slides(cell) && cell.Passable() {

				prev[n] = c
				ex, ey, _, _ := dungeon.SlideEnd(d, st, n.x, n.y, f)
				e := node{ex, ey}
				if e == n {
					q = append(q, n)
					continue
				}
				if _, seen := prev[e]; seen {
					continue
				}
				prev[e] = n
				ec := dungeon.CellAt(d, st, ex, ey)
				if ec.Tile != nil && !dungeon.Slides(ec) && len(ec.Tile.Do) > 0 && !dungeon.Spent(st, st.Location, ex, ey) && !a.used[usedKey(g, ex, ey)] {
					goals = append(goals, e)
				}
				q = append(q, e)
				continue
			}
			if cell.Tile != nil && len(cell.Tile.Do) > 0 && !dungeon.Spent(st, st.Location, n.x, n.y) && !a.used[usedKey(g, n.x, n.y)] {
				prev[n] = c
				goals = append(goals, n)
			}
			if !cell.Passable() {
				continue
			}
			prev[n] = c
			q = append(q, n)
		}
	}
	rank := func(n node) int {
		c := dungeon.CellAt(d, st, n.x, n.y)
		switch string(c.Char) {
		case "T":
			return 0
		case ">":
			return 2
		}
		return 1
	}
	sort.SliceStable(goals, func(i, j int) bool { return rank(goals[i]) < rank(goals[j]) })
	if len(unseen) > 0 {
		goals = append(goals, unseen[0])
	}
	for _, goal := range goals {
		ch := string(dungeon.CellAt(d, st, goal.x, goal.y).Char)
		if ch == "S" || ch == "P" {
			continue
		}
		var way []rpg.Pos
		for c := goal; c != start; c = prev[c] {
			way = append([]rpg.Pos{{In: st.Location, X: c.x, Y: c.y}}, way...)
		}
		a.path = way
		a.target = ch
		a.goal = [2]int{goal.x, goal.y}
		if ch == "T" {
			a.chests++
		}
		return
	}
}

func (a *autopilot) worn(g *Game) bool {
	hp, mhp := 0, 0
	for _, m := range g.st.Members() {
		if !m.Alive() {
			return true
		}
		hp += m.HP
		mhp += m.Stat("mhp")
	}
	return float64(hp) < a.cfg.ReturnHP*float64(mhp)
}

func (a *autopilot) useReturn(g *Game) bool {
	for _, id := range g.st.Bag() {
		it := g.db.Items[id]
		if it.Event == "" || g.db.Events[it.Event] == nil || it.Command {
			continue
		}
		if it.EventOK.Set() {
			if ok, err := it.EventOK.Bool(g.st.Env()); err != nil || !ok {
				continue
			}
		}
		a.returns++
		log.Printf("autopilot: back to town with %s (floor %v)", it.Name, g.st.Vars[dungeon.VarFloor])
		g.act("use_item", traceLine{"item": id})
		a.path, a.diveAt = nil, 0
		return true
	}
	return false
}

const commandEvery = 30

func (a *autopilot) useCommand(g *Game) bool {
	if g.st.Steps-a.cmdAt < commandEvery {
		return false
	}
	for _, id := range g.st.Bag() {
		it := g.db.Items[id]
		if !it.Command || it.Event == "" || g.db.Events[it.Event] == nil || !it.EventOK.Set() {
			continue
		}
		if ok, err := it.EventOK.Bool(g.st.Env()); err != nil || !ok {
			continue
		}
		a.cmdAt = g.st.Steps
		a.commands++
		log.Printf("autopilot: uses %s (floor %v)", it.Name, g.st.Vars[dungeon.VarFloor])
		g.act("use_item", traceLine{"item": id})
		a.path = nil
		return true
	}
	return false
}

func (a *autopilot) inTown(g *Game) {
	st := g.st
	loc := g.db.Locations[st.Location]

	if a.worn(g) || a.anyHurt(g) {
		for _, l := range g.db.Order["locations"] {
			for i, act := range g.db.Locations[l].Actions {
				if hasHeal(g.db, act.Do) {
					g.act("run", traceLine{"location": l, "action": i})
					return
				}
			}
		}
	}

	for _, m := range rpg.AllActors(st) {
		for m.SP > 0 {
			best := ""
			for _, n := range m.Class().Tree {
				if m.NodeOpen(n.ID) && (best == "" || m.Tree[n.ID] < m.Tree[best]) {
					best = n.ID
				}
			}
			if best == "" || !g.act("raise", traceLine{"actor": m.ID, "node": best}) {
				break
			}
		}
		g.act("equip_best", traceLine{"actor": m.ID})
	}

	a.buyReturn(g)

	var dives []data.Action
	var where []string
	var index []int
	for _, l := range g.db.Order["locations"] {
		for j, act := range g.db.Locations[l].Actions {
			if hasDive(act.Do) && whenOK(g, act.When) {
				dives = append(dives, act)
				where = append(where, l)
				index = append(index, j)
			}
		}
	}
	if len(dives) == 0 {
		a.problem(g, "no way into the dungeon from town (%s)", loc.Name)
		g.quit = true
		return
	}
	i := len(dives) - 1
	if a.cfg.Dive != "" {
		for j, d := range dives {
			if strings.Contains(d.Label, a.cfg.Dive) {
				i = j
			}
		}
	}
	a.dives++
	log.Printf("autopilot: dive %d (%s)", a.dives, dives[i].Label)
	g.act("run", traceLine{"location": where[i], "action": index[i]})
	a.diveAt = -1
}

func (a *autopilot) anyHurt(g *Game) bool {
	for _, m := range g.st.Members() {
		if m.HP < m.Stat("mhp") || m.MP < m.Stat("mmp")/2 {
			return true
		}
	}
	return false
}

func (a *autopilot) buyReturn(g *Game) {
	for _, id := range sortedKeys(itemSet(g.db)) {
		it := g.db.Items[id]
		if it.Event == "" || g.st.Items[id] > 0 || it.Price <= 0 || it.Price > g.st.Gold {
			continue
		}
		if shop := shopOf(g, id); shop != "" {
			g.act("buy", traceLine{"shop": shop, "item": id, "count": 1, "price": it.Price})
			return
		}
	}
	if pot := g.db.Items["potion"]; pot != nil && g.st.Items["potion"] < 3 && g.st.Gold >= pot.Price*3 {
		g.act("buy", traceLine{"shop": shopOf(g, "potion"), "item": "potion", "count": 3, "price": pot.Price})
	}
}

func shopOf(g *Game, id string) string {
	for _, shop := range g.db.Ordered("shops") {
		if slices.Contains(g.db.Shops[shop].Goods, id) {
			return shop
		}
	}
	return ""
}

func hasHeal(db *data.DB, cmds []data.Command) bool {
	for _, c := range cmds {
		if c.Heal != "" || (c.Call != "" && db.Events[c.Call] != nil && hasHeal(db, db.Events[c.Call].Do)) ||
			hasHeal(db, c.Then) || hasHeal(db, c.Else) {
			return true
		}
	}
	return false
}

func hasDive(cmds []data.Command) bool {
	for _, c := range cmds {
		if c.Dive != "" {
			return true
		}
	}
	return false
}

func whenOK(g *Game, e data.Expr) bool {
	if !e.Set() {
		return true
	}
	ok, err := e.Bool(g.st.Env())
	return err == nil && ok
}

func (a *autopilot) arrived(g *Game) {
	if a.diveAt == -1 {
		if f, ok := g.st.Vars[dungeon.VarFloor].(float64); ok {
			a.diveAt = int(f)
		}
	}
}

func (a *autopilot) finish(g *Game) {
	var b strings.Builder
	fmt.Fprintf(&b, "autopilot report (%s of play, %d ticks)\n", time.Since(a.start).Round(time.Second), a.ticks)
	fmt.Fprintf(&b, "  dives %d, floors walked %d, deepest B%dF, back to town %d\n", a.dives, a.floorsWalked, a.deepest, a.returns)
	fmt.Fprintf(&b, "  action items used %d\n", a.commands)
	fmt.Fprintf(&b, "  battles %d, wipes %d, chests aimed at %d\n", a.battles, a.wipes, a.chests)
	if g.st != nil {
		fmt.Fprintf(&b, "  gold %d\n", g.st.Gold)
		for _, m := range rpg.AllActors(g.st) {
			fmt.Fprintf(&b, "  %s Lv%d HP %d/%d SP %d tree %v sets %v\n", m.Name(), m.Level, m.HP, m.Stat("mhp"), m.SP, m.Tree, m.Sets())
		}
	}
	fmt.Fprintf(&b, "  problems %d\n", len(a.problems))
	for _, p := range a.problems {
		fmt.Fprintf(&b, "    - %s\n", p)
	}
	for _, e := range a.errs {
		fmt.Fprintf(&b, "    - event error: %s\n", e)
	}
	if a.cfg.Report != "" {
		if err := os.WriteFile(a.cfg.Report, []byte(b.String()), 0o600); err != nil {
			g.failure = err
		}
	}
	if g.opts.Report != nil {
		if _, err := io.WriteString(g.opts.Report, b.String()); err != nil {
			g.failure = err
		}
	}
}

func itemSet(db *data.DB) map[string]bool {
	m := map[string]bool{}
	for id := range db.Items {
		m[id] = true
	}
	return m
}
