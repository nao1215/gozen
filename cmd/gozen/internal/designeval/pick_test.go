package designeval

import (
	"testing"

	"github.com/nao1215/gozen/internal/event"
	data "github.com/nao1215/gozen/internal/layer2/gamedata"
	"github.com/nao1215/gozen/internal/testgame"
	"github.com/nao1215/gozen/internal/world"
)

func pickBot() *bot {
	return &bot{c: &Config{}, used: map[string]int{}, asked: map[string]int{}}
}

func TestPickIDsSelectsByWordsIDsAndRotation(t *testing.T) {
	opts := []string{"Take the offer", "Decline", "Ask a question", "Leave"}
	ids := []string{"yes", "no", "ask", ""}
	for _, tc := range []struct {
		name  string
		setup func(b *bot)
		words []string
		rot   []string
		want  int
	}{
		{"first when nothing is wanted", func(*bot) {}, nil, nil, 0},
		{"a word selects the option that contains it", func(*bot) {}, []string{"question"}, nil, 2},
		{"an @id selects the option with that id", func(*bot) {}, []string{"@ask"}, nil, 2},
		{"an @id that matches no option is ignored", func(*bot) {}, []string{"@ghost"}, nil, 0},
		{"the first word that matches wins", func(*bot) {}, []string{"nothing", "Leave", "question"}, nil, 3},
		{"the persona prefers", func(b *bot) { b.p.Prefer = []string{"Leave"} }, nil, nil, 3},
		{"the game prefers after the persona", func(b *bot) { b.c.Game.Prefer = []string{"question"} }, nil, nil, 2},
		{"the persona avoids", func(b *bot) { b.p.Avoid = []string{"Take"} }, nil, nil, 1},
		{"never words are avoided", func(b *bot) { b.c.Game.Never = []string{"Take", "Decline"} }, nil, nil, 2},
		{"opt-in words are avoided unless the context names them", func(b *bot) { b.c.Game.OptIn = []string{"Take"} }, nil, nil, 1},
		{"a context word that names an opt-in option allows it", func(b *bot) { b.c.Game.OptIn = []string{"Take"} }, []string{"Take"}, nil, 0},
		{"avoiding an id", func(b *bot) { b.p.Avoid = []string{"@yes"} }, nil, nil, 1},
		{"everything avoided picks the last option", func(b *bot) { b.p.Avoid = []string{"a", "e", "o"} }, nil, nil, 3},
		{"the accepted mark narrows the candidates", func(b *bot) { b.accept = "Ask" }, nil, nil, 2},
		{"an accepted mark nobody has changes nothing", func(b *bot) { b.accept = "zzz" }, nil, nil, 0},
		{"rotation takes the least used", func(b *bot) { b.used["Leave"] = 3; b.used["Decline"] = 1 }, nil, []string{"Leave", "Decline"}, 1},
		{"rotation takes the earliest option among the equally used", func(*bot) {}, nil, []string{"Leave", "Decline"}, 1},
		{"rotation by id", func(b *bot) { b.used["@yes"] = 5 }, nil, []string{"@yes", "@no"}, 1},
		{"context words narrow before rotation", func(*bot) {}, []string{"Ask"}, []string{"Leave", "question"}, 2},
		{"rotation that matches nothing falls back to the words", func(*bot) {}, []string{"Leave"}, []string{"zzz"}, 3},
	} {
		t.Run(tc.name, func(t *testing.T) {
			b := pickBot()
			tc.setup(b)
			if got := b.pickIDs(opts, ids, tc.words, tc.rot); got != tc.want {
				t.Fatalf("picked %d, want %d", got, tc.want)
			}
		})
	}
	b := pickBot()
	if got := b.pickIDs(opts, ids[:1], []string{"@no"}, nil); got != 0 {
		t.Fatalf("an id the options do not carry selected %d", got)
	}
	b.pickIDs(opts, ids, nil, []string{"Leave"})
	b.pickIDs(opts, ids, nil, []string{"Leave"})
	if b.used["Leave"] != 2 {
		t.Fatalf("rotation use count: %v", b.used)
	}
}

func TestChooseBreaksLoopsAndHandlesNoOptions(t *testing.T) {
	b := pickBot()
	b.ctx = []string{"Decline"}
	if b.choose(nil, event.Choice{}) != 0 {
		t.Fatal("a choice without options must answer 0")
	}
	q := event.Choice{Prompt: "Door", Options: []string{"Open", "Decline", "Back"}}
	for range 2 {
		if got := b.choose(nil, q); got != 1 {
			t.Fatalf("the first two asks follow the words, got %d", got)
		}
	}
	if got := b.choose(nil, q); got != 2 {
		t.Fatalf("the third ask of the same question must take the last option to leave the loop, got %d", got)
	}
	var logged int
	b.c.Debug = func(string, ...any) { logged++ }
	b.asked = map[string]int{}
	b.p.ID = "p"
	b.db = testgame.Load(t, nil)
	b.st = testgame.State(b.db, 1)
	b.choose(nil, q)
	if logged != 1 {
		t.Fatalf("the choice was logged %d times", logged)
	}
}

func TestHasAnyIgnoresEmptyWords(t *testing.T) {
	for _, tc := range []struct {
		s     string
		words []string
		want  bool
	}{
		{"Open the gate", []string{"gate"}, true},
		{"Open the gate", []string{"", "door"}, false},
		{"Open the gate", []string{""}, false},
		{"Open the gate", nil, false},
		{"", []string{"a"}, false},
		{"門を開ける", []string{"開"}, true},
	} {
		if got := hasAny(tc.s, tc.words); got != tc.want {
			t.Errorf("hasAny(%q, %q) = %v", tc.s, tc.words, got)
		}
	}
}

func TestEntrySelectorsRoundTrip(t *testing.T) {
	act := &data.Action{ID: "rest", Label: "Rest"}
	person := &data.Person{ID: "host", Name: "Host"}
	a := world.Entry{Label: "Rest here", Action: act}
	p := world.Entry{Label: "Host", Person: person}
	plain := world.Entry{Label: "Plain"}
	for _, tc := range []struct {
		e    world.Entry
		want string
	}{{a, "@rest"}, {p, "@host"}, {plain, "Plain"}} {
		if got := entrySelector(tc.e); got != tc.want {
			t.Errorf("selector of %q = %q", tc.e.Label, got)
		}
		if !entryMatches(tc.e, entrySelector(tc.e)) {
			t.Errorf("an entry does not match its own selector %q", tc.want)
		}
	}
	if entryMatches(a, "@host") || entryMatches(p, "@rest") || entryMatches(plain, "@rest") {
		t.Error("an id selector matched the wrong entry")
	}
	if !entryMatches(a, "Rest") || entryMatches(a, "Sleep") {
		t.Error("a word selector must be a substring of the label")
	}
}

func TestNoRetryTroopsFindsNestedBattles(t *testing.T) {
	db := testgame.Load(t, map[string]string{"data/e.yaml": `events:
  deep:
    do:
      - if: "true"
        then:
          - {battle: one, retry: false}
        else:
          - {battle: two}
      - choice:
          - {label: A, do: [{battle: boss, retry: false, on_win: [{battle: two, retry: false}]}]}
`})
	got := noRetryTroops(db)
	for _, id := range []string{"one", "boss", "two"} {
		if !got[id] {
			t.Errorf("%s: a battle that says retry: false must not be retried: %v", id, got)
		}
	}
	if got["unknown"] || len(got) != 3 {
		t.Errorf("unexpected: %v", got)
	}
}
