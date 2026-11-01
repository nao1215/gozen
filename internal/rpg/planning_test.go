package rpg_test

import (
	"bytes"
	"testing"

	data "github.com/nao1215/gozen/internal/layer2/gamedata"
	"github.com/nao1215/gozen/internal/testgame"
)

func TestPlanningCalendarCounts(t *testing.T) {
	db := testgame.Load(t, nil)
	db.Game.Planning = &data.Planning{Calendar: data.Calendar{Start: "2030-07-31", End: "2030-08-04", Phases: []string{"day"}, WeekdaysOff: []int{0, 6}, Closed: []string{"2030-08-01", "2030-08-03"}}}
	st := testgame.State(db, 17)
	before, err := st.Save()
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		source string
		want   float64
	}{
		{`calendar_days("2030-07-31", "2030-07-31")`, 0},
		{`calendar_days("2030-07-31", "2030-08-04")`, 4},
		{`calendar_days("2030-08-04", "2030-07-31")`, -4},
		{`calendar_days("2028-02-28", "2028-03-01")`, 2},
		{`calendar_workdays("2030-07-31", "2030-08-04")`, 2},
		{`calendar_workdays("2030-07-31", "2030-08-06")`, 4},
		{`calendar_workdays("0001-01-01", "9999-12-31")`, 2608614},
		{`calendar_workdays("2030-08-02", "2030-08-02")`, 1},
		{`calendar_workdays("2030-08-01", "2030-08-01")`, 0},
		{`calendar_workdays("2030-08-04", "2030-07-31")`, 0},
		{`calendar_workdays("2030-08-05", "2030-08-11")`, 5},
		{`calendar_days("0001-01-01", "9999-12-31")`, 3652058},
	} {
		got, err := mustExpr(t, tc.source).Eval(st.Env())
		if err != nil || got != tc.want {
			t.Errorf("%s = %v, %v; want %v", tc.source, got, err, tc.want)
		}
	}
	for _, source := range []string{`calendar_days("2030-02-30", "2030-03-01")`, `calendar_days(0, "2030-03-01")`, `calendar_days("2030-03-01")`, `calendar_workdays("2030-03-01", false)`, `calendar_workdays("2030-03-01", "2030-3-02")`} {
		if _, err := mustExpr(t, source).Eval(st.Env()); err == nil {
			t.Errorf("%s accepted", source)
		}
	}
	after, err := st.Save()
	if err != nil || !bytes.Equal(before, after) {
		t.Fatalf("calendar reads changed saved state: %v", err)
	}
	// Duplicate closures and a closure already on a weekend count only once.
	db.Game.Planning.Calendar.WeekdaysOff = []int{0, 6, 6}
	db.Game.Planning.Calendar.Closed = []string{"2030-08-01", "2030-08-01", "2030-08-03"}
	got, err := mustExpr(t, `calendar_workdays("2030-07-31", "2030-08-04")`).Eval(st.Env())
	if err != nil || got != 2.0 {
		t.Fatalf("duplicate closures: %v, %v", got, err)
	}
}
