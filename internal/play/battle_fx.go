package play

import (
	"image/color"
	"math"
	"slices"

	"github.com/hajimehoshi/ebiten/v2"

	data "github.com/nao1215/gozen/internal/layer2/gamedata"
	"github.com/nao1215/gozen/internal/layer5/gfx"
)

type effectAnim struct {
	kind       string
	x, y, w, h float64
	t, dur     int
	color      color.NRGBA
	power      int
	seed       uint32
}

type screenFx struct {
	kind   string
	x, y   float64
	t, dur int
	color  color.NRGBA
	alpha  float64
}

var fxDefault = map[string]color.NRGBA{
	"slash":   {0x7c, 0xc4, 0xff, 0xff},
	"pierce":  {0x9c, 0xd8, 0xff, 0xff},
	"strike":  {0xff, 0xd8, 0xa0, 0xff},
	"fire":    {0xff, 0x8a, 0x3a, 0xff},
	"ice":     {0xa8, 0xe4, 0xff, 0xff},
	"thunder": {0xff, 0xe8, 0x66, 0xff},
	"holy":    {0xff, 0xf0, 0xb0, 0xff},
	"dark":    {0xa0, 0x60, 0xff, 0xff},
	"magic":   {0xc8, 0xb4, 0xff, 0xff},
	"heal":    {0x8c, 0xff, 0x9e, 0xff},
	"buff":    {0xff, 0xd8, 0x60, 0xff},
	"debuff":  {0xb0, 0x70, 0xff, 0xff},
}

var elemental = []string{"fire", "ice", "thunder", "holy", "dark"}

func fxKind(use *data.Usage) string {
	if use == nil {
		return "slash"
	}
	if use.Fx != "" {
		return use.Fx
	}
	if use.Anim == "none" {
		return "none"
	}
	for _, e := range use.Effects {
		if slices.Contains(elemental, e.Element) {
			return e.Element
		}
	}
	switch use.Anim {
	case "magic":
		return "magic"
	case "heal":
		return "heal"
	case "debuff":
		return "debuff"
	case "buff":
		if use.Scope == "one_enemy" || use.Scope == "all_enemies" || use.Scope == "random_enemy" {
			return "debuff"
		}
		return "buff"
	}
	return "slash"
}

func physicalFx(kind string) bool { return kind == "slash" || kind == "pierce" || kind == "strike" }

func fxColor(kind string, c data.Color) color.NRGBA {
	if c.Valid {
		return c.NRGBA
	}
	if d, ok := fxDefault[kind]; ok {
		return d
	}
	return colorWhite
}

func (s *battleScene) addFx(g *Game, kind string, x, y, w, h float64, c color.NRGBA, power int) int {
	if kind == "" || kind == "none" {
		return 0
	}
	base := map[string]int{"slash": 30, "pierce": 30, "strike": 32, "fire": 42, "ice": 42, "thunder": 38, "holy": 44, "dark": 42, "magic": 40, "heal": 44, "buff": 38, "debuff": 38, "cast": 30}[kind]
	if base == 0 {
		base = 36
	}
	if power == 0 {
		base = base * 3 / 4
	}
	if power >= 2 {
		base += 8
	}
	if !s.auto && !g.in.fast && !g.in.held[btnOK] {
		base = base * 3 / 2
	}
	dur := g.fxTicks(base)
	if g.in.fast || g.in.held[btnOK] {
		dur = max(8, dur/2)
	}
	s.fxSeq++
	dur = max(8, dur)
	s.fx = append(s.fx, effectAnim{kind: kind, x: x, y: y, w: w, h: h, dur: dur, color: c, power: power, seed: s.fxSeq * 2654435761})
	return dur
}

func fxHit(kind string) float64 {
	return map[string]float64{
		"slash": 0.2, "pierce": 0.24, "strike": 0.16, "fire": 0.2, "ice": 0.24,
		"thunder": 0.18, "holy": 0.24, "dark": 0.36, "magic": 0.3, "heal": 0.22,
	}[kind]
}

func (s *battleScene) addScreen(g *Game, kind string, x, y float64, c color.NRGBA, alpha float64, dur int) {
	dur = g.fxTicks(dur)
	if g.in.fast || g.in.held[btnOK] {
		dur = max(6, dur/2)
	}
	s.scr = append(s.scr, screenFx{kind: kind, x: x, y: y, dur: max(6, dur), color: c, alpha: alpha})
}

func (g *Game) fxTicks(n int) int {
	t := math.Max(1, g.db.Game.Battle.Tempo)
	return max(1, int(float64(n)/(math.Sqrt(t)*g.cfg.battleScale())))
}

func (s *battleScene) stepFx() {
	s.fx = slices.DeleteFunc(s.fx, func(e effectAnim) bool { return e.t >= e.dur })
	for i := range s.fx {
		s.fx[i].t++
	}
	s.scr = slices.DeleteFunc(s.scr, func(e screenFx) bool { return e.t >= e.dur })
	for i := range s.scr {
		s.scr[i].t++
	}
}

func rnd(seed uint32, i int) float64 {
	//nolint:gosec // Visual hash intentionally wraps modulo 2^32.
	x := seed ^ uint32(i)*0x9e3779b9
	x ^= x >> 16
	x *= 0x7feb352d
	x ^= x >> 15
	x *= 0x846ca68b
	x ^= x >> 16
	return float64(x) / 4294967296
}

func clamp01(v float64) float64 { return math.Max(0, math.Min(1, v)) }

func easeOut(p float64) float64 { p = clamp01(p); return 1 - (1-p)*(1-p)*(1-p) }

func mixColor(a, b color.NRGBA, k float64) color.NRGBA {
	k = clamp01(k)
	m := func(x, y uint8) uint8 { return uint8(float64(x) + (float64(y)-float64(x))*k) }
	return color.NRGBA{m(a.R, b.R), m(a.G, b.G), m(a.B, b.B), 0xff}
}

func lighten(c color.NRGBA, k float64) color.NRGBA { return mixColor(c, colorWhite, k) }

type fxBatch struct {
	vs []ebiten.Vertex
	is []uint32
}

func (b *fxBatch) reset() { b.vs, b.is = b.vs[:0], b.is[:0] }

func (b *fxBatch) v(x, y float64, c color.NRGBA, a float64) uint32 {
	a = clamp01(a)
	b.vs = append(b.vs, ebiten.Vertex{
		DstX: float32(x), DstY: float32(y), SrcX: 1, SrcY: 1,
		ColorR: float32(c.R) / 255, ColorG: float32(c.G) / 255, ColorB: float32(c.B) / 255, ColorA: float32(a),
	})
	return uint32(len(b.vs) - 1) //nolint:gosec // One frame cannot allocate 2^32 vertices.
}

func (b *fxBatch) tri(i, j, k uint32) { b.is = append(b.is, i, j, k) }

func (b *fxBatch) flush(dst *ebiten.Image) {
	if len(b.is) == 0 {
		b.reset()
		return
	}
	gfx.ScaleVertices(b.vs)

	dst.DrawTriangles32(b.vs, b.is, whiteImage, &ebiten.DrawTrianglesOptions{})
	b.reset()
}

func (b *fxBatch) full() bool { return len(b.vs) > 60000 }

func (b *fxBatch) glow(cx, cy, r, ry float64, c color.NRGBA, a float64) {
	b.fan(cx, cy, r, ry, c, a, 0)
}

func (b *fxBatch) fan(cx, cy, r, ry float64, c color.NRGBA, a, edge float64) {
	if r <= 0 || a <= 0 {
		return
	}
	if ry <= 0 {
		ry = r
	}
	const n = 20
	m := b.v(cx, cy, c, a)
	first := uint32(len(b.vs)) //nolint:gosec // One frame cannot allocate 2^32 vertices.
	for i := range n {
		ang := float64(i) / n * 2 * math.Pi
		b.v(cx+math.Cos(ang)*r, cy+math.Sin(ang)*ry, c, a*edge)
	}
	for i := range n {
		b.tri(m, first+uint32(i), first+uint32((i+1)%n))
	}
}

func (b *fxBatch) ring(cx, cy, r, ry, w float64, c color.NRGBA, a float64) {
	if r <= 0 || a <= 0 {
		return
	}
	k := 1.0
	if ry > 0 {
		k = ry / r
	}
	const n = 28
	base := uint32(len(b.vs)) //nolint:gosec // One frame cannot allocate 2^32 vertices.
	for i := range n {
		ang := float64(i) / n * 2 * math.Pi
		cs, sn := math.Cos(ang), math.Sin(ang)
		ri, ro := math.Max(0, r-w/2), r+w/2
		b.v(cx+cs*ri, cy+sn*ri*k, c, 0)
		b.v(cx+cs*r, cy+sn*r*k, c, a)
		b.v(cx+cs*ro, cy+sn*ro*k, c, 0)
	}
	for i := range n {
		j := (i + 1) % n
		p, q := base+uint32(i*3), base+uint32(j*3)
		b.tri(p, p+1, q)
		b.tri(q, p+1, q+1)
		b.tri(p+1, p+2, q+1)
		b.tri(q+1, p+2, q+2)
	}
}

func (b *fxBatch) line(x0, y0, x1, y1, w float64, c color.NRGBA, a float64) {
	dx, dy := x1-x0, y1-y0
	l := math.Hypot(dx, dy)
	if l < 0.01 || a <= 0 {
		return
	}
	nx, ny := -dy/l*w/2, dx/l*w/2
	c0, c1 := b.v(x0, y0, c, a), b.v(x1, y1, c, a)
	l0, l1 := b.v(x0+nx, y0+ny, c, 0), b.v(x1+nx, y1+ny, c, 0)
	r0, r1 := b.v(x0-nx, y0-ny, c, 0), b.v(x1-nx, y1-ny, c, 0)
	b.tri(c0, c1, l0)
	b.tri(l0, c1, l1)
	b.tri(c0, c1, r0)
	b.tri(r0, c1, r1)
}

func (b *fxBatch) blade(x0, y0, x1, y1, w float64, c color.NRGBA, a float64) {
	dx, dy := x1-x0, y1-y0
	l := math.Hypot(dx, dy)
	if l < 0.01 || a <= 0 {
		return
	}
	nx, ny := -dy/l*w/2, dx/l*w/2
	mx, my := (x0+x1)/2, (y0+y1)/2
	t0, t1, m := b.v(x0, y0, c, a), b.v(x1, y1, c, a), b.v(mx, my, c, a)
	sl, sr := b.v(mx+nx, my+ny, c, 0), b.v(mx-nx, my-ny, c, 0)
	b.tri(t0, m, sl)
	b.tri(t0, m, sr)
	b.tri(t1, m, sl)
	b.tri(t1, m, sr)
}

func (b *fxBatch) poly(pts [][2]float64, c color.NRGBA, a float64) {
	b.polyFade(pts, c, a, a)
}

func (b *fxBatch) polyFade(pts [][2]float64, c color.NRGBA, a, edge float64) {
	if len(pts) < 3 || a <= 0 {
		return
	}
	var mx, my float64
	for _, p := range pts {
		mx += p[0]
		my += p[1]
	}
	n := float64(len(pts))
	m := b.v(mx/n, my/n, c, a)
	first := uint32(len(b.vs)) //nolint:gosec // One frame cannot allocate 2^32 vertices.
	for _, p := range pts {
		b.v(p[0], p[1], c, edge)
	}
	for i := range pts {
		b.tri(m, first+uint32(i), first+uint32((i+1)%len(pts))) //nolint:gosec // Shape vertex indexes are bounded by the frame vertex allocation.
	}
}

func (b *fxBatch) edge(x0, y0, x1, y1, w float64, c color.NRGBA, a float64) {
	dx, dy := x1-x0, y1-y0
	l := math.Hypot(dx, dy)
	if l < 0.01 || a <= 0 {
		return
	}
	nx, ny := -dy/l*w/2, dx/l*w/2
	i := b.v(x0+nx, y0+ny, c, a)
	j := b.v(x1+nx, y1+ny, c, a)
	k := b.v(x1-nx, y1-ny, c, a)
	l0 := b.v(x0-nx, y0-ny, c, a)
	b.tri(i, j, k)
	b.tri(i, k, l0)
}

func (b *fxBatch) crystal(bx, by, ang, l, w float64, face, deep color.NRGBA, a, light float64) {
	if l < 1 || a <= 0 {
		return
	}
	dx, dy := math.Cos(ang), math.Sin(ang)
	nx, ny := -dy, dx
	pt := func(t, s float64) [2]float64 { return [2]float64{bx + dx*l*t + nx*w/2*s, by + dy*l*t + ny*w/2*s} }
	bl, br, sl, sr, tip := pt(0, -0.85), pt(0, 0.85), pt(0.74, -1), pt(0.74, 1), pt(1, 0)
	mb, ms := pt(0, 0.1), pt(0.74, 0.2)
	b.poly([][2]float64{bl, sl, tip, ms, mb}, mixColor(face, deep, 0.45), a*0.82)
	b.poly([][2]float64{mb, ms, tip, sr, br}, lighten(face, 0.3), a*0.72)
	g := pt(0.5, 0.15)
	b.glow(g[0], g[1], w*0.9, l*0.32, lighten(face, 0.8), a*0.55*light)
	for _, e := range [][2][2]float64{{bl, sl}, {sl, tip}, {tip, sr}, {sr, br}} {
		b.edge(e[0][0], e[0][1], e[1][0], e[1][1], math.Max(2, w*0.09), deep, a*0.8)
	}
	for _, e := range [][2][2]float64{{sl, tip}, {tip, sr}, {ms, tip}} {
		b.edge(e[0][0], e[0][1], e[1][0], e[1][1], math.Max(1.2, w*0.04), colorWhite, a*0.95)
	}
	b.line(mb[0], mb[1], ms[0], ms[1], math.Max(2, w*0.12), colorWhite, a*0.45)
}

func (b *fxBatch) crescent(cx, cy, r, a0, span, w float64, u0, u1 float64, c color.NRGBA, a, soft float64) {
	if a <= 0 || u1 <= u0 || w <= 0 {
		return
	}
	const n = 18
	var prev [2]uint32
	for i := 0; i <= n; i++ {
		u := u0 + (u1-u0)*float64(i)/n
		ang := a0 + span*u
		th := w * math.Sin(math.Pi*u)
		cs, sn := math.Cos(ang), math.Sin(ang)
		o := b.v(cx+cs*r, cy+sn*r, c, a)
		in := b.v(cx+cs*(r-th), cy+sn*(r-th), c, a*soft)
		if i > 0 {
			b.tri(prev[0], prev[1], o)
			b.tri(prev[1], in, o)
		}
		prev = [2]uint32{o, in}
	}
}

func (b *fxBatch) splinter(x, y, sz, rot float64, c, rim color.NRGBA, a float64) {
	if a <= 0 || sz <= 0 {
		return
	}
	var pts [4][2]float64
	rs := [4]float64{1, 0.45, 0.8, 0.4}
	for i := range 4 {
		ang := rot + float64(i)*math.Pi/2
		pts[i] = [2]float64{x + math.Cos(ang)*sz*rs[i], y + math.Sin(ang)*sz*rs[i]*0.6}
	}
	b.poly(pts[:], c, a*0.85)
	for i := range 4 {
		j := (i + 1) % 4
		b.edge(pts[i][0], pts[i][1], pts[j][0], pts[j][1], 1.6, rim, a)
	}
}

func (b *fxBatch) flame(x, y, h, w, sway float64, col color.NRGBA, a float64) {
	if h <= 0 || a <= 0 {
		return
	}
	var pts [17][2]float64
	n := 0
	for j := range 4 {
		phi := math.Pi/2 - float64(j)/3*math.Pi/2
		pts[n] = [2]float64{x + math.Cos(phi)*w/2, y + math.Sin(phi)*w/2}
		n++
	}
	for j := 1; j <= 4; j++ {
		t := float64(j) / 4
		pts[n] = [2]float64{x + w/2*math.Pow(1-t, 1.4) + sway*t*t, y - h*t}
		n++
	}
	for j := 3; j >= 1; j-- {
		t := float64(j) / 4
		pts[n] = [2]float64{x - w/2*math.Pow(1-t, 1.4) + sway*t*t, y - h*t}
		n++
	}
	for j := range 3 {
		phi := math.Pi - float64(j)/3*math.Pi/2
		pts[n] = [2]float64{x + math.Cos(phi)*w/2, y + math.Sin(phi)*w/2}
		n++
	}
	b.polyFade(pts[:n], col, a, a*0.6)
}

func (b *fxBatch) star(cx, cy, r, inner, rot float64, n int, c color.NRGBA, a float64) {
	if r <= 0 || a <= 0 {
		return
	}
	m := b.v(cx, cy, lighten(c, 0.7), a)
	first := uint32(len(b.vs)) //nolint:gosec // One frame cannot allocate 2^32 vertices.
	for i := range 2 * n {
		rr := r
		if i%2 == 1 {
			rr = r * inner
		}
		ang := rot + float64(i)/float64(2*n)*2*math.Pi
		b.v(cx+math.Cos(ang)*rr, cy+math.Sin(ang)*rr, c, a*0.15)
	}
	for i := range 2 * n {
		b.tri(m, first+uint32(i), first+uint32((i+1)%(2*n))) //nolint:gosec // Shape vertex indexes are bounded by the frame vertex allocation.
	}
}

func (s *battleScene) drawEffects(g *Game, dst *ebiten.Image) {
	sw, sh := g.screen()
	for _, e := range s.scr {
		p := float64(e.t) / float64(e.dur)
		switch e.kind {
		case "flash":
			a := e.alpha * (1 - p) * (1 - p)
			fill(dst, withAlpha(e.color, uint8(255*clamp01(a))))
		case "wave":
			b := &s.fxb
			r := easeOut(p) * math.Hypot(sw, sh) * 0.8
			b.ring(e.x, e.y, r, r*0.55, 80+120*p, e.color, e.alpha*(1-p))
			b.ring(e.x, e.y, r*0.82, r*0.45, 30, lighten(e.color, 0.6), e.alpha*(1-p))
		}
	}
	for _, e := range s.fx {
		s.drawFx(dst, e)
		if s.fxb.full() {
			s.fxb.flush(dst)
		}
	}
	s.fxb.flush(dst)
}

func (s *battleScene) drawFx(_ *ebiten.Image, e effectAnim) {
	b := &s.fxb
	p := float64(e.t) / float64(e.dur)
	cx, cy := e.x+e.w/2, e.y+e.h*0.45
	R := math.Max(50, math.Min(150, math.Min(e.w, e.h)*0.55))
	k := []float64{0.75, 1, 1.3}[min(2, max(0, e.power))]
	R *= k
	c := e.color
	white := colorWhite
	fade := 1 - clamp01((p-0.6)/0.4)
	r := func(i int) float64 { return rnd(e.seed, i) }

	sparks := func(n int, from, speed, width float64, col color.NRGBA, start int) {
		q := clamp01((p - from) / (1 - from))
		if q <= 0 || q >= 1 {
			return
		}
		for i := range n {
			ang := r(start+i) * 2 * math.Pi
			sp := speed * (0.5 + r(start+i+50))
			d := easeOut(q) * sp
			x, y := cx+math.Cos(ang)*d, cy+math.Sin(ang)*d+q*q*R*0.4
			tail := sp * 0.18 * (1 - q)
			b.line(x, y, x-math.Cos(ang)*tail, y-math.Sin(ang)*tail, width, col, (1-q)*0.95)
		}
	}
	seg := func(a, z float64) float64 { return clamp01((p - a) / (z - a)) }

	gather := func(n int, x, y, far, u float64, col color.NRGBA, start int) {
		for i := range n {
			t := clamp01(u*1.3 - r(start+i)*0.3)
			if t <= 0 || t >= 1 {
				continue
			}
			ang := r(start+i+30)*2*math.Pi + t*1.2
			d := far * (1 - easeOut(t)) * (0.6 + 0.5*r(start+i+60))
			px, py := x+math.Cos(ang)*d, y+math.Sin(ang)*d*0.8
			b.glow(px, py, 9*k, 0, col, 0.8*t)
			b.fan(px, py, 2.5*k, 0, white, t, 0.6)
		}
	}

	if w0 := fxHit(e.kind); w0 > 0 && slices.Contains([]string{"strike", "fire", "thunder", "holy", "heal"}, e.kind) {
		u := p / w0
		if u < 1 {
			switch e.kind {
			case "strike":
				rr := R * (1.5 - 1.25*easeOut(u))
				b.ring(cx, cy, rr, 0, 10*k, c, 0.9*u)
				b.ring(cx, cy, rr, 0, 3*k, white, u)
				b.glow(cx, cy, R*0.5*u, 0, lighten(c, 0.5), 0.6*u)
			case "fire":
				gather(10+4*e.power, cx, cy, R*1.5, u, c, 2000)
				b.glow(cx, cy, R*(0.25+0.4*u), 0, c, 0.8*u)
				b.flame(cx, cy+R*0.12*u, R*0.5*u, R*0.3*u, math.Sin(u*12)*R*0.05, color.NRGBA{0xff, 0xe8, 0x90, 0xff}, u)
			case "thunder":
				b.glow(cx, math.Max(0, e.y-R*2.2), R*1.4, R*0.5, lighten(c, 0.3), 0.5*u)
				for i := range 4 + 2*e.power {
					if (e.t+i)%3 == 0 {
						continue
					}
					ang := r(2100+i+e.t/2*7) * 2 * math.Pi
					x0, y0 := cx+math.Cos(ang)*R*0.6, cy+math.Sin(ang)*R*0.6
					x1, y1 := x0+(r(2200+i+e.t)-0.5)*R*0.5, y0+(r(2300+i+e.t)-0.5)*R*0.5
					b.line(x0, y0, x1, y1, 10*k, c, 0.7*u)
					b.edge(x0, y0, x1, y1, 2*k, white, u)
				}
			case "holy":
				for i := range 10 + 4*e.power {
					t := clamp01(u*1.4 - r(2400+i)*0.4)
					x := cx + (r(2410+i)-0.5)*R*1.6
					y := e.y - R*1.2 + t*(cy-e.y+R*1.2)
					b.star(x, y, 12*k, 0.2, 0, 4, white, t*(1-t)*3)
				}
				b.ring(cx, e.y-R*0.3, R*0.7*easeOut(u), R*0.18*easeOut(u), 6*k, c, u)
			case "heal":
				gather(10+4*e.power, cx, e.y+e.h*0.85, R*1.3, u, c, 2500)
				b.ring(cx, e.y+e.h*0.85, R*0.7*easeOut(u), R*0.16*easeOut(u), 8*k, c, u)
			}
			return
		}
		p = (p - w0) / (1 - w0)
		fade = 1 - clamp01((p-0.6)/0.4)
	}
	switch e.kind {
	case "slash":

		n := []int{1, 2, 3}[min(2, e.power)]
		for i := range n {
			st := float64(i) * 0.07
			phi := []float64{-0.55, 2.45, 0.95}[i] + (r(i)-0.5)*0.4
			rr := R * (1.7 + 0.3*r(i+10))
			span := 1.25
			ox, oy := cx-math.Cos(phi)*rr, cy-math.Sin(phi)*rr
			a0 := phi - span/2
			sx, sy := ox+math.Cos(a0)*rr, oy+math.Sin(a0)*rr
			if u := seg(st, st+0.13); u < 1 {
				b.star(sx, sy, (10+26*u)*k, 0.12, u*1.5, 4, white, u)
				b.glow(sx, sy, 30*k*u, 0, c, 0.7*u)
			}
			sw := easeOut(seg(st+0.13, st+0.25))
			if sw <= 0 {
				continue
			}
			t := seg(st+0.25, 1)
			a := 1 - t*t
			thin := 1 - 0.7*t
			drift := t * 0.12
			u0 := math.Max(0, sw-0.85) + drift
			b.crescent(ox, oy, rr, a0, span, 56*k*thin, u0, math.Min(1, sw+drift), mixColor(c, color.NRGBA{0x20, 0x50, 0xc0, 0xff}, 0.35), a*0.9, 0.1)
			b.crescent(ox, oy, rr-4*k, a0, span, 20*k*thin, u0, math.Min(1, sw+drift), lighten(c, 0.6), a, 0.3)
			b.crescent(ox, oy, rr-6*k, a0, span, 7*k*thin, u0, math.Min(1, sw+drift), white, a, 1)
			if sw < 1 {
				ha := a0 + span*sw
				b.star(ox+math.Cos(ha)*rr, oy+math.Sin(ha)*rr, 26*k, 0.15, 0.3, 4, white, 1)
			}
		}
		if u := seg(0.18, 0.4); u > 0 && u < 1 {
			b.glow(cx, cy, R*(0.6+0.5*u), 0, white, 0.7*(1-u))
			b.star(cx, cy, R*(0.5+0.6*u), 0.1, 0.8, 4, lighten(c, 0.5), 1-u)
		}
		if p > 0.2 {
			pp := p
			p = 0.2 + (pp-0.2)*1.0
			sparks(6+4*e.power, 0.2, R*1.4, 4*k, lighten(c, 0.5), 10)
			p = pp
		}
	case "pierce":

		n := []int{1, 1, 3}[min(2, e.power)]
		for i := range n {
			off := float64(i-n/2) * R * 0.32
			x0, y0 := cx-R*2.3, cy+R*0.55+off
			x1, y1 := cx, cy+off*0.3
			if u := seg(float64(i)*0.04, 0.16); u < 1 {
				b.glow(x0, y0, R*0.45*u, 0, c, 0.8*u)
				b.star(x0, y0, R*0.4*u, 0.1, 0, 4, white, u)
			}
			q := easeOut(seg(0.14+float64(i)*0.03, 0.24+float64(i)*0.03))
			if q <= 0 {
				continue
			}
			t := seg(0.3, 1)
			a := 1 - t
			hx, hy := x0+(x1-x0)*q, y0+(y1-y0)*q
			tx, ty := x0+(x1-x0)*math.Max(0, q-0.75-0.25*t), y0+(y1-y0)*math.Max(0, q-0.75-0.25*t)
			b.blade(tx, ty, hx, hy, 34*k, mixColor(c, color.NRGBA{0x20, 0x50, 0xc0, 0xff}, 0.35), a*0.9)
			b.blade(tx+(hx-tx)*0.3, ty+(hy-ty)*0.3, hx, hy, 9*k, white, a)
			for j := range 4 {
				o := (r(i*10+j+20) - 0.5) * R * 0.5
				l := R * (0.6 + 0.8*r(i*10+j+30))
				dx, dy := (x1-x0)/math.Hypot(x1-x0, y1-y0), (y1-y0)/math.Hypot(x1-x0, y1-y0)
				ex, ey := hx-dx*R*0.3-dy*o, hy-dy*R*0.3+dx*o
				b.edge(ex-dx*l, ey-dy*l, ex, ey, 5*k, mixColor(c, color.NRGBA{0x20, 0x50, 0xc0, 0xff}, 0.35), a*0.6)
				b.edge(ex-dx*l, ey-dy*l, ex, ey, 2*k, white, a*0.8)
			}
		}
		if u := seg(0.22, 0.75); u > 0 && u < 1 {
			b.glow(cx, cy, R*0.8, 0, white, 0.8*(1-u)*(1-u))
			b.star(cx, cy, R*(0.6+1.0*easeOut(u)), 0.06, 0.2, 4, c, 1-u)
			b.star(cx, cy, R*(0.4+0.6*easeOut(u)), 0.06, 0.2, 4, white, 1-u)
			b.ring(cx, cy, R*(0.3+1.0*easeOut(u)), R*(0.2+0.6*easeOut(u)), 10*k*(1-u)+2, lighten(c, 0.4), 1-u)
			for j := range 6 + 3*e.power {
				ang := -0.2 + (r(300+j)-0.5)*1.4
				d := R * (0.3 + 1.4*easeOut(u)) * (0.6 + 0.6*r(310+j))
				b.splinter(cx+math.Cos(ang)*d, cy+math.Sin(ang)*d+u*u*R*0.6, 8*k, u*6+ang, lighten(c, 0.4), white, 1-u)
			}
		}
	case "strike":
		q := easeOut(p / 0.6)
		b.glow(cx, cy, R*(0.6+0.6*q), 0, c, 0.5*(1-clamp01(p/0.4)))
		b.glow(cx, cy, R*0.45, 0, white, 0.6*(1-clamp01(p/0.25)))
		b.ring(cx, cy, R*(0.3+1.2*q), 0, 22*k*(1-q)+4, c, fade*(1-q))
		for i := range 8 {
			ang := float64(i)/8*2*math.Pi + r(i)*0.4
			l0, l1 := R*(0.3+0.6*q), R*(0.6+1.0*q)
			b.blade(cx+math.Cos(ang)*l0, cy+math.Sin(ang)*l0, cx+math.Cos(ang)*l1, cy+math.Sin(ang)*l1, 12*k, lighten(c, 0.4), (1-q)*0.9)
		}
		if p < 0.3 {
			t := p / 0.3
			b.star(cx, cy, R*(0.75+0.5*t), 0.3, r(5), 8, mixColor(c, color.NRGBA{0xff, 0x7a, 0x20, 0xff}, 0.65), 1-t)
			b.star(cx, cy, R*(0.45+0.3*t), 0.3, r(5)+0.2, 8, white, 1-t)
		}

		for i := range 6 + 4*e.power {
			ang := -math.Pi*r(30+i) - 0.1
			sp := R * (0.8 + r(60+i))
			x := cx + math.Cos(ang)*sp*p
			y := cy + math.Sin(ang)*sp*p + p*p*R*1.6
			b.splinter(x, y, (7+5*r(90+i))*k, p*8*(r(70+i)-0.5)+ang, mixColor(c, white, 0.3), color.NRGBA{0x50, 0x38, 0x20, 0xff}, fade)
		}
	case "fire":

		hot := color.NRGBA{0xff, 0xf4, 0xc0, 0xff}
		red := color.NRGBA{0xd8, 0x30, 0x10, 0xff}
		foot := math.Max(cy+R*0.75, e.y+e.h*0.88)
		q := easeOut(p / 0.3)
		b.glow(cx, cy, R*(0.6+1.0*q), 0, c, 0.22*fade)
		b.glow(cx, cy, R*(0.5+0.4*q), 0, hot, 0.85*(1-clamp01(p/0.3)))
		if p < 0.3 {
			b.ring(cx, cy, R*(0.3+1.2*q), 0, 16*k, hot, 0.9*(1-q))
		}
		n := 9 + 6*e.power
		for layer := range 2 {
			for i := range n {
				born := r(i+100) * 0.3
				life := 0.45 + 0.3*r(i)
				t := (p - born) / life
				if t <= 0 || t >= 1 {
					continue
				}
				f := (r(i+200) - 0.5) * 2
				x := cx + f*math.Max(R*0.8, e.w*0.38)
				y := foot - t*R*0.5 - (1-math.Abs(f))*R*0.15
				h := R * (1.0 + 0.9*r(i+300)) * (1 - math.Abs(f)*0.4) * math.Min(1, t*5) * (1 - t*0.45)
				w := h * (0.42 + 0.15*r(i+400))
				sway := math.Sin(t*9+r(i)*6) * w * 0.7
				a := (1 - t) * math.Min(1, t*8)
				if layer == 0 {
					col := mixColor(c, red, clamp01(t*1.4+0.1))
					b.flame(x, y, h, w, sway, col, a*0.95)
				} else {
					col := mixColor(hot, c, clamp01(t*1.2))
					b.flame(x, y-h*0.05, h*0.6, w*0.5, sway*0.6, col, a)
				}
			}
		}

		for i := range 10 + 8*e.power {
			born := 0.15 + r(i+500)*0.4
			t := clamp01((p - born) / 0.45)
			if t <= 0 || t >= 1 {
				continue
			}
			x := cx + (r(i+510)-0.5)*R*1.8 + math.Sin(t*7+r(i+520)*6)*R*0.12
			y := foot - R*0.3 - t*R*(1.2+0.8*r(i+530))
			b.glow(x, y, 7*k, 0, c, (1-t)*0.8)
			b.fan(x, y, 2.5*k, 0, hot, 1-t, 0.7)
		}
		sparks(8+6*e.power, 0.05, R*1.6, 3*k, hot, 400)
	case "ice":

		deep := color.NRGBA{0x10, 0x3c, 0x8c, 0xff}
		face := mixColor(c, color.NRGBA{0x38, 0xa6, 0xf8, 0xff}, 0.6)
		foot := e.y + e.h*0.94
		W := math.Max(R*1.05, e.w*0.4)
		pw := min(2, max(0, e.power))

		if p < 0.4 {
			q := clamp01(p / 0.34)
			for i := range 14 + 10*pw {
				a0 := r(900+i)*2*math.Pi + q*1.6
				d := R * (0.25 + 1.6*(1-easeOut(q))) * (0.6 + 0.6*r(920+i))
				x, y := cx+math.Cos(a0)*d, foot-R*0.45+math.Sin(a0)*d*0.55
				al := clamp01(q*3) * (1 - clamp01((p-0.3)/0.1))
				b.glow(x, y, 10*k, 0, face, al*0.7)
				b.fan(x, y, 3.5*k, 0, white, al, 0.6)
				if i%3 == 0 {
					b.star(x, y, 14*k, 0.2, q, 4, white, al)
				}
			}
		}

		fq := easeOut(clamp01((p - 0.08) / 0.3))
		if fq > 0 {
			fr := W * 1.35 * fq
			b.glow(cx, foot, fr, fr*0.24, face, 0.55*fade)
			b.ring(cx, foot, fr, fr*0.24, 7*k, white, 0.85*fade)
			for i := range 8 + 4*pw {
				ang := float64(i)/float64(8+4*pw)*2*math.Pi + r(940+i)*0.4
				l := fr * (0.55 + 0.4*r(950+i))
				mx, my := cx+math.Cos(ang+0.15)*l*0.5, foot+math.Sin(ang+0.15)*l*0.5*0.24
				ex, ey := cx+math.Cos(ang)*l, foot+math.Sin(ang)*l*0.24
				b.edge(cx, foot, mx, my, 2, white, 0.75*fade)
				b.edge(mx, my, ex, ey, 1.5, white, 0.6*fade)
			}
		}

		n := []int{7, 11, 16}[pw]
		for row := range 2 {
			for i := range n {
				back := r(800+i) < 0.4
				if back != (row == 0) {
					continue
				}
				f := (float64(i)+0.5)/float64(n)*2 - 1 + (r(810+i)-0.5)*0.22
				x := cx + f*W
				ang := -math.Pi/2 + f*0.6 + (r(820+i)-0.5)*0.4
				l := R * (0.6 + 1.0*r(830+i)) * (1.55 - 0.75*math.Abs(f))
				w := l * (0.2 + 0.12*r(840+i))
				col := mixColor(face, white, 0.1+0.4*r(870+i))
				if back {
					l *= 0.75
					col = mixColor(col, deep, 0.25)
				}
				born := 0.16 + math.Abs(f)*0.1 + r(850+i)*0.06
				g := clamp01((p - born) / 0.11)
				if g <= 0 {
					continue
				}
				grow := 1 + 2.2*(g-1)*(g-1)*(g-1) + 1.2*(g-1)*(g-1)
				brk := 0.56 + r(860+i)*0.1
				sh := clamp01((p - brk) / 0.08)
				if sh < 1 {
					light := 0.5 + 0.5*math.Sin(p*18+r(880+i)*6)
					b.crystal(x, foot+4, ang, l*grow, w*math.Min(1, g*1.4), col, deep, (1-sh)*0.95, light)
				}

				if q := clamp01((p - brk) / (1 - brk)); q > 0 && q < 1 {
					dx, dy := math.Cos(ang), math.Sin(ang)
					for j := range 2 + pw {
						t := 0.25 + 0.6*r(1000+i*7+j)
						sx, sy := x+dx*l*t, foot+dy*l*t
						va := ang + (r(1100+i*7+j)-0.5)*2.4
						sp := R * (0.6 + 0.9*r(1200+i*7+j))
						px := sx + math.Cos(va)*sp*easeOut(q)
						py := sy + math.Sin(va)*sp*easeOut(q)*0.7 + q*q*R*0.9
						sz := w * (0.5 + 0.45*r(1300+i*7+j)) * (1 - q*0.4)
						b.splinter(px, py, sz, q*7*(r(1400+i*7+j)-0.5)+va, lighten(col, 0.2), white, (1-q)*0.95)
					}
				}
			}
		}

		if q := clamp01((p - 0.22) / 0.12); q > 0 && q < 1 {
			b.glow(cx, foot-R*0.7, R*(1.3+0.5*q), R*1.1, lighten(c, 0.6), 0.85*(1-q))
			b.glow(cx, foot-R*0.7, R*0.8, R*0.6, white, 0.9*(1-q))
			b.star(cx, foot-R*0.8, R*(0.9+0.6*q)*k, 0.05, 0.3, 4, white, (1 - q))
			b.star(cx, foot-R*0.8, R*(0.5+0.4*q)*k, 0.05, 0.3+math.Pi/4, 4, face, (1-q)*0.8)
		}

		if p > 0.5 {
			q := clamp01((p - 0.5) / 0.5)
			b.glow(cx, foot-R*0.25, W*1.2, R*0.45, lighten(c, 0.6), 0.35*(1-q))
			for i := range 6 + 4*pw {
				x := cx + (r(1500+i)-0.5)*W*2
				y := foot - R*(0.2+1.2*r(1510+i)) - q*R*0.3
				tw := math.Sin(q * math.Pi * (1 + r(1520+i)))
				b.star(x, y, 11*k, 0.2, 0, 4, white, math.Max(0, tw)*(1-q))
			}
		}
	case "thunder":

		top := math.Max(0, e.y-R*3)
		if p < 0.55 && (e.t/2)%3 != 2 {
			fr := e.t / 3
			segs := 9
			px, py := cx+(r(fr*31)-0.5)*R*0.6, top
			for i := 1; i <= segs; i++ {
				f := float64(i) / float64(segs)
				nx := cx + (r(fr*31+i)-0.5)*R*0.9*(1-f)
				ny := top + (cy-top)*f
				b.line(px, py, nx, ny, 60*k, c, 0.55)
				b.line(px, py, nx, ny, 24*k, mixColor(c, color.NRGBA{0xff, 0xb0, 0x00, 0xff}, 0.4), 1)
				b.edge(px, py, nx, ny, 5*k, white, 1)
				if i == segs/2 && e.power >= 1 {
					bx, by := nx+(r(fr*31+70)-0.3)*R*1.2, ny+R*0.6
					b.line(nx, ny, bx, by, 18*k, c, 0.5)
					b.line(nx, ny, bx, by, 4*k, white, 0.9)
				}
				px, py = nx, ny
			}
		}
		b.glow(cx, cy, R*1.2, 0, c, 0.45*(1-clamp01(p/0.6)))
		b.glow(cx, cy, R*0.5, 0, white, 0.6*(1-clamp01(p/0.3)))
		if p < 0.55 && (e.t/2)%3 != 2 {
			b.star(cx, cy, R*(0.6+0.3*r(e.t/3)), 0.12, r(e.t/3+9)*3, 6, c, 0.95)
			b.star(cx, cy, R*0.4, 0.15, r(e.t/3+5)*3, 6, white, 1)
		}
		if t := clamp01(p / 0.5); t < 1 {
			b.ring(cx, cy, R*(0.3+1.1*easeOut(t)), 0, 12*k*(1-t)+3, c, 1-t)
		}
		sparks(8+5*e.power, 0.1, R*1.5, 3*k, lighten(c, 0.5), 600)
	case "holy":

		q := clamp01(p / 0.25)
		wd := R * 0.9 * easeOut(q) * fade
		top := e.y - R*1.5
		b.line(cx, top, cx, cy+R*0.9, wd*2, c, 0.5*fade)
		b.line(cx, top, cx, cy+R*0.9, wd*0.6, white, 0.8*fade)
		b.edge(cx-wd*0.5, top, cx-wd*0.5, cy+R*0.9, 2, white, 0.7*fade)
		b.edge(cx+wd*0.5, top, cx+wd*0.5, cy+R*0.9, 2, white, 0.7*fade)
		for i := range 2 {
			t := clamp01((p - float64(i)*0.15) / 0.6)
			if t > 0 && t < 1 {
				y := top + (cy+R*0.9-top)*easeOut(t)
				b.ring(cx, y, R*0.9, R*0.22, 8*k, c, (1-t)*0.95)
				b.ring(cx, y, R*0.9, R*0.22, 2.5*k, white, 1-t)
			}
		}
		b.star(cx, cy, R*(0.6+0.8*easeOut(p/0.5)), 0.08, 0, 4, lighten(c, 0.3), fade)
		b.glow(cx, cy, R*1.1, 0, c, 0.35*fade)
		for i := range 10 + 4*e.power {
			born := r(i) * 0.5
			t := clamp01((p - born) / 0.5)
			if t <= 0 || t >= 1 {
				continue
			}
			x := cx + (r(i+40)-0.5)*R*1.6
			y := cy + R*0.6 - t*R*1.8
			b.star(x, y, 10*k, 0.25, 0, 4, white, 1-t)
		}
	case "dark":

		core := color.NRGBA{0x2a, 0x10, 0x48, 0xff}
		q := clamp01(p / 0.36)
		b.glow(cx, cy, R*(1.2-0.5*q), 0, c, 0.35*fade)
		if p < 0.4 {
			cr := R * (0.25 + 0.3*q)
			b.fan(cx, cy, cr, 0, core, 0.85, 0.7)
			b.ring(cx, cy, cr, 0, 8*k, lighten(c, 0.3), 0.9)
		}
		arms := 5 + 3*e.power
		for i := range arms {
			a0 := float64(i)/float64(arms)*2*math.Pi + r(i)*0.5
			al := clamp01(q*3) * (1 - clamp01((p-0.32)/0.06))
			if al <= 0 {
				continue
			}
			px, py := 0.0, 0.0
			for j := range 13 {
				u := float64(j) / 12
				d := R * (0.25 + 1.5*(1-u)*(1-0.6*q))
				ang := a0 + u*2.2 + q*3
				x, y := cx+math.Cos(ang)*d, cy+math.Sin(ang)*d*0.8
				if j > 0 {
					b.line(px, py, x, y, (4+10*u)*k, c, al*0.9)
					b.edge(px, py, x, y, (1+2*u)*k, lighten(c, 0.5), al*u)
				}
				px, py = x, y
			}
		}
		if p > 0.34 {
			t := clamp01((p - 0.34) / 0.66)
			b.glow(cx, cy, R*(0.6+0.4*t), 0, lighten(c, 0.6), 0.8*(1-t)*(1-t))
			b.ring(cx, cy, R*(0.3+1.4*easeOut(t)), 0, 30*k*(1-t)+4, c, 1-t)
			b.ring(cx, cy, R*(0.2+1.1*easeOut(t)), 0, 8*k, lighten(c, 0.5), 1-t)
			for i := range 8 + 4*e.power {
				ang := r(i+40) * 2 * math.Pi
				d0, d1 := R*(0.2+1.0*easeOut(t)), R*(0.5+1.5*easeOut(t))
				b.blade(cx+math.Cos(ang)*d0, cy+math.Sin(ang)*d0, cx+math.Cos(ang)*d1, cy+math.Sin(ang)*d1, 16*k, c, (1-t)*0.95)
				b.blade(cx+math.Cos(ang)*d0, cy+math.Sin(ang)*d0, cx+math.Cos(ang)*d1, cy+math.Sin(ang)*d1, 4*k, white, 1-t)
			}
		}
	case "magic":

		ca := clamp01(p/0.12) * (1 - seg(0.28, 0.4))
		s.circle(b, cx, cy+R*0.55, R*1.05, c, p, ca)
		if u := seg(0, 0.3); u < 1 {
			gather(12+6*e.power, cx, cy, R*1.6, u, c, 3000)
			b.glow(cx, cy, R*(0.15+0.35*u), 0, c, 0.8*u)
			b.fan(cx, cy, R*0.12*u, 0, white, u, 0.5)
			for i := range 6 {
				ang := float64(i)/6*2*math.Pi + p*2
				x := cx + math.Cos(ang)*R*0.9
				y := cy + R*0.55 + math.Sin(ang)*R*0.45
				b.line(x, y, x, y-R*0.9*u, 10*k, lighten(c, 0.3), 0.6*u)
			}
		}
		if t := seg(0.28, 1); t > 0 {
			b.glow(cx, cy, R*(0.6+0.8*easeOut(t)), 0, c, 0.6*(1-t))
			b.glow(cx, cy, R*0.6, 0, white, 0.9*(1-t)*(1-t))
			b.star(cx, cy, R*(0.8+0.9*easeOut(t)), 0.1, 0.4, 8, c, 1-t)
			b.star(cx, cy, R*(0.5+0.5*easeOut(t)), 0.1, 0.4, 8, white, (1-t)*0.9)
			b.ring(cx, cy, R*(0.4+1.3*easeOut(t)), 0, 20*k*(1-t)+3, lighten(c, 0.4), 1-t)
			b.ring(cx, cy, R*(0.3+1.0*easeOut(t)), R*(0.3+1.0*easeOut(t))*0.4, 6*k, white, 1-t)
			for i := range 10 + 6*e.power {
				ang := r(3100+i) * 2 * math.Pi
				d := R * (0.2 + 1.5*easeOut(t)) * (0.6 + 0.5*r(3110+i))
				x, y := cx+math.Cos(ang)*d, cy+math.Sin(ang)*d*0.85
				b.glow(x, y, 14*k, 0, c, (1-t)*0.9)
				b.fan(x, y, 4*k, 0, white, 1-t, 0.6)
			}
		}
	case "heal":

		foot := e.y + e.h*0.85
		q := easeOut(seg(0, 0.25))
		b.ring(cx, foot, R*(0.7+0.3*q), R*(0.16+0.06*q), 10*k, c, fade)
		b.ring(cx, foot, R*(0.7+0.3*q), R*(0.16+0.06*q), 3*k, white, fade)
		top := e.y - R*0.3
		cw := R * 0.9 * q * fade
		b.line(cx, foot, cx, top, cw*2, c, 0.35*fade)
		b.line(cx, foot, cx, top, cw*0.5, white, 0.5*fade)
		if u := seg(0, 0.3); u < 1 {
			b.glow(cx, (foot+top)/2, R*1.1, (foot-top)*0.6, lighten(c, 0.6), 0.6*(1-u))
		}
		for i := range 12 + 5*e.power {
			born := r(i) * 0.45
			t := clamp01((p - born) / 0.55)
			if t <= 0 || t >= 1 {
				continue
			}
			x := cx + (r(i+40)-0.5)*e.w*0.8 + math.Sin(t*6+r(i+50)*6)*R*0.1
			y := foot - t*(e.h*0.9+R*0.4)
			a := (1 - t) * math.Min(1, t*6)
			b.glow(x, y, 14*k, 0, c, a*0.8)
			if i%2 == 0 {
				d := 14 * k
				b.edge(x-d, y, x+d, y, 7*k, white, a)
				b.edge(x, y-d, x, y+d, 7*k, white, a)
				b.edge(x-d*0.8, y, x+d*0.8, y, 3.5*k, mixColor(c, color.NRGBA{0x20, 0xa0, 0x40, 0xff}, 0.4), a)
				b.edge(x, y-d*0.8, x, y+d*0.8, 3.5*k, mixColor(c, color.NRGBA{0x20, 0xa0, 0x40, 0xff}, 0.4), a)
			} else {
				b.star(x, y, 12*k, 0.2, 0, 4, white, a)
			}
		}
	case "buff", "debuff":

		up := e.kind == "buff"
		foot, head := e.y+e.h*0.92, e.y+e.h*0.08
		dir := -1.0
		if !up {
			dir = 1
		}
		b.glow(cx, (foot+head)/2, R*1.1, (foot-head)*0.6, c, 0.3*fade*(0.8+0.2*math.Sin(p*20)))
		for i := range 3 {
			t := seg(float64(i)*0.15, float64(i)*0.15+0.55)
			if t <= 0 || t >= 1 {
				continue
			}
			from, to := foot, head
			if !up {
				from, to = head, foot
			}
			y := from + (to-from)*easeOut(t)
			b.ring(cx, y, R*(0.9-0.2*t), R*0.2, 18*k, c, (1-t)*0.95)
			b.ring(cx, y, R*(0.9-0.2*t), R*0.2, 3*k, white, 1-t)
		}
		for i := range 5 + 2*e.power {
			t := seg(r(3200+i)*0.4, r(3200+i)*0.4+0.5)
			if t <= 0 || t >= 1 {
				continue
			}
			ax := cx + (r(3210+i)-0.5)*R*1.6
			ay := (foot+head)/2 - dir*R*0.8 + dir*t*R*1.4
			d := 26 * k
			a := (1 - t) * math.Min(1, t*5)

			b.edge(ax-d, ay-dir*d*0.6, ax, ay+dir*d*0.4, 12*k, c, a)
			b.edge(ax+d, ay-dir*d*0.6, ax, ay+dir*d*0.4, 12*k, c, a)
			b.edge(ax-d, ay-dir*d*0.6, ax, ay+dir*d*0.4, 2.5*k, white, a)
			b.edge(ax+d, ay-dir*d*0.6, ax, ay+dir*d*0.4, 2.5*k, white, a)
		}
		for i := range 8 + 4*e.power {
			t := seg(r(3300+i)*0.5, r(3300+i)*0.5+0.5)
			if t <= 0 || t >= 1 {
				continue
			}
			x := cx + (r(3310+i)-0.5)*e.w*0.8
			y := foot - t*(foot-head)
			if !up {
				y = head + t*(foot-head)
			}
			b.glow(x, y, 8*k, 0, c, (1-t)*0.9)
			b.fan(x, y, 2.5*k, 0, white, 1-t, 0.6)
		}
	case "cast":
		s.circle(b, cx, cy, math.Max(R*0.9, e.w*0.45), c, p, clamp01(p/0.2)*fade)
	}
}

func (s *battleScene) circle(b *fxBatch, cx, cy, r float64, c color.NRGBA, p, a float64) {
	if a <= 0 {
		return
	}
	ry := r * 0.5
	k := ry / r
	rot := p * 2.4
	b.ring(cx, cy, r, ry, 8, c, a)
	b.ring(cx, cy, r*0.78, ry*0.78, 5, lighten(c, 0.4), a)
	b.glow(cx, cy, r*1.05, ry*1.05, c, 0.3*a)
	pt := func(i int, n int, rr float64, off float64) (float64, float64) {
		ang := rot + off + float64(i)/float64(n)*2*math.Pi
		return cx + math.Cos(ang)*rr, cy + math.Sin(ang)*rr*k
	}

	for i := range 6 {
		x0, y0 := pt(i, 6, r*0.78, 0)
		x1, y1 := pt(i+2, 6, r*0.78, 0)
		b.line(x0, y0, x1, y1, 5, lighten(c, 0.3), a)
	}

	for i := range 16 {
		x0, y0 := pt(i, 16, r*0.8, -rot*2)
		x1, y1 := pt(i, 16, r*0.96, -rot*2)
		b.line(x0, y0, x1, y1, 4, c, a*0.9)
	}
}
