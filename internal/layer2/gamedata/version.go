package gamedata

import (
	"errors"
	"fmt"
	"reflect"
	"regexp"
	"strconv"

	"github.com/goccy/go-yaml"

	"github.com/nao1215/gozen/internal/layer1/decodeguard"
	lang "github.com/nao1215/gozen/text"
)

const (
	// LegacySchemaVersion は版指定のない従来の YAML の仕様。
	LegacySchemaVersion = 0
	// CurrentSchemaVersion は読み込める最新の YAML の仕様版。
	CurrentSchemaVersion = 1
)

var gameIDPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_-]*$`)

// checkVersionHeader は全体の型変換・データ読み込みより先に版だけを検査する。
// 将来の項目があっても未知版の診断を優先し、null や文字列を０に丸めない。
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

// versionErrors は YAML と Go から構築した設定の両方に同じ範囲を適用する。
func versionErrors(g Game) []error {
	var errs []error
	if g.SchemaVersion != LegacySchemaVersion && g.SchemaVersion != CurrentSchemaVersion {
		errs = append(errs, fmt.Errorf(lang.T("game.yaml schema_version: 仕様版 %d には対応していません（対応版 %d・%d）。対応するエンジンを使用してください"), g.SchemaVersion, LegacySchemaVersion, CurrentSchemaVersion))
	}
	if g.DataVersion < 0 {
		errs = append(errs, fmt.Errorf(lang.T("game.yaml data_version: 作品のデータ版は０以上にしてください（指定値 %d）"), g.DataVersion))
	}
	return errs
}

func gameVersionErrors(g Game) []error {
	errs := versionErrors(g)
	if g.SchemaVersion == CurrentSchemaVersion && !gameIDPattern.MatchString(g.ID) {
		errs = append(errs, fmt.Errorf(lang.T("game.yaml id: 仕様版 %d では作品の ID が必要です。英数字で始まる英数字・_・- だけの名前を指定してください"), CurrentSchemaVersion))
	}
	return errs
}
