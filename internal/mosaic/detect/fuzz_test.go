package detect

import (
	"slices"
	"testing"
)

// FuzzParseNamesMetadata: whatever the metadata text is, a successful parse gives class
// names that pass the class check, one per index 0..n-1, and never panics.
func FuzzParseNamesMetadata(f *testing.F) {
	for _, s := range []string{"{0: 'a', 1: 'b'}", "{1: 'b', 0: \"a\"}", "", "{}", "{0: 'a', 0: 'b'}", "{99999999999999999999: 'x'}", "{0: ''}", "{-1: 'a'}"} {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, s string) {
		names, err := ParseNamesMetadata(s)
		if err != nil {
			return
		}
		if verr := ValidateClasses(names); verr != nil {
			t.Fatalf("parsed %q to %q which is not valid: %v", s, names, verr)
		}
	})
}

// FuzzParseLabelsJSON: a successful parse is a valid class list.
func FuzzParseLabelsJSON(f *testing.F) {
	for _, s := range []string{`["a","b"]`, `[]`, `["a",""]`, `{}`, `null`, `["a","a"]`, `[1]`} {
		f.Add([]byte(s))
	}
	f.Fuzz(func(t *testing.T, b []byte) {
		names, err := ParseLabelsJSON(b)
		if err != nil {
			return
		}
		if verr := ValidateClasses(names); verr != nil || slices.Contains(names, "") {
			t.Fatalf("parsed %q to %q: %v", b, names, verr)
		}
	})
}
