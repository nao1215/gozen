package headless_test

import (
	"errors"
	"fmt"
	"testing"

	"github.com/nao1215/gozen/headless"
	"github.com/nao1215/gozen/internal/layer1/optional"
	"github.com/nao1215/gozen/internal/testgame"
	"github.com/nao1215/gozen/rpg"
)

// Explore は選択肢の全部の項目と、戦闘の勝ち・負け・逃げを枝として試し、状態の検査を道ごとに走らせる。
func TestExploreTriesEveryBranch(t *testing.T) {
	db := testgame.Load(t, map[string]string{"data/z.yaml": `
events:
  root:
    do:
      - choice:
          - label: A
            do: [{set: {var.a: 1}}]
          - label: B
            do:
              - choice:
                  - {label: B1, do: [{set: {var.b: 1}}]}
                  - {label: B2, do: [{set: {var.b: 2}}, {gold: "-1000"}]}
          - label: C
            do:
              - battle: one
                on_win: [{set: {var.won: 1}}]
                on_lose: [{set: {var.lost: 1}}]
`})
	st := testgame.State(db, 1)
	paths := headless.Explore(db, st, db.Events["root"].Do, headless.Config{
		Check: func(st *rpg.State) error {
			if st.Gold < 0 {
				return errors.New("negative gold")
			}
			return nil
		},
	})
	seen := map[string]bool{}
	for _, p := range paths {
		for _, k := range []string{"a", "b", "won", "lost"} {
			if v := optional.Value[float64](p.St.Vars[k]); v > 0 {
				seen[fmt.Sprintf("%s=%g", k, v)] = true
			}
		}
		for _, err := range p.Errs {
			t.Errorf("%v", err)
		}
	}
	for _, want := range []string{"a=1", "b=1", "b=2", "won=1", "lost=1"} {
		if !seen[want] {
			t.Errorf("%s was never reached (%d paths): %v", want, len(paths), seen)
		}
	}
	if st.Vars["a"] != nil {
		t.Error("Explore changed the state it was given")
	}
}

// Explore は終わらないイベント・ない場所への移動・壊れた呼び出しを、道の誤りとして返す（止まらない）。
func TestExploreReportsBrokenEvents(t *testing.T) {
	db := testgame.Load(t, map[string]string{"data/z.yaml": `
events:
  forever:
    trigger: auto
    location: vault
    do: [{set: {var.n: "var.n + 1"}}]
  root:
    do: [{move: vault}]
`})
	st := testgame.State(db, 1)
	paths := headless.Explore(db, st, db.Events["root"].Do, headless.Config{})
	if len(paths) != 1 || len(paths[0].Errs) == 0 {
		t.Fatalf("an auto event that never stops was not reported: %+v", paths)
	}
}

// 文と項目が同じでも、別の場所の選択肢はそれぞれ枝分かれさせる（後の方の 2 つめを試さずに済ませない）。
func TestExploreTellsTheSameQuestionInTwoPlaces(t *testing.T) {
	db := testgame.Load(t, map[string]string{"data/z.yaml": `
events:
  root:
    do:
      - choice:
          - label: Yes
            do: [{set: {var.a: 1}}]
          - label: No
            do: [{set: {var.a: 2}}]
      - choice:
          - label: Yes
            do: [{set: {var.b: 1}}]
          - label: No
            do: [{set: {var.b: 2}}]
`})
	st := testgame.State(db, 1)
	paths := headless.Explore(db, st, db.Events["root"].Do, headless.Config{})
	seen := map[string]bool{}
	for _, p := range paths {
		for _, k := range []string{"a", "b"} {
			if v := optional.Value[float64](p.St.Vars[k]); v > 0 {
				seen[fmt.Sprintf("%s=%g", k, v)] = true
			}
		}
	}
	for _, want := range []string{"a=1", "a=2", "b=1", "b=2"} {
		if !seen[want] {
			t.Errorf("%s was never reached (%d paths): %v", want, len(paths), seen)
		}
	}
}
