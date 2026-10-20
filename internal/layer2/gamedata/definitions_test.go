package gamedata

import (
	"fmt"
	"reflect"
	"slices"
	"strings"
	"testing"
	"testing/fstest"
)

const (
	definitionGame = `start: {location: camp, party: [scout]}`
	definitionData = `
classes:
  ranger:
    stats: {mhp: 10, mmp: 0, atk: 1, def: 1, mat: 1, mdf: 1, agi: 1, luk: 1}
    exp_for: level * 10
actors: {scout: {class: ranger}}
skills: {attack: {}, guard: {}}
locations: {camp: {}}
`
)

func loadDefinition(extra string) (*DB, error) {
	return Load(fstest.MapFS{
		"game.yaml":            {Data: []byte(definitionGame)},
		"data/base.yaml":       {Data: []byte(definitionData)},
		"data/unfinished.yaml": {Data: []byte(extra)},
	})
}

// Pick up the definition sections from the type so that every case also applies to new sections.
func pointerDefinitionSections() []string {
	var sections []string
	for field := range reflect.TypeFor[file]().Fields() {
		if field.Type.Kind() == reflect.Map && field.Type.Elem().Kind() == reflect.Pointer {
			sections = append(sections, field.Tag.Get("yaml"))
		}
	}
	return sections
}

func wantDefinitionError(t *testing.T, src string, want ...string) {
	t.Helper()
	_, err := loadDefinition(src)
	if err == nil {
		t.Fatalf("不正な定義を受け入れた。%s", src)
	}
	for _, text := range append([]string{"data/unfinished.yaml"}, want...) {
		if !strings.Contains(err.Error(), text) {
			t.Errorf("診断に %q が無い。%v", text, err)
		}
	}
}

func TestNullDefinitions(t *testing.T) {
	for _, section := range pointerDefinitionSections() {
		for _, value := range []string{"null", "~", ""} {
			t.Run(section+"/"+value, func(t *testing.T) {
				wantDefinitionError(t, section+":\n  sample: "+value+"\n", section+".sample")
			})
		}
	}
	for _, tc := range []struct{ src, path string }{
		{`locations: {sample: {dungeon: {tiles: {S: null}}}}`, "locations.sample.dungeon.tiles.S"},
		{`locations: {sample: {dungeon: null}}`, "locations.sample.dungeon"},
		{`locations: {sample: {dungeon: {generate: null}}}`, "locations.sample.dungeon.generate"},
		{`items: {sample: {use: null}}`, "items.sample.use"},
		{`items: {sample: {use: {effects: [null]}}}`, "items.sample.use.effects[0]"},
		{`events: {sample: {do: [null]}}`, "events.sample.do[0]"},
		{`events: {sample: {do: [{give: null}]}}`, "events.sample.do[0].give"},
		{`music: {songs: {sample: null}}`, "music.songs.sample"},
		{`fusions: [null]`, "fusions[0]"},
	} {
		t.Run(tc.path, func(t *testing.T) { wantDefinitionError(t, tc.src, tc.path) })
	}
	// Do not lose multiple empty definitions in one file or the diagnostics of another file.
	_, err := Load(fstest.MapFS{
		"game.yaml":   {Data: []byte(definitionGame)},
		"data/a.yaml": {Data: []byte("items: {first: null, second: null}")},
		"data/b.yaml": {Data: []byte("events: {third: null}")},
	})
	if err == nil {
		t.Fatal("空の定義を受け入れた")
	}
	for _, want := range []string{"data/a.yaml", "items.first", "items.second", "data/b.yaml", "events.third"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("診断に %q が無い。%v", want, err)
		}
	}
}

func TestMalformedDefinitions(t *testing.T) {
	for _, section := range pointerDefinitionSections() {
		for _, value := range []string{"[]", "[{}]", "wrong", "42", "true"} {
			t.Run(section+"/"+value, func(t *testing.T) {
				wantDefinitionError(t, fmt.Sprintf("%s: {sample: %s}", section, value), section+".sample")
			})
		}
	}
	for _, tc := range []struct{ src, path string }{
		{`classes: {sample: {}}`, "classes.sample"},
		{`actors: {sample: {}}`, "actors.sample"},
		{`enemies: {sample: {}}`, "enemies.sample"},
		{`troops: {sample: {}}`, "troops.sample"},
		{`stone_sets: {sample: {}}`, "stone_sets.sample"},
		{`locations: {sample: {dungeon: {}}}`, "locations.sample.dungeon"},
		{`items: {sample: {use: {effects: [{}]}}}`, "items.sample.use.effects[0]"},
		{`events: {sample: {do: [{}]}}`, "events.sample.do[0]"},
		{`actors: {sample: {level: wrong}}`, "actors.sample"},
		{`classes: {sample: {stats: []}}`, "classes.sample.stats"},
		{`locations: {sample: {dungeon: {tiles: {S: []}}}}`, "locations.sample.dungeon.tiles.S"},
		{`locations: {sample: {map: {points: {a: []}}}}`, "locations.sample.map.points.a"},
		{`locations: {sample: {map: {points: {a: null}}}}`, "locations.sample.map.points.a"},
		{`locations: {sample: {stage: {spots: {a: []}}}}`, "locations.sample.stage.spots.a"},
		{`locations: {sample: {stage: {spots: {a: null}}}}`, "locations.sample.stage.spots.a"},
		{`music: {songs: {sample: {}}}`, "music.songs.sample"},
		{`fusions: [{}]`, "fusions[0]"},
	} {
		t.Run(tc.src, func(t *testing.T) { wantDefinitionError(t, tc.src, tc.path) })
	}
	t.Run("合法な空マップ", func(t *testing.T) {
		for _, section := range []string{"items", "skills", "states", "locations", "events", "shops"} {
			if _, err := loadDefinition(section + ": {sample: {}}"); err != nil {
				t.Errorf("%s の空マップは合法。%v", section, err)
			}
		}
		for _, src := range []string{
			"", "# 書きかけの空ファイル\n", "music: {}", "ai: {sample: []}",
			`locations: {sample: {dungeon: {map: "ST", tiles: {S: {start: true}, T: {}}}}}`,
		} {
			if _, err := loadDefinition(src); err != nil {
				t.Errorf("合法な定義を拒否した。%s。%v", src, err)
			}
		}
	})
}

// Do not miss additions of pointer-value maps, and check that nested ones also have null cases.
func TestNullDefinitionsCoverage(t *testing.T) {
	var paths []string
	active := map[reflect.Type]bool{}
	var walk func(reflect.Type, string)
	walk = func(typ reflect.Type, path string) {
		for typ.Kind() == reflect.Pointer {
			typ = typ.Elem()
		}
		switch typ.Kind() { //nolint:exhaustive // Only schema container/scalar kinds need handling; other reflection kinds are intentionally ignored.
		case reflect.Map:
			if typ.Elem().Kind() == reflect.Pointer {
				paths = append(paths, path)
			}
			walk(typ.Elem(), path+".*")
		case reflect.Slice, reflect.Array:
			walk(typ.Elem(), path+"[]")
		case reflect.Struct:
			if active[typ] {
				return
			}
			active[typ] = true
			defer delete(active, typ)
			for name, field := range definitionFields(typ) {
				walk(field, strings.TrimPrefix(path+"."+name, "."))
			}
		}
	}
	walk(reflect.TypeFor[file](), "")
	want := append(pointerDefinitionSections(), "locations.*.dungeon.tiles", "locations.*.map.points", "locations.*.stage.spots")
	slices.Sort(paths)
	slices.Sort(want)
	if !slices.Equal(paths, want) {
		t.Fatalf("ポインタ値の定義が増えた。入れ子の再現例を追加する。対象 %v、検査済み %v", paths, want)
	}
}
