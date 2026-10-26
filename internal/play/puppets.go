package play

import (
	"hash/fnv"
	"math/rand/v2"
	"path"
	"strings"

	"github.com/hajimehoshi/ebiten/v2"

	"github.com/nao1215/gozen/internal/battle"
	"github.com/nao1215/gozen/internal/event"
	"github.com/nao1215/gozen/internal/layer5/gfx"
	"github.com/nao1215/gozen/internal/layer5/puppet"
)

// puppetSet holds the moving portraits of actors with a puppet (Actor.Puppet),
// made on the first talk and kept until the assets are reloaded or the game closes.
type puppetSet struct {
	live   map[string]*puppet.Puppet
	failed map[string]bool
	// staged is the tick each actor's puppet last stood on the novel stage (ui.stage_puppets).
	staged map[string]int
}

// talkPuppet returns the moving portrait for the speaker and the index of the face
// shown, or nil when the speaker has none, it failed to load, or it lacks the face.
func (g *Game) talkPuppet(s event.Say) (*puppet.Puppet, int) {
	return g.actorPuppet(s.Actor, s.Face)
}

// actorPuppet returns the moving portrait of an actor and the index of the face picture
// face in it, made on first use, or nil when the actor has none, it failed to load, or it
// lacks the face.
func (g *Game) actorPuppet(actor, face string) (*puppet.Puppet, int) {
	if actor == "" || face == "" {
		return nil, 0
	}
	a := g.db.Actors[actor]
	if a == nil || a.Puppet == "" || g.puppets.failed[actor] {
		return nil, 0
	}
	p := g.puppets.live[actor]
	if p == nil {
		rig, err := puppet.Load(g.opts.FS, "assets/"+a.Puppet)
		if err != nil {
			// gozen check reports a broken puppet; the game falls back to the face picture.
			if g.puppets.failed == nil {
				g.puppets.failed = map[string]bool{}
			}
			g.puppets.failed[actor] = true
			return nil, 0
		}
		h := fnv.New64a()
		h.Write([]byte(actor))
		p = puppet.New(rig, rand.New(rand.NewPCG(h.Sum64(), 1)), ebiten.DefaultTPS) //nolint:gosec // Animation timing, not security-sensitive data.
		if g.puppets.live == nil {
			g.puppets.live = map[string]*puppet.Puppet{}
		}
		g.puppets.live[actor] = p
	}
	i, ok := p.Rig().Face(strings.TrimSuffix(path.Base(face), path.Ext(face)))
	if !ok {
		return nil, 0
	}
	return p, i
}

func (g *Game) enemyPuppet(u *battle.Unit, picture string, hp int) *puppet.Puppet {
	if u == nil || u.Def == nil || u.Def.Puppet == "" {
		return nil
	}
	key := "enemy." + u.EID
	if g.puppets.failed[key] {
		return nil
	}
	p := g.puppets.live[key]
	if p == nil {
		rig, err := puppet.Load(g.opts.FS, "assets/"+u.Def.Puppet)
		if err != nil {
			if g.puppets.failed == nil {
				g.puppets.failed = map[string]bool{}
			}
			g.puppets.failed[key] = true
			return nil
		}
		h := fnv.New64a()
		h.Write([]byte(key))
		p = puppet.New(rig, rand.New(rand.NewPCG(h.Sum64(), 1)), ebiten.DefaultTPS) //nolint:gosec // Animation timing.
		if g.puppets.live == nil {
			g.puppets.live = map[string]*puppet.Puppet{}
		}
		g.puppets.live[key] = p
	}
	if i, ok := p.Rig().Face(strings.TrimSuffix(path.Base(picture), path.Ext(picture))); ok {
		p.Motion.SetFace(i)
	}
	p.Motion.SetHealth(float64(hp) / float64(max(1, u.MHP())))
	p.Motion.Step()
	return p
}

// openPuppet shows the line's face: a new speaker starts on it, the same one fades to it.
// The novel stage's puppets change their faces as their figures do (stepStagePuppets).
func (g *Game) openPuppet(s event.Say, still bool) {
	if g.stagePuppets() {
		return
	}
	p, face := g.talkPuppet(s)
	if p == nil {
		return
	}
	p.Motion.SetFace(face)
	if !still {
		p.Motion.SkipFade()
	}
}

// stepPuppet moves the speaker's portrait and its mouth while the text is typing.
func (g *Game) stepPuppet(t *talk) {
	if g.stagePuppets() {
		g.stepStagePuppets(t)
		return
	}
	p, _ := g.talkPuppet(t.say)
	if p == nil {
		return
	}
	p.Motion.Speak(t.shown < t.total && t.choice == nil)
	p.Motion.Step()
}

// drawPuppet draws like drawStanding: centered on cx, standing on bottom, h tall,
// tinted by cs when given.
func (g *Game) drawPuppet(dst *ebiten.Image, p *puppet.Puppet, cx, bottom, h float64, alpha float32, flip bool, cs *ebiten.ColorScale) {
	w, ih := p.Rig().Size()
	iw := float64(w)
	s := h / float64(ih)
	op := &ebiten.DrawImageOptions{}
	if flip {
		op.GeoM.Scale(-1, 1)
		op.GeoM.Translate(iw, 0)
	}
	op.GeoM.Scale(s, s)
	op.GeoM.Translate(cx-iw*s/2, bottom-h)
	if k := gfx.Scale(); k != 1 {
		op.GeoM.Scale(k, k)
	}
	if cs != nil {
		op.ColorScale = *cs
	}
	op.ColorScale.ScaleAlpha(alpha)
	p.Draw(dst, op)
}

func (g *Game) closePuppets() {
	for _, p := range g.puppets.live {
		p.Close()
	}
	g.puppets = puppetSet{}
}

// stagePuppets reports whether the novel stage draws moving portraits (ui.stage_puppets).
func (g *Game) stagePuppets() bool { return g.novel() && g.db.Game.UI.StagePuppets }

// stageAbsent is how many ticks a figure may be away from the stage and still fade to
// its next face; one away longer comes back on its face at once.
const stageAbsent = 30

// stagePuppet returns the moving portrait that draws the stage figure key (speakerKey's
// form) with the face picture face, turned to that face, or nil when the stage shows the
// picture: ui.stage_puppets is off, the figure is no actor with a puppet, or the puppet
// lacks the face.
func (g *Game) stagePuppet(key, face string) *puppet.Puppet {
	actor, ok := strings.CutPrefix(key, "actor:")
	if !ok || !g.stagePuppets() {
		return nil
	}
	p, i := g.actorPuppet(actor, face)
	if p != nil {
		// A stage portrait is often drawn far below its size (a tall picture fit to the screen).
		p.Reduce = true
		if g.puppets.staged == nil {
			g.puppets.staged = map[string]int{}
		}
		if last, seen := g.puppets.staged[actor]; !seen || g.tick-last > stageAbsent {
			p.Motion.SetFace(i)
			p.Motion.SkipFade()
		} else {
			p.Motion.SetFace(i) // fades over the puppet's fade
		}
		g.puppets.staged[actor] = g.tick
	}
	return p
}

// stepStagePuppets moves every moving portrait standing on the novel stage once a tick:
// breathing, sway and blinking for all, the mouth only for the speaker's while the text
// is typing.
func (g *Game) stepStagePuppets(t *talk) {
	speaker := ""
	if t.say.Actor != "" {
		speaker = speakerKey(t.say)
	}
	talking := t.shown < t.total && t.choice == nil
	for _, f := range g.stagedFaces() {
		p := g.stagePuppet(f[0], f[1])
		if p == nil {
			continue
		}
		p.Motion.Speak(talking && f[0] == speaker)
		p.Motion.Step()
	}
}

// stagedFaces are the figures standing on the novel stage now, as [key, face] in the
// order they are drawn, each once: those an event stood with stand, or the speakers
// placed by themselves, with those leaving.
func (g *Game) stagedFaces() [][2]string {
	var out [][2]string
	add := func(key, face string) {
		if key == "" || face == "" {
			return
		}
		for _, o := range out {
			if o[0] == key {
				return
			}
		}
		out = append(out, [2]string{key, face})
	}
	if g.castOn {
		for _, f := range g.cast {
			if f.leave == "" || g.tick-f.left < castOut {
				add(f.key, f.face)
			}
		}
		return out
	}
	add(g.stage[0].id, g.stage[0].face)
	add(g.stage[1].id, g.stage[1].face)
	if o := g.stageOut; o.id != "" && g.tick-g.stageOutT < 10 {
		add(o.id, o.face)
	}
	if g.tick-g.stageLeftT < 12 {
		for _, o := range g.stageLeft {
			add(o.id, o.face)
		}
	}
	return out
}
