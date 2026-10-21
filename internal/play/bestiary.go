package play

import (
	"fmt"
	"strings"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/nao1215/gozen/internal/expr"
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

type bestiaryScreen struct {
	ids  []string
	sel  int
	done func()
}

func newBestiary(g *Game, done func()) *bestiaryScreen {
	return &bestiaryScreen{ids: g.db.Order["enemies"], done: done}
}
func (s *bestiaryScreen) modal() bool { return true }
func (s *bestiaryScreen) update(g *Game) {
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
		s.drawPage(g, dst, s.ids[s.sel], left, sh)
	}
	gfx.DrawTextOutlineColor(dst, "Up / Down: choose    X / F4: close", sw/2, sh-38, sizeS*0.85, g.th.text, g.th.window, 1)
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
func (s *bestiaryScreen) drawPage(g *Game, dst *ebiten.Image, id string, left, sh float64) {
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
	gfx.DrawTextOutlineColor(dst, e.Name, x+220, 82, sizeL, g.th.text, g.th.window, 1)
	lines, _ := gfx.FitLines(e.Lore, sizeS*0.9, w-220, 4)
	for i, line := range lines {
		gfx.DrawText(dst, line, x+220, 128+float64(i)*22, sizeS*0.9, g.th.text)
	}
	if defeated < 1 {
		gfx.DrawText(dst, "Defeat this enemy to reveal its records.", x+20, 280, sizeS, g.th.text)
		return
	}
	y := 285.0
	gfx.DrawTextOutlineColor(dst, fmt.Sprintf("Defeated: %d", int(defeated)), x+20, y, sizeS, g.th.cursor, g.th.window, 1)
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
}
func (g *Game) screenWidth() float64 { w, _ := g.screen(); return w }

var _ scene = (*bestiaryScreen)(nil)
var _ modal = (*bestiaryScreen)(nil)
