package play

import (
	"image"
	"image/color"
	"math"
	"slices"

	"github.com/hajimehoshi/ebiten/v2"

	data "github.com/nao1215/gozen/internal/layer2/gamedata"
	"github.com/nao1215/gozen/internal/layer5/gfx"
	lang "github.com/nao1215/gozen/internal/text"
)

// chartLayout assigns stable, separated slots, keeping each affiliation together.
func chartLayout(ids []string, profiles map[string]*data.Profile) []image.Point {
	groups := []string{}
	for _, id := range ids {
		group := profiles[id].Affiliation
		if !slices.Contains(groups, group) {
			groups = append(groups, group)
		}
	}
	points := make([]image.Point, len(ids))
	row := 0
	for _, group := range groups {
		members := []int{}
		for i, id := range ids {
			if profiles[id].Affiliation == group {
				members = append(members, i)
			}
		}
		for j, i := range members {
			points[i] = image.Pt(360+(j%2)*480, 180+(row+j/2)*210)
		}
		row += (len(members)+1)/2 + 1
	}
	return points
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
		g.sfx("cancel")
		g.pop()
		if s.done != nil {
			s.done()
		}
		return
	}
	if len(s.ids) == 0 {
		return
	}
	points := chartLayout(s.ids, g.db.Profiles)
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
		for i, p := range points {
			x, y := s.chartPoint(g, p)
			if g.in.mouse.In(image.Rect(int(x-43*s.zoom), int(y-43*s.zoom), int(x+43*s.zoom), int(y+43*s.zoom))) && s.opened(g, s.ids[i]) {
				s.sel, s.face, s.chart = i, 0, false
				g.sfx("decide")
				break
			}
		}
	}
	if g.in.ok() && s.opened(g, s.current()) {
		s.face, s.chart = 0, false
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
	fill(dst, withAlpha(colorBlack, 0xc0))
	gfx.FillRect(dst, 0, 0, float32(sw), profileBand, withAlpha(g.th.window, 0xe0), false)
	g.text(dst, lang.T("相関図"), 30, 12, sizeL, g.th.text)
	points := chartLayout(s.ids, g.db.Profiles)
	s.camX, s.camY = s.chartView(g, points)
	index := map[string]int{}
	for i, id := range s.ids {
		index[id] = i
	}
	for i, id := range s.ids {
		for _, r := range g.db.Profiles[id].Relations {
			j, ok := index[r.Who]
			if !ok || i == j || !s.chartEdgeVisible(g, id, r.Who) {
				continue
			}
			x1, y1 := s.chartPoint(g, points[i])
			x2, y2 := s.chartPoint(g, points[j])
			dx, dy := x2-x1, y2-y1
			length := math.Hypot(dx, dy)
			if length < 1 {
				continue
			}
			ux, uy := dx/length, dy/length
			// Offset each directed edge so two different calls can be read separately.
			off := 7 * s.zoom
			x1, y1 = x1-uy*off+ux*39*s.zoom, y1+ux*off+uy*39*s.zoom
			x2, y2 = x2-uy*off-ux*39*s.zoom, y2+ux*off-uy*39*s.zoom
			c := chartColor(r.Kind)
			if i != s.sel && j != s.sel {
				c.A = 65
			}
			gfx.StrokeLine(dst, float32(x1), float32(y1), float32(x2), float32(y2), float32(2.5*s.zoom), c, true)
			// A relation with no return edge is directed.
			reverse := false
			for _, back := range g.db.Profiles[r.Who].Relations {
				if back.Who == id {
					reverse = true
					break
				}
			}
			if !reverse {
				ax, ay := x2-ux*12*s.zoom, y2-uy*12*s.zoom
				gfx.StrokeLine(dst, float32(x2), float32(y2), float32(ax-uy*7*s.zoom), float32(ay+ux*7*s.zoom), 2, c, true)
				gfx.StrokeLine(dst, float32(x2), float32(y2), float32(ax+uy*7*s.zoom), float32(ay-ux*7*s.zoom), 2, c, true)
			}
			if r.Calls != "" {
				label := data.Render(r.Calls, s.env)
				g.text(dst, label, x1+(x2-x1)*0.42-gfx.TextWidth(label, sizeS*0.7)/2, y1+(y2-y1)*0.42-18, sizeS*0.7, c)
			}
		}
	}
	for i, id := range s.ids {
		x, y := s.chartPoint(g, points[i])
		if x < -100 || x > sw+100 || y < profileBand-100 || y > sh+100 {
			continue
		}
		open := s.opened(g, id)
		c := g.th.border
		if i == s.sel {
			c = g.th.cursor
		}
		size := 78 * s.zoom
		gfx.FillRoundRect(dst, float32(x-size/2-3), float32(y-size/2-3), float32(size+6), float32(size+6), 12, c)
		gfx.FillRoundRect(dst, float32(x-size/2), float32(y-size/2), float32(size), float32(size), 10, g.th.window)
		if open {
			if pic := g.db.ProfileIcon(id); pic != "" {
				bob := 0.0
				if a := g.db.Actors[g.db.ProfileActor(id)]; a != nil && a.Puppet != "" {
					bob = math.Sin(float64(s.chartTick)*0.04) * 2
				}
				g.drawFace(dst, pic, x-size/2+4, y-size/2+4+bob, size-8, 1, false)
			} else {
				g.text(dst, "?", x-8, y-17, sizeL, g.th.dim)
			}
		} else {
			g.text(dst, "?", x-8, y-17, sizeL, g.th.dim)
		}
		name := "?"
		if open {
			name = g.db.ProfileName(id)
		}
		g.text(dst, name, x-gfx.TextWidth(name, sizeS*0.9)/2, y+size/2+7, sizeS*0.9, g.th.text)
		group := g.db.Profiles[id].Affiliation
		if group != "" && (i == 0 || g.db.Profiles[s.ids[i-1]].Affiliation != group) {
			g.text(dst, group, x-size/2, y-size/2-28, sizeS*0.8, g.th.accent)
		}
	}
	gfx.FillRect(dst, 0, float32(sh-47), float32(sw), 47, withAlpha(g.th.window, 0xe0), false)
	g.text(dst, lang.T("Tab: 詳細/相関図   矢印: 選択   Z: 詳細   ホイール: 拡大縮小   X / F6: もどる"), 24, sh-36, sizeS*0.85, g.th.text)
}
