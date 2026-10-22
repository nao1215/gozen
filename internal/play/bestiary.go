package play

import (
	"fmt"
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
	choice  map[string]int  // species to the recipe shown for it
	folds   map[string]bool // node path to an explicit open (true) or folded (false)
	tree    *data.LineageNode
	treeFor string
	inTree  bool
	cur     int
	scroll  int
}

func newBestiary(g *Game, done func()) *bestiaryScreen {
	return &bestiaryScreen{ids: g.db.Order["enemies"], done: done, lineage: data.BuildLineage(g.db), choice: map[string]int{}, folds: map[string]bool{}}
}
func (s *bestiaryScreen) modal() bool { return true }

// lineageRow is a visible line of the lineage tree.
type lineageRow struct {
	node       *data.LineageNode
	hidden     bool // the species is not yet in the book, so it is drawn as "?"
	expandable bool
	expanded   bool
}

func (s *bestiaryScreen) lineageRows(g *Game) []lineageRow {
	if len(s.ids) == 0 {
		return nil
	}
	id := s.ids[s.sel]
	if s.tree == nil || s.treeFor != id {
		s.tree, s.treeFor = s.lineage.Tree(id, s.choice), id
	}
	if s.tree == nil {
		return nil
	}
	var rows []lineageRow
	var walk func(n *data.LineageNode)
	walk = func(n *data.LineageNode) {
		hidden := !g.bestiarySeen(n.Species)
		row := lineageRow{node: n, hidden: hidden, expandable: len(n.Parents) > 0 && !hidden}
		if row.expandable {
			open, set := s.folds[n.Path]
			row.expanded = open
			if !set {
				row.expanded = n.Depth < data.DefaultOpenDepth
			}
		}
		rows = append(rows, row)
		if row.expanded {
			for _, p := range n.Parents {
				walk(p)
			}
		}
	}
	walk(s.tree)
	return rows
}

func (s *bestiaryScreen) resetTree() {
	s.tree, s.cur, s.scroll = nil, 0, 0
	s.folds = map[string]bool{}
}

func (s *bestiaryScreen) updateTree(g *Game) {
	rows := s.lineageRows(g)
	if len(rows) == 0 {
		s.inTree = false
		return
	}
	s.cur = min(s.cur, len(rows)-1)
	row := rows[s.cur]
	switch {
	case g.in.cancel():
		g.sfx("cancel")
		s.inTree = false
	case g.in.down():
		s.cur = (s.cur + 1) % len(rows)
		g.sfx("cursor")
	case g.in.up():
		s.cur = (s.cur + len(rows) - 1) % len(rows)
		g.sfx("cursor")
	case g.in.right():
		if row.expandable && !row.expanded {
			s.folds[row.node.Path] = true
			g.sfx("cursor")
		}
	case g.in.left():
		if row.expandable && row.expanded {
			s.folds[row.node.Path] = false
			g.sfx("cursor")
		} else {
			s.inTree = false
		}
	case g.in.ok():
		if i := slices.Index(s.ids, row.node.Species); i >= 0 && !row.hidden && i != s.sel {
			g.sfx("decide")
			s.sel = i
			s.resetTree()
			s.inTree = false
			return
		}
		g.sfx("buzzer")
	}
	if g.in.tab() != 0 {
		s.nextRecipe(g, row.node)
	}
	s.keepInView(g)
}

// nextRecipe switches the cursor's species to its next way of being obtained.
func (s *bestiaryScreen) nextRecipe(g *Game, n *data.LineageNode) {
	if n.Choices < 2 || !g.bestiarySeen(n.Species) {
		g.sfx("buzzer")
		return
	}
	path := n.Path
	s.choice[n.Species] = (n.Choice + 1) % n.Choices
	s.tree = nil
	g.sfx("cursor")
	rows := s.lineageRows(g)
	s.cur = 0
	for i, r := range rows {
		if r.node.Path == path {
			s.cur = i
		}
	}
}

func (s *bestiaryScreen) keepInView(g *Game) {
	n := s.lineageCapacity(g)
	if s.cur < s.scroll {
		s.scroll = s.cur
	}
	if s.cur >= s.scroll+n {
		s.scroll = s.cur - n + 1
	}
}

const lineageRowH = 28.0

func (s *bestiaryScreen) lineageCapacity(g *Game) int {
	_, sh := g.screen()
	return max(1, int((sh-260)/lineageRowH))
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
		if rows := s.lineageRows(g); len(rows) > 0 && g.bestiarySeen(s.ids[s.sel]) {
			s.inTree, s.cur, s.scroll = true, 0, 0
			g.sfx("decide")
		}
	}
	if g.in.tab() != 0 && s.tree != nil {
		s.nextRecipe(g, s.tree)
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
		hint = "Up / Down: choose    Left / Right: fold    Z: open page    Tab: other recipe    X: back"
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

// drawLineage shows the family tree on its own framed page.
func (s *bestiaryScreen) drawLineage(g *Game, dst *ebiten.Image, x, top, w, h float64) {
	bestiaryPanel(g, dst, x, top, w, h)
	rows := s.lineageRows(g)
	if len(rows) == 0 {
		return
	}
	x += 24
	w -= 48
	top += 20
	head := "Lineage"
	if s.tree.Choices > 1 {
		head += fmt.Sprintf("   (recipe %d of %d)", s.tree.Choice+1, s.tree.Choices)
	}
	g.text(dst, head, x, top, sizeS, g.th.accent)
	n := min(s.lineageCapacity(g), max(1, int((h-76)/lineageRowH)))
	start := min(s.scroll, max(0, len(rows)-n))
	y := top + 30
	for i := start; i < len(rows) && i < start+n; i++ {
		row := rows[i]
		indent := min(float64(row.node.Depth)*30, max(0, w-180))
		if i == s.cur && s.inTree {
			gfx.FillRoundRect(dst, float32(x+indent-6), float32(y-3), float32(w-indent), float32(lineageRowH-2), 6, withAlpha(g.th.accent, 0x55))
		}
		if row.node.Depth > 0 {
			gfx.FillRect(dst, float32(x+indent-22), float32(y-12), 2, 24, withAlpha(g.th.dim, 0xa0), false)
			gfx.FillRect(dst, float32(x+indent-22), float32(y+11), 14, 2, withAlpha(g.th.dim, 0xff), false)
		}
		label := s.lineageLabel(g, row)
		if fitted, _ := gfx.FitLines(label, sizeS*0.85, w-indent-24, 1); len(fitted) > 0 {
			label = fitted[0]
		}
		g.text(dst, label, x+indent+2, y, sizeS*0.85, g.th.text)
		y += lineageRowH
	}
	if len(rows) > n {
		track := float32(n)*lineageRowH - 4
		gfx.FillRect(dst, float32(x+w-6), float32(top+30), 3, track, withAlpha(g.th.dim, 0x80), false)
		gfx.FillRect(dst, float32(x+w-6), float32(top+30)+track*float32(start)/float32(len(rows)), 3, track*float32(n)/float32(len(rows)), g.th.accent, false)
	}
}

func (s *bestiaryScreen) speciesName(g *Game, id string, hidden bool) string {
	if hidden {
		return "?"
	}
	e := g.db.Enemies[id]
	if e == nil {
		return id
	}
	return e.Name + sexSign(e.Sex)
}

func (s *bestiaryScreen) lineageLabel(g *Game, row lineageRow) string {
	n := row.node
	label := s.speciesName(g, n.Species, row.hidden)
	if row.hidden {
		return label
	}
	switch {
	case n.Ref == "seen":
		return label + "   (shown above)"
	case n.Ref == "cycle":
		return label + "   (loop, see above)"
	}
	switch n.Recipe.Kind {
	case data.RecipeCapture:
		where := "unknown"
		if len(n.Sites) > 0 {
			where = strings.Join(n.Sites[:min(2, len(n.Sites))], ", ")
			if len(n.Sites) > 2 {
				where += fmt.Sprintf(" +%d", len(n.Sites)-2)
			}
		}
		label += "   Capture: " + where
	case data.RecipeFamily:
		label += "   <- " + g.db.MonsterFamilies[n.Recipe.Families[0]] + " x " + g.db.MonsterFamilies[n.Recipe.Families[1]]
		if row.expandable && !row.expanded {
			label += "   ..."
		}
	case data.RecipePair:
		label += "   <- " + s.speciesName(g, n.Recipe.Parents[0], !g.bestiarySeen(n.Recipe.Parents[0])) + " x " + s.speciesName(g, n.Recipe.Parents[1], !g.bestiarySeen(n.Recipe.Parents[1]))
		if row.expandable && !row.expanded {
			label += "   ..."
		}
	}
	if n.Choices > 1 {
		label += fmt.Sprintf("   [%d/%d]", n.Choice+1, n.Choices)
	}
	return label
}
func (g *Game) screenWidth() float64 { w, _ := g.screen(); return w }

var (
	_ scene = (*bestiaryScreen)(nil)
	_ modal = (*bestiaryScreen)(nil)
)
