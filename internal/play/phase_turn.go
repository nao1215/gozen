package play

import (
	"image/color"
	"maps"
	"math"
	"slices"
	"strconv"

	"github.com/hajimehoshi/ebiten/v2"

	data "github.com/nao1215/gozen/internal/model/gamedata"
	"github.com/nao1215/gozen/internal/screen/gfx"
)

// The time of day is seen, not read: the calendar's looks give each phase a
// clock beside the date, a light washed over workspace surfaces, and a turn
// card when play moves into it (planning.calendar.looks).

const (
	turnIn   = 14
	turnHold = 56
	turnOut  = 16
	// turnSkip is how long the card stays before a press may skip it.
	turnSkip = 20
)

// phaseTurn is a turn card on the way: the look it shows and its age in ticks.
type phaseTurn struct {
	name string
	look data.PhaseLook
	note string
	age  int
}

// phaseLook is the look of the planning phase now ("closed" on a closed day).
func (g *Game) phaseLook() (string, data.PhaseLook, bool) {
	if g.st == nil || g.st.Planning == nil || g.db.Game.Planning == nil {
		return "", data.PhaseLook{}, false
	}
	c := g.db.Game.Planning.Calendar
	if len(c.Looks) == 0 {
		return "", data.PhaseLook{}, false
	}
	name := ""
	if g.st.PlanningClosedDay() {
		name = "closed"
	} else if p := g.st.Planning.Phase; p >= 0 && p < len(c.Phases) {
		name = c.Phases[p]
	}
	look, ok := c.Looks[name]
	return name, look, ok
}

// turnKey names the place in the calendar the player last saw.
func (g *Game) turnKey() string {
	if g.st == nil || g.st.Planning == nil {
		return ""
	}
	if g.st.Planning.Finished {
		return "finished"
	}
	return g.st.Planning.Date + "/" + strconv.Itoa(g.st.Planning.Phase)
}

// stepTurn starts a turn card when the calendar moved since the last tick and
// ages the card on the way. It reports whether the card holds the game, the
// way a fade does. The first look after a start or a load shows no card.
func (g *Game) stepTurn() bool {
	key := g.turnKey()
	if key != g.turnSeen {
		first := g.turnSeen == ""
		g.turnSeen = key
		name, look, ok := g.phaseLook()
		if !first && ok && key != "finished" {
			g.emit("turn", traceLine{"phase": name, "date": g.st.Planning.Date})
			if (look.Card == nil || *look.Card) && !g.opts.headless {
				note := ""
				if look.Note != "" {
					note = data.Render(g.db.Tr(look.Note), g.st.Env())
				}
				g.turn = &phaseTurn{name: name, look: look, note: note}
			}
		}
	}
	t := g.turn
	if t == nil {
		return false
	}
	t.age++
	if t.age > turnSkip && t.age < turnIn+turnHold && g.in.advance() {
		t.age = turnIn + turnHold
	}
	if t.age >= turnIn+turnHold+turnOut {
		g.turn = nil
	}
	return true
}

// turnAlpha is how much of the card shows at its age.
func (t *phaseTurn) alpha() float64 {
	switch {
	case t.age < turnIn:
		return float64(t.age) / turnIn
	case t.age >= turnIn+turnHold:
		return math.Max(0, 1-float64(t.age-turnIn-turnHold)/turnOut)
	}
	return 1
}

// drawTurn covers the screen with the light of the new phase and its name,
// clock and note in the middle, under the date.
func (g *Game) drawTurn(dst *ebiten.Image) {
	t := g.turn
	if t == nil {
		return
	}
	a := t.alpha()
	base := color.NRGBA{18, 22, 30, 255}
	if t.look.Color.Valid {
		c := t.look.Color.NRGBA
		// The card is the phase's light seen dimmed, so white text stays legible.
		base = color.NRGBA{uint8(float64(c.R) * .42), uint8(float64(c.G) * .42), uint8(float64(c.B) * .42), 255}
	}
	fill(dst, withAlpha(base, uint8(235*a)))
	sw, sh := g.screen()
	label := t.look.Label
	if label == "" {
		label = g.phaseLabel(t.name)
	}
	ink := withAlpha(color.NRGBA{255, 255, 255, 255}, uint8(255*a))
	soft := withAlpha(color.NRGBA{235, 238, 244, 255}, uint8(220*a))
	big := sizeXL * 1.8
	y := sh/2 - big
	if date := g.turnDate(); date != "" {
		g.text(dst, date, sw/2-gfx.TextWidth(date, sizeL)/2, y-sizeL*1.8, sizeL, soft)
	}
	g.text(dst, label, sw/2-gfx.TextWidth(label, big)/2, y, big, ink)
	y += big * 1.35
	if t.look.Color.Valid {
		w := 160.0
		gfx.FillRoundRect(dst, float32(sw/2-w/2), float32(y-10), float32(w), 4, 2, withAlpha(t.look.Color.NRGBA, uint8(255*a)))
	}
	if t.look.Time != "" {
		g.text(dst, t.look.Time, sw/2-gfx.TextWidth(t.look.Time, sizeXL)/2, y, sizeXL, ink)
		y += sizeXL * 1.4
	}
	if t.note != "" {
		g.text(dst, t.note, sw/2-gfx.TextWidth(t.note, sizeM)/2, y, sizeM, soft)
	}
}

// turnDate is the date of the card as the planning views write it.
func (g *Game) turnDate() string {
	for _, id := range slices.Sorted(maps.Keys(g.db.Game.Screens)) {
		if s := g.db.Game.Screens[id]; s != nil && s.View.DateFormat != "" {
			return planningDate(g.st.Planning.Date, "", s.View)
		}
	}
	return g.st.Planning.Date
}

// phaseLabel is the shown name of a phase: the first planning view that
// translates it, else the name itself.
func (g *Game) phaseLabel(name string) string {
	for _, id := range slices.Sorted(maps.Keys(g.db.Game.Screens)) {
		if s := g.db.Game.Screens[id]; s != nil {
			if label := s.View.Phases[name]; label != "" {
				return label
			}
		}
	}
	return name
}

// drawPhaseWash covers a workspace surface with the light of the phase.
func (g *Game) drawPhaseWash(dst *ebiten.Image) {
	_, look, ok := g.phaseLook()
	if !ok || look.Wash <= 0 || !look.Color.Valid {
		return
	}
	fill(dst, withAlpha(look.Color.NRGBA, uint8(255*math.Min(data.MaxPhaseWash, look.Wash))))
}

// phaseClock is the clock text of the phase now, for the date in a header.
func (g *Game) phaseClock() (string, color.NRGBA, bool) {
	_, look, ok := g.phaseLook()
	if !ok {
		return "", color.NRGBA{}, false
	}
	return look.Time, look.Color.NRGBA, look.Color.Valid
}

// phaseDate follows a header's date and phase with the clock of the phase.
func (g *Game) phaseDate(date string) string {
	if clock, _, _ := g.phaseClock(); clock != "" {
		return date + " " + clock
	}
	return date
}

// drawPhaseMark draws a small round mark of the phase's light where a
// header's date starts and returns the width it takes.
func (g *Game) drawPhaseMark(dst *ebiten.Image, x, y, size float64) float64 {
	_, light, lit := g.phaseClock()
	if !lit {
		return 0
	}
	r := size * .36
	gfx.FillCircle(dst, float32(x+r), float32(y+size*.62), float32(r), light, true)
	return r*2 + 8
}
