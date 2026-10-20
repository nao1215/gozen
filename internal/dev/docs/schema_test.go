package docs

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"sort"
	"strings"
	"testing"

	"github.com/goccy/go-yaml"
)

// schemaFields reads data/schema/types.schema.json: every type and its fields,
// including the fields of its one-of shapes (a picture written as a map).
func schemaFields(t testing.TB) map[string][]string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(root, "data", "schema", "types.schema.json"))
	if err != nil {
		t.Fatal(err)
	}
	type node struct {
		Properties map[string]json.RawMessage `json:"properties"`
		OneOf      []node                     `json:"oneOf"`
		AnyOf      []node                     `json:"anyOf"`
	}
	var s struct {
		Defs map[string]node `json:"$defs"`
	}
	if err := json.Unmarshal(b, &s); err != nil {
		t.Fatal(err)
	}
	out := map[string][]string{}
	for name, n := range s.Defs {
		set := map[string]bool{}
		for f := range n.Properties {
			set[f] = true
		}
		for _, o := range append(slices.Clone(n.OneOf), n.AnyOf...) {
			for f := range o.Properties {
				set[f] = true
			}
		}
		fields := []string{}
		for f := range set {
			fields = append(fields, f)
		}
		sort.Strings(fields)
		out[name] = fields
	}
	return out
}

// schemaDescriptions reads website/data/schema_fields/*.yaml: type -> field ("_" for the type) -> text.
func schemaDescriptions(t testing.TB) (map[string]map[string]string, []string) {
	t.Helper()
	files, err := filepath.Glob(filepath.Join(root, "website", "data", "schema_fields", "*.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	all := map[string]map[string]string{}
	var bad []string
	for _, f := range files {
		b, err := os.ReadFile(f) //nolint:gosec // A data file of the website.
		if err != nil {
			t.Fatal(err)
		}
		var part map[string]map[string]string
		if err := yaml.UnmarshalWithOptions(b, &part, yaml.Strict()); err != nil {
			t.Fatalf("%s: %v", f, err)
		}
		for typ, fields := range part {
			if _, dup := all[typ]; dup {
				bad = append(bad, fmt.Sprintf("%s: the type %s is described twice", filepath.Base(f), typ))
			}
			all[typ] = fields
		}
	}
	return all, bad
}

var japanese = regexp.MustCompile(`[\p{Han}\p{Hiragana}\p{Katakana}]`)

// TestSchemaDescriptions keeps the Schema page complete: every type and field
// of the YAML has an English description in website/data/schema_fields, there
// is none for a type or field that no longer exists, and every topic of
// website/data/schema_sections.yaml names real types. A field added to the Go
// types fails here until it is described (the site's build fails too).
func TestSchemaDescriptions(t *testing.T) {
	fields := schemaFields(t)
	desc, bad := schemaDescriptions(t)
	check := func(where, text string) {
		switch {
		case strings.TrimSpace(text) == "":
			bad = append(bad, where+": no description")
		case japanese.MatchString(text):
			bad = append(bad, where+": the description is not in English")
		case len(strings.Fields(text)) < 3:
			bad = append(bad, where+": the description is too short to say what it is for")
		}
	}
	for _, typ := range sortedKeys(fields) {
		d, ok := desc[typ]
		if !ok {
			bad = append(bad, typ+": no descriptions; add the type with `_` and its fields to website/data/schema_fields")
			continue
		}
		check(typ+"._", d["_"])
		for _, f := range fields[typ] {
			check(typ+"."+f, d[f])
		}
		for f := range d {
			if f != "_" && !slices.Contains(fields[typ], f) {
				bad = append(bad, fmt.Sprintf("%s.%s: described but not a field of the schema", typ, f))
			}
		}
	}
	for typ := range desc {
		if _, ok := fields[typ]; !ok {
			bad = append(bad, typ+": described but not a type of the schema")
		}
	}
	b, err := os.ReadFile(filepath.Join(root, "website", "data", "schema_sections.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	var sections struct {
		Sections []struct {
			Title string   `yaml:"title"`
			Types []string `yaml:"types"`
		} `yaml:"sections"`
	}
	if err := yaml.UnmarshalWithOptions(b, &sections, yaml.Strict()); err != nil {
		t.Fatal(err)
	}
	seen := map[string]bool{}
	for _, s := range sections.Sections {
		for _, typ := range s.Types {
			if _, ok := fields[typ]; !ok {
				bad = append(bad, fmt.Sprintf("schema_sections.yaml: %q (%s) is not a type of the schema", typ, s.Title))
			}
			if seen[typ] {
				bad = append(bad, fmt.Sprintf("schema_sections.yaml: %s is listed twice", typ))
			}
			seen[typ] = true
		}
	}
	sort.Strings(bad)
	if len(bad) > 0 {
		t.Fatalf("the Schema page would be incomplete:\n%s", strings.Join(bad, "\n"))
	}
}

// TestSiteSchema checks the built Schema page (GOZEN_SITE_DIR, make website-check):
// a section for every type, with a row for every field.
func TestSiteSchema(t *testing.T) {
	dir := os.Getenv("GOZEN_SITE_DIR")
	if dir == "" {
		t.Skip("set GOZEN_SITE_DIR to the built site (make website-check)")
	}
	b, err := os.ReadFile(filepath.Join(dir, "schema", "index.html")) //nolint:gosec // The built site.
	if err != nil {
		t.Fatal(err)
	}
	if bad := checkSchemaPage(string(b), schemaFields(t)); len(bad) > 0 {
		t.Fatalf("the built Schema page is missing:\n%s", strings.Join(bad, "\n"))
	}
}

var h2 = regexp.MustCompile(`<h2[^>]*\sid=["']?([^"'\s>]+)`)

func checkSchemaPage(page string, fields map[string][]string) []string {
	sections := map[string]string{}
	locs := h2.FindAllStringSubmatchIndex(page, -1)
	for i, m := range locs {
		end := len(page)
		if i+1 < len(locs) {
			end = locs[i+1][0]
		}
		sections[page[m[2]:m[3]]] = page[m[0]:end]
	}
	var bad []string
	for _, typ := range sortedKeys(fields) {
		sec, ok := sections[strings.ToLower(typ)]
		if !ok {
			bad = append(bad, "no section for the type "+typ)
			continue
		}
		for _, f := range fields[typ] {
			if !strings.Contains(sec, "<code>"+f+"</code>") {
				bad = append(bad, fmt.Sprintf("%s: no row for %s", typ, f))
			}
		}
	}
	return bad
}

func TestCheckSchemaPage(t *testing.T) {
	page := `<h2 id=location>Location</h2><table><tr><td><code>name</code></td></tr></table><h2 id="exit">Exit</h2><td><code>to</code>`
	got := strings.Join(checkSchemaPage(page, map[string][]string{"Location": {"name", "area"}, "Exit": {"to"}, "Shop": {"items"}}), "\n")
	if got != "Location: no row for area\nno section for the type Shop" {
		t.Errorf("got:\n%s", got)
	}
}
