package battle

import (
	"strings"

	"github.com/nao1215/gozen/internal/layer2/gamedata"
	"github.com/nao1215/gozen/internal/rpg"
)

// tacticRules keeps invalid rows at the same index as Rules, so once and the reason numbers map to the stored rows.
type tacticRules struct {
	labels  []string
	rules   []gamedata.Rule
	reasons []RuleReason
	details []string
	skills  []string
	manual  bool
}

func snapshotTactics(a *rpg.Actor) *tacticRules {
	rows := a.CompileTactics()
	if rows == nil {
		return nil
	}
	t := &tacticRules{labels: make([]string, len(rows)), rules: make([]gamedata.Rule, len(rows)), reasons: make([]RuleReason, len(rows)), details: make([]string, len(rows)),
		skills: a.Skills(), manual: a.Tactics != nil && a.Tactics.Mode == rpg.TacticsManual}
	for i, row := range rows {
		t.rules[i] = row.Rule
		t.labels[i] = row.Condition
		switch {
		case len(row.Errors) > 0:
			t.reasons[i] = RuleInvalidTactic
			var reasons []string
			for _, err := range row.Errors {
				reasons = append(reasons, err.Error())
			}
			t.details[i] = strings.Join(reasons, "\n")
		case !row.Enabled:
			t.reasons[i] = RuleDisabled
		}
	}
	return t
}

func (b *Battle) gambit(u *Unit) bool {
	return (u.tactics != nil && (u.tactics.manual || b.DB.Game.Battle.SimpleTactics)) || b.DB.Game.Battle.AI == "gambit"
}
