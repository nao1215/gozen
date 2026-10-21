package rpg

import (
	"fmt"
	"slices"
	"strings"

	"github.com/nao1215/gozen/internal/expr"
	"github.com/nao1215/gozen/internal/layer1/optional"
	data "github.com/nao1215/gozen/internal/layer2/gamedata"
)

// A campaign's state lives in variables with generated names (ADR 0030); these read and
// write it for the campaign's names (region, faction, campaign, generals, general, target).

// Campaign is what the campaign's names read of the state: its variables and who has
// joined.
func (s *State) Campaign() data.CampaignState {
	ids := make([]string, 0, len(s.Actors))
	for id, a := range s.Actors {
		if a != nil {
			ids = append(ids, id)
		}
	}
	return data.CampaignState{Vars: s.Vars, JoinedIDs: ids}
}

// campaignName reports whether name is one of the campaign's expression names.
func campaignName(name string) bool { return slices.Contains(data.CampaignNames, name) }

// setCampaign writes region.<id>.<field>, target.<field>, generals.<id>.troops and
// general.troops. done is false when path is none of them.
func (s *State) setCampaign(path string, v any) (bool, error) {
	kind, rest, _ := strings.Cut(path, ".")
	number := func() (float64, error) {
		f, ok := expr.ToNumber(v)
		if !ok {
			return 0, fmt.Errorf("set %s: want a number, got %v", path, v)
		}
		return f, nil
	}
	switch kind {
	case "region", "target":
		var id, field string
		if kind == "region" {
			id, field, _ = strings.Cut(rest, ".")
		} else {
			id, field = optional.Value[string](s.Vars[data.VarTarget]), rest
			if id == "" {
				return true, fmt.Errorf("set %s: no order has a target now", path)
			}
		}
		if s.DB.Regions[id] == nil {
			return true, fmt.Errorf("set %s: no region %q", path, id)
		}
		switch {
		case field == "owner":
			f := optional.Value[string](v)
			if _, ok := s.DB.Factions[f]; !ok && f != "" {
				return true, fmt.Errorf("set %s: %v is not a faction", path, v)
			}
			s.Vars[data.RegionVar(id, "owner")] = f
		case field == "troops" || slices.Contains(data.ValueNames(s.DB), field):
			n, err := number()
			if err != nil {
				return true, err
			}
			if field == "troops" {
				n = max(0, float64(int(n)))
			}
			s.Vars[data.RegionVar(id, field)] = n
		default:
			return true, fmt.Errorf("cannot set %q", path)
		}
		return true, nil
	case "generals", "general":
		var id, field string
		if kind == "generals" {
			id, field, _ = strings.Cut(rest, ".")
		} else {
			id, field = optional.Value[string](s.Vars[data.VarGeneral]), rest
			if id == "" {
				return true, fmt.Errorf("set %s: no order has a general now", path)
			}
		}
		if c := s.DB.Game.Campaign; c == nil || field != "troops" {
			return true, fmt.Errorf("cannot set %q", path)
		} else if _, ok := c.Generals[id]; !ok {
			return true, fmt.Errorf("set %s: %q is not a general", path, id)
		}
		n, err := number()
		if err != nil {
			return true, err
		}
		s.Vars[data.GeneralVar(id, "troops")] = max(0, float64(int(n)))
		return true, nil
	}
	return false, nil
}
