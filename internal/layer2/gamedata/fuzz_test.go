package gamedata

import (
	"testing"
	"testing/fstest"
)

// FuzzLoad runs both entry points, the global settings and the data, from parsing through semantic validation without the main game.
func FuzzLoad(f *testing.F) {
	for _, seed := range []struct {
		game bool
		src  string
	}{
		{true, definitionGame},
		{true, "schema_version: 1\nid: neutral\n" + definitionGame},
		{true, "schema_version: 99\n"},
		{false, ""},
		{false, "events: {sample: {do: [{if: 'gold > 0', then: [{gold: 1}]}]}}"},
		{false, "events: {sample: {do: [{image: ''}]}}"},
		{false, "items: {sample: null}"},
		{false, "events: {sample: {do: [{if: '('}]}}"},
		{false, "events: ["},
		{false, "items: {sample: &value {}, other: *value}"},
	} {
		f.Add(seed.game, []byte(seed.src))
	}
	f.Fuzz(func(t *testing.T, game bool, src []byte) {
		// Explore the range of ordinary YAML editing without exhausting execution resources merely by growing the input.
		if len(src) > 64*1024 {
			return
		}
		fsys := diagnosticFS(nil)
		name := "data/fuzz.yaml"
		if game {
			name = "game.yaml"
		}
		fsys[name] = &fstest.MapFile{Data: src}
		db, err := Load(fsys)
		if err == nil {
			if db == nil {
				t.Fatal("読み込み成功なのに DB がない")
			}
			if problems := db.Check(); len(problems) != 0 {
				t.Fatalf("受理したデータの再検査が失敗 %v", problems)
			}
		}
	})
}
