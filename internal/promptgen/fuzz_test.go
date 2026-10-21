package promptgen

import (
	"strings"
	"testing"
)

// FuzzParseCatalog: no catalog text makes Parse, Build or Lint panic, and a catalog that
// parses builds text that never lacks the fixed negative age guards.
func FuzzParseCatalog(f *testing.F) {
	f.Add([]byte(base))
	f.Add([]byte(""))
	f.Add([]byte("version: 1"))
	f.Add([]byte("---\n---\n"))
	f.Fuzz(func(t *testing.T, data []byte) {
		c, err := Parse(data)
		if err != nil {
			return
		}
		c.Lint()
		for _, id := range sortedKeys(c.c.Scenes) {
			p, err := c.Scene(id)
			if err != nil {
				t.Fatal(err)
			}
			r, err := c.Build(p)
			if err != nil {
				continue
			}
			if !strings.Contains(r.Negative, "child") || !strings.Contains(r.Negative, "underage") {
				t.Fatalf("scene %s lost the age guards: %q", id, r.Negative)
			}
		}
	})
}

// FuzzParsePlan: no plan text panics, and a plan that parses can be built or refused.
func FuzzParsePlan(f *testing.F) {
	for _, s := range []string{"select: {}", "select: {place: [field]}", "free: text", "", "select: [a]"} {
		f.Add([]byte(s))
	}
	c, err := Parse([]byte(base))
	if err != nil {
		f.Fatal(err)
	}
	f.Fuzz(func(t *testing.T, data []byte) {
		p, err := ParsePlan(data)
		if err != nil {
			return
		}
		r, err := c.Build(p)
		if err != nil {
			return
		}
		if !strings.Contains(r.Negative, "child") || !strings.Contains(r.Negative, "underage") {
			t.Fatalf("a plan lost the age guards: %q", r.Negative)
		}
	})
}
