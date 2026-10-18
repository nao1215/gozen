package expr

import "testing"

func TestResultTypeWithoutEvaluation(t *testing.T) {
	resolve := func(r Ref) ValueType {
		if r.Name == "var" && len(r.Path) == 1 {
			return map[string]ValueType{"n": NumberType, "s": StringType, "b": BoolType}[r.Path[0].Name]
		}
		return UnknownType
	}
	for _, tc := range []struct {
		source string
		want   ValueType
	}{
		{"1", NumberType}, {"true", BoolType}, {"'x'", StringType}, {"nil", NilType},
		{"var.n", NumberType}, {"var['s']", StringType}, {"var.b", BoolType},
		{"rand(9)", NumberType}, {"chance(0.5)", BoolType}, {"str(rand())", StringType},
		{"var.b ? var.n : var.s", NumberType | StringType}, {"var.n + var.s", StringType},
		{"-var.s", InvalidType}, {"var.s - 1", InvalidType}, {"var.n + 1", NumberType},
		{"var.b && true", BoolType}, {"var.n < 3", BoolType}, {"!var.b", BoolType},
		{"unknown()", UnknownType},
	} {
		if got := mustParse(tc.source).ResultType(resolve); got != tc.want {
			t.Errorf("%s の型 %v、期待 %v", tc.source, got, tc.want)
		}
	}
}
