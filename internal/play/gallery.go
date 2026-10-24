package play

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	"io/fs"
	"log"
	"slices"
	"strings"

	"github.com/hajimehoshi/ebiten/v2"

	"github.com/nao1215/gozen/internal/event"
	data "github.com/nao1215/gozen/internal/layer2/gamedata"
	"github.com/nao1215/gozen/internal/layer4/itemuse"
	"github.com/nao1215/gozen/internal/layer5/gfx"
	"github.com/nao1215/gozen/internal/rpg"
	lang "github.com/nao1215/gozen/internal/text"
)

const galleryFormatVersion = 1

type galleryRecord struct {
	FormatVersion int             `json:"format_version"`
	GameID        string          `json:"game_id"`
	Unlocks       map[string]bool `json:"unlocks"`
}

func readUnlocks(b []byte, gameID string) (map[string]bool, bool, error) {
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(b, &raw); err != nil {
		return nil, false, err
	}
	if raw == nil {
		return nil, false, errors.New(lang.T("回想の開放記録が辞書でない"))
	}
	if _, ok := raw["format_version"]; !ok {
		var unlocked map[string]bool
		err := json.Unmarshal(b, &unlocked)
		return unlocked, true, err
	}
	var record galleryRecord
	d := json.NewDecoder(bytes.NewReader(b))
	d.DisallowUnknownFields()
	if err := d.Decode(&record); err != nil {
		return nil, false, err
	}
	if record.FormatVersion != galleryFormatVersion {
		return nil, false, fmt.Errorf(lang.T("回想の保存形式版 %d には対応していない"), record.FormatVersion)
	}
	if raw["game_id"] == nil || bytes.Equal(raw["game_id"], []byte("null")) || record.GameID != gameID {
		return nil, false, fmt.Errorf(lang.T("回想の作品 ID が一致しない（%q / %q）"), record.GameID, gameID)
	}
	if record.Unlocks == nil {
		return nil, false, errors.New(lang.T("回想の開放記録がない"))
	}
	return record.Unlocks, false, nil
}

func (g *Game) galleryPath() string { return "gallery.json" }

func (g *Game) loadUnlocks() {
	g.unlocked = map[string]bool{}
	g.unlockReadErr, g.unlockNeedsBackup = nil, false
	b, err := g.storeRead(g.galleryPath())
	if err != nil {
		if !errors.Is(err, fs.ErrNotExist) {
			g.unlockReadErr = err
			log.Printf("gallery: %v", err)
		}
		return
	}
	unlocked, legacy, err := readUnlocks(b, g.db.Game.ID)
	if err != nil {
		g.unlockReadErr = err
		log.Printf("gallery: %v", err)
		return
	}
	g.unlocked = unlocked
	g.unlockNeedsBackup = g.db.Game.Gallery.MigrateUnlocks(unlocked) || legacy
}

func (g *Game) unlock(key string) {
	if g.unlocked == nil {
		g.unlocked = map[string]bool{}
	}
	if g.unlocked[key] || g.replaying {
		return
	}
	g.unlocked[key] = true
	g.emit("unlock", traceLine{"key": key})
	g.saveUnlocks()
}

func (g *Game) saveUnlocks() {
	if g.auto != nil || g.opts.Script != "" || g.opts.Replay != "" || g.replaying || g.unlockReadErr != nil {
		return
	}
	b, err := json.MarshalIndent(galleryRecord{galleryFormatVersion, g.db.Game.ID, g.unlocked}, "", " ")
	if err == nil {
		if g.unlockNeedsBackup {
			_, err = g.writePreserving(g.galleryPath(), b)
		} else {
			err = g.store.Write(g.galleryPath(), b)
		}
	}
	if err != nil {
		log.Printf("gallery: %v", err)
		return
	}
	g.unlockNeedsBackup = false
}

func (g *Game) seenPicture(file string) {
	gal := g.db.Game.Gallery
	for _, cg := range gal.CGs {
		if cg.Has(file) {
			g.unlock("cg:" + cg.Key())
		}
	}
	for _, sc := range gal.Scenes {
		if sc.Seen != "" && data.MatchAny([]string{sc.Seen}, file) {
			g.unlock("pic:" + file)
			g.checkUnlocks()
			break
		}
	}
}

func (g *Game) sceneOpens(sc data.GalleryScene) bool {
	if sc.Flag != "" && !g.st.Flags[sc.Flag] {
		return false
	}
	if sc.When.Set() {
		ok, err := sc.When.Bool(g.st.Env())
		if err != nil || !ok {
			return false
		}
	}
	if sc.Seen != "" {
		for k, v := range g.unlocked {
			if f, ok := strings.CutPrefix(k, "pic:"); ok && v && data.MatchAny([]string{sc.Seen}, f) {
				return true
			}
		}
		return false
	}
	return true
}

func (g *Game) checkUnlocks() {
	if g.st == nil || g.replaying {
		return
	}
	gal := g.db.Game.Gallery
	for _, cg := range gal.CGs {
		if cg.Flag != "" && g.st.Flags[cg.Flag] {
			g.unlock("cg:" + cg.Key())
		}
	}
	for _, sc := range gal.Scenes {
		if !g.unlocked[sc.Key()] && g.sceneOpens(sc) {
			g.unlock(sc.Key())
		}
	}
	g.checkProfiles()
}

func (g *Game) hasGallery() bool {
	return len(g.db.Game.Gallery.CGs)+len(g.db.Game.Gallery.Scenes) > 0
}

type galleryScreen struct {
	tab        int
	music      *musicRoom
	menu       *list
	view       string
	pages      []string
	page       int
	age        int
	check      *sceneCheck
	scenePages *list
}

const galleryMusicTab = 2

func newGallery(g *Game) *galleryScreen {
	s := &galleryScreen{}
	s.build(g)
	return s
}

func (s *galleryScreen) liveKeep(g *Game) scene {
	s.refresh(g)
	return s
}

func (s *galleryScreen) refresh(g *Game) {
	s.scenePages = nil
	sel, top := 0, 0
	if s.menu != nil {
		sel, top = s.menu.sel, s.menu.top
	}
	if !slices.Contains(s.tabIDs(g), s.tab) {
		s.check = nil
		s.tab = 0
	}
	if r := s.music; r != nil {
		song := ""
		if r.playing >= 0 && r.playing < len(r.entries) {
			song = r.entries[r.playing].label
		}
		r.entries = musicEntries(g.db)
		r.pool, r.songs = g.musicBackgrounds(), g.songPictures()
		r.playing = -1
		for i, e := range r.entries {
			if song != "" && e.label == song {
				r.playing = i
			}
		}
	}
	s.build(g)
	if n := len(s.menu.items); n > 0 && s.tab != galleryMusicTab {
		s.menu.sel = min(sel, n-1)
		s.menu.top = min(top, s.menu.sel)
	}
	if s.view == "" || s.tab != 0 {
		return
	}
	cgs := g.db.Game.Gallery.CGs
	if s.menu.sel < len(cgs) {
		s.pages = cgs[s.menu.sel].Pages(g.db.FS)
	} else {
		s.pages = nil
	}
	if len(s.pages) == 0 {
		s.view, s.pages, s.page = "", nil, 0
		return
	}
	s.page = min(s.page, len(s.pages)-1)
	s.view = s.pages[s.page]
}

func (s *galleryScreen) tabIDs(g *Game) []int {
	ids := []int{0, 1}
	if len(g.db.Music.Songs) > 0 {
		ids = append(ids, galleryMusicTab)
	}
	if g.opts.ReviewLog != "" && g.opts.GalleryAll && g.db.Game.SceneReview.Configured() {
		ids = append(ids, galleryCheckTab)
	}
	return ids
}

func (s *galleryScreen) tabLabels(g *Game) []string {
	labels := []string{lang.T("CG"), lang.T("シーン")}
	for _, id := range s.tabIDs(g)[2:] {
		if id == galleryMusicTab {
			labels = append(labels, lang.T("音楽"))
		} else {
			title := g.db.Game.SceneReview.Title
			if title == "" {
				title = lang.T("回想検査")
			}
			labels = append(labels, title)
		}
	}
	return labels
}

func (s *galleryScreen) build(g *Game) {
	sw, sh := g.screen()
	var items []listItem
	gal := g.db.Game.Gallery
	if s.tab == galleryCheckTab {
		s.buildCheck(g)
		return
	}
	if s.tab == galleryMusicTab {
		if s.music == nil {
			s.music = newMusicRoom(g)
		}
		s.menu = newList(s.music.items(), 40, 130, sw*0.30, int((sh-300)/44))
		s.syncMusicList()
		return
	}
	if s.tab == 0 {
		for _, cg := range gal.CGs {
			if g.opts.GalleryAll || g.unlocked["cg:"+cg.Key()] {
				items = append(items, listItem{label: cg.Title})
			} else {
				items = append(items, listItem{label: lang.T("？？？"), disabled: true})
			}
		}
	} else {
		for _, sc := range gal.Scenes {
			if g.opts.GalleryAll || g.unlocked[sc.Key()] {
				items = append(items, listItem{label: sc.Title})
			} else {
				items = append(items, listItem{label: lang.T("？？？"), disabled: true})
			}
		}
	}
	s.menu = newList(items, 60, 150, sw*0.36, int((sh-260)/44))
}

func (s *galleryScreen) update(g *Game) {
	s.age++
	if s.scenePages != nil {
		if g.in.cancel() {
			s.scenePages = nil
			g.sfx("cancel")
			return
		}
		if chose, _ := s.scenePages.update(g); chose {
			sc := g.db.Game.Gallery.Scenes[s.menu.sel]
			page := s.scenePages.sel
			s.scenePages = nil
			g.emit("gallery", traceLine{"scene": sc.Event, "page": page})
			r := newSceneReplay(g, sc)
			g.push(r)
			if page > 0 {
				g.startReplayPage(sc.Pages[page-1])
			}
		}
		return
	}
	if s.music != nil {
		s.music.tick()

		if s.music.bg != s.music.traced && s.tab == galleryMusicTab {
			s.music.traced = s.music.bg
			g.emit("gallery", traceLine{"background": s.music.bg})
		}
	}
	if s.updateCheckModal(g) {
		return
	}
	if s.view != "" {

		back, fwd := false, false
		if g.in.click && len(s.pages) > 1 {
			r := s.pageLabelRect(g)
			if g.in.mouse.In(r) {
				back = g.in.mouse.X < (r.Min.X+r.Max.X)/2
				fwd = !back
			}
		}
		switch {
		case back && s.page == 0, fwd && s.page+1 >= len(s.pages):
		case (g.in.left() || back) && s.page > 0:
			s.page--
			s.view = s.pages[s.page]
			g.sfx("cursor")
		case (g.in.ok() || g.in.click || g.in.right()) && s.page+1 < len(s.pages):

			s.page++
			s.view = s.pages[s.page]
			g.sfx("cursor")
		case g.in.left() || g.in.right():
		case g.in.ok() || g.in.cancel() || g.in.click:
			s.view, s.pages, s.page = "", nil, 0
			g.sfx("cancel")
		}
		return
	}
	clickTab := -1
	if g.in.click {
		for i, id := range s.tabIDs(g) {
			if g.in.mouse.In(tabRect(i)) {
				clickTab = id
			}
		}
	}
	hidden := s.tab == galleryMusicTab && s.music != nil && s.music.hidden
	if !hidden && (g.in.left() || g.in.right() || (clickTab >= 0 && clickTab != s.tab)) {
		ids := s.tabIDs(g)
		at := slices.Index(ids, s.tab)
		n := len(ids)
		switch {
		case clickTab >= 0:
			s.tab = clickTab
		case g.in.left():
			s.tab = ids[(at+n-1)%n]
		default:
			s.tab = ids[(at+1)%n]
		}
		g.sfx("cursor")
		s.build(g)
		return
	}
	if !hidden && g.in.cancel() {
		g.sfx("cancel")
		if s.music != nil && s.music.played {

			s.music.played = false
			if c := g.db.Game.TitleBGM; c.Song != "" {
				g.Music(c)
			} else {
				s.music.play(g, -1)
			}
		}
		g.pop()
		return
	}
	if clickTab >= 0 {
		return
	}
	if s.tab == galleryMusicTab {
		s.updateMusic(g)
		return
	}
	if s.tab == galleryCheckTab {
		s.updateCheck(g)
		return
	}
	if s.tab == 0 && g.opts.GalleryAll && g.cgReviewEnabled() && g.in.pressed[btnNext] {
		g.sfx("decide")
		g.push(newCGReview(g))
		return
	}
	chose, _ := s.menu.update(g)
	if !chose || len(s.menu.items) == 0 || s.menu.items[s.menu.sel].disabled {
		return
	}
	gal := g.db.Game.Gallery
	if s.tab == 0 {
		s.pages = gal.CGs[s.menu.sel].Pages(g.db.FS)
		if len(s.pages) == 0 {
			return
		}
		s.page, s.view = 0, s.pages[0]
		g.emit("gallery", traceLine{"picture": s.view, "pages": len(s.pages)})
		return
	}
	sc := gal.Scenes[s.menu.sel]
	if len(sc.Pages) > 0 {
		items := []listItem{{label: lang.T("最初から")}}
		for _, page := range sc.Pages {
			items = append(items, listItem{label: page.Title})
		}
		sw, sh := g.screen()
		s.scenePages = newList(items, 60, 150, sw*0.36, int((sh-260)/44))
		return
	}
	g.emit("gallery", traceLine{"scene": sc.Event})
	g.push(newSceneReplay(g, sc))
}

func (s *galleryScreen) draw(g *Game, dst *ebiten.Image) {
	sw, sh := g.screen()
	if s.tab == galleryMusicTab && s.view == "" {
		g.drawMusicBG(dst, s.music)
		if s.music.hidden {
			return
		}
		fill(dst, withAlpha(colorBlack, 0x20))
	} else {
		g.drawPicture(dst, g.db.Game.TitleBG, 0, 0, sw, sh, 1)
		fill(dst, withAlpha(colorBlack, 0x90))
	}
	if s.view != "" {
		g.drawCG(dst, data.CG{File: s.view}, 1)
		if len(s.pages) > 1 {
			gfx.DrawTextOutlineColor(dst, s.pageLabel(), sw-90, sh-44, sizeS, g.th.text, g.th.window, 1)
		}
		return
	}
	for i, t := range s.tabLabels(g) {
		c := g.th.dim
		if s.tabIDs(g)[i] == s.tab {
			c = g.th.accent
		}
		gfx.DrawTextOutlineColor(dst, t, 120+float64(i)*180, 70, sizeL, c, g.th.window, 1)
	}
	gfx.DrawTextOutlineColor(dst, lang.T("ギャラリー"), sw/2, 30, sizeM, g.th.text, g.th.window, 1)
	if s.tab == galleryCheckTab {
		s.drawCheck(g, dst)
		return
	}
	if s.tab == galleryMusicTab {
		s.drawMusic(g, dst)
		gfx.DrawTextOutlineColor(dst, lang.T("←→: 欄   Z: ながす・とめる   Tab: つぎの曲   V: 絵だけ   X: もどる"), sw/2, sh-44, sizeS*0.9, g.th.text, g.th.window, 1)
		return
	}
	s.menu.draw(g, dst, true)

	gal := g.db.Game.Gallery
	pic := ""
	if len(s.menu.items) > 0 && !s.menu.items[s.menu.sel].disabled {
		if s.tab == 0 {
			if pages := gal.CGs[s.menu.sel].Pages(g.db.FS); len(pages) > 0 {
				pic = pages[0]
			}
		} else {
			pic = gal.Scenes[s.menu.sel].Picture
		}
	}
	x, y, w := sw*0.44, 150.0, sw*0.52
	h := w * 9 / 16
	g.window(dst, x-8, y-8, w+16, h+16)
	if pic != "" {
		g.drawPicture(dst, data.Picture{File: pic}, x, y, w, h, 1)
	} else {
		gfx.DrawText(dst, lang.T("？？？"), x+w/2, y+h/2-20, sizeL, g.th.dim)
	}
	n, open := 0, 0
	for _, it := range s.menu.items {
		n++
		if !it.disabled {
			open++
		}
	}

	if len(s.menu.items) > 0 && !s.menu.items[s.menu.sel].disabled {
		ls, sz := gfx.FitLines(s.menu.items[s.menu.sel].label, sizeM, w, 1)
		if len(ls) > 0 {
			gfx.DrawTextOutlineColor(dst, ls[0], x+w/2, y+h+20, sz, g.th.text, g.th.window, 1)
		}
	}
	gfx.DrawTextOutlineColor(dst, lang.T("ひらいた数")+": "+itoa(open)+" / "+itoa(n), x+w/2, y+h+64, sizeS, g.th.dim, g.th.window, 1)
	hint := lang.T("←→: CG / シーン   Z: みる   X: もどる")
	if len(g.db.Music.Songs) > 0 {
		hint = lang.T("←→: CG / シーン / 音楽   Z: みる   X: もどる")
	}
	if s.tab == 0 && g.opts.GalleryAll && g.cgReviewEnabled() {
		hint += lang.T("   Tab: 流し見（札を付ける）")
	}
	gfx.DrawTextOutlineColor(dst, hint, sw/2, sh-44, sizeS*0.9, g.th.text, g.th.window, 1)
	if s.scenePages != nil {
		fill(dst, withAlpha(colorBlack, 0xc0))
		gfx.DrawTextOutlineColor(dst, lang.T("どこから見る？"), sw/2, 80, sizeL, g.th.text, g.th.window, 1)
		s.scenePages.draw(g, dst, true)
		if page := s.scenePages.sel; page > 0 {
			pic := gal.Scenes[s.menu.sel].Pages[page-1].Picture
			if pic != "" {
				g.drawPicture(dst, data.Picture{File: pic}, x, y, w, h, 1)
			}
		}
		gfx.DrawTextOutlineColor(dst, lang.T("Z: みる   X: もどる"), sw/2, sh-44, sizeS, g.th.text, g.th.window, 1)
	}
}

func (s *galleryScreen) pageLabel() string {
	return "◀ " + itoa(s.page+1) + " / " + itoa(len(s.pages)) + " ▶"
}

func (s *galleryScreen) pageLabelRect(g *Game) image.Rectangle {
	sw, sh := g.screen()
	w := gfx.TextWidth(s.pageLabel(), sizeS) + 24
	return image.Rect(int(sw-90-w/2), int(sh-56), int(sw-90+w/2), int(sh-8))
}

func tabRect(i int) image.Rectangle {
	cx := 120 + float64(i)*180
	return image.Rect(int(cx-80), 60, int(cx+80), 112)
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b []byte
	for n > 0 {
		b = append([]byte{byte('0' + n%10)}, b...)
		n /= 10
	}
	return string(b)
}

type sceneReplay struct {
	saved    *rpg.State
	events   *event.Runner
	song     string
	log      []logLine
	bg       data.Picture
	sc       data.GalleryScene
	autoRead bool
}

func newSceneReplay(g *Game, sc data.GalleryScene) *sceneReplay {
	r := &sceneReplay{saved: g.st, events: g.events, song: g.baseSong, bg: sc.Background, log: g.backlog, sc: sc, autoRead: g.autoRead}
	g.backlog, g.logView = nil, nil
	g.evLines, g.evAnswers = 0, nil
	st := rpg.New(g.db, g.rng())
	if sc.Location != "" {
		st.Location = sc.Location
	} else if id := g.db.EventPlaces()[sc.Event]; id != "" {
		if l := g.db.Locations[id]; l.Dungeon == nil {
			st.Location = id
		} else if r.bg.File == "" {
			r.bg = l.Background
			if r.bg.File == "" {
				r.bg = l.BattleBG
			}
		}
	}
	if sc.Flag != "" {
		st.Flags[sc.Flag] = true
	}
	g.st = st
	g.replaying = true

	g.songOverride, g.moodOverride = "", ""
	g.playLocationMusic()

	if cue := sc.Music; cue.Song != "" {
		g.Music(cue)
	} else if cue := g.db.Game.Gallery.Music; cue.Song != "" {
		g.Music(cue)
	}
	g.events = event.New(g.db, st, event.HostFunc(g.Handle))
	g.events.BlockAside = g.AsideBlocked
	g.events.Dev, g.events.UseItem = g.opts.Development, itemuse.For(g.db, st)
	g.req = nil
	do := g.db.Events[sc.Event].Do
	if g.opts.capture != nil {
		var defaults []data.Command
		if len(sc.Set) > 0 {
			defaults = []data.Command{{SetV: sc.Set}}
		}
		if err := g.captureSetup(defaults); err != nil {
			g.failure = err
			return r
		}
		g.events.Log = func(err error) { g.failure = err }
	} else if len(sc.Set) > 0 {
		do = append([]data.Command{{SetV: sc.Set}}, do...)
	}
	g.events.Start(do)
	return r
}

func (r *sceneReplay) update(g *Game) {
	if g.pump() {
		return
	}

	g.st, g.events, g.req = r.saved, r.events, nil
	g.backlog, g.logView = r.log, nil
	g.replaying = false
	g.autoRead = r.autoRead
	if g.fast != nil && g.fast.page {
		g.fast = nil
	}
	g.Image(data.CG{})
	if c := g.db.Game.TitleBGM; c.Song != "" {
		g.Music(c)
	}
	g.pop()
}

func (g *Game) startReplayPage(page data.GalleryPage) {
	g.autoRead = false
	g.fast = &fastTo{lines: page.Line, picture: page.Picture, answers: page.Answers, page: true}
}

func (g *Game) seekReplayPage() {
	for range 256 {
		g.talk.shown = g.talk.total
		g.fastStep()
		if g.fast == nil {

			g.cgAge, g.cgOldT, g.fadeT = g.cg.FadeTicks(), 0, 0
			return
		}
		g.waitT, g.tapT = 0, 0
		if !g.pump() {
			g.fast = nil
			return
		}
	}

	g.in.pressed = [numButtons]bool{}
}

func (r *sceneReplay) draw(g *Game, dst *ebiten.Image) {
	sw, sh := g.screen()
	loc := g.db.Locations[g.st.Location]
	if r.bg.File != "" {
		g.drawPicture(dst, r.bg, 0, 0, sw, sh, 1)
	} else if loc != nil && loc.Background.File != "" {
		g.drawPicture(dst, loc.Background, 0, 0, sw, sh, 1)
	} else {
		fill(dst, colorBlack)
	}
}
