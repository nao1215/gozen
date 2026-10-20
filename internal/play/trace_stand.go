package play

import (
	"fmt"
	"math"
	"strings"

	data "github.com/nao1215/gozen/internal/layer2/gamedata"
)

type standFrame struct {
	speaker, face string
	standing      bool
	figures       []standFig
}

type standFig struct {
	ID       string  `json:"id"`
	Face     string  `json:"face"`
	Side     string  `json:"side"`
	X        float64 `json:"x"`
	Alpha    float64 `json:"alpha"`
	Speaking bool    `json:"speaking,omitempty"`
	Leaving  bool    `json:"leaving,omitempty"`
}

const standStable = 4

func (g *Game) traceStandSpeaker(face string, standing bool, key string) {
	if g.tr == nil || g.tr.stand == nil {
		return
	}
	g.tr.stand.face, g.tr.stand.standing = face, standing
	if face != "" {
		g.tr.stand.speaker = key
	}
}

func (g *Game) traceStandFigures(figs []stageFigure) {
	if g.tr == nil || g.tr.stand == nil {
		return
	}
	sw, _ := g.screen()
	for _, f := range figs {
		a := float64(f.alpha * g.stageK)
		if a < 0.05 {
			continue
		}
		x := f.x / sw
		side := "left"
		if x > 0.5 {
			side = "right"
		}
		g.tr.stand.figures = append(g.tr.stand.figures, standFig{
			ID: f.id, Face: f.face, Side: side,
			X: math.Round(x*100) / 100, Alpha: math.Round(a*10) / 10, Speaking: f.dim == 0 && !f.leaving, Leaving: f.leaving,
		})
	}
}

func (g *Game) isBackground(file string) bool {
	if file == "" {
		return false
	}
	for _, l := range g.db.Locations {
		if l.Background.File == file {
			return true
		}
	}
	return false
}

func (g *Game) traceStandEnd() {
	tr := g.tr
	f := tr.stand
	tr.stand = nil
	if f == nil {
		return
	}
	hides := g.cg.File != "" && !g.cg.Faces
	in := 1.0
	if fd := g.cg.FadeTicks(); fd > 0 {
		in = math.Min(1, float64(g.cgAge)/float64(fd))
	}
	over := false
	if hides && in >= 0.5 {
		for _, fg := range f.figures {
			if fg.Alpha >= 0.2 {
				over = true
			}
		}
	}
	loc := ""
	if g.st != nil {
		loc = g.st.Location
	}
	idle := !g.talk.active && len(f.figures) == 0

	fading := g.cgOldT > 0 || (g.cg.File != "" && g.cgAge < g.cg.FadeTicks())
	var b strings.Builder
	fmt.Fprintf(&b, "%s|%s|%v|%s|%s|%v|%v|%v|%d|", f.speaker, f.face, f.standing, loc, g.cg.File, hides, over, fading, g.stageGen)
	for _, fg := range f.figures {
		fmt.Fprintf(&b, "%s,%s,%.2f,%.1f,%v,%v;", fg.ID, fg.Face, fg.X, fg.Alpha, fg.Speaking, fg.Leaving)
	}
	key := b.String()
	if idle {
		key = "idle"
	}
	if key != tr.standPending {
		tr.standPending, tr.standN, tr.standAt = key, 0, g.tick
	}
	tr.standN++

	if key == tr.standLast || ((tr.standN < 2 || g.tick-tr.standAt < standStable) && !over) {
		return
	}
	tr.standLast = key
	if idle {
		g.emit("stand", traceLine{"since": tr.standAt, "idle": true})
		return
	}
	l := traceLine{"since": tr.standAt, "set": g.stageGen, "hides": hides}
	if f.speaker != "" {
		l["speaker"], l["face"], l["standing"] = f.speaker, f.face, f.standing
	}
	if len(f.figures) > 0 {
		l["figures"] = f.figures
	}
	if loc != "" {
		l["location"] = loc
	}
	if g.cg.File != "" {
		l["cg"] = g.cg.File
		l["cg_page"] = data.PageBase(g.cg.File)
		if hides && g.isBackground(g.cg.File) {
			l["cg_bg"] = true
		}
	}
	if over {
		l["over"] = true
	}
	if fading {
		l["fading"] = true
	}
	g.emit("stand", l)
}
