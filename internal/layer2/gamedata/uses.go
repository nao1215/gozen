package gamedata

// Uses reports whether any command in the game has the verb (such as sp_suggest).
// It walks events, places (person talk, items, exits, on-arrival, dungeon tiles, complete, walked) and in-battle events of enemy groups,
// descending into the branches of if, choice and battle. It is used to show a screen field only when the game uses it.
func (db *DB) Uses(verb string) bool {
	found := false
	db.EachCommand(func(c *Command) {
		if v, err := c.Verb(); err == nil && v == verb {
			found = true
		}
	})
	return found
}

// EachCommand walks every command in the game (descending into the branches of if, choice and battle) and passes it to fn.
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
		if s := l.Stage; s != nil {
			for _, id := range s.SpotIDs() {
				if sp := s.Spots[id]; sp != nil {
					in(sp.Do)
				}
			}
			for _, t := range s.Things {
				in(t.Do)
			}
		}
		if a := l.Map; a != nil {
			for _, id := range a.PointIDs() {
				if p := a.Points[id]; p != nil {
					in(p.Do)
				}
			}
			for _, r := range a.Roads {
				in(r.Do)
			}
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
