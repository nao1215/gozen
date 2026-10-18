package rpg_test

import (
	"testing"

	"github.com/nao1215/gozen/expr"
)

func mustExpr(t *testing.T, src string) *expr.Expr {
	t.Helper()
	e, err := expr.Parse(src)
	if err != nil {
		t.Fatal(err)
	}
	return e
}
