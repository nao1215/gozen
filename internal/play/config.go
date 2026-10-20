package play

import (
	"cmp"
	"encoding/json"
	"errors"
	"fmt"
	"image/color"
	"io/fs"
	"log"
	"math"
	"slices"

	"github.com/hajimehoshi/ebiten/v2"

	"github.com/nao1215/gozen/internal/layer5/gfx"
	"github.com/nao1215/gozen/internal/layer5/sound"
	lang "github.com/nao1215/gozen/internal/text"
)

type config struct {
	TextSpeed  int  `json:"text_speed"`
	AutoWait   int  `json:"auto_wait"`
	Window     int  `json:"window"`
	BGM        int  `json:"bgm"`
	SE         int  `json:"se"`
	Fullscreen bool `json:"fullscreen"`

	BattleSpeed int  `json:"battle_speed"`
	SkipAll     bool `json:"skip_all"`
	ManualSP    bool `json:"manual_sp"`

	BattleAuto *bool `json:"battle_auto,omitempty"`

	// Voice is the volume of voices (0 to 10), VoiceBy that of each voiced actor (ADR 0028).
	Voice   int            `json:"voice"`
	VoiceBy map[string]int `json:"voice_by,omitempty"`
	// Language is the language the player chose, one of game.yaml's languages (ADR 0027).
	Language string `json:"language,omitempty"`
}

func defaultConfig() config {
	return config{TextSpeed: 2, AutoWait: 2, Window: 9, BGM: 7, SE: 7, Voice: 8}
}

func (c config) textStep() int { return []int{1, 2, 3, 5, 9999}[clampInt(c.TextSpeed, 0, 4)] }

func (c config) autoScale() float64 {
	return []float64{0.5, 0.75, 1, 1.4, 2}[clampInt(c.AutoWait, 0, 4)]
}

func (c config) battleScale() float64 { return []float64{1, 1.5, 2.2}[clampInt(c.BattleSpeed, 0, 2)] }

func clampInt(v, lo, hi int) int { return max(lo, min(hi, v)) }

func (g *Game) configPath() string { return "config.json" }

func (g *Game) loadConfig() {
	g.cfg = defaultConfig()
	if b, err := g.storeRead(g.configPath()); err == nil {
		if err := json.Unmarshal(b, &g.cfg); err != nil {
			g.failure = fmt.Errorf("settings: %w", err)
			return
		}
	} else if !errors.Is(err, fs.ErrNotExist) {
		g.failure = fmt.Errorf("settings: %w", err)
		return
	}
	g.applyConfig()
	g.loadRead()
}

func (g *Game) saveConfig() error {
	b, err := json.MarshalIndent(g.cfg, "", "  ")
	if err == nil {
		err = g.store.Write(g.configPath(), b)
	}
	if err != nil {
		log.Printf("config: %v", err)
	}
	return err
}

func (g *Game) applyConfig() {
	if g.windowA == 0 {
		g.windowA = g.th.window.A
	}
	g.th.window.A = uint8(float64(g.windowA) * float64(clampInt(g.cfg.Window, 0, 10)) / 10)
	sound.SetVolumes(float64(clampInt(g.cfg.BGM, 0, 10))/10, float64(clampInt(g.cfg.SE, 0, 10))/10)
	sound.SetVoiceVolume(float64(clampInt(g.cfg.Voice, 0, 10)) / 10)
	if g.cfg.Fullscreen != ebiten.IsFullscreen() {
		ebiten.SetFullscreen(g.cfg.Fullscreen)
	}
}

func (g *Game) toggleFullscreen() {
	g.cfg.Fullscreen = !g.cfg.Fullscreen
	g.applyConfig()
	if err := g.saveConfig(); err != nil {
		g.failure = err
	}
}

type configScreen struct {
	list *list
	done func()
	err  string
	// voiceRow is the row of the voice volume (-1: the game has no voices), followed by a
	// row for each of voiced.
	voiceRow int
	voiced   []string
	// langRow is the row of the language (-1: the game offers one).
	langRow int
}

func newConfigScreen(g *Game) *configScreen {
	s := &configScreen{list: newList(nil, 0, 0, 640, 11)}
	sw, sh := g.screen()
	s.list.x, s.list.y = (sw-640)/2, sh*0.13
	s.refresh(g)
	return s
}

func (s *configScreen) overlay() bool { return true }

func (s *configScreen) liveKeep(g *Game) scene {
	s.refresh(g)
	return s
}
func (s *configScreen) modal() bool { return true }

func (s *configScreen) refresh(g *Game) {
	c := g.cfg
	speed := []string{lang.T("おそい"), lang.T("ふつう"), lang.T("はやい"), lang.T("とてもはやい"), lang.T("いっしゅん")}
	wait := []string{lang.T("みじかい"), lang.T("ややみじかい"), lang.T("ふつう"), lang.T("ややながい"), lang.T("ながい")}
	bar := func(v int) string { return fmt.Sprintf("%3d%%", clampInt(v, 0, 10)*10) }
	on := map[bool]string{true: lang.T("オン"), false: lang.T("オフ")}
	s.list.items = []listItem{
		{label: lang.T("文字の速さ"), right: "◀ " + speed[clampInt(c.TextSpeed, 0, 4)] + " ▶"},
		{label: lang.T("オートの待ち"), right: "◀ " + wait[clampInt(c.AutoWait, 0, 4)] + " ▶"},
		{label: lang.T("ウィンドウの濃さ"), right: "◀ " + bar(c.Window) + " ▶"},
		{label: lang.T("BGM の音量"), right: "◀ " + bar(c.BGM) + " ▶"},
		{label: lang.T("効果音の音量"), right: "◀ " + bar(c.SE) + " ▶"},
		{label: lang.T("全画面"), right: "◀ " + on[c.Fullscreen] + " ▶"},
		{label: lang.T("戦闘の速さ"), right: "◀ " + []string{lang.T("ふつう"), lang.T("はやい"), lang.T("とてもはやい")}[clampInt(c.BattleSpeed, 0, 2)] + " ▶"},
		{label: lang.T("戦闘をオートではじめる"), right: "◀ " + on[g.autoStartBattle()] + " ▶"},
		{label: lang.T("早送りで飛ばす文"), right: "◀ " + map[bool]string{false: lang.T("読んだ文だけ"), true: lang.T("すべて")}[c.SkipAll] + " ▶"},
	}
	if g.showSP() {
		s.list.items = append(s.list.items, listItem{label: lang.T("SP の割り振り"), right: "◀ " + map[bool]string{false: lang.T("おまかせ"), true: lang.T("自分で")}[c.ManualSP] + " ▶"})
	}
	s.voiceRow, s.voiced = -1, nil
	if g.usesVoices() {
		s.voiceRow, s.voiced = len(s.list.items), g.voicedActors()
		s.list.items = append(s.list.items, listItem{label: lang.T("ボイスの音量"), right: "◀ " + bar(c.Voice) + " ▶"})
		for _, id := range s.voiced {
			v, ok := c.VoiceBy[id]
			if !ok {
				v = 10
			}
			s.list.items = append(s.list.items, listItem{label: fmt.Sprintf(lang.T("　%s の声"), g.db.Actors[id].Name), right: "◀ " + bar(v) + " ▶"})
		}
	}
	s.langRow = -1
	if offered := g.db.Game.Offered(); len(offered) > 1 {
		s.langRow = len(s.list.items)
		s.list.items = append(s.list.items, listItem{label: lang.T("言語"), right: "◀ " + languageName(g.language()) + " ▶"})
	}
	s.list.items = append(s.list.items, listItem{label: lang.T("もどる")})
}

func (g *Game) showSP() bool {
	if g.spUsed == nil {
		v := g.db.Uses("sp_suggest")
		g.spUsed = &v
	}
	return *g.spUsed
}

func (s *configScreen) update(g *Game) {
	l := s.list
	last := len(l.items) - 1
	d, wrap := 0, false
	if g.in.left() {
		d = -1
	}
	if g.in.right() {
		d = 1
	}

	if g.in.click {
		for i := l.top; i < min(len(l.items), l.top+l.rows); i++ {
			r := l.rowRect(i)
			if !g.in.mouse.In(r) || i == last {
				continue
			}
			l.sel = i
			x := float64(g.in.mouse.X)
			switch {
			case x >= float64(r.Max.X)-60:
				d = 1
			case x >= float64(r.Min.X)+l.w*0.55:
				d = -1
			default:
				d, wrap = 1, true
			}
			g.in.click = false
		}
	}

	if d == 0 && g.in.ok() && l.sel != last {
		d, wrap = 1, true
	}
	if d != 0 && l.sel != last {
		s.change(g, d, wrap)
		return
	}
	chose, cancel := l.update(g)
	if cancel || (chose && l.sel == last) {
		if err := g.saveConfig(); err != nil && s.err == "" {
			s.err = lang.T("設定を保存できなかった: ") + err.Error()
			g.sfx("buzzer")
			return
		}
		g.sfx("cancel")
		g.pop()
		if s.done != nil {
			s.done()
		}
	}
}

const cfgSP = 9

func (s *configScreen) change(g *Game, d int, wrap bool) {
	c := &g.cfg
	step := func(v, lo, hi int) int {
		v += d
		if wrap && v > hi {
			return lo
		}
		return clampInt(v, lo, hi)
	}
	if s.langRow >= 0 && s.list.sel == s.langRow {
		offered := g.db.Game.Offered()
		i := slices.Index(offered, g.language())
		c.Language = offered[(i+d+len(offered))%len(offered)]
		if err := g.useLanguage(); err != nil {
			log.Printf("language: %v", err)
		}
	}
	switch i := s.list.sel - s.voiceRow; {
	case s.voiceRow >= 0 && i == 0:
		c.Voice = step(c.Voice, 0, 10)
	case s.voiceRow >= 0 && i > 0 && i <= len(s.voiced):
		id := s.voiced[i-1]
		v, ok := c.VoiceBy[id]
		if !ok {
			v = 10
		}
		if c.VoiceBy == nil {
			c.VoiceBy = map[string]int{}
		}
		c.VoiceBy[id] = step(v, 0, 10)
	}
	switch s.list.sel {
	case 0:
		c.TextSpeed = step(c.TextSpeed, 0, 4)
	case 1:
		c.AutoWait = step(c.AutoWait, 0, 4)
	case 2:
		c.Window = step(c.Window, 0, 10)
	case 3:
		c.BGM = step(c.BGM, 0, 10)
	case 4:
		c.SE = step(c.SE, 0, 10)
	case 5:
		c.Fullscreen = !c.Fullscreen
	case 6:
		c.BattleSpeed = step(c.BattleSpeed, 0, 2)
	case 7:
		v := !g.autoStartBattle()
		c.BattleAuto = &v
	case 8:
		c.SkipAll = !c.SkipAll
	case cfgSP:
		if g.showSP() {
			c.ManualSP = !c.ManualSP
		}
	}
	g.applyConfig()
	g.sfx("cursor")
	s.refresh(g)
}

func (s *configScreen) draw(g *Game, dst *ebiten.Image) {
	sw, sh := g.screen()
	fill(dst, withAlpha(colorBlack, 0x80))
	l := s.list

	gfx.FillRoundRect(dst, float32(l.x-30), float32(l.y-80), float32(l.w+60), float32(l.height()+190), 16, withAlpha(g.th.window, 0xff))
	g.window(dst, l.x-30, l.y-80, l.w+60, l.height()+190)
	gfx.DrawTextOutlineColor(dst, lang.T("せってい"), sw/2, l.y-62, sizeM, g.th.text, g.th.window, 1)
	l.draw(g, dst, true)
	if s.err != "" {
		ls, sz := gfx.FitLines(s.err+lang.T("（もう一度 閉じると そのまま 閉じる）"), sizeS, l.w, 2)
		for i, ln := range ls {
			gfx.DrawTextOutlineColor(dst, ln, sw/2, l.y-36+float64(i)*gfx.LineHeight(sz)-float64(len(ls)-1)*gfx.LineHeight(sz), sz, color.NRGBA{0xff, 0x90, 0x90, 0xff}, g.th.window, 1)
		}
	}

	y := l.y + l.height() + 16

	for i, bx := 0, l.x+6; bx < l.x+l.w-6; i, bx = i+1, bx+24 {
		c := color.NRGBA{0xe8, 0xe4, 0xdc, 0xff}
		if i%2 == 1 {
			c = color.NRGBA{0x30, 0x34, 0x40, 0xff}
		}
		rect(dst, bx, y+6, math.Min(24, l.x+l.w-6-bx), 58, c)
	}
	g.window(dst, l.x, y, l.w, 70)
	sample := lang.T("見本の文章です。ウィンドウの濃さを確かめてください。")
	if l.sel == cfgSP && g.showSP() {
		sample = lang.T("おまかせ：SP が余っていれば、振り方を見せて勧めます。頼めばまとめて振ります。")
		if g.cfg.ManualSP {
			sample = lang.T("自分で：勧めません。スキルツリーで自分で振ります。")
		}
	}
	ls, sz := gfx.FitLines(sample, sizeS, l.w-48, 2)
	for i, ln := range ls {
		g.text(dst, ln, l.x+24, y+20+float64(i)*gfx.LineHeight(sz)-float64(len(ls)-1)*gfx.LineHeight(sz)/2, sz, g.th.text)
	}
	_ = sh
}

// language is the language shown: the one chosen in the settings when the game offers it,
// otherwise game.yaml's language.
func (g *Game) language() string {
	if l := g.cfg.Language; l != "" && slices.Contains(g.db.Game.Offered(), l) {
		return l
	}
	return cmp.Or(g.db.Game.Language, lang.English)
}

// useLanguage shows the game's text and the engine's words in the language (ADR 0027).
func (g *Game) useLanguage() error {
	code := g.language()
	g.db.Show(code)
	return lang.Use(code, g.db.EngineWords(code))
}

// languageName is a language as the settings show it, in that language.
func languageName(code string) string {
	switch code {
	case lang.English:
		return "English"
	case lang.Japanese:
		return "日本語"
	}
	return code
}
