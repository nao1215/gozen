package play

import (
	"fmt"
	"strings"
	"testing"
)

const partsBoard = `planning:
  calendar: {start: '2030-01-01', end: '2030-01-09', phases: [morning]}
  entities: {potter: {name: Potter}}
  activities:
    glaze: {name: Bowl glazing, effort: 2}
    fire: {name: Kiln firing, effort: 1, after: [glaze]}
screens:
  studio:
    title: Studio
    kind: workspace
    view: {fullscreen: true}
    entries:
    - {label: Hours, text: 8h a day, style: plate, bounds: [32, 150, 300, 120]}
    - {label: Count, style: button, bounds: [32, 300, 140, 38], do: [{set: {var.counted: 'var.counted+1'}}]}
    - label: '{activity.glaze.name}'
      bounds: [360, 150, 280, 150]
      do: [{set: {var.opened: 'var.opened+1'}}]
      ticket: {due: Jan 3, state: open, planned: '0.5', progress: '0.25', behind: 'true'}
    - label: '{activity.fire.name}'
      bounds: [660, 150, 280, 150]
      ticket: {due: Jan 5, state: waiting, progress: '0', blocked: 'true', after: ['{activity.glaze.name}', '']}
    - {label: Fire, carousel: tools, cost: 1 slot, text: Fire the kiln., bounds: [200, 500, 800, 190], do: [{set: {var.fired: 'var.fired+1'}}]}
    - {label: Glaze, carousel: tools, cost: 1 slot, text: Glaze a bowl., unavailable: 'true', reason: The kiln is busy., bounds: [200, 500, 800, 190], do: [{set: {var.glazed: 'var.glazed+1'}}]}
    - {label: Pack, carousel: tools, text: Pack the bowls., bounds: [200, 500, 800, 190], do: [{set: {var.packed: 'var.packed+1'}}]}
`

func openPartsBoard(t *testing.T) (*Game, *authorScreen) {
	t.Helper()
	fsys := wldWorldFiles(t, partsBoard, nil)
	fsys["game.yaml"].Data = []byte(strings.Replace(string(fsys["game.yaml"].Data), "schema_version: 4", "schema_version: 10", 1))
	g := wldOpenWith(t, Options{FS: fsys, Location: "camp"})
	wldFrames(t, g, 30)
	g.openScreen("studio", func() {})
	wldFrames(t, g, 4)
	return g, sysAs[*authorScreen](t, g.stack[len(g.stack)-1])
}

func top(t *testing.T, g *Game) *authorScreen {
	t.Helper()
	return sysAs[*authorScreen](t, g.stack[len(g.stack)-1])
}

// Left and right turn the carousel; confirming runs the middle member, and a
// refused member keeps its place but runs nothing.
func TestCarouselTurnsAndRefuses(t *testing.T) {
	g, s := openPartsBoard(t)
	if c := s.ringCenter(); s.entries[c].Label != "Fire" {
		t.Fatal("the first member is not in the middle", s.entries[c].Label)
	}
	wldPlay(t, g, "press right\nwait 4\npress z\nwait 8\n", 40)
	if s := top(t, g); s.entries[s.ringCenter()].Label != "Glaze" || g.st.Vars["glazed"] != nil {
		t.Fatal("a refused member ran or the carousel did not turn", s.entries[s.ringCenter()].Label, g.st.Vars)
	}
	wldPlay(t, g, "press right\nwait 4\npress z\nwait 8\n", 40)
	if g.st.Vars["packed"] != float64(1) {
		t.Fatal("the middle member did not run", g.st.Vars)
	}
	// Turning past the end comes back to the first member.
	wldPlay(t, g, "press right\nwait 4\npress z\nwait 8\n", 40)
	if g.st.Vars["fired"] != float64(1) {
		t.Fatal("the carousel did not wrap around", g.st.Vars)
	}
	// After a refresh the same member stays in the middle.
	if s := top(t, g); s.entries[s.ringCenter()].Label != "Fire" {
		t.Fatal("refresh moved the carousel", s.entries[s.ringCenter()].Label)
	}
}

// A click on a side box turns it to the middle; a click on the middle runs it.
func TestCarouselClicks(t *testing.T) {
	g, s := openPartsBoard(t)
	_, boxes, arrows := s.carouselLayout()
	var side, middle carouselBox
	for _, b := range boxes {
		if b.step == 1 {
			side = b
		}
		if b.step == 0 {
			middle = b
		}
	}
	if side.size == 0 || middle.size <= side.size {
		t.Fatal("the middle box is not the largest", boxes)
	}
	click := func(x, y float64) { wldPlay(t, g, fmt.Sprintf("click %d %d\nwait 8\n", int(x), int(y)), 40) }
	click(side.x+side.size/2, side.y+side.size/2)
	if s := top(t, g); s.entries[s.ringCenter()].Label != "Glaze" || len(g.st.Vars) != 0 && g.st.Vars["glazed"] != nil {
		t.Fatal("a side click ran or did not turn", g.st.Vars)
	}
	click(float64(arrows[1].Min.X+4), float64(arrows[1].Min.Y+4))
	click(middle.x+middle.size/2, middle.y+middle.size/2)
	if g.st.Vars["packed"] != float64(1) {
		t.Fatal("a click on the middle did not run it", g.st.Vars)
	}
	// The band behind the boxes is not a control.
	click(210, 680)
	if g.st.Vars["counted"] != nil || g.st.Vars["opened"] != nil {
		t.Fatal("the empty band ran another control", g.st.Vars)
	}
}

// Up and down treat the carousel as one stop; a script pick never chooses a
// refused member, and look lists it as disabled.
func TestCarouselFocusAndPick(t *testing.T) {
	g, s := openPartsBoard(t)
	var stops []string
	for range 4 {
		wldPlay(t, g, "press down\nwait 2\n", 20)
		s = top(t, g)
		var actions []int
		for i, e := range s.entries {
			if e.Target.ID == "" && len(e.Bounds) == 4 && (len(e.Do) > 0 || e.Destination != "") {
				actions = append(actions, i)
			}
		}
		stops = append(stops, s.entries[actions[s.focus]].Label)
	}
	if strings.Join(stops, ",") != "Bowl glazing,Fire,Count,Bowl glazing" {
		t.Fatal("focus stopped on a carousel member off the middle", stops)
	}
	for _, p := range s.pickables(g) {
		if p.label == "Glaze" && !p.disabled || p.label == "Pack" && p.disabled {
			t.Fatal("pick sees a refused member as enabled", p)
		}
	}
	if err := g.scriptPick(&g.in, "Glaze"); err == nil {
		t.Fatal("pick chose a refused member")
	}
	wldPlay(t, g, "pick Pack\nwait 8\n", 40)
	if g.st.Vars["packed"] != float64(1) {
		t.Fatal("pick did not run the member it chose", g.st.Vars)
	}
}

// Plates, buttons, tickets with a dependency line and the carousel all draw.
