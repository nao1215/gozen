package play

import (
	"testing"

	data "github.com/nao1215/gozen/internal/layer2/gamedata"
)

func TestCalendarHiddenClosedReservation(t *testing.T) {
	p := &data.Planning{Calendar: data.Calendar{WeekdaysOff: []int{0, 6}}, Schedule: []data.Reservation{{Date: "2030-08-03", Label: "Visitor", HideClosed: true}}}
	for _, hidden := range []bool{true, false} {
		p.Schedule[0].HideClosed = hidden
		for _, c := range calendarCells(p, data.PlanningView{}, "2030-08-01", 0, 0, 700, 600) {
			if c.date == "2030-08-03" && (len(c.labels) == 0) != hidden {
				t.Fatalf("hidden=%v labels=%v", hidden, c.labels)
			}
		}
	}
}
