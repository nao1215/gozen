package event_test

import (
	"testing"

	"github.com/nao1215/gozen/internal/event"
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
