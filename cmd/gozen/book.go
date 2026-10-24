package main

import (
	"fmt"
	"maps"
	"slices"
	"sort"
	"strings"

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
	var b strings.Builder
	p := func(format string, a ...any) {
		appendf(&b, format, a...)
	}
	g := db.Game
	p("# %s\n\n%s\n\n", g.Title, g.Subtitle)
	p("start: %s, party %s, gold %d, items %s\n\n", g.Start.Location, listOrNone(g.Start.Party), g.Start.Gold, startItems(g.Start.Items))

	p("%s", lang.T("## 場所\n\n"))
	for _, id := range db.Ordered("locations") {
		l := db.Locations[id]
		if l == nil {
			continue
		}
		p(lang.T("### %s（%s）\n\n"), l.Name, id)
		if l.Desc != "" {
			p("%s\n\n", l.Desc)
		}
		if l.Music.Song != "" {
			p(lang.T("- 曲: %s/%s\n"), l.Music.Song, l.Music.Mood)
		}
		if len(l.OnEnter) > 0 {
			p("%s", lang.T("- 着いたとき\n"))
			commands(p, l.OnEnter, "  ")
		}
		for _, pe := range l.People {
			p(lang.T("- 人: %s%s\n"), pe.Name, when(pe.When))
			commands(p, pe.Talk, "  ")
		}
		for _, a := range l.Actions {
			p(lang.T("- できること: %s%s\n"), a.Label, when(a.When))
			commands(p, a.Do, "  ")
		}
		for _, e := range l.Exits {
			p(lang.T("- 行き先: %s → %s%s\n"), e.Label, e.To, when(e.When))
			commands(p, e.Do, "  ")
		}
		if a := l.Map; a != nil {
			for _, id := range a.PointIDs() {
				pt := a.Points[id]
				p(lang.T("- 地図の点: %s → %s%s\n"), id, pt.To, when(pt.When))
				commands(p, pt.Do, "  ")
			}
			for _, r := range a.Roads {
				p(lang.T("- 道: %v（%d 歩）%s\n"), r.Between, r.StepsOrOne(), when(r.When))
				commands(p, r.Do, "  ")
			}
		}
		if s := l.Stage; s != nil {
			for _, id := range s.SpotIDs() {
				p(lang.T("- 立ち位置: %s\n"), id)
				commands(p, s.Spots[id].Do, "  ")
			}
			for _, t := range s.Things {
				if t.Action != "" {
					continue
				}
				p(lang.T("- 調べる物: %s%s\n"), t.Name, when(t.When))
				commands(p, t.Do, "  ")
			}
		}
		if d := l.Dungeon; d != nil {
			dungeonBook(p, l)
		}
		p("\n")
	}

	p("%s", lang.T("## 敵\n\n| id | 名前 | HP | 攻 | 防 | 魔 | 魔防 | 速 | EXP | G | 弱点・耐性 | 状態耐性 | 番人 |\n|---|---|---|---|---|---|---|---|---|---|---|---|---|\n"))
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
		p("| %s | %s | %d | %d | %d | %d | %d | %d | %d | %d | %s | %s | %s |\n", id, e.Name, s["mhp"], s["atk"], s["def"], s["mat"], s["mdf"], s["agi"],
			e.Exp, e.Gold, rates(e.Resist), rates(e.SResist), boss)
	}
	p("\n## Monster book pages\n\n")
	lin := db.Lineage
	for _, id := range db.Ordered("enemies") {
		e := db.Enemies[id]
		if e == nil {
			continue
		}
		p("### %s (%s)\n\n", e.Name, id)
		if e.Lore != "" {
			p("%s\n\n", e.Lore)
		}
		p("- Picture: `%s`\n- Stats: %s\n- Weaknesses and resistances: %s\n", e.Image.File, stats(e.Stats), rates(e.Resist))
		if e.Family != "" {
			p("- Family: %s; rank: %d\n", e.Family, e.Rank)
		}
		if e.Recruit != nil {
			p("- Recruit: %.0f%% after victory; class: %s; level: %d\n", e.Recruit.Chance*100, e.Recruit.Class, max(1, e.Recruit.Level))
			if e.Recruit.BonusItem != "" || e.Recruit.BonusSkill != "" {
				p("- Recruit bonus: +%.0f%% per matching item or party skill (item: %s; skill: %s)\n", e.Recruit.Bonus*100, e.Recruit.BonusItem, e.Recruit.BonusSkill)
			}
		}
		var skills []string
		for _, r := range e.AI.Rules {
			if r.Use != "" {
				skills = append(skills, r.Use)
			}
		}
		if e.Sex != "" {
			p("- Sex: %s\n", e.Sex)
		}
		p("- Skills: %s\n", listOrNone(skills))
		var drops []string
		for _, d := range e.Drops {
			drops = append(drops, fmt.Sprintf("%s (%.0f%%)", d.Item, d.Chance*100))
		}
		p("- Drops: %s\n", listOrNone(drops))
		if lines := lineageLines(db, lin, id); len(lines) > 0 {
			p("- Lineage:\n")
			for _, line := range lines {
				p("%s\n", line)
			}
		}
		p("\n")
	}
	if db.Breeding != nil {
		p("## Monster breeding\n\nMinimum level: %d; inherited skill slots: %d.\n\n", db.Breeding.MinLevel, db.Breeding.SkillSlots)
		for _, rule := range db.Breeding.Pairs {
			p("- %s x %s -> %s (min level %d, min plus %d)\n", rule.Parents[0], rule.Parents[1], rule.Child, rule.MinLevel, rule.MinPlus)
		}
		for _, rule := range db.Breeding.Families {
			p("- %s x %s -> ranked choice of %s\n", rule.Parents[0], rule.Parents[1], strings.Join(rule.Children, ", "))
		}
		p("\n")
	}
	p("%s", lang.T("\n## 敵の群れ\n\n"))
	for _, id := range db.Ordered("troops") {
		t := db.Troops[id]
		if t == nil {
			continue
		}
		p("- %s: %v%s\n", id, t.Members, map[bool]string{true: "", false: lang.T("（逃げられない）")}[t.Escapable()])
		for _, ev := range t.Events {
			p(lang.T("  - 戦闘中 %s\n"), when(ev.When))
			commands(p, ev.Do, "    ")
		}
	}
	p("%s", lang.T("\n## 行動ルール（ガンビット）\n\n"))
	for _, id := range db.Ordered("ai") {
		p("- %s:", id)
		for _, r := range db.AI[id] {
			p(" [%s%s]", r.Use+r.Item, when(r.When))
		}
		p("\n")
	}

	p("%s", lang.T("\n## スキル\n\n| id | 名前 | MP | 範囲 | カットイン | 説明 |\n|---|---|---|---|---|---|\n"))
	for _, id := range db.Ordered("skills") {
		s := db.Skills[id]
		if s == nil {
			continue
		}
		cut := ""
		if s.Cutin {
			cut = "○"
		}
		p("| %s | %s | %d | %s | %s | %s |\n", id, s.Name, s.MP, s.Scope, cut, s.Desc)
	}
	p("%s", lang.T("\n## 道具・装備\n\n| id | 名前 | 種類 | 値段 | 能力 | 説明 |\n|---|---|---|---|---|---|\n"))
	for _, id := range db.Ordered("items") {
		it := db.Items[id]
		if it == nil {
			continue
		}
		p("| %s | %s | %s | %d | %s | %s |\n", id, it.Name, it.Kind, it.Price, stats(it.Stats), it.Desc)
	}
	p("%s", lang.T("\n## 職業とスキルツリー\n\n"))
	for _, id := range db.Ordered("classes") {
		c := db.Classes[id]
		if c == nil {
			continue
		}
		p(lang.T("### %s（%s）\n\n"), c.Name, id)
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
			p(lang.T("- %s（%s）%s\n"), name, n.ID, map[bool]string{true: "", false: " ← " + strings.Join(req, ", ")}[len(req) == 0])
		}
		p("\n")
	}

	p("%s", lang.T("## イベント（台詞の順）\n\n"))
	for _, id := range db.Ordered("events") {
		ev := db.Events[id]
		if ev == nil {
			continue
		}
		p("### %s\n\n", id)
		commands(p, ev.Do, "")
		p("\n")
	}

	flowBook(p, db)

	p("%s", lang.T("## 曲\n\n"))
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
		p(lang.T("- %s（%s）: %s\n"), k, s.Name, strings.Join(moods, " "))
	}
	for _, ctx := range slices.Sorted(maps.Keys(db.Music.Rules)) {
		rules := db.Music.Rules[ctx]
		for _, r := range rules {
			p(lang.T("- ルール %s: %s → %s%s\n"), ctx, r.When.String(), r.Mood+r.Play, map[bool]string{true: lang.T("（すぐ）"), false: ""}[r.Now])
		}
	}
	p("%s", lang.T("\n## ギャラリー\n\n"))
	for _, c := range g.Gallery.CGs {
		p(lang.T("- CG: %s（%s）\n"), c.Title, c.File)
	}
	for _, s := range g.Gallery.Scenes {
		p(lang.T("- シーン: %s（%s、flag.%s）\n"), s.Title, s.Event, s.Flag)
	}
	return tidyBook(b.String())
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

// tidyBook drops the sections (## headings) that have nothing under them but
// a table header, and leaves at most one blank line between blocks.
func tidyBook(s string) string {
	lines := strings.Split(s, "\n")
	var out []string
	for i := 0; i < len(lines); {
		if !strings.HasPrefix(lines[i], "## ") {
			out = append(out, lines[i])
			i++
			continue
		}
		j := i + 1
		for j < len(lines) && !strings.HasPrefix(lines[j], "## ") {
			j++
		}
		if !emptySection(lines[i+1 : j]) {
			out = append(out, lines[i:j]...)
		}
		i = j
	}
	var kept []string
	blank := 0
	for _, l := range out {
		if strings.TrimSpace(l) == "" {
			blank++
			if blank > 1 {
				continue
			}
		} else {
			blank = 0
		}
		kept = append(kept, l)
	}
	return strings.TrimRight(strings.Join(kept, "\n"), "\n") + "\n"
}

// emptySection reports whether the body of a section holds no entry: only blank
// lines and the two lines of a table header.
func emptySection(body []string) bool {
	var rows []string
	for _, l := range body {
		if strings.TrimSpace(l) != "" {
			rows = append(rows, l)
		}
	}
	switch len(rows) {
	case 0:
		return true
	case 2:
		return strings.HasPrefix(rows[0], "|") && strings.HasPrefix(rows[1], "|---")
	}
	return false
}

func dungeonBook(p func(string, ...any), l *bookLocation) {
	d := l.Dungeon
	if gen := d.Generate; gen != nil {
		p(lang.T("- 迷宮: %dx%d、部屋 %d（種から作る。階ごとの種・手書き %d）\n"), gen.Width, gen.Height, gen.Rooms, len(gen.Floors))
		for _, pl := range gen.Place {
			name := pl.Char
			if t := d.Tiles[pl.Char]; t != nil && t.Name != "" {
				name += " " + t.Name
			}
			cnt := "1"
			if pl.Count.Set() {
				cnt = pl.Count.String()
			}
			p(lang.T("  - 置く物 %s: %s\n"), name, cnt)
		}
	}
	for _, r := range d.Roamers {
		var ts []string
		for _, t := range r.Troops {
			ts = append(ts, t.Troop)
		}
		if r.Appear.Set() {
			p(lang.T("  - 追ってくる者 %s（%s、出る %s、%s 歩に 1 マス）\n"), r.Name, r.Move, r.Appear.String(), r.Every.String())
			commands(p, r.Meet, "    ")
			continue
		}
		p(lang.T("  - 強敵シンボル %s（%s、数 %s）: %v\n"), r.Name, r.Move, r.Count.String(), ts)
	}
	var chars []string
	for c := range d.Tiles {
		chars = append(chars, c)
	}
	sort.Strings(chars)
	for _, c := range chars {
		if t := d.Tiles[c]; t != nil && len(t.Do) > 0 {
			p(lang.T("  - タイル %s %s%s\n"), c, t.Name, when(t.When))
			commands(p, t.Do, "    ")
		}
	}
	for _, lk := range d.Looks {
		p(lang.T("  - 見た目 %s: 壁 %s、戦闘背景 %s\n"), lk.When.String(), lk.Wall.File, lk.BattleBG.File)
	}
	if e := l.Encounters; e.Rate.Set() {
		p(lang.T("  - エンカウント: %s（ゲージ %.2f）\n"), e.Rate.String(), e.Gauge)
		for _, t := range e.Troops {
			p("    - %s ×%.0f%s\n", t.Troop, t.Weight, when(t.When))
		}
	}
}

func commands(p func(string, ...any), cmds []bookCommand, ind string) {
	for _, c := range cmds {
		verb := c.Kind
		switch verb {
		case "say":
			who := c.Say.Name
			if who != "" {
				p(lang.T("%s- %s「%s」\n"), ind, who, c.Say.Text)
			} else {
				p("%s- %s\n", ind, c.Say.Text)
			}
		case "choice":
			if c.Prompt != "" {
				p(lang.T("%s- 問い: %s\n"), ind, c.Prompt)
			}
			for _, o := range c.Choice {
				p(lang.T("%s- 選択「%s」%s\n"), ind, o.Label, when(o.When))
				commands(p, o.Do, ind+"  ")
			}
		case "if":
			p(lang.T("%s- もし %s\n"), ind, c.If.String())
			commands(p, c.Then, ind+"  ")
			if len(c.Else) > 0 {
				p(lang.T("%s- そうでなければ\n"), ind)
				commands(p, c.Else, ind+"  ")
			}
		case "battle":
			f := ""
			if len(c.Fighters) > 0 {
				f = fmt.Sprintf(lang.T("（%v だけで）"), c.Fighters)
			}
			p(lang.T("%s- 戦闘 %s%s\n"), ind, c.Battle, f)
			if len(c.OnWin) > 0 {
				p(lang.T("%s- 勝ったら\n"), ind)
				commands(p, c.OnWin, ind+"  ")
			}
			if len(c.OnLose) > 0 {
				p(lang.T("%s- 負けたら\n"), ind)
				commands(p, c.OnLose, ind+"  ")
			}
			if len(c.OnEscape) > 0 {
				p("%s- On escape\n", ind)
				commands(p, c.OnEscape, ind+"  ")
			}
		case "set":
			var kv []string
			for k, v := range c.SetV {
				kv = append(kv, k+" = "+v.String())
			}
			sort.Strings(kv)
			p(lang.T("%s- （%s）\n"), ind, strings.Join(kv, ", "))
		case "call":
			p("%s- → %s\n", ind, c.Call)
		case "give":
			p(lang.T("%s- （%s を %d 手に入れる）\n"), ind, c.Give.Item, c.Give.N())
		case "image":
			if c.Image.File != "" {
				p(lang.T("%s- [絵 %s]\n"), ind, c.Image.File)
			}
		case "move", "dive":
			floor := ""
			if c.Floor.Set() {
				floor = " " + c.Floor.String()
			}
			p(lang.T("%s- （%s %s%s）\n"), ind, verb, c.Move+c.Dive, floor)
		default:
			p("%s- [%s]\n", ind, verb)
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
func flowBook(p func(string, ...any), db *bookReport) {
	ids := db.Ordered("events")
	if len(ids) == 0 {
		return
	}
	p("%s", lang.T("## 流れ\n\n"))
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
		p("- %s: %s\n", id, strings.Join(parts, "; "))
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
		p("\n```mermaid\ngraph TD\n%s\n```\n", strings.Join(edges, "\n"))
	}
	p("\n")
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
