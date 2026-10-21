package play

import (
	"bytes"
	"image"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
)

func sysItems(labels ...string) []listItem {
	var out []listItem
	for _, l := range labels {
		out = append(out, listItem{label: l})
	}
	return out
}

// TestListCursorWrapsAndScrolls: up from the top goes to the last row, down from the last
// goes to the first, and the visible window follows the cursor.
func TestListCursorWrapsAndScrolls(t *testing.T) {
	g := sysOpen(t, Options{}, nil)
	l := newList(sysItems("a", "b", "c", "d", "e"), 0, 0, 400, 3)
	sysKey(g, btnUp)
	if chose, cancel := l.update(g); chose || cancel || l.sel != 4 || l.top != 2 {
		t.Fatalf("up from the top: sel %d top %d", l.sel, l.top)
	}
	sysKey(g, btnDown)
	l.update(g)
	if l.sel != 0 || l.top != 0 {
		t.Fatalf("down from the bottom: sel %d top %d", l.sel, l.top)
	}
	for range 3 {
		sysKey(g, btnDown)
		l.update(g)
	}
	if l.sel != 3 || l.top != 1 {
		t.Fatalf("scrolled to sel %d top %d", l.sel, l.top)
	}
	sysKey(g, btnOK)
	if chose, _ := l.update(g); !chose {
		t.Fatal("ok did not choose")
	}
	sysKey(g, btnCancel)
	if _, cancel := l.update(g); !cancel {
		t.Fatal("cancel did not cancel")
	}
	g.in.wheel = 1
	sysKey(g)
	g.in.wheel = 1
	l.update(g)
	if l.sel != 2 {
		t.Fatalf("wheel up did not move the cursor: %d", l.sel)
	}
}

// TestListDisabledRowsCannotBeChosen and the mouse.
func TestListDisabledRowsAndMouse(t *testing.T) {
	g := sysOpen(t, Options{}, nil)
	items := sysItems("a", "b", "c")
	items[1].disabled = true
	l := newList(items, 10, 20, 300, 3)
	l.sel = 1
	sysKey(g, btnOK)
	if chose, cancel := l.update(g); chose || cancel {
		t.Fatal("a disabled row was chosen")
	}
	// hovering a row selects it only when the pointer moved.
	r := l.rowRect(2)
	sysKey(g)
	g.in.mouse = image.Pt(r.Min.X+5, r.Min.Y+5)
	l.update(g)
	if l.sel != 1 {
		t.Fatal("a pointer that did not move changed the cursor")
	}
	g.in.moved = true
	l.update(g)
	if l.sel != 2 {
		t.Fatalf("hover: %d", l.sel)
	}
	// clicking a row chooses it; clicking outside does nothing.
	sysKey(g)
	g.in.click, g.in.mouse = true, image.Pt(r.Min.X+5, r.Min.Y+5)
	if chose, _ := l.update(g); !chose {
		t.Fatal("click on a row did not choose")
	}
	sysKey(g)
	g.in.click, g.in.mouse = true, image.Pt(900, 900)
	if chose, _ := l.update(g); chose {
		t.Fatal("click outside chose")
	}
	empty := newList(nil, 0, 0, 100, 3)
	sysKey(g, btnOK)
	if chose, cancel := empty.update(g); chose || cancel {
		t.Fatal("an empty list reacts")
	}
	if empty.height() <= 0 {
		t.Fatal("height of an empty list")
	}
}

// TestListDrawShowsNotesAndLongLabels: drawing a list with notes, a right column, long labels and
// more rows than fit traces the rows the player sees.
func TestListDrawShowsNotesAndLongLabels(t *testing.T) {
	var trace bytes.Buffer
	g := sysOpen(t, Options{Trace: &trace, Location: "camp"}, nil)
	items := sysItems("short", "a very long label that cannot possibly fit in the narrow list that holds it", "third", "fourth")
	items[0].note, items[0].right = "!", "10G"
	items[1].note, items[1].disabled = "new item", true
	items[2].note, items[2].noteColor = "x", colorWhite
	l := newList(items, 20, 20, 260, 2)
	l.top = 1
	l.sel = 1
	img := ebiten.NewImage(1280, 720)
	g.tr.drawn = map[string]bool{}
	l.draw(g, img, true)
	l.drawRows(g, img, false)
	if len(g.tr.lists) != 1 {
		t.Fatalf("%d lists traced", len(g.tr.lists))
	}
	tl := g.tr.lists[0]
	if tl.Sel != 1 || len(tl.Items) != 4 || tl.Items[0] != "short 10G" || tl.Items[1] != items[1].label+" (x)" {
		t.Fatalf("traced %+v", tl)
	}
	h := g.drawLines(img, "one two three four five six seven eight nine ten eleven twelve", 0, 0, 120, sizeS, colorWhite)
	if h <= 0 {
		t.Fatal("no height for drawn lines")
	}
	if two := g.drawLines(img, "one", 0, 0, 120, sizeS, colorWhite); two >= h {
		t.Fatalf("one word is as tall as twelve: %v %v", two, h)
	}
}

// TestHPColorFollowsTheRate: green above half, amber below, red under a quarter.
func TestHPColorFollowsTheRate(t *testing.T) {
	low, mid, high := hpColor(0.1), hpColor(0.4), hpColor(0.9)
	if low == mid || mid == high || low == high {
		t.Fatalf("colors %v %v %v", low, mid, high)
	}
	if hpColor(0.25) != mid || hpColor(0.5) != high {
		t.Fatal("the thresholds belong to the higher colour")
	}
}

// TestConfigClicksAdjustRows: a click in the right edge raises a value, in the middle lowers it,
// anywhere else steps and wraps; the last row ignores clicks as values.
func TestConfigClicksAdjustRows(t *testing.T) {
	g := sysOpen(t, Options{}, nil)
	g.push(newConfigScreen(g))
	s := g.stack[len(g.stack)-1].(*configScreen)
	row := slicesIndexRow(t, s, rowText)
	r := s.list.rowRect(row)
	g.cfg.TextSpeed = 1
	sysKey(g)
	g.in.click, g.in.mouse = true, image.Pt(r.Max.X-10, r.Min.Y+5)
	s.update(g)
	if g.cfg.TextSpeed != 2 || s.list.sel != row || g.in.click {
		t.Fatalf("right edge: speed %d sel %d", g.cfg.TextSpeed, s.list.sel)
	}
	sysKey(g)
	g.in.click, g.in.mouse = true, image.Pt(r.Min.X+int(s.list.w*0.6), r.Min.Y+5)
	s.update(g)
	if g.cfg.TextSpeed != 1 {
		t.Fatalf("middle: speed %d", g.cfg.TextSpeed)
	}
	g.cfg.TextSpeed = 4
	sysKey(g)
	g.in.click, g.in.mouse = true, image.Pt(r.Min.X+20, r.Min.Y+5)
	s.update(g)
	if g.cfg.TextSpeed != 0 {
		t.Fatalf("left part wraps from the top: speed %d", g.cfg.TextSpeed)
	}
	// ok on a row steps it, left lowers.
	sysKey(g, btnOK)
	s.update(g)
	if g.cfg.TextSpeed != 1 {
		t.Fatalf("ok: speed %d", g.cfg.TextSpeed)
	}
	sysKey(g, btnLeft)
	s.update(g)
	if g.cfg.TextSpeed != 0 {
		t.Fatalf("left: speed %d", g.cfg.TextSpeed)
	}
	if s.row(-1) != rowBack || s.row(999) != rowBack {
		t.Fatal("a row outside the list is not 'back'")
	}
}

// TestConfigRowsStepTheirOwnValues: every row changes its own setting and nothing else.
func TestConfigRowsStepTheirOwnValues(t *testing.T) {
	g := sysOpen4(t, Options{Location: "camp"}, map[string]string{"data/sp.yaml": "events:\n  plan:\n    do:\n      - sp_suggest: plan\n"})
	orig := ebiten.IsFullscreen()
	t.Cleanup(func() { ebiten.SetFullscreen(orig) })
	s := newConfigScreen(g)
	before := g.cfg
	for _, c := range []struct {
		row  cfgRow
		read func(config) any
	}{
		{rowAutoWait, func(c config) any { return c.AutoWait }},
		{rowBGM, func(c config) any { return c.BGM }},
		{rowSE, func(c config) any { return c.SE }},
		{rowFullscreen, func(c config) any { return c.Fullscreen }},
		{rowSP, func(c config) any { return c.ManualSP }},
		{rowSkipAll, func(c config) any { return c.SkipAll }},
		{rowBattleSpeed, func(c config) any { return c.BattleSpeed }},
	} {
		i := slicesIndexRow(t, s, c.row)
		s.list.sel = i
		old := c.read(g.cfg)
		s.change(g, 1, false)
		if c.read(g.cfg) == old {
			t.Errorf("row %d did not change its value", c.row)
		}
		if g.cfg.TextSpeed != before.TextSpeed {
			t.Errorf("row %d changed the text speed", c.row)
		}
	}
	// the sample text under the list follows the SP row.
	s.list.sel = slicesIndexRow(t, s, rowSP)
	sysSafeDraw(g)
	g.push(s)
	sysFrames(t, g, 2)
}

// TestConfigScreenKeepsTheRowsOnReload: after a reload the rows are made again from the data.
func TestConfigScreenKeepsTheRowsOnReload(t *testing.T) {
	g := sysOpen(t, Options{}, nil)
	s := newConfigScreen(g)
	n := len(s.list.items)
	if got := s.liveKeep(g); got != scene(s) || len(s.list.items) != n {
		t.Fatal("the screen was not kept")
	}
	if languageName("fr") != "fr" || languageName("en") != "English" || languageName("ja") != "日本語" {
		t.Fatal("language names")
	}
}

// TestTitleMenuChoices: the title's items open the load list, the gallery and the settings, and
// the last one quits.
func TestTitleMenuChoices(t *testing.T) {
	g := sysOpen4(t, Options{Seed: 1}, map[string]string{"game.yaml+": "gallery: {cgs: [{title: One, file: x.png}]}\n"})
	g.newGame()
	if err := g.save(0); err != nil {
		t.Fatal(err)
	}
	g.st = nil
	g.replace(newTitle(g))
	ti := g.stack[0].(*title)
	choose := func(i int) {
		t.Helper()
		ti.menu.sel = i
		sysKey(g, btnOK)
		ti.update(g)
	}
	choose(1)
	if _, ok := g.stack[len(g.stack)-1].(*saveMenu); !ok {
		t.Fatalf("continue opened %T", g.stack[len(g.stack)-1])
	}
	g.pop()
	choose(2)
	if _, ok := g.stack[len(g.stack)-1].(*galleryScreen); !ok {
		t.Fatalf("gallery opened %T", g.stack[len(g.stack)-1])
	}
	g.pop()
	choose(3)
	if _, ok := g.stack[len(g.stack)-1].(*configScreen); !ok {
		t.Fatalf("settings opened %T", g.stack[len(g.stack)-1])
	}
	g.pop()
	if canQuit {
		choose(len(ti.menu.items) - 1)
		if !g.quit {
			t.Fatal("the last item did not quit")
		}
		if err := g.Update(); !sysQuit(err) {
			t.Fatalf("update after quit: %v", err)
		}
	}
	// the menu is rebuilt on reload with the cursor kept.
	g.quit = false
	ti.menu.sel = 3
	if got := ti.liveKeep(g); got != scene(ti) || ti.menu.sel != 3 {
		t.Fatalf("liveKeep moved the cursor to %d", ti.menu.sel)
	}
}

// TestTitleMenuTracesWhatIsDrawn: the menu drawn on the title is what the trace shows.
func TestTitleMenuTracesWhatIsDrawn(t *testing.T) {
	var trace bytes.Buffer
	g := sysOpen(t, Options{Trace: &trace, Seed: 1}, nil)
	sysFrames(t, g, 3)
	if err := g.Close(); err != nil {
		t.Fatal(err)
	}
	menus := traceLines(t, trace.Bytes(), "menu")
	if len(menus) == 0 {
		t.Fatal("no menu in the trace")
	}
	items, _ := menus[0]["items"].([]any)
	if len(items) < 4 || items[0] != "New Game" || items[1] != "Continue (x)" {
		t.Fatalf("menu %v", menus[0])
	}
}

// TestEndingAndGameOverDraw: the ending after a while thanks the player, and falls back to the
// picture shown before; a game over song plays.
func TestEndingAndGameOverDraw(t *testing.T) {
	g := sysMusicOpen(t, Options{Seed: 1, Location: "hall"})
	g.db.Game.GameOver.Music.Song = "fight"
	g.cg.File = "x.png"
	g.replace(newEnding(g))
	e := g.stack[0].(*ending)
	if e.pic != "x.png" {
		t.Fatalf("ending picture %q", e.pic)
	}
	e.age = 151
	sysFrames(t, g, 1)
	g.cg.File = ""
	g.cgOld.File, g.cgOldT = "x.png", 5
	if e2 := newEnding(g); e2.pic != "x.png" {
		t.Fatalf("fell back to %q", e2.pic)
	}
	g.replace(newGameOver(g))
	sysPlaying(t, g, "fight")
}
