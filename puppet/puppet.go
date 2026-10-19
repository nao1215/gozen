// Package puppet moves a single portrait picture like a streaming avatar: it
// blinks, moves its mouth while speaking, breathes, sways and tilts its head,
// lets its hair trail behind, plays one-shot reactions, and fades between faces.
//
// A puppet is one base picture plus small difference pictures (closed eyes, mouth
// shapes, other faces) and a few numbers in YAML. The head and hair are moved by
// deforming a mesh over the picture, so no layered artwork or rigging tool is
// needed. The YAML format is described in docs/architecture/puppet.md.
//
// A host loads the rig once, makes a Puppet per on-screen figure, calls Update
// from its Update and Draw from its Draw. Puppets are not safe for concurrent use
// and must be used from the Ebitengine game thread.
package puppet

import (
	"fmt"
	"io/fs"
	"math/rand/v2"

	"github.com/hajimehoshi/ebiten/v2"

	"github.com/nao1215/gozen/internal/layer5/puppet"
)

// Rig is a loaded puppet definition with its pictures decoded on the CPU. It is
// read-only after Load and may be shared by several puppets.
type Rig struct{ r *puppet.Rig }

// Load reads the puppet YAML at path in fsys and the PNG pictures it names
// (relative to the YAML's directory), and checks them: unknown keys, missing or
// misplaced pictures, overlays outside the base, and bad strengths are errors.
// A difference picture as large as the base is cut down to the part that differs.
// The FS is borrowed only during the call.
func Load(fsys fs.FS, path string) (*Rig, error) {
	r, err := puppet.Load(fsys, path)
	if err != nil {
		return nil, err
	}
	return &Rig{r: r}, nil
}

// Size is the base picture's size in pixels; Draw's transform maps this area.
func (r *Rig) Size() (width, height int) { return r.r.Size() }

// Config sets up a puppet.
type Config struct {
	// Rand drives blink timing, idle motion and mouth shapes. Nil uses a fixed
	// seed, so the motion is reproducible.
	Rand *rand.Rand
	// TPS is how many times per second Update is called (0 means ebiten.DefaultTPS).
	TPS int
	// Face is the first face (empty means the rig's first face).
	Face string
}

// Puppet is one moving figure.
type Puppet struct {
	p   *puppet.Puppet
	rig *Rig
}

// New makes a puppet from rig. GPU images are made on the first Draw.
func New(rig *Rig, cfg Config) (*Puppet, error) {
	if rig == nil || rig.r == nil {
		return nil, fmt.Errorf("puppet: nil rig")
	}
	if cfg.TPS < 0 {
		return nil, fmt.Errorf("puppet: TPS %d is negative", cfg.TPS)
	}
	if cfg.TPS == 0 {
		cfg.TPS = ebiten.DefaultTPS
	}
	rng := cfg.Rand
	if rng == nil {
		rng = rand.New(rand.NewPCG(1, 2)) //nolint:gosec // Reproducible animation timing, not security-sensitive data.
	}
	p := &Puppet{p: puppet.New(rig.r, rng, cfg.TPS), rig: rig}
	if cfg.Face != "" {
		i, ok := rig.r.Face(cfg.Face)
		if !ok {
			return nil, fmt.Errorf("puppet: no face %q", cfg.Face)
		}
		p.p.Motion.SetFace(i)
		p.p.Motion.SkipFade()
	}
	return p, nil
}

// Update advances the motion by one tick (1/TPS seconds).
func (p *Puppet) Update() { p.p.Motion.Step() }

// Draw draws the current frame. op gives the transform from base pixels to dst,
// the color scale and the blend; nil draws at the origin. The picture is always
// filtered linearly. Head and hair may move outside the base area by a few
// percent of the head radius, and reactions such as hop move the whole picture.
func (p *Puppet) Draw(dst *ebiten.Image, op *ebiten.DrawImageOptions) { p.p.Draw(dst, op) }

// Speak starts moving the mouth, for example while a line is being shown.
func (p *Puppet) Speak() { p.p.Motion.Speak(true) }

// Stop stops the mouth and returns it to the face's own mouth.
func (p *Puppet) Stop() { p.p.Motion.Speak(false) }

// React plays a one-shot reaction: a name from the rig's reactions or one of the
// built-in kinds hop, shrink, shake, nod and tilt. Reactions add up while playing.
func (p *Puppet) React(name string) error {
	r, ok := p.rig.r.Spec.ReactionOf(name)
	if !ok {
		return fmt.Errorf("puppet: no reaction %q", name)
	}
	p.p.Motion.React(r)
	return nil
}

// SetFace fades to the named face. The current face is a no-op.
func (p *Puppet) SetFace(name string) error {
	i, ok := p.rig.r.Face(name)
	if !ok {
		return fmt.Errorf("puppet: no face %q", name)
	}
	p.p.Motion.SetFace(i)
	return nil
}

// Close releases the GPU images. Later Draw calls draw nothing. Close the puppet
// when the figure leaves the screen for good; the Rig stays usable.
func (p *Puppet) Close() { p.p.Close() }
