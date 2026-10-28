package main

import (
	"fmt"
	"maps"
	"slices"
	"sort"
	"strings"

	md "github.com/nao1215/markdown"

	"github.com/nao1215/gozen/internal/layer2/gamedata"
	lang "github.com/nao1215/gozen/internal/text"
)

func loadBook(dir string) (*bookReport, error) {
	fsys, err := gameFS(dir)
	if err != nil {
		return nil, err
	}
	if err := gamedata.UseWords(fsys); err != nil {
		return nil, err
	}
	db, err := gamedata.LoadDir(fsys, ".")
	if err != nil {
		return nil, err
	}
	return buildBook(db), nil
}

func renderBook(db *bookReport) string {
	root := md.NewMarkdown(nil, md.WithBlockSpacing())
	b := md.NewMarkdown(nil, md.WithBlockSpacing())
	title := ""
	section := func(next string) {
		if body := strings.TrimSpace(b.String()); body != "" {
			if title != "" {
				root.H2(title)
			}
			root.PlainText(body)
		}
		b = md.NewMarkdown(nil, md.WithBlockSpacing())
		title = next
	}
	g := db.Game
	b.H1(g.Title)
	if g.Subtitle != "" {
		b.PlainText(g.Subtitle)
	}
	b.PlainTextf("start: %s, party %s, gold %d, items %s", g.Start.Location, listOrNone(g.Start.Party), g.Start.Gold, startItems(g.Start.Items))

	section(lang.T("場所一覧"))
	for _, id := range db.Ordered("locations") {
		l := db.Locations[id]
		if l == nil {
			continue
		}
		b.H3f(lang.T("%s（%s）"), l.Name, id)
		if l.Desc != "" {
			b.PlainTextf("%s", l.Desc)
		}
		if l.Music.Song != "" {
			reportBullet(b, "", lang.T("曲: %s/%s"), l.Music.Song, l.Music.Mood)
		}
		if len(l.OnEnter) > 0 {
			reportBullet(b, "", "%s", lang.T("着いたとき"))
			commands(b, l.OnEnter, "  ")
		}
		for _, pe := range l.People {
			reportBullet(b, "", lang.T("人: %s%s"), pe.Name, when(pe.When))
			commands(b, pe.Talk, "  ")
		}
		for _, a := range l.Actions {
			reportBullet(b, "", lang.T("できること: %s%s"), a.Label, when(a.When))
			commands(b, a.Do, "  ")
		}
		for _, e := range l.Exits {
			reportBullet(b, "", lang.T("行き先: %s → %s%s"), e.Label, e.To, when(e.When))
			commands(b, e.Do, "  ")
		}
		if a := l.Map; a != nil {
			for _, id := range a.PointIDs() {
				pt := a.Points[id]
				reportBullet(b, "", lang.T("地図の点: %s → %s%s"), id, pt.To, when(pt.When))
				commands(b, pt.Do, "  ")
			}
			for _, r := range a.Roads {
				reportBullet(b, "", lang.T("道: %v（%d 歩）%s"), r.Between, r.StepsOrOne(), when(r.When))
				commands(b, r.Do, "  ")
			}
		}
		if s := l.Stage; s != nil {
			for _, id := range s.SpotIDs() {
				reportBullet(b, "", lang.T("立ち位置: %s"), id)
				commands(b, s.Spots[id].Do, "  ")
			}
			for _, t := range s.Things {
				if t.Action != "" {
					continue
				}
				reportBullet(b, "", lang.T("調べる物: %s%s"), t.Name, when(t.When))
				commands(b, t.Do, "  ")
			}
		}
		if d := l.Dungeon; d != nil {
			dungeonBook(b, l)
		}

	}

	section(lang.T("敵"))
	enemyTable := md.TableSet{EscapeCells: true, Header: []string{"id", lang.T("名前"), "HP", lang.T("攻"), lang.T("防"), lang.T("魔"), lang.T("魔防"), lang.T("速"), "EXP", "G", lang.T("弱点・耐性"), lang.T("状態耐性"), lang.T("番人")}}
	for _, id := range db.Ordered("enemies") {
		e := db.Enemies[id]
		if e == nil {
			continue
		}
		s := e.Stats
		boss := ""
		if e.Boss {
			boss = "○"
		}
		enemyTable.Rows = append(enemyTable.Rows, reportCells(id, e.Name, s["mhp"], s["atk"], s["def"], s["mat"], s["mdf"], s["agi"], e.Exp, e.Gold, rates(e.Resist), rates(e.SResist), boss))
	}
	if len(enemyTable.Rows) > 0 {
		b.Table(enemyTable)
	}
	section("Monster book pages")
	lin := db.Lineage
	for _, id := range db.Ordered("enemies") {
		e := db.Enemies[id]
		if e == nil {
			continue
		}
		b.H3f("%s (%s)", e.Name, id)
		if e.Lore != "" {
			b.PlainTextf("%s", e.Lore)
		}
		reportBullet(b, "", "Picture: %s", md.Code(e.Image.File))
		reportBullet(b, "", "Stats: %s", stats(e.Stats))
		reportBullet(b, "", "Weaknesses and resistances: %s", rates(e.Resist))
		if e.Family != "" {
			reportBullet(b, "", "Family: %s; rank: %d", e.Family, e.Rank)
		}
		if e.Recruit != nil {
			reportBullet(b, "", "Recruit: %.0f%% after victory; class: %s; level: %d", e.Recruit.Chance*100, e.Recruit.Class, max(1, e.Recruit.Level))
			if e.Recruit.BonusItem != "" || e.Recruit.BonusSkill != "" {
				reportBullet(b, "", "Recruit bonus: +%.0f%% per matching item or party skill (item: %s; skill: %s)", e.Recruit.Bonus*100, e.Recruit.BonusItem, e.Recruit.BonusSkill)
			}
		}
		var skills []string
		for _, r := range e.AI.Rules {
			if r.Use != "" {
				skills = append(skills, r.Use)
			}
		}
		if e.Sex != "" {
			reportBullet(b, "", "Sex: %s", e.Sex)
		}
		reportBullet(b, "", "Skills: %s", listOrNone(skills))
		var drops []string
		for _, d := range e.Drops {
			drops = append(drops, fmt.Sprintf("%s (%.0f%%)", d.Item, d.Chance*100))
		}
		reportBullet(b, "", "Drops: %s", listOrNone(drops))
		if lines := lineageLines(db, lin, id); len(lines) > 0 {
			reportBullet(b, "", "Lineage:")
			for _, line := range lines {
				b.PlainTextf("%s", line)
			}
		}

	}
	if db.Breeding != nil {
		section("Monster breeding")
		b.PlainTextf("Minimum level: %d; inherited skill slots: %d.", db.Breeding.MinLevel, db.Breeding.SkillSlots)
		for _, rule := range db.Breeding.Pairs {
			reportBullet(b, "", "%s x %s -> %s (min level %d, min plus %d)", rule.Parents[0], rule.Parents[1], rule.Child, rule.MinLevel, rule.MinPlus)
		}
		for _, rule := range db.Breeding.Families {
			reportBullet(b, "", "%s x %s -> ranked choice of %s", rule.Parents[0], rule.Parents[1], strings.Join(rule.Children, ", "))
		}

	}
	section(lang.T("敵の群れ"))
	for _, id := range db.Ordered("troops") {
		t := db.Troops[id]
		if t == nil {
			continue
		}
		reportBullet(b, "", "%s: %v%s", id, t.Members, map[bool]string{true: "", false: lang.T("（逃げられない）")}[t.Escapable()])
		for _, ev := range t.Events {
			reportBullet(b, "  ", lang.T("戦闘中 %s"), when(ev.When))
			commands(b, ev.Do, "    ")
		}
	}
	section(lang.T("行動ルール（ガンビット）"))
	for _, id := range db.Ordered("ai") {
		var rules []string
		for _, r := range db.AI[id] {
			rules = append(rules, fmt.Sprintf("[%s%s]", r.Use+r.Item, when(r.When)))
		}
		reportBullet(b, "", "%s: %s", id, strings.Join(rules, " "))
	}

	section(lang.T("スキル"))
	skillTable := md.TableSet{EscapeCells: true, Header: []string{"id", lang.T("名前"), "MP", lang.T("範囲"), lang.T("カットイン"), lang.T("説明")}}
	for _, id := range db.Ordered("skills") {
		s := db.Skills[id]
		if s == nil {
			continue
		}
		cut := ""
		if s.Cutin {
			cut = "○"
		}
		skillTable.Rows = append(skillTable.Rows, reportCells(id, s.Name, s.MP, s.Scope, cut, s.Desc))
	}
	if len(skillTable.Rows) > 0 {
		b.Table(skillTable)
	}
	section(lang.T("道具・装備"))
	itemTable := md.TableSet{EscapeCells: true, Header: []string{"id", lang.T("名前"), lang.T("種類"), lang.T("値段"), lang.T("能力"), lang.T("説明")}}
	for _, id := range db.Ordered("items") {
		it := db.Items[id]
		if it == nil {
			continue
		}
		itemTable.Rows = append(itemTable.Rows, reportCells(id, it.Name, it.Kind, it.Price, stats(it.Stats), it.Desc))
	}
	if len(itemTable.Rows) > 0 {
		b.Table(itemTable)
	}
	section(lang.T("職業とスキルツリー"))
	for _, id := range db.Ordered("classes") {
		c := db.Classes[id]
		if c == nil {
			continue
		}
		b.H3f(lang.T("%s（%s）"), c.Name, id)
		for _, n := range c.Tree {
			name := n.Name
			if sk := db.Skills[n.Skill]; sk != nil {
				name = sk.Name
			} else if n.Skill != "" && name == "" {
				name = n.Skill + lang.T("（未知のスキル）")
			}
			var req []string
			for _, r := range n.Requires {
				req = append(req, fmt.Sprintf("%s Lv%d", r.Node, r.Level))
			}
			reportBullet(b, "", lang.T("%s（%s）%s"), name, n.ID, map[bool]string{true: "", false: " ← " + strings.Join(req, ", ")}[len(req) == 0])
		}

	}

	section(lang.T("イベント（台詞の順）"))
	for _, id := range db.Ordered("events") {
		ev := db.Events[id]
		if ev == nil {
			continue
		}
		b.H3f("%s", id)
		commands(b, ev.Do, "")

	}

	section(lang.T("流れ"))
	flowBook(b, db)

	section(lang.T("曲"))
	var songs []string
	for k := range db.Music.Songs {
		songs = append(songs, k)
	}
	sort.Strings(songs)
	for _, k := range songs {
		s := db.Music.Songs[k]
		var moods []string
		for m := range s.Moods {
			moods = append(moods, m)
		}
		sort.Strings(moods)
		reportBullet(b, "", lang.T("%s（%s）: %s"), k, s.Name, strings.Join(moods, " "))
	}
	for _, ctx := range slices.Sorted(maps.Keys(db.Music.Rules)) {
		rules := db.Music.Rules[ctx]
		for _, r := range rules {
			reportBullet(b, "", lang.T("ルール %s: %s → %s%s"), ctx, r.When.String(), r.Mood+r.Play, map[bool]string{true: lang.T("（すぐ）"), false: ""}[r.Now])
		}
	}
	section(lang.T("ギャラリー"))
	for _, c := range g.Gallery.CGs {
		reportBullet(b, "", lang.T("CG: %s（%s）"), c.Title, c.File)
	}
	for _, s := range g.Gallery.Scenes {
		reportBullet(b, "", lang.T("シーン: %s（%s、flag.%s）"), s.Title, s.Event, s.Flag)
	}
	section("")
	return reportMarkdown(root)
}

// listOrNone joins ids with ", ", or says (none).
func listOrNone(ids []string) string {
	if len(ids) == 0 {
		return "(none)"
	}
	return strings.Join(ids, ", ")
}

// startItems lists the starting items as "id ×n" in id order, or (none).
func startItems(items map[string]int) string {
	var out []string
	for _, id := range slices.Sorted(maps.Keys(items)) {
		out = append(out, fmt.Sprintf("%s ×%d", id, items[id]))
	}
	return listOrNone(out)
}

func dungeonBook(b *md.Markdown, l *bookLocation) {
	d := l.Dungeon
	if gen := d.Generate; gen != nil {
		reportBullet(b, "", lang.T("迷宮: %dx%d、部屋 %d（種から作る。階ごとの種・手書き %d）"), gen.Width, gen.Height, gen.Rooms, len(gen.Floors))
		for _, pl := range gen.Place {
			name := pl.Char
			if t := d.Tiles[pl.Char]; t != nil && t.Name != "" {
				name += " " + t.Name
			}
			cnt := "1"
			if pl.Count.Set() {
				cnt = pl.Count.String()
			}
			reportBullet(b, "  ", lang.T("置く物 %s: %s"), name, cnt)
		}
	}
	for _, r := range d.Roamers {
		var ts []string
		for _, t := range r.Troops {
			ts = append(ts, t.Troop)
		}
		if r.Appear.Set() {
			reportBullet(b, "  ", lang.T("追ってくる者 %s（%s、出る %s、%s 歩に 1 マス）"), r.Name, r.Move, r.Appear.String(), r.Every.String())
			commands(b, r.Meet, "    ")
			continue
		}
		reportBullet(b, "  ", lang.T("強敵シンボル %s（%s、数 %s）: %v"), r.Name, r.Move, r.Count.String(), ts)
	}
	var chars []string
	for c := range d.Tiles {
		chars = append(chars, c)
	}
	sort.Strings(chars)
	for _, c := range chars {
		if t := d.Tiles[c]; t != nil && len(t.Do) > 0 {
			reportBullet(b, "  ", lang.T("タイル %s %s%s"), c, t.Name, when(t.When))
			commands(b, t.Do, "    ")
		}
	}
	for _, lk := range d.Looks {
		reportBullet(b, "  ", lang.T("見た目 %s: 壁 %s、戦闘背景 %s"), lk.When.String(), lk.Wall.File, lk.BattleBG.File)
	}
	if e := l.Encounters; e.Rate.Set() {
		reportBullet(b, "  ", lang.T("エンカウント: %s（ゲージ %.2f）"), e.Rate.String(), e.Gauge)
		for _, t := range e.Troops {
			reportBullet(b, "    ", "%s ×%.0f%s", t.Troop, t.Weight, when(t.When))
		}
	}
}

func commands(b *md.Markdown, cmds []bookCommand, ind string) {
	for _, c := range cmds {
		verb := c.Kind
		switch verb {
		case "respond":
			reportBullet(b, ind, "respond %s: %s (from %s)", c.Respond.Who, c.Respond.Text, c.Respond.From)
		case "say":
			who := c.Say.Name
			if who != "" {
				reportBullet(b, ind, lang.T("%s「%s」"), who, c.Say.Text)
			} else {
				reportBullet(b, ind, "%s", c.Say.Text)
			}
		case "choice":
			if c.Prompt != "" {
				reportBullet(b, ind, lang.T("問い: %s"), c.Prompt)
			}
			for _, o := range c.Choice {
				reportBullet(b, ind, lang.T("選択「%s」%s"), o.Label, when(o.When))
				commands(b, o.Do, ind+"  ")
			}
		case "if":
			reportBullet(b, ind, lang.T("もし %s"), c.If.String())
			commands(b, c.Then, ind+"  ")
			if len(c.Else) > 0 {
				reportBullet(b, ind, "%s", lang.T("そうでなければ"))
				commands(b, c.Else, ind+"  ")
			}
		case "battle":
			f := ""
			if len(c.Fighters) > 0 {
				f = fmt.Sprintf(lang.T("（%v だけで）"), c.Fighters)
			}
			reportBullet(b, ind, lang.T("戦闘 %s%s"), c.Battle, f)
			if len(c.OnWin) > 0 {
				reportBullet(b, ind, "%s", lang.T("勝ったら"))
				commands(b, c.OnWin, ind+"  ")
			}
			if len(c.OnLose) > 0 {
				reportBullet(b, ind, "%s", lang.T("負けたら"))
				commands(b, c.OnLose, ind+"  ")
			}
			if len(c.OnEscape) > 0 {
				reportBullet(b, ind, "On escape")
				commands(b, c.OnEscape, ind+"  ")
			}
		case "set":
			var kv []string
			for k, v := range c.SetV {
				kv = append(kv, k+" = "+v.String())
			}
			sort.Strings(kv)
			reportBullet(b, ind, lang.T("（%s）"), strings.Join(kv, ", "))
		case "call":
			reportBullet(b, ind, "→ %s", c.Call)
		case "give":
			reportBullet(b, ind, lang.T("（%s を %d 手に入れる）"), c.Give.Item, c.Give.N())
		case "image":
			if c.Image.File != "" {
				reportBullet(b, ind, lang.T("[絵 %s]"), c.Image.File)
			}
		case "move", "dive":
			floor := ""
			if c.Floor.Set() {
				floor = " " + c.Floor.String()
			}
			reportBullet(b, ind, lang.T("（%s %s%s）"), verb, c.Move+c.Dive, floor)
		default:
			reportBullet(b, ind, "[%s]", verb)
		}
	}
}

func when(e bookExpr) string {
	if !e.Set() {
		return ""
	}
	return fmt.Sprintf(lang.T("（条件 %s）"), e.String())
}

func rates(m map[string]float64) string {
	var ks []string
	for k := range m {
		ks = append(ks, k)
	}
	sort.Strings(ks)
	var out []string
	for _, k := range ks {
		out = append(out, fmt.Sprintf("%s×%.2g", k, m[k]))
	}
	return strings.Join(out, " ")
}

func stats(s gamedata.Stats) string {
	var ks []string
	for k := range s {
		ks = append(ks, k)
	}
	sort.Strings(ks)
	var out []string
	for _, k := range ks {
		out = append(out, fmt.Sprintf("%s%+d", k, s[k]))
	}
	return strings.Join(out, " ")
}

// flowBook writes the flow of the events (ADR 0029): what each event sets and reads, which
// events it calls and where it moves, from the index shared with the checks and the
// --dev panel (DB.StateUses), and a mermaid graph of the calls, the moves and the places
// whose arrival starts an event.
func flowBook(b *md.Markdown, db *bookReport) {
	ids := db.Ordered("events")
	if len(ids) == 0 {
		return
	}

	var edges []string
	for _, f := range db.Flow {
		id := f.ID
		ev := db.Events[id]
		sets, reads, calls, moves := f.Sets, f.Reads, f.Calls, f.Moves
		var parts []string
		for _, part := range []struct {
			label string
			list  []string
		}{{"sets", sets}, {"reads", reads}, {"calls", calls}, {"moves", moves}} {
			if len(part.list) > 0 {
				parts = append(parts, part.label+" "+strings.Join(part.list, ", "))
			}
		}
		if len(parts) == 0 {
			parts = []string{"-"}
		}
		reportBullet(b, "", "%s: %s", id, strings.Join(parts, "; "))
		for _, c := range calls {
			edges = append(edges, fmt.Sprintf("  %s --> %s", id, c))
		}
		for _, m := range moves {
			edges = append(edges, fmt.Sprintf("  %s -->|move| at_%s", id, m))
		}
		if ev.Trigger != "" && ev.Location != "" {
			label := ev.Trigger
			if ev.When.Set() {
				label += " " + strings.ReplaceAll(ev.When.String(), `"`, "'")
			}
			edges = append(edges, fmt.Sprintf("  at_%s -->|%s| %s", ev.Location, label, id))
		}
	}
	if len(edges) > 0 {
		b.CodeBlocks(md.SyntaxHighlight("mermaid"), "graph TD\n"+strings.Join(edges, "\n"))
	}
}

// eventLinks are the events a command list calls and the places it moves to, into its
// branches, in the order written.
func eventLinks(cmds []gamedata.Command) (calls, moves []string) {
	var walk func(cs []gamedata.Command)
	walk = func(cs []gamedata.Command) {
		for i := range cs {
			c := &cs[i]
			if c.Call != "" && !slices.Contains(calls, c.Call) {
				calls = append(calls, c.Call)
			}
			if c.Move != "" && !slices.Contains(moves, c.Move) {
				moves = append(moves, c.Move)
			}
			walk(c.Then)
			walk(c.Else)
			walk(c.OnWin)
			walk(c.OnLose)
			walk(c.OnRun)
			for _, o := range c.Choice {
				walk(o.Do)
			}
		}
	}
	walk(cmds)
	return calls, moves
}

// bookVoices exposes the runtime's recording inventory to external voice generators.
func (c *command) bookVoices(dir string) error {
	fsys, err := gameFS(dir)
	if err != nil {
		return err
	}
	db, err := gamedata.LoadDir(fsys, ".")
	if err != nil {
		return err
	}
	lines := db.StreamVoiceLines()
	if c.report.Machine() {
		return c.report.Result(c.out, "ok", map[string]any{"version": 1, "lines": lines})
	}
	doc := md.NewMarkdown(nil)
	doc.H1("Stream voices")
	for _, line := range lines {
		reportBullet(doc, "", "%s / %s: %s (file: %s; runtime fields: %s)", line.Channel, line.Language, line.Text, line.File, strings.Join(line.Fields, ", "))
	}
	c.out.Print(reportMarkdown(doc))
	return nil
}
