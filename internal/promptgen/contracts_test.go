package promptgen

import (
	"encoding/json"
	"errors"
	"io"
	"os"
	"strings"
	"testing"
)

func catalog(t *testing.T) *Catalog {
	t.Helper()
	c, err := Load(os.DirFS("../../docs/examples/production"), "catalog.yaml")
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func TestNeutralGoldenAndDeterminism(t *testing.T) {
	c := catalog(t)
	p, err := c.Scene("portrait")
	if err != nil {
		t.Fatal(err)
	}
	r, err := c.Build(p)
	if err != nil {
		t.Fatal(err)
	}
	got, err := json.MarshalIndent(r, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	want, err := os.ReadFile("testdata/portrait.json")
	if err != nil {
		t.Fatal(err)
	}
	if string(got)+"\n" != string(want) {
		t.Fatal("neutral portrait golden changed")
	}
	for range 10 {
		again, err := c.Build(p)
		if err != nil {
			t.Fatal(err)
		}
		b, err := json.MarshalIndent(again, "", "  ")
		if err != nil {
			t.Fatal(err)
		}
		if string(b) != string(got) {
			t.Fatal("nondeterministic result")
		}
	}
	if !strings.Contains(r.Positive, "adult") || !strings.Contains(r.Negative, "underage") || r.FixedNegativeCount != 2 {
		t.Fatalf("age guards missing: %+v", r)
	}
	if r.NegativeCount != 2 || len(r.Dropped) != 1 || r.Dropped[0].Reason != "limit" {
		t.Fatalf("negative priority/limit failed: %+v", r)
	}
	p.Select["subject"][0] = "unknown"
	fresh, err := c.Scene("portrait")
	if err != nil || fresh.Select["subject"][0] != "traveler" {
		t.Fatal("Scene leaked mutable state")
	}
}

func TestSelectionsAndStrictInput(t *testing.T) {
	c := catalog(t)
	for _, p := range []Plan{
		{Select: map[string][]string{"unknown": {"x"}}},
		{Select: map[string][]string{"subject": {"unknown"}}},
		{Select: map[string][]string{"subject": {"traveler", "traveler"}}},
		{Select: map[string][]string{"subject": {"traveler"}, "setting": {"studio"}}},
	} {
		if _, err := c.Build(p); err == nil {
			t.Errorf("accepted invalid selection: %+v", p)
		}
	}
	r, err := c.Build(Plan{})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(r.Positive, "adult") {
		t.Fatal("person guard added to landscape")
	}
	b, err := os.ReadFile("../../docs/examples/production/catalog.yaml")
	if err != nil {
		t.Fatal(err)
	}
	for _, bad := range []string{
		string(b) + "\nunknown: true\n", string(b) + "\n---\nextra: true\n",
		strings.ReplaceAll(string(b), "quiet garden", "child portrait"),
		strings.ReplaceAll(string(b), "quiet garden", "adult"),
		strings.ReplaceAll(string(b), "quiet garden", "(garden:1.5)"),
	} {
		if _, err := Parse([]byte(bad)); err == nil {
			t.Errorf("accepted invalid catalog")
		}
	}
	if _, err := ParsePlan([]byte("select: {}\nfree_text: test\n")); err == nil {
		t.Fatal("accepted free text")
	}
	if _, err := Load(os.DirFS("."), "../catalog.yaml"); err == nil {
		t.Fatal("accepted escaping FS path")
	}
}

func TestRulesReplacementAndFailedIO(t *testing.T) {
	b, err := os.ReadFile("../../docs/examples/production/catalog.yaml")
	if err != nil {
		t.Fatal(err)
	}
	rules := []byte("rules:\n- id: garden_rule\n  kind: forbidden\n  severity: error\n  terms: [quiet garden]\n")
	c, err := Parse(b, rules)
	if err != nil {
		t.Fatal(err)
	}
	r, err := c.Build(Plan{})
	var validation *ValidationError
	if !errors.As(err, &validation) || r.Positive != "" || r.Negative != "" {
		t.Fatalf("failed validation produced usable text: %+v %v", r, err)
	}
	if len(c.Lint()) == 0 {
		t.Fatal("lint skipped validation")
	}
	if err := r.WriteText(brokenWriter{}); !errors.Is(err, io.ErrClosedPipe) {
		t.Fatalf("write error lost: %v", err)
	}
	if err := (Diagnostic{}).WriteText(brokenWriter{}); !errors.Is(err, io.ErrClosedPipe) {
		t.Fatalf("diagnostic write error lost: %v", err)
	}
	// Replacements may remove configurable terms, never age guards or required terms.
	for _, word := range []string{"adult", "underage", "child", "soft light"} {
		s := strings.Replace(string(b), "requires: [{layer: setting, id: garden}]", "requires: [{layer: setting, id: garden}]\n        replaces: [{side: positive, word: "+word+"}]", 1)
		if _, err := Parse([]byte(s)); err == nil {
			t.Errorf("replacement accepted protected/required term %s", word)
		}
	}
}

type brokenWriter struct{}

func (brokenWriter) Write([]byte) (int, error) { return 0, io.ErrClosedPipe }
