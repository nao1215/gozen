package rpg_test

import (
	"math/rand/v2"
	"slices"
	"strings"
	"testing"

	data "github.com/nao1215/gozen/internal/layer2/gamedata"
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
	rnd, err := rand.NewPCG(3, 5).MarshalBinary()
	if err != nil {
		t.Fatal(err)
	}
	withCursor, err := rpg.WithCursor(b, c, rnd)
	if err != nil {
		t.Fatal(err)
	}
	got, err = rpg.Parse(db, withCursor, nil)
	if err != nil || got.Resume == nil || got.Resume.Cursor.Event != "greet" || got.Resume.Cursor.Line != 3 ||
		!slices.Equal(got.Resume.Cursor.Answers, []int{1}) || !slices.Equal(got.Resume.Rand, rnd) {
		t.Fatalf("the cursor did not come back: %v %+v", err, got)
	}
	for name, edit := range map[string]func(map[string]any){
		"version 1":          func(m map[string]any) { m["format_version"] = 1 },
		"no line":            func(m map[string]any) { object(m, "cursor")["line"] = 0 },
		"no event":           func(m map[string]any) { delete(object(m, "cursor"), "event") },
		"both":               func(m map[string]any) { object(m, "cursor")["arrival"] = true },
		"unknown fields":     func(m map[string]any) { object(m, "cursor")["page"] = 1 },
		"an answer below -1": func(m map[string]any) { object(m, "cursor")["answers"] = []any{0, -2} },
	} {
		if _, err := rpg.Parse(db, rewriteSave(t, withCursor, edit), nil); err == nil {
			t.Errorf("%s: accepted", name)
		}
	}
	if _, err := rpg.WithCursor(b, rpg.Cursor{Line: 1}, nil); err == nil {
		t.Errorf("a cursor without an event: %v", err)
	}
	if _, err := rpg.WithCursor(b, rpg.Cursor{Event: "greet", Line: 1, Answers: []int{-2}}, nil); err == nil {
		t.Errorf("a cursor with an answer below -1: %v", err)
	}
	// -1 is a phone put down with nothing kept (a page left without its buttons).
	if _, err := rpg.WithCursor(b, rpg.Cursor{Event: "greet", Line: 1, Answers: []int{-1}}, nil); err != nil {
		t.Errorf("a cursor with a phone put down unread: %v", err)
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

// Entry indices alone cannot distinguish an edited screen with identical commands.
func TestCursorChecksOwningScreen(t *testing.T) {
	loadDB := func() *data.DB {
		t.Helper()
		db := testgame.Load(t, nil)
		db.Game.SchemaVersion = data.SchemaVersion10
		db.Game.Screens = map[string]*data.AuthorScreen{
			"stock": {Title: "Stock", Entries: []data.ScreenEntry{
				{Label: "Boards", Do: []data.Command{{Notice: "Ready"}}},
				{Label: "Tools", Do: []data.Command{{Notice: "Ready"}}},
			}},
		}
		return db
	}
	db := loadDB()
	st := testgame.State(db, 17)
	cur := rpg.Cursor{Event: "screen:stock:0", Line: 1}
	if err := cur.Seal(db); err != nil {
		t.Fatal(err)
	}
	b, err := rpg.WithCursor(saveBytes(t, st), cur, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := rpg.Parse(db, b, nil); err != nil {
		t.Fatalf("unchanged entry rejected: %v", err)
	}
	edited := loadDB()
	entries := edited.Game.Screens["stock"].Entries
	entries[0], entries[1] = entries[1], entries[0]
	if _, err := rpg.Parse(edited, b, nil); err == nil {
		t.Fatal("accepted a different entry at the same index with identical commands")
	}
	if _, err := rpg.Parse(db, b, nil); err != nil {
		t.Fatalf("original definition no longer loads: %v", err)
	}
}

func TestScreenReplayCacheOwnership(t *testing.T) {
	db := testgame.Load(t, nil)
	db.Game.SchemaVersion = data.SchemaVersion10
	answer := 0
	c := rpg.Cursor{Event: "greet", Inputs: []rpg.ReplayInput{{Request: strings.Repeat("0", 64)}, {Answer: &answer}}}
	if err := c.Seal(db); err != nil {
		t.Fatal(err)
	}
	b, err := rpg.WithCursor(saveBytes(t, testgame.State(db, 17)), c, nil)
	if err != nil {
		t.Fatal(err)
	}
	for range 3 {
		st, err := rpg.Parse(db, b, nil)
		if err != nil {
			t.Fatal(err)
		}
		in := st.Resume.Cursor.Inputs
		if in[0].Request != strings.Repeat("0", 64) || *in[1].Answer != 0 {
			t.Fatal("parse retained caller-owned replay inputs")
		}
		in[0].Request = "changed"
		*in[1].Answer = 99
	}
}

func TestGameOutcomeCursorVersionAndOwnership(t *testing.T) {
	db := testgame.Load(t, nil)
	db.Game.SchemaVersion = data.SchemaVersion12
	st := testgame.State(db, 7)
	base := saveBytes(t, st)
	outcome := rpg.GameOutcome{ID: "trail", Definition: strings.Repeat("a", 64), Invocation: 1, Status: "cancelled"}
	cursor := rpg.Cursor{Event: "greet", Line: 1, Games: []rpg.GameOutcome{outcome}}
	if err := cursor.Seal(db); err != nil {
		t.Fatal(err)
	}
	saved, err := rpg.WithCursor(base, cursor, nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, version := range []int{1, 2, 3} {
		old := rewriteSave(t, saved, func(m map[string]any) { m["format_version"] = version })
		if _, err := rpg.Parse(db, old, nil); err == nil {
			t.Fatalf("format %d accepted casual outcomes", version)
		}
	}
	loaded, err := rpg.Parse(db, saved, nil)
	if err != nil {
		t.Fatal(err)
	}
	loaded.Resume.Cursor.Games[0].Score = 999
	again, err := rpg.Parse(db, saved, nil)
	if err != nil || again.Resume.Cursor.Games[0].Score != 0 {
		t.Fatal("cached save shares mutable outcome storage")
	}
	for _, mutate := range []func(*rpg.Cursor){
		func(c *rpg.Cursor) { c.Games[0].Invocation = 2 },
		func(c *rpg.Cursor) { c.Games[0].Status = "running" },
		func(c *rpg.Cursor) { c.Games[0].Score = -1 },
		func(c *rpg.Cursor) { c.Games[0].Definition = "short" },
		func(c *rpg.Cursor) { c.Line = 0 },
		func(c *rpg.Cursor) { c.Inputs = []rpg.ReplayInput{{Request: strings.Repeat("b", 64)}} },
		func(c *rpg.Cursor) { c.Games = make([]rpg.GameOutcome, 4097) },
	} {
		bad := cursor
		bad.Games = slices.Clone(cursor.Games)
		mutate(&bad)
		if _, err := rpg.WithCursor(base, bad, nil); err == nil {
			t.Fatalf("invalid outcome cursor accepted: %+v", bad)
		}
	}
}
