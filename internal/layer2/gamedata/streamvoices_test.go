package gamedata_test

import (
	"reflect"
	"strings"
	"testing"

	"github.com/nao1215/gozen/internal/react"
	"github.com/nao1215/gozen/internal/testgame"
)

func TestStreamVoiceLines(t *testing.T) {
	files := streamGame()
	files["data/streams.yaml"] = "voices: voice\n" + strings.Replace(files["data/streams.yaml"], "actor: ann", "actor: ann\n    voice_fallback: Thank you.", 1)
	files["streams/ann/1.yaml"] = strings.Replace(files["streams/ann/1.yaml"], "[sprout, 1]", "[sprout, 0]", 1)
	db := testgame.Load(t, files)
	book, err := react.Parse([]byte("people:\n  - id: ann\n    dodge:\n      unknown: [\"Thanks, {from}: {text}\", \"Mood {mood}\"]\n"))
	if err != nil {
		t.Fatal(err)
	}
	db.ReactionBooks["ann"] = book
	db.Game.Languages = []string{"en", "ja"}
	db.Words["ja"] = map[string]string{
		"sprout":                 "Fern",
		"{from} picked beans.":   "Beans for {from}.",
		"For seeds":              "For flowers",
		"Thanks, {from}: {text}": "{from}, thanks for {text}",
	}
	lines := db.StreamVoiceLines()
	if !reflect.DeepEqual(lines, db.StreamVoiceLines()) {
		t.Fatal("inventory is not deterministic")
	}
	known := map[string]bool{}
	pending := map[string]bool{}
	for _, l := range lines {
		if l.Channel != "ann" {
			continue
		}
		if l.File != db.Streams.Voice(l.Channel, l.Text) {
			t.Fatalf("wrong recording path: %+v", l)
		}
		if len(l.Fields) == 0 {
			known[l.Text] = true
		} else {
			pending[l.Text] = true
		}
	}
	for _, want := range []string{"sprout picked beans.", "Thanks, sprout: For seeds", "Thanks, leaf: For seeds", "Thank you.", "Beans for Fern.", "Fern, thanks for For flowers"} {
		if !known[want] {
			t.Errorf("missing candidate %q", want)
		}
	}
	for _, want := range []string{"Welcome, {from}.", "{days} days in a row, {from}!", "Mood {mood}", "Beans for {from}."} {
		if !pending[want] {
			t.Errorf("missing runtime template %q", want)
		}
	}
	if known["leaf picked beans."] || known["Welcome, sprout."] {
		t.Fatal("invented an NPC answer or a player greeting")
	}
}
