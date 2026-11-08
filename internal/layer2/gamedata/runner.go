package gamedata

import (
	"errors"
	"fmt"
	"maps"
	"slices"
	"strings"

	"github.com/nao1215/gozen/internal/diag"
)

// RunnerDefinition is an authored finite scrolling course consumed by the
// casual rules and player. Loading applies defaults; Validate never mutates it.
// Rows use printable ASCII tile keys and are ordered by their arrival in view.
type RunnerDefinition struct {
	Visuals          *RunnerVisuals        `yaml:"visuals"`
	Title            string                `yaml:"title"`
	Width            int                   `yaml:"width"`
	ViewRows         int                   `yaml:"view_rows"`
	PlayerRow        int                   `yaml:"player_row"`
	StartX           float64               `yaml:"start_x"`
	Radius           float64               `yaml:"radius"`
	Speed            float64               `yaml:"speed"`
	SteerSpeed       float64               `yaml:"steer_speed"`
	SteerStart       float64               `yaml:"steer_start"`
	RampTicks        int                   `yaml:"ramp_ticks"`
	BoostRate        float64               `yaml:"boost_rate"`
	BoostMax         float64               `yaml:"boost_max"`
	Lives            int                   `yaml:"lives"`
	MaxLives         int                   `yaml:"max_lives"`
	LifeEvery        int                   `yaml:"life_every"`
	Stock            int                   `yaml:"stock"`
	MaxStock         int                   `yaml:"max_stock"`
	RetryRows        int                   `yaml:"retry_rows"`
	ClearTicks       int                   `yaml:"clear_ticks"`
	FinishRows       int                   `yaml:"finish_rows"`
	FinishSpeed      float64               `yaml:"finish_speed"`
	FinishSteerSpeed float64               `yaml:"finish_steer_speed"`
	Ahead            string                `yaml:"ahead"`
	PlayerImage      string                `yaml:"player_image"`
	PlayerColor      Color                 `yaml:"player_color"`
	Background       string                `yaml:"background"`
	PickupSound      string                `yaml:"pickup_sound"`
	ActionSound      string                `yaml:"action_sound"`
	FailSound        string                `yaml:"fail_sound"`
	FinishSound      string                `yaml:"finish_sound"`
	Tiles            map[string]RunnerTile `yaml:"tiles"`
	Stages           []RunnerStage         `yaml:"stages"`
}

// RunnerTile defines one course cell for the casual rules and renderer. Kind is
// empty, wall, pickup, life or stock. Value adds score for a collected cell;
// ordinary pickups also advance the one-per-cell extra-life counter.
type RunnerTile struct {
	Kind  string `yaml:"kind"`
	Value int    `yaml:"value"`
	Image string `yaml:"image"`
	Color Color  `yaml:"color"`
}

// RunnerStage groups rows and presentation for one course. Zero Speed and
// SteerSpeed inherit the definition's rates; an empty Background inherits it too.
type RunnerStage struct {
	Clear      bool     `yaml:"clear"`
	ID         string   `yaml:"id"`
	Speed      float64  `yaml:"speed"`
	SteerSpeed float64  `yaml:"steer_speed"`
	Background string   `yaml:"background"`
	Rows       []string `yaml:"rows"`
}

func (r *RunnerDefinition) defaults() {
	if r.Width == 0 {
		r.Width = 9
	}
	if r.ViewRows == 0 {
		r.ViewRows = 16
	}
	if r.PlayerRow == 0 {
		r.PlayerRow = r.ViewRows - 3
	}
	if r.StartX == 0 {
		r.StartX = float64(r.Width) / 2
	}
	if r.Radius == 0 {
		r.Radius = .15
	}
	if r.Speed == 0 {
		r.Speed = 3
	}
	if r.SteerSpeed == 0 {
		r.SteerSpeed = 7
	}
	if r.SteerStart == 0 {
		r.SteerStart = r.SteerSpeed
	}
	if r.BoostMax == 0 {
		r.BoostMax = 1
	}
	if r.MaxLives == 0 {
		r.MaxLives = r.Lives
	}
	if r.MaxStock == 0 {
		r.MaxStock = r.Stock
	}
	if r.FinishRows == 0 {
		r.FinishRows = r.ViewRows
	}
	if r.Ahead == "" && r.Width >= 3 && r.Width <= 64 {
		r.Ahead = strings.Repeat(".", r.Width)
	}
}

// Validate checks a fully defaulted runner definition before the casual runtime
// allocates attempt state. It checks bounds and references without changing rows
// or tiles. Asset existence is checked by DB.Check against the game's filesystem.
func (r *RunnerDefinition) Validate() error {
	if r == nil {
		return errors.New("runner definition is null")
	}
	var errs []error
	number := func(name string, v, lo, hi float64) {
		if !finiteNumber(v) || v < lo || v > hi {
			errs = append(errs, fmt.Errorf("%s must be finite in [%g, %g]", name, lo, hi))
		}
	}
	for _, n := range []struct {
		name      string
		v, lo, hi int
	}{
		{"width", r.Width, 3, 64},
		{"view_rows", r.ViewRows, 4, 64},
		{"player_row", r.PlayerRow, 1, r.ViewRows - 2},
		{"ramp_ticks", r.RampTicks, 0, 3600},
		{"lives", r.Lives, 0, 999},
		{"max_lives", r.MaxLives, r.Lives, 999},
		{"life_every", r.LifeEvery, 0, 1000000},
		{"stock", r.Stock, 0, 999},
		{"max_stock", r.MaxStock, r.Stock, 999},
		{"retry_rows", r.RetryRows, 0, 65536},
		{"clear_ticks", r.ClearTicks, 0, 3600},
		{"finish_rows", r.FinishRows, r.PlayerRow + 2, 128},
	} {
		number(n.name, float64(n.v), float64(n.lo), float64(n.hi))
	}
	number("radius", r.Radius, .01, .49)
	number("start_x", r.StartX, r.Radius, float64(r.Width)-r.Radius)
	number("speed", r.Speed, .01, 60)
	number("steer_speed", r.SteerSpeed, .01, 60)
	number("steer_start", r.SteerStart, .01, r.SteerSpeed)
	number("boost_rate", r.BoostRate, 0, 10)
	number("boost_max", r.BoostMax, 1, 10)
	number("finish_speed", r.FinishSpeed, 0, 60/r.BoostMax)
	number("finish_steer_speed", r.FinishSteerSpeed, 0, 60)
	if r.FinishSteerSpeed > 0 && r.FinishSteerSpeed < r.SteerStart {
		errs = append(errs, errors.New("finish_steer_speed must not be below steer_start"))
	}
	if r.Speed*r.BoostMax > 60 {
		errs = append(errs, errors.New("speed times boost_max must not exceed 60 rows per second"))
	}
	errs = append(errs, r.validateTiles())
	row := func(where, text string) {
		if len(text) != r.Width {
			errs = append(errs, fmt.Errorf("%s must have width %d", where, r.Width))
			return
		}
		for _, key := range []byte(text) {
			if _, ok := r.Tiles[string(key)]; !ok {
				errs = append(errs, fmt.Errorf("%s references unknown tile %q", where, string(key)))
				break
			}
		}
	}
	row("ahead", r.Ahead)
	if len(r.Stages) < 1 || len(r.Stages) > 256 {
		errs = append(errs, errors.New("stages must contain 1 to 256 stages"))
	}
	ids, total := map[string]bool{}, 0
	for i, stage := range r.Stages {
		where := fmt.Sprintf("stages[%d]", i)
		if !ValidGameID(stage.ID) || ids[stage.ID] {
			errs = append(errs, fmt.Errorf("%s needs a unique valid id", where))
		}
		ids[stage.ID] = true
		number(where+".speed", stage.Speed, 0, 60/r.BoostMax)
		number(where+".steer_speed", stage.SteerSpeed, 0, 60)
		if stage.SteerSpeed > 0 && stage.SteerSpeed < r.SteerStart {
			errs = append(errs, fmt.Errorf("%s.steer_speed must not be below steer_start", where))
		}
		if len(stage.Rows) == 0 {
			errs = append(errs, fmt.Errorf("%s.rows must not be empty", where))
		}
		if len(stage.Rows) > 65536-total {
			errs = append(errs, errors.New("total course rows must not exceed 65536"))
			break
		}
		total += len(stage.Rows)
		for j, text := range stage.Rows {
			row(fmt.Sprintf("%s.rows[%d]", where, j), text)
		}
	}
	errs = append(errs, r.Visuals.validate())
	return errors.Join(errs...)
}

func (r *RunnerDefinition) validateTiles() error {
	var errs []error
	if len(r.Tiles) < 1 || len(r.Tiles) > 94 {
		errs = append(errs, errors.New("tiles must contain 1 to 94 printable ASCII keys"))
	}
	if tile, ok := r.Tiles["."]; !ok || tile.Kind != "empty" {
		errs = append(errs, errors.New("tiles must define '.' as empty"))
	}
	for _, key := range slices.Sorted(maps.Keys(r.Tiles)) {
		tile := r.Tiles[key]
		if len(key) != 1 || key[0] < '!' || key[0] > '~' {
			errs = append(errs, fmt.Errorf("tile %q needs one printable non-space ASCII key", key))
		}
		switch tile.Kind {
		case "empty", "wall":
			if tile.Value != 0 {
				errs = append(errs, fmt.Errorf("tile %q: only collectible tiles have value", key))
			}
		case "pickup", "life", "stock":
			if tile.Value < 0 || tile.Value > 1000000 {
				errs = append(errs, fmt.Errorf("tile %q value must be in [0, 1000000]", key))
			}
		default:
			errs = append(errs, fmt.Errorf("tile %q has unknown kind %q", key, tile.Kind))
		}
	}
	return errors.Join(errs...)
}

func (c *checker) runners() {
	if len(c.db.Game.Runners) == 0 {
		return
	}
	c.needsVersion("game.yaml runners", "runners", SchemaVersion12)
	if len(c.db.Game.Runners) > 256 {
		c.errorf(diag.OutOfRange, "game.yaml runners", "at most 256 runners are allowed")
	}
	for _, id := range slices.Sorted(maps.Keys(c.db.Game.Runners)) {
		r := c.db.Game.Runners[id]
		w := "game.yaml runners." + id
		if !ValidGameID(id) {
			c.errorf(diag.Shape, w, "invalid runner id")
		}
		if err := r.Validate(); err != nil {
			c.errorf(diag.Shape, w, "%s", err)
		}
		if r == nil {
			continue
		}
		for _, asset := range []struct{ name, file string }{
			{"player_image", r.PlayerImage}, {"background", r.Background},
		} {
			c.file(w+"."+asset.name, asset.file)
		}
		for _, sound := range []struct{ name, value string }{{"pickup_sound", r.PickupSound}, {"action_sound", r.ActionSound}, {"fail_sound", r.FailSound}, {"finish_sound", r.FinishSound}} {
			if sound.value != "" {
				c.sfxCue(w+"."+sound.name, SFXCue{Name: sound.value})
			}
		}
		for _, key := range slices.Sorted(maps.Keys(r.Tiles)) {
			c.file(w+".tiles."+key+".image", r.Tiles[key].Image)
		}
		if v := r.Visuals; v != nil {
			c.file(w+".visuals.cutin", v.Cutin)
			for i, file := range v.Idle {
				c.file(fmt.Sprintf("%s.visuals.idle[%d]", w, i), file)
			}
			for key, reaction := range v.Reactions {
				for i, file := range reaction.Images {
					c.file(fmt.Sprintf("%s.visuals.reactions.%s.images[%d]", w, key, i), file)
				}
			}
		}
		for i, stage := range r.Stages {
			c.file(fmt.Sprintf("%s.stages[%d].background", w, i), stage.Background)
		}
	}
}

// RunnerVisuals gives presentation optional authored layout and reactions. Rects
// are x,y,width,height in the logical canvas; omitted layout fits the viewport.
type RunnerVisuals struct {
	BoardBackground bool                      `yaml:"board_background"`
	BoardColor      Color                     `yaml:"board_color"`
	CanvasColor     Color                     `yaml:"canvas_color"`
	PlayerSize      []float64                 `yaml:"player_size"`
	PlayerOffset    []float64                 `yaml:"player_offset"`
	Board           []float64                 `yaml:"board"`
	Portrait        []float64                 `yaml:"portrait"`
	Idle            []string                  `yaml:"idle"`
	Reactions       map[string]RunnerReaction `yaml:"reactions"`
	ReadyTicks      int                       `yaml:"ready_ticks"`
	Cutin           string                    `yaml:"cutin"`
	CutinTicks      int                       `yaml:"cutin_ticks"`
}

// RunnerReaction selects a temporary portrait for a rules event. Images cycle
// on successive Every-sized streaks; omitted Every means every matching event.
type RunnerReaction struct {
	Event    string   `yaml:"event"`
	Images   []string `yaml:"images"`
	Ticks    int      `yaml:"ticks"`
	Priority int      `yaml:"priority"`
	Every    int      `yaml:"every"`
}

func (v *RunnerVisuals) validate() error {
	if v == nil {
		return nil
	}
	var errs []error
	for _, box := range [][]float64{v.Board, v.Portrait} {
		if len(box) == 0 {
			continue
		}
		if len(box) != 4 {
			errs = append(errs, errors.New("visual rectangle needs x,y,width,height"))
			continue
		}
		for i, n := range box {
			if !finiteNumber(n) || n < 0 || n > 16384 || i >= 2 && n == 0 {
				errs = append(errs, errors.New("visual rectangle must be finite and positive-sized"))
				break
			}
		}
	}
	for i, vector := range [][]float64{v.PlayerSize, v.PlayerOffset} {
		if len(vector) == 0 {
			continue
		}
		if len(vector) != 2 {
			errs = append(errs, errors.New("player size and offset need two components"))
			continue
		}
		for _, n := range vector {
			if !finiteNumber(n) || n < -4 || n > 4 || i == 0 && n <= 0 {
				errs = append(errs, errors.New("player size and offset must be finite within four cells, with positive size"))
				break
			}
		}
	}
	if len(v.Idle) > 5 || len(v.Reactions) > 32 || v.ReadyTicks < 0 || v.ReadyTicks > 3600 || v.CutinTicks < 0 || v.CutinTicks > 3600 {
		errs = append(errs, errors.New("runner visuals exceed bounded counts or timing"))
	}
	for key, r := range v.Reactions {
		name := r.Event
		if name == "" {
			name = key
		}
		event, _, _ := strings.Cut(name, ":")
		switch event {
		case "pickup", "life", "stock", "miss", "near_wall", "crash", "failed", "course", "completed", "cancelled", "retry", "action", "stage_clear":
		default:
			errs = append(errs, fmt.Errorf("unknown portrait reaction event %q", event))
		}
		for _, image := range r.Images {
			if image == "" {
				errs = append(errs, fmt.Errorf("empty reaction image in %q", key))
			}
		}
		if key == "" || len(key) > 64 || len(r.Images) < 1 || len(r.Images) > 16 || r.Ticks < 1 || r.Ticks > 3600 || r.Priority < 0 || r.Priority > 100 || r.Every < 0 || r.Every > 1000000 {
			errs = append(errs, fmt.Errorf("invalid portrait reaction %q", key))
		}
	}
	return errors.Join(errs...)
}
