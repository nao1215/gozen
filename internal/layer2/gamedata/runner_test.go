package gamedata_test

import (
	"math"
	"reflect"
	"strings"
	"testing"

	data "github.com/nao1215/gozen/internal/layer2/gamedata"
	"github.com/nao1215/gozen/internal/testgame"
)

const runnerDefinition = `
runners:
  trail:
    width: 5
    view_rows: 8
    speed: 4
    steer_speed: 8
    steer_start: 3
    ramp_ticks: 12
    boost_rate: 0.5
    boost_max: 1.3
    lives: 2
    max_lives: 4
    life_every: 5
    stock: 1
    max_stock: 3
    retry_rows: 10
    clear_ticks: 20
    tiles:
      '.': {kind: empty}
      '#': {kind: wall}
      '+': {kind: pickup, value: 10}
      'L': {kind: life}
      'A': {kind: stock}
    stages:
      - id: garden
        rows: ['.....', '#.+.#', '..L..', '..A..']
      - id: bridge
        speed: 5
        steer_speed: 9
        rows: ['.#...', '...#.']
`

func loadRunner(t *testing.T) *data.RunnerDefinition {
	t.Helper()
	db, err := testgame.TryLoad(map[string]string{"game.yaml": testgame.Game + "\nschema_version: 12\nid: runner-test\n" + runnerDefinition})
	if err != nil {
		t.Fatal(err)
	}
	return db.Game.Runners["trail"]
}

func TestRunnerLoadDefaultsAndValidation(t *testing.T) {
	r := loadRunner(t)
	if r.PlayerRow != 5 || r.StartX != 2.5 || r.Radius != .15 || r.FinishRows != 8 || r.Ahead != "....." {
		t.Fatalf("incorrect geometry defaults: %+v", r)
	}
	if r.Stages[1].Speed != 5 || r.Stages[1].SteerSpeed != 9 || r.Tiles["+"].Value != 10 {
		t.Fatal("authored stage or tile values changed")
	}
	before := *r
	if err := r.Validate(); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(before, *r) {
		t.Fatal("validation mutated the definition")
	}
	for _, tc := range []struct {
		name, want string
		change     func(*data.RunnerDefinition)
	}{
		{"width", "width", func(r *data.RunnerDefinition) { r.Width = 65 }},
		{"player beyond screen", "player_row", func(r *data.RunnerDefinition) { r.PlayerRow = r.ViewRows }},
		{"nan speed", "speed", func(r *data.RunnerDefinition) { r.Speed = math.NaN() }},
		{"infinite start", "start_x", func(r *data.RunnerDefinition) { r.StartX = math.Inf(1) }},
		{"fast scroll", "speed times boost_max", func(r *data.RunnerDefinition) { r.Speed = 60 }},
		{"stage too fast", "stages[0].speed", func(r *data.RunnerDefinition) { r.Stages[0].Speed = 60 }},
		{"negative boost", "boost_rate", func(r *data.RunnerDefinition) { r.BoostRate = -1 }},
		{"invalid radius", "radius", func(r *data.RunnerDefinition) { r.Radius = .5 }},
		{"initial lives above max", "max_lives", func(r *data.RunnerDefinition) { r.MaxLives = 1 }},
		{"unknown kind", "unknown kind", func(r *data.RunnerDefinition) { r.Tiles["+"] = data.RunnerTile{Kind: "script"} }},
		{"wall value", "only collectible", func(r *data.RunnerDefinition) { r.Tiles["#"] = data.RunnerTile{Kind: "wall", Value: 1} }},
		{"missing empty", "define '.' as empty", func(r *data.RunnerDefinition) { delete(r.Tiles, ".") }},
		{"row width", "must have width", func(r *data.RunnerDefinition) { r.Stages[0].Rows[0] = "...." }},
		{"unknown cell", "unknown tile", func(r *data.RunnerDefinition) { r.Stages[0].Rows[0] = "..?.." }},
		{"unknown preview cell", "ahead references", func(r *data.RunnerDefinition) { r.Ahead = "..X.." }},
		{"duplicate stage", "unique valid id", func(r *data.RunnerDefinition) { r.Stages[1].ID = "garden" }},
		{"empty course", "1 to 256", func(r *data.RunnerDefinition) { r.Stages = nil }},
		{"empty stage", "must not be empty", func(r *data.RunnerDefinition) { r.Stages[0].Rows = nil }},
		{"course budget", "65536", func(r *data.RunnerDefinition) { r.Stages[0].Rows = make([]string, 65537) }},
		{"stage budget", "1 to 256", func(r *data.RunnerDefinition) { r.Stages = make([]data.RunnerStage, 257) }},
		{"finish too early", "finish_rows", func(r *data.RunnerDefinition) { r.FinishRows = 1 }},
		{"finish too fast", "finish_speed", func(r *data.RunnerDefinition) { r.FinishSpeed = 60 }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := loadRunner(t)
			tc.change(r)
			if err := r.Validate(); err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("want %q, got %v", tc.want, err)
			}
		})
	}
	var absent *data.RunnerDefinition
	if err := absent.Validate(); err == nil {
		t.Fatal("accepted nil definition")
	}
}

func TestRunnerVersionAssetsAndZeroSemantics(t *testing.T) {
	for _, tc := range []struct{ name, game, want string }{
		{"old schema", "schema_version: 11\nid: runner-test\n" + runnerDefinition, "schema_version: 12"},
		{"null runner", "schema_version: 12\nid: runner-test\nrunners: {trail: null}", "expected a definition map"},
		{"missing player", "schema_version: 12\nid: runner-test\n" + strings.Replace(runnerDefinition, "    width: 5", "    width: 5\n    player_image: absent.png", 1), "absent.png"},
		{"missing stage background", "schema_version: 12\nid: runner-test\n" + strings.Replace(runnerDefinition, "      - id: garden", "      - id: garden\n        background: absent.png", 1), "absent.png"},
		{"missing tile", "schema_version: 12\nid: runner-test\n" + strings.Replace(runnerDefinition, "{kind: wall}", "{kind: wall, image: absent.png}", 1), "absent.png"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := testgame.TryLoad(map[string]string{"game.yaml": testgame.Game + "\n" + tc.game})
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("want %q, got %v", tc.want, err)
			}
		})
	}
	r := loadRunner(t)
	r.Lives, r.MaxLives, r.LifeEvery, r.Stock, r.MaxStock, r.RetryRows, r.ClearTicks, r.RampTicks = 0, 0, 0, 0, 0, 0, 0, 0
	r.BoostRate, r.BoostMax = 0, 1
	if err := r.Validate(); err != nil {
		t.Fatalf("zero disables optional rules: %v", err)
	}
}

func TestRunnerVisualBounds(t *testing.T) {
	for _, tc := range []struct {
		name    string
		visuals data.RunnerVisuals
	}{
		{"invalid sprite size", data.RunnerVisuals{PlayerSize: []float64{0, 1}}},
		{"invalid sprite offset", data.RunnerVisuals{PlayerOffset: []float64{5, 0}}},
		{"wrong sprite vector", data.RunnerVisuals{PlayerSize: []float64{1}}},
		{"wrong rectangle length", data.RunnerVisuals{Board: []float64{0, 0, 10}}},
		{"zero rectangle", data.RunnerVisuals{Portrait: []float64{0, 0, 10, 0}}},
		{"nan rectangle", data.RunnerVisuals{Board: []float64{0, 0, math.NaN(), 10}}},
		{"ready budget", data.RunnerVisuals{ReadyTicks: 3601}},
		{"cutin budget", data.RunnerVisuals{CutinTicks: -1}},
		{"too many idle images", data.RunnerVisuals{Idle: make([]string, 6)}},
		{"unknown reaction", data.RunnerVisuals{Reactions: map[string]data.RunnerReaction{"typo": {Images: []string{"portrait.png"}, Ticks: 1}}}},
		{"reaction no image", data.RunnerVisuals{Reactions: map[string]data.RunnerReaction{"pickup": {Ticks: 1}}}},
		{"reaction no duration", data.RunnerVisuals{Reactions: map[string]data.RunnerReaction{"pickup": {Images: []string{"portrait.png"}}}}},
		{"reaction priority budget", data.RunnerVisuals{Reactions: map[string]data.RunnerReaction{"pickup": {Images: []string{"portrait.png"}, Ticks: 1, Priority: 101}}}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := loadRunner(t)
			r.Visuals = &tc.visuals
			if err := r.Validate(); err == nil {
				t.Fatal("accepted invalid visual definition")
			}
		})
	}
}
