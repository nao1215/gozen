// Package itemuse performs the use command (using an item from the menu) by battle rules.
// event does not know about battle, so the screen and the simulation pass this function to event.Runner.UseItem.
package itemuse

import (
	"fmt"

	"github.com/nao1215/gozen/internal/battle"
	data "github.com/nao1215/gozen/internal/layer2/gamedata"
	"github.com/nao1215/gozen/internal/rpg"
)

// For is a function that uses an item on db and st. If target is "", the first party member receives it. The user is the first party member.
func For(db *data.DB, st *rpg.State) func(item, target string) ([]string, error) {
	return func(item, target string) ([]string, error) {
		members := st.Members()
		if len(members) == 0 {
			return nil, fmt.Errorf("no party member to use %s", item)
		}
		to := members[0]
		if target != "" {
			to = nil
			for _, a := range members {
				if a.ID == target {
					to = a
				}
			}
			if to == nil {
				return nil, fmt.Errorf("%s is not in the party", target)
			}
		}
		return battle.UseInMenu(db, st, members[0], "", item, to)
	}
}
