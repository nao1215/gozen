package play

import (
	"fmt"
	"image/color"
	"slices"
	"strings"

	"github.com/hajimehoshi/ebiten/v2"

	data "github.com/nao1215/gozen/internal/layer2/gamedata"
	"github.com/nao1215/gozen/internal/layer5/gfx"
)

// ranch is a menu for reserve exchange, release, and breeding. It only queues
// commands; the shared event runner owns every state transition and effect.
type ranch struct {
	step          string
	first, second string
	skills        []string
	options       []string
	list          *list
	done          func()
}

func newRanch(g *Game, done func()) *ranch {
	r := &ranch{done: done}
	r.show(g, "actions")
	return r
}
func (r *ranch) modal() bool { return true }

func (r *ranch) show(g *Game, step string) {
	r.step = step
	r.options = nil
	items := []listItem{}
	add := func(id, name string) {
		r.options = append(r.options, id)
		items = append(items, listItem{label: name, id: id})
	}
	switch step {
	case "actions":
		add("swap", "Swap party / reserve")
		add("breed", "Breed monsters")
		add("release", "Release monster")
	case "swap_in":
		for _, id := range g.st.ReserveIDs() {
			add(id, r.label(g, id))
		}
	case "swap_out":
		for _, id := range g.st.Party {
			add(id, r.label(g, id))
		}
	case "breed_first", "breed_second", "release_id":
		var ids []string
		for id, a := range g.st.Actors {
			if a.Monster == "" {
				continue
			}
			if step == "breed_second" && r.validSecond(g, id) || step == "breed_first" && r.hasPartner(g, id) || step == "release_id" {
				ids = append(ids, id)
			}
		}
		slices.Sort(ids)
		for _, id := range ids {
			add(id, r.label(g, id))
		}
	case "skills":
		a, b := g.st.Actors[r.first], g.st.Actors[r.second]
		if a != nil && b != nil {
			known := append(a.Skills(), b.Skills()...)
			slices.Sort(known)
			known = slices.Compact(known)
			for _, id := range known {
				name := id
				if skill := g.db.Skills[id]; skill != nil {
					name = skill.Name
				}
				if slices.Contains(r.skills, id) {
					name = "[x] " + name
				} else {
					name = "[ ] " + name
				}
				add(id, name)
			}
		}
		add("confirm", "Confirm fusion")
	}
	r.list = newList(items, 38, 108, 360, 10)
	r.list.size = sizeS
}

// hasPartner reports whether the monster can breed with any other owned monster.
func (r *ranch) hasPartner(g *Game, id string) bool {
	for other, a := range g.st.Actors {
		if a.Monster != "" && r.canPair(g, id, other) {
			return true
		}
	}
	return false
}

func (r *ranch) canPair(g *Game, first, second string) bool {
	a, b := g.st.Actors[first], g.st.Actors[second]
	return first != second && a != nil && b != nil &&
		(a.Sex == "male" && b.Sex == "female" || a.Sex == "female" && b.Sex == "male") &&
		g.db.BreedResult(a.Monster, b.Monster, a.Level, b.Level, a.Plus, b.Plus) != ""
}

func (r *ranch) validSecond(g *Game, id string) bool {
	a, b := g.st.Actors[r.first], g.st.Actors[id]
	return id != r.first && a != nil && b != nil &&
		(a.Sex == "male" && b.Sex == "female" || a.Sex == "female" && b.Sex == "male") &&
		g.db.BreedResult(a.Monster, b.Monster, a.Level, b.Level, a.Plus, b.Plus) != ""
}

func sexSign(sex string) string {
	switch sex {
	case "male":
		return " ♂"
	case "female":
		return " ♀"
	default:
		return ""
	}
}

func (r *ranch) label(g *Game, id string) string {
	a := g.st.Actors[id]
	if a == nil {
		return id
	}
	place := "reserve"
	if slices.Contains(g.st.Party, id) {
		place = "party"
	}
	return fmt.Sprintf("%s%s  Lv %d  +%d  (%s, %s)", a.Name(), sexSign(a.Sex), a.Level, a.Plus, id, place)
}

func (r *ranch) close(g *Game) {
	g.pop()
	if r.done != nil {
		r.done()
	}
}

func (r *ranch) queue(g *Game, cmd data.Command) {
	r.close(g)
	g.events.Start([]data.Command{cmd})
}

func (r *ranch) update(g *Game) {
	chosen, cancel := r.list.update(g)
	if cancel {
		switch r.step {
		case "actions":
			r.close(g)
		case "skills":
			r.show(g, "breed_second")
		default:
			r.show(g, "actions")
		}
		return
	}
	if !chosen {
		return
	}
	id := r.options[r.list.sel]
	switch r.step {
	case "actions":
		switch id {
		case "swap":
			r.show(g, "swap_in")
		case "breed":
			r.show(g, "breed_first")
		case "release":
			r.show(g, "release_id")
		}
	case "swap_in":
		r.first = id
		r.show(g, "swap_out")
	case "swap_out":
		in, out := r.first, id
		g.push(newConfirm("Swap "+r.label(g, in)+" with "+r.label(g, out)+"?", func() { r.queue(g, data.Command{Reserve: &data.ReserveSwap{In: in, Out: out}}) }))
	case "release_id":
		g.push(newConfirm("Release "+r.label(g, id)+" permanently?", func() { r.queue(g, data.Command{Release: id}) }))
	case "breed_first":
		r.first = id
		r.show(g, "breed_second")
	case "breed_second":
		a, b := g.st.Actors[r.first], g.st.Actors[id]
		if a == nil || b == nil || g.db.BreedResult(a.Monster, b.Monster, a.Level, b.Level, a.Plus, b.Plus) == "" {
			g.sfx("buzzer")
			return
		}
		r.second, r.skills = id, nil
		r.show(g, "skills")
	case "skills":
		if id == "confirm" {
			a, b := g.st.Actors[r.first], g.st.Actors[r.second]
			child := g.db.BreedResult(a.Monster, b.Monster, a.Level, b.Level, a.Plus, b.Plus)
			preview := g.childName(child)
			parents, skills := []string{r.first, r.second}, slices.Clone(r.skills)
			g.push(newConfirm("Fuse the parents into "+preview+"? They will leave the ranch.", func() { r.queue(g, data.Command{Breed: &data.BreedCmd{Parents: parents, Skills: skills}}) }))
			return
		}
		if slices.Contains(r.skills, id) {
			r.skills = slices.DeleteFunc(r.skills, func(s string) bool { return s == id })
		} else {
			if g.db.Breeding == nil || len(r.skills) >= g.db.Breeding.SkillSlots {
				g.sfx("buzzer")
				return
			}
			r.skills = append(r.skills, id)
		}
		sel := r.list.sel
		r.show(g, "skills")
		r.list.sel = min(sel, len(r.list.items)-1)
	}
}

// childName is the species name, or "?" until the monster book has seen it.
func (g *Game) childName(species string) string {
	if species == "" || !g.bestiarySeen(species) {
		return "?"
	}
	return g.db.Enemies[species].Name
}

// drawParents stands the chosen (or highlighted) parents on the right, where the location's
// picture shows through. The child is only a dark shape until the book has seen it.
func (r *ranch) drawParents(g *Game, dst *ebiten.Image) {
	sw, sh := g.screen()
	if sw < 1100 || r.step == "actions" {
		return
	}
	hovered := ""
	if len(r.options) > 0 && r.list.sel < len(r.options) {
		hovered = r.options[r.list.sel]
	}
	a, b := r.first, r.second
	switch r.step {
	case "breed_first", "release_id", "swap_in", "swap_out":
		a, b = hovered, ""
	case "breed_second":
		b = hovered
	}
	x0 := 880.0
	area := sw - x0 - 40
	for i, id := range []string{a, b} {
		inst := g.st.Actors[id]
		if inst == nil || inst.Monster == "" {
			continue
		}
		e := g.db.Enemies[inst.Monster]
		if e == nil {
			continue
		}
		drawBestiaryImage(g, dst, e.Image.File, x0+float64(i)*area/2+20, sh-640, area/2-40, 560)
	}
}

func (r *ranch) draw(g *Game, dst *ebiten.Image) {
	sw, sh := g.screen()
	fill(dst, color.NRGBA{0, 0, 0, 0x50})
	g.window(dst, 20, 20, min(sw-40, 820), sh-40)
	r.drawParents(g, dst)
	gfx.DrawTextOutlineColor(dst, "Monster Ranch", 48, 42, sizeL, g.th.accent, g.th.window, 1)
	r.list.draw(g, dst, true)
	x := 420.0
	y := 118.0
	lines := []string{fmt.Sprintf("Party: %d / %d", len(g.st.Party), g.db.Game.MaxParty), fmt.Sprintf("Reserve: %d", len(g.st.ReserveIDs()))}
	if a := g.st.Actors[r.first]; a != nil {
		lines = append(lines, "First: "+r.label(g, r.first))
	}
	if b := g.st.Actors[r.second]; b != nil {
		lines = append(lines, "Second: "+r.label(g, r.second))
	}
	if r.step == "skills" {
		a, b := g.st.Actors[r.first], g.st.Actors[r.second]
		child := g.db.BreedResult(a.Monster, b.Monster, a.Level, b.Level, a.Plus, b.Plus)
		lines = append(lines, "Child: "+g.childName(child), fmt.Sprintf("Inherited skills: %d / %d", len(r.skills), g.db.Breeding.SkillSlots))
	}
	if (r.step == "breed_first" || r.step == "breed_second") && len(r.options) == 0 {
		lines = append(lines, "No pair can breed. A pair needs one male and one female,", "the minimum level and a recipe for their species.")
	} else if r.step == "breed_first" || r.step == "breed_second" {
		hidden := 0
		for id, a := range g.st.Actors {
			if a.Monster != "" && !slices.Contains(r.options, id) && id != r.first {
				hidden++
			}
		}
		if hidden > 0 {
			lines = append(lines, fmt.Sprintf("%d hidden: no partner of the opposite sex with a recipe.", hidden))
		}
		if r.step == "breed_second" && r.list.sel < len(r.options) {
			if a, b := g.st.Actors[r.first], g.st.Actors[r.options[r.list.sel]]; a != nil && b != nil {
				lines = append(lines, "Child: "+g.childName(g.db.BreedResult(a.Monster, b.Monster, a.Level, b.Level, a.Plus, b.Plus)))
			}
		}
	}
	if len(r.options) > 0 && r.list.sel < len(r.options) {
		if a := g.st.Actors[r.options[r.list.sel]]; a != nil {
			family := "-"
			if e := g.db.Enemies[a.Monster]; e != nil {
				family = e.Family
			}
			lines = append(lines,
				"Family: "+family+"  Class: "+a.Def().Class,
				fmt.Sprintf("HP %d/%d  MP %d/%d", a.HP, a.Stat("mhp"), a.MP, a.Stat("mmp")),
				fmt.Sprintf("ATK %d  DEF %d  AGI %d", a.Stat("atk"), a.Stat("def"), a.Stat("agi")),
				"Skills: "+strings.Join(a.Skills(), ", "))
		}
	}
	for _, line := range lines {
		gfx.DrawText(dst, line, x, y, sizeS, g.th.text)
		y += 35
	}
	gfx.DrawText(dst, strings.ToUpper(strings.ReplaceAll(r.step, "_", " "))+"    Z: choose    X: back", 48, sh-52, sizeS, g.th.dim)
}

var (
	_ scene = (*ranch)(nil)
	_ modal = (*ranch)(nil)
)
