package promptgen

import (
	"fmt"
	"io"
	"slices"
	"strings"

	lang "github.com/nao1215/gozen/text"
)

type Rule struct {
	ID       string              `yaml:"id"`
	Kind     string              `yaml:"kind"`
	Severity string              `yaml:"severity"`
	Scope    string              `yaml:"scope"`
	Terms    []string            `yaml:"terms"`
	Target   string              `yaml:"target"`
	Colors   map[string][]string `yaml:"colors"`
	Minimum  int                 `yaml:"minimum"`
	Limit    *int                `yaml:"limit"`
}

type Diagnostic struct {
	Rule     string `json:"rule"`
	Severity string `json:"severity"`
	Message  string `json:"message"`
	Words    []Word `json:"words"`
	Context  string `json:"context,omitempty"`
}

type ValidationError struct{ Diagnostics []Diagnostic }

func (e *ValidationError) Error() string {
	var messages []string
	for _, d := range e.Diagnostics {
		if d.Severity == "error" {
			messages = append(messages, d.Rule+" "+d.Message)
		}
	}
	return strings.Join(messages, "; ")
}

func diagnostic(id, severity, message string, words []Word) Diagnostic {
	return Diagnostic{Rule: id, Severity: severity, Message: message, Words: slices.Clone(words)}
}

func hasErrors(ds []Diagnostic) bool {
	for _, d := range ds {
		if d.Severity == "error" {
			return true
		}
	}
	return false
}

func sideWords(words []Word, side string) []Word {
	var out []Word
	for _, w := range words {
		if w.Side == side {
			out = append(out, w)
		}
	}
	return out
}

func (g *Catalog) validateRules() error {
	seen := map[string]bool{}
	for _, r := range g.rules {
		if !identifier.MatchString(r.ID) || seen[r.ID] || !slices.Contains([]string{"error", "warning", "info"}, r.Severity) || !slices.Contains([]string{"", "group", "global"}, r.Scope) {
			return problem(lang.T("検証規則の名前・重さ・対象が不正です。%s"), r.ID)
		}
		seen[r.ID] = true
		switch r.Kind {
		case "conflict":
			if len(r.Terms) != 2 {
				return problem(lang.T("矛盾の規則には２語が必要です。%s"), r.ID)
			}
		case "forbidden", "risk":
			if len(r.Terms) == 0 {
				return problem(lang.T("検証規則の語が必要です。%s"), r.ID)
			}
		case "color":
			if r.Target == "" || len(r.Colors) < 2 {
				return problem(lang.T("色の対象と２色以上が必要です。%s"), r.ID)
			}
		case "emphasis":
			if r.Limit == nil || *r.Limit < 0 {
				return problem(lang.T("強調の上限が必要です。%s"), r.ID)
			}
		default:
			return problem(lang.T("未知の検証規則です。%s"), r.Kind)
		}
		if r.Minimum < 0 || r.Minimum > len(r.Terms) || (r.Kind != "risk" && r.Minimum != 0) || (r.Kind != "color" && (r.Target != "" || len(r.Colors) != 0)) || (r.Kind != "emphasis" && r.Limit != nil) || ((r.Kind == "emphasis" || r.Kind == "color") && len(r.Terms) != 0) {
			return problem(lang.T("検証規則の欄の組み合わせが不正です。%s"), r.ID)
		}
		seenTerms := map[string]bool{}
		for _, w := range r.Terms {
			if !validWord(w) || seenTerms[core(w)] {
				return problem(lang.T("規則の語が不正か重複しています。%s"), r.ID)
			}
			seenTerms[core(w)] = true
		}
		for _, color := range sortedKeys(r.Colors) {
			if color == "" || len(r.Colors[color]) == 0 {
				return problem(lang.T("色の語が必要です。%s"), r.ID)
			}
			for _, w := range r.Colors[color] {
				if !validWord(w) || seenTerms[core(w)] {
					return problem(lang.T("色の語が不正か複数の色に属しています。%s"), r.ID)
				}
				seenTerms[core(w)] = true
			}
		}
	}
	return nil
}

func ruleGroups(words []Word, scope string) [][]Word {
	if scope == "global" {
		return [][]Word{words}
	}
	var common []Word
	groups := map[string][]Word{}
	var order []string
	for _, w := range words {
		if w.Group == "" {
			common = append(common, w)
			continue
		}
		if _, ok := groups[w.Group]; !ok {
			order = append(order, w.Group)
		}
		groups[w.Group] = append(groups[w.Group], w)
	}
	if len(order) == 0 {
		return [][]Word{common}
	}
	var out [][]Word
	for _, key := range order {
		out = append(out, append(slices.Clone(common), groups[key]...))
	}
	return out
}

func (g *Catalog) validateResult(r Result) []Diagnostic {
	out := g.dialect.ValidateOutput(r)

	seen := map[[3]string]bool{}
	positives := map[string]bool{}
	for _, w := range r.Words {
		key := [3]string{w.Side, w.Group, w.Word}
		if seen[key] {
			out = append(out, diagnostic("core.duplicate", "error", lang.T("統合後に重複があります"), []Word{w}))
		}
		seen[key] = true
		if w.Side == "positive" {
			positives[w.Word] = true
		}
		if !w.Fixed && w.Strength > 4 {
			out = append(out, diagnostic("core.emphasis", "error", lang.T("強調の段階が上限を超えています"), []Word{w}))
		}
	}
	for _, w := range r.Words {
		if w.Side == "negative" && positives[w.Word] {
			out = append(out, diagnostic("core.polarity", "error", lang.T("正負の同じ語が残っています"), []Word{w}))
		}
	}
	for _, rule := range g.rules {
		groups := ruleGroups(sideWords(r.Words, "positive"), rule.Scope)
		if rule.Kind == "emphasis" {
			groups = append(groups, sideWords(r.Words, "negative"))
		}
		for _, group := range groups {
			var hits []Word
			distinct := map[string]bool{}
			colors := map[string]bool{}
			for _, w := range group {
				hit := false
				for _, term := range rule.Terms {
					if core(term) == w.Word {
						hit = true
						distinct[w.Word] = true
					}
				}
				if rule.Kind == "color" {
					for _, color := range sortedKeys(rule.Colors) {
						for _, term := range rule.Colors[color] {
							if core(term) == w.Word {
								colors[color] = true
								hit = true
							}
						}
					}
				}
				if rule.Kind == "emphasis" && !w.Fixed && w.Strength > *rule.Limit {
					hit = true
				}
				if hit {
					hits = append(hits, w)
				}
			}
			triggered := false
			switch rule.Kind {
			case "forbidden", "emphasis":
				triggered = len(hits) > 0
			case "conflict":
				triggered = len(distinct) == 2
			case "color":
				triggered = len(colors) > 1
			case "risk":
				minimum := rule.Minimum
				if minimum == 0 {
					minimum = len(rule.Terms)
				}
				triggered = len(distinct) >= minimum
			}
			if triggered {
				out = append(out, diagnostic(rule.ID, rule.Severity, lang.T("検証規則に一致しました")+" "+rule.Target, hits))
			}
		}
	}
	return out
}

func (g *Catalog) Lint() []Diagnostic {
	out := []Diagnostic{}
	inspect := func(context string, p Plan, partial bool) {
		r, err := g.build(p, partial)
		ds := r.Diagnostics
		if err != nil && len(ds) == 0 {
			ds = []Diagnostic{diagnostic("core.build", "error", err.Error(), nil)}
		}
		for _, d := range ds {
			d.Context = context
			out = append(out, d)
		}
	}
	for _, l := range g.c.Layers {
		for _, id := range sortedKeys(l.Items) {
			p := Plan{Select: map[string][]string{}}
			for _, other := range g.c.Layers {
				p.Select[other.ID] = []string{}
			}
			seen := map[reference]bool{}
			var add func(reference)
			add = func(r reference) {
				if seen[r] {
					return
				}
				seen[r] = true
				p.Select[r.Layer] = append(p.Select[r.Layer], r.ID)
				for _, layer := range g.c.Layers {
					if layer.ID == r.Layer {
						for _, dep := range layer.Items[r.ID].Requires {
							add(dep)
						}
					}
				}
			}
			add(reference{l.ID, id})
			inspect("item/"+l.ID+"/"+id, p, true)
		}
	}
	for _, id := range sortedKeys(g.c.Scenes) {
		inspect("scene/"+id, g.c.Scenes[id], false)
	}
	return out
}

func (d Diagnostic) WriteText(w io.Writer) error {
	var b strings.Builder
	fmt.Fprintf(&b, "%s %s %s %s\n", d.Context, d.Severity, d.Rule, d.Message) //nolint:errcheck // strings.Builder writes always succeed.
	for _, word := range d.Words {
		fmt.Fprintf(&b, "  %s %s group=%s sources=%+v\n", word.Side, word.Word, word.Group, word.Sources) //nolint:errcheck // strings.Builder writes always succeed.
	}
	_, err := io.WriteString(w, b.String())
	return err
}
