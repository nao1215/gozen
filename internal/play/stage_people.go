package play

import (
	"image/color"
	"math"
	"slices"
	"sort"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/vector"

	"github.com/nao1215/gozen/internal/layer1/geom"
	data "github.com/nao1215/gozen/internal/layer2/gamedata"
	"github.com/nao1215/gozen/internal/layer5/gfx"
	"github.com/nao1215/gozen/internal/layer5/stage"
	"github.com/nao1215/gozen/internal/world"
)

// The people of a scene: the location's people placed by stage.people, standing or walking a
// loop, as dolls with the colors of their standing pictures. Now and then one says a line of
// their bubbles in a balloon (the screen only). Talking to one is the menu's talk entry.

const (
	bubbleNear  = 4.0 // meters: someone this near says a line
	bubbleShow  = 3.5 // seconds a balloon shows
	bubbleAside = 2.8 // seconds a far one's balloon shows
	bubbleRest  = 15  // seconds before the same person speaks again
	bubbleEvery = 5   // seconds between the far ones' lines
	personRoom  = 0.45
)

var (
	colorWhiteNRGBA  = color.NRGBA{255, 255, 255, 255}
	colorShadowNRGBA = color.NRGBA{20, 30, 50, 255}
	colorInkNRGBA    = color.NRGBA{55, 45, 55, 255}
)

type stagePerson struct {
	def    *data.StagePerson
	person *data.Person
	w      walker
	next   int // the next point of the path
	speed  float64
	spec   dollSpec
	fade   float64 // 0 shown .. 1 gone (when its when stops holding)
	shown  float64 // the balloon shows until (seconds of the scene's clock)
	rest   float64 // no balloon until
	line   string
	said   int
}

// newStagePeople places the location's people of the stage.
func (g *Game) newStagePeople(s *stageScene) []*stagePerson {
	l := s.location(g)
	var out []*stagePerson
	for i := range l.Stage.People {
		def := &l.Stage.People[i]
		var person *data.Person
		for j := range l.People {
			if l.People[j].ID == def.ID {
				person = &l.People[j]
			}
		}
		if person == nil {
			continue
		}
		p := &stagePerson{def: def, person: person, speed: def.Speed}
		if p.speed == 0 {
			p.speed = 1.1
		}
		p.w = walker{at: def.Feet().Pt(), facing: stage.Facing(def.Facing)}
		if def.Facing == "" {
			p.w.facing = geom.Pt{Y: 1}
		}
		if len(def.Path) > 1 {
			p.next = 1
		}
		p.spec = newDollSpec(person.Image, def.Doll, s.sc.Camera)
		if !g.holds(person.When) {
			p.fade = 1
		}
		g.dollSheet(p.spec)
		out = append(out, p)
	}
	return out
}

func (g *Game) holds(e data.Expr) bool {
	ok, err := e.Bool(g.st.Env())
	return err == nil && ok
}

// advancePeople walks the people along their paths and fades those whose when changed.
func (s *stageScene) advancePeople(g *Game) {
	for _, p := range s.people {
		want := 1.0
		if g.holds(p.person.When) {
			want = 0
		}
		p.fade += math.Max(-tickSecond*3, math.Min(tickSecond*3, want-p.fade))
		p.w.moving = false
		if len(p.def.Path) < 2 || p.person.ID == s.talking {
			continue
		}
		// a point reached turns to the next at once
		var left float64
		for range len(p.def.Path) {
			_, _, left = s.sc.Toward(p.w.at, p.def.Path[p.next].Pt(), 1)
			if left >= 0.05 {
				break
			}
			p.next = (p.next + 1) % len(p.def.Path)
		}
		if left < 0.05 {
			continue
		}
		dx, dy, _ := s.sc.Toward(p.w.at, p.def.Path[p.next].Pt(), p.speed*tickSecond)
		ppm := s.sc.PPM(p.w.at.Y)
		to := geom.Pt{X: p.w.at.X + dx*ppm, Y: p.w.at.Y - dy*ppm*math.Sin(s.sc.Camera*math.Pi/180)}
		if geom.Dist(to, s.player.at) < geom.Dist(p.w.at, s.player.at) && s.sc.Meters(to, s.player.at) < personRoom {
			continue // the leader is in the way: wait
		}
		p.w.step(to, s.sc)
	}
}

// crowded reports whether the leader stepping to p would walk into someone.
func (s *stageScene) crowded(p geom.Pt) bool {
	for _, q := range s.people {
		if q.fade < 0.5 && s.sc.Meters(p, q.w.at) < personRoom && s.sc.Meters(p, q.w.at) < s.sc.Meters(s.player.at, q.w.at) {
			return true
		}
	}
	return false
}

// stageWalkerOf is the doll of one of the location's people in the scene, or nil.
func (g *Game) stageWalkerOf(s *stageScene, id string) *walker {
	for _, p := range s.people {
		if p.person.ID == id && p.fade < 0.5 {
			return &p.w
		}
	}
	return nil
}

// stagePeopleFigures adds the location's people to draw to out.
func (g *Game) stagePeopleFigures(s *stageScene, out []stage.Figure) []stage.Figure {
	for _, p := range s.people {
		if p.fade >= 1 {
			continue
		}
		out = append(out, stage.Figure{At: p.w.at, Facing: p.w.facing, Moving: p.w.moving, Phase: p.w.phase, Sheet: g.dollSheet(p.spec), Fade: p.fade})
	}
	return out
}

// lines are the person's balloon lines that hold now.
func (g *Game) lines(p *data.Person) []string {
	var out []string
	for _, b := range p.Bubbles {
		if g.holds(b.When) {
			out = append(out, data.Render(b.Text, g.st.Env()))
		}
	}
	return out
}

// sayNow puts a balloon over someone for secs seconds (their next line, in turn).
func (s *stageScene) sayNow(g *Game, p *stagePerson, secs float64) {
	ls := g.lines(p.person)
	if len(ls) == 0 || p.fade > 0 {
		return
	}
	p.line = ls[p.said%len(ls)]
	p.said++
	p.shown, p.rest = s.clock+secs, s.clock+bubbleRest
}

// chatter is the balloons' timing: someone near says a line, and now and then a far one.
func (s *stageScene) chatter(g *Game) {
	var near *stagePerson
	bd := bubbleNear
	for _, p := range s.people {
		if d := s.sc.Meters(p.w.at, s.player.at); p.fade == 0 && d < bd {
			near, bd = p, d
		}
	}
	if near != nil && near.shown < s.clock && near.rest < s.clock {
		s.sayNow(g, near, bubbleShow)
	}
	if s.clock > s.aside && len(s.people) > 0 {
		s.aside = s.clock + bubbleEvery
		p := s.people[int(s.clock*7)%len(s.people)]
		if p.rest < s.clock {
			s.sayNow(g, p, bubbleAside)
		}
	}
}

// balloonSize is the text height of the balloons in UI units: game.yaml ui.balloon (a
// fraction of the screen's height), 0.037 when not given.
func (g *Game) balloonSize() float64 {
	_, sh := g.screen()
	k := g.db.Game.UI.Balloon
	if k <= 0 {
		k = defaultBalloon
	}
	return sh * k
}

const (
	defaultBalloon = 0.037 // the balloons' text, of the screen's height
	balloonFace    = 3.0   // the face's diameter in text heights
	balloonFarAt   = 8.0   // meters from the leader where the balloons are a step smaller
	balloonFar     = 0.85  // that step
	balloonMin     = 0.03  // the smallest text kept readable, of the screen's height
	balloonMinW    = 3.4   // the narrowest box, in box heights (short lines get the same box)
)

// balloon is one balloon laid out on the screen (UI units).
type balloon struct {
	p             *stagePerson
	x, y, w, h    float64 // the box
	headX, headY  float64 // the top of the doll's head
	ts, face, pad float64
	alpha         float64
}

// balloonScale is the one step of the balloons' size: the same for everyone at a distance.
func (s *stageScene) balloonScale(p *stagePerson) float64 {
	if s.sc.Meters(p.w.at, s.player.at) > balloonFarAt {
		return balloonFar
	}
	return 1
}

// drawStageBalloons draws the balloons over the people: a round framed face and the line,
// just over the doll's head with a short tail pointing at it. Every balloon is the same
// height (a far one a step smaller); they stay on the screen and do not overlap (a far one
// moves up out of the way of a nearer one, or down when there is no room), and the tail
// keeps pointing at its own head.
func (g *Game) drawStageBalloons(dst *ebiten.Image, s *stageScene) {
	pw, ph := g.pixels()
	sw, sh := g.screen()
	base := g.balloonSize()
	var bs []*balloon
	for _, p := range s.people {
		left := p.shown - s.clock
		if left <= 0 || p.line == "" || p.fade > 0 {
			continue
		}
		hx, hy := s.r.HeadOf(p.w.at, pw, ph)
		ts := math.Max(sh*balloonMin, base*s.balloonScale(p))
		b := &balloon{
			p: p, ts: ts, face: ts * balloonFace, pad: ts * 0.4, headX: hx / g.ui, headY: hy / g.ui,
			alpha: math.Min(1, math.Min(left, bubbleShow-left)/0.25),
		}
		b.h = b.face + b.pad*1.4
		b.w = math.Min(sw-16, math.Max(b.h*balloonMinW, b.pad*0.7+b.face+b.pad+gfx.TextWidth(p.line, ts)+b.pad*1.6))
		b.x = math.Max(8, math.Min(sw-8-b.w, b.headX-b.w/2))
		b.y = b.headY - b.gap() - b.tail() - b.h
		bs = append(bs, b)
	}
	// the nearer one (lower on the screen) keeps its place
	sort.Slice(bs, func(i, j int) bool { return bs[i].headY > bs[j].headY })
	for i, b := range bs {
		b.y = math.Max(8, math.Min(sh-8-b.h, b.y))
		for range 8 {
			moved := false
			for _, o := range bs[:i] {
				if b.x < o.x+o.w+6 && o.x < b.x+b.w+6 && b.y < o.y+o.h+6 && o.y < b.y+b.h+6 {
					if up := o.y - b.h - 6; up >= 8 {
						b.y = up
					} else {
						b.y = o.y + o.h + 6
					}
					moved = true
				}
			}
			if !moved {
				break
			}
		}
		b.y = math.Max(8, math.Min(sh-8-b.h, b.y))
	}
	for _, b := range slices.Backward(bs) { // the far ones first, the near one on top
		g.drawBalloon(dst, b)
	}
}

func (b *balloon) gap() float64  { return b.ts * 0.25 }
func (b *balloon) tail() float64 { return b.ts * 0.55 }

func (g *Game) drawBalloon(dst *ebiten.Image, b *balloon) {
	a := b.alpha
	white := withAlpha(colorWhiteNRGBA, uint8(248*a))
	gfx.FillRoundRect(dst, float32(b.x+3), float32(b.y+5), float32(b.w), float32(b.h), float32(b.h/2), withAlpha(colorShadowNRGBA, uint8(80*a)))
	// the tail: from the box's edge facing the head to just over the head
	u := float32(g.ui)
	tipY := b.headY - b.gap()
	baseY := b.y + b.h - 2
	if tipY < b.y+b.h/2 { // the box was pushed below the head
		baseY = b.y + 2
	}
	tx := math.Max(b.x+b.h/2, math.Min(b.x+b.w-b.h/2, b.headX))
	w := b.ts * 0.4
	var path vector.Path
	path.MoveTo(float32(tx-w)*u, float32(baseY)*u)
	path.LineTo(float32(tx+w)*u, float32(baseY)*u)
	path.LineTo(float32(b.headX)*u, float32(tipY)*u)
	path.Close()
	op := &vector.DrawPathOptions{AntiAlias: true}
	op.ColorScale.ScaleWithColor(white)
	vector.FillPath(dst, &path, nil, op)
	gfx.FillRoundRect(dst, float32(b.x), float32(b.y), float32(b.w), float32(b.h), float32(b.h/2), white)
	gfx.StrokeRoundRect(dst, float32(b.x+1), float32(b.y+1), float32(b.w-2), float32(b.h-2), float32(b.h/2-1), 2, withAlpha(g.th.border, uint8(200*a)))
	fx, fy := b.x+b.pad*0.7, b.y+b.pad*0.7
	if face := g.dollFace(b.p.spec); face != nil {
		io := &ebiten.DrawImageOptions{Filter: ebiten.FilterLinear}
		fb := face.Bounds()
		io.GeoM.Scale(b.face/float64(fb.Dx()), b.face/float64(fb.Dy()))
		io.GeoM.Translate(fx, fy)
		io.ColorScale.ScaleAlpha(float32(a))
		gfx.DrawImage(dst, face, io)
	}
	r := b.face / 2
	gfx.StrokeCircle(dst, float32(fx+r), float32(fy+r), float32(r), float32(math.Max(2, b.ts*0.12)), withAlpha(g.th.border, uint8(255*a)), true)
	g.text(dst, b.p.line, fx+b.face+b.pad, b.y+b.h/2-b.ts*0.62, b.ts, withAlpha(colorInkNRGBA, uint8(255*a)))
}

// drawReach names what OK takes now (someone or something within reach).
func (g *Game) drawReach(dst *ebiten.Image, s *stageScene) {
	e, ok := s.reach, s.reachOK
	if !ok || s.goal != nil {
		return
	}
	sw, sh := g.screen()
	label := e.Label
	if e.Kind == world.Do && e.Thing != nil && e.Thing.Ask != "" {
		label = data.Render(e.Thing.Ask, g.st.Env()) + "：" + label
	}
	w := gfx.TextWidth(label, sizeM) + 48
	gfx.FillRoundRect(dst, float32(sw/2-w/2), float32(sh-92), float32(w), 46, 14, g.th.window)
	gfx.DrawText(dst, label, sw/2, sh-84, sizeM, g.th.text)
}
