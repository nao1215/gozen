package designeval

import (
	"math"
	"slices"
	"testing"
)

func near(a, b float64) bool { return math.Abs(a-b) < 1e-9 }

func TestSummarizeMeasuresGapsAndTheWaitForARelease(t *testing.T) {
	r := &Run{
		Persona: Persona{ID: "p", Name: "P"},
		End:     600, Cleared: true, ClearT: 480, Days: 2, Deepest: 3, Level: 7, Battles: 9, Lost: 1, Retries: 1, Wipes: 1, Stranded: 2,
		ReleaseSec: 30,
		Segments: []Segment{
			{From: 0, To: 100, Place: "Camp"},
			{From: 100, To: 300, Place: "Cave", Dungeon: true, Floor: 1},
			{From: 300, To: 600, Place: "Camp"},
		},
		Events: []Event{
			// Out of order on purpose: the summary sorts by time.
			{T: 300, Day: 2, Kind: "gift", Reward: true, Who: "Mote", Flow: "gift"},
			{T: 100, Day: 1, Kind: "hint", Expect: true},
			{T: 60, Day: 1, Kind: "chest", Reward: true},
			{T: 200, Day: 1, Kind: "hint", Expect: true},
			{T: 450, Day: 2, Kind: "gift", Reward: true, Who: "Mote", Flow: "other"},
			{T: 470, Day: 2, Kind: "hint", Expect: true},
			// After the clear: not counted.
			{T: 550, Day: 2, Kind: "chest", Reward: true},
		},
	}
	s := Summarize(r, "gift")
	if s.ID != "p" || !s.Cleared || !near(s.ClearMin, 8) || !near(s.EndMin, 10) || !near(s.ReleaseMin, 0.5) {
		t.Fatalf("header: %+v", s)
	}
	if s.Days != 2 || s.Deepest != 3 || s.Level != 7 || s.Battles != 9 || s.Lost != 1 || s.Retries != 1 || s.Wipes != 1 || s.Stranded != 2 {
		t.Fatalf("counts: %+v", s)
	}
	if s.Rewards["gift"] != 2 || s.Rewards["chest"] != 1 || s.Expects != 3 {
		t.Fatalf("rewards %v expects %d", s.Rewards, s.Expects)
	}
	if s.ReleaseWho["Mote"] != 2 || s.ReleaseFlow["gift"] != 1 || s.ReleaseFlow["other"] != 1 {
		t.Fatalf("who/flow: %v %v", s.ReleaseWho, s.ReleaseFlow)
	}
	// Promises at 100 and 200 were paid at 300 (the wait is counted from the first); the one at 470 was not.
	if s.Release.N != 1 || !near(s.Release.Avg, 200.0/60) || !near(s.Release.Max, 200.0/60) || !near(s.Release.From, 100.0/60) {
		t.Fatalf("release wait: %+v", s.Release)
	}
	if s.Release.At != "Cave" || s.Release.Day != 1 {
		t.Fatalf("the wait was mostly spent in the cave on day 1: %+v", s.Release)
	}
	if s.Unreleased != 1 {
		t.Fatalf("promises left after the last payoff: %d", s.Unreleased)
	}
	if !near(s.Minutes["town"], 100.0/60+180.0/60) || !near(s.Minutes["dungeon"], 200.0/60) {
		t.Fatalf("minutes: %v", s.Minutes)
	}
	// Reward times 60, 300, 450 up to the clear at 480: points 0, 60, 300, 450, 480.
	if s.Gap.N != 4 || !near(s.Gap.Avg, 480.0/4/60) || !near(s.Gap.Max, 240.0/60) || !near(s.Gap.From, 1) {
		t.Fatalf("gap: %+v", s.Gap)
	}
	if len(s.Gap.Top) != 3 || !near(s.Gap.Top[0].Len, 4) || s.Gap.Top[0].At != "Cave" {
		t.Fatalf("top spans: %+v", s.Gap.Top)
	}
	if k := s.KindGap["gift"]; k.N != 3 {
		t.Fatalf("gift gap: %+v", k)
	}
	if s.Dry.N == 0 {
		t.Fatal("dry spell not measured")
	}
}

func TestSummarizeWithoutEventsOrClear(t *testing.T) {
	s := Summarize(&Run{Persona: Persona{ID: "q"}, End: 120}, "x")
	if s.Cleared || s.Gap.N != 1 || !near(s.Gap.Max, 2) || s.Release.N != 0 || s.Unreleased != 0 {
		t.Fatalf("%+v", s)
	}
	if len(s.Gap.Top) != 1 {
		t.Fatalf("one span only: %+v", s.Gap.Top)
	}
	s = Summarize(&Run{Persona: Persona{ID: "q"}}, "x")
	if s.Gap.N != 0 || s.Dry.N != 0 {
		t.Fatalf("an empty run has no gaps: %+v", s)
	}
}

func TestGapsIgnoreEventsLessThanASecondApart(t *testing.T) {
	g := gaps([]float64{10, 10.5, 10.9, 70}, 70, &Run{})
	// Points: 0, 10, 70 (10.5 and 10.9 fold into 10).
	if g.N != 2 || !near(g.Max, 1) || !near(g.From, 10.0/60) {
		t.Fatalf("%+v", g)
	}
	unsorted := []float64{70, 10}
	gaps(unsorted, 70, &Run{})
	if !slices.IsSorted(unsorted) {
		t.Error("gaps must sort its input times")
	}
}

func TestPlaceInPicksTheLongestStayAndTheDay(t *testing.T) {
	r := &Run{
		Segments: []Segment{{From: 0, To: 50, Place: "A"}, {From: 50, To: 200, Place: "B"}, {From: 200, To: 260, Place: "A"}},
		Events:   []Event{{T: 0, Day: 1}, {T: 100, Day: 2}, {T: 250, Day: 3}},
	}
	if p, d := placeIn(r, 0, 260); p != "B" || d != 1 {
		t.Errorf("whole run: %q day %d", p, d)
	}
	if p, d := placeIn(r, 120, 260); p != "B" || d != 2 {
		t.Errorf("from 120: %q day %d", p, d)
	}
	if p, d := placeIn(r, 240, 260); p != "A" || d != 2 {
		t.Errorf("from 240: %q day %d", p, d)
	}
	if p, d := placeIn(r, 900, 950); p != "" || d != 3 {
		t.Errorf("outside any segment: %q day %d", p, d)
	}
}

func TestDuelAveragesMarkWhatLeavesTheTargets(t *testing.T) {
	c := &Config{Duels: Duels{
		First: []float64{0.4, 0.6}, Later: 0.5, Personas: []string{"a", "b"},
		List: []Duel{{Troop: "x"}, {Troop: "y"}, {Troop: "z"}},
	}}
	sums := []Summary{
		{ID: "a", Duels: []*DuelRec{
			{Name: "X", Opened: true, First: 0.9, Again: 0.7, Grown: true, Later: 0.3},
			{Name: "Y", Opened: true, First: 0.5, Again: 0.5},
			{Name: "Z"},
		}},
		{ID: "b", Duels: []*DuelRec{
			{Name: "X", Opened: true, First: 0.7, Again: 0.5, Grown: true, Later: 0.5},
			{Name: "Y", Opened: true, First: 0.5, Again: 0.5},
			{Name: "Z"},
		}},
		// Not among the personas the duels are measured with.
		{ID: "c", Duels: []*DuelRec{{Name: "X", Opened: true, First: 0, Again: 0}}},
	}
	got := DuelAverages(c, sums)
	if len(got) != 3 {
		t.Fatalf("%+v", got)
	}
	x := got[0]
	if x.Name != "X" || !near(x.First, 0.8) || !near(x.Again, 0.6) || !near(x.Later, 0.4) || x.NFirst != 2 || x.NLater != 2 {
		t.Fatalf("X: %+v", x)
	}
	if !slices.Equal(x.Off, []string{"first", "later"}) {
		t.Fatalf("X is off the targets in its first fight and the later one: %v", x.Off)
	}
	if y := got[1]; len(y.Off) != 0 || !near(y.First, 0.5) {
		t.Fatalf("Y: %+v", y)
	}
	if z := got[2]; z.NFirst != 0 || z.Name != "Z" || len(z.Off) != 0 {
		t.Fatalf("Z: %+v", z)
	}
	// With no persona list every persona counts, and a low rematch is flagged.
	c.Duels.Personas = nil
	c.Duels.First = nil
	c.Duels.Later = 0.9
	x = DuelAverages(c, sums)[0]
	if x.NFirst != 3 || !slices.Contains(x.Off, "again") {
		t.Fatalf("all personas count: %+v", x)
	}
}
