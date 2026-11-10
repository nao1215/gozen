package play

import (
	"math"
	"slices"
	"strings"
	"testing"
	"testing/fstest"

	data "github.com/nao1215/gozen/internal/model/gamedata"
	"github.com/nao1215/gozen/internal/rules/event"
	"github.com/nao1215/gozen/internal/testgame"
)

func ranchGame(t *testing.T) *Game {
	t.Helper()
	fsys := gameFiles(t)
	for path, body := range testgame.MonsterFiles() {
		fsys[path] = &fstest.MapFile{Data: []byte(body)}
	}
	fsys["game.yaml"] = &fstest.MapFile{Data: []byte(testgame.MonsterFiles()["game.yaml"] + "\nfonts: {regular: test.ttf}\n")}
	g, err := New(Options{FS: fsys, NoSound: true, Location: "camp", SaveDir: t.TempDir(), ShotDir: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := g.Close(); err != nil {
			t.Error(err)
		}
	})
	return g
}

func TestRanchPreviewAndFusionCommand(t *testing.T) {
	g := ranchGame(t)
	a, _ := g.st.Recruit("mote", 2, 0, nil)
	b, _ := g.st.Recruit("mote", 2, 0, nil)
	g.st.Actors[a].Sex, g.st.Actors[b].Sex = "male", "female"
	r := newRanch(g, nil)
	g.push(r)
	r.show(g, "breed_first")
	if len(r.options) != 2 {
		t.Fatalf("parents = %v", r.options)
	}
	r.list.sel = 0
	g.in.pressed[btnOK] = true
	r.update(g)
	g.in.pressed[btnOK] = false
	if r.first != a || r.step != "breed_second" {
		t.Fatalf("first %s step %s", r.first, r.step)
	}
	r.list.sel = 0
	g.in.pressed[btnOK] = true
	r.update(g)
	g.in.pressed[btnOK] = false
	if r.second != b || r.step != "skills" {
		t.Fatalf("second %s step %s", r.second, r.step)
	}
	if !strings.Contains(strings.Join(r.options, ","), "fire") {
		t.Fatalf("inheritable skills = %v", r.options)
	}
	for i, id := range r.options {
		if id == "fire" {
			r.list.sel = i
			break
		}
	}
	g.in.pressed[btnOK] = true
	r.update(g)
	g.in.pressed[btnOK] = false
	if len(r.skills) != 1 || r.skills[0] != "fire" {
		t.Fatalf("chosen skills = %v", r.skills)
	}
	r.list.sel = len(r.options) - 1
	// The fusion's row names the child (hidden until the book has seen it)
	// and what is lost; choosing it is the one answer the ranch asks for.
	if row := r.list.items[len(r.list.items)-1].label; !strings.Contains(row, "?") || !strings.Contains(row, "parents leave") {
		t.Fatalf("the fusion's row = %q", row)
	}
	depth := len(g.stack)
	g.in.pressed[btnOK] = true
	r.update(g)
	g.in.pressed[btnOK] = false
	if len(g.stack) != depth-1 || !g.events.Running() {
		t.Fatalf("choosing the fusion asked again: top %T", g.stack[len(g.stack)-1])
	}
	var birthWait bool
	var birthLines []string
	for i := 0; g.events.Running() && i < 20; i++ {
		switch req := g.events.Next().(type) {
		case event.Wait:
			birthWait = math.Abs(req.Seconds-float64(birthDuration)/60) < 1e-9
		case event.Say:
			birthLines = append(birthLines, req.Text)
		}
	}
	if g.st.Actors[a] != nil || g.st.Actors[b] != nil || g.st.Vars["bestiary.boss.caught"] != float64(1) {
		t.Fatalf("fusion state = %+v", g.st.Actors)
	}
	if !birthWait || g.birth == nil || g.birth.effect.ChildName == "" || len(birthLines) != 0 || !strings.Contains(g.birth.effect.Summary, "Inherited skills: Spark") {
		t.Fatalf("birth presentation: wait=%v effect=%+v lines=%v", birthWait, g.birth, birthLines)
	}
	if !g.birth.effect.FirstDiscovery || !strings.Contains(g.birth.effect.Result, "Skills: Spark") || !strings.Contains(g.birth.effect.Result, "Lv 1") {
		t.Fatalf("first discovery and result were not combined: %+v", g.birth.effect)
	}
}

func TestRanchLayoutLeavesAltarVisible(t *testing.T) {
	for _, screen := range [][2]float64{{960, 540}, {1280, 720}} {
		sw, sh := screen[0], screen[1]
		actions := ranchScreenLayout(sw, sh, "actions", 3)
		selection := ranchScreenLayout(sw, sh, "breed_first", 8)
		if actions.panelW > 510 || actions.panelH >= sh*0.65 {
			t.Fatalf("actions panel is too large at %.0fx%.0f: %+v", sw, sh, actions)
		}
		faceLeft := actions.figureX - sh*0.05
		if actions.panelX+actions.panelW >= faceLeft || selection.panelX+selection.panelW >= faceLeft {
			t.Fatalf("ranch panel hides the keeper's face at %.0fx%.0f: actions=%+v selection=%+v", sw, sh, actions, selection)
		}
		// Yorn is shown from the waist up: his feet fall below the screen, his head stays on it.
		if actions.figureH < sh || actions.figureBottom <= sh || actions.figureBottom-actions.figureH < 0 {
			t.Fatalf("Yorn is not shown waist-up at %.0fx%.0f: %+v", sw, sh, actions)
		}
		if selection.panelY+selection.panelH > sh {
			t.Fatalf("selection leaves screen at %.0fx%.0f: %+v", sw, sh, selection)
		}
	}
}

func TestRanchActionsUseTalkWindow(t *testing.T) {
	g := ranchGame(t)
	r := newRanch(g, nil)
	if len(r.options) != 4 || r.options[3] != "leave" {
		t.Fatalf("actions = %v", r.options)
	}
	// The engine names nobody: without breeding.keeper the line is neutral.
	if r.talk.say.Name != "" || !strings.Contains(r.talk.say.Text, "two monsters") {
		t.Fatalf("ranch speaker = %+v", r.talk.say)
	}
	g.db.Breeding.Keeper = data.BreedKeeper{Name: "Mara", Text: "Welcome back."}
	if say := newRanch(g, nil).talk.say; say.Name != "Mara" || say.Text != "Welcome back." {
		t.Fatalf("the game's keeper = %+v", say)
	}
	_, by, _, _ := g.talkBox()
	if r.list.y+r.list.height() >= by || r.list.x+r.list.w >= ranchScreenLayout(1280, 720, "actions", 4).figureX-720*0.05 || by <= 720*0.3 {
		t.Fatalf("choice box hides the talk or the keeper: %+v", r.list)
	}
}

func TestScriptedEventBattleReachesRecruitChoice(t *testing.T) {
	fsys := gameFiles(t)
	for path, body := range testgame.MonsterFiles() {
		if path == "data/main.yaml" {
			body = strings.Replace(body, "recruit: {chance: 1,", "recruit: {chance: 0.001,", 1)
		}
		fsys[path] = &fstest.MapFile{Data: []byte(body)}
	}
	fsys["game.yaml"] = &fstest.MapFile{Data: []byte(testgame.MonsterFiles()["game.yaml"] + "\nfonts: {regular: test.ttf}\n")}
	g, err := New(Options{
		FS: fsys, NoSound: true, Development: true, Location: "camp", Level: 6, Seed: 7,
		SaveDir: t.TempDir(), ShotDir: t.TempDir(),
		Script: "guarantee-recruit mote\ndo battle: one\nplay join 5\nwait 4\nquit\n",
	})
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := g.Close(); err != nil {
			t.Error(err)
		}
	}()
	for range 10000 {
		if err := g.Update(); err != nil {
			t.Fatal(err)
		}
		if g.in.script.pc >= 3 {
			if !g.talk.active || g.talk.choice == nil || !strings.Contains(g.talk.asked.Prompt, "Mote") || g.recruit == nil || g.recruit.offer.Image == "" || g.recruit.leave != 0 {
				t.Fatalf("script stopped before recruit choice: pc=%d, waiting=%v", g.in.script.pc, g.talk.choice)
			}
			if !strings.Contains(g.talk.say.Name, "♂") && !strings.Contains(g.talk.say.Name, "♀") {
				t.Fatalf("recruit name lacks sex sign: %q", g.talk.say.Name)
			}
			return
		}
	}
	t.Fatal("script never reached the recruit choice")
}

// TestRanchReleaseRequiresConfirmation: a release cannot be taken back, so it
// asks first; No leaves the roster and Yes closes the ranch once.
func TestRanchReleaseRequiresConfirmation(t *testing.T) {
	g := ranchGame(t)
	g.db.Game.MaxParty = len(g.st.Party)
	id, ok := g.st.Recruit("mote", 2, 0, nil)
	if !ok || !slices.Contains(g.st.ReserveIDs(), id) {
		t.Fatal("fixture did not recruit into reserve")
	}
	out := g.st.Party[0]
	closed := 0
	r := newRanch(g, func() { closed++ })
	g.push(r)
	depth := len(g.stack)
	r.draw(g, covScreen())
	g.in = input{pressed: [numButtons]bool{btnOK: true}}
	r.list.sel = 2
	r.update(g)
	if r.step != "release_id" {
		t.Fatal("wrong action selected", r.step)
	}
	r.draw(g, covScreen())
	r.update(g)
	c, ok := g.stack[len(g.stack)-1].(*confirm)
	if !ok || closed != 0 || g.st.Actors[id] == nil || g.st.Party[0] != out {
		t.Fatal("selection applied before confirmation")
	}
	g.in = input{pressed: [numButtons]bool{btnCancel: true}}
	for range 6 {
		c.update(g)
	}
	if len(g.stack) != depth || closed != 0 || g.st.Actors[id] == nil || g.st.Party[0] != out {
		t.Fatal("cancel changed roster")
	}
	g.in = input{pressed: [numButtons]bool{btnOK: true}}
	r.update(g)
	c = sysAs[*confirm](t, g.stack[len(g.stack)-1])
	for range 6 {
		c.update(g)
	}
	if len(g.stack) != depth-1 || closed != 1 {
		t.Fatal("confirmation did not close the ranch once")
	}
	for i := 0; g.events.Running() && i < 10; i++ {
		g.events.Next()
	}
	if g.st.Actors[id] != nil || g.st.Party[0] != out {
		t.Fatal("release changed the wrong actor")
	}
}

// TestRanchSwapsAtOnce: choosing the party member to send out swaps without a
// question, and the notice it leaves puts the two back when pressed.
func TestRanchSwapsAtOnce(t *testing.T) {
	g := ranchGame(t)
	g.db.Game.MaxParty = len(g.st.Party)
	id, ok := g.st.Recruit("mote", 2, 0, nil)
	if !ok || !slices.Contains(g.st.ReserveIDs(), id) {
		t.Fatal("fixture did not recruit into reserve")
	}
	out := g.st.Party[0]
	closed := 0
	r := newRanch(g, func() { closed++ })
	g.push(r)
	depth := len(g.stack)
	g.in = input{pressed: [numButtons]bool{btnOK: true}}
	r.update(g) // swap
	r.update(g) // the reserve monster
	if r.step != "swap_out" || r.first != id {
		t.Fatalf("reserve selection lost: step %s first %s", r.step, r.first)
	}
	r.update(g) // the party member: the swap itself
	if len(g.stack) != depth-1 || closed != 1 {
		t.Fatalf("the second monster did not swap at once: top %T, closed %d", g.stack[len(g.stack)-1], closed)
	}
	for i := 0; g.events.Running() && i < 10; i++ {
		g.events.Next()
	}
	if g.st.Party[0] != id || !slices.Contains(g.st.ReserveIDs(), out) {
		t.Fatal("swap did not exchange party and reserve")
	}
	g.in = input{}
	undo := noticeOffer(t, g)
	undo.act()
	for i := 0; g.events.Running() && i < 10; i++ {
		g.events.Next()
	}
	if g.st.Party[0] != out || !slices.Contains(g.st.ReserveIDs(), id) {
		t.Fatal("the notice did not put the two back")
	}
}
