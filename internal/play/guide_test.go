package play

import (
	"bytes"
	"strings"
	"testing"

	data "github.com/nao1215/gozen/internal/layer2/gamedata"
)

const guideBoard = `planning:
  calendar: {start: '2030-01-01', end: '2030-01-09', phases: [morning]}
  entities: {potter: {name: Potter}}
  activities: {glaze: {name: Bowl glazing, effort: 2}}
flows:
  day:
    steps:
    - {label: Look, screen: look_board}
    - {label: Act, screen: act_board}
    restart: 'var.day'
    next: Onward
screens:
  look_board:
    title: Look
    kind: workspace
    view: {fullscreen: true, light: true}
    entries:
    - {label: Hours, style: plate, bounds: [40, 150, 200, 100]}
    - {label: Peek, style: button, bounds: [40, 300, 200, 48], do: [{set: {var.peeked: 1}}]}
    guide:
      steps:
      - {who: guide, text: 'The kiln log first.'}
      - {who: guide, text: 'Then on to the work.', focus: [next]}
  act_board:
    title: Act
    kind: workspace
    view: {fullscreen: true, light: true, header: 'Day', header_face: x.png, header_tint: '#e8a07a'}
    entries:
    - {label: Work, style: button, mark: work, bounds: [40, 150, 200, 48], do: [{set: {var.worked: 'var.worked+1'}}]}
    - {label: Rest, style: button, bounds: [40, 300, 200, 48], do: [{set: {var.rested: 1}}]}
    guide:
      when: 'var.ready > 0'
      steps:
      - {who: scout, text: 'Work it twice.', focus: [work], until: 'var.worked >= 2'}
      - {who: scout, text: 'Once more.', focus: [work]}
`

func openGuideGame(t *testing.T, trace *bytes.Buffer) *Game {
	t.Helper()
	fsys := wldWorldFiles(t, guideBoard, nil)
	fsys["game.yaml"].Data = []byte(strings.Replace(string(fsys["game.yaml"].Data), "schema_version: 4", "schema_version: 10", 1))
	opts := Options{FS: fsys, Location: "camp", Seed: 5}
	if trace != nil {
		opts.Trace = trace
	}
	g := wldOpenWith(t, opts)
	g.guideScripted = true
	wldFrames(t, g, 30)
	return g
}

// A guide shows line by line over its screen the first time: a line without
// focus is pressed to go on, a line with focus refuses every other place and
// goes on when a lit place acts; it never shows again once finished.
func TestGuideLeadsOnceAndBlocksTheRest(t *testing.T) {
	var buf bytes.Buffer
	g := openGuideGame(t, &buf)
	g.openScreen("look_board", func() {})
	wldFrames(t, g, 4)
	g.Draw(covScreen())
	if g.guide == nil || g.guide.screen != "look_board" || g.guide.step != 0 {
		t.Fatal("the guide did not start", g.guide)
	}
	// The Peek button is not lit: a click on it is refused while a line shows.
	wldPlay(t, g, "click 100 320\nwait 4\n", 20)
	if g.guide.step != 1 {
		t.Fatal("a line without focus did not go on when pressed", g.guide.step)
	}
	wldPlay(t, g, "click 100 320\nwait 4\npress cancel\nwait 4\n", 20)
	if g.st.Vars["peeked"] != nil || top(t, g).name != "look_board" {
		t.Fatal("a place not lit, or cancel, was used during the guide", g.st.Vars["peeked"], top(t, g).name)
	}
	if len(traceLines(t, buf.Bytes(), "guide_blocked")) == 0 {
		t.Fatal("the refused press is not in the trace")
	}
	g.Draw(covScreen())
	wldPlay(t, g, "pick Onward\nwait 6\n", 30)
	if top(t, g).name != "act_board" {
		t.Fatal("the lit next action did not go on", top(t, g).name)
	}
	if !g.unlocked[data.GuideKey("look_board")] || g.guide != nil {
		t.Fatal("the finished guide was not recorded", g.unlocked, g.guide)
	}
	// The act board's guide waits for its condition.
	wldPlay(t, g, "pick Rest\nwait 6\n", 30)
	if g.guide != nil || g.st.Vars["rested"] == nil {
		t.Fatal("a held-back guide showed or blocked the screen", g.guide)
	}
	g.st.Vars["ready"] = 1.0
	wldFrames(t, g, 4)
	g.Draw(covScreen())
	if g.guide == nil || g.guide.screen != "act_board" {
		t.Fatal("the guide did not start once its condition held")
	}
	// until: the lit place is used twice before the line goes on.
	wldPlay(t, g, "pick Work\nwait 6\n", 30)
	if g.guide == nil || g.guide.step != 0 {
		t.Fatal("a line with until went on before it held", g.guide)
	}
	wldPlay(t, g, "pick Rest\nwait 6\npick Work\nwait 6\n", 40)
	if g.in.script.pc != 0 || g.st.Vars["worked"] != 1.0 {
		t.Fatal("restricted pick or its successor ran")
	}
	wldPlay(t, g, "pick Work\nwait 6\n", 30)
	if g.guide == nil || g.guide.step != 1 || g.st.Vars["worked"] != 2.0 {
		t.Fatal("until did not end the line", g.guide, g.st.Vars["worked"])
	}
	wldPlay(t, g, "pick Work\nwait 6\n", 30)
	if g.guide != nil || !g.unlocked[data.GuideKey("act_board")] {
		t.Fatal("the last lit action did not finish the guide", g.guide)
	}
	lines := traceLines(t, buf.Bytes(), "guide")
	if len(lines) != 4 || lines[0]["text"] != "The kiln log first." || lines[2]["who"] != "scout" {
		t.Fatal("the guide's lines in the trace", lines)
	}
	// Seen guides do not show again, here or in a later new game of this player.
	g.pop()
	g.openScreen("look_board", func() {})
	wldFrames(t, g, 6)
	if g.guide != nil {
		t.Fatal("a finished guide showed again")
	}
	g.newGame()
	wldFrames(t, g, 6)
	g.openScreen("look_board", func() {})
	wldFrames(t, g, 6)
	if g.guide != nil {
		t.Fatal("a finished guide showed in a new game")
	}
}

// Scripted plays start as a player who has seen the guides.
func TestGuideOffInScriptsByDefault(t *testing.T) {
	g := openGuideGame(t, nil)
	g.guideScripted = false
	g.openScreen("look_board", func() {})
	wldFrames(t, g, 6)
	if g.guide != nil {
		t.Fatal("a guide showed in a scripted play without guides on")
	}
	wldPlay(t, g, "guides on\nwait 2\n", 10)
	g.pop()
	g.openScreen("look_board", func() {})
	wldFrames(t, g, 6)
	if g.guide == nil {
		t.Fatal("guides on did not show the guide")
	}
}

// Real plays persist guide completion outside save slots. Scripted plays keep
// their existing opt-in and do not write gallery progress.
func TestGuideCompletionReloadsFromGallery(t *testing.T) {
	g := openGuideGame(t, nil)
	g.openScreen("look_board", func() {})
	wldPlay(t, g, "wait 4\npress z\nwait 4\npick Onward\nwait 6\n", 60)
	g.opts.Script = ""
	g.saveUnlocks()
	if _, err := g.storeRead("gallery.json"); err != nil {
		t.Fatal(err)
	}
	opts := g.opts
	opts.Script = "wait 1000000000\n"
	if err := g.Close(); err != nil {
		t.Fatal(err)
	}
	next := wldOpenWith(t, opts)
	next.guideScripted = true
	wldFrames(t, next, 30)
	next.openScreen("look_board", func() {})
	wldFrames(t, next, 6)
	if next.guide != nil || !next.unlocked[data.GuideKey("look_board")] {
		t.Fatal("completed guide reappeared after reopening the gallery")
	}
}

func TestInteractiveFirstVisitGuideIsTraced(t *testing.T) {
	files := wldWorldFiles(t, guideBoard, nil)
	files["game.yaml"].Data = []byte(strings.Replace(string(files["game.yaml"].Data), "schema_version: 4", "schema_version: 10", 1))
	var buf bytes.Buffer
	g, err := New(Options{FS: files, NoSound: true, Seed: 7, Trace: &buf, SaveDir: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := g.Close(); err != nil {
			t.Error(err)
		}
	})
	g.newGame()
	g.openScreen("look_board", func() {})
	wldFrames(t, g, 4)
	if g.testRun() || g.guideScripted || g.guide == nil {
		t.Fatal("interactive first visit did not show its default guide")
	}
	if len(traceLines(t, buf.Bytes(), "guide")) == 0 || len(traceLines(t, buf.Bytes(), "act")) != 0 {
		t.Fatalf("interactive guide must be detectable without a guides action: %s", &buf)
	}
	// Replays keep the ADR 0055 default, even for this same screen.
	g.rep = &replay{}
	if g.guidesOn() {
		t.Fatal("replay unexpectedly enabled guides")
	}
	g.rep = nil
}
