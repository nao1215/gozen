package play

import (
	"testing"

	"github.com/nao1215/gozen/internal/base/savefile"
	"github.com/nao1215/gozen/internal/testgame"
)

// TestCarryKeepsNamedValuesForNewGame: carry keeps only the named variables and
// flags (exact names and * prefixes), and a new state starts with them.
func TestCarryKeepsNamedValuesForNewGame(t *testing.T) {
	db := testgame.Load(t, map[string]string{"game.yaml": "schema_version: 10\nid: neutral\ncarry: [var.runs, flag.seen_*]\n" + testgame.Game})
	g := &Game{db: db, store: savefile.DirStore(t.TempDir())}
	st := testgame.State(db, 1)
	st.Vars["runs"], st.Vars["gold_like"] = 2.0, 9.0
	st.Flags["seen_end"], st.Flags["met"] = true, true
	g.keepCarry(st)
	fresh := testgame.State(db, 2)
	g.applyCarry(fresh)
	if fresh.Vars["runs"] != 2.0 || !fresh.Flags["seen_end"] {
		t.Fatalf("carried values missing: %v %v", fresh.Vars, fresh.Flags)
	}
	if _, ok := fresh.Vars["gold_like"]; ok || fresh.Flags["met"] {
		t.Fatalf("an unnamed value was carried: %v %v", fresh.Vars, fresh.Flags)
	}
	// A new process reads them back from the store.
	again := &Game{db: db, store: g.store}
	later := testgame.State(db, 3)
	again.applyCarry(later)
	if later.Vars["runs"] != 2.0 {
		t.Fatalf("not read back from carry.json: %v", later.Vars)
	}
}

func TestCarryAllowlistShrinksOnReload(t *testing.T) {
	db := testgame.Load(t, nil)
	db.Game.Carry = []string{"var.runs", "flag.seen_*"}
	g := &Game{db: db, store: savefile.DirStore(t.TempDir())}
	st := testgame.State(db, 1)
	st.Vars["runs"] = 2.0
	st.Flags["seen_a"], st.Flags["seen_b"] = true, true
	g.keepCarry(st)
	db.Game.Carry = []string{"flag.seen_a"}
	fresh := testgame.State(db, 2)
	g.applyCarry(fresh)
	if fresh.Vars["runs"] != nil || fresh.Flags["seen_b"] || !fresh.Flags["seen_a"] {
		t.Fatal("removed carry restored", fresh.Vars, fresh.Flags)
	}
	g.keepCarry(fresh)
	again := &Game{db: db, store: g.store}
	again.loadCarry()
	if len(again.carry.Vars) != 0 || len(again.carry.Flags) != 1 {
		t.Fatal("stale entries saved", again.carry)
	}
	db.Game.Carry = nil
	g.applyCarry(testgame.State(db, 3))
	if len(g.carry.Vars)+len(g.carry.Flags) != 0 {
		t.Fatal("carry not cleared")
	}
}

// A carry.json whose vars or flags are null (hand-edited, or
// truncated by another tool) is taken as it is, and the next carry command writes
// into a nil map.
func TestCarryRecordWithNullMaps(t *testing.T) {
	db := testgame.Load(t, nil)
	db.Game.Carry = []string{"var.runs", "flag.seen_*"}
	store := savefile.DirStore(t.TempDir())
	if err := store.Write("carry.json", []byte(`{"game_id":"`+db.Game.ID+`","vars":null,"flags":null}`)); err != nil {
		t.Fatal(err)
	}
	g := &Game{db: db, store: store}
	st := testgame.State(db, 1)
	st.Vars["runs"] = 2.0
	st.Flags["seen_end"] = true
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("carry panicked on a record with null maps: %v", r)
		}
	}()
	g.keepCarry(st)
}

func TestCarryRejectsStructuredAndMismatchedValues(t *testing.T) {
	db := testgame.Load(t, map[string]string{"game.yaml": "schema_version: 10\nid: neutral\ncarry: [var.*]\nstate: {names: {var.runs: {type: number, initial: 0}}}\n" + testgame.Game})
	g := &Game{db: db, carry: &carryRecord{Vars: map[string]any{"runs": "bad", "nested": map[string]any{"x": 1}, "valid": 2.0}}}
	g.filterCarry()
	if len(g.carry.Vars) != 1 || g.carry.Vars["valid"] != 2.0 {
		t.Fatal(g.carry.Vars)
	}
}
