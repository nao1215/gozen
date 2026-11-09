package play

import (
	"bytes"
	"math/rand/v2"
	"strings"
	"testing"

	"github.com/nao1215/gozen/internal/layer1/traceformat"
	"github.com/nao1215/gozen/internal/rpg"
)

const flowBoard = `planning:
  calendar: {start: '2030-01-01', end: '2030-01-09', phases: [morning]}
  entities: {potter: {name: Potter}}
  activities: {glaze: {name: Bowl glazing, effort: 2}}
flows:
  day:
    steps:
    - {label: Look, screen: look_board, enter: brief}
    - {label: People, screen: people_board}
    - {label: Act, screen: act_board}
    restart: 'var.day'
    skip: 'var.skip > 0'
    next: Onward
screens:
  look_board:
    title: Look
    kind: workspace
    view: {fullscreen: true, light: true}
    entries: [{label: Hours, style: plate, bounds: [40, 150, 200, 100]}]
  people_board:
    title: People
    kind: workspace
    view: {fullscreen: true, light: true}
    entries: [{label: Staff, style: plate, bounds: [40, 150, 200, 100]}]
  act_board:
    title: Act
    kind: workspace
    view: {fullscreen: true, light: true}
    entries: [{label: Work, style: button, bounds: [40, 150, 200, 48], do: [{set: {var.worked: 1}}]}]
`

func openFlowGame(t *testing.T, trace *bytes.Buffer) *Game {
	t.Helper()
	fsys := wldWorldFiles(t, flowBoard, map[string]string{"data/flow_events.yaml": "events:\n  brief:\n    do:\n    - {set: {var.briefed: 'var.briefed+1'}}\n    - say: Late work ahead\n"})
	fsys["game.yaml"].Data = []byte(strings.Replace(string(fsys["game.yaml"].Data), "schema_version: 4", "schema_version: 10", 1))
	opts := Options{FS: fsys, Location: "camp"}
	if trace != nil {
		opts.Trace, opts.Seed = trace, 5
	}
	g := wldOpenWith(t, opts)
	wldFrames(t, g, 30)
	return g
}

// A step not reached opens the current step; next finishes a step and goes
// on; a finished step is pressed to go back.
func TestFlowGoesInOrder(t *testing.T) {
	var buf bytes.Buffer
	g := openFlowGame(t, &buf)
	g.openScreen("act_board", func() {})
	wldFrames(t, g, 4)
	if s := top(t, g); s.name != "look_board" {
		t.Fatal("a step not reached opened", s.name)
	}
	g.Draw(covScreen())
	wldPlay(t, g, "press z\nwait 6\npress z\nwait 10\npick Onward\nwait 4\nlook\nwait 2\n", 120)
	if s := top(t, g); s.name != "people_board" || len(g.stack) != 2 {
		t.Fatal("next did not replace the screen with the next step", s.name, len(g.stack))
	}
	looks := traceLines(t, buf.Bytes(), "look")
	l := looks[len(looks)-1]
	if l["flow"] != "day" || l["step"] != 2.0 || !strings.Contains(strings.Join(traceformat.Strings(l["disabled"]), ","), "3 Act") {
		t.Fatal("look does not show the step or the locked step", l)
	}
	if err := g.scriptPick(&g.in, "3 Act"); err == nil {
		t.Fatal("pick went to a step not reached")
	}
	g.Draw(covScreen())
	stops := top(t, g).flowStops(g)
	wldPlay(t, g, "click "+itoa(stops[0].r.Min.X+8)+" "+itoa(stops[0].r.Min.Y+8)+"\nwait 4\n", 30)
	if s := top(t, g); s.name != "look_board" {
		t.Fatal("a finished step did not open", s.name)
	}
	g.pop()
	g.openScreen("act_board", func() {})
	wldFrames(t, g, 4)
	if s := top(t, g); s.name != "people_board" {
		t.Fatal("the flow did not open its current step", s.name)
	}
	// The keys reach next too.
	for range 4 {
		if s := top(t, g); s.focus == len(s.surfaceActions())+len(s.flowStops(g))-1 {
			break
		}
		wldPlay(t, g, "press down\nwait 2\n", 20)
	}
	wldPlay(t, g, "press z\nwait 4\n", 20)
	if s := top(t, g); s.name != "act_board" {
		t.Fatal("OK on next did not go on", s.name)
	}
	if len(top(t, g).flowStops(g)) != 3 {
		t.Fatal("the last step has a next action")
	}
}

// A new restart value starts again from the first step; skip opens any step;
// the progress is saved with the game.
func TestFlowRestartSkipAndSave(t *testing.T) {
	g := openFlowGame(t, nil)
	g.openScreen("look_board", func() {})
	wldPlay(t, g, "wait 4\npress z\nwait 6\npress z\nwait 10\npick Onward\nwait 4\n", 120)
	b, err := g.st.Save()
	if err != nil {
		t.Fatal(err)
	}
	loaded, err := rpg.Parse(g.db, b, fixedRand())
	if err != nil || loaded.Flows["day"].Reached != 1 {
		t.Fatal("the progress was not saved", err, loaded.Flows)
	}
	g.st.Vars["day"] = float64(2)
	s := top(t, g)
	s.refresh(g)
	if s := top(t, g); s.name != "look_board" {
		t.Fatal("a new restart value did not start again", s.name)
	}
	wldPlay(t, g, "wait 4\npress z\nwait 6\npress z\nwait 10\n", 120)
	g.st.Vars["skip"] = float64(1)
	g.pop()
	g.openScreen("act_board", func() {})
	wldFrames(t, g, 4)
	if s := top(t, g); s.name != "act_board" {
		t.Fatal("skip did not open the step", s.name)
	}
}

// A step's enter event runs over its screen once per restart value, its lines
// show to look as talk, and the record of it is saved.
func TestFlowEnterRunsOnce(t *testing.T) {
	var buf bytes.Buffer
	g := openFlowGame(t, &buf)
	g.openScreen("look_board", func() {})
	wldPlay(t, g, "wait 6\nlook\nwait 2\n", 30)
	looks := traceLines(t, buf.Bytes(), "look")
	if l := looks[len(looks)-1]; l["on"] != "talk" || l["text"] != "Late work ahead" {
		t.Fatal("the enter event does not show to look", l)
	}
	wldPlay(t, g, "press z\nwait 6\npress z\nwait 10\n", 120)
	if s := top(t, g); s.name != "look_board" || s.running || g.st.Vars["briefed"] != float64(1) {
		t.Fatal("the enter event did not run over the step", s.name, s.running, g.st.Vars)
	}
	g.pop()
	g.openScreen("look_board", func() {})
	wldFrames(t, g, 10)
	if g.st.Vars["briefed"] != float64(1) {
		t.Fatal("the enter event ran twice", g.st.Vars)
	}
	b, err := g.st.Save()
	if err != nil {
		t.Fatal(err)
	}
	if loaded, err := rpg.Parse(g.db, b, fixedRand()); err != nil || len(loaded.Flows["day"].Entered) != 1 {
		t.Fatal("the entered steps were not saved", err)
	}
	g.st.Vars["day"] = float64(3)
	top(t, g).refresh(g)
	wldPlay(t, g, "wait 4\npress z\nwait 6\npress z\nwait 10\n", 120)
	if g.st.Vars["briefed"] != float64(2) {
		t.Fatal("a new restart value did not run the enter event again", g.st.Vars)
	}
}

// fixedRand is a seeded generator for reading a save back.
func fixedRand() *rand.Rand {
	return rand.New(rand.NewPCG(1, 2)) //nolint:gosec // A fixed seed for a test, not for security.
}

const flowHoldBoard = `planning:
  calendar: {start: '2030-01-01', end: '2030-01-09', phases: [morning]}
  entities: {potter: {name: Potter}}
  activities: {glaze: {name: Bowl glazing, effort: 2}}
flows:
  day:
    steps:
    - {label: Look, screen: look_board}
    - {label: People, screen: people_board, when: 'var.closed == 0'}
    - {label: Work, screen: act_board, hold: true, done: 'var.worked > 0'}
    - {label: Rest, screen: rest_board}
    restart: 'var.day'
    next: Onward
screens:
  look_board:
    title: Look
    kind: workspace
    view: {fullscreen: true, light: true}
    entries: [{label: Hours, style: plate, bounds: [40, 150, 200, 100]}]
  people_board:
    title: People
    kind: workspace
    view: {fullscreen: true, light: true}
    entries: [{label: Staff, style: plate, bounds: [40, 150, 200, 100]}]
  act_board:
    title: Act
    kind: workspace
    view: {fullscreen: true, light: true}
    entries: [{label: Work, style: button, bounds: [40, 150, 200, 48], do: [{set: {var.worked: 1}}]}]
  rest_board:
    title: Rest
    kind: workspace
    view: {fullscreen: true, light: true}
    entries: [{label: Sleep, style: button, bounds: [40, 150, 200, 48], do: [{set: {var.day: 'var.day+1', var.worked: 0}}]}]
`

func openFlowHoldGame(t *testing.T) *Game {
	t.Helper()
	fsys := wldWorldFiles(t, flowHoldBoard, nil)
	fsys["game.yaml"].Data = []byte(strings.Replace(string(fsys["game.yaml"].Data), "schema_version: 4", "schema_version: 10", 1))
	g := wldOpenWith(t, Options{FS: fsys, Location: "camp"})
	wldFrames(t, g, 30)
	return g
}

// A held step has no next action; its done finishes it, the screen gives way
// to the next step, and it and the steps before it do not open again until a
// new restart value; then the flow starts again from its first step.
func TestFlowHeldStep(t *testing.T) {
	g := openFlowHoldGame(t)
	g.openScreen("look_board", func() {})
	wldPlay(t, g, "wait 4\npick Onward\nwait 4\npick Onward\nwait 4\n", 60)
	if s := top(t, g); s.name != "act_board" {
		t.Fatal("next did not reach the held step", s.name)
	}
	g.Draw(covScreen())
	for _, st := range top(t, g).flowStops(g) {
		if st.next {
			t.Fatal("a held step has a next action")
		}
	}
	wldPlay(t, g, "pick Work\nwait 6\n", 60)
	if s := top(t, g); s.name != "rest_board" || len(g.stack) != 2 {
		t.Fatal("the finished held step did not give way", s.name, len(g.stack))
	}
	for _, name := range []string{"act_board", "look_board"} {
		g.pop()
		g.openScreen(name, func() {})
		wldFrames(t, g, 4)
		if s := top(t, g); s.name != "rest_board" {
			t.Fatal("a step at or before a finished held step opened", name, s.name)
		}
	}
	for _, st := range top(t, g).flowStops(g) {
		if st.enabled {
			t.Fatal("a step at or before a finished held step can be pressed", st.label)
		}
	}
	// A step screen uncovered after its held step finished gives way too.
	g.pop()
	g.st.Vars["worked"] = float64(0)
	g.st.Vars["day"] = float64(5)
	g.openScreen("look_board", func() {})
	wldPlay(t, g, "wait 4\npick Onward\nwait 4\npick Onward\nwait 4\n", 60)
	g.openScreen("rest_board", func() {})
	wldFrames(t, g, 4)
	g.st.Vars["worked"] = float64(1)
	g.pop()
	wldFrames(t, g, 4)
	if s := top(t, g); s.name != "rest_board" {
		t.Fatal("an uncovered finished held step did not give way", s.name)
	}
	wldPlay(t, g, "pick Sleep\nwait 6\n", 60)
	if s := top(t, g); s.name != "look_board" {
		t.Fatal("a new restart value did not give way to the first step", s.name)
	}
}

// A step left out by its when is passed over and not shown in the band; the
// band numbers the steps shown.
func TestFlowStepWhen(t *testing.T) {
	g := openFlowHoldGame(t)
	g.st.Vars["closed"] = float64(1)
	g.openScreen("people_board", func() {})
	wldFrames(t, g, 4)
	if s := top(t, g); s.name != "look_board" {
		t.Fatal("a left-out step opened", s.name)
	}
	g.Draw(covScreen())
	var labels []string
	for _, st := range top(t, g).flowStops(g) {
		if !st.next {
			labels = append(labels, st.label)
		}
	}
	if strings.Join(labels, ",") != "1 Look,2 Work,3 Rest" {
		t.Fatal("the band shows a left-out step", labels)
	}
	wldPlay(t, g, "pick Onward\nwait 4\n", 30)
	if s := top(t, g); s.name != "act_board" {
		t.Fatal("next did not pass over the left-out step", s.name)
	}
}

// A flow entered under a new restart value passes over the first steps that
// are already finished by their done (the day entered after the morning).
func TestFlowNewValueStartsPastDoneSteps(t *testing.T) {
	board := strings.Replace(flowHoldBoard, "- {label: Look, screen: look_board}", "- {label: Look, screen: look_board, done: 'var.worked > 0'}", 1)
	fsys := wldWorldFiles(t, board, nil)
	fsys["game.yaml"].Data = []byte(strings.Replace(string(fsys["game.yaml"].Data), "schema_version: 4", "schema_version: 10", 1))
	g := wldOpenWith(t, Options{FS: fsys, Location: "camp"})
	wldFrames(t, g, 30)
	g.st.Vars["closed"] = float64(1)
	g.st.Vars["worked"] = float64(1)
	g.openScreen("look_board", func() {})
	wldFrames(t, g, 4)
	if s := top(t, g); s.name != "rest_board" {
		t.Fatal("a new restart value did not pass over the finished steps", s.name)
	}
}

// Automatic completion survives a falling done condition without an enter event.
func TestFlowAutomaticCompletionPersists(t *testing.T) {
	g := openFlowHoldGame(t)
	g.openScreen("look_board", func() {})
	wldPlay(t, g, "wait 4\npick Onward\nwait 4\npick Onward\nwait 4\npick Work\nwait 6\n", 100)
	if p := g.st.Flows["day"]; p.Reached != 3 {
		t.Fatalf("saved progress: %+v", p)
	}
	g.st.Vars["worked"] = float64(0)
	if got := g.flowOpen("act_board"); got != "rest_board" {
		t.Fatalf("completed held step reopened: %s", got)
	}
}
