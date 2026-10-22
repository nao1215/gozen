package play

import (
	"fmt"
	"image/color"
	"math"
	"slices"
	"strings"

	"github.com/hajimehoshi/ebiten/v2"

	"github.com/nao1215/gozen/internal/expr"
	data "github.com/nao1215/gozen/internal/layer2/gamedata"
	"github.com/nao1215/gozen/internal/layer5/gfx"
)

func (g *Game) bestiaryInMenus() bool {
	return g.db != nil && len(g.db.Enemies) > 0 && (g.db.Game.UI.Bestiary || g.opts.Development)
}

func (g *Game) bestiaryKey() {
	if !g.in.pressed[btnBestiary] || !g.opts.Development || !g.bestiaryInMenus() || g.memo != nil || g.replaying || g.panelModal() || len(g.stack) == 0 {
		return
	}
	if _, open := g.stack[len(g.stack)-1].(*bestiaryScreen); open {
		return
	}
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
	ids  []string
	sel  int
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
	return &bestiaryScreen{ids: g.db.Order["enemies"], done: done, lineage: data.BuildLineage(g.db), choice: map[string]int{}}
}
func (s *bestiaryScreen) modal() bool { return true }

// lineageTile is one picture in the two-generation view, in reading order.
type lineageTile struct {
	node       *data.LineageNode
	x, y, w, h float64
	parent     int // index of the tile below which this parent is shown
}

func (s *bestiaryScreen) lineageTiles(x, top, w, h float64) []lineageTile {
	if s.root == "" {
		return nil
	}
	root := s.lineage.Tree(s.root, s.choice)
	if root == nil {
		return nil
	}
	// Four equal slots leave room for the four grandparents without horizontal scrolling.
	gap := 12.0
	cardW := min(172, (w-48-3*gap)/4)
	cardH := min(132, (h-98)/3-12)
	left := x + (w-4*cardW-3*gap)/2
	rowY := func(row int) float64 { return top + 62 + float64(row)*(h-98)/3 }
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
	case g.in.ok():
		s.history = append(s.history, lineageView{s.root, s.cur})
		s.root, s.cur = tiles[s.cur].node.Species, 0
		g.sfx("decide")
	case g.in.pressed[btnBack]:
		if i := slices.Index(s.ids, tiles[s.cur].node.Species); i >= 0 && g.bestiarySeen(tiles[s.cur].node.Species) {
			s.sel, s.inTree = i, false
			s.history = nil
			g.sfx("decide")
		} else {
			g.sfx("buzzer")
		}
	case g.in.tab() != 0:
		s.nextRecipe(g)
	}
}

// nextRecipe switches only the species at the top of the view.
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
			scoreCandidate := dx*dx + 5*dy*dy
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
	if s.inTree && !g.in.pressed[btnBestiary] {
		s.updateTree(g)
		return
	}
	if g.in.cancel() || g.in.pressed[btnBestiary] {
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
	if g.in.down() {
		s.sel = (s.sel + 1) % len(s.ids)
		g.sfx("cursor")
	}
	if g.in.up() {
		s.sel = (s.sel + len(s.ids) - 1) % len(s.ids)
		g.sfx("cursor")
	}
	if g.in.right() || g.in.ok() {
		if g.bestiarySeen(s.ids[s.sel]) && s.lineage.Tree(s.ids[s.sel], s.choice) != nil {
			s.inTree, s.cur, s.root = true, 0, s.ids[s.sel]
			s.history = nil
			g.sfx("decide")
		}
	}
}

func (s *bestiaryScreen) draw(g *Game, dst *ebiten.Image) {
	sw, sh := g.screen()
	g.drawPicture(dst, g.db.Game.TitleBG, 0, 0, sw, sh, 1)
	fill(dst, withAlpha(colorBlack, 0xb8))
	bestiaryPanel(g, dst, 20, 20, sw-40, sh-40)
	g.text(dst, "Monsters", 48, 40, sizeL, g.th.text)
	listX, listW, top, rowH := 42.0, 252.0, 100.0, 48.0
	rows := max(1, int((sh-top-88)/rowH))
	first := max(0, s.sel-rows/2)
	first = min(first, max(0, len(s.ids)-rows))
	bestiaryPanel(g, dst, listX, top-8, listW, float64(rows)*rowH+16)
	for i := first; i < min(len(s.ids), first+rows); i++ {
		id := s.ids[i]
		y := top + float64(i-first)*rowH
		if i == s.sel {
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
	if len(s.ids) > 0 {
		if s.inTree {
			s.drawLineage(g, dst, 322, 104, sw-350, sh-185)
		} else {
			s.drawPage(g, dst, s.ids[s.sel], 322, 100, sw-350, sh-175)
		}
	}
	hint := "Up / Down: choose    Z: lineage    X / F4: close"
	if s.inTree {
		hint = "Arrows: choose picture    Z: view ancestry    Q: open page    Tab: other recipe    X: back"
	}
	g.text(dst, gfx.Ellipsis(hint, sizeS*0.8, sw-100), 48, sh-48, sizeS*0.8, g.th.dim)
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
	if defeated < 1 {
		if e.Recruit != nil {
			write("Recruitable after victory", sizeS*0.9, 1)
		}
		write("Defeat this enemy to reveal its records.", sizeS*0.9, 2)
		return
	}
	write(fmt.Sprintf("Defeated: %d    Caught: %d", int(defeated), int(g.bestiaryCaught(id))), sizeS*0.9, 1)
	if e.Family != "" {
		write(fmt.Sprintf("Family: %s    Rank: %d", e.Family, e.Rank), sizeS*0.85, 1)
	}
	write("Sex: "+speciesSexText(e.Sex), sizeS*0.85, 1)
	y += 5
	stats := e.Stats
	write(fmt.Sprintf("HP %d   MP %d   ATK %d   DEF %d", stats["mhp"], stats["mmp"], stats["atk"], stats["def"]), sizeS*0.85, 1)
	write(fmt.Sprintf("MAT %d   MDF %d   AGI %d   LUK %d", stats["mat"], stats["mdf"], stats["agi"], stats["luk"]), sizeS*0.85, 1)
	y += 5
	var res []string
	for k, v := range e.Resist {
		label := k
		if v > 1 {
			label += " weak"
		} else if v < 1 {
			label += " resist"
		}
		res = append(res, label)
	}
	if len(res) == 0 {
		res = []string{"None recorded"}
	}
	slices.Sort(res)
	write("Resistances: "+strings.Join(res, ", "), sizeS*0.8, 2)
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
	write("Skills: "+strings.Join(skills, ", "), sizeS*0.8, 2)
	var drops []string
	for _, d := range e.Drops {
		if it := g.db.Items[d.Item]; it != nil {
			drops = append(drops, it.Name)
		}
	}
	if len(drops) == 0 {
		drops = []string{"None"}
	}
	write("Drops: "+strings.Join(drops, ", "), sizeS*0.8, 2)
	y += 8
	write("Z: view lineage", sizeS*0.8, 1)
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

// drawLineage shows seven pictures at most: child, parents and grandparents.
func (s *bestiaryScreen) drawLineage(g *Game, dst *ebiten.Image, x, top, w, h float64) {
	bestiaryPanel(g, dst, x, top, w, h)
	tiles := s.lineageTiles(x, top, w, h)
	if len(tiles) == 0 {
		return
	}
	path := make([]string, 0, len(s.history)+1)
	for _, view := range s.history {
		path = append(path, s.speciesName(g, view.species, !g.bestiarySeen(view.species)))
	}
	path = append(path, s.speciesName(g, s.root, !g.bestiarySeen(s.root)))
	crumb := gfx.Ellipsis(strings.Join(path, " > "), sizeS*0.7, w-48)
	g.text(dst, crumb, x+24, top+18, sizeS*0.7, g.th.dim)
	head := "Lineage"
	root := tiles[0].node
	if root.Choices > 1 {
		head += fmt.Sprintf("   recipe %d/%d", root.Choice+1, root.Choices)
	}
	g.text(dst, head, x+24, top+40, sizeS*0.8, g.th.accent)
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
			connect(tiles[0], tiles[i])
		}
	}
	for i := 3; i < len(tiles); i++ {
		connect(tiles[tiles[i].parent], tiles[i])
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
		caption = "Parents pictured below"
	}
	g.text(dst, gfx.Ellipsis(caption, sizeS*0.53, tile.w-12), tile.x+6, tile.y+tile.h-21, sizeS*0.53, g.th.dim)
	if n.Recipe.Kind == data.RecipeFamily {
		example := "e.g. " + s.speciesName(g, n.Recipe.Parents[0], !g.bestiarySeen(n.Recipe.Parents[0])) + " + " + s.speciesName(g, n.Recipe.Parents[1], !g.bestiarySeen(n.Recipe.Parents[1]))
		g.text(dst, gfx.Ellipsis(example, sizeS*0.46, tile.w-12), tile.x+6, tile.y+tile.h-10, sizeS*0.46, g.th.dim)
	}
}
func (g *Game) screenWidth() float64 { w, _ := g.screen(); return w }

var (
	_ scene = (*bestiaryScreen)(nil)
	_ modal = (*bestiaryScreen)(nil)
)
