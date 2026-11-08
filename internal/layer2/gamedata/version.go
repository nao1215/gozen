package gamedata

import (
	"errors"
	"fmt"
	"reflect"
	"regexp"
	"strconv"

	"github.com/goccy/go-yaml"

	"github.com/nao1215/gozen/internal/layer1/decodeguard"
	"github.com/nao1215/gozen/internal/savefile"
	lang "github.com/nao1215/gozen/internal/text"
)

const (
	// LegacySchemaVersion is the spec of legacy YAML with no version specified.
	LegacySchemaVersion = 0
	// SchemaVersion1 is the spec version that distinguishes an omitted number from 0.
	SchemaVersion1 = 1
	// SchemaVersion2 adds to 1 the commands (pick, turn, step, walk, use, equip,
	// custom commands declared in game.yaml's verbs), the left-hand sides of set (gold, item, actor, party, location),
	// and game.yaml's verbs and screens (ADR 0021). YAML of version 1 is read as is.
	SchemaVersion2 = 2
	// SchemaVersion4 adds to 3 the vocabulary of novel games (ADR 0025 to 0029): ui.rollback
	// first, then the choice id, stand, voice, the map forms of effect, music, wait and sfx,
	// and the languages of the text. YAML of version 3 is read as is.
	SchemaVersion4 = 4
	// SchemaVersion6 adds monster recruitment and breeding (ADR 0036).
	SchemaVersion6 = 6
	// SchemaVersion7 adds battle forms, HP pictures, and enemy motion (ADR 0037).
	SchemaVersion7 = 7
	// SchemaVersion8 adds bounded stacked spaces (ADR 0040).
	SchemaVersion8 = 8
	// SchemaVersion9 adds live streams (ADR 0048).
	SchemaVersion9 = 9
	// SchemaVersion10 adds typed event arguments (ADR 0054), planning with its
	// screen kinds, views and workspace entries, say.ensemble and say.instant
	// (ADR 0053), and ui.material.
	SchemaVersion10 = 10
	// SchemaVersion11 adds compact events and single-item commands, and enables
	// authored tactic rows by default. Versions 0 through 10 keep their defaults.
	SchemaVersion11 = 11
	// SchemaVersion12 adds finite scrolling games and recorded game outcomes.
	SchemaVersion12 = 12
	// CurrentSchemaVersion is the latest YAML spec version that can be loaded. 3 adds to 2 the place presentation map and stage,
	// the doll, speech bubbles, and the spot of expressions and set (ADR 0022); 4 adds the novel game vocabulary; 5 adds
	// campaigns (ADR 0030); 6 adds monster collection; 7 adds battle forms and pictures; 8 adds stacked spaces; 9 adds live streams; 10 adds event arguments, planning and materials; 11 adds compact authoring and enabled-by-default tactic rows.
	CurrentSchemaVersion = SchemaVersion12
)

// MaxRollback is the most earlier events ui.rollback may keep (the history is a copy of the
// state at the start of each event, so it is kept small).
const MaxRollback = 64

// DefaultRollback is how many earlier events the player can roll back into when ui.rollback is left out.
const DefaultRollback = 8

var gameIDPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_-]*$`)

// ValidGameID shares the persistent identity constraint between loading and
// template creation, so a CLI-generated game has the same save identity rules.
func ValidGameID(id string) bool { return gameIDPattern.MatchString(id) && savefile.ValidID(id) }

// checkVersionHeader checks only the version, before the overall type conversion and data loading.
// Even if future fields exist, the unknown-version diagnostic takes priority, and null or strings are not rounded to 0.
func checkVersionHeader(b []byte) error {
	var raw yaml.MapSlice
	if err := decodeguard.Run(func() error { return yaml.UnmarshalWithOptions(b, &raw, yaml.UseOrderedMap(), yaml.Strict()) }); err != nil {
		return fmt.Errorf("game.yaml: %s", yaml.FormatError(err, false, true))
	}
	g := Game{}
	for _, it := range raw {
		var dst *int
		switch it.Key {
		case "schema_version":
			dst = &g.SchemaVersion
		case "data_version":
			dst = &g.DataVersion
		default:
			continue
		}
		v := reflect.ValueOf(it.Value)
		if !v.IsValid() || (!v.CanInt() && !v.CanUint()) {
			return fmt.Errorf(lang.T("game.yaml %s: 版は０以上の整数で指定してください"), it.Key)
		}
		n, err := strconv.Atoi(fmt.Sprint(it.Value))
		if err != nil {
			return fmt.Errorf(lang.T("game.yaml %s: 版が整数の範囲を超えています"), it.Key)
		}
		*dst = n
	}
	return errors.Join(versionErrors(g)...)
}

// versionErrors applies the same range to both YAML and settings built from Go.
func versionErrors(g Game) []error {
	var errs []error
	if g.SchemaVersion < LegacySchemaVersion || g.SchemaVersion > CurrentSchemaVersion {
		errs = append(errs, fmt.Errorf(lang.T("game.yaml schema_version: 仕様版 %d には対応していません（対応版 %d〜%d）。対応するエンジンを使用してください"), g.SchemaVersion, LegacySchemaVersion, CurrentSchemaVersion))
	}
	if g.DataVersion < 0 {
		errs = append(errs, fmt.Errorf(lang.T("game.yaml data_version: 作品のデータ版は０以上にしてください（指定値 %d）"), g.DataVersion))
	}
	return errs
}

func gameVersionErrors(g Game) []error {
	errs := versionErrors(g)
	if g.SchemaVersion >= SchemaVersion1 && !ValidGameID(g.ID) {
		errs = append(errs, fmt.Errorf(lang.T("game.yaml id: 仕様版 %d では作品の ID が必要です。英数字で始まる英数字・_・- だけの、予約語でない 255 バイト以内の名前を指定してください"), g.SchemaVersion))
	}
	return errs
}
