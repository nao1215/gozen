package event_test

import (
	"testing"

	"github.com/nao1215/gozen/internal/rules/event"
	"github.com/nao1215/gozen/internal/testgame"
)

// TestPhoneStayingKeys: a page used as a form. Each key that stays toggles its
// number and the same page comes back at once (Again) with its selected and
// texts worked out anew; the button that does not stay puts the phone down and
// the event goes on, its stack no deeper than before the keys were pressed.
func TestPhoneStayingKeys(t *testing.T) {
	db := testgame.Load(t, map[string]string{"data/z.yaml": `
events:
  e:
    do:
      - phone:
          app: page
          title: "Picked {var.count}"
          var: var.key
          rows:
            - {button: "1", id: k1, columns: 3, stay: true, selected: var.n1 == 1, set: {var.n1: 1 - var.n1, var.count: var.count + 1 - 2 * var.n1}}
            - {button: "2", id: k2, columns: 3, stay: true, selected: var.n2 == 1, set: {var.n2: 1 - var.n2, var.count: var.count + 1 - 2 * var.n2}}
            - {text: "{var.count}", columns: 3, selected: var.count > 0}
            - {button: Buy, id: buy, when: var.count > 0}
      - say: "bought {var.key}"
`})
	st := testgame.State(db, 1)
	h := &phoneHost{}
	r := event.New(db, st, h)
	r.Log = func(err error) { t.Error(err) }
	r.Start(db.Events["e"].Do)
	if _, ok := r.Next().(event.Tap); !ok {
		t.Fatal("the page did not wait")
	}
	first := h.got[0]
	if first.Again || first.Phone.Rows[0].Lit || first.Phone.Rows[2].Lit || !first.Phone.Rows[3].Hidden {
		t.Fatalf("first page: again %v, lit %v %v, buy hidden %v", first.Again, first.Phone.Rows[0].Lit, first.Phone.Rows[2].Lit, first.Phone.Rows[3].Hidden)
	}
	r.Answer(0) // key 1
	if _, ok := r.Next().(event.Tap); !ok {
		t.Fatal("a key that stays put the phone down")
	}
	again := h.got[1]
	if !again.Again || !again.Phone.Rows[0].Lit || again.Phone.Rows[1].Lit || again.Phone.Title != "Picked 1" || again.Phone.Rows[2].Text != "1" || again.Phone.Rows[3].Hidden {
		t.Fatalf("after key 1: again %v, lit %v %v, title %q, text %q, buy hidden %v",
			again.Again, again.Phone.Rows[0].Lit, again.Phone.Rows[1].Lit, again.Phone.Title, again.Phone.Rows[2].Text, again.Phone.Rows[3].Hidden)
	}
	for range 30 { // toggling many times neither nests events nor ends the page
		r.Answer(1)
		if _, ok := r.Next().(event.Tap); !ok {
			t.Fatal("toggling key 2 ended the page")
		}
	}
	last := h.got[len(h.got)-1]
	if last.Phone.Rows[1].Lit || last.Phone.Title != "Picked 1" {
		t.Fatalf("an even number of presses left key 2 lit %v, title %q", last.Phone.Rows[1].Lit, last.Phone.Title)
	}
	shown := len(h.got)
	r.Answer(2) // buy
	say, ok := r.Next().(event.Say)
	if !ok || say.Text != "bought buy" {
		t.Fatalf("buy did not put the phone down and go on: %+v", say)
	}
	if len(h.got) != shown {
		t.Fatal("the page was shown again after buy")
	}
	if got := st.Vars["n1"]; got != 1.0 {
		t.Fatalf("var.n1 = %v", got)
	}
}

// TestPhoneStayingKeysUnattended: a run without a player never presses a key
// that stays (it would toggle forever); it takes the first button that ends.
func TestPhoneStayingKeysUnattended(t *testing.T) {
	db := testgame.Load(t, map[string]string{"data/z.yaml": `
events:
  e:
    do:
      - phone:
          app: page
          var: var.key
          rows:
            - {button: "1", id: k1, columns: 3, stay: true, set: {var.n1: 1}}
            - {button: Buy, id: buy}
`})
	st := testgame.State(db, 1)
	r := event.New(db, st, &phoneHost{})
	r.Log = func(err error) { t.Error(err) }
	r.Start(db.Events["e"].Do)
	r.RunUnattended()
	if v, ok := st.Vars["n1"]; ok && v != 0.0 {
		t.Logf("n1=%v", v)
		t.Fatal("unattended play pressed a key that stays")
	}
}
