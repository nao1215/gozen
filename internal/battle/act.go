package battle

import (
	"maps"
	"math"
	"slices"

	lang "github.com/nao1215/gozen/internal/text"

	"github.com/nao1215/gozen/internal/expr"
	data "github.com/nao1215/gozen/internal/layer2/gamedata"
)

// act plays one action.
func (b *Battle) act(a Action) []Step {
	u := a.User
	if a.Pass {
		return nil
	}
	var steps []Step
	r, sid := u.restrict()
	if r == "" {
		r, sid = u.restrictByChance()
	}
	switch {
	case r == "skip":
		st := b.DB.States[sid]
		msg := st.Messages.Stay
		if msg == "" {
			msg = lang.T("{target}は うごけない！")
		}
		return []Step{{Kind: Message, Text: b.text(msg, nil, u, "", 0), Target: u}}
	case r == "confuse":
		st := b.DB.States[sid]
		if st.Messages.Stay != "" {
			steps = append(steps, Step{Kind: Message, Text: b.text(st.Messages.Stay, nil, u, "", 0), Target: u})
		}
		everyone := living(append(append([]*Unit{}, b.Party...), b.Foes...))
		a = Action{User: u, Skill: b.DB.Game.Battle.Attack, Target: everyone[b.Rand.IntN(len(everyone))]}
	case a.Skill == "" && a.Item == "":
		a = b.decide(u, DecisionAction)
	}
	if a.Skill == b.DB.Game.Battle.Guard {
		u.Guard = true
	}
	use := b.usage(&a)
	if use == nil {
		return append(steps, Step{Kind: Message, Text: u.Name + lang.T("は ようすを みている。"), User: u})
	}
	name := ""
	if a.Item != "" {
		if b.St.Items[a.Item] <= 0 || u.Enemy {
			return append(steps, Step{Kind: Message, Text: u.Name + lang.T("は ") + b.DB.Items[a.Item].Name + lang.T("を もっていない！"), User: u})
		}
		it := b.DB.Items[a.Item]
		name = it.Name
		if it.Consumed() {
			b.St.AddItem(a.Item, -1)
		}
	} else {
		s := b.DB.Skills[a.Skill]
		name = s.Name
		if u.MP < s.MP {
			return append(steps, Step{Kind: Message, Text: u.Name + lang.T("は ") + name + lang.T("を となえようとしたが MPが たりない！"), User: u})
		}
		u.MP -= s.MP
	}
	if a.tactic != nil {
		b.observeTactics(u, a.tactic.results, a.tactic.selected)
	}
	if a.Say != "" {
		steps = append(steps, Step{Kind: Message, Text: b.text(a.Say, u, a.Target, name, 0), User: u})
	}
	msg := use.Message
	if msg == "" {
		msg = lang.T("{user}の {skill}！")
		if a.Item != "" {
			msg = lang.T("{user}は {skill}を つかった！")
		}
	}
	actIndex := len(steps)
	steps = append(steps, Step{Kind: Act, User: u, Skill: a.Skill + a.Item, Text: b.text(msg, u, a.Target, name, 0), Anim: use.Anim, Color: use.Color, HP: u.HP, MP: u.MP})
	attack := a.Skill == b.DB.Game.Battle.Attack
	b.using = a.Skill
	defer func() { b.using = "" }()
	physical := (use.Anim == "slash" || attack) && (use.Scope == "one_enemy" || use.Scope == "random_enemy")
	var countered []*Unit
	targets := b.targets(u, use.Scope, a.Target, use.Hits)
	var reached []*Unit
	if a.tactic != nil {
		defer func() { steps[actIndex].Text += b.tacticLog(a, reached, name) }()
	}
	for _, t := range targets {
		if use.Scope != "dead_ally" && use.Scope != "all_dead_allies" && !t.Alive() {
			continue
		}
		if physical && !contains(u.allies(), t) { // someone may take it for t
			if c, msg := b.coverer(t); c != nil {
				steps = append(steps, Step{Kind: Message, Text: b.text(msg, t, c, name, 0), User: c, Target: t})
				t = c
			}
		}
		reached = append(reached, t)
		if physical && !b.countering && t.Alive() && !contains(countered, t) {
			countered = append(countered, t) // after the action: maybe a counter
		}
		if use.Hit.Set() {
			p, err := use.Hit.Number(b.effEnv(u, t))
			if err == nil && b.Rand.Float64() >= p {
				steps = append(steps, Step{Kind: Miss, User: u, Target: t, Text: t.Name + lang.T("は ひらりと みをかわした！"), HP: t.HP, MP: t.MP})
				continue
			}
		}
		crit := b.critical(u, t, use)
		for i := range use.Effects {
			steps = append(steps, b.effect(u, t, &use.Effects[i], use, name, critFlag(crit))...)
			if b.Result != Ongoing {
				return steps
			}
		}
		if attack && t.Alive() {
			for _, g := range u.weaponStates() {
				steps = append(steps, b.addState(u, t, g.State, g.Chance, data.Expr{}, true)...)
			}
		}
	}
	for _, t := range countered {
		if b.Result == Ongoing && t.Alive() && u.Alive() {
			steps = append(steps, b.counter(t, u)...)
		}
	}
	if attack && a.Item == "" && u.Alive() && b.Result == Ongoing && !b.countering && u.again < 3 && b.rollAgain(u) {
		// state again: after an ordinary attack, once more (up to 3 times in a row)
		if foes := living(u.foes()); len(foes) > 0 {
			u.again++
			steps = append(steps, b.act(Action{User: u, Skill: a.Skill, Target: foes[b.Rand.IntN(len(foes))]})...)
			u.again--
		}
	}
	if use.Sacrifice && u.Alive() && b.Result == Ongoing { // blown up with it
		u.HP = 0
		steps = append(steps, b.fall(u))
		b.judge()
	}
	return steps
}

// coverer is an ally of t who takes an attack for it (a state with cover that holds), and
// the message.
func (b *Battle) coverer(t *Unit) (*Unit, string) {
	for _, c := range living(t.allies()) {
		if c == t || c.restricted() {
			continue
		}
		for _, sid := range c.sortedStates() {
			st := b.DB.States[sid]
			if st == nil || !st.Cover.Set() {
				continue
			}
			if ok, err := st.Cover.Bool(b.effEnv(c, t)); err == nil && ok {
				msg := st.Messages.Cover
				if msg == "" {
					msg = lang.T("{target}が {user}を かばった！")
				}
				return c, msg
			}
		}
	}
	return nil, ""
}

// counter is t striking back at u with the attack skill (a state with counter, its chance drawn).
func (b *Battle) counter(t, u *Unit) []Step {
	if t.restricted() {
		return nil
	}
	for _, sid := range t.sortedStates() {
		st := b.DB.States[sid]
		if st == nil || !st.Counter.Set() {
			continue
		}
		p, err := st.Counter.Number(b.effEnv(t, u))
		if err != nil || b.Rand.Float64() >= p {
			continue
		}
		msg := st.Messages.Counter
		if msg == "" {
			msg = lang.T("{target}の カウンター！")
		}
		b.countering = true
		steps := []Step{{Kind: Message, Text: b.text(msg, u, t, "", 0), User: t}}
		steps = append(steps, b.act(Action{User: t, Skill: b.DB.Game.Battle.Attack, Target: u})...)
		b.countering = false
		return steps
	}
	return nil
}

// restrictByChance rolls for a state that only sometimes stops an action (restrict_chance), once per action.
func (u *Unit) restrictByChance() (string, string) {
	for _, s := range u.sortedStates() {
		st := u.b.DB.States[s]
		if st.RestrictChance > 0 && st.Restrict == "skip" && u.b.Rand.Float64() < st.RestrictChance {
			return "skip", s
		}
	}
	return "", ""
}

// restricted reports whether a state keeps the unit from acting.
func (u *Unit) restricted() bool { r, _ := u.restrict(); return r != "" }

type critFlag bool

func (b *Battle) critical(u, t *Unit, use *data.Usage) bool {
	e := use.Crit
	if !e.Set() {
		e = b.DB.Game.Battle.Critical
	}
	if !e.Set() {
		return false
	}
	p, err := e.Number(b.effEnv(u, t))
	return err == nil && b.Rand.Float64() < p
}

// targets are the units a scope reaches: chosen (or a random one when chosen is gone) for
// a one_* scope, everyone for all_*, hits random ones for random_enemy.
func (b *Battle) targets(u *Unit, scope string, chosen *Unit, hits int) []*Unit {
	pickOne := func(pool []*Unit) []*Unit {
		if len(pool) == 0 {
			return nil
		}
		if chosen != nil && contains(pool, chosen) {
			return []*Unit{chosen}
		}
		if u.Enemy { // the foes strike at the front row more
			return []*Unit{b.frontFirst(pool)}
		}
		return []*Unit{pool[b.Rand.IntN(len(pool))]}
	}
	var out []*Unit
	switch scope {
	case "one_enemy":
		if chosen != nil && chosen.Alive() && !contains(u.foes(), chosen) && !contains(u.allies(), chosen) {
			chosen = nil
		}
		if chosen != nil && chosen.Alive() {
			out = []*Unit{chosen} // a confused unit may hit its own side
		} else {
			out = pickOne(living(u.foes()))
		}
	case "all_enemies":
		out = living(u.foes())
	case "random_enemy":
		for range hits {
			if l := living(u.foes()); len(l) > 0 {
				out = append(out, l[b.Rand.IntN(len(l))])
			}
		}
		return out
	case "one_ally":
		out = pickOne(living(u.allies()))
	case "all_allies":
		out = living(u.allies())
	case "self":
		out = []*Unit{u}
	case "dead_ally":
		out = pickOne(fallen(u.allies()))
	case "all_dead_allies":
		out = fallen(u.allies())
	case "everyone":
		out = living(append(append([]*Unit{}, u.allies()...), u.foes()...))
	}
	var rep []*Unit
	for _, t := range out {
		for range hits {
			rep = append(rep, t)
		}
	}
	return rep
}

func contains(us []*Unit, u *Unit) bool {
	return slices.Contains(us, u)
}

// effEnv is the scope of an effect: a (the user), b (the target) and the battle's names.
func (b *Battle) effEnv(u, t *Unit) expr.Env {
	return expr.Chain{expr.Map{"a": u, "b": t}, b.Env(u)}
}

// effect applies one effect of a skill, an item or a state to t.
func (b *Battle) effect(u, t *Unit, e *data.Effect, use *data.Usage, skill string, crit ...critFlag) []Step {
	env := b.effEnv(u, t)
	if ok, err := e.When.Bool(env); err != nil || !ok {
		return nil
	}
	if e.OnUser { // what is taken from the target goes to the user (b stays the target)
		t = u
	}
	stat := e.Stat
	if stat == "" {
		stat = "hp"
	}
	variance := b.DB.Game.Battle.Variance
	if e.Variance != nil {
		variance = *e.Variance
	}
	vary := func(v float64) float64 { return v * (1 + variance*(2*b.Rand.Float64()-1)) }
	isCrit := len(crit) > 0 && bool(crit[0])
	var steps []Step
	switch {
	case e.Damage.Set():
		if !t.Alive() {
			return nil
		}
		v, err := e.Damage.Number(env)
		if err != nil {
			return []Step{{Kind: Message, Text: lang.T("(式のエラー: ") + err.Error() + ")"}}
		}
		v = vary(v)
		el := e.Element
		if el == "weapon" || (el == "" && skill != "" && u.weaponElement() != "" && use != nil && b.isAttack(use)) {
			el = u.weaponElement()
		}
		weak, resisted := false, false
		if el != "" && t.Def != nil {
			if r, ok := t.Def.Resist[el]; ok {
				v *= r
				weak = r > 1
				resisted = r < 0.7
			}
		}
		if isCrit {
			v *= b.DB.Game.Battle.CriticalRate
		}
		if t.Def != nil && t.Def.Hard > 0 && !isCrit && use != nil && b.isAttack(use) {
			v = min(v, float64(t.Def.Hard)) // a hard shell: the plain attack only scratches it
		}
		for st := range t.States {
			if d := b.DB.States[st]; d != nil && (d.DamageRate > 0 || b.DB.Game.SchemaVersion >= data.SchemaVersion1) {
				v *= d.DamageRate
			}
		}
		if t.Guard {
			v *= b.DB.Game.Battle.GuardRate
		}
		n := max(0, int(math.Round(v)))
		var lost int // Actual amount removed, used for drain.
		endured := false
		if stat == "mp" {
			n = min(n, t.MP)
			lost = n
			t.MP -= n
		} else {
			lost = min(n, t.HP)
			t.HP = max(0, t.HP-n)
			if t.HP == 0 && b.endure(t) { // survive a fatal blow with 1 HP (state endure)
				t.HP, lost = 1, lost-1
				endured = true
			}
		}
		text := e.Message
		switch {
		case text != "":
		case n == 0:
			text = lang.T("{target}に ダメージを あたえられない！")
		case t.Enemy:
			text = lang.T("{target}に {amount}の ダメージ！")
		default:
			text = lang.T("{target}は {amount}の ダメージを うけた！")
		}
		if stat == "mp" && e.Message == "" {
			text = lang.T("{target}の MPが {amount} へった！")
		}
		if isCrit {
			text = lang.T("かいしんの いちげき！ ") + text
		}
		steps = append(steps, Step{Kind: Damage, User: u, Target: t, Amount: n, Stat: stat, Crit: isCrit, Weak: weak, Resisted: resisted, Element: el, Text: b.text(text, u, t, skill, n), HP: t.HP, MP: t.MP})
		if endured {
			steps = append(steps, Step{Kind: Message, Target: t, Text: b.text(lang.T("{target}は ふみとどまった！"), u, t, skill, 0), HP: t.HP, MP: t.MP})
		}
		if e.Drain > 0 && lost > 0 {
			// Absorption is computed from the amount actually reduced, and the display shows the amount actually increased (the part capped by the maximum is not counted).
			g := int(math.Round(float64(lost) * e.Drain))
			if stat == "mp" {
				before := u.MP
				u.MP = min(u.MMP(), u.MP+g)
				g = u.MP - before
			} else {
				before := u.HP
				u.HP = min(u.MHP(), u.HP+g)
				g = u.HP - before
			}
			if g > 0 { // nothing taken (full already): no line of 0
				steps = append(steps, Step{Kind: Heal, User: u, Target: u, Amount: g, Stat: stat, Text: b.text(lang.T("{user}は {amount} すいとった！"), u, u, skill, g), HP: u.HP, MP: u.MP})
			}
		}
		if e.Recoil > 0 && lost > 0 && u.HP > 1 {
			// recoil: also reduce the user's HP by a fraction of the damage dealt (does not knock them out)
			r := min(u.HP-1, int(math.Round(float64(lost)*e.Recoil)))
			u.HP -= r
			steps = append(steps, Step{Kind: Damage, User: u, Target: u, Amount: r, Text: b.text(lang.T("{user}は 反動で {amount}の ダメージを うけた！"), u, u, skill, r), HP: u.HP, MP: u.MP})
		}
		if t.HP == 0 {
			steps = append(steps, b.fall(t))
		} else if n > 0 {
			for _, s := range t.sortedStates() {
				st := b.DB.States[s]
				if st.Wake > 0 && b.Rand.Float64() < st.Wake {
					delete(t.States, s)
					steps = append(steps, b.stateOff(t, s))
				}
			}
		}
	case e.Heal.Set():
		if !t.Alive() {
			return nil
		}
		v, err := e.Heal.Number(env)
		if err != nil {
			return []Step{{Kind: Message, Text: lang.T("(式のエラー: ") + err.Error() + ")"}}
		}
		n := max(0, int(math.Round(vary(v))))
		if stat == "mp" {
			n = min(n, t.MMP()-t.MP)
			t.MP += n
		} else {
			n = min(n, t.MHP()-t.HP)
			t.HP += n
		}
		text := e.Message
		if text == "" {
			text = lang.T("{target}の HPが {amount} かいふくした！")
			if stat == "mp" {
				text = lang.T("{target}の MPが {amount} かいふくした！")
			}
		}
		if e.Quiet { // no text (only light and numbers). If full and nothing is restored, show nothing
			if n == 0 {
				return nil
			}
			return []Step{{Kind: Heal, User: u, Target: t, Amount: n, Stat: stat, HP: t.HP, MP: t.MP}}
		}
		steps = append(steps, Step{Kind: Heal, User: u, Target: t, Amount: n, Stat: stat, Text: b.text(text, u, t, skill, n), HP: t.HP, MP: t.MP})
	case e.AddState != "":
		steps = append(steps, b.addState(u, t, e.AddState, e.Chance, e.Turns, use != nil && hasDamage(use))...)
	case e.Remove != "":
		for _, s := range t.sortedStates() {
			if s == e.Remove || (e.Remove == "bad" && b.DB.States[s].Bad) {
				delete(t.States, s)
				off := b.stateOff(t, s)
				off.User = u // removed by a skill (distinguished from expiry)
				steps = append(steps, off)
			}
		}
	case e.Revive.Set():
		if t.Alive() || t.Fled {
			return nil
		}
		v, err := e.Revive.Number(env)
		if err != nil {
			return []Step{{Kind: Message, Text: lang.T("(式のエラー: ") + err.Error() + ")"}}
		}
		t.HP = max(1, min(t.MHP(), int(math.Round(v))))
		text := e.Message
		if text == "" {
			text = lang.T("{target}は いきかえった！")
		}
		steps = append(steps, Step{Kind: Revive, User: u, Target: t, Amount: t.HP, Text: b.text(text, u, t, skill, t.HP), HP: t.HP, MP: t.MP})
	case e.Summon != "":
		return b.summon(u, e)
	case e.Escape:
		if u.Enemy {
			u.Fled = true
			steps = append(steps, Step{Kind: Fall, Target: u, Text: u.Name + lang.T("は にげだした！"), HP: u.HP})
			b.judge()
			return steps
		}
		if !b.Troop.Escapable() {
			return []Step{{Kind: Message, Text: lang.T("しかし にげられない！")}}
		}
		b.Result = Fled
		return []Step{{Kind: Escape, Text: lang.T("うまく にげきれた！")}}
	}
	return steps
}

func (b *Battle) isAttack(use *data.Usage) bool {
	s := b.DB.Skills[b.DB.Game.Battle.Attack]
	return s != nil && &s.Usage == use
}

func (b *Battle) fall(t *Unit) Step {
	t.fellWith = maps.Clone(t.States)
	clear(t.States)
	t.Guard = false
	text := lang.T("{target}を たおした！")
	if !t.Enemy {
		text = lang.T("{target}は ちからつきた…")
	}
	return Step{Kind: Fall, Target: t, Text: b.text(text, nil, t, "", 0), HP: 0, MP: t.MP}
}

func hasDamage(use *data.Usage) bool {
	for _, e := range use.Effects {
		if e.Damage.Set() {
			return true
		}
	}
	return false
}

// addState puts state s on t with the chance (1 when not given) times t's resistance. A
// failure says so unless quiet (a state riding on a hit fails without a word).
func (b *Battle) addState(u, t *Unit, s string, chance, length data.Expr, quiet bool) []Step {
	if !t.Alive() {
		return nil
	}
	st := b.DB.States[s]
	p := 1.0
	if chance.Set() {
		v, err := chance.Number(b.effEnv(u, t))
		if err != nil {
			return nil
		}
		p = v
	}
	p *= t.stateResist(s)
	if b.Rand.Float64() >= p {
		if quiet {
			return nil
		}
		return []Step{{Kind: Miss, Target: t, Text: t.Name + lang.T("には きかなかった！"), HP: t.HP, MP: t.MP}}
	}
	if st.Fatal { // an instant death: the state takes the life, and is not kept
		msg := st.Messages.Add
		if msg == "" {
			msg = lang.T("{target}は いのちを うばわれた！")
		}
		t.HP = 0
		steps := []Step{{Kind: StateOn, User: u, Target: t, State: s, Text: b.text(msg, u, t, "", 0), HP: t.HP, MP: t.MP, Color: st.Color}}
		steps = append(steps, b.fall(t))
		b.judge()
		return steps
	}
	turns := st.Turns
	if length.Set() { // the skill's turns (an expression; a.lv can lengthen it with learned level)
		if v, err := length.Number(b.effEnv(u, t)); err == nil {
			turns = max(1, int(math.Round(v)))
		}
	}
	if turns <= 0 {
		turns = stateForever
	}
	t.States[s] = turns
	msg := st.Messages.Add
	if msg == "" {
		msg = lang.T("{target}は ") + st.Name + lang.T("に なった！")
	}
	return []Step{{Kind: StateOn, User: u, Target: t, State: s, Text: b.text(msg, u, t, "", 0), HP: t.HP, MP: t.MP, Color: st.Color}}
}

// summon calls the enemy e.Summon to the foes' side (only foes summon): in the place of a
// fallen one, or at the end while fewer than the most stand.
func (b *Battle) summon(u *Unit, e *data.Effect) []Step {
	if !u.Enemy {
		return nil
	}
	most := e.SummonMax
	if most <= 0 {
		most = 5
	}
	alive, slot := 0, -1
	for i, f := range b.Foes {
		if f.Alive() && !f.Fled {
			alive++
		} else if slot < 0 && !f.Def.Boss {
			slot = i
		}
	}
	if alive >= most {
		return []Step{{Kind: Message, Text: lang.T("しかし なにも あらわれなかった。")}}
	}
	var n *Unit
	if slot >= 0 {
		n = newEnemyUnit(b, e.Summon, slot)
		b.gone = append(b.gone, b.Foes[slot]) // its kill and its rewards stay
		b.Foes[slot] = n
	} else {
		n = newEnemyUnit(b, e.Summon, len(b.Foes))
		b.Foes = append(b.Foes, n)
	}
	// the letters among the standing ones of its kind: the first gets A once a second comes
	used := map[rune]bool{}
	var bare *Unit
	for _, f := range b.Foes {
		if f == n || f.EID != e.Summon || !f.Alive() {
			continue
		}
		if f.Name == f.Def.Name {
			bare = f
		} else if r := []rune(f.Name); len(r) > 0 {
			used[r[len(r)-1]] = true
		}
	}
	next := func() string {
		for c := 'A'; c <= 'Z'; c++ {
			if !used[c] {
				used[c] = true
				return string(c)
			}
		}
		return ""
	}
	if bare != nil {
		bare.Name += next()
	}
	if bare != nil || len(used) > 0 {
		n.Name += next()
	}
	text := e.Message
	if text == "" {
		text = lang.T("{target}が あらわれた！")
	}
	return []Step{{Kind: Summon, User: u, Target: n, Text: b.text(text, u, n, "", 0), HP: n.HP, MP: n.MP}}
}

// endure reports whether t can survive a fatal blow (uses one charge if the state's endure count remains).
func (b *Battle) endure(t *Unit) bool {
	for _, sid := range t.sortedStates() {
		st := b.DB.States[sid]
		if st == nil || !st.Endure.Set() {
			continue
		}
		n, err := st.Endure.Number(b.effEnv(t, t))
		if err != nil || float64(t.endured[sid]) >= n {
			continue
		}
		if t.endured == nil {
			t.endured = map[string]int{}
		}
		t.endured[sid]++
		return true
	}
	return false
}

// rollAgain reports whether to attack once more, with the probability of the state's again.
func (b *Battle) rollAgain(u *Unit) bool {
	for _, sid := range u.sortedStates() {
		st := b.DB.States[sid]
		if st == nil || !st.Again.Set() {
			continue
		}
		if p, err := st.Again.Number(b.effEnv(u, u)); err == nil && p > 0 && b.Rand.Float64() < p { // if 0, do not draw a random number
			return true
		}
	}
	return false
}
