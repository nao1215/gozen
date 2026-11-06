package play

import (
	"testing"

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
