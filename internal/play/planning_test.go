package play

import (
	"image"
	"math"
	"math/rand/v2"
	"slices"
	"testing"

	data "github.com/nao1215/gozen/internal/layer2/gamedata"
	"github.com/nao1215/gozen/internal/testgame"
)

func TestNativeViewport(t *testing.T) {
	for _, tc := range []struct {
		w, h, dsf float64
		pw, ph    int
		k         float64
	}{{1920, 1080, 1, 1920, 1080, 1.5}, {3840, 2160, 1, 3840, 2160, 3}, {2560, 1080, 1, 1920, 1080, 1.5}, {1280, 720, 1.5, 1920, 1080, 1.5}, {1920, 1080, 2, 3840, 2160, 3}} {
		v := nativeView(tc.w, tc.h, tc.dsf)
		if v.w != tc.pw || v.h != tc.ph || v.ui != tc.k {
			t.Fatalf("%+v: %+v", tc, v)
		}
	}
	if nativeView(0, 720, 1) != (gameView{}) {
		t.Fatal("invalid window accepted")
	}
}

func TestPlanningCellsStayInsideAndDisjoint(t *testing.T) {
	for _, count := range []int{1, 8, 9, 16} {
		for _, cols := range []int{1, 4} {
			cells := planningCells(24, 148, 1232, 452, count, cols)
			for i, r := range cells {
				if r.Min.X < 24 || r.Min.Y < 148 || r.Max.X > 1256 || r.Max.Y > 600 || r.Empty() {
					t.Fatalf("outside: %v", r)
				}
				for _, other := range cells[:i] {
					if r.Overlaps(other) {
						t.Fatalf("overlap: %v %v", r, other)
					}
				}
			}
		}
	}
	// Eight member rows reserve a gauge's text, bar and threshold labels.
	for _, r := range planningCells(24, 148, 1232, 502, 8, 1) {
		if r.Dy() < 50 {
			t.Fatalf("member row cannot fit labels: %v", r)
		}
	}
}

func TestPlanningSeedDerivation(t *testing.T) {
	g := &Game{opts: Options{Seed: 17}}
	baseline := g.newPCG().Uint64()
	g.db = &data.DB{}
	xor := uint64(12345)
	g.db.Game.Planning = &data.Planning{SeedXor: &xor}
	want := rand.NewPCG(17, 17^xor)
	got := g.newPCG()
	for range 20 {
		if got.Uint64() != want.Uint64() {
			t.Fatal("imported stream diverged")
		}
	}
	if g.newPCG().Uint64() == baseline {
		t.Fatal("stream setting ignored")
	}
}

func TestEnsembleCellsLeaveDialogueClear(t *testing.T) {
	// The box dialogue starts at 470; the view reserves its heading and margins.
	cells := planningCells(36, 76, 1208, 344, 8, 4)
	for _, r := range cells {
		if r.Max.Y >= 446 || r.Dy() < 100 {
			t.Fatalf("participant overlaps dialogue or loses name: %v", r)
		}
	}
}

func TestPlanningAuthoredDateLabels(t *testing.T) {
	v := data.PlanningView{DateFormat: "1/2({weekday})", Weekdays: []string{"Su", "Mo", "Tu", "We", "Th", "Fr", "Sa"}, Phases: map[string]string{"dawn": "Early"}}
	if got := planningDate("2030-04-01", "dawn", v); got != "4/1(Mo) Early" {
		t.Fatal(got)
	}
	if got := planningDate("2030-04-01", "", v); got != "4/1(Mo)" {
		t.Fatal(got)
	}
	if got := planningEventLabel(data.PlanningView{}); got != "events" {
		t.Fatal(got)
	}
}

func TestResidentVisibilityUsesPublicState(t *testing.T) {
	db := testgame.Load(t, nil)
	db.Game.Planning = &data.Planning{Entities: map[string]data.Entity{"potter": {}, "weaver": {}}}
	g := &Game{db: db, st: testgame.State(db, 1)}
	g.st.Planning.Entities["potter"].Observations["shown"] = true
	g.st.Planning.Entities["weaver"].Attributes["secret"] = true
	db.Game.Planning.Attributes = map[string]data.Attribute{"secret": {Hidden: true}}
	if got := g.visibleResidents(data.PlanningView{Visible: "secret"}); len(got) != 0 {
		t.Fatal(got)
	}
	v := data.PlanningView{Order: []string{"weaver", "potter"}, Visible: "shown"}
	if got := g.visibleResidents(v); !slices.Equal(got, []string{"potter"}) {
		t.Fatal(got)
	}
	g.st.Planning.Entities["weaver"].Observations["shown"] = true
	if got := g.visibleResidents(v); !slices.Equal(got, v.Order) {
		t.Fatal(got)
	}
	delete(g.st.Planning.Entities["potter"].Observations, "shown")
	if got := g.visibleResidents(v); !slices.Equal(got, []string{"weaver"}) {
		t.Fatal(got)
	}
	v.Visible = ""
	if got := g.visibleResidents(v); !slices.Equal(got, v.Order) {
		t.Fatal(got)
	}
}

func TestPortraitTransform(t *testing.T) {
	bounds := image.Rect(0, 0, 1000, 2000)
	for _, face := range []image.Rectangle{image.Rect(400, 80, 600, 280), image.Rect(50, 900, 350, 1200), image.Rect(800, 0, 1000, 200)} {
		s, x, y := portraitTransform(bounds, face, 20, 40, 300, 60)
		if math.Abs(float64(face.Min.Y)*s+y-40) > 0.001 || math.Abs(float64(face.Dy())*s-60) > 0.001 || math.Abs(float64(face.Min.X+face.Dx()/2)*s+x-170) > 0.001 {
			t.Fatalf("face %v: %v %v %v", face, s, x, y)
		}
	}
	for _, face := range []image.Rectangle{{}, image.Rect(2000, 0, 2100, 100)} {
		s, _, _ := portraitTransform(bounds, face, 0, 0, 300, 60)
		if s <= 0 || math.IsNaN(s) {
			t.Fatal("invalid fallback")
		}
	}
}

func TestEnsembleRegionsKeepFacesAboveSummary(t *testing.T) {
	for _, h := range []float64{120, 160, 240, 400} {
		face, summary := ensembleHeights(h, 2)
		if face < 0 || summary < min(48, h-28) || math.Abs(face+summary+28-h) > 0.001 {
			t.Fatalf("height %v: face %v summary %v", h, face, summary)
		}
		face, summary = ensembleHeights(h, 0)
		if face != h-28 || summary != 0 {
			t.Fatal("empty summary reserves portrait space")
		}
	}
}
