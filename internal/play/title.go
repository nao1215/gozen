package play

import (
	"math"
	"slices"

	lang "github.com/nao1215/gozen/internal/text"

	"github.com/hajimehoshi/ebiten/v2"

	data "github.com/nao1215/gozen/internal/layer2/gamedata"
	"github.com/nao1215/gozen/internal/layer5/gfx"
	"github.com/nao1215/gozen/internal/layer5/sound"
)

type title struct {
	menu *list
	age  int
}

func newTitle(g *Game) *title {
	t := &title{menu: g.titleMenu()}
	if c := g.db.Game.TitleBGM; c.Song != "" {
		mood := c.Mood
		if mood == "" {
			mood = sound.Neutral
		}
		sound.PlaySong(c.Song, mood)
		g.baseSong, g.baseMood = c.Song, mood
	}
	return t
}

func (t *title) liveKeep(g *Game) scene {
	m := g.titleMenu()
	m.sel = min(t.menu.sel, len(m.items)-1)
	t.menu = m
	return t
}

func (g *Game) titleMenu() *list {
	sw, sh := g.screen()
	hasSave := false
	for i := range saveSlots + 1 {
		if _, ok := g.slotLabel(i); ok {
			hasSave = true
		}
	}
	items := []listItem{{label: lang.T("はじめから")}, {label: lang.T("つづきから"), disabled: !hasSave}, {label: lang.T("ギャラリー"), disabled: !g.hasGallery()}, {label: lang.T("せってい")}}
	if g.profilesInMenus() {
		items = slices.Insert(items, 3, listItem{label: lang.T("キャラクター"), id: "profiles"})
	}
	if g.bestiaryInMenus() {
		at := 3
		if g.profilesInMenus() {
			at = 4
		}
		items = slices.Insert(items, at, listItem{label: lang.T("まもののほん"), id: "bestiary"})
	}
	if canQuit {
		items = append(items, listItem{label: lang.T("おわる")})
	}
	m := newList(items, sw*0.08, sh*0.5, 340, max(5, len(items)))
	if g.titleCentered() {
		m.x = (sw - 340) / 2
	}
	if hasSave {
		m.sel = 1
	}
	return m
}

func (t *title) update(g *Game) {
	t.age++
	if g.in.pressed[btnConfig] {
		g.sfx("decide")
		g.push(newConfigScreen(g))
		return
	}
	chose, _ := t.menu.update(g)
	if !chose {
		return
	}
	sel := t.menu.sel
	if t.menu.items[sel].id == "profiles" {
		g.sfx("decide")
		g.push(newProfiles(g, nil))
		return
	}
	if t.menu.items[sel].id == "bestiary" {
		g.sfx("decide")
		g.push(newBestiary(g, nil))
		return
	}
	if g.profilesInMenus() && sel >= 3 {
		sel--
	}
	if g.bestiaryInMenus() && sel >= 3 {
		sel--
	}
	switch sel {
	case 0:
		g.newGame()
	case 1:
		g.push(newSaveMenu(g, false, func() {}))
	case 2:
		g.sfx("decide")
		g.push(newGallery(g))
	case 3:
		g.sfx("decide")
		g.push(newConfigScreen(g))
	case 4:
		g.quit = true
	}
}

func (t *title) draw(g *Game, dst *ebiten.Image) {
	sw, sh := g.screen()
	gm := g.db.Game
	g.drawPicture(dst, gm.TitleBG, 0, 0, sw, sh, 1)
	fill(dst, withAlpha(colorBlack, 0x40))
	if len(gm.TitleFigures) > 0 {
		for i, f := range gm.TitleFigures {
			rise := math.Max(0, math.Min(1, float64(t.age-i*8)/40))
			bottom, h := f.Bottom, f.Height
			if bottom == 0 && gm.SchemaVersion == data.LegacySchemaVersion {
				bottom = 1
			}
			if h == 0 {
				h = 0.95
			}
			g.drawStanding(dst, f.File, sw*f.X, sh*bottom+(1-rise)*30, sh*h, float32(rise), false, nil)
		}
	} else if gm.TitleArt.File != "" {
		rise := math.Min(1, float64(t.age)/40)
		const bob = 0
		cs := &ebiten.ColorScale{}
		tint(cs, gm.TitleArt)
		g.drawStanding(dst, gm.TitleArt.File, sw*0.72, sh*1.25+bob+(1-rise)*60, sh*1.25-headRoom, float32(rise), false, cs)
	}
	a := float32(math.Min(1, float64(t.age)/30))
	tx := sw * 0.3
	if g.titleCentered() {
		tx = sw / 2
	}
	tt := gm.TitleText
	size, track, subSize, subTrack := 96.0, 0.12, 30.0, 0.3
	if tt.Size > 0 {
		size = tt.Size
	}
	if tt.Tracking != nil {
		track = *tt.Tracking
	}
	if tt.SubtitleSize > 0 {
		subSize = tt.SubtitleSize
	}
	if tt.SubtitleTracking != nil {
		subTrack = *tt.SubtitleTracking
	}
	fill, edge := tt.Color.Or(g.th.text), tt.Edge.Or(withAlpha(g.th.window, 0xff))
	gfx.DrawTitleText(dst, gm.Title, tx, sh*0.1, size, track, fill, edge, a, false)
	if gm.Subtitle != "" {
		// 124 pixels below a title of 96, as before title_text.
		subEdge := tt.Edge.Or(g.th.window)
		gfx.DrawTitleText(dst, gm.Subtitle, tx, sh*0.1+size*124/96, subSize, subTrack, fill, subEdge, a, true)
	}
	t.menu.draw(g, dst, true)

	hx := sw*0.08 + 170
	if g.titleCentered() {
		hx = sw / 2
	}
	gfx.DrawTextOutlineColor(dst, lang.T("Z / Enter: けってい   X / Esc: もどる   O: せってい   M: ミュート   F11: ぜんがめん"), hx, sh-44, sizeS*0.8, g.th.text, g.th.window, a)
}

type gameOver struct{ age int }

func newGameOver(g *Game) *gameOver {
	if c := g.db.Game.GameOver.Music; c.Song != "" {
		g.Music(c)
	}
	return &gameOver{}
}

func (s *gameOver) update(g *Game) {
	s.age++
	if s.age > 90 && (g.in.advance() || g.in.cancel()) {
		if ev := g.db.Game.GameOver.Event; ev != "" && g.db.Events[ev] != nil {
			for _, a := range g.st.Members() {
				a.Recover()
			}
			g.replace(g.newPlace())
			g.events.Start(g.db.Events[ev].Do)
			return
		}
		g.st = nil
		g.replace(newTitle(g))
	}
}

func (s *gameOver) draw(g *Game, dst *ebiten.Image) {
	sw, sh := g.screen()
	fill(dst, colorBlack)
	a := float32(math.Min(1, float64(s.age)/80))
	g.drawPicture(dst, g.db.Game.GameOver.Background, 0, 0, sw, sh, a)
	text := g.db.Game.GameOver.Text
	if text == "" {
		text = "GAME OVER"
	}
	gfx.DrawTextOutlineColor(dst, text, sw/2, sh*0.42, 56, g.th.text, colorBlack, a)
}

type ending struct {
	age int
	pic string
}

func newEnding(g *Game) *ending {
	pic := g.cg.File
	if pic == "" && g.cgOldT > 0 {
		pic = g.cgOld.File
	}
	e := &ending{pic: pic}
	if e.pic == "" {
		e.pic = g.db.Game.TitleBG.File
	}
	return e
}

func (s *ending) update(g *Game) {
	s.age++
	if s.age > 150 && (g.in.advance() || g.in.cancel()) {
		g.st = nil
		g.replace(newTitle(g))
	}
}

func (s *ending) draw(g *Game, dst *ebiten.Image) {
	sw, sh := g.screen()
	fill(dst, colorBlack)
	a := float32(math.Min(1, float64(s.age)/90))
	g.drawPicture(dst, data.Picture{File: s.pic, Blur: true, Bright: 0.6}, 0, 0, sw, sh, a)
	if img := g.pics.get(s.pic); img != nil { // a game with no picture shown and no title background ends on the text alone
		iw, ih := float64(img.Bounds().Dx()), float64(img.Bounds().Dy())
		k := math.Min(sw/iw, sh/ih)
		gfx.DrawImageScaled(dst, img, (sw-iw*k)/2, (sh-ih*k)/2, k, a*0.85)
	}
	gfx.DrawTextOutlineColor(dst, "THE END", sw/2, sh*0.4, 96, g.th.text, g.th.nameTag, a)
	gfx.DrawTextOutlineColor(dst, g.db.Game.Title, sw/2, sh*0.4+130, sizeL, g.th.text, g.th.window, a)
	if s.age > 150 {
		gfx.DrawTextOutlineColor(dst, "Thank you for playing!", sw/2, sh-90, sizeM, g.th.text, g.th.window, 1)
	}
}

func (g *Game) titleCentered() bool {
	return len(g.db.Game.TitleFigures) == 0 && g.db.Game.TitleArt.File == ""
}
