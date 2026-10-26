package event_test

import (
	"testing"

	"github.com/nao1215/gozen/internal/event"
	data "github.com/nao1215/gozen/internal/layer2/gamedata"
	"github.com/nao1215/gozen/internal/testgame"
)

// An order that names its general, those marching with it and its target is held to what
// the choices would have offered: no general twice, no more than the order takes, a
// general who can lead it, and a target it can be given.
func TestOrderNamedPartsAreChecked(t *testing.T) {
	for _, c := range []struct {
		name string
		cmd  data.OrderCmd
	}{
		{"the leader marching with itself", data.OrderCmd{ID: "attack", General: "scout", With: []string{"scout"}, Target: "field"}},
		{"more generals than the order takes", data.OrderCmd{ID: "attack", General: "scout", With: []string{"guide"}, Target: "field"}},
		{"a general who is not one", data.OrderCmd{ID: "attack", General: "nobody", Target: "field"}},
		{"a target the order cannot be given", data.OrderCmd{ID: "attack", General: "scout", Target: "home"}},
	} {
		t.Run(c.name, func(t *testing.T) {
			db := testgame.LoadCampaign(t, nil)
			st := testgame.State(db, 1)
			troops := data.GeneralTroops(db, st.Campaign(), "scout")
			r := event.New(db, st, &host{})
			r.Start([]data.Command{{Order: &c.cmd}})
			for range 20 {
				if r.Next() == nil {
					break
				}
			}
			cs := st.Campaign()
			if data.Used(cs, "attack") != 0 || data.RegionOwner(db, cs, c.cmd.Target) != db.Regions[c.cmd.Target].Owner || data.GeneralTroops(db, cs, "scout") != troops {
				t.Fatalf("the order was given: used %d, %s held by %q, scout's troops %g (were %g)", data.Used(cs, "attack"), c.cmd.Target, data.RegionOwner(db, cs, c.cmd.Target), data.GeneralTroops(db, cs, "scout"), troops)
			}
		})
	}
}
