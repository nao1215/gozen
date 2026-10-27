package gamedata

import (
	"bytes"
	"cmp"
	"fmt"
	"io/fs"
	"maps"
	"slices"
	"strings"

	"github.com/goccy/go-yaml"

	"github.com/nao1215/gozen/internal/diag"
	"github.com/nao1215/gozen/internal/layer1/decodeguard"
	lang "github.com/nao1215/gozen/internal/text"
	"github.com/nao1215/gozen/internal/text/catalog"
)

// The languages of a game's text (ADR 0027): the files under data/ are written in one
// language (written_in), and lang/<code>.yaml maps each source text to its translation
// for another (gettext). The names of the definitions are translated in place when a
// language is shown (Show); the lines, choices and notices are translated as they are
// shown (Tr), so the key of a read line stays its source.

// LangDir holds the translations, lang/<code>.yaml.
const LangDir = "lang"

// langFile is lang/<code>.yaml.
type langFile struct {
	Text map[string]string `yaml:"text"`
}

// Written is the language the files under data/ are written in (written_in; language
// when it is left out).
func (g *Game) Written() string { return cmp.Or(g.WrittenIn, g.Language, lang.English) }

// Offered are the languages the player may choose (languages; language when left out).
func (g *Game) Offered() []string {
	if len(g.Languages) > 0 {
		return g.Languages
	}
	return []string{cmp.Or(g.Language, lang.English)}
}

// loadLanguages reads lang/<code>.yaml of each language offered other than the one written.
// A language without its file has every text untranslated (check warns).
func (db *DB) loadLanguages() error {
	db.Words = map[string]map[string]string{}
	var errs []error
	for _, code := range append(slices.Clone(db.Game.Offered()), db.Game.Language) {
		if code == "" || code == db.Game.Written() || db.Words[code] != nil {
			continue
		}
		name := LangDir + "/" + code + ".yaml"
		b, err := fs.ReadFile(db.FS, name)
		if err != nil {
			db.Words[code] = map[string]string{}
			continue
		}
		var f langFile
		if err := decodeguard.Run(func() error {
			return yaml.NewDecoder(bytes.NewReader(b), yaml.Strict()).Decode(&f)
		}); err != nil {
			errs = append(errs, fmt.Errorf("%s: %s", name, yaml.FormatError(err, false, true)))
			continue
		}
		db.Words[code] = f.Text
		if f.Text == nil {
			db.Words[code] = map[string]string{}
		}
	}
	return joinDiagnostics(errs)
}

// Tr is the translation of a source text in the language shown (the source itself when
// it is the language written, or when the text has no translation yet).
func (db *DB) Tr(s string) string { return db.TrIn(db.shown, s) }

// TrIn is the translation of a source text in a language (the source when there is none).
func (db *DB) TrIn(code, s string) string {
	if s == "" || code == "" || code == db.Game.Written() {
		return s
	}
	if t, ok := db.Words[code][s]; ok && t != "" {
		return t
	}
	return s
}

// Shown is the language the game's text is shown in.
func (db *DB) Shown() string { return cmp.Or(db.shown, db.Game.Written()) }

// Show shows the game's text in a language: the names of the definitions are translated
// in place (from their sources), and Tr translates the lines into it.
func (db *DB) Show(code string) {
	if db.names == nil {
		db.names = map[*string]string{}
		for _, p := range db.nameFields() {
			db.names[p] = *p
		}
	}
	db.shown = code
	for p, src := range db.names {
		*p = db.Tr(src)
	}
}

// EngineWords are the words game.yaml's text and lang/<code>.yaml give the engine's own
// keys in a language (ADR 0019: one key space), for text.Use.
func (db *DB) EngineWords(code string) map[string]string {
	out := map[string]string{}
	if code == db.Game.Language {
		maps.Copy(out, db.Game.Text)
	}
	maps.Copy(out, db.Words[code])
	return out
}

// nameFields are the names and descriptions of the definitions shown as text.
func (db *DB) nameFields() []*string {
	g := &db.Game
	out := []*string{&g.Title, &g.Subtitle}
	for i := range g.Gallery.Scenes {
		out = append(out, &g.Gallery.Scenes[i].Title)
	}
	for _, c := range db.Classes {
		out = append(out, &c.Name)
	}
	for _, a := range db.Actors {
		out = append(out, &a.Name, &a.Profile)
	}
	for _, it := range db.Items {
		out = append(out, &it.Name, &it.Desc)
	}
	for _, s := range db.Skills {
		out = append(out, &s.Name, &s.Desc)
	}
	for _, s := range db.States {
		out = append(out, &s.Name)
	}
	for _, e := range db.Enemies {
		out = append(out, &e.Name, &e.Lore)
	}
	for _, s := range db.Shops {
		out = append(out, &s.Name, &s.Greeting)
	}
	for _, p := range db.Profiles {
		out = append(out, p.texts()...)
	}
	for _, r := range db.Regions {
		out = append(out, &r.Name)
	}
	for _, f := range db.Factions {
		out = append(out, &f.Name)
	}
	if c := g.Campaign; c != nil {
		for _, t := range c.Clash.Tactics {
			if t != nil {
				out = append(out, &t.Label)
			}
		}
		for _, sk := range c.Clash.Skills {
			if sk != nil {
				out = append(out, &sk.Label)
			}
		}
	}
	for _, l := range db.Locations {
		out = append(out, &l.Name, &l.Desc)
		for i := range l.People {
			out = append(out, &l.People[i].Name)
		}
		for i := range l.Actions {
			out = append(out, &l.Actions[i].Label)
		}
		for i := range l.Exits {
			out = append(out, &l.Exits[i].Label)
		}
	}
	return out
}

// SourceText is a text of the game shown on the screens, and where it is written.
type SourceText struct {
	Path, Text string
}

// SourceTexts are the texts of the game a translation covers: the names and descriptions
// of the definitions, and the lines, speakers, choices, prompts and notices of the
// commands of events and places, in a stable order.
func (db *DB) SourceTexts() []SourceText {
	var out []SourceText
	add := func(p, s string) {
		if strings.TrimSpace(s) != "" {
			out = append(out, SourceText{p, s})
		}
	}
	src := func(p *string) string {
		if s, ok := db.names[p]; ok {
			return s
		}
		return *p
	}
	g := &db.Game
	add("game.yaml title", src(&g.Title))
	add("game.yaml subtitle", src(&g.Subtitle))
	for _, name := range slices.Sorted(maps.Keys(g.Screens)) {
		screen := g.Screens[name]
		if screen == nil {
			continue
		}
		add("game.yaml screens."+name+".title", screen.Title)
		for i, entry := range screen.Entries {
			w := fmt.Sprintf("game.yaml screens.%s.entries[%d]", name, i)
			add(w+".label", entry.Label)
			add(w+".text", entry.Text)
		}
	}
	for i := range g.Gallery.Scenes {
		add(fmt.Sprintf("game.yaml gallery.scenes[%d].title", i), src(&g.Gallery.Scenes[i].Title))
	}
	sorted := func(m []string) []string { return slices.Sorted(slices.Values(m)) }
	for _, id := range sorted(slices.Collect(maps.Keys(db.Classes))) {
		add("classes."+id+".name", src(&db.Classes[id].Name))
	}
	for _, id := range sorted(slices.Collect(maps.Keys(db.Actors))) {
		a := db.Actors[id]
		add("actors."+id+".name", src(&a.Name))
		add("actors."+id+".profile", src(&a.Profile))
	}
	for _, id := range sorted(slices.Collect(maps.Keys(db.Items))) {
		it := db.Items[id]
		add("items."+id+".name", src(&it.Name))
		add("items."+id+".desc", src(&it.Desc))
	}
	for _, id := range sorted(slices.Collect(maps.Keys(db.Skills))) {
		s := db.Skills[id]
		add("skills."+id+".name", src(&s.Name))
		add("skills."+id+".desc", src(&s.Desc))
	}
	for _, id := range sorted(slices.Collect(maps.Keys(db.States))) {
		add("states."+id+".name", src(&db.States[id].Name))
	}
	for _, id := range sorted(slices.Collect(maps.Keys(db.Enemies))) {
		add("enemies."+id+".name", src(&db.Enemies[id].Name))
		add("enemies."+id+".lore", src(&db.Enemies[id].Lore))
	}
	for _, id := range sorted(slices.Collect(maps.Keys(db.Shops))) {
		s := db.Shops[id]
		add("shops."+id+".name", src(&s.Name))
		add("shops."+id+".greeting", src(&s.Greeting))
	}
	for _, id := range sorted(slices.Collect(maps.Keys(db.Profiles))) {
		p := db.Profiles[id]
		add("profiles."+id+".name", src(&p.Name))
		add("profiles."+id+".title", src(&p.Title))
		add("profiles."+id+".text", src(&p.Text))
		for i := range p.Fields {
			add(fmt.Sprintf("profiles.%s.fields[%d].label", id, i), src(&p.Fields[i].Label))
			add(fmt.Sprintf("profiles.%s.fields[%d].value", id, i), src(&p.Fields[i].Value))
		}
		for i := range p.Relations {
			add(fmt.Sprintf("profiles.%s.relations[%d].calls", id, i), src(&p.Relations[i].Calls))
			add(fmt.Sprintf("profiles.%s.relations[%d].text", id, i), src(&p.Relations[i].Text))
		}
	}
	for _, id := range RegionIDs(db) {
		add("regions."+id+".name", src(&db.Regions[id].Name))
	}
	for _, id := range FactionIDs(db) {
		add("factions."+id+".name", src(&db.Factions[id].Name))
	}
	if st := db.Streams; st != nil {
		all := func(w string, lines []string) {
			for i, l := range lines {
				add(fmt.Sprintf("%s[%d]", w, i), l)
			}
		}
		all(st.file+" crowd.names", st.Crowd.Names)
		all(st.file+" crowd.lines.any", st.Crowd.Lines.Any)
		all(st.file+" crowd.lines.morning", st.Crowd.Lines.Morning)
		all(st.file+" crowd.lines.evening", st.Crowd.Lines.Evening)
		all(st.file+" crowd.tip_lines", st.Crowd.TipLines)
		var episodes []*Episode
		for i, ch := range st.Channels {
			w := fmt.Sprintf("%s channels[%d]", st.file, i)
			add(w+".tag", ch.Tag)
			add(w+".tagline", ch.Tagline)
			for j, g := range ch.Greet {
				all(fmt.Sprintf("%s.greet[%d]", w, j), g)
			}
			all(w+".first", ch.First)
			all(w+".streak", ch.Streak)
			all(w+".stayed", ch.Stayed)
			all(w+".bridges", ch.Bridges)
			episodes = append(episodes, ch.Shows...)
		}
		for _, d := range st.Duos {
			episodes = append(episodes, d.Shows...)
		}
		for _, e := range episodes {
			add(e.file+" title", e.Title)
			all(e.file+" lobby", e.Lobby)
			for i, part := range e.Parts {
				for j, b := range part {
					w := fmt.Sprintf("%s parts[%d][%d]", e.file, i, j)
					switch {
					case b.Line != nil:
						add(w, b.Line.Text)
					case b.Chat != nil:
						all(w+".chat", b.Chat)
					case b.Ask != nil:
						add(w+".ask.q", b.Ask.Q)
						for k, n := range b.Ask.NPC {
							add(fmt.Sprintf("%s.ask.npc[%d]", w, k), n.Name)
						}
						for k, o := range b.Ask.Options {
							add(fmt.Sprintf("%s.ask.options[%d].c", w, k), o.C)
							all(fmt.Sprintf("%s.ask.options[%d].r", w, k), replyTexts(o.R))
						}
					}
				}
			}
		}
	}
	for _, id := range slices.Sorted(maps.Keys(db.ReactionBooks)) {
		for i, l := range db.ReactionBooks[id].Lines(id) {
			add(fmt.Sprintf("reactions.%s[%d]", id, i), l)
		}
	}
	var cmds func(w string, cs []Command)
	cmds = func(w string, cs []Command) {
		for i := range cs {
			c := &cs[i]
			cw := fmt.Sprintf("%s[%d]", w, i)
			if c.Respond != nil {
				add(cw+".respond.text", c.Respond.Text)
				add(cw+".respond.from", c.Respond.From)
			}
			if c.Say != nil {
				if _, actor := db.Actors[c.Say.Who]; !actor {
					add(cw+".say.who", c.Say.Who)
				}
				add(cw+".say", c.Say.Text)
				for j, alt := range c.Say.Instead {
					add(fmt.Sprintf("%s.say.instead[%d]", cw, j), alt.Text)
				}
			}
			add(cw+".prompt", c.Prompt)
			add(cw+".notice", c.Notice)
			for j, o := range c.Choice {
				add(fmt.Sprintf("%s.choice[%d].label", cw, j), o.Label)
				cmds(fmt.Sprintf("%s.choice[%d].do", cw, j), o.Do)
			}
			cmds(cw+".then", c.Then)
			cmds(cw+".else", c.Else)
			cmds(cw+".on_win", c.OnWin)
			cmds(cw+".on_lose", c.OnLose)
			cmds(cw+".on_escape", c.OnRun)
		}
	}
	for _, id := range sorted(slices.Collect(maps.Keys(db.Locations))) {
		l := db.Locations[id]
		w := "locations." + id
		add(w+".name", src(&l.Name))
		add(w+".desc", src(&l.Desc))
		cmds(w+".on_enter", l.OnEnter)
		if a := l.Map; a != nil {
			for _, id := range a.PointIDs() {
				p := a.Points[id]
				if p == nil {
					continue
				}
				for i, advice := range p.Advice {
					aw := fmt.Sprintf("%s.map.points.%s.advice[%d]", w, id, i)
					add(aw+".heading", advice.Heading)
					add(aw+".text", advice.Text)
				}
			}
		}

		for i := range l.People {
			add(fmt.Sprintf("%s.people[%d].name", w, i), src(&l.People[i].Name))
			cmds(fmt.Sprintf("%s.people[%d].talk", w, i), l.People[i].Talk)
		}
		for i := range l.Actions {
			add(fmt.Sprintf("%s.actions[%d].label", w, i), src(&l.Actions[i].Label))
			cmds(fmt.Sprintf("%s.actions[%d].do", w, i), l.Actions[i].Do)
		}
		for i := range l.Exits {
			add(fmt.Sprintf("%s.exits[%d].label", w, i), src(&l.Exits[i].Label))
			cmds(fmt.Sprintf("%s.exits[%d].do", w, i), l.Exits[i].Do)
		}
	}
	for _, name := range sorted(slices.Collect(maps.Keys(g.Screens))) {
		if screen := g.Screens[name]; screen != nil {
			for i, entry := range screen.Entries {
				cmds(fmt.Sprintf("game.yaml screens.%s.entries[%d].do", name, i), entry.Do)
			}
		}
	}
	for _, id := range sorted(slices.Collect(maps.Keys(db.Events))) {
		cmds("events."+id+".do", db.Events[id].Do)
	}
	if c := db.Game.Campaign; c != nil {
		for _, id := range sorted(slices.Collect(maps.Keys(c.Clash.Tactics))) {
			if t := c.Clash.Tactics[id]; t != nil {
				add("campaign.clash.tactics."+id+".label", src(&t.Label))
			}
		}
		for _, id := range sorted(slices.Collect(maps.Keys(c.Clash.Skills))) {
			if sk := c.Clash.Skills[id]; sk != nil {
				add("campaign.clash.skills."+id+".label", src(&sk.Label))
			}
		}
		for _, id := range c.OrderIDs() {
			o := c.Orders[id]
			if o == nil {
				continue
			}
			w := "campaign.orders." + id
			cmds(w+".do", o.Do)
			cmds(w+".on_win", o.OnWin)
			cmds(w+".on_lose", o.OnLose)
			cmds(w+".on_draw", o.OnDraw)
		}
	}
	return out
}

// languages checks written_in, languages and the translations.
func (c *checker) languages() {
	g := &c.db.Game
	if g.WrittenIn != "" {
		c.needsVersion("game.yaml written_in", "written_in", SchemaVersion4)
		if _, err := catalog.Words(g.WrittenIn); err != nil {
			c.errorf(diag.UnknownReference, "game.yaml written_in", "%q is not a built-in language (use %s or %s)", g.WrittenIn, catalog.English, catalog.Japanese)
		}
	}
	if len(g.Languages) > 0 {
		c.needsVersion("game.yaml languages", "languages", SchemaVersion4)
		seen := map[string]bool{}
		for i, code := range g.Languages {
			w := fmt.Sprintf("game.yaml languages[%d]", i)
			if _, err := catalog.Words(code); err != nil {
				c.errorf(diag.UnknownReference, w, "%q is not a built-in language (use %s or %s)", code, catalog.English, catalog.Japanese)
			}
			if seen[code] {
				c.errorf(diag.Duplicate, w, "%q is written twice", code)
			}
			seen[code] = true
		}
		if !seen[cmp.Or(g.Language, lang.English)] {
			c.errorf(diag.UnknownReference, "game.yaml languages", "the language shown (%s) is not one of languages", cmp.Or(g.Language, lang.English))
		}
	}
	if len(c.db.Words) == 0 {
		return
	}
	texts := c.db.SourceTexts()
	sources := map[string]bool{}
	for _, t := range texts {
		sources[t.Text] = true
	}
	engine, err := catalog.Words(lang.Japanese) // the keys of the engine's words
	if err != nil {
		engine = map[string]string{}
	}
	for _, code := range slices.Sorted(maps.Keys(c.db.Words)) {
		words := c.db.Words[code]
		file := LangDir + "/" + code + ".yaml"
		for _, src := range slices.Sorted(maps.Keys(words)) {
			if _, ok := engine[src]; !sources[src] && !ok {
				c.warnf(diag.UnknownSource, file, "%q is not a text of the game (nor a word of the engine)", src)
			}
			if a, b := sortedInterpolations(src), sortedInterpolations(words[src]); !slices.Equal(a, b) {
				c.errorf(diag.Shape, file, "the translation of %q has the expressions {%s}, not {%s}", src, strings.Join(b, "}, {"), strings.Join(a, "}, {"))
			}
		}
		for _, t := range texts {
			if w, ok := words[t.Text]; !ok || w == "" {
				c.warnf(diag.Untranslated, t.Path, "not translated to %s: %q", code, t.Text)
			}
		}
	}
}

func sortedInterpolations(s string) []string {
	return slices.Sorted(slices.Values(Interpolations(s)))
}
