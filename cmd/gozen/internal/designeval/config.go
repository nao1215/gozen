package designeval

import (
	"fmt"
	"io/fs"
	"math"
	"regexp"

	"github.com/nao1215/gozen/cmd/internal/cli"
	"github.com/nao1215/gozen/internal/layer1/decodeguard"

	"github.com/goccy/go-yaml"

	data "github.com/nao1215/gozen/schema"
)

type Config struct {
	Debug    func(string, ...any) `yaml:"-" json:"-"`
	Time     Time                 `yaml:"time"`
	Game     Game                 `yaml:"game"`
	Rewards  []Rule               `yaml:"rewards"`
	Expects  []Rule               `yaml:"expects"`
	Release  string               `yaml:"release"`
	Targets  Targets              `yaml:"targets"`
	Memos    Memos                `yaml:"memos"`
	Duels    Duels                `yaml:"duels"`
	Personas []Persona            `yaml:"personas"`
}

type Duels struct {
	Tries    int       `yaml:"tries"`
	Fighters []string  `yaml:"fighters"`
	First    []float64 `yaml:"first"`
	Later    float64   `yaml:"later"`
	Personas []string  `yaml:"personas"`
	List     []Duel    `yaml:"list"`
}

type Duel struct {
	Troop   string    `yaml:"troop"`
	Rematch string    `yaml:"rematch"`
	Open    data.Expr `yaml:"open"`
	Later   data.Expr `yaml:"later"`
}

type Time struct {
	CharsPerSec float64     `yaml:"chars_per_sec"`
	Line        float64     `yaml:"line"`
	Notice      float64     `yaml:"notice"`
	Choice      float64     `yaml:"choice"`
	Step        float64     `yaml:"step"`
	Turn        float64     `yaml:"turn"`
	Move        float64     `yaml:"move"`
	Menu        float64     `yaml:"menu"`
	Battle      float64     `yaml:"battle"`
	BattleTurn  float64     `yaml:"battle_turn"`
	Retry       float64     `yaml:"retry"`
	Shop        float64     `yaml:"shop"`
	Stranded    float64     `yaml:"stranded"`
	Images      []ImageTime `yaml:"images"`
}

type ImageTime struct {
	Match string  `yaml:"match"`
	Sec   float64 `yaml:"sec"`
	re    *regexp.Regexp
}

type Game struct {
	Dungeon     string         `yaml:"dungeon"`
	Gate        string         `yaml:"gate"`
	Dive        []string       `yaml:"dive"`
	DiveWords   []string       `yaml:"dive_words"`
	Inn         string         `yaml:"inn"`
	Sleep       string         `yaml:"sleep"`
	Talk        string         `yaml:"talk"`
	SleepWords  []string       `yaml:"sleep_words"`
	Evening     []string       `yaml:"evening"`
	Town        []string       `yaml:"town"`
	NotByDay    []string       `yaml:"not_by_day"`
	Shops       []string       `yaml:"shops"`
	Supplies    map[string]int `yaml:"supplies"`
	SupplyShop  string         `yaml:"supply_shop"`
	Stones      bool           `yaml:"stones"`
	BackItem    string         `yaml:"back_item"`
	BackEvent   string         `yaml:"back_event"`
	Stairs      string         `yaml:"stairs"`
	StairsWords []string       `yaml:"stairs_words"`
	GuardTile   string         `yaml:"guard_tile"`
	GateTile    string         `yaml:"gate_tile"`
	GateWords   []string       `yaml:"gate_words"`
	HomeWords   []string       `yaml:"home_words"`
	Bosses      map[int]string `yaml:"bosses"`
	BossDone    data.Expr      `yaml:"boss_done"`
	Floor       data.Expr      `yaml:"floor"`
	Deepest     data.Expr      `yaml:"deepest"`
	Cleared     data.Expr      `yaml:"cleared"`
	Place       string         `yaml:"place"`
	OptIn       []string       `yaml:"opt_in"`
	Never       []string       `yaml:"never"`
	Prefer      []string       `yaml:"prefer"`
	SkipTiles   string         `yaml:"skip_tiles"`
	RepeatTiles string         `yaml:"repeat_tiles"`
	WornHP      float64        `yaml:"worn_hp"`
	WornMP      float64        `yaml:"worn_mp"`
	Retry       int            `yaml:"retry"`
	MaxTurns    int            `yaml:"max_turns"`
	Days        int            `yaml:"days"`
	Tries       int            `yaml:"tries"`
}

type Rule struct {
	Kind   string            `yaml:"kind"`
	Label  string            `yaml:"label"`
	Flow   string            `yaml:"flow"`
	Who    string            `yaml:"who"`
	Names  map[string]string `yaml:"names"`
	Images string            `yaml:"images"`
	First  bool              `yaml:"first"`
	Events string            `yaml:"events"`
	When   data.Expr         `yaml:"when"`
	Value  data.Expr         `yaml:"value"`
	Skills bool              `yaml:"skills"`
	Unless string            `yaml:"unless"`
	re     *regexp.Regexp
}

type Targets struct {
	Persona   string  `yaml:"persona"`
	CloseTo   string  `yaml:"close_to"`
	Within    float64 `yaml:"within"`
	MustClear string  `yaml:"must_clear"`
}

type Memos struct {
	Dry       string  `yaml:"dry"`
	Gap       string  `yaml:"gap"`
	Boss      string  `yaml:"boss"`
	BossBelow float64 `yaml:"boss_below"`
	Stuck     string  `yaml:"stuck"`
}

type Persona struct {
	ID          string   `yaml:"id"`
	Name        string   `yaml:"name"`
	Desc        string   `yaml:"desc"`
	Seed        uint64   `yaml:"seed"`
	Floors      int      `yaml:"floors"`
	Explore     string   `yaml:"explore"`
	Prefer      []string `yaml:"prefer"`
	Avoid       []string `yaml:"avoid"`
	Policies    []Policy `yaml:"policies"`
	Rotate      []string `yaml:"rotate"`
	Town        []Visit  `yaml:"town"`
	Night       []string `yaml:"night"`
	NightAlways bool     `yaml:"night_always"`
	Grind       float64  `yaml:"grind"`
	Days        int      `yaml:"days"`
}

type Visit struct {
	At     string    `yaml:"at"`
	Do     string    `yaml:"do"`
	When   data.Expr `yaml:"when"`
	Words  []string  `yaml:"words"`
	Rotate []string  `yaml:"rotate"`
}

func Load(fsys fs.FS, path string) (*Config, error) {
	b, err := fs.ReadFile(fsys, path)
	if err != nil {
		return nil, err
	}
	var c Config
	if err := decodeguard.Run(func() error { return yaml.UnmarshalWithOptions(b, &c, yaml.Strict()) }); err != nil {
		return nil, fmt.Errorf("%s: %s", path, yaml.FormatError(err, false, true))
	}
	return &c, c.check(nil)
}

func (c *Config) Validate(db *data.DB) error { return c.check(db) }
func (c *Config) check(db *data.DB) error {
	if c.Game.Days < 0 || c.Game.Tries < 0 || c.Game.MaxTurns < 0 || c.Duels.Tries < 0 {
		return fmt.Errorf("counts must be nonnegative")
	}
	for _, v := range []float64{c.Time.CharsPerSec, c.Time.Line, c.Time.Notice, c.Time.Choice, c.Time.Step, c.Time.Turn, c.Time.Move, c.Time.Menu, c.Time.Battle, c.Time.BattleTurn, c.Time.Retry, c.Time.Shop, c.Time.Stranded} {
		if math.IsNaN(v) || math.IsInf(v, 0) || v < 0 {
			return fmt.Errorf("time coefficients must be finite and nonnegative")
		}
	}
	for i := range c.Time.Images {
		re, err := regexp.Compile(c.Time.Images[i].Match)
		if err != nil {
			return fmt.Errorf("time.images[%d]: %w", i, err)
		}
		c.Time.Images[i].re = re
	}
	for _, rs := range [][]Rule{c.Rewards, c.Expects} {
		for i := range rs {
			r := &rs[i]
			n := 0
			for _, set := range []bool{r.Images != "", r.Events != "", r.Value.Set(), r.Skills} {
				if set {
					n++
				}
			}
			if n != 1 {
				return fmt.Errorf("rule %q (%s): write one of images, events, value, skills", r.Label, r.Kind)
			}
			src := r.Images + r.Events
			if src != "" {
				re, err := regexp.Compile(src)
				if err != nil {
					return fmt.Errorf("rule %q: %w", r.Label, err)
				}
				r.re = re
			}
			if r.Kind == "" {
				return fmt.Errorf("rule %q: no kind", r.Label)
			}
		}
	}
	if len(c.Personas) == 0 {
		return fmt.Errorf("no personas")
	}
	seen := map[string]bool{}
	for _, p := range c.Personas {
		if p.Days < 0 || p.Floors < 0 || math.IsNaN(p.Grind) || math.IsInf(p.Grind, 0) || p.Grind < 0 || p.Grind > 1 {
			return fmt.Errorf("persona %q: invalid days, floors or grind", p.ID)
		}
		if !cli.ID(p.ID) || seen[p.ID] {
			return fmt.Errorf("persona %q: no id or the same id twice", p.ID)
		}
		seen[p.ID] = true
		switch p.Explore {
		case "", "stairs", "events", "all":
		default:
			return fmt.Errorf("persona %s: explore %q (want stairs, events or all)", p.ID, p.Explore)
		}
	}
	for _, id := range []string{c.Targets.Persona, c.Targets.CloseTo, c.Targets.MustClear} {
		if id != "" && !seen[id] {
			return fmt.Errorf("targets: unknown persona %q", id)
		}
	}
	if err := c.checkChoiceIDs(db); err != nil {
		return err
	}
	if err := c.checkPolicies(db); err != nil {
		return err
	}
	if db == nil {
		return nil
	}
	for _, k := range []string{c.Memos.Dry, c.Memos.Gap, c.Memos.Boss, c.Memos.Stuck} {
		ok := k == ""
		for _, mk := range db.Game.Memo.Kinds {
			ok = ok || mk.ID == k
		}
		if !ok {
			return fmt.Errorf("memos: %q is not a kind of game.yaml memo.kinds", k)
		}
	}
	g := c.Game
	locs := append([]string{g.Dungeon, g.Gate, g.Inn}, g.Evening...)
	locs = append(locs, g.Town...)
	for _, p := range c.Personas {
		for _, v := range p.Town {
			locs = append(locs, v.At)
		}
	}
	for _, l := range locs {
		if l != "" && db.Locations[l] == nil {
			return fmt.Errorf("game: unknown location %q", l)
		}
	}
	if g.Dungeon != "" && db.Locations[g.Dungeon].Dungeon == nil {
		return fmt.Errorf("game.dungeon %q is not a dungeon", g.Dungeon)
	}
	var items []string
	if g.BackItem != "" {
		items = append(items, g.BackItem)
	}
	for id := range g.Supplies {
		items = append(items, id)
	}
	for _, id := range items {
		if db.Items[id] == nil {
			return fmt.Errorf("game: unknown item %q", id)
		}
	}
	for _, id := range append(append([]string{}, g.Shops...), g.SupplyShop) {
		if id != "" && db.Shops[id] == nil {
			return fmt.Errorf("game: unknown shop %q", id)
		}
	}
	for f, t := range g.Bosses {
		if db.Troops[t] == nil {
			return fmt.Errorf("game.bosses %d: unknown troop %q", f, t)
		}
	}
	if n := len(c.Duels.First); n != 0 && n != 2 {
		return fmt.Errorf("duels.first: write the low and the high (got %d numbers)", n)
	}
	for _, id := range c.Duels.Personas {
		if !seen[id] {
			return fmt.Errorf("duels.personas: unknown persona %q", id)
		}
	}
	for _, d := range c.Duels.List {
		for _, t := range []string{d.Troop, d.Rematch} {
			if t != "" && db.Troops[t] == nil {
				return fmt.Errorf("duels: unknown troop %q", t)
			}
		}
	}
	for _, id := range c.Duels.Fighters {
		if db.Actors[id] == nil {
			return fmt.Errorf("duels.fighters: unknown actor %q", id)
		}
	}
	if g.BackEvent != "" && db.Events[g.BackEvent] == nil {
		return fmt.Errorf("game.back_event: unknown event %q", g.BackEvent)
	}
	return nil
}

func (c *Config) defaults() {
	t := &c.Time
	if t.CharsPerSec <= 0 {
		t.CharsPerSec = 12
	}
	g := &c.Game
	if g.MaxTurns == 0 {
		g.MaxTurns = 60
	}
	if g.Days == 0 {
		g.Days = 30
	}
	if g.Tries == 0 {
		g.Tries = 10
	}
	if c.Duels.Tries == 0 {
		c.Duels.Tries = 20
	}
	if g.Stairs == "" {
		g.Stairs = ">"
	}
}
