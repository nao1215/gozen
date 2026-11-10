package play

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	"github.com/nao1215/gozen/internal/base/savefile"
	lang "github.com/nao1215/gozen/internal/base/text"
	data "github.com/nao1215/gozen/internal/model/gamedata"
	"github.com/nao1215/gozen/internal/model/gamepackage"
	"github.com/nao1215/gozen/internal/rules/event"
	"github.com/nao1215/gozen/internal/screen/gfx"
	"github.com/nao1215/gozen/internal/screen/sound"
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
				FS: os.DirFS("../../../e2e/atago/fixtures/input-readiness"), Location: "studio", SaveDir: t.TempDir(), Trace: &trace,
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
	g, err := New(Options{FS: fsys, Location: "camp", Event: "notify", Line: 1, Seed: 7, Trace: &recording, NoSound: true, Store: savefile.MemStore()})
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

// A home delegates to authored events and returns without recursive home calls.
// Saved flags rebuild static history; replay must not duplicate a sent reply.
func TestPhoneFlagNavigationSaveReplay(t *testing.T) {
	g, _ := btlOpen(t, Options{Location: "camp"}, "schema_version: 11\nui: {message: novel}\n", `
events:
  flag_phone:
    - set: {flag.contact: true, var.sent_count: 0}
    - phone:
        app: home
        title: Home
        rows:
          - {button: Hidden, when: flag.secret, event: contact_list}
          - {button: Contacts, event: contact_list}
    - say: Saved
    - say: End
  contact_list:
    - phone:
        app: contacts
        title: Contacts
        rows:
          - {item: {title: Hidden}, button: Hidden, when: flag.secret, event: thread}
          - {item: {title: Sam, value: '0100'}, button: Message, when: flag.contact, event: thread}
  thread:
    - phone:
        app: chat
        title: Sam
        history: true
        messages:
          - {from: Sam, text: Welcome, when: flag.contact}
          - {mine: true, text: Thanks, when: flag.sent}
        replies:
          - {button: Thanks, id: thanks, when: '!flag.sent', set: {flag.sent: true, var.sent_count: 'var.sent_count + 1'}}
`)
	waitPhone := func(title string) {
		t.Helper()
		for range 180 {
			g.in = input{}
			g.pump()
			if g.phone != nil && g.phone.cue.Title == title && g.phone.ready() {
				return
			}
		}
		t.Fatalf("phone %q not reached; line=%q error=%v", title, at(g), g.failure)
	}
	g.enterAt = ""
	g.events.Start(g.db.Events["flag_phone"].Do)
	waitPhone("Home")
	for range 70 {
		g.phone.putDown(1, -1, false)
		waitPhone("Contacts")
		if !g.phone.cue.Rows[0].Hidden || g.phone.cue.Rows[1].Hidden {
			t.Fatal("contact unlock condition lost")
		}
		g.phone.putDown(-4, -4, false)
		waitPhone("Home")
		frames, _ := g.events.Frames()
		if len(frames) > 4 {
			t.Fatalf("home visits grew event stack: %d", len(frames))
		}
	}
	g.phone.putDown(1, -1, false)
	waitPhone("Contacts")
	g.phone.putDown(1, -1, false)
	waitPhone("Sam")
	if !g.phone.cue.Messages[1].Hidden {
		t.Fatal("unsent message was visible")
	}
	g.phone.putDown(-4, -4, false)
	waitPhone("Contacts")
	if g.st.Flags["sent"] || g.st.Vars["sent_count"] != float64(0) {
		t.Fatal("closing without sending changed flags")
	}
	g.phone.putDown(1, -1, false)
	waitPhone("Sam")
	g.phone.putDown(-10, -1, false)
	waitPhone("Contacts")
	g.phone.putDown(1, -1, false)
	waitPhone("Sam")
	if g.phone.cue.Messages[1].Hidden || !g.phone.cue.Replies[0].Hidden {
		t.Fatal("sent flag did not rebuild authored history and hide reply")
	}
	g.phone.putDown(-4, -4, false)
	waitPhone("Contacts")
	g.phone.putDown(-4, -4, false)
	waitPhone("Home")
	g.phone.putDown(-4, -4, false)
	for range 90 {
		g.in = input{}
		g.pump()
	}
	if at(g) != "Saved" {
		t.Fatalf("closing home did not resume story: %q", at(g))
	}
	if err := g.save(0); err != nil {
		t.Fatal(err)
	}
	st, err := g.load(0)
	if err != nil {
		t.Fatal(err)
	}
	g.loadSavedGame(st)
	if g.failure != nil || g.phone != nil || at(g) != "Saved" || !g.st.Flags["sent"] || g.st.Vars["sent_count"] != float64(1) {
		t.Fatalf("saved phone results: phone=%v line=%q flags=%v vars=%v error=%v", g.phone != nil, at(g), g.st.Flags, g.st.Vars, g.failure)
	}
}

func TestPhoneEmbeddedGameOwnership(t *testing.T) {
	files := gameFiles(t)
	parent, err := New(Options{FS: files, NoSound: true, Store: savefile.MemStore(), headless: true})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if e := parent.Close(); e != nil {
			t.Error(e)
		}
	})
	restore := parent.activatePresentation()
	defer restore()
	gfx.SetScale(1.75)
	parentScale := gfx.Scale()
	child, err := newGame(Options{FS: files, NoSound: true, Store: phoneGameStore{Store: parent.store, prefix: "phone-save-test-"}, headless: true}, true)
	if err != nil {
		t.Fatal(err)
	}
	if gfx.Scale() != parentScale {
		t.Fatal("child constructor changed parent scale")
	}
	parent.newGame()
	resumeChild := child.activatePresentation()
	child.newGame()
	resumeChild()
	if child.st == parent.st || child.audio == parent.audio || child.graphics == parent.graphics {
		t.Fatal("child shares mutable runtime resources")
	}
	if err := child.store.Write("slot.json", []byte("child")); err != nil {
		t.Fatal(err)
	}
	if _, err := parent.store.Read("slot.json"); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("child wrote host save: %v", err)
	}
	closedListener := 0
	child.hiddenCleanup = func() { closedListener++ }
	if err := child.Close(); err != nil {
		t.Fatal(err)
	}
	if closedListener != 1 || child.hiddenCleanup != nil {
		t.Fatal("child retained browser visibility listener")
	}
	if gfx.Scale() != parentScale {
		t.Fatal("child Close changed parent presentation")
	}
	if _, err := New(Options{FS: files, NoSound: true}); err == nil {
		t.Fatal("closing child released outer runtime ownership")
	}
	if err := parent.Update(); err != nil {
		t.Fatal(err)
	}
}

func TestPhoneGameStoreIsolation(t *testing.T) {
	host := savefile.MemStore()
	a := phoneGameStore{Store: host, prefix: "phone-save-a-"}
	b := phoneGameStore{Store: host, prefix: "phone-save-b-"}
	if err := a.Write("slot.json", []byte("a")); err != nil {
		t.Fatal(err)
	}
	if err := b.Write("slot.json", []byte("b")); err != nil {
		t.Fatal(err)
	}
	for _, bad := range []string{"", "../slot", ".hidden", "a/b", `a\b`} {
		if err := a.Write(bad, nil); !errors.Is(err, fs.ErrInvalid) {
			t.Fatalf("accepted %q: %v", bad, err)
		}
	}
	names, err := a.List()
	if err != nil || !reflect.DeepEqual(names, []string{"slot.json"}) {
		t.Fatalf("list: %v %v", names, err)
	}
	if err := a.Remove("slot.json"); err != nil {
		t.Fatal(err)
	}
	value, err := b.Read("slot.json")
	if err != nil || string(value) != "b" {
		t.Fatalf("removed another release save: %q %v", value, err)
	}
}

func TestPhoneEmbeddedGamePauseAndReturn(t *testing.T) {
	parent := phoneGame(t, data.PhoneCue{App: "store"})
	parent.db.Game.Phone = &data.PhoneApps{Apps: []data.PhoneApp{{ID: "child"}}}
	parent.packages = &phonePackages{opened: map[string]*gamepackage.Installed{"child": {FS: gameFiles(t), Release: gamepackage.Release{ID: "neutral/child", Version: "1.0.0"}}}}
	p := parent.phone
	p.age, p.turned = phoneRise, phoneTurn
	beforeFrames := parent.st.Frames
	launchTime := time.Unix(4567890, 0)
	parent.opts.Now = func() time.Time { return launchTime }
	if err := parent.startPhoneGame(p, "child"); err != nil {
		t.Fatal(err)
	}
	child := p.child.game
	if !child.opts.Now().Equal(launchTime) {
		t.Fatal("child did not receive deterministic launch clock")
	}
	if err := child.startPhoneGame(newPhoneView(data.PhoneCue{}), "child"); err == nil {
		t.Fatal("nested embedding accepted")
	}
	parent.desk = &streamDesk{base: time.Unix(1000, 0), tick0: parent.tick}
	beforeTime := parent.streamNow()
	kind, items := parent.scriptPickables()
	if kind != "title" || len(items) == 0 {
		t.Fatal("script controls did not follow child foreground")
	}
	script, err := parseScript("wait 100")
	parent.in.script = script
	if err != nil {
		t.Fatal(err)
	}
	for range 3 {
		if err := parent.Update(); err != nil {
			t.Fatal(err)
		}
	}
	if !parent.streamNow().Equal(beforeTime) {
		t.Fatal("host stream clock advanced during child")
	}
	if parent.st.Frames != beforeFrames || child.tick == 0 {
		t.Fatalf("host advanced or child did not: host %d/%d child %d", parent.st.Frames, beforeFrames, child.tick)
	}
	parent.desk = nil
	// The host's explicit exit leaves the original app in place.
	parent.in = input{esc: true, shift: true}
	if err := parent.updatePhoneGame(p); err != nil {
		t.Fatal(err)
	}
	if p.child != nil || !child.closed || parent.phone != p || p.cue.App != "store" || p.closing != 0 {
		t.Fatal("child exit discarded the parent app")
	}
	if parent.in.esc || parent.in.click {
		t.Fatal("exit input leaked into parent")
	}
}

func TestPhoneInstalledHomePreservesResults(t *testing.T) {
	cue := data.PhoneCue{App: "home", Var: "picked", Rows: []data.PhoneRow{{Button: "Contacts", ID: "contacts"}}}
	parent := phoneGame(t, cue)
	parent.db.Game.Phone = &data.PhoneApps{Apps: []data.PhoneApp{{ID: "child"}}}
	parent.packages = &phonePackages{opened: map[string]*gamepackage.Installed{"child": {FS: gameFiles(t), Release: gamepackage.Release{ID: "neutral/child", Version: "1.0.0", Title: "Child"}}}}
	p := parent.phone
	before := cue.Results()
	parent.addPhoneGameApps(p)
	if len(p.cue.Rows) != 2 || p.appRows[1] != "child" || len(cue.Rows) != 1 {
		t.Fatal("installed app changed authored rows")
	}
	p.putDown(1, -1, false)
	if p.closing != 0 || p.selectedApp != "child" {
		t.Fatal("installed app became an authored result")
	}
	p.putDown(-4, -4, false)
	want := -1
	for i, r := range before {
		if r.Row == -4 && r.Notice == -4 {
			want = i
		}
	}
	if p.result != want {
		t.Fatalf("dismiss result moved: %d, want %d", p.result, want)
	}
}

// TestPhoneHomeErrorClearedByNextLaunch: the error of an app that failed to
// open leaves the home screen once another app has opened well.
func TestPhoneHomeErrorClearedByNextLaunch(t *testing.T) {
	parent := phoneGame(t, data.PhoneCue{App: "home", Rows: []data.PhoneRow{{Button: "Contacts", ID: "contacts"}}})
	parent.db.Game.Phone = &data.PhoneApps{Apps: []data.PhoneApp{{ID: "child"}}}
	parent.packages = &phonePackages{opened: map[string]*gamepackage.Installed{"child": {FS: gameFiles(t), Release: gamepackage.Release{ID: "neutral/child", Version: "1.0.0", Title: "Child"}}}}
	p := parent.phone
	phoneTicks(parent, phoneRise)
	p.selectedApp = "missing"
	parent.in = input{}
	parent.updatePhoneHome(p)
	if p.gameError == "" {
		t.Fatal("an unknown app opened without an error")
	}
	p.selectedApp = "child"
	parent.updatePhoneHome(p)
	if p.child == nil {
		t.Fatalf("the installed app did not open: %s", p.gameError)
	}
	if p.gameError != "" {
		t.Fatalf("the earlier app's error stays over the home screen: %q", p.gameError)
	}
}

// TestPhoneHomeShowsInstalledAppsWhileRising: the installed apps are on the
// home screen from the first frame drawn, not only once the phone has risen.
func TestPhoneHomeShowsInstalledAppsWhileRising(t *testing.T) {
	parent := phoneGame(t, data.PhoneCue{App: "home", Rows: []data.PhoneRow{{Button: "Contacts", ID: "contacts"}}})
	parent.db.Game.Phone = &data.PhoneApps{Apps: []data.PhoneApp{{ID: "child"}}}
	parent.packages = &phonePackages{opened: map[string]*gamepackage.Installed{"child": {FS: gameFiles(t), Release: gamepackage.Release{ID: "neutral/child", Version: "1.0.0", Title: "Child"}}}}
	p := parent.phone
	phoneTicks(parent, 2) // drawn from here on, while it rises
	if len(p.cue.Rows) != 2 {
		t.Fatalf("the rising home screen shows %d rows, want the authored row and the installed app", len(p.cue.Rows))
	}
}

func TestPhoneChildFailurePreservesHost(t *testing.T) {
	parent := phoneGame(t, data.PhoneCue{App: "game", Game: "child"})
	parent.db.Game.Phone = &data.PhoneApps{Apps: []data.PhoneApp{{ID: "child"}}}
	parent.packages = &phonePackages{opened: map[string]*gamepackage.Installed{"child": {FS: gameFiles(t), Release: gamepackage.Release{ID: "neutral/child", Version: "1.0.0"}}}}
	p := parent.phone
	p.age, p.turned = phoneRise, phoneTurn
	if err := parent.startPhoneGame(p, "child"); err != nil {
		t.Fatal(err)
	}
	child := p.child.game
	child.failure = errors.New("child failed")
	parent.in = input{}
	before := parent.st
	if err := parent.updatePhoneGame(p); err != nil {
		t.Fatalf("child error escaped into host: %v", err)
	}
	if p.child != nil || !child.closed || parent.st != before || parent.failure != nil || p.closing != 0 || !strings.Contains(p.gameError, "child failed") {
		t.Fatal("child error did not preserve host with visible failure")
	}
}

func TestPhoneChildRecordingIncludesTurnFrames(t *testing.T) {
	parent := phoneGame(t, data.PhoneCue{App: "game", Game: "child"})
	parent.db.Game.Phone = &data.PhoneApps{Apps: []data.PhoneApp{{ID: "child"}}}
	parent.packages = &phonePackages{opened: map[string]*gamepackage.Installed{"child": {FS: gameFiles(t), Release: gamepackage.Release{ID: "neutral/child", Version: "1.0.0", Orientation: "landscape"}}}}
	var pcm bytes.Buffer
	parent.opts.Audio = &pcm
	parent.audioSession().InitRecord(&pcm, 60)
	p := parent.phone
	if err := parent.startPhoneGame(p, "child"); err != nil {
		t.Fatal(err)
	}
	for range 5 {
		if err := parent.updatePhoneGame(p); err != nil {
			t.Fatal(err)
		}
	}
	const frameBytes = sound.SampleRate / 60 * 8
	if pcm.Len() != 5*frameBytes {
		t.Fatalf("turn lost audio frames: %d bytes", pcm.Len())
	}
	p.turned = phoneTurn
	parent.in = input{pheld: true}
	for range 3 {
		if err := parent.updatePhoneGame(p); err != nil {
			t.Fatal(err)
		}
	}
	if pcm.Len() != 8*frameBytes {
		t.Fatalf("held-entry gate lost audio frames: %d bytes", pcm.Len())
	}
	parent.in = input{esc: true, shift: true}
	if err := parent.updatePhoneGame(p); err != nil {
		t.Fatal(err)
	}
	if pcm.Len() != 9*frameBytes {
		t.Fatal("return lost audio frame")
	}
}

func TestEmbeddedSilentAudioTimeline(t *testing.T) {
	parent := sysMusicOpen(t, Options{})
	child, err := newGame(Options{FS: parent.opts.FS, NoSound: true, recordedAudio: true, Store: savefile.MemStore()}, true)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if e := child.Close(); e != nil {
			t.Error(e)
		}
	}()
	if !child.audioSession().Active() || child.tr != nil {
		t.Fatal("embedded silent replay lost music timing or created a second trace")
	}
}

func BenchmarkPresentationContextScope(b *testing.B) {
	words, err := lang.NewContext("en", nil)
	if err != nil {
		b.Fatal(err)
	}
	g := &Game{graphics: gfx.NewContext(), words: words}
	restore := g.activatePresentation()
	defer restore()
	b.ReportAllocs()
	for b.Loop() {
		g.activatePresentation()()
	}
}

func TestPhoneSpeechOwnership(t *testing.T) {
	oldCan, oldSpeak, oldStop := streamCanSpeak, streamSpeak, streamStopSpeak
	t.Cleanup(func() { streamCanSpeak, streamSpeak, streamStopSpeak = oldCan, oldSpeak, oldStop })
	starts, stops := 0, 0
	streamCanSpeak = func() bool { return true }
	streamSpeak = func(string, string, float64, float64) func() bool { starts++; return func() bool { return false } }
	streamStopSpeak = func() { stops++ }
	parent := streamsGame(t, "0")
	w := newStreamWatch(parent, parent.desk.shows[0], 100)
	parent.push(w)
	parent.desk.speak = true
	w.caption, w.captionWho = "Please wait.", 0
	w.readAloud(parent, 0, w.caption)
	resume := parent.suspendPhoneSpeech()
	if starts != 1 || stops != 1 || w.spoken != nil {
		t.Fatal("parent speech did not suspend")
	}
	child := &Game{stack: []scene{&streamWatch{spoken: func() bool { return false }}}}
	child.stopOwnedSpeech()
	resume()
	if starts != 2 || stops != 2 || w.spoken == nil {
		t.Fatal("child speech did not stop before parent restart")
	}
	resume = parent.suspendPhoneSpeech()
	parent.stack = parent.stack[:len(parent.stack)-1]
	resume()
	if starts != 2 {
		t.Fatal("restarted a watch that no longer owns the screen")
	}
}

func TestPhoneChildResetDuringInput(t *testing.T) {
	parent := phoneGame(t, data.PhoneCue{App: "game", Game: "child"})
	parent.db.Game.Phone = &data.PhoneApps{Apps: []data.PhoneApp{{ID: "child"}}}
	parent.packages = &phonePackages{opened: map[string]*gamepackage.Installed{"child": {FS: gameFiles(t), Release: gamepackage.Release{ID: "neutral/child", Version: "1.0.0"}}}}
	if err := parent.startPhoneGame(parent.phone, "child"); err != nil {
		t.Fatal(err)
	}
	child := parent.phone.child.game
	script, err := parseScript("do []\nwait 20")
	if err != nil {
		t.Fatal(err)
	}
	script.do = func(string) { parent.closePhone() }
	parent.in.script = script
	if err := parent.Update(); err != nil {
		t.Fatal(err)
	}
	if parent.phone != nil || !child.closed {
		t.Fatal("input reset retained child runtime")
	}
}

func TestPhoneHostCloseDoesNotResumeChildResources(t *testing.T) {
	parent := phoneGame(t, data.PhoneCue{App: "game", Game: "child"})
	parent.db.Game.Phone = &data.PhoneApps{Apps: []data.PhoneApp{{ID: "child"}}}
	parent.packages = &phonePackages{opened: map[string]*gamepackage.Installed{"child": {FS: gameFiles(t), Release: gamepackage.Release{ID: "neutral/child", Version: "1.0.0"}}}}
	if err := parent.startPhoneGame(parent.phone, "child"); err != nil {
		t.Fatal(err)
	}
	child := parent.phone.child.game
	resumed := false
	parent.phone.child.resume = func() { resumed = true }
	if err := parent.Close(); err != nil {
		t.Fatal(err)
	}
	if resumed || !child.closed {
		t.Fatal("host shutdown resumed paused devices or retained child")
	}
}

func TestPhoneChildScriptFailure(t *testing.T) {
	parent := phoneGame(t, data.PhoneCue{App: "game", Game: "child"})
	parent.db.Game.Phone = &data.PhoneApps{Apps: []data.PhoneApp{{ID: "child"}}}
	parent.packages = &phonePackages{opened: map[string]*gamepackage.Installed{"child": {FS: gameFiles(t), Release: gamepackage.Release{ID: "neutral/child", Version: "1.0.0"}}}}
	if err := parent.startPhoneGame(parent.phone, "child"); err != nil {
		t.Fatal(err)
	}
	script, err := parseScript("press ok")
	if err != nil {
		t.Fatal(err)
	}
	script.ready = func(string) bool { return false }
	script.blockedPC, script.blocked = 0, 6000
	parent.in.script = script
	before := parent.phone.child.game.tick
	if err := parent.Update(); err == nil || !strings.Contains(err.Error(), "timed out") {
		t.Fatalf("script failure lost: %v", err)
	}
	if parent.phone.child.game.tick != before {
		t.Fatal("child advanced after fatal scripted input")
	}
}

func TestPhoneChildAudioFailureDuringEntry(t *testing.T) {
	for _, gate := range []string{"rotation", "held input"} {
		t.Run(gate, func(t *testing.T) {
			parent := phoneGame(t, data.PhoneCue{App: "game", Game: "child"})
			parent.db.Game.Phone = &data.PhoneApps{Apps: []data.PhoneApp{{ID: "child"}}}
			parent.packages = &phonePackages{opened: map[string]*gamepackage.Installed{"child": {FS: gameFiles(t), Release: gamepackage.Release{ID: "neutral/child", Version: "1.0.0"}}}}
			p := parent.phone
			if err := parent.startPhoneGame(p, "child"); err != nil {
				t.Fatal(err)
			}
			child := p.child.game
			child.audioSession().InitRecord(failingTrace{err: errors.New("child audio failed")}, 60)
			if gate == "held input" {
				p.age, p.turned = phoneRise, phoneTurn
				parent.in = input{pheld: true}
			}
			if err := parent.updatePhoneGame(p); err != nil {
				t.Fatalf("child error escaped into host: %v", err)
			}
			if p.child != nil || !child.closed || parent.failure != nil || !strings.Contains(p.gameError, "child audio failed") {
				t.Fatal("entry failure did not preserve host with visible child error")
			}
		})
	}
}

func TestPhoneAtEventEntryIsNotSkippedWithoutSeek(t *testing.T) {
	for _, line := range []int{0, 1} {
		t.Run(fmt.Sprintf("line_%d", line), func(t *testing.T) {
			g, _ := btlOpen(t, Options{Event: "phone_entry", Line: line}, "", "events:\n  phone_entry:\n    do:\n    - phone: {app: chat, title: Entry, messages: [{text: Read this first.}]}\n    - say: After the phone.\n")
			sysFrames(t, g, 60)
			if line == 0 {
				if g.phone == nil || g.talk.active || g.fast != nil {
					t.Fatal("default event entry skipped its first phone")
				}
			} else if g.phone != nil || !g.talk.active || g.talk.say.Text != "After the phone." {
				t.Fatal("explicit first-line seek did not retain its replay behavior")
			}
		})
	}
}
