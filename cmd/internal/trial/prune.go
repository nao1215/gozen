package trial

import (
	"fmt"
	"strings"

	"github.com/goccy/go-yaml"

	"github.com/nao1215/gozen/expr"
)

func (a *analysis) blocked() []any {
	out := []any{}
	for _, c := range a.cfg.Blocked {
		out = append(out, cloneNode(c))
	}
	return append(out, yaml.MapSlice{{Key: "end", Value: true}})
}

func (a *analysis) gates(fs files) (int, error) {
	n := 0
	events := map[string]yaml.MapSlice{}
	for _, sp := range fs.section("events") {
		for _, it := range sp.m {
			if m, ok := it.Value.(yaml.MapSlice); ok {
				events[keyOf(it.Key)] = m
			}
		}
	}
	var walk func(v any) any
	walk = func(v any) any {
		switch x := v.(type) {
		case yaml.MapSlice:
			for i, it := range x {
				x[i].Value = walk(it.Value)
			}
			return x
		case []any:
			var out []any
			for i, e := range x {
				m, ok := e.(yaml.MapSlice)
				if !ok {
					out = append(out, walk(e))
					continue
				}
				if c, ok := get(m, "call"); ok {
					if s, _ := str(c); s != "" && match(a.cfg.Cut, s) {
						out = append(out, a.blocked()...)
						n++
						continue
					}
				}
				if g := a.diveGate(m); g != nil {

					at := len(out)
					names := refNames(a.exprs.parse(floorOf(m)))
					if !setsAny(x[:i], names, events, map[string]bool{}) {
						at = 0
					}
					out = append(out[:at], append([]any{g}, out[at:]...)...)
					n++
				}
				out = append(out, walk(m))
			}
			return out
		}
		return v
	}
	for _, p := range fs.names() {
		fs[p] = walk(fs[p])
	}

	for _, sp := range fs.section("events") {
		keep := yaml.MapSlice{}
		for _, it := range sp.m {
			if !match(a.cfg.Cut, keyOf(it.Key)) {
				keep = append(keep, it)
			}
		}
		fs.setSection(sp.file, "events", keep)
	}
	return n, nil
}

func floorOf(m yaml.MapSlice) any {
	if f, ok := get(m, "floor"); ok {
		return f
	}
	return 1
}

func (a *analysis) diveGate(m yaml.MapSlice) any {
	d, ok := get(m, "dive")
	if !ok {
		return nil
	}
	loc, _ := str(d)
	limit, ok := a.cfg.DiveMax[loc]
	if !ok {
		return nil
	}
	f := floorOf(m)
	src, _ := str(f)
	r := a.eval(f, nil)
	if r.Num && !r.Any && !r.StrAny && len(r.Strs) == 0 && r.Hi <= float64(limit) {
		return nil
	}
	return yaml.MapSlice{
		{Key: "if", Value: fmt.Sprintf("(%s) > %d", src, limit)},
		{Key: "then", Value: a.blocked()},
	}
}

func refNames(e *expr.Expr) map[string]bool {
	out := map[string]bool{}
	if e == nil {
		return out
	}
	for _, r := range e.Refs() {
		if (r.Name == "var" || r.Name == "flag") && len(r.Path) > 0 {
			out[r.Name+"."+r.Path[0].Name] = true
		}
		if r.Name == "floor" && len(r.Path) == 0 {
			out["var.floor"] = true
		}
	}
	return out
}

func setsAny(v any, names map[string]bool, events map[string]yaml.MapSlice, seen map[string]bool) bool {
	switch x := v.(type) {
	case yaml.MapSlice:
		for _, it := range x {
			k := keyOf(it.Key)
			if k == "set" {
				if m, ok := it.Value.(yaml.MapSlice); ok {
					for _, s := range m {
						if names[keyOf(s.Key)] {
							return true
						}
					}
				}
			}
			if k == "call" {
				if s, _ := str(it.Value); s != "" && !seen[s] {
					seen[s] = true
					if ev, ok := events[s]; ok && setsAny(ev, names, events, seen) {
						return true
					}
				}
			}
			if setsAny(it.Value, names, events, seen) {
				return true
			}
		}
	case []any:
		for _, e := range x {
			if setsAny(e, names, events, seen) {
				return true
			}
		}
	}
	return false
}

type pruner struct {
	a *analysis
	d domains
}

func (p *pruner) cond(v any) expr.Range { return p.a.eval(v, p.d) }

func (p *pruner) node(v any) any {
	switch x := v.(type) {
	case []any:
		return p.list(x, "")
	case yaml.MapSlice:
		return p.mapping(x)
	}
	return v
}

func (p *pruner) mapping(m yaml.MapSlice) yaml.MapSlice {
	out := make(yaml.MapSlice, 0, len(m))
	for _, it := range m {
		v := it.Value
		if l, ok := v.([]any); ok {
			v = p.list(l, keyOf(it.Key))
		} else {
			v = p.node(v)
		}
		out = append(out, yaml.MapItem{Key: it.Key, Value: v})
	}
	return out
}

func isIf(m yaml.MapSlice) bool {
	if _, ok := get(m, "if"); !ok {
		return false
	}
	for _, it := range m {
		switch keyOf(it.Key) {
		case "if", "then", "else":
		default:
			return false
		}
	}
	return true
}

func (p *pruner) list(l []any, key string) []any {
	out := []any{}
	for _, e := range l {
		m, ok := e.(yaml.MapSlice)
		if !ok {
			out = append(out, p.node(e))
			continue
		}
		if isIf(m) {
			c, _ := get(m, "if")
			r := p.cond(c)
			branch := ""
			switch {
			case !r.CanBeFalse():
				branch = "then"
			case !r.CanBeTrue():
				branch = "else"
			}
			if branch != "" {
				if b, ok := get(m, branch); ok {
					if bl, ok := b.([]any); ok {
						out = append(out, p.list(bl, branch)...)
					}
				}
				continue
			}
		}
		if w, ok := get(m, "when"); ok && !p.cond(w).CanBeTrue() {
			continue
		}
		if key == "place" || key == "roamers" {
			if c, ok := get(m, "count"); ok {
				r := p.cond(c)
				if r.Num && !r.Any && !r.StrAny && len(r.Strs) == 0 && !r.True && r.Hi <= 0 {
					continue
				}
			}
		}
		pm := p.mapping(m)
		if ch, ok := get(pm, "choice"); ok {
			if cl, ok := ch.([]any); ok && len(cl) == 0 {
				continue
			}
		}
		out = append(out, pm)
	}
	return out
}

func (p *pruner) prune(base files) files {
	out := files{}
	for _, name := range base.names() {
		out[name] = p.node(base[name])
	}
	for _, sp := range out.section("events") {
		keep := yaml.MapSlice{}
		for _, it := range sp.m {
			ev, ok := it.Value.(yaml.MapSlice)
			if ok {
				if t, _ := get(ev, "trigger"); t != nil && t != "" {
					if w, ok := get(ev, "when"); ok && !p.cond(w).CanBeTrue() {
						continue
					}
				}
			}
			keep = append(keep, it)
		}
		out.setSection(sp.file, "events", keep)
	}
	for _, sp := range out.section("locations") {
		for i, it := range sp.m {
			l, ok := it.Value.(yaml.MapSlice)
			if !ok {
				continue
			}
			if d, ok := get(l, "dungeon"); ok {
				if dm, ok := d.(yaml.MapSlice); ok {
					sp.m[i].Value = set(l, "dungeon", unusedTiles(dm))
				}
			}
		}
	}
	return out
}

func unusedTiles(d yaml.MapSlice) yaml.MapSlice {
	if _, ok := get(d, "map"); ok {
		return d
	}
	g, ok := get(d, "generate")
	gm, ok2 := g.(yaml.MapSlice)
	if !ok || !ok2 {
		return d
	}
	t, _ := get(d, "tiles")
	tiles, ok := t.(yaml.MapSlice)
	if !ok {
		return d
	}
	for {
		used := map[string]bool{}
		if s, ok := get(gm, "start"); ok {
			v, _ := str(s)
			used[v] = true
		}
		if pl, ok := get(gm, "place"); ok {
			l := sequence(pl)
			for _, e := range l {
				if m, ok := e.(yaml.MapSlice); ok {
					c, _ := get(m, "char")
					v, _ := str(c)
					used[v] = true
				}
			}
		}
		for _, it := range tiles {
			if m, ok := it.Value.(yaml.MapSlice); ok {
				for _, k := range []string{"else", "after"} {
					if c, ok := get(m, k); ok {
						v, _ := str(c)
						used[v] = true
					}
				}
			}
		}
		keep := yaml.MapSlice{}
		for _, it := range tiles {
			if used[keyOf(it.Key)] {
				keep = append(keep, it)
			}
		}
		if len(keep) == len(tiles) {
			break
		}
		tiles = keep
	}
	return set(d, "tiles", tiles)
}

var deletable = []string{"events", "troops", "enemies", "locations", "skills", "ai", "shops", "songs"}

var refKeys = map[string]string{
	"call": "events", "event": "events", "after_win": "events",
	"battle": "troops", "troop": "troops",
	"members": "enemies", "summon": "enemies",
	"move": "locations", "to": "locations", "dive": "locations", "location": "locations",
	"music": "songs", "play": "songs", "battle_music": "songs", "title_music": "songs", "song": "songs",
	"skill": "skills", "use": "skills", "skills": "skills", "attack_skill": "skills", "guard_skill": "skills",
	"ai": "ai", "shop": "shops",
}

var (
	engineSongs  = []string{"battle"}
	engineSkills = []string{"attack", "guard"}
)

type ref struct{ sec, id string }

func refs(v any, add func(ref)) {
	switch x := v.(type) {
	case yaml.MapSlice:
		for _, it := range x {
			k := keyOf(it.Key)
			if sec, ok := refKeys[k]; ok {
				scalar := func(e any) {
					if s, ok := e.(string); ok && s != "" {
						if sec == "songs" {
							s, _, _ = strings.Cut(s, "/")
						}
						add(ref{sec, s})
					}
				}
				scalar(it.Value)
				if l, ok := it.Value.([]any); ok {
					for _, e := range l {
						scalar(e)
					}
				}
			}
			refs(it.Value, add)
		}
	case []any:
		for _, e := range x {
			refs(e, add)
		}
	}
}

type defs map[string]map[string]any

func defsOf(fs files) defs {
	out := defs{}
	for _, s := range deletable {
		out[s] = map[string]any{}
	}
	for _, sec := range deletable {
		if sec == "songs" {
			continue
		}
		for _, sp := range fs.section(sec) {
			for _, it := range sp.m {
				out[sec][keyOf(it.Key)] = it.Value
			}
		}
	}
	for _, sp := range fs.section("music") {
		if s, ok := get(sp.m, "songs"); ok {
			if sm, ok := s.(yaml.MapSlice); ok {
				for _, it := range sm {
					out["songs"][keyOf(it.Key)] = it.Value
				}
			}
		}
	}
	return out
}

func reach(fs files, scenes []yaml.MapSlice, asides []yaml.MapSlice, allow map[string]bool, why map[string]string) (map[string]map[string]bool, []yaml.MapSlice) {
	ds := defsOf(fs)
	got := map[string]map[string]bool{}
	for _, s := range deletable {
		got[s] = map[string]bool{}
	}

	triggered := map[string][]string{}
	for _, id := range sortedKeys(ds["events"]) {
		ev := mapping(ds["events"][id])
		if t, _ := get(ev, "trigger"); t != nil && t != "" {
			l, _ := get(ev, "location")
			s, _ := str(l)
			triggered[s] = append(triggered[s], id)
		}
	}
	faces := map[string]bool{}
	whos := map[string]bool{}
	var queue []ref
	owner := "game.yaml"
	add := func(r ref) {
		if got[r.sec] == nil || got[r.sec][r.id] {
			return
		}
		if _, ok := ds[r.sec][r.id]; !ok {
			return
		}
		if r.sec == "events" && allow != nil && !allow[r.id] {
			return
		}
		got[r.sec][r.id] = true
		if why != nil {
			why[r.sec+"."+r.id] = owner
		}
		queue = append(queue, r)
	}
	scan := func(v any) {
		refs(v, add)
		speakers(v, faces, whos)
	}

	g := del(del(fs.game(), "gallery"), "asides")
	scan(g)
	gal, _ := get(fs.game(), "gallery")
	gm := mapping(gal)
	if m, ok := get(gm, "music"); ok {
		scan(yaml.MapSlice{{Key: "music", Value: m}})
	}
	for _, s := range engineSongs {
		add(ref{"songs", s})
	}
	for _, s := range engineSkills {
		add(ref{"skills", s})
	}
	for _, p := range fs.names() {
		if p == "game.yaml" {
			continue
		}
		m := mapping(fs[p])
		for _, it := range m {
			sec := keyOf(it.Key)
			owner = p + " " + sec
			if sec == "music" {
				if mm, ok := it.Value.(yaml.MapSlice); ok {
					scan(del(mm, "songs"))
				}
				continue
			}
			if !isDeletable(sec) {
				scan(it.Value)
			}
		}
	}
	for _, sc := range scenes {
		t, _ := get(sc, "title")
		owner = fmt.Sprintf("gallery.scenes %v", t)
		scan(sc)
	}
	var active []yaml.MapSlice
	used := make([]bool, len(asides))
	for {
		for len(queue) > 0 {
			r := queue[0]
			queue = queue[1:]
			v := ds[r.sec][r.id]
			owner = r.sec + "." + r.id
			if r.sec == "events" {
				ev := mapping(v)
				d, _ := get(ev, "do")
				scan(d)
				continue
			}
			scan(v)
			if r.sec == "locations" {
				for _, id := range triggered[r.id] {
					add(ref{"events", id})
				}
			}
		}
		more := false
		for i, as := range asides {
			if used[i] || !asideActive(as, faces, whos) {
				continue
			}
			used[i] = true
			more = true
			owner = fmt.Sprintf("asides[%d]", i)
			active = append(active, as)
			c, _ := get(as, "call")
			scan(yaml.MapSlice{{Key: "call", Value: c}})
		}
		if !more {
			break
		}
	}
	return got, active
}

func isDeletable(sec string) bool {
	for _, s := range deletable {
		if s == sec {
			return true
		}
	}
	return false
}

func speakers(v any, faces, whos map[string]bool) {
	switch x := v.(type) {
	case yaml.MapSlice:
		if s, ok := get(x, "say"); ok {
			if sm, ok := s.(yaml.MapSlice); ok {
				if f, ok := get(sm, "face"); ok {
					if fs, _ := str(f); fs != "" {
						faces[cleanPath(fs)] = true
					}
				}
				if w, ok := get(sm, "who"); ok {
					if ws, _ := str(w); ws != "" {
						whos[ws] = true
					}
				}
			}
		}
		for _, it := range x {
			speakers(it.Value, faces, whos)
		}
	case []any:
		for _, e := range x {
			speakers(e, faces, whos)
		}
	}
}

func asideActive(as yaml.MapSlice, faces, whos map[string]bool) bool {
	if a, ok := get(as, "actor"); ok {
		if s, _ := str(a); s != "" {
			return true
		}
	}
	if f, ok := get(as, "face"); ok {
		pat, _ := str(f)
		pat = cleanPath(pat)
		for face := range faces {
			if ok, err := pathMatch(pat, face); err == nil && ok {
				return true
			}
		}
	}
	if n, ok := get(as, "name"); ok {
		s, _ := str(n)
		return whos[s]
	}
	return false
}

func keepOnly(fs files, got map[string]map[string]bool) {
	for _, sec := range deletable {
		if sec == "songs" {
			continue
		}
		for _, sp := range fs.section(sec) {
			keep := yaml.MapSlice{}
			for _, it := range sp.m {
				if got[sec][keyOf(it.Key)] {
					keep = append(keep, it)
				}
			}
			fs.setSection(sp.file, sec, keep)
		}
	}
	for _, sp := range fs.section("music") {
		s, ok := get(sp.m, "songs")
		sm, ok2 := s.(yaml.MapSlice)
		if !ok || !ok2 {
			continue
		}
		keep := yaml.MapSlice{}
		for _, it := range sm {
			if got["songs"][keyOf(it.Key)] {
				keep = append(keep, it)
			}
		}
		m := set(sp.m, "songs", keep)

		fs.setSection(sp.file, "music", m)
	}
}

func dropCalls(fs files, events map[string]bool) {
	var walk func(v any) any
	walk = func(v any) any {
		switch x := v.(type) {
		case yaml.MapSlice:
			for i, it := range x {
				x[i].Value = walk(it.Value)
			}
			return x
		case []any:
			out := []any{}
			for _, e := range x {
				if m, ok := e.(yaml.MapSlice); ok && len(m) == 1 && keyOf(m[0].Key) == "call" {
					if s, _ := str(m[0].Value); !events[s] {
						continue
					}
				}
				out = append(out, walk(e))
			}
			return out
		}
		return v
	}
	for _, p := range fs.names() {
		fs[p] = walk(fs[p])
	}
}
