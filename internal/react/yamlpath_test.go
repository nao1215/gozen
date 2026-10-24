package react

import (
	"errors"
	"io/fs"
	"strings"
	"testing"
)

// Book.YAML returns part of the book as YAML, as written (inside a list, select by id or index).
func TestBookYAML(t *testing.T) {
	b, err := Parse([]byte(host))
	if err != nil {
		t.Fatal(err)
	}
	got, err := b.YAML("people.host.asks[0].id")
	if err != nil || strings.TrimSpace(string(got)) != "pet" {
		t.Fatalf("%q %v", got, err)
	}
	if got, err := b.YAML("people[0].faces"); err != nil || !strings.Contains(string(got), "smile") {
		t.Fatalf("%q %v", got, err)
	}
	if all, err := b.YAML(""); err != nil || string(all) != host {
		t.Fatalf("the whole book: %v", err)
	}
	for _, p := range []string{"people.nobody", "people[9]", "lexicon.cat.x", "people[x]"} {
		if _, err := b.YAML(p); !errors.Is(err, fs.ErrNotExist) {
			t.Errorf("%s: %v", p, err)
		}
	}
}
