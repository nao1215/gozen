package puppet

import (
	"math"
	"math/rand/v2"

	"github.com/nao1215/gozen/internal/layer2/puppetdef"
)

// Pose is everything the mesh and the overlays need for one frame.
type Pose struct {
	HeadAngle  float64    // radians, positive turns clockwise on screen
	HeadShift  [2]float64 // pixels
	HeadSquash float64    // 0 is none; a nod squashes the head toward the neck
	Hair       [2]float64 // lag of the hair behind the head, pixels
	Anchor     [2]float64 // how far the head center moved (without the body)
	Breath     float64    // vertical stretch, fraction of the height
	Body       [2]float64 // shift of the whole picture
	BodyScale  [2]float64 // scale of the whole picture about its bottom center
	Blink      float64    // 0 open to 1 closed
	Mouth      int        // frame index, -1 is the face's own mouth
	Face       int        // face index
	PrevFace   int        // face fading out
	Fade       float64    // 0 shows PrevFace, 1 shows Face
}

// params are the resolved strengths in pixels, radians, and seconds.
type params struct {
	breathDepth, breathPeriod float64
	swayAngle, swayShift      float64
	swayPeriod                float64
	tiltAngle                 float64
	tiltEvery                 [2]float64
	hairGain, hairK, hairC    float64
	talkNod                   float64
	fade                      float64
	unit                      float64 // the head radius, or a fifth of the height without a head
	hasHead                   bool
}

func resolveParams(s *puppetdef.Spec, w, h int) params {
	p := params{
		breathDepth: 0.006, breathPeriod: 3.8,
		swayAngle: 2.5 * math.Pi / 180, swayPeriod: 6.5,
		tiltAngle: 7 * math.Pi / 180, tiltEvery: [2]float64{5, 11},
		hairGain: 1, hairK: 60, hairC: 7,
		fade: 0.25,
		unit: float64(h) / 5,
	}
	if s.Head != nil {
		p.hasHead = true
		p.unit = s.Head.Radius
	}
	p.swayShift = 0.025 * p.unit
	p.talkNod = 0.02 * p.unit
	m := s.Motion
	if b := m.Breath; b != nil {
		p.breathDepth = b.Depth
		if b.Period > 0 {
			p.breathPeriod = b.Period
		}
	}
	if w := m.Sway; w != nil {
		p.swayAngle = w.Angle * math.Pi / 180
		p.swayShift = w.Shift
		if w.Period > 0 {
			p.swayPeriod = w.Period
		}
	}
	if t := m.Tilt; t != nil {
		p.tiltAngle = t.Angle * math.Pi / 180
		if t.Every != nil {
			p.tiltEvery = [2]float64{t.Every[0], t.Every[1]}
		}
	}
	if hs := m.Hair; hs != nil {
		p.hairGain = hs.Gain
		if hs.Stiffness > 0 {
			p.hairK = hs.Stiffness
		}
		if hs.Damping > 0 {
			p.hairC = hs.Damping
		}
	}
	if t := m.Talk; t != nil {
		p.talkNod = t.Nod
	}
	if s.Fade > 0 {
		p.fade = s.Fade
	}
	if !p.hasHead {
		p.swayAngle, p.swayShift, p.tiltAngle, p.talkNod = 0, 0, 0, 0
	}
	return p
}

type active struct {
	kind         string
	amount, time float64
	t            float64
}

// Motion is the animation state. It is deterministic for a given random source
// and step, and it never touches the GPU.
type Motion struct {
	rig *Rig
	p   params
	rng *rand.Rand
	dt  float64
	t   float64

	phase [6]float64

	nextBlink, blinkT  float64
	blinkAgain, second bool

	speaking  bool
	talk      float64 // 0 to 1, eases in and out
	mouth     int
	nextMouth float64

	tiltT, tiltLen, tiltDir float64
	nextTilt                float64

	reactions []active

	hair, hairV [2]float64

	face, prev int
	fade       float64
}

// NewMotion starts the motion at time zero. tps is updates per second.
func NewMotion(rig *Rig, rng *rand.Rand, tps int) *Motion {
	w, h := rig.Size()
	m := &Motion{rig: rig, p: resolveParams(rig.Spec, w, h), rng: rng, dt: 1 / float64(tps), mouth: -1, fade: 1}
	for i := range m.phase {
		m.phase[i] = rng.Float64() * 2 * math.Pi
	}
	m.nextBlink = m.blinkGap()
	m.nextTilt = m.between(m.p.tiltEvery)
	if m.p.hasHead {
		p := m.pose(false)
		a := m.anchor(p)
		m.hair = [2]float64{a[0] + p.Body[0], a[1] + p.Body[1]}
	}
	return m
}

func (m *Motion) between(r [2]float64) float64 { return r[0] + m.rng.Float64()*(r[1]-r[0]) }

func (m *Motion) blinkGap() float64 {
	if e := m.rig.Faces[m.face].Eyes; e != nil {
		return m.between(e.Every)
	}
	return m.between([2]float64{2, 6})
}

// Speak starts or stops the mouth.
func (m *Motion) Speak(on bool) {
	m.speaking = on
}

// SetFace fades to face i. Setting the current face does nothing.
func (m *Motion) SetFace(i int) {
	if i == m.face {
		return
	}
	m.prev, m.face, m.fade = m.face, i, 0
	m.mouth = -1
}

// SkipFade finishes a face change at once (for the first face).
func (m *Motion) SkipFade() { m.prev, m.fade = m.face, 1 }

// React plays a one-shot reaction; it adds to any that are playing.
func (m *Motion) React(r puppetdef.Reaction) {
	a := active{kind: r.Kind, amount: r.Amount, time: r.Time}
	if a.amount == 0 {
		a.amount = 1
	}
	if a.time == 0 {
		a.time = map[string]float64{"hop": 0.45, "shrink": 0.5, "shake": 0.6, "nod": 0.5, "tilt": 1.4, "freeze": 1.5}[r.Kind]
	}
	if !(a.time > 0) { // an unknown kind: nothing to play
		return
	}
	if len(m.reactions) >= 8 { // a burst of calls cannot pile up without bound
		m.reactions = m.reactions[1:]
	}
	m.reactions = append(m.reactions, a)
}

// Step advances one update.
func (m *Motion) Step() {
	dt := m.dt
	frozen := m.frozen()
	if !frozen {
		m.t += dt
	}
	if m.fade < 1 {
		m.fade = math.Min(1, m.fade+dt/m.p.fade)
	}
	if !frozen {
		m.stepBlink(dt)
	}
	m.stepMouth(dt)
	if m.speaking && !frozen {
		m.talk = math.Min(1, m.talk+dt*4)
	} else {
		m.talk = math.Max(0, m.talk-dt*2)
	}
	if !frozen {
		m.stepTilt(dt)
	}
	kept := m.reactions[:0]
	for _, a := range m.reactions {
		a.t += dt
		if a.t < a.time {
			kept = append(kept, a)
		}
	}
	m.reactions = kept
	if !frozen {
		m.stepHair(dt)
	}
}

// frozen reports whether a freeze reaction is playing: breathing, sway, blinks,
// tilts, hair and the mouth stop where they are (a stream's sudden silence).
func (m *Motion) frozen() bool {
	for _, a := range m.reactions {
		if a.kind == "freeze" {
			return true
		}
	}
	return false
}

const blinkLen = 0.16

func (m *Motion) stepBlink(dt float64) {
	if m.blinkT > 0 {
		m.blinkT += dt
		if m.blinkT >= blinkLen {
			m.blinkT = 0
			if m.blinkAgain {
				m.blinkAgain, m.second = false, true
				m.nextBlink = 0.08
			} else {
				m.nextBlink = m.blinkGap()
			}
		}
		return
	}
	m.nextBlink -= dt
	if m.nextBlink <= 0 {
		m.blinkT = dt
		if m.second {
			m.second = false
		} else if e := m.rig.Faces[m.face].Eyes; e != nil && m.rng.Float64() < e.Double {
			m.blinkAgain = true
		}
	}
}

func (m *Motion) blink() float64 {
	if m.blinkT <= 0 {
		return 0
	}
	u := m.blinkT / blinkLen
	switch {
	case u < 0.3:
		return u / 0.3
	case u < 0.55:
		return 1
	default:
		return math.Max(0, 1-(u-0.55)/0.45)
	}
}

func (m *Motion) stepMouth(dt float64) {
	mo := m.rig.Faces[m.face].Mouth
	if mo == nil || !m.speaking {
		m.mouth = -1
		return
	}
	m.nextMouth -= dt
	if m.nextMouth > 0 && m.mouth >= 0 {
		return
	}
	m.nextMouth = (0.7 + 0.6*m.rng.Float64()) / mo.Speed
	n := len(mo.Frames)
	next := m.rng.IntN(n)
	if next == m.mouth {
		next = (next + 1 + m.rng.IntN(n-1)) % n
	}
	m.mouth = next
}

// mouthFrame is the overlay to draw: -1 when silent or on the face's own mouth.
func (m *Motion) mouthFrame() int {
	mo := m.rig.Faces[m.face].Mouth
	if m.mouth < 0 || mo == nil || m.mouth >= len(mo.Frames) || mo.Frames[m.mouth] == nil {
		return -1
	}
	return m.mouth
}

func (m *Motion) stepTilt(dt float64) {
	if m.p.tiltAngle == 0 {
		return
	}
	if m.tiltLen > 0 {
		m.tiltT += dt
		if m.tiltT >= m.tiltLen {
			m.tiltLen, m.tiltT = 0, 0
			m.nextTilt = m.between(m.p.tiltEvery)
		}
		return
	}
	m.nextTilt -= dt
	if m.nextTilt <= 0 {
		m.tiltLen = 1.8 + 1.6*m.rng.Float64()
		m.tiltDir = 1
		if m.rng.IntN(2) == 0 {
			m.tiltDir = -1
		}
		m.tiltDir *= 0.6 + 0.4*m.rng.Float64()
	}
}

// tilt eases in, holds, and eases out over tiltLen.
func (m *Motion) tilt() float64 {
	if m.tiltLen == 0 {
		return 0
	}
	u := m.tiltT / m.tiltLen
	var k float64
	switch {
	case u < 0.25:
		k = smooth(u / 0.25)
	case u < 0.7:
		k = 1
	default:
		k = 1 - smooth((u-0.7)/0.3)
	}
	return k * m.tiltDir * m.p.tiltAngle
}

func (m *Motion) stepHair(dt float64) {
	if m.p.hairGain == 0 || !m.p.hasHead {
		return
	}
	p := m.pose(false)
	a := m.anchor(p)
	target := [2]float64{a[0] + p.Body[0], a[1] + p.Body[1]}
	for i := range 2 {
		acc := m.p.hairK*(target[i]-m.hair[i]) - m.p.hairC*m.hairV[i]
		m.hairV[i] += acc * dt
		m.hair[i] += m.hairV[i] * dt
	}
}

// anchor is where the head center moves to, which the hair spring follows.
func (m *Motion) anchor(p Pose) [2]float64 {
	h := m.rig.Spec.Head
	c := [2]float64{h.Center[0], h.Center[1]}
	n := [2]float64{h.Neck[0], h.Neck[1]}
	q := headPoint(c, n, p)
	return [2]float64{q[0] - c[0], q[1] - c[1]}
}

// Pose is the current frame.
func (m *Motion) Pose() Pose { return m.pose(true) }

func (m *Motion) pose(withHair bool) Pose {
	p := Pose{BodyScale: [2]float64{1, 1}, Mouth: m.mouthFrame(), Face: m.face, PrevFace: m.prev, Fade: m.fade, Blink: m.blink()}
	t := m.t
	tau := 2 * math.Pi
	if m.p.breathPeriod > 0 {
		p.Breath = m.p.breathDepth * 0.5 * (1 - math.Cos(tau*t/m.p.breathPeriod+m.phase[0]))
	}
	sp := m.p.swayPeriod
	p.HeadAngle = m.p.swayAngle*(0.65*math.Sin(tau*t/sp+m.phase[1])+0.35*math.Sin(tau*t/(sp*0.61)+m.phase[2])) + m.tilt()
	p.HeadShift[0] = m.p.swayShift * math.Sin(tau*t/(sp*1.3)+m.phase[3])
	p.HeadShift[1] = 0.5 * m.p.swayShift * math.Sin(tau*t/(sp*0.77)+m.phase[4])
	if m.talk > 0 {
		p.HeadShift[1] += m.talk * m.p.talkNod * (0.5 + 0.5*math.Sin(tau*t*1.9+m.phase[5]))
		p.HeadAngle += m.talk * m.p.swayAngle * 0.4 * math.Sin(tau*t*0.9)
	}
	p.Body[0] = 0.3 * p.HeadShift[0]
	for _, a := range m.reactions {
		m.apply(&p, a)
	}
	if withHair && m.p.hairGain > 0 && m.p.hasHead {
		p.Anchor = m.anchor(p)
		limit := 0.15 * m.p.unit
		for i := range 2 {
			lag := m.hair[i] - p.Anchor[i] - p.Body[i]
			p.Hair[i] = clamp(lag*m.p.hairGain, -limit, limit)
		}
	}
	return p
}

func (m *Motion) apply(p *Pose, a active) {
	u := a.t / a.time
	k := a.amount
	unit := m.p.unit
	switch a.kind {
	case "hop":
		p.Body[1] -= k * 0.22 * unit * math.Sin(math.Pi*u)
		s := k * 0.03 * math.Sin(2*math.Pi*u)
		p.BodyScale[0] *= 1 - s
		p.BodyScale[1] *= 1 + s
	case "shrink":
		s := k * 0.06 * math.Sin(math.Pi*math.Min(1, u*1.6)) * (1 - 0.3*u)
		p.BodyScale[0] *= 1 + s*0.6
		p.BodyScale[1] *= 1 - s
	case "shake":
		e := math.Sin(2*math.Pi*3*u) * (1 - u)
		p.Body[0] += k * 0.07 * unit * e
		if m.p.hasHead {
			p.HeadAngle += k * 0.06 * e
		}
	case "nod":
		e := math.Sin(math.Pi * u)
		if m.p.hasHead {
			p.HeadShift[1] += k * 0.08 * unit * e
			p.HeadSquash += k * 0.04 * e
		} else {
			p.Body[1] += k * 0.08 * unit * e
		}
	case "tilt":
		var e float64
		switch {
		case u < 0.25:
			e = smooth(u / 0.25)
		case u < 0.7:
			e = 1
		default:
			e = 1 - smooth((u-0.7)/0.3)
		}
		if m.p.hasHead {
			p.HeadAngle += k * 12 * math.Pi / 180 * e
		}
	}
}

func smooth(u float64) float64 {
	u = clamp(u, 0, 1)
	return u * u * (3 - 2*u)
}

func clamp(v, lo, hi float64) float64 { return math.Max(lo, math.Min(hi, v)) }
