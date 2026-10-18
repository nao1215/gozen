package gamedata_test

import (
	"fmt"
	"io/fs"
	"reflect"
	"strings"
	"testing"
	"testing/fstest"

	data "github.com/nao1215/gozen/internal/layer2/gamedata"
	"github.com/nao1215/gozen/internal/testgame"
)

func TestDataVersionCompatibility(t *testing.T) {
	t.Run("版なしの既存データ", func(t *testing.T) {
		legacy := testgame.Load(t, nil)
		explicit := testgame.Load(t, map[string]string{"game.yaml": "schema_version: 0\ndata_version: 0\n" + testgame.Game})
		if legacy.Game.SchemaVersion != data.LegacySchemaVersion || legacy.Game.DataVersion != 0 || legacy.Game.ID != "" {
			t.Fatalf("旧仕様の版と ID が変わった: %+v", legacy.Game)
		}
		if !reflect.DeepEqual(legacy.Game, explicit.Game) {
			t.Fatal("版なしと明示した旧版の実効設定が異なる")
		}
		if legacy.Game.Battle.Variance != 0.0001 || legacy.Game.Battle.CriticalRate != 1.5 || legacy.Music.Volume != 0.7 {
			t.Fatal("従来の既定値が変わった")
		}
	})
	t.Run("新版の最小データ", func(t *testing.T) {
		fsys := fstest.MapFS{
			"game.yaml": {Data: []byte("schema_version: 1\ndata_version: 42\nid: neutral-game_1\nstart: {location: camp, party: [scout]}\n")},
			"data/main.yaml": {Data: []byte(`
classes:
  adventurer:
    stats: {mhp: "10", mmp: "0", atk: "1", def: "1", mat: "1", mdf: "1", agi: "1", luk: "1"}
    exp_for: "level * 10"
actors: {scout: {class: adventurer}}
skills: {attack: {}, guard: {}}
locations: {camp: {}}
`)},
		}
		db, err := data.Load(fsys)
		if err != nil {
			t.Fatal(err)
		}
		if db.Game.SchemaVersion != data.CurrentSchemaVersion || db.Game.DataVersion != 42 || db.Game.ID != "neutral-game_1" {
			t.Fatalf("版を保持していない: %+v", db.Game)
		}
	})
	for _, tc := range []struct {
		name, header, want string
	}{
		{"未来版", "schema_version: 2", "schema_version"},
		{"負の仕様版", "schema_version: -1", "schema_version"},
		{"負のデータ版", "data_version: -1", "data_version"},
		{"新版の ID なし", "schema_version: 1", "id"},
		{"新版の ID 空", "schema_version: 1\nid: ''", "id"},
		{"新版の ID パス", "schema_version: 1\nid: ../other", "id"},
		{"新版の ID 区切り", "schema_version: 1\nid: 'a\\b'", "id"},
		{"新版の ID 空白", "schema_version: 1\nid: ' ' ", "id"},
		{"版の重複", "schema_version: 0\nschema_version: 1", "schema_version"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			db, err := testgame.TryLoad(map[string]string{"game.yaml": tc.header + "\n" + testgame.Game})
			if err == nil || db != nil || !strings.Contains(err.Error(), "game.yaml") || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("不正な設定を読み込んだ、または診断に場所がない: db=%v err=%v", db != nil, err)
			}
		})
	}
	for _, key := range []string{"schema_version", "data_version"} {
		for _, value := range []string{"null", "", "'1'", "true", "1.0", "1.5", "[]", "{}", "18446744073709551615"} {
			t.Run(key+"/"+value, func(t *testing.T) {
				_, err := testgame.TryLoad(map[string]string{"game.yaml": key + ": " + value + "\n" + testgame.Game})
				if err == nil || !strings.Contains(err.Error(), key) || !strings.Contains(err.Error(), "game.yaml") {
					t.Fatalf("整数以外や範囲外を受け入れた: %v", err)
				}
			})
		}
	}
	t.Run("作品のデータ版は仕様版を選ばない", func(t *testing.T) {
		for _, header := range []string{"data_version: 99\n", "schema_version: 1\nid: neutral\n", "schema_version: 0\nid: 'old name'\n"} {
			if _, err := testgame.TryLoad(map[string]string{"game.yaml": header + testgame.Game}); err != nil {
				t.Fatal(err)
			}
		}
	})
	t.Run("未来版は本体の型変換とファイル読み取りより前に拒否", func(t *testing.T) {
		fsys := &versionOnlyFS{game: []byte("schema_version: 99\nfuture_option: true\nstart: [new, syntax]\n")}
		db, err := data.Load(fsys)
		if err == nil || db != nil || !strings.Contains(err.Error(), "schema_version") || !strings.Contains(err.Error(), "99") || fsys.other {
			t.Fatalf("版の診断が後回し: db=%v err=%v other=%v", db != nil, err, fsys.other)
		}
	})
	t.Run("Go で作った設定も検査", func(t *testing.T) {
		for _, g := range []data.Game{{SchemaVersion: 2}, {DataVersion: -1}, {SchemaVersion: 1}} {
			db := &data.DB{Game: g}
			if errs := db.Check(); len(errs) == 0 {
				t.Fatalf("不正な版や ID を受け入れた: %+v", g)
			}
		}
	})
}

// versionOnlyFS は game.yaml 以外を開く前に拒否したことを確かめる。
type versionOnlyFS struct {
	game  []byte
	other bool
}

func (f *versionOnlyFS) Open(name string) (fs.File, error) {
	if name != "game.yaml" {
		f.other = true
		return nil, fmt.Errorf("データ本体を先に開いた: %s", name)
	}
	return fstest.MapFS{"game.yaml": {Data: f.game}}.Open(name)
}
