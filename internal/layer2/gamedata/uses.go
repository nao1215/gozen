package gamedata

// Uses は ゲームの どこかの コマンドに verb（sp_suggest など）が あるか。
// イベント・場所（人の 話・項目・出口・着いた とき・迷宮の タイル・complete・walked）・敵の 組の 戦闘中の イベントを、
// if・choice・battle の 枝の 中まで たどる。画面の 欄を ゲームが 使う 時だけ 出すのに 使う。
func (db *DB) Uses(verb string) bool {
	found := false
	db.EachCommand(func(c *Command) {
		if v, err := c.Verb(); err == nil && v == verb {
			found = true
		}
	})
	return found
}

// EachCommand は ゲームの すべての コマンドを（if・choice・battle の 枝の 中まで）たどって fn に 渡す。
func (db *DB) EachCommand(fn func(*Command)) {
	var in func(cs []Command)
	in = func(cs []Command) {
		for i := range cs {
			c := &cs[i]
			fn(c)
			in(c.Then)
			in(c.Else)
			in(c.OnWin)
			in(c.OnLose)
			in(c.OnRun)
			for _, o := range c.Choice {
				in(o.Do)
			}
		}
	}
	for _, e := range db.Events {
		in(e.Do)
	}
	for _, l := range db.Locations {
		in(l.OnEnter)
		for _, p := range l.People {
			in(p.Talk)
		}
		for _, a := range l.Actions {
			in(a.Do)
		}
		for _, x := range l.Exits {
			in(x.Do)
		}
		if d := l.Dungeon; d != nil {
			for _, t := range d.Tiles {
				if t != nil {
					in(t.Do)
				}
			}
			in(d.Complete)
			in(d.Walked)
		}
	}
	for _, t := range db.Troops {
		for _, e := range t.Events {
			in(e.Do)
		}
	}
}
