package trial

import (
	"fmt"
	"math"
	"path"
	"strings"

	"github.com/goccy/go-yaml"

	"github.com/nao1215/gozen/battle"
	"github.com/nao1215/gozen/expr"
	"github.com/nao1215/gozen/internal/layer3/dungeon"
)

func pathMatch(p, name string) (bool, error) { return path.Match(p, name) }

var engineVars = []string{
	dungeon.VarFloor, dungeon.VarSeed, dungeon.VarDeep, dungeon.VarExplored, dungeon.VarLeft,
	dungeon.VarRoamCalm, dungeon.VarRoamerFled, dungeon.VarRoamerBeaten, dungeon.VarHuntFrom, dungeon.VarHuntNear, "gauge",
}

var enginePrefixes = []string{battle.VarLastKill, "kill_", "known_"}

type exprCache map[string]*expr.Expr

func (c exprCache) parse(v any) *expr.Expr {
	s, ok := str(v)
	if !ok {
		return nil
	}
	if e, ok := c[s]; ok {
		return e
	}
	e, err := expr.Parse(s)
	if err != nil {
		e = nil
	}
	c[s] = e
	return e
}

type domains map[string]expr.Range

type analysis struct {
	cfg      *Config
	assume   map[string]expr.Range
	analyzed map[string]bool
	exprs    exprCache
}

func (a *analysis) look(d domains) func(expr.Ref) expr.Range {
	return func(r expr.Ref) expr.Range {
		if r.Name == "floor" && len(r.Path) == 0 {
			if v, ok := a.assume["var."+dungeon.VarFloor]; ok {
				return v
			}
			return expr.Top()
		}
		if (r.Name != "var" && r.Name != "flag") || len(r.Path) != 1 || r.Path[0].Name == "" {
			return expr.Top()
		}
		k := r.Name + "." + r.Path[0].Name
		if v, ok := a.assume[k]; ok {
			return v
		}
		if v, ok := d[k]; ok {
			return v
		}
		return expr.Top()
	}
}

func (a *analysis) eval(v any, d domains) expr.Range {
	e := a.exprs.parse(v)
	if e == nil {
		return expr.Top()
	}
	return e.Abstract(a.look(d))
}

func initial(k string) expr.Range {
	if strings.HasPrefix(k, "flag.") {
		return expr.Boolean(true, false)
	}
	return expr.Number(0)
}

type assignment struct {
	key   string
	val   any
	owner string
}

func assignments(v any, out *[]assignment, owner string) {
	switch x := v.(type) {
	case yaml.MapSlice:
		for _, it := range x {
			if keyOf(it.Key) == "set" {
				if m, ok := it.Value.(yaml.MapSlice); ok {
					for _, s := range m {
						k := keyOf(s.Key)
						if strings.HasPrefix(k, "var.") || strings.HasPrefix(k, "flag.") {
							*out = append(*out, assignment{k, s.Value, owner})
						}
					}
				}
			}
			assignments(it.Value, out, owner)
		}
	case []any:
		for _, e := range x {
			assignments(e, out, owner)
		}
	}
}

func suggestNames(v any, out map[string]bool) {
	switch x := v.(type) {
	case yaml.MapSlice:
		for _, it := range x {
			k := keyOf(it.Key)
			if strings.HasPrefix(k, "stone_") || strings.HasPrefix(k, "sp_") {
				if s, ok := it.Value.(string); ok && s != "" {
					out[s] = true
				}
			}
			suggestNames(it.Value, out)
		}
	case []any:
		for _, e := range x {
			suggestNames(e, out)
		}
	}
}

func newAnalysis(cfg *Config, base files) *analysis {
	a := &analysis{cfg: cfg, assume: map[string]expr.Range{}, analyzed: map[string]bool{}, exprs: exprCache{}}
	for k, v := range cfg.Assume {
		r, err := assumed(v)
		if err != nil {
			continue
		}
		a.assume[k] = r
	}
	var as []assignment
	suggest := map[string]bool{}
	for _, p := range base.names() {
		assignments(base[p], &as, p)
		suggestNames(base[p], suggest)
	}

	for _, sc := range galleryScenes(base) {
		if f, ok := get(sc, "flag"); ok {
			if s, _ := str(f); s != "" {
				as = append(as, assignment{"flag." + s, true, "gallery"})
			}
		}
	}
	for _, s := range as {
		a.analyzed[s.key] = true
	}
	for k := range a.analyzed {
		name := strings.TrimPrefix(strings.TrimPrefix(k, "var."), "flag.")
		drop := match(cfg.Unknown, k)
		if strings.HasPrefix(k, "var.") {
			for _, v := range engineVars {
				drop = drop || name == v
			}
			for _, p := range enginePrefixes {
				drop = drop || strings.HasPrefix(name, p)
			}
			for s := range suggest {
				drop = drop || name == s || strings.HasPrefix(name, s+"_")
			}
		}
		if _, ok := a.assume[k]; ok || drop {
			delete(a.analyzed, k)
		}
	}
	return a
}

func (a *analysis) bottom() domains {
	d := domains{}
	for k := range a.analyzed {
		d[k] = initial(k)
	}
	return d
}

func (d domains) equal(o domains) bool {
	if len(d) != len(o) {
		return false
	}
	for k, v := range d {
		if w, ok := o[k]; !ok || !v.Equal(w) {
			return false
		}
	}
	return true
}

func (a *analysis) domains(fs files, gallery bool, start domains, why map[string][]string) domains {
	var as []assignment
	for _, p := range fs.names() {
		v := fs[p]
		if p == "game.yaml" {
			assignments(del(fs.game(), "gallery"), &as, p)
			continue
		}
		m, ok := v.(yaml.MapSlice)
		if !ok {
			continue
		}
		for _, sec := range m {
			if sm, ok := sec.Value.(yaml.MapSlice); ok {
				for _, it := range sm {
					assignments(it.Value, &as, keyOf(sec.Key)+"."+keyOf(it.Key))
				}
			} else {
				assignments(sec.Value, &as, p+" "+keyOf(sec.Key))
			}
		}
	}
	if gallery {
		for _, sc := range galleryScenes(fs) {
			t, _ := get(sc, "title")
			owner := fmt.Sprintf("gallery.scenes %v", t)
			if f, ok := get(sc, "flag"); ok {
				if s, _ := str(f); s != "" {
					as = append(as, assignment{"flag." + s, true, owner})
				}
			}
			if m, ok := get(sc, "set"); ok {
				assignments(yaml.MapSlice{{Key: "set", Value: m}}, &as, owner)
			}
		}
	}
	d := domains{}
	for k := range a.analyzed {
		d[k] = initial(k)
		if v, ok := start[k]; ok {
			d[k] = d[k].Union(v)
		}
	}
	for round := 0; round < 40; round++ {
		changed := map[string]bool{}
		for _, s := range as {
			if !a.analyzed[s.key] {
				continue
			}
			v := a.eval(s.val, d)
			if strings.HasPrefix(s.key, "flag.") {
				v = expr.Boolean(v.CanBeFalse(), v.CanBeTrue())
			} else {
				v = asVar(v)
			}
			n := d[s.key].Union(v)
			if !n.Equal(d[s.key]) {
				d[s.key] = n
				changed[s.key] = true
				if why != nil {
					why[s.key] = append(why[s.key], s.owner+" ("+v.String()+")")
				}
			}
		}
		if len(changed) == 0 {
			break
		}
		if round >= 8 {
			for k := range changed {
				d[k] = widen(d[k])
			}
		}
	}
	return d
}

func asVar(v expr.Range) expr.Range {
	if v.Any {
		return v
	}
	out := expr.Range{Strs: v.Strs, StrAny: v.StrAny}
	if v.Num {
		out = out.Union(expr.Range{Num: true, Lo: v.Lo, Hi: v.Hi, Set: v.Set})
	}
	if v.False || v.Nil {
		out = out.Union(expr.Number(0))
	}
	if v.True {
		out = out.Union(expr.Number(1))
	}
	return out
}

func widen(r expr.Range) expr.Range {
	if r.Any {
		return r
	}
	if r.Num {
		r.Lo, r.Hi, r.Set = math.Inf(-1), math.Inf(1), nil
	}
	if len(r.Strs) > 0 {
		r.Strs, r.StrAny = nil, true
	}
	return r
}

func galleryScenes(fs files) []yaml.MapSlice {
	g, _ := get(fs.game(), "gallery")
	gm := mapping(g)
	sc, _ := get(gm, "scenes")
	l := sequence(sc)
	var out []yaml.MapSlice
	for _, e := range l {
		if m, ok := e.(yaml.MapSlice); ok {
			out = append(out, m)
		}
	}
	return out
}
