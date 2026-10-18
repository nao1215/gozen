package main

import (
	"fmt"
	"maps"
	"os"
	"slices"
	"sort"
	"strings"

	"github.com/nao1215/gozen/data"
)

func (c *command) book(dir string) error {
	db, err := data.Load(os.DirFS(dir), ".")
	if err != nil {
		return err
	}
	var b strings.Builder
	p := func(format string, a ...any) {
		appendf(&b, format, a...)
	}
	g := db.Game
	p("# %s\n\n%s\n\n", g.Title, g.Subtitle)
	p("start: %s, party %v, gold %d, items %v\n\n", g.Start.Location, g.Start.Party, g.Start.Gold, g.Start.Items)

	p("## 場所\n\n")
	for _, id := range db.Ordered("locations") {
		l := db.Locations[id]
		if l == nil {
			continue
		}
		p("### %s（%s）\n\n", l.Name, id)
		if l.Desc != "" {
			p("%s\n\n", l.Desc)
		}
		if l.Music.Song != "" {
			p("- 曲: %s/%s\n", l.Music.Song, l.Music.Mood)
		}
		if len(l.OnEnter) > 0 {
			p("- 着いたとき\n")
			commands(p, db, l.OnEnter, "  ")
		}
		for _, pe := range l.People {
			p("- 人: %s%s\n", pe.Name, when(pe.When))
			commands(p, db, pe.Talk, "  ")
		}
		for _, a := range l.Actions {
			p("- できること: %s%s\n", a.Label, when(a.When))
			commands(p, db, a.Do, "  ")
		}
		for _, e := range l.Exits {
			p("- 行き先: %s → %s%s\n", e.Label, e.To, when(e.When))
			commands(p, db, e.Do, "  ")
		}
		if d := l.Dungeon; d != nil {
			dungeonBook(p, db, l)
		}
		p("\n")
	}

	p("## 敵\n\n| id | 名前 | HP | 攻 | 防 | 魔 | 魔防 | 速 | EXP | G | 弱点・耐性 | 状態耐性 | 番人 |\n|---|---|---|---|---|---|---|---|---|---|---|---|---|\n")
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
	p("\n## 敵の群れ\n\n")
	for _, id := range db.Ordered("troops") {
		t := db.Troops[id]
		if t == nil {
			continue
		}
		p("- %s: %v%s\n", id, t.Members, map[bool]string{true: "", false: "（逃げられない）"}[t.Escapable()])
		for _, ev := range t.Events {
			p("  - 戦闘中 %s\n", when(ev.When))
			commands(p, db, ev.Do, "    ")
		}
	}
	p("\n## 行動ルール（ガンビット）\n\n")
	for _, id := range db.Ordered("ai") {
		p("- %s:", id)
		for _, r := range db.AI[id] {
			p(" [%s%s]", r.Use+r.Item, when(r.When))
		}
		p("\n")
	}

	p("\n## スキル\n\n| id | 名前 | MP | 範囲 | カットイン | 説明 |\n|---|---|---|---|---|---|\n")
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
	p("\n## 道具・装備\n\n| id | 名前 | 種類 | 値段 | 能力 | 説明 |\n|---|---|---|---|---|---|\n")
	for _, id := range db.Ordered("items") {
		it := db.Items[id]
		if it == nil {
			continue
		}
		p("| %s | %s | %s | %d | %s | %s |\n", id, it.Name, it.Kind, it.Price, stats(it.Stats), it.Desc)
	}
	p("\n## 職業とスキルツリー\n\n")
	for _, id := range db.Ordered("classes") {
		c := db.Classes[id]
		if c == nil {
			continue
		}
		p("### %s（%s）\n\n", c.Name, id)
		for _, n := range c.Tree {
			name := n.Name
			if sk := db.Skills[n.Skill]; sk != nil {
				name = sk.Name
			} else if n.Skill != "" && name == "" {
				name = n.Skill + "（未知のスキル）"
			}
			var req []string
			for _, r := range n.Requires {
				req = append(req, fmt.Sprintf("%s Lv%d", r.Node, r.Level))
			}
			p("- %s（%s）%s\n", name, n.ID, map[bool]string{true: "", false: " ← " + strings.Join(req, ", ")}[len(req) == 0])
		}
		p("\n")
	}

	p("## イベント（台詞の順）\n\n")
	for _, id := range db.Ordered("events") {
		ev := db.Events[id]
		if ev == nil {
			continue
		}
		p("### %s\n\n", id)
		commands(p, db, ev.Do, "")
		p("\n")
	}

	p("## 曲\n\n")
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
		p("- %s（%s）: %s\n", k, s.Name, strings.Join(moods, " "))
	}
	for _, ctx := range slices.Sorted(maps.Keys(db.Music.Rules)) {
		rules := db.Music.Rules[ctx]
		for _, r := range rules {
			p("- ルール %s: %s → %s%s\n", ctx, r.When.String(), r.Mood+r.Play, map[bool]string{true: "（すぐ）", false: ""}[r.Now])
		}
	}
	p("\n## ギャラリー\n\n")
	for _, c := range g.Gallery.CGs {
		p("- CG: %s（%s）\n", c.Title, c.File)
	}
	for _, s := range g.Gallery.Scenes {
		p("- シーン: %s（%s、flag.%s）\n", s.Title, s.Event, s.Flag)
	}
	c.out.Print(b.String())
	return nil
}

func dungeonBook(p func(string, ...any), db *data.DB, l *data.Location) {
	d := l.Dungeon
	if gen := d.Generate; gen != nil {
		p("- 迷宮: %dx%d、部屋 %d（種から作る。階ごとの種・手書き %d）\n", gen.Width, gen.Height, gen.Rooms, len(gen.Floors))
		for _, pl := range gen.Place {
			name := pl.Char
			if t := d.Tiles[pl.Char]; t != nil && t.Name != "" {
				name += " " + t.Name
			}
			cnt := "1"
			if pl.Count.Set() {
				cnt = pl.Count.String()
			}
			p("  - 置く物 %s: %s\n", name, cnt)
		}
	}
	for _, r := range d.Roamers {
		var ts []string
		for _, t := range r.Troops {
			ts = append(ts, t.Troop)
		}
		if r.Appear.Set() {
			p("  - 追ってくる者 %s（%s、出る %s、%s 歩に 1 マス）\n", r.Name, r.Move, r.Appear.String(), r.Every.String())
			commands(p, db, r.Meet, "    ")
			continue
		}
		p("  - 強敵シンボル %s（%s、数 %s）: %v\n", r.Name, r.Move, r.Count.String(), ts)
	}
	var chars []string
	for c := range d.Tiles {
		chars = append(chars, c)
	}
	sort.Strings(chars)
	for _, c := range chars {
		if t := d.Tiles[c]; t != nil && len(t.Do) > 0 {
			p("  - タイル %s %s%s\n", c, t.Name, when(t.When))
			commands(p, db, t.Do, "    ")
		}
	}
	for _, lk := range d.Looks {
		p("  - 見た目 %s: 壁 %s、戦闘背景 %s\n", lk.When.String(), lk.Wall.File, lk.BattleBG.File)
	}
	if e := l.Encounters; e.Rate.Set() {
		p("  - エンカウント: %s（ゲージ %.2f）\n", e.Rate.String(), e.Gauge)
		for _, t := range e.Troops {
			p("    - %s ×%.0f%s\n", t.Troop, t.Weight, when(t.When))
		}
	}
}

func commands(p func(string, ...any), db *data.DB, cmds []data.Command, ind string) {
	for _, c := range cmds {
		verb, err := c.Verb()
		if err != nil {
			p("%s- invalid command: %v\n", ind, err)
			continue
		}
		switch verb {
		case "say":
			who := c.Say.Who
			if a := db.Actors[who]; a != nil {
				who = a.Name
			}
			if who != "" {
				p("%s- %s「%s」\n", ind, who, c.Say.Text)
			} else {
				p("%s- %s\n", ind, c.Say.Text)
			}
		case "choice":
			if c.Prompt != "" {
				p("%s- 問い: %s\n", ind, c.Prompt)
			}
			for _, o := range c.Choice {
				p("%s- 選択「%s」%s\n", ind, o.Label, when(o.When))
				commands(p, db, o.Do, ind+"  ")
			}
		case "if":
			p("%s- もし %s\n", ind, c.If.String())
			commands(p, db, c.Then, ind+"  ")
			if len(c.Else) > 0 {
				p("%s- そうでなければ\n", ind)
				commands(p, db, c.Else, ind+"  ")
			}
		case "battle":
			f := ""
			if len(c.Fighters) > 0 {
				f = fmt.Sprintf("（%v だけで）", c.Fighters)
			}
			p("%s- 戦闘 %s%s\n", ind, c.Battle, f)
			if len(c.OnWin) > 0 {
				p("%s- 勝ったら\n", ind)
				commands(p, db, c.OnWin, ind+"  ")
			}
			if len(c.OnLose) > 0 {
				p("%s- 負けたら\n", ind)
				commands(p, db, c.OnLose, ind+"  ")
			}
		case "set":
			var kv []string
			for k, v := range c.SetV {
				kv = append(kv, k+" = "+v.String())
			}
			sort.Strings(kv)
			p("%s- （%s）\n", ind, strings.Join(kv, ", "))
		case "call":
			p("%s- → %s\n", ind, c.Call)
		case "give":
			p("%s- （%s を %d 手に入れる）\n", ind, c.Give.Item, c.Give.N())
		case "image":
			if c.Image.File != "" {
				p("%s- [絵 %s]\n", ind, c.Image.File)
			}
		case "move", "dive":
			p("%s- （%s %s%s）\n", ind, verb, c.Move+c.Dive, c.Floor.String())
		}
	}
}

func when(e data.Expr) string {
	if !e.Set() {
		return ""
	}
	return "（条件 " + e.String() + "）"
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

func stats(s data.Stats) string {
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
