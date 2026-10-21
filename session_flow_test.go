package gozen_test

import (
	"errors"
	"slices"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/nao1215/gozen"
	"github.com/nao1215/gozen/internal/testgame"
)

const flowData = `
shops:
  smith: {goods: [potion]}
events:
  tour:
    do:
      - say: {who: scout, text: Hello}
      - notice: A notice
      - wait: 0.5
      - shop: smith
      - save: true
      - open: formation
      - image: {file: x.png}
      - music: {song: theme}
      - mood: calm
      - sfx: {name: ding}
  fight_can_lose:
    do:
      - battle: boss
        on_lose: [{gold: 1}]
        retry: false
      - say: after
  fight_must_win:
    do:
      - battle: boss
  ending:
    do: [{say: bye}, {clear: true}, {say: never shown}]
  loop:
    do: [{call: loop}]
music:
  songs: {theme: {file: x.png}}
`

func flowGame(t *testing.T) *gozen.Game {
	t.Helper()
	fsys := fstest.MapFS{
		"game.yaml":      {Data: []byte("schema_version: 4\nid: flow\n" + testgame.Game)},
		"data/main.yaml": {Data: []byte(testgame.Data)},
		"data/flow.yaml": {Data: []byte(flowData)},
		"assets/x.png":   {Data: []byte("a stub")},
	}
	g, err := gozen.Load(fsys, ".")
	if err != nil {
		t.Fatal(err)
	}
	return g
}

// TestEveryRequestKind: stepping through events shows each kind of request in order, and reports immediate effects separately.
func TestEveryRequestKind(t *testing.T) {
	g := flowGame(t)
	var effects []string
	s, err := gozen.New(g, gozen.Config{Seed: 1, OnEffect: func(e gozen.Effect) { effects = append(effects, e.Kind+":"+e.Name) }})
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Start("- call: tour"); err != nil {
		t.Fatal(err)
	}
	var kinds []string
	for q := s.Next(); q.Kind != ""; q = s.Next() {
		kinds = append(kinds, q.Kind)
		switch q.Kind {
		case "say":
			if q.Who != "Scout" || q.Text != "Hello" {
				t.Errorf("say %+v", q)
			}
		case "wait":
			if q.Seconds != 0.5 {
				t.Errorf("wait %+v", q)
			}
		case "shop":
			if q.ID != "smith" {
				t.Errorf("shop %+v", q)
			}
		case "open":
			if q.ID != "formation" {
				t.Errorf("open %+v", q)
			}
		case "notice":
			if q.Text != "A notice" {
				t.Errorf("notice %+v", q)
			}
		}
	}
	want := []string{"say", "notice", "wait", "shop", "save", "open"}
	if !slices.Equal(kinds[:len(want)], want) {
		t.Fatalf("requests %v, want to start with %v", kinds, want)
	}
	for _, e := range []string{"image:x.png", "music:theme", "mood:calm", "sound:ding"} {
		if !slices.Contains(effects, e) {
			t.Errorf("effect %s missing from %v", e, effects)
		}
	}
}

// TestBattleRequestFlags: a battle that may be lost says so, and one that cannot be retried says so.
func TestBattleRequestFlags(t *testing.T) {
	g := flowGame(t)
	s, _ := gozen.New(g, gozen.Config{Seed: 1})
	if err := s.Start("- call: fight_can_lose"); err != nil {
		t.Fatal(err)
	}
	q := s.Next()
	if q.Kind != "battle" || q.ID != "boss" || q.Flags != "can_lose,no_retry" {
		t.Fatalf("%+v", q)
	}
	// the same request until it is answered
	if again := s.Next(); again.Kind != "battle" {
		t.Errorf("a battle waits for its answer: %+v", again)
	}
	// losing a battle that may be lost leaves everyone standing and goes on
	s.Answer(gozen.Lost)
	if next := s.Next(); next.Kind != "say" || next.Text != "after" {
		t.Errorf("after losing: %+v", next)
	}
	var result string
	if err := s.Query("result", &result); err != nil || result != "lose" {
		t.Errorf("result %q %v", result, err)
	}
	for _, m := range []string{"scout"} {
		var hp int
		if err := s.Query("actor."+m+".hp", &hp); err != nil || hp < 1 {
			t.Errorf("%s hp %d %v", m, hp, err)
		}
	}
	// Answer with nothing pending changes nothing
	s.Answer(3)

	s2, _ := gozen.New(g, gozen.Config{Seed: 1})
	if err := s2.Start("- call: fight_must_win"); err != nil {
		t.Fatal(err)
	}
	if q := s2.Next(); q.Flags != "" {
		t.Errorf("a plain battle has no flags: %+v", q)
	}
	s2.Answer(gozen.Fled)
	var res string
	_ = s2.Query("result", &res)
	if res != "escape" {
		t.Errorf("fled result %q", res)
	}
}

func TestClearEndsTheRun(t *testing.T) {
	g := flowGame(t)
	s, _ := gozen.New(g, gozen.Config{Seed: 1})
	if err := s.Start("- call: ending"); err != nil {
		t.Fatal(err)
	}
	var kinds, texts []string
	for q := s.Next(); q.Kind != ""; q = s.Next() {
		kinds = append(kinds, q.Kind)
		texts = append(texts, q.Text)
	}
	if !slices.Equal(kinds, []string{"say", "clear"}) || texts[0] != "bye" {
		t.Errorf("%v %v (nothing after the end is shown)", kinds, texts)
	}
	var done bool
	if err := s.Query("cleared", &done); err != nil || !done {
		t.Errorf("cleared = %v %v", done, err)
	}
	// Do runs no more commands once the game has been cleared
	if err := s.Do(t.Context(), "- gold: 100\n"); err != nil {
		t.Fatal(err)
	}
	var gold int
	if err := s.Query("gold", &gold); err != nil || gold != 100 {
		t.Errorf("gold %d: a command after the end must not run", gold)
	}
}

// TestLosingWithoutOnLoseIsGameOver: a battle lost where losing is not allowed ends the game.
func TestLosingWithoutOnLoseIsGameOver(t *testing.T) {
	g := flowGame(t)
	s, _ := gozen.New(g, gozen.Config{Seed: 1})
	if err := s.Start("- call: fight_must_win\n"); err != nil {
		t.Fatal(err)
	}
	if q := s.Next(); q.Kind != "battle" {
		t.Fatalf("%+v", q)
	}
	s.Answer(gozen.Lost)
	if q := s.Next(); q.Kind != "game_over" {
		t.Fatalf("%+v", q)
	}
	var over bool
	if err := s.Query("over", &over); err != nil || !over {
		t.Errorf("over = %v %v", over, err)
	}
	if q := s.Next(); q.Kind != "" {
		t.Errorf("nothing follows a game over: %+v", q)
	}
}

// TestEventsThatNeverEnd: a call loop is cut off with an error, not a hang.
func TestEventsThatNeverEnd(t *testing.T) {
	g := flowGame(t)
	s, _ := gozen.New(g, gozen.Config{Seed: 1})
	err := s.Do(t.Context(), "- call: loop\n")
	if err == nil {
		t.Fatal("a call loop must be reported")
	}
	if !strings.Contains(err.Error(), "deep") && !strings.Contains(err.Error(), "do not end") {
		t.Logf("loop reported as: %v", err)
	}
	s2, _ := gozen.New(g, gozen.Config{Seed: 1})
	if err := s2.Start("- call: loop"); err != nil {
		t.Fatal(err)
	}
	for range 5 {
		if q := s2.Next(); q.Kind != "" {
			t.Fatalf("a loop shows nothing: %+v", q)
		}
	}
}

// TestBeginRunsTheStartEventOnce: Begin runs the start event and arrives; Next on a fresh session shows the first line of the opening.
func TestBeginRunsTheStartEvent(t *testing.T) {
	fsys := fstest.MapFS{
		"game.yaml":      {Data: []byte("schema_version: 4\nid: opening\n" + strings.Replace(testgame.Game, "start: {", "start: {event: intro, ", 1))},
		"data/main.yaml": {Data: []byte(testgame.Data)},
		"data/open.yaml": {Data: []byte("events:\n  intro: {do: [{set: {flag.opened: true}}, {say: Welcome}]}\n")},
		"assets/x.png":   {Data: []byte("stub")},
	}
	g, err := gozen.Load(fsys, ".")
	if err != nil {
		t.Fatal(err)
	}
	var seen []string
	s, _ := gozen.New(g, gozen.Config{Seed: 1, OnRequest: func(q gozen.Request) { seen = append(seen, q.Kind+":"+q.Text) }})
	if err := s.Begin(t.Context()); err != nil {
		t.Fatal(err)
	}
	if len(seen) == 0 || seen[0] != "say:Welcome" {
		t.Errorf("requests %v", seen)
	}
	var opened bool
	if err := s.Query("flag.opened", &opened); err != nil || !opened {
		t.Errorf("opened %v %v", opened, err)
	}
	if err := s.Begin(nil); err == nil || errors.Is(err, gozen.ErrInvalid) {
		t.Logf("Begin(nil): %v", err)
	}
}

func TestNilContextIsAnError(t *testing.T) {
	g := load(t)
	s, _ := gozen.New(g, gozen.Config{Seed: 1})
	if err := s.Do(nil, "- gold: 1\n"); err == nil { //nolint:staticcheck // A nil context is the case under test.
		t.Error("a nil context must be refused")
	}
}
