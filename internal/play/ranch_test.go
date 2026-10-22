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
			birthWait = math.Abs(req.Seconds-float64(birthDuration)/60) < 1e-9
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
		if actions.panelX+actions.panelW+100 >= actions.figureX {
			t.Fatalf("no visible altar between menu and Yorn at %.0fx%.0f: %+v", sw, sh, actions)
		}
		// Yorn is shown from the waist up: his feet fall below the screen, his head stays on it.
		if actions.figureH < sh || actions.figureBottom <= sh || actions.figureBottom-actions.figureH < 0 {
			t.Fatalf("Yorn is not shown waist-up at %.0fx%.0f: %+v", sw, sh, actions)
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
	for tier, duration := range []int{288, 324, 360} {
		g.Handle(event.BreedEffect{ParentNames: [2]string{"Mote ♂", "Mote ♀"}, ChildName: "Boss ♀", Tier: tier})
		b := g.birth
		if b.duration() != duration {
			t.Fatalf("tier %d duration %d", tier, b.duration())
		}
		for _, tc := range []struct {
			age   int
			phase birthPhase
		}{
			{0, birthParents},
			{29, birthParents},
			{30, birthYornExit},
			{53, birthYornExit},
			{54, birthFade},
			{89, birthFade},
			{90, birthMerge},
			{119, birthMerge},
			{120, birthSilhouette},
			{b.silhouetteEnd() - 1, birthSilhouette},
			{b.silhouetteEnd(), birthReveal},
			{b.revealEnd() - 1, birthReveal},
			{b.revealEnd(), birthView},
			{b.viewEnd() - 1, birthView},
			{b.viewEnd(), birthResult},
			{b.duration(), birthResult},
		} {
			b.age = tc.age
			if got := b.phase(); got != tc.phase {
				t.Fatalf("tier %d age %d: %d != %d", tier, tc.age, got, tc.phase)
			}
		}
	}
	for _, size := range [][2]float64{{1280, 720}, {1920, 1080}} {
		sw, sh := size[0], size[1]
		parents, crystal := birthPositions(sw, sh)
		if parents[0] != (birthPoint{sw * 0.22, sh * 0.78}) || parents[1] != (birthPoint{sw * 0.78, sh * 0.78}) || crystal != (birthPoint{sw * 0.5, sh * 0.52}) {
			t.Fatalf("positions at %v: %v %v", size, parents, crystal)
		}
		x, feet, h, w := birthChildFrame(sw, sh)
		if x != sw*0.5 || feet != sh*0.82 || h != sh*0.62 || w != sw*0.58 {
			t.Fatalf("child frame at %v: %v %v %v %v", size, x, feet, h, w)
		}
		for i, p := range parents {
			start := birthOrbPosition(sw, sh, i, 90, 120)
			end := birthOrbPosition(sw, sh, i, 120, 120)
			if math.Abs(start.x-p.x) > 0.01 || math.Abs(start.y-(p.y-sh*0.23)) > 0.01 || math.Abs(end.x-crystal.x) > 0.01 || math.Abs(end.y-crystal.y) > 0.01 {
				t.Fatalf("orb %d at %v: %v %v", i, size, start, end)
			}
		}
	}
	g.Handle(event.BreedEffect{})
	g.req = event.Wait{Seconds: float64(birthDuration) / 60}
	g.waitT = birthDuration
	g.in.held[btnOK] = true
	g.in.pressed[btnOK] = true
	g.pump()
	if g.birth.skipT != 0 || g.req == nil {
		t.Fatal("confirming breed must not skip or dismiss")
	}
	g.in.held[btnOK] = false
	g.in.pressed[btnOK] = false
	g.pump()
	g.in.held[btnOK] = true
	g.in.pressed[btnOK] = true
	g.pump()
	if g.birth.skipT != 1 || g.req == nil {
		t.Fatal("fresh confirm did not start skip")
	}
	g.in.pressed[btnOK] = false
	for range 12 {
		g.birth.skipT++
		if g.birth.skipT >= 12 {
			g.birth.skipT = 0
			g.birth.age = g.birth.duration()
		}
		g.pump()
	}
	if g.req == nil || g.waitT != 1 {
		t.Fatalf("result was not held: wait=%d req=%T", g.waitT, g.req)
	}
	g.in.held[btnOK] = false
	g.pump()
	g.in.held[btnOK] = true
	g.in.pressed[btnOK] = true
	g.pump()
	if g.req != nil {
		t.Fatal("fresh result confirm did not continue")
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
