package gamedata_test

import (
	"fmt"
	"reflect"
	"strconv"
	"strings"
	"testing"

	"github.com/goccy/go-yaml"

	data "github.com/nao1215/gozen/internal/layer2/gamedata"
	"github.com/nao1215/gozen/internal/layer3/dungeon"
	"github.com/nao1215/gozen/internal/testgame"
)

// mapAt walks nested YAML mappings by key and fails t when a fixture has another shape.
func mapAt(t *testing.T, m map[string]any, keys ...string) map[string]any {
	t.Helper()
	for _, k := range keys {
		m = testgame.As[map[string]any](t, m[k])
	}
	return m
}

// numericFixture は中立ゲームの生の YAML に１項目だけを指定して読み込む。
func numericFixture(t *testing.T, version int, target, value string) (*data.DB, error) {
	t.Helper()
	var game, definitions map[string]any
	if err := yaml.Unmarshal([]byte(testgame.Game), &game); err != nil {
		t.Fatal(err)
	}
	if err := yaml.Unmarshal([]byte(testgame.Data), &definitions); err != nil {
		t.Fatal(err)
	}
	game["schema_version"], game["id"] = version, "neutral"
	mapAt(t, definitions, "actors", "scout")["faces"] = "portrait"
	game["title_figures"] = []any{map[string]any{"file": "x.png", "x": 0.5}}
	game["card_bubbles"] = []any{map[string]any{}}
	game["meters"] = []any{map[string]any{"value": "gold"}}
	definitions["music"] = map[string]any{"songs": map[string]any{}}
	mapAt(t, definitions, "classes", "ranger")["tree"] = []any{
		map[string]any{"id": "root", "skill": "fire"},
		map[string]any{"id": "child", "skill": "heal", "requires": []any{map[string]any{"node": "root"}}},
	}
	mapAt(t, definitions, "locations")["maze"] = map[string]any{"dungeon": map[string]any{
		"generate": map[string]any{"width": 9, "height": 9, "start": "S", "place": []any{}},
		"tiles":    map[string]any{"S": map[string]any{"start": true}},
		"looks":    []any{map[string]any{"when": "true"}},
		"roamers":  []any{map[string]any{"picture": "x.png", "troops": []any{map[string]any{"troop": "one"}}}},
	}}
	mapAt(t, definitions, "events")["numeric"] = map[string]any{"do": []any{map[string]any{"image": map[string]any{"files": []any{"x.png"}}}}}
	mapAt(t, definitions, "events")["affection"] = map[string]any{"do": []any{map[string]any{"affect": map[string]any{"who": "all", "add": "sleep"}}}}
	parts := strings.Split(target, ".")
	var node any = definitions
	if parts[0] == "game" {
		node = game
	}
	for _, part := range parts[1 : len(parts)-1] {
		if index, err := strconv.Atoi(part); err == nil {
			node = testgame.As[[]any](t, node)[index]
		} else {
			m := testgame.As[map[string]any](t, node)
			if m[part] == nil {
				m[part] = map[string]any{}
			}
			node = m[part]
		}
	}
	key := parts[len(parts)-1]
	m := testgame.As[map[string]any](t, node)
	delete(m, key)
	if value != "" {
		var scalar any
		if err := yaml.Unmarshal([]byte(value), &scalar); err != nil {
			t.Fatal(err)
		}
		m[key] = scalar
	}
	gb, err := yaml.Marshal(game)
	if err != nil {
		t.Fatal(err)
	}
	db, err := yaml.Marshal(definitions)
	if err != nil {
		t.Fatal(err)
	}
	return testgame.TryLoad(map[string]string{"game.yaml": string(gb), "data/main.yaml": string(db), "assets/portrait/normal.png": "x"})
}

// numericValue は Go のフィールドから補完後の値を読む。期待値は下の表で独立に定義する。
func numericValue(db *data.DB, path string) any {
	v := reflect.ValueOf(db)
	for _, name := range strings.Split(path, ".") {
		for v.Kind() == reflect.Pointer {
			v = v.Elem()
		}
		switch v.Kind() { //nolint:exhaustive // Only schema container/scalar kinds need handling; other reflection kinds are intentionally ignored.
		case reflect.Struct:
			v = v.FieldByName(name)
		case reflect.Map:
			v = v.MapIndex(reflect.ValueOf(name))
		case reflect.Slice:
			i, err := strconv.Atoi(name)
			if err != nil {
				return nil
			}
			v = v.Index(i)
		}
	}
	if v.Kind() == reflect.Pointer {
		if v.IsNil() {
			return nil
		}
		v = v.Elem()
	}
	if v.Kind() == reflect.Int {
		return float64(v.Int())
	}
	return v.Interface()
}

func TestNumericPresenceContract(t *testing.T) {
	// 省略・０・正負と型の検査を、利用側で０が失われやすい項目すべてへ通す。
	fields := []struct {
		yaml, goPath        string
		missing, legacyZero any
		positive            float64
		zeroOK              bool
	}{
		{"game.max_party", "Game.MaxParty", 4., 4., 4, false},
		{"game.screen.width", "Game.Screen.Width", 1280., 1280., 1280, false},
		{"game.screen.height", "Game.Screen.Height", 720., 720., 720, false},
		{"data.classes.ranger.sp", "Classes.ranger.SP", 1., 1., 2, true},
		{"data.classes.ranger.start_sp", "Classes.ranger.StartSP", 0., 0., 2, true},
		{"data.classes.ranger.max_level", "Classes.ranger.Max", 99., 99., 50, false},
		{"data.classes.ranger.tree.0.max", "Classes.ranger.Tree.0.Max", 10., 10., 3, false},
		{"data.classes.ranger.tree.1.requires.0.level", "Classes.ranger.Tree.1.Requires.0.Level", 1., 1., 2, false},
		{"data.classes.ranger.skills.0.level", "Classes.ranger.Skills.0.Level", 1., 0., 2, false},
		{"data.actors.scout.level", "Actors.scout.Level", 1., 1., 2, false},
		{"data.actors.scout.face_shade", "Actors.scout.FaceShade", 0., 0., 0.3, true},
		{"data.actors.scout.face_shade_top", "Actors.scout.FaceShadeTop", 0., 0., 0.3, true},
		{"data.enemies.mote.level", "Enemies.mote.Level", 1., 1., 2, false},
		{"data.enemies.mote.scale", "Enemies.mote.Scale", 1., 1., 0.8, false},
		{"data.enemies.mote.actions", "Enemies.mote.Actions", 1., 1., 2, true},
		{"data.skills.fire.hits", "Skills.fire.Hits", 1., 1., 3, false},
		{"data.items.potion.use.hits", "Items.potion.Use.Hits", 1., 1., 3, false},
		{"data.skills.nap.effects.0.summon_max", "Skills.nap.Effects.0.SummonMax", 5., 0., 3, false},
		{"data.states.sleep.turns", "States.sleep.Turns", 0., 0., 3, true},
		{"data.states.sleep.damage_rate", "States.sleep.DamageRate", 1., 0., 0.5, true},
		{"data.states.sleep.restrict_chance", "States.sleep.RestrictChance", 0., 0., 0.5, true},
		{"game.battle.tempo", "Game.Battle.Tempo", 1., 0., 2, false},
		{"game.battle.auto_tempo", "Game.Battle.AutoTempo", 1., 0., 2, false},
		{"game.title_background.brightness", "Game.TitleBG.Bright", 1., 0., 0.5, true},
		{"data.events.numeric.do.0.image.brightness", "Events.numeric.Do.0.Image.Bright", 1., 0., 0.5, true},
		{"data.events.numeric.do.0.image.auto", "Events.numeric.Do.0.Image.Auto", 0., 0., 2, true},
		{"data.events.numeric.do.0.image.page", "Events.numeric.Do.0.Image.Page", 0., 0., 2, true},
		{"data.events.affection.do.0.affect.turns", "Events.affection.Do.0.Affect.Turns", 0., 0., 2, true},
		{"data.events.numeric.do.0.image.fade", "Events.numeric.Do.0.Image.Fade", 0.25, 0., 0.5, true},
		{"data.events.numeric.do.0.image.over_size", "Events.numeric.Do.0.Image.OverSize", 0.55, 0., 0.5, false},
		{"data.locations.maze.dungeon.depth", "Locations.maze.Dungeon.Depth", 5., 5., 3, false},
		{"data.locations.maze.dungeon.darkness", "Locations.maze.Dungeon.DarkSet", nil, 0., 0.5, true},
		{"data.locations.maze.dungeon.looks.0.darkness", "Locations.maze.Dungeon.Looks.0.DarkSet", nil, 0., 0.5, true},
		{"data.locations.maze.dungeon.generate.rooms", "Locations.maze.Dungeon.Generate.Rooms", 0., 0., 3, true},
		{"data.locations.maze.dungeon.generate.loops", "Locations.maze.Dungeon.Generate.Loops", 0.08, 0., 0.2, true},
		{"data.locations.maze.dungeon.tiles.S.size", "Locations.maze.Dungeon.Tiles.S.Size", 0.6, 0.6, 0.8, false},
		{"data.locations.maze.dungeon.roamers.0.size", "Locations.maze.Dungeon.Roamers.0.Size", 0.8, 0., 0.7, false},
		{"data.locations.maze.dungeon.roamers.0.sight", "Locations.maze.Dungeon.Roamers.0.Sight", 4., 0., 3, false},
		{"data.locations.maze.dungeon.roamers.0.far", "Locations.maze.Dungeon.Roamers.0.Far", 0., 0., 3, true},
		{"data.enemies.mote.ai.0.weight", "Enemies.mote.AI.Rules.0.Weight", 1., 0., 2, false},
		{"data.locations.maze.dungeon.roamers.0.troops.0.weight", "Locations.maze.Dungeon.Roamers.0.Troops.0.Weight", 1., 0., 2, false},
		{"game.ui.backlog", "Game.UI.Backlog", 300., 0., 2, true},
		{"game.meters.0.max", "Game.Meters.0.Max", 100., 0., 80, false},
		{"game.title_figures.0.bottom", "Game.TitleFigures.0.Bottom", 1., 0., 0.8, true},
		{"game.title_figures.0.height", "Game.TitleFigures.0.Height", 0.95, 0., 0.8, false},
		{"game.card_bubbles.0.count", "Game.CardBubbles.0.Count", 4., 0., 2, true},
		{"game.card_bubbles.0.size", "Game.CardBubbles.0.Size", 3., 0., 2, false},
		{"game.card_bubbles.0.speed", "Game.CardBubbles.0.Speed", 30., 0., 20, false},
		{"game.card_bubbles.0.rise", "Game.CardBubbles.0.Rise", 50., 0., 20, false},
		{"data.music.bars", "Music.Bars", 8., 8., 2, false},
		{"data.music.beats_per_bar", "Music.Beats", 4., 4., 2, false},
	}
	for _, fx := range []string{"shake", "flash", "pop", "hitstop", "charge", "finish"} {
		fields = append(fields, struct {
			yaml, goPath        string
			missing, legacyZero any
			positive            float64
			zeroOK              bool
		}{"game.ui.battle_fx." + fx, "Game.UI.BattleFx." + strings.ToUpper(fx[:1]) + fx[1:], nil, 0., 2, true})
	}
	for _, fx := range []string{"hitstop", "charge", "finish", "win"} {
		fields = append(fields, struct {
			yaml, goPath        string
			missing, legacyZero any
			positive            float64
			zeroOK              bool
		}{"game.ui.battle_fx.auto." + fx, "Game.UI.BattleFx.Auto." + strings.ToUpper(fx[:1]) + fx[1:], nil, 0., 2, true})
	}
	for _, fx := range []string{"pitch", "volume"} {
		fields = append(fields, struct {
			yaml, goPath        string
			missing, legacyZero any
			positive            float64
			zeroOK              bool
		}{"game.ui.sfx_vary." + fx, "Game.UI.SfxVary." + strings.ToUpper(fx[:1]) + fx[1:], nil, 0., 0.2, true})
	}
	legacyNegativeRejected := map[string]bool{}
	for _, name := range []string{
		"game.max_party", "game.title_figures.0.bottom", "game.title_figures.0.height", "data.classes.ranger.sp", "data.classes.ranger.start_sp",
		"data.classes.ranger.tree.0.max", "data.classes.ranger.tree.1.requires.0.level",
		"data.actors.scout.level", "data.actors.scout.face_shade", "data.actors.scout.face_shade_top",
		"data.states.sleep.restrict_chance", "data.events.numeric.do.0.image.fade",
		"data.events.numeric.do.0.image.auto", "data.events.numeric.do.0.image.page",
		"data.locations.maze.dungeon.depth", "data.locations.maze.dungeon.darkness",
		"data.locations.maze.dungeon.looks.0.darkness", "data.locations.maze.dungeon.generate.rooms",
		"data.locations.maze.dungeon.generate.loops", "data.locations.maze.dungeon.tiles.S.size",
		"game.meters.0.max", "game.card_bubbles.0.count", "game.card_bubbles.0.size",
		"game.card_bubbles.0.speed", "game.card_bubbles.0.rise",
	} {
		legacyNegativeRejected[name] = true
	}
	for _, f := range fields {
		t.Run(f.yaml, func(t *testing.T) {
			for _, value := range []string{"", "0", fmt.Sprint(f.positive), "-1", "null", "'2'", ".nan", ".inf", "-.inf"} {
				t.Run("新版/"+value, func(t *testing.T) {
					db, err := numericFixture(t, 1, f.yaml, value)
					bad := value == "-1" || value == "null" || value == "'2'" || strings.Contains(value, "inf") || value == ".nan" || (value == "0" && !f.zeroOK)
					if bad {
						key := f.yaml[strings.LastIndex(f.yaml, ".")+1:]
						if err == nil || !strings.Contains(err.Error(), key) {
							t.Fatalf("%s の診断なし %v", value, err)
						}
						return
					}
					if err != nil {
						t.Fatal(err)
					}
					want := f.missing
					if value != "" {
						want, err = strconv.ParseFloat(value, 64)
						if err != nil {
							t.Fatal(err)
						}
					}
					if got := numericValue(db, f.goPath); got != want {
						t.Fatalf("実効値 %v、期待 %v", got, want)
					}
				})
			}
			for _, value := range []string{"", fmt.Sprint(f.positive), "-1"} {
				t.Run("旧版/"+value, func(t *testing.T) {
					old, err := numericFixture(t, 0, f.yaml, value)
					if value == "-1" && legacyNegativeRejected[f.yaml] {
						if err == nil {
							t.Fatal("旧版の負数の拒否が消えた")
						}
						return
					}
					if err != nil {
						t.Fatal(err)
					}
					want := f.legacyZero
					if value == "" && f.missing == nil {
						want = nil
					}
					if value != "" {
						want, err = strconv.ParseFloat(value, 64)
						if err != nil {
							t.Fatal(err)
						}
					}
					if f.yaml == "game.screen.height" {
						want = 720.
					}
					if got := numericValue(old, f.goPath); got != want {
						t.Fatalf("旧版の実効値 %v、期待 %v", got, want)
					}
				})
			}
			old, err := numericFixture(t, 0, f.yaml, "0")
			if err != nil {
				t.Fatal(err)
			}
			if got := numericValue(old, f.goPath); got != f.legacyZero {
				t.Fatalf("旧版の０ %v、期待 %v", got, f.legacyZero)
			}
		})
	}
	t.Run("暗さを適用する", func(t *testing.T) {
		for _, version := range []int{0, 1} {
			for _, value := range []string{"", "0", "0.3"} {
				db, err := numericFixture(t, version, "data.locations.maze.dungeon.looks.0.darkness", value)
				if err != nil {
					t.Fatal(err)
				}
				st := testgame.State(db, 1)
				st.Location = "maze"
				want := 0.5
				if value != "" {
					want, err = strconv.ParseFloat(value, 64)
					if err != nil {
						t.Fatal(err)
					}
				}
				if got := dungeon.Styled(db, st).Dark; got != want {
					t.Fatalf("版%d 指定%s 適用後 %v、期待 %v", version, value, got, want)
				}
				if db.Locations["maze"].Dungeon.Dark != 0.5 {
					t.Fatal("継承元を書き換えた")
				}
			}
		}
	})
}

func TestNumericPresenceSP(t *testing.T) {
	for _, version := range []int{0, 1} {
		db, err := numericFixture(t, version, "data.classes.ranger.sp", "0")
		if err != nil {
			t.Fatal(err)
		}
		st := testgame.State(db, 1)
		a := st.Actors["scout"]
		before := a.SP
		a.GainExp(1000)
		if a.Level <= 1 {
			t.Fatal("レベルが上がっていない")
		}
		want := before
		if version == 0 {
			want += a.Level - 1
		}
		if a.SP != want {
			t.Fatalf("版%d SP=%d、期待%d", version, a.SP, want)
		}
	}
}

func TestNumericPresenceBounds(t *testing.T) {
	for _, target := range []string{"data.locations.maze.dungeon.generate.width", "data.locations.maze.dungeon.generate.height"} {
		for _, value := range []string{"", "0", "-1", "8", "10", "null", "'9'", "9.5"} {
			if _, err := numericFixture(t, 1, target, value); err == nil {
				t.Errorf("%s=%s を受け入れた", target, value)
			}
		}
		if _, err := numericFixture(t, 1, target, "11"); err != nil {
			t.Error(err)
		}
	}
	for _, value := range []string{"0.5", "1.5"} {
		if _, err := numericFixture(t, 1, "data.actors.scout.level", value); err == nil {
			t.Fatal("整数の小数を受け入れた")
		}
	}
	for _, target := range []string{"data.locations.maze.dungeon.darkness", "data.locations.maze.dungeon.looks.0.darkness", "data.locations.maze.dungeon.generate.loops", "game.ui.sfx_vary.pitch"} {
		if _, err := numericFixture(t, 1, target, "1.1"); err == nil {
			t.Fatal("上限超過を受け入れた", target)
		}
	}
	if _, err := numericFixture(t, 1, "game.battle.auto_tempo", "0.5"); err == nil {
		t.Fatal("実効値にならないオート倍率を受け入れた")
	}
}

func TestNumericPresenceDetails(t *testing.T) {
	for _, value := range []string{"", "0", "0.8", "-1", "null", "'0.8'", ".nan", ".inf"} {
		_, err := numericFixture(t, 1, "game.ui.stage_scale.sprite", value)
		bad := value != "" && value != "0.8"
		if bad && (err == nil || !strings.Contains(err.Error(), "stage_scale")) {
			t.Fatalf("立ち絵の倍率%sの診断なし %v", value, err)
		}
		if !bad && err != nil {
			t.Fatal(err)
		}
	}
	for _, tc := range []struct {
		top  string
		want float64
	}{{"", 0.6}, {", face_shade_top: 0", 0}, {", face_shade_top: 0.2", 0.2}} {
		db := testgame.Load(t, map[string]string{
			"game.yaml":                  "schema_version: 1\nid: neutral\n" + testgame.Game,
			"data/main.yaml":             strings.Replace(testgame.Data, "scout: {name: Scout,", "scout: {faces: portrait, face_shade: 0.6"+tc.top+", name: Scout,", 1),
			"assets/portrait/normal.png": "x",
		})
		if db.Actors["scout"].FaceShadeTop != tc.want {
			t.Fatalf("上端の暗さが継承・上書きされていない %+v", db.Actors["scout"])
		}
	}
	t.Run("Goからの変更も補完せず検査する", func(t *testing.T) {
		db, err := numericFixture(t, 1, "data.classes.ranger.sp", "0")
		if err != nil {
			t.Fatal(err)
		}
		db.Classes["ranger"].SP = -1
		db.Enemies["mote"].Actions = -1
		errs := fmt.Sprint(db.Check())
		for _, key := range []string{".sp", ".actions"} {
			if !strings.Contains(errs, key) {
				t.Fatal("Go の不正値を診断しない", errs)
			}
		}
		if db.Classes["ranger"].SP != -1 {
			t.Fatal("Check が値を補完した")
		}
	})
}
