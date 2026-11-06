package event_test

import (
	"testing"

	"github.com/nao1215/gozen/internal/event"
	data "github.com/nao1215/gozen/internal/layer2/gamedata"
	"github.com/nao1215/gozen/internal/testgame"
)

type phoneHost struct{ got []event.PhoneEffect }

func (h *phoneHost) Handle(e event.Effect) {
	if p, ok := e.(event.PhoneEffect); ok {
		h.got = append(h.got, p)
	}
}

// TestPhoneWaitsWithFilledTexts: the phone command hands the screen its phone
// with every text interpolated, waits on a Tap, and changes no state.
func TestPhoneWaitsWithFilledTexts(t *testing.T) {
	db := testgame.Load(t, map[string]string{"data/z.yaml": `
events:
  e:
    do:
      - set: {var.n: 3}
      - phone:
          app: chat
          title: "Team {var.n}"
          messages:
            - {from: "Ann {var.n}", text: "{var.n} new"}
            - {video: {picture: x.png, text: "Clip {var.n}"}}
      - say: next
`})
	st := testgame.State(db, 1)
	h := &phoneHost{}
	r := event.New(db, st, h)
	r.Log = func(err error) { t.Error(err) }
	r.Start(db.Events["e"].Do)
	if _, ok := r.Next().(event.Tap); !ok {
		t.Fatal("the phone did not wait on a tap")
	}
	if len(h.got) != 1 {
		t.Fatalf("phone effects: %d", len(h.got))
	}
	p := h.got[0].Phone
	if p.Title != "Team 3" || p.Messages[0].From != "Ann 3" || p.Messages[0].Text != "3 new" || p.Messages[1].Video.Text != "Clip 3" {
		t.Fatalf("texts not filled: %+v %+v", p, p.Messages[1].Video)
	}
	if db.Events["e"].Do[1].Phone.Title != "Team {var.n}" {
		t.Fatal("filling changed the event's own phone")
	}
	if say, ok := r.Next().(event.Say); !ok || say.Text != "next" {
		t.Fatalf("the event did not go on after the phone: %+v", say)
	}
}

// TestPhoneResults: a page's button and a call keep what the player pressed
// in the phone's var and their sets; putting it down otherwise keeps nothing.
func TestPhoneResults(t *testing.T) {
	db := testgame.Load(t, map[string]string{"data/z.yaml": `
events:
  e:
    do:
      - phone:
          app: page
          title: Sign in
          var: var.mfa
          rows:
            - {text: Approve the sign-in?}
            - {button: Approve, set: {flag.approved: true}}
            - {button: Deny, id: deny}
      - phone:
          app: lock
          var: var.oncall
          notices: [{title: Alert, call: true, set: {var.woke: 1}}, {title: Alert, call: true}]
      - phone:
          app: page
          var: var.none
          rows: [{button: Go}]
      - say: next
`})
	st := testgame.State(db, 1)
	r := event.New(db, st, &phoneHost{})
	r.Log = func(err error) { t.Error(err) }
	r.Start(db.Events["e"].Do)
	if tap, ok := r.Next().(event.Tap); !ok || tap.Results != 2 {
		t.Fatalf("the page did not wait with its two buttons: %+v", tap)
	}
	r.Answer(0)
	if tap, ok := r.Next().(event.Tap); !ok || tap.Results != 3 {
		t.Fatalf("the lock screen did not wait with two answers and a decline: %+v", tap)
	}
	r.Answer(1)
	if tap, ok := r.Next().(event.Tap); !ok || tap.Results != 1 {
		t.Fatalf("the last page did not wait: %+v", tap)
	}
	r.Answer(-1)
	if say, ok := r.Next().(event.Say); !ok || say.Text != "next" {
		t.Fatalf("the event did not go on: %+v", say)
	}
	if st.Vars["mfa"] != "Approve" || st.Flags["approved"] != true {
		t.Fatalf("the button was not kept: %v %v", st.Vars["mfa"], st.Flags)
	}
	if st.Vars["oncall"] != "answer" || st.Vars["woke"] != nil {
		t.Fatalf("the second call answered: %v woke %v", st.Vars["oncall"], st.Vars["woke"])
	}
	if _, ok := st.Vars["none"]; ok {
		t.Fatal("a phone put down without a button kept a result")
	}
}

// TestPhoneFeedLikes: a feed whose posts have a set waits with one result for
// each of them and keeps the sets of the posts its answer marks liked, in the
// posts' order; a feed only browsed, or one put down unread, keeps nothing.
func TestPhoneFeedLikes(t *testing.T) {
	db := testgame.Load(t, map[string]string{"data/z.yaml": `
events:
  e:
    do:
      - set: {var.mood: 5}
      - phone:
          app: feed
          posts:
            - {who: ann, picture: x.png, set: {var.mood: var.mood - 1, var.order: '"ann"'}}
            - {who: bo, picture: x.png}
            - {who: cy, text: Lunch, set: {flag.liked_cy: true, var.order: 'var.order + "cy"'}}
            - {who: di, text: Rain, set: {flag.liked_di: true}}
      - phone:
          app: feed
          posts: [{who: ann, picture: x.png, set: {flag.again: true}}]
      - phone:
          app: feed
          posts: [{who: ann, picture: x.png}]
      - say: next
`})
	st := testgame.State(db, 1)
	r := event.New(db, st, &phoneHost{})
	r.Log = func(err error) { t.Error(err) }
	r.Start(db.Events["e"].Do)
	if tap, ok := r.Next().(event.Tap); !ok || tap.Results != 3 {
		t.Fatalf("the feed did not wait with its three kept posts: %+v", tap)
	}
	if !r.Running() {
		t.Fatal("the runner stopped while the feed waited for its likes")
	}
	r.Answer(1 | 2) // ann and cy liked, di not
	if tap, ok := r.Next().(event.Tap); !ok || tap.Results != 1 {
		t.Fatalf("the second feed did not wait: %+v", tap)
	}
	r.Answer(-1)
	if tap, ok := r.Next().(event.Tap); !ok || tap.Results != 0 {
		t.Fatalf("a feed without sets waited for a result: %+v", tap)
	}
	if say, ok := r.Next().(event.Say); !ok || say.Text != "next" {
		t.Fatalf("the event did not go on: %+v", say)
	}
	if st.Vars["mood"] != float64(4) || st.Flags["liked_cy"] != true || st.Flags["liked_di"] == true {
		t.Fatalf("likes kept: mood %v flags %v", st.Vars["mood"], st.Flags)
	}
	if st.Vars["order"] != "anncy" {
		t.Fatalf("the sets ran out of the posts' order: %v", st.Vars["order"])
	}
	if st.Flags["again"] == true {
		t.Fatal("a feed put down unread kept a like")
	}
}

// TestPhoneFeedBrowsedKeepsNothing: browse turns likes off, so the feed waits
// for no result even with a post's set (gozen check rejects the set).
func TestPhoneFeedBrowsedKeepsNothing(t *testing.T) {
	db := testgame.Load(t, map[string]string{"data/z.yaml": `
events:
  e:
    do:
      - phone: {app: feed, browse: true, posts: [{picture: x.png}]}
`})
	cue := *db.Events["e"].Do[0].Phone
	cue.Posts[0].Set = map[string]data.Expr{"flag.x": data.MustExpr("true")}
	if cue.Liking() != nil || cue.Liked(1) != nil {
		t.Fatalf("a browsed feed keeps likes: %v %v", cue.Liking(), cue.Liked(1))
	}
	cue.Browse = false
	if got := cue.Liking(); len(got) != 1 || len(cue.Liked(1)) != 1 || len(cue.Liked(2)) != 0 {
		t.Fatalf("the feed's likes: %v %v", got, cue.Liked(2))
	}
}

// TestPhoneLocked: a row's locked condition is worked out when the phone comes up.
func TestPhoneLocked(t *testing.T) {
	db := testgame.Load(t, map[string]string{"data/z.yaml": `
events:
  e:
    do:
      - phone: {app: page, rows: [{pictures: [x.png], locked: '!flag.supported', label: "Supporters {var.n}"}, {picture: x.png, locked: true}, {picture: x.png}]}
      - set: {flag.supported: true}
      - phone: {app: page, rows: [{pictures: [x.png], locked: '!flag.supported'}]}
`})
	st := testgame.State(db, 1)
	h := &phoneHost{}
	r := event.New(db, st, h)
	r.Log = func(err error) { t.Error(err) }
	r.Start(db.Events["e"].Do)
	r.Next()
	r.Next()
	rows := h.got[0].Phone.Rows
	if !rows[0].Shut || !rows[1].Shut || rows[2].Shut || rows[0].Label != "Supporters 0" {
		t.Fatalf("locked rows: %v %v %v %q", rows[0].Shut, rows[1].Shut, rows[2].Shut, rows[0].Label)
	}
	if h.got[1].Phone.Rows[0].Shut {
		t.Fatal("the grid stayed locked after flag.supported was set")
	}
}

// TestPhoneBanner: a banner does not wait; the event goes on at once.
func TestPhoneBanner(t *testing.T) {
	db := testgame.Load(t, map[string]string{"data/z.yaml": `
events:
  open: {do: [{say: opened}]}
  e:
    do:
      - phone: {app: banner, notices: [{app: Chat, title: "Ann", text: "Lunch?", event: open}]}
      - say: next
`})
	st := testgame.State(db, 1)
	h := &phoneHost{}
	r := event.New(db, st, h)
	r.Log = func(err error) { t.Error(err) }
	r.Start(db.Events["e"].Do)
	if say, ok := r.Next().(event.Say); !ok || say.Text != "next" || len(h.got) != 1 {
		t.Fatalf("the banner waited: %+v, effects %d", say, len(h.got))
	}
}
