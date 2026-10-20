package promptgen

import (
	"cmp"
	"fmt"
	"io"
	"slices"
	"strings"

	lang "github.com/nao1215/gozen/internal/text"
)

type Source struct {
	Layer    string `json:"layer"`
	ID       string `json:"id"`
	Stage    string `json:"stage"`
	Priority int    `json:"priority"`
	Emphasis string `json:"emphasis"`
	Strength int    `json:"strength"`
}

type Word struct {
	Word     string   `json:"word"`
	Side     string   `json:"side"`
	Group    string   `json:"group,omitempty"`
	Priority int      `json:"priority"`
	Emphasis string   `json:"emphasis"`
	Strength int      `json:"strength"`
	Sources  []Source `json:"sources"`
	Fixed    bool     `json:"fixed,omitempty"`
}

type Dropped struct {
	Word
	Reason string   `json:"reason"`
	By     []Source `json:"by,omitempty"`
}

type Result struct {
	Positive           string       `json:"positive"`
	Negative           string       `json:"negative"`
	Words              []Word       `json:"words"`
	Dropped            []Dropped    `json:"dropped"`
	NegativeLimit      int          `json:"negative_limit"`
	NegativeCount      int          `json:"negative_count"`
	FixedNegativeCount int          `json:"fixed_negative_count"`
	Diagnostics        []Diagnostic `json:"diagnostics"`
	Dialect            string       `json:"dialect"`
}

type selected struct {
	l  layer
	id string
	it item
}

func (g *Catalog) selections(p Plan, partial bool) ([]selected, error) {
	layers := g.LayerIDs()
	for _, id := range sortedKeys(p.Select) {
		if !slices.Contains(layers, id) {
			return nil, problem(lang.T("未知の層です。%s"), id)
		}
	}
	var out []selected
	present := map[reference]bool{}
	for _, l := range g.c.Layers {
		ids, exists := p.Select[l.ID]
		if !exists {
			ids = l.Defaults
		}
		if (!partial && len(ids) < l.Min) || (l.Max > 0 && len(ids) > l.Max) {
			return nil, problem(lang.T("層の選択数が範囲外です。%s"), l.ID)
		}
		for _, id := range ids {
			it, ok := l.Items[id]
			r := reference{l.ID, id}
			if !ok || present[r] {
				return nil, problem(lang.T("項目が未知か重複しています。%s/%s"), l.ID, id)
			}
			present[r] = true
			out = append(out, selected{l, id, it})
		}
	}
	for _, s := range out {
		for _, r := range s.it.Requires {
			if !present[r] {
				return nil, problem(lang.T("%s/%s には %s/%s が必要です"), s.l.ID, s.id, r.Layer, r.ID)
			}
		}
		for _, r := range s.it.Conflicts {
			if present[r] {
				return nil, problem(lang.T("%s/%s と %s/%s は排他です"), s.l.ID, s.id, r.Layer, r.ID)
			}
		}
	}
	return out, nil
}

func (g *Catalog) Build(p Plan) (Result, error) {
	return g.build(p, false)
}

func (g *Catalog) build(p Plan, partial bool) (Result, error) {
	sel, err := g.selections(p, partial)
	if err != nil {
		return Result{}, err
	}
	r := Result{Words: []Word{}, Dropped: []Dropped{}, NegativeLimit: g.limit, Dialect: g.c.Dialect, Diagnostics: []Diagnostic{}}
	var raw []Word
	for _, s := range sel {
		for _, side := range []string{"positive", "negative"} {
			ts := s.it.Positive
			if side == "negative" {
				ts = s.it.Negative
			}
			for _, t := range ts {
				strength := g.emphasis(t)
				emphasis := g.c.Emphasis[strength]
				priority := g.c.Priorities[t.Priority]
				w := Word{
					Word: core(t.Word), Side: side, Priority: priority, Strength: strength, Emphasis: emphasis,
					Sources: []Source{{s.l.ID, s.id, t.Priority, priority, emphasis, strength}},
				}
				if side == "positive" && s.l.Separate {
					w.Group = s.l.ID + "/" + s.id
				}
				var by []Source
				for _, owner := range sel {
					for _, rep := range owner.it.Replaces {
						if rep.Side == side && core(rep.Word) == w.Word && (rep.Layer == "" || rep.Layer == s.l.ID) && (rep.ID == "" || rep.ID == s.id) {
							by = append(by, Source{Layer: owner.l.ID, ID: owner.id})
						}
					}
				}
				if len(by) > 0 {
					r.Dropped = append(r.Dropped, Dropped{w, "replaced", by})
				} else {
					raw = append(raw, w)
				}
			}
		}
	}

	var merged []Word
	indices := map[[3]string]int{}
	for _, w := range raw {
		key := [3]string{w.Side, w.Group, w.Word}
		if i, ok := indices[key]; ok {
			merged[i].Priority = min(merged[i].Priority, w.Priority)
			if w.Strength > merged[i].Strength {
				merged[i].Strength = w.Strength
				merged[i].Emphasis = w.Emphasis
			}
			merged[i].Sources = append(merged[i].Sources, w.Sources...)
		} else {
			indices[key] = len(merged)
			merged = append(merged, w)
		}
	}

	pos := map[string][]int{}
	neg := map[string]int{}
	for i, w := range merged {
		if w.Side == "positive" {
			pos[w.Word] = append(pos[w.Word], i)
		} else {
			neg[w.Word] = i
		}
	}
	removed := map[int][]Source{}
	for _, word := range sortedKeys(pos) {
		n, exists := neg[word]
		if !exists {
			continue
		}
		best := pos[word][0]
		for _, i := range pos[word] {
			if merged[i].Priority < merged[best].Priority {
				best = i
			}
		}
		if merged[best].Priority == merged[n].Priority {
			ds := []Diagnostic{diagnostic("core.polarity", "error", fmt.Sprintf(lang.T("正負の優先度が同点です。%s"), word), []Word{merged[best], merged[n]})}
			return Result{Diagnostics: ds}, &ValidationError{ds}
		}
		if merged[best].Priority < merged[n].Priority {
			removed[n] = merged[best].Sources
		} else {
			for _, i := range pos[word] {
				removed[i] = merged[n].Sources
			}
		}
	}
	var positives, negatives []Word
	for i, w := range merged {
		if by, ok := removed[i]; ok {
			r.Dropped = append(r.Dropped, Dropped{w, "conflict", by})
			continue
		}
		if w.Side == "positive" {
			positives = append(positives, w)
		} else {
			negatives = append(negatives, w)
		}
	}
	slices.SortStableFunc(negatives, func(a, b Word) int { return cmp.Compare(a.Priority, b.Priority) })
	mandatory := 0
	for _, w := range negatives {
		if w.Priority == 0 {
			mandatory++
		}
	}
	if mandatory > g.limit {
		return Result{}, problem(lang.T("段0の負語が上限を超えています。%d > %d"), mandatory, g.limit)
	}
	if len(negatives) > g.limit {
		for _, w := range negatives[g.limit:] {
			r.Dropped = append(r.Dropped, Dropped{w, "limit", nil})
		}
		negatives = negatives[:g.limit]
	}
	r.NegativeCount = len(negatives)

	person := hasPerson(sel)
	for _, side := range []string{"positive", "negative"} {
		if side == "positive" && !person {
			continue
		}
		for _, text := range guardWords(side) {
			w := Word{Word: text, Side: side, Priority: -1, Strength: 1, Fixed: true, Sources: []Source{}}
			if side == "positive" {
				positives = append(positives, w)
			} else {
				negatives = append(negatives, w)
				r.FixedNegativeCount++
			}
		}
	}
	r.Positive = g.renderPositive(positives, sel)
	r.Negative = g.dialect.Render([]Block{{Words: negatives}})
	r.Words = append(r.Words, positives...)
	r.Words = append(r.Words, negatives...)
	r.Diagnostics = g.validateResult(r)
	if hasErrors(r.Diagnostics) {
		r.Positive = ""
		r.Negative = ""
		return r, &ValidationError{r.Diagnostics}
	}
	return r, nil
}

func (g *Catalog) renderPositive(ws []Word, sel []selected) string {
	boundaries := map[string]string{}
	for _, s := range sel {
		boundary := s.l.Separator
		if boundary == "" {
			boundary = g.c.Separator
		}
		boundaries[s.l.ID+"/"+s.id] = boundary
	}
	var blocks []Block
	previous := ""
	for _, w := range ws {
		if len(blocks) == 0 || w.Group != previous {
			boundary := g.c.Separator
			if w.Group != "" {
				boundary = boundaries[w.Group]
			}
			if w.Fixed {
				boundary = "inline"
			}
			blocks = append(blocks, Block{Boundary: boundary})
		}
		blocks[len(blocks)-1].Words = append(blocks[len(blocks)-1].Words, w)
		previous = w.Group
	}
	return g.dialect.Render(blocks)
}

func (r Result) WriteText(w io.Writer) error {
	var b strings.Builder
	fmt.Fprintf(&b, "positive: %s\nnegative: %s\n", r.Positive, r.Negative)
	fmt.Fprintf(&b, lang.T("負語数。設定 %d/%d、固定 %d\n"), r.NegativeCount, r.NegativeLimit, r.FixedNegativeCount)
	for _, word := range r.Words {
		fmt.Fprintf(&b, "%s %s [%s] priority=%d fixed=%t\n", word.Side, word.Word, word.Group, word.Priority, word.Fixed)
		for _, s := range word.Sources {
			fmt.Fprintf(&b, "  %s/%s stage=%s priority=%d emphasis=%s strength=%d\n", s.Layer, s.ID, s.Stage, s.Priority, s.Emphasis, s.Strength)
		}
	}
	for _, d := range r.Dropped {
		fmt.Fprintf(&b, lang.T("脱落 %s %s reason=%s sources=%+v by=%+v\n"), d.Side, d.Word.Word, d.Reason, d.Sources, d.By)
	}
	for _, d := range r.Diagnostics {
		fmt.Fprintf(&b, "%s %s %s\n", d.Severity, d.Rule, d.Message)
	}
	_, err := io.WriteString(w, b.String())
	return err
}
