package play

import (
	"bytes"
	"fmt"
	"strconv"
	"strings"
	"testing"

	"github.com/nao1215/gozen/internal/base/geom"
	data "github.com/nao1215/gozen/internal/model/gamedata"
	"github.com/nao1215/gozen/internal/rules/dungeon"
	"github.com/nao1215/gozen/internal/rules/event"
)

// TestScriptReceiverGates drives the actual receiver while a command is pending,
// then checks its observable effect, not just the readiness predicate.
func TestScriptReceiverGates(t *testing.T) {
	for _, name := range []string{"talk join line", "talk join choice", "talk text start", "event wait", "event tap", "battle result", "clash cut-in", "clash start"} {
		t.Run(name, func(t *testing.T) {
			g := sysInn(t, Options{})
			g.fadeT = 0
			command := "press ok"
			var step func()
			var handled func() bool
			switch name {
			case "talk join line", "talk join choice", "talk text start":
				g.talk.open(g, event.Say{Text: strings.Repeat("A long line. ", 30)})
				if name != "talk text start" {
					g.talk.open(g, event.Say{Text: "Welcome."})
					g.recruit = &recruitView{start: g.tick}
					g.talk.shown = g.talk.total
				}
				if name == "talk join choice" {
					g.talk.choice = &list{items: []listItem{{label: "Continue"}}}
					command = "pick Continue"
				}
				step = func() { g.talk.update(g) }
				if name == "talk text start" {
					handled = func() bool { return g.talk.shown == g.talk.total }
				} else {
					handled = func() bool { return !g.talk.active }
				}
			case "event wait", "event tap":
				g.events.Start([]data.Command{{Say: &data.Say{Text: strings.Repeat("Next line. ", 30)}}})
				if name == "event wait" {
					g.req, g.waitT = event.Wait{}, 6
					handled = func() bool { return g.talk.active && g.talk.shown == g.talk.total }
				} else {
					g.req, g.tapT = event.Tap{}, 6
					handled = func() bool { return g.req == nil }
				}
				step = func() { g.pump() }
			case "battle result":
				b := &battleScene{phase: bpResult, results: []string{"Victory", "Reward"}}
				g.stack = append(g.stack, b)
				step = func() { b.result(g) }
				handled = func() bool { return len(b.results) == 1 }
			case "clash cut-in", "clash start":
				c := &clashScene{wait: 100}
				if name == "clash cut-in" {
					c.cut = &clashCut{}
				}
				g.stack = append(g.stack, c)
				step = func() { c.update(g) }
				handled = func() bool {
					if name == "clash cut-in" {
						return c.cut == nil
					}
					return c.fast
				}
			}
			s, err := parseScript(command + "\nwait 1000")
			if err != nil {
				t.Fatal(err)
			}
			s.ready = g.scriptReady
			s.acknowledge = true
			s.pick = func(in *input, text string) {
				if err := g.scriptPick(in, text); err != nil {
					t.Fatal(err)
				}
			}
			for tick := range 200 {
				sysKey(g)
				s.step(&g.in)

				g.tick++
				step()
				g.in.finishOffer()
				if tick == 0 && s.pc != 0 {
					t.Fatal("command consumed while receiver was blocked")
				}
				if s.err != nil {
					t.Fatal(s.err)
				}
				if s.pc != 0 {
					if !handled() {
						t.Fatal("command consumed without its receiver handling it")
					}
					return
				}
			}
			t.Fatal("command never reached the receiver")
		})
	}
}

func TestScriptAtlasMovingInput(t *testing.T) {
	for _, command := range []string{"press ok", "click"} {
		t.Run(command, func(t *testing.T) {
			g, a := wldAtlas(t, "fx_mark")
			a.path = geom.Polyline{a.at.Add(geom.Pt{X: -20}), a.at}
			if command == "click" {
				point := a.location(g).Map.Points[a.choices[a.sel].Point].At.Pt()
				x, y := a.screen(g, point)
				command = fmt.Sprintf("click %d %d", int(x), int(y))
			}
			s, err := parseScript(command + "\nwait 1000")
			if err != nil {
				t.Fatal(err)
			}
			s.ready = g.scriptReady
			s.acknowledge = true
			for range 1000 {
				sysKey(g)
				moving := a.path != nil
				s.step(&g.in)

				a.update(g)
				g.in.finishOffer()
				if moving && s.pc != 0 {
					t.Fatal("command consumed while the map token was moving")
				}
				if s.pc != 0 {
					if len(a.queue) == 0 {
						t.Fatal("ready map did not start the selected journey")
					}
					return
				}
			}
			t.Fatal("map input never released after movement")
		})
	}
}

func TestScriptReceiverIgnoresSuspendedEvent(t *testing.T) {
	g := sysInn(t, Options{})
	g.events.Start([]data.Command{{Say: &data.Say{Text: "After the menu."}}})
	g.req, g.waitT = event.Wait{}, 60
	answered := false
	g.push(newConfirm("Continue?", func() { answered = true }))
	s, err := parseScript("press ok\nwait 1000")
	if err != nil {
		t.Fatal(err)
	}
	s.ready = g.scriptReady
	s.acknowledge = true
	for tick := range 20 {
		sysKey(g)
		s.step(&g.in)

		g.stack[len(g.stack)-1].update(g)
		g.in.finishOffer()
		if tick == 0 && s.pc != 0 {
			t.Fatal("command consumed while the confirmation was opening")
		}
		if s.pc != 0 {
			if !answered || g.waitT != 60 {
				t.Fatal("the foreground menu waited for or advanced its suspended event")
			}
			return
		}
	}
	t.Fatal("foreground menu never received input")
}

// Inputs are acknowledged by the receiver, including early-return paths that
// cannot be predicted from the screen type or the event runner's running flag.
func TestScriptAcknowledgesReceiver(t *testing.T) {
	for _, name := range []string{"atlas road talk", "dungeon transition", "dungeon space", "battle hitstop", "author return press", "author return click", "author return pick"} {
		t.Run(name, func(t *testing.T) {
			var g *Game
			var trace bytes.Buffer
			command := "press ok"
			var handled func() bool
			switch name {
			case "atlas road talk":
				var a *atlasScene
				g, a = wldAtlas(t, "fx_mark")
				a.queue = []data.Command{{Reach: "fx_mark"}}
				g.events.Start([]data.Command{{Say: &data.Say{Text: "Roadside greeting."}}})
				g.req = g.events.Next()
				g.begin(g.req)
				g.talk.shown = g.talk.total
				handled = func() bool { return !g.talk.active && len(a.queue) == 1 }
			case "dungeon transition", "dungeon space":
				g = wldMaze(t, "fx_maze", 1, 1, 1)
				d := sysAs[*dungeonScene](t, g.stack[0])
				if name == "dungeon space" {
					d.space = &spaceView{remaining: 12}
				} else {
					d.transT = 12
				}
				command = "press up"
				handled = func() bool { return g.st.Pos.X == 2 }
			case "battle hitstop":
				g, _ = btlOpen(t, Options{Battle: "one"}, "", "")
				b := btlAsk(t, g)
				b.phase, b.auto, b.dr.freeze = bpPlay, true, 12
				command = "press cancel"
				handled = func() bool { return !b.auto }
			default:
				g = wldOpenWith(t, Options{Trace: &trace, FS: wldWorldFiles(t, `screens:
  board:
    title: Board
    entries:
      - label: Continue
        do: [{set: {flag.continued: true}}]
`, nil), Location: "camp"})
				wldFrames(t, g, 30)
				g.openScreen("board", nil)
				a := sysAs[*authorScreen](t, g.stack[len(g.stack)-1])
				a.running, a.origin = true, g.st.Location
				switch name {
				case "author return click":
					r := a.list.rowRect(0)
					command = fmt.Sprintf("click %d %d", r.Min.X+2, r.Min.Y+2)
				case "author return pick":
					command = "pick Continue"
				}
				handled = func() bool { return a.running }
			}
			g.fadeT = 0
			g.turnSeen = g.turnKey()
			s := sysScript(t, command+"\nwait 1000")
			s.acknowledge, s.ready = true, g.scriptReady
			s.pick = func(in *input, label string) {
				if err := g.scriptPick(in, label); err != nil {
					t.Fatal(err)
				}
			}
			g.in = input{script: s}
			for tick := range 6002 {
				if err := g.Update(); err != nil {
					t.Fatal(err)
				}
				if s.pc == 0 {
					continue
				}
				if tick == 0 && name != "atlas road talk" {
					t.Fatal("command consumed on the receiver's early-return tick")
				}
				if !handled() {
					t.Fatal("command consumed without its intended effect")
				}
				if name == "author return pick" {
					rows := traceLines(t, trace.Bytes(), "input")
					if len(rows) != 1 || rows[0]["tick"] != float64(g.tick) {
						t.Fatalf("pick activation missing from input trace: %v", rows)
					}
				}
				return
			}
			t.Fatal("input never reached its receiver")
		})
	}
}

func TestScriptDiscardedInput(t *testing.T) {
	t.Run("chart zoom", func(t *testing.T) {
		g := bookGame(t, Options{Location: "camp", Development: true}, "")
		view := newProfiles(g, nil)
		view.chart, view.focus, view.focusFrame = true, view.sel, 0
		s, err := parseScript("press ok\nwait 1000")
		if err != nil {
			t.Fatal(err)
		}
		s.acknowledge = true
		for tick := range chartFocusFrames + 2 {
			sysKey(g)
			s.step(&g.in)
			view.updateChart(g)
			g.in.finishOffer()
			if tick == 0 && s.pc != 0 {
				t.Fatal("zoom discarded and acknowledged OK")
			}
			if s.pc != 0 {
				if view.chart || !view.chartPage {
					t.Fatal("OK did not open profile")
				}
				return
			}
		}
		t.Fatal("OK never reached profile")
	})
	for _, mode := range []string{"typing", "fade"} {
		t.Run("banner "+mode, func(t *testing.T) {
			g := sysInn(t, Options{})
			g.banners = []phoneBanner{{n: data.PhoneNotice{Event: "notice"}, age: bannerSlide}}
			s, err := parseScript("click 0 600\nwait 1000")
			if err != nil {
				t.Fatal(err)
			}
			s.acknowledge = true
			if mode == "fade" {
				g.fadeT, g.in.script = 2, s
				if err := g.Update(); err != nil {
					t.Fatal(err)
				}
				g.in.script = nil
			} else {
				sysKey(g)
				s.step(&g.in)
				g.updateBanners()
				g.phone = &phoneView{typing: 2}
				g.updatePhoneChat(g.phone)
				g.in.finishOffer()
			}
			if s.pc != 0 {
				t.Fatal("outside click acknowledged by banner")
			}
			sysKey(g)
			s.step(&g.in)
			if !g.in.advance() {
				t.Fatal("pending click disappeared")
			}
			g.in.finishOffer()
			if s.pc == 0 {
				t.Fatal("ready receiver did not accept click")
			}
		})
	}
}

func TestScriptEntitiesPick(t *testing.T) {
	for _, mode := range []string{"action", "row", "flow"} {
		t.Run(mode, func(t *testing.T) {
			g := openGuideGame(t, nil)
			name, label := "act_board", "Rest"
			if mode == "flow" {
				name, label = "look_board", "Onward"
			} else {
				g.db.Game.Flows = nil
			}
			def := g.db.Game.Screens[name]
			def.Kind, def.Guide = "entities", nil
			if mode == "row" {
				def.View.Detail, label = "record", "@potter"
			}
			g.openScreen(name, func() {})
			view := top(t, g)
			s, err := parseScript("pick " + label + "\nwait 1000")
			if err != nil {
				t.Fatal(err)
			}
			s.acknowledge = true
			s.pick = func(in *input, text string) {
				if err := g.scriptPick(in, text); err != nil {
					t.Fatal(err)
				}
			}
			for range 10 {
				sysKey(g)
				s.step(&g.in)
				view.update(g)
				g.in.finishOffer()
			}
			if s.pc == 0 {
				t.Fatal("entity pick not received")
			}
			switch mode {
			case "action":
				if g.st.Vars["rested"] != 1.0 {
					t.Fatal("entity action did not run")
				}
			case "row":
				if view.target != "potter" {
					t.Fatal("entity detail did not open")
				}
			case "flow":
				if top(t, g).name != "act_board" {
					t.Fatal("entity flow did not advance")
				}
			}
		})
	}
}

func TestScriptGuideRestrictedPick(t *testing.T) {
	for _, label := range []string{"Work", "Rest"} {
		t.Run(label, func(t *testing.T) {
			g := openGuideGame(t, nil)
			g.db.Game.Flows = nil
			g.st.Vars["ready"] = 1.0
			g.openScreen("act_board", func() {})
			view := top(t, g)
			sysKey(g)
			view.update(g)
			s, err := parseScript("pick " + label + "\nwait 10000")
			if err != nil {
				t.Fatal(err)
			}
			s.acknowledge = true
			s.pick = func(in *input, text string) {
				if err := g.scriptPick(in, text); err != nil {
					t.Fatal(err)
				}
			}
			for range 6002 {
				sysKey(g)
				s.step(&g.in)
				view.update(g)
				g.in.finishOffer()
				if s.err != nil {
					break
				}
			}
			if label == "Rest" {
				if g.st.Vars["rested"] != nil || s.pc != 0 || s.err == nil {
					t.Fatalf("restricted pick accepted: pc=%d error=%v vars=%v", s.pc, s.err, g.st.Vars)
				}
			} else if s.err != nil || g.st.Vars["worked"] != 1.0 {
				t.Fatalf("highlighted pick failed: %v %v", s.err, g.st.Vars)
			}
		})
	}
}

func TestScriptCameraHitAcknowledgement(t *testing.T) {
	g := wldOpenWith(t, Options{FS: atlasCameraFiles(t), Location: "large"})
	a := sysAs[*atlasScene](t, g.stack[0])
	s := sysScript(t, "click 0 0\nwait 1000")
	s.acknowledge = true
	sysKey(g)
	s.step(&g.in)
	a.updateCamera(g)
	g.in.finishOffer()
	if s.pc != 0 {
		t.Fatal("camera acknowledged a click outside its buttons")
	}
	// The pending command must hit a button whose action can change the view.
	b := a.cameraButtons(g)[5]
	s.cmds[0].x, s.cmds[0].y = int(b.x+2), int(b.y+2)
	sysKey(g)
	s.step(&g.in)
	if !a.updateCamera(g) {
		t.Fatal("camera button did not act")
	}
	g.in.finishOffer()
	if s.pc == 0 {
		t.Fatal("camera button was not acknowledged")
	}
}

func TestScriptBirthReleaseBeforeRetry(t *testing.T) {
	g := ranchGame(t)
	g.fadeT = 0
	g.Handle(event.BreedEffect{})
	g.req = event.Wait{Seconds: float64(birthDuration) / 60}
	g.waitT = birthDuration
	s := sysScript(t, "gap 0\npress ok\npress ok\nwait 1000")
	s.acknowledge, s.ready = true, g.scriptReady
	g.in = input{script: s}
	for range 1000 {
		if err := g.Update(); err != nil {
			t.Fatal(err)
		}
		if !g.birth.waiting {
			if s.pc < 3 {
				t.Fatal("birth finished without accepting both confirmations")
			}
			return
		}
	}
	t.Fatalf("birth stuck waiting for release: ready=%v pc=%d", g.birth.ready, s.pc)
}

func TestScriptTipTextDelivered(t *testing.T) {
	g, w, _ := voicedShow(t)
	w.tips, w.typing = true, false
	s := sysScript(t, "type for seeds\nwait 1000")
	s.acknowledge, s.typeText = true, g.typeMemo
	g.in = input{script: s}
	s.step(&g.in)
	w.input(g)
	g.in.finishOffer()
	if s.pc == 0 || w.field.text != "for seeds" {
		t.Fatalf("tip text lost: pc=%d text=%q", s.pc, w.field.text)
	}
	g.in = input{script: s}
	g.in.pressed[btnOK] = true
	w.input(g)
	if len(w.cheers) != 1 || w.cheers[0].text != "for seeds" {
		t.Fatal("typed message was not sent with the tip")
	}
}

func TestScriptTagModalObservations(t *testing.T) {
	for _, verb := range []string{"look", "snapshot"} {
		t.Run(verb, func(t *testing.T) {
			g := sysInn(t, Options{})
			g.openTag("neutral.png")
			s := sysScript(t, verb+"\nwait 1000")
			s.ready, s.acknowledge = g.scriptReady, true
			observed := false
			observe := func() { observed = true }
			s.look, s.snapshot = observe, observe
			g.in = input{script: s}
			for range 4 {
				if err := g.Update(); err != nil {
					t.Fatal(err)
				}
			}
			if !observed || g.tag == nil || g.tag.box == nil {
				t.Fatalf("idle tag modal not observed: pc=%d", s.pc)
			}
		})
	}
}

func TestScriptDateCardRetainsConfirmation(t *testing.T) {
	g := wldNovel(t, "ev_auto")
	g.fadeT = 0
	g.turn = &phaseTurn{age: turnSkip}
	g.talk.open(g, event.Say{Text: strings.Repeat("A line. ", 30)})
	s := sysScript(t, "press ok\nwait 1000")
	s.ready, s.acknowledge = g.scriptReady, true
	g.in = input{script: s}
	for range turnIn + turnHold + turnOut - turnSkip {
		if err := g.Update(); err != nil {
			t.Fatal(err)
		}
		if s.pc != 0 {
			t.Fatal("date card consumed screen confirmation")
		}
	}
	for range 100 {
		if err := g.Update(); err != nil {
			t.Fatal(err)
		}
		if s.pc > 0 {
			return
		}
	}
	t.Fatal("confirmation did not reach the screen")
}

func TestScriptNovelUnavailableActions(t *testing.T) {
	for _, command := range []string{"log click", "press back"} {
		t.Run(command, func(t *testing.T) {
			g := wldNovel(t, "ev_auto")
			g.backlog = nil
			g.evLines = 0
			if command == "log click" {
				for i, b := range g.novelButtons() {
					if b.b == btnLog {
						r := g.novelButtonRects()[i]
						command = fmt.Sprintf("click %d %d", r.Min.X+2, r.Min.Y+2)
					}
				}
			}
			s := sysScript(t, command+"\nwait 1000")
			s.acknowledge = true
			sysKey(g)
			s.step(&g.in)
			g.novelInput()
			g.in.finishOffer()
			if s.pc != 0 || g.logView != nil {
				t.Fatal("unavailable novel action acknowledged")
			}
			if strings.HasPrefix(command, "click") {
				g.remember(logLine{text: "Earlier line."})
				sysKey(g)
				s.step(&g.in)
				g.novelInput()
				g.in.finishOffer()
				if s.pc == 0 || g.logView == nil {
					t.Fatal("available log did not open")
				}
			}
		})
	}
}

func TestScriptLoadNoticeObservations(t *testing.T) {
	for _, verb := range []string{"look", "snapshot"} {
		t.Run(verb, func(t *testing.T) {
			g := sysInn(t, Options{})
			g.loadNotice = &loadNotice{lines: []string{"A saved item changed."}}
			s := sysScript(t, verb+"\nwait 1000")
			s.ready, s.acknowledge = g.scriptReady, true
			observed := false
			observe := func() { observed = true }
			s.look, s.snapshot = observe, observe
			g.in = input{script: s}
			for range 4 {
				if err := g.Update(); err != nil {
					t.Fatal(err)
				}
			}
			if !observed || g.loadNotice == nil {
				t.Fatal("load notice observation stalled")
			}
		})
	}
}

func TestScriptCameraTabWithoutChoices(t *testing.T) {
	g := wldOpenWith(t, Options{FS: atlasCameraFiles(t), Location: "large"})
	a := sysAs[*atlasScene](t, g.stack[0])
	choices := a.choices
	a.choices = nil
	s := sysScript(t, "press tab\nwait 1000")
	s.acknowledge = true
	sysKey(g)
	s.step(&g.in)
	a.updateCamera(g)
	g.in.finishOffer()
	if s.pc != 0 {
		t.Fatal("empty map acknowledged Tab")
	}
	a.choices = choices
	before := a.sel
	sysKey(g)
	s.step(&g.in)
	a.updateCamera(g)
	g.in.finishOffer()
	if s.pc == 0 || a.sel != (before+1)%len(choices) {
		t.Fatal("Tab did not select the next choice")
	}
}

func TestScriptSkipAcknowledgement(t *testing.T) {
	for _, name := range []string{"novel choice", "novel choice click", "novel phone result", "phone result", "unread scene"} {
		t.Run(name, func(t *testing.T) {
			g := wldNovel(t, "ev_auto")
			var trace bytes.Buffer
			g.tr = newTracer(&trace)
			command := "press skip"
			step := func() { g.novelInput() }
			switch strings.TrimSuffix(name, " click") {
			case "novel choice":
				g.talk.choice = newList([]listItem{{label: "Continue"}}, 0, 0, 200, 1)
			case "novel phone result", "phone result":
				g.phone = &phoneView{cue: data.PhoneCue{App: "page", Var: "answer", Rows: []data.PhoneRow{{Button: "Continue", ID: "continue"}}}, age: phoneRise, turned: phoneTurn, watchMsg: -1}
				if name == "phone result" {
					step = func() { g.updatePhone() }
				}
			case "unread scene":
				command = "press scene_skip"
				g.cfg.SkipAll = false
				g.read = nil
			}
			if strings.HasSuffix(name, " click") {
				for i, b := range g.novelButtons() {
					if b.b == btnSkip {
						r := g.novelButtonRects()[i]
						command = fmt.Sprintf("click %d %d", r.Min.X+2, r.Min.Y+2)
					}
				}
			}
			s := sysScript(t, command+"\nwait 1000")
			s.acknowledge = true
			offer := func() {
				sysKey(g)
				s.step(&g.in)
				step()
				g.in.finishOffer()
			}
			for range 3 {
				offer()
				if s.pc != 0 || g.skipping || g.fast != nil {
					t.Fatal("blocked skip acknowledged or acted")
				}
			}
			g.talk.choice = nil
			if g.phone != nil {
				g.phone.cue.Rows = nil
				g.phone.cue.Var = ""
			}
			g.cfg.SkipAll = true
			offer()
			if s.pc == 0 || command != "press scene_skip" && !g.skipping || command == "press scene_skip" && len(traceLines(t, trace.Bytes(), "skip")) == 0 {
				t.Fatal("available skip did not act and acknowledge")
			}
		})
	}
}

func TestScriptStopsSkipAtChoice(t *testing.T) {
	g := wldNovel(t, "ev_auto")
	g.talk.choice = newList([]listItem{{label: "Continue"}}, 0, 0, 200, 1)
	g.skipping = true
	s := sysScript(t, "press skip\nwait 1000")
	s.acknowledge = true
	sysKey(g)
	s.step(&g.in)
	g.novelInput()
	g.in.finishOffer()
	if s.pc != 1 || g.skipping || g.talk.choice == nil {
		t.Fatalf("stop skip must act without answering the choice: pc=%d skipping=%v", s.pc, g.skipping)
	}
}

func TestScriptChoiceCancelRemainsPending(t *testing.T) {
	for _, muted := range []bool{false, true} {
		t.Run(strconv.FormatBool(muted), func(t *testing.T) {
			g := sysInn(t, Options{})
			g.talk.open(g, event.Say{Text: "Choose a route."})
			choice := newList([]listItem{{label: "Continue"}}, 0, 0, 200, 1)
			g.talk.choice = choice
			if muted {
				g.talk.held, g.talk.muted = choice, 20000
			}
			s := sysScript(t, "press cancel\npress ok")
			s.acknowledge = true
			for range 6002 {
				sysKey(g)
				s.step(&g.in)
				done, _ := g.talk.update(g)
				g.in.finishOffer()
				if s.pc != 0 || done || g.talk.choice != choice || !g.talk.active {
					t.Fatal("cancel advanced the script without closing the choice")
				}
				if s.err != nil {
					return
				}
			}
			t.Fatal("unhandled cancel did not expire")
		})
	}
}

func TestScriptHoldChoiceDuration(t *testing.T) {
	g := sysInn(t, Options{})
	l := &list{items: []listItem{{label: "First"}, {label: "Second"}, {label: "Third"}}}
	s := sysScript(t, "hold down 2")
	s.acknowledge = true
	for tick := range 4 {
		sysKey(g)
		s.step(&g.in)
		if tick > 0 {
			l.updateChoice(g)
		}
		g.in.finishOffer()
		if tick == 0 && s.cmds[0].n != 2 {
			t.Fatal("unaccepted hold lost duration")
		}
	}
	if s.pc != 1 || s.err != nil || l.sel != 1 {
		t.Fatalf("hold stalled or repeated: pc=%d err=%v selection=%d", s.pc, s.err, l.sel)
	}
}

func TestScriptOKNeedsWorldAction(t *testing.T) {
	for _, scene := range []string{"dungeon", "stage"} {
		t.Run(scene, func(t *testing.T) {
			var g *Game
			var update func()
			var enable func()
			if scene == "stage" {
				var st *stageScene
				g, st, _ = wldStage(t)
				st.player.at = geom.Pt{X: 30, Y: 150}
				update = func() { st.update(g) }
				enable = func() { st.player.at = geom.Pt{X: 185, Y: 150} }
			} else {
				g = wldMaze(t, "fx_maze", 1, 1, 1)
				ds, ok := g.stack[0].(*dungeonScene)
				if !ok {
					t.Fatalf("scene %T", g.stack[0])
				}
				update = func() { ds.update(g) }
				enable = func() {
					g.st.Pos.X, g.st.Pos.Y, g.st.Pos.Facing = 1, 3, 1
					tile := dungeon.Front(g.db, g.st).Tile
					if tile == nil {
						t.Fatal("missing front tile")
					}
					tile.Trigger = "bump"
					tile.Do = []data.Command{{Say: &data.Say{Text: "A marker."}}}
				}
			}
			s := sysScript(t, "press ok")
			s.acknowledge = true
			for range 3 {
				sysKey(g)
				s.step(&g.in)
				update()
				g.in.finishOffer()
				if s.pc != 0 {
					t.Fatal("OK consumed without a nearby action")
				}
			}
			enable()
			sysKey(g)
			s.step(&g.in)
			update()
			g.in.finishOffer()
			if s.pc != 1 {
				t.Fatal("nearby action did not accept pending OK")
			}
		})
	}
}

// TestScriptTapFlows drives the tap flows as a script does, through Update:
// the phone's home bar puts a page down and its event goes on; a tap outside a
// confirmation takes the question back; a fallen party's choice is picked by
// the name of its way.
func TestScriptTapFlows(t *testing.T) {
	g := sysOpen(t, Options{Location: "camp", SaveDir: t.TempDir(), Development: true, Script: "wait 100000"}, nil)
	sysFrames(t, g, 12)
	run := func(src string, frames int) {
		t.Helper()
		sc := sysScript(t, src+"\nwait 100000")
		s := g.in.script
		s.cmds, s.pc, s.wait = sc.cmds, 0, 0
		sysFrames(t, g, frames)
		if s.pc < len(sc.cmds)-1 {
			t.Fatalf("the script stopped at command %d of %q", s.pc, src)
		}
	}

	run("do [{phone: {app: page, rows: [{text: News}]}}, {set: {var.read: 1}}]\nwait 40", 60)
	if g.phone == nil {
		t.Fatal("the phone did not come up")
	}
	sx, sy, w, h := g.phoneScreenRect(false)
	run(fmt.Sprintf("click %d %d\nwait 40", int(sx+w/2), int(sy+h-11)), 80)
	if g.phone != nil || g.st.Vars["read"] != float64(1) {
		t.Fatalf("the home bar: phone up %v, read %v", g.phone != nil, g.st.Vars["read"])
	}

	asked, refused := 0, 0
	c := newConfirm("Overwrite?", func() { asked++ })
	c.onNo = func() { refused++ }
	g.push(c)
	run("wait 10\nclick 6 6\nwait 4", 40)
	if asked != 0 || refused != 1 || g.stack[len(g.stack)-1] == scene(c) {
		t.Fatalf("a tap outside the confirmation: yes %d no %d", asked, refused)
	}

	retried := 0
	g.push(newChoice("The party has fallen.", "Retry", "Give up", func() { retried++ }))
	run("wait 10\npick Retry\nwait 4", 40)
	if retried != 1 {
		t.Fatalf("pick Retry answered %d times", retried)
	}
}
