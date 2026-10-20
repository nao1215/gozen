package gamedata

import (
	"testing"

	"github.com/nao1215/gozen/internal/expr"
)

func TestUsesFindsVerbInBranches(t *testing.T) {
	db := &DB{
		Events:    map[string]*Event{"a": {Do: []Command{{If: mustIf(t), Then: []Command{{Choice: []ChoiceOption{{Do: []Command{{SPSuggest: "sp"}}}}}}}}}},
		Locations: map[string]*Location{"camp": {}},
	}
	if !db.Uses("sp_suggest") {
		t.Error("if の then の choice の 中の sp_suggest を 見つけない")
	}
	if db.Uses("stone_suggest") {
		t.Error("無い stone_suggest を 見つけた")
	}
}

func mustIf(t *testing.T) Expr {
	t.Helper()
	p, err := expr.Parse("true")
	if err != nil {
		t.Fatal(err)
	}
	return Expr{p}
}
