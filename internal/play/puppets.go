package play

import (
	"hash/fnv"
	"math/rand/v2"
	"path"
	"strings"

	"github.com/hajimehoshi/ebiten/v2"

	"github.com/nao1215/gozen/event"
	"github.com/nao1215/gozen/internal/layer5/gfx"
	"github.com/nao1215/gozen/internal/layer5/puppet"
)

// puppetSet holds the moving portraits of actors with a puppet (Actor.Puppet),
// made on the first talk and kept until the assets are reloaded or the game closes.
type puppetSet struct {
	live   map[string]*puppet.Puppet
	failed map[string]bool
}

// talkPuppet returns the moving portrait for the speaker and the index of the face
// shown, or nil when the speaker has none, it failed to load, or it lacks the face.
func (g *Game) talkPuppet(s event.Say) (*puppet.Puppet, int) {
	if s.Actor == "" || s.Face == "" {
		return nil, 0
	}
	a := g.db.Actors[s.Actor]
	if a == nil || a.Puppet == "" || g.puppets.failed[s.Actor] {
		return nil, 0
	}
	p := g.puppets.live[s.Actor]
	if p == nil {
		rig, err := puppet.Load(g.opts.FS, "assets/"+a.Puppet)
		if err != nil {
			// gozen check reports a broken puppet; the game falls back to the face picture.
			if g.puppets.failed == nil {
				g.puppets.failed = map[string]bool{}
			}
			g.puppets.failed[s.Actor] = true
			return nil, 0
		}
		h := fnv.New64a()
		h.Write([]byte(s.Actor))
		p = puppet.New(rig, rand.New(rand.NewPCG(h.Sum64(), 1)), ebiten.DefaultTPS) //nolint:gosec // Animation timing, not security-sensitive data.
		if g.puppets.live == nil {
			g.puppets.live = map[string]*puppet.Puppet{}
		}
		g.puppets.live[s.Actor] = p
	}
	i, ok := p.Rig().Face(strings.TrimSuffix(path.Base(s.Face), path.Ext(s.Face)))
	if !ok {
		return nil, 0
	}
	return p, i
}

// openPuppet shows the line's face: a new speaker starts on it, the same one fades to it.
func (g *Game) openPuppet(s event.Say, still bool) {
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
	p, _ := g.talkPuppet(t.say)
	if p == nil {
		return
	}
	p.Motion.Speak(t.shown < t.total && t.choice == nil)
	p.Motion.Step()
}

// drawPuppet draws like drawStanding: centered on cx, standing on bottom, h tall.
func (g *Game) drawPuppet(dst *ebiten.Image, p *puppet.Puppet, cx, bottom, h float64, alpha float32, flip bool) {
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
	op.ColorScale.ScaleAlpha(alpha)
	p.Draw(dst, op)
}

func (g *Game) closePuppets() {
	for _, p := range g.puppets.live {
		p.Close()
	}
	g.puppets = puppetSet{}
}
