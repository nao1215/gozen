package play

import (
	"fmt"
	"image"
	"image/color"
	"math"
	"slices"
	"strconv"
	"strings"

	"github.com/hajimehoshi/ebiten/v2"

	"github.com/nao1215/gozen/internal/base/expr"
	lang "github.com/nao1215/gozen/internal/base/text"
	data "github.com/nao1215/gozen/internal/model/gamedata"
	"github.com/nao1215/gozen/internal/screen/gfx"
)

func (g *Game) bestiaryInMenus() bool {
	return g.db != nil && len(g.db.Enemies) > 0 && (g.db.Game.UI.Bestiary || g.opts.Development)
}

func (g *Game) bestiaryKey() {
	if !g.in.inspect(g.in.pressed[btnBestiary]) || !g.opts.Development || !g.bestiaryInMenus() || g.memo != nil || g.replaying || g.panelModal() || len(g.stack) == 0 {
		return
	}
	if _, open := g.stack[len(g.stack)-1].(*bestiaryScreen); open {
		return
	}
	g.in.query(true)
	g.in.pressed[btnBestiary] = false
	g.sfx("decide")
	g.push(newBestiary(g, nil))
}

func (g *Game) bestiarySeen(id string) bool {
	return g.opts.Development || g.st != nil && expr.Truthy(g.st.Vars["bestiary."+id+".seen"])
}

func (g *Game) bestiaryDefeated(id string) float64 {
	if g.st == nil {
		if g.opts.Development {
			return 1
		}
		return 0
	}
	n, _ := expr.ToNumber(g.st.Vars["bestiary."+id+".defeated"])
	if g.opts.Development && n < 1 {
		return 1
	}
	return n
}

func (g *Game) bestiaryCaught(id string) float64 {
	if g.st == nil {
		return 0
	}
	n, _ := expr.ToNumber(g.st.Vars["bestiary."+id+".caught"])
	return n
}

type bestiaryScreen struct {
	ids []string
	// list is the species' list: its cursor, the rows shown and the pointer over them.
	// The screen draws the rows itself (a picture and a name).
	list *list
	done func()

	// The lineage panel (ADR 0036): the family tree of the selected species.
	lineage *data.Lineage
	choice  map[string]int
	root    string
	history []lineageView
	inTree  bool
	cur     int
}

type lineageView struct {
	species string
	cursor  int
}

func newBestiary(g *Game, done func()) *bestiaryScreen {
	s := &bestiaryScreen{ids: g.db.Order["enemies"], done: done, lineage: data.BuildLineage(g.db), choice: map[string]int{}}
	items := make([]listItem, len(s.ids))
	for i, id := range s.ids {
		items[i].label = id
	}
	s.list = &list{items: items, x: 42, y: 88, w: 252, rowH: 48, size: sizeS, press: true}
	s.layout(g)
	return s
}

// layout fits the list's rows to the screen.
func (s *bestiaryScreen) layout(g *Game) {
	_, sh := g.screen()
	s.list.rows = max(1, int((sh-100-88)/s.list.rowH))
}

// keyHint is a part of the line of keys under a screen: what it says and, when tap is set,
// the button a press on it stands for.
type keyHint struct {
	text string
	key  button
	tap  bool
}

// hints is the line of keys under the book, for the list or for the lineage.
func (s *bestiaryScreen) hints() []keyHint {
	if s.inTree {
		return []keyHint{
			{text: lang.T("やじるし: えを えらぶ")},
			{lang.T("Z: せんぞを みる"), btnOK, true},
			{lang.T("Q: ページを ひらく"), btnBack, true},
			{lang.T("Tab: べつの くみあわせ"), btnNext, true},
			{lang.T("X: もどる"), btnCancel, true},
		}
	}
	return []keyHint{
		{text: lang.T("↑↓: えらぶ")},
		{lang.T("Z: けいず"), btnOK, true},
		{lang.T("X / F4: とじる"), btnCancel, true},
	}
}

// hintGap is written between two hints of the line.
const hintGap = "    "

// hintRects are the places of the hints in the line written on the screen's foot.
func (s *bestiaryScreen) hintRects(g *Game) []image.Rectangle {
	_, sh := g.screen()
	before := ""
	var out []image.Rectangle
	for _, h := range s.hints() {
		x := 48 + gfx.TextWidth(before, sizeS*0.8)
		out = append(out, image.Rect(int(x-8), int(sh-56), int(x+gfx.TextWidth(h.text, sizeS*0.8)+8), int(sh-20)))
		before += h.text + hintGap
	}
	return out
}

// tapHints turns a click on a hint into a press of its button.
func (s *bestiaryScreen) tapHints(g *Game) {
	if !g.in.inspect(g.in.click) {
		return
	}
	hints := s.hints()
	for i, r := range s.hintRects(g) {
		if hints[i].tap && g.in.mouse.In(r) {
			g.in.pressed[hints[i].key], g.in.click = true, false
		}
	}
}
func (s *bestiaryScreen) modal() bool { return true }

// lineageTile is one picture in the two-generation view, in reading order.
type lineageTile struct {
	node       *data.LineageNode
	x, y, w, h float64
	parent     int // index of this tile's descendant in the next row
}

func (s *bestiaryScreen) lineageTiles(x, top, w, h float64) []lineageTile {
	if s.root == "" {
		return nil
	}
	root := s.lineage.Tree(s.root, s.choice)
	if root == nil {
		return nil
	}
	// Fill the panel's usable area while preserving clear branches at both sizes.
	margin := 24.0
	gap := max(28, w*0.035)
	cardW := (w - 2*margin - 3*gap) / 4
	cardH := min(150, (h-100-2*52)/3)
	left := x + (w-4*cardW-3*gap)/2
	rowStep := (h - 62 - margin - cardH) / 2
	rowY := func(row int) float64 { return top + 62 + float64(2-row)*rowStep }
	place := func(n *data.LineageNode, row int, center float64) lineageTile {
		return lineageTile{node: n, x: center - cardW/2, y: rowY(row), w: cardW, h: cardH, parent: -1}
	}
	centers := [4]float64{}
	for i := range centers {
		centers[i] = left + cardW/2 + float64(i)*(cardW+gap)
	}
	tiles := []lineageTile{place(root, 0, (centers[1]+centers[2])/2)}
	if root.Recipe.Kind == data.RecipeCapture {
		return tiles
	}
	for i, id := range root.Recipe.Parents {
		parent := s.lineage.Tree(id, s.choice)
		if parent == nil {
			continue
		}
		tile := place(parent, 1, (centers[2*i]+centers[2*i+1])/2)
		tile.parent = 0
		tiles = append(tiles, tile)
	}
	for i := 1; i < min(3, len(tiles)); i++ {
		parent := tiles[i].node
		if parent.Recipe.Kind == data.RecipeCapture {
			continue
		}
		for j, id := range parent.Recipe.Parents {
			if grand := s.lineage.Tree(id, s.choice); grand != nil {
				tile := place(grand, 2, centers[2*(i-1)+j])
				tile.parent = i
				tiles = append(tiles, tile)
			}
		}
	}
	return tiles
}

func (s *bestiaryScreen) updateTree(g *Game) {
	sw, sh := g.screen()
	tiles := s.lineageTiles(322, 104, sw-350, sh-185)
	if len(tiles) == 0 {
		s.inTree = false
		return
	}
	s.cur = min(s.cur, len(tiles)-1)
	// A click on a picture chooses it; one on the picture chosen views its ancestry.
	enter := false
	if g.in.inspect(g.in.click) {
		for i, t := range tiles {
			if !inBox(float64(g.in.mouse.X), float64(g.in.mouse.Y), t.x, t.y, t.w, t.h) {
				continue
			}
			g.in.query(true)
			if enter = i == s.cur; !enter {
				s.cur = i
				g.sfx("cursor")
			}
		}
	}
	switch {
	case g.in.cancel():
		g.sfx("cancel")
		if len(s.history) == 0 {
			s.inTree = false
		} else {
			last := s.history[len(s.history)-1]
			s.history = s.history[:len(s.history)-1]
			s.root, s.cur = last.species, last.cursor
		}
	case g.in.up() || g.in.down() || g.in.left() || g.in.right():
		if next := lineageNeighbor(tiles, s.cur, g.in.left(), g.in.right(), g.in.up(), g.in.down()); next != s.cur {
			s.cur = next
			g.sfx("cursor")
		}
	case enter || g.in.ok():
		s.history = append(s.history, lineageView{s.root, s.cur})
		s.root, s.cur = tiles[s.cur].node.Species, 0
		g.sfx("decide")
	case g.in.pressedKey(btnBack):
		if i := slices.Index(s.ids, tiles[s.cur].node.Species); i >= 0 && g.bestiarySeen(tiles[s.cur].node.Species) {
			s.list.sel, s.inTree = i, false
			s.history = nil
			g.sfx("decide")
		} else {
			g.sfx("buzzer")
		}
	case g.in.tab() != 0:
		s.nextRecipe(g)
	}
}

// nextRecipe switches only the species at the bottom of the view.
func (s *bestiaryScreen) nextRecipe(g *Game) {
	n := s.lineage.Tree(s.root, s.choice)
	if n == nil || n.Choices < 2 {
		g.sfx("buzzer")
		return
	}
	s.choice[n.Species] = (n.Choice + 1) % n.Choices
	s.cur = 0
	g.sfx("cursor")
}

func lineageNeighbor(tiles []lineageTile, current int, left, right, up, down bool) int {
	a := tiles[current]
	best, score := current, 1e12
	for i, b := range tiles {
		dx, dy := b.x+b.w/2-a.x-a.w/2, b.y+b.h/2-a.y-a.h/2
		if i == current || left && dx >= -1 || right && dx <= 1 || up && dy >= -1 || down && dy <= 1 {
			continue
		}
		if left || right {
			scoreCandidate := dx*dx + 7*dy*dy
			if scoreCandidate < score {
				best, score = i, scoreCandidate
			}
		} else {
			scoreCandidate := 5*dx*dx + dy*dy
			if scoreCandidate < score {
				best, score = i, scoreCandidate
			}
		}
	}
	return best
}

func (s *bestiaryScreen) update(g *Game) {
	s.layout(g)
	s.tapHints(g)
	if s.inTree && !g.in.inspect(g.in.pressed[btnBestiary]) {
		s.updateTree(g)
		return
	}
	if g.in.cancel() || g.in.pressedKey(btnBestiary) {
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
	// A click on a row chooses it; one on the row chosen opens its lineage, as OK does.
	was := s.list.sel
	hit := s.list.browse(g)
	if g.in.inspect(g.in.pressed[btnRight] || g.in.pressed[btnOK]) || hit >= 0 && hit == was && g.in.inspect(g.in.click) {
		g.in.query(s.openLineage(g))
	}
}

// openLineage reports whether the selected, seen species has an ancestry view.
func (s *bestiaryScreen) openLineage(g *Game) bool {
	id := s.ids[s.list.sel]
	if !g.bestiarySeen(id) || s.lineage.Tree(id, s.choice) == nil {
		return false
	}
	s.inTree, s.cur, s.root = true, 0, id
	s.history = nil
	g.sfx("decide")
	return true
}

func (s *bestiaryScreen) draw(g *Game, dst *ebiten.Image) {
	sw, sh := g.screen()
	g.drawPicture(dst, g.db.Game.TitleBG, 0, 0, sw, sh, 1)
	fill(dst, withAlpha(colorBlack, 0xb8))
	bestiaryPanel(g, dst, 20, 20, sw-40, sh-40)
	g.text(dst, lang.T("まもののほん"), 48, 40, sizeL, g.th.text)
	s.layout(g)
	l := s.list
	listX, listW, rowH := l.x, l.w, l.rowH
	bestiaryPanel(g, dst, listX, l.y+4, listW, float64(l.rows)*rowH+16)
	for _, i := range l.shown() {
		id := s.ids[i]
		y := float64(l.rowRect(i).Min.Y)
		if i == l.sel {
			gfx.FillRoundRect(dst, float32(listX+6), float32(y), float32(listW-12), float32(rowH-2), 8, withAlpha(g.th.accent, 0x70))
		}
		e := g.db.Enemies[id]
		name := "???"
		if g.bestiarySeen(id) {
			name = e.Name
			drawBestiaryImage(g, dst, e.Image.File, listX+10, y+4, 38, 38)
		}
		g.text(dst, gfx.Ellipsis(name, sizeS*0.9, listW-68), listX+56, y+11, sizeS*0.9, g.th.text)
	}
	l.drawMarks(g, dst)
	if len(s.ids) > 0 {
		if s.inTree {
			s.drawLineage(g, dst, 322, 104, sw-350, sh-185)
		} else {
			s.drawPage(g, dst, s.ids[l.sel], 322, 100, sw-350, sh-175)
		}
	}
	hints := s.hints()
	var line []string
	for _, h := range hints {
		line = append(line, h.text)
	}
	g.text(dst, gfx.Ellipsis(strings.Join(line, hintGap), sizeS*0.8, sw-100), 48, sh-48, sizeS*0.8, g.th.dim)
	for i, r := range s.hintRects(g) {
		if hints[i].tap {
			g.traceControl(hints[i].text, roundRect(float64(r.Min.X), float64(r.Min.Y), float64(r.Dx()), float64(r.Dy())), true, false, "")
		}
	}
}

func bestiaryPanel(g *Game, dst *ebiten.Image, x, y, w, h float64) {
	gfx.FillRoundRect(dst, float32(x), float32(y), float32(w), float32(h), 16, withAlpha(g.th.window, 0xff))
	g.window(dst, x, y, w, h)
}

func drawBestiaryImage(g *Game, dst *ebiten.Image, path string, x, y, w, h float64) {
	if path == "" {
		return
	}
	img := g.pics.get(path)
	if img == nil {
		return
	}
	iw, ih := float64(img.Bounds().Dx()), float64(img.Bounds().Dy())
	scale := min(w/iw, h/ih)
	op := &ebiten.DrawImageOptions{Filter: ebiten.FilterLinear}
	op.GeoM.Scale(scale, scale)
	op.GeoM.Translate(x+(w-iw*scale)/2, y+(h-ih*scale)/2)
	gfx.DrawImage(dst, img, op)
}

func (s *bestiaryScreen) drawPage(g *Game, dst *ebiten.Image, id string, x, top, w, h float64) {
	e := g.db.Enemies[id]
	bestiaryPanel(g, dst, x, top, w, h)
	figureX, figureW := x+16, min(300, w*0.36)
	textX, textW := figureX+figureW+24, w-figureW-72
	seen := g.bestiarySeen(id)
	defeated := g.bestiaryDefeated(id)
	if !seen {
		g.text(dst, "Not yet encountered", textX, top+24, sizeM, g.th.text)
		return
	}
	drawBestiaryImage(g, dst, e.Image.File, figureX, top+36, figureW, h-65)
	y := top + 22
	write := func(label string, size float64, limit int) {
		lines, fitted := gfx.FitLines(label, size, textW, limit)
		for _, line := range lines {
			if y+gfx.LineHeight(fitted) > top+h-12 {
				return
			}
			g.text(dst, line, textX, y, fitted, g.th.text)
			y += gfx.LineHeight(fitted)
		}
	}
	write(e.Name+sexSign(e.Sex), sizeL, 1)
	y += 8
	write(e.Lore, sizeS*0.85, 4)
	y += 14
	// The record as a small form: headings, dim labels and aligned values.
	labelW := min(150, textW*0.3)
	heading := func(label string) {
		y += 6
		g.text(dst, label, textX, y, sizeS*0.85, g.th.accent)
		y += gfx.LineHeight(sizeS*0.85) + 2
		gfx.FillRect(dst, float32(textX), float32(y-3), float32(textW), 1, withAlpha(g.th.dim, 0x80), false)
	}
	row := func(label, value string) {
		if y+gfx.LineHeight(sizeS*0.85) > top+h-12 {
			return
		}
		g.text(dst, label, textX, y, sizeS*0.8, g.th.dim)
		lines, fitted := gfx.FitLines(value, sizeS*0.85, textW-labelW, 2)
		for _, line := range lines {
			g.text(dst, line, textX+labelW, y, fitted, g.th.text)
			y += gfx.LineHeight(fitted)
		}
	}
	heading("Record")
	row("Defeated", strconv.Itoa(int(defeated)))
	row("Caught", strconv.Itoa(int(g.bestiaryCaught(id))))
	if e.Family != "" {
		row("Family", fmt.Sprintf("%s  (rank %d)", e.Family, e.Rank))
	}
	row("Sex", speciesSexText(e.Sex))
	heading("Species base stats")
	stats := e.Stats
	cellW := textW / 4
	for r, keys := range [2][4]string{{"mhp", "mmp", "atk", "def"}, {"mat", "mdf", "agi", "luk"}} {
		for c, k := range keys {
			label := strings.ToUpper(strings.TrimPrefix(k, "m"))
			switch k {
			case "mat", "mdf":
				label = strings.ToUpper(k)
			}
			cx := textX + float64(c)*cellW
			g.text(dst, label, cx, y+float64(r)*gfx.LineHeight(sizeS*0.9), sizeS*0.75, g.th.dim)
			g.text(dst, strconv.Itoa(stats[k]), cx+cellW*0.42, y+float64(r)*gfx.LineHeight(sizeS*0.9), sizeS*0.9, g.th.text)
		}
	}
	y += 2 * gfx.LineHeight(sizeS*0.9)
	if defeated < 1 {
		heading("Battle")
		row("Records", "Defeat this enemy to reveal battle records")
		return
	}
	var res []string
	for k, v := range e.Resist {
		label := k
		if v > 1 {
			label += " (weak)"
		} else if v < 1 {
			label += " (resists)"
		}
		res = append(res, label)
	}
	if len(res) == 0 {
		res = []string{"none recorded"}
	}
	slices.Sort(res)
	var skills []string
	for _, r := range e.AI.Rules {
		if skill := g.db.Skills[r.Use]; skill != nil {
			skills = append(skills, skill.Name)
		} else if r.Use != "" {
			skills = append(skills, r.Use)
		}
	}
	if len(skills) == 0 {
		skills = []string{"Strike"}
	}
	var drops []string
	for _, d := range e.Drops {
		if it := g.db.Items[d.Item]; it != nil {
			drops = append(drops, it.Name)
		}
	}
	if len(drops) == 0 {
		drops = []string{"none"}
	}
	heading("Battle")
	row("Elements", strings.Join(res, ", "))
	row("Skills", strings.Join(skills, ", "))
	row("Drops", strings.Join(drops, ", "))
	y += 10
	g.text(dst, "Z: view lineage", textX, min(y, top+h-12-gfx.LineHeight(sizeS*0.8)), sizeS*0.8, g.th.dim)
}

func speciesSexText(sex string) string {
	switch sex {
	case "male":
		return "male only"
	case "female":
		return "female only"
	default:
		return "male or female"
	}
}

// drawLineage shows seven pictures at most: grandparents, parents and child.
func (s *bestiaryScreen) drawLineage(g *Game, dst *ebiten.Image, x, top, w, h float64) {
	bestiaryPanel(g, dst, x, top, w, h)
	g.text(dst, "Breeding routes", x+24, top+6, sizeS*0.85, g.th.accent)
	tiles := s.lineageTiles(x, top, w, h)
	if len(tiles) == 0 {
		return
	}
	path := make([]string, 0, len(s.history))
	for _, view := range s.history {
		path = append(path, s.speciesName(g, view.species, !g.bestiarySeen(view.species)))
	}
	path = append(path, s.speciesName(g, s.root, !g.bestiarySeen(s.root)))
	crumb := gfx.Ellipsis(strings.Join(path, " > "), sizeS*0.7, w-48)
	g.text(dst, crumb, x+24, top+29, sizeS*0.7, g.th.dim)
	// Draw connectors first, so the picture cards remain legible over them.
	connect := func(a, b lineageTile) {
		cx, cy := a.x+a.w/2, a.y+a.h
		px, py := b.x+b.w/2, b.y
		mid := (cy + py) / 2
		lineageLine(dst, cx, cy, cx, mid, g.th.dim)
		lineageLine(dst, cx, mid, px, mid, g.th.dim)
		lineageLine(dst, px, mid, px, py, g.th.dim)
	}
	if len(tiles) > 1 {
		for i := 1; i < min(3, len(tiles)); i++ {
			connect(tiles[i], tiles[0])
		}
	}
	for i := 3; i < len(tiles); i++ {
		connect(tiles[i], tiles[tiles[i].parent])
	}
	for i, tile := range tiles {
		s.drawLineageTile(g, dst, tile, s.inTree && i == s.cur)
	}
}

func lineageLine(dst *ebiten.Image, x1, y1, x2, y2 float64, c color.Color) {
	if x1 == x2 {
		gfx.FillRect(dst, float32(x1-1), float32(min(y1, y2)), 2, float32(math.Abs(y2-y1)), c, false)
	} else {
		gfx.FillRect(dst, float32(min(x1, x2)), float32(y1-1), float32(math.Abs(x2-x1)), 2, c, false)
	}
}

func (s *bestiaryScreen) speciesName(g *Game, id string, hidden bool) string {
	if hidden {
		return "?"
	}
	if e := g.db.Enemies[id]; e != nil {
		if e.Sex == "" || e.Sex == "any" {
			return e.Name + " ♂/♀"
		}
		return e.Name + sexSign(e.Sex)
	}
	return id
}

func (s *bestiaryScreen) drawLineageTile(g *Game, dst *ebiten.Image, tile lineageTile, selected bool) {
	n := tile.node
	hidden := !g.bestiarySeen(n.Species)
	c := g.th.dim
	if selected {
		c = g.th.accent
	}
	gfx.FillRoundRect(dst, float32(tile.x-3), float32(tile.y-3), float32(tile.w+6), float32(tile.h+6), 9, c)
	gfx.FillRoundRect(dst, float32(tile.x), float32(tile.y), float32(tile.w), float32(tile.h), 7, g.th.window)
	if hidden {
		g.text(dst, "?", tile.x+tile.w/2-8, tile.y+22, sizeL, g.th.dim)
	} else if e := g.db.Enemies[n.Species]; e != nil {
		drawBestiaryImage(g, dst, e.Image.File, tile.x+8, tile.y+4, tile.w-16, tile.h-49)
	}
	if !hidden && tile.parent == 0 && n.Recipe.Kind != data.RecipeCapture {
		g.text(dst, "more +", tile.x+tile.w-56, tile.y+4, sizeS*0.5, g.th.accent)
	}
	g.text(dst, gfx.Ellipsis(s.speciesName(g, n.Species, hidden), sizeS*0.68, tile.w-12), tile.x+6, tile.y+tile.h-43, sizeS*0.68, g.th.text)
	if hidden {
		return
	}
	caption := ""
	switch n.Recipe.Kind {
	case data.RecipeCapture:
		place := "unknown"
		if len(n.Sites) > 0 {
			place = n.Sites[0]
		}
		caption = "Capture: " + place
	case data.RecipeFamily:
		caption = g.db.MonsterFamilies[n.Recipe.Families[0]] + " x " + g.db.MonsterFamilies[n.Recipe.Families[1]]
	case data.RecipePair:
		caption = "Parents pictured above"
	}
	g.text(dst, gfx.Ellipsis(caption, sizeS*0.53, tile.w-12), tile.x+6, tile.y+tile.h-21, sizeS*0.53, g.th.dim)
	if n.Recipe.Kind == data.RecipeFamily {
		example := "e.g. " + s.speciesName(g, n.Recipe.Parents[0], !g.bestiarySeen(n.Recipe.Parents[0])) + " + " + s.speciesName(g, n.Recipe.Parents[1], !g.bestiarySeen(n.Recipe.Parents[1]))
		g.text(dst, gfx.Ellipsis(example, sizeS*0.46, tile.w-12), tile.x+6, tile.y+tile.h-10, sizeS*0.46, g.th.dim)
	}
}

var (
	_ scene = (*bestiaryScreen)(nil)
	_ modal = (*bestiaryScreen)(nil)
)
