package play

import (
	"image/color"
	"math"

	"github.com/hajimehoshi/ebiten/v2"

	"github.com/nao1215/gozen/internal/event"
	data "github.com/nao1215/gozen/internal/layer2/gamedata"
	"github.com/nao1215/gozen/internal/layer5/sound"
)

// Staging (ADR 0028): the portraits an event stands on the novel window's stage with
// stand, the map forms of effect (a portrait's shake, a reaction of a moving portrait, a
// fade through a color), and the waits for a sound.

// castFigure is a portrait stood by stand.
type castFigure struct {
	key, face string // key is speakerKey's: actor:<id> or name:<who>
	x         float64
	enter     string
	entered   int
	leave     string
	left      int // the tick it began to leave; leave != "" marks a departure, including tick 0
}

// colorFade is a fade of the screen through a color: out, hold, then back in.
type colorFade struct {
	c              color.NRGBA
	out, hold, age int
}

const (
	castIn  = 12 // ticks a portrait takes to come in
	castOut = 12 // and to leave
)

func castKey(db *data.DB, who string) string {
	if _, ok := db.Actors[who]; ok {
		return "actor:" + who
	}
	return "name:" + who
}

// stand applies a stand: an event that uses it places its portraits only as written.
func (g *Game) stand(e event.StandEffect) {
	if !g.castOn {
		g.castOn = true
		g.leaveStage() // the portraits placed by the speakers so far leave
	}
	if e.None {
		for i := range g.cast {
			if g.cast[i].leave == "" {
				g.cast[i].leave, g.cast[i].left = "fade", g.tick
			}
		}
		return
	}
	key := castKey(g.db, e.Who)
	for i := range g.cast {
		f := &g.cast[i]
		if f.key != key || f.leave != "" {
			continue
		}
		if e.Leave != "" {
			f.leave, f.left = e.Leave, g.tick
			return
		}
		if f.x != e.At {
			g.stageGen++ // a move written in the event, not a portrait that wandered
		}
		if !e.Default {
			f.face = e.Face
		}
		f.x = e.At
		return
	}
	if e.Leave != "" {
		return
	}
	g.pics.get(e.Face)
	g.cast = append(g.cast, castFigure{key: key, face: e.Face, x: e.At, enter: e.Enter, entered: g.tick})
}

// castSays lets the speaker's line change the face of its portrait.
func (g *Game) castSays(s event.Say) {
	if s.Face == "" {
		return
	}
	k := speakerKey(s)
	for i := range g.cast {
		if g.cast[i].key == k && g.cast[i].leave == "" {
			g.cast[i].face = s.Face
		}
	}
}

// clearCast ends the staging of an event.
func (g *Game) clearCast() {
	g.cast, g.castOn = nil, false
}

// castFigures are the portraits stood now, where they are drawn.
func (g *Game) castFigures(speaker string) []stageFigure {
	sw, _ := g.screen()
	var out []stageFigure
	kept := g.cast[:0]
	for _, f := range g.cast {
		if f.leave != "" && g.tick-f.left >= castOut {
			continue
		}
		kept = append(kept, f)
		x, alpha := f.x, float32(1)
		if k := float64(g.tick-f.entered) / castIn; k < 1 {
			if f.enter == "slide" {
				x = edge(f.x) + (f.x-edge(f.x))*easeOut(k)
			} else {
				alpha = float32(k)
			}
		}
		if f.leave != "" {
			k := float64(g.tick-f.left) / castOut
			if f.leave == "slide" {
				x = f.x + (edge(f.x)-f.x)*k*k
			} else {
				alpha *= float32(1 - k)
			}
		}
		dim := float32(0.75)
		if f.key == speaker {
			dim = 0
		}
		out = append(out, stageFigure{f.key, f.face, sw*x + g.figShakeX(f.key), alpha, false, dim, f.leave != ""})
	}
	g.cast = kept
	return out
}

// edge is the side of the screen a portrait at x slides from or to.
func edge(x float64) float64 {
	if x < 0.5 {
		return -0.25
	}
	return 1.25
}

// figShakeX is how far a portrait shaken by effect: {name: shake, who} is moved now.
func (g *Game) figShakeX(key string) float64 {
	n := g.figShake[key]
	if n <= 0 {
		return 0
	}
	return math.Sin(float64(n)*1.9) * 14 * math.Min(1, float64(n)/10)
}

// visual applies an effect: the old words keep their old look; a map times them, shakes a
// portrait, or fades through a color; any other word is a reaction of a moving portrait.
func (g *Game) visual(e event.VisualEffect) {
	c := e.Cue
	ticks := func(def int) int {
		if c.Seconds > 0 {
			return int(math.Round(c.Seconds * 60))
		}
		return def
	}
	switch c.Name {
	case "shake":
		if c.Who != "" {
			if g.figShake == nil {
				g.figShake = map[string]int{}
			}
			g.figShake[castKey(g.db, c.Who)] = ticks(24)
			return
		}
		if c.Seconds > 0 {
			g.shakeFor = ticks(24)
			return
		}
		g.Effect("shake")
	case "flash":
		g.flash = ticks(20)
	case "fade":
		if !c.Color.Valid && c.Seconds == 0 && c.Hold == 0 {
			g.Effect("fade")
			return
		}
		col := colorBlack
		if c.Color.Valid {
			col = c.Color.NRGBA
			col.A = 0xff
		}
		g.fadeColor = &colorFade{c: col, out: ticks(30), hold: int(math.Round(c.Hold * 60))}
	default:
		g.react(c.Who, c.Name)
	}
}

// react plays a reaction on the moving portrait of who (the last speaker when empty).
func (g *Game) react(who, name string) {
	if who == "" {
		who = g.talk.say.Actor
	}
	p := g.puppets.live[who]
	if p == nil {
		return // the portrait does not move (or has not been shown yet)
	}
	if r, ok := p.Rig().Spec.ReactionOf(name); ok {
		p.Motion.React(r)
	}
}

// stepStaging ages the shakes and the fade through a color.
func (g *Game) stepStaging() {
	for k, n := range g.figShake {
		if n <= 1 {
			delete(g.figShake, k)
		} else {
			g.figShake[k] = n - 1
		}
	}
	if g.shakeFor > 0 {
		g.shakeFor--
		g.shake = max(g.shake, 12)
	}
	if f := g.fadeColor; f != nil {
		f.age++
		if f.age >= 2*f.out+f.hold {
			g.fadeColor = nil
		}
	}
}

// drawFadeColor covers the screen with the color of a fade.
func (g *Game) drawFadeColor(screen *ebiten.Image) {
	f := g.fadeColor
	if f == nil {
		return
	}
	a := 1.0
	switch {
	case f.age < f.out:
		a = float64(f.age) / float64(max(1, f.out))
	case f.age >= f.out+f.hold:
		a = 1 - float64(f.age-f.out-f.hold)/float64(max(1, f.out))
	}
	fill(screen, withAlpha(f.c, uint8(255*math.Max(0, math.Min(1, a)))))
}

// music plays the music command: with a fade in, or a stop with a fade out.
func (g *Game) music(c data.MusicCue) {
	switch {
	case c.Stop:
		g.songOverride, g.moodOverride, g.baseSong = "", "", ""
		sound.StopFade(c.Fade)
	case c.Fade > 0:
		sound.PlaySongFade(c.Song, g.setMusic(c), c.Fade)
	default:
		g.Music(c)
	}
}

// soundTicks is how long a wait for a sound effect lasts, in ticks.
func soundTicks(name string) int {
	return int(math.Ceil(sound.EffectSeconds(name) * 60))
}
