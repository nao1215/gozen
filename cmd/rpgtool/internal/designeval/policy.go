package designeval

import (
	"context"
	"fmt"
	"slices"
	"strings"

	"github.com/nao1215/gozen/data"
	"github.com/nao1215/gozen/world"
)

type Policy struct {
	ID     string    `yaml:"id"`
	Phase  string    `yaml:"phase"`
	When   data.Expr `yaml:"when"`
	Try    []Attempt `yaml:"try"`
	Reward string    `yaml:"reward"`
	Words  []string  `yaml:"words"`
	Rotate []string  `yaml:"rotate"`
	Accept string    `yaml:"accept"`
}

type Attempt struct {
	Tile    string `yaml:"tile"`
	Item    string `yaml:"item"`
	At      string `yaml:"at"`
	Entry   string `yaml:"entry"`
	Explore string `yaml:"explore"`
}

func (c *Config) checkPolicies(db *data.DB) error {
	kinds := map[string]bool{}
	for _, r := range c.Rewards {
		kinds[r.Kind] = true
	}
	for _, p := range c.Personas {
		seen := map[string]bool{}
		for _, r := range p.Policies {
			where := fmt.Sprintf("persona %s policy %s", p.ID, r.ID)
			if r.ID == "" || seen[r.ID] {
				return fmt.Errorf("%s: missing or duplicate id", where)
			}
			seen[r.ID] = true
			if !slices.Contains([]string{"step", "town", "night"}, r.Phase) {
				return fmt.Errorf("%s: unknown phase %q", where, r.Phase)
			}
			if r.Reward != "" && !kinds[r.Reward] {
				return fmt.Errorf("%s: unknown reward %q", where, r.Reward)
			}
			if len(r.Try) == 0 {
				return fmt.Errorf("%s: no attempts", where)
			}
			for _, a := range r.Try {
				n := 0
				for _, s := range []string{a.Tile, a.Item, a.Entry, a.Explore} {
					if s != "" {
						n++
					}
				}
				if n != 1 {
					return fmt.Errorf("%s: write one of tile, item, entry, explore", where)
				}
				if a.Tile != "" && len([]rune(a.Tile)) != 1 {
					return fmt.Errorf("%s: tile needs one character", where)
				}
				if a.Explore != "" && !slices.Contains([]string{"events", "all"}, a.Explore) {
					return fmt.Errorf("%s: unknown exploration %q", where, a.Explore)
				}
				if (a.Entry != "") != (a.At != "") {
					return fmt.Errorf("%s: entry needs at, other attempts do not use at", where)
				}
				if db == nil {
					continue
				}
				if a.Item != "" {
					it := db.Items[a.Item]
					if it == nil || it.Event == "" || db.Events[it.Event] == nil {
						return fmt.Errorf("%s: item %q needs an event", where, a.Item)
					}
				}
				if a.Entry != "" {
					loc := db.Locations[a.At]
					found := 0
					if loc != nil {
						for _, x := range loc.Actions {
							if x.ID == a.Entry {
								found++
							}
						}
						for _, x := range loc.People {
							if x.ID == a.Entry {
								found++
							}
						}
					}
					if found != 1 {
						return fmt.Errorf("%s: entry %q at %q must resolve once", where, a.Entry, a.At)
					}
				}
			}
		}
	}
	return nil
}

func (b *bot) policies(phase string) bool {
	if b.policyBusy || b.run.Cleared || (phase == "step" && !b.inDungeon()) {
		return false
	}
	b.policyBusy = true
	defer func() { b.policyBusy = false }()
	acted := false
	for _, p := range b.p.Policies {
		if p.Phase != phase || (p.When.Set() && !b.holds(p.When)) {
			continue
		}
		key := fmt.Sprintf("%s|%d|%s|%d", p.ID, b.day, b.st.Location, b.floor())
		if b.policyFailed[key] {
			continue
		}
		n := b.rewardCounts[p.Reward]
		success := false
		for _, a := range p.Try {
			attempted := false
			b.with(p.Words, p.Rotate, p.Accept, func() { attempted = b.attempt(a) })
			acted = acted || attempted
			if b.run.Cleared || b.rewardCounts[p.Reward] > n || (p.When.Set() && !b.holds(p.When)) || (p.Reward == "" && attempted) {
				success = true
				break
			}
			if phase == "step" && !b.inDungeon() {
				break
			}
		}
		if !success {
			b.policyFailed[key] = true
		}
		if b.run.Cleared {
			break
		}
	}
	return acted
}

func (b *bot) attempt(a Attempt) bool {
	switch {
	case a.Tile != "":
		return b.inDungeon() && b.tileOnFloor(a.Tile) && b.walkChar(a.Tile, b.ctx)
	case a.Item != "":
		it := b.db.Items[a.Item]
		if it == nil || b.st.ItemCount(a.Item, false) == 0 || (it.EventOK.Set() && !b.holds(it.EventOK)) {
			return false
		}
		ev := b.db.Events[it.Event]
		if ev == nil {
			return false
		}
		b.adv(b.c.Time.Menu, b.where())
		b.act(func() error {
			err := b.d.Run(context.Background(), ev.Do)
			if err == nil && (it.Consume == nil || *it.Consume) {
				b.st.AddItem(a.Item, -1)
			}
			return err
		})
		return true
	case a.Entry != "":
		if !b.goTo(a.At) || b.run.Cleared {
			return false
		}
		for _, e := range world.Entries(b.db, b.st) {
			if e.Action != nil && e.Action.ID == a.Entry || e.Person != nil && e.Person.ID == a.Entry {
				b.adv(b.c.Time.Menu, b.where())
				b.act(func() error { return b.d.Run(context.Background(), e.Commands(b.db, b.st)) })
				return true
			}
		}
	case a.Explore != "":
		if !b.inDungeon() {
			return false
		}
		b.walkEvents(false)
		if a.Explore == "all" {
			b.walkUnseen()
		}
		return true
	}
	return false
}

func entryMatches(e world.Entry, selector string) bool {
	if id, ok := strings.CutPrefix(selector, "@"); ok {
		return e.Action != nil && e.Action.ID == id || e.Person != nil && e.Person.ID == id
	}
	return strings.Contains(e.Label, selector)
}

func entrySelector(e world.Entry) string {
	if e.Action != nil && e.Action.ID != "" {
		return "@" + e.Action.ID
	}
	if e.Person != nil && e.Person.ID != "" {
		return "@" + e.Person.ID
	}
	return e.Label
}

func (c *Config) checkChoiceIDs(db *data.DB) error {
	if db == nil {
		return nil
	}
	ids := map[string]bool{}
	db.EachCommand(func(cmd *data.Command) {
		for _, o := range cmd.Choice {
			if o.ID != "" {
				ids[o.ID] = true
			}
		}
	})
	words := append(slices.Clone(c.Game.OptIn), c.Game.Never...)
	words = append(words, c.Game.Prefer...)
	for _, ws := range [][]string{c.Game.DiveWords, c.Game.SleepWords, c.Game.StairsWords, c.Game.GateWords, c.Game.HomeWords} {
		words = append(words, ws...)
	}
	for _, p := range c.Personas {
		for _, ws := range [][]string{p.Prefer, p.Avoid, p.Rotate, p.Night} {
			words = append(words, ws...)
		}
		for _, r := range p.Policies {
			words = append(words, r.Words...)
			words = append(words, r.Rotate...)
		}
		for _, v := range p.Town {
			words = append(words, v.Words...)
			words = append(words, v.Rotate...)
			if id, ok := strings.CutPrefix(v.Do, "@"); ok {
				loc := db.Locations[v.At]
				found := 0
				if loc != nil {
					for _, a := range loc.Actions {
						if a.ID == id {
							found++
						}
					}
					for _, a := range loc.People {
						if a.ID == id {
							found++
						}
					}
				}
				if id == "" || found != 1 {
					return fmt.Errorf("persona %s: entry %q at %q must resolve once", p.ID, id, v.At)
				}
			}
		}
	}
	for _, w := range words {
		if id, ok := strings.CutPrefix(w, "@"); ok && !ids[id] {
			return fmt.Errorf("unknown choice id %q", id)
		}
	}
	return nil
}
