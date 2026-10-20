package gamedata

import (
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"

	"github.com/goccy/go-yaml"
)

// schemaEnglish is the English wording of the generated schema, read from the
// files the site's Schema page uses: website/data/schema_en.yaml (the contract
// cells and the type labels) and website/data/schema_fields/*.yaml (a
// description of every type and field). The contracts are written in Japanese
// in this file's tables; types.schema.json carries them in English.
type schemaEnglish struct {
	cells  map[string]string
	labels []struct {
		Ja string `yaml:"ja"`
		En string `yaml:"en"`
	}
	fields map[string]map[string]string
}

var nonASCII = regexp.MustCompile(`[^\x00-\x7F]`)

func loadSchemaEnglish(t *testing.T) *schemaEnglish {
	t.Helper()
	const site = "../../../website/data"
	b, err := os.ReadFile(filepath.Join(site, "schema_en.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	var tr struct {
		Cells map[string]string `yaml:"cells"`
		Types []struct {
			Ja string `yaml:"ja"`
			En string `yaml:"en"`
		} `yaml:"types"`
	}
	if err := yaml.Unmarshal(b, &tr); err != nil {
		t.Fatal(err)
	}
	e := &schemaEnglish{cells: tr.Cells, labels: tr.Types, fields: map[string]map[string]string{}}
	files, err := filepath.Glob(filepath.Join(site, "schema_fields", "*.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	sort.Strings(files)
	for _, name := range files {
		b, err := os.ReadFile(name) //nolint:gosec // Paths come from a glob of the repository's site data.
		if err != nil {
			t.Fatal(err)
		}
		var part map[string]map[string]string
		if err := yaml.Unmarshal(b, &part); err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		maps.Copy(e.fields, part)
	}
	return e
}

// cell is the English of one contract cell; problems collects the cells
// without one.
func (e *schemaEnglish) cell(s string, problems *[]string) string {
	if !nonASCII.MatchString(s) {
		return s
	}
	en, ok := e.cells[s]
	if !ok {
		*problems = append(*problems, fmt.Sprintf("no English for the contract cell %q; add it to website/data/schema_en.yaml", s))
		return s
	}
	return en
}

// label is the English of a type label (x-type).
func (e *schemaEnglish) label(s string, problems *[]string) string {
	for _, r := range e.labels {
		s = strings.ReplaceAll(s, r.Ja, r.En)
	}
	if nonASCII.MatchString(strings.ReplaceAll(s, "→", "")) {
		*problems = append(*problems, fmt.Sprintf("no English for the type label %q; add it to website/data/schema_en.yaml", s))
	}
	return s
}

// translate puts the schema of b into English: the contract cells and type
// labels of every field, and the description of every type and field. It
// returns what has no English.
func (b *schemaBuilder) translate(e *schemaEnglish) []string {
	var problems []string
	for _, name := range sortedNames(b.defs) {
		def := child(b.defs, name)
		desc := e.fields[name]
		if desc["_"] == "" {
			problems = append(problems, name+": no description `_` in website/data/schema_fields")
		}
		def["description"] = desc["_"]
		props := def
		if anyOf, ok := def["anyOf"].([]any); ok {
			for _, a := range anyOf {
				if n, ok := a.(schemaNode); ok && n["properties"] != nil {
					props = n
				}
			}
		}
		fields := child(props, "properties")
		for _, field := range sortedNames(fields) {
			n := child(fields, field)
			c, ok := n["x-contract"].(fieldContract)
			if !ok {
				continue
			}
			en := fieldContract{
				Default:       e.cell(c.Default, &problems),
				LegacyDefault: e.cell(c.LegacyDefault, &problems),
				Zero:          e.cell(c.Zero, &problems),
				Required:      e.cell(c.Required, &problems),
				Reference:     e.cell(c.Reference, &problems),
				Version:       e.cell(c.Version, &problems),
			}
			if en.LegacyDefault != en.Default {
				// The Schema page writes the two defaults as one cell and reads its
				// wording from schema_en.yaml.
				e.cell(fmt.Sprintf("旧版 %s／新版 %s", en.LegacyDefault, en.Default), &problems)
			}
			n["x-contract"] = en
			if l, ok := n["x-type"].(string); ok {
				n["x-type"] = e.label(l, &problems)
			}
			if desc[field] == "" {
				problems = append(problems, fmt.Sprintf("%s.%s: no description in website/data/schema_fields", name, field))
			}
			n["description"] = desc[field]
		}
	}
	return problems
}

func sortedNames(n schemaNode) []string {
	names := make([]string, 0, len(n))
	for k := range n {
		names = append(names, k)
	}
	sort.Strings(names)
	return names
}
