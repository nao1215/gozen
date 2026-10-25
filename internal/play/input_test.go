package play

import (
	"image"
	"strconv"
	"strings"
	"testing"
)

func TestScriptHeldControls(t *testing.T) {
	s, err := parseScript("hold up+right+shift 3\nwait 1\nquit")
	if err != nil {
		t.Fatal(err)
	}
	in := input{script: s}
	for tick := range 3 {
		in.update()
		if !in.held[btnUp] || !in.held[btnRight] || !in.shift || in.pressed[btnUp] != (tick == 0) {
			t.Fatalf("held tick %d: %+v", tick, in)
		}
	}
	in.update()
	if in.held[btnUp] || in.shift {
		t.Fatal("hold did not release")
	}
	for _, src := range []string{"hold up 0", "hold up -1", "hold nowhere 3", "hold up+ 3", "hold up 36001", "hold up"} {
		if _, err := parseScript(src); err == nil {
			t.Errorf("accepted %s", src)
		}
	}
}

func TestScriptBattleLog(t *testing.T) {
	for _, src := range []string{"battle-log", "battle-log hidden", "battle-log off extra"} {
		if _, err := parseScript(src); err == nil {
			t.Errorf("accepted %q", src)
		}
	}
	s, err := parseScript("battle-log off\nbattle-log on\nquit")
	if err != nil {
		t.Fatal(err)
	}
	var got []bool
	s.battleLog = func(enabled bool) { got = append(got, enabled) }
	sysStepAll(t, s, 12)
	if len(got) != 2 || got[0] || !got[1] {
		t.Fatalf("log states %v", got)
	}
}

func TestScriptBattleHP(t *testing.T) {
	for _, src := range []string{"battle-hp enemy", "battle-hp foe 0 5", "battle-hp enemy -1 5", "battle-hp enemy 0 0"} {
		if _, err := parseScript(src); err == nil {
			t.Errorf("accepted %q", src)
		}
	}
	s, err := parseScript("battle-hp enemy 1 25\nquit")
	if err != nil {
		t.Fatal(err)
	}
	called := false
	s.battleHP = func(team string, index, hp int) {
		called = team == "enemy" && index == 1 && hp == 25
	}
	sysStepAll(t, s, 10)
	if !called {
		t.Fatal("battle-hp callback not called")
	}
}

// sysStepAll runs a parsed script until it quits (or max ticks) and returns the inputs it
// produced tick by tick.
func sysStepAll(t *testing.T, s *script, limit int) []input {
	t.Helper()
	var out []input
	in := &input{}
	for range limit {
		in.pressed = [numButtons]bool{}
		in.click, in.digit = false, 0
		s.step(in)
		out = append(out, *in)
		if s.quit {
			return out
		}
	}
	t.Fatalf("the script did not quit within %d ticks", limit)
	return nil
}

// TestParseScriptRejects: every malformed line is reported with its line number.
func TestParseScriptRejects(t *testing.T) {
	cases := map[string]string{
		"unknown verb":          "dance",
		"wait without number":   "wait",
		"wait word":             "wait soon",
		"wait negative":         "wait -3",
		"read word":             "read x",
		"gap word":              "gap x",
		"click one coordinate":  "click",
		"click only x":          "click 100",
		"walk without a tile":   "walk",
		"face without a side":   "face",
		"click word":            "click a b",
		"click bad y":           "click 3 b",
		"press nothing":         "press",
		"press unknown":         "press fire",
		"press two digits":      "press 12",
		"press bad count":       "press ok x",
		"press negative count":  "press ok -1",
		"shot nothing":          "shot",
		"shot path":             "shot a/b",
		"shot backslash":        "shot a\\b",
		"shot parent":           "shot ../x",
		"shot dot":              "shot .",
		"shot dotdot":           "shot ..",
		"skip nothing":          "skip",
		"skip word":             "skip x",
		"skip negative":         "skip -2",
		"jump nothing":          "jump",
		"do nothing":            "do",
		"do spaces":             "do    ",
		"memo only kind":        "memo note",
		"lines bad count":       "lines x y",
		"error on later line":   "wait 1\n\n# fine\nbogus 3",
		"unknown with argument": "teleport 1 2",
	}
	for name, src := range cases {
		_, err := parseScript(src)
		if err == nil {
			t.Errorf("%s: %q accepted", name, src)
			continue
		}
		if !strings.Contains(err.Error(), "script line") {
			t.Errorf("%s: error does not name the line: %v", name, err)
		}
	}
	_, err := parseScript("wait 1\n\n# fine\nbogus 3")
	if err == nil || !strings.Contains(err.Error(), "line 4") {
		t.Errorf("the error should be at line 4: %v", err)
	}
}

// TestParseScriptAccepts: the documented verbs parse into the commands that were written.
func TestParseScriptAccepts(t *testing.T) {
	src := `# a comment
wait 5
press ok
press z 3
press 7
press 0
click 100 200
shot field
skip 30 sprint
gap 2
read 10
until idle
walk gate
walk gate arrival
face north
lines scene 4
lines scene
play fight 6
play fight
memo note  a note with   spaces
type hello world
jump intro 9
jump intro
do {set: {flag.x: true}}
quit
`
	s, err := parseScript(src)
	if err != nil {
		t.Fatal(err)
	}
	var verbs []string
	for _, c := range s.cmds {
		verbs = append(verbs, c.verb)
	}
	want := "wait press press press press click shot skip gap read until walk walk face lines lines play play memo type jump jump do quit"
	if strings.Join(verbs, " ") != want {
		t.Fatalf("verbs:\n%s\nwant\n%s", strings.Join(verbs, " "), want)
	}
	byIndex := func(i int) scriptCmd { return s.cmds[i] }
	if c := byIndex(0); c.n != 5 {
		t.Errorf("wait: %+v", c)
	}
	if c := byIndex(2); c.arg != "z" || c.n != 3 {
		t.Errorf("press z 3: %+v", c)
	}
	if c := byIndex(5); c.x != 100 || c.y != 200 || c.n != 1 {
		t.Errorf("click: %+v", c)
	}
	if c := byIndex(7); c.n != 30 || c.name != "sprint" {
		t.Errorf("skip: %+v", c)
	}
	if c := byIndex(12); c.arg != "gate" || c.name != "arrival" {
		t.Errorf("walk with a name: %+v", c)
	}
	if c := byIndex(14); c.arg != "scene" || c.n != 4 {
		t.Errorf("lines with a count: %+v", c)
	}
	if c := byIndex(15); c.n != 0 {
		t.Errorf("lines without a count: %+v", c)
	}
	if c := byIndex(18); c.arg != "note" || c.name != "a note with   spaces" {
		t.Errorf("memo: %+v", c)
	}
	if c := byIndex(19); c.arg != "hello world" {
		t.Errorf("type: %+v", c)
	}
	if c := byIndex(20); c.arg != "intro" || c.n != 9 {
		t.Errorf("jump with a line: %+v", c)
	}
	if c := byIndex(21); c.n != 1 {
		t.Errorf("jump without a line: %+v", c)
	}
	if c := byIndex(22); c.arg != "{set: {flag.x: true}}" {
		t.Errorf("do: %+v", c)
	}
}

// TestParseScriptBlankAndComments: only blank lines and comments gives an empty script that
// quits at once.
func TestParseScriptBlankAndComments(t *testing.T) {
	s, err := parseScript("\n   \n# nothing\n\t# nothing either\n")
	if err != nil {
		t.Fatal(err)
	}
	if len(s.cmds) != 0 {
		t.Fatalf("%d commands", len(s.cmds))
	}
	in := sysStepAll(t, s, 3)
	if len(in) != 1 {
		t.Fatalf("an empty script should quit on the first tick, took %d", len(in))
	}
}

// TestScriptWaitCountsTicks: `wait N` holds the script for N ticks before the next command.
func TestScriptWaitCountsTicks(t *testing.T) {
	s, err := parseScript("wait 4\npress ok\nquit")
	if err != nil {
		t.Fatal(err)
	}
	ins := sysStepAll(t, s, 100)
	first := -1
	for i, in := range ins {
		if in.pressed[btnOK] {
			first = i
			break
		}
	}
	// the wait command takes a tick and then the four ticks of waiting pass.
	if first != 5 {
		t.Fatalf("ok pressed at tick %d, want 5", first)
	}
}

// TestScriptPressVariants: a cancel press is also a menu press; a digit sets the digit (0 is
// ten); a count presses that many times; the gap spaces them.
func TestScriptPressVariants(t *testing.T) {
	count := func(src string, f func(input) bool) (n int, ticks []int) {
		s, err := parseScript(src)
		if err != nil {
			t.Fatal(err)
		}
		for i, in := range sysStepAll(t, s, 500) {
			if f(in) {
				n++
				ticks = append(ticks, i)
			}
		}
		return n, ticks
	}
	n, _ := count("press cancel\nquit", func(in input) bool { return in.pressed[btnCancel] && in.pressed[btnMenu] })
	if n != 1 {
		t.Errorf("cancel should press cancel and menu together: %d", n)
	}
	n, _ = count("press menu\nquit", func(in input) bool { return in.pressed[btnMenu] && !in.pressed[btnCancel] })
	if n != 1 {
		t.Errorf("menu alone: %d", n)
	}
	n, _ = count("press 5\nquit", func(in input) bool { return in.digit == 5 })
	if n != 1 {
		t.Errorf("digit 5: %d", n)
	}
	n, _ = count("press 0\nquit", func(in input) bool { return in.digit == 10 })
	if n != 1 {
		t.Errorf("digit 0 is ten: %d", n)
	}
	n, ticks := count("gap 3\npress down 4\nquit", func(in input) bool { return in.pressed[btnDown] })
	if n != 4 {
		t.Fatalf("press down 4 pressed %d times", n)
	}
	for i := 1; i < len(ticks); i++ {
		if ticks[i]-ticks[i-1] != 4 {
			t.Errorf("gap 3 should space presses by 4 ticks: %v", ticks)
			break
		}
	}
	n, ticks = count("press up 3\nquit", func(in input) bool { return in.pressed[btnUp] })
	if n != 3 || ticks[1]-ticks[0] != 9 {
		t.Errorf("default gap: %d %v", n, ticks)
	}
	for name, b := range scriptButtons {
		n, _ := count("press "+name+"\nquit", func(in input) bool { return in.pressed[b] })
		if n != 1 {
			t.Errorf("press %s did not press its button", name)
		}
	}
}

// TestScriptClick: click puts the pointer at the point for one tick.
func TestScriptClick(t *testing.T) {
	s, err := parseScript("click 33 44\nquit")
	if err != nil {
		t.Fatal(err)
	}
	ins := sysStepAll(t, s, 30)
	if !ins[0].click || ins[0].mouse != image.Pt(33, 44) {
		t.Fatalf("click at %+v %v", ins[0].mouse, ins[0].click)
	}
	if ins[1].click {
		t.Fatal("click lasted more than one tick")
	}
}

// TestScriptSkipHoldsFast: `skip N` holds the fast-forward for N ticks.
func TestScriptSkipHoldsFast(t *testing.T) {
	s, err := parseScript("skip 5\nquit")
	if err != nil {
		t.Fatal(err)
	}
	fast := 0
	for _, in := range sysStepAll(t, s, 30) {
		if in.fast {
			fast++
		}
	}
	if fast != 5 {
		t.Fatalf("fast for %d ticks, want 5", fast)
	}
}

// TestScriptShotNamesAndSkipShots: `shot NAME` hands the name to the screen for one frame.
func TestScriptShotNames(t *testing.T) {
	s, err := parseScript("shot first\nwait 2\nshot second\nquit")
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	in := &input{}
	for range 40 {
		s.step(in)
		if s.shot != "" {
			got = append(got, s.shot)
			s.shot = ""
		}
		if s.quit {
			break
		}
	}
	if strings.Join(got, ",") != "first,second" {
		t.Fatalf("shots %v", got)
	}
}

// TestScriptReadPressesWhenTheLineIsOld: `read N` presses ok once the line has been shown
// for N ticks, and goes on at a choice.
func TestScriptReadPressesWhenTheLineIsOld(t *testing.T) {
	s, err := parseScript("read 10\nquit")
	if err != nil {
		t.Fatal(err)
	}
	age, what := 0, "text"
	s.waiting = func() (string, int) { age++; return what, age }
	s.idle = func() bool { return false }
	pressedAt := 0
	in := &input{}
	for tick := 1; tick < 100; tick++ {
		in.pressed = [numButtons]bool{}
		s.step(in)
		if in.pressed[btnOK] && pressedAt == 0 {
			pressedAt = tick
			what = "choice"
		}
		if s.quit {
			break
		}
	}
	if pressedAt != 10 {
		t.Fatalf("ok pressed on tick %d, want 10 (the line's age)", pressedAt)
	}
	if !s.quit {
		t.Fatal("read did not finish at a choice")
	}
}

// TestScriptReadFinishesWhenIdle and the safety limit.
func TestScriptReadFinishesWhenIdle(t *testing.T) {
	s := sysScript(t, "read 3\nquit")
	s.waiting = func() (string, int) { return "", 0 }
	s.idle = func() bool { return true }
	sysStepAll(t, s, 5)
	s2 := sysScript(t, "read 3\nquit")
	sysStepAll(t, s2, 7000) // no events at all: the limit ends it
}

// TestScriptUntilPressesOKUntilIdle: `until` keeps pressing ok every sixth tick until the
// game is idle.
func TestScriptUntilPressesOKUntilIdle(t *testing.T) {
	s := sysScript(t, "until\nquit")
	busy := 20
	s.idle = func() bool { busy--; return busy <= 0 }
	presses := 0
	for _, in := range sysStepAll(t, s, 100) {
		if in.pressed[btnOK] {
			presses++
		}
	}
	if presses < 2 || presses > 4 {
		t.Fatalf("pressed ok %d times", presses)
	}
	s2 := sysScript(t, "until\nquit")
	s2.idle = func() bool { return false }
	sysStepAll(t, s2, 4000) // gives up after 3000 ticks
}

// TestScriptWalkWithoutWalker: walk and face are skipped when nothing can walk; with a walker
// the script presses the button it asks for until it is done.
func TestScriptWalkWithoutWalker(t *testing.T) {
	s := sysScript(t, "walk gate\nface north\nquit")
	sysStepAll(t, s, 5)

	s = sysScript(t, "walk gate\nquit")
	calls := 0
	var asked []string
	s.walk = func(arg string) (button, bool) {
		calls++
		asked = append(asked, arg)
		if calls > 3 {
			return btnNone, true
		}
		return btnRight, false
	}
	pressed := 0
	for _, in := range sysStepAll(t, s, 50) {
		if in.pressed[btnRight] && in.held[btnRight] {
			pressed++
		}
	}
	if pressed != 3 || asked[0] != "gate" {
		t.Fatalf("pressed %d asked %v", pressed, asked)
	}
	s = sysScript(t, "face north\nquit")
	s.walk = func(arg string) (button, bool) { asked = []string{arg}; return btnNone, true }
	sysStepAll(t, s, 10)
	if asked[0] != "face:north" {
		t.Fatalf("face asked %v", asked)
	}
}

// TestScriptWalkFollowsAnEventOnTheWay: a walk that starts an event (the game is busy) waits
// for it to end, reading it with shots named after the walk.
func TestScriptWalkFollowsAnEventOnTheWay(t *testing.T) {
	s := sysScript(t, "walk door lamp\nquit")
	busy := true
	n := 0
	s.walk = func(string) (button, bool) { return btnNone, !busy }
	s.idle = func() bool {
		n++
		if n > 30 {
			busy = false
		}
		return !busy
	}
	s.arrived = func() bool { return false }
	s.waiting = func() (string, int) { return "text", 10 }
	var shots []string
	in := &input{}
	for range 200 {
		in.pressed = [numButtons]bool{}
		s.step(in)
		if s.shot != "" {
			shots = append(shots, s.shot)
			s.shot = ""
		}
		if s.quit {
			break
		}
	}
	if !s.quit || len(shots) == 0 || !strings.HasPrefix(shots[0], "lamp_w") {
		t.Fatalf("quit=%v shots=%v", s.quit, shots)
	}
}

// TestScriptLinesFilmsEachLine: `lines NAME N` films each line of text it passes up to N and
// presses ok to read on.
func TestScriptLinesFilmsEachLine(t *testing.T) {
	s := sysScript(t, "lines scene 2\nquit")
	s.waiting = func() (string, int) { return "text", 10 }
	s.idle = func() bool { return false }
	var shots []string
	in := &input{}
	for range 100 {
		in.pressed = [numButtons]bool{}
		s.step(in)
		if s.shot != "" {
			shots = append(shots, s.shot)
			s.shot = ""
		}
		if s.quit {
			break
		}
	}
	if strings.Join(shots, ",") != "scene_001,scene_002" {
		t.Fatalf("shots %v", shots)
	}
	// a choice ends it after one shot.
	s = sysScript(t, "lines pick\nquit")
	s.waiting = func() (string, int) { return "choice", 0 }
	shots = nil
	for range 20 {
		s.step(in)
		if s.shot != "" {
			shots = append(shots, s.shot)
			s.shot = ""
		}
	}
	if len(shots) != 1 || shots[0] != "pick_001" || !s.quit {
		t.Fatalf("choice: shots %v quit %v", shots, s.quit)
	}
	// the game going quiet ends it.
	s = sysScript(t, "lines idle\nquit")
	s.waiting = func() (string, int) { return "", 0 }
	s.idle = func() bool { return true }
	sysStepAll(t, s, 100)
}

// TestScriptPlayFilmsTextChoicesAndBattles covers the `play` verb.
func TestScriptPlayFilmsTextChoicesAndBattles(t *testing.T) {
	s := sysScript(t, "play p\nquit")
	state, age := "text", 10
	s.waiting = func() (string, int) { return state, age }
	s.idle = func() bool { return false }
	s.battleAuto = func() { state = "choice" }
	var shots []string
	in := &input{}
	for range 100 {
		in.pressed = [numButtons]bool{}
		s.step(in)
		if s.shot != "" {
			shots = append(shots, s.shot)
			s.shot = ""
			if len(shots) == 2 {
				state = "command"
			}
		}
		if s.quit {
			break
		}
	}
	if !s.quit || len(shots) < 3 || shots[0] != "p_001" || shots[1] != "p_002" || !strings.HasPrefix(shots[2], "p_b") {
		t.Fatalf("shots %v quit %v", shots, s.quit)
	}
	s = sysScript(t, "play p\nquit")
	s.waiting = func() (string, int) { return "", 0 }
	s.idle = func() bool { return true }
	sysStepAll(t, s, 200)
}

// TestScriptHooks: type, memo, jump and do call their hooks with the written text; without a
// hook they only pass.
func TestScriptHooks(t *testing.T) {
	s := sysScript(t, "type  hi there\nmemo bug  it broke\njump intro 7\ndo {say: x}\nquit")
	var got []string
	s.typeText = func(x string) { got = append(got, "type:"+x) }
	s.memoNote = func(k, x string) { got = append(got, "memo:"+k+":"+x) }
	s.jump = func(name string, line int) { got = append(got, "jump:"+name+":"+strconv.Itoa(line)) }
	s.do = func(src string) { got = append(got, "do:"+src) }
	sysStepAll(t, s, 50)
	want := "type:hi there,memo:bug:it broke,jump:intro:7,do:{say: x}"
	if strings.Join(got, ",") != want {
		t.Fatalf("hooks %v, want %s", got, want)
	}
	s = sysScript(t, "type a\nmemo n b\njump x\ndo {}\nquit")
	sysStepAll(t, s, 50)
}

// TestScriptMidEvent: the walking helper films a command prompt, a text and a choice.
func TestScriptMidEvent(t *testing.T) {
	s := &script{}
	in := &input{}
	s.waiting = func() (string, int) { return "command", 0 }
	autos := 0
	s.battleAuto = func() { autos++ }
	s.midEvent(in, "n")
	if s.shot != "n_b001" || autos != 1 {
		t.Fatalf("command: %q %d", s.shot, autos)
	}
	s.shot = ""
	s.waiting = func() (string, int) { return "text", 2 }
	s.midEvent(in, "n")
	if s.shot != "" || in.pressed[btnOK] {
		t.Fatal("a new line is not read yet")
	}
	s.waiting = func() (string, int) { return "text", 9 }
	s.wait = 0
	s.midEvent(in, "n")
	if s.shot != "n_001" {
		t.Fatalf("text shot %q", s.shot)
	}
	s.midEvent(in, "n")
	if !in.pressed[btnOK] {
		t.Fatal("the shown line was not passed")
	}
	s.waiting = nil
	s.inBattle = func() bool { return true }
	s.shot = ""
	for range 20 {
		s.midEvent(in, "n")
	}
	if s.shot == "" {
		t.Fatal("a battle is not filmed")
	}
}

// TestInputWithoutDevices: with no keys, pointer or touch the live path reports nothing pressed
// and keeps the pointer where the last position left it.
func TestInputWithoutDevices(t *testing.T) {
	in := &input{ui: 2}
	in.pressed[btnOK] = true
	in.click, in.wheel = true, -1
	in.update()
	for b, p := range in.pressed {
		if p {
			t.Errorf("button %d pressed with no keys", b)
		}
	}
	if in.click || in.rclick || in.wheel != 0 || in.digit != 0 || in.fast {
		t.Fatalf("stale state: %+v", in)
	}
	if got := in.toUI(200, 100); got != image.Pt(100, 50) {
		t.Fatalf("toUI at ui 2: %v", got)
	}
	in.ui = 1
	if got := in.toUI(200, 100); got != image.Pt(200, 100) {
		t.Fatalf("toUI at ui 1: %v", got)
	}
	in.ui = 0
	if got := in.toUI(7, 9); got != image.Pt(7, 9) {
		t.Fatalf("toUI at ui 0: %v", got)
	}
}

// TestInputHelpers: tab direction, advance, openList and the wheel.
func TestInputHelpers(t *testing.T) {
	in := &input{}
	if in.tab() != 0 {
		t.Fatal("tab without press")
	}
	in.pressed[btnNext] = true
	if in.tab() != 1 {
		t.Fatal("tab forward")
	}
	in.shift = true
	if in.tab() != -1 {
		t.Fatal("shift tab back")
	}
	in = &input{}
	if in.advance() || in.up() || in.down() {
		t.Fatal("nothing happens without input")
	}
	in.wheel = -1
	if !in.advance() || !in.down() || in.up() {
		t.Fatal("wheel down advances")
	}
	in.wheel = 1
	if !in.up() || in.down() {
		t.Fatal("wheel up")
	}
	in = &input{}
	in.click = true
	if !in.advance() {
		t.Fatal("click advances")
	}
	in = &input{}
	in.rclick = true
	if !in.cancel() {
		t.Fatal("right click cancels")
	}
	if !in.openList() || in.rclick {
		t.Fatal("right click opens the list and is used up")
	}
	in = &input{longPress: true}
	if !in.openList() || !in.longUsed {
		t.Fatal("long press opens the list and marks it used")
	}
	in = &input{}
	in.pressed[btnMenu] = true
	if !in.openList() {
		t.Fatal("menu key opens the list")
	}
	in.held[btnOK] = true
	if in.openList() {
		t.Fatal("menu with ok held does not open the list")
	}
	in = &input{}
	in.pressed[btnLeft], in.pressed[btnRight] = true, true
	if !in.left() || !in.right() {
		t.Fatal("left right")
	}
}

// TestTouchHeldFingerMovesThePointer: a followed finger that has not lifted moves the pointer
// and a long press is its own signal.
func TestTouchHeldFingerMovesThePointer(t *testing.T) {
	in := &input{ui: 2, touching: true, touchID: 99}
	in.touch()
	if !in.touching || !in.moved || in.mouse != in.touchAt {
		t.Fatalf("held finger: %+v", in)
	}
	in = &input{touchDone: true}
	in.touch()
	if in.touchDone {
		t.Fatal("touchDone should be cleared when no finger is down")
	}
}
