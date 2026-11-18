package play

import (
	"image"
	"math"
	"slices"
	"strings"
	"testing"

	"github.com/nao1215/gozen/internal/rules/event"
	"github.com/nao1215/gozen/internal/screen/gfx"
)

func TestDungeonSnapSettingRoundTrip(t *testing.T) {
	dir := t.TempDir()
	g := wldWorld(t, "fx_maze", Options{SaveDir: dir})
	if g.cfg.DungeonSnap {
		t.Fatal("existing settings must retain animated transitions")
	}
	s := newConfigScreen(g)
	s.list.sel = slicesIndexRow(t, s, rowDungeonSnap)
	s.change(g, 1, false)
	if !g.cfg.DungeonSnap {
		t.Fatal("settings did not enable instant transitions")
	}
	if err := g.saveConfig(); err != nil {
		t.Fatal(err)
	}
	if err := g.Close(); err != nil {
		t.Fatal(err)
	}
	restored := wldWorld(t, "fx_maze", Options{SaveDir: dir})
	if !restored.cfg.DungeonSnap {
		t.Fatal("instant transitions did not survive restarting the game")
	}
}

func TestSettingsFitAuthoredScreen(t *testing.T) {
	for _, viewport := range []struct {
		name string
		w, h int
	}{{"portrait", 360, 540}, {"landscape", 640, 360}, {"desktop", 960, 720}} {
		t.Run(viewport.name, func(t *testing.T) {
			g := wldWorld(t, "fx_maze", Options{})
			g.view.w, g.view.h, g.ui = viewport.w, viewport.h, 1
			s := newConfigScreen(g)
			l := s.list
			if l.x-30 < 0 || l.x+l.w+30 > float64(viewport.w) || l.y-80 < 0 || l.y+l.height()+110 > float64(viewport.h) {
				t.Fatalf("settings panel leaves %dx%d viewport: list=%+v height=%v", viewport.w, viewport.h, l, l.height())
			}
			if viewport.name == "desktop" && (l.w != 640 || l.rows != 11 || l.x != 160 || math.Abs(l.y-720*0.13) > 0.001) {
				t.Fatalf("ordinary desktop layout changed: %+v", l)
			}
			for i := range l.items {
				if l.items[i].right != "" && gfx.TextWidth(l.items[i].right, l.size) > l.w-144+0.01 {
					t.Fatalf("setting value leaves no readable label room: %q", l.items[i].right)
				}
				if i > 0 {
					sysKey(g, btnDown)
					s.update(g)
				}
				if l.sel != i {
					t.Fatalf("keyboard cannot reach setting %d: selected %d", i, l.sel)
				}
				r := l.rowRect(i)
				if r.Min.X < 0 || r.Min.Y < 0 || r.Max.X > viewport.w || r.Max.Y > viewport.h {
					t.Fatalf("setting %q is unreachable inside viewport: %v", l.items[i].label, r)
				}
				if !slices.Contains(l.shown(), i) {
					t.Fatalf("setting %q cannot be scrolled into view", l.items[i].label)
				}
				// Move away within the visible rows, then select this row by pointer.
				for _, other := range l.shown() {
					if other != i {
						l.sel = other
						break
					}
				}
				sysKey(g)
				g.in.moved, g.in.mouse = true, image.Pt(r.Min.X+50, (r.Min.Y+r.Max.Y)/2)
				s.update(g)
				if l.sel != i {
					t.Fatalf("pointer cannot reach setting %d: selected %d", i, l.sel)
				}
				g.in.moved = false
			}
			// Changing a value rebuilds its width constraints before the next draw.
			l.sel = slicesIndexRow(t, s, rowText)
			for range 5 {
				s.change(g, 1, true)
				for _, item := range l.items {
					if item.right != "" && gfx.TextWidth(item.right, l.size) > l.w-144+0.01 {
						t.Fatalf("changed value no longer fits: %q", item.right)
					}
				}
			}
			l.sel = len(l.items) - 1
			l.scroll()
			closed := false
			s.done = func() { closed = true }
			g.push(s)
			back := l.rowRect(len(l.items) - 1)
			sysKey(g)
			g.in.mouse, g.in.click = image.Pt(back.Min.X+50, (back.Min.Y+back.Max.Y)/2), true
			s.update(g)
			if !closed {
				t.Fatal("visible Back setting cannot be activated by pointer")
			}
		})
	}
}

// TestTextSizeSettingPagesLargerLines: the text size of the settings draws the message
// window's lines larger with a line fewer a page, pages the line shown again at once, and
// is kept with the settings.
func TestTextSizeSettingPagesLargerLines(t *testing.T) {
	const pageShrinkTest = 0.8
	dir := t.TempDir()
	g := novelGame(t, Options{SaveDir: dir})
	long := strings.Repeat("The wind runs over the pass and the lamps of the inn come on one by one. ", 6)
	g.talk.open(g, event.Say{Text: long})
	base, lines := g.talk.size, 0
	for _, p := range g.talk.pages {
		lines = max(lines, len(p))
	}
	if lines != talkLines {
		t.Fatalf("a long line fills %d lines a page at the game's size, want %d", lines, talkLines)
	}
	s := newConfigScreen(g)
	s.list.sel = slicesIndexRow(t, s, rowTextSize)
	for want, k := range []float64{1.2, 1.4} {
		s.change(g, 1, false)
		if g.cfg.TextSize != want+1 {
			t.Fatalf("text size = %d, want %d", g.cfg.TextSize, want+1)
		}
		if g.talk.size < base || g.talk.size < base*k*pageShrinkTest {
			t.Errorf("at %v the line shown is drawn at %v, not larger than the game's %v", k, g.talk.size, base)
		}
		for i, p := range g.talk.pages {
			if len(p) > talkLines-1 {
				t.Errorf("at %v page %d has %d lines, want %d at most", k, i, len(p), talkLines-1)
			}
		}
	}
	s.change(g, 1, false) // the largest stays the largest
	if g.cfg.TextSize != 2 {
		t.Fatalf("text size went past the largest: %d", g.cfg.TextSize)
	}
	if err := g.saveConfig(); err != nil {
		t.Fatal(err)
	}
	if err := g.Close(); err != nil {
		t.Fatal(err)
	}
	if again := novelGame(t, Options{SaveDir: dir}); again.cfg.TextSize != 2 || again.cfg.textScale() != 1.4 {
		t.Fatalf("the text size did not survive restarting: %d", again.cfg.TextSize)
	}
}
