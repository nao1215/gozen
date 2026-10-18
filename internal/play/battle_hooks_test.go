package play

import (
	"testing"

	"github.com/nao1215/gozen/battle"
	"github.com/nao1215/gozen/event"
	data "github.com/nao1215/gozen/internal/layer2/gamedata"
	"github.com/nao1215/gozen/internal/testgame"
)

func TestBattleRetryKeepsPreparedState(t *testing.T) {
	g := openGame(t, Options{Seed: 31, Location: "camp"})
	g.db.Game.Battle.Retry = true
	g.db.Game.Battle.Before = "prepare"
	g.db.Events["prepare"] = &data.Event{Do: []data.Command{{SetV: map[string]data.Expr{"var.energy": {Expr: testgame.Expr("var.energy - 1")}}}}}
	g.st.Vars["energy"] = 5.
	g.events.Start([]data.Command{{Battle: "one"}})
	req := g.events.Next()
	if _, ok := req.(event.Battle); !ok {
		t.Fatalf("request %T", req)
	}
	g.begin(req)
	for range 2 {
		s := g.currentBattle()
		s.b.Result, s.endT = battle.Lost, 10
		s.result(g)
		q, ok := g.stack[len(g.stack)-1].(*confirm)
		if !ok {
			t.Fatalf("retry request %T", g.stack[len(g.stack)-1])
		}
		for range 6 {
			q.update(g)
		}
		g.in.pressed[btnOK] = true
		q.update(g)
		g.in.pressed[btnOK] = false
		if g.st.Vars["energy"] != 4. || g.currentBattle() == s {
			t.Fatalf("retry state %v", g.st.Vars)
		}
	}
}

func TestMeterLimitsAndEditorOpacity(t *testing.T) {
	g := &Game{st: testgame.State(testgame.Load(t, nil), 32)}
	for _, tc := range []struct {
		name, limit             string
		fixed, value, want, max float64
	}{
		{"legacy default", "", 0, 150, 100, 100},
		{"fixed", "", 20, -2, 0, 20},
		{"dynamic", "var.capacity", 20, 7, 7, 12},
		{"clamped", "var.capacity", 20, 25, 12, 12},
		{"zero", "0", 20, 3, 1, 1},
		{"negative", "-2", 20, 3, 1, 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			g.st.Vars["energy"], g.st.Vars["capacity"] = tc.value, 12.
			m := data.Meter{Value: data.Expr{Expr: testgame.Expr("var.energy")}, Max: tc.fixed}
			if tc.limit != "" {
				m.MaxValue = data.Expr{Expr: testgame.Expr(tc.limit)}
			}
			if value, limit := g.meterValue(&m); value != tc.want || limit != tc.max {
				t.Fatalf("value/limit=%g/%g", value, limit)
			}
		})
	}
	if (&tacticEditor{}).overlay() {
		t.Fatal("editor draws over previous screen")
	}
}

func TestMemoInputRouting(t *testing.T) {
	for _, tc := range []struct {
		name, commit, chars, composing, want string
		handled, consumed                    bool
	}{
		{name: "commit wins", commit: "試験", chars: "試験", want: "試験", consumed: true},
		{name: "fallback", chars: "入力", want: "入力", consumed: true},
		{name: "composition", composing: "入力", chars: "nyu", consumed: true},
		{name: "IME key", handled: true, chars: "x", consumed: true},
		{name: "idle"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			m := &memoBox{comp: tc.composing, committed: true}
			consumed, err := m.readInput(func() (bool, error) {
				if tc.commit != "" {
					m.committed = true
					m.text += tc.commit
				}
				return tc.handled, nil
			}, func(rs []rune) []rune { return append(rs, []rune(tc.chars)...) })
			if err != nil || consumed != tc.consumed || m.text != tc.want {
				t.Fatalf("consumed=%v text=%q err=%v", consumed, m.text, err)
			}
			m.comp = ""
			consumed, err = m.readInput(func() (bool, error) { return false, nil }, func(rs []rune) []rune { return rs })
			if err != nil || consumed {
				t.Fatalf("next frame consumed=%v err=%v", consumed, err)
			}
		})
	}
}

func TestMemoWindowInputByDefault(t *testing.T) {
	g := openGame(t, Options{Location: "camp"})
	g.openMemo()
	g.memo.age = 4
	g.in.pressed[btnOK] = true
	g.updateMemo()
	if !g.memo.typed || g.memo.ext != nil || g.memoUnfocused {
		t.Fatal("default input started external entry")
	}
}
