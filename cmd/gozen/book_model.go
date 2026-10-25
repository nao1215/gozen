package main

import (
	"maps"
	"slices"
	"strings"

	"github.com/nao1215/gozen/internal/layer2/gamedata"
)

// bookReport is a detached synopsis, shared by Markdown and structured output.
// It contains no filesystem, parsed expression or runtime state.
type bookReport struct {
	Game            bookGame                  `json:"game"`
	Locations       map[string]*bookLocation  `json:"locations"`
	Enemies         map[string]*bookMonster   `json:"monsters"`
	MonsterFamilies map[string]string         `json:"monster_families"`
	Breeding        *bookBreeding             `json:"breeding,omitempty"`
	Troops          map[string]*bookTroop     `json:"troops"`
	AI              map[string][]bookRule     `json:"ai"`
	Skills          map[string]*bookSkill     `json:"skills"`
	Items           map[string]*bookItem      `json:"items"`
	Classes         map[string]*bookClass     `json:"classes"`
	Events          map[string]*bookEvent     `json:"events"`
	Music           bookMusic                 `json:"music"`
	Flow            []bookFlow                `json:"flow"`
	Lineage         map[string]lineageMonster `json:"lineage"`
	order           map[string][]string       `json:"-"`
}
type bookGame struct {
	ID       string      `json:"id"`
	Title    string      `json:"title"`
	Subtitle string      `json:"subtitle"`
	Start    bookStart   `json:"start"`
	Gallery  bookGallery `json:"gallery"`
}
type bookStart struct {
	Location string         `json:"location"`
	Party    []string       `json:"party"`
	Gold     int            `json:"gold"`
	Items    map[string]int `json:"items"`
}
type bookGallery struct {
	CGs    []bookCG    `json:"cgs"`
	Scenes []bookScene `json:"scenes"`
}
type bookCG struct {
	Title string `json:"title"`
	File  string `json:"file"`
}
type bookScene struct {
	Title string `json:"title"`
	Event string `json:"event"`
	Flag  string `json:"flag"`
}
type bookAction struct {
	ID     string        `json:"id,omitempty"`
	Name   string        `json:"name,omitempty"`
	Label  string        `json:"label,omitempty"`
	To     string        `json:"to,omitempty"`
	Action string        `json:"action,omitempty"`
	When   bookExpr      `json:"when,omitempty"`
	Do     []bookCommand `json:"commands"`
	Talk   []bookCommand `json:"talk,omitempty"`
}
type bookLocation struct {
	Name       string        `json:"name"`
	Desc       string        `json:"description"`
	Music      bookCue       `json:"music"`
	OnEnter    []bookCommand `json:"on_enter"`
	People     []bookAction  `json:"people"`
	Actions    []bookAction  `json:"actions"`
	Exits      []bookAction  `json:"exits"`
	Map        *bookMap      `json:"map,omitempty"`
	Stage      *bookStage    `json:"stage,omitempty"`
	Dungeon    *bookDungeon  `json:"dungeon,omitempty"`
	Encounters bookEncounter `json:"encounters"`
}
type bookCue struct {
	Song string `json:"song"`
	Mood string `json:"mood"`
}
type bookMap struct {
	Points map[string]bookAction `json:"points"`
	Roads  []bookRoad            `json:"roads"`
}
type bookRoad struct {
	Between []string      `json:"between"`
	Steps   int           `json:"steps"`
	When    bookExpr      `json:"when,omitempty"`
	Do      []bookCommand `json:"commands"`
}
type bookStage struct {
	Spots  map[string]bookAction `json:"spots"`
	Things []bookAction          `json:"things"`
}
type bookDungeon struct {
	Generate *bookGenerate          `json:"generate,omitempty"`
	Roamers  []bookRoamer           `json:"roamers"`
	Tiles    map[string]*bookAction `json:"tiles"`
	Looks    []bookLook             `json:"looks"`
}
type bookGenerate struct {
	Width  int             `json:"width"`
	Height int             `json:"height"`
	Rooms  int             `json:"rooms"`
	Floors []int           `json:"floors"`
	Place  []bookPlacement `json:"place"`
}
type bookPlacement struct {
	Char  string   `json:"tile"`
	Count bookExpr `json:"count"`
}
type bookRoamer struct {
	Name   string               `json:"name"`
	Move   string               `json:"move"`
	Appear bookExpr             `json:"appear"`
	Every  bookExpr             `json:"every"`
	Count  bookExpr             `json:"count"`
	Meet   []bookCommand        `json:"meet"`
	Troops []bookEncounterTroop `json:"troops"`
}
type bookLook struct {
	When     bookExpr    `json:"when"`
	Wall     bookPicture `json:"wall"`
	BattleBG bookPicture `json:"battle_background"`
}
type bookPicture struct {
	File string `json:"file"`
}
type bookEncounter struct {
	Rate   bookExpr             `json:"rate"`
	Gauge  float64              `json:"gauge"`
	Troops []bookEncounterTroop `json:"troops"`
}
type bookEncounterTroop struct {
	Troop  string   `json:"troop"`
	Weight float64  `json:"weight"`
	When   bookExpr `json:"when"`
}
type bookMonster struct {
	Name    string             `json:"name"`
	Stats   gamedata.Stats     `json:"stats"`
	Exp     int                `json:"experience"`
	Gold    int                `json:"gold"`
	Resist  map[string]float64 `json:"resist"`
	SResist map[string]float64 `json:"state_resist"`
	Boss    bool               `json:"boss"`
	Lore    string             `json:"lore"`
	Image   bookPicture        `json:"image"`
	Family  string             `json:"family"`
	Rank    int                `json:"rank"`
	Sex     string             `json:"sex"`
	Recruit *bookRecruit       `json:"recruit,omitempty"`
	AI      bookAI             `json:"ai"`
	Drops   []bookDrop         `json:"drops"`
}
type bookRecruit struct {
	Chance     float64 `json:"chance"`
	Class      string  `json:"class"`
	Level      int     `json:"level"`
	BonusItem  string  `json:"bonus_item"`
	BonusSkill string  `json:"bonus_skill"`
	Bonus      float64 `json:"bonus"`
}
type bookAI struct {
	Rules []bookRule `json:"rules"`
}
type bookRule struct {
	Use  string   `json:"use"`
	Item string   `json:"item"`
	When bookExpr `json:"when"`
}
type bookDrop struct {
	Item   string  `json:"item"`
	Chance float64 `json:"chance"`
}
type bookBreeding struct {
	MinLevel   int          `json:"min_level"`
	SkillSlots int          `json:"skill_slots"`
	Pairs      []bookPair   `json:"pairs"`
	Families   []bookFamily `json:"families"`
}
type bookPair struct {
	Parents  []string `json:"parents"`
	Child    string   `json:"child"`
	MinLevel int      `json:"min_level"`
	MinPlus  int      `json:"min_plus"`
}
type bookFamily struct {
	Parents  []string `json:"parents"`
	Children []string `json:"children"`
}
type bookTroop struct {
	Members   []string    `json:"members"`
	CanEscape bool        `json:"escapable"`
	Events    []bookEvent `json:"events"`
}
type bookSkill struct {
	Name  string `json:"name"`
	MP    int    `json:"mp"`
	Scope string `json:"scope"`
	Cutin bool   `json:"cutin"`
	Desc  string `json:"description"`
}
type bookItem struct {
	Name  string         `json:"name"`
	Kind  string         `json:"kind"`
	Price int            `json:"price"`
	Stats gamedata.Stats `json:"stats"`
	Desc  string         `json:"description"`
}
type bookClass struct {
	Name string         `json:"name"`
	Tree []bookTreeNode `json:"tree"`
}
type bookTreeNode struct {
	ID       string         `json:"id"`
	Name     string         `json:"name"`
	Skill    string         `json:"skill"`
	Requires []bookRequired `json:"requires"`
}
type bookRequired struct {
	Node  string `json:"node"`
	Level int    `json:"level"`
}
type bookEvent struct {
	Trigger  string        `json:"trigger"`
	Location string        `json:"location"`
	When     bookExpr      `json:"when"`
	Do       []bookCommand `json:"commands"`
}
type bookMusic struct {
	Songs map[string]bookSong        `json:"songs"`
	Rules map[string][]bookMusicRule `json:"rules"`
}
type bookSong struct {
	Name  string          `json:"name"`
	Moods map[string]bool `json:"moods"`
}
type bookMusicRule struct {
	When bookExpr `json:"when"`
	Play string   `json:"play"`
	Mood string   `json:"mood"`
	Now  bool     `json:"now"`
}
type bookFlow struct {
	ID    string   `json:"event"`
	Sets  []string `json:"sets"`
	Reads []string `json:"reads"`
	Calls []string `json:"calls"`
	Moves []string `json:"moves"`
}
type bookCommand struct {
	Respond  *bookResponse       `json:"respond,omitempty"`
	Kind     string              `json:"verb"`
	Say      *bookSay            `json:"say,omitempty"`
	Prompt   string              `json:"prompt,omitempty"`
	Choice   []bookChoice        `json:"choices,omitempty"`
	If       bookExpr            `json:"if,omitempty"`
	Then     []bookCommand       `json:"then,omitempty"`
	Else     []bookCommand       `json:"else,omitempty"`
	Battle   string              `json:"battle,omitempty"`
	Fighters []string            `json:"fighters,omitempty"`
	OnWin    []bookCommand       `json:"on_win,omitempty"`
	OnLose   []bookCommand       `json:"on_lose,omitempty"`
	OnEscape []bookCommand       `json:"on_escape,omitempty"`
	SetV     map[string]bookExpr `json:"set,omitempty"`
	Call     string              `json:"call,omitempty"`
	Give     bookGive            `json:"give,omitzero"`
	Image    bookPicture         `json:"image,omitzero"`
	Move     string              `json:"move,omitempty"`
	Dive     string              `json:"dive,omitempty"`
	Floor    bookExpr            `json:"floor,omitempty"`
}
type bookResponse struct {
	Who  string `json:"actor"`
	Text string `json:"text"`
	From string `json:"from,omitempty"`
}

type bookSay struct {
	Who  string `json:"actor"`
	Name string `json:"name"`
	Text string `json:"text"`
}
type bookChoice struct {
	Label string        `json:"label"`
	When  bookExpr      `json:"when"`
	Do    []bookCommand `json:"commands"`
}
type bookGive struct {
	Item  string `json:"item"`
	Count int    `json:"count"`
}

type bookExpr string

func (e bookExpr) String() string { return string(e) }
func (e bookExpr) Set() bool      { return e != "" }
func bookExpression(e gamedata.Expr) bookExpr {
	if !e.Set() {
		return ""
	}
	return bookExpr(e.String())
}
func (b *bookReport) Ordered(kind string) []string { return b.order[kind] }
func (m *bookMap) PointIDs() []string              { return slices.Sorted(maps.Keys(m.Points)) }
func (s *bookStage) SpotIDs() []string             { return slices.Sorted(maps.Keys(s.Spots)) }
func (r bookRoad) StepsOrOne() int                 { return r.Steps }
func (t *bookTroop) Escapable() bool               { return t.CanEscape }
func (g bookGive) N() int                          { return g.Count }

func bookCommands(db *gamedata.DB, cs []gamedata.Command) []bookCommand {
	out := []bookCommand{}
	for _, c := range cs {
		kind, err := c.Verb()
		if err != nil {
			kind = "invalid"
		}
		v := bookCommand{Kind: kind, Prompt: c.Prompt, If: bookExpression(c.If), Then: bookCommands(db, c.Then), Else: bookCommands(db, c.Else), Battle: c.Battle, Fighters: slices.Clone(c.Fighters), OnWin: bookCommands(db, c.OnWin), OnLose: bookCommands(db, c.OnLose), OnEscape: bookCommands(db, c.OnRun), Call: c.Call, Move: c.Move, Dive: c.Dive, Floor: bookExpression(c.Floor)}
		if c.Respond != nil {
			v.Respond = &bookResponse{c.Respond.Who, c.Respond.Text, c.Respond.From}
		}
		if c.Image != nil {
			v.Image = bookPicture{c.Image.File}
		}
		if c.Say != nil {
			name := c.Say.Who
			if a := db.Actors[name]; a != nil {
				name = a.Name
			}
			v.Say = &bookSay{c.Say.Who, name, c.Say.Text}
		}
		if c.Give != nil {
			v.Give = bookGive{c.Give.Item, c.Give.N()}
		}
		for _, o := range c.Choice {
			v.Choice = append(v.Choice, bookChoice{o.Label, bookExpression(o.When), bookCommands(db, o.Do)})
		}
		if len(c.SetV) > 0 {
			v.SetV = map[string]bookExpr{}
			for k, e := range c.SetV {
				v.SetV[k] = bookExpression(e)
			}
		}
		out = append(out, v)
	}
	return out
}

func buildBook(db *gamedata.DB) *bookReport {
	b := &bookReport{Locations: map[string]*bookLocation{}, Enemies: map[string]*bookMonster{}, MonsterFamilies: bookMapCopy(db.MonsterFamilies), Troops: map[string]*bookTroop{}, AI: map[string][]bookRule{}, Skills: map[string]*bookSkill{}, Items: map[string]*bookItem{}, Classes: map[string]*bookClass{}, Events: map[string]*bookEvent{}, Music: bookMusic{Songs: map[string]bookSong{}, Rules: map[string][]bookMusicRule{}}, Flow: []bookFlow{}, Lineage: map[string]lineageMonster{}, order: map[string][]string{}}
	for _, kind := range []string{"locations", "enemies", "troops", "ai", "skills", "items", "classes", "events"} {
		b.order[kind] = slices.Clone(db.Ordered(kind))
	}
	g := db.Game
	b.Game = bookGame{ID: g.ID, Title: g.Title, Subtitle: g.Subtitle, Start: bookStart{g.Start.Location, append([]string{}, g.Start.Party...), g.Start.Gold, bookMapCopy(g.Start.Items)}, Gallery: bookGallery{CGs: []bookCG{}, Scenes: []bookScene{}}}
	for _, v := range g.Gallery.CGs {
		b.Game.Gallery.CGs = append(b.Game.Gallery.CGs, bookCG{v.Title, v.File})
	}
	for _, v := range g.Gallery.Scenes {
		b.Game.Gallery.Scenes = append(b.Game.Gallery.Scenes, bookScene{v.Title, v.Event, v.Flag})
	}
	for _, id := range b.Ordered("locations") {
		l := db.Locations[id]
		if l == nil {
			continue
		}
		v := &bookLocation{Name: l.Name, Desc: l.Desc, Music: bookCue{l.Music.Song, l.Music.Mood}, OnEnter: bookCommands(db, l.OnEnter), People: []bookAction{}, Actions: []bookAction{}, Exits: []bookAction{}, Encounters: bookEncounter{Rate: bookExpression(l.Encounters.Rate), Gauge: l.Encounters.Gauge, Troops: []bookEncounterTroop{}}}
		for _, x := range l.People {
			v.People = append(v.People, bookAction{Name: x.Name, When: bookExpression(x.When), Talk: bookCommands(db, x.Talk), Do: []bookCommand{}})
		}
		for _, x := range l.Actions {
			v.Actions = append(v.Actions, bookAction{Label: x.Label, When: bookExpression(x.When), Do: bookCommands(db, x.Do)})
		}
		for _, x := range l.Exits {
			v.Exits = append(v.Exits, bookAction{Label: x.Label, To: x.To, When: bookExpression(x.When), Do: bookCommands(db, x.Do)})
		}
		if m := l.Map; m != nil {
			v.Map = &bookMap{Points: map[string]bookAction{}, Roads: []bookRoad{}}
			for _, id := range m.PointIDs() {
				x := m.Points[id]
				v.Map.Points[id] = bookAction{ID: id, To: x.To, When: bookExpression(x.When), Do: bookCommands(db, x.Do)}
			}
			for _, x := range m.Roads {
				v.Map.Roads = append(v.Map.Roads, bookRoad{append([]string{}, x.Between...), x.StepsOrOne(), bookExpression(x.When), bookCommands(db, x.Do)})
			}
		}
		if st := l.Stage; st != nil {
			v.Stage = &bookStage{Spots: map[string]bookAction{}, Things: []bookAction{}}
			for _, id := range st.SpotIDs() {
				v.Stage.Spots[id] = bookAction{ID: id, Do: bookCommands(db, st.Spots[id].Do)}
			}
			for _, x := range st.Things {
				v.Stage.Things = append(v.Stage.Things, bookAction{Name: x.Name, Action: x.Action, When: bookExpression(x.When), Do: bookCommands(db, x.Do)})
			}
		}
		if d := l.Dungeon; d != nil {
			v.Dungeon = &bookDungeon{Roamers: []bookRoamer{}, Tiles: map[string]*bookAction{}, Looks: []bookLook{}}
			if gen := d.Generate; gen != nil {
				v.Dungeon.Generate = &bookGenerate{gen.Width, gen.Height, gen.Rooms, slices.Sorted(maps.Keys(gen.Floors)), []bookPlacement{}}
				for _, x := range gen.Place {
					v.Dungeon.Generate.Place = append(v.Dungeon.Generate.Place, bookPlacement{x.Char, bookExpression(x.Count)})
				}
			}
			for _, x := range d.Roamers {
				r := bookRoamer{Name: x.Name, Move: x.Move, Appear: bookExpression(x.Appear), Every: bookExpression(x.Every), Count: bookExpression(x.Count), Meet: bookCommands(db, x.Meet), Troops: []bookEncounterTroop{}}
				for _, t := range x.Troops {
					r.Troops = append(r.Troops, bookEncounterTroop{t.Troop, t.Weight, bookExpression(t.When)})
				}
				v.Dungeon.Roamers = append(v.Dungeon.Roamers, r)
			}
			for id, x := range d.Tiles {
				if x != nil {
					v.Dungeon.Tiles[id] = &bookAction{ID: id, Name: x.Name, When: bookExpression(x.When), Do: bookCommands(db, x.Do)}
				}
			}
			for _, x := range d.Looks {
				v.Dungeon.Looks = append(v.Dungeon.Looks, bookLook{bookExpression(x.When), bookPicture{x.Wall.File}, bookPicture{x.BattleBG.File}})
			}
		}
		for _, x := range l.Encounters.Troops {
			v.Encounters.Troops = append(v.Encounters.Troops, bookEncounterTroop{x.Troop, x.Weight, bookExpression(x.When)})
		}
		b.Locations[id] = v
	}
	lin := gamedata.BuildLineage(db)
	for _, id := range b.Ordered("enemies") {
		e := db.Enemies[id]
		if e == nil {
			continue
		}
		v := &bookMonster{Name: e.Name, Stats: bookMapCopy(e.Stats), Exp: e.Exp, Gold: e.Gold, Resist: bookMapCopy(e.Resist), SResist: bookMapCopy(e.SResist), Boss: e.Boss, Lore: e.Lore, Image: bookPicture{e.Image.File}, Family: e.Family, Rank: e.Rank, Sex: e.Sex, AI: bookAI{Rules: []bookRule{}}, Drops: []bookDrop{}}
		if r := e.Recruit; r != nil {
			v.Recruit = &bookRecruit{r.Chance, db.MonsterClass(id), max(1, r.Level), r.BonusItem, r.BonusSkill, r.Bonus}
		}
		for _, r := range e.AI.Rules {
			v.AI.Rules = append(v.AI.Rules, bookRule{r.Use, r.Item, bookExpression(r.When)})
		}
		for _, d := range e.Drops {
			v.Drops = append(v.Drops, bookDrop{d.Item, d.Chance})
		}
		b.Enemies[id] = v
		b.Lineage[id] = lineageMonster{ID: id, Name: e.Name, Family: e.Family, Rank: e.Rank, Sex: e.Sex, Recipes: append([]gamedata.Recipe{}, lin.Recipes(id)...), Lineage: lin.Tree(id, nil)}
	}
	if x := db.Breeding; x != nil {
		b.Breeding = &bookBreeding{MinLevel: x.MinLevel, SkillSlots: x.SkillSlots, Pairs: []bookPair{}, Families: []bookFamily{}}
		for _, r := range x.Pairs {
			b.Breeding.Pairs = append(b.Breeding.Pairs, bookPair{append([]string{}, r.Parents...), r.Child, r.MinLevel, r.MinPlus})
		}
		for _, r := range x.Families {
			b.Breeding.Families = append(b.Breeding.Families, bookFamily{append([]string{}, r.Parents...), append([]string{}, r.Children...)})
		}
	}
	for _, id := range b.Ordered("troops") {
		x := db.Troops[id]
		if x == nil {
			continue
		}
		v := &bookTroop{append([]string{}, x.Members...), x.Escapable(), []bookEvent{}}
		for _, e := range x.Events {
			v.Events = append(v.Events, bookEvent{When: bookExpression(e.When), Do: bookCommands(db, e.Do)})
		}
		b.Troops[id] = v
	}
	for _, id := range b.Ordered("ai") {
		b.AI[id] = []bookRule{}
		for _, r := range db.AI[id] {
			b.AI[id] = append(b.AI[id], bookRule{r.Use, r.Item, bookExpression(r.When)})
		}
	}
	for _, id := range b.Ordered("skills") {
		x := db.Skills[id]
		if x != nil {
			b.Skills[id] = &bookSkill{x.Name, x.MP, x.Scope, x.Cutin, x.Desc}
		}
	}
	for _, id := range b.Ordered("items") {
		x := db.Items[id]
		if x != nil {
			b.Items[id] = &bookItem{x.Name, x.Kind, x.Price, bookMapCopy(x.Stats), x.Desc}
		}
	}
	for _, id := range b.Ordered("classes") {
		x := db.Classes[id]
		if x == nil {
			continue
		}
		v := &bookClass{x.Name, []bookTreeNode{}}
		for _, n := range x.Tree {
			r := bookTreeNode{ID: n.ID, Name: n.Name, Skill: n.Skill, Requires: []bookRequired{}}
			for _, p := range n.Requires {
				r.Requires = append(r.Requires, bookRequired{p.Node, p.Level})
			}
			v.Tree = append(v.Tree, r)
		}
		b.Classes[id] = v
	}
	uses := db.StateUses()
	for _, id := range b.Ordered("events") {
		e := db.Events[id]
		if e == nil {
			continue
		}
		b.Events[id] = &bookEvent{e.Trigger, e.Location, bookExpression(e.When), bookCommands(db, e.Do)}
		f := bookFlow{ID: id, Sets: []string{}, Reads: []string{}, Calls: []string{}, Moves: []string{}}
		for _, name := range slices.Sorted(maps.Keys(uses)) {
			if strings.HasPrefix(name, "ran.") {
				continue
			}
			in := func(paths []string) bool {
				return slices.ContainsFunc(paths, func(p string) bool { return strings.HasPrefix(p, "events."+id+".") })
			}
			if in(uses[name].Sets) {
				f.Sets = append(f.Sets, name)
			}
			if in(uses[name].Reads) {
				f.Reads = append(f.Reads, name)
			}
		}
		calls, moves := eventLinks(e.Do)
		f.Calls = append(f.Calls, calls...)
		f.Moves = append(f.Moves, moves...)
		b.Flow = append(b.Flow, f)
	}
	for id, x := range db.Music.Songs {
		v := bookSong{x.Name, map[string]bool{}}
		for m := range x.Moods {
			v.Moods[m] = true
		}
		b.Music.Songs[id] = v
	}
	for id, rs := range db.Music.Rules {
		b.Music.Rules[id] = []bookMusicRule{}
		for _, r := range rs {
			b.Music.Rules[id] = append(b.Music.Rules[id], bookMusicRule{bookExpression(r.When), r.Play, r.Mood, r.Now})
		}
	}
	return b
}

func bookMapCopy[M ~map[K]V, K comparable, V any](source M) M {
	out := make(M, len(source))
	maps.Copy(out, source)
	return out
}

type bookSection struct {
	id   string
	data any
}

func (b *bookReport) sections() []bookSection {
	return []bookSection{{"game", b.Game}, {"locations", b.Locations}, {"monsters", b.Enemies}, {"monster_families", b.MonsterFamilies}, {"breeding", b.Breeding}, {"troops", b.Troops}, {"ai", b.AI}, {"skills", b.Skills}, {"items", b.Items}, {"classes", b.Classes}, {"events", b.Events}, {"music", b.Music}, {"flow", b.Flow}, {"lineage", b.Lineage}}
}

func (b *bookReport) lineageSection() any {
	rows := []lineageMonster{}
	for _, id := range b.Ordered("enemies") {
		if row, ok := b.Lineage[id]; ok {
			rows = append(rows, row)
		}
	}
	return struct {
		Monsters []lineageMonster `json:"monsters"`
	}{rows}
}
