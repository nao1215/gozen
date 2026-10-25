package play

import (
	"errors"
	"fmt"
	"image/color"
	"maps"
	"slices"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/inpututil"

	"github.com/nao1215/gozen/internal/event"
	data "github.com/nao1215/gozen/internal/layer2/gamedata"
	"github.com/nao1215/gozen/internal/layer5/gfx"
	lang "github.com/nao1215/gozen/internal/text"
)

// The development panel (F8 under --dev, ADR 0029): the state, a table of the flags the
// events set and read, the events to jump to, a line of YAML to run, and the frame meter.
// Nothing in it changes the game but set (dev_set) and the jump to an event (jump), the
// same paths as Session.Do; the script has jump and do for the same work.

type panelTab int

const (
	tabState panelTab = iota
	tabFlags
	tabScenes
	tabCommand
	tabFrames
	numTabs
)

var tabNames = [numTabs]string{"状態", "フラグ", "シーン", "コマンド", "フレーム"}

type devPanel struct {
	tab  panelTab
	sel  int
	top  int
	text string // the Command tab's line
	msg  string // the result of the last edit or command
}

// togglePanel opens the panel on its first tab, or closes it.
func (g *Game) togglePanel() {
	if g.panel != nil {
		g.closePanel()
		return
	}
	g.panel = &devPanel{}
	g.emit("panel", traceLine{"tab": "state"})
}

func (g *Game) closePanel() {
	if g.panel != nil && g.panel.tab == tabFrames && g.meter.on {
		g.meter.toggle()
	}
	g.panel = nil
}

// panelModal reports whether the panel's body is shown, which holds the game.
func (g *Game) panelModal() bool { return g.panel != nil && g.panel.tab != tabFrames }

// stateRows are the names of the state tab: the declared names, then the flags, vars,
// items and choices set now, gold.
func (g *Game) stateRows() []string {
	seen := map[string]bool{}
	var out []string
	add := func(n string) {
		if !seen[n] {
			seen[n] = true
			out = append(out, n)
		}
	}
	for _, n := range slices.Sorted(maps.Keys(g.db.Game.State.Names)) {
		add(n)
	}
	if g.st == nil {
		return out
	}
	for _, k := range slices.Sorted(maps.Keys(g.st.Flags)) {
		add("flag." + k)
	}
	for _, k := range slices.Sorted(maps.Keys(g.st.Vars)) {
		add("var." + k)
	}
	add("gold")
	for _, k := range slices.Sorted(maps.Keys(g.st.Items)) {
		add("item." + k)
	}
	for _, k := range slices.Sorted(maps.Keys(g.st.Chose)) {
		add("chose." + k)
	}
	return out
}

// stateValue is a name's value now, as text.
func (g *Game) stateValue(name string) string {
	if g.st == nil {
		return ""
	}
	kind, key, _ := strings.Cut(name, ".")
	switch kind {
	case "flag":
		return strconv.FormatBool(g.st.Flags[key])
	case "var":
		if v, ok := g.st.Vars[key]; ok {
			return fmt.Sprint(v)
		}
		return "-"
	case "gold":
		return strconv.Itoa(g.st.Gold)
	case "item":
		return strconv.Itoa(g.st.Items[key])
	case "chose":
		return strconv.Quote(g.st.Chose[key])
	}
	return ""
}

// panelEvents are the events in the order written.
func (g *Game) panelEvents() []string { return g.db.Ordered("events") }

// rows is how many rows the tab has.
func (g *Game) panelRows() int {
	switch g.panel.tab {
	case tabState:
		return len(g.stateRows())
	case tabFlags, tabScenes:
		return len(g.panelEvents())
	case tabCommand, tabFrames, numTabs:
	}
	return 0
}

// updatePanel moves in the panel and acts on its rows.
func (g *Game) updatePanel() {
	p := g.panel
	in := &g.in
	if p.tab == tabCommand && in.script == nil && in.replay == nil {
		p.text += string(ebiten.AppendInputChars(nil))
		if inpututil.IsKeyJustPressed(ebiten.KeyBackspace) && p.text != "" {
			_, n := utf8.DecodeLastRuneInString(p.text)
			p.text = p.text[:len(p.text)-n]
		}
		if inpututil.IsKeyJustPressed(ebiten.KeyEnter) || inpututil.IsKeyJustPressed(ebiten.KeyNumpadEnter) {
			g.devDo(p.text)
			return
		}
		if inpututil.IsKeyJustPressed(ebiten.KeyEscape) {
			g.closePanel()
		}
		if in.pressed[btnLeft] || in.pressed[btnRight] {
			g.panelTab(in.pressed[btnRight])
		}
		return
	}
	switch {
	case in.pressed[btnLeft] || in.pressed[btnRight]:
		g.panelTab(in.pressed[btnRight])
	case in.cancel() || in.esc:
		g.closePanel()
	case in.up():
		p.sel = max(0, p.sel-1)
	case in.down():
		p.sel = min(max(0, g.panelRows()-1), p.sel+1)
	case in.ok():
		g.panelAct(1)
	}
}

func (g *Game) panelTab(next bool) {
	p := g.panel
	d := numTabs - 1
	if next {
		d = 1
	}
	p.tab = (p.tab + d) % numTabs
	p.sel, p.top, p.msg = 0, 0, ""
	if p.tab == tabFrames && !g.meter.on {
		g.meter.toggle()
	} else if p.tab != tabFrames && g.meter.on {
		g.meter.toggle()
	}
	g.emit("panel", traceLine{"tab": []string{"state", "flags", "scenes", "command", "frames"}[p.tab]})
}

// panelAct acts on the selected row: a flag flips, a number steps, an event starts.
func (g *Game) panelAct(d int) {
	p := g.panel
	switch p.tab {
	case tabState:
		rows := g.stateRows()
		if p.sel >= len(rows) {
			return
		}
		name := rows[p.sel]
		kind, _, _ := strings.Cut(name, ".")
		switch kind {
		case "flag":
			g.devDo(fmt.Sprintf("- set: {%s: %t}", name, g.stateValue(name) != "true"))
		case "var", "item", "gold":
			if _, err := strconv.ParseFloat(g.stateValue(name), 64); err == nil || g.stateValue(name) == "-" {
				g.devDo(fmt.Sprintf("- set: {%s: \"%s + %d\"}", name, name, d))
			}
		}
	case tabScenes:
		if evs := g.panelEvents(); p.sel < len(evs) {
			g.devJump(evs[p.sel], 1)
		}
	case tabCommand:
		// A script types the line (type) and presses OK; a player presses Enter.
		g.devDo(p.text)
	case tabFlags, tabFrames, numTabs:
	}
}

// devDo runs a list of YAML commands as the panel and the script do: a list of sets runs
// at once (dev_set); anything else starts like an event (dev_do).
func (g *Game) devDo(src string) {
	src = strings.TrimSpace(src)
	if g.panel != nil {
		g.panel.msg = ""
	}
	if !data.IsScript(src) {
		src = "- " + src
	}
	cmds, ds := data.CompileScript(g.db, src, true)
	if len(ds) > 0 {
		g.devCommandError(errors.New(ds.Error()))
		g.emit("dev_do", traceLine{"do": src, "error": ds.Error()})
		return
	}
	if g.st == nil { // the title: no game to run the commands in
		msg := lang.T("ゲームが始まっていない（始めるか読み込んでから）")
		g.devCommandError(errors.New(msg))
		g.emit("dev_do", traceLine{"do": src, "error": msg})
		return
	}
	setOnly := !slices.ContainsFunc(cmds, func(c data.Command) bool { v, err := c.Verb(); return err != nil || v != "set" })
	if setOnly {
		r := event.New(g.db, g.st, event.HostFunc(g.Handle))
		r.Dev = true
		r.Log = g.devCommandError
		r.Start(cmds)
		for range 100 {
			if r.Next() == nil && !r.Running() {
				break
			}
		}
		g.emit("dev_set", traceLine{"do": src})
		g.devMessage(lang.T("変えた"))
		return
	}
	g.events.Start(cmds)
	g.emit("dev_do", traceLine{"do": src})
	g.devMessage(lang.T("始めた（パネルを閉じると進む）"))
}

func (g *Game) devMessage(s string) {
	if g.panel != nil {
		g.panel.msg = s
	}
}

func (g *Game) devCommandError(err error) {
	g.devMessage(err.Error())
	if g.opts.capture != nil {
		g.failure = errors.Join(g.failure, err)
	}
}

// devJump starts an event from the state as it is, at a line, like --event (jump).
func (g *Game) devJump(name string, line int) {
	cmds := g.commandsNamed(name)
	if cmds == nil || g.st == nil {
		g.devMessage(fmt.Sprintf(lang.T("イベント %s がない"), name))
		return
	}
	g.stopSkip("jump")
	g.events.Stop()
	g.req, g.talk = nil, talk{}
	g.recruit = nil
	g.enterAt = ""
	g.events.Start(cmds)
	g.fast = &fastTo{lines: max(1, line)}
	g.emit("jump", traceLine{"event": name, "line": max(1, line)})
	g.closePanel()
}

// drawPanel draws the panel's tab bar and body (the Frames tab draws only the meter).
func (g *Game) drawPanel(screen *ebiten.Image) {
	p := g.panel
	if p == nil || p.tab == tabFrames {
		return
	}
	sw, sh := g.screen()
	fill(screen, color.NRGBA{0x10, 0x14, 0x1c, 0xe8})
	const size = 18.0
	x := 24.0
	for i, n := range tabNames {
		c := g.th.dim
		if panelTab(i) == p.tab {
			c = g.th.accent
		}
		label := lang.T(n)
		gfx.DrawTextLeft(screen, label, x, 14, size, c)
		x += gfx.TextWidth(label, size) + 28
	}
	gfx.DrawTextLeft(screen, lang.T("← →: タブ　↑↓: 選ぶ　Z: 実行　X / F8: 閉じる"), 24, sh-34, 14, g.th.dim)
	if p.msg != "" {
		gfx.DrawTextLeft(screen, p.msg, 24, sh-60, 14, g.th.cursor)
	}
	rowH := size * 1.4
	rows := int((sh - 130) / rowH)
	if p.sel < p.top {
		p.top = p.sel
	}
	if p.sel >= p.top+rows {
		p.top = p.sel - rows + 1
	}
	y := 52.0
	table := func(cells [][]string) {
		xs := panelColumns(cells, 24+gfx.TextWidth("> ", size), size)
		for i, row := range cells {
			if i < p.top || i >= p.top+rows {
				continue
			}
			c := g.th.text
			if i == p.sel {
				c = g.th.cursor
				gfx.DrawTextLeft(screen, ">", 24, y, size, c)
			}
			for j, s := range row {
				gfx.DrawTextLeft(screen, s, xs[j], y, size, c)
			}
			y += rowH
		}
	}
	switch p.tab {
	case tabState:
		table(g.stateTable())
	case tabFlags:
		g.drawFlagTable(screen, y, rows, rowH, size)
	case tabScenes:
		table(g.scenesTable())
	case tabCommand:
		gfx.DrawTextLeft(screen, lang.T("YAML の命令を一行（例: set: {flag.met: true}）。Enter で実行"), 24, y, size, g.th.dim)
		gfx.DrawTextLeft(screen, "> "+p.text+"_", 24, y+rowH*1.5, size, g.th.text)
	case tabFrames, numTabs:
	}
	_ = sw
}

// panelGap is the room between two columns of the panel's tables.
const panelGap = 24.0

// panelColumns are the left edges of the columns of a table of cells drawn from x in the
// panel's font at size: each column starts where the widest cell of the one before it
// ends, measured in the font, so the values line up whatever the length of a name.
func panelColumns(cells [][]string, x, size float64) []float64 {
	var widths []float64
	for _, row := range cells {
		for j, s := range row {
			if j >= len(widths) {
				widths = append(widths, 0)
			}
			widths[j] = max(widths[j], gfx.TextWidth(s, size))
		}
	}
	xs := make([]float64, len(widths))
	for j := range widths {
		xs[j] = x
		x += widths[j] + panelGap
	}
	return xs
}

// stateTable is the state tab: each name, its value and how many places write and read it.
func (g *Game) stateTable() [][]string {
	uses := g.panelUses()
	var out [][]string
	for _, n := range g.stateRows() {
		w, r := 0, 0
		if u := uses[n]; u != nil {
			w, r = len(u.Sets), len(u.Reads)
		}
		out = append(out, []string{n, g.stateValue(n), fmt.Sprintf("writes %d", w), fmt.Sprintf("reads %d", r)})
	}
	return out
}

// scenesTable is the scenes tab: each event, its trigger, whether it runs once, how many
// times it ran and how much of it was read.
func (g *Game) scenesTable() [][]string {
	var out [][]string
	for _, id := range g.panelEvents() {
		ev := g.db.Events[id]
		trig := ev.Trigger
		if ev.Location != "" {
			trig += "@" + ev.Location
		}
		once := ""
		if ev.Once {
			once = "once"
			if g.st != nil && g.st.Done[id] {
				once = "done"
			}
		}
		ran := 0
		if g.st != nil {
			ran = g.st.Ran[id][0]
		}
		read, all := g.readRate(id)
		rate := "-"
		if all > 0 {
			rate = fmt.Sprintf("%d%%", read*100/all)
		}
		out = append(out, []string{id, trig, once, fmt.Sprintf("ran %d", ran), "read " + rate})
	}
	return out
}

// panelUses is the index of names, made once for the data (DB.StateUses).
func (g *Game) panelUses() map[string]*data.NameUse {
	if g.usesDB != g.db {
		g.uses, g.usesDB = g.db.StateUses(), g.db
	}
	return g.uses
}

// drawFlagTable draws the events against the flags: S for set, R for read.
func (g *Game) drawFlagTable(screen *ebiten.Image, y float64, rows int, rowH, size float64) {
	p := g.panel
	uses := g.panelUses()
	var flags []string
	for _, n := range slices.Sorted(maps.Keys(uses)) {
		if strings.HasPrefix(n, "flag.") {
			flags = append(flags, n)
		}
	}
	sw, _ := g.screen()
	const colW = 44.0
	// The flags' columns start after the longest event id, measured in the font.
	nameW := 0.0
	for _, id := range g.panelEvents() {
		nameW = max(nameW, gfx.TextWidth(id, size*0.9))
	}
	nameW = min(nameW+panelGap, sw/2)
	fit := max(0, int((sw-48-nameW)/colW))
	shown := flags[:min(fit, len(flags))]
	for j, f := range shown {
		v := "F"
		if g.st != nil && g.st.Flags[strings.TrimPrefix(f, "flag.")] {
			v = "T"
		}
		gfx.DrawTextLeft(screen, fmt.Sprintf("%d%s", j+1, v), 24+nameW+float64(j)*colW, y, size*0.8, g.th.dim)
	}
	if len(flags) > len(shown) {
		gfx.DrawTextLeft(screen, fmt.Sprintf("+%d", len(flags)-len(shown)), sw-60, y, size*0.8, g.th.dim)
	}
	y += rowH
	for i, id := range g.panelEvents() {
		if i < p.top || i >= p.top+rows-2 {
			continue
		}
		c := g.th.text
		if i == p.sel {
			c = g.th.cursor
		}
		if id == g.evName {
			c = g.th.accent
		}
		gfx.DrawTextLeft(screen, id, 24, y, size*0.9, c)
		prefix := "events." + id + "."
		for j, f := range shown {
			u := uses[f]
			s := ""
			if slices.ContainsFunc(u.Sets, func(w string) bool { return strings.HasPrefix(w, prefix) }) {
				s += "S"
			}
			if slices.ContainsFunc(u.Reads, func(w string) bool { return strings.HasPrefix(w, prefix) }) {
				s += "R"
			}
			gfx.DrawTextLeft(screen, s, 24+nameW+float64(j)*colW, y, size*0.9, g.th.text)
		}
		y += rowH
	}
	y += rowH / 2
	var legend []string
	for j, f := range shown {
		legend = append(legend, fmt.Sprintf("%d %s", j+1, strings.TrimPrefix(f, "flag.")))
	}
	ls := gfx.Wrap(strings.Join(legend, "  "), size*0.75, sw-48)
	for _, l := range ls[:min(2, len(ls))] {
		gfx.DrawTextLeft(screen, l, 24, y, size*0.75, g.th.dim)
		y += rowH * 0.8
	}
}
