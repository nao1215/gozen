package play

import (
	"image"
	"math/rand/v2"
	"slices"
	"sort"
	"time"

	"github.com/hajimehoshi/ebiten/v2"

	data "github.com/nao1215/gozen/internal/layer2/gamedata"
	"github.com/nao1215/gozen/internal/layer5/gfx"
	"github.com/nao1215/gozen/internal/layer5/sound"
	lang "github.com/nao1215/gozen/text"
)

type musicEntry struct {
	song, mood string
	label      string
	oneShot    bool
}

func musicEntries(db *data.DB) []musicEntry {
	m := db.Music
	ids := slices.Clone(db.Order["songs"])
	if len(ids) != len(m.Songs) {
		ids = ids[:0]
		for id := range m.Songs {
			ids = append(ids, id)
		}
		sort.Strings(ids)
	}
	var out []musicEntry
	for _, id := range ids {
		s, ok := m.Songs[id]
		if !ok {
			continue
		}
		name := s.Name
		if name == "" {
			name = id
		}
		out = append(out, musicEntry{song: id, mood: sound.Neutral, label: name})
		if s.File != "" {
			continue
		}
		for _, mood := range m.OneShot {
			if _, has := s.Moods[mood]; !has || mood == sound.Neutral {
				continue
			}
			mn := m.MoodNames[mood]
			if mn == "" {
				mn = mood
			}
			out = append(out, musicEntry{song: id, mood: mood, label: name + lang.T("・") + mn, oneShot: true})
		}
	}
	return out
}

func (g *Game) musicBackgrounds() []string {
	var out []string
	for _, cg := range g.db.Game.Gallery.CGs {
		if g.opts.GalleryAll || g.unlocked["cg:"+cg.Key()] {
			out = append(out, cg.Pages(g.db.FS)...)
		}
	}
	return out
}

func (g *Game) pictureOpen(file string) bool {
	if g.opts.GalleryAll {
		return true
	}
	for _, cg := range g.db.Game.Gallery.CGs {
		if cg.Has(file) {
			return g.unlocked["cg:"+cg.Key()]
		}
	}
	return true
}

func (g *Game) songPictures() map[string][]string {
	out := map[string][]string{}
	for song, files := range g.db.Game.Gallery.MusicPictures {
		for _, f := range files {
			if f != "" && g.pictureOpen(f) {
				out[song] = append(out[song], f)
			}
		}
	}
	return out
}

func pickBackground(pool []string, cur string, r *rand.Rand) string {
	if len(pool) == 0 {
		return ""
	}
	if len(pool) == 1 {
		return pool[0]
	}
	for {
		p := pool[r.IntN(len(pool))]
		if p != cur {
			return p
		}
	}
}

const (
	musicBGHold  = 7 * 60
	musicBGFade  = 90
	musicBGQuick = 20
)

type musicRoom struct {
	entries []musicEntry
	playing int
	shotT   int
	played  bool
	hidden  bool

	pool     []string
	songs    map[string][]string
	song     string
	queue    []string
	bg, prev string
	bgT      int
	fade     int
	traced   string
	rnd      *rand.Rand
}

func newMusicRoom(g *Game) *musicRoom {
	r := &musicRoom{entries: musicEntries(g.db), playing: -1, rnd: g.rng()}
	if r.rnd == nil {
		n := uint64(time.Now().UnixNano())
		r.rnd = rand.New(rand.NewPCG(n, n*7+1)) //nolint:gosec // Music-room shuffle, not cryptographic randomness.
	}
	r.pool = g.musicBackgrounds()
	r.songs = g.songPictures()
	r.bg = pickBackground(r.pool, "", r.rnd)
	r.fade = musicBGFade
	r.bgT = musicBGFade
	return r
}

func (r *musicRoom) nextBackground(fade int) {
	next := ""
	for next == "" && len(r.queue) > 0 {
		next, r.queue = r.queue[0], r.queue[1:]
		if next == r.bg {
			next = ""
		}
	}
	if next == "" {
		next = pickBackground(r.pool, r.bg, r.rnd)
	}
	if next == "" || next == r.bg {
		if r.bgT >= musicBGHold {
			r.bgT = r.fade
		}
		return
	}
	r.prev, r.bg, r.bgT, r.fade = r.bg, next, 0, fade
}

func (r *musicRoom) items() []listItem {
	items := make([]listItem, len(r.entries))
	for i, e := range r.entries {
		items[i] = listItem{label: e.label}
		if i == r.playing {
			items[i].right = "♪"
		}
	}
	return items
}

func (r *musicRoom) play(g *Game, i int) {
	if i < 0 || i >= len(r.entries) {
		r.playing, r.shotT = -1, 0
		r.song, r.queue = "", nil
		sound.Stop()
		g.songOverride, g.moodOverride = "", ""
		g.baseSong, g.baseMood = "", ""
		g.emit("gallery", traceLine{"music": "", "stop": true})
		return
	}
	e := r.entries[i]
	r.playing, r.played = i, true
	r.shotT = 0
	if e.oneShot {
		r.shotT = r.moduleTicks(g, e.song)
	}
	if e.song != r.song {
		r.song, r.queue = e.song, slices.Clone(r.songs[e.song])
		r.nextBackground(musicBGQuick)
	}
	g.Music(data.MusicCue{Song: e.song, Mood: e.mood})
	g.emit("gallery", traceLine{"music": e.song, "mood": e.mood, "title": e.label})
}

func (r *musicRoom) moduleTicks(g *Game, song string) int {
	s := g.db.Music.Songs[song]
	if s.BPM <= 0 {
		return 0
	}
	beats := g.db.Music.Bars * g.db.Music.Beats
	return beats * 60 * 60 / s.BPM
}

func (r *musicRoom) step(g *Game, d, from int) {
	n := len(r.entries)
	if n == 0 {
		return
	}
	cur := r.playing
	if cur < 0 {
		cur = from - d
	}
	r.play(g, ((cur+d)%n+n)%n)
}

func (r *musicRoom) tick() {
	r.bgT++
	if r.playing >= 0 && r.bgT >= musicBGHold {
		r.nextBackground(musicBGFade)
	}
	if r.shotT > 0 {
		r.shotT--
		if r.shotT == 0 && r.playing >= 0 {
			song := r.entries[r.playing].song
			for i, e := range r.entries {
				if e.song == song && !e.oneShot {
					r.playing = i
					break
				}
			}
		}
	}
}

func musicPanel(sw, sh float64) (x, y, w, h float64) {
	w, h = 520, 110
	return sw - w - 40, sh - 176, w, h
}

func musicButtons(sw, sh float64) [3]image.Rectangle {
	x, y, w, h := musicPanel(sw, sh)
	var out [3]image.Rectangle
	bw, gap := 96.0, 16.0
	left := x + w/2 - (3*bw+2*gap)/2
	for i := range 3 {
		bx := left + float64(i)*(bw+gap)
		out[i] = image.Rect(int(bx), int(y+h-50), int(bx+bw), int(y+h-12))
	}
	return out
}

func (s *galleryScreen) updateMusic(g *Game) {
	r := s.music
	if r.hidden {
		if g.in.ok() || g.in.cancel() || g.in.click || g.in.pressed[btnHide] {
			r.hidden = false
		}
		return
	}
	if g.in.pressed[btnHide] {
		r.hidden = true
		return
	}
	if t := g.in.tab(); t != 0 {
		g.sfx("cursor")
		r.step(g, t, s.menu.sel)
		s.syncMusicList()
		return
	}
	if g.in.click {
		sw, sh := g.screen()
		for i, b := range musicButtons(sw, sh) {
			if !g.in.mouse.In(b) {
				continue
			}
			g.sfx("decide")
			switch i {
			case 0:
				r.step(g, -1, s.menu.sel)
			case 1:
				if r.playing >= 0 {
					r.play(g, -1)
				} else {
					r.play(g, s.menu.sel)
				}
			case 2:
				r.step(g, 1, s.menu.sel)
			}
			s.syncMusicList()
			return
		}
	}
	chose, _ := s.menu.update(g)
	if !chose {
		return
	}
	if s.menu.sel == r.playing {
		r.play(g, -1)
	} else {
		r.play(g, s.menu.sel)
	}
	s.syncMusicList()
}

func (s *galleryScreen) syncMusicList() {
	r := s.music
	sel, top := s.menu.sel, s.menu.top
	s.menu.items = r.items()
	s.menu.sel, s.menu.top = sel, top
	if r.playing >= 0 {
		s.menu.sel = r.playing
	}
	s.menu.scroll()
}

func (g *Game) drawMusicBG(dst *ebiten.Image, r *musicRoom) {
	sw, sh := g.screen()
	if r.bg == "" {
		g.drawPicture(dst, g.db.Game.TitleBG, 0, 0, sw, sh, 1)
		return
	}
	a := float32(min(1, float64(r.bgT)/float64(max(1, r.fade))))
	if r.prev != "" && a < 1 {
		g.drawCG(dst, data.CG{File: r.prev}, 1)
	} else if r.prev == "" {
		fill(dst, colorBlack)
	}
	g.drawCG(dst, data.CG{File: r.bg}, a)
}

func (s *galleryScreen) drawMusic(g *Game, dst *ebiten.Image) {
	r := s.music
	sw, sh := g.screen()
	if r.hidden {
		return
	}
	l := s.menu
	g.traceList(l)
	panel := withAlpha(g.th.window, 0xa8)
	gfx.FillRoundRect(dst, float32(l.x), float32(l.y), float32(l.w), float32(l.height()), 16, panel)
	l.drawRows(g, dst, true)

	x, y, w, h := musicPanel(sw, sh)
	gfx.FillRoundRect(dst, float32(x), float32(y), float32(w), float32(h), 16, panel)
	now := lang.T("とまっています")
	c := g.th.dim
	if r.playing >= 0 {
		now = "♪ " + r.entries[r.playing].label
		c = g.th.text
	}
	ls, sz := gfx.FitLines(now, sizeM, w-40, 1)
	if len(ls) > 0 {
		gfx.DrawTextOutlineColor(dst, ls[0], x+w/2, y+14, sz, c, g.th.window, 1)
	}
	labels := [3]string{"◀◀", "■", "▶▶"}
	if r.playing < 0 {
		labels[1] = "▶"
	}
	for i, b := range musicButtons(sw, sh) {
		bx, by := float64(b.Min.X), float64(b.Min.Y)
		bw, bh := float64(b.Dx()), float64(b.Dy())
		bc := withAlpha(g.th.window, 0xc0)
		if g.in.mouse.In(b) {
			bc = withAlpha(g.th.accent, 0x90)
		}
		gfx.FillRoundRect(dst, float32(bx), float32(by), float32(bw), float32(bh), 10, bc)
		gfx.StrokeRoundRect(dst, float32(bx), float32(by), float32(bw), float32(bh), 10, 1.5, withAlpha(g.th.border, 0x90))
		gfx.DrawText(dst, labels[i], bx+bw/2, by+(bh-sizeS*1.3)/2, sizeS, g.th.text)
	}
}
