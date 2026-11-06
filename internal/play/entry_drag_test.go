package play

import (
	"bytes"
	"strings"
	"testing"
)

const dragBoard = `planning:
  calendar: {start: '2030-01-01', end: '2030-01-09', phases: [morning]}
  entities: {potter: {name: Potter}}
  activities: {glaze: {name: Bowl glazing, effort: 2}}
screens:
  desk:
    title: Desk
    kind: workspace
    view: {fullscreen: true, light: true}
    entries:
    - {label: Bowl, each: {param: id, items: [bowl]}, drag: '{arg.id}', bounds: [40, 150, 200, 100]}
    - {label: Cup, drag: cup, bounds: [40, 300, 200, 100]}
    - {label: Shelf, drop: item, accepts: 'arg.item != "cup"', hint: 'Put {arg.item}', bounds: [400, 150, 240, 120], do: [{set: {var.got: 'arg.item'}}]}
    - {label: Count, style: button, bounds: [700, 150, 140, 38], do: [{set: {var.n: 'var.n+1'}}]}
    - {label: Day, style: plate, peek: 'Busy day', bounds: [700, 300, 200, 80]}
`

func openDragBoard(t *testing.T, trace *bytes.Buffer) *Game {
	t.Helper()
	fsys := wldWorldFiles(t, dragBoard, nil)
	fsys["game.yaml"].Data = []byte(strings.Replace(string(fsys["game.yaml"].Data), "schema_version: 4", "schema_version: 10", 1))
	opts := Options{FS: fsys, Location: "camp"}
	if trace != nil {
		opts.Trace, opts.Seed = trace, 5
	}
	g := wldOpenWith(t, opts)
	wldFrames(t, g, 30)
	g.openScreen("desk", func() {})
	wldFrames(t, g, 4)
	return g
}

// A press moved over a drop place drags a copy and drops it: the place's do
// gets the carried value, and the trace says what was dropped where.
func TestDragDropsWithTheMouse(t *testing.T) {
	var buf bytes.Buffer
	g := openDragBoard(t, &buf)
	wldPlay(t, g, "drag 140 200 520 210\nwait 8\n", 60)
	if g.st.Vars["got"] != "bowl" {
		t.Fatal("the drop did not run with the carried value", g.st.Vars)
	}
	if s := top(t, g); s.carry != nil || s.name != "desk" {
		t.Fatal("the drop left something carried or left the screen", s.name, s.carry)
	}
	drops := traceLines(t, buf.Bytes(), "screen_drop")
	if len(drops) != 1 || drops[0]["entry"] != 2.0 || drops[0]["source"] != "bowl" {
		t.Fatal("no screen_drop line", drops)
	}
	// Dropped on nothing, the drag only lets go.
	wldPlay(t, g, "drag 140 200 300 650\nwait 8\n", 60)
	if s := top(t, g); s.carry != nil || len(traceLines(t, buf.Bytes(), "screen_drop")) != 1 {
		t.Fatal("a drag dropped on nothing did something", s.carry)
	}
}

// A tap picks up, a tap on the place drops; a tap on empty space puts down.
func TestDragByTapping(t *testing.T) {
	g := openDragBoard(t, nil)
	wldPlay(t, g, "click 140 200\nwait 4\n", 30)
	if s := top(t, g); s.carry == nil || s.carry.value != "bowl" || s.carry.hints[2] != "Put bowl" {
		t.Fatal("a tap did not pick up the card", s.carry)
	}
	wldPlay(t, g, "click 300 650\nwait 4\n", 30)
	if s := top(t, g); s.carry != nil {
		t.Fatal("a tap on empty space did not put it down")
	}
	wldPlay(t, g, "click 140 200\nwait 4\nclick 520 210\nwait 8\n", 60)
	if g.st.Vars["got"] != "bowl" {
		t.Fatal("tap and tap did not drop", g.st.Vars)
	}
	// Tapping the carried card again puts it down.
	wldPlay(t, g, "click 140 200\nwait 4\nclick 140 200\nwait 4\n", 40)
	if s := top(t, g); s.carry != nil {
		t.Fatal("a second tap on the carried card kept it")
	}
}

// The keys stop on the carried card and the place: OK picks up, OK drops, and
// cancel only puts down without closing the screen.
func TestDragWithTheKeys(t *testing.T) {
	g := openDragBoard(t, nil)
	wldPlay(t, g, "press z\nwait 4\npress down\nwait 2\npress down\nwait 2\npress z\nwait 8\n", 60)
	if g.st.Vars["got"] != "bowl" {
		t.Fatal("OK, move and OK did not drop", g.st.Vars)
	}
	wldPlay(t, g, "press up\nwait 2\npress up\nwait 2\npress z\nwait 4\n", 40)
	if s := top(t, g); s.carry == nil {
		t.Fatal("OK on the card did not pick it up")
	}
	wldPlay(t, g, "press x\nwait 4\n", 30)
	if s := top(t, g); s.carry != nil || s.name != "desk" {
		t.Fatal("cancel closed the screen or kept the card", s.name, s.carry)
	}
}

// A place whose accepts is false takes nothing: the drop runs nothing.
func TestDropRefused(t *testing.T) {
	var buf bytes.Buffer
	g := openDragBoard(t, &buf)
	wldPlay(t, g, "drag 140 350 520 210\nwait 8\n", 60)
	if g.st.Vars["got"] != nil {
		t.Fatal("a refused place ran", g.st.Vars)
	}
	if r := traceLines(t, buf.Bytes(), "screen_refused"); len(r) != 1 || r[0]["source"] != "cup" {
		t.Fatal("no screen_refused line", r)
	}
}

// What is carried survives a refresh, even across another control's action.
func TestCarryKeptOverRefresh(t *testing.T) {
	g := openDragBoard(t, nil)
	wldPlay(t, g, "click 140 200\nwait 4\n", 30)
	if n := top(t, g).refreshed(g); n.carry == nil || n.carry.value != "bowl" || n.carry.entry != 0 {
		t.Fatal("refresh dropped the carried card")
	}
	wldPlay(t, g, "click 770 170\nwait 10\n", 40)
	if s := top(t, g); g.st.Vars["n"] != float64(1) || s.carry == nil || !s.carry.ok[2] {
		t.Fatal("another action or its refresh lost the carried card", g.st.Vars, s.carry)
	}
}

// pick reaches cards and places by label; a place is disabled until it takes
// what is carried, and look names what is held.
func TestDragPickAndLook(t *testing.T) {
	var buf bytes.Buffer
	g := openDragBoard(t, &buf)
	if err := g.scriptPick(&g.in, "Shelf"); err == nil {
		t.Fatal("pick chose a place with nothing carried")
	}
	wldPlay(t, g, "pick Cup\nwait 4\nlook\nwait 2\n", 30)
	if err := g.scriptPick(&g.in, "Shelf"); err == nil {
		t.Fatal("pick chose a place that refuses the cup")
	}
	looks := traceLines(t, buf.Bytes(), "look")
	l := looks[len(looks)-1]
	if l["held"] != "Cup" || !strings.Contains(strings.Join(strs(l["disabled"]), ","), "Shelf") {
		t.Fatal("look does not show the held card and the refusing place", l)
	}
	wldPlay(t, g, "pick Bowl\nwait 4\npick Shelf\nwait 8\n", 40)
	if g.st.Vars["got"] != "bowl" {
		t.Fatal("pick did not drop", g.st.Vars)
	}
}

// A peek opens beside its plate, shows in look, and closes on a press outside
// or cancel; it runs nothing.
func TestPeek(t *testing.T) {
	var buf bytes.Buffer
	g := openDragBoard(t, &buf)
	wldPlay(t, g, "click 800 340\nwait 4\nlook\nwait 2\n", 30)
	looks := traceLines(t, buf.Bytes(), "look")
	if s := top(t, g); s.peek == nil || looks[len(looks)-1]["peek"] != "Busy day" {
		t.Fatal("a press on the plate did not open its peek", looks)
	}
	g.Draw(covScreen())
	wldPlay(t, g, "click 300 650\nwait 4\n", 30)
	if s := top(t, g); s.peek != nil {
		t.Fatal("a press outside did not close the peek")
	}
	wldPlay(t, g, "pick Day\nwait 4\npress x\nwait 4\n", 30)
	if s := top(t, g); s.peek != nil || s.name != "desk" || len(g.st.Vars) != 0 {
		t.Fatal("cancel did not close only the peek", s.peek, g.st.Vars)
	}
}

// While a card is dragged, the copy, the rings, the hints and the refused
// mark draw; the place that takes the card is edged in the accent.
func TestDragDraws(t *testing.T) {
	g := openDragBoard(t, nil)
	s := top(t, g)
	s.pickUp(g, 0)
	s.press = &pointerPress{on: 0, dragging: true}
	g.in.mouse.X, g.in.mouse.Y = 500, 400
	screen := covScreen()
	g.Draw(screen)
	s.pickUp(g, 1)
	g.Draw(screen)
	if s.carry.ok[2] {
		t.Fatal("the place takes the cup")
	}
}

func TestPointerReleaseOutsidePartCancelsTap(t *testing.T) {
	g := openDragBoard(t, nil)
	wldPlay(t, g, "click 140 200\nwait 4\ndrag 520 210 300 650\nwait 8\n", 80)
	if g.st.Vars["got"] != nil || top(t, g).carry == nil {
		t.Fatal("releasing outside the drop place committed the drop", g.st.Vars)
	}
	wldPlay(t, g, "press x\nwait 4\ndrag 800 340 300 650\nwait 8\n", 80)
	if top(t, g).peek != nil {
		t.Fatal("releasing outside the peek opened it")
	}
}
