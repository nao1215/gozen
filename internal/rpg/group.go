package rpg

import (
	"strings"

	"github.com/nao1215/gozen/internal/expr"
)

// Group exposes aggregate actor fields to host-supplied expression environments.
// It borrows its members and must be used with their ownership constraints.
type Group struct {
	members []expr.Object
}

// NewGroup is a group of members (each with at least hp, mhp, alive and states fields).
func NewGroup(members []expr.Object) *Group { return &Group{members: members} }

func num(o expr.Object, f string) (float64, bool) {
	v, ok := o.Field(f)
	if !ok {
		return 0, false
	}
	return expr.ToNumber(v)
}

func alive(o expr.Object) bool {
	v, _ := o.Field("alive")
	return expr.Truthy(v)
}

// Field implements expr.Object.
func (g *Group) Field(name string) (any, bool) {
	var standing []expr.Object
	for _, m := range g.members {
		if alive(m) {
			standing = append(standing, m)
		}
	}
	switch name {
	case "size":
		return float64(len(g.members)), true
	case "alive":
		return float64(len(standing)), true
	case "dead":
		return float64(len(g.members) - len(standing)), true
	case "members":
		out := make([]any, len(g.members))
		for i, m := range g.members {
			out[i] = m
		}
		return out, true
	case "hp_rate":
		var hp, mhp float64
		for _, m := range standing {
			h, _ := num(m, "hp")
			mh, _ := num(m, "mhp")
			hp += h
			mhp += mh
		}
		if mhp == 0 {
			return 0.0, true
		}
		return hp / mhp, true
	}
	if st, ok := strings.CutPrefix(name, "with_"); ok {
		n := 0
		for _, m := range standing {
			if s, ok := m.Field("states"); ok {
				if so, ok := s.(expr.Object); ok {
					if v, _ := so.Field(st); expr.Truthy(v) {
						n++
					}
				}
			}
		}
		return float64(n), true
	}
	if id, ok := strings.CutPrefix(name, "kind_"); ok {
		n := 0
		for _, m := range standing {
			if v, valid := m.Field("id"); valid && v == id {
				n++
			}
		}
		return float64(n), true
	}
	if it, ok := strings.CutPrefix(name, "wearing_"); ok {
		n := 0
		for _, m := range standing {
			if w, ok := m.Field("wears"); ok {
				if wo, ok := w.(expr.Object); ok {
					if v, _ := wo.Field(it); expr.Truthy(v) {
						n++
					}
				}
			}
		}
		return float64(n), true
	}
	for _, agg := range []string{"min_", "max_", "avg_", "sum_"} {
		f, ok := strings.CutPrefix(name, agg)
		if !ok {
			continue
		}
		if len(standing) == 0 {
			return 0.0, true
		}
		var acc float64
		for i, m := range standing {
			v, ok := num(m, f)
			if !ok {
				return nil, false
			}
			switch {
			case agg == "min_" && (i == 0 || v < acc), agg == "max_" && (i == 0 || v > acc):
				acc = v
			case agg == "avg_" || agg == "sum_":
				acc += v
			}
		}
		if agg == "avg_" {
			acc /= float64(len(standing))
		}
		return acc, true
	}
	return nil, false
}
