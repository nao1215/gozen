package gamedata

import (
	"cmp"
	"fmt"
	"io/fs"
	"path"
	"sort"
	"strings"

	lang "github.com/nao1215/gozen/internal/text"

	"github.com/nao1215/gozen/internal/text/catalog"

	"github.com/goccy/go-yaml"
)

// DB is a loaded game: game.yaml and every section of the data files.
type DB struct {
	FS        fs.FS // the game's directory
	Game      Game
	Classes   map[string]*Class
	Actors    map[string]*Actor
	Items     map[string]*Item
	Skills    map[string]*Skill
	States    map[string]*State
	Enemies   map[string]*Enemy
	Troops    map[string]*Troop
	Locations map[string]*Location
	Events    map[string]*Event
	Shops     map[string]*Shop
	AI        map[string][]Rule
	Elements  map[string]string // element id -> name
	StoneSets map[string]*StoneSet
	Fusions   []Fusion
	Music     Music
	// Profiles are the pages of the character book (ADR 0031).
	Profiles map[string]*Profile
	// Regions and Factions are a campaign's (schema_version 5, ADR 0030).
	Regions  map[string]*Region
	Factions map[string]*Faction

	TacticConditions map[string]*TacticCondition
	TacticTargets    map[string]*TacticTarget
	TacticActions    map[string]*TacticAction

	// Order is the ids of each section in the order they were written (files in name
	// order), for the menus.
	Order map[string][]string

	standIns    map[string]bool            // placeholder full-screen picture (stand_ins in game.yaml; see standin.go)
	sources     map[string]sourcePosition  // logical paths after completion and merging, and their original positions.
	sourceFiles map[string]*sourceDocument // per-file positions before type conversion.

	// sceneWarns are the warnings of the maps and stages that Check found (SceneWarnings).
	sceneWarns  []error
	sceneWarned bool
	// flagWarns are the warnings of flags that Check found (FlagWarnings).
	flagWarns []error

	// Words are the translations of lang/<code>.yaml by language, source to translation
	// (ADR 0027). shown is the language shown, names the sources of the names Show translates.
	Words map[string]map[string]string
	shown string
	names map[*string]string
}

// file is one data file: any of the sections.
type file struct {
	Classes   map[string]*Class    `yaml:"classes"`
	Actors    map[string]*Actor    `yaml:"actors"`
	Items     map[string]*Item     `yaml:"items"`
	Skills    map[string]*Skill    `yaml:"skills"`
	States    map[string]*State    `yaml:"states"`
	Enemies   map[string]*Enemy    `yaml:"enemies"`
	Troops    map[string]*Troop    `yaml:"troops"`
	Locations map[string]*Location `yaml:"locations"`
	Events    map[string]*Event    `yaml:"events"`
	Shops     map[string]*Shop     `yaml:"shops"`
	AI        map[string][]Rule    `yaml:"ai"`
	Elements  map[string]string    `yaml:"elements"`
	StoneSets map[string]*StoneSet `yaml:"stone_sets"`
	Fusions   []Fusion             `yaml:"fusions"`
	Music     *Music               `yaml:"music"`
	Profiles  map[string]*Profile  `yaml:"profiles"`
	Regions   map[string]*Region   `yaml:"regions"`
	Factions  map[string]*Faction  `yaml:"factions"`

	TacticConditions map[string]*TacticCondition `yaml:"tactic_conditions"`
	TacticTargets    map[string]*TacticTarget    `yaml:"tactic_targets"`
	TacticActions    map[string]*TacticAction    `yaml:"tactic_actions"`
}

// Load reads game.yaml and every data/**/*.yaml of the game in fsys, and checks them
// (see Check). The error lists every problem found.
func Load(fsys fs.FS) (*DB, error) {
	db := &DB{
		TacticConditions: map[string]*TacticCondition{}, TacticTargets: map[string]*TacticTarget{}, TacticActions: map[string]*TacticAction{},
		FS:      fsys,
		Classes: map[string]*Class{}, Actors: map[string]*Actor{}, Items: map[string]*Item{},
		Skills: map[string]*Skill{}, States: map[string]*State{}, Enemies: map[string]*Enemy{},
		Troops: map[string]*Troop{}, Locations: map[string]*Location{}, Events: map[string]*Event{},
		Shops: map[string]*Shop{}, AI: map[string][]Rule{}, Elements: map[string]string{}, StoneSets: map[string]*StoneSet{},
		Profiles: map[string]*Profile{},
		Order:    map[string][]string{}, sources: map[string]sourcePosition{}, sourceFiles: map[string]*sourceDocument{},
		Music:   Music{Volume: 0.7},
		Regions: map[string]*Region{}, Factions: map[string]*Faction{},
	}
	b, err := fs.ReadFile(fsys, "game.yaml")
	if err != nil {
		return nil, err
	}
	doc, err := db.readSource(b, "game.yaml")
	if err != nil {
		return nil, err
	}
	// Before interpreting items of the new spec as the old spec, check that the version is supported.
	if err := checkVersionHeader(b); err != nil {
		return nil, doc.locateReported(err)
	}
	rawGame, err := doc.decodeDefinitions(&db.Game)
	if err != nil {
		return nil, err
	}
	if errs := gameVersionErrors(db.Game); len(errs) > 0 {
		return nil, doc.locateReported(joinDiagnostics(errs))
	}
	if err := db.resolveNumbers(rawGame, &db.Game, "game.yaml"); err != nil {
		return nil, err
	}
	db.keepCampaignOrder(rawGame)
	bc := &db.Game.Battle
	for _, field := range []struct {
		name     string
		value    *float64
		fallback float64
	}{{"critical_rate", &bc.CriticalRate, 1.5}, {"variance", &bc.Variance, 0.1}, {"guard_rate", &bc.GuardRate, 0.5}} {
		db.numberDefault(sectionMap(rawGame, "battle"), field.name, field.value, field.fallback)
	}
	var files []string
	err = fs.WalkDir(fsys, "data", func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() && (strings.HasSuffix(p, ".yaml") || strings.HasSuffix(p, ".yml")) {
			files = append(files, p)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	sort.Strings(files)
	var errs []error
	for _, p := range files {
		if err := db.loadFile(p); err != nil {
			errs = append(errs, err)
		}
	}
	if len(errs) > 0 {
		return nil, joinDiagnostics(errs)
	}
	c := &checker{db: db}
	c.requiredDefinitions()
	if len(c.errs) > 0 {
		return nil, joinDiagnostics(c.errs)
	}
	db.defaults()
	if err := db.loadStandIns(); err != nil {
		return nil, err
	}
	if err := db.loadLanguages(); err != nil {
		return nil, err
	}
	if errs := db.Check(); len(errs) > 0 {
		return db, joinDiagnostics(errs)
	}
	db.Show(cmp.Or(db.Game.Language, lang.English))
	return db, nil
}

func (db *DB) loadFile(p string) error {
	b, err := fs.ReadFile(db.FS, p)
	if err != nil {
		return err
	}
	var f file
	doc, err := db.readSource(b, p)
	if err != nil {
		return err
	}
	raw, err := doc.decodeDefinitions(&f)
	if err != nil {
		return err
	}
	if err := db.resolveNumbers(raw, &f, p); err != nil {
		return err
	}
	order := map[string]yaml.MapSlice{}
	for _, it := range raw {
		if m, ok := it.Value.(yaml.MapSlice); ok {
			order[fmt.Sprint(it.Key)] = m
		}
	}
	var errs []error
	merge := func(section string, add func(id string) bool) {
		for _, it := range order[section] {
			id := fmt.Sprint(it.Key)
			if !add(id) {
				errs = append(errs, duplicateDiagnostic(doc.position(section+"."+id), section+"."+id, db.sources[section+"."+id]))
				continue
			}
			db.Order[section] = append(db.Order[section], id)
			db.keepSources(doc, section+"."+id, section+"."+id)
		}
	}
	merge("tactic_conditions", putter(db.TacticConditions, f.TacticConditions))
	merge("tactic_targets", putter(db.TacticTargets, f.TacticTargets))
	merge("tactic_actions", putter(db.TacticActions, f.TacticActions))
	merge("classes", putter(db.Classes, f.Classes))
	merge("actors", putter(db.Actors, f.Actors))
	merge("items", putter(db.Items, f.Items))
	merge("skills", putter(db.Skills, f.Skills))
	merge("states", putter(db.States, f.States))
	merge("enemies", putter(db.Enemies, f.Enemies))
	merge("troops", putter(db.Troops, f.Troops))
	merge("locations", putter(db.Locations, f.Locations))
	for _, it := range order["locations"] {
		if l := f.Locations[fmt.Sprint(it.Key)]; l != nil {
			l.keepOrder(it.Value)
		}
	}
	merge("events", putter(db.Events, f.Events))
	merge("shops", putter(db.Shops, f.Shops))
	merge("ai", putter(db.AI, f.AI))
	merge("elements", putter(db.Elements, f.Elements))
	merge("stone_sets", putter(db.StoneSets, f.StoneSets))
	merge("profiles", putter(db.Profiles, f.Profiles))
	merge("regions", putter(db.Regions, f.Regions))
	merge("factions", putter(db.Factions, f.Factions))
	for i := range f.Fusions {
		db.keepSources(doc, fmt.Sprintf("fusions[%d]", i), fmt.Sprintf("fusions[%d]", len(db.Fusions)+i))
	}
	db.Fusions = append(db.Fusions, f.Fusions...)
	if f.Music != nil {
		if _, exists := db.sources["music"]; exists {
			errs = append(errs, duplicateDiagnostic(doc.position("music"), "music", db.sources["music"]))
		} else {
			db.Music = *f.Music
			db.numberDefault(order["music"], "volume", &db.Music.Volume, 0.7)
			db.keepSources(doc, "music", "music")
			// order the songs were written in (the order of the gallery's music column)
			for _, it := range order["music"] {
				if fmt.Sprint(it.Key) != "songs" {
					continue
				}
				if songs, ok := it.Value.(yaml.MapSlice); ok {
					for _, so := range songs {
						db.Order["songs"] = append(db.Order["songs"], fmt.Sprint(so.Key))
					}
				}
			}
		}
	}
	return joinDiagnostics(errs)
}

// putter adds src[id] to dst, reporting false when dst already has it.
func putter[V any](dst, src map[string]V) func(string) bool {
	return func(id string) bool {
		if _, dup := dst[id]; dup {
			return false
		}
		dst[id] = src[id]
		return true
	}
}

// word is the game's word for the engine's key in its own language and text, without the
// process-wide words (the game is being loaded; the host has not chosen them yet).
func (g *Game) word(key string) string {
	if w, ok := g.Text[key]; ok {
		return w
	}
	if words, err := catalog.Words(g.Language); err == nil {
		if w, ok := words[key]; ok {
			return w
		}
	}
	return key
}

// defaults fills in what was left out.
func (db *DB) defaults() {
	g := &db.Game
	if g.Screen.Width == 0 {
		g.Screen = Screen{1280, 720}
	}
	if g.MaxParty == 0 {
		g.MaxParty = 4
	}
	if len(g.Slots) == 0 {
		g.Slots = []Slot{{ID: "weapon", Name: g.word("武器")}, {ID: "armor", Name: g.word("防具")}, {ID: "accessory", Name: g.word("装飾品")}}
	}
	b := &g.Battle
	if b.FrontWeight == nil {
		b.FrontWeight = new(4.0)
	}
	if b.BackWeight == nil {
		b.BackWeight = new(1.0)
	}
	for _, c := range db.Classes {
		if c.SP == 0 && g.SchemaVersion == LegacySchemaVersion {
			c.SP = 1
		}
		for i := range c.Tree {
			n := &c.Tree[i]
			if n.Max == 0 {
				n.Max = 10
			}
			if n.ID == "" {
				n.ID = n.Skill
			}
			for j := range n.Requires {
				if n.Requires[j].Level == 0 {
					n.Requires[j].Level = 1
				}
			}
		}
	}
	if b.Attack == "" {
		b.Attack = "attack"
	}
	if b.Guard == "" {
		b.Guard = "guard"
	}
	t := &g.Terms
	for _, d := range []struct {
		p   *string
		def string
	}{{&t.Currency, "G"}, {&t.Level, "Lv"}, {&t.HP, "HP"}, {&t.MP, "MP"}, {&t.Exp, "EXP"}} {
		if *d.p == "" {
			*d.p = d.def
		}
	}
	if db.Music.Bars == 0 {
		db.Music.Bars = 8
	}
	if db.Music.Beats == 0 {
		db.Music.Beats = 4
	}
	for _, a := range db.Actors {
		if a.Level == 0 {
			a.Level = 1
		}
		if a.Face == "" {
			a.Face = "normal"
		}
	}
	for _, c := range db.Classes {
		if c.Max == 0 {
			c.Max = 99
		}
	}
	for _, l := range db.Locations {
		if l.Dungeon != nil {
			l.Dungeon.parse()
		}
	}

	for _, e := range db.Enemies {
		if e.Scale == 0 {
			e.Scale = 1
		}
		if e.Actions == 0 && g.SchemaVersion == LegacySchemaVersion {
			e.Actions = 1
		}
		if e.Level == 0 {
			e.Level = 1
		}
	}
	for _, s := range db.Skills {
		if s.Scope == "" {
			s.Scope = "one_enemy"
		}
		if len(s.Where) == 0 {
			s.Where = []string{"battle"}
		}
		if s.Hits == 0 {
			s.Hits = 1
		}
	}
	for _, it := range db.Items {
		if it.Kind == "" {
			it.Kind = "item"
		}
		if u := it.Use; u != nil {
			if u.Scope == "" {
				u.Scope = "one_ally"
			}
			if len(u.Where) == 0 {
				u.Where = []string{"battle", "menu"}
			}
			if u.Hits == 0 {
				u.Hits = 1
			}
		}
	}
}

// Ordered is the ids of a section in the order they were written.
func (db *DB) Ordered(section string) []string { return db.Order[section] }

// FacePicture is the picture of the actor's face, in the outfit when one is given and its
// picture exists (assets/<faces>/<outfit>/<face>.png), else the usual one.
func (db *DB) FacePicture(a *Actor, face, outfit string) string {
	if outfit != "" {
		if p := a.Faces + "/" + outfit + "/" + face + ".png"; db.Exists("assets/" + p) {
			return p
		}
	}
	return a.Faces + "/" + face + ".png"
}

// Exists reports whether the game has the file (a path under the game's directory).
func (db *DB) Exists(p string) bool {
	_, err := fs.Stat(db.FS, path.Clean(p))
	return err == nil
}

// Rules are a battler's AI rules: inline, or the named set.
func (db *DB) Rules(ref AIRef) []Rule {
	if ref.Name != "" {
		return db.AI[ref.Name]
	}
	return ref.Rules
}
