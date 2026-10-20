package battle

import (
	"errors"
	"slices"

	lang "github.com/nao1215/gozen/internal/text"

	data "github.com/nao1215/gozen/internal/layer2/gamedata"
	"github.com/nao1215/gozen/internal/rpg"
)

// UseInMenu uses a skill (of user) or an item (from the bag) outside battle, on target (for
// a one_* scope; the whole party for all_allies). It returns the messages to show.
func UseInMenu(db *data.DB, st *rpg.State, user *rpg.Actor, skill, item string, target *rpg.Actor) ([]string, error) {
	b := &Battle{DB: db, St: st, Troop: &data.Troop{}, Rand: st.Rand, fired: map[int]bool{}, Turn: 1, phase: "menu"}
	var uu, tu *Unit
	for i, a := range st.Members() {
		u := newActorUnit(b, a, i)
		b.Party = append(b.Party, u)
		if a == user {
			uu = u
		}
		if a == target {
			tu = u
		}
	}
	if uu == nil {
		uu = b.Party[0]
	}
	var use *data.Usage
	var name string
	if item != "" {
		it := db.Items[item]
		if it == nil || it.Use == nil || !slices.Contains(it.Use.Where, "menu") {
			return nil, errors.New(lang.T("ここでは つかえない"))
		}
		if st.Items[item] <= 0 {
			return nil, errors.New(lang.T("もっていない"))
		}
		use, name = it.Use, it.Name
	} else {
		s := db.Skills[skill]
		if s == nil || !slices.Contains(s.Where, "menu") {
			return nil, errors.New(lang.T("ここでは つかえない"))
		}
		if uu.MP < s.MP {
			return nil, errors.New(lang.T("MPが たりない"))
		}
		use, name = &s.Usage, s.Name
	}
	switch use.Scope {
	case "one_ally", "dead_ally", "self", "all_allies", "all_dead_allies":
	default:
		return nil, errors.New(lang.T("ここでは つかえない"))
	}
	if !fits(use.Scope, tu, b.Party) {
		return nil, errors.New(lang.T("その なかまには つかえない"))
	}
	// Decide the targets first, and if it works on nobody (healing only, with everyone at full), consume neither MP nor the item.
	targets := b.targets(uu, use.Scope, tu, use.Hits)
	if len(targets) > 0 && !slices.ContainsFunc(targets, func(t *Unit) bool { return !wasted(use, t) }) {
		return nil, errors.New(lang.T("つかっても こうかが ない"))
	}
	if item != "" {
		if db.Items[item].Consumed() {
			st.AddItem(item, -1)
		}
	} else {
		uu.MP -= db.Skills[skill].MP
		b.using = skill // make the expression's a.lv the same learned level as in battle
	}
	var msgs []string
	for _, t := range targets {
		for i := range use.Effects {
			for _, s := range b.effect(uu, t, &use.Effects[i], use, name) {
				msgs = append(msgs, s.Text)
			}
		}
	}
	if len(msgs) == 0 {
		msgs = append(msgs, lang.T("しかし なにも おこらなかった。"))
	}
	for _, u := range b.Party {
		a := u.Actor
		a.HP, a.MP = u.HP, u.MP
		clear(a.States) // remove vanished states, and keep the states that were applied (such as regen) with their remaining turns
		for s, n := range u.States {
			if n > 0 {
				a.States[s] = n
			}
		}
	}
	return msgs, nil
}

// Usable reports whether the skill or item can be used from the menu.
func Usable(db *data.DB, skill, item string) bool {
	var u *data.Usage
	if item != "" {
		if it := db.Items[item]; it != nil {
			u = it.Use
		}
	} else if s := db.Skills[skill]; s != nil {
		u = &s.Usage
	}
	return u != nil && slices.Contains(u.Where, "menu")
}

// NeedsTarget reports whether using it from the menu asks for one member.
func NeedsTarget(db *data.DB, skill, item string) bool {
	var u *data.Usage
	if item != "" {
		u = db.Items[item].Use
	} else {
		u = &db.Skills[skill].Usage
	}
	return u != nil && (u.Scope == "one_ally" || u.Scope == "dead_ally")
}

// CanTarget reports whether the member may be chosen as the target of the skill or item
// from the menu: a living one for one_ally, a fallen one for dead_ally.
func CanTarget(db *data.DB, skill, item string, a *rpg.Actor) bool {
	var u *data.Usage
	if item != "" {
		u = db.Items[item].Use
	} else {
		u = &db.Skills[skill].Usage
	}
	if u == nil || a == nil {
		return false
	}
	switch u.Scope {
	case "one_ally":
		return a.Alive()
	case "dead_ally":
		return !a.Alive()
	}
	return true
}

// fits reports whether the menu use reaches someone: the chosen one must be alive (one_ally)
// or fallen (dead_ally); a scope on the whole party needs someone it can reach. The
// battle's retargeting (to a random other member) is not used outside battle.
func fits(scope string, t *Unit, party []*Unit) bool {
	switch scope {
	case "one_ally":
		return t != nil && t.Alive()
	case "dead_ally":
		return t != nil && !t.Alive()
	case "all_allies":
		return len(living(party)) > 0
	case "all_dead_allies":
		return len(fallen(party)) > 0
	}
	return true
}

// wasted reports whether a use on the target would do nothing: only healing, on one whose
// HP (or MP) is full (a potion is not used up on a member in full health).
func wasted(use *data.Usage, t *Unit) bool {
	if len(use.Effects) == 0 || !t.Alive() {
		return false
	}
	for _, e := range use.Effects {
		if !e.Heal.Set() {
			return false
		}
		if e.Stat == "mp" {
			if t.MP < t.MMP() {
				return false
			}
		} else if t.HP < t.MHP() {
			return false
		}
	}
	return true
}
