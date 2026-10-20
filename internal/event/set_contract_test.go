package event_test

import (
	"testing"

	"github.com/nao1215/gozen/internal/event"
	"github.com/nao1215/gozen/internal/testgame"
)

func TestSetAssignmentContract(t *testing.T) {
	for _, version := range []string{"", "schema_version: 1\nid: contract\n"} {
		for _, declaration := range []string{testgame.Game, testgame.DeclaredGame} {
			for _, written := range []string{
				`set: {var.b: "var.a + rand(100)", var.a: "var.b + rand(100)"}`,
				`set: {var.a: "var.b + rand(100)", var.b: "var.a + rand(100)"}`,
			} {
				db := testgame.Load(t, map[string]string{"game.yaml": version + declaration, "data/set.yaml": "events: {contract: {do: [{" + written + "}]}}"})
				for seed := uint64(1); seed <= 32; seed++ {
					st := testgame.State(db, seed)
					st.Vars["a"], st.Vars["b"] = 2.0, 7.0
					control := testgame.State(db, seed)
					// Evaluate a first regardless of the YAML order; b reads the updated a.
					wantA := 7.0 + float64(control.Rand.IntN(100))
					wantB := wantA + float64(control.Rand.IntN(100))
					r := event.New(db, st, &host{})
					r.Log = func(err error) { t.Error(err) }
					r.Start(db.Events["contract"].Do)
					if req := r.Next(); req != nil {
						t.Fatalf("予期しない待機 %v", req)
					}
					if st.Vars["a"] != wantA || st.Vars["b"] != wantB || st.Rand.Uint64() != control.Rand.Uint64() {
						t.Fatalf("代入順か乱数消費が変わった seed=%d 値=%v", seed, st.Vars)
					}
				}
			}
		}
	}
}

func TestDeclaredStateNamesGeneratedCommands(t *testing.T) {
	db := testgame.Load(t, map[string]string{"game.yaml": testgame.DeclaredGame, "data/sp.yaml": spData, "data/generated.yaml": `
events:
  generated:
    do:
      - sp_suggest: offer
      - sp_decline: offer
      - stone_suggest: stones
      - stone_decline: stones
`})
	st := testgame.State(db, 1)
	st.Join("monk")
	r := event.New(db, st, &host{})
	r.Log = func(err error) { t.Error(err) }
	r.Start(db.Events["generated"].Do)
	if req := r.Next(); req != nil {
		t.Fatalf("予期しない待機 %v", req)
	}
	if page, ok := st.Vars["offer_page1"].(string); !ok || page == "" {
		t.Fatalf("SP の頁を生成できない %v", st.Vars)
	}
	if _, ok := st.Vars["offer_no"].(string); !ok {
		t.Fatalf("SP の生成名 %v", st.Vars)
	}
	if _, ok := st.Vars["stones_no"].(string); !ok {
		t.Fatalf("石の生成名 %v", st.Vars)
	}
}
