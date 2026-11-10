package react

import "testing"

// A } with no { before it is text to the book's check, so the field after it is filled
// as the check promised; Fill reads a line the same way.
func TestLineWithStrayBraceFillsItsFields(t *testing.T) {
	b, err := Parse([]byte("people:\n  - id: host\n    rules:\n      - {id: r, say: ['} hello {from}']}\n"))
	if err != nil {
		t.Fatal(err)
	}
	m, err := New(b, "host", Config{})
	if err != nil {
		t.Fatal(err)
	}
	r, err := m.React(Stimulus{Kind: KindSay, From: "ann", Text: "x"})
	if err != nil {
		t.Fatal(err)
	}
	if got := r.Lines[0]; got != "} hello ann" {
		t.Errorf("line = %q: the accepted field {from} was not filled", got)
	}
	for l, want := range map[string]string{"a } b {x} c": "a } b X c", "{x}{x}": "XX", "open {x": "open {x", "{y} end": " end", "": ""} {
		if got := Fill(l, map[string]string{"x": "X"}); got != want {
			t.Errorf("Fill(%q) = %q, want %q", l, got, want)
		}
	}
	if _, err := Parse([]byte("people:\n  - id: host\n    rules:\n      - {id: r, say: ['open {from']}\n")); err == nil {
		t.Error("an unclosed { was accepted")
	}
}
