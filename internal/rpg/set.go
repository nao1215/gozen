package rpg

import (
	"encoding/json"
	"fmt"
	"maps"
	"slices"
	"strings"

	"github.com/nao1215/gozen/internal/expr"
	"github.com/nao1215/gozen/internal/layer1/safeint"
	data "github.com/nao1215/gozen/internal/layer2/gamedata"
)

// setOther handles the left-hand sides of set other than flag and var (spec version 2, ADR 0021). It returns false for an unhandled left-hand side.
// gold, item.<ID>, party, location, actor.<ID>.<hp|mp|exp|level|sp|tactics>.
// Arrival handling and the notice for a member joining do not happen (it just sets the state to that value).
func (s *State) setOther(path string, v any) (bool, error) {
	number := func() (int, error) {
		f, ok := expr.ToNumber(v)
		if !ok {
			return 0, fmt.Errorf("set %s: want a number, got %v", path, v)
		}
		return safeint.From(f), nil
	}
	switch {
	case path == "gold":
		n, err := number()
		if err == nil {
			s.Gold = max(0, n)
		}
		return true, err
	case path == "calm": // stops dungeon encounter battles (trial play). Not saved
		s.Calm = expr.Truthy(v)
		return true, nil
	case path == "pos.x" || path == "pos.y" || path == "steps":
		n, err := number()
		if err != nil {
			return true, err
		}
		switch path {
		case "pos.x":
			s.Pos.X = n
		case "pos.y":
			s.Pos.Y = n
		default:
			s.Steps = max(0, n)
		}
		if path != "steps" {
			s.Pos.In = s.Location
		}
		return true, nil
	case path == "pos.facing":
		f := data.FacingIndex(fmt.Sprint(v))
		if f < 0 {
			return true, fmt.Errorf("set pos.facing: %v is not north, east, south or west", v)
		}
		s.Pos.Facing = f
		return true, nil
	case path == "roamers": // none: the floor's strong enemies disappear (trial play)
		if v != "none" {
			return true, fmt.Errorf("set roamers: only none, not %v", v)
		}
		s.Roamers = nil
		return true, nil
	case path == "location":
		loc, ok := v.(string)
		if !ok || s.DB.Locations[loc] == nil {
			return true, fmt.Errorf("set location: %v is not a location", v)
		}
		s.Location = loc
		s.Spot = ""
		return true, nil
	case path == "spot": // map point / scene standing position (--dev). Empty means the place's entrance
		id, ok := v.(string)
		if !ok || (id != "" && !s.DB.Locations[s.Location].Spotted(id)) {
			return true, fmt.Errorf("set spot: %v is not a point or a spot of %s", v, s.Location)
		}
		s.Spot = id
		return true, nil
	case path == "party":
		return true, s.setParty(v)
	case strings.HasPrefix(path, "item."):
		id := strings.TrimPrefix(path, "item.")
		if s.DB.Items[id] == nil {
			return true, fmt.Errorf("set %s: no item %q", path, id)
		}
		n, err := number()
		if err != nil {
			return true, err
		}
		if n <= 0 {
			delete(s.Items, id)
		} else {
			s.Items[id] = n
		}
		return true, nil
	case strings.HasPrefix(path, "actor."):
		id, field, _ := strings.Cut(strings.TrimPrefix(path, "actor."), ".")
		a := s.Actors[id]
		if a == nil {
			return true, fmt.Errorf("set %s: %s has not joined", path, id)
		}
		if field == "tactics" {
			return true, s.setTactics(a, v)
		}
		if !slices.Contains(data.DevSetFields, field) {
			return true, fmt.Errorf("cannot set %q", path)
		}
		n, err := number()
		if err != nil {
			return true, err
		}
		a.setValue(field, max(0, n))
		return true, nil
	}
	return false, nil
}

// setValue writes an ability value directly (only with --dev; the caller checks).
func (a *Actor) setValue(field string, n int) {
	switch field {
	case "hp":
		a.HP = min(n, a.Stat("mhp"))
	case "mp":
		a.MP = min(n, a.Stat("mmp"))
	case "sp":
		a.SP = n
	case "exp":
		if n > a.Exp {
			a.GainExp(n - a.Exp)
		} else {
			a.Exp = n
		}
	case "level":
		n = min(max(1, n), a.Class().Max)
		if n > a.Level {
			a.GainExp(max(0, a.ExpFor(n)-a.Exp))
		} else if n < a.Level {
			a.Level, a.Exp = n, a.ExpFor(n)
			a.clamp()
		}
	}
}

func (s *State) setParty(v any) error {
	var list []any
	switch x := v.(type) {
	case []any:
		list = x
	case string: // an expression cannot hold a list, so a string of IDs separated by , or whitespace ('scout, guide')
		for _, id := range strings.FieldsFunc(x, func(r rune) bool { return r == ',' || r == ' ' }) {
			list = append(list, id)
		}
	default:
		return fmt.Errorf("set party: want actor ids separated by commas, got %v", v)
	}
	if len(list) == 0 || len(list) > s.DB.Game.MaxParty {
		return fmt.Errorf("set party: 1 to %d actors, got %d", s.DB.Game.MaxParty, len(list))
	}
	var party []string
	for _, x := range list {
		id, ok := x.(string)
		if !ok || s.DB.Actors[id] == nil || slices.Contains(party, id) {
			return fmt.Errorf("set party: %v is not an actor (or is written twice)", x)
		}
		party = append(party, id)
	}
	for _, id := range party {
		if s.Actors[id] == nil {
			s.Actors[id] = NewActor(s.DB, id)
		}
	}
	s.Party = party
	return nil
}

// setTactics sets the tactics to auto, manual, or a preset name.
func (s *State) setTactics(a *Actor, v any) error {
	name, ok := v.(string)
	if !ok {
		return fmt.Errorf("set actor.%s.tactics: want auto, manual or a preset, got %v", a.ID, v)
	}
	if strings.HasPrefix(name, "{") { // {mode: manual, rows: [...]} (same shape as saved tactics)
		t := ActorTactics{Version: TacticsVersion}
		if err := json.Unmarshal([]byte(name), &t); err != nil {
			return fmt.Errorf("set actor.%s.tactics: %w", a.ID, err)
		}
		t.Version = TacticsVersion
		return a.SetTactics(t)
	}
	t := ActorTactics{Version: TacticsVersion, Mode: name}
	if name != TacticsAuto && name != TacticsManual {
		if _, ok := s.DB.Game.Battle.TacticPresets[name]; !ok {
			return fmt.Errorf("set actor.%s.tactics: %q is not auto, manual or a preset", a.ID, name)
		}
		t = ActorTactics{Version: TacticsVersion, Mode: TacticsPreset, Preset: name}
	}
	return a.SetTactics(t)
}

// tacticsView is actor.<ID>.tactics (mode, preset, rows).
func tacticsView(a *Actor) expr.Map {
	t := a.TacticsDraft()
	rows := make([]any, 0, len(t.Rows))
	for _, r := range t.Rows {
		row := expr.Map{"enabled": r.Enabled, "condition": r.Condition, "target": r.Target, "action": r.Action}
		if r.Percent != nil {
			row["percent"] = float64(*r.Percent)
		}
		rows = append(rows, row)
	}
	return expr.Map{"mode": t.Mode, "preset": t.Preset, "rows": rows}
}

// repairsView is the list of corrections made when the save was loaded (repairs).
func (s *State) repairsView() []any {
	out := make([]any, 0, len(s.LoadReport.Changes))
	for _, c := range s.LoadReport.Changes {
		out = append(out, expr.Map{"id": c.ID, "path": c.Path, "reason": c.Reason})
	}
	return out
}

func toAny(xs []string) []any {
	out := make([]any, len(xs))
	for i, x := range xs {
		out[i] = x
	}
	return out
}

// MarkRan counts a run of the event id (the internal ran command at the head of its do).
func (s *State) MarkRan(id string) {
	if s.Ran == nil {
		s.Ran = map[string][2]int{}
	}
	last := 0
	for _, r := range s.Ran {
		last = max(last, r[1])
	}
	r := s.Ran[id]
	s.Ran[id] = [2]int{r[0] + 1, last + 1}
}

// ranView is ran.<id>: count (how many times the event ran) and order (when it last ran,
// counting from 1 over all events; 0 never).
type ranView struct{ s *State }

func (v ranView) Field(id string) (any, bool) {
	r := v.s.Ran[id]
	return expr.Map{"count": float64(r[0]), "order": float64(r[1])}, true
}

func (v ranView) Keys() []string { return slices.Sorted(maps.Keys(v.s.Ran)) }
