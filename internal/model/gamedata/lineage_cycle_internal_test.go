package gamedata

import "testing"

func TestLineageCycleStops(t *testing.T) {
	lin := &Lineage{recipes: map[string][]Recipe{
		"a": {{Kind: RecipePair, Parents: [2]string{"b", "b"}}},
		"b": {{Kind: RecipePair, Parents: [2]string{"a", "a"}}},
	}}
	root := lin.Tree("a", nil)
	if root.Parents[0].Parents[0].Ref != "cycle" || root.Parents[1].Ref != "seen" {
		t.Fatalf("cycle and repeated nodes: %+v", root)
	}
}
