package play

import (
	"testing"
)

// TestMemoLeavesOutVarsThatAreNotValues: a memo.vars entry whose expression is the bare name of a
// function (floor instead of var.floor) is not a value. The note is still written, with the
// other vars, instead of failing to encode.
func TestMemoLeavesOutVarsThatAreNotValues(t *testing.T) {
	dir := t.TempDir()
	g := sysOpen4(t, Options{Seed: 3, Location: "hall", MemoDir: dir}, map[string]string{
		"data/hall.yaml": sysCorridor,
		"game.yaml+":     "memo: {kinds: [{id: bug, label: Bug}], vars: [{name: coins, value: \"gold\"}, {name: depth, value: \"floor\"}]}\n",
	})
	m, _ := sysQuickNote(t, g, "bug")
	if m.Vars["coins"] != 100.0 {
		t.Fatalf("vars %v", m.Vars)
	}
	if _, ok := m.Vars["depth"]; ok {
		t.Fatalf("a function was kept as a var: %v", m.Vars)
	}
}
