package play

import (
	"image"
	"image/color"
	"math"

	"github.com/hajimehoshi/ebiten/v2"

	data "github.com/nao1215/gozen/internal/layer2/gamedata"
	"github.com/nao1215/gozen/internal/layer5/gfx"
	lang "github.com/nao1215/gozen/internal/text"
)

const chartFocusFrames = 20

// chartFocusSet includes incoming and outgoing relations, including You when present.
func chartFocusSet(ids []string, profiles map[string]*data.Profile, focus int) []bool {
	set := make([]bool, len(ids))
	if focus < 0 || focus >= len(ids) {
		return set
	}
	set[focus] = true
	for i, id := range ids {
		if profiles[id] == nil {
			continue
		}
		for _, r := range profiles[id].Relations {
			if r.Who == ids[focus] {
				set[i] = true
			}
			if id == ids[focus] {
				for j, target := range ids {
					if r.Who == target {
						set[j] = true
					}
				}
			}
		}
	}
	return set
}

func chartFocusPoints(points []image.Point, set []bool, focus, frame int) []image.Point {
	out := append([]image.Point(nil), points...)
	if focus < 0 || focus >= len(points) || frame == 0 {
		return out
	}
	neighbors := make([]int, 0, len(points)-1)
	for i, related := range set {
		if related && i != focus {
			neighbors = append(neighbors, i)
		}
	}
	target := make([]image.Point, len(points))
	for n, i := range neighbors {
		angle := -math.Pi/2 + 2*math.Pi*float64(n)/float64(len(neighbors))
		target[i] = image.Pt(int(math.Round(175*math.Cos(angle))), int(math.Round(175*math.Sin(angle))))
	}
	t := float64(frame) / chartFocusFrames
	t = t * t * (3 - 2*t)
	for i := range out {
		if !set[i] {
			continue
		}
		out[i] = image.Pt(int(math.Round(float64(points[i].X)*(1-t)+float64(target[i].X)*t)), int(math.Round(float64(points[i].Y)*(1-t)+float64(target[i].Y)*t)))
	}
	return out
}

func chartOpacity(related bool, frame int) uint8 {
	if related {
		return 255
	}
	return uint8(max(0, min(255, 255-243*frame/chartFocusFrames))) // Clamped to 0..255 first.
}

func (s *profileScreen) chartDisplay(g *Game, base []image.Point) ([]image.Point, []bool) {
	ids := append(append([]string{}, s.ids...), "you")
	set := chartFocusSet(ids[:len(base)], g.db.Profiles, s.focus)
	fullX, fullY := s.chartView(g, base)
	if s.focus < 0 {
		s.camX, s.camY = fullX, fullY
		return base, set
	}
	t := float64(s.focusFrame) / chartFocusFrames
	t = t * t * (3 - 2*t)
	s.camX, s.camY = fullX*(1-t), fullY*(1-t)
	return chartFocusPoints(base, set, s.focus, s.focusFrame), set
}

// chartRoute picks the shortest deterministic two-segment detour that clears other faces.
func chartRoute(points []image.Point, from, to int) []image.Point {
	a, b := points[from], points[to]
	dx, dy := float64(b.X-a.X), float64(b.Y-a.Y)
	length := math.Hypot(dx, dy)
	if length < 1 {
		return []image.Point{a, b}
	}
	candidates := []image.Point{{X: (a.X + b.X) / 2, Y: (a.Y + b.Y) / 2}}
	for _, offset := range []float64{75, -75, 120, -120, 170, -170, 230, -230} {
		candidates = append(candidates, image.Pt(int(math.Round(float64(a.X+b.X)/2-dy/length*offset)), int(math.Round(float64(a.Y+b.Y)/2+dx/length*offset))))
	}
	best, score := candidates[0], math.MaxFloat64
	for _, mid := range candidates {
		cost := math.Hypot(float64(mid.X-a.X), float64(mid.Y-a.Y)) + math.Hypot(float64(b.X-mid.X), float64(b.Y-mid.Y))
		for i, face := range points {
			if i == from || i == to {
				continue
			}
			if chartSegmentDistance(a, mid, face) < 54 || chartSegmentDistance(mid, b, face) < 54 {
				cost += 10000
			}
		}
		if cost < score {
			best, score = mid, cost
		}
	}
	if best == candidates[0] {
		return []image.Point{a, b}
	}
	return []image.Point{a, best, b}
}

func chartSegmentDistance(a, b, p image.Point) float64 {
	dx, dy := float64(b.X-a.X), float64(b.Y-a.Y)
	denom := dx*dx + dy*dy
	if denom == 0 {
		return math.Hypot(float64(p.X-a.X), float64(p.Y-a.Y))
	}
	t := max(0, min(1, (float64(p.X-a.X)*dx+float64(p.Y-a.Y)*dy)/denom))
	return math.Hypot(float64(p.X-a.X)-t*dx, float64(p.Y-a.Y)-t*dy)
}

// chartLayout assigns compact, stable rows. Profile order keeps affiliations together.
func chartLayout(ids []string, profiles map[string]*data.Profile) []image.Point {
	points := make([]image.Point, len(ids))
	cols := min(4, max(2, int(math.Ceil(math.Sqrt(float64(len(ids)))))))
	rows := (len(ids) + cols - 1) / cols
	for i := range ids {
		row, col := i/cols, i%cols
		count := min(cols, len(ids)-row*cols)
		points[i] = image.Pt((2*col-count+1)*105, (2*row-rows+1)*88)
	}
	// Swaps within an affiliation preserve its cluster while reducing crossings.
	// The screen computes its layout each frame, so bound the search on large books.
	if len(ids) > 10 {
		return points
	}
	for range min(2, len(ids)) {
		changed := false
		for i := range ids {
			if profiles[ids[i]] == nil {
				continue
			}
			for j := i + 1; j < len(ids); j++ {
				if profiles[ids[j]] == nil || profiles[ids[i]].Affiliation != profiles[ids[j]].Affiliation {
					continue
				}
				before := chartCrossings(ids, profiles, points)
				points[i], points[j] = points[j], points[i]
				if chartCrossings(ids, profiles, points) < before {
					changed = true
				} else {
					points[i], points[j] = points[j], points[i]
				}
			}
		}
		if !changed {
			break
		}
	}
	return points
}

func chartCrossings(ids []string, profiles map[string]*data.Profile, points []image.Point) int {
	index := make(map[string]int, len(ids))
	for i, id := range ids {
		index[id] = i
	}
	type edge struct{ a, b int }
	edges := []edge{}
	for i, id := range ids {
		if profiles[id] == nil {
			continue
		}
		for _, r := range profiles[id].Relations {
			if j, ok := index[r.Who]; ok && i != j {
				edges = append(edges, edge{i, j})
			}
		}
	}
	cross := func(a, b, c image.Point) int {
		return (b.X-a.X)*(c.Y-a.Y) - (b.Y-a.Y)*(c.X-a.X)
	}
	count := 0
	for i, e := range edges {
		for _, f := range edges[i+1:] {
			if e.a == f.a || e.a == f.b || e.b == f.a || e.b == f.b {
				continue
			}
			a, b, c, d := points[e.a], points[e.b], points[f.a], points[f.b]
			if cross(a, b, c)*cross(a, b, d) < 0 && cross(c, d, a)*cross(c, d, b) < 0 {
				count++
			}
		}
	}
	return count
}

func chartHasPlayer(ids []string, profiles map[string]*data.Profile) bool {
	for _, id := range ids {
		for _, r := range profiles[id].Relations {
			if r.Who == "you" {
				return true
			}
		}
	}
	return false
}

func chartPoints(ids []string, profiles map[string]*data.Profile) []image.Point {
	if chartHasPlayer(ids, profiles) {
		return chartLayout(append(append([]string{}, ids...), "you"), profiles)
	}
	return chartLayout(ids, profiles)
}

func (s *profileScreen) chartView(g *Game, points []image.Point) (float64, float64) {
	sw, sh := g.screen()
	minX, maxX, minY, maxY := math.MaxFloat64, -math.MaxFloat64, math.MaxFloat64, -math.MaxFloat64
	for _, p := range points {
		minX, maxX = min(minX, float64(p.X)), max(maxX, float64(p.X))
		minY, maxY = min(minY, float64(p.Y)), max(maxY, float64(p.Y))
	}
	if len(points) == 0 {
		return 0, 0
	}
	// Keep the selected face in view, but centre a chart smaller than the viewport.
	x, y := float64(points[s.sel].X), float64(points[s.sel].Y)
	spanX, spanY := sw/s.zoom-160, (sh-120)/s.zoom-140
	if maxX-minX < spanX {
		x = (minX + maxX) / 2
	} else {
		x = max(minX+spanX/2, min(x, maxX-spanX/2))
	}
	if maxY-minY < spanY {
		y = (minY + maxY) / 2
	} else {
		y = max(minY+spanY/2, min(y, maxY-spanY/2))
	}
	return x, y
}

func (s *profileScreen) chartPoint(g *Game, p image.Point) (float64, float64) {
	sw, sh := g.screen()
	return sw/2 + (float64(p.X)-s.camX)*s.zoom, (sh+profileBand-50)/2 + (float64(p.Y)-s.camY)*s.zoom
}

func (s *profileScreen) chartMove(points []image.Point, dx, dy int) {
	if len(points) == 0 {
		return
	}
	from := points[s.sel]
	best, pick := math.MaxFloat64, s.sel
	for i, p := range points {
		if i >= len(s.ids) { // The player node has no detail page to select.
			break
		}
		if i == s.sel {
			continue
		}
		x, y := float64(p.X-from.X), float64(p.Y-from.Y)
		forward := x*float64(dx) + y*float64(dy)
		if forward <= 0 {
			continue
		}
		cross := math.Abs(x*float64(dy) - y*float64(dx))
		score := forward + cross*3
		if score < best {
			best, pick = score, i
		}
	}
	s.sel = pick
}

func (s *profileScreen) updateChart(g *Game) {
	if g.in.cancel() || g.in.pressed[btnBook] {
		if s.focus >= 0 && !g.in.pressed[btnBook] {
			s.focusBack = true
			g.sfx("cancel")
		} else {
			g.sfx("cancel")
			g.pop()
			if s.done != nil {
				s.done()
			}
			return
		}
	}
	if len(s.ids) == 0 {
		return
	}
	points := chartPoints(s.ids, g.db.Profiles)
	if s.focus >= 0 {
		if s.focusBack {
			s.focusFrame = max(0, s.focusFrame-1)
			if s.focusFrame == 0 {
				s.focus, s.focusBack = -1, false
			}
		} else if s.focusFrame < chartFocusFrames {
			s.focusFrame++
		}
		clickedFocus := false
		if g.in.click && s.focusFrame == chartFocusFrames {
			shown, _ := s.chartDisplay(g, points)
			x, y := s.chartPoint(g, shown[s.focus])
			clickedFocus = g.in.mouse.In(image.Rect(int(x-43*s.zoom), int(y-43*s.zoom), int(x+43*s.zoom), int(y+43*s.zoom)))
		}
		if (g.in.ok() || clickedFocus) && s.focusFrame == chartFocusFrames && !s.focusBack && s.opened(g, s.current()) {
			s.face, s.chart, s.chartPage = 0, false, true
			g.sfx("decide")
		}
		s.chartTick++
		s.trace(g)
		return
	}
	old := s.sel
	switch {
	case g.in.left():
		s.chartMove(points, -1, 0)
	case g.in.right():
		s.chartMove(points, 1, 0)
	case g.in.up():
		s.chartMove(points, 0, -1)
	case g.in.down():
		s.chartMove(points, 0, 1)
	}
	if old != s.sel {
		g.sfx("cursor")
	}
	if g.in.wheel > 0 {
		s.zoom = min(1.8, s.zoom*1.12)
	}
	if g.in.wheel < 0 {
		s.zoom = max(0.55, s.zoom/1.12)
	}
	if g.in.click {
		for i, p := range points[:len(s.ids)] {
			x, y := s.chartPoint(g, p)
			if g.in.mouse.In(image.Rect(int(x-43*s.zoom), int(y-43*s.zoom), int(x+43*s.zoom), int(y+43*s.zoom))) && s.opened(g, s.ids[i]) {
				s.sel, s.focus, s.focusFrame = i, i, 0
				g.sfx("decide")
				break
			}
		}
	}
	if s.focus < 0 && g.in.ok() && s.opened(g, s.current()) {
		s.focus, s.focusFrame = s.sel, 0
		g.sfx("decide")
	}
	s.camX, s.camY = s.chartView(g, points)
	s.chartTick++
	s.trace(g)
}

func chartColor(kind string) color.NRGBA {
	switch kind {
	case "like":
		return color.NRGBA{0xf0, 0xa1, 0x75, 0xff}
	case "love":
		return color.NRGBA{0xf3, 0x69, 0x9f, 0xff}
	case "rival", "enemy":
		return color.NRGBA{0xe0, 0x69, 0x62, 0xff}
	case "family":
		return color.NRGBA{0xe4, 0xbd, 0x65, 0xff}
	case "friend":
		return color.NRGBA{0x69, 0xbc, 0xa6, 0xff}
	default:
		return color.NRGBA{0x9b, 0xac, 0xc3, 0xff}
	}
}

func (s *profileScreen) chartEdgeVisible(g *Game, from, to string) bool {
	return g.db.Profiles[to] != nil && s.opened(g, from) && s.opened(g, to)
}

func (s *profileScreen) drawChart(g *Game, dst *ebiten.Image) {
	sw, sh := g.screen()
	g.drawPicture(dst, g.db.Game.TitleBG, 0, 0, sw, sh, 1)
	fill(dst, withAlpha(colorBlack, 0xe8))
	gfx.FillRoundRect(dst, 16, profileBand+12, float32(sw-32), float32(sh-profileBand-72), 14, color.NRGBA{0x0d, 0x1b, 0x25, 0xec})
	gfx.FillRect(dst, 0, 0, float32(sw), profileBand, withAlpha(g.th.window, 0xe0), false)
	g.text(dst, lang.T("相関図"), 30, 12, sizeL, g.th.text)
	points, related := s.chartDisplay(g, chartPoints(s.ids, g.db.Profiles))
	index := map[string]int{}
	for i, id := range s.ids {
		index[id] = i
	}
	if len(points) > len(s.ids) {
		index["you"] = len(s.ids)
	}
	for i, id := range s.ids {
		for _, r := range g.db.Profiles[id].Relations {
			j, ok := index[r.Who]
			if !ok || i == j || (r.Who != "you" && !s.chartEdgeVisible(g, id, r.Who)) || !s.opened(g, id) {
				continue
			}
			route := chartRoute(points, i, j)
			screen := make([][2]float64, len(route))
			for k, p := range route {
				screen[k][0], screen[k][1] = s.chartPoint(g, p)
			}
			start, end := screen[0], screen[len(screen)-1]
			dx, dy := screen[1][0]-start[0], screen[1][1]-start[1]
			firstLength := math.Hypot(dx, dy)
			lastDX, lastDY := end[0]-screen[len(screen)-2][0], end[1]-screen[len(screen)-2][1]
			lastLength := math.Hypot(lastDX, lastDY)
			if firstLength < 1 || lastLength < 1 {
				continue
			}
			ux, uy := dx/firstLength, dy/firstLength
			vx, vy := lastDX/lastLength, lastDY/lastLength
			// Separate opposing directions and stop the stroke at the face border.
			off := 7 * s.zoom
			screen[0] = [2]float64{start[0] - uy*off + ux*39*s.zoom, start[1] + ux*off + uy*39*s.zoom}
			screen[len(screen)-1] = [2]float64{end[0] - vy*off - vx*39*s.zoom, end[1] + vx*off - vy*39*s.zoom}
			c := chartColor(r.Kind)
			if s.focus >= 0 {
				c.A = chartOpacity(i == s.focus || j == s.focus, s.focusFrame)
			} else if i != s.sel && j != s.sel {
				c.A = 140
			}
			for k := 0; k+1 < len(screen); k++ {
				gfx.StrokeLine(dst, float32(screen[k][0]), float32(screen[k][1]), float32(screen[k+1][0]), float32(screen[k+1][1]), float32(2.5*s.zoom), c, true)
			}
			// A relation with no return edge is directed.
			reverse := false
			if target := g.db.Profiles[r.Who]; target != nil {
				for _, back := range target.Relations {
					if back.Who == id {
						reverse = true
						break
					}
				}
			}
			if !reverse {
				x2, y2 := screen[len(screen)-1][0], screen[len(screen)-1][1]
				ax, ay := x2-vx*12*s.zoom, y2-vy*12*s.zoom
				gfx.StrokeLine(dst, float32(x2), float32(y2), float32(ax-vy*7*s.zoom), float32(ay+vx*7*s.zoom), 2, c, true)
				gfx.StrokeLine(dst, float32(x2), float32(y2), float32(ax+vy*7*s.zoom), float32(ay-vx*7*s.zoom), 2, c, true)
			}
		}
	}
	if len(points) > len(s.ids) {
		x, y := s.chartPoint(g, points[len(s.ids)])
		alpha := uint8(255)
		if s.focus >= 0 {
			alpha = chartOpacity(related[len(s.ids)], s.focusFrame)
		}
		size := 78 * s.zoom
		gfx.FillRoundRect(dst, float32(x-size/2-3), float32(y-size/2-3), float32(size+6), float32(size+6), 12, withAlpha(g.th.border, alpha))
		gfx.FillRoundRect(dst, float32(x-size/2), float32(y-size/2), float32(size), float32(size), 10, withAlpha(g.th.window, alpha))
		// A head and shoulders silhouette makes the player distinct from authored faces.
		gfx.FillRoundRect(dst, float32(x-12*s.zoom), float32(y-24*s.zoom), float32(24*s.zoom), float32(24*s.zoom), 12, withAlpha(g.th.dim, alpha))
		gfx.FillRoundRect(dst, float32(x-24*s.zoom), float32(y+3*s.zoom), float32(48*s.zoom), float32(27*s.zoom), 13, withAlpha(g.th.dim, alpha))
		g.text(dst, "You", x-gfx.TextWidth("You", sizeS*0.9)/2, y+size/2+7, sizeS*0.9, withAlpha(g.th.text, alpha))
	}
	for i, id := range s.ids {
		x, y := s.chartPoint(g, points[i])
		if x < -100 || x > sw+100 || y < profileBand-100 || y > sh+100 {
			continue
		}
		open := s.opened(g, id)
		alpha := uint8(255)
		if s.focus >= 0 {
			alpha = chartOpacity(related[i], s.focusFrame)
		}
		c := g.th.border
		if i == s.sel {
			c = g.th.cursor
		}
		size := 78 * s.zoom
		gfx.FillRoundRect(dst, float32(x-size/2-3), float32(y-size/2-3), float32(size+6), float32(size+6), 12, withAlpha(c, alpha))
		gfx.FillRoundRect(dst, float32(x-size/2), float32(y-size/2), float32(size), float32(size), 10, withAlpha(g.th.window, alpha))
		if open {
			if pic := g.db.ProfileIcon(id); pic != "" {
				bob := 0.0
				if a := g.db.Actors[g.db.ProfileActor(id)]; a != nil && a.Puppet != "" {
					bob = math.Sin(float64(s.chartTick)*0.04) * 2
				}
				g.drawFace(dst, pic, x-size/2+4, y-size/2+4+bob, size-8, float32(alpha)/255, false)
			} else {
				g.text(dst, "?", x-8, y-17, sizeL, withAlpha(g.th.dim, alpha))
			}
		} else {
			g.text(dst, "?", x-8, y-17, sizeL, withAlpha(g.th.dim, alpha))
		}
		name := "?"
		if open {
			name = g.db.ProfileName(id)
		}
		g.text(dst, name, x-gfx.TextWidth(name, sizeS*0.9)/2, y+size/2+7, sizeS*0.9, withAlpha(g.th.text, alpha))
	}
	groups := map[string]image.Point{}
	for i, id := range s.ids {
		group := g.db.Profiles[id].Affiliation
		if group == "" {
			continue
		}
		p, ok := groups[group]
		if !ok || points[i].Y < p.Y || (points[i].Y == p.Y && points[i].X < p.X) {
			groups[group] = points[i]
		}
	}
	for _, id := range s.ids {
		if s.focus >= 0 {
			break
		}
		group := g.db.Profiles[id].Affiliation
		if p, ok := groups[group]; ok {
			x, y := s.chartPoint(g, p)
			g.text(dst, group, x-39*s.zoom, y-68*s.zoom, sizeS*0.8, g.th.accent)
			delete(groups, group)
		}
	}
	legend := []struct{ label, kind string }{{"Family", "family"}, {"Friend", "friend"}, {"Like", "like"}, {"Love", "love"}, {"Rival / enemy", "enemy"}, {"Other", "other"}}
	lx, ly := float64(30), float64(profileBand+28)
	for _, item := range legend {
		gfx.StrokeLine(dst, float32(lx), float32(ly+7), float32(lx+23), float32(ly+7), 3, chartColor(item.kind), true)
		g.text(dst, item.label, lx+30, ly, sizeS*0.7, g.th.text)
		lx += 30 + gfx.TextWidth(item.label, sizeS*0.7) + 22
	}
	g.text(dst, "Arrow: one-way", lx, ly, sizeS*0.7, g.th.text)
	gfx.FillRect(dst, 0, float32(sh-47), float32(sw), 47, withAlpha(g.th.window, 0xe0), false)
	g.text(dst, lang.T("Tab: ページ   矢印: 選択   Z: 注目/詳細   X: 全体/もどる   F6: とじる"), 24, sh-36, sizeS*0.85, g.th.text)
}
