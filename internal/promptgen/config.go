// Package promptgen assembles deterministic prompts from user-owned catalogs.
// Its only external consumer is the gozen prompt command (internal/promptgen/command); it sends no generation jobs.
package promptgen

import (
	"fmt"
	"io/fs"
	"path"

	"regexp"
	"slices"
	"strings"

	"github.com/goccy/go-yaml"
	"github.com/goccy/go-yaml/parser"

	lang "github.com/nao1215/gozen/text"
)

type config struct {
	Version         int             `yaml:"version"`
	Dialect         string          `yaml:"dialect"`
	Emphasis        []string        `yaml:"emphasis"`
	DefaultEmphasis string          `yaml:"default_emphasis"`
	Validation      string          `yaml:"validation"`
	Priorities      map[string]int  `yaml:"priorities"`
	NegativeLimit   *int            `yaml:"negative_limit"`
	Separator       string          `yaml:"separator"`
	Layers          []layer         `yaml:"layers"`
	Scenes          map[string]Plan `yaml:"scenes"`
}

type layer struct {
	ID        string          `yaml:"id"`
	Person    bool            `yaml:"person"`
	Min       int             `yaml:"min"`
	Max       int             `yaml:"max"`
	Defaults  []string        `yaml:"defaults"`
	Separate  bool            `yaml:"separate"`
	Separator string          `yaml:"separator"`
	Items     map[string]item `yaml:"items"`
}

type item struct {
	Positive  []term        `yaml:"positive"`
	Negative  []term        `yaml:"negative"`
	Requires  []reference   `yaml:"requires"`
	Conflicts []reference   `yaml:"conflicts"`
	Replaces  []replacement `yaml:"replaces"`
}

type term struct {
	Word     string `yaml:"word"`
	Priority string `yaml:"priority"`
	Emphasis string `yaml:"emphasis"`
}

type reference struct {
	Layer string `yaml:"layer"`
	ID    string `yaml:"id"`
}

type replacement struct {
	Side  string `yaml:"side"`
	Word  string `yaml:"word"`
	Layer string `yaml:"layer"`
	ID    string `yaml:"id"`
}

// Plan selects catalog IDs by layer; it cannot inject free-form terms.
type Plan struct {
	Select map[string][]string `yaml:"select" json:"select"`
}

// Catalog owns validated configuration for the gozen prompt command. Build does not mutate it.
type Catalog struct {
	c       config
	limit   int
	dialect sdxl
	rules   []Rule
}

var identifier = regexp.MustCompile(`^[a-z][a-z0-9_]*$`)

func problem(format string, args ...any) error { return fmt.Errorf(format, args...) }

// Parse validates one catalog document and optional rule documents.
func Parse(data []byte, ruleData ...[]byte) (*Catalog, error) {
	var c config
	if err := decode(data, &c); err != nil {
		return nil, err
	}
	d, err := dialectByName(c.Dialect)
	if err != nil {
		return nil, err
	}
	g := &Catalog{c: c, limit: 100, dialect: d}
	if c.NegativeLimit != nil {
		g.limit = *c.NegativeLimit
	}
	if c.Validation != "" && len(ruleData) == 0 {
		return nil, problem("%s", lang.T("宣言した検証規則を読み込んでください"))
	}
	for _, data := range ruleData {
		var file struct {
			Rules []Rule `yaml:"rules"`
		}
		if err := decode(data, &file); err != nil {
			return nil, err
		}
		g.rules = append(g.rules, file.Rules...)
	}
	if err := g.validate(); err != nil {
		return nil, err
	}
	if err := g.validateRules(); err != nil {
		return nil, err
	}
	for _, id := range sortedKeys(c.Scenes) {
		if !identifier.MatchString(id) {
			return nil, problem(lang.T("構図 ID が不正です。%s"), id)
		}
	}
	return g, nil
}

// Load reads a catalog and optional relative validation rules from fsys.
func Load(fsys fs.FS, name string) (*Catalog, error) {
	if fsys == nil || !fs.ValidPath(name) {
		return nil, fmt.Errorf("invalid catalog filesystem or path")
	}
	data, err := fs.ReadFile(fsys, name)
	if err != nil {
		return nil, err
	}
	var c config
	if err := decode(data, &c); err != nil {
		return nil, err
	}
	if c.Validation == "" {
		return Parse(data)
	}
	if !fs.ValidPath(c.Validation) || strings.ContainsAny(c.Validation, "\\:") {
		return nil, fmt.Errorf("validation rules must use a relative filesystem path")
	}
	rules, err := fs.ReadFile(fsys, path.Join(path.Dir(name), c.Validation))
	if err != nil {
		return nil, err
	}
	return Parse(data, rules)
}

// ParsePlan rejects unknown fields; Build checks selected IDs.
func ParsePlan(data []byte) (Plan, error) {
	var p Plan
	err := decode(data, &p)
	return p, err
}

// LayerIDs returns a copy of layer IDs in catalog order for CLI flags.
func (g *Catalog) LayerIDs() []string {
	var ids []string
	for _, l := range g.c.Layers {
		ids = append(ids, l.ID)
	}
	return ids
}

// Scene returns a copy of a named selection so CLI overrides cannot mutate the catalog.
func (g *Catalog) Scene(id string) (Plan, error) {
	p, ok := g.c.Scenes[id]
	if !ok {
		return Plan{}, problem(lang.T("未知の構図です。%s"), id)
	}
	copy := Plan{Select: map[string][]string{}}
	for k, v := range p.Select {
		copy.Select[k] = slices.Clone(v)
	}
	return copy, nil
}

func sortedKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	slices.Sort(keys)
	return keys
}

func core(s string) string { return strings.ToLower(strings.Join(strings.Fields(s), " ")) }

func validWord(s string) bool { return abstractTerm(s) == nil }

func separator(s string) bool { return s == "" || s == "inline" || s == "block" }

func (g *Catalog) emphasis(t term) int {
	name := t.Emphasis
	if name == "" {
		name = g.c.DefaultEmphasis
	}
	return slices.Index(g.c.Emphasis, name)
}

func (g *Catalog) validate() error {
	if g.c.Version != 1 || g.limit < 0 || len(g.c.Priorities) == 0 || len(g.c.Layers) == 0 || !separator(g.c.Separator) {
		return problem("%s", lang.T("設定の版・上限・段・層・区切りが不正です"))
	}
	if len(g.c.Emphasis) == 0 || slices.Index(g.c.Emphasis, g.c.DefaultEmphasis) < 0 {
		return problem("%s", lang.T("強調の段階と既定値が必要です"))
	}
	seenEmphasis := map[string]bool{}
	for _, name := range g.c.Emphasis {
		if !identifier.MatchString(name) || seenEmphasis[name] {
			return problem(lang.T("強調の段階が不正です。%s"), name)
		}
		seenEmphasis[name] = true
	}
	for _, name := range sortedKeys(g.c.Priorities) {
		if !identifier.MatchString(name) || g.c.Priorities[name] < 0 {
			return problem(lang.T("優先度の段が不正です。%s"), name)
		}
	}
	layers := map[string]layer{}
	for _, l := range g.c.Layers {
		_, exists := layers[l.ID]
		if !identifier.MatchString(l.ID) || slices.Contains([]string{"config", "plan", "scene", "scenes", "format", "help", "h", "lint"}, l.ID) || exists || l.Min < 0 || l.Max < 0 || (l.Max > 0 && l.Max < l.Min) || len(l.Items) == 0 || !separator(l.Separator) {
			return problem(lang.T("層の定義が不正です。%s"), l.ID)
		}
		layers[l.ID] = l
	}
	for _, l := range g.c.Layers {
		seen := map[string]bool{}
		for _, id := range l.Defaults {
			if _, ok := l.Items[id]; !ok || seen[id] {
				return problem(lang.T("既定の選択が不正です。%s/%s"), l.ID, id)
			}
			seen[id] = true
		}
		if l.Max > 0 && len(l.Defaults) > l.Max {
			return problem(lang.T("既定の選択が上限を超えます。%s"), l.ID)
		}
		for _, id := range sortedKeys(l.Items) {
			it := l.Items[id]
			if !identifier.MatchString(id) {
				return problem(lang.T("項目 ID が不正です。%s/%s"), l.ID, id)
			}
			for _, side := range []string{"positive", "negative"} {
				ts := it.Positive
				if side == "negative" {
					ts = it.Negative
				}
				for _, t := range ts {
					_, ok := g.c.Priorities[t.Priority]
					if !ok || g.dialect.ValidateTerm(t.Word) != nil || g.emphasis(t) < 0 {
						return problem(lang.T("語・強調・段が不正です。%s/%s %s"), l.ID, id, t.Word)
					}
					if err := checkGuard(side, t.Word); err != nil {
						return err
					}
					if g.emphasis(t) > 4 || g.dialect.Weight(g.emphasis(t)) > sdxlMaxWeight {
						return problem(lang.T("強調が上限を超えています。%s/%s %s"), l.ID, id, t.Word)
					}
				}
			}
			for _, r := range append(slices.Clone(it.Requires), it.Conflicts...) {
				if _, ok := layers[r.Layer].Items[r.ID]; !ok || (r.Layer == l.ID && r.ID == id) {
					return problem(lang.T("依存・排他の参照が不正です。%s/%s"), r.Layer, r.ID)
				}
			}
			for _, r := range it.Replaces {
				if (r.Side != "positive" && r.Side != "negative") || !validWord(r.Word) || (r.ID != "" && r.Layer == "") {
					return problem(lang.T("置換の指定が不正です。%s/%s"), l.ID, id)
				}
				if protected(r.Word) {
					return problem(lang.T("固定語は置換できません。%s"), r.Word)
				}
				found := false
				for _, target := range g.c.Layers {
					if r.Layer != "" && r.Layer != target.ID {
						continue
					}
					for _, targetID := range sortedKeys(target.Items) {
						if r.ID != "" && r.ID != targetID {
							continue
						}
						ts := target.Items[targetID].Positive
						if r.Side == "negative" {
							ts = target.Items[targetID].Negative
						}
						for _, t := range ts {
							if core(t.Word) == core(r.Word) {
								found = true
								if g.c.Priorities[t.Priority] == 0 {
									return problem(lang.T("段0の語は置換できません。%s"), r.Word)
								}
							}
						}
					}
				}
				if !found {
					return problem(lang.T("置換する語が定義されていません。%s"), r.Word)
				}
			}
		}
	}
	return nil
}

func decode(data []byte, into any) error {
	file, err := parser.ParseBytes(data, 0)
	if err != nil {
		return err
	}
	if len(file.Docs) != 1 || file.Docs[0].Body == nil {
		return problem("%s", lang.T("YAML は１つの空でない文書にしてください"))
	}
	return yaml.NodeToValue(file.Docs[0].Body, into, yaml.Strict())
}
