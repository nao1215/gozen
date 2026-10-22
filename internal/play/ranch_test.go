package play

import (
	"math"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/nao1215/gozen/internal/event"
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
	g.in.pressed[btnOK] = true
	r.update(g)
	g.in.pressed[btnOK] = false
	c, ok := g.stack[len(g.stack)-1].(*confirm)
	if !ok || !strings.Contains(c.msg, "?") {
		t.Fatalf("hidden preview = %T %+v", g.stack[len(g.stack)-1], c)
	}
	g.pop()
	c.yes()
	var birthWait bool
	var birthLines []string
	for i := 0; g.events.Running() && i < 20; i++ {
		switch req := g.events.Next().(type) {
		case event.Wait:
			birthWait = math.Abs(req.Seconds-float64(birthDuration+birthCutInFrames)/60) < 1e-9
		case event.Say:
			birthLines = append(birthLines, req.Text)
		}
	}
	if g.st.Actors[a] != nil || g.st.Actors[b] != nil || g.st.Vars["bestiary.boss.caught"] != float64(1) {
		t.Fatalf("fusion state = %+v", g.st.Actors)
	}
	if !birthWait || g.birth == nil || g.birth.effect.ChildName == "" || len(birthLines) != 2 || !strings.Contains(birthLines[0], "Inherited skills: Spark") || !strings.Contains(birthLines[1], "Breeding strength: +") {
		t.Fatalf("birth presentation: wait=%v effect=%+v lines=%v", birthWait, g.birth, birthLines)
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
		if actions.panelX+actions.panelW+100 >= actions.figureX {
			t.Fatalf("no visible altar between menu and Yorn at %.0fx%.0f: %+v", sw, sh, actions)
		}
		if actions.figureH < sh*0.9 || actions.figureBottom > sh || actions.figureBottom-actions.figureH < 0 {
			t.Fatalf("Yorn does not fit near full height at %.0fx%.0f: %+v", sw, sh, actions)
		}
		if selection.panelX+selection.panelW+100 >= selection.figureX || selection.panelY+selection.panelH > sh {
			t.Fatalf("selection obscures Yorn or leaves screen at %.0fx%.0f: %+v", sw, sh, selection)
		}
	}
}

func TestRanchActionsUseTalkWindow(t *testing.T) {
	g := ranchGame(t)
	r := newRanch(g, nil)
	if len(r.options) != 4 || r.options[3] != "leave" {
		t.Fatalf("actions = %v", r.options)
	}
	if r.talk.say.Name != "Yorn" || !strings.Contains(r.talk.say.Text, "two monsters") {
		t.Fatalf("ranch speaker = %+v", r.talk.say)
	}
	_, by, _, _ := g.talkBox()
	if r.list.y+r.list.height() >= by || r.list.x+r.list.w >= ranchScreenLayout(1280, 720, "actions", 4).figureX {
		t.Fatalf("choice box hides the talk or Yorn: %+v", r.list)
	}
}

func TestBirthTimelineSkipAndPositions(t *testing.T) {
	g := ranchGame(t)
	g.Handle(event.BreedEffect{ParentNames: [2]string{"Mote ♂", "Mote ♀"}, ChildName: "Boss ♀"})
	for _, tc := range []struct {
		age   int
		phase birthPhase
	}{{0, birthParents}, {64, birthParents}, {65, birthSpiral}, {167, birthSpiral}, {168, birthFlash}, {199, birthFlash}, {200 + birthCutInFrames, birthChild}, {birthDuration + birthCutInFrames, birthChild}} {
		g.birth.age = tc.age
		if got := g.birth.phase(); got != tc.phase {
			t.Fatalf("age %d: phase %d, want %d", tc.age, got, tc.phase)
		}
	}
	for _, size := range [][2]float64{{1280, 720}, {1920, 1080}} {
		parents, crystal := birthPositions(size[0], size[1])
		if !(parents[0].x < crystal.x && crystal.x < parents[1].x) || parents[0].y != parents[1].y || crystal.y >= parents[0].y {
			t.Fatalf("birth positions at %v: %+v %+v", size, parents, crystal)
		}
		for i, p := range parents {
			start := birthOrbPosition(size[0], size[1], i, 65, 168)
			end := birthOrbPosition(size[0], size[1], i, 168, 168)
			if math.Abs(start.x-p.x) > 0.01 || math.Abs(start.y-(p.y-size[1]*0.1)) > 0.01 || math.Abs(end.x-crystal.x) > 0.01 || math.Abs(end.y-crystal.y) > 0.01 {
				t.Fatalf("orb %d does not travel from parent to crystal at %v: start=%+v end=%+v", i, size, start, end)
			}
		}
	}
	g.req = event.Wait{Seconds: float64(birthDuration+birthCutInFrames) / 60}
	g.waitT = birthDuration + birthCutInFrames
	g.in.pressed[btnOK] = true
	g.pump()
	if g.waitT != 0 || g.req != nil || g.birth.age != birthDuration+birthCutInFrames {
		t.Fatalf("skip left birth pending: wait=%d req=%T age=%d", g.waitT, g.req, g.birth.age)
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
	defer func() { _ = g.Close() }()
	for i := 0; i < 10000; i++ {
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
