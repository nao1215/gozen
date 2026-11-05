package gamedata

import (
	"fmt"
	"slices"

	"github.com/nao1215/gozen/internal/diag"
)

// scheduleDependencies checks the declared ordering without trying to solve
// arbitrary expressions. The declaration order is the stable conflict priority.
func (c *checker) scheduleDependencies(w string, p *Planning) {
	byID := map[string]Reservation{}
	for _, r := range p.Schedule {
		byID[r.ID] = r
	}
	visiting, visited := map[string]bool{}, map[string]bool{}
	var visit func(string)
	visit = func(id string) {
		if visiting[id] {
			c.errorf(diag.Shape, w+".schedule", "reservation dependency cycle at %q", id)
			return
		}
		if visited[id] {
			return
		}
		visiting[id] = true
		for _, dep := range byID[id].After {
			if _, ok := byID[dep]; ok {
				visit(dep)
			}
		}
		visiting[id] = false
		visited[id] = true
	}
	for i, r := range p.Schedule {
		at := fmt.Sprintf("%s.schedule[%d]", w, i)
		visit(r.ID)
		for _, dep := range r.After {
			before, ok := byID[dep]
			if !ok {
				c.errorf(diag.UnknownReference, at+".after", "unknown reservation %q", dep)
				continue
			}
			if r.Date != "" && before.Date != "" && (before.Date > r.Date || before.Date == r.Date && (slices.Index(p.Calendar.Phases, before.Phase) > slices.Index(p.Calendar.Phases, r.Phase) || r.Exclusive != "" && r.Exclusive == before.Exclusive)) {
				c.errorf(diag.Shape, at+".after", "reservation %q cannot precede this fixed date/phase or exclusive group", dep)
			}
		}
		if r.Date != "" && (r.Date < p.Calendar.Start || r.Date > p.Calendar.End) {
			c.errorf(diag.OutOfRange, at+".date", "reservation is outside the calendar")
		}
	}
}
