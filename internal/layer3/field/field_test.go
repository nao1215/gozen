package field_test

import (
	"strings"
	"testing"

	"github.com/nao1215/gozen/internal/layer3/field"
	"github.com/nao1215/gozen/internal/testgame"
)

func labels(es []field.Entry) []string {
	out := make([]string, len(es))
	for i, e := range es {
		out[i] = e.Label
	}
	return out
}

// TestEntriesOfACamp: the menu of a place is its people (those whose when holds), actions, search, exits and the menu, in that order.
func TestEntriesOfACamp(t *testing.T) {
	db := testgame.Load(t, map[string]string{"data/more.yaml": `
locations:
  hub:
    name: Hub
    encounters: {rate: "1", search: Look around, troops: [{troop: one}]}
    people: [{name: Guard, talk: [{say: Halt}]}]
    actions: [{label: Rest, do: [{heal: all}]}, {label: Secret, when: "flag.key", do: []}]
    exits: [{to: camp}, {to: vault, when: "item.key > 0"}]
`})
	st := testgame.State(db, 1)
	st.Location = "hub"
	es := field.Entries(db, st)
	var kinds []field.Kind
	for _, e := range es {
		kinds = append(kinds, e.Kind)
	}
	want := []field.Kind{field.Talk, field.Do, field.Search, field.Exit, field.Menu}
	if len(kinds) != len(want) {
		t.Fatalf("entries %v", labels(es))
	}
	for i := range want {
		if kinds[i] != want[i] {
			t.Fatalf("entry %d is kind %d, want %d: %v", i, kinds[i], want[i], labels(es))
		}
	}
	if !strings.Contains(es[0].Label, "Guard") || es[1].Label != "Rest" || es[2].Label != "Look around" || !strings.HasSuffix(es[3].Label, "Camp") {
		t.Errorf("labels %v", labels(es))
	}
	// a condition that starts to hold adds its entry
	st.Flags["key"] = true
	st.AddItem("key", 1)
	if got := field.Entries(db, st); len(got) != len(es)+2 {
		t.Errorf("flag.key and the key open two entries, got %v", labels(got))
	}
	// Commands
	if cmds := es[0].Commands(db, st); len(cmds) != 1 || cmds[0].Say.Text != "Halt" {
		t.Errorf("talk: %+v", cmds)
	}
	if cmds := es[1].Commands(db, st); len(cmds) != 1 || cmds[0].Heal != "all" {
		t.Errorf("action: %+v", cmds)
	}
	if cmds := es[2].Commands(db, st); len(cmds) != 1 || cmds[0].Battle != "one" {
		t.Errorf("search: %+v", cmds)
	}
	if cmds := es[3].Commands(db, st); len(cmds) != 1 || cmds[0].Move != "camp" {
		t.Errorf("exit: %+v", cmds)
	}
	if cmds := es[4].Commands(db, st); cmds != nil {
		t.Errorf("the menu runs no commands, got %+v", cmds)
	}
}

func TestNoMenuHidesTheMenuEntry(t *testing.T) {
	db := testgame.Load(t, map[string]string{"data/more.yaml": `
locations:
  cell:
    name: Cell
    no_menu: true
    exits: [{to: camp}]
`})
	st := testgame.State(db, 1)
	st.Location = "cell"
	es := field.Entries(db, st)
	if len(es) != 1 || es[0].Kind != field.Exit {
		t.Fatalf("%v", labels(es))
	}
}

func TestMarksAndNewStories(t *testing.T) {
	db := testgame.Load(t, map[string]string{"data/more.yaml": `
locations:
  hub:
    name: Hub
    people:
      - {name: Elder, new: "flag.fresh", mark: "flag.fresh", talk: [{say: Hm}]}
      - {name: Boy, talk: []}
`})
	st := testgame.State(db, 1)
	st.Location = "hub"
	es := field.Entries(db, st)
	if es[0].New || es[0].Marked || strings.HasSuffix(es[0].Label, "！") {
		t.Fatalf("nothing is new yet: %+v", es[0])
	}
	st.Flags["fresh"] = true
	es = field.Entries(db, st)
	if !es[0].New || !es[0].Marked || !strings.HasSuffix(es[0].Label, "！") {
		t.Errorf("a new story is marked and the label carries the mark: %+v", es[0])
	}
	if es[1].New || es[1].Marked {
		t.Errorf("another person is not marked: %+v", es[1])
	}
}

// TestArriveRunsEnterEventsOnce: the enter events of a place run on arrival, a once event only the first time,
// and each is counted with a Ran command first.
func TestArriveRunsEnterEventsOnce(t *testing.T) {
	db := testgame.Load(t, nil)
	st := testgame.State(db, 1)
	st.Location = "field"
	first := field.Arrive(db, st, false)
	if len(first) != 2 || first[0].Ran != "greet" || first[1].Say.Text != "The trail opens." {
		t.Fatalf("first arrival: %+v", first)
	}
	if !st.Done["greet"] {
		t.Error("a once event is marked done")
	}
	if again := field.Arrive(db, st, false); len(again) != 0 {
		t.Errorf("the once event runs again: %+v", again)
	}
}

// TestArriveRandomBattles: a place with an encounter rate may start a battle on arrival when asked, never when the party is calm,
// and the draw is from the state's random numbers (the same seed gives the same arrivals).
func TestArriveRandomBattles(t *testing.T) {
	db := testgame.Load(t, map[string]string{"data/more.yaml": `
locations:
  plain:
    name: Plain
    encounters: {rate: "0.5", troops: [{troop: one}, {troop: two, when: "false"}]}
`})
	count := func(seed uint64, calm, battles bool) int {
		st := testgame.State(db, seed)
		st.Location, st.Calm = "plain", calm
		n := 0
		for range 400 {
			for _, c := range field.Arrive(db, st, battles) {
				if c.Battle != "one" {
					t.Fatalf("a troop whose when is false was drawn: %+v", c)
				}
				n++
			}
		}
		return n
	}
	n := count(1, false, true)
	if n < 120 || n > 280 {
		t.Errorf("a rate of 0.5 gave %d battles in 400 arrivals", n)
	}
	if n != count(1, false, true) {
		t.Error("the same seed gave different arrivals")
	}
	if count(1, true, true) != 0 || count(1, false, false) != 0 {
		t.Error("a calm party, or an arrival without battles, meets none")
	}
}

func TestAutoEventsHoldOnceAndByLocation(t *testing.T) {
	db := testgame.Load(t, map[string]string{"data/more.yaml": `
events:
  tick: {trigger: auto, once: true, when: "flag.go", do: [{gold: 1}]}
  here: {trigger: auto, location: vault, do: [{gold: 2}]}
  empty: {trigger: auto, when: "flag.never", do: [{gold: 3}]}
`})
	st := testgame.State(db, 1)
	if got := field.Auto(db, st); got != nil {
		t.Fatalf("nothing holds yet: %+v", got)
	}
	st.Flags["go"] = true
	if got := field.Auto(db, st); len(got) != 1 {
		t.Fatalf("flag.go holds: %+v", got)
	}
	if got := field.Auto(db, st); got != nil {
		t.Fatalf("a once event runs once: %+v", got)
	}
	st.Location = "vault"
	got := field.Auto(db, st)
	if len(got) != 1 || got[0].Gold.String() != "2" {
		t.Fatalf("the vault event: %+v", got)
	}
}

func TestTalkOnEnterTakesTurns(t *testing.T) {
	db := testgame.Load(t, map[string]string{"data/more.yaml": `
locations:
  inn:
    name: Inn
    talk_on_enter: true
    people:
      - {name: A, talk: [{say: one}]}
      - {name: B, talk: [{say: two}]}
      - {name: C, when: "flag.hidden", talk: [{say: three}]}
`})
	st := testgame.State(db, 1)
	st.Location = "inn"
	var said []string
	for range 5 {
		for _, c := range field.Arrive(db, st, false) {
			said = append(said, c.Say.Text)
		}
	}
	if strings.Join(said, ",") != "one,two,one,two,one" {
		t.Errorf("people take turns on entering: %v", said)
	}
	if es := field.Entries(db, st); len(es) != 1 {
		t.Errorf("people who talk on entering are not in the menu: %v", labels(es))
	}
}

// TestSceneEntries covers the maps and stages: points, locked roads, things, spots and once-only ones.
func TestSceneEntries(t *testing.T) {
	db := testgame.LoadScenes(t, nil)
	st := testgame.State(db, 1)
	st.Location = "world"
	es := field.Entries(db, st)
	var haveTown, haveRuins bool
	for _, e := range es {
		switch {
		case strings.Contains(e.Label, "Town"):
			haveTown = e.Kind == field.Exit && e.Choice != nil
		case strings.Contains(e.Label, "Ruins"):
			haveRuins = true
		}
	}
	if !haveTown {
		t.Errorf("the town is a point of the map: %v", labels(es))
	}
	if !haveRuins {
		t.Errorf("a point whose when does not hold is still shown, locked: %v", labels(es))
	}
	for _, e := range es {
		if strings.Contains(e.Label, "Ruins") && (e.Exit == nil || e.Exit.To != "") {
			t.Errorf("a locked point leads nowhere: %+v", e.Exit)
		}
	}

	st.Location = "plaza"
	es = field.Entries(db, st)
	var keeper, board, bench *field.Entry
	for i := range es {
		e := &es[i]
		switch {
		case e.Person != nil && e.Person.ID == "keeper":
			keeper = e
		case e.Thing != nil && e.Thing.ID == "board":
			board = e
		case e.SpotDo == "bench":
			bench = e
		}
	}
	if keeper == nil || keeper.Spot == "" {
		t.Errorf("the keeper stands somewhere on the stage: %v", labels(es))
	}
	if board == nil || bench == nil {
		t.Fatalf("a thing and a spot with do: %v", labels(es))
	}
	if cmds := board.Commands(db, st); len(cmds) == 0 {
		t.Error("a thing runs its do")
	}
	if !st.Done["plaza/board"] && !anyDone(st.Done, "board") {
		t.Errorf("a once thing is marked done: %v", st.Done)
	}
	if st.Spot != board.Spot {
		t.Errorf("taking an entry puts the party at its spot, %q != %q", st.Spot, board.Spot)
	}
	for _, e := range field.Entries(db, st) {
		if e.Thing != nil && e.Thing.ID == "board" {
			t.Error("a done once thing is no longer offered")
		}
	}
	if cmds := bench.Commands(db, st); len(cmds) == 0 || st.Spot != "bench" {
		t.Errorf("a spot with do: %+v at %q", cmds, st.Spot)
	}
	if cmds := field.Step(db, st, "bench"); cmds != nil {
		t.Error("a once spot gives nothing the second time")
	}
	if cmds := field.Step(db, st, "nowhere"); cmds != nil {
		t.Error("an unknown spot gives nothing")
	}
	if cmds := field.Step(db, st, "gate"); cmds != nil || st.Spot != "gate" {
		t.Errorf("a spot without do just stands there: %+v at %q", cmds, st.Spot)
	}
	st.Location = "camp"
	if cmds := field.Step(db, st, "gate"); cmds != nil {
		t.Error("a place without a stage has no spots")
	}
}

func anyDone(m map[string]bool, part string) bool {
	for k, v := range m {
		if v && strings.Contains(k, part) {
			return true
		}
	}
	return false
}
