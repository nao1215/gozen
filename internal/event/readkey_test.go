package event_test

import (
	"testing"

	"github.com/nao1215/gozen/internal/event"
	"github.com/nao1215/gozen/internal/testgame"
)

// TestLineKeyIsTheSource: a line's key is its source as written (ADR 0026). It stays the same
// whatever its {expressions} hold, and differs with the speaker or the text.
func TestLineKeyIsTheSource(t *testing.T) {
	db := testgame.Load(t, map[string]string{"data/z.yaml": `events: {count: {do: [
{say: {who: scout, text: "Gold {gold}."}},
{say: {who: Courier, text: "Gold {gold}."}},
{choice: [{label: "Keep {gold}"}, {label: Spend}], prompt: "And now?"}
]}}`})
	lines := func(gold int) []event.Request {
		st := testgame.State(db, 1)
		st.Gold = gold
		r := event.New(db, st, &host{})
		r.Log = func(err error) { t.Error(err) }
		r.Start(db.Events["count"].Do)
		out := []event.Request{r.Next(), r.Next(), r.Next()}
		return out
	}
	a, b := lines(5), lines(9)
	sa, sb := testgame.As[event.Say](t, a[0]), testgame.As[event.Say](t, b[0])
	if sa.Text == sb.Text {
		t.Fatalf("the fixture should show different text: %q", sa.Text)
	}
	if sa.Key == 0 || sa.Key != sb.Key || sa.Key != event.SourceKey("scout", "Gold {gold}.") {
		t.Errorf("keys of one source: %x and %x", sa.Key, sb.Key)
	}
	if other := testgame.As[event.Say](t, a[1]); other.Key == sa.Key {
		t.Error("another speaker has the same key")
	}
	ca, cb := testgame.As[event.Choice](t, a[2]), testgame.As[event.Choice](t, b[2])
	if len(ca.Keys) != 2 || ca.Keys[0] != cb.Keys[0] || ca.Keys[0] == ca.Keys[1] || ca.Keys[0] != event.ChoiceKey("And now?", "Keep {gold}") {
		t.Errorf("option keys %x and %x", ca.Keys, cb.Keys)
	}
	if event.ChoiceKey("And now?", "Spend") == event.SourceKey("And now?", "Spend") {
		t.Error("an option and a line share a key")
	}
}

// TestChoseRecordsTheOption: a choice with an id remembers the option taken: its id, or its
// number as written (not as shown, when an option before it is hidden by when).
func TestChoseRecordsTheOption(t *testing.T) {
	db := testgame.Load(t, map[string]string{"game.yaml": "schema_version: 4\nid: b\n" + testgame.Game, "data/z.yaml": `events: {dinner: {do: [
{choice: [{id: soup, label: Soup}, {label: Bread}], id: first},
{choice: [{label: Hidden, when: "false"}, {label: Tea}], id: second},
{choice: [{label: Plain}]}
]}}`})
	st := testgame.State(db, 1)
	r := event.New(db, st, &host{})
	r.Log = func(err error) { t.Error(err) }
	r.Start(db.Events["dinner"].Do)
	for _, n := range []int{0, 0, 0} {
		if _, ok := r.Next().(event.Choice); !ok {
			t.Fatal("no choice")
		}
		r.Answer(n)
	}
	if st.Chose["first"] != "soup" || st.Chose["second"] != "1" || len(st.Chose) != 2 {
		t.Fatalf("chose %v", st.Chose)
	}
}
