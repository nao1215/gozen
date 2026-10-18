package play

import (
	"fmt"
	"image/color"
	"math"

	"github.com/hajimehoshi/ebiten/v2"

	"github.com/nao1215/gozen/battle"
	data "github.com/nao1215/gozen/internal/layer2/gamedata"
	"github.com/nao1215/gozen/internal/layer5/gfx"
	"github.com/nao1215/gozen/internal/layer5/sound"
	lang "github.com/nao1215/gozen/text"
)

const (
	hitstopCrit  = 7
	hitstopWeak  = 5
	chargeTicks  = 54
	finishStop   = 12
	finishStopB  = 26
	finishSlow   = 24
	finishSlowB  = 70
	bossWinHold  = 90
	bossWinStamp = 12
)

var colorGold = color.NRGBA{0xff, 0xe0, 0x60, 0xff}

type drama struct {
	freeze     int
	slow       int
	charge     int
	chargeLen  int
	chargeU    *battle.Unit
	chargeC    color.NRGBA
	chargeDone bool
	bossWon    bool
	winT       int
	winSong    string
}

func (g *Game) fxk(p *float64) float64 { return g.db.Game.UI.BattleFx.Get(p) }

func (s *battleScene) dramaK(g *Game, p, auto *float64, def float64) float64 {
	k := g.fxk(p)
	if s.auto {
		k *= data.BattleFxAuto{}.Get(auto, def)
	}
	return k
}

func (g *Game) holdTicks(n int, k float64) int {
	if g.in.fast || k <= 0 {
		return 0
	}
	return g.fxTicks(int(math.Round(float64(n) * k)))
}

func (s *battleScene) stepDrama(g *Game) bool {
	if s.dr.freeze > 0 {
		s.dr.freeze--
		return true
	}
	if s.dr.charge > 0 {
		s.dr.charge--
		s.chargeTick(g)
	}
	if s.dr.slow > 0 {
		s.dr.slow--
		if s.dr.slow%2 == 1 {
			return true
		}
	}
	return false
}

func (s *battleScene) hitDrama(g *Game, st battle.Step) {
	fx := g.db.Game.UI.BattleFx
	switch {
	case st.Crit:
		s.dr.freeze = max(s.dr.freeze, g.holdTicks(hitstopCrit, s.dramaK(g, fx.Hitstop, fx.Auto.Hitstop, 0)))
		g.shake = max(g.shake, int(18*g.fxk(fx.Shake)))
	case st.Weak:
		s.dr.freeze = max(s.dr.freeze, g.holdTicks(hitstopWeak, s.dramaK(g, fx.Hitstop, fx.Auto.Hitstop, 0)))
		g.shake = max(g.shake, int(13*g.fxk(fx.Shake)))
	}
}

func (g *Game) flashAlpha(a float64) float64 {
	return a * g.fxk(g.db.Game.UI.BattleFx.Flash)
}

func (s *battleScene) lastFoe(u *battle.Unit) bool {
	for _, o := range s.b.Foes {
		if o != u && s.shownHP[o] > 0 && !o.Fled {
			if _, gone := s.gone[o]; !gone {
				return false
			}
		}
	}
	return true
}

func (s *battleScene) finishDrama(g *Game, u *battle.Unit) {
	fx := g.db.Game.UI.BattleFx
	k := g.fxk(fx.Finish)
	stop, slow, shake := finishStop, finishSlow, 12.0
	if u.Def != nil && u.Def.Boss {
		stop, slow, shake = finishStopB, finishSlowB, 24
	}
	hold := s.dramaK(g, fx.Finish, fx.Auto.Finish, 0)
	s.dr.freeze = max(s.dr.freeze, g.holdTicks(stop, hold))
	s.dr.slow = max(s.dr.slow, g.holdTicks(slow, hold))
	g.shake = max(g.shake, int(shake*g.fxk(fx.Shake)*math.Min(1, k+0.3)))
	g.emit("battle_fx", traceLine{"what": "finish", "boss": u.Def != nil && u.Def.Boss, "stop": s.dr.freeze, "slow": s.dr.slow})
}

func (s *battleScene) startCharge(g *Game, st battle.Step) bool {
	u := st.User
	if u == nil || !u.Enemy || u.Def == nil || !u.Def.Boss || g.in.fast {
		return false
	}
	use := s.usage(st.Skill)
	if use == nil || (!use.Cutin && use.Scope != "all_enemies" && use.Scope != "everyone") {
		return false
	}
	fx := g.db.Game.UI.BattleFx
	n := g.holdTicks(chargeTicks, s.dramaK(g, fx.Charge, fx.Auto.Charge, 0))
	if n < 8 {
		return false
	}
	s.dr.charge, s.dr.chargeLen, s.dr.chargeU = n, n, u
	s.dr.chargeC = fxColor(fxKind(use), st.Color)
	s.msg = []string{fmt.Sprintf(lang.T("%sは ちからを ためている……"), u.Name)}
	s.actor = u
	s.stepT = n
	g.sfx("magic")
	g.emit("battle_fx", traceLine{"what": "charge", "foe": u.Name, "ticks": n})
	return true
}

func (s *battleScene) chargeTick(g *Game) {
	u := s.dr.chargeU
	if u == nil {
		return
	}
	p := 1 - float64(s.dr.charge)/float64(max(1, s.dr.chargeLen))
	s.shakeU[u] = max(s.shakeU[u], int(3+7*p))
	k := g.fxk(g.db.Game.UI.BattleFx.Shake)
	g.shake = max(g.shake, int((1+4*p)*k))
	if s.dr.charge == 0 {
		s.dr.chargeU = nil
	}
}

func (s *battleScene) chargeScale(u *battle.Unit) float64 {
	if s.dr.chargeU != u || s.dr.chargeLen == 0 {
		return 1
	}
	p := 1 - float64(s.dr.charge)/float64(s.dr.chargeLen)
	return 1 + 0.07*easeOut(p)
}

func (s *battleScene) drawCharge(g *Game, dst *ebiten.Image) {
	u := s.dr.chargeU
	if u == nil || s.dr.chargeLen == 0 {
		return
	}
	r := s.unitRect(g, u)
	cx, cy := float64(r.Min.X+r.Max.X)/2, float64(r.Min.Y)+float64(r.Dy())*0.5
	p := 1 - float64(s.dr.charge)/float64(s.dr.chargeLen)
	b := &s.fxb
	for i := range 3 {
		q := math.Mod(p*3+float64(i)/3, 1)
		rad := (1 - easeOut(q)) * float64(r.Dx()) * 0.9
		b.ring(cx, cy, rad+10, (rad+10)*0.55, 6+10*(1-q), s.dr.chargeC, 0.55*q*(0.4+0.6*p))
	}
	b.flush(dst)
}

func (g *Game) popSize(p popup) (size, lift float64) {
	k := g.fxk(g.db.Game.UI.BattleFx.Pop)
	size, pop := 40.0, 0.35
	if p.big {
		size, pop = 40+30*k, 1.1*k
	}
	t := float64(p.t)
	if p.big {
		size *= 1 + pop*math.Exp(-t/5)*math.Cos(t*0.9)
		lift = math.Abs(math.Sin(t*0.45)) * 34 * k * math.Exp(-t/9)
	} else if p.t < 8 {
		size *= 1 + pop*(1-t/8)
	}
	return max(size, 20), lift
}

func (s *battleScene) bossWinMusic(g *Game) bool {
	cue := s.b.Troop.WinMusic
	if cue.Song == "" {
		cue = g.db.Game.Battle.BossWinMusic
	}
	if cue.Song == "" {
		return false
	}
	if _, ok := g.db.Music.Songs[cue.Song]; !ok {
		return false
	}
	mood := cue.Mood
	if mood == "" {
		mood = sound.Neutral
	}
	sound.PlayOnce(cue.Song, mood)
	g.baseSong, g.baseMood, g.lastMood = cue.Song, mood, mood
	s.dr.winSong = cue.Song
	g.emit("battle_music", traceLine{"win_song": cue.Song, "mood": mood})
	return true
}

func (g *Game) songTicks(id string) int {
	s, ok := g.db.Music.Songs[id]
	if !ok || s.File != "" || s.BPM <= 0 {
		return 20 * 60
	}
	bars, beats := g.db.Music.Bars, g.db.Music.Beats
	if bars <= 0 {
		bars = 8
	}
	if beats <= 0 {
		beats = 4
	}
	return int(float64(bars*beats)*60/float64(s.BPM)*60) + 30
}

func (g *Game) holdWinSong(song string) {
	g.fanfareSong, g.fanfareT = song, g.songTicks(song)
}

func (s *battleScene) winHold(g *Game) int {
	fx := g.db.Game.UI.BattleFx
	return g.holdTicks(bossWinHold, max(0.3, g.fxk(fx.Finish))*s.dramaK(g, nil, fx.Auto.Win, 0.4))
}

func (g *Game) stepFanfare() {
	if g.fanfareSong == "" {
		return
	}
	if g.baseSong != g.fanfareSong {
		g.fanfareSong, g.fanfareT = "", 0
		return
	}
	if g.fanfareT > 0 && (!sound.Active() || sound.WantedSong() != "") {
		g.fanfareT--
		return
	}
	g.fanfareSong = ""
	if g.currentBattle() == nil {
		g.playLocationMusic()
	}
}

func (s *battleScene) drawBossWin(g *Game, dst *ebiten.Image) {
	sw, sh := g.screen()
	t := s.dr.winT
	open := math.Min(1, float64(t)/10)
	h := 190 * open
	y := sh*0.40 - h/2
	gfx.FillRect(dst, 0, float32(y), float32(sw), float32(h), withAlpha(g.th.window, uint8(235*open)), false)
	stripe := withAlpha(colorGold, uint8(230*open))
	gfx.FillRect(dst, 0, float32(y), float32(sw), 5, stripe, false)
	gfx.FillRect(dst, 0, float32(y+h-5), float32(sw), 5, stripe, false)
	if open < 1 {
		return
	}
	title := lang.T("迷宮の番人")
	if s.bossTitle != "" {
		title = s.bossTitle
	}
	slide := easeOut(math.Min(1, float64(t-6)/14))
	gfx.DrawTextOutlineColor(dst, title+"　"+s.boss, sw/2+(1-slide)*sw*0.4, y+22, sizeM, colorGold, g.th.window, float32(slide))

	q := math.Min(1, float64(t)/bossWinStamp)
	size := sizeL * 2.2 * (1 + 1.3*(1-easeOut(q)))
	a := float32(math.Min(1, q*1.6))
	gfx.DrawTextOutlineColor(dst, lang.T("番人を たおした！"), sw/2, y+h*0.30, size, colorWhite, g.th.window, a)
	if len(s.results) > 0 && t > bossWinStamp+8 {
		gfx.DrawText(dst, s.results[0], sw/2, y+h-52, sizeM, g.th.text)
	}
}
