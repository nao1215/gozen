package play

import (
	"image"
	"image/color"
	"math"

	"github.com/hajimehoshi/ebiten/v2"

	"github.com/nao1215/gozen/internal/layer5/gfx"
	lang "github.com/nao1215/gozen/internal/text"
)

// The grid layout of the character book (ui.profile_book.layout: grid): a band on top
// with the way back and the title, a grid of face cards on the left grouped by
// affiliation (a shadow and ??? for one not met yet), and on the right the standing
// picture with the buttons of its faces under it and a board with the profile. What is
// read sits on boards; what is pressed (back, the faces, the cards) has a shape of its
// own.

const (
	gridBand    = 64.0  // the height of the top band
	gridTop     = 80.0  // where the grid and the profile begin
	gridCardW   = 84.0  // a face card
	gridCardH   = 106.0 // a face card
	gridGap     = 8.0   // between the cards
	gridHeading = 30.0  // the height of a group's heading
)

// gridCell is where a card or a heading stands in the grid, before scrolling.
type gridCell struct {
	index   int // the profile's index, -1 for a heading
	heading string
	x, y    float64
}

// gridArea is the part of the screen the grid scrolls in.
func (s *profileScreen) gridArea(g *Game) (x, y, w, h float64) {
	sw, sh := g.screen()
	return 24, gridTop, sw*0.45 - 24, sh - gridTop - 20
}

// gridCells lays the cards out in rows, a heading before each affiliation (none when no
// profile has one), and returns the height of the whole grid.
func (s *profileScreen) gridCells(g *Game) ([]gridCell, float64) {
	x0, _, w, _ := s.gridArea(g)
	cols := max(1, int((w+gridGap)/(gridCardW+gridGap)))
	var out []gridCell
	y, col := 0.0, 0
	group := "\x00"
	grouped := false
	for _, id := range s.ids {
		if g.db.Profiles[id].Affiliation != "" {
			grouped = true
		}
	}
	for i, id := range s.ids {
		if a := g.db.Profiles[id].Affiliation; grouped && a != group {
			if col > 0 {
				y += gridCardH + gridGap
				col = 0
			}
			group = a
			out = append(out, gridCell{index: -1, heading: a, x: x0, y: y})
			y += gridHeading
		}
		out = append(out, gridCell{index: i, x: x0 + float64(col)*(gridCardW+gridGap), y: y})
		col++
		if col == cols {
			col = 0
			y += gridCardH + gridGap
		}
	}
	if col > 0 {
		y += gridCardH
	}
	return out, y
}

// gridCard is the rectangle of profile i's card on the screen (scrolled).
func (s *profileScreen) gridCard(g *Game, cells []gridCell, i int) (image.Rectangle, bool) {
	_, top, _, _ := s.gridArea(g)
	for _, c := range cells {
		if c.index == i {
			y := top + c.y - s.scroll
			return image.Rect(int(c.x), int(y), int(c.x+gridCardW), int(y+gridCardH)), true
		}
	}
	return image.Rectangle{}, false
}

// gridBack is the button back, on the band's left.
func (s *profileScreen) gridBack() image.Rectangle { return image.Rect(24, 12, 24+120, 12+40) }

// gridFigure is where the standing picture stands: its center, bottom and height.
func (s *profileScreen) gridFigure(g *Game) (cx, bottom, h float64) {
	_, sh := g.screen()
	gx, _, gw, _ := s.gridArea(g)
	bx, _, _, _ := s.gridBoard(g)
	return (gx + gw + bx) / 2, sh - 76, sh - 76 - gridTop + 12
}

// gridBoard is the board of the profile, right of the picture.
func (s *profileScreen) gridBoard(g *Game) (x, y, w, h float64) {
	sw, sh := g.screen()
	return sw * 0.72, gridTop, sw*0.28 - 24, sh - gridTop - 20
}

func (s *profileScreen) gridFaceButtons(g *Game, id string) []image.Rectangle {
	cx, _, _ := s.gridFigure(g)
	_, sh := g.screen()
	return s.faceButtonsAt(s.faceLabels(g, id), cx, sh-62, 40)
}

// gridStep moves the selection to the nearest card above (dir -1) or below (dir 1).
func (s *profileScreen) gridStep(cells []gridCell, dir int) int {
	var cur gridCell
	for _, c := range cells {
		if c.index == s.sel {
			cur = c
		}
	}
	best, bestRow, bestDX := -1, 0.0, math.Inf(1)
	for _, c := range cells {
		if c.index < 0 || (dir < 0 && c.y >= cur.y) || (dir > 0 && c.y <= cur.y) {
			continue
		}
		row := math.Abs(c.y - cur.y)
		dx := math.Abs(c.x - cur.x)
		if best < 0 || row < bestRow || row == bestRow && dx < bestDX {
			best, bestRow, bestDX = c.index, row, dx
		}
	}
	if best < 0 {
		return s.sel
	}
	return best
}

// keepVisible scrolls the grid so the selected card is in view.
func (s *profileScreen) keepVisible(g *Game, cells []gridCell, height float64) {
	_, _, _, h := s.gridArea(g)
	for i, c := range cells {
		if c.index != s.sel {
			continue
		}
		top := c.y
		if i > 0 && cells[i-1].index < 0 {
			top = cells[i-1].y // show the group's heading with its first row
		}
		if top < s.scroll {
			s.scroll = top
		}
		if c.y+gridCardH > s.scroll+h {
			// The view begins at a row or a heading, so no card is cut at the top.
			want := c.y + gridCardH - h
			s.scroll = c.y
			for _, d := range cells {
				if d.y >= want && d.y < s.scroll {
					s.scroll = d.y
				}
			}
		}
	}
	// Past the end the view stops at the first row or heading that shows the last row.
	end := 0.0
	for _, c := range cells {
		if c.y >= height-h {
			end = c.y
			break
		}
	}
	s.scroll = math.Max(0, math.Min(s.scroll, end))
}

func (s *profileScreen) closeBook(g *Game) {
	g.sfx("cancel")
	g.pop()
	if s.done != nil {
		s.done()
	}
}

func (s *profileScreen) updateGrid(g *Game) {
	if g.in.cancel() || g.in.pressed[btnBook] || g.in.click && g.in.mouse.In(s.gridBack()) {
		s.closeBook(g)
		return
	}
	n := len(s.ids)
	if n == 0 {
		return
	}
	cells, height := s.gridCells(g)
	id := s.current()
	faces := s.shownFaces(g, id)
	switch {
	case g.in.left() || g.in.pressed[btnLog]:
		s.pick(g, (s.sel+n-1)%n)
	case g.in.right() || g.in.pressed[btnAuto] && !g.in.pressed[btnLeft]:
		s.pick(g, (s.sel+1)%n)
	case g.in.up():
		s.pick(g, s.gridStep(cells, -1))
	case g.in.down():
		s.pick(g, s.gridStep(cells, 1))
	case g.in.ok() && len(faces) > 1 && s.opened(g, id):
		s.face = (s.face + 1) % len(faces)
		g.sfx("cursor")
	case g.in.click:
		_, top, _, h := s.gridArea(g)
		for i := range s.ids {
			if r, ok := s.gridCard(g, cells, i); ok && g.in.mouse.In(r) && float64(r.Min.Y) >= top-1 && float64(r.Max.Y) <= top+h+1 {
				s.pick(g, i)
			}
		}
		if s.opened(g, id) {
			for i, r := range s.gridFaceButtons(g, id) {
				if g.in.mouse.In(r) && i != s.face {
					s.face = i
					g.sfx("cursor")
				}
			}
		}
	}
	s.keepVisible(g, cells, height)
	if p := s.puppet(g); p != nil {
		p.Motion.Step()
	}
	s.trace(g)
}

func (s *profileScreen) drawGrid(g *Game, dst *ebiten.Image) {
	sw, sh := g.screen()
	if g.hasMaterial() {
		light := g.lightPalette()
		previous := g.th
		defer func() { g.th = previous }()
		g.th.window, g.th.text, g.th.dim, g.th.accent, g.th.cursor = withAlpha(light.window, 0xd8), light.text, light.dim, light.accent, light.accent
		g.th.border, g.th.shadow = color.NRGBA{}, color.NRGBA{}
		fill(dst, light.paper)
		if !g.drawMaterialWallpaper(dst) {
			g.prepareMaterial(dst, "")
		}
		g.prepareMaterial(dst, "")
	} else {
		g.drawPicture(dst, g.db.Game.TitleBG, 0, 0, sw, sh, 1)
		fill(dst, withAlpha(colorBlack, 0xa8))
	}
	// The band: the way back, the title and how many are met. Read-only text sits on
	// the band's plate; only the way back is a button.
	g.drawPlate(dst, image.Rect(0, 0, int(sw), int(gridBand)))
	s.gridButton(g, dst, s.gridBack(), "‹ "+lang.T("もどる"), g.in.mouse.In(s.gridBack()), 18)
	g.text(dst, g.profileTitle(), 164, 15, sizeL, g.th.text)
	met := 0
	for _, id := range s.ids {
		if s.opened(g, id) {
			met++
		}
	}
	count := itoa(met) + " / " + itoa(len(s.ids))
	hint := lang.T("←→↑↓: えらぶ   Z: 姿   X: もどる")
	g.text(dst, count, sw-24-gfx.TextWidth(count, sizeM), 18, sizeM, g.th.text)
	g.text(dst, hint, sw-48-gfx.TextWidth(count, sizeM)-gfx.TextWidth(hint, 15), 24, 15, g.th.dim)
	cells, height := s.gridCells(g)
	x, top, w, h := s.gridArea(g)
	for _, c := range cells {
		y := top + c.y - s.scroll
		if c.index < 0 {
			if y >= top-1 && y+gridHeading <= top+h+1 {
				g.text(dst, c.heading, c.x+4, y+4, 16, g.th.dim)
				gfx.FillRect(dst, float32(c.x+4), float32(y+gridHeading-6), float32(w-8), 1, withAlpha(g.th.dim, 0x60), false)
			}
			continue
		}
		if y < top-1 || y+gridCardH > top+h+1 {
			continue
		}
		s.drawGridCard(g, dst, c.index, image.Rect(int(c.x), int(y), int(c.x+gridCardW), int(y+gridCardH)))
	}
	if height > h {
		// A thin bar at the grid's right edge shows where the view is.
		bx := x + w + 6
		gfx.FillRoundRect(dst, float32(bx), float32(top), 4, float32(h), 2, withAlpha(g.th.dim, 0x30))
		gfx.FillRoundRect(dst, float32(bx), float32(top+h*s.scroll/height), 4, float32(h*h/height), 2, withAlpha(g.th.accent, 0xa0))
	}
	if id := s.current(); id != "" {
		s.drawGridFigure(g, dst, id)
		bx, by, bw, bh := s.gridBoard(g)
		g.drawPlate(dst, image.Rect(int(bx), int(by), int(bx+bw), int(by+bh)))
		if s.opened(g, id) {
			s.drawProfileText(g, dst, id, bx, by, bw, bh)
		} else {
			g.text(dst, lang.T("？？？"), bx+24, by+16, sizeL*1.2, g.th.dim)
			s.lines(g, dst, lang.T("まだ出会っていない。"), bx+24, by+16+gfx.LineHeight(sizeL*1.2)+8, bw-48, 18, by+bh-8, g.th.dim)
		}
	}
}

// drawGridCard draws a face card: the face cut from the standing picture (its shadow
// when locked, an outline when there is no picture) and the name under it.
func (s *profileScreen) drawGridCard(g *Game, dst *ebiten.Image, i int, r image.Rectangle) {
	id := s.ids[i]
	open := s.opened(g, id)
	x, y, w, h := float64(r.Min.X), float64(r.Min.Y), float64(r.Dx()), float64(r.Dy())
	hover := g.in.mouse.In(r)
	tint := color.NRGBA{250, 252, 253, 255}
	if hover {
		tint = color.NRGBA{255, 255, 255, 255}
	}
	if !g.materialSurface(dst, x, y, w, h, tint, "card") {
		gfx.FillRoundRect(dst, float32(x), float32(y), float32(w), float32(h), 12, withAlpha(g.th.window, 0xd0))
	}
	face := w - 20
	fx, fy := x+10, y+8
	pic := ""
	if faces := g.db.ProfileFaces(id); len(faces) > 0 {
		pic = g.profilePicture(id, faces[0])
	}
	if pic != "" {
		s.drawIcon(g, dst, pic, fx, fy, face, !open)
	} else {
		drawOutline(dst, fx, fy, face, open, g.th.dim, true)
	}
	name := lang.T("？？？")
	c := g.th.dim
	if open {
		name, c = g.db.ProfileName(id), g.th.text
	}
	ls, sz := gfx.FitLines(name, 15, w-8, 1)
	if len(ls) > 0 {
		g.text(dst, ls[0], x+(w-gfx.TextWidth(ls[0], sz))/2, fy+face+6, sz, c)
	}
	edge, width := color.NRGBA{255, 255, 255, 200}, float32(1)
	if i == s.sel {
		edge, width = g.th.accent, 3
	}
	gfx.StrokeRoundRect(dst, float32(x), float32(y), float32(w), float32(h), 12, width, edge)
}

// drawOutline is the face of one without a picture: a head and shoulders, on a faint
// square in a card.
func drawOutline(dst *ebiten.Image, x, y, size float64, open bool, c color.NRGBA, square bool) {
	if square {
		gfx.FillRoundRect(dst, float32(x), float32(y), float32(size), float32(size), 10, color.NRGBA{0xff, 0xff, 0xff, 24})
	}
	a := uint8(0x70)
	if !open {
		c, a = color.NRGBA{0x10, 0x10, 0x18, 0xff}, 0xd0
	}
	gfx.FillCircle(dst, float32(x+size/2), float32(y+size*0.38), float32(size*0.2), withAlpha(c, a), true)
	gfx.FillRoundRect(dst, float32(x+size*0.2), float32(y+size*0.64), float32(size*0.6), float32(size*0.36), float32(size*0.18), withAlpha(c, a))
}

func (s *profileScreen) drawGridFigure(g *Game, dst *ebiten.Image, id string) {
	open := s.opened(g, id)
	cx, bottom, h := s.gridFigure(g)
	if p := s.puppet(g); p != nil {
		if open {
			g.drawPuppet(dst, p, cx, bottom, h, 1, false, nil)
		}
	} else if pic := g.profilePicture(id, s.faceName(g)); pic != "" {
		cs := &ebiten.ColorScale{}
		if !open {
			darken(cs)
		}
		cx, h = s.fitFigure(g, pic, cx, h)
		g.drawStanding(dst, pic, cx, bottom, h, 1, false, cs)
	} else {
		size := h * 0.3
		drawOutline(dst, cx-size/2, bottom-h*0.7, size, open, g.th.dim, false)
	}
	if !open {
		return
	}
	labels := s.faceLabels(g, id)
	for i, r := range s.gridFaceButtons(g, id) {
		s.gridButton(g, dst, r, labels[i], i == s.face, 17)
	}
}

// fitFigure centers the opaque part of the picture between the grid and the board, and
// lowers its height when that part is wider than the room there (the feet stay put).
func (s *profileScreen) fitFigure(g *Game, pic string, cx, h float64) (float64, float64) {
	img := g.pics.get(pic)
	if img == nil || img.Bounds().Dy() == 0 {
		return cx, h
	}
	gx, _, gw, _ := s.gridArea(g)
	bx, _, _, _ := s.gridBoard(g)
	room := bx - (gx + gw) - 36
	c := g.pics.content(pic)
	k := h / float64(img.Bounds().Dy())
	if w := float64(c.Dx()) * k; w > room && room > 0 {
		h *= room / w
		k = h / float64(img.Bounds().Dy())
	}
	// The picture stands centered on cx; shift it so its opaque part is.
	off := (float64(c.Min.X+c.Max.X)/2 - float64(img.Bounds().Dx())/2) * k
	return (gx+gw+bx)/2 - off, h
}

// gridButton draws a pressable pill: glass with a bright edge, its label centered, an
// accent ring when it is the one chosen.
func (s *profileScreen) gridButton(g *Game, dst *ebiten.Image, r image.Rectangle, label string, on bool, size float64) {
	r, tint := g.controlMaterial(r)
	x, y, w, h := float64(r.Min.X), float64(r.Min.Y), float64(r.Dx()), float64(r.Dy())
	radius := float32(min(18, h/2))
	if !g.materialSurface(dst, x, y, w, h, tint, "control") {
		gfx.FillRoundRect(dst, float32(x), float32(y), float32(w), float32(h), radius, tint)
	}
	edge, width := color.NRGBA{255, 255, 255, 230}, float32(1.2)
	if on {
		edge, width = g.th.accent, 2.5
	}
	gfx.StrokeRoundRect(dst, float32(x), float32(y), float32(w), float32(h), radius, width, edge)
	label = gfx.Ellipsis(label, size, w-16)
	g.text(dst, label, x+(w-gfx.TextWidth(label, size))/2, y+h/2-gfx.LineHeight(size)/2+1, size, g.th.text)
}
