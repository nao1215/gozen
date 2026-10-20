package text

import (
	"testing"

	"github.com/nao1215/gozen/internal/text/catalog"
)

// reset puts the process words back to the state before any Use.
func reset() {
	mu.Lock()
	active = nil
	mu.Unlock()
}

func TestAGameChangesTheWords(t *testing.T) {
	defer reset()
	if T("にげられない！") != "にげられない！" {
		t.Fatal("before Use the key itself should be shown")
	}
	if err := Use(Japanese, map[string]string{"にげられない！": "No way out!"}); err != nil {
		t.Fatal(err)
	}
	if got := T("にげられない！"); got != "No way out!" {
		t.Fatalf("got %q", got)
	}
	if got := T("うまく にげきれた！"); got != "うまく にげきれた！" {
		t.Fatalf("a Japanese word the game does not change: %q", got)
	}
}

func TestLanguagesAndTheGamesWordsOnTop(t *testing.T) {
	defer reset()
	en, err := catalog.Words(English)
	if err != nil {
		t.Fatal(err)
	}
	for _, language := range []string{"", English} {
		if err := Use(language, map[string]string{"にげられない！": "Stuck!"}); err != nil {
			t.Fatal(err)
		}
		if got := T("うまく にげきれた！"); got != en["うまく にげきれた！"] || got == "" {
			t.Fatalf("%q: the built-in English word: %q", language, got)
		}
		if got := T("にげられない！"); got != "Stuck!" {
			t.Fatalf("%q: the game's word should win: %q", language, got)
		}
	}
	if got := T("not a key"); got != "not a key" {
		t.Fatalf("an unknown key is shown as is: %q", got)
	}
	if err := Use("fr", nil); err == nil {
		t.Fatal("an unknown language was accepted")
	}
	if got := T("にげられない！"); got != "Stuck!" {
		t.Fatalf("a failed Use changed the words: %q", got)
	}
	if err := Use(Japanese, nil); err != nil {
		t.Fatal(err)
	}
	if got := T("うまく にげきれた！"); got != "うまく にげきれた！" {
		t.Fatalf("Japanese shows the key: %q", got)
	}
}

func TestUseCopiesCallerMap(t *testing.T) {
	defer reset()
	words := map[string]string{"trail": "path"}
	if err := Use(English, words); err != nil {
		t.Fatal(err)
	}
	words["trail"] = "changed"
	if got := T("trail"); got != "path" {
		t.Fatalf("caller mutation changed translation: %s", got)
	}
}
