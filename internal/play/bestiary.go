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

func (s *bestiaryScreen) lineageTop(g *Game) float64 {
	if len(s.ids) > 0 && g.bestiaryDefeated(s.ids[s.sel]) >= 1 {
		return 505
	}
	return 320
}

func (s *bestiaryScreen) lineageCapacity(g *Game) int {
	_, sh := g.screen()
	return max(3, int((sh-60-s.lineageTop(g)-30)/lineageRowH))
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
	fill(dst, withAlpha(colorBlack, 0xa8))
	gfx.FillRect(dst, 0, 0, float32(sw), 64, withAlpha(g.th.window, 0xe0), false)
	gfx.DrawTextOutlineColor(dst, "Monsters", 28, 12, sizeL, g.th.text, g.th.window, 1)
	left := sw * 0.34
	rowH := 44.0
	top := 78.0
	for i, id := range s.ids {
		y := top + float64(i-s.sel)*rowH
		if y < 65 || y+rowH > sh-55 {
			continue
		}
		if i == s.sel {
			gfx.FillRoundRect(dst, 12, float32(y), float32(left-20), float32(rowH-2), 8, withAlpha(g.th.accent, 0x55))
		}
		e := g.db.Enemies[id]
		if g.bestiarySeen(id) {
			drawBestiaryImage(g, dst, e.Image.File, 18, y+3, 36, 36)
			gfx.DrawText(dst, e.Name, left*0.13, y+10, sizeS, g.th.text)
		} else {
			gfx.DrawText(dst, "???", left*0.13, y+10, sizeS, g.th.text)
		}
	}
	if len(s.ids) > 0 {
		s.drawPage(g, dst, s.ids[s.sel], left)
	}
	hint := "Up / Down: choose    Z: lineage    X / F4: close"
	if s.inTree {
		hint = "Up / Down: choose    Left / Right: fold    Z: open page    Tab: other recipe    X: back"
	}
	gfx.DrawTextOutlineColor(dst, hint, sw/2, sh-38, sizeS*0.85, g.th.text, g.th.window, 1)
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

func (s *bestiaryScreen) drawPage(g *Game, dst *ebiten.Image, id string, left float64) {
	e := g.db.Enemies[id]
	x := left + 28
	w := g.screenWidth() - x - 32
	if w < 200 {
		return
	}
	seen := g.bestiarySeen(id)
	defeated := g.bestiaryDefeated(id)
	if !seen {
		gfx.DrawTextOutlineColor(dst, "Not yet encountered", x+20, 100, sizeM, g.th.text, g.th.window, 1)
		return
	}
	drawBestiaryImage(g, dst, e.Image.File, x+16, 78, 180, 170)
	gfx.DrawTextOutlineColor(dst, e.Name+sexSign(e.Sex), x+220, 82, sizeL, g.th.text, g.th.window, 1)
	lines, _ := gfx.FitLines(e.Lore, sizeS*0.9, w-220, 4)
	for i, line := range lines {
		gfx.DrawText(dst, line, x+220, 128+float64(i)*22, sizeS*0.9, g.th.text)
	}
	if defeated < 1 {
		if e.Recruit != nil {
			gfx.DrawText(dst, "Recruitable after victory", x+20, 250, sizeS, g.th.accent)
		}
		gfx.DrawText(dst, "Defeat this enemy to reveal its records.", x+20, 280, sizeS, g.th.text)
		s.drawLineage(g, dst, x+20, w-20)
		return
	}
	y := 285.0
	gfx.DrawTextOutlineColor(dst, fmt.Sprintf("Defeated: %d", int(defeated)), x+20, y, sizeS, g.th.cursor, g.th.window, 1)
	if e.Family != "" {
		gfx.DrawText(dst, fmt.Sprintf("Family: %s    Rank: %d    Sex: %s    Caught: %d", e.Family, e.Rank, speciesSexText(e.Sex), int(g.bestiaryCaught(id))), x+260, y, sizeS*0.82, g.th.text)
	}
	y += 32
	stats := e.Stats
	gfx.DrawText(dst, fmt.Sprintf("HP %d   MP %d   ATK %d   DEF %d   MAT %d   MDF %d   AGI %d   LUK %d", stats["mhp"], stats["mmp"], stats["atk"], stats["def"], stats["mat"], stats["mdf"], stats["agi"], stats["luk"]), x+20, y, sizeS*0.82, g.th.text)
	y += 30
	gfx.DrawTextOutlineColor(dst, "Resistances and weaknesses", x+20, y, sizeS*0.9, g.th.accent, g.th.window, 1)
	y += 23
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
	gfx.DrawText(dst, strings.Join(res, ", "), x+20, y, sizeS*0.8, g.th.text)
	y += 28
	gfx.DrawTextOutlineColor(dst, "Skills", x+20, y, sizeS*0.9, g.th.accent, g.th.window, 1)
	y += 23
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
	gfx.DrawText(dst, strings.Join(skills, ", "), x+20, y, sizeS*0.8, g.th.text)
	y += 28
	gfx.DrawTextOutlineColor(dst, "Drops", x+20, y, sizeS*0.9, g.th.accent, g.th.window, 1)
	y += 23
	var drops []string
	for _, d := range e.Drops {
		if it := g.db.Items[d.Item]; it != nil {
			drops = append(drops, it.Name)
		}
	}
	if len(drops) == 0 {
		drops = []string{"None"}
	}
	gfx.DrawText(dst, strings.Join(drops, ", "), x+20, y, sizeS*0.8, g.th.text)
	s.drawLineage(g, dst, x+20, w-20)
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

// drawLineage shows the family tree of the page's species in the space under its records.
func (s *bestiaryScreen) drawLineage(g *Game, dst *ebiten.Image, x, w float64) {
	rows := s.lineageRows(g)
	if len(rows) == 0 {
		return
	}
	top := s.lineageTop(g)
	head := "Lineage"
	if s.tree.Choices > 1 {
		head += fmt.Sprintf("   (recipe %d of %d)", s.tree.Choice+1, s.tree.Choices)
	}
	gfx.DrawTextOutlineColor(dst, head, x, top, sizeS*0.9, g.th.accent, g.th.window, 1)
	n := s.lineageCapacity(g)
	start := min(s.scroll, max(0, len(rows)-n))
	y := top + 30
	for i := start; i < len(rows) && i < start+n; i++ {
		row := rows[i]
		indent := min(float64(row.node.Depth)*30, max(0, w-180))
		if i == s.cur && s.inTree {
			gfx.FillRoundRect(dst, float32(x+indent-6), float32(y-3), float32(w-indent), float32(lineageRowH-2), 6, withAlpha(g.th.accent, 0x55))
		}
		if row.node.Depth > 0 {
			gfx.FillRect(dst, float32(x+indent-22), float32(y+11), 14, 2, withAlpha(g.th.dim, 0xff), false)
		}
		label := s.lineageLabel(g, row)
		if fitted, _ := gfx.FitLines(label, sizeS*0.85, w-indent-24, 1); len(fitted) > 0 {
			label = fitted[0]
		}
		gfx.DrawText(dst, label, x+indent+2, y, sizeS*0.85, g.th.text)
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
