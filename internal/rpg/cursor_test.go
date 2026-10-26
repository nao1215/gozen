package rpg_test

import (
	"slices"
	"testing"

	"github.com/nao1215/gozen/internal/rpg"
	"github.com/nao1215/gozen/internal/testgame"
)

// TestSaveFormatTwo: a save is written at format version 2; a save of version 1 is still
// read; a cursor goes on a save and comes back with Parse; a bad cursor, or one on a save of
// version 1, is refused (ADR 0025).
func TestSaveFormatTwo(t *testing.T) {
	db := testgame.Load(t, nil)
	db.Game.ID = "cursor-test"
	st := testgame.State(db, 1)
	st.Chose = map[string]string{"dinner": "soup"}
	b := saveBytes(t, st)
	got, err := rpg.Parse(db, b, nil)
	if err != nil || got.Chose["dinner"] != "soup" || got.Resume != nil {
		t.Fatalf("version 2 without a cursor: %v %+v", err, got)
	}
	old := rewriteSave(t, b, func(m map[string]any) {
		m["format_version"] = 1
		delete(object(m, "state"), "chose")
	})
	if _, err := rpg.Parse(db, old, nil); err != nil {
		t.Fatalf("a save of version 1 is not read: %v", err)
	}
	c := rpg.Cursor{Event: "greet", Line: 3, Answers: []int{1}}
	withCursor, err := rpg.WithCursor(b, c, []byte{1, 2, 3})
	if err != nil {
		t.Fatal(err)
	}
	got, err = rpg.Parse(db, withCursor, nil)
	if err != nil || got.Resume == nil || got.Resume.Cursor.Event != "greet" || got.Resume.Cursor.Line != 3 ||
		!slices.Equal(got.Resume.Cursor.Answers, []int{1}) || !slices.Equal(got.Resume.Rand, []byte{1, 2, 3}) {
		t.Fatalf("the cursor did not come back: %v %+v", err, got)
	}
	for name, edit := range map[string]func(map[string]any){
		"version 1":         func(m map[string]any) { m["format_version"] = 1 },
		"no line":           func(m map[string]any) { object(m, "cursor")["line"] = 0 },
		"no event":          func(m map[string]any) { delete(object(m, "cursor"), "event") },
		"both":              func(m map[string]any) { object(m, "cursor")["arrival"] = true },
		"unknown fields":    func(m map[string]any) { object(m, "cursor")["page"] = 1 },
		"a negative answer": func(m map[string]any) { object(m, "cursor")["answers"] = []any{0, -1} },
	} {
		if _, err := rpg.Parse(db, rewriteSave(t, withCursor, edit), nil); err == nil {
			t.Errorf("%s: accepted", name)
		}
	}
	if _, err := rpg.WithCursor(b, rpg.Cursor{Line: 1}, nil); err == nil {
		t.Errorf("a cursor without an event: %v", err)
	}
	if _, err := rpg.WithCursor(b, rpg.Cursor{Event: "greet", Line: 1, Answers: []int{-1}}, nil); err == nil {
		t.Errorf("a cursor with a negative answer: %v", err)
	}
}

// TestChoseExpression: chose.<id> is the option taken, "" before.
func TestChoseExpression(t *testing.T) {
	db := testgame.Load(t, nil)
	st := testgame.State(db, 1)
	for _, c := range []struct{ src, want string }{{`chose.dinner == ""`, "true"}, {`chose.dinner == "soup"`, "false"}} {
		v, err := testgame.Expr(c.src).Eval(st.Env())
		if err != nil || fmtBool(v) != c.want {
			t.Errorf("%s: %v %v", c.src, v, err)
		}
	}
	st.Chose = map[string]string{"dinner": "soup"}
	if v, err := testgame.Expr(`chose.dinner == "soup"`).Eval(st.Env()); err != nil || v != true {
		t.Errorf("after the answer: %v %v", v, err)
	}
}

func fmtBool(v any) string {
	if b, ok := v.(bool); ok && b {
		return "true"
	}
	return "false"
}
