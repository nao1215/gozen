package play

import (
	"fmt"
	"slices"
	"strings"

	lang "github.com/nao1215/gozen/internal/base/text"
	"github.com/nao1215/gozen/internal/base/traceformat"
	data "github.com/nao1215/gozen/internal/model/gamedata"
)

// A script's pick and look commands let a program play through the ordinary
// screens by what they show: look writes what can be acted on now, and pick
// selects the visible control whose label contains a text and confirms it with
// the ordinary OK input, as a player moving the cursor would. Nothing reached
// through them bypasses an entry's condition or a screen's layout.

// pickable is one control that pick can reach: its label, whether it is
// disabled, and how to select it.
type pickable struct {
	label    string
	detail   string
	picture  string // the bust picture and dot shown on the control, as the player sees them
	disabled bool
	selectIt func()
	target   any
	focus    int // keyboard stop for an authored surface; independent of auxiliary controls
}

// scriptPickables lists the controls of what is on top: a phone page's
// buttons on the screen, a confirmation, the choices of a question, an open popup, a screen's actions and seats, or a list
// menu. kind names which of them it is.
func (g *Game) scriptPickables() (kind string, items []pickable) {
	if g.phone != nil && g.phone.child != nil {
		child := g.phone.child.game
		defer child.activatePresentation()()
		return child.scriptPickables()
	}
	if p := g.phone; p != nil && p.ready() && p.watch == nil && p.viewPic == "" && slices.Contains([]string{"page", "home", "contacts"}, p.cue.App) {
		return "phone:" + p.cue.App, g.pagePickables(p)
	}
	if len(g.stack) == 0 {
		return "", nil
	}
	top := g.stack[len(g.stack)-1]
	if c, ok := top.(*confirm); ok {
		return "confirm", []pickable{
			{target: c, label: lang.T("はい"), selectIt: func() { c.no = false }},
			{target: c, label: lang.T("いいえ"), selectIt: func() { c.no = true }},
		}
	}
	if g.talk.active {
		if l := g.talk.choice; l != nil {
			return "choice", listPickables(l)
		}
		return "talk", nil
	}
	switch v := top.(type) {
	case *authorScreen:
		if v.running {
			return "busy", nil
		}
		return "screen:" + v.name, v.pickables(g)
	case *sysMenu:
		return "menu", listPickables(v.l)
	case *saveMenu:
		return "save", listPickables(v.slots)
	case *title:
		return "title", listPickables(v.menu)
	}
	return "", nil
}

func listPickables(l *list) []pickable {
	var out []pickable
	for i, it := range l.items {
		if it.heading {
			continue
		}
		out = append(out, pickable{target: l, label: it.label, disabled: it.disabled, selectIt: func() { l.sel = i }})
	}
	return out
}

// pickables are the popup's rows while it is open, else the surface's direct
// actions and seats (a seat's label is "@" and its entity ID), else the list.
func (s *authorScreen) pickables(g *Game) []pickable {
	if s.popup != nil {
		return listPickables(s.popup)
	}
	def := g.db.Game.Screens[s.name]
	if def == nil || !def.View.Fullscreen {
		return listPickables(s.list)
	}
	env := s.env(g)
	var out []pickable
	actions := s.surfaceActions()
	for j, i := range actions {
		label := data.Render(g.db.Tr(s.entries[i].Label), s.rows[i].Env(env))
		disabled := s.unavailable(g, i)
		if s.entries[i].Drop != "" {
			// A drop place is pressed only while something it takes is carried.
			disabled = s.carry == nil || !s.carry.ok[i]
		}
		var picture string
		if b := s.entries[i].Bust; b != nil {
			picture = b.Picture
		}
		if d := s.entries[i].Dot; d != "" {
			picture = strings.TrimSpace(picture + " " + d)
		}
		out = append(out, pickable{target: s, focus: j, label: label, detail: s.entries[i].Text, picture: picture, disabled: disabled, selectIt: func() { s.focus = j; s.syncRing(i) }})
	}
	if g.inboxSurface() == s {
		_, name := g.actorFacePath(g.st.Inbox[0].Who)
		out = append(out, pickable{target: &s.inboxArmed, label: lang.T("通知") + ": " + name, detail: g.st.Inbox[0].Text, selectIt: func() { s.inboxArmed = true }})
	}
	ids, _ := s.surfaceTargets(g)
	if def.Kind == "entities" && def.View.Detail != "" {
		ids = s.tableResidents(g)
	}
	for k, id := range ids {
		out = append(out, pickable{target: s, focus: len(actions) + k, label: "@" + id, selectIt: func() { s.focus = len(actions) + k }})
	}
	rowCount := len(ids)
	if def.Kind == "entities" {
		rowCount = len(s.tableResidents(g))
	}
	for k, st := range s.flowStops(g) {
		out = append(out, pickable{target: s, focus: len(actions) + rowCount + k, label: st.label, disabled: !st.enabled, selectIt: func() { s.focus = len(actions) + rowCount + k }})
	}
	return out
}

// scriptPick selects the first enabled control whose label contains text and
// presses OK. A label equal to text wins over one that only contains it.
func (g *Game) scriptPick(in *input, text string) error {
	kind, items := g.scriptPickables()
	if o := in.offer; o != nil {
		receives := false
		for _, it := range items {
			receives = receives || it.target == o.target
		}
		if !receives {
			return nil
		}
	}
	found := -1
	for i, it := range items {
		if it.disabled {
			continue
		}
		if it.label == text {
			found = i
			break
		}
		if found < 0 && strings.Contains(it.label, text) {
			found = i
		}
	}
	if found < 0 {
		labels := make([]string, 0, len(items))
		for _, it := range items {
			labels = append(labels, it.label)
		}
		return fmt.Errorf("pick %q: no enabled control on %q among %q", text, kind, labels)
	}
	if surface, ok := items[found].target.(*authorScreen); ok && g.guideOn(surface) {
		if step := g.guideStep(surface); step != nil {
			// Use the keyboard stop, excluding auxiliary controls such as the inbox.
			if !slices.Contains(surface.guideStops(g, step), items[found].focus) {
				return nil
			}
		}
	}
	if in.offer != nil {
		if items[found].target != in.offer.target {
			return nil
		}
		in.offer.accepted = true
	}
	g.selectPick(kind, items, found)
	in.pressed[btnOK] = true
	return nil
}

// scriptLook writes what can be acted on now as a look trace line.
func (g *Game) scriptLook() {
	kind, items := g.scriptPickables()
	labels := make([]string, 0, len(items))
	disabled := make([]string, 0)
	details := map[string]string{}
	pictures := map[string]string{}
	for _, it := range items {
		if it.disabled {
			disabled = append(disabled, it.label)
			continue
		}
		labels = append(labels, it.label)
		if it.detail != "" {
			details[it.label] = it.detail
		}
		if it.picture != "" {
			pictures[it.label] = it.picture
		}
	}
	depth := 0
	for _, view := range g.stack {
		if _, ok := view.(*authorScreen); ok {
			depth++
		}
	}
	line := traceLine{"on": kind, "controls": labels, "depth": depth}
	if len(details) > 0 {
		line["details"] = details
	}
	if len(disabled) > 0 {
		line["disabled"] = disabled
	}
	if len(pictures) > 0 {
		line["pictures"] = pictures
	}
	if g.talk.active {
		line["who"], line["text"] = g.talk.say.Name, g.talk.say.Text
	}
	if g.st != nil && len(g.st.Inbox) > 0 {
		line["inbox"] = len(g.st.Inbox)
	}
	if m := g.inboxShown; m != nil {
		line["message"] = traceLine{"who": m.Who, "text": m.Text}
	}
	if len(g.stack) > 0 {
		if s, ok := g.stack[len(g.stack)-1].(*authorScreen); ok {
			s.lookParts(g, line)
		}
	}
	if g.st != nil && g.st.Planning != nil {
		line["date"], line["phase"] = g.st.Planning.Date, g.st.Planning.Phase
	}
	g.emit("look", line)
}

// lookParts adds what a surface carries and the open peek to a look line.
func (s *authorScreen) lookParts(g *Game, line traceLine) {
	label := func(i int) string {
		return data.Render(g.db.Tr(s.entries[i].Label), s.rows[i].Env(s.env(g)))
	}
	if c := s.carry; c != nil && c.entry < len(s.entries) {
		line["held"] = label(c.entry)
	}
	if k := s.peek; k != nil && k.entry < len(s.entries) {
		line["peek"] = s.entries[k.entry].Peek
	}
	if flow, step := g.db.FlowOf(s.name); flow != "" {
		line["flow"], line["step"], line["reached"] = flow, step+1, g.flowReached(flow)+1
	}
}

// Observations wait for a tick in which the foreground queried input without
// receiving an action. They do not predict any screen's animation or event gates.
func (g *Game) scriptReady(verb string) bool {
	// Run-only date cards must never receive commands intended for the screen.
	if g.turn != nil {
		return false
	}
	if verb == "look" || verb == "snapshot" {
		return g.in.idleRead
	}
	return true
}

// selectPick records the resolved control, including its index to disambiguate
// duplicate labels. Replay invokes it only when that same receiver is reached.
func (g *Game) selectPick(kind string, items []pickable, index int) {
	items[index].selectIt()
	if g.tr != nil {
		g.emit("pick", traceLine{"on": kind, "index": index, "label": items[index].label})
	}
}

func (g *Game) replayPick(target any) {
	r := g.rep
	line := r.picks[g.tick]
	if line == nil || r.pickTick == g.tick {
		return
	}
	kind, items := g.scriptPickables()
	index := int(traceformat.Number(line["index"]))
	if kind != line["on"] || index < 0 || index >= len(items) || items[index].label != line["label"] || items[index].disabled {
		g.failure = fmt.Errorf("replay: pick target changed at tick %d", g.tick)
		return
	}
	if items[index].target != target {
		return
	}
	r.pickTick = g.tick
	g.selectPick(kind, items, index)
	g.in.pressed[btnOK] = true
}
