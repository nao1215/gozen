package play

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/nao1215/gozen/internal/savefile"

	"github.com/nao1215/gozen/internal/event"
	data "github.com/nao1215/gozen/internal/layer2/gamedata"
)

// Phone lifecycle tests integrate the view with event replay, saves and battle waits.
func TestPhoneReleasedOnResetAndClose(t *testing.T) {
	for _, action := range []string{"load", "close"} {
		t.Run(action, func(t *testing.T) {
			g := phoneGame(t, data.PhoneCue{App: "chat", Messages: []data.PhoneMessage{{Text: "Hello"}}})
			g.banners = append(g.banners, phoneBanner{n: data.PhoneNotice{Text: "Old"}})
			if action == "load" {
				g.loadGame(g.st)
				if len(g.banners) != 0 {
					t.Fatal("a load kept the banners of the game before it")
				}
			} else if err := g.Close(); err != nil {
				t.Fatal(err)
			}
			if g.phone != nil {
				t.Fatal("the discarded phone is still attached to the game")
			}
		})
	}
}

func TestPhoneSaveDoesNotLoseEvent(t *testing.T) {
	g := phoneGame(t, data.PhoneCue{App: "chat", Messages: []data.PhoneMessage{{Text: "Hello"}}})
	g.events.Start([]data.Command{{Phone: &g.phone.cue}, {Say: &data.Say{Text: "Continue here"}}})
	g.pump()
	if _, err := g.saveBytes(); err == nil {
		t.Fatal("saving during a phone discarded the waiting event continuation")
	}
}

func TestPhoneCursorReplayAndRollback(t *testing.T) {
	g, _ := btlOpen(t, Options{Location: "camp"}, "schema_version: 11\nui: {message: novel}\n", `
events:
  phone_story:
    - say: Before
    - phone: {app: chat, messages: [{text: Hello}]}
    - say: After
    - say: End
`)
	g.enterAt = ""
	g.events.Start(g.db.Events["phone_story"].Do)
	g.liveCatchUp(&fastTo{lines: 3, strict: true}, "phone_story", nil)
	if g.failure != nil || at(g) != "End" || g.phone != nil {
		t.Fatalf("replay stopped at %q with phone=%v: %v", at(g), g.phone != nil, g.failure)
	}
	if err := g.save(0); err != nil {
		t.Fatal(err)
	}
	st, err := g.load(0)
	if err != nil || st.Resume == nil {
		t.Fatalf("load cursor: %v", err)
	}
	g.loadSavedGame(st)
	if g.failure != nil || at(g) != "End" || g.phone != nil {
		t.Fatalf("load stopped at %q with phone=%v: %v", at(g), g.phone != nil, g.failure)
	}
	g.rollBack()
	if at(g) != "After" || g.phone != nil {
		t.Fatalf("Back replay stopped at %q with phone=%v", at(g), g.phone != nil)
	}
	g.rollBack()
	if at(g) != "Before" || g.phone != nil {
		t.Fatalf("Back across the phone stopped at %q", at(g))
	}
	g.talk.shown = g.talk.total
	g.in = input{pressed: [numButtons]bool{btnOK: true}}
	g.pump()
	g.in = input{}
	g.pump()
	if g.phone == nil || g.phone.shown != 1 || g.phone.age > 1 {
		t.Fatal("advancing after Back did not show a fresh phone")
	}
}

func TestPhoneInBattleWaitsAndContinues(t *testing.T) {
	g, _ := btlOpen(t, Options{Battle: "one"}, "", "")
	s := btlBattle(t, g)
	s.runner.Start([]data.Command{
		{Phone: &data.PhoneCue{App: "feed", Posts: []data.PhonePost{{Picture: "x.png"}}}},
		{Say: &data.Say{Text: "Battle continues"}},
	})
	s.phase, s.req = bpEvent, nil
	s.event(g)
	if g.phone == nil {
		t.Fatal("battle did not open its phone")
	}
	for range phoneRise {
		g.in = input{}
		s.event(g)
	}
	g.in = input{pressed: [numButtons]bool{btnOK: true}}
	s.event(g)
	for range phoneRise + 2 {
		g.in = input{}
		s.event(g)
	}
	if g.phone != nil || at(g) != "Battle continues" {
		t.Fatalf("battle phone did not resume its event: phone=%v line=%q", g.phone != nil, at(g))
	}
}

// TestPhoneResultBlocksScreenSave: a screen transcript keeps no phone results,
// so a phone with results (a feed's likes) shown in a screen's root keeps the
// root from being saved mid-screen instead of losing the likes on load.
func TestPhoneResultBlocksScreenSave(t *testing.T) {
	g := phoneGame(t, data.PhoneCue{App: "chat", Messages: []data.PhoneMessage{{Text: "Hello"}}})
	g.begin(event.Tap{})
	if g.screenTrace.blocked {
		t.Fatal("a phone without results blocked the screen transcript")
	}
	g.begin(event.Tap{Results: 2})
	if !g.screenTrace.blocked {
		t.Fatal("a phone with results left the screen transcript open to a save")
	}
}

func TestPhoneScriptLinesAndRead(t *testing.T) {
	for _, command := range []string{"lines chat", "press ok\npress ok\npress ok", "read 4", "wait 40\nlook\nread 4", "wait 40\nsnapshot\nread 4"} {
		t.Run(command, func(t *testing.T) {
			var trace bytes.Buffer
			_, err := Simulate(context.Background(), Options{
				FS: os.DirFS("../../e2e/atago/fixtures/input-readiness"), Location: "studio", SaveDir: t.TempDir(), Trace: &trace,
				Script: "wait 20\ndo open: board\nwait 10\npick Chat\n" + command + "\nsnapshot\nquit",
			}, SimulateConfig{})
			if err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(trace.String(), `"chatted":1`) {
				t.Fatalf("phone did not finish: %s", &trace)
			}
		})
	}
}

func TestPhoneScriptInputWaitsForReceiver(t *testing.T) {
	for _, verb := range []string{"press", "click"} {
		t.Run(verb, func(t *testing.T) {
			g := phoneGame(t, data.PhoneCue{App: "chat", Messages: []data.PhoneMessage{{Text: "Hello"}, {Text: "Next"}}})
			g.stack = append(g.stack, &authorScreen{running: true})
			g.phone.age, g.phone.turned = 0, phoneTurn
			s := &script{cmds: []scriptCmd{{verb: verb, arg: "ok"}, {verb: "wait", n: 1000}}, ready: g.scriptReady, acknowledge: true}
			s.step(&g.in)
			g.updatePhone()
			g.in.finishOffer()
			if s.pc != 0 {
				t.Fatal("input consumed while phone was rising")
			}
			g.phone.age = phoneRise
			g.phone.typing = phoneTyping
			for g.phone.typing > 0 {
				sysKey(g)
				s.step(&g.in)
				g.updatePhoneChat(g.phone)
				g.in.finishOffer()
				if s.pc != 0 {
					t.Fatalf("input consumed during typing: remaining=%d", g.phone.typing)
				}
			}
			sysKey(g)
			s.step(&g.in)
			g.updatePhoneChat(g.phone)
			g.in.finishOffer()
			if s.pc != 1 {
				t.Fatalf("ready phone did not receive input: pc=%d", s.pc)
			}
		})
	}
}

// Raw pointer edges must survive banner consumption in the recording, while
// dispatch still consumes them before the opened event receives input.
func TestPhoneBannerRecordingReplay(t *testing.T) {
	fsys := gameFiles(t)
	fsys["game.yaml"].Data = append([]byte("schema_version: 11\n"), fsys["game.yaml"].Data...)
	fsys["data/banner.yaml"] = &fstest.MapFile{Data: []byte(`events:
  notify:
    - say: Ready.
    - phone: {app: banner, notices: [{title: Notice, event: opened}]}
  opened:
    - set: {var.opened: 'var.opened + 1', flag.opened: true}
    - phone: {app: banner, var: var.result, notices: [{title: Dismiss me}]}
    - set: {var.finished: 'var.finished + 1'}
`)}
	var recording bytes.Buffer
	g, err := New(Options{FS: fsys, Location: "camp", Event: "notify", Seed: 7, Trace: &recording, NoSound: true, Store: savefile.MemStore()})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := g.Close(); err != nil {
			t.Error(err)
		}
	})
	sw, _ := g.screen()
	// Supply physical-frame input through the existing input adapter, without
	// installing a game replay or bypassing Update's normal dispatch.
	g.in.replay = &replay{inputs: map[int]traceLine{
		20:  {"press": []any{"ok"}},
		60:  {"click": true, "pointer_held": true, "pointer_up": true, "mouse": []any{sw / 2, float64(40)}},
		100: {"esc": true, "press": []any{"cancel", "menu"}},
	}}
	type frame struct {
		Banners          []phoneBanner
		Phone            bool
		Closing          int
		Opened, Finished any
		Result           any
	}
	snapshot := func(g *Game) frame {
		f := frame{Banners: append([]phoneBanner(nil), g.banners...), Phone: g.phone != nil, Opened: g.st.Vars["opened"], Finished: g.st.Vars["finished"], Result: g.st.Vars["result"]}
		if g.phone != nil {
			f.Closing = g.phone.closing
		}
		return f
	}
	var frames []frame
	for range 140 {
		if err := g.Update(); err != nil {
			t.Fatal(err)
		}
		frames = append(frames, snapshot(g))
	}
	if g.st.Vars["opened"] != float64(1) || g.st.Vars["finished"] != float64(1) || g.st.Vars["result"] != "dismiss" || !g.st.Flags["opened"] {
		t.Fatalf("event did not run exactly once and dismiss: %+v\n%s", snapshot(g), &recording)
	}
	g.traceEnd()
	inputs := traceLines(t, recording.Bytes(), "input")
	if len(inputs) != 5 || inputs[3]["click"] != true || inputs[3]["pointer_up"] != true || inputs[3]["pointer_held"] != true || inputs[4]["esc"] != true {
		t.Fatalf("lost or duplicated raw input: %v", inputs)
	}
	if opens := traceLines(t, recording.Bytes(), "banner_open"); len(opens) != 1 {
		t.Fatalf("banner dispatched %d times", len(opens))
	}
	path := filepath.Join(t.TempDir(), "banner.jsonl")
	if err := os.WriteFile(path, recording.Bytes(), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := g.Close(); err != nil {
		t.Fatal(err)
	}
	var replayTrace bytes.Buffer
	r, err := New(Options{FS: fsys, Replay: path, Trace: &replayTrace, NoSound: true, Store: savefile.MemStore()})
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := r.Close(); err != nil {
			t.Error(err)
		}
	}()
	for i, want := range frames {
		if err := r.Update(); err != nil {
			t.Fatal(err)
		}
		if got := snapshot(r); !reflect.DeepEqual(got, want) {
			t.Fatalf("frame %d: got %+v, want %+v", i+1, got, want)
		}
	}
	r.traceEnd()
	if r.rep.diverge != "" || !r.rep.finished {
		t.Fatalf("replay: %s", r.rep.report())
	}
	if opens := traceLines(t, replayTrace.Bytes(), "banner_open"); len(opens) != 1 {
		t.Fatalf("replay dispatched %d times", len(opens))
	}
}
