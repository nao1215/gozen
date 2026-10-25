package play

import (
	"fmt"
	"image/color"
	"maps"
	"math"

	lang "github.com/nao1215/gozen/internal/text"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/vector"

	data "github.com/nao1215/gozen/internal/layer2/gamedata"
	"github.com/nao1215/gozen/internal/layer3/dungeon"
	"github.com/nao1215/gozen/internal/layer5/gfx"
	"github.com/nao1215/gozen/internal/rpg"
	"github.com/nao1215/gozen/internal/world"
)

type dungeonScene struct {
	space      *spaceView
	view, prev *ebiten.Image
	trans      string
	transT     int
	holdT      int
	upHeld     bool

	keep       map[string]bool
	age        int
	busy       bool
	shownFloor float64
	bannerT    int
	// vs and is are the vertex and index buffers the view reuses frame after frame
	// (DrawTriangles copies them), so drawing the walls allocates nothing.
	vs []ebiten.Vertex
	is []uint16
}

const transTicks = 10

func newDungeonScene(g *Game) *dungeonScene {
	dungeon.Ensure(g.db, g.st)
	g.enterAt = g.st.Location
	return &dungeonScene{trans: "fade", transT: transTicks * 2}
}

func (s *dungeonScene) arrive() {
	if s.space != nil {
		s.space.remaining = 0
	}
	s.age = 0
	s.snapshot()
	s.trans, s.transT = "fade", transTicks*2
}

func (s *dungeonScene) snapshot() {
	if s.view == nil {
		return
	}
	if s.prev == nil || s.prev.Bounds() != s.view.Bounds() {
		if s.prev != nil {
			s.prev.Deallocate()
		}
		s.prev = ebiten.NewImage(s.view.Bounds().Dx(), s.view.Bounds().Dy())
	}
	s.prev.Clear()
	s.prev.DrawImage(s.view, nil)
}

func (s *dungeonScene) update(g *Game) {
	s.age++
	if s.space != nil {
		s.space.remaining = max(0, s.space.remaining-1)
	}
	if !g.talk.active && len(g.stack) == 1 {
		s.bannerT = max(0, s.bannerT-1)
	}
	s.transT = max(0, s.transT-1)

	freshUp := g.in.held[btnUp] && !s.upHeld || g.in.wheel > 0
	s.upHeld = g.in.held[btnUp]
	if g.pump() {
		s.busy = true
		return
	}
	if freshUp && g.st != nil {
		g.st.Bumped = ""
	}
	s.busy = false
	dungeon.SettleRoamers(g.st)
	if g.enterAt != "" {
		loc := g.enterAt
		g.enterAt = ""
		if g.st.Location == loc {
			cmds := g.startArrival(false)
			if len(cmds) > 0 {
				return
			}
		}
	}
	if cmds := world.Auto(g.db, g.st); cmds != nil {
		g.events.Start(cmds)
		return
	}
	if cmds := dungeon.Hunters(g.db, g.st); cmds != nil {
		g.events.Start(cmds)
		return
	}
	if g.in.pressed[btnMenu] && !g.in.held[btnOK] {
		g.sfx("decide")
		g.push(newMenu(g))
		return
	}
	if s.space != nil && s.space.remaining > 0 {
		return
	}
	if s.transT > transTicks/2 {
		return
	}
	in := &g.in
	if in.held[btnUp] {
		s.holdT++
	} else {
		s.holdT = 0
	}
	sw, _ := g.screen()
	mx := float64(in.mouse.X)
	switch {
	case in.up() && in.wheel == 0, s.holdT > 14 && s.transT == 0:
		s.forward(g)
	case in.down() && in.wheel == 0:
		s.turn(g, 2, "back")
	case in.left(), in.click && mx < sw/3:
		s.turn(g, -1, "left")
	case in.right(), in.click && mx > sw*2/3:
		s.turn(g, 1, "right")
	case in.ok(), in.click:
		s.keep = maps.Clone(g.st.Done)
		if cmds := dungeon.Look(g.db, g.st); cmds != nil {
			g.sfx("decide")
			g.events.Start(cmds)
		} else if in.click {
			s.forward(g)
		}
	}
}

func (s *dungeonScene) forward(g *Game) {
	isSpace := dungeon.Of(g.db, g.st).Space != nil
	if !isSpace && !g.cfg.DungeonSnap {
		s.snapshot()
	}
	was := g.st.Pos
	s.keep = maps.Clone(g.st.Done)
	moved, cmds := dungeon.Forward(g.db, g.st)
	if moved {
		s.trans, s.transT = "forward", transTicks
		now := g.st.Pos
		if now.Facing != was.Facing || abs(now.X-was.X)+abs(now.Y-was.Y) > 2 {
			s.trans, s.transT = "fade", transTicks*2
		}
		if isSpace {
			s.spaceMotion(was, 0)
			s.transT = 0
		}
		g.sfx("step")
	} else {
		s.trans, s.transT = "bump", transTicks
		if cmds != nil {
			g.sfx("decide")
			s.holdT = 0
		} else {
			g.sfx("cancel")
		}
	}
	g.events.Start(cmds)
}

func (s *dungeonScene) turn(g *Game, n int, how string) {
	if dungeon.Of(g.db, g.st).Space != nil {
		s.spaceMotion(g.st.Pos, float64(n)*math.Pi/2)
		dungeon.Turn(g.st, n)
		g.sfx("cursor")
		return
	}
	if !g.cfg.DungeonSnap {
		s.snapshot()
	}
	dungeon.Turn(g.st, n)
	s.trans, s.transT = how, transTicks
	g.sfx("cursor")
}

func (s *dungeonScene) draw(g *Game, dst *ebiten.Image) {
	sw, sh := g.screen()
	d := dungeon.Styled(g.db, g.st)
	if d == nil {
		return
	}
	pw, ph := g.pixels()
	if s.view == nil || s.view.Bounds().Dx() != int(pw) || s.view.Bounds().Dy() != int(ph) {
		if s.view != nil {
			s.view.Deallocate()
		}
		if s.prev != nil {
			s.prev.Deallocate()
			s.prev = nil
		}
		s.view = ebiten.NewImage(int(pw), int(ph))
	}
	if g.cfg.DungeonSnap && s.prev != nil {
		s.prev.Deallocate()
		s.prev = nil
	}
	s.view.Clear()
	vst := g.st
	if s.keep != nil && g.busy() {
		c := *g.st
		c.Done = s.keep
		vst = &c
	} else {
		s.keep = nil
	}
	if d.Space != nil {
		s.drawSpace(g, s.view, d)
		dst.DrawImage(s.view, nil)
	} else {
		v := &viewer{g: g, d: d, st: vst, dst: s.view, w: pw, h: ph, vs: s.vs, is: s.is}
		v.draw()
		s.vs, s.is = v.vs, v.is
		s.drawWithTransition(dst, pw, ph, pw/sw)
	}
	if d.Generate != nil {
		if f, ok := g.st.Vars[dungeon.VarFloor].(float64); ok && f != g.bannerFloor {
			g.bannerFloor = f
			s.shownFloor, s.bannerT = f, 150
		}
	}
	if s.bannerT > 0 && !g.talk.active && g.cg.File == "" && len(g.stack) == 1 {
		s.drawBanner(g, dst, sw, sh)
	}
	if g.talk.active || g.cg.File != "" || len(g.stack) > 1 {
		return
	}
	loc := g.db.Locations[g.st.Location]

	g.window(dst, 36, 28, 400, 150)
	area := loc.Area
	if d.Space != nil {
		area = fmt.Sprintf("Level %d | %s", g.st.Pos.Level+1, loc.Area)
	}
	if a := lookArea(g); a != "" {
		area = a
	}
	if area != "" {
		g.text(dst, gfx.Ellipsis(area, sizeS, 220), 64, 42, sizeS, g.th.border)
	}
	g.text(dst, loc.Name, 64, 70, sizeL, g.th.text)
	compass := []string{lang.T("北"), lang.T("東"), lang.T("南"), lang.T("西")}[g.st.Pos.Facing&3]
	gfx.DrawTextRight(dst, fmt.Sprintf(lang.T("%sを むいている"), compass), 416, 46, sizeS*0.85, g.th.cursor)
	if !d.NoMap {
		s.drawMap(g, dst, d, sw-36-232, 28, 232, true)
	} else if len(dungeon.HuntersOn(g.db, g.st)) > 0 {
		s.drawMap(g, dst, d, sw-36-232, 28, 232, false)
	}

	if d.Generate != nil {
		if f, ok := g.st.Vars[dungeon.VarFloor].(float64); ok {
			gfx.DrawTextRight(dst, fmt.Sprintf("B%dF", int(f)), 416, 74, sizeL, g.th.accent)
			if e, ok := g.st.Vars[dungeon.VarExplored].(float64); ok {
				gfx.DrawTextRight(dst, fmt.Sprintf(lang.T("探索 %d%%"), int(e)), 416, 116, sizeS*0.8, g.th.cursor)
			}
		}
	}
	if loc.Encounters.Gauge > 0 {
		x, y, w := sw-36-232, 28+232+14.0, 232.0
		g.window(dst, x, y, w, 44)
		v := dungeon.Gauge(g.st)
		c := color.NRGBA{0x6c, 0xd8, 0x8c, 0xff}
		switch {
		case v > 0.7:
			c = color.NRGBA{0xff, 0x60, 0x60, 0xff}
		case v > 0.4:
			c = color.NRGBA{0xff, 0xc8, 0x50, 0xff}
		}
		g.text(dst, lang.T("けはい"), x+14, y+10, sizeS*0.8, g.th.dim)
		bar(dst, x+82, y+18, w-100, 10, v, c)
	}

	if ms := g.shownMeters("", false); len(ms) > 0 {
		my := 28 + 232 + 14.0
		if loc.Encounters.Gauge > 0 {
			my += 44 + 8
		}
		g.drawMeters(dst, ms, sw-36-232, my, 232)
	}

	front := dungeon.Front(g.db, g.st)

	spent := front.Tile != nil && front.Tile.Once && front.Tile.After == "" && dungeon.Spent(g.st, g.st.Location, front.X, front.Y)
	if t := front.Tile; t != nil && t.Name != "" && !spent {

		hint := t.Name
		if t.Hint != "" {
			hint += fmt.Sprintf(lang.T("　（%s）"), t.Hint)
		}
		w := gfx.TextWidth(hint, sizeM) + 60
		y := sh - 220
		if t.Flat && !front.Solid {
			y = sh - 340
		}
		g.window(dst, (sw-w)/2, y, w, 56)
		gfx.DrawText(dst, hint, sw/2, y+12, sizeM, g.th.text)
	}
	g.drawPartyHUD(dst, sh-140)
	g.text(dst, lang.T("↑すすむ ←→むきをかえる ↓ふりむく"), 62, 116, sizeS*0.72, g.th.border)
	g.text(dst, lang.T("X メニュー"), 62, 142, sizeS*0.72, g.th.border)
}

func (s *dungeonScene) drawWithTransition(dst *ebiten.Image, sw, sh, ui float64) {
	if s.transT == 0 || s.prev == nil {
		dst.DrawImage(s.view, nil)
		return
	}
	k := float64(s.transT) / transTicks
	if s.trans == "fade" {
		k /= 2
	}
	op := &ebiten.DrawImageOptions{Filter: ebiten.FilterLinear}
	switch s.trans {
	case "forward":
		dst.DrawImage(s.view, nil)
		z := 1 + (1-k)*0.35
		op.GeoM.Translate(-sw/2, -sh/2)
		op.GeoM.Scale(z, z)
		op.GeoM.Translate(sw/2, sh/2)
		op.ColorScale.ScaleAlpha(float32(k))
		dst.DrawImage(s.prev, op)
	case "left", "right":
		dir := 1.0
		if s.trans == "right" {
			dir = -1
		}
		p := 1 - k
		e := 1 - (1-p)*(1-p)
		op.GeoM.Translate(dir*sw*e, 0)
		dst.DrawImage(s.prev, op)
		op = &ebiten.DrawImageOptions{Filter: ebiten.FilterLinear}
		op.GeoM.Translate(-dir*sw*(1-e), 0)
		dst.DrawImage(s.view, op)
	case "bump":
		op.GeoM.Translate(math.Sin(k*math.Pi*3)*10*k*ui, 0)
		dst.DrawImage(s.view, op)
	default:
		dst.DrawImage(s.view, nil)
		op.ColorScale.ScaleAlpha(float32(k))
		dst.DrawImage(s.prev, op)
	}
}

func (s *dungeonScene) drawMap(g *Game, dst *ebiten.Image, d *data.Dungeon, x, y, size float64, cells bool) {
	g.window(dst, x, y, size, size)
	const most = 15
	w, h := d.Size()
	nx, ny := min(w, most), min(h, most)
	n := max(nx, ny, 9)
	cell := (size - 24) / float64(n)
	p := g.st.Pos

	fx := min(max(0, p.X-nx/2), w-nx)
	fy := min(max(0, p.Y-ny/2), h-ny)
	ox := x + 12 + float64(n-nx)*cell/2
	oy := y + 12 + float64(n-ny)*cell/2
	floor := color.NRGBA{0xf4, 0xe4, 0xec, 0xe0}
	wall := color.NRGBA{0x5a, 0x44, 0x58, 0xff}
	door := color.NRGBA{0xc8, 0x8a, 0x4a, 0xff}
	shown := dungeon.Shown(g.db, g.st)
	for my := fy; my < fy+ny; my++ {
		for mx := fx; mx < fx+nx; mx++ {
			if i := my*w + mx; !cells || i >= len(shown) || !shown[i] {
				continue
			}
			c := dungeon.CellAt(d, g.st, mx, my)
			px, py := ox+float64(mx-fx)*cell, oy+float64(my-fy)*cell
			switch c.Look {
			case "wall":
				rect(dst, px, py, cell, cell, wall)
			case "door":
				rect(dst, px+1, py+1, cell-2, cell-2, floor)
				rect(dst, px+cell*0.25, py+cell*0.25, cell*0.5, cell*0.5, door)
			default:
				rect(dst, px+1, py+1, cell-2, cell-2, floor)
			}
			if c.Tile != nil && c.Tile.Mark.Valid {
				mark, ink := c.Tile.Mark.NRGBA, colorBlack
				if c.Tile.Once && c.Tile.After == "" && dungeon.Spent(g.st, g.st.Location, mx, my) {
					mark = spentMark(mark)
					ink = color.NRGBA{0xa0, 0xa0, 0xa0, 0xff}
				}
				if c.Tile.Symbol != "" {
					rect(dst, px, py, cell, cell, mark)
					sz := max(cell*0.9, 11)
					gfx.DrawText(dst, c.Tile.Symbol, px+cell/2, py+cell/2-sz*0.5, sz, ink)
				} else {
					gfx.FillRoundRect(dst, float32(px+cell*0.2), float32(py+cell*0.2), float32(cell*0.6), float32(cell*0.6), float32(cell*0.3), mark)
				}
			}
		}
	}

	for _, r := range dungeon.HuntersOn(g.db, g.st) {
		k := d.Roamers[r.Kind]
		hx := min(max(r.X, fx), fx+nx-1)
		hy := min(max(r.Y, fy), fy+ny-1)
		px, py := ox+float64(hx-fx)*cell, oy+float64(hy-fy)*cell
		sz := max(cell*1.25, 22)
		mx, my := px+cell/2-sz/2, py+cell/2-sz/2
		mark := k.Mark.Or(color.NRGBA{0xc0, 0x90, 0xff, 0xff})
		gfx.FillRoundRect(dst, float32(mx-2), float32(my-2), float32(sz+4), float32(sz+4), float32(sz/2+2), mark)
		if k.Icon.File != "" {
			img := g.pics.get(k.Icon.File)
			iw, ih := float64(img.Bounds().Dx()), float64(img.Bounds().Dy())
			op := &ebiten.DrawImageOptions{Filter: ebiten.FilterLinear}
			op.GeoM.Scale(sz/iw, sz/ih)
			op.GeoM.Translate(mx, my)
			tint(&op.ColorScale, k.Icon)
			gfx.DrawImage(dst, img, op)
		}
	}

	cx, cy := ox+(float64(p.X-fx)+0.5)*cell, oy+(float64(p.Y-fy)+0.5)*cell
	a := float64(p.Facing) * math.Pi / 2
	pt := func(f, s float64) (float32, float32) {
		return float32(cx + (f*math.Sin(a)+s*math.Cos(a))*cell), float32(cy + (-f*math.Cos(a)+s*math.Sin(a))*cell)
	}

	x0, y0 := pt(0.62, 0)
	x1, y1 := pt(-0.45, -0.48)
	x2, y2 := pt(-0.45, 0.48)
	triangle(dst, x0, y0, x1, y1, x2, y2, color.NRGBA{0x20, 0x10, 0x10, 0xff})
	x0, y0 = pt(0.45, 0)
	x1, y1 = pt(-0.32, -0.33)
	x2, y2 = pt(-0.32, 0.33)
	triangle(dst, x0, y0, x1, y1, x2, y2, color.NRGBA{0xff, 0x40, 0x40, 0xff})
}

func spentMark(c color.NRGBA) color.NRGBA {
	gray := (int(c.R) + int(c.G) + int(c.B)) / 3
	f := func(v uint8) uint8 { return uint8(min(255, max(0, (int(v)+gray)/2*45/100))) }
	return color.NRGBA{f(c.R), f(c.G), f(c.B), c.A}
}

type viewer struct {
	g     *Game
	d     *data.Dungeon
	st    *rpg.State
	dst   *ebiten.Image
	w, h  float64
	focal float64
	cells map[[2]int]dungeon.Cell
	// vs and is are reused by every draw of the view (the scene keeps them between frames).
	vs []ebiten.Vertex
	is []uint16
}

const (
	nearZ   = 0.2
	eyeBack = 0.1
	horizon = 0.42
)

func (v *viewer) cell(f, s int) dungeon.Cell {
	k := [2]int{f, s}
	if c, ok := v.cells[k]; ok {
		return c
	}
	x, y := dungeon.Rel(v.st.Pos, f, s)
	c := dungeon.CellAt(v.d, v.st, x, y)
	if f == 0 && s == 0 && block(c) {
		c.Look = "floor"
	}
	v.cells[k] = c
	return c
}

func block(c dungeon.Cell) bool { return c.Look == "wall" || c.Look == "door" }

func (v *viewer) px(x, z float64) float32 { return float32(v.w/2 + x*v.focal/(z+eyeBack)) }
func (v *viewer) py(y, z float64) float32 { return float32(v.h*horizon - y*v.focal/(z+eyeBack)) }
func (v *viewer) fog(z float64) float32 {
	return float32(math.Max(0, math.Min(1, math.Exp(-v.d.Dark*0.55*(z+eyeBack-1)))))
}

const near = nearZ - eyeBack

func (v *viewer) draw() {
	v.focal = v.h * 0.9
	v.cells = map[[2]int]dungeon.Cell{}
	depth := v.d.Depth
	ceil := v.d.Ceiling.Or(color.NRGBA{0x3a, 0x2c, 0x40, 0xff})
	floor := v.d.Floor.Or(color.NRGBA{0x8a, 0x6a, 0x78, 0xff})

	far := v.fog(float64(depth) + 1)
	v.quad(0, 0, float32(v.w), 0, float32(v.w), float32(v.h*horizon), 0, float32(v.h*horizon), shadeOf(ceil, 0.5*far), shadeOf(ceil, 0.5*far), shadeOf(ceil, far*0.3), shadeOf(ceil, far*0.3))
	v.quad(0, float32(v.h*horizon), float32(v.w), float32(v.h*horizon), float32(v.w), float32(v.h), 0, float32(v.h), shadeOf(floor, far*0.3), shadeOf(floor, far*0.3), shadeOf(floor, far*0.6), shadeOf(floor, far*0.6))
	for f := depth; f >= -1; f-- {
		span := max(1, f+1)

		for s := -span; s <= span; s++ {
			c := v.cell(f, s)
			if block(c) {
				continue
			}
			mx, my := dungeon.Rel(v.st.Pos, f, s)
			checker := float32(1)
			if (mx+my)%2 == 0 {
				checker = 0.86
			}
			fc, cc := shadeOf(floor, checker), shadeOf(ceil, checker)
			if v.d.FloorTex.File != "" {
				fc = shadeOf(color.NRGBA{0xff, 0xff, 0xff, 0xff}, 0.8+checker/5)
			}
			if v.d.CeilingTex.File != "" {
				cc = shadeOf(color.NRGBA{0xff, 0xff, 0xff, 0xff}, 0.8+checker/5)
			}
			v.flat(float64(s), float64(f), -0.5, fc, v.d.FloorTex)
			v.flat(float64(s), float64(f), 0.5, cc, v.d.CeilingTex)
			if t := c.Tile; t != nil && t.Flat && t.Picture.File != "" {
				rot := 0
				if i := data.FacingIndex(t.Slide); i >= 0 {
					rot = i
				}
				if t.Away {
					rot = -1
				}
				v.floorDecal(v.d.Pic(t.Picture), float64(s), float64(f), t.Size, rot)
			} else if t != nil && (t.Slide != "" || t.Spin != "") {
				v.floorSign(t, floor, float64(s), float64(f))
			}
		}

		for o := span; o >= 1; o-- {
			for _, s := range []int{-o, o} {
				c := v.cell(f, s)
				if !block(c) {
					continue
				}
				in := s + 1
				if s > 0 {
					in = s - 1
				}
				if block(v.cell(f, in)) {
					continue
				}

				x := float64(s) + 0.5
				ax, az, bx, bz := x, float64(f), x, float64(f+1)
				if s > 0 {
					x = float64(s) - 0.5
					ax, az, bx, bz = x, float64(f+1), x, float64(f)
				}
				v.face(c, ax, az, bx, bz, 0.72, false)
			}
		}

		for o := span; o >= 0; o-- {
			for _, s := range []int{-o, o} {
				if o == 0 && s > 0 {
					continue
				}
				if f <= 0 && s == 0 {
					continue
				}
				c := v.cell(f, s)
				if c.Tile != nil && c.Tile.Picture.File != "" && !block(c) && !c.Tile.Flat {
					v.sprite(c.Tile, float64(s), float64(f)+0.5)
				}
			}
		}

		if f < 0 {
			continue
		}
		for s := -span; s <= span; s++ {
			c := v.cell(f, s)
			if block(c) && !block(v.cell(f-1, s)) {
				v.face(c, float64(s)-0.5, float64(f), float64(s)+0.5, float64(f), 1, true)
			}
		}
	}
}

func (v *viewer) face(c dungeon.Cell, ax, az, bx, bz float64, shade float32, front bool) {
	pic := v.d.Wall

	var over data.Picture
	if c.Look == "door" {
		pic = v.d.Door
		if c.Tile != nil && c.Tile.Picture.File != "" {
			pic, over = v.d.Wall, v.d.Pic(c.Tile.Picture)
		}
	}
	t0, t1 := 0.0, 1.0
	if az < near && bz < near {
		return
	}
	if az < near {
		t0 = (near - az) / (bz - az)
	}
	if bz < near {
		t1 = (near - az) / (bz - az)
	}

	n := 1
	tAt := func(i int) float64 { return t0 + (t1-t0)*float64(i)/float64(n) }
	if az != bz {
		pt := func(t float64) float64 { return float64(v.px(ax+(bx-ax)*t, az+(bz-az)*t)) }
		s0, s1 := pt(t0), pt(t1)
		n = int(min(300, max(1, math.Abs(s1-s0)/6)))
		tAt = func(i int) float64 {
			if i == 0 {
				return t0
			}
			if i == n {
				return t1
			}

			k := (s0 + (s1-s0)*float64(i)/float64(n) - v.w/2) / v.focal
			den := k*(bz-az) - (bx - ax)
			if den == 0 {
				return t0 + (t1-t0)*float64(i)/float64(n)
			}
			return min(t1, max(t0, (ax-k*(az+eyeBack))/den))
		}
	}
	base := color.NRGBA{0x9a, 0x7a, 0x92, 0xff}
	if c.Look == "door" && over.File == "" {
		base = color.NRGBA{0xa8, 0x70, 0x40, 0xff}
	}
	v.strips(pic, base, n, tAt, ax, az, bx, bz, shade)
	if over.File != "" {
		if b := v.g.pics.get(over.File).Bounds(); math.Abs(float64(b.Dx())/float64(b.Dy())-1) < 0.1 {
			v.strips(over, base, n, tAt, ax, az, bx, bz, shade)
		} else if front {
			v.standing(over, (ax+bx)/2, az)
		}
	}

	if front && c.Tile != nil && c.Tile.Look == "wall" && c.Tile.Picture.File != "" {
		v.decal(c.Tile, (ax+bx)/2, az)
	}
}

func (v *viewer) strips(pic data.Picture, base color.NRGBA, n int, tAt func(int) float64, ax, az, bx, bz float64, shade float32) {
	var img *ebiten.Image
	if pic.File != "" {
		img = v.g.pics.get(pic.File)
		base = color.NRGBA{0xff, 0xff, 0xff, 0xff}
		if pic.Tint.Valid {
			base = pic.Tint.NRGBA
		}
		if brightness := pic.Brightness(); brightness != 1 {
			base = shadeOf(base, float32(brightness))
		}
	}

	rows := 1
	if n > 1 {
		rows = 16
	}
	vs, is := v.vs[:0], v.is[:0]
	for i := 0; i <= n; i++ {
		t := tAt(i)
		x, z := ax+(bx-ax)*t, az+(bz-az)*t
		col := shadeOf(base, v.fog(z)*shade)
		u := float32(t)
		sx := v.px(x, z)
		for j := 0; j <= rows; j++ {
			f := float64(j) / float64(rows)
			vx := ebiten.Vertex{DstX: sx, DstY: v.py(0.5-f, z), ColorR: float32(col.R) / 255, ColorG: float32(col.G) / 255, ColorB: float32(col.B) / 255, ColorA: 1}
			if img != nil {
				tw := float32(img.Bounds().Dx())
				vx.SrcX = 0.5 + u*(tw-1)
				vx.SrcY = float32(f * float64(img.Bounds().Dy()))
			} else {
				vx.SrcX, vx.SrcY = 1, 1
			}
			vs = append(vs, vx)
		}
		if i > 0 {
			for j := range rows {
				a := uint16((i-1)*(rows+1) + j) //nolint:gosec // The strip uses a small fixed subdivision count, at most 16 rows.
				b := uint16(i*(rows+1) + j)     //nolint:gosec // The strip uses a small fixed subdivision count, at most 16 rows.
				is = append(is, a, a+1, b, a+1, b+1, b)
			}
		}
	}
	src := img
	if src == nil {
		src = whiteImage
	}
	v.dst.DrawTriangles(vs, is, src, &ebiten.DrawTrianglesOptions{Filter: ebiten.FilterLinear})
	v.vs, v.is = vs, is
}

func (v *viewer) standing(pic data.Picture, x, z float64) {
	img := v.g.pics.get(pic.File)
	iw, ih := float64(img.Bounds().Dx()), float64(img.Bounds().Dy())
	ww, hh := 0.95, 0.95*ih/iw
	if hh > 0.95 {
		ww, hh = 0.95*iw/ih, 0.95
	}
	s := float64(v.px(x+ww/2, z)-v.px(x-ww/2, z)) / iw
	op := &ebiten.DrawImageOptions{Filter: ebiten.FilterLinear}
	op.GeoM.Scale(s, s)
	op.GeoM.Translate(float64(v.px(x-ww/2, z)), float64(v.py(hh-0.5, z)))
	tint(&op.ColorScale, pic)
	f := v.fog(z)
	op.ColorScale.Scale(f, f, f, 1)
	v.dst.DrawImage(img, op)
}

func (v *viewer) decal(t *data.Tile, x, z float64) {
	pic := v.d.Pic(t.Picture)
	img := v.g.pics.get(pic.File)
	iw, ih := float64(img.Bounds().Dx()), float64(img.Bounds().Dy())
	hh := math.Min(t.Size, 0.9)
	ww := hh * iw / ih
	if ww > 0.9 {
		ww, hh = 0.9, 0.9*ih/iw
	}
	s := float64(v.px(x+ww/2, z)-v.px(x-ww/2, z)) / iw
	op := &ebiten.DrawImageOptions{Filter: ebiten.FilterLinear}
	op.GeoM.Scale(s, s)
	op.GeoM.Translate(float64(v.px(x-ww/2, z)), float64(v.py(hh/2, z)))
	tint(&op.ColorScale, t.Picture)
	f := v.fog(z)
	op.ColorScale.Scale(f, f, f, 1)
	v.dst.DrawImage(img, op)
}

func (v *viewer) sprite(t *data.Tile, x, z float64) {
	if z < near {
		return
	}
	img := v.g.pics.get(v.d.Pic(t.Picture).File)
	iw, ih := float64(img.Bounds().Dx()), float64(img.Bounds().Dy())
	hpx := t.Size * v.focal / (z + eyeBack)
	s := hpx / ih
	cx := float64(v.px(x, z))

	bottom := float64(v.py(-0.5, z))
	op := &ebiten.DrawImageOptions{Filter: ebiten.FilterLinear}
	op.GeoM.Scale(s, s)
	op.GeoM.Translate(cx-iw*s/2, bottom-hpx)
	tint(&op.ColorScale, t.Picture)
	f := v.fog(z)
	op.ColorScale.Scale(f, f, f, 1)
	v.dst.DrawImage(img, op)
}

func (v *viewer) flat(x, z, y float64, col color.NRGBA, pic data.Picture) {
	z0, z1 := math.Max(z, near), z+1
	if z1 <= near {
		return
	}
	if pic.File == "" {
		c0, c1 := shadeOf(col, v.fog(z0)), shadeOf(col, v.fog(z1))
		v.quad(v.px(x-0.5, z1), v.py(y, z1), v.px(x+0.5, z1), v.py(y, z1), v.px(x+0.5, z0), v.py(y, z0), v.px(x-0.5, z0), v.py(y, z0), c1, c1, c0, c0)
		return
	}

	img := v.g.pics.get(pic.File)
	if pic.Tint.Valid {
		col = color.NRGBA{uint8(uint16(col.R) * uint16(pic.Tint.R) / 255), uint8(uint16(col.G) * uint16(pic.Tint.G) / 255), uint8(uint16(col.B) * uint16(pic.Tint.B) / 255), 0xff} //nolint:gosec // The product of two bytes divided by 255 is in [0,255].
	}
	if brightness := pic.Brightness(); brightness != 1 {
		col = shadeOf(col, float32(brightness))
	}
	tw, th := float32(img.Bounds().Dx()), float32(img.Bounds().Dy())

	fx0, fy0 := dungeon.Rel(v.st.Pos, 1, 0)
	rx0, ry0 := dungeon.Rel(v.st.Pos, 0, 1)
	fwx, fwy := float64(fx0-v.st.Pos.X), float64(fy0-v.st.Pos.Y)
	rtx, rty := float64(rx0-v.st.Pos.X), float64(ry0-v.st.Pos.Y)
	mx, my := dungeon.Rel(v.st.Pos, int(z), int(x))
	const n = 8
	t0 := (z0 - z)
	vs, is := v.vs[:0], v.is[:0]
	for j := 0; j <= n; j++ {
		fz := t0 + (1-t0)*float64(j)/n
		zz := z + fz
		c := shadeOf(col, v.fog(zz))
		for i := 0; i <= n; i++ {
			fx := float64(i) / n
			vx := vert(v.px(x-0.5+fx, zz), v.py(y, zz), c)
			lx, lz := fx-0.5, fz-0.5
			u, w := 0.5+lx*rtx+lz*fwx, 0.5+lx*rty+lz*fwy
			if mx&1 == 1 {
				u = 1 - u
			}
			if my&1 == 1 {
				w = 1 - w
			}
			vx.SrcX = 0.5 + float32(u)*(tw-1)
			vx.SrcY = 0.5 + float32(w)*(th-1)
			vs = append(vs, vx)
		}
		if j > 0 {
			for i := range n {
				a := uint16((j-1)*(n+1) + i)
				b := uint16(j*(n+1) + i)
				is = append(is, a, a+1, b, a+1, b+1, b)
			}
		}
	}
	v.dst.DrawTriangles(vs, is, img, &ebiten.DrawTrianglesOptions{Filter: ebiten.FilterLinear})
	v.vs, v.is = vs, is
}

func (v *viewer) floorSign(t *data.Tile, floor color.NRGBA, x, z float64) {
	mark := t.Mark.Or(color.NRGBA{0xc0, 0xc0, 0xc0, 0xff})
	mix := color.NRGBA{uint8((int(floor.R) + int(mark.R)) / 2), uint8((int(floor.G) + int(mark.G)) / 2), uint8((int(floor.B) + int(mark.B)) / 2), 0xff} //nolint:gosec // The average of two bytes is in [0,255].

	pt := func(u, w float64) (float32, float32, bool) {
		zz := z + 0.5 + w
		if zz < near {
			return 0, 0, false
		}
		return v.px(x+u, zz), v.py(-0.5, zz), true
	}
	poly := func(c color.NRGBA, ps ...[2]float64) {
		var vs []ebiten.Vertex
		for _, p := range ps {
			px, py, ok := pt(p[0], p[1])
			if !ok {
				return
			}
			vs = append(vs, vert(px, py, shadeOf(c, v.fog(z+0.5+p[1]))))
		}
		var is []uint16
		for i := 1; i+1 < len(vs); i++ {
			is = append(is, 0, uint16(i), uint16(i+1))
		}
		v.dst.DrawTriangles(vs, is, whiteImage, nil)
	}
	poly(mix, [2]float64{-0.42, 0.42}, [2]float64{0.42, 0.42}, [2]float64{0.42, -0.42}, [2]float64{-0.42, -0.42})
	ink := shadeOf(mark, 0.45)

	turn := func(p [2]float64, n int) [2]float64 {
		for range n & 3 {
			p = [2]float64{p[1], -p[0]}
		}
		return p
	}
	arrow := func(n int, scale, ox, ow float64) {
		head := [][2]float64{{0, 0.34}, {0.24, 0.04}, {-0.24, 0.04}}
		stem := [][2]float64{{-0.08, 0.04}, {0.08, 0.04}, {0.08, -0.32}, {-0.08, -0.32}}
		for _, shape := range [][][2]float64{head, stem} {
			ps := make([][2]float64, len(shape))
			for i, p := range shape {
				q := turn([2]float64{p[0] * scale, p[1] * scale}, n)
				ps[i] = [2]float64{q[0] + ox, q[1] + ow}
			}
			poly(ink, ps...)
		}
	}
	if t.Slide != "" && t.Slide != "facing" {
		arrow(data.FacingIndex(t.Slide)-v.st.Pos.Facing, 1, 0, 0)
	}
	if t.Spin != "" {
		for i := range 4 {
			off := turn([2]float64{0.22, 0}, i)
			arrow(i+2, 0.42, off[0], off[1])
		}
	}
}

func (v *viewer) floorDecal(pic data.Picture, x, z, size float64, rot int) {
	if size <= 0 || size > 1 {
		size = 1
	}
	za, zb := z+0.5-size/2, z+0.5+size/2
	z0 := math.Max(za, near)
	if zb <= z0 {
		return
	}
	img := v.g.pics.get(pic.File)
	tw, th := float32(img.Bounds().Dx()), float32(img.Bounds().Dy())
	base := color.NRGBA{0xff, 0xff, 0xff, 0xff}
	if pic.Tint.Valid {
		base = pic.Tint.NRGBA
	}
	if brightness := pic.Brightness(); brightness != 1 {
		base = shadeOf(base, float32(brightness))
	}
	fx0, fy0 := dungeon.Rel(v.st.Pos, 1, 0)
	rx0, ry0 := dungeon.Rel(v.st.Pos, 0, 1)
	fwx, fwy := float64(fx0-v.st.Pos.X), float64(fy0-v.st.Pos.Y)
	rtx, rty := float64(rx0-v.st.Pos.X), float64(ry0-v.st.Pos.Y)
	const n = 8
	vs, is := v.vs[:0], v.is[:0]
	for j := 0; j <= n; j++ {
		zz := z0 + (zb-z0)*float64(j)/n
		c := shadeOf(base, v.fog(zz))
		for i := 0; i <= n; i++ {
			lx := -size/2 + size*float64(i)/n
			lz := zz - (z + 0.5)
			dx, dy := lx*rtx+lz*fwx, lx*rty+lz*fwy
			if rot < 0 {
				dx, dy = lx, -lz
			}
			for range max(rot, 0) & 3 {
				dx, dy = dy, -dx
			}
			vx := vert(v.px(x+lx, zz), v.py(-0.5, zz), c)
			vx.SrcX = 0.5 + float32(0.5+dx/size)*(tw-1)
			vx.SrcY = 0.5 + float32(0.5+dy/size)*(th-1)
			vs = append(vs, vx)
		}
		if j > 0 {
			for i := range n {
				a := uint16((j-1)*(n+1) + i)
				b := uint16(j*(n+1) + i)
				is = append(is, a, a+1, b, a+1, b+1, b)
			}
		}
	}
	v.dst.DrawTriangles(vs, is, img, &ebiten.DrawTrianglesOptions{Filter: ebiten.FilterLinear})
	v.vs, v.is = vs, is
}

func (v *viewer) quad(x0, y0, x1, y1, x2, y2, x3, y3 float32, c0, c1, c2, c3 color.NRGBA) {
	vs := append(v.vs[:0], vert(x0, y0, c0), vert(x1, y1, c1), vert(x2, y2, c2), vert(x3, y3, c3))
	is := append(v.is[:0], 0, 1, 2, 0, 2, 3)
	v.dst.DrawTriangles(vs, is, whiteImage, nil)
	v.vs, v.is = vs, is
}

func vert(x, y float32, c color.NRGBA) ebiten.Vertex {
	return ebiten.Vertex{DstX: x, DstY: y, SrcX: 1, SrcY: 1, ColorR: float32(c.R) / 255, ColorG: float32(c.G) / 255, ColorB: float32(c.B) / 255, ColorA: float32(c.A) / 255}
}

func triangle(dst *ebiten.Image, x0, y0, x1, y1, x2, y2 float32, c color.NRGBA) {
	k := float32(gfx.Scale())
	x0, y0, x1, y1, x2, y2 = x0*k, y0*k, x1*k, y1*k, x2*k, y2*k
	var p vector.Path
	p.MoveTo(x0, y0)
	p.LineTo(x1, y1)
	p.LineTo(x2, y2)
	p.Close()
	op := &vector.DrawPathOptions{AntiAlias: true}
	op.ColorScale.ScaleWithColor(c)
	vector.FillPath(dst, &p, nil, op)
}

func shadeOf(c color.NRGBA, k float32) color.NRGBA {
	f := func(v uint8) uint8 { return uint8(math.Min(255, float64(float32(v)*k))) }
	return color.NRGBA{f(c.R), f(c.G), f(c.B), c.A}
}

var whiteImage = func() *ebiten.Image {
	img := ebiten.NewImage(3, 3)
	img.Fill(color.White)
	return img
}()

func (s *dungeonScene) drawBanner(g *Game, dst *ebiten.Image, sw, sh float64) {
	t := 150 - s.bannerT
	a := math.Min(1, float64(t)/20)
	if s.bannerT < 30 {
		a = float64(s.bannerT) / 30
	}
	y := sh*0.36 - 10*(1-math.Min(1, float64(t)/20))
	w := sw * 0.5 * math.Min(1, float64(t)/14)
	c := withAlpha(g.th.accent, uint8(200*a))
	gfx.StrokeLine(dst, float32(sw/2-w/2), float32(y-14), float32(sw/2+w/2), float32(y-14), 2, c, true)
	gfx.StrokeLine(dst, float32(sw/2-w/2), float32(y+108), float32(sw/2+w/2), float32(y+108), 2, c, true)
	name := g.db.Locations[g.st.Location].Name
	if a := lookArea(g); a != "" {
		name += lang.T("　") + a
	}
	gfx.DrawTextOutlineColor(dst, name, sw/2, y, sizeM, g.th.text, g.th.window, float32(a))
	gfx.DrawTextOutlineColor(dst, fmt.Sprintf("B%dF", int(s.shownFloor)), sw/2, y+30, 64, colorWhite, g.th.nameTag, float32(a))
}

func lookArea(g *Game) string {
	d := dungeon.Of(g.db, g.st)
	if d == nil {
		return ""
	}
	env := g.st.Env()
	for _, lk := range d.Looks {
		if ok, err := lk.When.Bool(env); err == nil && ok {
			return lk.Area
		}
	}
	return ""
}
