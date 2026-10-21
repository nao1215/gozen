package promptgen

import "testing"

// TestHostileYAMLIsAnErrorNotAPanic: catalogs, rule files and plans are untrusted input
// (SECURITY.md). The YAML decoder panics on some shapes (a tagged scalar where a list is
// wanted); Parse, ParsePlan and Load must report them as errors.
func TestHostileYAMLIsAnErrorNotAPanic(t *testing.T) {
	for _, y := range []string{
		"emphasis: !x 0\n",
		"version: 1\ndialect: sdxl\nemphasis: !x 0\n",
		"layers: !x 0\n",
		"priorities: !x 0\n",
	} {
		if _, err := Parse([]byte(y)); err == nil {
			t.Errorf("Parse accepted %q", y)
		}
	}
	if _, err := Parse([]byte(base), []byte("rules: !x 0\n")); err == nil {
		t.Error("a rule file with a tagged scalar was accepted")
	}
	if _, err := ParsePlan([]byte("select: !x 0\n")); err == nil {
		t.Error("a plan with a tagged scalar was accepted")
	}
}
