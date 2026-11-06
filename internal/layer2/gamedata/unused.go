package gamedata

import (
	"fmt"
	"maps"
	"slices"

	"github.com/nao1215/gozen/internal/diag"
	lang "github.com/nao1215/gozen/internal/text"
)

// DefinitionWarnings returns positioned warnings for definitions with no known
// YAML consumer. checkdata uses them for text and structured authoring reports.
// Check refreshes the result; callers must not mutate the returned slice.
// This is an incoming-reference check, not a reachability proof. Triggered
// events are entry points. Dynamic expression lookup conservatively retains
// every definition of its kind. External scripts are outside this analysis.
func (db *DB) DefinitionWarnings() []error { return db.definitionWarns }

func (c *checker) useDefinition(kind, id string) {
	if c.definitionUses == nil || kind == "" || id == "" {
		return
	}
	if c.definitionUses[kind] == nil {
		c.definitionUses[kind] = map[string]bool{}
	}
	c.definitionUses[kind][id] = true
}

// A dynamic lookup is distinct from the literal ID "*".
func (c *checker) dynamicDefinition(kind string) {
	if c.definitionDynamic != nil && kind != "" {
		c.definitionDynamic[kind] = true
	}
}

func (c *checker) unusedDefinitions() []error {
	var out []error
	db := c.db
	// Restrict this list to kinds whose consumers pass through the semantic
	// reference checks. Planning collections and combat species have implicit
	// consumers and deliberately remain outside this exact check.
	for _, section := range []struct {
		kind, path string
		ids        []string
	}{
		{"event", "events", slices.Sorted(maps.Keys(db.Events))},
		{"screen", "game.yaml.screens", slices.Sorted(maps.Keys(db.Game.Screens))},
		{"location", "locations", slices.Sorted(maps.Keys(db.Locations))},
		{"item", "items", slices.Sorted(maps.Keys(db.Items))},
		{"troop", "troops", slices.Sorted(maps.Keys(db.Troops))},
		{"shop", "shops", slices.Sorted(maps.Keys(db.Shops))},
		{"ai", "ai", slices.Sorted(maps.Keys(db.AI))},
	} {
		used := c.definitionUses[section.kind]
		if c.definitionDynamic[section.kind] {
			continue
		}
		for _, id := range section.ids {
			if used[id] {
				continue
			}
			if section.kind == "event" && db.Events[id] != nil && db.Events[id].Trigger != "" {
				continue
			}
			// Phase cards read date and phase labels from authored views even
			// when no command opens that screen (play.turnDate/phaseLabel).
			if section.kind == "screen" {
				if screen := db.Game.Screens[id]; screen != nil && (screen.View.DateFormat != "" || len(screen.View.Phases) > 0) {
					continue
				}
			}
			path := section.path + "." + id
			message := fmt.Sprintf(lang.T("%s %q を参照する定義がありません。利用箇所を接続するか、不要なら削除してください"), section.kind, id)
			out = append(out, coded(db.diagnostic(path, message), diag.UnusedDefinition))
		}
	}
	return sortedDiagnostics(out)
}
