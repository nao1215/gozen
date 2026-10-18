package text

import "testing"

func TestAGameChangesTheWords(t *testing.T) {
	defer Use(nil)
	if T("にげられない！") != "にげられない！" {
		t.Fatal("the engine's own word should be kept")
	}
	Use(map[string]string{"にげられない！": "Can't run away!"})
	if got := T("にげられない！"); got != "Can't run away!" {
		t.Fatalf("got %q", got)
	}
	if got := T("うまく にげきれた！"); got != "うまく にげきれた！" {
		t.Fatalf("a word the game does not change: %q", got)
	}
}

func TestUseCopiesCallerMap(t *testing.T) {
	defer Use(nil)
	words := map[string]string{"trail": "path"}
	Use(words)
	words["trail"] = "changed"
	if got := T("trail"); got != "path" {
		t.Fatalf("caller mutation changed translation: %s", got)
	}
}
