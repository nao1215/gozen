package data

import (
	"errors"
	"fmt"
	"io/fs"

	"github.com/nao1215/gozen/internal/layer2/gamedata"
)

// CurrentSchemaVersion is the supported YAML schema version, distinct from save versions.
const CurrentSchemaVersion = gamedata.CurrentSchemaVersion

// LegacySchemaVersion accepts versionless YAML definitions.
const LegacySchemaVersion = gamedata.LegacySchemaVersion

// Load reads a game directory rooted at path within fsys. Use "." for its root.
// It validates references, expressions and numeric presence. On error it returns nil.
// It does not close or mutate fsys; keep it available while using the DB's assets.
func Load(fsys fs.FS, path string) (*DB, error) {
	if fsys == nil || !fs.ValidPath(path) {
		return nil, fmt.Errorf("load game %q: %w", path, fs.ErrInvalid)
	}
	sub, err := fs.Sub(fsys, path)
	if err != nil {
		return nil, err
	}
	db, err := gamedata.Load(sub)
	if err != nil {
		return nil, err
	}
	return db, nil
}

// Validate checks a programmatically assembled DB without modifying it.
// Definitions must include explicit defaults, a non-nil FS, and Order entries
// matching every section. Load supplies these and defaults for YAML.
func Validate(db *DB) error {
	if db == nil {
		return errors.New("validate game: nil database")
	}
	if db.FS == nil {
		return errors.New("validate game: nil filesystem")
	}
	var structural []error
	structural = append(structural, definitionErrors("classes", db.Classes, db.Order["classes"])...)
	structural = append(structural, definitionErrors("actors", db.Actors, db.Order["actors"])...)
	structural = append(structural, definitionErrors("items", db.Items, db.Order["items"])...)
	structural = append(structural, definitionErrors("skills", db.Skills, db.Order["skills"])...)
	structural = append(structural, definitionErrors("states", db.States, db.Order["states"])...)
	structural = append(structural, definitionErrors("enemies", db.Enemies, db.Order["enemies"])...)
	structural = append(structural, definitionErrors("troops", db.Troops, db.Order["troops"])...)
	structural = append(structural, definitionErrors("locations", db.Locations, db.Order["locations"])...)
	structural = append(structural, definitionErrors("events", db.Events, db.Order["events"])...)
	structural = append(structural, definitionErrors("shops", db.Shops, db.Order["shops"])...)
	structural = append(structural, definitionErrors("stone_sets", db.StoneSets, db.Order["stone_sets"])...)
	structural = append(structural, definitionErrors("tactic_conditions", db.TacticConditions, db.Order["tactic_conditions"])...)
	structural = append(structural, definitionErrors("tactic_targets", db.TacticTargets, db.Order["tactic_targets"])...)
	structural = append(structural, definitionErrors("tactic_actions", db.TacticActions, db.Order["tactic_actions"])...)
	if len(structural) != 0 {
		return errors.Join(structural...)
	}
	return errors.Join(db.Check()...)
}

// definitionErrors rejects malformed maps before semantic checks dereference them.
func definitionErrors[T any](section string, defs map[string]*T, order []string) []error {
	var errs []error
	seen := make(map[string]bool, len(order))
	for _, id := range order {
		if seen[id] || defs[id] == nil {
			errs = append(errs, fmt.Errorf("%s.%s: duplicate order entry or missing definition", section, id))
		}
		seen[id] = true
	}
	for id, def := range defs {
		if def == nil || !seen[id] {
			errs = append(errs, fmt.Errorf("%s.%s: nil definition or missing order entry", section, id))
		}
	}
	return errs
}
