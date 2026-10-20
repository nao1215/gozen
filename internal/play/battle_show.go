package play

import (
	"fmt"
	"image"
	"image/color"
	"math"
	"strings"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/vector"

	"github.com/nao1215/gozen/internal/battle"
	"github.com/nao1215/gozen/internal/layer5/gfx"
	"github.com/nao1215/gozen/internal/rpg"
	lang "github.com/nao1215/gozen/internal/text"
)

type cutIn struct {
	u    *battle.Unit
	path string
	line string
	bust bool
	box  []float64
	name string
	t    int
}

const cutInTicks = 64

const (
	introTicks = 26
	bossTicks  = 110
)

func (s *battleScene) startCutIn(g *Game, st battle.Step) bool {
	use := s.usage(st.Skill)
	if use == nil || !use.Cutin || st.User == nil {
		return false
	}
	name := st.Skill
	if sk := s.b.DB.Skills[st.Skill]; sk != nil {
		name = sk.Name
	} else if it := s.b.DB.Items[st.Skill]; it != nil {
		name = it.Name
	}
	path, line, bust := "", "", false
	var box []float64
	if st.User.Enemy {
		path = st.User.Def.Image.File
	} else if st.User.Actor != nil {
		d := st.User.Actor.Def()
		path = g.faceOf(st.User.Actor.ID, "")
		if d.Cutin != "" {
			path, bust, box = d.Cutin, true, d.CutinBox
		}
		if d.NoStanding && d.Cutin == "" {
			path = "-"
		}
		if n := len(d.CutinLines); n > 0 {
			s.cutCount++
			line = d.CutinLines[(s.cutCount+st.User.Index)%n]
		}
	}
	if path == "" {
		return false
	}
	s.cut = &cutIn{u: st.User, path: path, name: name, line: line, bust: bust, box: box}
	g.sfx("cutin")
	g.emit("battle", traceLine{"cutin": name})
	return true
}

func (s *battleScene) drawCutIn(g *Game, dst *ebiten.Image) {
	c := s.cut
	sw, sh := g.screen()
	p := float64(c.t) / cutInTicks

	open := math.Min(1, float64(c.t)/8)
	if c.t > cutInTicks-10 {
		open = math.Max(0, float64(cutInTicks-c.t)/10)
	}
	open = 1 - (1-open)*(1-open)
	fill(dst, withAlpha(colorBlack, uint8(110*open)))
	bandH := 250.0 * open
	cy := sh * 0.44
	if bandH < 2 {
		return
	}
	accent := g.th.accent
	if c.u.Enemy {
		accent = color.NRGBA{0xff, 0x8a, 0x9a, 0xff}
	}
	slant := 60.0

	band := func(y0, y1 float64, col color.NRGBA) {
		var path vector.Path
		path.MoveTo(float32((-slant)*g.ui), float32((y1)*g.ui))
		path.LineTo(float32((sw+slant)*g.ui), float32((y1-slant)*g.ui))
		path.LineTo(float32((sw+slant)*g.ui), float32((y0-slant)*g.ui))
		path.LineTo(float32((-slant)*g.ui), float32((y0)*g.ui))
		path.Close()
		op := &vector.DrawPathOptions{AntiAlias: true}
		op.ColorScale.ScaleWithColor(col)
		vector.FillPath(dst, &path, nil, op)
	}
	top, bottom := cy-bandH/2+slant/2, cy+bandH/2+slant/2
	band(top-8, bottom+8, withAlpha(accent, 0xe0))
	band(top, bottom, withAlpha(g.th.window, 0xf4))

	k := g.ui
	if s.cutBuf == nil || s.cutBuf.Bounds().Dx() != int(sw*k) {
		s.cutBuf = ebiten.NewImage(int(sw*k), int(360*k))
	}
	buf := s.cutBuf
	buf.Clear()
	bh := 360.0

	for i := range 14 {
		y := math.Mod(float64(i)*53.7, bh)
		x := math.Mod(float64(i)*311.3-float64(c.t)*38, sw+400) - 200
		gfx.StrokeLine(buf, float32(x), float32(y), float32(x+180+float64(i%3)*60), float32(y), 2, withAlpha(accent, 0x60), true)
	}
	if c.path != "-" {
		img := g.pics.get(c.path)
		head := g.pics.head(c.path)
		if c.bust {
			head = g.pics.foundHead(c.path)
			if len(c.box) == 3 {
				w, h := float64(img.Bounds().Dx()), float64(img.Bounds().Dy())
				sz := c.box[2] * w
				head = image.Rect(int(c.box[0]*w-sz/2), int(c.box[1]*h-sz/2), int(c.box[0]*w+sz/2), int(c.box[1]*h+sz/2))
			}
		}
		if c.u.Enemy || head.Empty() {
			b := img.Bounds()
			head = image.Rect(b.Min.X, b.Min.Y, b.Max.X, b.Min.Y+min(b.Dy(), b.Dx()))
		}
		scale := bh * 1.25 / float64(head.Dy())
		if c.u.Enemy {
			scale = bh * 1.1 / float64(head.Dy())
		}

		anchor, at := 0.42, bh/2
		if c.bust {
			scale = bh * 0.72 / float64(head.Dy())
			if len(c.box) == 3 {
				scale = bh * 0.55 / float64(head.Dy())
			}
			anchor, at = 0.5, bh/2-bh*0.06
		}

		slide := 1 - math.Pow(1-math.Min(1, p*4), 3)
		hx := sw*0.28 + (1-slide)*(-sw*0.5) + p*40
		op := &ebiten.DrawImageOptions{Filter: ebiten.FilterLinear}
		op.GeoM.Translate(-float64(head.Min.X+head.Max.X)/2, -float64(head.Min.Y)-float64(head.Dy())*anchor)
		op.GeoM.Scale(scale, scale)
		op.GeoM.Translate(hx, at)
		gfx.DrawImage(buf, img, op)
	}

	cop := &ebiten.DrawImageOptions{Filter: ebiten.FilterLinear}
	sub, ok := buf.SubImage(image.Rect(0, int((bh/2-bandH/2)*k), int(sw*k), int((bh/2+bandH/2)*k))).(*ebiten.Image)
	if !ok {
		return
	}
	cop.GeoM.Translate(0, (cy-bandH/2+slant/2-slant/2)*k)
	dst.DrawImage(sub, cop)

	ns := 1 - math.Pow(1-math.Min(1, math.Max(0, p*4-0.4)), 3)
	nx := sw*0.72 + (1-ns)*sw*0.4
	a := float32(open)
	gfx.DrawTextOutlineColor(dst, c.u.Name, nx, cy-54, sizeM, g.th.text, g.th.window, a)
	gfx.DrawTextOutlineColor(dst, c.name, nx, cy-12, sizeL*1.5, colorWhite, accent, a)
	if c.line != "" {
		gfx.DrawTextOutlineColor(dst, fmt.Sprintf(lang.T("「%s」"), c.line), nx, cy+50, sizeM, g.th.text, g.th.window, a)
	}
}

func (s *battleScene) drawIntro(g *Game, dst *ebiten.Image) {
	sw, sh := g.screen()
	if s.introT < introTicks {
		p := float64(s.introT) / introTicks
		a := uint8(255 * (1 - p))
		for i := range 9 {
			ang := float64(i)*0.7 + 0.3
			r := (1 - p) * sw * 0.7
			cx, cy := sw/2+math.Cos(ang)*r*0.3, sh/2+math.Sin(ang)*r*0.2
			l := 120 + 200*(1-p)
			gfx.StrokeLine(dst, float32(cx-math.Cos(ang)*l), float32(cy-math.Sin(ang)*l),
				float32(cx+math.Cos(ang)*l), float32(cy+math.Sin(ang)*l), float32(3+6*(1-p)), color.NRGBA{0xff, 0xff, 0xff, a}, true)
		}
	}
	if s.boss == "" || s.introT >= bossTicks {
		return
	}
	t := s.introT
	open := math.Min(1, float64(t)/12)
	if t > bossTicks-14 {
		open = float64(bossTicks-t) / 14
	}
	h := 120 * open
	y := sh*0.5 - h/2
	fill2 := withAlpha(g.th.window, uint8(230*open))
	gfx.FillRect(dst, 0, float32(y), float32(sw), float32(h), fill2, false)
	stripe := color.NRGBA{0xff, 0x8a, 0x9a, uint8(220 * open)}
	gfx.FillRect(dst, 0, float32(y), float32(sw), 4, stripe, false)
	gfx.FillRect(dst, 0, float32(y+h-4), float32(sw), 4, stripe, false)
	if open < 0.6 {
		return
	}
	slide := math.Min(1, float64(t-6)/16)
	x := sw/2 + (1-slide)*sw*0.6
	title := lang.T("迷宮の番人")
	if s.bossTitle != "" {
		title = s.bossTitle
	}
	gfx.DrawTextOutlineColor(dst, title, x, y+14, sizeS, stripe, g.th.window, 1)
	gfx.DrawTextOutlineColor(dst, s.boss, x, y+44, sizeL*1.4, colorWhite, g.th.window, 1)
}

func (s *battleScene) foeLayer(g *Game) *ebiten.Image {
	pw, ph := g.pixels()
	if s.layer == nil {
		s.layer = ebiten.NewImage(int(pw), int(ph))
	}
	return s.layer
}

func (s *battleScene) known(g *Game, u *battle.Unit) string {
	if u.Def == nil {
		return ""
	}
	var weak, strong []string
	for _, el := range g.db.Ordered("elements") {
		r, ok := u.Def.Resist[el]
		if !ok || g.st.Vars["known_"+u.EID+"_"+el] == nil {
			continue
		}
		name := g.db.Elements[el]
		if r > 1 {
			weak = append(weak, name)
		} else if r < 0.7 {
			strong = append(strong, name)
		}
	}
	out := ""
	if len(weak) > 0 {
		out = lang.T("弱") + " " + strings.Join(weak, " ")
	}
	if len(strong) > 0 {
		if out != "" {
			out += " / "
		}
		out += lang.T("耐") + " " + strings.Join(strong, " ")
	}
	return out
}

func (s *battleScene) stateTurns(g *Game, u *battle.Unit) string {
	st := s.shownStates(u)
	var names []string
	for _, bad := range []bool{true, false} {
		for _, id := range g.db.Ordered("states") {
			d := g.db.States[id]
			if d == nil || d.Bad != bad || st[id] <= 0 {
				continue
			}
			if n := st[id]; n < rpg.StateForever {
				names = append(names, fmt.Sprintf(lang.T("%s（のこり%d）"), d.Name, n))
			} else {
				names = append(names, d.Name)
			}
		}
	}
	return strings.Join(names, "　")
}

func (s *battleScene) stateNames(g *Game, u *battle.Unit) string {
	var names []string
	for _, id := range u.StateList() {
		if d := g.db.States[id]; d != nil {
			names = append(names, d.Name)
		}
	}
	return strings.Join(names, lang.T("・"))
}
