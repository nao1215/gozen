package play

import (
	"log"
	"math"

	"github.com/nao1215/gozen/internal/event"
	data "github.com/nao1215/gozen/internal/layer2/gamedata"
	"github.com/nao1215/gozen/internal/layer5/sound"
)

// Voices (ADR 0028): a line's voice plays when the line opens, unless the line is passed
// (skip mode, Ctrl over a line read, scene skip, a replay); the next line stops it when
// ui.voice.cut (the default), and auto waits for it when ui.voice.auto_wait (the default).

// lineVoice plays the voice of a line that opens, or stops the one before.
func (g *Game) lineVoice(t *talk, s event.Say, passed bool) {
	if s.Voice == "" {
		if g.db.Game.UI.Voice.Cuts() {
			sound.StopVoice()
		}
		return
	}
	if passed || g.live.quiet || g.fast != nil || g.skipping {
		if g.db.Game.UI.Voice.Cuts() {
			sound.StopVoice()
		}
		return
	}
	g.playVoice(t, s)
}

// playVoice plays a line's voice now and notes how long auto waits for it.
func (g *Game) playVoice(t *talk, s event.Say) {
	l := traceLine{"file": s.Voice}
	if s.Actor != "" {
		l["who"] = s.Actor
	}
	pcm, err := sound.VoiceClip(g.db.FS, "assets/"+s.Voice)
	if err != nil {
		log.Printf("voice %s: %v", s.Voice, err)
		l["missing"] = true
		g.emit("voice", l)
		return
	}
	sound.PlayVoice(pcm, g.voiceGain(s.Actor))
	t.voiceT = int(math.Ceil(sound.ClipSeconds(pcm) * 60))
	l["ticks"] = t.voiceT
	g.emit("voice", l)
}

// voiceGain is the volume of a speaker's voice (the settings' row for that actor; 1 when unset).
func (g *Game) voiceGain(actor string) float64 {
	if v, ok := g.cfg.VoiceBy[actor]; ok {
		return float64(clampInt(v, 0, 10)) / 10
	}
	return 1
}

// voicedActors are the actors with voices, in the order of the data (for the settings).
func (g *Game) voicedActors() []string {
	var out []string
	for _, id := range g.db.Ordered("actors") {
		if a := g.db.Actors[id]; a != nil && a.Voices != "" {
			out = append(out, id)
		}
	}
	return out
}

// usesVoices reports whether the game has voices (the settings show their rows).
func (g *Game) usesVoices() bool {
	if len(g.voicedActors()) > 0 {
		return true
	}
	found := false
	g.db.EachCommand(func(c *data.Command) {
		if c.Say != nil && c.Say.Voice != "" {
			found = true
		}
	})
	return found
}

// landedVoice plays the voice of the line a replay landed on (a roll back, a load, a scene skip).
func (g *Game) landedVoice() {
	if t := &g.talk; t.active && t.choice == nil && t.say.Voice != "" && !g.live.quiet && g.fast == nil {
		g.playVoice(t, t.say)
	}
}
