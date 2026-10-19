package optional

import "testing"

func TestValue(t *testing.T) {
	vars := map[string]any{"n": 2.5, "s": "x", "b": true}
	if got := Value[float64](vars["n"]); got != 2.5 {
		t.Errorf("number = %v", got)
	}
	if got := Value[float64](vars["missing"]); got != 0 {
		t.Errorf("missing = %v", got)
	}
	if got := Value[float64](vars["s"]); got != 0 {
		t.Errorf("wrong type = %v", got)
	}
	if got := Value[string](vars["s"]); got != "x" {
		t.Errorf("string = %q", got)
	}
	if got := Value[bool](vars["b"]); !got {
		t.Error("bool = false")
	}
}
